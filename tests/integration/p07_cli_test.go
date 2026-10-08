package integration_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
	"github.com/calebhabesh/orbit/model"
)

func TestP07CLIBidirectionalReconciliation(t *testing.T) {
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

	folder := strings.Repeat("c", 64)

	// Initialize and register both nodes
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

	// Offline edits:
	// 1. Edit-edit conflict: "conflict.txt"
	if err := os.WriteFile(filepath.Join(rootA, "conflict.txt"), []byte("edit from node A"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootB, "conflict.txt"), []byte("edit from node B"), 0o600); err != nil {
		t.Fatal(err)
	}

	// 2. Equal-byte conflict: "same.txt"
	if err := os.WriteFile(filepath.Join(rootA, "same.txt"), []byte("identical payload across nodes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootB, "same.txt"), []byte("identical payload across nodes"), 0o600); err != nil {
		t.Fatal(err)
	}

	// 3. Executable file on Node A: "run.sh"
	if err := os.WriteFile(filepath.Join(rootA, "run.sh"), []byte("#!/bin/sh\necho ok\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	// Scan on both nodes while offline
	scanOutA, err := exec.Command(binary, "engine", "scan", "--state", stateA, "--folder", folder).CombinedOutput()
	if err != nil {
		t.Fatalf("scan A: %v\n%s", err, scanOutA)
	}
	scanOutB, err := exec.Command(binary, "engine", "scan", "--state", stateB, "--folder", folder).CombinedOutput()
	if err != nil {
		t.Fatalf("scan B: %v\n%s", err, scanOutB)
	}

	// Start server on Node A
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

	// Node B syncs from Node A
	syncOutB, err := exec.Command(binary, "engine", "sync", "--state", stateB, "--folder", folder, "--peer-url", urlA, "--peer-device", devA, "--peer-certificate", certPathA, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("sync B: %v\n%s", err, syncOutB)
	}

	// Stop server A
	_ = serveCmdA.Process.Kill()
	_ = serveCmdA.Wait()

	// Start server on Node B
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

	// Node A syncs from Node B
	syncOutA, err := exec.Command(binary, "engine", "sync", "--state", stateA, "--folder", folder, "--peer-url", urlB, "--peer-device", devB, "--peer-certificate", certPathB, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("sync A: %v\n%s", err, syncOutA)
	}

	// Stop server B
	_ = serveCmdB.Process.Kill()
	_ = serveCmdB.Wait()

	// Now check conflicts CLI command on Node A and Node B
	for _, node := range []struct {
		name     string
		state    string
		root     string
		wantConf string
		localDev string
	}{
		{"Node A", stateA, rootA, "edit from node A", devA},
		{"Node B", stateB, rootB, "edit from node B", devB},
	} {
		// 1. Text output of conflicts command
		confText, err := exec.Command(binary, "engine", "conflicts", "--state", node.state, "--folder", folder).CombinedOutput()
		if err != nil {
			t.Fatalf("%s conflicts command: %v\n%s", node.name, err, confText)
		}
		if !strings.Contains(string(confText), "conflict path=conflict.txt kind=edit-edit heads=2") {
			t.Fatalf("%s missing expected conflict text:\n%s", node.name, confText)
		}
		if !strings.Contains(string(confText), "conflict path=same.txt kind=equal-content heads=2") {
			t.Fatalf("%s missing expected equal-content text:\n%s", node.name, confText)
		}

		// 2. JSON output of conflicts command
		confJSON, err := exec.Command(binary, "engine", "conflicts", "--state", node.state, "--folder", folder, "--json").CombinedOutput()
		if err != nil {
			t.Fatalf("%s conflicts JSON command: %v\n%s", node.name, err, confJSON)
		}
		var parsed struct {
			Conflicts []repository.ConflictSet `json:"conflicts"`
		}
		if err := json.Unmarshal(confJSON, &parsed); err != nil {
			t.Fatalf("%s unmarshal conflicts JSON: %v\n%s", node.name, err, confJSON)
		}
		if len(parsed.Conflicts) != 2 {
			t.Fatalf("%s expected 2 conflicts, got %d", node.name, len(parsed.Conflicts))
		}
		byPath := map[string]repository.ConflictSet{}
		for _, c := range parsed.Conflicts {
			byPath[c.Path] = c
		}
		c1, ok := byPath["conflict.txt"]
		if !ok || c1.ConflictKind != "edit-edit" || len(c1.Heads) != 2 {
			t.Fatalf("%s conflict.txt: %+v", node.name, c1)
		}
		c2, ok := byPath["same.txt"]
		if !ok || c2.ConflictKind != "equal-content" || len(c2.Heads) != 2 {
			t.Fatalf("%s same.txt: %+v", node.name, c2)
		}

		// 3. Verify working tree content preserved on disk
		content, err := os.ReadFile(filepath.Join(node.root, "conflict.txt"))
		if err != nil || string(content) != node.wantConf {
			t.Fatalf("%s conflict.txt on disk=%q, want %q", node.name, string(content), node.wantConf)
		}
	}

	// Verify run.sh on Node B was applied and executable bit is set
	statB, err := os.Stat(filepath.Join(rootB, "run.sh"))
	if err != nil {
		t.Fatalf("run.sh missing on Node B: %v", err)
	}
	if statB.Mode().Perm()&0o111 == 0 {
		t.Fatalf("run.sh on Node B is not executable: perm=%v", statB.Mode().Perm())
	}

	// Verify subsequent scans on both nodes capture ZERO extra versions
	for _, node := range []struct {
		name  string
		state string
	}{
		{"Node A", stateA},
		{"Node B", stateB},
	} {
		scanOut, err := exec.Command(binary, "engine", "scan", "--state", node.state, "--folder", folder).CombinedOutput()
		if err != nil {
			t.Fatalf("%s subsequent scan: %v\n%s", node.name, err, scanOut)
		}
		if strings.Contains(string(scanOut), "captured") {
			t.Fatalf("%s subsequent scan unexpectedly captured new versions:\n%s", node.name, scanOut)
		}
	}

	// Verify agreement with independent DAG model oracle
	oracle := model.New()
	_ = oracle.Accept(model.Event{ID: "A1", Author: "A", Path: "conflict.txt", Content: "edit from node A"}, true)
	_ = oracle.Accept(model.Event{ID: "B1", Author: "B", Path: "conflict.txt", Content: "edit from node B"}, true)
	oracleHeads := oracle.Heads("conflict.txt")
	if len(oracleHeads) != 2 || oracleHeads[0] != "A1" || oracleHeads[1] != "B1" {
		t.Fatalf("oracle heads=%v, want [A1, B1]", oracleHeads)
	}
}
