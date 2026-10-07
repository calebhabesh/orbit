package terminal_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

// A setup step that grows with the folder (here capture) must not be cut short
// by the bound on steps holding terminalMu. Before, every resume restarted the
// capture and a folder slower to hash than the bound never became ready.
// Queries keep answering while it runs.
func TestTerminalSetupLongStepOutlivesStepBoundAndReleasesQueries(t *testing.T) {
	f := fresh(t)
	root := filepath.Join(f.root, "slow")
	os.Mkdir(root, 0700)
	for _, name := range []string{"a", "b", "c"} {
		os.WriteFile(filepath.Join(root, name), []byte("bytes "+name), 0600)
	}
	p, _ := setupReview(t, f, root, "adopt")
	const bound = 100 * time.Millisecond
	var capturing atomic.Bool
	ws := workspace.New(f.db, workspace.Options{FaultHook: func(name string) error {
		if name == workspace.HookCaptureRead {
			capturing.Store(true)
			time.Sleep(3 * bound)
		}
		return nil
	}})
	interrupted := false
	ctrl := control.New(f.db, ws, control.Options{LocalDevice: f.device, SetupStepBound: bound, FaultHook: func(name string) error {
		if name == "terminal.setup.reviewed" && !interrupted {
			interrupted = true
			return errors.New("interrupt before the worker resumes")
		}
		return nil
	}})
	op := enrollmentRandom(t)
	if _, err := ctrl.TerminalMutate(context.Background(), tc.Mutation{Version: "1", Kind: "adopt", OperationID: op, Setup: &p}); err == nil {
		t.Fatal("expected interruption")
	}
	resumed := make(chan error, 1)
	go func() { resumed <- ctrl.ResumeSetupJobs(context.Background()) }()
	for deadline := time.Now().Add(10 * time.Second); !capturing.Load(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("capture never started")
		}
	}
	started := time.Now()
	r, err := ctrl.TerminalQuery(context.Background(), tc.Query{Version: "1", Kind: "operation", ID: op})
	if err != nil || r.State != "running" || time.Since(started) > bound {
		t.Fatalf("query during capture: state=%s err=%v waited=%v", r.State, err, time.Since(started))
	}
	if err := <-resumed; err != nil {
		t.Fatal(err)
	}
	r, err = ctrl.TerminalQuery(context.Background(), tc.Query{Version: "1", Kind: "operation", ID: op})
	if err != nil || r.State != "completed" || !r.Readiness.Ready() {
		code := ""
		if r.Error != nil {
			code = r.Error.Code + ": " + r.Error.Message
		}
		t.Fatalf("one resume did not complete setup: state=%s phase=%s err=%v %s", r.State, r.Operation.Phase, err, code)
	}
}
