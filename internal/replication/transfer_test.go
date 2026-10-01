package replication

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

type syncFixture struct {
	t            *testing.T
	ctx          context.Context
	cancel       context.CancelFunc
	senderID     Identity
	receiverID   Identity
	senderRepo   *repository.DB
	receiverRepo *repository.DB
	senderWork   *workspace.Workspace
	receiverWork *workspace.Workspace
	senderRoot   string
	receiverRoot string
	folder       history.ID
	membership   protocol.Membership
	server       *Server
	client       *Client
	listener     net.Listener
	approved     repository.ApprovedMembership
}

func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	senderDevice, receiverDevice := fixedID('S'), fixedID('R')
	folder := fixedID('F')

	senderDir := t.TempDir()
	receiverDir := t.TempDir()

	senderID, err := LoadOrCreateIdentity(filepath.Join(senderDir, "identity"), senderDevice, now)
	if err != nil {
		t.Fatal(err)
	}
	receiverID, err := LoadOrCreateIdentity(filepath.Join(receiverDir, "identity"), receiverDevice, now)
	if err != nil {
		t.Fatal(err)
	}

	senderRepo, err := repository.Open(context.Background(), filepath.Join(senderDir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	receiverRepo, err := repository.Open(context.Background(), filepath.Join(receiverDir, "state"))
	if err != nil {
		t.Fatal(err)
	}

	senderRoot := filepath.Join(senderDir, "root")
	receiverRoot := filepath.Join(receiverDir, "root")
	if err := os.Mkdir(senderRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(receiverRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := senderRepo.EnsureFolder(context.Background(), folder, senderDevice, 1); err != nil {
		t.Fatal(err)
	}
	if err := receiverRepo.EnsureFolder(context.Background(), folder, receiverDevice, 1); err != nil {
		t.Fatal(err)
	}

	senderWork := workspace.New(senderRepo, workspace.Options{})
	if _, err := senderWork.Register(context.Background(), folder, senderRoot); err != nil {
		t.Fatal(err)
	}
	receiverWork := workspace.New(receiverRepo, workspace.Options{})
	if _, err := receiverWork.Register(context.Background(), folder, receiverRoot); err != nil {
		t.Fatal(err)
	}

	membership := protocol.Membership{
		Folder:   folder,
		Revision: 1,
		Active: []protocol.ActiveMember{
			{Device: senderDevice, KeyPin: senderID.KeyPin},
			{Device: receiverDevice, KeyPin: receiverID.KeyPin},
		},
	}
	if _, err := senderRepo.ApproveMembership(context.Background(), membership); err != nil {
		t.Fatal(err)
	}
	approved, err := receiverRepo.ApproveMembership(context.Background(), membership)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	server := NewServer(senderRepo, senderID)
	server.now = func() time.Time { return now }
	go func() { _ = server.Serve(ctx, listener) }()

	client, err := NewClient("https://"+listener.Addr().String(), receiverID, senderID.Leaf, senderID.KeyPin)
	if err != nil {
		t.Fatal(err)
	}

	fix := &syncFixture{
		t:            t,
		ctx:          ctx,
		cancel:       cancel,
		senderID:     senderID,
		receiverID:   receiverID,
		senderRepo:   senderRepo,
		receiverRepo: receiverRepo,
		senderWork:   senderWork,
		receiverWork: receiverWork,
		senderRoot:   senderRoot,
		receiverRoot: receiverRoot,
		folder:       folder,
		membership:   membership,
		server:       server,
		client:       client,
		listener:     listener,
		approved:     approved,
	}
	t.Cleanup(func() {
		client.CloseIdleConnections()
		cancel()
		_ = listener.Close()
		_ = senderRepo.Close()
		_ = receiverRepo.Close()
	})
	return fix
}

func (fix *syncFixture) newSyncer(options TransferOptions) *Syncer {
	return NewSyncer(
		fix.receiverRepo,
		fix.receiverWork,
		fix.client,
		fix.receiverID.DeviceID,
		fix.senderID.DeviceID,
		fix.folder,
		fix.approved,
		options,
	)
}

func TestSyncerEmptyFile(t *testing.T) {
	fix := newSyncFixture(t)
	emptyPath := filepath.Join(fix.senderRoot, "empty.txt")
	if err := os.WriteFile(emptyPath, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	scanRes, err := fix.senderWork.Scan(context.Background(), fix.folder)
	if err != nil || len(scanRes.Captured) != 1 {
		t.Fatalf("scan = %+v err = %v", scanRes, err)
	}

	syncer := fix.newSyncer(TransferOptions{})
	res, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync error = %v", err)
	}
	if res.Inventoried != 1 || res.MetadataAdded != 1 || res.ChunksFetched != 0 || res.VersionsStored != 1 || res.ReceiptsSent != 1 || res.VersionsApplied != 1 {
		t.Fatalf("unexpected sync result = %+v", res)
	}

	// Verify empty file arrives byte-for-byte in receiver root
	receivedPath := filepath.Join(fix.receiverRoot, "empty.txt")
	info, err := os.Stat(receivedPath)
	if err != nil {
		t.Fatalf("stat received file: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("expected 0-byte file, got size = %d", info.Size())
	}

	// Verify sender recorded the durable receipt from receiver
	peers, err := fix.senderRepo.PeerProgress(context.Background(), fix.folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 || peers[0].Peer != fix.receiverID.DeviceID || !peers[0].Receipt {
		t.Fatalf("sender peer progress = %+v", peers)
	}
}

func TestSyncerSmallAndMultiChunkWithRepeatedChunks(t *testing.T) {
	fix := newSyncFixture(t)

	// Small file
	smallContent := []byte("hello verified small file")
	if err := os.WriteFile(filepath.Join(fix.senderRoot, "small.txt"), smallContent, 0o600); err != nil {
		t.Fatal(err)
	}

	// Multi-chunk file with repeated chunks (2 identical chunks of 1 MiB)
	chunkBlock := bytes.Repeat([]byte("K"), int(history.ChunkSize))
	repeatedContent := make([]byte, 2*len(chunkBlock))
	copy(repeatedContent[:len(chunkBlock)], chunkBlock)
	copy(repeatedContent[len(chunkBlock):], chunkBlock)
	if err := os.WriteFile(filepath.Join(fix.senderRoot, "repeated.bin"), repeatedContent, 0o600); err != nil {
		t.Fatal(err)
	}

	scanRes, err := fix.senderWork.Scan(context.Background(), fix.folder)
	if err != nil || len(scanRes.Captured) != 2 {
		t.Fatalf("scan = %+v err = %v", scanRes, err)
	}

	syncer := fix.newSyncer(TransferOptions{})
	res, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync error = %v", err)
	}

	// For small.txt: 1 chunk fetched.
	// For repeated.bin: 1 unique chunk fetched, 1 position reused.
	// Total: 2 chunks fetched, 1 reused, 2 versions stored, 2 applied.
	if res.ChunksFetched != 2 || res.ChunksReused != 1 || res.VersionsStored != 2 || res.VersionsApplied != 2 {
		t.Fatalf("unexpected sync result = %+v", res)
	}

	// Verify byte-for-byte arrival
	gotSmall, err := os.ReadFile(filepath.Join(fix.receiverRoot, "small.txt"))
	if err != nil || !bytes.Equal(gotSmall, smallContent) {
		t.Fatalf("small content mismatch: %v", err)
	}
	gotRepeated, err := os.ReadFile(filepath.Join(fix.receiverRoot, "repeated.bin"))
	if err != nil || !bytes.Equal(gotRepeated, repeatedContent) {
		t.Fatalf("repeated content mismatch: len=%d err=%v", len(gotRepeated), err)
	}

	// Verify sender received receipts for both versions
	peers, err := fix.senderRepo.PeerProgress(context.Background(), fix.folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 2 {
		t.Fatalf("sender peer progress entries count = %d, want 2", len(peers))
	}
	for _, p := range peers {
		if !p.Receipt {
			t.Fatalf("expected receipt for peer %x version %v, got false", p.Peer, p.Version)
		}
	}
}

type corruptChunkClient struct {
	PeerClient
}

func (c *corruptChunkClient) Chunk(ctx context.Context, request ChunkRequest, expected history.Chunk) ([]byte, error) {
	data, err := c.PeerClient.Chunk(ctx, request, expected)
	if err != nil {
		return nil, err
	}
	// Corrupt first byte
	corrupted := make([]byte, len(data))
	copy(corrupted, data)
	if len(corrupted) > 0 {
		corrupted[0] ^= 0xff
	}
	if sha256.Sum256(corrupted) != expected.Digest {
		return nil, errors.New("peer chunk payload digest mismatch")
	}
	return corrupted, nil
}

func TestSyncerCorruptionRejected(t *testing.T) {
	fix := newSyncFixture(t)
	content := []byte("content to be corrupted")
	if err := os.WriteFile(filepath.Join(fix.senderRoot, "corrupt.txt"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fix.senderWork.Scan(context.Background(), fix.folder); err != nil {
		t.Fatal(err)
	}

	corruptClient := &corruptChunkClient{PeerClient: fix.client}
	syncer := NewSyncer(
		fix.receiverRepo,
		fix.receiverWork,
		corruptClient,
		fix.receiverID.DeviceID,
		fix.senderID.DeviceID,
		fix.folder,
		fix.approved,
		TransferOptions{Retries: 2},
	)

	_, err := syncer.Sync(context.Background())
	if err == nil {
		t.Fatal("expected sync to fail with corrupted chunks")
	}

	// Receiver must not have applied the file
	if _, err := os.Stat(filepath.Join(fix.receiverRoot, "corrupt.txt")); !os.IsNotExist(err) {
		t.Fatalf("corrupted file should not exist in workspace, stat err = %v", err)
	}

	// Sender must not have received receipt
	peers, err := fix.senderRepo.PeerProgress(context.Background(), fix.folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) > 0 && peers[0].Receipt {
		t.Fatalf("receipt should not be recorded for corrupted transfer: %+v", peers)
	}
}

func TestSyncerInterruptedResume(t *testing.T) {
	fix := newSyncFixture(t)

	// Create 2-chunk file with distinct chunks
	chunk1 := bytes.Repeat([]byte("A"), int(history.ChunkSize))
	chunk2 := bytes.Repeat([]byte("B"), int(history.ChunkSize))
	content := append(chunk1, chunk2...)
	if err := os.WriteFile(filepath.Join(fix.senderRoot, "resume.bin"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fix.senderWork.Scan(context.Background(), fix.folder); err != nil {
		t.Fatal(err)
	}

	// First sync interrupted after first chunk is verified
	verifiedCount := 0
	faultHook := func(name string) error {
		if name == HookChunkVerified {
			verifiedCount++
			if verifiedCount == 1 {
				return errors.New("injected interruption after chunk 1")
			}
		}
		return nil
	}

	syncer1 := fix.newSyncer(TransferOptions{Workers: 1, Hook: faultHook})
	_, err := syncer1.Sync(context.Background())
	if err == nil {
		t.Fatal("expected sync1 to fail with injected interruption")
	}

	// Verify chunk 0 was installed and marked verified in the transfer record
	c0Chunk := history.Chunk{Digest: sha256.Sum256(chunk1), Length: uint64(len(chunk1))}
	avail, err := fix.receiverRepo.VerifiedChunk(context.Background(), c0Chunk)
	if err != nil || !avail {
		t.Fatalf("chunk 0 should be available after partial sync: avail=%t err=%v", avail, err)
	}

	// Second sync without fault hook resumes and reuses verified chunk
	syncer2 := fix.newSyncer(TransferOptions{Workers: 1})
	res2, err := syncer2.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync2 error = %v", err)
	}
	if res2.ChunksReused != 1 {
		t.Fatalf("expected 1 chunk reused, got %d", res2.ChunksReused)
	}
	if res2.ChunksFetched != 1 {
		t.Fatalf("expected 1 chunk fetched, got %d", res2.ChunksFetched)
	}
	if res2.VersionsApplied != 1 {
		t.Fatalf("expected 1 version applied, got %d", res2.VersionsApplied)
	}

	// Verify final file arrives byte-for-byte
	got, err := os.ReadFile(filepath.Join(fix.receiverRoot, "resume.bin"))
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("resumed content mismatch: len=%d err=%v", len(got), err)
	}
}

func TestSyncerLostReceiptReplaySafe(t *testing.T) {
	fix := newSyncFixture(t)
	content := []byte("receipt replay test")
	if err := os.WriteFile(filepath.Join(fix.senderRoot, "receipt.txt"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fix.senderWork.Scan(context.Background(), fix.folder); err != nil {
		t.Fatal(err)
	}

	// First sync: fail right before sending receipt
	hook := func(name string) error {
		if name == HookBeforeReceipt {
			return errors.New("injected network loss before receipt send")
		}
		return nil
	}
	syncer1 := fix.newSyncer(TransferOptions{Hook: hook})
	_, err := syncer1.Sync(context.Background())
	if err == nil {
		t.Fatal("expected sync to fail before receipt")
	}

	// Check: content is ready on receiver, but sender has not recorded receipt
	ids, err := fix.receiverRepo.VersionIDs(context.Background(), fix.folder)
	if err != nil || len(ids) != 1 {
		t.Fatalf("version IDs on receiver = %+v", ids)
	}
	ready, err := fix.receiverRepo.ContentReady(context.Background(), ids[0])
	if err != nil || !ready {
		t.Fatalf("content should be ready on receiver: ready=%t err=%v", ready, err)
	}
	peers, err := fix.senderRepo.PeerProgress(context.Background(), fix.folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) > 0 && peers[0].Receipt {
		t.Fatalf("sender should not have receipt yet: %+v", peers)
	}

	// Second sync: resumes, sends receipt, and applies version
	syncer2 := fix.newSyncer(TransferOptions{})
	res2, err := syncer2.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync2 error = %v", err)
	}
	if res2.ChunksFetched != 0 {
		t.Fatalf("expected 0 chunks fetched on receipt replay, got %d", res2.ChunksFetched)
	}
	if res2.ReceiptsSent != 1 {
		t.Fatalf("expected 1 receipt sent, got %d", res2.ReceiptsSent)
	}
	if res2.VersionsApplied != 1 {
		t.Fatalf("expected 1 version applied, got %d", res2.VersionsApplied)
	}

	// Sender now has confirmed receipt
	peers, err = fix.senderRepo.PeerProgress(context.Background(), fix.folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 || !peers[0].Receipt {
		t.Fatalf("sender should now have receipt: %+v", peers)
	}

	// Third sync: replay is safe and idempotent
	res3, err := syncer2.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync3 replay error = %v", err)
	}
	if res3.ChunksFetched != 0 || res3.VersionsStored != 0 || res3.VersionsApplied != 0 {
		t.Fatalf("unexpected sync3 result = %+v", res3)
	}
}

func TestSyncerStatusDifferentiatesStoredFromApplied(t *testing.T) {
	fix := newSyncFixture(t)
	content := []byte("stored vs applied")
	if err := os.WriteFile(filepath.Join(fix.senderRoot, "status.txt"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fix.senderWork.Scan(context.Background(), fix.folder); err != nil {
		t.Fatal(err)
	}

	// Sync with publisher = nil (stores content without applying to workspace)
	syncerNoPub := NewSyncer(
		fix.receiverRepo,
		nil, // no publisher
		fix.client,
		fix.receiverID.DeviceID,
		fix.senderID.DeviceID,
		fix.folder,
		fix.approved,
		TransferOptions{},
	)
	res, err := syncerNoPub.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync error = %v", err)
	}
	if res.VersionsStored != 1 || res.VersionsApplied != 0 {
		t.Fatalf("expected stored=1 applied=0, got %+v", res)
	}

	// Receiver file status: Stored=true, Applied=false
	ids, err := fix.receiverRepo.VersionIDs(context.Background(), fix.folder)
	if err != nil || len(ids) != 1 {
		t.Fatalf("version IDs = %+v", ids)
	}
	status, err := fix.receiverRepo.VersionStatus(context.Background(), ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if !status.Stored || status.Applied || status.ContentState != "ready" {
		t.Fatalf("expected stored=true applied=false, got %+v", status)
	}

	// File not yet present in receiver's workspace
	if _, err := os.Stat(filepath.Join(fix.receiverRoot, "status.txt")); !os.IsNotExist(err) {
		t.Fatalf("file should not exist in workspace before apply")
	}

	// Now apply the single head through workspace
	if err := fix.receiverWork.Apply(context.Background(), ids[0]); err != nil {
		t.Fatalf("apply error = %v", err)
	}
	statusAfter, err := fix.receiverRepo.VersionStatus(context.Background(), ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if !statusAfter.Stored || !statusAfter.Applied {
		t.Fatalf("expected stored=true applied=true after apply, got %+v", statusAfter)
	}
	got, err := os.ReadFile(filepath.Join(fix.receiverRoot, "status.txt"))
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("content mismatch after apply: %v", err)
	}
}

func TestSyncerSenderDisappearsAfterStoreBeforeApply(t *testing.T) {
	fix := newSyncFixture(t)
	content := []byte("sender disappears test content")
	if err := os.WriteFile(filepath.Join(fix.senderRoot, "disappear.txt"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fix.senderWork.Scan(context.Background(), fix.folder); err != nil {
		t.Fatal(err)
	}

	// Receiver syncs with publisher = nil: stores chunks & metadata, marks content ready
	syncer := NewSyncer(
		fix.receiverRepo,
		nil,
		fix.client,
		fix.receiverID.DeviceID,
		fix.senderID.DeviceID,
		fix.folder,
		fix.approved,
		TransferOptions{},
	)
	if _, err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("sync error = %v", err)
	}

	// Sender is terminated completely (shutdown listener, cancel context)
	fix.client.CloseIdleConnections()
	fix.cancel()
	_ = fix.listener.Close()

	// Verify receiver can independently apply the file from local repository
	ids, err := fix.receiverRepo.VersionIDs(context.Background(), fix.folder)
	if err != nil || len(ids) != 1 {
		t.Fatalf("version IDs = %+v", ids)
	}
	if err := fix.receiverWork.Apply(context.Background(), ids[0]); err != nil {
		t.Fatalf("independent apply failed after sender disappearance: %v", err)
	}

	// Verify file is correctly written byte-for-byte in receiver root
	appliedData, err := os.ReadFile(filepath.Join(fix.receiverRoot, "disappear.txt"))
	if err != nil || !bytes.Equal(appliedData, content) {
		t.Fatalf("applied content mismatch: %v", err)
	}
}

type failingChunkClient struct {
	PeerClient
	failingChunk history.Digest
}

func (c *failingChunkClient) Chunk(ctx context.Context, req ChunkRequest, chunk history.Chunk) ([]byte, error) {
	if chunk.Digest == c.failingChunk {
		return nil, &WireError{Status: 410, Body: ErrorResponse{Code: "CONTENT_EXPIRED", Message: "content expired on primary peer", Retryable: false}}
	}
	return c.PeerClient.Chunk(ctx, req, chunk)
}

func TestSyncerChunkFallbackTransfer(t *testing.T) {
	fix := newSyncFixture(t)
	content := []byte("fallback chunk transfer content")
	if err := os.WriteFile(filepath.Join(fix.senderRoot, "fallback.txt"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fix.senderWork.Scan(context.Background(), fix.folder); err != nil {
		t.Fatal(err)
	}

	chunkDigest := sha256.Sum256(content)
	failingPrimary := &failingChunkClient{
		PeerClient:   fix.client,
		failingChunk: chunkDigest,
	}

	// 1. Without fallbacks, sync should fail
	noFallbackSyncer := NewSyncer(
		fix.receiverRepo,
		fix.receiverWork,
		failingPrimary,
		fix.receiverID.DeviceID,
		fix.senderID.DeviceID,
		fix.folder,
		fix.approved,
		TransferOptions{Retries: 1},
	)
	if _, err := noFallbackSyncer.Sync(context.Background()); err == nil {
		t.Fatal("expected sync failure without fallback peers")
	}

	// 2. With fallback peer (fix.client), chunk is retrieved from fallback
	syncer := NewSyncer(
		fix.receiverRepo,
		fix.receiverWork,
		failingPrimary,
		fix.receiverID.DeviceID,
		fix.senderID.DeviceID,
		fix.folder,
		fix.approved,
		TransferOptions{
			Retries:   1,
			Fallbacks: []PeerClient{fix.client},
		},
	)
	res, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync with fallback failed: %v", err)
	}
	if res.ChunksFetched != 1 {
		t.Fatalf("chunks fetched = %d, want 1", res.ChunksFetched)
	}
	if res.VersionsApplied != 1 {
		t.Fatalf("versions applied = %d, want 1", res.VersionsApplied)
	}

	// Verify file is correctly written
	appliedData, err := os.ReadFile(filepath.Join(fix.receiverRoot, "fallback.txt"))
	if err != nil || !bytes.Equal(appliedData, content) {
		t.Fatalf("applied content mismatch: %v", err)
	}
}

func TestSyncInventoryLargerThanMemoryQueue(t *testing.T) {
	fix := newSyncFixture(t)
	for i := 0; i < MaxQueuedVersions+1; i++ {
		_, err := fix.senderRepo.CreateLocalVersion(fix.ctx, repository.LocalVersionRequest{Folder: fix.folder, Path: fmt.Sprintf("dir-%04d", i), Kind: history.KindDirectory, AuthoredRevision: 1})
		if err != nil {
			t.Fatal(err)
		}
	}
	syncer := fix.newSyncer(TransferOptions{})
	syncer.publisher = nil
	result, err := syncer.Sync(fix.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.MetadataAdded != MaxQueuedVersions+1 || result.ReceiptsSent != MaxQueuedVersions+1 {
		t.Fatalf("incomplete inventory: %+v", result)
	}
}
