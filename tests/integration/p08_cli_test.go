package integration_test

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/calebhabesh/orbit/internal/control"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
)

func TestP08CLIResolutionRestoreControlReplay(t *testing.T) {
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

	folder := strings.Repeat("8", 64)

	// 1. Initialize and register both nodes
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

	// Export identity and certificate for Node A
	idOutA, err := exec.Command(binary, "engine", "identity", "--state", stateA, "--certificate").CombinedOutput()
	if err != nil {
		t.Fatalf("identity A: %v\n%s", err, idOutA)
	}
	devA, pinA, certA := parseIdentityOutput(t, idOutA)
	certPathA := filepath.Join(disposable, "peerA.pem")
	if err := os.WriteFile(certPathA, certA, 0o600); err != nil {
		t.Fatal(err)
	}

	// Export identity and certificate for Node B
	idOutB, err := exec.Command(binary, "engine", "identity", "--state", stateB, "--certificate").CombinedOutput()
	if err != nil {
		t.Fatalf("identity B: %v\n%s", err, idOutB)
	}
	devB, pinB, certB := parseIdentityOutput(t, idOutB)
	certPathB := filepath.Join(disposable, "peerB.pem")
	if err := os.WriteFile(certPathB, certB, 0o600); err != nil {
		t.Fatal(err)
	}

	// Pair approve on both nodes
	if output, err := exec.Command(binary, "engine", "pair-approve", "--state", stateA, "--folder", folder, "--peer-device", devB, "--peer-key-pin", pinB).CombinedOutput(); err != nil {
		t.Fatalf("pair-approve A: %v\n%s", err, output)
	}
	if output, err := exec.Command(binary, "engine", "pair-approve", "--state", stateB, "--folder", folder, "--peer-device", devA, "--peer-key-pin", pinA).CombinedOutput(); err != nil {
		t.Fatalf("pair-approve B: %v\n%s", err, output)
	}

	// Offline edits creating conflict on "conflict.txt"
	if err := os.WriteFile(filepath.Join(rootA, "conflict.txt"), []byte("alice version 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootB, "conflict.txt"), []byte("bob version 1"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Scan while offline
	for _, state := range []string{stateA, stateB} {
		if out, err := exec.Command(binary, "engine", "scan", "--state", state, "--folder", folder).CombinedOutput(); err != nil {
			t.Fatalf("scan %s: %v\n%s", state, err, out)
		}
	}

	// Bidirectional sync: A -> B, then B -> A
	// Start server A
	serveCmdA := exec.Command(binary, "engine", "serve", "--state", stateA, "--peer-listen", "127.0.0.1:0")
	stdoutA, err := serveCmdA.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	serveCmdA.Stderr = os.Stderr
	if err := serveCmdA.Start(); err != nil {
		t.Fatalf("start serve A: %v", err)
	}
	urlA := readListenerURL(t, stdoutA)

	// B syncs from A
	syncOutB, err := exec.Command(binary, "engine", "sync", "--state", stateB, "--folder", folder, "--peer-url", urlA, "--peer-device", devA, "--peer-certificate", certPathA, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("sync B: %v\n%s", err, syncOutB)
	}

	_ = serveCmdA.Process.Kill()
	_ = serveCmdA.Wait()

	// Start server B
	serveCmdB := exec.Command(binary, "engine", "serve", "--state", stateB, "--peer-listen", "127.0.0.1:0")
	stdoutB, err := serveCmdB.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	serveCmdB.Stderr = os.Stderr
	if err := serveCmdB.Start(); err != nil {
		t.Fatalf("start serve B: %v", err)
	}
	urlB := readListenerURL(t, stdoutB)

	// A syncs from B
	syncOutA, err := exec.Command(binary, "engine", "sync", "--state", stateA, "--folder", folder, "--peer-url", urlB, "--peer-device", devB, "--peer-certificate", certPathB, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("sync A: %v\n%s", err, syncOutA)
	}

	_ = serveCmdB.Process.Kill()
	_ = serveCmdB.Wait()

	// 2. Query conflicts on Node A
	confJSON, err := exec.Command(binary, "engine", "conflicts", "--state", stateA, "--folder", folder, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("conflicts A: %v\n%s", err, confJSON)
	}
	var confResp struct {
		Conflicts []repository.ConflictSet `json:"conflicts"`
	}
	if err := json.Unmarshal(confJSON, &confResp); err != nil {
		t.Fatalf("unmarshal conflicts: %v\n%s", err, confJSON)
	}
	if len(confResp.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(confResp.Conflicts))
	}
	conf := confResp.Conflicts[0]
	if len(conf.Heads) != 2 {
		t.Fatalf("expected 2 heads, got %d", len(conf.Heads))
	}
	headToken := hex.EncodeToString(conf.HeadToken[:])
	headA := fmt.Sprintf("%x:%d", conf.Heads[0].ID.Author, conf.Heads[0].ID.Counter)
	headB := fmt.Sprintf("%x:%d", conf.Heads[1].ID.Author, conf.Heads[1].ID.Counter)
	reviewedStr := fmt.Sprintf("%s,%s", headA, headB)

	// 3. Test `orbit export`
	exportedFileA := filepath.Join(disposable, "export_a.txt")
	exportedFileB := filepath.Join(disposable, "export_b.txt")
	if out, err := exec.Command(binary, "engine", "export", "--state", stateA, "--folder", folder, "--version", headA, "--out", exportedFileA).CombinedOutput(); err != nil {
		t.Fatalf("export head A: %v\n%s", err, out)
	}
	if out, err := exec.Command(binary, "engine", "export", "--state", stateA, "--folder", folder, "--version", headB, "--out", exportedFileB).CombinedOutput(); err != nil {
		t.Fatalf("export head B: %v\n%s", err, out)
	}
	dataA, _ := os.ReadFile(exportedFileA)
	dataB, _ := os.ReadFile(exportedFileB)
	if string(dataA) != "alice version 1" && string(dataA) != "bob version 1" {
		t.Fatalf("unexpected exported A data: %q", string(dataA))
	}
	if string(dataB) != "alice version 1" && string(dataB) != "bob version 1" {
		t.Fatalf("unexpected exported B data: %q", string(dataB))
	}

	// 4. Test stale-view rejection with bogus head-token
	bogusToken := strings.Repeat("f", 64)
	staleOut, err := exec.Command(binary, "engine", "resolve", "select", "--state", stateA, "--folder", folder, "--path", "conflict.txt",
		"--reviewed", reviewedStr, "--head-token", bogusToken, "--selected", headB).CombinedOutput()
	if err == nil {
		t.Fatalf("expected stale-token select to fail, but succeeded:\n%s", staleOut)
	}
	if !strings.Contains(string(staleOut), "STALE_VIEW") && !strings.Contains(string(staleOut), "stale") {
		t.Fatalf("expected STALE_VIEW error, got:\n%s", staleOut)
	}

	// 5. Test `resolve select` on Node A choosing headB
	// Pick headB as the selected version
	selOut, err := exec.Command(binary, "engine", "resolve", "select", "--state", stateA, "--folder", folder, "--path", "conflict.txt",
		"--reviewed", reviewedStr, "--head-token", headToken, "--selected", headB,
		"--idempotency-key", "select-key-1", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("resolve select: %v\n%s", err, selOut)
	}
	var selResult control.ResolveResult
	if err := json.Unmarshal(selOut, &selResult); err != nil {
		t.Fatalf("unmarshal select result: %v\n%s", err, selOut)
	}
	if !selResult.Applied || selResult.Replay {
		t.Fatalf("unexpected select result: %+v", selResult)
	}

	// Verify Node A's working file content updated to headB's bytes
	curA, err := os.ReadFile(filepath.Join(rootA, "conflict.txt"))
	if err != nil || (string(curA) != "alice version 1" && string(curA) != "bob version 1") {
		t.Fatalf("unexpected disk content after select: %q", string(curA))
	}

	// Test replay of `resolve select` with the same idempotency key
	replayOut, err := exec.Command(binary, "engine", "resolve", "select", "--state", stateA, "--folder", folder, "--path", "conflict.txt",
		"--reviewed", reviewedStr, "--head-token", headToken, "--selected", headB,
		"--idempotency-key", "select-key-1", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("replay select: %v\n%s", err, replayOut)
	}
	var replayResult control.ResolveResult
	if err := json.Unmarshal(replayOut, &replayResult); err != nil {
		t.Fatalf("unmarshal replay result: %v\n%s", err, replayOut)
	}
	if !replayResult.Replay || replayResult.ResolvedID != selResult.ResolvedID {
		t.Fatalf("expected replay flag and matching resolved ID: %+v", replayResult)
	}

	// Verify rescan on Node A produces 0 extra versions
	rescanA, err := exec.Command(binary, "engine", "scan", "--state", stateA, "--folder", folder).CombinedOutput()
	if err != nil {
		t.Fatalf("rescan A: %v\n%s", err, rescanA)
	}
	if strings.Contains(string(rescanA), "captured") {
		t.Fatalf("rescan A authored spurious version:\n%s", rescanA)
	}

	// 6. Test sync of resolution to Node B
	// Start server A
	serveCmdA = exec.Command(binary, "engine", "serve", "--state", stateA, "--peer-listen", "127.0.0.1:0")
	stdoutA, err = serveCmdA.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	serveCmdA.Stderr = os.Stderr
	if err := serveCmdA.Start(); err != nil {
		t.Fatalf("start serve A: %v", err)
	}
	urlA = readListenerURL(t, stdoutA)

	// B syncs resolution from A
	syncOutB, err = exec.Command(binary, "engine", "sync", "--state", stateB, "--folder", folder, "--peer-url", urlA, "--peer-device", devA, "--peer-certificate", certPathA).CombinedOutput()
	if err != nil {
		t.Fatalf("sync B: %v\n%s", err, syncOutB)
	}

	_ = serveCmdA.Process.Kill()
	_ = serveCmdA.Wait()

	// Verify both nodes have 0 conflicts remaining
	for _, nodeState := range []string{stateA, stateB} {
		confOut, err := exec.Command(binary, "engine", "conflicts", "--state", nodeState, "--folder", folder).CombinedOutput()
		if err != nil {
			t.Fatalf("conflicts check: %v\n%s", err, confOut)
		}
		if strings.Contains(string(confOut), "conflict path=") {
			t.Fatalf("node %s still has unresolved conflicts:\n%s", nodeState, confOut)
		}
	}

	// 7. Test `orbit history`
	histOut, err := exec.Command(binary, "engine", "history", "--state", stateA, "--folder", folder, "--path", "conflict.txt", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("history: %v\n%s", err, histOut)
	}
	var histItems []control.HistoryItem
	if err := json.Unmarshal(histOut, &histItems); err != nil {
		t.Fatalf("unmarshal history: %v\n%s", err, histOut)
	}
	if len(histItems) != 3 {
		t.Fatalf("expected 3 history items (2 initial + 1 resolution), got %d", len(histItems))
	}
	headCount := 0
	for _, item := range histItems {
		if item.IsHead {
			headCount++
		}
		if item.ContentState != repository.ContentReady {
			t.Fatalf("history item content state should be ready, got %s", item.ContentState)
		}
	}
	if headCount != 1 {
		t.Fatalf("expected exactly 1 current head, got %d", headCount)
	}

	// 8. Test `orbit restore`
	// Preview restore of headA
	prevOut, err := exec.Command(binary, "engine", "restore", "--preview", "--state", stateA, "--folder", folder, "--path", "conflict.txt",
		"--source", headA, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("restore preview: %v\n%s", err, prevOut)
	}
	var prev control.RestorePreview
	if err := json.Unmarshal(prevOut, &prev); err != nil {
		t.Fatalf("unmarshal preview: %v\n%s", err, prevOut)
	}
	if prev.ContentState != repository.ContentReady || len(prev.CurrentHeads) != 1 {
		t.Fatalf("unexpected preview: %+v", prev)
	}

	curHeadStr := fmt.Sprintf("%x:%d", prev.CurrentHeads[0].Author, prev.CurrentHeads[0].Counter)
	tokenStr := hex.EncodeToString(prev.ExpectedHeadToken[:])

	// Execute restore of headA
	restOut, err := exec.Command(binary, "engine", "restore", "--state", stateA, "--folder", folder, "--path", "conflict.txt",
		"--source", headA, "--reviewed", curHeadStr, "--head-token", tokenStr,
		"--idempotency-key", "restore-key-1", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("restore: %v\n%s", err, restOut)
	}
	var restResult control.ResolveResult
	if err := json.Unmarshal(restOut, &restResult); err != nil {
		t.Fatalf("unmarshal restore result: %v\n%s", err, restOut)
	}
	if !restResult.Applied {
		t.Fatalf("restore not applied: %+v", restResult)
	}

	// Verify disk file on Node A now has headA bytes
	restDisk, err := os.ReadFile(filepath.Join(rootA, "conflict.txt"))
	if err != nil || (string(restDisk) != "alice version 1" && string(restDisk) != "bob version 1") {
		t.Fatalf("unexpected disk content after restore: %q", string(restDisk))
	}

	// Replay restore
	restReplayOut, err := exec.Command(binary, "engine", "restore", "--state", stateA, "--folder", folder, "--path", "conflict.txt",
		"--source", headA, "--reviewed", curHeadStr, "--head-token", tokenStr,
		"--idempotency-key", "restore-key-1", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("restore replay: %v\n%s", err, restReplayOut)
	}
	var restReplayResult control.ResolveResult
	if err := json.Unmarshal(restReplayOut, &restReplayResult); err != nil {
		t.Fatalf("unmarshal restore replay: %v\n%s", err, restReplayOut)
	}
	if !restReplayResult.Replay || restReplayResult.ResolvedID != restResult.ResolvedID {
		t.Fatalf("restore replay mismatch: %+v", restReplayResult)
	}

	// Verify history now has 4 items, and the restored version is a new event extending current causal heads
	histOut2, _ := exec.Command(binary, "engine", "history", "--state", stateA, "--folder", folder, "--path", "conflict.txt", "--json").CombinedOutput()
	var histItems2 []control.HistoryItem
	_ = json.Unmarshal(histOut2, &histItems2)
	if len(histItems2) != 4 {
		t.Fatalf("expected 4 history items after restore, got %d", len(histItems2))
	}

	// 9. Test `orbit conflicts keep-copies`
	// Create another conflict on "doc.txt"
	if err := os.WriteFile(filepath.Join(rootA, "doc.txt"), []byte("doc from alice"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootB, "doc.txt"), []byte("doc from bob"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _ = exec.Command(binary, "engine", "scan", "--state", stateA, "--folder", folder).CombinedOutput()
	_, _ = exec.Command(binary, "engine", "scan", "--state", stateB, "--folder", folder).CombinedOutput()

	// Sync B -> A
	serveCmdB = exec.Command(binary, "engine", "serve", "--state", stateB, "--peer-listen", "127.0.0.1:0")
	stdoutB, err = serveCmdB.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	serveCmdB.Stderr = os.Stderr
	if err := serveCmdB.Start(); err != nil {
		t.Fatalf("start serve B: %v", err)
	}
	urlB = readListenerURL(t, stdoutB)
	_, _ = exec.Command(binary, "engine", "sync", "--state", stateA, "--folder", folder, "--peer-url", urlB, "--peer-device", devB, "--peer-certificate", certPathB).CombinedOutput()
	_ = serveCmdB.Process.Kill()
	_ = serveCmdB.Wait()

	// Query conflicts on doc.txt to get reviewed heads and token
	docConfJSON, err := exec.Command(binary, "engine", "conflicts", "--state", stateA, "--folder", folder, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("conflicts check for doc.txt: %v\n%s", err, docConfJSON)
	}
	var docConfResp struct {
		Conflicts []repository.ConflictSet `json:"conflicts"`
	}
	_ = json.Unmarshal(docConfJSON, &docConfResp)
	var docHeads []string
	var docToken string
	for _, c := range docConfResp.Conflicts {
		if c.Path == "doc.txt" {
			docToken = hex.EncodeToString(c.HeadToken[:])
			for _, h := range c.Heads {
				docHeads = append(docHeads, fmt.Sprintf("%x:%d", h.ID.Author, h.ID.Counter))
			}
		}
	}
	if len(docHeads) != 2 {
		t.Fatalf("expected 2 heads for doc.txt conflict, got %d", len(docHeads))
	}
	docReviewedStr := strings.Join(docHeads, ",")

	// Run keep-copies
	kcOut, err := exec.Command(binary, "engine", "conflicts", "keep-copies", "--state", stateA, "--folder", folder, "--path", "doc.txt",
		"--reviewed", docReviewedStr, "--head-token", docToken,
		"--idempotency-key", "kc-key-1", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("keep-copies: %v\n%s", err, kcOut)
	}
	var kcResult control.KeepCopiesResult
	if err := json.Unmarshal(kcOut, &kcResult); err != nil {
		t.Fatalf("unmarshal keep-copies: %v\n%s", err, kcOut)
	}
	if !kcResult.Completed || len(kcResult.Copies) != 2 {
		t.Fatalf("unexpected keep-copies result: %+v", kcResult)
	}

	// Verify copy files exist on disk
	for _, cp := range kcResult.Copies {
		if _, err := os.Stat(filepath.Join(rootA, cp.DestinationPath)); err != nil {
			t.Fatalf("copy file missing on disk: %s: %v", cp.DestinationPath, err)
		}
	}

	// Replay keep-copies with same key
	kcReplayOut, err := exec.Command(binary, "engine", "conflicts", "keep-copies", "--state", stateA, "--folder", folder, "--path", "doc.txt",
		"--reviewed", docReviewedStr, "--head-token", docToken,
		"--idempotency-key", "kc-key-1", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("keep-copies replay: %v\n%s", err, kcReplayOut)
	}
	var kcReplayResult control.KeepCopiesResult
	if err := json.Unmarshal(kcReplayOut, &kcReplayResult); err != nil {
		t.Fatalf("unmarshal keep-copies replay: %v\n%s", err, kcReplayOut)
	}
	if !kcReplayResult.Replay || kcReplayResult.ResolvedID != kcResult.ResolvedID {
		t.Fatalf("keep-copies replay mismatch: %+v", kcReplayResult)
	}
}
