package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
)

func TestRetentionPolicyGetSet(t *testing.T) {
	ctx := context.Background()
	db := openTestRepository(t, Options{})
	folder, author := repositoryID('F'), repositoryID('A')
	if err := db.EnsureFolder(ctx, folder, author, 1); err != nil {
		t.Fatal(err)
	}

	// Default policy
	policy, err := db.GetRetentionPolicy(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if policy.RetentionDays != 30 || policy.MinSuperseded != 20 {
		t.Fatalf("unexpected default policy: %+v", policy)
	}

	// Update policy
	newPolicy := RetentionPolicy{RetentionDays: 7, MinSuperseded: 5}
	if err := db.SetRetentionPolicy(ctx, folder, newPolicy); err != nil {
		t.Fatal(err)
	}

	got, err := db.GetRetentionPolicy(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if got != newPolicy {
		t.Fatalf("got policy %+v, want %+v", got, newPolicy)
	}

	// Invalid policy
	if err := db.SetRetentionPolicy(ctx, folder, RetentionPolicy{RetentionDays: -1, MinSuperseded: 5}); err == nil {
		t.Fatal("expected error on negative retention days")
	}
}

func TestGCPreservesHeadsFallbackAndPins(t *testing.T) {
	ctx := context.Background()
	state := t.TempDir()
	db, err := OpenWithOptions(ctx, state, Options{})
	if err != nil {
		t.Fatal(err)
	}
	folder, author := repositoryID('F'), repositoryID('A')
	if err := db.EnsureFolder(ctx, folder, author, 1); err != nil {
		t.Fatal(err)
	}

	// 1. Create a version with chunk1 (this will become superseded)
	val1 := []byte("superseded-content")
	m1, err := db.StoreFile(ctx, bytes.NewReader(val1), false)
	if err != nil {
		t.Fatal(err)
	}
	v1, err := db.CreateLocalVersion(ctx, LocalVersionRequest{
		Folder: folder, Path: "doc.txt", Kind: history.KindFile, Manifest: m1, AuthoredRevision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 2. Create version 2 with chunk2 (this is the current head)
	val2 := []byte("current-head-content")
	m2, err := db.StoreFile(ctx, bytes.NewReader(val2), false)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := db.CreateLocalVersion(ctx, LocalVersionRequest{
		Folder: folder, Path: "doc.txt", Basis: []history.VersionID{v1.ID}, Kind: history.KindFile, Manifest: m2, AuthoredRevision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 3. Create an unreferenced orphan chunk3
	val3 := []byte("orphan-chunk")
	d3 := sha256.Sum256(val3)
	if err := db.InstallChunk(ctx, d3, uint64(len(val3)), bytes.NewReader(val3)); err != nil {
		t.Fatal(err)
	}

	// 4. Create an unreferenced chunk4 but PIN it
	val4 := []byte("pinned-chunk")
	d4 := sha256.Sum256(val4)
	if err := db.InstallChunk(ctx, d4, uint64(len(val4)), bytes.NewReader(val4)); err != nil {
		t.Fatal(err)
	}
	if err := db.Pin(ctx, d4, "transfer", "t-1"); err != nil {
		t.Fatal(err)
	}

	// Configure retention: 0 days retention, 0 min superseded -> v1 is eligible for GC!
	aggressivePolicy := RetentionPolicy{RetentionDays: 0, MinSuperseded: 0}
	now := time.Now().Add(24 * time.Hour) // 1 day in the future

	// Preview GC
	preview, err := db.GCPreview(ctx, folder, &aggressivePolicy, now)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Candidates != 2 { // v1's chunk + orphan chunk3
		t.Fatalf("candidates=%d, want 2", preview.Candidates)
	}

	// Run GC
	report, err := db.RunGC(ctx, folder, &aggressivePolicy, now)
	if err != nil {
		t.Fatal(err)
	}
	if report.UnlinkedObjects != 2 {
		t.Fatalf("unlinked=%d, want 2", report.UnlinkedObjects)
	}

	// Check files on disk
	// chunk1 (v1) should be gone
	if _, err := os.Stat(db.objectPath(m1.Chunks[0].Digest)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("v1 chunk still exists: %v", err)
	}
	// chunk3 (orphan) should be gone
	if _, err := os.Stat(db.objectPath(d3)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan chunk still exists: %v", err)
	}
	// chunk2 (head v2) MUST exist and be intact!
	if err := db.verifyObjectFile(m2.Chunks[0].Digest, m2.Chunks[0].Length); err != nil {
		t.Fatalf("head chunk corrupted or missing: %v", err)
	}
	// chunk4 (pinned) MUST exist and be intact!
	if err := db.verifyObjectFile(d4, uint64(len(val4))); err != nil {
		t.Fatalf("pinned chunk corrupted or missing: %v", err)
	}

	// Availability of v1 should be ContentExpired
	availV1, err := db.ContentAvailability(ctx, v1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if availV1 != ContentExpired {
		t.Fatalf("v1 availability = %s, want %s", availV1, ContentExpired)
	}

	// Availability of v2 should be ContentReady
	availV2, err := db.ContentAvailability(ctx, v2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if availV2 != ContentReady {
		t.Fatalf("v2 availability = %s, want %s", availV2, ContentReady)
	}

	// Causal metadata of v1 is still intact in history!
	known, err := db.MetadataKnown(ctx, v1.ID)
	if err != nil || !known {
		t.Fatalf("v1 metadata missing: known=%v err=%v", known, err)
	}
}

func TestGCPreservesFallbackContentWhileHeadPending(t *testing.T) {
	ctx := context.Background()
	state := t.TempDir()
	db, err := OpenWithOptions(ctx, state, Options{})
	if err != nil {
		t.Fatal(err)
	}
	folder, author, remote := repositoryID('F'), repositoryID('A'), repositoryID('B')
	if err := db.EnsureFolder(ctx, folder, author, 1); err != nil {
		t.Fatal(err)
	}

	// 1. Create and apply version 1 locally
	val1 := []byte("applied-v1-bytes")
	m1, err := db.StoreFile(ctx, bytes.NewReader(val1), false)
	if err != nil {
		t.Fatal(err)
	}
	v1, err := db.CreateLocalVersion(ctx, LocalVersionRequest{
		Folder: folder, Path: "file.txt", Kind: history.KindFile, Manifest: m1, AuthoredRevision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 2. Receive remote version 2 (head) with content pending
	val2 := []byte("remote-v2-bytes")
	d2 := sha256.Sum256(val2)
	m2 := &history.Manifest{Size: uint64(len(val2)), Digest: d2, Chunks: []history.Chunk{{Digest: d2, Length: uint64(len(val2))}}}
	env2 := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: remote, Counter: 1},
		Path:             "file.txt",
		Parents:          []history.VersionID{v1.ID},
		Vector:           []history.ClockEntry{{Author: author, Counter: v1.ID.Counter}, {Author: remote, Counter: 1}},
		Kind:             history.KindFile,
		Manifest:         m2,
		AuthoredRevision: 1,
	}
	if err := db.ImportMetadata(ctx, env2); err != nil {
		t.Fatal(err)
	}

	// Even with aggressive 0-day retention, v1 is FALLBACK CONTENT because v2 is pending!
	aggressivePolicy := RetentionPolicy{RetentionDays: 0, MinSuperseded: 0}
	now := time.Now().Add(24 * time.Hour)

	protected, err := db.ComputeProtectedChunks(ctx, folder, &aggressivePolicy, now)
	if err != nil {
		t.Fatal(err)
	}
	if !protected[m1.Chunks[0].Digest] {
		t.Fatal("v1 chunk must be protected as pending-publication fallback content")
	}

	report, err := db.RunGC(ctx, folder, &aggressivePolicy, now)
	if err != nil {
		t.Fatal(err)
	}
	if report.UnlinkedObjects != 0 {
		t.Fatalf("unlinked=%d, want 0 (v1 is fallback)", report.UnlinkedObjects)
	}

	if err := db.verifyObjectFile(m1.Chunks[0].Digest, m1.Chunks[0].Length); err != nil {
		t.Fatalf("v1 chunk was unlinked: %v", err)
	}
}

func TestGCServeLeaseExclusion(t *testing.T) {
	ctx := context.Background()
	state := t.TempDir()
	db, err := OpenWithOptions(ctx, state, Options{})
	if err != nil {
		t.Fatal(err)
	}
	folder, author := repositoryID('F'), repositoryID('A')
	if err := db.EnsureFolder(ctx, folder, author, 1); err != nil {
		t.Fatal(err)
	}

	val := []byte("chunk-with-lease")
	d := sha256.Sum256(val)
	if err := db.InstallChunk(ctx, d, uint64(len(val)), bytes.NewReader(val)); err != nil {
		t.Fatal(err)
	}

	// Acquire serve lease
	if err := db.AcquireServeLease(ctx, d, "lease-101"); err != nil {
		t.Fatal(err)
	}

	// GC should not collect it because serve lease pins it
	zeroPolicy := RetentionPolicy{RetentionDays: 0, MinSuperseded: 0}
	report, err := db.RunGC(ctx, folder, &zeroPolicy, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if report.UnlinkedObjects != 0 {
		t.Fatalf("unlinked=%d with active serve lease", report.UnlinkedObjects)
	}

	// Release serve lease
	if err := db.ReleaseServeLease(ctx, d, "lease-101"); err != nil {
		t.Fatal(err)
	}

	// Now GC unlinks it
	report, err = db.RunGC(ctx, folder, &zeroPolicy, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if report.UnlinkedObjects != 1 {
		t.Fatalf("unlinked=%d, want 1 after lease release", report.UnlinkedObjects)
	}
}

func TestGCSuspendedDuringMaintenance(t *testing.T) {
	ctx := context.Background()
	state := t.TempDir()
	db, err := OpenWithOptions(ctx, state, Options{})
	if err != nil {
		t.Fatal(err)
	}
	folder, author, peer := repositoryID('F'), repositoryID('A'), repositoryID('B')
	if err := db.EnsureFolder(ctx, folder, author, 1); err != nil {
		t.Fatal(err)
	}

	val := []byte("content-during-maint")
	m, err := db.StoreFile(ctx, bytes.NewReader(val), false)
	if err != nil {
		t.Fatal(err)
	}
	v1, err := db.CreateLocalVersion(ctx, LocalVersionRequest{
		Folder: folder, Path: "doc.txt", Kind: history.KindFile, Manifest: m, AuthoredRevision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Create second version so v1 is superseded
	val2 := []byte("head-content")
	m2, err := db.StoreFile(ctx, bytes.NewReader(val2), false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.CreateLocalVersion(ctx, LocalVersionRequest{
		Folder: folder, Path: "doc.txt", Basis: []history.VersionID{v1.ID}, Kind: history.KindFile, Manifest: m2, AuthoredRevision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Save active resumable maintenance (e.g. retiring peer)
	if err := db.SaveResumableMaintenance(ctx, "maint-1", folder, peer, "computing_snapshot", []byte("{}"), time.Now()); err != nil {
		t.Fatal(err)
	}

	// Attempt GC
	zeroPolicy := RetentionPolicy{RetentionDays: 0, MinSuperseded: 0}
	report, err := db.RunGC(ctx, folder, &zeroPolicy, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Suspended {
		t.Fatal("GC should be suspended while maintenance is in progress")
	}
	if report.UnlinkedObjects != 0 {
		t.Fatalf("unlinked=%d during suspended GC", report.UnlinkedObjects)
	}

	// Finish maintenance
	if err := db.DeleteResumableMaintenance(ctx, "maint-1"); err != nil {
		t.Fatal(err)
	}

	// Now GC can run
	report, err = db.RunGC(ctx, folder, &zeroPolicy, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if report.Suspended {
		t.Fatal("GC should not be suspended after maintenance is deleted")
	}
	if report.UnlinkedObjects != 1 {
		t.Fatalf("unlinked=%d, want 1", report.UnlinkedObjects)
	}
}

func TestGCInterruptedCrashRecovery(t *testing.T) {
	ctx := context.Background()
	state := t.TempDir()

	// 1. Crash after HookGCIntent (intent acquired, but not unlinked)
	var intentRan bool
	db1, err := OpenWithOptions(ctx, state, Options{
		FaultHook: func(name string) error {
			if name == HookGCIntent {
				intentRan = true
				return errors.New("simulated crash at HookGCIntent")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	folder, author := repositoryID('F'), repositoryID('A')
	if err := db1.EnsureFolder(ctx, folder, author, 1); err != nil {
		t.Fatal(err)
	}

	orphanVal := []byte("orphan-bytes-for-intent")
	orphanDigest := sha256.Sum256(orphanVal)
	if err := db1.InstallChunk(ctx, orphanDigest, uint64(len(orphanVal)), bytes.NewReader(orphanVal)); err != nil {
		t.Fatal(err)
	}

	zeroPolicy := RetentionPolicy{RetentionDays: 0, MinSuperseded: 0}
	_, err = db1.RunGC(ctx, folder, &zeroPolicy, time.Now().Add(time.Hour))
	if err == nil || !intentRan {
		t.Fatalf("expected fault at HookGCIntent, got %v", err)
	}
	db1.Close()

	// Reopen state - startup reconciliation should retain the object file and clear intent
	db2, err := OpenWithOptions(ctx, state, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// Verify file is still present and valid
	if err := db2.verifyObjectFile(orphanDigest, uint64(len(orphanVal))); err != nil {
		t.Fatalf("object lost after intent crash: %v", err)
	}
	// gc_intents row should be cleared
	var intentCount int
	_ = db2.db.QueryRowContext(ctx, `SELECT count(*) FROM gc_intents`).Scan(&intentCount)
	if intentCount != 0 {
		t.Fatalf("intent count = %d, want 0 after recovery", intentCount)
	}
	db2.Close()

	// 2. Crash after HookGCUnlink (unlinked, but metadata not finalized)
	var unlinkRan bool
	db3, err := OpenWithOptions(ctx, state, Options{
		FaultHook: func(name string) error {
			if name == HookGCUnlink {
				unlinkRan = true
				return errors.New("simulated crash at HookGCUnlink")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = db3.RunGC(ctx, folder, &zeroPolicy, time.Now().Add(time.Hour))
	if err == nil || !unlinkRan {
		t.Fatalf("expected fault at HookGCUnlink, got %v", err)
	}
	db3.Close()

	// Reopen state - startup reconciliation should finalize unlinked unreferenced object
	db4, err := OpenWithOptions(ctx, state, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// Object should be finalized (removed from objects table and gc_intents)
	var objCount int
	_ = db4.db.QueryRowContext(ctx, `SELECT count(*) FROM objects WHERE digest=?`, orphanDigest[:]).Scan(&objCount)
	if objCount != 0 {
		t.Fatalf("unlinked object was not finalized on recovery: %d", objCount)
	}
	db4.Close()
}

func TestMetadataBudgetExceeded(t *testing.T) {
	ctx := context.Background()
	state := t.TempDir()
	// Set a tiny metadata budget (e.g. 500 bytes)
	db, err := OpenWithOptions(ctx, state, Options{MetadataBudgetBytes: 500})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder, author := repositoryID('F'), repositoryID('A')
	if err := db.EnsureFolder(ctx, folder, author, 1); err != nil {
		t.Fatal(err)
	}

	// SQLite database file already exceeds 500 bytes (min sqlite db is 4096 bytes page)
	_, err = db.CreateLocalVersion(ctx, LocalVersionRequest{
		Folder: folder, Path: "f.txt", Kind: history.KindTombstone, AuthoredRevision: 1,
	})
	if !errors.Is(err, ErrMetadataBudgetExceeded) {
		t.Fatalf("expected ErrMetadataBudgetExceeded, got: %v", err)
	}
}

func TestDetailedStorageUsage(t *testing.T) {
	ctx := context.Background()
	state := t.TempDir()
	db, err := OpenWithOptions(ctx, state, Options{BudgetBytes: 1000000, MetadataBudgetBytes: 5000000})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder, author := repositoryID('F'), repositoryID('A')
	if err := db.EnsureFolder(ctx, folder, author, 1); err != nil {
		t.Fatal(err)
	}

	usage, err := db.DetailedStorageUsage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if usage.MetadataBudgetBytes != 5000000 || usage.DataBudgetBytes != 1000000 {
		t.Fatalf("unexpected budgets: metadata=%d data=%d", usage.MetadataBudgetBytes, usage.DataBudgetBytes)
	}
	if usage.StateFilesystem.TotalBytes == 0 || usage.StateFilesystem.FreeBytes == 0 {
		t.Fatalf("filesystem usage empty: %+v", usage.StateFilesystem)
	}
}
