package control

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/repository"
)

func TestWorkControlOperations(t *testing.T) {
	ctx := context.Background()
	env := setupTestEnv(t)

	// 1. Create a file and run WorkScan
	filePath := filepath.Join(env.rootDir, "file1.txt")
	if err := os.WriteFile(filePath, []byte("hello continuous sync"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	scanRes, err := env.ctrl.WorkScan(ctx, WorkScanRequest{
		Folder:         &env.folder,
		FullContent:    false,
		IdempotencyKey: "scan-key-1",
	})
	if err != nil {
		t.Fatalf("work scan: %v", err)
	}
	if scanRes.CapturedCount != 1 {
		t.Fatalf("expected 1 captured, got %d", scanRes.CapturedCount)
	}
	if scanRes.Replay {
		t.Fatalf("expected fresh result, got replay")
	}

	// Test idempotency replay
	scanReplay, err := env.ctrl.WorkScan(ctx, WorkScanRequest{
		Folder:         &env.folder,
		FullContent:    false,
		IdempotencyKey: "scan-key-1",
	})
	if err != nil {
		t.Fatalf("work scan replay: %v", err)
	}
	if !scanReplay.Replay {
		t.Fatalf("expected replay result")
	}

	// 2. Enqueue durable tasks and check WorkStatus and WorkList
	_, err = env.db.EnqueueDurableTask(ctx, repository.DurableTask{
		ID:        "task-1",
		Folder:    env.folder,
		Kind:      "scan",
		State:     "queued",
		CreatedNS: time.Now().UnixNano(),
	})
	if err != nil {
		t.Fatalf("enqueue task 1: %v", err)
	}

	_, err = env.db.EnqueueDurableTask(ctx, repository.DurableTask{
		ID:        "task-2",
		Folder:    env.folder,
		Kind:      "sync",
		State:     "exhausted",
		ErrorCode: "ROOT_UNAVAILABLE",
		CreatedNS: time.Now().UnixNano(),
	})
	if err != nil {
		t.Fatalf("enqueue task 2: %v", err)
	}

	statusRes, err := env.ctrl.WorkStatus(ctx, WorkStatusRequest{
		Folder: &env.folder,
	})
	if err != nil {
		t.Fatalf("work status: %v", err)
	}
	if len(statusRes.Folders) != 1 {
		t.Fatalf("expected 1 folder status, got %d", len(statusRes.Folders))
	}
	folderStatus := statusRes.Folders[0]
	if folderStatus.QueuedTasks != 1 || folderStatus.ExhaustedTasks != 1 {
		t.Fatalf("unexpected task counts: queued=%d exhausted=%d", folderStatus.QueuedTasks, folderStatus.ExhaustedTasks)
	}
	if !folderStatus.Paused || folderStatus.PauseReason != "ROOT_UNAVAILABLE" {
		t.Fatalf("expected paused folder with ROOT_UNAVAILABLE, got paused=%v reason=%s", folderStatus.Paused, folderStatus.PauseReason)
	}

	// 3. WorkList
	listRes, err := env.ctrl.WorkList(ctx, WorkListRequest{
		Folder: env.folder,
	})
	if err != nil {
		t.Fatalf("work list: %v", err)
	}
	if len(listRes.Tasks) != 2 {
		t.Fatalf("expected 2 tasks in list, got %d", len(listRes.Tasks))
	}

	// 4. WorkRetry (retry exhausted task-2)
	retryRes, err := env.ctrl.WorkRetry(ctx, WorkRetryRequest{
		TaskID: "task-2",
	})
	if err != nil {
		t.Fatalf("work retry: %v", err)
	}
	if retryRes.RetriedCount != 1 {
		t.Fatalf("expected 1 retried, got %d", retryRes.RetriedCount)
	}

	t2, err := env.db.GetDurableTask(ctx, "task-2")
	if err != nil {
		t.Fatalf("get task 2: %v", err)
	}
	if t2.State != "queued" {
		t.Fatalf("expected task 2 state to be queued, got %s", t2.State)
	}

	// 5. WorkCancel (cancel task-1)
	cancelRes, err := env.ctrl.WorkCancel(ctx, WorkCancelRequest{
		TaskID: "task-1",
	})
	if err != nil {
		t.Fatalf("work cancel: %v", err)
	}
	if cancelRes.Status != "canceled" {
		t.Fatalf("expected canceled status, got %s", cancelRes.Status)
	}

	t1, err := env.db.GetDurableTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("get task 1: %v", err)
	}
	if t1.State != "canceled" {
		t.Fatalf("expected task 1 state to be canceled, got %s", t1.State)
	}
}
