package scheduler_test

import (
	"context"
	"errors"
	"io"
	"syscall"
	"testing"

	"github.com/calebhabesh/file-sync/internal/scheduler"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

func TestRetryClassification(t *testing.T) {
	c := scheduler.RetryClassifier{}

	if !c.IsTransient(workspace.ErrUnstableFile) {
		t.Fatal("expected ErrUnstableFile to be transient")
	}
	if !c.IsTransient(syscall.ECONNREFUSED) {
		t.Fatal("expected ECONNREFUSED to be transient")
	}
	if !c.IsTransient(io.ErrUnexpectedEOF) {
		t.Fatal("expected io.ErrUnexpectedEOF to be transient")
	}

	if c.IsTransient(workspace.ErrRootUnavailable) {
		t.Fatal("expected ErrRootUnavailable to be permanent")
	}
	if c.IsTransient(workspace.ErrStructuralConflict) {
		t.Fatal("expected ErrStructuralConflict to be permanent")
	}
	if c.IsTransient(context.Canceled) {
		t.Fatal("expected context.Canceled to be non-transient")
	}

	if c.ErrorCode(workspace.ErrRootUnavailable) != "ROOT_UNAVAILABLE" {
		t.Fatalf("expected ROOT_UNAVAILABLE, got %s", c.ErrorCode(workspace.ErrRootUnavailable))
	}
	if c.ErrorCode(workspace.ErrUnstableFile) != "UNSTABLE_FILE" {
		t.Fatalf("expected UNSTABLE_FILE, got %s", c.ErrorCode(workspace.ErrUnstableFile))
	}
	if c.ErrorCode(errors.New("some io error")) != "IO_ERROR" {
		t.Fatalf("expected IO_ERROR, got %s", c.ErrorCode(errors.New("some io error")))
	}
}

func TestRetryBackoffCapping(t *testing.T) {
	c := scheduler.RetryClassifier{}

	b1 := c.Backoff(1)
	if b1 < scheduler.BaseRetryDelay {
		t.Fatalf("backoff 1 = %v, want at least %v", b1, scheduler.BaseRetryDelay)
	}

	b5 := c.Backoff(5)
	if b5 <= b1 {
		t.Fatalf("expected backoff 5 (%v) > backoff 1 (%v)", b5, b1)
	}

	b10 := c.Backoff(10)
	if b10 > scheduler.MaxRetryDelay*2 {
		t.Fatalf("backoff exceeded cap: %v", b10)
	}
}
