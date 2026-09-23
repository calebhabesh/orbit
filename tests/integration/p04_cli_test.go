package integration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/calebhabesh/file-sync/internal/testkit"
)

func TestP04CLIRegisterScanInspect(t *testing.T) {
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
	state, root := filepath.Join(disposable, "state"), filepath.Join(disposable, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	folder := strings.Repeat("f", 64)
	for _, args := range [][]string{
		{"init", "--state", state},
		{"register", "--state", state, "--folder", folder, "--root", root},
	} {
		if output, err := exec.Command(binary, args...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
	}
	identityOutput, err := exec.Command(binary, "identity", "--state", state, "--certificate").CombinedOutput()
	if err != nil || !strings.Contains(string(identityOutput), "key-pin=") || !strings.Contains(string(identityOutput), "BEGIN CERTIFICATE") {
		t.Fatalf("identity export: %v\n%s", err, identityOutput)
	}
	peerDevice, peerPin := strings.Repeat("b", 64), strings.Repeat("c", 64)
	for attempt := 0; attempt < 2; attempt++ {
		output, err := exec.Command(binary, "pair-approve", "--state", state, "--folder", folder, "--peer-device", peerDevice, "--peer-key-pin", peerPin).CombinedOutput()
		if err != nil || !strings.Contains(string(output), "approved membership revision=1") {
			t.Fatalf("pair approval attempt %d: %v\n%s", attempt+1, err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("captured"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(binary, "scan", "--state", state, "--folder", folder).CombinedOutput()
	if err != nil || !strings.Contains(string(output), "captured note.txt") {
		t.Fatalf("scan: %v\n%s", err, output)
	}
	output, err = exec.Command(binary, "inspect", "--state", state, "--folder", folder).CombinedOutput()
	if err != nil || !strings.Contains(string(output), "note.txt kind=1 basis=1") {
		t.Fatalf("inspect: %v\n%s", err, output)
	}
}
