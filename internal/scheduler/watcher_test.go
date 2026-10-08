package scheduler_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/scheduler"
)

func TestWatcherFileEventsAndDebouncing(t *testing.T) {
	dir := t.TempDir()
	w, err := scheduler.NewWatcher(50 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	var folder history.ID
	folder[0] = 0x12
	if err := w.WatchFolder(folder, dir); err != nil {
		t.Fatal(err)
	}

	testFile := filepath.Join(dir, "example.txt")

	// Write file rapidly 3 times
	for i := 0; i < 3; i++ {
		if err := os.WriteFile(testFile, []byte("test content"), 0o600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Should receive exactly 1 debounced event
	select {
	case evt := <-w.Events():
		if evt.Folder != folder {
			t.Fatalf("unexpected folder: %x", evt.Folder)
		}
		if evt.Path != "example.txt" {
			t.Fatalf("unexpected path: %s", evt.Path)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for watcher event")
	}

	// Verify no immediate second event
	select {
	case extra := <-w.Events():
		t.Fatalf("unexpected duplicate event: %+v", extra)
	case <-time.After(100 * time.Millisecond):
		// Clean debounce
	}
}

func TestWatcherSubdirectoryCreation(t *testing.T) {
	dir := t.TempDir()
	w, err := scheduler.NewWatcher(50 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	var folder history.ID
	folder[0] = 0x34
	if err := w.WatchFolder(folder, dir); err != nil {
		t.Fatal(err)
	}

	// Create new subdirectory
	subDir := filepath.Join(dir, "nested")
	if err := os.Mkdir(subDir, 0o700); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	// Create file inside newly watched subdirectory
	subFile := filepath.Join(subDir, "inner.txt")
	if err := os.WriteFile(subFile, []byte("inner data"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Drain events to find inner.txt
	foundInner := false
	timeout := time.After(1 * time.Second)
	for !foundInner {
		select {
		case evt := <-w.Events():
			if evt.Path == filepath.Join("nested", "inner.txt") {
				foundInner = true
			}
		case <-timeout:
			t.Fatal("timed out waiting for inner file event")
		}
	}
}
