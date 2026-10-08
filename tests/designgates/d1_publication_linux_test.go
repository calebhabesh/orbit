//go:build linux

package designgates

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/calebhabesh/orbit/internal/testkit"
	"golang.org/x/sys/unix"
)

func TestD1PreRenameStatCannotCloseSaveByRenameRace(t *testing.T) {
	root := publicationRoot(t)
	target := filepath.Join(root, "document")
	stage := filepath.Join(root, ".orbit-internal", "stage")
	mustWrite(t, target, "captured-old")
	mustWrite(t, stage, "remote")

	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	editorTemp := filepath.Join(root, ".editor-save")
	mustWrite(t, editorTemp, "local-save")
	if err := os.Rename(editorTemp, target); err != nil {
		t.Fatal(err)
	}
	afterEditor, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, afterEditor) {
		t.Fatal("save-by-rename did not replace the inode")
	}

	// This is the unsafe implementation being falsified: the earlier stat was
	// valid, but a subsequent plain rename discards the editor's new inode.
	if err := os.Rename(stage, target); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, target); got != "remote" {
		t.Fatalf("unexpected target after rename: %q", got)
	}
	if _, err := os.Stat(editorTemp); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("editor temporary unexpectedly survived: %v", err)
	}
	t.Log("counterexample: target passed pre-rename stat, editor atomically saved a new inode, then plain rename removed its only name")
}

func TestD1ExchangePreservesObservedOverwriteAndSaveByRename(t *testing.T) {
	t.Run("in-place-overwrite-before-exchange", func(t *testing.T) {
		root := publicationRoot(t)
		target := filepath.Join(root, "document")
		stage := filepath.Join(root, ".orbit-internal", "stage")
		mustWrite(t, target, "captured-old")
		mustWrite(t, stage, "remote")
		mustWrite(t, target, "local-overwrite")
		exchange(t, target, stage)
		if got := mustRead(t, target); got != "remote" {
			t.Fatalf("published target = %q", got)
		}
		if got := mustRead(t, stage); got != "local-overwrite" {
			t.Fatalf("displaced candidate = %q", got)
		}
	})

	t.Run("save-by-rename", func(t *testing.T) {
		root := publicationRoot(t)
		target := filepath.Join(root, "document")
		stage := filepath.Join(root, ".orbit-internal", "stage")
		mustWrite(t, target, "captured-old")
		mustWrite(t, stage, "remote")
		editorTemp := filepath.Join(root, ".editor-save")
		mustWrite(t, editorTemp, "local-save")
		if err := os.Rename(editorTemp, target); err != nil {
			t.Fatal(err)
		}
		exchange(t, target, stage)
		if got := mustRead(t, target); got != "remote" {
			t.Fatalf("published target = %q", got)
		}
		if got := mustRead(t, stage); got != "local-save" {
			t.Fatalf("displaced candidate = %q", got)
		}
	})

	t.Run("open-descriptor", func(t *testing.T) {
		root := publicationRoot(t)
		target := filepath.Join(root, "document")
		stage := filepath.Join(root, ".orbit-internal", "stage")
		mustWrite(t, target, "old-content")
		mustWrite(t, stage, "remote-data")
		writer, err := os.OpenFile(target, os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer writer.Close()
		exchange(t, target, stage)
		if _, err := writer.WriteAt([]byte("late-write!"), 0); err != nil {
			t.Fatal(err)
		}
		if got := mustRead(t, target); got != "remote-data" {
			t.Fatalf("visible target changed through old descriptor: %q", got)
		}
		if got := mustRead(t, stage); got != "late-write!" {
			t.Fatalf("displaced inode did not receive late write: %q", got)
		}
		t.Log("limit: exchange preserves the displaced inode, but no finite observation proves a long-lived writer has stopped mutating it")
	})
}

func TestD1StageMustShareTargetFilesystem(t *testing.T) {
	root := publicationRoot(t)
	target := filepath.Join(root, "document")
	stage := filepath.Join(root, ".orbit-internal", "stage")
	mustWrite(t, target, "old")
	mustWrite(t, stage, "new")
	targetInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	stageInfo, err := os.Stat(stage)
	if err != nil {
		t.Fatal(err)
	}
	if targetInfo.Sys().(*syscall.Stat_t).Dev != stageInfo.Sys().(*syscall.Stat_t).Dev {
		t.Fatal("test staging unexpectedly crosses filesystems")
	}
	exchange(t, target, stage)
}

func TestD1DescriptorRootedOpenRejectsSymlinkSwap(t *testing.T) {
	root := publicationRoot(t)
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "secret"), "outside")
	parent := filepath.Join(root, "parent")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(parent, "file"), "inside")
	rootFD, err := unix.Open(root, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(rootFD)
	if err := os.Rename(parent, filepath.Join(root, "old-parent")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, parent); err != nil {
		t.Fatal(err)
	}
	how := &unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV,
	}
	fd, err := unix.Openat2(rootFD, "parent/secret", how)
	if err == nil {
		unix.Close(fd)
		t.Fatal("descriptor-rooted open followed a swapped symlink")
	}
	if !errors.Is(err, syscall.ELOOP) && !errors.Is(err, syscall.EXDEV) {
		t.Fatalf("unexpected openat2 error: %v", err)
	}
	t.Logf("openat2 rejected swapped intermediate symlink: %v", err)
}

func TestD1RecoveryAtEveryExchangeGapPreservesVariants(t *testing.T) {
	type gap struct {
		name string
		act  func(t *testing.T, target, stage, recovery string)
	}
	gaps := []gap{
		{name: "staged"},
		{name: "exchange-complete", act: func(t *testing.T, target, stage, _ string) {
			exchange(t, target, stage)
		}},
		{name: "recovery-named", act: func(t *testing.T, target, stage, recovery string) {
			exchange(t, target, stage)
			if err := os.Rename(stage, recovery); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range gaps {
		t.Run(tc.name, func(t *testing.T) {
			root := publicationRoot(t)
			target := filepath.Join(root, "document")
			stage := filepath.Join(root, ".orbit-internal", "stage")
			recovery := filepath.Join(root, ".orbit-internal", "recovery")
			mustWrite(t, target, "old")
			mustWrite(t, stage, "new")
			if tc.act != nil {
				tc.act(t, target, stage, recovery)
			}
			variants := map[string]string{}
			for name, path := range map[string]string{"target": target, "stage": stage, "recovery": recovery} {
				contents, err := os.ReadFile(path)
				if err == nil {
					variants[name] = string(contents)
				} else if !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
			}
			seen := map[string]bool{}
			for _, contents := range variants {
				seen[contents] = true
			}
			if !seen["old"] || !seen["new"] {
				t.Fatalf("gap lost a variant: %v", variants)
			}
			t.Logf("recovery observation: %v", variants)
		})
	}
}

func publicationRoot(t *testing.T) string {
	t.Helper()
	disposable := testkit.NewDisposable(t)
	root := filepath.Join(disposable, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := testkit.ValidateDestructiveTarget(disposable, root); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".orbit-internal"), 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func exchange(t *testing.T, first, second string) {
	t.Helper()
	if err := unix.Renameat2(unix.AT_FDCWD, first, unix.AT_FDCWD, second, unix.RENAME_EXCHANGE); err != nil {
		if errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.EOPNOTSUPP) {
			t.Skipf("RENAME_EXCHANGE is unsupported by this kernel/filesystem: %v", err)
		}
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}
