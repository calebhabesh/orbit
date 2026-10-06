package replication

import (
	"context"
	"errors"
	"github.com/calebhabesh/file-sync/internal/history"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

type refusingBudget struct{ calls atomic.Int32 }

func (b *refusingBudget) Acquire(context.Context, *history.ID, int) error {
	b.calls.Add(1)
	return errors.New("marked budget cancellation")
}

type countedChunks struct {
	PeerClient
	calls atomic.Int32
}

func (c *countedChunks) Chunk(ctx context.Context, req ChunkRequest, ch history.Chunk) ([]byte, error) {
	c.calls.Add(1)
	return c.PeerClient.Chunk(ctx, req, ch)
}

func TestWANW11BandwidthReservationPrecedesNetworkIO(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(f.senderRoot, "budget.txt"), []byte("budgeted bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.senderWork.Scan(ctx, f.folder); err != nil {
		t.Fatal(err)
	}
	client := &countedChunks{PeerClient: f.client}
	limiter := &refusingBudget{}
	syncer := f.newSyncer(TransferOptions{Workers: 1, Limiter: limiter})
	syncer.client = client
	if _, err := syncer.Sync(ctx); err == nil {
		t.Fatal("denied budget ignored")
	}
	if limiter.calls.Load() == 0 || client.calls.Load() != 0 {
		t.Fatal("network bytes arrived before bandwidth admission", limiter.calls.Load(), client.calls.Load())
	}
	if _, err := os.Stat(filepath.Join(f.receiverRoot, "budget.txt")); !os.IsNotExist(err) {
		t.Fatal("denied transfer published", err)
	}
}
