package scheduler_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/scheduler"
	"github.com/calebhabesh/orbit/internal/workspace"
)

// An unfinished join publishes its folder itself. Scheduled work on that
// working tree waits for the join, without spending task attempts.
func TestSchedulerDefersWorkOnJoiningFolder(t *testing.T) {
	ctx := context.Background()
	db, err := repository.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	root := t.TempDir()
	folder := history.ID{0x56}
	if err := db.EnsureFolder(ctx, folder, history.ID{1}, 1); err != nil {
		t.Fatal(err)
	}
	ws := workspace.New(db, workspace.Options{})
	if _, err := ws.Register(ctx, folder, root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "doc.txt"), []byte("joining"), 0o600); err != nil {
		t.Fatal(err)
	}
	var joining atomic.Bool
	joining.Store(true)
	var checks atomic.Int32
	s, err := scheduler.NewScheduler(db, ws, scheduler.SchedulerOptions{
		Profile: scheduler.GetProfile(scheduler.ProfileLaptop),
		NoWatch: true,
		Joining: func(context.Context) (map[history.ID]bool, error) {
			checks.Add(1)
			return map[history.ID]bool{folder: joining.Load()}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	for deadline := time.Now().Add(5 * time.Second); checks.Load() < 2; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("joining check ran %d times", checks.Load())
		}
	}
	if _, err := db.Projection(ctx, folder, "doc.txt"); err == nil {
		t.Fatal("scheduled scan captured a joining folder")
	}
	tasks, err := s.ListTasks(repository.TaskFilter{Folder: folder})
	if err != nil || len(tasks) == 0 {
		t.Fatalf("tasks=%+v err=%v", tasks, err)
	}
	for _, task := range tasks {
		if task.Attempts != 0 || task.State == "exhausted" {
			t.Fatalf("deferred task was charged: %+v", task)
		}
	}
	joining.Store(false)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if projection, err := db.Projection(ctx, folder, "doc.txt"); err == nil && projection.Kind == history.KindFile {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("scan did not run after the join finished")
		}
	}
}
