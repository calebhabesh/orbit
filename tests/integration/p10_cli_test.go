package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	"github.com/calebhabesh/orbit/internal/replication"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
	"github.com/calebhabesh/orbit/internal/workspace"
)

func TestP10StorageAccountingAndCLI(t *testing.T) {
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

	folder := strings.Repeat("a", 64)

	// 1. Initialize and register folder
	for _, args := range [][]string{
		{"init", "--state", state},
		{"register", "--state", state, "--folder", folder, "--root", root},
	} {
		if output, err := exec.Command(binary, append([]string{"engine"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
	}

	// 2. Test `orbit storage usage` text and JSON
	usageText, err := exec.Command(binary, "engine", "storage", "usage", "--state", state).CombinedOutput()
	if err != nil {
		t.Fatalf("storage usage text: %v\n%s", err, usageText)
	}
	if !strings.Contains(string(usageText), "state_filesystem:") || !strings.Contains(string(usageText), "database:") {
		t.Fatalf("unexpected storage usage text output:\n%s", usageText)
	}

	usageJSON, err := exec.Command(binary, "engine", "storage", "usage", "--state", state, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("storage usage json: %v\n%s", err, usageJSON)
	}
	var usageRes control.StorageUsageResult
	if err := json.Unmarshal(usageJSON, &usageRes); err != nil {
		t.Fatalf("unmarshal storage usage json: %v\n%s", err, usageJSON)
	}
	if usageRes.Usage.StateFilesystem.TotalBytes == 0 {
		t.Fatal("expected non-zero state filesystem total bytes")
	}
	if len(usageRes.Usage.Folders) != 1 {
		t.Fatalf("expected 1 folder in storage usage, got %d", len(usageRes.Usage.Folders))
	}

	// 3. Test `orbit storage recovery reclaim`
	scratchDir := filepath.Join(root, ".orbit-internal")
	orphanFile := filepath.Join(scratchDir, "recovery-op-orphan")
	orphanContent := []byte("orphaned recovery file content")
	if err := os.WriteFile(orphanFile, orphanContent, 0o600); err != nil {
		t.Fatal(err)
	}

	reclaimJSON, err := exec.Command(binary, "engine", "storage", "recovery", "reclaim", "--state", state, "--folder", folder, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("recovery reclaim: %v\n%s", err, reclaimJSON)
	}
	var reclaimRes control.ReclaimRecoveryResult
	if err := json.Unmarshal(reclaimJSON, &reclaimRes); err != nil {
		t.Fatalf("unmarshal reclaim json: %v\n%s", err, reclaimJSON)
	}
	if reclaimRes.ReclaimedCount != 1 {
		t.Fatalf("reclaimed count = %d, want 1", reclaimRes.ReclaimedCount)
	}
	if reclaimRes.ReclaimedBytes != uint64(len(orphanContent)) {
		t.Fatalf("reclaimed bytes = %d, want %d", reclaimRes.ReclaimedBytes, len(orphanContent))
	}
	if _, err := os.Stat(orphanFile); !os.IsNotExist(err) {
		t.Fatalf("orphan recovery file still exists after reclaim: %v", err)
	}
}

func TestP10RetentionExpiryAndCrashSafeGC(t *testing.T) {
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

	folder := strings.Repeat("b", 64)
	var folderID history.ID
	folderBytes, _ := hex.DecodeString(folder)
	copy(folderID[:], folderBytes)

	// 1. Init & Register A and B
	for _, args := range [][]string{
		{"init", "--state", stateA},
		{"init", "--state", stateB},
		{"register", "--state", stateA, "--folder", folder, "--root", rootA},
		{"register", "--state", stateB, "--folder", folder, "--root", rootB},
	} {
		if output, err := exec.Command(binary, append([]string{"engine"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
	}

	// 2. Export identities & certificates
	idOutA, err := exec.Command(binary, "engine", "identity", "--state", stateA, "--certificate").CombinedOutput()
	if err != nil {
		t.Fatalf("identity A: %v\n%s", err, idOutA)
	}
	devA, pinA, certA := parseIdentityOutput(t, idOutA)
	certPathA := filepath.Join(disposable, "peerA.pem")
	if err := os.WriteFile(certPathA, certA, 0o600); err != nil {
		t.Fatal(err)
	}

	idOutB, err := exec.Command(binary, "engine", "identity", "--state", stateB, "--certificate").CombinedOutput()
	if err != nil {
		t.Fatalf("identity B: %v\n%s", err, idOutB)
	}
	devB, pinB, certB := parseIdentityOutput(t, idOutB)
	certPathB := filepath.Join(disposable, "peerB.pem")
	if err := os.WriteFile(certPathB, certB, 0o600); err != nil {
		t.Fatal(err)
	}

	var parsedDevA, parsedDevB history.ID
	aBytes, _ := hex.DecodeString(devA)
	copy(parsedDevA[:], aBytes)
	bBytes, _ := hex.DecodeString(devB)
	copy(parsedDevB[:], bBytes)

	var parsedPinA, parsedPinB history.Digest
	paBytes, _ := hex.DecodeString(pinA)
	copy(parsedPinA[:], paBytes)
	pbBytes, _ := hex.DecodeString(pinB)
	copy(parsedPinB[:], pbBytes)

	// 3. Establish initial Membership {A, B}
	bundleRev1 := control.MembershipExportResult{
		Membership: protocol.Membership{
			Folder:   folderID,
			Revision: 1,
			Active: []protocol.ActiveMember{
				{Device: parsedDevA, KeyPin: parsedPinA},
				{Device: parsedDevB, KeyPin: parsedPinB},
			},
		},
	}
	bundleRev1Bytes, _ := json.Marshal(bundleRev1)
	bundlePath1 := filepath.Join(disposable, "membership_rev1.json")
	if err := os.WriteFile(bundlePath1, bundleRev1Bytes, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, st := range []string{stateA, stateB} {
		if out, err := exec.Command(binary, "engine", "membership", "import", "--state", st, "--folder", folder, "--file", bundlePath1, "--approve").CombinedOutput(); err != nil {
			t.Fatalf("membership import on %s: %v\n%s", st, err, out)
		}
	}

	// 4. Create version 1 on A: report.txt
	v1Content := []byte("version 1 initial report content for retention test")
	if err := os.WriteFile(filepath.Join(rootA, "report.txt"), v1Content, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(binary, "engine", "scan", "--state", stateA, "--folder", folder).CombinedOutput(); err != nil {
		t.Fatalf("scan A v1: %v\n%s", err, out)
	}

	// Sync A -> B (v1)
	serveCmdA1 := exec.Command(binary, "engine", "serve", "--state", stateA, "--peer-listen", "127.0.0.1:0")
	stdoutA1, err := serveCmdA1.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	serveCmdA1.Stderr = os.Stderr
	if err := serveCmdA1.Start(); err != nil {
		t.Fatalf("start serve A1: %v", err)
	}
	urlA1 := readListenerURL(t, stdoutA1)

	if out, err := exec.Command(binary, "engine", "sync", "--state", stateB, "--folder", folder, "--peer-url", urlA1, "--peer-device", devA, "--peer-certificate", certPathA, "--json").CombinedOutput(); err != nil {
		t.Fatalf("sync B v1: %v\n%s", err, out)
	}
	_ = serveCmdA1.Process.Kill()
	_ = serveCmdA1.Wait()

	// 5. Create version 2 on A: report.txt (supersedes v1)
	v2Content := []byte("version 2 updated head report content that supersedes v1")
	if err := os.WriteFile(filepath.Join(rootA, "report.txt"), v2Content, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(binary, "engine", "scan", "--state", stateA, "--folder", folder).CombinedOutput(); err != nil {
		t.Fatalf("scan A v2: %v\n%s", err, out)
	}

	// Sync A -> B (v2)
	serveCmdA2 := exec.Command(binary, "engine", "serve", "--state", stateA, "--peer-listen", "127.0.0.1:0")
	stdoutA2, err := serveCmdA2.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	serveCmdA2.Stderr = os.Stderr
	if err := serveCmdA2.Start(); err != nil {
		t.Fatalf("start serve A2: %v", err)
	}
	urlA2 := readListenerURL(t, stdoutA2)

	if out, err := exec.Command(binary, "engine", "sync", "--state", stateB, "--folder", folder, "--peer-url", urlA2, "--peer-device", devA, "--peer-certificate", certPathA, "--json").CombinedOutput(); err != nil {
		t.Fatalf("sync B v2: %v\n%s", err, out)
	}
	_ = serveCmdA2.Process.Kill()
	_ = serveCmdA2.Wait()

	// Verify Node B working copy has v2
	bReport, err := os.ReadFile(filepath.Join(rootB, "report.txt"))
	if err != nil || !bytes.Equal(bReport, v2Content) {
		t.Fatalf("Node B working copy mismatch: %v", err)
	}

	// 6. Test retention preview on B (default 30-day retention protects superseded v1)
	prevOutB, err := exec.Command(binary, "engine", "storage", "retention", "preview", "--state", stateB, "--folder", folder, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("retention preview B: %v\n%s", err, prevOutB)
	}
	var prevResB control.RetentionPreviewResult
	if err := json.Unmarshal(prevOutB, &prevResB); err != nil {
		t.Fatalf("unmarshal retention preview B: %v", err)
	}
	if prevResB.Preview.TotalVersions != 2 || prevResB.Preview.HeadVersions != 1 || prevResB.Preview.ExpiredVersions != 0 {
		t.Fatalf("unexpected retention preview on B: %+v", prevResB.Preview)
	}

	// 7. Change retention policy to 0 days, 0 min-superseded
	changeOutB, err := exec.Command(binary, "engine", "storage", "retention", "change", "--state", stateB, "--folder", folder,
		"--retention-days", "0", "--min-superseded", "0", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("retention change B: %v\n%s", err, changeOutB)
	}
	var changeResB control.RetentionChangeResult
	if err := json.Unmarshal(changeOutB, &changeResB); err != nil {
		t.Fatalf("unmarshal retention change B: %v", err)
	}
	if changeResB.Policy.RetentionDays != 0 || changeResB.Policy.MinSuperseded != 0 {
		t.Fatalf("unexpected policy on B: %+v", changeResB.Policy)
	}

	// 8. Preview GC on B: now candidate_chunks must be 1 (v1's chunk)
	gcPrevOutB, err := exec.Command(binary, "engine", "storage", "gc", "preview", "--state", stateB, "--folder", folder, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("gc preview B: %v\n%s", err, gcPrevOutB)
	}
	var gcPrevResB control.GCPreviewResult
	if err := json.Unmarshal(gcPrevOutB, &gcPrevResB); err != nil {
		t.Fatalf("unmarshal gc preview B: %v", err)
	}
	if gcPrevResB.Report.Candidates != 1 {
		t.Fatalf("expected 1 GC candidate on B, got %d", gcPrevResB.Report.Candidates)
	}

	// 9. Run GC on B with idempotency key
	key := "gc-test-run-p10"
	gcRunOut1, err := exec.Command(binary, "engine", "storage", "gc", "run", "--state", stateB, "--folder", folder,
		"--idempotency-key", key, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("gc run B: %v\n%s", err, gcRunOut1)
	}
	var gcRunRes1 control.GCRunResult
	if err := json.Unmarshal(gcRunOut1, &gcRunRes1); err != nil {
		t.Fatalf("unmarshal gc run B: %v", err)
	}
	if gcRunRes1.Report.UnlinkedObjects != 1 || gcRunRes1.Replay {
		t.Fatalf("unexpected gc run result: %+v", gcRunRes1)
	}

	// Replay GC run with same idempotency key: must be replay=true
	gcRunOut2, err := exec.Command(binary, "engine", "storage", "gc", "run", "--state", stateB, "--folder", folder,
		"--idempotency-key", key, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("gc replay B: %v\n%s", err, gcRunOut2)
	}
	var gcRunRes2 control.GCRunResult
	if err := json.Unmarshal(gcRunOut2, &gcRunRes2); err != nil {
		t.Fatalf("unmarshal gc replay B: %v", err)
	}
	if !gcRunRes2.Replay {
		t.Fatal("expected second GC run to be replay")
	}

	// 10. Verify post-GC state on Node B:
	// Current head v2 is still intact and valid on disk
	bReportAfter, err := os.ReadFile(filepath.Join(rootB, "report.txt"))
	if err != nil || !bytes.Equal(bReportAfter, v2Content) {
		t.Fatalf("current head content altered or missing after GC: %v", err)
	}

	// Check history: causal metadata for both v1 and v2 is STILL preserved!
	histOutB, err := exec.Command(binary, "engine", "history", "--state", stateB, "--folder", folder, "--path", "report.txt", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("history B: %v\n%s", err, histOutB)
	}
	var histResB []control.HistoryItem
	if err := json.Unmarshal(histOutB, &histResB); err != nil {
		t.Fatalf("unmarshal history B: %v", err)
	}
	if len(histResB) != 2 {
		t.Fatalf("expected 2 historical versions in history, got %d", len(histResB))
	}
}

func TestP10LongOfflinePeerNoResurrectedDeletions(t *testing.T) {
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
	stateC := filepath.Join(disposable, "state-c")
	rootC := filepath.Join(disposable, "root-c")

	for _, dir := range []string{stateA, rootA, stateB, rootB, stateC, rootC} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	folder := strings.Repeat("c", 64)
	var folderID history.ID
	folderBytes, _ := hex.DecodeString(folder)
	copy(folderID[:], folderBytes)

	// Initialize A, B, C
	for _, args := range [][]string{
		{"init", "--state", stateA},
		{"init", "--state", stateB},
		{"init", "--state", stateC},
		{"register", "--state", stateA, "--folder", folder, "--root", rootA},
		{"register", "--state", stateB, "--folder", folder, "--root", rootB},
		{"register", "--state", stateC, "--folder", folder, "--root", rootC},
	} {
		if output, err := exec.Command(binary, append([]string{"engine"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
	}

	idOutA, _ := exec.Command(binary, "engine", "identity", "--state", stateA, "--certificate").CombinedOutput()
	devA, pinA, certA := parseIdentityOutput(t, idOutA)
	certPathA := filepath.Join(disposable, "peerA.pem")
	_ = os.WriteFile(certPathA, certA, 0o600)

	idOutB, _ := exec.Command(binary, "engine", "identity", "--state", stateB, "--certificate").CombinedOutput()
	devB, pinB, certB := parseIdentityOutput(t, idOutB)
	certPathB := filepath.Join(disposable, "peerB.pem")
	_ = os.WriteFile(certPathB, certB, 0o600)

	idOutC, _ := exec.Command(binary, "engine", "identity", "--state", stateC, "--certificate").CombinedOutput()
	devC, pinC, certC := parseIdentityOutput(t, idOutC)
	certPathC := filepath.Join(disposable, "peerC.pem")
	_ = os.WriteFile(certPathC, certC, 0o600)

	var pDevA, pDevB, pDevC history.ID
	aB, _ := hex.DecodeString(devA)
	copy(pDevA[:], aB)
	bB, _ := hex.DecodeString(devB)
	copy(pDevB[:], bB)
	cB, _ := hex.DecodeString(devC)
	copy(pDevC[:], cB)

	var pPinA, pPinB, pPinC history.Digest
	paB, _ := hex.DecodeString(pinA)
	copy(pPinA[:], paB)
	pbB, _ := hex.DecodeString(pinB)
	copy(pPinB[:], pbB)
	pcB, _ := hex.DecodeString(pinC)
	copy(pPinC[:], pcB)

	bundleRev1 := control.MembershipExportResult{
		Membership: protocol.Membership{
			Folder:   folderID,
			Revision: 1,
			Active: []protocol.ActiveMember{
				{Device: pDevA, KeyPin: pPinA},
				{Device: pDevB, KeyPin: pPinB},
				{Device: pDevC, KeyPin: pPinC},
			},
		},
	}
	bundleBytes, _ := json.Marshal(bundleRev1)
	bundlePath := filepath.Join(disposable, "membership_rev1.json")
	_ = os.WriteFile(bundlePath, bundleBytes, 0o600)

	for _, st := range []string{stateA, stateB, stateC} {
		if out, err := exec.Command(binary, "engine", "membership", "import", "--state", st, "--folder", folder, "--file", bundlePath, "--approve").CombinedOutput(); err != nil {
			t.Fatalf("import: %v\n%s", err, out)
		}
	}

	// 1. Create delete_me.txt on Node A
	delContent := []byte("this file will be deleted while C is offline")
	if err := os.WriteFile(filepath.Join(rootA, "delete_me.txt"), delContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(binary, "engine", "scan", "--state", stateA, "--folder", folder).CombinedOutput(); err != nil {
		t.Fatalf("scan A: %v\n%s", err, out)
	}

	// Sync to B and C
	serveCmdA := exec.Command(binary, "engine", "serve", "--state", stateA, "--peer-listen", "127.0.0.1:0")
	stdoutA, _ := serveCmdA.StdoutPipe()
	serveCmdA.Stderr = os.Stderr
	_ = serveCmdA.Start()
	urlA := readListenerURL(t, stdoutA)

	if out, err := exec.Command(binary, "engine", "sync", "--state", stateB, "--folder", folder, "--peer-url", urlA, "--peer-device", devA, "--peer-certificate", certPathA).CombinedOutput(); err != nil {
		t.Fatalf("sync B: %v\n%s", err, out)
	}
	if out, err := exec.Command(binary, "engine", "sync", "--state", stateC, "--folder", folder, "--peer-url", urlA, "--peer-device", devA, "--peer-certificate", certPathA).CombinedOutput(); err != nil {
		t.Fatalf("sync C: %v\n%s", err, out)
	}
	_ = serveCmdA.Process.Kill()
	_ = serveCmdA.Wait()

	// Verify C has delete_me.txt
	if _, err := os.Stat(filepath.Join(rootC, "delete_me.txt")); err != nil {
		t.Fatalf("delete_me.txt missing from C: %v", err)
	}

	// 2. Node C is now offline!
	// Node A deletes delete_me.txt
	if err := os.Remove(filepath.Join(rootA, "delete_me.txt")); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(binary, "engine", "scan", "--state", stateA, "--folder", folder).CombinedOutput(); err != nil {
		t.Fatalf("scan A delete: %v\n%s", err, out)
	}

	// Sync A -> B (B receives tombstone)
	serveCmdA2 := exec.Command(binary, "engine", "serve", "--state", stateA, "--peer-listen", "127.0.0.1:0")
	stdoutA2, _ := serveCmdA2.StdoutPipe()
	serveCmdA2.Stderr = os.Stderr
	_ = serveCmdA2.Start()
	urlA2 := readListenerURL(t, stdoutA2)

	if out, err := exec.Command(binary, "engine", "sync", "--state", stateB, "--folder", folder, "--peer-url", urlA2, "--peer-device", devA, "--peer-certificate", certPathA).CombinedOutput(); err != nil {
		t.Fatalf("sync B delete: %v\n%s", err, out)
	}
	_ = serveCmdA2.Process.Kill()
	_ = serveCmdA2.Wait()

	// Node B runs aggressive GC
	_, _ = exec.Command(binary, "engine", "storage", "retention", "change", "--state", stateB, "--folder", folder,
		"--retention-days", "0", "--min-superseded", "0").CombinedOutput()
	_, _ = exec.Command(binary, "engine", "storage", "gc", "run", "--state", stateB, "--folder", folder).CombinedOutput()

	// 3. Node C comes back online and syncs with Node B!
	serveCmdB := exec.Command(binary, "engine", "serve", "--state", stateB, "--peer-listen", "127.0.0.1:0")
	stdoutB, _ := serveCmdB.StdoutPipe()
	serveCmdB.Stderr = os.Stderr
	_ = serveCmdB.Start()
	urlB := readListenerURL(t, stdoutB)

	// C syncs from B: B provides the tombstone
	if out, err := exec.Command(binary, "engine", "sync", "--state", stateC, "--folder", folder, "--peer-url", urlB, "--peer-device", devB, "--peer-certificate", certPathB).CombinedOutput(); err != nil {
		t.Fatalf("sync C from B: %v\n%s", err, out)
	}
	_ = serveCmdB.Process.Kill()
	_ = serveCmdB.Wait()

	// 4. Invariant Check (I10, I15):
	// Tombstone dominates C's old version. delete_me.txt must be deleted on C and must NOT be resurrected on B!
	if _, err := os.Stat(filepath.Join(rootB, "delete_me.txt")); !os.IsNotExist(err) {
		t.Fatalf("delete_me.txt was resurrected on Node B! stat err: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootC, "delete_me.txt")); !os.IsNotExist(err) {
		t.Fatalf("delete_me.txt was NOT deleted on Node C! stat err: %v", err)
	}
}

func TestP10InterruptedGCFaultRecovery(t *testing.T) {
	ctx := context.Background()
	state := t.TempDir()

	// 1. Crash after HookGCIntent
	var intentRan bool
	db1, err := repository.OpenWithOptions(ctx, state, repository.Options{
		FaultHook: func(name string) error {
			if name == repository.HookGCIntent {
				intentRan = true
				return fmt.Errorf("injected crash at HookGCIntent")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	folder, author := repositoryID('F'), repositoryID('A')
	if err := db1.EnsureFolder(ctx, folder, author, 1); err != nil {
		t.Fatal(err)
	}

	orphanVal := []byte("orphan content for fault test")
	orphanDigest := sha256.Sum256(orphanVal)
	if err := db1.InstallChunk(ctx, orphanDigest, uint64(len(orphanVal)), bytes.NewReader(orphanVal)); err != nil {
		t.Fatal(err)
	}

	zeroPolicy := repository.RetentionPolicy{RetentionDays: 0, MinSuperseded: 0}
	_, err = db1.RunGC(ctx, folder, &zeroPolicy, time.Now().Add(time.Hour))
	if err == nil || !intentRan {
		t.Fatalf("expected HookGCIntent fault, got %v", err)
	}
	db1.Close()

	// Reopen state - startup reconciliation should safely clear intent and retain object
	db2, err := repository.OpenWithOptions(ctx, state, repository.Options{})
	if err != nil {
		t.Fatal(err)
	}
	avail, err := db2.VerifiedChunk(ctx, history.Chunk{Digest: orphanDigest, Length: uint64(len(orphanVal))})
	if err != nil || !avail {
		t.Fatalf("object lost after intent crash: avail=%v err=%v", avail, err)
	}
	db2.Close()

	// 2. Crash after HookGCUnlink
	var unlinkRan bool
	db3, err := repository.OpenWithOptions(ctx, state, repository.Options{
		FaultHook: func(name string) error {
			if name == repository.HookGCUnlink {
				unlinkRan = true
				return fmt.Errorf("injected crash at HookGCUnlink")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = db3.RunGC(ctx, folder, &zeroPolicy, time.Now().Add(time.Hour))
	if err == nil || !unlinkRan {
		t.Fatalf("expected HookGCUnlink fault, got %v", err)
	}
	db3.Close()

	// Reopen state - startup reconciliation must finalize unlinked unreferenced object
	db4, err := repository.OpenWithOptions(ctx, state, repository.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer db4.Close()

	availAfter, err := db4.VerifiedChunk(ctx, history.Chunk{Digest: orphanDigest, Length: uint64(len(orphanVal))})
	if err != nil {
		t.Fatal(err)
	}
	if availAfter {
		t.Fatal("expected unlinked object to be finalized on recovery")
	}
}

type fallbackMockClient struct {
	replication.PeerClient
	missingDigest history.Digest
}

func (m *fallbackMockClient) Chunk(ctx context.Context, req replication.ChunkRequest, chunk history.Chunk) ([]byte, error) {
	if chunk.Digest == m.missingDigest {
		return nil, &replication.WireError{
			Status: 410,
			Body:   replication.ErrorResponse{Code: "CONTENT_EXPIRED", Message: "chunk expired on primary", Retryable: false},
		}
	}
	return m.PeerClient.Chunk(ctx, req, chunk)
}

func TestP10MultiPeerFallbackChunkTransfer(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)

	stateB := filepath.Join(disposable, "state-b")
	stateC := filepath.Join(disposable, "state-c")
	stateD := filepath.Join(disposable, "state-d")
	rootD := filepath.Join(disposable, "root-d")
	for _, dir := range []string{stateB, stateC, stateD, rootD} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	dbB, err := repository.Open(ctx, stateB)
	if err != nil {
		t.Fatal(err)
	}
	defer dbB.Close()

	dbC, err := repository.Open(ctx, stateC)
	if err != nil {
		t.Fatal(err)
	}
	defer dbC.Close()

	dbD, err := repository.Open(ctx, stateD)
	if err != nil {
		t.Fatal(err)
	}
	defer dbD.Close()

	wsD := workspace.New(dbD, workspace.Options{})
	folder := repositoryID('F')
	devB, devC, devD := repositoryID('B'), repositoryID('C'), repositoryID('D')

	if err := dbB.EnsureFolder(ctx, folder, devB, 1); err != nil {
		t.Fatal(err)
	}
	if err := dbC.EnsureFolder(ctx, folder, devC, 1); err != nil {
		t.Fatal(err)
	}
	if err := dbD.EnsureFolder(ctx, folder, devD, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := wsD.Register(ctx, folder, rootD); err != nil {
		t.Fatal(err)
	}

	membership := protocol.Membership{
		Folder:   folder,
		Revision: 1,
		Active: []protocol.ActiveMember{
			{Device: devB, KeyPin: history.Digest(repositoryID('1'))},
			{Device: devC, KeyPin: history.Digest(repositoryID('2'))},
			{Device: devD, KeyPin: history.Digest(repositoryID('3'))},
		},
	}
	var appMem repository.ApprovedMembership
	for _, db := range []*repository.DB{dbB, dbC, dbD} {
		var err error
		appMem, err = db.ApproveMembership(ctx, membership)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Create file on C
	content := []byte("multi peer fallback transfer content")
	chunkDigest := sha256.Sum256(content)
	manifest, err := dbC.StoreFile(ctx, bytes.NewReader(content), false)
	if err != nil {
		t.Fatal(err)
	}
	v, err := dbC.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "fallback_item.txt",
		Kind:             history.KindFile,
		Manifest:         manifest,
		AuthoredRevision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Import metadata onto B and D
	env, err := dbC.Envelope(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := dbB.ImportMetadata(ctx, env); err != nil {
		t.Fatal(err)
	}
	if err := dbD.ImportMetadata(ctx, env); err != nil {
		t.Fatal(err)
	}

	// Node C has the chunk installed. Node B does NOT have the chunk (expired / missing).
	// Mock client for C that has the chunk
	clientC := &mockPeerClient{
		chunkFunc: func(_ context.Context, _ replication.ChunkRequest, ch history.Chunk) ([]byte, error) {
			if ch.Digest == chunkDigest {
				return content, nil
			}
			return nil, fmt.Errorf("chunk not found: %x", ch.Digest)
		},
	}

	// Primary client B claims to have expired the chunk
	clientB := &fallbackMockClient{
		PeerClient:    clientC,
		missingDigest: chunkDigest,
	}

	// 1. Without fallback, syncer should fail to get the chunk
	noFallbackSyncer := replication.NewSyncer(
		dbD,
		wsD,
		clientB,
		devD,
		devB,
		folder,
		appMem,
		replication.TransferOptions{Retries: 1},
	)
	if _, err := noFallbackSyncer.Sync(ctx); err == nil {
		t.Fatal("expected sync to fail when primary has expired chunk and no fallback provided")
	}

	// 2. With fallback client C, syncer queries primary B, gets expired, falls back to C, and succeeds!
	syncer := replication.NewSyncer(
		dbD,
		wsD,
		clientB,
		devD,
		devB,
		folder,
		appMem,
		replication.TransferOptions{
			Retries:   1,
			Fallbacks: []replication.PeerClient{clientC},
		},
	)
	res, err := syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("sync with fallback failed: %v", err)
	}
	if res.ChunksFetched != 1 {
		t.Fatalf("chunks fetched = %d, want 1", res.ChunksFetched)
	}
	if res.VersionsApplied != 1 {
		t.Fatalf("versions applied = %d, want 1", res.VersionsApplied)
	}

	// Verify working tree has the file applied
	got, err := os.ReadFile(filepath.Join(rootD, "fallback_item.txt"))
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("content mismatch on D: %v", err)
	}
}

type mockPeerClient struct {
	chunkFunc func(context.Context, replication.ChunkRequest, history.Chunk) ([]byte, error)
}

func (m *mockPeerClient) Hello(_ context.Context, _ replication.HelloRequest) (replication.HelloResponse, error) {
	devB := repositoryID('B')
	return replication.HelloResponse{ProtocolVersion: replication.ProtocolVersion, DeviceID: hex.EncodeToString(devB[:])}, nil
}

func (m *mockPeerClient) Inventory(_ context.Context, _ replication.InventoryRequest) (replication.InventoryResponse, error) {
	devC := repositoryID('C')
	return replication.InventoryResponse{
		Done: true,
		Entries: []replication.InventoryEntry{
			{
				AuthorID:       hex.EncodeToString(devC[:]),
				Counter:        "1",
				Availability:   "ready",
				EnvelopeDigest: hex.EncodeToString(make([]byte, 32)),
			},
		},
	}, nil
}

func (m *mockPeerClient) Versions(_ context.Context, _ replication.VersionsRequest) (replication.VersionsResponse, error) {
	return replication.VersionsResponse{}, nil
}

func (m *mockPeerClient) Chunk(ctx context.Context, req replication.ChunkRequest, chunk history.Chunk) ([]byte, error) {
	if m.chunkFunc != nil {
		return m.chunkFunc(ctx, req, chunk)
	}
	return nil, errors.New("chunk not found")
}

func (m *mockPeerClient) Receipts(_ context.Context, req replication.ReceiptsRequest) (replication.ReceiptsResponse, error) {
	return replication.ReceiptsResponse{Accepted: req.Versions}, nil
}

func (m *mockPeerClient) Status(_ context.Context, _ replication.StatusRequest) (replication.StatusResponse, error) {
	return replication.StatusResponse{}, nil
}

func (m *mockPeerClient) MembershipGet(_ context.Context, _ replication.MembershipGetRequest) (replication.MembershipGetResponse, error) {
	return replication.MembershipGetResponse{}, nil
}

func repositoryID(fill byte) history.ID {
	var id history.ID
	for i := range id {
		id[i] = fill
	}
	return id
}
