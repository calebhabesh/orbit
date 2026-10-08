package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/calebhabesh/orbit/internal/history"
)

func repositoryID(label byte) history.ID {
	var id history.ID
	for i := range id {
		id[i] = label
	}
	return id
}

func openTestRepository(t *testing.T, options Options) *DB {
	t.Helper()
	db, err := OpenWithOptions(context.Background(), t.TempDir(), options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestStreamingManifestRepeatedChunksZeroByteAndWholeVerification(t *testing.T) {
	ctx := context.Background()
	db := openTestRepository(t, Options{})
	block := bytes.Repeat([]byte("r"), int(history.ChunkSize))
	content := append(append([]byte{}, block...), block...)
	manifest, err := db.StoreFile(ctx, bytes.NewReader(content), true)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Size != uint64(len(content)) || len(manifest.Chunks) != 2 || manifest.Chunks[0].Digest != manifest.Chunks[1].Digest || !manifest.Executable {
		t.Fatalf("unexpected repeated-chunk manifest: %+v", manifest)
	}
	if err := db.VerifyManifest(manifest); err != nil {
		t.Fatal(err)
	}
	empty, err := db.StoreFile(ctx, bytes.NewReader(nil), false)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Size != 0 || len(empty.Chunks) != 0 || empty.Digest != sha256.Sum256(nil) {
		t.Fatalf("bad empty manifest: %+v", empty)
	}
	bad := *manifest
	bad.Chunks = append([]history.Chunk(nil), manifest.Chunks...)
	bad.Digest = sha256.Sum256([]byte("wrong"))
	if !errors.Is(db.VerifyManifest(&bad), ErrContentMismatch) {
		t.Fatal("whole-file digest mismatch was accepted")
	}
}

func TestDuplicateObjectIsVerifiedAndNeverOverwritten(t *testing.T) {
	ctx := context.Background()
	db := openTestRepository(t, Options{})
	value := []byte("immutable")
	digest := sha256.Sum256(value)
	if err := db.InstallChunk(ctx, digest, uint64(len(value)), bytes.NewReader(value)); err != nil {
		t.Fatal(err)
	}
	if err := db.InstallChunk(ctx, digest, uint64(len(value)), bytes.NewReader(value)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(db.objectPath(digest), []byte("corrupted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(db.InstallChunk(ctx, digest, uint64(len(value)), bytes.NewReader(value)), ErrContentMismatch) {
		t.Fatal("corrupt duplicate was replaced or accepted")
	}
}

func TestLocalCounterVersionReadinessAndReferencesCommitAtomically(t *testing.T) {
	ctx := context.Background()
	folder, author := repositoryID('F'), repositoryID('A')
	state := t.TempDir()
	fail := true
	db, err := OpenWithOptions(ctx, state, Options{FaultHook: func(name string) error {
		if fail && name == HookBeforeVersionCommit {
			return errors.New("injected commit failure")
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.EnsureFolder(ctx, folder, author, 1); err != nil {
		t.Fatal(err)
	}
	manifest, err := db.StoreFile(ctx, bytes.NewReader([]byte("captured")), false)
	if err != nil {
		t.Fatal(err)
	}
	request := LocalVersionRequest{Folder: folder, Path: "note.txt", Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1}
	if _, err := db.CreateLocalVersion(ctx, request); err == nil {
		t.Fatal("injected transaction failure succeeded")
	}
	fail = false
	version, err := db.CreateLocalVersion(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if version.ID.Counter != 1 {
		t.Fatalf("counter=%d, want 1 after rollback", version.ID.Counter)
	}
	ready, err := db.ContentReady(ctx, version.ID)
	if err != nil || !ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	receipt, err := db.CanIssueDurableReceipt(ctx, version.ID)
	if err != nil || !receipt {
		t.Fatalf("receipt=%v err=%v", receipt, err)
	}
	orphans, err := db.OrphanObjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 0 {
		t.Fatalf("referenced object classified orphan: %x", orphans)
	}
}

func TestRemoteMetadataRemainsPendingUntilWholeFileReady(t *testing.T) {
	ctx := context.Background()
	folder, local, remote := repositoryID('F'), repositoryID('A'), repositoryID('B')
	db := openTestRepository(t, Options{})
	if err := db.EnsureFolder(ctx, folder, local, 1); err != nil {
		t.Fatal(err)
	}
	manifest, err := db.StoreFile(ctx, bytes.NewReader([]byte("remote")), false)
	if err != nil {
		t.Fatal(err)
	}
	envelope := history.Envelope{ID: history.VersionID{Folder: folder, Author: remote, Counter: 1}, Path: "remote.txt", Vector: []history.ClockEntry{{Author: remote, Counter: 1}}, Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1}
	if err := db.ImportMetadata(ctx, envelope); err != nil {
		t.Fatal(err)
	}
	changed := envelope
	changed.Manifest = cloneManifest(envelope.Manifest)
	changed.Manifest.Executable = true
	if !errors.Is(db.ImportMetadata(ctx, changed), history.ErrDuplicateID) {
		t.Fatal("changed duplicate version identity was accepted")
	}
	known, _ := db.MetadataKnown(ctx, envelope.ID)
	ready, _ := db.ContentReady(ctx, envelope.ID)
	receipt, _ := db.CanIssueDurableReceipt(ctx, envelope.ID)
	if !known || ready || receipt {
		t.Fatalf("known=%v ready=%v receipt=%v", known, ready, receipt)
	}
	// Temporarily release the owner to reproduce a conflicting external writer
	// at reopen. A running exclusive owner correctly prevents that writer.
	if err := db.db.Close(); err != nil {
		t.Fatal(err)
	}
	locker, err := sql.Open("sqlite", filepath.Join(db.stateDir, "metadata.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	lockTx, err := locker.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockTx.ExecContext(ctx, `INSERT INTO installation_metadata(key,value) VALUES('busy-lock',x'01')`); err != nil {
		t.Fatal(err)
	}
	db.db, err = sql.Open("sqlite", filepath.Join(db.stateDir, "metadata.sqlite")+"?_pragma=busy_timeout(1)&_pragma=locking_mode(EXCLUSIVE)")
	if err != nil {
		t.Fatal(err)
	}
	db.db.SetMaxOpenConns(1)
	db.db.SetMaxIdleConns(1)
	if err := db.MarkContentReady(ctx, envelope.ID); err == nil {
		t.Fatal("SQLite busy condition marked content ready")
	}
	if err := lockTx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := locker.Close(); err != nil {
		t.Fatal(err)
	}
	if ready, err := db.ContentReady(ctx, envelope.ID); err != nil || ready {
		t.Fatalf("SQLite busy condition left false readiness: ready=%v err=%v", ready, err)
	}
	db.hook = func(name string) error {
		if name == HookBeforeCheckpoint {
			return errors.New("checkpoint failure")
		}
		return nil
	}
	if err := db.Checkpoint(ctx); err == nil {
		t.Fatal("checkpoint fault succeeded")
	}
	if ready, _ := db.ContentReady(ctx, envelope.ID); ready {
		t.Fatal("checkpoint failure left false readiness")
	}
	db.hook = func(name string) error {
		if name == HookBeforeReadyCommit {
			return errors.New("busy/checkpoint-like failure")
		}
		return nil
	}
	if err := db.MarkContentReady(ctx, envelope.ID); err == nil {
		t.Fatal("readiness fault succeeded")
	}
	if ready, _ := db.ContentReady(ctx, envelope.ID); ready {
		t.Fatal("failed readiness commit became ready")
	}
	db.hook = nil
	if err := db.MarkContentReady(ctx, envelope.ID); err != nil {
		t.Fatal(err)
	}
	if ready, _ := db.ContentReady(ctx, envelope.ID); !ready {
		t.Fatal("verified content did not become ready")
	}
}

func TestOrphansPinsReservationsBudgetAndBackup(t *testing.T) {
	ctx := context.Background()
	state := t.TempDir()
	db, err := OpenWithOptions(ctx, state, Options{})
	if err != nil {
		t.Fatal(err)
	}
	value := []byte("orphan")
	digest := sha256.Sum256(value)
	if err := db.InstallChunk(ctx, digest, uint64(len(value)), bytes.NewReader(value)); err != nil {
		t.Fatal(err)
	}
	orphans, err := db.OrphanObjects(ctx)
	if err != nil || len(orphans) != 1 || orphans[0] != digest {
		t.Fatalf("orphans=%x err=%v", orphans, err)
	}
	if err := db.Pin(ctx, digest, "restore", "op-1"); err != nil {
		t.Fatal(err)
	}
	orphans, _ = db.OrphanObjects(ctx)
	if len(orphans) != 0 {
		t.Fatal("pinned object remained orphan candidate")
	}
	if err := db.Unpin(ctx, digest, "restore", "op-1"); err != nil {
		t.Fatal(err)
	}
	if err := db.Reserve(ctx, "transfer-1", 123, "incoming chunk"); err != nil {
		t.Fatal(err)
	}
	usage, err := db.StorageUsage(ctx)
	if err != nil || usage.Reserved != 123 {
		t.Fatalf("usage=%+v err=%v", usage, err)
	}
	if err := db.PutInstallationValue(ctx, "backup-probe", []byte("present")); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(state, "operations", "backup.sqlite")
	if err := db.Backup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", backup)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var got []byte
	if err := raw.QueryRow(`SELECT value FROM installation_metadata WHERE key='backup-probe'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if string(got) != "present" {
		t.Fatalf("backup value=%q", got)
	}
}

func TestDiskFullFaultLeavesNoInstalledObject(t *testing.T) {
	ctx := context.Background()
	db := openTestRepository(t, Options{FaultHook: func(name string) error {
		if name == HookObjectFlushed {
			return syscall.ENOSPC
		}
		return nil
	}})
	value := []byte("no space")
	digest := sha256.Sum256(value)
	if err := db.InstallChunk(ctx, digest, uint64(len(value)), bytes.NewReader(value)); err == nil {
		t.Fatal("ENOSPC fault succeeded")
	}
	if _, err := os.Stat(db.objectPath(digest)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("object exists after pre-install fault: %v", err)
	}
}

func TestMalformedManifestAndBudgetAreRejected(t *testing.T) {
	ctx := context.Background()
	db := openTestRepository(t, Options{})
	folder, local, remote := repositoryID('F'), repositoryID('A'), repositoryID('B')
	if err := db.EnsureFolder(ctx, folder, local, 1); err != nil {
		t.Fatal(err)
	}
	value := []byte("short")
	digest := sha256.Sum256(value)
	bad := &history.Manifest{Size: uint64(len(value)) + 1, Digest: digest, Chunks: []history.Chunk{{Digest: digest, Length: uint64(len(value))}}}
	envelope := history.Envelope{ID: history.VersionID{Folder: folder, Author: remote, Counter: 1}, Path: "bad", Vector: []history.ClockEntry{{Author: remote, Counter: 1}}, Kind: history.KindFile, Manifest: bad, AuthoredRevision: 1}
	if !errors.Is(db.ImportMetadata(ctx, envelope), history.ErrInvalidEnvelope) {
		t.Fatal("malformed manifest lengths were accepted")
	}

	state := t.TempDir()
	limited, err := OpenWithOptions(ctx, state, Options{BudgetBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer limited.Close()
	if err := limited.InstallChunk(ctx, digest, uint64(len(value)), bytes.NewReader(value)); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("budget error=%v", err)
	}
}

func TestKnownLocalIdentityCounterIsNeverReusedAfterRollbackSignal(t *testing.T) {
	ctx := context.Background()
	db := openTestRepository(t, Options{})
	folder, author := repositoryID('F'), repositoryID('A')
	if err := db.EnsureFolder(ctx, folder, author, 1); err != nil {
		t.Fatal(err)
	}
	old := history.Envelope{ID: history.VersionID{Folder: folder, Author: author, Counter: 1}, Path: "old.txt", Vector: []history.ClockEntry{{Author: author, Counter: 1}}, Kind: history.KindTombstone, AuthoredRevision: 1}
	if err := db.ImportMetadata(ctx, old); err != nil {
		t.Fatal(err)
	}
	_, err := db.CreateLocalVersion(ctx, LocalVersionRequest{Folder: folder, Path: "new.txt", Kind: history.KindTombstone, AuthoredRevision: 1})
	if !errors.Is(err, history.ErrDuplicateID) {
		t.Fatalf("rolled-back identity counter reuse error=%v", err)
	}
}
