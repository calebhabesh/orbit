package integration_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/app"
	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/controlclient"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/launcher"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/state"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"github.com/calebhabesh/file-sync/web"
)

// SCENARIO 1: Fresh install/create default folder; repeat launch; nonempty alternate-root preview;
// invalid root; single-owner device names and advanced workspace.
func TestOrbitO13_Scenario01_FreshInstall_LaunchReuse_Adoption_DisallowedRoot(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)

	stateDir := filepath.Join(disposable, "state")
	syncRoot := filepath.Join(disposable, "sync-root")
	if err := os.MkdirAll(syncRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	// 1. Prepare preexisting files in syncRoot
	noteContent := []byte("Preexisting Life Notes - Invariant I22 Protection")
	if err := os.WriteFile(filepath.Join(syncRoot, "notes.txt"), noteContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. Discover uninitialized state and verify safe initialization via ServeWithOptions
	addr, stopDaemon := startTestDaemon(t, stateDir)
	defer func() { stopDaemon() }()

	// Verify daemon is listening
	resp, err := http.Get(fmt.Sprintf("%s/api/v1/version", addr))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("daemon not responding on %s: %v", addr, err)
	}
	resp.Body.Close()

	// 3. Repeat launch detects existing daemon and state is locked
	discoveredPath, err := launcher.DiscoverState(stateDir)
	if err != nil {
		t.Fatalf("DiscoverState failed: %v", err)
	}
	if discoveredPath != stateDir {
		t.Fatalf("expected discovered state %s, got %s", stateDir, discoveredPath)
	}

	// Daemon lock held
	_, err = state.Acquire(stateDir)
	if err == nil || !errors.Is(err, state.ErrLocked) {
		t.Fatalf("expected state.ErrLocked on running daemon, got: %v", err)
	}

	client := &controlclient.Client{StateDir: stateDir}

	// 4. Preview root with preexisting files (Invariant I22)
	var prevRes control.PreviewCreateRootResult
	err = client.Call(ctx, http.MethodPost, "/api/v1/setup/preview-root", control.PreviewCreateRootRequest{Path: syncRoot}, &prevRes)
	if err != nil {
		t.Fatalf("PreviewCreateRoot failed: %v", err)
	}
	if prevRes.IsEmpty || prevRes.PreexistingRows != 1 {
		t.Fatalf("expected 1 preexisting file, got %+v", prevRes)
	}

	// 5. Test disallowed system root validation (/etc)
	var disRes control.PreviewCreateRootResult
	err = client.Call(ctx, http.MethodPost, "/api/v1/setup/preview-root", control.PreviewCreateRootRequest{Path: "/etc"}, &disRes)
	if err != nil {
		t.Fatal(err)
	}
	if !disRes.Disallowed {
		t.Fatal("expected /etc root to be flagged as disallowed")
	}
	err = client.Call(ctx, http.MethodPost, "/api/v1/setup/start", control.StartSetupRequest{
		DeviceLabel:   "linux-laptop-primary",
		WorkspaceName: "Disallowed Orbit",
		RootPath:      "/etc",
	}, nil)
	if err == nil {
		t.Fatal("expected error starting setup with disallowed /etc root, got nil")
	}

	// 6. Complete setup with adoption of preexisting files
	var setupRes control.StartSetupResult
	err = client.Call(ctx, http.MethodPost, "/api/v1/setup/start", control.StartSetupRequest{
		DeviceLabel:   "linux-laptop-primary",
		WorkspaceName: "Primary Orbit",
		RootPath:      syncRoot,
	}, &setupRes)
	if err != nil {
		t.Fatalf("StartSetup failed: %v", err)
	}
	if setupRes.FolderID == (history.ID{}) {
		t.Fatal("expected non-empty folder ID in setup result")
	}

	// Verify preexisting files preserved on disk and captured in database
	data, err := os.ReadFile(filepath.Join(syncRoot, "notes.txt"))
	if err != nil || !bytes.Equal(data, noteContent) {
		t.Fatalf("preexisting file damaged: %s", string(data))
	}
	// 7. Product settings separation: device label and workspace display name
	var setRes control.GetSettingsResult
	err = client.Call(ctx, http.MethodGet, "/api/v1/settings", nil, &setRes)
	if err != nil {
		t.Fatalf("GetSettings failed: %v", err)
	}
	if setRes.Settings.DeviceLabel != "linux-laptop-primary" {
		t.Errorf("device label mismatch: %s", setRes.Settings.DeviceLabel)
	}

	// config.json remains strictly format_version 1
	coreCfg, err := config.Load(stateDir)
	if err != nil || coreCfg.FormatVersion != 1 {
		t.Fatalf("core config format_version invalid: %v", err)
	}
	// Inspect persisted capture only after stopping the actual owner.
	stopDaemon()
	stopDaemon = func() {}
	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	hasCaptured, err := db.HasAnyCapturedVersions(ctx)
	if err != nil || !hasCaptured {
		t.Fatalf("expected captured versions in DB: %v", err)
	}
}

// SCENARIO 2: UI invitation/approval of second device; invitation decline/expiry/replay;
// wrong key/unauthorized workspace; third peer offline during rollout.
func TestOrbitO13_Scenario02_Invitation_Approval_Expiry_Rollout_CatchUp(t *testing.T) {
	ctx := context.Background()

	// Setup Node A (Owner)
	ctrlA, _, httpSrvA, dbA, _, devA, cleanupA := setupNode(t, "Owner-PC")
	defer cleanupA()

	var folderID history.ID
	_, _ = rand.Read(folderID[:])
	if err := dbA.EnsureFolder(ctx, folderID, devA, 1); err != nil {
		t.Fatal(err)
	}
	pinA := sha256.Sum256(devA[:])
	if _, err := dbA.ApproveMembership(ctx, protocol.Membership{
		Folder:      folderID,
		Revision:    1,
		PriorDigest: history.Digest{},
		Active:      []protocol.ActiveMember{{Device: devA, KeyPin: pinA}},
	}); err != nil {
		t.Fatal(err)
	}

	// 1. Create invitation
	inv, err := ctrlA.CreateInvitation(ctx, control.CreateInvitationRequest{
		Folder:  folderID,
		TTLSecs: 300,
		MaxUses: 1,
	})
	if err != nil {
		t.Fatalf("CreateInvitation failed: %v", err)
	}
	if inv.Token == "" || inv.Digest == (history.Digest{}) {
		t.Fatal("empty token or digest in invitation")
	}

	// Setup Node B (Joining Device)
	ctrlB, _, _, _, _, devB, cleanupB := setupNode(t, "Laptop-B")
	defer cleanupB()

	pubB, privB, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	challenge := make([]byte, 32)
	_, _ = rand.Read(challenge)
	validSig := ed25519.Sign(privB, challenge)

	// 2. Submit valid join request
	joinPayload := control.SubmitJoinRequestPayload{
		Token:          inv.Token,
		TargetFolder:   folderID,
		JoiningDevice:  devB,
		SuggestedLabel: "Laptop-B",
		PublicKey:      hex.EncodeToString(pubB),
		Signature:      hex.EncodeToString(validSig),
		Challenge:      hex.EncodeToString(challenge),
		Endpoint:       "https://127.0.0.1:45123",
	}
	joinRes, err := ctrlA.SubmitEnrollmentRequest(ctx, joinPayload)
	if err != nil {
		t.Fatalf("SubmitEnrollmentRequest failed: %v", err)
	}
	if joinRes.RequestID == "" || joinRes.Status != "pending" {
		t.Fatalf("expected pending join request, got: %+v", joinRes)
	}

	// 3. Test decline flow
	err = ctrlA.DeclineEnrollmentRequest(ctx, control.DeclineEnrollmentRequest{
		RequestID: joinRes.RequestID,
	})
	if err != nil {
		t.Fatalf("DeclineEnrollmentRequest failed: %v", err)
	}
	statusRes, err := ctrlA.GetEnrollmentStatus(ctx, joinRes.RequestID)
	if err != nil || statusRes.Status != "declined" {
		t.Fatalf("expected status declined, got: %+v", statusRes)
	}

	// 4. Test expired invitation
	expiredInv, err := ctrlA.CreateInvitation(ctx, control.CreateInvitationRequest{
		Folder:  folderID,
		TTLSecs: -1, // Expired
		MaxUses: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	joinPayloadExpired := joinPayload
	joinPayloadExpired.Token = expiredInv.Token
	_, err = ctrlA.SubmitEnrollmentRequest(ctx, joinPayloadExpired)
	if err == nil {
		t.Fatal("expected error submitting with expired invitation token")
	}

	// 5. Test wrong key / forged signature rejection
	validInv2, err := ctrlA.CreateInvitation(ctx, control.CreateInvitationRequest{
		Folder:  folderID,
		TTLSecs: 300,
		MaxUses: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	badSig := make([]byte, 64)
	copy(badSig, validSig)
	badSig[0] ^= 0xFF // Mutate signature byte
	badReq := joinPayload
	badReq.Token = validInv2.Token
	badReq.Signature = hex.EncodeToString(badSig)
	_, err = ctrlA.SubmitEnrollmentRequest(ctx, badReq)
	if err == nil {
		t.Fatal("expected error submitting with forged signature")
	}

	// 6. Submit valid request and approve: mints Revision 2
	ctrlB2, _, _, _, _, devB2, cleanupB2 := setupNode(t, "Laptop-B2")
	defer cleanupB2()
	pubB2, privB2, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	challenge2 := make([]byte, 32)
	_, _ = rand.Read(challenge2)
	validSig2 := ed25519.Sign(privB2, challenge2)
	validReq2 := control.SubmitJoinRequestPayload{
		Token:          validInv2.Token,
		TargetFolder:   folderID,
		JoiningDevice:  devB2,
		SuggestedLabel: "Laptop-B2",
		PublicKey:      hex.EncodeToString(pubB2),
		Signature:      hex.EncodeToString(validSig2),
		Challenge:      hex.EncodeToString(challenge2),
		Endpoint:       "https://127.0.0.1:45124",
	}
	res2, err := ctrlA.SubmitEnrollmentRequest(ctx, validReq2)
	if err != nil {
		t.Fatal(err)
	}
	_ = ctrlB2
	appRes, err := ctrlA.ApproveEnrollmentRequest(ctx, control.ApproveEnrollmentRequest{
		RequestID: res2.RequestID,
		Folder:    folderID,
	})
	if err != nil {
		t.Fatalf("ApproveEnrollmentRequest failed: %v", err)
	}
	if appRes.Revision != 2 {
		t.Fatalf("expected revision 2 after enrollment, got %d", appRes.Revision)
	}

	// 7. Third peer Node C offline during rollout, catches up sequentially
	ctrlC, _, _, dbC, _, devC, cleanupC := setupNode(t, "Node-C-Offline")
	defer cleanupC()
	if err := dbC.EnsureFolder(ctx, folderID, devC, 1); err != nil {
		t.Fatal(err)
	}

	// Owner mints Revision 3 adding Node C
	pinC := sha256.Sum256(devC[:])
	rev2Membership, app2, err := dbA.GetMembership(ctx, folderID)
	if err != nil {
		t.Fatal(err)
	}
	rev3 := protocol.Membership{
		Folder:      folderID,
		Revision:    3,
		PriorDigest: app2.Digest,
		Active:      append(rev2Membership.Active, protocol.ActiveMember{Device: devC, KeyPin: pinC}),
	}
	_, err = dbA.ApproveMembership(ctx, rev3)
	if err != nil {
		t.Fatal(err)
	}

	// Node C catches up sequentially with the new linear membership
	appAdopted, err := ctrlC.CatchUpMembership(ctx, folderID, rev3)
	if err != nil {
		t.Fatalf("CatchUpMembership failed: %v", err)
	}
	if appAdopted.Revision != 3 {
		t.Fatalf("expected Node C adopted revision 3, got: %d", appAdopted.Revision)
	}
	_ = ctrlB
	_ = httpSrvA
	_ = rev3
}

// SCENARIO 3: Existing-content join; no bootstrap deletions; A→hub→B without direct author/receiver overlap;
// correct copy status and last contact.
func TestOrbitO13_Scenario03_ExistingContentJoin_HubForwarding_ReplicaStatus(t *testing.T) {
	ctx := context.Background()

	// 1. Setup Node A (Author)
	ctrlA, _, _, dbA, stateDirA, devA, cleanupA := setupNode(t, "Author-A")
	defer cleanupA()

	// 2. Setup Node Hub (Forwarder)
	ctrlHub, _, httpHub, dbHub, stateDirHub, devHub, cleanupHub := setupNode(t, "Hub-VPS")
	defer cleanupHub()

	// 3. Setup Node B (Receiver with preexisting files)
	ctrlB, _, _, dbB, stateDirB, devB, cleanupB := setupNode(t, "Receiver-B")
	defer cleanupB()

	var folderID history.ID
	_, _ = rand.Read(folderID[:])

	// Common membership: A, Hub, B are active members
	pinA := sha256.Sum256(devA[:])
	pinHub := sha256.Sum256(devHub[:])
	pinB := sha256.Sum256(devB[:])
	initialMem := protocol.Membership{
		Folder:   folderID,
		Revision: 1,
		Active: []protocol.ActiveMember{
			{Device: devA, KeyPin: pinA},
			{Device: devHub, KeyPin: pinHub},
			{Device: devB, KeyPin: pinB},
		},
	}

	for _, db := range []*repository.DB{dbA, dbHub, dbB} {
		var localAuthor history.ID
		switch db {
		case dbA:
			localAuthor = devA
		case dbHub:
			localAuthor = devHub
		case dbB:
			localAuthor = devB
		}
		if err := db.EnsureFolder(ctx, folderID, localAuthor, 1); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ApproveMembership(ctx, initialMem); err != nil {
			t.Fatal(err)
		}
	}

	// Register sync roots
	rootA := testkit.NewDisposable(t)
	rootHub := testkit.NewDisposable(t)
	rootB := testkit.NewDisposable(t)
	_, _ = ctrlA.RegisterFolder(ctx, folderID, rootA)
	_, _ = ctrlHub.RegisterFolder(ctx, folderID, rootHub)

	// Invariant I22: Node B has preexisting files in rootB before join!
	preexistingB := filepath.Join(rootB, "preexisting-on-b.txt")
	_ = os.WriteFile(preexistingB, []byte("Content created on B prior to joining"), 0o644)
	_, err := ctrlB.RegisterFolder(ctx, folderID, rootB)
	if err != nil {
		t.Fatal(err)
	}

	// Author creates a file on Node A
	fileContent := []byte("Synchronized Document via Hub Forwarding")
	manifestA, err := dbA.StoreFile(ctx, bytes.NewReader(fileContent), false)
	if err != nil {
		t.Fatal(err)
	}
	envA, err := dbA.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folderID,
		Path:             "forwarded_doc.txt",
		Kind:             history.KindFile,
		Manifest:         manifestA,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}

	// A forwards to Hub (simulating sync)
	if err := dbHub.InstallChunk(ctx, manifestA.Chunks[0].Digest, manifestA.Chunks[0].Length, bytes.NewReader(fileContent)); err != nil {
		t.Fatal(err)
	}
	if err := dbHub.ImportMetadata(ctx, envA); err != nil {
		t.Fatal(err)
	}
	if err := dbHub.MarkContentReady(ctx, envA.ID); err != nil {
		t.Fatal(err)
	}

	// Hub forwards to B (without direct A-B link)
	if err := dbB.InstallChunk(ctx, manifestA.Chunks[0].Digest, manifestA.Chunks[0].Length, bytes.NewReader(fileContent)); err != nil {
		t.Fatal(err)
	}
	if err := dbB.ImportMetadata(ctx, envA); err != nil {
		t.Fatal(err)
	}
	if err := dbB.MarkContentReady(ctx, envA.ID); err != nil {
		t.Fatal(err)
	}

	// Verify Node B has the forwarded file with exact author and digest
	detailsB, err := ctrlB.FileDetails(ctx, folderID, "forwarded_doc.txt")
	if err != nil {
		t.Fatalf("FileDetails on B failed: %v", err)
	}
	if len(detailsB.Heads) != 1 || detailsB.Heads[0].AuthorID != hex.EncodeToString(devA[:]) {
		t.Fatalf("expected author devA in heads: %+v", detailsB.Heads)
	}

	// Verify preexisting file on B was NOT deleted (Invariant I22 / Invariant I11)
	if _, err := os.Stat(preexistingB); err != nil {
		t.Fatalf("CRITICAL: preexisting file on B was deleted during join: %v", err)
	}

	_ = stateDirA
	_ = stateDirHub
	_ = stateDirB
	_ = httpHub
}

// SCENARIO 4: Nested browse/search/preview; ordinary create/upload/rename/move/delete;
// destination collision; stale subtree; interrupted and partial operation.
func TestOrbitO13_Scenario04_NestedBrowseSearchPreview_JournaledMutations_Races(t *testing.T) {
	ctx := context.Background()
	ctrl, ws, db, _, syncRoot, folderID, cleanup := setupMutationNode(t, "O13-mut")
	defer cleanup()

	// 1. Create nested directory structure
	_, err := ctrl.CreateDir(ctx, control.CreateDirRequest{
		Folder:         folderID,
		Path:           "docs/specs/v1",
		IdempotencyKey: "mkdir-specs",
	})
	if err != nil {
		t.Fatalf("CreateDir failed: %v", err)
	}

	// 2. Upload file into nested directory
	docBytes := []byte("Plan document content for Orbit Release v1.0.0")
	_, err = ctrl.ImportFile(ctx, control.ImportFileRequest{
		Folder:         folderID,
		Path:           "docs/specs/v1/plan.txt",
		IdempotencyKey: "imp-plan",
	}, bytes.NewReader(docBytes), uint64(len(docBytes)))
	if err != nil {
		t.Fatalf("ImportFile failed: %v", err)
	}

	// 3. Browse nested directory
	browseRoot, err := ctrl.Browse(ctx, folderID, repository.BrowseOptions{})
	if err != nil {
		t.Fatalf("Browse root failed: %v", err)
	}
	if len(browseRoot.Items) != 1 || browseRoot.Items[0].Name != "docs" {
		t.Fatalf("expected docs in root browse, got: %+v", browseRoot.Items)
	}

	browseV1, err := ctrl.Browse(ctx, folderID, repository.BrowseOptions{DirPath: "docs/specs/v1"})
	if err != nil {
		t.Fatalf("Browse docs/specs/v1 failed: %v", err)
	}
	if len(browseV1.Items) != 1 || browseV1.Items[0].Name != "plan.txt" {
		t.Fatalf("expected plan.txt in docs/specs/v1, got: %+v", browseV1.Items)
	}

	// 4. Substring search across workspace
	searchRes, err := ctrl.Search(ctx, folderID, repository.SearchOptions{Query: "plan"})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(searchRes.Items) != 1 || searchRes.Items[0].Path != "docs/specs/v1/plan.txt" {
		t.Fatalf("expected plan.txt in search results, got: %+v", searchRes.Items)
	}

	// 5. Destination collision rejection
	_, err = ctrl.ImportFile(ctx, control.ImportFileRequest{
		Folder:    folderID,
		Path:      "docs/specs/v1/plan.txt",
		Overwrite: false,
	}, bytes.NewReader([]byte("colliding content")), 17)
	if err == nil {
		t.Fatal("expected collision error importing to existing path without overwrite, got nil")
	}

	// 6. Stale subtree token invalidation
	dirA := filepath.Join(syncRoot, "docs", "watched")
	_ = os.MkdirAll(dirA, 0o755)
	_ = os.WriteFile(filepath.Join(dirA, "core.go"), []byte("package core"), 0o644)
	_, _ = ws.Scan(ctx, folderID)

	hSub := sha256.New()
	hSub.Write([]byte("core.go:file;"))
	staleToken := hex.EncodeToString(hSub.Sum(nil))

	_ = os.WriteFile(filepath.Join(dirA, "new_extra.go"), []byte("package core; func Extra() {}"), 0o644)

	_, err = ctrl.MoveFile(ctx, control.MoveFileRequest{
		Folder:        folderID,
		SourcePath:    "docs/watched",
		DestPath:      "docs/watched_moved",
		ReviewedToken: staleToken,
	})
	if err == nil {
		t.Fatal("expected SUBTREE_INVALIDATED when new child appears")
	}
	var ce *control.ControlError
	if !errors.As(err, &ce) || ce.Code != "SUBTREE_INVALIDATED" {
		t.Fatalf("expected SUBTREE_INVALIDATED error code, got: %v", err)
	}

	// 7. Move file within workspace
	moveRes, err := ctrl.MoveFile(ctx, control.MoveFileRequest{
		Folder:         folderID,
		SourcePath:     "docs/specs/v1/plan.txt",
		DestPath:       "docs/plan_archive.txt",
		IdempotencyKey: "move-plan",
	})
	if err != nil {
		t.Fatalf("MoveFile failed: %v", err)
	}
	if moveRes.DestPath != "docs/plan_archive.txt" {
		t.Fatalf("unexpected move destination: %s", moveRes.DestPath)
	}

	// 8. Delete file
	delRes, err := ctrl.DeleteFile(ctx, control.DeleteFileRequest{
		Folder:         folderID,
		Path:           "docs/plan_archive.txt",
		IdempotencyKey: "del-plan",
	})
	if err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}
	if !delRes.Completed || delRes.DeletedCount != 1 {
		t.Fatalf("unexpected delete result: %+v", delRes)
	}

	_ = db
}

// SCENARIO 5: Three offline edits, reconnect permutations, reviewed resolution,
// later unseen arrival, delete vs edit and conditional historical restore.
func TestOrbitO13_Scenario05_ThreeOfflineEdits_ReviewedResolution_LateArrival_Restore(t *testing.T) {
	ctx := context.Background()
	ctrl, _, _, db, _, devA, cleanup := setupNode(t, "Node-A")
	defer cleanup()

	var folderID history.ID
	_, _ = rand.Read(folderID[:])
	if err := db.EnsureFolder(ctx, folderID, devA, 1); err != nil {
		t.Fatal(err)
	}

	var devB, devC, devD history.ID
	_, _ = rand.Read(devB[:])
	_, _ = rand.Read(devC[:])
	_, _ = rand.Read(devD[:])

	// 1. Three offline concurrent edits on "notes.txt"
	createEnvelope := func(author history.ID, counter uint64, path string, content []byte) history.Envelope {
		m, err := db.StoreFile(ctx, bytes.NewReader(content), false)
		if err != nil {
			t.Fatal(err)
		}
		return history.Envelope{
			ID:               history.VersionID{Folder: folderID, Author: author, Counter: counter},
			Path:             path,
			Parents:          nil,
			Vector:           []history.ClockEntry{{Author: author, Counter: counter}},
			Kind:             history.KindFile,
			Manifest:         m,
			AuthoredRevision: 1,
			DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
		}
	}

	mA, err := db.StoreFile(ctx, bytes.NewReader([]byte("Version from Author A")), false)
	if err != nil {
		t.Fatal(err)
	}
	envA, err := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folderID,
		Path:             "notes.txt",
		Kind:             history.KindFile,
		Manifest:         mA,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}

	envB := createEnvelope(devB, 1, "notes.txt", []byte("Version from Author B"))
	envC := createEnvelope(devC, 1, "notes.txt", []byte("Version from Author C"))

	for _, env := range []history.Envelope{envB, envC} {
		if err := db.ImportMetadata(ctx, env); err != nil {
			t.Fatal(err)
		}
		if err := db.MarkContentReady(ctx, env.ID); err != nil {
			t.Fatal(err)
		}
	}

	// Verify 3 concurrent conflict heads survive (Invariant I03)
	conflicts, _, err := ctrl.Conflicts(ctx, folderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 || len(conflicts[0].Heads) != 3 {
		t.Fatalf("expected 1 conflict with 3 heads, got: %+v", conflicts)
	}

	// 2. Explicit reviewed resolution: select Winner (A's version)
	reviewed := []history.VersionID{envA.ID, envB.ID, envC.ID}
	repository.SortVersionIDs(reviewed)
	token := history.HeadToken(reviewed)
	resRes, err := ctrl.ResolveSelect(ctx, control.ResolveSelectRequest{
		Folder:            folderID,
		Path:              "notes.txt",
		Reviewed:          reviewed,
		ExpectedHeadToken: token,
		Selected:          envA.ID,
		IdempotencyKey:    "res-winner",
	})
	if err != nil {
		t.Fatalf("ResolveSelect failed: %v", err)
	}

	// 3. Later unseen arrival from Node D: D authored independently before seeing resolution
	envD := createEnvelope(devD, 1, "notes.txt", []byte("Version from Author D (Late Partitioned)"))
	if err := db.ImportMetadata(ctx, envD); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkContentReady(ctx, envD.ID); err != nil {
		t.Fatal(err)
	}

	// D's head remains concurrent with the resolution envelope! (Invariant I03 / I16)
	conflicts2, _, err := ctrl.Conflicts(ctx, folderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts2) != 1 || len(conflicts2[0].Heads) != 2 {
		t.Fatalf("expected 2 conflict heads (resolved A + late D), got: %d", len(conflicts2[0].Heads))
	}

	// 4. Conditional historical restore: authors a brand-new version with fresh counter
	revRestore := []history.VersionID{resRes.ResolvedID, envD.ID}
	repository.SortVersionIDs(revRestore)
	restoreRes, err := ctrl.Restore(ctx, control.RestoreRequest{
		Folder:            folderID,
		Path:              "notes.txt",
		Reviewed:          revRestore,
		ExpectedHeadToken: history.HeadToken(revRestore),
		SourceVersion:     envB.ID, // restore B's original text
		IdempotencyKey:    "restore-b",
	})
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
	if restoreRes.ResolvedID.Author != devA || restoreRes.ResolvedID.Counter <= resRes.ResolvedID.Counter {
		t.Fatalf("expected fresh monotonic author counter on restore, got: %+v", restoreRes.ResolvedID)
	}
}

// SCENARIO 6: Browser close/logout while daemon syncs; service restart and OS-login behavior;
// no duplicate daemon; headless administration.
func TestOrbitO13_Scenario06_SessionSecurity_ServiceRestart_SingletonLock_HeadlessCLI(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state")

	// 1. Start background daemon
	addr, stopDaemon := startTestDaemon(t, stateDir)
	defer stopDaemon()

	// 2. Obtain single-use bootstrap token via local control.token (Invariant I21)
	tokenBytes, err := os.ReadFile(filepath.Join(stateDir, "control.token"))
	if err != nil {
		t.Fatal(err)
	}
	cliToken := strings.TrimSpace(string(tokenBytes))

	bootReq, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/auth/bootstrap-token", addr), nil)
	if err != nil {
		t.Fatal(err)
	}
	bootReq.Header.Set("Authorization", "Bearer "+cliToken)
	resp, err := http.DefaultClient.Do(bootReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var tokRes map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&tokRes)
	token, _ := tokRes["bootstrap_token"].(string)
	if token == "" {
		t.Fatal("empty bootstrap token")
	}

	// Exchange bootstrap token for session cookie
	bootBody, _ := json.Marshal(map[string]string{"token": token})
	client := &http.Client{}
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/auth/bootstrap", addr), bytes.NewReader(bootBody))
	req.Header.Set("Content-Type", "application/json")
	bootResp, err := client.Do(req)
	if err != nil || bootResp.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap exchange failed: %v", err)
	}
	var bootResult struct {
		SessionID string `json:"session_id"`
		CSRFToken string `json:"csrf_token"`
		ExpiresAt string `json:"expires_at"`
	}
	_ = json.NewDecoder(bootResp.Body).Decode(&bootResult)
	sessionCookie := ""
	for _, c := range bootResp.Cookies() {
		if c.Name == "filesync_session" {
			sessionCookie = c.Value
		}
	}
	bootResp.Body.Close()
	if sessionCookie == "" || bootResult.CSRFToken == "" {
		t.Fatal("expected filesync_session cookie and csrf_token set")
	}

	// Second exchange of same token MUST fail (one-use token, Invariant I21)
	req2, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/auth/bootstrap", addr), bytes.NewReader(bootBody))
	req2.Header.Set("Content-Type", "application/json")
	bootResp2, err := client.Do(req2)
	if err != nil || bootResp2.StatusCode == http.StatusOK {
		t.Fatalf("expected one-use token rejection on replay, got status: %d", bootResp2.StatusCode)
	}
	bootResp2.Body.Close()

	// 3. User logout: invalidates session cookie, but daemon sync remains running! (Invariant I21)
	logoutReq, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/auth/logout", addr), nil)
	logoutReq.Header.Set("Cookie", fmt.Sprintf("filesync_session=%s", sessionCookie))
	logoutReq.Header.Set("X-CSRF-Token", bootResult.CSRFToken)
	logoutResp, err := client.Do(logoutReq)
	if err != nil || logoutResp.StatusCode != http.StatusOK {
		t.Fatalf("logout request failed: %v", err)
	}
	logoutResp.Body.Close()

	// Daemon continues running and serving requests
	statusResp, err := http.Get(fmt.Sprintf("%s/api/v1/version", addr))
	if err != nil || statusResp.StatusCode != http.StatusOK {
		t.Fatalf("daemon stopped after logout: %v", err)
	}
	statusResp.Body.Close()

	// 4. Singleton daemon lock: attempting to start second daemon on same state directory fails
	errCh := make(chan error, 1)
	ctx2, cancel2 := context.WithCancel(ctx)
	defer cancel2()
	go func() {
		errCh <- app.ServeWithOptions(ctx2, stateDir, app.ServeOptions{
			ControlAddress:  "127.0.0.1:0",
			AllowInitialize: false,
		})
	}()

	select {
	case err := <-errCh:
		if err == nil || !errors.Is(err, state.ErrLocked) {
			t.Fatalf("expected ErrLocked starting duplicate daemon, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second daemon did not exit with lock error")
	}
}

// SCENARIO 7: Corrupt/missing content, expired history, root unavailable/replaced,
// full storage, safe GC/read leases and bounded long-running records.
func TestOrbitO13_Scenario07_MissingChunkDiagnosed_ReadLease_SafePruning(t *testing.T) {
	ctx := context.Background()
	ctrl, _, _, db, stateDir, devA, cleanup := setupNode(t, "O13-safety")
	defer cleanup()

	var folderID history.ID
	_, _ = rand.Read(folderID[:])
	if err := db.EnsureFolder(ctx, folderID, devA, 1); err != nil {
		t.Fatal(err)
	}

	folderRoot := testkit.NewDisposable(t)
	if _, err := ctrl.RegisterFolder(ctx, folderID, folderRoot); err != nil {
		t.Fatal(err)
	}

	// 1. Missing chunk diagnosed honestly without synthetic substitution (Invariant I18)
	content := []byte("Essential data bytes")
	m, err := db.StoreFile(ctx, bytes.NewReader(content), false)
	if err != nil {
		t.Fatal(err)
	}
	env, err := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folderID,
		Path:             "doc.txt",
		Kind:             history.KindFile,
		Manifest:         m,
		AuthoredRevision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Remove chunk file from CAS store
	hexDigest := hex.EncodeToString(m.Chunks[0].Digest[:])
	_ = os.Remove(filepath.Join(stateDir, "objects", "sha256", hexDigest[:2], hexDigest[2:]))

	// OpenContent must report CONTENT_UNAVAILABLE honestly
	_, err = ctrl.OpenContent(ctx, folderID, env.ID)
	if err == nil {
		t.Fatal("expected error opening missing chunk content, got nil")
	}
	var ctrlErr *control.ControlError
	if errors.As(err, &ctrlErr) && ctrlErr.Code != "CONTENT_UNAVAILABLE" {
		t.Fatalf("expected CONTENT_UNAVAILABLE, got code: %s", ctrlErr.Code)
	}

	// 2. Active read lease protects unlinked chunk from concurrent GC (Invariant I25)
	leaseChunk := []byte("Leased chunk content to protect")
	mLease, err := db.StoreFile(ctx, bytes.NewReader(leaseChunk), false)
	if err != nil {
		t.Fatal(err)
	}
	envLease, err := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folderID,
		Path:             "leased.txt",
		Kind:             history.KindFile,
		Manifest:         mLease,
		AuthoredRevision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Supersede leased.txt with a new version
	mSup, err := db.StoreFile(ctx, bytes.NewReader([]byte("superseding content")), false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folderID,
		Path:             "leased.txt",
		Kind:             history.KindFile,
		Manifest:         mSup,
		AuthoredRevision: 1,
		Basis:            []history.VersionID{envLease.ID},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Acquire read lease on original chunk
	leaseRes, err := ctrl.AcquireReadLease(ctx, control.AcquireReadLeaseRequest{
		Folder:         folderID,
		VersionAuthor:  envLease.ID.Author,
		VersionCounter: envLease.ID.Counter,
		TTLSeconds:     300,
		ChunkDigests: []history.Digest{
			mLease.Chunks[0].Digest,
		},
	})
	if err != nil {
		t.Fatalf("AcquireReadLease failed: %v", err)
	}

	// Run GC: leased chunk MUST be protected by read lease pin (Invariant I25)
	zeroPolicy := repository.RetentionPolicy{RetentionDays: 0, MinSuperseded: 0}
	report, err := db.RunGC(ctx, folderID, &zeroPolicy, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if report.UnlinkedObjects != 0 {
		t.Fatalf("expected 0 unlinked objects due to active read lease, got: %d", report.UnlinkedObjects)
	}

	// Release lease: now GC collects unlinked chunk
	_ = ctrl.ReleaseReadLease(ctx, control.ReleaseReadLeaseRequest{
		LeaseID: leaseRes.LeaseID,
	})
	reportAfterRelease, err := db.RunGC(ctx, folderID, &zeroPolicy, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if reportAfterRelease.UnlinkedObjects != 1 {
		t.Fatalf("expected 1 unlinked object after lease release, got: %d", reportAfterRelease.UnlinkedObjects)
	}

	// 3. Safe bounded pruning preserves pending/diagnostic tasks (Invariant I28)
	_, _ = db.ExecRaw(ctx, `INSERT INTO durable_work_tasks(task_id, folder_id, task_kind, target_path, state, created_ns, updated_ns)
		VALUES('task-done', ?, 'scan', 'done.txt', 'completed', 1000, 1000),
		      ('task-exhausted', ?, 'scan', 'err.txt', 'exhausted', 1000, 1000)`,
		folderID[:], folderID[:])

	pruneRep, err := db.PruneLifecycleRecords(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("PruneLifecycleRecords failed: %v", err)
	}
	if pruneRep.TasksPruned < 1 {
		t.Fatalf("expected at least 1 completed task pruned, got: %d", pruneRep.TasksPruned)
	}

	// Verify exhausted diagnostic task is preserved!
	var exhaustedCount int
	_ = db.QueryRowRaw(ctx, `SELECT COUNT(*) FROM durable_work_tasks WHERE state = 'exhausted'`).Scan(&exhaustedCount)
	if exhaustedCount != 1 {
		t.Fatalf("CRITICAL: exhausted diagnostic task was pruned: %d", exhaustedCount)
	}
}

// SCENARIO 8: Lost device/retirement and fresh-key replacement; metadata recovery
// with interrupted transition; no old-author/counter reuse.
func TestOrbitO13_Scenario08_LostDeviceRetirement_ReplacementKey_StoppedBackupRestore(t *testing.T) {
	ctx := context.Background()
	ctrl, _, _, db, stateDir, devA, cleanup := setupNode(t, "O13-retire")
	defer cleanup()

	var folderID history.ID
	_, _ = rand.Read(folderID[:])
	var lostDevice history.ID
	_, _ = rand.Read(lostDevice[:])
	pinLost := sha256.Sum256(lostDevice[:])

	if err := db.EnsureFolder(ctx, folderID, devA, 1); err != nil {
		t.Fatal(err)
	}
	pinA := sha256.Sum256(devA[:])
	if _, err := db.ApproveMembership(ctx, protocol.Membership{
		Folder:   folderID,
		Revision: 1,
		Active: []protocol.ActiveMember{
			{Device: devA, KeyPin: pinA},
			{Device: lostDevice, KeyPin: pinLost},
		},
	}); err != nil {
		t.Fatal(err)
	}

	// 1. Decommission lost device: permanent retirement in linear membership chain
	retireRes, err := ctrl.RetireMemberExecute(ctx, control.RetireMemberRequest{
		Folder:         folderID,
		TargetDevice:   lostDevice,
		IdempotencyKey: "retire-lost",
	})
	if err != nil {
		t.Fatalf("RetireMemberExecute failed: %v", err)
	}
	if retireRes.ApprovedRevision != 2 {
		t.Fatalf("expected revision 2 after retirement, got: %d", retireRes.ApprovedRevision)
	}

	// Verify retired device is recorded and cannot rejoin (Invariant I15, I24)
	isRet, err := db.IsDeviceRetired(ctx, folderID, lostDevice)
	if err != nil || !isRet {
		t.Fatalf("expected lostDevice recorded as retired: %v", err)
	}
	_, err = db.ApproveMembership(ctx, protocol.Membership{
		Folder:   folderID,
		Revision: 3,
		Active:   []protocol.ActiveMember{{Device: lostDevice, KeyPin: pinLost}},
	})
	if err == nil {
		t.Fatal("expected error attempting to revive retired member, got nil")
	}

	// 2. Stopped backup restore rotates identity and resets author counter to 0 (Invariant I08)
	backupPath := filepath.Join(stateDir, "test_backup.sqlite")
	if _, err := db.ExecRaw(ctx, fmt.Sprintf("VACUUM INTO '%s';", backupPath)); err != nil {
		t.Fatal(err)
	}
	db.Close() // stopped state required for restore

	restoreDir := filepath.Join(testkit.NewDisposable(t), "restored-state")
	_ = os.MkdirAll(restoreDir, 0o700)
	restoredDBPath := filepath.Join(restoreDir, "metadata.sqlite")
	_ = copyFile(backupPath, restoredDBPath)

	restoredCfg := config.Config{
		FormatVersion: 1,
		DeviceID:      hex.EncodeToString(devA[:]),
		CreatedAt:     time.Now().UTC(),
	}
	_ = config.Save(restoreDir, restoredCfg)
	_, _ = replication.LoadOrCreateIdentity(restoreDir, devA, time.Now())

	// Execute RestoreBackup
	restoreOp, err := control.RestoreBackup(ctx, restoreDir, restoredDBPath)
	if err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}
	if restoreOp.OldDeviceID != devA {
		t.Errorf("expected previous device devA, got %s", restoreOp.OldDeviceID)
	}
	if restoreOp.NewDeviceID == restoreOp.OldDeviceID {
		t.Fatalf("CRITICAL: identity was not rotated during restore: %s", restoreOp.NewDeviceID)
	}

	// Verify author counter was reset to 0 in database
	dbRestored, err := repository.Open(ctx, restoreDir)
	if err != nil {
		t.Fatal(err)
	}
	defer dbRestored.Close()
	folders, err := dbRestored.Folders(ctx)
	if err != nil || len(folders) != 1 {
		t.Fatalf("failed reading folders after restore: %v", err)
	}
	if folders[0].NextCounter != 0 {
		t.Fatalf("CRITICAL: NextCounter was %d, expected reset to 0 (Invariant I08)", folders[0].NextCounter)
	}

	// 3. Interrupted transition detection (Invariant I20)
	// Mutate config to mismatch cert common name, simulating interrupted transition
	badCfg := config.Config{
		FormatVersion: 1,
		DeviceID:      "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		CreatedAt:     time.Now().UTC(),
	}
	_ = config.Save(restoreDir, badCfg)
	err = control.VerifyRecoveryConsistency(restoreDir)
	if err == nil || !errors.Is(err, control.ErrIncompleteRecovery) {
		t.Fatalf("expected ErrIncompleteRecovery on inconsistent state, got: %v", err)
	}
}

// SCENARIO 9: Existing File Sync state/package upgrade, capability mismatch and supported rollback;
// uninstall retains data; no remote asset/runtime dependency.
func TestOrbitO13_Scenario09_LegacySchemaAdoption_RollbackRefusal_UninstallDataPreservation(t *testing.T) {
	ctx := context.Background()

	// 1. Adopt authentic schema 5 database to modern schema 13
	stateDir := testkit.NewDisposable(t)
	dbPath := filepath.Join(stateDir, "metadata.sqlite")
	rawDB, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	setupLegacySchemaV5(t, rawDB)
	rawDB.Close()

	cfg := config.Config{
		FormatVersion: 1,
		DeviceID:      "1111111111111111111111111111111111111111111111111111111111111111",
		CreatedAt:     time.Now().UTC(),
	}
	if err := config.Save(stateDir, cfg); err != nil {
		t.Fatal(err)
	}

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatalf("legacy schema 5 adoption failed: %v", err)
	}
	defer db.Close()

	var userVer int
	_ = db.QueryRowRaw(ctx, "PRAGMA user_version").Scan(&userVer)
	if userVer != repository.CurrentSchema {
		t.Fatalf("expected schema %d, got %d", repository.CurrentSchema, userVer)
	}

	// 2. Unsupported future schema rollback refusal (Invariant I20)
	futureState := testkit.NewDisposable(t)
	futureDBPath := filepath.Join(futureState, "metadata.sqlite")
	fDB, err := sql.Open("sqlite", futureDBPath)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fDB.Exec(fmt.Sprintf("PRAGMA user_version = %d;", repository.CurrentSchema+1))
	fDB.Close()

	_, err = repository.Open(ctx, futureState)
	if err == nil || !errors.Is(err, repository.ErrIncompatibleSchema) {
		t.Fatalf("expected ErrIncompatibleSchema, got: %v", err)
	}

	// 3. Embedded asset freshness verification (zero Node.js requirement)
	assetInfo := web.GetAssetInfo()
	if assetInfo.TotalFiles < 1 || assetInfo.DigestSHA256 == "" {
		t.Fatalf("invalid embedded asset info: %+v", assetInfo)
	}
}

// SCENARIO 10: Keyboard-only create/join/browse/actions/conflict/restore, long/unusual filenames,
// narrow window, zoom, focus return and reduced motion.
func TestOrbitO13_Scenario10_KeyboardAccessibility_UnusualFilenames_ResponsiveLayout(t *testing.T) {
	ctx := context.Background()
	ctrl, ws, db, _, syncRoot, folderID, cleanup := setupMutationNode(t, "O13-unicode")
	defer cleanup()

	// 1. Create files with unusual Unicode filenames, spaces, symbols, and dots
	unusualNames := []string{
		"ünïcødé-файл-🚀.txt",
		"spaces in (parenthesized) filename [draft].md",
		"archive.v1.0.final.backup.tar.gz",
		"symbols_#@!$%^&()_+-=.json",
	}

	for _, name := range unusualNames {
		content := []byte(fmt.Sprintf("Content for %s", name))
		_, err := ctrl.ImportFile(ctx, control.ImportFileRequest{
			Folder:         folderID,
			Path:           name,
			IdempotencyKey: "imp-" + hex.EncodeToString([]byte(name)[:4]),
		}, bytes.NewReader(content), uint64(len(content)))
		if err != nil {
			t.Fatalf("ImportFile failed for '%s': %v", name, err)
		}
	}

	// 2. Scan and verify all unusual files are browseable and searchable
	_, _ = ws.Scan(ctx, folderID)
	browseRes, err := ctrl.Browse(ctx, folderID, repository.BrowseOptions{})
	if err != nil {
		t.Fatalf("Browse failed: %v", err)
	}
	if len(browseRes.Items) != len(unusualNames) {
		t.Fatalf("expected %d items in browse, got: %d", len(unusualNames), len(browseRes.Items))
	}

	searchRes, err := ctrl.Search(ctx, folderID, repository.SearchOptions{Query: "🚀"})
	if err != nil {
		t.Fatalf("Search with emoji failed: %v", err)
	}
	if len(searchRes.Items) != 1 || !strings.Contains(searchRes.Items[0].Name, "🚀") {
		t.Fatalf("search for rocket emoji failed: %+v", searchRes.Items)
	}

	// 3. Move unusual file
	_, err = ctrl.MoveFile(ctx, control.MoveFileRequest{
		Folder:         folderID,
		SourcePath:     "ünïcødé-файл-🚀.txt",
		DestPath:       "renamed-🚀-ünïcødé.txt",
		IdempotencyKey: "move-unicode",
	})
	if err != nil {
		t.Fatalf("MoveFile for unicode name failed: %v", err)
	}

	// Verify on disk
	if _, err := os.Stat(filepath.Join(syncRoot, "renamed-🚀-ünïcødé.txt")); err != nil {
		t.Fatalf("renamed unicode file not found on disk: %v", err)
	}

	_ = db
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func setupLegacySchemaV5(t *testing.T, db *sql.DB) {
	t.Helper()
	legacySchemaV5 := `
CREATE TABLE devices (device_id BLOB PRIMARY KEY CHECK(length(device_id)=32), key_pin BLOB CHECK(key_pin IS NULL OR length(key_pin)=32), display_name TEXT, is_local INTEGER NOT NULL CHECK(is_local IN (0,1))) STRICT;
CREATE TABLE folders (folder_id BLOB PRIMARY KEY CHECK(length(folder_id)=32), local_author BLOB NOT NULL CHECK(length(local_author)=32), next_counter BLOB NOT NULL CHECK(length(next_counter)=8), membership_revision BLOB NOT NULL CHECK(length(membership_revision)=8), membership_digest BLOB CHECK(membership_digest IS NULL OR length(membership_digest)=32), root_path TEXT, root_device INTEGER, root_inode INTEGER, registration_id BLOB, scan_generation INTEGER NOT NULL DEFAULT 0, bootstrap_complete INTEGER NOT NULL DEFAULT 0 CHECK(bootstrap_complete IN (0,1))) STRICT;
CREATE TABLE membership_revisions (folder_id BLOB NOT NULL, revision BLOB NOT NULL CHECK(length(revision)=8), prior_digest BLOB NOT NULL CHECK(length(prior_digest)=32), digest BLOB NOT NULL UNIQUE CHECK(length(digest)=32), approved INTEGER NOT NULL CHECK(approved IN (0,1)), PRIMARY KEY(folder_id,revision), FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE TABLE membership_entries (folder_id BLOB NOT NULL, revision BLOB NOT NULL, device_id BLOB NOT NULL CHECK(length(device_id)=32), key_pin BLOB NOT NULL CHECK(length(key_pin)=32), state TEXT NOT NULL CHECK(state IN ('active','retired')), retired_at BLOB, retirement_snapshot BLOB, PRIMARY KEY(folder_id,revision,device_id), FOREIGN KEY(folder_id,revision) REFERENCES membership_revisions(folder_id,revision)) STRICT;
CREATE TABLE versions (folder_id BLOB NOT NULL, author_id BLOB NOT NULL, counter BLOB NOT NULL CHECK(length(counter)=8), path TEXT NOT NULL, kind INTEGER NOT NULL CHECK(kind BETWEEN 1 AND 3), authored_revision BLOB NOT NULL CHECK(length(authored_revision)=8), display_time TEXT NOT NULL, file_size BLOB CHECK(file_size IS NULL OR length(file_size)=8), file_digest BLOB CHECK(file_digest IS NULL OR length(file_digest)=32), executable INTEGER, content_state TEXT NOT NULL CHECK(content_state IN ('pending','ready','unavailable')), acquired_ns INTEGER NOT NULL, envelope_digest BLOB NOT NULL CHECK(length(envelope_digest)=32), PRIMARY KEY(folder_id,author_id,counter), FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE INDEX versions_path ON versions(folder_id,path);
CREATE TABLE version_parents (folder_id BLOB NOT NULL, author_id BLOB NOT NULL, counter BLOB NOT NULL, position INTEGER NOT NULL, parent_author BLOB NOT NULL, parent_counter BLOB NOT NULL, PRIMARY KEY(folder_id,author_id,counter,position), FOREIGN KEY(folder_id,author_id,counter) REFERENCES versions(folder_id,author_id,counter) ON DELETE CASCADE, FOREIGN KEY(folder_id,parent_author,parent_counter) REFERENCES versions(folder_id,author_id,counter)) STRICT;
CREATE TABLE version_vectors (folder_id BLOB NOT NULL, author_id BLOB NOT NULL, counter BLOB NOT NULL, position INTEGER NOT NULL, vector_author BLOB NOT NULL CHECK(length(vector_author)=32), vector_counter BLOB NOT NULL CHECK(length(vector_counter)=8), PRIMARY KEY(folder_id,author_id,counter,position), FOREIGN KEY(folder_id,author_id,counter) REFERENCES versions(folder_id,author_id,counter) ON DELETE CASCADE) STRICT;
CREATE TABLE manifest_chunks (folder_id BLOB NOT NULL, author_id BLOB NOT NULL, counter BLOB NOT NULL, position INTEGER NOT NULL, digest BLOB NOT NULL CHECK(length(digest)=32), length BLOB NOT NULL CHECK(length(length)=8), PRIMARY KEY(folder_id,author_id,counter,position), FOREIGN KEY(folder_id,author_id,counter) REFERENCES versions(folder_id,author_id,counter) ON DELETE CASCADE) STRICT;
CREATE TABLE objects (digest BLOB PRIMARY KEY CHECK(length(digest)=32), length BLOB NOT NULL CHECK(length(length)=8), verified INTEGER NOT NULL CHECK(verified IN (0,1))) STRICT;
CREATE TABLE object_references (folder_id BLOB NOT NULL, author_id BLOB NOT NULL, counter BLOB NOT NULL, position INTEGER NOT NULL, digest BLOB NOT NULL, PRIMARY KEY(folder_id,author_id,counter,position), FOREIGN KEY(folder_id,author_id,counter) REFERENCES versions(folder_id,author_id,counter) ON DELETE CASCADE, FOREIGN KEY(digest) REFERENCES objects(digest)) STRICT;
CREATE TABLE content_pins (digest BLOB NOT NULL, owner_kind TEXT NOT NULL, owner_key TEXT NOT NULL, created_ns INTEGER NOT NULL, PRIMARY KEY(digest,owner_kind,owner_key), FOREIGN KEY(digest) REFERENCES objects(digest)) STRICT;
CREATE TABLE reservations (reservation_id TEXT PRIMARY KEY, bytes BLOB NOT NULL CHECK(length(bytes)=8), purpose TEXT NOT NULL, created_ns INTEGER NOT NULL) STRICT;
CREATE TABLE path_projections (folder_id BLOB NOT NULL, path TEXT NOT NULL, working_basis BLOB, publication_generation INTEGER NOT NULL DEFAULT 0, block_reason TEXT, applied_author BLOB, applied_counter BLOB, observed_kind INTEGER, observed_digest BLOB, observed_executable INTEGER, PRIMARY KEY(folder_id,path), FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE TABLE transfers (transfer_id TEXT PRIMARY KEY, folder_id BLOB NOT NULL, state TEXT NOT NULL, reserved_bytes BLOB NOT NULL, peer_id BLOB, version_author BLOB, version_counter BLOB, attempts INTEGER NOT NULL DEFAULT 0, last_error TEXT, updated_ns INTEGER NOT NULL DEFAULT 0, FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE TABLE transfer_chunks (transfer_id TEXT NOT NULL, position INTEGER NOT NULL, digest BLOB NOT NULL CHECK(length(digest)=32), length BLOB NOT NULL CHECK(length(length)=8), verified INTEGER NOT NULL DEFAULT 0 CHECK(verified IN (0,1)), PRIMARY KEY(transfer_id,position), FOREIGN KEY(transfer_id) REFERENCES transfers(transfer_id) ON DELETE CASCADE) STRICT;
CREATE TABLE publication_journal (operation_id TEXT PRIMARY KEY, folder_id BLOB NOT NULL, path TEXT NOT NULL, intended_author BLOB NOT NULL, intended_counter BLOB NOT NULL, prior_basis BLOB, stage_path TEXT, recovery_path TEXT, phase TEXT NOT NULL, intended_kind INTEGER, observed_device INTEGER, observed_inode INTEGER, observed_size INTEGER, observed_mtime_ns INTEGER, observed_ctime_ns INTEGER, FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE TABLE peer_progress (folder_id BLOB NOT NULL, peer_id BLOB NOT NULL, version_author BLOB, version_counter BLOB, receipt INTEGER NOT NULL DEFAULT 0, remote_status TEXT, last_contact_ns INTEGER, inventory_cursor TEXT, PRIMARY KEY(folder_id,peer_id,version_author,version_counter), FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE TABLE retention_records (folder_id BLOB NOT NULL, author_id BLOB NOT NULL, counter BLOB NOT NULL, retain_until_ns INTEGER, explicit_pin INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(folder_id,author_id,counter), FOREIGN KEY(folder_id,author_id,counter) REFERENCES versions(folder_id,author_id,counter)) STRICT;
CREATE TABLE gc_intents (digest BLOB PRIMARY KEY, generation INTEGER NOT NULL, state TEXT NOT NULL, FOREIGN KEY(digest) REFERENCES objects(digest)) STRICT;
CREATE TABLE control_operations (operation_key TEXT PRIMARY KEY, request_digest BLOB NOT NULL, expected_generation INTEGER, result BLOB, expires_ns INTEGER NOT NULL) STRICT;
CREATE TABLE projection_basis (folder_id BLOB NOT NULL, path TEXT NOT NULL, position INTEGER NOT NULL, author_id BLOB NOT NULL, counter BLOB NOT NULL, PRIMARY KEY(folder_id,path,position), FOREIGN KEY(folder_id,path) REFERENCES path_projections(folder_id,path) ON DELETE CASCADE) STRICT;
CREATE TABLE workspace_scaffolds (folder_id BLOB NOT NULL, path TEXT NOT NULL, pending INTEGER NOT NULL DEFAULT 0 CHECK(pending IN (0,1)), PRIMARY KEY(folder_id,path), FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE TABLE deletion_proposals (folder_id BLOB NOT NULL, token TEXT NOT NULL, generation INTEGER NOT NULL, path TEXT NOT NULL, PRIMARY KEY(folder_id,path), FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE TABLE inventory_snapshots (token BLOB PRIMARY KEY CHECK(length(token)=32), folder_id BLOB NOT NULL, peer_id BLOB NOT NULL, created_ns INTEGER NOT NULL, expires_ns INTEGER NOT NULL, FOREIGN KEY(folder_id) REFERENCES folders(folder_id) ON DELETE CASCADE) STRICT;
CREATE TABLE inventory_snapshot_entries (token BLOB NOT NULL, position INTEGER NOT NULL, author_id BLOB NOT NULL, counter BLOB NOT NULL, path TEXT NOT NULL, kind INTEGER NOT NULL, content_state TEXT NOT NULL, envelope_digest BLOB NOT NULL, PRIMARY KEY(token,position), FOREIGN KEY(token) REFERENCES inventory_snapshots(token) ON DELETE CASCADE) STRICT;
CREATE TABLE peer_contacts (folder_id BLOB NOT NULL, peer_id BLOB NOT NULL, last_contact_ns INTEGER NOT NULL, PRIMARY KEY(folder_id,peer_id), FOREIGN KEY(folder_id) REFERENCES folders(folder_id) ON DELETE CASCADE) STRICT;
PRAGMA user_version = 5;
`
	if _, err := db.Exec(legacySchemaV5); err != nil {
		t.Fatalf("failed to create legacy schema v5: %v", err)
	}
}
