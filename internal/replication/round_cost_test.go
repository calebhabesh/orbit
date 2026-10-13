package replication

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
)

// countingClient counts peer requests and adds a fixed delay to each one to
// model a WAN round trip.
type countingClient struct {
	PeerClient
	delay    time.Duration
	receipts atomic.Int64
	status   atomic.Int64
	total    atomic.Int64
}

func (c *countingClient) wait() {
	c.total.Add(1)
	time.Sleep(c.delay)
}

func (c *countingClient) Hello(ctx context.Context, r HelloRequest) (HelloResponse, error) {
	c.wait()
	return c.PeerClient.Hello(ctx, r)
}

func (c *countingClient) Inventory(ctx context.Context, r InventoryRequest) (InventoryResponse, error) {
	c.wait()
	return c.PeerClient.Inventory(ctx, r)
}

func (c *countingClient) Versions(ctx context.Context, r VersionsRequest) (VersionsResponse, error) {
	c.wait()
	return c.PeerClient.Versions(ctx, r)
}

func (c *countingClient) Chunk(ctx context.Context, r ChunkRequest, want history.Chunk) ([]byte, error) {
	c.wait()
	return c.PeerClient.Chunk(ctx, r, want)
}

func (c *countingClient) Receipts(ctx context.Context, r ReceiptsRequest) (ReceiptsResponse, error) {
	c.receipts.Add(1)
	c.wait()
	return c.PeerClient.Receipts(ctx, r)
}

func (c *countingClient) Status(ctx context.Context, r StatusRequest) (StatusResponse, error) {
	c.status.Add(1)
	c.wait()
	return c.PeerClient.Status(ctx, r)
}

// An idle round re-asserts every durable receipt in batches, not one request
// per version (an idle round once cost O(versions) round trips).
func TestSyncerBatchesReceiptReassertion(t *testing.T) {
	fix := newSyncFixture(t)
	ctx := context.Background()
	const versions = 300
	for i := 0; i < versions; i++ {
		if err := os.WriteFile(filepath.Join(fix.senderRoot, fmt.Sprintf("f%04d", i)), []byte(fmt.Sprintf("small file %d\n", i)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fix.senderWork.Scan(ctx, fix.folder); err != nil {
		t.Fatal(err)
	}
	if _, err := fix.newSyncer(TransferOptions{}).Sync(ctx); err != nil {
		t.Fatal(err)
	}
	client := &countingClient{PeerClient: fix.client}
	syncer := NewSyncer(fix.receiverRepo, fix.receiverWork, client, fix.receiverID.DeviceID, fix.senderID.DeviceID, fix.folder, fix.approved, TransferOptions{})
	result, err := syncer.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.ReceiptsSent != versions || result.VersionsStored != 0 {
		t.Fatalf("idle round result = %+v", result)
	}
	if got, want := client.receipts.Load(), int64((versions+MaxVersionBatch-1)/MaxVersionBatch); got != want {
		t.Fatalf("receipt requests = %d, want %d", got, want)
	}
	ids, err := fix.senderRepo.PeerProgress(ctx, fix.folder)
	if err != nil {
		t.Fatal(err)
	}
	receipts := 0
	for _, p := range ids {
		if p.Receipt && p.Peer == fix.receiverID.DeviceID {
			receipts++
		}
	}
	if receipts != versions {
		t.Fatalf("sender recorded %d receipts, want %d", receipts, versions)
	}
}

// TestSyncRoundCost measures an idle sync round (nothing new to fetch) as the
// folder grows. Opt-in: ORBIT_ROUND_COST=1, ORBIT_ROUND_COST_DELAY_MS sets
// the per-request delay (default 20).
func TestSyncRoundCost(t *testing.T) {
	if os.Getenv("ORBIT_ROUND_COST") != "1" {
		t.Skip("set ORBIT_ROUND_COST=1")
	}
	delay := 20 * time.Millisecond
	if v := os.Getenv("ORBIT_ROUND_COST_DELAY_MS"); v != "" {
		ms, err := strconv.Atoi(v)
		if err != nil {
			t.Fatal(err)
		}
		delay = time.Duration(ms) * time.Millisecond
	}
	fix := newSyncFixture(t)
	ctx := context.Background()
	written := 0
	for _, size := range []int{300, 600, 900} {
		for ; written < size; written++ {
			if err := os.WriteFile(filepath.Join(fix.senderRoot, fmt.Sprintf("f%04d", written)), []byte(fmt.Sprintf("small file %d\n", written)), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := fix.senderWork.Scan(ctx, fix.folder); err != nil {
			t.Fatal(err)
		}
		if _, err := fix.newSyncer(TransferOptions{}).Sync(ctx); err != nil {
			t.Fatal(err)
		}
		client := &countingClient{PeerClient: fix.client, delay: delay}
		syncer := NewSyncer(fix.receiverRepo, fix.receiverWork, client, fix.receiverID.DeviceID, fix.senderID.DeviceID, fix.folder, fix.approved, TransferOptions{})
		start := time.Now()
		result, err := syncer.Sync(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("versions=%d idle_round=%s requests=%d receipts=%d status=%d fetched=%d",
			size, time.Since(start).Round(time.Millisecond), client.total.Load(), client.receipts.Load(), client.status.Load(), result.VersionsStored)
	}
}
