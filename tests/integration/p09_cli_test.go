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
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
	"github.com/calebhabesh/orbit/model"
)

func TestP09ThreePeerForwardingAndMembershipLifecycle(t *testing.T) {
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

	folder := strings.Repeat("9", 64)
	var folderID history.ID
	folderBytes, _ := hex.DecodeString(folder)
	copy(folderID[:], folderBytes)

	// 1. Initialize and register three nodes A, B, C
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

	// 2. Export identities and certificates
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

	idOutC, err := exec.Command(binary, "engine", "identity", "--state", stateC, "--certificate").CombinedOutput()
	if err != nil {
		t.Fatalf("identity C: %v\n%s", err, idOutC)
	}
	devC, pinC, certC := parseIdentityOutput(t, idOutC)
	certPathC := filepath.Join(disposable, "peerC.pem")
	if err := os.WriteFile(certPathC, certC, 0o600); err != nil {
		t.Fatal(err)
	}

	var parsedDevA, parsedDevB, parsedDevC history.ID
	aBytes, _ := hex.DecodeString(devA)
	copy(parsedDevA[:], aBytes)
	bBytes, _ := hex.DecodeString(devB)
	copy(parsedDevB[:], bBytes)
	cBytes, _ := hex.DecodeString(devC)
	copy(parsedDevC[:], cBytes)

	var parsedPinA, parsedPinB, parsedPinC history.Digest
	paBytes, _ := hex.DecodeString(pinA)
	copy(parsedPinA[:], paBytes)
	pbBytes, _ := hex.DecodeString(pinB)
	copy(parsedPinB[:], pbBytes)
	pcBytes, _ := hex.DecodeString(pinC)
	copy(parsedPinC[:], pcBytes)

	// 3. Establish initial Revision 1 Membership with Active: {A, B, C}
	bundleRev1 := control.MembershipExportResult{
		Membership: protocol.Membership{
			Folder:   folderID,
			Revision: 1,
			Active: []protocol.ActiveMember{
				{Device: parsedDevA, KeyPin: parsedPinA},
				{Device: parsedDevB, KeyPin: parsedPinB},
				{Device: parsedDevC, KeyPin: parsedPinC},
			},
		},
	}
	bundleRev1Bytes, err := json.MarshalIndent(bundleRev1, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	bundlePath1 := filepath.Join(disposable, "membership_rev1.json")
	if err := os.WriteFile(bundlePath1, bundleRev1Bytes, 0o600); err != nil {
		t.Fatal(err)
	}

	// Preview and approve revision 1 on all three nodes
	for _, st := range []string{stateA, stateB, stateC} {
		prevOut, err := exec.Command(binary, "engine", "membership", "preview", "--state", st, "--folder", folder, "--file", bundlePath1, "--json").CombinedOutput()
		if err != nil {
			t.Fatalf("membership preview on %s: %v\n%s", st, err, prevOut)
		}
		var prev control.MembershipPreviewResult
		if err := json.Unmarshal(prevOut, &prev); err != nil {
			t.Fatalf("unmarshal preview on %s: %v", st, err)
		}
		if !prev.ValidTransition {
			t.Fatalf("expected valid transition for rev1 on %s: %+v", st, prev)
		}

		impOut, err := exec.Command(binary, "engine", "membership", "import", "--state", st, "--folder", folder, "--file", bundlePath1, "--approve").CombinedOutput()
		if err != nil {
			t.Fatalf("membership import on %s: %v\n%s", st, err, impOut)
		}
	}

	// Verify peers list on Node B
	peersOutB, err := exec.Command(binary, "engine", "peers", "list", "--state", stateB, "--folder", folder, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("peers list B: %v\n%s", err, peersOutB)
	}
	var peersB control.PeerListResult
	if err := json.Unmarshal(peersOutB, &peersB); err != nil {
		t.Fatalf("unmarshal peers list B: %v", err)
	}
	if peersB.Revision != 1 || len(peersB.Active) != 3 || len(peersB.Retired) != 0 {
		t.Fatalf("unexpected peers list on B: %+v", peersB)
	}

	// =========================================================================
	// Scenario 1: Three-peer forwarding (A -> B -> C and C -> B -> A)
	// Topology: A <-> B <-> C (A and C have NO direct link)
	// =========================================================================

	// Step 1a: Forwarding A -> B -> C
	aContent := []byte("content from node A forwarded through B to C")
	if err := os.WriteFile(filepath.Join(rootA, "docA.txt"), aContent, 0o600); err != nil {
		t.Fatal(err)
	}

	scanOutA, err := exec.Command(binary, "engine", "scan", "--state", stateA, "--folder", folder).CombinedOutput()
	if err != nil {
		t.Fatalf("scan A: %v\n%s", err, scanOutA)
	}

	// Serve A, B syncs from A
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

	syncOutB, err := exec.Command(binary, "engine", "sync", "--state", stateB, "--folder", folder, "--peer-url", urlA, "--peer-device", devA, "--peer-certificate", certPathA, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("sync B from A: %v\n%s", err, syncOutB)
	}

	_ = serveCmdA.Process.Kill()
	_ = serveCmdA.Wait()

	// Serve B, C syncs from B (C receives A's file via B forwarding)
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

	syncOutC, err := exec.Command(binary, "engine", "sync", "--state", stateC, "--folder", folder, "--peer-url", urlB, "--peer-device", devB, "--peer-certificate", certPathB, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("sync C from B: %v\n%s", err, syncOutC)
	}

	_ = serveCmdB.Process.Kill()
	_ = serveCmdB.Wait()

	// Verify on Node C:
	// 1. File exists byte-for-byte on disk
	contentOnC, err := os.ReadFile(filepath.Join(rootC, "docA.txt"))
	if err != nil {
		t.Fatalf("docA.txt missing on Node C: %v", err)
	}
	if string(contentOnC) != string(aContent) {
		t.Fatalf("docA.txt on C=%q, want %q", string(contentOnC), string(aContent))
	}

	// 2. Status on Node C: author identity is devA (NOT devB!) (Invariant I08)
	statOutC, err := exec.Command(binary, "engine", "status", "--state", stateC, "--folder", folder, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("status C: %v\n%s", err, statOutC)
	}
	var statC struct {
		Versions []repository.VersionStatus `json:"versions"`
		Peers    []repository.PeerProgress  `json:"peers"`
	}
	if err := json.Unmarshal(statOutC, &statC); err != nil {
		t.Fatalf("unmarshal status C: %v", err)
	}
	foundDocA := false
	for _, v := range statC.Versions {
		if v.Path == "docA.txt" {
			foundDocA = true
			if hex.EncodeToString(v.ID.Author[:]) != devA {
				t.Fatalf("expected author to be %s, got %x", devA, v.ID.Author)
			}
			if !v.Stored || !v.Applied {
				t.Fatalf("expected docA.txt to be stored and applied on C: %+v", v)
			}
		}
	}
	if !foundDocA {
		t.Fatal("docA.txt not found in Node C version status")
	}

	// Step 1b: Reverse forwarding C -> B -> A
	cContent := []byte("content from node C reverse forwarded through B to A")
	if err := os.WriteFile(filepath.Join(rootC, "docC.txt"), cContent, 0o600); err != nil {
		t.Fatal(err)
	}

	scanOutC, err := exec.Command(binary, "engine", "scan", "--state", stateC, "--folder", folder).CombinedOutput()
	if err != nil {
		t.Fatalf("scan C: %v\n%s", err, scanOutC)
	}

	// Serve C, B syncs from C
	serveCmdC := exec.Command(binary, "engine", "serve", "--state", stateC, "--peer-listen", "127.0.0.1:0")
	stdoutC, err := serveCmdC.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	serveCmdC.Stderr = os.Stderr
	if err := serveCmdC.Start(); err != nil {
		t.Fatalf("start serve C: %v", err)
	}
	urlC := readListenerURL(t, stdoutC)

	syncOutBFromC, err := exec.Command(binary, "engine", "sync", "--state", stateB, "--folder", folder, "--peer-url", urlC, "--peer-device", devC, "--peer-certificate", certPathC, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("sync B from C: %v\n%s", err, syncOutBFromC)
	}

	_ = serveCmdC.Process.Kill()
	_ = serveCmdC.Wait()

	// Serve B, A syncs from B
	serveCmdB2 := exec.Command(binary, "engine", "serve", "--state", stateB, "--peer-listen", "127.0.0.1:0")
	stdoutB2, err := serveCmdB2.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	serveCmdB2.Stderr = os.Stderr
	if err := serveCmdB2.Start(); err != nil {
		t.Fatalf("start serve B2: %v", err)
	}
	urlB2 := readListenerURL(t, stdoutB2)

	syncOutAFromB, err := exec.Command(binary, "engine", "sync", "--state", stateA, "--folder", folder, "--peer-url", urlB2, "--peer-device", devB, "--peer-certificate", certPathB, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("sync A from B: %v\n%s", err, syncOutAFromB)
	}

	_ = serveCmdB2.Process.Kill()
	_ = serveCmdB2.Wait()

	// Verify on Node A:
	contentOnA, err := os.ReadFile(filepath.Join(rootA, "docC.txt"))
	if err != nil {
		t.Fatalf("docC.txt missing on Node A: %v", err)
	}
	if string(contentOnA) != string(cContent) {
		t.Fatalf("docC.txt on A=%q, want %q", string(contentOnA), string(cContent))
	}

	// Status on Node A has author devC
	statOutA, err := exec.Command(binary, "engine", "status", "--state", stateA, "--folder", folder, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("status A: %v\n%s", err, statOutA)
	}
	var statA struct {
		Versions []repository.VersionStatus `json:"versions"`
		Peers    []repository.PeerProgress  `json:"peers"`
	}
	if err := json.Unmarshal(statOutA, &statA); err != nil {
		t.Fatalf("unmarshal status A: %v", err)
	}
	foundDocC := false
	for _, v := range statA.Versions {
		if v.Path == "docC.txt" {
			foundDocC = true
			if hex.EncodeToString(v.ID.Author[:]) != devC {
				t.Fatalf("expected author to be %s, got %x", devC, v.ID.Author)
			}
		}
	}
	if !foundDocC {
		t.Fatal("docC.txt not found in Node A version status")
	}

	// Verify direct progress labeling in text status on Node B
	statusTextB, err := exec.Command(binary, "engine", "status", "--state", stateB, "--folder", folder).CombinedOutput()
	if err != nil {
		t.Fatalf("status text B: %v\n%s", err, statusTextB)
	}
	if !strings.Contains(string(statusTextB), "direct=true") {
		t.Fatalf("expected direct=true in status output:\n%s", statusTextB)
	}

	// =========================================================================
	// Scenario 2: Three offline concurrent edits and resolution late-arrival
	// Invariants: I02 (convergence), I03 (concurrent unreviewed heads survive)
	// =========================================================================

	sharedPath := "shared.txt"
	if err := os.WriteFile(filepath.Join(rootA, sharedPath), []byte("edit from A"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootB, sharedPath), []byte("edit from B"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootC, sharedPath), []byte("edit from C"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Scan offline on all 3 nodes
	for _, st := range []string{stateA, stateB, stateC} {
		if out, err := exec.Command(binary, "engine", "scan", "--state", st, "--folder", folder).CombinedOutput(); err != nil {
			t.Fatalf("scan %s: %v\n%s", st, err, out)
		}
	}

	// Sync A to B
	serveA2 := exec.Command(binary, "engine", "serve", "--state", stateA, "--peer-listen", "127.0.0.1:0")
	stdoutA2, _ := serveA2.StdoutPipe()
	serveA2.Stderr = os.Stderr
	_ = serveA2.Start()
	urlA2 := readListenerURL(t, stdoutA2)

	_, err = exec.Command(binary, "engine", "sync", "--state", stateB, "--folder", folder, "--peer-url", urlA2, "--peer-device", devA, "--peer-certificate", certPathA).CombinedOutput()
	if err != nil {
		t.Fatalf("sync B from A2: %v", err)
	}
	_ = serveA2.Process.Kill()
	_ = serveA2.Wait()

	// Check conflicts on B: should have 2 heads (A and B)
	confOutB, err := exec.Command(binary, "engine", "conflicts", "--state", stateB, "--folder", folder, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("conflicts B: %v\n%s", err, confOutB)
	}
	var confB struct {
		Conflicts []repository.ConflictSet `json:"conflicts"`
	}
	if err := json.Unmarshal(confOutB, &confB); err != nil {
		t.Fatalf("unmarshal conflicts B: %v", err)
	}
	var sharedConf *repository.ConflictSet
	for i := range confB.Conflicts {
		if confB.Conflicts[i].Path == sharedPath {
			sharedConf = &confB.Conflicts[i]
			break
		}
	}
	if sharedConf == nil || len(sharedConf.Heads) != 2 {
		t.Fatalf("expected 2 conflicting heads on B for %s, got: %+v", sharedPath, sharedConf)
	}

	headTokenStr := hex.EncodeToString(sharedConf.HeadToken[:])
	var headIDs []string
	var selectedHead string
	for _, h := range sharedConf.Heads {
		hID := fmt.Sprintf("%x:%d", h.ID.Author, h.ID.Counter)
		headIDs = append(headIDs, hID)
		if hex.EncodeToString(h.ID.Author[:]) == devA {
			selectedHead = hID
		}
	}
	reviewedStr := strings.Join(headIDs, ",")

	// Resolve A and B on Node B by selecting A
	resOutB, err := exec.Command(binary, "engine", "resolve", "select", "--state", stateB, "--folder", folder,
		"--path", sharedPath, "--selected", selectedHead, "--reviewed", reviewedStr,
		"--head-token", headTokenStr, "--idempotency-key", "resolve-ab-1", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("resolve select on B: %v\n%s", err, resOutB)
	}
	var resResult control.ResolveResult
	if err := json.Unmarshal(resOutB, &resResult); err != nil {
		t.Fatalf("unmarshal resolve result B: %v", err)
	}
	if resResult.Action != "select" {
		t.Fatalf("unexpected resolve action: %s", resResult.Action)
	}

	// Late-arrival of C's edit: sync C to B
	serveC2 := exec.Command(binary, "engine", "serve", "--state", stateC, "--peer-listen", "127.0.0.1:0")
	stdoutC2, _ := serveC2.StdoutPipe()
	serveC2.Stderr = os.Stderr
	_ = serveC2.Start()
	urlC2 := readListenerURL(t, stdoutC2)

	_, err = exec.Command(binary, "engine", "sync", "--state", stateB, "--folder", folder, "--peer-url", urlC2, "--peer-device", devC, "--peer-certificate", certPathC).CombinedOutput()
	if err != nil {
		t.Fatalf("sync B from C2: %v", err)
	}
	_ = serveC2.Process.Kill()
	_ = serveC2.Wait()

	// Check conflicts on B again: C was concurrent and unreviewed, so it MUST remain concurrent! (I03)
	confOutB2, err := exec.Command(binary, "engine", "conflicts", "--state", stateB, "--folder", folder, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("conflicts B2: %v\n%s", err, confOutB2)
	}
	var confB2 struct {
		Conflicts []repository.ConflictSet `json:"conflicts"`
	}
	if err := json.Unmarshal(confOutB2, &confB2); err != nil {
		t.Fatalf("unmarshal conflicts B2: %v", err)
	}
	var sharedConf2 *repository.ConflictSet
	for i := range confB2.Conflicts {
		if confB2.Conflicts[i].Path == sharedPath {
			sharedConf2 = &confB2.Conflicts[i]
			break
		}
	}
	if sharedConf2 == nil || len(sharedConf2.Heads) != 2 {
		t.Fatalf("expected 2 concurrent heads on B after late arrival (resolution + C), got: %+v", sharedConf2)
	}

	// Sync B to A and B to C so all nodes converge
	serveB3 := exec.Command(binary, "engine", "serve", "--state", stateB, "--peer-listen", "127.0.0.1:0")
	stdoutB3, _ := serveB3.StdoutPipe()
	serveB3.Stderr = os.Stderr
	_ = serveB3.Start()
	urlB3 := readListenerURL(t, stdoutB3)

	_, err = exec.Command(binary, "engine", "sync", "--state", stateA, "--folder", folder, "--peer-url", urlB3, "--peer-device", devB, "--peer-certificate", certPathB).CombinedOutput()
	if err != nil {
		t.Fatalf("sync A from B3: %v", err)
	}
	_, err = exec.Command(binary, "engine", "sync", "--state", stateC, "--folder", folder, "--peer-url", urlB3, "--peer-device", devB, "--peer-certificate", certPathB).CombinedOutput()
	if err != nil {
		t.Fatalf("sync C from B3: %v", err)
	}
	_ = serveB3.Process.Kill()
	_ = serveB3.Wait()

	// Verify all 3 nodes have converged to the exact same heads for shared.txt
	for _, node := range []struct {
		name  string
		state string
	}{
		{"Node A", stateA},
		{"Node B", stateB},
		{"Node C", stateC},
	} {
		confOut, err := exec.Command(binary, "engine", "conflicts", "--state", node.state, "--folder", folder, "--json").CombinedOutput()
		if err != nil {
			t.Fatalf("%s conflicts: %v\n%s", node.name, err, confOut)
		}
		var parsed struct {
			Conflicts []repository.ConflictSet `json:"conflicts"`
		}
		if err := json.Unmarshal(confOut, &parsed); err != nil {
			t.Fatalf("%s unmarshal conflicts: %v", node.name, err)
		}
		var nodeShared *repository.ConflictSet
		for i := range parsed.Conflicts {
			if parsed.Conflicts[i].Path == sharedPath {
				nodeShared = &parsed.Conflicts[i]
				break
			}
		}
		if nodeShared == nil || len(nodeShared.Heads) != 2 {
			t.Fatalf("%s expected 2 heads for %s, got: %+v", node.name, sharedPath, nodeShared)
		}
	}

	// Verify DAG model oracle agrees with this head set
	oracle := model.New()
	_ = oracle.Accept(model.Event{ID: "A1", Author: "A", Path: sharedPath, Content: "edit from A"}, true)
	_ = oracle.Accept(model.Event{ID: "B1", Author: "B", Path: sharedPath, Content: "edit from B"}, true)
	_ = oracle.Accept(model.Event{ID: "Res", Author: "B", Path: sharedPath, Parents: []string{"A1", "B1"}, Content: "edit from A"}, true)
	_ = oracle.Accept(model.Event{ID: "C1", Author: "C", Path: sharedPath, Content: "edit from C"}, true)
	oracleHeads := oracle.Heads(sharedPath)
	if len(oracleHeads) != 2 {
		t.Fatalf("oracle heads expected 2, got: %v", oracleHeads)
	}

	// =========================================================================
	// Scenario 3: Retirement lifecycle of Node C
	// Invariants: I14 (Access termination upon revocation/retirement)
	// =========================================================================

	// Preview retirement of Node C on Node B
	prevRetOut, err := exec.Command(binary, "engine", "peers", "retire", "--state", stateB, "--folder", folder,
		"--peer-device", devC, "--preview", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("peers retire preview: %v\n%s", err, prevRetOut)
	}
	var prevRet control.RetireMemberPreview
	if err := json.Unmarshal(prevRetOut, &prevRet); err != nil {
		t.Fatalf("unmarshal retire preview: %v", err)
	}
	if prevRet.CurrentRevision != 1 || prevRet.NextRevision != 2 || prevRet.TargetDevice != parsedDevC {
		t.Fatalf("unexpected retire preview: %+v", prevRet)
	}
	if prevRet.AcceptedVersionsCount == 0 {
		t.Fatalf("expected accepted versions for retiree > 0, got %d", prevRet.AcceptedVersionsCount)
	}

	// Execute retirement of Node C on Node B
	retOut, err := exec.Command(binary, "engine", "peers", "retire", "--state", stateB, "--folder", folder,
		"--peer-device", devC, "--idempotency-key", "retire-c-exec", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("peers retire execute: %v\n%s", err, retOut)
	}
	var retResult control.RetireMemberResult
	if err := json.Unmarshal(retOut, &retResult); err != nil {
		t.Fatalf("unmarshal retire result: %v", err)
	}
	if retResult.ApprovedRevision != 2 || retResult.TargetDevice != parsedDevC || retResult.Replay {
		t.Fatalf("unexpected retire result: %+v", retResult)
	}

	// Replay retirement of Node C on Node B with same key
	retReplayOut, err := exec.Command(binary, "engine", "peers", "retire", "--state", stateB, "--folder", folder,
		"--peer-device", devC, "--idempotency-key", "retire-c-exec", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("peers retire replay: %v\n%s", err, retReplayOut)
	}
	var retReplayResult control.RetireMemberResult
	if err := json.Unmarshal(retReplayOut, &retReplayResult); err != nil {
		t.Fatalf("unmarshal retire replay result: %v", err)
	}
	if !retReplayResult.Replay || retReplayResult.ApprovedRevision != retResult.ApprovedRevision {
		t.Fatalf("expected replay flag: %+v", retReplayResult)
	}

	// Export membership revision 2 bundle from Node B
	bundleRev2Path := filepath.Join(disposable, "membership_rev2.json")
	expOut, err := exec.Command(binary, "engine", "membership", "export", "--state", stateB, "--folder", folder,
		"--file", bundleRev2Path).CombinedOutput()
	if err != nil {
		t.Fatalf("membership export rev2: %v\n%s", err, expOut)
	}

	// On survivor Node A, preview and import revision 2
	prevAOut, err := exec.Command(binary, "engine", "membership", "preview", "--state", stateA, "--folder", folder,
		"--file", bundleRev2Path, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("membership preview on A: %v\n%s", err, prevAOut)
	}
	var prevA control.MembershipPreviewResult
	if err := json.Unmarshal(prevAOut, &prevA); err != nil {
		t.Fatalf("unmarshal membership preview on A: %v", err)
	}
	if !prevA.ValidTransition || !prevA.PriorDigestMatches || len(prevA.NewlyRetired) != 1 || prevA.NewlyRetired[0] != parsedDevC {
		t.Fatalf("unexpected membership preview on A: %+v", prevA)
	}

	impAOut, err := exec.Command(binary, "engine", "membership", "import", "--state", stateA, "--folder", folder,
		"--file", bundleRev2Path, "--approve").CombinedOutput()
	if err != nil {
		t.Fatalf("membership import on A: %v\n%s", err, impAOut)
	}

	// Verify Node A peer list shows C retired
	peersAOut, err := exec.Command(binary, "engine", "peers", "list", "--state", stateA, "--folder", folder, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("peers list on A: %v\n%s", err, peersAOut)
	}
	var peersA control.PeerListResult
	if err := json.Unmarshal(peersAOut, &peersA); err != nil {
		t.Fatalf("unmarshal peers list on A: %v", err)
	}
	if peersA.Revision != 2 || len(peersA.Active) != 2 || len(peersA.Retired) != 1 || peersA.Retired[0].Device != parsedDevC {
		t.Fatalf("unexpected peers list on A: %+v", peersA)
	}

	// Access Termination (Invariant I14):
	// Node C (retired) attempts to connect to Node B
	serveB4 := exec.Command(binary, "engine", "serve", "--state", stateB, "--peer-listen", "127.0.0.1:0")
	stdoutB4, _ := serveB4.StdoutPipe()
	serveB4.Stderr = os.Stderr
	_ = serveB4.Start()
	urlB4 := readListenerURL(t, stdoutB4)

	// Node C attempts to sync with Node B - MUST FAIL
	syncFailOut, err := exec.Command(binary, "engine", "sync", "--state", stateC, "--folder", folder,
		"--peer-url", urlB4, "--peer-device", devB, "--peer-certificate", certPathB).CombinedOutput()
	if err == nil {
		t.Fatalf("expected sync from retired Node C to fail, but succeeded:\n%s", syncFailOut)
	}
	_ = serveB4.Process.Kill()
	_ = serveB4.Wait()

	// =========================================================================
	// Scenario 4: Divergent existing-folder enrollment (preview and bootstrap)
	// Invariant: I15 (Non-destructive enrollment of existing folders)
	// =========================================================================

	stateD := filepath.Join(disposable, "state-d")
	rootD := filepath.Join(disposable, "root-d")
	if err := os.Mkdir(stateD, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(rootD, 0o700); err != nil {
		t.Fatal(err)
	}

	if out, err := exec.Command(binary, "engine", "init", "--state", stateD).CombinedOutput(); err != nil {
		t.Fatalf("init D: %v\n%s", err, out)
	}

	// Pre-create existing files in rootD
	// 1. Existing file identical to a_doc
	if err := os.WriteFile(filepath.Join(rootD, "docA.txt"), aContent, 0o600); err != nil {
		t.Fatal(err)
	}
	// 2. Existing file with divergent content
	if err := os.WriteFile(filepath.Join(rootD, "local_divergent.txt"), []byte("local divergent content"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Register folder on D
	if out, err := exec.Command(binary, "engine", "register", "--state", stateD, "--folder", folder, "--root", rootD).CombinedOutput(); err != nil {
		t.Fatalf("register D: %v\n%s", err, out)
	}

	// Enroll preview
	enrPrevOut, err := exec.Command(binary, "engine", "enroll", "preview", "--state", stateD, "--folder", folder,
		"--root", rootD, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("enroll preview D: %v\n%s", err, enrPrevOut)
	}
	var enrPrev control.EnrollPreviewResult
	if err := json.Unmarshal(enrPrevOut, &enrPrev); err != nil {
		t.Fatalf("unmarshal enroll preview D: %v", err)
	}
	if enrPrev.LocalFilesCount != 2 || len(enrPrev.ExistingPaths) != 2 {
		t.Fatalf("unexpected enroll preview: %+v", enrPrev)
	}

	// Enroll bootstrap
	enrBootOut, err := exec.Command(binary, "engine", "enroll", "bootstrap", "--state", stateD, "--folder", folder,
		"--root", rootD, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("enroll bootstrap D: %v\n%s", err, enrBootOut)
	}
	var enrBoot control.EnrollBootstrapResult
	if err := json.Unmarshal(enrBootOut, &enrBoot); err != nil {
		t.Fatalf("unmarshal enroll bootstrap D: %v", err)
	}
	if enrBoot.CapturedCount != 2 {
		t.Fatalf("expected 2 captured files in bootstrap, got %d", enrBoot.CapturedCount)
	}

	// Verify existing files are intact on disk
	dContent, err := os.ReadFile(filepath.Join(rootD, "local_divergent.txt"))
	if err != nil || string(dContent) != "local divergent content" {
		t.Fatalf("local divergent content corrupted: %v", err)
	}

	// =========================================================================
	// Scenario 5: Metadata loss reinstall under new identity
	// =========================================================================

	stateE := filepath.Join(disposable, "state-e")
	rootE := filepath.Join(disposable, "root-e")
	if err := os.Mkdir(stateE, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(rootE, 0o700); err != nil {
		t.Fatal(err)
	}

	// Initialize Node E
	if out, err := exec.Command(binary, "engine", "init", "--state", stateE).CombinedOutput(); err != nil {
		t.Fatalf("init E: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(rootE, "reinstall_note.txt"), []byte("valuable work preserved"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(binary, "engine", "register", "--state", stateE, "--folder", folder, "--root", rootE).CombinedOutput(); err != nil {
		t.Fatalf("register E: %v\n%s", err, out)
	}

	// Simulate catastrophic state loss: wipe stateE
	if err := os.RemoveAll(stateE); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(stateE, 0o700); err != nil {
		t.Fatal(err)
	}

	// Reinitialize with fresh identity
	if out, err := exec.Command(binary, "engine", "init", "--state", stateE).CombinedOutput(); err != nil {
		t.Fatalf("reinstall init E: %v\n%s", err, out)
	}
	if out, err := exec.Command(binary, "engine", "register", "--state", stateE, "--folder", folder, "--root", rootE).CombinedOutput(); err != nil {
		t.Fatalf("reinstall register E: %v\n%s", err, out)
	}

	// Re-enroll folder via bootstrap
	bootEOut, err := exec.Command(binary, "engine", "enroll", "bootstrap", "--state", stateE, "--folder", folder,
		"--root", rootE, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("reinstall bootstrap E: %v\n%s", err, bootEOut)
	}
	var bootEResult control.EnrollBootstrapResult
	if err := json.Unmarshal(bootEOut, &bootEResult); err != nil {
		t.Fatalf("unmarshal bootstrap E: %v", err)
	}
	if bootEResult.CapturedCount != 1 {
		t.Fatalf("expected 1 captured file after reinstall, got %d", bootEResult.CapturedCount)
	}
	preserved, err := os.ReadFile(filepath.Join(rootE, "reinstall_note.txt"))
	if err != nil || string(preserved) != "valuable work preserved" {
		t.Fatalf("work was not preserved after reinstall: %v", err)
	}
}
