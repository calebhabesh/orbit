package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/scheduler"
)

func TestBandwidthLimiterRequestLargerThanRateMakesProgress(t *testing.T) {
	for _, perPeer := range []bool{false, true} {
		limiter := scheduler.NewBandwidthLimiter(256 * 1024)
		peer := history.ID{1}
		if perPeer {
			limiter = scheduler.NewBandwidthLimiter(0)
			limiter.SetPeerRate(peer, 256*1024)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		start := time.Now()
		err := limiter.Acquire(ctx, &peer, 1024*1024)
		cancel()
		if err != nil {
			t.Fatalf("chunk larger than per-second rate never progressed: %v", err)
		}
		if time.Since(start) < 2*time.Second {
			t.Fatal("large request bypassed rate budget")
		}
	}
}
