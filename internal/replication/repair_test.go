package replication

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
)

func TestRepairCorruptVersionFromAuthorizedPeer(t *testing.T) {
	fix := newSyncFixture(t)
	ctx := context.Background()

	// 1. Author version on sender and sync to receiver
	content := []byte("hello world verified repair payload")
	path := filepath.Join(fix.senderRoot, "repair.txt")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	scanRes, err := fix.senderWork.Scan(ctx, fix.folder)
	if err != nil || len(scanRes.Captured) != 1 {
		t.Fatalf("scan: %+v, %v", scanRes, err)
	}
	vTarget := scanRes.Captured[0]

	syncer := fix.newSyncer(TransferOptions{})
	if _, err := syncer.Sync(ctx); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	availInitial, err := fix.receiverRepo.ContentAvailability(ctx, vTarget.ID)
	if err != nil || availInitial != repository.ContentReady {
		t.Fatalf("expected initial ContentReady, got %v (%v)", availInitial, err)
	}

	// 2. Inject corruption into receiver's chunk object on disk
	digest := vTarget.Manifest.Chunks[0].Digest
	chunkPath := filepath.Join(fix.receiverRepo.StateDir(), "objects", "sha256", fmt.Sprintf("%02x", digest[0]), fmt.Sprintf("%x", digest[1:]))
	if err := os.WriteFile(chunkPath, []byte("injected bit rot on receiver chunk!"), 0o600); err != nil {
		t.Fatalf("corrupt chunk: %v", err)
	}

	// Quarantine the corrupt chunk on receiver
	affected, err := fix.receiverRepo.QuarantineChunk(ctx, digest, "injected bit rot")
	if err != nil {
		t.Fatalf("QuarantineChunk: %v", err)
	}
	if len(affected) != 1 || affected[0] != vTarget.ID {
		t.Fatalf("expected affected version %v, got %v", vTarget.ID, affected)
	}

	avail, err := fix.receiverRepo.ContentAvailability(ctx, vTarget.ID)
	if err != nil || avail != repository.ContentCorrupt {
		t.Fatalf("expected ContentCorrupt, got %v (%v)", avail, err)
	}

	canReceipt, err := fix.receiverRepo.CanIssueDurableReceipt(ctx, vTarget.ID)
	if err != nil || canReceipt {
		t.Fatalf("expected CanIssueDurableReceipt=false for corrupt chunk, got %v (%v)", canReceipt, err)
	}

	// 3. Run Repairer
	repairer := NewRepairer(fix.receiverRepo, fix.receiverID.DeviceID, []RepairPeer{
		{DeviceID: fix.senderID.DeviceID, Client: fix.client},
	})

	report, err := repairer.RepairVersion(ctx, fix.folder, vTarget.ID, nil)
	if err != nil {
		t.Fatalf("RepairVersion failed: %v", err)
	}
	if report.Status != "repaired" || report.RepairedChunks != 1 {
		t.Fatalf("unexpected repair report: %+v", report)
	}

	// 4. Verify version is restored to ready
	availRestored, err := fix.receiverRepo.ContentAvailability(ctx, vTarget.ID)
	if err != nil || availRestored != repository.ContentReady {
		t.Fatalf("expected ContentReady, got %v (%v)", availRestored, err)
	}

	canReceiptRestored, err := fix.receiverRepo.CanIssueDurableReceipt(ctx, vTarget.ID)
	if err != nil || !canReceiptRestored {
		t.Fatalf("expected CanIssueDurableReceipt=true after repair, got %v (%v)", canReceiptRestored, err)
	}

	// Verify restored chunk file matches expected content
	restoredData, err := os.ReadFile(chunkPath)
	if err != nil {
		t.Fatalf("read restored chunk: %v", err)
	}
	if !bytes.Equal(restoredData, content) {
		t.Fatalf("restored data mismatch: got %q, want %q", restoredData, content)
	}
}

func TestRepairSharedChunkRestoresBothVersions(t *testing.T) {
	fix := newSyncFixture(t)
	ctx := context.Background()

	content := []byte("shared chunk between docA and docB")
	if err := os.WriteFile(filepath.Join(fix.senderRoot, "docA.txt"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fix.senderRoot, "docB.txt"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	scanRes, err := fix.senderWork.Scan(ctx, fix.folder)
	if err != nil || len(scanRes.Captured) != 2 {
		t.Fatalf("scan = %+v, %v", scanRes, err)
	}
	vA := scanRes.Captured[0]
	vB := scanRes.Captured[1]

	syncer := fix.newSyncer(TransferOptions{})
	if _, err := syncer.Sync(ctx); err != nil {
		t.Fatalf("sync error = %v", err)
	}

	digest := vA.Manifest.Chunks[0].Digest

	// Quarantine shared chunk on receiver
	affected, err := fix.receiverRepo.QuarantineChunk(ctx, digest, "shared corruption")
	if err != nil {
		t.Fatalf("quarantine: %v", err)
	}
	if len(affected) != 2 {
		t.Fatalf("expected 2 affected versions, got %d (%v)", len(affected), affected)
	}

	availA, _ := fix.receiverRepo.ContentAvailability(ctx, vA.ID)
	availB, _ := fix.receiverRepo.ContentAvailability(ctx, vB.ID)
	if availA != repository.ContentCorrupt || availB != repository.ContentCorrupt {
		t.Fatalf("expected both corrupt, got A=%v, B=%v", availA, availB)
	}

	repairer := NewRepairer(fix.receiverRepo, fix.receiverID.DeviceID, []RepairPeer{
		{DeviceID: fix.senderID.DeviceID, Client: fix.client},
	})

	// Repairing vA should restore the shared chunk, restoring BOTH vA and vB!
	report, err := repairer.RepairVersion(ctx, fix.folder, vA.ID, nil)
	if err != nil {
		t.Fatalf("RepairVersion: %v", err)
	}
	if report.Status != "repaired" {
		t.Fatalf("expected repaired status, got %s", report.Status)
	}

	availARestored, _ := fix.receiverRepo.ContentAvailability(ctx, vA.ID)
	availBRestored, _ := fix.receiverRepo.ContentAvailability(ctx, vB.ID)
	if availARestored != repository.ContentReady || availBRestored != repository.ContentReady {
		t.Fatalf("expected both ready after repairing shared chunk, got A=%v, B=%v", availARestored, availBRestored)
	}
}

func TestRepairNoRemainingCopyYieldsUnavailable(t *testing.T) {
	fix := newSyncFixture(t)
	ctx := context.Background()

	content := []byte("unique chunk lost on all peers")
	digest := sha256.Sum256(content)
	chunk := history.Chunk{Digest: digest, Length: uint64(len(content))}
	manifest := &history.Manifest{
		Size:   uint64(len(content)),
		Chunks: []history.Chunk{chunk},
		Digest: digest,
	}

	// Receiver creates version and quarantines it
	_ = fix.receiverRepo.InstallChunk(ctx, digest, chunk.Length, bytes.NewReader(content))
	vRecv, _ := fix.receiverRepo.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           fix.folder,
		Path:             "lost.txt",
		Kind:             history.KindFile,
		Manifest:         manifest,
		AuthoredRevision: 1,
	})
	_, _ = fix.receiverRepo.QuarantineChunk(ctx, digest, "corrupt")

	// Notice: sender does NOT have this chunk!
	repairer := NewRepairer(fix.receiverRepo, fix.receiverID.DeviceID, []RepairPeer{
		{DeviceID: fix.senderID.DeviceID, Client: fix.client},
	})

	report, err := repairer.RepairVersion(ctx, fix.folder, vRecv.ID, nil)
	if !errors.Is(err, ErrRepairFailed) {
		t.Fatalf("expected ErrRepairFailed, got %v", err)
	}
	if report.Status != "unavailable" {
		t.Fatalf("expected status unavailable, got %s", report.Status)
	}

	// Content remains corrupt/unavailable
	avail, _ := fix.receiverRepo.ContentAvailability(ctx, vRecv.ID)
	if avail != repository.ContentCorrupt {
		t.Fatalf("expected still corrupt, got %v", avail)
	}
}

func TestRepairUnauthorizedFolderRejected(t *testing.T) {
	fix := newSyncFixture(t)
	ctx := context.Background()

	unauthorizedFolder := history.ID{0x99}
	targetID := history.VersionID{
		Folder:  unauthorizedFolder,
		Author:  fix.receiverID.DeviceID,
		Counter: 1,
	}

	repairer := NewRepairer(fix.receiverRepo, fix.receiverID.DeviceID, []RepairPeer{
		{DeviceID: fix.senderID.DeviceID, Client: fix.client},
	})

	_, err := repairer.RepairVersion(ctx, unauthorizedFolder, targetID, nil)
	if !errors.Is(err, repository.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

type corruptMockClient struct {
	PeerClient
	badBytes []byte
}

func (m *corruptMockClient) Chunk(ctx context.Context, req ChunkRequest, exp history.Chunk) ([]byte, error) {
	return m.badBytes, nil
}

func TestRepairCorruptPeerResponseDiagnosedNoInfiniteRetry(t *testing.T) {
	fix := newSyncFixture(t)
	ctx := context.Background()

	content := []byte("clean content")
	digest := sha256.Sum256(content)
	chunk := history.Chunk{Digest: digest, Length: uint64(len(content))}
	manifest := &history.Manifest{
		Size:   uint64(len(content)),
		Chunks: []history.Chunk{chunk},
		Digest: digest,
	}

	_ = fix.receiverRepo.InstallChunk(ctx, digest, chunk.Length, bytes.NewReader(content))
	vRecv, _ := fix.receiverRepo.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           fix.folder,
		Path:             "corrupt-resp.txt",
		Kind:             history.KindFile,
		Manifest:         manifest,
		AuthoredRevision: 1,
	})
	_, _ = fix.receiverRepo.QuarantineChunk(ctx, digest, "corrupt")

	// Mock peer returns bad bytes
	mock := &corruptMockClient{
		PeerClient: fix.client,
		badBytes:   []byte("garbage bad bytes from peer"),
	}

	repairer := NewRepairer(fix.receiverRepo, fix.receiverID.DeviceID, []RepairPeer{
		{DeviceID: fix.senderID.DeviceID, Client: mock},
	})

	report, err := repairer.RepairVersion(ctx, fix.folder, vRecv.ID, nil)
	if !errors.Is(err, ErrRepairFailed) {
		t.Fatalf("expected ErrRepairFailed, got %v", err)
	}
	if report.Status != "unavailable" {
		t.Fatalf("expected unavailable, got %s", report.Status)
	}

	// Verify peer integrity incident was recorded
	incidents, err := fix.receiverRepo.PeerIntegrityIncidents(ctx, fix.senderID.DeviceID, fix.folder)
	if err != nil || incidents == 0 {
		t.Fatalf("expected peer integrity incidents recorded, got %d (%v)", incidents, err)
	}
}
