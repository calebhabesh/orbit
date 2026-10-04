package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestRelocationPreservesHistoryAndRecovery(t *testing.T) {
	for _, hook := range []string{"", HookRelocationPrepared, HookRelocationMoved, HookRelocationCommitted} {
		t.Run(hook, func(t *testing.T) {
			ctx := context.Background()
			w, db, folder, root := testWorkspace(t)
			// Explicit disposable marker; all simulated failures are in t.TempDir roots.
			if err := os.WriteFile(filepath.Join(db.StateDir(), ".disposable"), []byte("relocation test"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "note"), []byte("original"), 0o755); err != nil {
				t.Fatal(err)
			}
			if _, err := w.Scan(ctx, folder); err != nil {
				t.Fatal(err)
			}
			before, err := db.Root(ctx, folder)
			if err != nil {
				t.Fatal(err)
			}
			heads, err := db.Heads(ctx, folder, "note")
			if err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(filepath.Dir(root), "relocated")
			w.hook = func(name string) error {
				if name == hook {
					return errors.New("simulated stop")
				}
				return nil
			}
			_, err = w.Relocate(ctx, folder, root, dest)
			if hook != "" && err == nil {
				t.Fatal("fault hook was not reached")
			}
			if hook == "" && err != nil {
				t.Fatal(err)
			}
			// A new Workspace exercises disk-backed recovery rather than in-memory state.
			restarted := New(db, Options{})
			if err := restarted.Revalidate(ctx, folder); err != nil {
				t.Fatal(err)
			}
			reg, err := db.Root(ctx, folder)
			if err != nil {
				t.Fatal(err)
			}
			want := dest
			if hook == HookRelocationPrepared {
				want = root
			}
			if reg.Path != want || reg.Paused || reg.RegistrationID != before.RegistrationID || reg.Inode != before.Inode {
				t.Fatalf("registration=%+v before=%+v", reg, before)
			}
			result, err := restarted.Scan(ctx, folder)
			if err != nil || len(result.Captured) != 0 || result.Deletion != nil {
				t.Fatalf("scan fabricated changes: %+v %v", result, err)
			}
			after, err := db.Heads(ctx, folder, "note")
			if err != nil || len(after) != 1 || after[0].ID != heads[0].ID {
				t.Fatalf("history changed: %+v %v", after, err)
			}
			data, err := os.ReadFile(filepath.Join(want, "note"))
			if err != nil || string(data) != "original" {
				t.Fatalf("bytes=%q error=%v", data, err)
			}
			if _, err := restarted.Relocate(ctx, folder, root, dest); err != nil {
				t.Fatalf("retry: %v", err)
			}
		})
	}
}

func TestRelocationDestinationRefusalsAndPausedState(t *testing.T) {
	ctx := context.Background()
	w, db, folder, root := testWorkspace(t)
	existing := t.TempDir()
	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(existing, link); err != nil {
		t.Fatal(err)
	}
	for _, dest := range []string{existing, root + "/child", filepath.Dir(root), db.StateDir() + "/child", link + "/child", filepath.Join(t.TempDir(), "missing", "child")} {
		if _, err := w.Relocate(ctx, folder, root, dest); err == nil {
			t.Fatalf("accepted %s", dest)
		}
	}
	if _, err := w.Relocate(ctx, folder, "stale", filepath.Join(t.TempDir(), "dest")); err == nil {
		t.Fatal("accepted stale current location")
	}
	if err := w.Pause(ctx, folder, "user pause"); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "dest")
	if _, err := w.Relocate(ctx, folder, root, dest); err != nil {
		t.Fatal(err)
	}
	reg, err := db.Root(ctx, folder)
	if err != nil || !reg.Paused || reg.PauseReason != "user pause" {
		t.Fatalf("pause lost: %+v %v", reg, err)
	}
}

func TestRelocationWaitsForCapture(t *testing.T) {
	ctx := context.Background()
	w, _, folder, root := testWorkspace(t)
	if err := os.WriteFile(filepath.Join(root, "note"), []byte("captured"), 0o600); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	w.hook = func(name string) error {
		if name == HookCaptureRead {
			close(entered)
			<-release
		}
		return nil
	}
	scanDone := make(chan error, 1)
	go func() { _, err := w.Scan(ctx, folder); scanDone <- err }()
	<-entered
	moveDone := make(chan error, 1)
	dest := filepath.Join(t.TempDir(), "dest")
	go func() { _, err := w.Relocate(ctx, folder, root, dest); moveDone <- err }()
	select {
	case err := <-moveDone:
		t.Fatalf("move overtook capture: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-scanDone; err != nil {
		t.Fatal(err)
	}
	if err := <-moveDone; err != nil {
		t.Fatal(err)
	}
}

func TestRelocationAcrossFilesystems(t *testing.T) {
	ctx := context.Background()
	w, db, folder, root := testWorkspace(t)
	var src, other unix.Stat_t
	if err := unix.Stat(root, &src); err != nil {
		t.Fatal(err)
	}
	if err := unix.Stat("/dev/shm", &other); err != nil || src.Dev == other.Dev {
		t.Skip("a second writable filesystem is unavailable")
	}
	parent, err := os.MkdirTemp("/dev/shm", "orbit-relocation-test-")
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { os.RemoveAll(parent) })
	if err := os.WriteFile(filepath.Join(parent, ".disposable"), []byte("relocation test"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "note"), []byte("across disks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}
	heads, _ := db.Heads(ctx, folder, "note")
	result, err := w.Relocate(ctx, folder, root, filepath.Join(parent, "dest"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.SourceRetained {
		t.Fatal("source safety copy not reported")
	}
	replay, err := w.Relocate(ctx, folder, root, result.Path)
	if err != nil || !replay.SourceRetained {
		t.Fatalf("replay lost safety copy: %+v %v", replay, err)
	}

	for _, base := range []string{root, result.Path} {
		data, err := os.ReadFile(filepath.Join(base, "note"))
		if err != nil || string(data) != "across disks" {
			t.Fatalf("%s: %q %v", base, data, err)
		}
		stat, err := os.Stat(filepath.Join(base, "note"))
		if err != nil || stat.Mode().Perm() != 0o755 {
			t.Fatalf("mode lost: %v %v", stat, err)
		}
	}
	scan, err := w.Scan(ctx, folder)
	if err != nil || len(scan.Captured) != 0 {
		t.Fatalf("fabricated changes: %+v %v", scan, err)
	}
	after, _ := db.Heads(ctx, folder, "note")
	if len(after) != 1 || after[0].ID != heads[0].ID {
		t.Fatal("history changed")
	}
}

func TestRelocationCrossFilesystemRecoveryAndCopyRefusal(t *testing.T) {
	for _, scenario := range []string{HookRelocationPrepared, HookRelocationMoved, HookRelocationCommitted, "editor-change", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			w, db, folder, root := testWorkspace(t)
			var src, other unix.Stat_t
			if err := unix.Stat(root, &src); err != nil {
				t.Fatal(err)
			}
			if err := unix.Stat("/dev/shm", &other); err != nil || src.Dev == other.Dev {
				t.Skip("second filesystem unavailable")
			}
			parent, err := os.MkdirTemp("/dev/shm", "orbit-relocation-fault-")
			if err != nil {
				t.Skip(err)
			}
			if err := os.WriteFile(filepath.Join(parent, ".filesync-disposable"), []byte("disposable"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.RemoveAll(parent) })
			path := filepath.Join(root, "note")
			if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := w.Scan(ctx, folder); err != nil {
				t.Fatal(err)
			}
			if scenario == "symlink" {
				if err := os.Symlink("/etc/passwd", filepath.Join(root, "unsupported")); err != nil {
					t.Fatal(err)
				}
			}
			w.hook = func(name string) error {
				if scenario == "editor-change" && name == HookRelocationCopied {
					return os.WriteFile(path, []byte("editor update"), 0o600)
				}
				if name == scenario {
					return errors.New("simulated interruption")
				}
				return nil
			}
			dest := filepath.Join(parent, "dest")
			if _, err := w.Relocate(ctx, folder, root, dest); err == nil {
				t.Fatal("expected safe refusal or fault")
			}
			restarted := New(db, Options{})
			if err := restarted.Revalidate(ctx, folder); err != nil {
				t.Fatal(err)
			}
			reg, err := db.Root(ctx, folder)
			if err != nil {
				t.Fatal(err)
			}
			want := root
			if scenario == HookRelocationMoved || scenario == HookRelocationCommitted {
				want = dest
			}
			if reg.Path != want || reg.Paused {
				t.Fatalf("bad recovery: %+v", reg)
			}
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			expected := "original"
			if scenario == "editor-change" {
				expected = "editor update"
			}
			if string(original) != expected {
				t.Fatalf("original changed: %q", original)
			}
			if want == dest {
				data, err := os.ReadFile(filepath.Join(dest, "note"))
				if err != nil || string(data) != "original" {
					t.Fatalf("copy %q %v", data, err)
				}
			}
		})
	}
}
