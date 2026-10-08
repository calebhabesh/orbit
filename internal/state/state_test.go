package state

import (
	"errors"
	"os"
	"os/exec"
	"testing"
)

func TestValidateDirectoryRejectsBroadPermissions(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDirectory(dir); err == nil {
		t.Fatal("ValidateDirectory accepted group-accessible state")
	}
}

func TestExclusiveLockAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	lock, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()

	cmd := exec.Command(os.Args[0], "-test.run=^TestLockHelper$")
	cmd.Env = append(os.Environ(), "ORBIT_LOCK_HELPER=1", "ORBIT_LOCK_DIR="+dir)
	err = cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 42 {
		t.Fatalf("second process error = %v, want exit code 42", err)
	}
}

func TestLockHelper(t *testing.T) {
	if os.Getenv("ORBIT_LOCK_HELPER") != "1" {
		return
	}
	lock, err := Acquire(os.Getenv("ORBIT_LOCK_DIR"))
	if errors.Is(err, ErrLocked) {
		os.Exit(42)
	}
	if err == nil {
		lock.Close()
	}
	os.Exit(43)
}
