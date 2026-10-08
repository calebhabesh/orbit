package scheduler_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/scheduler"
	"github.com/calebhabesh/orbit/internal/workspace"
)

func TestSchedulerStartStopAndScanExecution(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()
	rootDir := t.TempDir()

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var folder history.ID
	folder[0] = 0x55
	localDev := history.ID{1}
	if err := db.EnsureFolder(ctx, folder, localDev, 1); err != nil {
		t.Fatal(err)
	}
	_, err = db.ApproveMembership(ctx, protocol.Membership{
		Folder:   folder,
		Revision: 1,
		Active:   []protocol.ActiveMember{{Device: localDev, KeyPin: history.Digest{1}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	ws := workspace.New(db, workspace.Options{})
	reg, err := ws.Register(ctx, folder, rootDir)
	if err != nil {
		t.Fatal(err)
	}
	_ = reg

	// Write an initial file
	if err := os.WriteFile(filepath.Join(rootDir, "doc.txt"), []byte("scheduled doc"), 0o600); err != nil {
		t.Fatal(err)
	}

	prof := scheduler.GetProfile(scheduler.ProfileLaptop)
	prof.ReconcileInterval = 50 * time.Millisecond // fast ticker for test
	s, err := scheduler.NewScheduler(db, ws, scheduler.SchedulerOptions{
		Profile: prof,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// Wait for scan to execute and capture version
	var captured bool
	for i := 0; i < 50; i++ {
		time.Sleep(20 * time.Millisecond)
		proj, pErr := db.Projection(ctx, folder, "doc.txt")
		if pErr == nil && proj.Kind == history.KindFile {
			captured = true
			break
		}
	}
	if !captured {
		t.Fatal("expected doc.txt to be scanned and captured by scheduler")
	}

	// Verify status
	status, err := s.Status(folder)
	if err != nil {
		t.Fatal(err)
	}
	if status.Paused {
		t.Fatalf("expected folder not paused, got: %+v", status)
	}

	// Stop cleanly
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestSchedulerRootUnavailablePausesFolder(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()
	rootDir := t.TempDir()

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var folder history.ID
	folder[0] = 0x77
	localDev := history.ID{1}
	if err := db.EnsureFolder(ctx, folder, localDev, 1); err != nil {
		t.Fatal(err)
	}
	_, err = db.ApproveMembership(ctx, protocol.Membership{
		Folder:   folder,
		Revision: 1,
		Active:   []protocol.ActiveMember{{Device: localDev, KeyPin: history.Digest{1}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	ws := workspace.New(db, workspace.Options{})
	_, err = ws.Register(ctx, folder, rootDir)
	if err != nil {
		t.Fatal(err)
	}

	s, err := scheduler.NewScheduler(db, ws, scheduler.SchedulerOptions{
		Profile: scheduler.GetProfile(scheduler.ProfilePi),
		NoWatch: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	// Wait for the startup scan to complete before submitting the fault scan.
	// Otherwise an earlier scan can pause the folder while this task stays queued.
	deadline := time.Now().Add(2 * time.Second)
	for {
		completed, err := db.ListDurableTasks(ctx, repository.TaskFilter{Folder: folder, Kind: "scan", State: "completed", Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		if len(completed) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("startup scan did not complete")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Invalidate root marker
	markerPath := filepath.Join(rootDir, ".orbit-internal", "registration")
	_ = os.Remove(markerPath)

	// Submit scan task
	taskID, err := s.Submit(repository.DurableTask{
		Folder: folder,
		Kind:   "scan",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for task to fail and folder to be paused
	var paused bool
	for i := 0; i < 50; i++ {
		time.Sleep(20 * time.Millisecond)
		st, _ := s.Status(folder)
		task, err := db.GetDurableTask(ctx, taskID)
		if err != nil {
			t.Fatal(err)
		}
		// Pause state and durable task outcome are recorded at separate boundaries.
		if st.Paused && st.PauseReason == "ROOT_UNAVAILABLE" && task.ErrorCode == "ROOT_UNAVAILABLE" {
			paused = true
			break
		}
	}
	if !paused {
		t.Fatal("expected folder to be paused with ROOT_UNAVAILABLE")
	}

	task, err := db.GetDurableTask(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.ErrorCode != "ROOT_UNAVAILABLE" {
		t.Fatalf("task error code = %s, want ROOT_UNAVAILABLE", task.ErrorCode)
	}
}
