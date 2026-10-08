package scheduler_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/scheduler"
)

func TestWANW11ContinuousSmallWorkCannotStarveLargeRetry(t *testing.T) {
	ctx := context.Background()
	db, err := repository.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	folder, dev := history.ID{1}, history.ID{2}
	if err = db.EnsureFolder(ctx, folder, dev, 1); err != nil {
		t.Fatal(err)
	}
	q := scheduler.NewQueue(db, 1024)
	large, err := q.Enqueue(ctx, repository.DurableTask{Folder: folder, Kind: "scan", TargetPath: "large", FileSize: 1 << 40})
	if err != nil {
		t.Fatal(err)
	}
	for cycle := range 2 {
		selectedLarge := false
		for turn := 0; turn <= scheduler.MaxReadySkips; turn++ {
			_, err = q.Enqueue(ctx, repository.DurableTask{Folder: folder, Kind: "scan", TargetPath: fmt.Sprintf("tiny-%d-%d", cycle, turn), FileSize: 1})
			if err != nil {
				t.Fatal(err)
			}
			task, e := q.NextReadyTask(ctx, nil, time.Now())
			if e != nil || task == nil {
				t.Fatal(task, e)
			}
			if task.ID == large {
				selectedLarge = true
				stored, e := db.GetDurableTask(ctx, large)
				if e != nil || stored.AgeCounter != 0 {
					t.Fatal("dispatch did not reset durable age", stored, e)
				}
				if e = q.UpdateState(ctx, large, "retry", cycle+1, "route lost", "CONNECTION_FAILED", time.Now().Add(-time.Second).UnixNano()); e != nil {
					t.Fatal(e)
				}
				break
			}
			if e = q.UpdateState(ctx, task.ID, "completed", 0, "", "", 0); e != nil {
				t.Fatal(e)
			}
			// Restart the queue: persisted aging and the original task ID must survive.
			q = scheduler.NewQueue(db, 1024)
			if e = q.LoadFromDB(ctx); e != nil {
				t.Fatal(e)
			}
		}
		if !selectedLarge {
			t.Fatal("large task starved under continuous tiny work", cycle)
		}
	}
	t.Log("1-TiB declared task selected within nine ready dispatches twice, including retry and queue reload; no 1-TiB payload transferred")
}

func TestWANW11ServiceQuotaRetryAllowsQuietRefill(t *testing.T) {
	c := scheduler.RetryClassifier{}
	for attempt := 1; attempt < 6; attempt++ {
		if got := c.BackoffFor(&network.ServiceError{Code: protocol.NetworkQuota}, attempt); got < network.ServiceRetryQuietPeriod {
			t.Fatal(got)
		}
	}
	if c.IsTransient(&network.ServiceError{Code: protocol.NetworkIdentityMismatch}) {
		t.Fatal("identity treated as network failure")
	}
}
