package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/scheduler"
)

func TestCoalescingCannotRedispatchRunningPeerSync(t *testing.T) {
	ctx := context.Background()
	db, err := repository.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	folder, peer := history.ID{1}, history.ID{2}
	if err := db.EnsureFolder(ctx, folder, history.ID{3}, 1); err != nil {
		t.Fatal(err)
	}
	queue := scheduler.NewQueue(db, 1024)
	task := repository.DurableTask{Folder: folder, Peer: &peer, Kind: "sync"}
	id, err := queue.Enqueue(ctx, task)
	if err != nil {
		t.Fatal(err)
	}
	next, err := queue.NextReadyTask(ctx, nil, time.Now())
	if err != nil || next == nil || next.ID != id {
		t.Fatalf("next=%+v err=%v", next, err)
	}
	again, err := queue.Enqueue(ctx, task)
	if err != nil || again != id {
		t.Fatalf("coalesced=%s err=%v", again, err)
	}
	next, err = queue.NextReadyTask(ctx, nil, time.Now())
	if err != nil || next != nil {
		t.Fatalf("running work dispatched twice: %+v %v", next, err)
	}
}

func TestQueueRestartLoadsActiveWorkBeyondCompletedHistory(t *testing.T) {
	ctx := context.Background()
	db, err := repository.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	folder := history.ID{1}
	if err := db.EnsureFolder(ctx, folder, history.ID{2}, 1); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < repository.MaxQueueCapacity; i++ {
		id, err := db.EnqueueDurableTask(ctx, repository.DurableTask{Folder: folder, Kind: "scan"})
		if err != nil {
			t.Fatal(err)
		}
		if err := db.UpdateDurableTaskState(ctx, id, "completed", 0, "", "", 0); err != nil {
			t.Fatal(err)
		}
	}
	id, err := db.EnqueueDurableTask(ctx, repository.DurableTask{Folder: folder, Kind: "scan", TargetPath: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	queue := scheduler.NewQueue(db, 1024)
	if err := queue.LoadFromDB(ctx); err != nil {
		t.Fatal(err)
	}
	next, err := queue.NextReadyTask(ctx, nil, time.Now())
	if err != nil || next == nil || next.ID != id {
		t.Fatalf("pending work hidden by completed history: %+v %v", next, err)
	}
}
