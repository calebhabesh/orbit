package integration_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitAndVersionFromBuiltCLI(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "orbit")
	build := exec.Command("go", "build", "-o", binary, "./cmd/orbit")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	stateDir := filepath.Join(t.TempDir(), "state")
	initCommand := exec.Command(binary, "engine", "init", "--state", stateDir)
	if output, err := initCommand.CombinedOutput(); err != nil {
		t.Fatalf("init CLI: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "metadata.sqlite")); err != nil {
		t.Fatal(err)
	}
	versionCommand := exec.Command(binary, "engine", "version")
	output, err := versionCommand.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(output), "orbit ") {
		t.Fatalf("unexpected version output: %q", output)
	}
}
