package testkit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateDestructiveTarget(t *testing.T) {
	root := NewDisposable(t)
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDestructiveTarget(root, target); err != nil {
		t.Fatalf("valid target rejected: %v", err)
	}
	if err := ValidateDestructiveTarget(root, root); err == nil {
		t.Fatal("root itself accepted as destructive target")
	}
	outside := t.TempDir()
	if err := ValidateDestructiveTarget(root, outside); err == nil {
		t.Fatal("outside directory accepted as destructive target")
	}
	symlink := filepath.Join(root, "link")
	if err := os.Symlink(target, symlink); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDestructiveTarget(root, symlink); err == nil {
		t.Fatal("symlink accepted as destructive target")
	}
}

func TestWANW15DestructiveTargetRefusesSymlinkedRoot(t *testing.T) {
	root := NewDisposable(t)
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDestructiveTarget(alias, filepath.Join(alias, "target")); err == nil {
		t.Fatal("symlinked disposable root accepted")
	}
}
