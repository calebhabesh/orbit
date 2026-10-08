package scheduler_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/scheduler"
)

func TestBandwidthLimiterUnlimited(t *testing.T) {
	lim := scheduler.NewBandwidthLimiter(0)
	ctx := context.Background()
	start := time.Now()
	if err := lim.Acquire(ctx, nil, 1024*1024); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("unlimited acquire took too long: %v", elapsed)
	}
}

func TestBandwidthLimiterThrottlingAndCancellation(t *testing.T) {
	// 50 KB/sec limit
	rate := int64(50 * 1024)
	lim := scheduler.NewBandwidthLimiter(rate)
	ctx := context.Background()

	// Initial burst consumes tokens
	if err := lim.Acquire(ctx, nil, int(rate)); err != nil {
		t.Fatal(err)
	}

	// Next acquire of 50 KB should take ~1 second
	start := time.Now()
	if err := lim.Acquire(ctx, nil, int(rate)); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed < 800*time.Millisecond {
		t.Fatalf("expected throttle to take ~1s, took %v", elapsed)
	}

	// Test cancellation while waiting for tokens
	cancelCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := lim.Acquire(cancelCtx, nil, int(rate*10))
	if err == nil {
		t.Fatal("expected context deadline error, got nil")
	}
}

func TestThrottledReaderAndWriter(t *testing.T) {
	lim := scheduler.NewBandwidthLimiter(0) // unlimited
	ctx := context.Background()

	data := []byte("hello throttled world")
	r := scheduler.ThrottledReader(ctx, nil, bytes.NewReader(data), lim)
	readBuf, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(readBuf) != string(data) {
		t.Fatalf("readBuf = %q, want %q", readBuf, data)
	}

	var writeBuf bytes.Buffer
	w := scheduler.ThrottledWriter(ctx, nil, &writeBuf, lim)
	n, err := w.Write(data)
	if err != nil || n != len(data) {
		t.Fatalf("write failed: %v (n=%d)", err, n)
	}
	if writeBuf.String() != string(data) {
		t.Fatalf("writeBuf = %q, want %q", writeBuf.String(), data)
	}
}
