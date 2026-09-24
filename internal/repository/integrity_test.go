package repository

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
)

func TestQuarantineCorruptChunkAndAffectedVersions(t *testing.T) {
	ctx := context.Background()
	db := openTestRepository(t, Options{})
	stateDir := db.stateDir

	folder := history.ID{0x01}
	author := history.ID{0xaa}
	if err := db.EnsureFolder(ctx, folder, author, 1); err != nil {
		t.Fatalf("EnsureFolder: %v", err)
	}

	// Create a shared chunk
	sharedData := []byte("shared chunk payload between v1 and v2")
	sharedDigest := sha256.Sum256(sharedData)
	if err := db.InstallChunk(ctx, sharedDigest, uint64(len(sharedData)), bytesReader(sharedData)); err != nil {
		t.Fatalf("InstallChunk shared: %v", err)
	}

	manifest1 := &history.Manifest{
		Size:   uint64(len(sharedData)),
		Chunks: []history.Chunk{{Digest: sharedDigest, Length: uint64(len(sharedData))}},
		Digest: sharedDigest,
	}

	v1, err := db.CreateLocalVersion(ctx, LocalVersionRequest{
		Folder:           folder,
		Path:             "doc1.txt",
		Kind:             history.KindFile,
		Manifest:         manifest1,
		AuthoredRevision: 1,
	})
	if err != nil {
		t.Fatalf("CreateLocalVersion v1: %v", err)
	}

	manifest2 := &history.Manifest{
		Size:   uint64(len(sharedData)),
		Chunks: []history.Chunk{{Digest: sharedDigest, Length: uint64(len(sharedData))}},
		Digest: sharedDigest,
	}

	v2, err := db.CreateLocalVersion(ctx, LocalVersionRequest{
		Folder:           folder,
		Path:             "doc2.txt",
		Kind:             history.KindFile,
		Manifest:         manifest2,
		AuthoredRevision: 1,
	})
	if err != nil {
		t.Fatalf("CreateLocalVersion v2: %v", err)
	}

	// Verify initial availability
	avail1, err := db.ContentAvailability(ctx, v1.ID)
	if err != nil || avail1 != ContentReady {
		t.Fatalf("expected v1 ready, got %v (%v)", avail1, err)
	}
	avail2, err := db.ContentAvailability(ctx, v2.ID)
	if err != nil || avail2 != ContentReady {
		t.Fatalf("expected v2 ready, got %v (%v)", avail2, err)
	}

	// Durable receipt can be issued initially
	canReceipt1, err := db.CanIssueDurableReceipt(ctx, v1.ID)
	if err != nil || !canReceipt1 {
		t.Fatalf("expected can receipt v1, got %v (%v)", canReceipt1, err)
	}

	// Injected corruption of the shared chunk on disk
	sharedPath := filepath.Join(stateDir, "objects", "sha256", fmt.Sprintf("%02x", sharedDigest[0]), fmt.Sprintf("%x", sharedDigest[1:]))
	if err := os.WriteFile(sharedPath, []byte("tampered corrupted bytes!"), 0o600); err != nil {
		t.Fatalf("corrupt file: %v", err)
	}

	// Quarantine the corrupt chunk
	affected, err := db.QuarantineChunk(ctx, sharedDigest, "injected test corruption")
	if err != nil {
		t.Fatalf("QuarantineChunk: %v", err)
	}

	// Verify both versions are diagnosed as affected
	if len(affected) != 2 {
		t.Fatalf("expected 2 affected versions, got %d", len(affected))
	}
	foundV1, foundV2 := false, false
	for _, id := range affected {
		if id == v1.ID {
			foundV1 = true
		}
		if id == v2.ID {
			foundV2 = true
		}
	}
	if !foundV1 || !foundV2 {
		t.Fatalf("expected both v1 and v2 affected, got %v", affected)
	}

	// Verify chunk was moved to quarantine/ and removed from objects/sha256/
	if _, err := os.Stat(sharedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected original object to be removed, got err=%v", err)
	}
	quarantineEntries, err := db.ListQuarantinedChunks(ctx)
	if err != nil || len(quarantineEntries) != 1 {
		t.Fatalf("expected 1 quarantined entry, got %d (err=%v)", len(quarantineEntries), err)
	}
	if quarantineEntries[0].Digest != sharedDigest || quarantineEntries[0].Repaired {
		t.Fatalf("unexpected quarantined record: %+v", quarantineEntries[0])
	}

	// Verify ContentAvailability now reports ContentCorrupt
	avail1After, _ := db.ContentAvailability(ctx, v1.ID)
	if avail1After != ContentCorrupt {
		t.Fatalf("expected v1 corrupt, got %v", avail1After)
	}
	avail2After, _ := db.ContentAvailability(ctx, v2.ID)
	if avail2After != ContentCorrupt {
		t.Fatalf("expected v2 corrupt, got %v", avail2After)
	}

	// Verify DiagnoseVersionAvailability reports corrupt with detailed chunk
	diag1, err := db.DiagnoseVersionAvailability(ctx, v1.ID)
	if err != nil {
		t.Fatalf("DiagnoseVersionAvailability: %v", err)
	}
	if diag1.Status != "corrupt" || len(diag1.CorruptChunks) != 1 || diag1.CorruptChunks[0] != sharedDigest {
		t.Fatalf("unexpected diag1: %+v", diag1)
	}

	// No success receipt can be issued for corrupt content!
	canReceiptAfter, err := db.CanIssueDurableReceipt(ctx, v1.ID)
	if canReceiptAfter {
		t.Fatalf("CanIssueDurableReceipt must return false for corrupt content, got true")
	}

	// Test Unquarantine / repair
	// Reinstall the verified chunk bytes
	if err := db.InstallChunk(ctx, sharedDigest, uint64(len(sharedData)), bytesReader(sharedData)); err != nil {
		t.Fatalf("reinstall chunk: %v", err)
	}
	if err := db.UnquarantineChunk(ctx, sharedDigest); err != nil {
		t.Fatalf("UnquarantineChunk: %v", err)
	}

	// Both versions restored to ready
	avail1Restored, _ := db.ContentAvailability(ctx, v1.ID)
	if avail1Restored != ContentReady {
		t.Fatalf("expected v1 restored to ready, got %v", avail1Restored)
	}
	avail2Restored, _ := db.ContentAvailability(ctx, v2.ID)
	if avail2Restored != ContentReady {
		t.Fatalf("expected v2 restored to ready, got %v", avail2Restored)
	}

	canReceiptRestored, err := db.CanIssueDurableReceipt(ctx, v1.ID)
	if err != nil || !canReceiptRestored {
		t.Fatalf("expected can receipt restored, got %v (%v)", canReceiptRestored, err)
	}
}

func TestCheckIntegrityAutoQuarantineAndReporting(t *testing.T) {
	ctx := context.Background()
	db := openTestRepository(t, Options{})
	stateDir := db.stateDir

	folder := history.ID{0x02}
	author := history.ID{0xbb}
	if err := db.EnsureFolder(ctx, folder, author, 1); err != nil {
		t.Fatalf("EnsureFolder: %v", err)
	}

	data1 := []byte("clean content data")
	digest1 := sha256.Sum256(data1)
	if err := db.InstallChunk(ctx, digest1, uint64(len(data1)), bytesReader(data1)); err != nil {
		t.Fatalf("InstallChunk 1: %v", err)
	}

	data2 := []byte("content to be corrupted")
	digest2 := sha256.Sum256(data2)
	if err := db.InstallChunk(ctx, digest2, uint64(len(data2)), bytesReader(data2)); err != nil {
		t.Fatalf("InstallChunk 2: %v", err)
	}

	manifest1 := &history.Manifest{
		Size:   uint64(len(data1)),
		Chunks: []history.Chunk{{Digest: digest1, Length: uint64(len(data1))}},
		Digest: digest1,
	}
	_, err := db.CreateLocalVersion(ctx, LocalVersionRequest{
		Folder:           folder,
		Path:             "file1.bin",
		Kind:             history.KindFile,
		Manifest:         manifest1,
		AuthoredRevision: 1,
	})
	if err != nil {
		t.Fatalf("CreateLocalVersion 1: %v", err)
	}

	manifest2 := &history.Manifest{
		Size:   uint64(len(data2)),
		Chunks: []history.Chunk{{Digest: digest2, Length: uint64(len(data2))}},
		Digest: digest2,
	}
	ver, err := db.CreateLocalVersion(ctx, LocalVersionRequest{
		Folder:           folder,
		Path:             "file2.bin",
		Kind:             history.KindFile,
		Manifest:         manifest2,
		AuthoredRevision: 1,
	})
	if err != nil {
		t.Fatalf("CreateLocalVersion 2: %v", err)
	}

	// Corrupt chunk 2 on disk
	chunk2Path := filepath.Join(stateDir, "objects", "sha256", fmt.Sprintf("%02x", digest2[0]), fmt.Sprintf("%x", digest2[1:]))
	if err := os.WriteFile(chunk2Path, []byte("bad corrupt bytes"), 0o600); err != nil {
		t.Fatalf("corrupt chunk 2: %v", err)
	}

	// Run integrity check with AutoQuarantine
	res, err := db.CheckIntegrity(ctx, IntegrityCheckRequest{
		Folder:         folder,
		AutoQuarantine: true,
	})
	if err != nil {
		t.Fatalf("CheckIntegrity: %v", err)
	}

	if res.CleanChunks != 1 {
		t.Fatalf("expected 1 clean chunk, got %d", res.CleanChunks)
	}
	if len(res.CorruptChunks) != 1 {
		t.Fatalf("expected 1 corrupt chunk, got %d", len(res.CorruptChunks))
	}
	if res.CorruptChunks[0].Digest != digest2 {
		t.Fatalf("expected corrupt chunk digest %x, got %x", digest2, res.CorruptChunks[0].Digest)
	}
	if len(res.CorruptChunks[0].AffectedVersions) != 1 || res.CorruptChunks[0].AffectedVersions[0] != ver.ID {
		t.Fatalf("expected affected version %v, got %v", ver.ID, res.CorruptChunks[0].AffectedVersions)
	}

	// Chunk 2 was auto-quarantined
	isQ, _, _ := db.IsChunkQuarantined(ctx, digest2)
	if !isQ {
		t.Fatal("expected chunk 2 to be quarantined")
	}
}

func TestCheckIntegrityCancelable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	db := openTestRepository(t, Options{})

	_, err := db.CheckIntegrity(ctx, IntegrityCheckRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestDiagnoseAllStates(t *testing.T) {
	ctx := context.Background()
	db := openTestRepository(t, Options{})
	stateDir := db.stateDir

	folder := history.ID{0x03}
	author := history.ID{0xcc}
	if err := db.EnsureFolder(ctx, folder, author, 1); err != nil {
		t.Fatalf("EnsureFolder: %v", err)
	}

	// 1. Ready file
	readyData := []byte("ready file content")
	readyDigest := sha256.Sum256(readyData)
	_ = db.InstallChunk(ctx, readyDigest, uint64(len(readyData)), bytesReader(readyData))
	mReady := &history.Manifest{Size: uint64(len(readyData)), Chunks: []history.Chunk{{Digest: readyDigest, Length: uint64(len(readyData))}}, Digest: readyDigest}
	vReady, _ := db.CreateLocalVersion(ctx, LocalVersionRequest{Folder: folder, Path: "ready.txt", Kind: history.KindFile, Manifest: mReady, AuthoredRevision: 1})

	dReady, _ := db.DiagnoseVersionAvailability(ctx, vReady.ID)
	if dReady.Status != "ready" {
		t.Fatalf("expected ready, got %s", dReady.Status)
	}

	// 2. Corrupt file
	corruptData := []byte("corrupt file content")
	corruptDigest := sha256.Sum256(corruptData)
	_ = db.InstallChunk(ctx, corruptDigest, uint64(len(corruptData)), bytesReader(corruptData))
	mCorrupt := &history.Manifest{Size: uint64(len(corruptData)), Chunks: []history.Chunk{{Digest: corruptDigest, Length: uint64(len(corruptData))}}, Digest: corruptDigest}
	vCorrupt, _ := db.CreateLocalVersion(ctx, LocalVersionRequest{Folder: folder, Path: "corrupt.txt", Kind: history.KindFile, Manifest: mCorrupt, AuthoredRevision: 1})
	_, _ = db.QuarantineChunk(ctx, corruptDigest, "injected")

	dCorrupt, _ := db.DiagnoseVersionAvailability(ctx, vCorrupt.ID)
	if dCorrupt.Status != "corrupt" {
		t.Fatalf("expected corrupt, got %s", dCorrupt.Status)
	}

	// 3. Missing protected file (head version whose chunk is removed from disk)
	missData := []byte("missing protected content")
	missDigest := sha256.Sum256(missData)
	_ = db.InstallChunk(ctx, missDigest, uint64(len(missData)), bytesReader(missData))
	mMiss := &history.Manifest{Size: uint64(len(missData)), Chunks: []history.Chunk{{Digest: missDigest, Length: uint64(len(missData))}}, Digest: missDigest}
	vMiss, _ := db.CreateLocalVersion(ctx, LocalVersionRequest{Folder: folder, Path: "missing.txt", Kind: history.KindFile, Manifest: mMiss, AuthoredRevision: 1})

	// Remove file on disk without unlinking in DB
	_ = os.Remove(filepath.Join(stateDir, "objects", "sha256", fmt.Sprintf("%02x", missDigest[0]), fmt.Sprintf("%x", missDigest[1:])))
	dMiss, _ := db.DiagnoseVersionAvailability(ctx, vMiss.ID)
	if dMiss.Status != "missing_protected" {
		t.Fatalf("expected missing_protected, got %s", dMiss.Status)
	}
}

func TestPeerIntegrityIncidents(t *testing.T) {
	ctx := context.Background()
	db := openTestRepository(t, Options{})

	peer := history.ID{0x55}
	folder := history.ID{0x66}
	chunk := history.Digest{0x77}

	if err := db.RecordPeerIntegrityIncident(ctx, peer, folder, chunk, "corrupt payload 1"); err != nil {
		t.Fatalf("RecordPeerIntegrityIncident: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := db.RecordPeerIntegrityIncident(ctx, peer, folder, chunk, "corrupt payload 2"); err != nil {
		t.Fatalf("RecordPeerIntegrityIncident: %v", err)
	}

	count, err := db.PeerIntegrityIncidents(ctx, peer, folder)
	if err != nil || count != 2 {
		t.Fatalf("expected 2 incidents, got %d (%v)", count, err)
	}
}
