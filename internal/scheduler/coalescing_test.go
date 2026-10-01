package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/scheduler"
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
