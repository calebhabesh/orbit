package scheduler

import (
	"context"
	"errors"
	"github.com/calebhabesh/orbit/internal/network"
	"testing"
	"time"
)

func TestWANW11BandwidthFIFOAndQueuedCancellation(t *testing.T) {
	l := NewBandwidthLimiter(64 * 1024)
	if err := l.Acquire(context.Background(), nil, 64*1024); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results := make(chan string, 3)
	go func() {
		if err := l.Acquire(ctx, nil, 128*1024); err != nil {
			results <- err.Error()
		} else {
			results <- "large"
		}
	}()
	wait := func(n int) {
		t.Helper()
		until := time.Now().Add(time.Second)
		for time.Now().Before(until) {
			l.mu.Lock()
			got := len(l.waiters)
			l.mu.Unlock()
			if got == n {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("queue admission", n)
	}
	wait(1)
	canceled, stop := context.WithCancel(ctx)
	go func() {
		err := l.Acquire(canceled, nil, 1)
		if !errors.Is(err, context.Canceled) {
			results <- "cancel failed"
		} else {
			results <- "canceled"
		}
	}()
	wait(2)
	go func() {
		if err := l.Acquire(ctx, nil, 1); err != nil {
			results <- err.Error()
		} else {
			results <- "tiny"
		}
	}()
	wait(3)
	stop()
	if got := <-results; got != "canceled" {
		t.Fatal(got)
	}
	if got := <-results; got != "large" {
		t.Fatal("tiny reservation overtook queued chunk", got)
	}
	if got := <-results; got != "tiny" {
		t.Fatal(got)
	}
	wait(0)
	if err := l.Acquire(canceled, nil, 1); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestWANW11BandwidthQueueBoundIsRetryable(t *testing.T) {
	l := NewBandwidthLimiter(1)
	// Model an already admitted bounded set without creating 128 blocked workers.
	l.waiters = make([]*bandwidthWaiter, 128)
	if err := l.Acquire(context.Background(), nil, 1); !errors.Is(err, network.ErrBackpressure) || !(RetryClassifier{}).IsTransient(err) {
		t.Fatal(err)
	}
}
