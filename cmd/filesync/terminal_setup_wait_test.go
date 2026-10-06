package main

import (
	"context"
	"errors"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"testing"
	"time"
)

func TestWANW11SetupWaitRetriesReadOnlyTimeoutWithSameOperation(t *testing.T) {
	initial := tc.Result{State: "running", Operation: &tc.Operation{ID: "original", State: "running"}}
	calls := 0
	result, err := waitSetupOperation(initial, "original", time.Now().Add(2*time.Second), func(ctx context.Context, q tc.Query) (tc.Result, error) {
		calls++
		if q.Kind != "operation" || q.ID != "original" {
			t.Fatal("poll replaced operation", q)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("poll has no original wait deadline")
		}
		if calls == 1 {
			return tc.Result{}, context.DeadlineExceeded
		}
		return tc.Result{State: "completed", Operation: &tc.Operation{ID: "original", State: "completed"}}, nil
	})
	if err != nil || result.State != "completed" || calls != 2 || result.Operation.ID != "original" {
		t.Fatal(result, err, calls)
	}
}

func TestWANW11SetupWaitDoesNotExtendDeadlineOrRetryRefusal(t *testing.T) {
	initial := tc.Result{State: "running", Operation: &tc.Operation{ID: "original", State: "running"}}
	start := time.Now()
	calls := 0
	result, err := waitSetupOperation(initial, "original", start.Add(350*time.Millisecond), func(ctx context.Context, _ tc.Query) (tc.Result, error) {
		calls++
		<-ctx.Done()
		return tc.Result{}, ctx.Err()
	})
	if err != nil || result.Operation == nil || result.Operation.ID != "original" || result.State != "running" || calls != 1 || time.Since(start) > time.Second {
		t.Fatal(result, err, calls, time.Since(start))
	}
	refusal := errors.New("control authentication refused")
	calls = 0
	_, err = waitSetupOperation(initial, "original", time.Now().Add(time.Second), func(context.Context, tc.Query) (tc.Result, error) { calls++; return tc.Result{}, refusal })
	if !errors.Is(err, refusal) || calls != 1 {
		t.Fatal(err, calls)
	}
}
