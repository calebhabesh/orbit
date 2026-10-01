package replication

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
)

type recoveringChunkPeer struct {
	PeerClient
	mu      sync.Mutex
	until   time.Time
	started chan struct{}
}

func (peer *recoveringChunkPeer) Chunk(ctx context.Context, request ChunkRequest, expected history.Chunk) ([]byte, error) {
	peer.mu.Lock()
	if peer.until.IsZero() {
		peer.until = time.Now().Add(250 * time.Millisecond)
		if peer.started != nil {
			close(peer.started)
		}
	}
	blocked := time.Now().Before(peer.until)
	peer.mu.Unlock()
	if blocked {
		return nil, &WireError{Status: http.StatusTooManyRequests, Body: ErrorResponse{Code: "RETRY_EXHAUSTED", Retryable: true}}
	}
	return peer.PeerClient.Chunk(ctx, request, expected)
}

func TestChunkBackpressureCancellationStopsSync(t *testing.T) {
	fix := newSyncFixture(t)
	if err := os.WriteFile(filepath.Join(fix.senderRoot, "cancel.bin"), bytes.Repeat([]byte("X"), int(history.ChunkSize)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fix.senderWork.Scan(fix.ctx, fix.folder); err != nil {
		t.Fatal(err)
	}
	syncer := fix.newSyncer(TransferOptions{})
	peer := &recoveringChunkPeer{PeerClient: fix.client, started: make(chan struct{})}
	syncer.client = peer
	ctx, cancel := context.WithCancel(fix.ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := syncer.Sync(ctx); done <- err }()
	select {
	case <-peer.started:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("chunk fetch did not start")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation not propagated: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("backpressure wait ignored cancellation")
	}
	if _, err := os.Stat(filepath.Join(fix.receiverRoot, "cancel.bin")); !os.IsNotExist(err) {
		t.Fatalf("incomplete contents published: %v", err)
	}
}

func TestParallelChunksWaitForPeerBackpressureRecovery(t *testing.T) {
	fix := newSyncFixture(t)
	var content []byte
	for i := byte(1); i <= 4; i++ {
		content = append(content, bytes.Repeat([]byte{i}, int(history.ChunkSize))...)
	}
	path := filepath.Join(fix.senderRoot, "backpressure.bin")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fix.senderWork.Scan(fix.ctx, fix.folder); err != nil {
		t.Fatal(err)
	}
	syncer := fix.newSyncer(TransferOptions{Workers: 4})
	syncer.client = &recoveringChunkPeer{PeerClient: fix.client}
	result, err := syncer.Sync(fix.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.ChunksFetched != 4 {
		t.Fatalf("incomplete transfer: %+v", result)
	}
	got, err := os.ReadFile(filepath.Join(fix.receiverRoot, "backpressure.bin"))
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("published contents differ: %v", err)
	}
}
