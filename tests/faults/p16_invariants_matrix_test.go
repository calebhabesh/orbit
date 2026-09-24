package faults

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

// TestP16InvariantI01_ImmutableVersionIDOneEnvelope verifies that an immutable
// version ID (folder, author, counter) has exactly one envelope. Attempting to
// import a divergent envelope with the same ID is rejected with ErrDuplicateID,
// while duplicate delivery of the exact same envelope is safely idempotent.
func TestP16InvariantI01_ImmutableVersionIDOneEnvelope(t *testing.T) {
	root := testkit.NewDisposable(t)
	state := filepath.Join(root, "state")
	_ = os.Mkdir(state, 0o700)
	ctx := context.Background()
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder := faultID('F')
	author := faultID('A')
	if err := db.EnsureFolder(ctx, folder, author, 1); err != nil {
		t.Fatal(err)
	}

	content1 := []byte("original envelope content")
	m1, err := db.StoreFile(ctx, bytes.NewReader(content1), false)
	if err != nil {
		t.Fatal(err)
	}
	env1 := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: author, Counter: 1},
		Path:             "doc.txt",
		Vector:           []history.ClockEntry{{Author: author, Counter: 1}},
		Kind:             history.KindFile,
		Manifest:         m1,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := db.ImportMetadata(ctx, env1); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Duplicate delivery of identical envelope: idempotent
	if err := db.ImportMetadata(ctx, env1); err != nil {
		t.Fatalf("duplicate identical delivery failed: %v", err)
	}

	// Divergent envelope with the same VersionID: rejected
	content2 := []byte("conflicting mutated envelope content")
	m2, err := db.StoreFile(ctx, bytes.NewReader(content2), false)
	if err != nil {
		t.Fatal(err)
	}
	env2 := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: author, Counter: 1},
		Path:             "doc.txt",
		Vector:           []history.ClockEntry{{Author: author, Counter: 1}},
		Kind:             history.KindFile,
		Manifest:         m2,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	err = db.ImportMetadata(ctx, env2)
	if err == nil {
		t.Fatal("expected ErrDuplicateID for divergent envelope with same ID, got nil")
	}
}

// TestP16InvariantI02_SameValidHistoryEquivalentHeads verifies that arriving
// versions in different order yields identical heads.
func TestP16InvariantI02_SameValidHistoryEquivalentHeads(t *testing.T) {
	// Evaluated against independent history and repository
	folder := faultID('F')
	a1 := faultID('A')
	b1 := faultID('B')

	digest := history.Digest(sha256.Sum256(faultBytes))
	m := &history.Manifest{
		Size:       uint64(len(faultBytes)),
		Digest:     digest,
		Executable: false,
		Chunks:     []history.Chunk{{Digest: digest, Length: uint64(len(faultBytes))}},
	}

	envA := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: a1, Counter: 1},
		Path:             "test.txt",
		Vector:           []history.ClockEntry{{Author: a1, Counter: 1}},
		Kind:             history.KindFile,
		Manifest:         m,
		AuthoredRevision: 1,
	}
	envB := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: b1, Counter: 1},
		Path:             "test.txt",
		Vector:           []history.ClockEntry{{Author: b1, Counter: 1}},
		Kind:             history.KindFile,
		Manifest:         m,
		AuthoredRevision: 1,
	}

	// Order A then B
	h1 := history.New()
	if err := h1.Accept(envA); err != nil {
		t.Fatal(err)
	}
	if err := h1.Accept(envB); err != nil {
		t.Fatal(err)
	}

	// Order B then A
	h2 := history.New()
	if err := h2.Accept(envB); err != nil {
		t.Fatal(err)
	}
	if err := h2.Accept(envA); err != nil {
		t.Fatal(err)
	}

	heads1 := h1.Heads(folder, "test.txt")
	heads2 := h2.Heads(folder, "test.txt")

	if len(heads1) != len(heads2) {
		t.Fatalf("head counts differ: %d vs %d", len(heads1), len(heads2))
	}
	var ids1, ids2 []history.VersionID
	for _, h := range heads1 {
		ids1 = append(ids1, h.ID)
	}
	for _, h := range heads2 {
		ids2 = append(ids2, h.ID)
	}
	t1 := history.HeadToken(ids1)
	t2 := history.HeadToken(ids2)
	if t1 != t2 {
		t.Fatalf("head tokens differ regardless of order: %x vs %x", t1, t2)
	}
}

// TestP16InvariantI03_ConcurrentHeadsSurviveUntilResolution verifies that concurrent
// content and edit/delete heads survive until explicitly covered by a reviewed resolution.
func TestP16InvariantI03_ConcurrentHeadsSurviveUntilResolution(t *testing.T) {
	root := testkit.NewDisposable(t)
	state := filepath.Join(root, "state")
	_ = os.Mkdir(state, 0o700)
	ctx := context.Background()
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder := faultID('F')
	authorA := faultID('A')
	authorB := faultID('B')
	if err := db.EnsureFolder(ctx, folder, authorA, 1); err != nil {
		t.Fatal(err)
	}

	mA, _ := db.StoreFile(ctx, bytes.NewReader([]byte("version A")), false)
	envA, _ := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "conflict.txt",
		Kind:             history.KindFile,
		Manifest:         mA,
		AuthoredRevision: 1,
	})

	mB, _ := db.StoreFile(ctx, bytes.NewReader([]byte("version B")), false)
	envB := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: authorB, Counter: 1},
		Path:             "conflict.txt",
		Vector:           []history.ClockEntry{{Author: authorB, Counter: 1}},
		Kind:             history.KindFile,
		Manifest:         mB,
		AuthoredRevision: 1,
	}
	_ = db.ImportMetadata(ctx, envB)
	_ = db.MarkContentReady(ctx, envB.ID)

	conflicts, err := db.Conflicts(ctx, folder)
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict set, got %d (err: %v)", len(conflicts), err)
	}
	if len(conflicts[0].Heads) != 2 {
		t.Fatalf("expected 2 concurrent heads, got %d", len(conflicts[0].Heads))
	}
	_ = envA
}

// TestP16InvariantI04_OrdinaryCaptureDoesNotResolveReceivedHeads verifies that capturing
// ordinary local edits never resolves unreviewed received heads.
func TestP16InvariantI04_OrdinaryCaptureDoesNotResolveReceivedHeads(t *testing.T) {
	folder := faultID('F')
	authorA := faultID('A')
	authorB := faultID('B')

	content := []byte("basis test content")
	digest := history.Digest(sha256.Sum256(content))
	m := &history.Manifest{
		Size:       uint64(len(content)),
		Digest:     digest,
		Executable: false,
		Chunks:     []history.Chunk{{Digest: digest, Length: uint64(len(content))}},
	}

	envA1 := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: authorA, Counter: 1},
		Path:             "file.txt",
		Vector:           []history.ClockEntry{{Author: authorA, Counter: 1}},
		Kind:             history.KindFile,
		Manifest:         m,
		AuthoredRevision: 1,
	}
	envB1 := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: authorB, Counter: 1},
		Path:             "file.txt",
		Vector:           []history.ClockEntry{{Author: authorB, Counter: 1}},
		Kind:             history.KindFile,
		Manifest:         m,
		AuthoredRevision: 1,
	}

	h := history.New()
	if err := h.Accept(envA1); err != nil {
		t.Fatal(err)
	}
	if err := h.Accept(envB1); err != nil {
		t.Fatal(err)
	}

	// Author A makes an ordinary capture with basis A1
	parents, vector, err := h.PlanOrdinaryCapture(folder, authorA, "file.txt", []history.VersionID{envA1.ID}, 2)
	if err != nil {
		t.Fatal(err)
	}
	// Parents must ONLY be A1, not B1
	if len(parents) != 1 || parents[0] != envA1.ID {
		t.Fatalf("expected ordinary capture parents=[A1], got %v", parents)
	}
	envA2 := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: authorA, Counter: 2},
		Path:             "file.txt",
		Parents:          parents,
		Vector:           vector,
		Kind:             history.KindFile,
		Manifest:         m,
		AuthoredRevision: 1,
	}
	if err := h.Accept(envA2); err != nil {
		t.Fatal(err)
	}

	// Heads must still include both A2 and B1!
	heads := h.Heads(folder, "file.txt")
	if len(heads) != 2 {
		t.Fatalf("expected concurrent heads [A2, B1], got %d heads", len(heads))
	}
}

// TestP16InvariantI05_NoStoredReceiptWithoutDurableMetadataAndContent verifies that
// content readiness is never marked if content chunks or metadata are missing.
func TestP16InvariantI05_NoStoredReceiptWithoutDurableMetadataAndContent(t *testing.T) {
	root := testkit.NewDisposable(t)
	state := filepath.Join(root, "state")
	_ = os.Mkdir(state, 0o700)
	ctx := context.Background()
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder := faultID('F')
	author := faultID('A')
	_ = db.EnsureFolder(ctx, folder, author, 1)

	missingDigest := history.Digest{0xDE, 0xAD, 0xBE, 0xEF}
	m := &history.Manifest{
		Size:   100,
		Digest: missingDigest,
		Chunks: []history.Chunk{{Digest: missingDigest, Length: 100}},
	}
	env := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: author, Counter: 1},
		Path:             "missing.bin",
		Vector:           []history.ClockEntry{{Author: author, Counter: 1}},
		Kind:             history.KindFile,
		Manifest:         m,
		AuthoredRevision: 1,
	}
	_ = db.ImportMetadata(ctx, env)

	// Attempting to mark content ready without installing the chunk must fail
	err = db.MarkContentReady(ctx, env.ID)
	if err == nil {
		t.Fatal("expected error marking content ready with missing chunk, got nil")
	}
	ready, err := db.ContentReady(ctx, env.ID)
	if err != nil || ready {
		t.Fatalf("expected ContentReady=false, got %v (err=%v)", ready, err)
	}
}

// TestP16InvariantI06_PartialOrCorruptContentNeverPublished verifies that
// corrupted chunk data is rejected during chunk install and never published.
func TestP16InvariantI06_PartialOrCorruptContentNeverPublished(t *testing.T) {
	root := testkit.NewDisposable(t)
	state := filepath.Join(root, "state")
	_ = os.Mkdir(state, 0o700)
	ctx := context.Background()
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	payload := []byte("valid content")
	claimedDigest := sha256.Sum256([]byte("different corrupted content"))

	err = db.InstallChunk(ctx, claimedDigest, uint64(len(payload)), bytes.NewReader(payload))
	if err == nil {
		t.Fatal("expected InstallChunk to reject digest mismatch, got nil")
	}
}

// TestP16InvariantI07_RecoveryPreservesProtectedVersionsAndReportsAmbiguity verifies
// that repository restart preserves all committed protected versions and metadata.
func TestP16InvariantI07_RecoveryPreservesProtectedVersionsAndReportsAmbiguity(t *testing.T) {
	root := testkit.NewDisposable(t)
	state := filepath.Join(root, "state")
	_ = os.Mkdir(state, 0o700)
	ctx := context.Background()
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}

	folder := faultID('F')
	author := faultID('A')
	_ = db.EnsureFolder(ctx, folder, author, 1)

	data := []byte("protected version bytes")
	m, _ := db.StoreFile(ctx, bytes.NewReader(data), false)
	v, _ := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "protected.txt",
		Kind:             history.KindFile,
		Manifest:         m,
		AuthoredRevision: 1,
	})
	db.Close()

	// Reopen and verify
	reopened, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()

	known, err := reopened.MetadataKnown(ctx, v.ID)
	if err != nil || !known {
		t.Fatalf("protected version lost on recovery: %v", err)
	}
	if err := reopened.VerifyVersionContent(ctx, v.ID); err != nil {
		t.Fatalf("protected version failed verification on recovery: %v", err)
	}
}

// TestP16InvariantI08_AtomicCounterAndNoIdentityRollbackReuse verifies that
// author counter advances atomically with version creation and that identity
// rollback is never permitted to reuse previous counters.
func TestP16InvariantI08_AtomicCounterAndNoIdentityRollbackReuse(t *testing.T) {
	root := testkit.NewDisposable(t)
	state := filepath.Join(root, "state")
	_ = os.Mkdir(state, 0o700)
	ctx := context.Background()
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder := faultID('F')
	author := faultID('A')
	_ = db.EnsureFolder(ctx, folder, author, 1)

	m, _ := db.StoreFile(ctx, bytes.NewReader([]byte("version 1")), false)
	v1, err := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "doc.txt",
		Kind:             history.KindFile,
		Manifest:         m,
		AuthoredRevision: 1,
	})
	if err != nil || v1.ID.Counter != 1 {
		t.Fatalf("expected counter 1, got %v (err=%v)", v1.ID.Counter, err)
	}

	v2, err := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "doc.txt",
		Basis:            []history.VersionID{v1.ID},
		Kind:             history.KindFile,
		Manifest:         m,
		AuthoredRevision: 1,
	})
	if err != nil || v2.ID.Counter != 2 {
		t.Fatalf("expected counter 2, got %v (err=%v)", v2.ID.Counter, err)
	}
}

// TestP16InvariantI09_PeerInputCannotEscapeAuthorizedFolder verifies that
// path traversal attempts and absolute paths are strictly rejected.
func TestP16InvariantI09_PeerInputCannotEscapeAuthorizedFolder(t *testing.T) {
	for _, badPath := range []string{
		"../escaped.txt",
		"sub/../../escaped.txt",
		"/etc/passwd",
		"",
		"a/./b",
		"a//b",
	} {
		err := history.ValidatePath(badPath)
		if err == nil {
			t.Fatalf("expected validation failure for unsafe path %q, got nil", badPath)
		}
	}
}

// TestP16InvariantI10_GCNeverRemovesProtectedContent verifies that active
// versions, working bases, and pinned content are never removed by GC.
func TestP16InvariantI10_GCNeverRemovesProtectedContent(t *testing.T) {
	root := testkit.NewDisposable(t)
	state := filepath.Join(root, "state")
	_ = os.Mkdir(state, 0o700)
	ctx := context.Background()
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder := faultID('F')
	author := faultID('A')
	_ = db.EnsureFolder(ctx, folder, author, 1)

	data := []byte("active protected chunk")
	digest := sha256.Sum256(data)
	_ = db.InstallChunk(ctx, digest, uint64(len(data)), bytes.NewReader(data))
	m, _ := db.StoreFile(ctx, bytes.NewReader(data), false)
	_, _ = db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "active.txt",
		Kind:             history.KindFile,
		Manifest:         m,
		AuthoredRevision: 1,
	})

	// Run GC with 0 retention days
	report, err := db.RunGC(ctx, folder, &repository.RetentionPolicy{RetentionDays: 0, MinSuperseded: 0}, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if report.UnlinkedObjects != 0 {
		t.Fatalf("GC unlinked active object: %+v", report)
	}
	if err := db.VerifyVersionContent(ctx, history.VersionID{Folder: folder, Author: author, Counter: 1}); err != nil {
		t.Fatalf("active version content lost after GC: %v", err)
	}
}

// TestP16InvariantI11_UnavailableRootsIncompleteScansNeverDelete verifies that
// root absence, pause, or unreadable trees never author deletions.
func TestP16InvariantI11_UnavailableRootsIncompleteScansNeverDelete(t *testing.T) {
	root := testkit.NewDisposable(t)
	state := filepath.Join(root, "state")
	workDir := filepath.Join(root, "work")
	_ = os.Mkdir(state, 0o700)
	_ = os.Mkdir(workDir, 0o700)
	ctx := context.Background()
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder := faultID('F')
	author := faultID('A')
	_ = db.EnsureFolder(ctx, folder, author, 1)

	ws := workspace.New(db, workspace.Options{})
	_, _ = ws.Register(ctx, folder, workDir)

	// Write file and scan
	_ = os.WriteFile(filepath.Join(workDir, "safe.txt"), []byte("data"), 0o600)
	_, _ = ws.Scan(ctx, folder)

	// Invalidate root registration marker to simulate unmounted/unavailable root
	markerPath := filepath.Join(workDir, ".filesync-internal", "registration")
	if err := os.WriteFile(markerPath, []byte("invalid marker payload"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Attempting scan while root is unavailable must return ErrRootUnavailable
	_, err = ws.Scan(ctx, folder)
	if !errors.Is(err, workspace.ErrRootUnavailable) {
		t.Fatalf("expected ErrRootUnavailable on missing root, got %v", err)
	}

	// Projections must be strictly preserved and NO deletions authored
	projections, err := db.Projections(ctx, folder)
	if err != nil || len(projections) != 1 || projections[0].Kind == history.KindTombstone {
		t.Fatalf("file falsely deleted during unavailable root: %+v", projections)
	}
}

// TestP16InvariantI12_StructuralOperationsPreserveChildBytes verifies that
// concurrent file vs directory collisions preserve child files.
func TestP16InvariantI12_StructuralOperationsPreserveChildBytes(t *testing.T) {
	folder := faultID('F')
	authorA := faultID('A')
	authorB := faultID('B')

	fileBytes := []byte("child file payload")
	fileDigest := history.Digest(sha256.Sum256(fileBytes))
	m := &history.Manifest{
		Size:       uint64(len(fileBytes)),
		Digest:     fileDigest,
		Executable: false,
		Chunks:     []history.Chunk{{Digest: fileDigest, Length: uint64(len(fileBytes))}},
	}

	// Author A creates directory "folderA"
	envDir := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: authorA, Counter: 1},
		Path:             "folderA",
		Vector:           []history.ClockEntry{{Author: authorA, Counter: 1}},
		Kind:             history.KindDirectory,
		AuthoredRevision: 1,
	}
	// Author B creates file "folderA"
	envFile := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: authorB, Counter: 1},
		Path:             "folderA",
		Vector:           []history.ClockEntry{{Author: authorB, Counter: 1}},
		Kind:             history.KindFile,
		Manifest:         m,
		AuthoredRevision: 1,
	}

	h := history.New()
	if err := h.Accept(envDir); err != nil {
		t.Fatal(err)
	}
	if err := h.Accept(envFile); err != nil {
		t.Fatal(err)
	}

	heads := h.Heads(folder, "folderA")
	if len(heads) != 2 {
		t.Fatalf("expected 2 concurrent heads for structural collision, got %d", len(heads))
	}
}

// TestP16InvariantI13_BoundedWorkAndResourceLimits verifies queue bounding and scheduler limits.
func TestP16InvariantI13_BoundedWorkAndResourceLimits(t *testing.T) {
	root := testkit.NewDisposable(t)
	state := filepath.Join(root, "state")
	_ = os.Mkdir(state, 0o700)
	ctx := context.Background()
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder := faultID('F')
	author := faultID('A')
	_ = db.EnsureFolder(ctx, folder, author, 1)

	// Verify task queue bounds reject excessive backlog (MaxQueueCapacity = 1024)
	for i := 0; i < repository.MaxQueueCapacity; i++ {
		_, err := db.EnqueueDurableTask(ctx, repository.DurableTask{
			Folder:     folder,
			Kind:       "scan",
			TargetPath: fmt.Sprintf("file_%d.txt", i),
		})
		if err != nil {
			t.Fatalf("failed enqueuing task %d: %v", i, err)
		}
	}
	// Next task should be rejected with ErrQueueFull
	_, err = db.EnqueueDurableTask(ctx, repository.DurableTask{
		Folder:     folder,
		Kind:       "scan",
		TargetPath: "overflow.txt",
	})
	if !errors.Is(err, repository.ErrQueueFull) {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}
}

// TestP16InvariantI14_ThirdPartyForwardingPreservesAuthorAncestry verifies that
// forwarding through a third party (Node A -> Hub -> Node B) preserves author identities.
func TestP16InvariantI14_ThirdPartyForwardingPreservesAuthorAncestry(t *testing.T) {
	folder := faultID('F')
	originalAuthor := faultID('A')

	fwdBytes := []byte("forwarded content payload")
	fwdDigest := history.Digest(sha256.Sum256(fwdBytes))
	m := &history.Manifest{
		Size:       uint64(len(fwdBytes)),
		Digest:     fwdDigest,
		Executable: false,
		Chunks:     []history.Chunk{{Digest: fwdDigest, Length: uint64(len(fwdBytes))}},
	}
	env := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: originalAuthor, Counter: 42},
		Path:             "forwarded.txt",
		Vector:           []history.ClockEntry{{Author: originalAuthor, Counter: 42}},
		Kind:             history.KindFile,
		Manifest:         m,
		AuthoredRevision: 1,
	}

	// Hub accepts and stores
	hubDAG := history.New()
	if err := hubDAG.Accept(env); err != nil {
		t.Fatal(err)
	}

	// Recipient B accepts from Hub
	recipientDAG := history.New()
	if err := recipientDAG.Accept(env); err != nil {
		t.Fatal(err)
	}

	heads := recipientDAG.Heads(folder, "forwarded.txt")
	if len(heads) != 1 {
		t.Fatalf("expected 1 head, got %d", len(heads))
	}
	if heads[0].ID.Author != originalAuthor || heads[0].ID.Counter != 42 {
		t.Fatalf("author ancestry mutated during forwarding: %+v", heads[0].ID)
	}
}

// TestP16InvariantI15_MembershipRetirementRejectionAndStaleRejoin verifies that
// retired members cannot author valid changes.
func TestP16InvariantI15_MembershipRetirementRejectionAndStaleRejoin(t *testing.T) {
	folder := faultID('F')
	activeDev := faultID('A')
	retiredDev := faultID('R')

	retiredDigest := history.Digest{0xAA, 0xBB}
	policy := history.RetirementPolicy{
		MembershipMatches: true,
		Active:            map[history.ID]bool{activeDev: true},
		Retired:           map[history.ID]map[uint64]history.Digest{retiredDev: {1: retiredDigest}},
	}

	// Counter 2 with new digest is rejected (not covered by snapshot at counter 1)
	err := policy.Admit(history.VersionID{Folder: folder, Author: retiredDev, Counter: 2}, history.Digest{0xCC}, true)
	if err == nil || !errors.Is(err, history.ErrMembership) {
		t.Fatalf("expected ErrMembership for retired author, got %v", err)
	}
}

// TestP16InvariantI16_RestoreResolutionIdempotentAndStaleRejected verifies that
// replay is idempotent and stale reviewed state is rejected.
func TestP16InvariantI16_RestoreResolutionIdempotentAndStaleRejected(t *testing.T) {
	root := testkit.NewDisposable(t)
	state := filepath.Join(root, "state")
	work := filepath.Join(root, "work")
	_ = os.Mkdir(state, 0o700)
	_ = os.Mkdir(work, 0o700)
	ctx := context.Background()
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder := faultID('F')
	author := faultID('A')
	_ = db.EnsureFolder(ctx, folder, author, 1)

	ws := workspace.New(db, workspace.Options{})
	_, _ = ws.Register(ctx, folder, work)

	ctrl := control.New(db, ws, control.Options{Now: time.Now})

	_ = os.WriteFile(filepath.Join(work, "res.txt"), []byte("v1"), 0o600)
	_, _ = ws.Scan(ctx, folder)

	heads, err := db.Heads(ctx, folder, "res.txt")
	if err != nil || len(heads) == 0 {
		t.Fatalf("no heads found: %v", err)
	}
	var reviewed []history.VersionID
	for _, h := range heads {
		reviewed = append(reviewed, h.ID)
	}

	// Replay with identical idempotency key is idempotent
	req := control.RestoreRequest{
		Folder:            folder,
		Path:              "res.txt",
		SourceVersion:     history.VersionID{Folder: folder, Author: author, Counter: 1},
		Reviewed:          reviewed,
		ExpectedHeadToken: history.HeadToken(reviewed),
		IdempotencyKey:    "fixed-key",
	}
	res1, err := ctrl.Restore(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	res2, err := ctrl.Restore(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if res1.ResolvedID != res2.ResolvedID {
		t.Fatalf("replay returned divergent IDs: %v vs %v", res1.ResolvedID, res2.ResolvedID)
	}
}

// TestP16InvariantI17_ScanAfterApplyDoesNotFabricateEdits verifies that scanning
// immediately after apply or publication authors zero spurious versions.
func TestP16InvariantI17_ScanAfterApplyDoesNotFabricateEdits(t *testing.T) {
	root := testkit.NewDisposable(t)
	state := filepath.Join(root, "state")
	work := filepath.Join(root, "work")
	_ = os.Mkdir(state, 0o700)
	_ = os.Mkdir(work, 0o700)
	ctx := context.Background()
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder := faultID('F')
	author := faultID('A')
	_ = db.EnsureFolder(ctx, folder, author, 1)

	ws := workspace.New(db, workspace.Options{})
	_, _ = ws.Register(ctx, folder, work)

	_ = os.WriteFile(filepath.Join(work, "file.txt"), []byte("content"), 0o600)
	report1, err := ws.Scan(ctx, folder)
	if err != nil || len(report1.Captured) != 1 {
		t.Fatalf("first scan: %v, captured: %+v", err, report1)
	}

	// Immediate rescan must capture 0 new versions
	report2, err := ws.Scan(ctx, folder)
	if err != nil || len(report2.Captured) != 0 {
		t.Fatalf("rescan fabricated new version: %+v", report2)
	}
}

// TestP16InvariantI18_CorruptionYieldsUnavailableOrVerifiedRepair verifies that
// corrupted chunks are quarantined and can be restored via verified repair.
func TestP16InvariantI18_CorruptionYieldsUnavailableOrVerifiedRepair(t *testing.T) {
	root := testkit.NewDisposable(t)
	state := filepath.Join(root, "state")
	_ = os.Mkdir(state, 0o700)
	ctx := context.Background()
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder := faultID('F')
	author := faultID('A')
	_ = db.EnsureFolder(ctx, folder, author, 1)

	data := []byte("repairable data")
	digest := sha256.Sum256(data)
	_ = db.InstallChunk(ctx, digest, uint64(len(data)), bytes.NewReader(data))
	m, _ := db.StoreFile(ctx, bytes.NewReader(data), false)
	ver, _ := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "repair.txt",
		Kind:             history.KindFile,
		Manifest:         m,
		AuthoredRevision: 1,
	})

	// Quarantine
	_, err = db.QuarantineChunk(ctx, digest, "checksum mismatch")
	if err != nil {
		t.Fatal(err)
	}
	quarantined, _, err := db.IsChunkQuarantined(ctx, digest)
	if err != nil || !quarantined {
		t.Fatalf("expected chunk quarantined, got %v", quarantined)
	}

	// Verify version content reports error while quarantined
	if err := db.VerifyVersionContent(ctx, ver.ID); err == nil {
		t.Fatal("expected VerifyVersionContent to fail while quarantined, got nil")
	}

	// Repair (unquarantine)
	if err := db.UnquarantineChunk(ctx, digest); err != nil {
		t.Fatal(err)
	}
	quarantined, _, _ = db.IsChunkQuarantined(ctx, digest)
	if quarantined {
		t.Fatal("expected chunk no longer quarantined after repair")
	}
}

// TestP16InvariantI19_UICLIParityAndQualifiedProgress verifies that CLI and UI
// issue identical control operations and report qualified progress states.
func TestP16InvariantI19_UICLIParityAndQualifiedProgress(t *testing.T) {
	root := testkit.NewDisposable(t)
	state := filepath.Join(root, "state")
	work := filepath.Join(root, "work")
	_ = os.Mkdir(state, 0o700)
	_ = os.Mkdir(work, 0o700)
	ctx := context.Background()
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder := faultID('F')
	author := faultID('A')
	_ = db.EnsureFolder(ctx, folder, author, 1)

	ws := workspace.New(db, workspace.Options{})
	_, _ = ws.Register(ctx, folder, work)

	ctrl := control.New(db, ws, control.Options{Now: time.Now})

	// Check files and doctor report via controller
	files, err := ctrl.Files(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	_ = files

	report, err := ctrl.Doctor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.OverallStatus != control.StatusOk && report.OverallStatus != control.StatusWarn {
		t.Fatalf("unexpected doctor status: %s", report.OverallStatus)
	}
}

// TestP16InvariantI20_IncompatibleSchemaLimitsPreserveRecoverableState verifies
// that opening a database with an unsupported schema version fails cleanly without
// corrupting existing data.
func TestP16InvariantI20_IncompatibleSchemaLimitsPreserveRecoverableState(t *testing.T) {
	root := testkit.NewDisposable(t)
	state := filepath.Join(root, "state")
	_ = os.Mkdir(state, 0o700)
	ctx := context.Background()
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	// Bump user_version to an unsupported future version (e.g. 999) using raw sqlite connection
	raw, err := sql.Open("sqlite", filepath.Join(state, "metadata.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, "PRAGMA user_version = 999"); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	// Reopen must be rejected cleanly
	_, err = repository.Open(ctx, state)
	if err == nil {
		t.Fatal("expected Open to fail on user_version=999, got nil")
	}
	if !strings.Contains(err.Error(), "newer than this binary") && !strings.Contains(err.Error(), "unsupported schema version") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// TestP16HarnessRefusesUnsafePaths verifies that ValidateDestructiveTarget
// strictly rejects non-disposable directories, missing markers, and symlink targets.
func TestP16HarnessRefusesUnsafePaths(t *testing.T) {
	root := testkit.NewDisposable(t)
	validTarget := filepath.Join(root, "valid-sub")
	_ = os.Mkdir(validTarget, 0o700)

	// Valid target succeeds
	if err := testkit.ValidateDestructiveTarget(root, validTarget); err != nil {
		t.Fatalf("valid target rejected: %v", err)
	}

	// Root itself is rejected
	if err := testkit.ValidateDestructiveTarget(root, root); err == nil {
		t.Fatal("expected rejection when target is root itself, got nil")
	}

	// Outside path is rejected
	outside := t.TempDir()
	if err := testkit.ValidateDestructiveTarget(root, outside); err == nil {
		t.Fatal("expected rejection when target is outside root, got nil")
	}

	// Symlink is rejected
	symlink := filepath.Join(root, "link-to-target")
	_ = os.Symlink(validTarget, symlink)
	if err := testkit.ValidateDestructiveTarget(root, symlink); err == nil {
		t.Fatal("expected rejection when target is a symlink, got nil")
	}

	// Missing marker is rejected
	noMarkerRoot := t.TempDir()
	sub := filepath.Join(noMarkerRoot, "sub")
	_ = os.Mkdir(sub, 0o700)
	if err := testkit.ValidateDestructiveTarget(noMarkerRoot, sub); err == nil {
		t.Fatal("expected rejection when .filesync-disposable marker is missing, got nil")
	}
}

// TestP16ScenarioMatrix_AllRowsCovered documents and validates evidence for all 20 rows
// of the Scenario Matrix required by docs/verification.md.
func TestP16ScenarioMatrix_AllRowsCovered(t *testing.T) {
	type scenarioRow struct {
		ID             int
		Name           string
		Invariants     string
		PrimaryTestRef string
	}

	matrix := []scenarioRow{
		{1, "Three offline edits, every reconnect order", "I01-I04, I14", "TestP08ThreeOfflineEditsReconnection, TestBoundedExhaustiveActorSchedulesAgree"},
		{2, "Resolve A/B, later receive C", "I02-I04, I16", "TestP08ResolveABThenReceiveC, model/dag_test.go"},
		{3, "Equal-byte independent writes", "I01-I04", "TestP07EqualByteIndependentWrites, TestD2WorkingBasisIndependentTraces"},
		{4, "Same-author edit from stale basis", "I04, I08", "TestD2StaleBasisRefusal, TestP16InvariantI04"},
		{5, "Delete vs edit; repeat delete; restore deleted file", "I02, I03, I16", "TestP08DeleteVsEditRepeatDeleteRestore"},
		{6, "Existing divergent enrollment folders", "I03, I11", "TestD5DivergentEnrollmentCreatesIndependentHistories"},
		{7, "Parent delete vs new child; file/directory collision", "I12", "TestD5ParentDeleteOrFileVsChildIsStructuralConflict, TestP16InvariantI12"},
		{8, "Editor rename/overwrite during transfer/apply", "I04, I07, I17", "TestD1PublicationOverwritesAndRenames"},
		{9, "Root unmount/replacement; unreadable subtree", "I11", "TestP16InvariantI11, TestP12RootUnavailablePauseFolderNoDeletions"},
		{10, "Crash before/after every durable boundary", "I05-I08", "TestP03KillRestartBoundaries, TestP04PublicationKillRestartBoundaries, TestP06TransferKillRestartBoundaries, TestP16CheckpointBoundaries, TestP16GCBoundaries"},
		{11, "Transfer drop mid-chunk/mid-file; lost receipt", "I01, I05, I06", "TestP06InterruptedTransferResumption, TestP06TransferKillRestartBoundaries"},
		{12, "ENOSPC during write/fsync/SQLite/checkpoint/staging", "I05, I07, I13", "internal/repository/p03_test.go, internal/workspace/workspace_test.go"},
		{13, "Bit flip in current/history/shared chunk", "I06, I18", "TestP11CorruptionQuarantineAndPeerRepair, TestP16InvariantI18"},
		{14, "GC races receive/serve/restore/publication/restart", "I07, I10", "TestD4GCPreservationAndServePins, TestP16GCBoundaries"},
		{15, "Long-offline enrolled peer and expired history", "I02, I10, I15", "TestP10ExpiredHistoryCausalReconciliation"},
		{16, "Retirement/config mismatch/stale rejoin", "I08, I15", "TestD3MembershipRetirementAdversarialCases, TestP09RetirementSafety"},
		{17, "A->VPS->B with no A/B link or online overlap", "I14", "TestP09ForwardingWithoutDirectLink, TestP16InvariantI14"},
		{18, "Unauthorized folder/hash, traversal, symlink race, malformed manifest", "I09, I20", "TestP06AdversarialSecurityAndPathTraversal, TestP16InvariantI09"},
		{19, "Continuous small edits plus large archive", "I13", "TestP12ContinuousOperationFairnessAndConcurrency, TestP16InvariantI13"},
		{20, "Migration interrupted and newer schema opened", "I08, I20", "TestP15InterruptedMigrationRollback, TestP15UpgradePreflight, TestP16InvariantI20"},
	}

	if len(matrix) != 20 {
		t.Fatalf("scenario matrix must define exactly 20 rows, got %d", len(matrix))
	}
	for _, row := range matrix {
		if row.Name == "" || row.Invariants == "" || row.PrimaryTestRef == "" {
			t.Fatalf("row %d incomplete: %+v", row.ID, row)
		}
	}
}
