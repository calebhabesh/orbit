package scheduler_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/app"
	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/scheduler"
	"github.com/calebhabesh/orbit/internal/workspace"
)

// e02Folder is the F02 fixture: a registered folder under a running
// scheduler whose root marker is removed so one scan exhausts with
// ROOT_UNAVAILABLE, then restored; heal then makes the exhausted scan
// complete (or be superseded), which the fixture waits for.
func e02Folder(t *testing.T, heal func(ctx context.Context, db *repository.DB, s *scheduler.Scheduler, ws *workspace.Workspace, folder, local history.ID, failed string)) (string, *repository.DB, *workspace.Workspace, history.ID) {
	var failed string
	var ws *workspace.Workspace
	var local history.ID
	ctx := context.Background()
	stateDir, rootDir := t.TempDir(), t.TempDir()
	// Control attention reads the device configuration, as in the daemon.
	if _, err := app.Initialize(ctx, stateDir, app.SystemDependencies()); err != nil {
		t.Fatal(err)
	}
	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var folder history.ID
	folder[0] = 0xe0
	local = history.ID{1}
	if err = db.EnsureFolder(ctx, folder, local, 1); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ApproveMembership(ctx, protocol.Membership{Folder: folder, Revision: 1, Active: []protocol.ActiveMember{{Device: local, KeyPin: history.Digest{1}}}}); err != nil {
		t.Fatal(err)
	}
	ws = workspace.New(db, workspace.Options{})
	if _, err = ws.Register(ctx, folder, rootDir); err != nil {
		t.Fatal(err)
	}
	s, err := scheduler.NewScheduler(db, ws, scheduler.SchedulerOptions{Profile: scheduler.GetProfile(scheduler.ProfilePi), NoWatch: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	waitState := func(id, want string) repository.DurableTask {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			task, err := db.GetDurableTask(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if task.State == want {
				return task
			}
			if time.Now().After(deadline) {
				t.Fatalf("task %s state %q, want %q", id, task.State, want)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	startup, err := db.ListDurableTasks(ctx, repository.TaskFilter{Folder: folder, Kind: "scan", Limit: 1})
	if err != nil || len(startup) == 0 {
		t.Fatal("startup scan missing", err)
	}
	waitState(startup[0].ID, "completed")

	// The root is briefly unavailable (marker missing), as in the trial.
	marker := filepath.Join(rootDir, ".orbit-internal", "registration")
	saved, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	failed, err = s.Submit(repository.DurableTask{Folder: folder, Kind: "scan"})
	if err != nil {
		t.Fatal(err)
	}
	if task := waitState(failed, "exhausted"); task.ErrorCode != "ROOT_UNAVAILABLE" {
		t.Fatalf("fault task error %q", task.ErrorCode)
	}
	// The root returns and the folder resumes (as after the trial restart).
	if err = os.WriteFile(marker, saved, 0o600); err != nil {
		t.Fatal(err)
	}
	s.ResumeFolder(folder)
	heal(ctx, db, s, ws, folder, local, failed)
	waitState(failed, "completed")
	return failed, db, ws, local
}

// F02: a scan exhausted on a briefly missing root is superseded when a later
// scan of the folder completes; attention clears and history keeps why.
func TestOnboardingE02F02RootUnavailableScanHealsAfterLaterScan(t *testing.T) {
	failed, db, ws, local := e02Folder(t, func(ctx context.Context, _ *repository.DB, s *scheduler.Scheduler, _ *workspace.Workspace, folder, _ history.ID, _ string) {
		if _, err := s.Submit(repository.DurableTask{Folder: folder, Kind: "scan"}); err != nil {
			t.Fatal(err)
		}
	})
	ctx := context.Background()
	task, err := db.GetDurableTask(ctx, failed)
	if err != nil {
		t.Fatal(err)
	}
	if task.ErrorCode != repository.SupersededCode || !strings.Contains(task.LastError, "superseded by completed scan") || !strings.Contains(task.LastError, "ROOT_UNAVAILABLE") {
		t.Fatalf("history does not record why: code %q error %q", task.ErrorCode, task.LastError)
	}
	ctrl := control.New(db, ws, control.Options{LocalDevice: local})
	r, err := ctrl.TerminalQuery(ctx, tc.Query{Version: tc.Version, Kind: "attention", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range r.Attention {
		if a.Code == "EXHAUSTED_WORK" {
			t.Fatalf("attention still lists EXHAUSTED_WORK: %q", a.Action)
		}
	}
}

// F03: a control retry while the scheduler runs re-queues the exhausted task
// and the running scheduler picks it up (WorkChanged), with no restart.
func TestOnboardingE02F03ControlRetryReachesRunningScheduler(t *testing.T) {
	e02Folder(t, func(ctx context.Context, db *repository.DB, s *scheduler.Scheduler, ws *workspace.Workspace, _, local history.ID, failed string) {
		ctrl := control.New(db, ws, control.Options{LocalDevice: local, WorkChanged: s.ReloadWork})
		if _, err := ctrl.WorkRetry(ctx, control.WorkRetryRequest{TaskID: failed}); err != nil {
			t.Fatal(err)
		}
	})
}
