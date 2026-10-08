package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/control"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
	"github.com/calebhabesh/orbit/internal/workspace"
)

func TestP11IntegrityScanAndQuarantineAffectedVersions(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(disposable, "orbit")
	build := exec.Command("go", "build", "-o", binary, "./cmd/orbit")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}

	state := filepath.Join(disposable, "state")
	root := filepath.Join(disposable, "root")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}

	folder := strings.Repeat("f", 64)

	// 1. Initialize and register folder
	for _, args := range [][]string{
		{"init", "--state", state},
		{"register", "--state", state, "--folder", folder, "--root", root},
	} {
		if output, err := exec.Command(binary, append([]string{"engine"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
	}

	// 2. Create two files sharing the same chunk
	sharedContent := []byte("shared content between fileA and fileB for p11 test")
	sharedDigest := sha256.Sum256(sharedContent)
	if err := os.WriteFile(filepath.Join(root, "fileA.txt"), sharedContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fileB.txt"), sharedContent, 0o600); err != nil {
		t.Fatal(err)
	}

	// Scan
	scanOut, err := exec.Command(binary, "engine", "scan", "--state", state, "--folder", folder).CombinedOutput()
	if err != nil {
		t.Fatalf("scan: %v\n%s", err, scanOut)
	}

	// 3. Check integrity on clean state
	checkCleanJSON, err := exec.Command(binary, "engine", "storage", "check", "--state", state, "--folder", folder, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("check clean: %v\n%s", err, checkCleanJSON)
	}
	var cleanRes control.StorageCheckResult
	if err := json.Unmarshal(checkCleanJSON, &cleanRes); err != nil {
		t.Fatalf("unmarshal check clean: %v\n%s", err, checkCleanJSON)
	}
	if cleanRes.TotalChunksChecked != 1 || cleanRes.CleanChunks != 1 || len(cleanRes.CorruptChunks) != 0 {
		t.Fatalf("unexpected clean check result: %+v", cleanRes)
	}

	// 4. Inject corruption into the chunk file on disk
	chunkPath := filepath.Join(state, "objects", "sha256", fmt.Sprintf("%02x", sharedDigest[0]), fmt.Sprintf("%x", sharedDigest[1:]))
	if err := os.WriteFile(chunkPath, []byte("injected bit rot corruption!"), 0o600); err != nil {
		t.Fatal(err)
	}

	// 5. Run storage check with auto-quarantine
	checkCorruptJSON, err := exec.Command(binary, "engine", "storage", "check", "--state", state, "--folder", folder, "--quarantine", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("check corrupt: %v\n%s", err, checkCorruptJSON)
	}
	var corruptRes control.StorageCheckResult
	if err := json.Unmarshal(checkCorruptJSON, &corruptRes); err != nil {
		t.Fatalf("unmarshal check corrupt: %v\n%s", err, checkCorruptJSON)
	}
	if len(corruptRes.CorruptChunks) != 1 {
		t.Fatalf("expected 1 corrupt chunk, got %d", len(corruptRes.CorruptChunks))
	}
	cDetail := corruptRes.CorruptChunks[0]
	if cDetail.Digest != sharedDigest {
		t.Fatalf("corrupt chunk digest mismatch: got %x want %x", cDetail.Digest, sharedDigest)
	}
	if len(cDetail.AffectedVersions) != 2 {
		t.Fatalf("expected 2 affected versions, got %d: %v", len(cDetail.AffectedVersions), cDetail.AffectedVersions)
	}
	if len(cDetail.AffectedPaths) != 2 {
		t.Fatalf("expected 2 affected paths (fileA.txt, fileB.txt), got %d: %v", len(cDetail.AffectedPaths), cDetail.AffectedPaths)
	}

	// Verify the corrupt chunk was moved to quarantine
	if _, err := os.Stat(chunkPath); !os.IsNotExist(err) {
		t.Fatalf("corrupt chunk file should be unlinked/moved from objects, but still exists: %s", chunkPath)
	}
	quarantineEntries, err := os.ReadDir(filepath.Join(state, "quarantine"))
	if err != nil || len(quarantineEntries) != 1 {
		t.Fatalf("expected 1 file in quarantine dir, got %d (err: %v)", len(quarantineEntries), err)
	}

	// 6. Acceptance Check: No success receipt is newly issued for known corrupt content
	ctx := context.Background()
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, vid := range cDetail.AffectedVersions {
		canReceipt, err := db.CanIssueDurableReceipt(ctx, vid)
		if err != nil {
			t.Fatalf("CanIssueDurableReceipt error: %v", err)
		}
		if canReceipt {
			t.Fatalf("CanIssueDurableReceipt returned true for corrupt version %v (violates S15 / I09 / I18)", vid)
		}
		avail, err := db.ContentAvailability(ctx, vid)
		if err != nil {
			t.Fatalf("ContentAvailability error: %v", err)
		}
		if avail != repository.ContentCorrupt {
			t.Fatalf("expected ContentCorrupt, got %v", avail)
		}
	}
}

func TestP11PeerAssistedRepairAndInvariantPreservation(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(disposable, "orbit")
	build := exec.Command("go", "build", "-o", binary, "./cmd/orbit")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}

	stateA := filepath.Join(disposable, "state-a")
	rootA := filepath.Join(disposable, "root-a")
	stateB := filepath.Join(disposable, "state-b")
	rootB := filepath.Join(disposable, "root-b")
	for _, dir := range []string{stateA, rootA, stateB, rootB} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	folder := strings.Repeat("e", 64)

	// Init both nodes
	for _, cmd := range [][]string{
		{binary, "init", "--state", stateA},
		{binary, "engine", "register", "--state", stateA, "--folder", folder, "--root", rootA},
		{binary, "init", "--state", stateB},
		{binary, "engine", "register", "--state", stateB, "--folder", folder, "--root", rootB},
	} {
		if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", cmd, err, out)
		}
	}

	// Pair approval
	outA, err := exec.Command(binary, "engine", "identity", "--state", stateA, "--certificate").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	devA, pinA, certA := parseIdentityOutput(t, outA)

	outB, err := exec.Command(binary, "engine", "identity", "--state", stateB, "--certificate").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	devB, pinB, certB := parseIdentityOutput(t, outB)

	certPathA := filepath.Join(disposable, "cert-a.pem")
	if err := os.WriteFile(certPathA, certA, 0o600); err != nil {
		t.Fatal(err)
	}
	certPathB := filepath.Join(disposable, "cert-b.pem")
	if err := os.WriteFile(certPathB, certB, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, cmd := range [][]string{
		{binary, "engine", "pair-approve", "--state", stateA, "--folder", folder, "--peer-device", devB, "--peer-key-pin", pinB},
		{binary, "engine", "pair-approve", "--state", stateB, "--folder", folder, "--peer-device", devA, "--peer-key-pin", pinA},
	} {
		if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", cmd, err, out)
		}
	}

	// Node A writes two files sharing the same chunk
	sharedContent := []byte("payload shared across files for authorized repair")
	sharedDigest := sha256.Sum256(sharedContent)
	if err := os.WriteFile(filepath.Join(rootA, "doc1.txt"), sharedContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootA, "doc2.txt"), sharedContent, 0o600); err != nil {
		t.Fatal(err)
	}

	if out, err := exec.Command(binary, "engine", "scan", "--state", stateA, "--folder", folder).CombinedOutput(); err != nil {
		t.Fatalf("scan A: %v\n%s", err, out)
	}

	// Node A serves over TLS
	serveCmdA := exec.Command(binary, "engine", "serve", "--state", stateA, "--peer-listen", "127.0.0.1:0")
	stdoutA, err := serveCmdA.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	serveCmdA.Stderr = os.Stderr
	if err := serveCmdA.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = serveCmdA.Process.Kill() }()
	urlA := readListenerURL(t, stdoutA)

	// Node B syncs from Node A
	syncOut, err := exec.Command(binary, "engine", "sync", "--state", stateB, "--folder", folder,
		"--peer-url", urlA, "--peer-device", devA, "--peer-certificate", certPathA).CombinedOutput()
	if err != nil {
		t.Fatalf("initial sync B: %v\n%s", err, syncOut)
	}

	// Verify doc1.txt and doc2.txt exist on B
	if b, err := os.ReadFile(filepath.Join(rootB, "doc1.txt")); err != nil || !bytes.Equal(b, sharedContent) {
		t.Fatalf("doc1 on B mismatch or missing: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(rootB, "doc2.txt")); err != nil || !bytes.Equal(b, sharedContent) {
		t.Fatalf("doc2 on B mismatch or missing: %v", err)
	}

	// Inspect version IDs on B
	ctx := context.Background()
	dbB, err := repository.Open(ctx, stateB)
	if err != nil {
		t.Fatal(err)
	}
	defer dbB.Close()

	var folderID history.ID
	rawFolder, _ := hex.DecodeString(folder)
	copy(folderID[:], rawFolder)

	heads, err := dbB.Heads(ctx, folderID, "doc1.txt")
	if err != nil || len(heads) != 1 {
		t.Fatalf("heads doc1: %v (%v)", heads, err)
	}
	vTarget := heads[0].ID

	// Inject corruption on Node B
	chunkPathB := filepath.Join(stateB, "objects", "sha256", fmt.Sprintf("%02x", sharedDigest[0]), fmt.Sprintf("%x", sharedDigest[1:]))
	if err := os.WriteFile(chunkPathB, []byte("corrupted payload on B!"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Node B runs storage check with auto-quarantine
	if err := dbB.Close(); err != nil {
		t.Fatal(err)
	}
	checkOut, err := exec.Command(binary, "engine", "storage", "check", "--state", stateB, "--folder", folder, "--quarantine", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("check on B: %v\n%s", err, checkOut)
	}
	var corruptRes control.StorageCheckResult
	_ = json.Unmarshal(checkOut, &corruptRes)
	if len(corruptRes.CorruptChunks) != 1 {
		t.Fatalf("expected 1 corrupt chunk on B, got %d", len(corruptRes.CorruptChunks))
	}

	// Acceptance check: Node B cannot issue receipt while corrupt
	dbB, err = repository.Open(ctx, stateB)
	if err != nil {
		t.Fatal(err)
	}
	canReceiptBefore, _ := dbB.CanIssueDurableReceipt(ctx, vTarget)
	if canReceiptBefore {
		t.Fatal("expected CanIssueDurableReceipt=false for corrupt chunk on B")
	}

	// Acceptance check: Node B repairs it correctly via authorized chunk transfer from Node A
	if err := dbB.Close(); err != nil {
		t.Fatal(err)
	}
	versionArg := fmt.Sprintf("%x:%d", vTarget.Author, vTarget.Counter)
	repairKey := "repair-p11-test-key-1"
	repairOut, err := exec.Command(binary, "engine", "storage", "repair",
		"--state", stateB,
		"--folder", folder,
		"--version", versionArg,
		"--peer-url", urlA,
		"--peer-device", devA,
		"--peer-certificate", certPathA,
		"--idempotency-key", repairKey,
		"--json",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("repair failed: %v\n%s", err, repairOut)
	}

	var repairRes control.StorageRepairResult
	if err := json.Unmarshal(repairOut, &repairRes); err != nil {
		t.Fatalf("unmarshal repair result: %v\n%s", err, repairOut)
	}
	if repairRes.Status != "repaired" || repairRes.RepairedChunks != 1 || repairRes.Replay {
		t.Fatalf("unexpected repair result: %+v", repairRes)
	}

	// Invariant Check (I06, I10): Causal version identity is strictly unchanged
	if repairRes.VersionID != vTarget {
		t.Fatalf("causal version identity changed! got %v want %v", repairRes.VersionID, vTarget)
	}

	// Shared chunk repair restored BOTH doc1.txt and doc2.txt to ready
	dbB, err = repository.Open(ctx, stateB)
	if err != nil {
		t.Fatal(err)
	}
	defer dbB.Close()
	headsDoc2, _ := dbB.Heads(ctx, folderID, "doc2.txt")
	if len(headsDoc2) == 1 {
		availDoc2, _ := dbB.ContentAvailability(ctx, headsDoc2[0].ID)
		if availDoc2 != repository.ContentReady {
			t.Fatalf("shared chunk repair failed to restore doc2.txt: avail=%v", availDoc2)
		}
	}

	// Restored chunk on B matches original bytes
	restoredBytes, err := os.ReadFile(chunkPathB)
	if err != nil || !bytes.Equal(restoredBytes, sharedContent) {
		t.Fatalf("restored chunk on B mismatch: %v", err)
	}

	// Can issue receipt again
	canReceiptAfter, _ := dbB.CanIssueDurableReceipt(ctx, vTarget)
	if !canReceiptAfter {
		t.Fatal("expected CanIssueDurableReceipt=true after successful repair")
	}

	// Idempotent replay
	if err := dbB.Close(); err != nil {
		t.Fatal(err)
	}
	replayOut, err := exec.Command(binary, "engine", "storage", "repair",
		"--state", stateB,
		"--folder", folder,
		"--version", versionArg,
		"--peer-url", urlA,
		"--peer-device", devA,
		"--peer-certificate", certPathA,
		"--idempotency-key", repairKey,
		"--json",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("replay failed: %v\n%s", err, replayOut)
	}
	var replayRes control.StorageRepairResult
	_ = json.Unmarshal(replayOut, &replayRes)
	if !replayRes.Replay {
		t.Fatalf("expected replay=true, got %+v", replayRes)
	}
}

func TestP11NoRemainingCopyYieldsExplicitUnavailable(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	state := filepath.Join(disposable, "state")
	root := filepath.Join(disposable, "root")
	_ = os.Mkdir(state, 0o700)
	_ = os.Mkdir(root, 0o700)

	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder, author := repositoryID('F'), repositoryID('A')
	_ = db.EnsureFolder(ctx, folder, author, 1)
	m := protocol.Membership{
		Folder:   folder,
		Revision: 1,
		Active:   []protocol.ActiveMember{{Device: author, KeyPin: history.Digest(repositoryID('K'))}},
	}
	if _, err := db.ApproveMembership(ctx, m); err != nil {
		t.Fatal(err)
	}

	lostContent := []byte("unique chunk lost everywhere")
	lostDigest := sha256.Sum256(lostContent)
	chunk := history.Chunk{Digest: lostDigest, Length: uint64(len(lostContent))}
	manifest := &history.Manifest{Size: chunk.Length, Chunks: []history.Chunk{chunk}, Digest: lostDigest}

	_ = db.InstallChunk(ctx, lostDigest, chunk.Length, bytes.NewReader(lostContent))
	v1, err := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "lost.txt",
		Kind:             history.KindFile,
		Manifest:         manifest,
		AuthoredRevision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Corrupt and quarantine
	_, _ = db.QuarantineChunk(ctx, lostDigest, "corrupt")

	// No peer has this chunk
	ctrl := control.New(db, workspace.New(db, workspace.Options{}), control.Options{
		LocalDevice: author,
		RepairPeers: nil, // zero peers
	})

	res, err := ctrl.StorageRepair(ctx, control.StorageRepairRequest{
		Folder:    folder,
		VersionID: v1.ID,
	})
	if err == nil {
		t.Fatal("expected repair error when no peers have content")
	}
	if res.Status != "unavailable" {
		t.Fatalf("expected status 'unavailable', got %q", res.Status)
	}
}

func TestP11UnauthorizedFolderRepairRejected(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	state := filepath.Join(disposable, "state")
	_ = os.Mkdir(state, 0o700)

	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	unauthorizedFolder := repositoryID('U')
	targetID := history.VersionID{Folder: unauthorizedFolder, Author: repositoryID('A'), Counter: 1}

	ctrl := control.New(db, workspace.New(db, workspace.Options{}), control.Options{})
	_, err = ctrl.StorageRepair(ctx, control.StorageRepairRequest{
		Folder:    unauthorizedFolder,
		VersionID: targetID,
	})
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "authoriz") && !strings.Contains(err.Error(), "unapproved") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestP11IntegrityScanAndRepairPinningPreservesGCInterleavings(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	state := filepath.Join(disposable, "state")
	root := filepath.Join(disposable, "root")
	_ = os.Mkdir(state, 0o700)
	_ = os.Mkdir(root, 0o700)

	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := workspace.New(db, workspace.Options{})
	folder, author := repositoryID('F'), repositoryID('A')
	_ = db.EnsureFolder(ctx, folder, author, 1)
	_, _ = ws.Register(ctx, folder, root)

	// Create v1
	contentV1 := []byte("v1 content to supersede")
	digestV1 := sha256.Sum256(contentV1)
	_ = os.WriteFile(filepath.Join(root, "file.txt"), contentV1, 0o600)
	scan1, err := ws.Scan(ctx, folder)
	if err != nil || len(scan1.Captured) != 1 {
		t.Fatalf("scan1 failed: %v", err)
	}

	// Create v2 superseding v1
	contentV2 := []byte("v2 new content")
	_ = os.WriteFile(filepath.Join(root, "file.txt"), contentV2, 0o600)
	scan2, err := ws.Scan(ctx, folder)
	if err != nil || len(scan2.Captured) != 1 {
		t.Fatalf("scan2 failed: %v", err)
	}

	// v1's chunk is now superseded. With 0 retention days and 0 min superseded, it is GC eligible.
	zeroPolicy := repository.RetentionPolicy{RetentionDays: 0, MinSuperseded: 0}

	// Pin v1's chunk as if an integrity check or repair is currently inspecting it
	pinID := "test-active-scan-pin"
	if err := db.Pin(ctx, digestV1, "integrity_check", pinID); err != nil {
		t.Fatalf("Pin: %v", err)
	}

	// Run GC while chunk is pinned
	gcReport1, err := db.RunGC(ctx, folder, &zeroPolicy, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("RunGC: %v", err)
	}
	if gcReport1.UnlinkedObjects != 0 {
		t.Fatalf("GC unlinked a pinned chunk! unlinked=%d", gcReport1.UnlinkedObjects)
	}

	// Verify chunk still exists and is verified
	verified, err := db.VerifiedChunk(ctx, history.Chunk{Digest: digestV1, Length: uint64(len(contentV1))})
	if err != nil || !verified {
		t.Fatalf("pinned chunk was lost: verified=%v err=%v", verified, err)
	}

	// Unpin
	if err := db.Unpin(ctx, digestV1, "integrity_check", pinID); err != nil {
		t.Fatalf("Unpin: %v", err)
	}

	// Run GC again: now it should be collected cleanly
	gcReport2, err := db.RunGC(ctx, folder, &zeroPolicy, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("RunGC after unpin: %v", err)
	}
	if gcReport2.UnlinkedObjects != 1 {
		t.Fatalf("expected unpinned chunk to be unlinked by GC, unlinked=%d", gcReport2.UnlinkedObjects)
	}
}

func TestP11AvailabilityStateClassification(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	state := filepath.Join(disposable, "state")
	_ = os.Mkdir(state, 0o700)

	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder, author := repositoryID('F'), repositoryID('A')
	remoteAuthor := repositoryID('R')
	_ = db.EnsureFolder(ctx, folder, author, 1)
	m := protocol.Membership{
		Folder:   folder,
		Revision: 1,
		Active: []protocol.ActiveMember{
			{Device: author, KeyPin: history.Digest(repositoryID('A'))},
			{Device: remoteAuthor, KeyPin: history.Digest(repositoryID('R'))},
		},
	}
	if _, err := db.ApproveMembership(ctx, m); err != nil {
		t.Fatal(err)
	}

	// 1. Ready state: installed and verified chunk
	contentReady := []byte("ready content")
	digestReady := sha256.Sum256(contentReady)
	chunkReady := history.Chunk{Digest: digestReady, Length: uint64(len(contentReady))}
	_ = db.InstallChunk(ctx, digestReady, chunkReady.Length, bytes.NewReader(contentReady))
	vReady, _ := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "ready.txt",
		Kind:             history.KindFile,
		Manifest:         &history.Manifest{Size: chunkReady.Length, Chunks: []history.Chunk{chunkReady}, Digest: digestReady},
		AuthoredRevision: 1,
	})
	diagReady, err := db.DiagnoseVersionAvailability(ctx, vReady.ID)
	if err != nil || diagReady.Status != "ready" {
		t.Fatalf("expected ready, got %+v (err: %v)", diagReady, err)
	}

	// 2. Corrupt state: chunk quarantined
	_, _ = db.QuarantineChunk(ctx, digestReady, "bit rot")
	diagCorrupt, err := db.DiagnoseVersionAvailability(ctx, vReady.ID)
	if err != nil || diagCorrupt.Status != "corrupt" {
		t.Fatalf("expected corrupt, got %+v (err: %v)", diagCorrupt, err)
	}
	_ = db.UnquarantineChunk(ctx, digestReady)

	// 3. Pending state: version envelope imported, chunk not yet stored
	chunkPending := history.Chunk{Digest: sha256.Sum256([]byte("pending data")), Length: 12}
	manifestPending := &history.Manifest{Size: chunkPending.Length, Chunks: []history.Chunk{chunkPending}, Digest: chunkPending.Digest}
	vPendingID := history.VersionID{Folder: folder, Author: remoteAuthor, Counter: 1}
	if err := db.ImportMetadata(ctx, history.Envelope{
		ID:               vPendingID,
		Path:             "pending.txt",
		Kind:             history.KindFile,
		Manifest:         manifestPending,
		Vector:           []history.ClockEntry{{Author: remoteAuthor, Counter: 1}},
		AuthoredRevision: 1,
	}); err != nil {
		t.Fatalf("import metadata: %v", err)
	}
	diagPending, err := db.DiagnoseVersionAvailability(ctx, vPendingID)
	if err != nil || diagPending.Status != "pending" {
		t.Fatalf("expected pending, got %+v (err: %v)", diagPending, err)
	}

	// 4. Missing protected state: active head whose chunk file was removed without quarantine
	// Mark chunk as missing on disk
	chunkPathReady := filepath.Join(state, "objects", "sha256", fmt.Sprintf("%02x", digestReady[0]), fmt.Sprintf("%x", digestReady[1:]))
	_ = os.Remove(chunkPathReady)
	diagMissing, err := db.DiagnoseVersionAvailability(ctx, vReady.ID)
	if err != nil || diagMissing.Status != "missing_protected" {
		t.Fatalf("expected missing_protected, got %+v (err: %v)", diagMissing, err)
	}

	// 5. Expired state: create a historical version and let retention expire
	contentExp := []byte("content to expire in test")
	digestExp := sha256.Sum256(contentExp)
	chunkExp := history.Chunk{Digest: digestExp, Length: uint64(len(contentExp))}
	_ = db.InstallChunk(ctx, digestExp, chunkExp.Length, bytes.NewReader(contentExp))
	vOld, _ := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "history.txt",
		Kind:             history.KindFile,
		Manifest:         &history.Manifest{Size: chunkExp.Length, Chunks: []history.Chunk{chunkExp}, Digest: digestExp},
		AuthoredRevision: 1,
	})
	// Create vNew superseding vOld
	contentNew := []byte("new replacement content")
	digestNew := sha256.Sum256(contentNew)
	chunkNew := history.Chunk{Digest: digestNew, Length: uint64(len(contentNew))}
	_ = db.InstallChunk(ctx, digestNew, chunkNew.Length, bytes.NewReader(contentNew))
	_, err = db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "history.txt",
		Basis:            []history.VersionID{vOld.ID},
		Kind:             history.KindFile,
		Manifest:         &history.Manifest{Size: chunkNew.Length, Chunks: []history.Chunk{chunkNew}, Digest: digestNew},
		AuthoredRevision: 1,
	})
	if err != nil {
		t.Fatalf("create vNew: %v", err)
	}
	// Expire old version's content
	_ = db.ExpireContent(ctx, vOld.ID)
	// Remove old chunk without quarantine to simulate expired/unlinked GC
	_ = os.Remove(filepath.Join(state, "objects", "sha256", fmt.Sprintf("%02x", digestExp[0]), fmt.Sprintf("%x", digestExp[1:])))
	// Set retention policy with 0 days and 0 min superseded
	zeroP := repository.RetentionPolicy{RetentionDays: 0, MinSuperseded: 0}
	_ = db.SetRetentionPolicy(ctx, folder, zeroP)
	diagExpired, err := db.DiagnoseVersionAvailability(ctx, vOld.ID)
	if err != nil || diagExpired.Status != "expired" {
		t.Fatalf("expected expired, got %+v (err: %v)", diagExpired, err)
	}
}
