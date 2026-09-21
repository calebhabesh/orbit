package testkit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const Marker = ".filesync-disposable"

func NewDisposable(t testing.TB) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, Marker), []byte("filesync test data only\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

// ValidateDestructiveTarget requires an explicit marker and an existing,
// non-symlink target strictly beneath the disposable root.
func ValidateDestructiveTarget(root, target string) error {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("make disposable root absolute: %w", err)
	}
	absoluteTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("make destructive target absolute: %w", err)
	}
	lexicalRelative, err := filepath.Rel(absoluteRoot, absoluteTarget)
	if err != nil || lexicalRelative == "." || lexicalRelative == ".." || filepath.IsAbs(lexicalRelative) || strings.HasPrefix(lexicalRelative, ".."+string(filepath.Separator)) {
		return errors.New("destructive target must be strictly beneath the disposable root")
	}
	walk := absoluteRoot
	for _, component := range strings.Split(lexicalRelative, string(filepath.Separator)) {
		walk = filepath.Join(walk, component)
		info, statErr := os.Lstat(walk)
		if statErr != nil {
			return fmt.Errorf("inspect destructive target component: %w", statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("destructive target contains a symlink")
		}
	}

	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve disposable root: %w", err)
	}
	marker, err := os.Lstat(filepath.Join(canonicalRoot, Marker))
	if err != nil || !marker.Mode().IsRegular() {
		return errors.New("disposable root marker is missing or invalid")
	}
	canonicalTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return fmt.Errorf("resolve destructive target: %w", err)
	}
	relative, err := filepath.Rel(canonicalRoot, canonicalTarget)
	if err != nil {
		return err
	}
	if relative == "." || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("destructive target must be strictly beneath the disposable root")
	}
	return nil
}
