package integration_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
)

func TestP06CLITwoPeerTransfer(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(disposable, "filesync")
	build := exec.Command("go", "build", "-o", binary, "./cmd/filesync")
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

	folder := strings.Repeat("a", 64)

	// Initialize both nodes
	for _, args := range [][]string{
		{"init", "--state", stateA},
		{"init", "--state", stateB},
		{"register", "--state", stateA, "--folder", folder, "--root", rootA},
		{"register", "--state", stateB, "--folder", folder, "--root", rootB},
	} {
		if output, err := exec.Command(binary, args...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
	}

	// Export identity and certificate for Node A
	idOutA, err := exec.Command(binary, "identity", "--state", stateA, "--certificate").CombinedOutput()
	if err != nil {
		t.Fatalf("identity A: %v\n%s", err, idOutA)
	}
	devA, pinA, certA := parseIdentityOutput(t, idOutA)
	certPathA := filepath.Join(disposable, "peerA.pem")
	if err := os.WriteFile(certPathA, certA, 0o600); err != nil {
		t.Fatal(err)
	}

	// Export identity and certificate for Node B
	idOutB, err := exec.Command(binary, "identity", "--state", stateB, "--certificate").CombinedOutput()
	if err != nil {
		t.Fatalf("identity B: %v\n%s", err, idOutB)
	}
	devB, pinB, _ := parseIdentityOutput(t, idOutB)

	// Pair approve on both nodes
	if output, err := exec.Command(binary, "pair-approve", "--state", stateA, "--folder", folder, "--peer-device", devB, "--peer-key-pin", pinB).CombinedOutput(); err != nil {
		t.Fatalf("pair-approve A: %v\n%s", err, output)
	}
	if output, err := exec.Command(binary, "pair-approve", "--state", stateB, "--folder", folder, "--peer-device", devA, "--peer-key-pin", pinA).CombinedOutput(); err != nil {
		t.Fatalf("pair-approve B: %v\n%s", err, output)
	}

	// Create test files on Node A: empty file, small text file, and multi-chunk file
	emptyFile := filepath.Join(rootA, "empty.txt")
	if err := os.WriteFile(emptyFile, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}

	smallContent := []byte("hello two-peer transfer slice")
	smallFile := filepath.Join(rootA, "greeting.txt")
	if err := os.WriteFile(smallFile, smallContent, 0o600); err != nil {
		t.Fatal(err)
	}

	// Multi-chunk file (2.5 MiB = two 1 MiB chunks + one 0.5 MiB chunk)
	largeContent := make([]byte, int(2.5*float64(history.ChunkSize)))
	for i := range largeContent {
		largeContent[i] = byte(i % 251)
	}
	largeFile := filepath.Join(rootA, "payload.bin")
	if err := os.WriteFile(largeFile, largeContent, 0o600); err != nil {
		t.Fatal(err)
	}

	// Scan on Node A
	if output, err := exec.Command(binary, "scan", "--state", stateA, "--folder", folder).CombinedOutput(); err != nil {
		t.Fatalf("scan A: %v\n%s", err, output)
	}

	// Inspect on Node A
	inspOut, err := exec.Command(binary, "inspect", "--state", stateA, "--folder", folder).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect A: %v\n%s", err, inspOut)
	}
	for _, expected := range []string{"empty.txt", "greeting.txt", "payload.bin"} {
		if !strings.Contains(string(inspOut), expected) {
			t.Fatalf("inspect missing %s: %s", expected, inspOut)
		}
	}

	// Start Node A serve daemon on loopback port
	serveCmd := exec.Command(binary, "serve", "--state", stateA, "--peer-listen", "127.0.0.1:0")
	stdoutPipe, err := serveCmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	serveCmd.Stderr = os.Stderr
	if err := serveCmd.Start(); err != nil {
		t.Fatalf("start serve: %v", err)
	}
	defer func() {
		_ = serveCmd.Process.Kill()
		_ = serveCmd.Wait()
	}()

	// Read peer-listener address from serve output
	peerURL := readListenerURL(t, stdoutPipe)

	// Node B runs sync to pull from Node A
	syncOut, err := exec.Command(
		binary,
		"sync",
		"--state", stateB,
		"--folder", folder,
		"--peer-url", peerURL,
		"--peer-device", devA,
		"--peer-certificate", certPathA,
		"--json",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("sync B: %v\n%s", err, syncOut)
	}

	var syncResult struct {
		Inventoried     int `json:"inventoried"`
		MetadataAdded   int `json:"metadata_added"`
		ChunksFetched   int `json:"chunks_fetched"`
		ChunksReused    int `json:"chunks_reused"`
		VersionsStored  int `json:"versions_stored"`
		ReceiptsSent    int `json:"receipts_sent"`
		VersionsApplied int `json:"versions_applied"`
	}
	if err := json.Unmarshal(syncOut, &syncResult); err != nil {
		t.Fatalf("parse sync JSON: %v\n%s", err, syncOut)
	}
	if syncResult.Inventoried != 3 || syncResult.VersionsStored != 3 || syncResult.VersionsApplied != 3 || syncResult.ReceiptsSent != 3 {
		t.Fatalf("unexpected sync result: %+v", syncResult)
	}

	// Verify all 3 files exist in rootB and match byte-for-byte
	for _, testFile := range []struct {
		name     string
		expected []byte
	}{
		{"empty.txt", []byte{}},
		{"greeting.txt", smallContent},
		{"payload.bin", largeContent},
	} {
		pathB := filepath.Join(rootB, testFile.name)
		got, err := os.ReadFile(pathB)
		if err != nil {
			t.Fatalf("read %s on B: %v", testFile.name, err)
		}
		if !bytes.Equal(got, testFile.expected) {
			t.Fatalf("content mismatch for %s: got len %d, want len %d", testFile.name, len(got), len(testFile.expected))
		}
	}

	// Status on Node B: verify versions are stored=true, applied=true
	statusOutB, err := exec.Command(binary, "status", "--state", stateB, "--folder", folder, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("status B: %v\n%s", err, statusOutB)
	}
	var bStatus struct {
		Versions []repository.VersionStatus `json:"versions"`
		Peers    []repository.PeerProgress  `json:"peers"`
	}
	if err := json.Unmarshal(statusOutB, &bStatus); err != nil {
		t.Fatalf("parse status B JSON: %v\n%s", err, statusOutB)
	}
	if len(bStatus.Versions) != 3 {
		t.Fatalf("expected 3 versions on B, got %d", len(bStatus.Versions))
	}
	for _, v := range bStatus.Versions {
		if !v.Stored || !v.Applied || v.ContentState != "ready" {
			t.Fatalf("version %s on B not stored+applied: %+v", v.Path, v)
		}
	}

	// Stop Node A serve daemon before opening workspace on Node A
	_ = serveCmd.Process.Kill()
	_ = serveCmd.Wait()

	// Status on Node A: verify durable receipts recorded for peer B
	statusOutA, err := exec.Command(binary, "status", "--state", stateA, "--folder", folder, "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("status A: %v\n%s", err, statusOutA)
	}
	var aStatus struct {
		Versions []repository.VersionStatus `json:"versions"`
		Peers    []repository.PeerProgress  `json:"peers"`
	}
	if err := json.Unmarshal(statusOutA, &aStatus); err != nil {
		t.Fatalf("parse status A JSON: %v\n%s", err, statusOutA)
	}
	if len(aStatus.Peers) != 3 {
		t.Fatalf("expected 3 peer progress entries on A, got %d", len(aStatus.Peers))
	}
	for _, p := range aStatus.Peers {
		if !p.Receipt {
			t.Fatalf("expected receipt for peer %x version %v, got false", p.Peer, p.Version)
		}
	}
}

func parseIdentityOutput(t *testing.T, output []byte) (string, string, []byte) {
	t.Helper()
	lines := strings.Split(string(output), "\n")
	var dev, pin string
	for _, line := range lines {
		if strings.HasPrefix(line, "device=") {
			parts := strings.Split(line, " ")
			for _, part := range parts {
				if strings.HasPrefix(part, "device=") {
					dev = strings.TrimPrefix(part, "device=")
				} else if strings.HasPrefix(part, "key-pin=") {
					pin = strings.TrimPrefix(part, "key-pin=")
				}
			}
			break
		}
	}
	if dev == "" || pin == "" {
		t.Fatalf("unable to parse device or pin from output: %s", output)
	}
	certIdx := strings.Index(string(output), "-----BEGIN CERTIFICATE-----")
	if certIdx == -1 {
		t.Fatalf("no certificate PEM found in output: %s", output)
	}
	return dev, pin, output[certIdx:]
}

func readListenerURL(t *testing.T, stdout io.Reader) string {
	t.Helper()
	scanner := bufio.NewScanner(stdout)
	timeout := time.After(5 * time.Second)
	ch := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "peer-listener=") {
				idx := strings.Index(line, "peer-listener=")
				part := line[idx+len("peer-listener="):]
				fields := strings.Fields(part)
				if len(fields) > 0 {
					ch <- "https://" + fields[0]
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			errCh <- err
		} else {
			errCh <- errors.New("serve stdout closed before peer-listener line was found")
		}
	}()

	select {
	case url := <-ch:
		return url
	case err := <-errCh:
		t.Fatalf("read serve listener: %v", err)
		return ""
	case <-timeout:
		t.Fatal("timed out waiting for serve daemon peer-listener")
		return ""
	}
}
