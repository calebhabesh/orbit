package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
)

func TestDurableWorkTasksLifecycleAndCoalescing(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := repository.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var folderID history.ID
	folderID[0] = 0xAA
	localDevice := history.ID{1}
	if err := db.EnsureFolder(ctx, folderID, localDevice, 1); err != nil {
		t.Fatal(err)
	}
	_, err = db.ApproveMembership(ctx, protocol.Membership{
		Folder:   folderID,
		Revision: 1,
		Active:   []protocol.ActiveMember{{Device: localDevice, KeyPin: history.Digest{1}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	reg := repository.RootRegistration{
		Folder:         folderID,
		Path:           t.TempDir(),
		Device:         1,
		Inode:          1,
		RegistrationID: [32]byte{1},
	}
	if err := db.RegisterRoot(ctx, reg); err != nil {
		t.Fatal(err)
	}

	// 1. Enqueue scan task
	taskID1, err := db.EnqueueDurableTask(ctx, repository.DurableTask{
		Folder: folderID,
		Kind:   "scan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if taskID1 == "" {
		t.Fatal("empty task id")
	}

	// 2. Coalescing: another full scan for same folder returns same taskID
	taskID2, err := db.EnqueueDurableTask(ctx, repository.DurableTask{
		Folder: folderID,
		Kind:   "scan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if taskID1 != taskID2 {
		t.Fatalf("expected coalescing to return taskID1 %s, got %s", taskID1, taskID2)
	}

	// 3. Get task
	task, err := db.GetDurableTask(ctx, taskID1)
	if err != nil {
		t.Fatal(err)
	}
	if task.State != "queued" || task.Kind != "scan" {
		t.Fatalf("unexpected task: %+v", task)
	}

	// 4. Update state to running
	if err := db.UpdateDurableTaskState(ctx, taskID1, "running", 1, "", "", 0); err != nil {
		t.Fatal(err)
	}

	// 5. Simulate crash and recovery: RecoverInFlightDurableTasks resets running -> queued
	recovered, err := db.RecoverInFlightDurableTasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if recovered != 1 {
		t.Fatalf("recovered = %d, want 1", recovered)
	}
	task, err = db.GetDurableTask(ctx, taskID1)
	if err != nil {
		t.Fatal(err)
	}
	if task.State != "queued" {
		t.Fatalf("expected state queued, got %s", task.State)
	}

	// 6. Transition to retry with backoff
	retryAfter := time.Now().Add(500 * time.Millisecond).UnixNano()
	if err := db.UpdateDurableTaskState(ctx, taskID1, "retry", 2, "transient network drop", "IO_TIMEOUT", retryAfter); err != nil {
		t.Fatal(err)
	}

	// 7. Retry manually
	if err := db.RetryDurableTask(ctx, taskID1); err != nil {
		t.Fatal(err)
	}
	task, err = db.GetDurableTask(ctx, taskID1)
	if err != nil {
		t.Fatal(err)
	}
	if task.State != "queued" || task.Attempts != 0 {
		t.Fatalf("expected state queued with attempts 0, got state=%s attempts=%d", task.State, task.Attempts)
	}

	// 8. Increment age
	if err := db.IncrementDurableTaskAge(ctx, []string{taskID1}); err != nil {
		t.Fatal(err)
	}
	task, err = db.GetDurableTask(ctx, taskID1)
	if err != nil {
		t.Fatal(err)
	}
	if task.AgeCounter != 1 {
		t.Fatalf("expected age 1, got %d", task.AgeCounter)
	}

	// 9. Cancel task
	if err := db.CancelDurableTask(ctx, taskID1); err != nil {
		t.Fatal(err)
	}
	task, err = db.GetDurableTask(ctx, taskID1)
	if err != nil {
		t.Fatal(err)
	}
	if task.State != "canceled" {
		t.Fatalf("expected state canceled, got %s", task.State)
	}
}

func TestDurableWorkQueueBounds(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := repository.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var folderID history.ID
	folderID[0] = 0xBB
	localDevice := history.ID{2}
	if err := db.EnsureFolder(ctx, folderID, localDevice, 1); err != nil {
		t.Fatal(err)
	}
	_, err = db.ApproveMembership(ctx, protocol.Membership{
		Folder:   folderID,
		Revision: 1,
		Active:   []protocol.ActiveMember{{Device: localDevice, KeyPin: history.Digest{1}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Enqueue up to capacity
	for i := 0; i < repository.MaxQueueCapacity; i++ {
		_, err := db.EnqueueDurableTask(ctx, repository.DurableTask{
			Folder:     folderID,
			Kind:       "scan",
			TargetPath: fmt.Sprintf("path/to/file_%d.txt", i),
		})
		if err != nil {
			t.Fatalf("failed enqueuing task %d: %v", i, err)
		}
	}

	// Next task should be rejected with ErrQueueFull
	_, err = db.EnqueueDurableTask(ctx, repository.DurableTask{
		Folder:     folderID,
		Kind:       "scan",
		TargetPath: "path/overflow.txt",
	})
	if err != repository.ErrQueueFull {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}

	count, err := db.CountQueuedDurableTasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != repository.MaxQueueCapacity {
		t.Fatalf("count = %d, want %d", count, repository.MaxQueueCapacity)
	}
}
