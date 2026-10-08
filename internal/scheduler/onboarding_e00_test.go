package scheduler_test

import (
	"context"
	"os"
	"path/filepath"
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

// E00 F02 (2026-10-08 trial): asserts approved behavior and deliberately fails
// until E02; opt in with ORBIT_ONBOARDING_BASELINE=1.
//
// A scan exhausted because the root was briefly unavailable must stop being
// attention once a later scan of the same folder completes.
func TestOnboardingE00F02RootUnavailableScanHealsAfterLaterScan(t *testing.T) {
	if os.Getenv("ORBIT_ONBOARDING_BASELINE") != "1" {
		t.Skip("deliberately failing E00 baseline; opt in with ORBIT_ONBOARDING_BASELINE=1")
	}
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
	defer db.Close()
	var folder history.ID
	folder[0] = 0xe0
	local := history.ID{1}
	if err = db.EnsureFolder(ctx, folder, local, 1); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ApproveMembership(ctx, protocol.Membership{Folder: folder, Revision: 1, Active: []protocol.ActiveMember{{Device: local, KeyPin: history.Digest{1}}}}); err != nil {
		t.Fatal(err)
	}
	ws := workspace.New(db, workspace.Options{})
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
	failed, err := s.Submit(repository.DurableTask{Folder: folder, Kind: "scan"})
	if err != nil {
		t.Fatal(err)
	}
	if task := waitState(failed, "exhausted"); task.ErrorCode != "ROOT_UNAVAILABLE" {
		t.Fatalf("fault task error %q", task.ErrorCode)
	}

	// The root returns; the folder resumes (as after the trial daemon restart)
	// and a later scan of the same folder completes.
	if err = os.WriteFile(marker, saved, 0o600); err != nil {
		t.Fatal(err)
	}
	s.ResumeFolder(folder)
	later, err := s.Submit(repository.DurableTask{Folder: folder, Kind: "scan"})
	if err != nil {
		t.Fatal(err)
	}
	waitState(later, "completed")

	task, err := db.GetDurableTask(ctx, failed)
	if err != nil {
		t.Fatal(err)
	}
	if task.State == "exhausted" {
		t.Errorf("superseded ROOT_UNAVAILABLE scan still exhausted after a later scan completed")
	}
	ctrl := control.New(db, ws, control.Options{LocalDevice: local})
	r, err := ctrl.TerminalQuery(ctx, tc.Query{Version: tc.Version, Kind: "attention", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range r.Attention {
		if a.Code == "EXHAUSTED_WORK" {
			t.Errorf("attention still lists EXHAUSTED_WORK; advice %q", a.Action)
		}
	}
}
