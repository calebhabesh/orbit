package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/replication"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/workspace"
)

type testEnv struct {
	db       *repository.DB
	ws       *workspace.Workspace
	ctrl     *Controller
	folder   history.ID
	authorA  history.ID
	authorB  history.ID
	stateDir string
	rootDir  string
}

func setupTestEnv(t *testing.T, opts ...Options) *testEnv {
	t.Helper()
	stateDir := t.TempDir()
	rootDir := t.TempDir()

	db, err := repository.Open(context.Background(), stateDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	ws := workspace.New(db, workspace.Options{})

	folder := testID('F')
	authorA := testID('A')
	authorB := testID('B')

	if err := db.EnsureFolder(context.Background(), folder, authorA, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Register(context.Background(), folder, rootDir); err != nil {
		t.Fatal(err)
	}

	ctrl := New(db, ws, opts...)
	return &testEnv{
		db:       db,
		ws:       ws,
		ctrl:     ctrl,
		folder:   folder,
		authorA:  authorA,
		authorB:  authorB,
		stateDir: stateDir,
		rootDir:  rootDir,
	}
}

func testID(b byte) history.ID {
	var id history.ID
	id[0] = b
	return id
}

func testDigest(b byte) history.Digest {
	var d history.Digest
	for i := range d {
		d[i] = b
	}
	return d
}

func fileManifest(content []byte, exec bool) *history.Manifest {
	h := sha256.Sum256(content)
	return &history.Manifest{
		Size:       uint64(len(content)),
		Digest:     history.Digest(h),
		Executable: exec,
		Chunks: []history.Chunk{{
			Digest: history.Digest(h),
			Length: uint64(len(content)),
		}},
	}
}

func createConflict(t *testing.T, env *testEnv, path string, contentA, contentB []byte, execA, execB bool) (history.Envelope, history.Envelope) {
	t.Helper()
	ctx := context.Background()

	// Install chunks for content A
	mA := fileManifest(contentA, execA)
	if err := env.db.InstallChunk(ctx, mA.Chunks[0].Digest, mA.Chunks[0].Length, bytes.NewReader(contentA)); err != nil {
		t.Fatal(err)
	}

	// Create local version A1
	envA, err := env.db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           env.folder,
		Path:             path,
		Basis:            nil,
		Kind:             history.KindFile,
		Manifest:         mA,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}

	mode := os.FileMode(0o600)
	if execA {
		mode = 0o755
	}
	if err := os.WriteFile(filepath.Join(env.rootDir, path), contentA, mode); err != nil {
		t.Fatal(err)
	}

	// Install chunks for content B
	mB := fileManifest(contentB, execB)
	if err := env.db.InstallChunk(ctx, mB.Chunks[0].Digest, mB.Chunks[0].Length, bytes.NewReader(contentB)); err != nil {
		t.Fatal(err)
	}

	// Import remote version B1
	envB := history.Envelope{
		ID:               history.VersionID{Folder: env.folder, Author: env.authorB, Counter: 1},
		Path:             path,
		Parents:          nil,
		Vector:           []history.ClockEntry{{Author: env.authorB, Counter: 1}},
		Kind:             history.KindFile,
		Manifest:         mB,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := env.db.ImportMetadata(ctx, envB); err != nil {
		t.Fatal(err)
	}
	if err := env.db.MarkContentReady(ctx, envB.ID); err != nil {
		t.Fatal(err)
	}

	return envA, envB
}

func TestResolveSelect(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	envA, envB := createConflict(t, env, "doc.txt", []byte("version A"), []byte("version B"), false, false)
	reviewed := []history.VersionID{envA.ID, envB.ID}
	token := history.HeadToken(reviewed)

	res, err := env.ctrl.ResolveSelect(ctx, ResolveSelectRequest{
		Folder:            env.folder,
		Path:              "doc.txt",
		Reviewed:          reviewed,
		ExpectedHeadToken: token,
		Selected:          envA.ID,
		IdempotencyKey:    "sel-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	if res.ResolvedID.Author != env.authorA {
		t.Fatalf("author = %v, want local author %v", res.ResolvedID.Author, env.authorA)
	}
	if len(res.Envelope.Parents) != 2 {
		t.Fatalf("parents = %d, want 2", len(res.Envelope.Parents))
	}
	if !res.Applied {
		t.Fatal("expected applied = true")
	}

	content, err := os.ReadFile(filepath.Join(env.rootDir, "doc.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "version A" {
		t.Fatalf("workspace content = %q, want %q", content, "version A")
	}

	// Verify no conflicts remain
	conflicts, _, err := env.ctrl.Conflicts(ctx, env.folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("remaining conflicts = %d, want 0", len(conflicts))
	}
}

func TestResolveSelectResponseLossAndReplay(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	envA, envB := createConflict(t, env, "doc.txt", []byte("version A"), []byte("version B"), false, false)
	reviewed := []history.VersionID{envA.ID, envB.ID}
	token := history.HeadToken(reviewed)

	req := ResolveSelectRequest{
		Folder:            env.folder,
		Path:              "doc.txt",
		Reviewed:          reviewed,
		ExpectedHeadToken: token,
		Selected:          envB.ID,
		IdempotencyKey:    "replay-key-1",
	}

	res1, err := env.ctrl.ResolveSelect(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if res1.Replay {
		t.Fatal("first call should not be replay")
	}

	// Simulate response loss: client retries exact same request
	res2, err := env.ctrl.ResolveSelect(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Replay {
		t.Fatal("second call should be marked replay")
	}
	if res2.ResolvedID != res1.ResolvedID {
		t.Fatalf("resolved ID changed on replay: %v vs %v", res2.ResolvedID, res1.ResolvedID)
	}

	// Verify only 1 resolution version exists in history
	hist, err := env.ctrl.History(ctx, env.folder, "doc.txt")
	if err != nil {
		t.Fatal(err)
	}
	// Total versions: envA, envB, plus 1 resolution version = 3
	if len(hist) != 3 {
		t.Fatalf("total history versions = %d, want 3", len(hist))
	}
}

func TestResolveSelectReusedKeyWithChangedArguments(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	envA, envB := createConflict(t, env, "doc.txt", []byte("version A"), []byte("version B"), false, false)
	reviewed := []history.VersionID{envA.ID, envB.ID}
	token := history.HeadToken(reviewed)

	// First call selects envA
	_, err := env.ctrl.ResolveSelect(ctx, ResolveSelectRequest{
		Folder:            env.folder,
		Path:              "doc.txt",
		Reviewed:          reviewed,
		ExpectedHeadToken: token,
		Selected:          envA.ID,
		IdempotencyKey:    "conflict-key",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Second call reuses same key but selects envB
	_, err = env.ctrl.ResolveSelect(ctx, ResolveSelectRequest{
		Folder:            env.folder,
		Path:              "doc.txt",
		Reviewed:          reviewed,
		ExpectedHeadToken: token,
		Selected:          envB.ID,
		IdempotencyKey:    "conflict-key",
	})
	if err == nil {
		t.Fatal("expected error on reused key with different arguments")
	}
	var ctrlErr *ControlError
	if !errors.As(err, &ctrlErr) || ctrlErr.Code != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("expected IDEMPOTENCY_CONFLICT, got: %v", err)
	}
}

func TestResolveSelectStaleViewPreCommit(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	envA, envB := createConflict(t, env, "doc.txt", []byte("version A"), []byte("version B"), false, false)
	reviewed := []history.VersionID{envA.ID, envB.ID}
	staleToken := history.Digest{0xff, 0xfe}

	// Send with stale token
	_, err := env.ctrl.ResolveSelect(ctx, ResolveSelectRequest{
		Folder:            env.folder,
		Path:              "doc.txt",
		Reviewed:          reviewed,
		ExpectedHeadToken: staleToken,
		Selected:          envA.ID,
		IdempotencyKey:    "stale-key",
	})
	if err == nil {
		t.Fatal("expected error with stale token")
	}
	var ctrlErr *ControlError
	if !errors.As(err, &ctrlErr) || ctrlErr.Code != "STALE_VIEW" {
		t.Fatalf("expected STALE_VIEW error, got: %v", err)
	}
	if len(ctrlErr.Heads) != 2 || ctrlErr.HeadToken == nil {
		t.Fatalf("stale view error missing heads or token: %v", ctrlErr)
	}
}

func TestResolveSelectUnseenVersionRemainsConcurrent(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	envA, envB := createConflict(t, env, "doc.txt", []byte("version A"), []byte("version B"), false, false)
	reviewed := []history.VersionID{envA.ID, envB.ID}
	token := history.HeadToken(reviewed)

	// Resolve [A1, B1] -> A2
	res, err := env.ctrl.ResolveSelect(ctx, ResolveSelectRequest{
		Folder:            env.folder,
		Path:              "doc.txt",
		Reviewed:          reviewed,
		ExpectedHeadToken: token,
		Selected:          envA.ID,
		IdempotencyKey:    "unseen-res",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Concurrently, third-party author C authored C1 before seeing the resolution
	authorC := testID('C')
	mC := fileManifest([]byte("version C"), false)
	if err := env.db.InstallChunk(ctx, mC.Chunks[0].Digest, mC.Chunks[0].Length, bytes.NewReader([]byte("version C"))); err != nil {
		t.Fatal(err)
	}
	envC := history.Envelope{
		ID:               history.VersionID{Folder: env.folder, Author: authorC, Counter: 1},
		Path:             "doc.txt",
		Parents:          nil,
		Vector:           []history.ClockEntry{{Author: authorC, Counter: 1}},
		Kind:             history.KindFile,
		Manifest:         mC,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := env.db.ImportMetadata(ctx, envC); err != nil {
		t.Fatal(err)
	}
	if err := env.db.MarkContentReady(ctx, envC.ID); err != nil {
		t.Fatal(err)
	}

	// Verify that unseen version C1 and resolution A2 remain concurrent heads!
	conflicts, _, err := env.ctrl.Conflicts(ctx, env.folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("conflicts count = %d, want 1", len(conflicts))
	}
	heads := conflicts[0].Heads
	if len(heads) != 2 {
		t.Fatalf("conflict heads = %d, want 2 (A2 and C1)", len(heads))
	}

	foundA2 := false
	foundC1 := false
	for _, h := range heads {
		if h.ID == res.ResolvedID {
			foundA2 = true
		}
		if h.ID == envC.ID {
			foundC1 = true
		}
	}
	if !foundA2 || !foundC1 {
		t.Fatalf("heads did not contain A2 and C1: %v", heads)
	}
}

func TestResolveManualMerge(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	envA, envB := createConflict(t, env, "doc.txt", []byte("line from A\n"), []byte("line from B\n"), false, false)
	reviewed := []history.VersionID{envA.ID, envB.ID}
	token := history.HeadToken(reviewed)

	merged := []byte("line from A\nline from B\n")
	res, err := env.ctrl.ResolveManualMerge(ctx, ResolveMergeRequest{
		Folder:            env.folder,
		Path:              "doc.txt",
		Reviewed:          reviewed,
		ExpectedHeadToken: token,
		Executable:        true,
		Content:           merged,
		IdempotencyKey:    "merge-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Envelope.Parents) != 2 {
		t.Fatalf("parents = %d, want 2", len(res.Envelope.Parents))
	}
	if !res.Envelope.Manifest.Executable {
		t.Fatal("manifest executable = false, want true")
	}

	content, err := os.ReadFile(filepath.Join(env.rootDir, "doc.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(merged) {
		t.Fatalf("workspace content = %q, want %q", content, merged)
	}

	fi, err := os.Stat(filepath.Join(env.rootDir, "doc.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&0o111 == 0 {
		t.Fatal("expected file to have executable bits set")
	}
}

func TestRestoreHistoricalVersion(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// 1. Capture v1
	contentV1 := []byte("first historical version")
	if err := os.WriteFile(filepath.Join(env.rootDir, "file.txt"), contentV1, 0o600); err != nil {
		t.Fatal(err)
	}
	scan1, err := env.ws.Scan(ctx, env.folder)
	if err != nil || len(scan1.Captured) != 1 {
		t.Fatalf("scan1 failed: %v", err)
	}
	v1 := scan1.Captured[0]

	// 2. Edit to v2
	contentV2 := []byte("second edited version")
	if err := os.WriteFile(filepath.Join(env.rootDir, "file.txt"), contentV2, 0o600); err != nil {
		t.Fatal(err)
	}
	scan2, err := env.ws.Scan(ctx, env.folder)
	if err != nil || len(scan2.Captured) != 1 {
		t.Fatalf("scan2 failed: %v", err)
	}
	v2 := scan2.Captured[0]
	_ = v2

	// 3. Delete file -> v3 (tombstone)
	if err := os.Remove(filepath.Join(env.rootDir, "file.txt")); err != nil {
		t.Fatal(err)
	}
	scan3, err := env.ws.Scan(ctx, env.folder)
	if err != nil {
		t.Fatalf("scan3 failed: %v", err)
	}
	var v3 history.Envelope
	if scan3.Deletion != nil {
		tombstones, err := env.ws.ApproveDeletions(ctx, env.folder, scan3.Deletion.Token)
		if err != nil || len(tombstones) != 1 {
			t.Fatalf("approve deletions failed: %v", err)
		}
		v3 = tombstones[0]
	} else if len(scan3.Captured) == 1 && scan3.Captured[0].Kind == history.KindTombstone {
		v3 = scan3.Captured[0]
	} else {
		t.Fatalf("scan3 did not capture tombstone: %+v", scan3)
	}

	// Preview restore
	preview, err := env.ctrl.PreviewRestore(ctx, RestorePreviewRequest{
		Folder:        env.folder,
		Path:          "file.txt",
		SourceVersion: v1.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.ContentState != repository.ContentReady {
		t.Fatalf("preview content state = %v, want ready", preview.ContentState)
	}
	if len(preview.CurrentHeads) != 1 || preview.CurrentHeads[0] != v3.ID {
		t.Fatalf("preview current heads = %v, want tombstone %v", preview.CurrentHeads, v3.ID)
	}

	// Execute restore of v1
	res, err := env.ctrl.Restore(ctx, RestoreRequest{
		Folder:            env.folder,
		Path:              "file.txt",
		SourceVersion:     v1.ID,
		Reviewed:          preview.CurrentHeads,
		ExpectedHeadToken: preview.ExpectedHeadToken,
		IdempotencyKey:    "restore-op-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify restored version has v3 (tombstone) as parent
	if len(res.Envelope.Parents) != 1 || res.Envelope.Parents[0] != v3.ID {
		t.Fatalf("restored version parents = %v, want [tombstone %v]", res.Envelope.Parents, v3.ID)
	}

	// Verify file is back on disk with v1 content
	got, err := os.ReadFile(filepath.Join(env.rootDir, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(contentV1) {
		t.Fatalf("restored disk content = %q, want %q", got, contentV1)
	}
}

func TestRestoreExecutableStatusFollowsSelectedVersion(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// v1: Executable = true
	contentV1 := []byte("#!/bin/sh\necho hello\n")
	if err := os.WriteFile(filepath.Join(env.rootDir, "script.sh"), contentV1, 0o755); err != nil {
		t.Fatal(err)
	}
	scan1, err := env.ws.Scan(ctx, env.folder)
	if err != nil || len(scan1.Captured) != 1 {
		t.Fatalf("scan1 failed: %v", err)
	}
	v1 := scan1.Captured[0]
	if !v1.Manifest.Executable {
		t.Fatal("v1 manifest should be executable")
	}

	// v2: Executable = false
	if err := os.Chmod(filepath.Join(env.rootDir, "script.sh"), 0o644); err != nil {
		t.Fatal(err)
	}
	scan2, err := env.ws.Scan(ctx, env.folder)
	if err != nil || len(scan2.Captured) != 1 {
		t.Fatalf("scan2 failed: %v", err)
	}
	v2 := scan2.Captured[0]
	if v2.Manifest.Executable {
		t.Fatal("v2 manifest should not be executable")
	}

	// Restore v1
	res, err := env.ctrl.Restore(ctx, RestoreRequest{
		Folder:            env.folder,
		Path:              "script.sh",
		SourceVersion:     v1.ID,
		Reviewed:          []history.VersionID{v2.ID},
		ExpectedHeadToken: history.HeadToken([]history.VersionID{v2.ID}),
		IdempotencyKey:    "restore-exec-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Envelope.Manifest.Executable {
		t.Fatal("restored version manifest should be executable")
	}

	fi, err := os.Stat(filepath.Join(env.rootDir, "script.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&0o111 == 0 {
		t.Fatal("restored disk file should have executable permission bits")
	}
}

func TestRestoreExpiredPayloadReturnsContentExpired(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	content := []byte("historical payload to expire")
	if err := os.WriteFile(filepath.Join(env.rootDir, "data.bin"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	scan1, err := env.ws.Scan(ctx, env.folder)
	if err != nil || len(scan1.Captured) != 1 {
		t.Fatalf("scan1 failed: %v", err)
	}
	v1 := scan1.Captured[0]

	// Create v2 so v1 is historical
	if err := os.WriteFile(filepath.Join(env.rootDir, "data.bin"), []byte("v2 data"), 0o600); err != nil {
		t.Fatal(err)
	}
	scan2, err := env.ws.Scan(ctx, env.folder)
	if err != nil || len(scan2.Captured) != 1 {
		t.Fatalf("scan2 failed: %v", err)
	}
	v2 := scan2.Captured[0]

	// Expire v1 content
	if err := env.db.ExpireContent(ctx, v1.ID); err != nil {
		t.Fatal(err)
	}

	// Verify ContentAvailability reports expired
	avail, err := env.db.ContentAvailability(ctx, v1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if avail != repository.ContentExpired {
		t.Fatalf("availability = %s, want expired", avail)
	}

	// Restore of expired payload must return CONTENT_EXPIRED
	_, err = env.ctrl.Restore(ctx, RestoreRequest{
		Folder:            env.folder,
		Path:              "data.bin",
		SourceVersion:     v1.ID,
		Reviewed:          []history.VersionID{v2.ID},
		ExpectedHeadToken: history.HeadToken([]history.VersionID{v2.ID}),
		IdempotencyKey:    "exp-1",
	})
	if err == nil {
		t.Fatal("expected error on restore of expired content")
	}
	var ctrlErr *ControlError
	if !errors.As(err, &ctrlErr) || ctrlErr.Code != "CONTENT_EXPIRED" {
		t.Fatalf("expected CONTENT_EXPIRED error, got: %v", err)
	}
}

func TestKeepCopiesDestinationCollisionPreservesFiles(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	envA, envB := createConflict(t, env, "doc.txt", []byte("version A"), []byte("version B"), false, false)
	reviewed := []history.VersionID{envA.ID, envB.ID}
	token := history.HeadToken(reviewed)

	// Create an existing file on disk at the intended destination path
	collisionPath := filepath.Join(env.rootDir, "existing.txt")
	existingContent := []byte("do not overwrite me!")
	if err := os.WriteFile(collisionPath, existingContent, 0o600); err != nil {
		t.Fatal(err)
	}

	// Attempt keep-copies specifying the colliding path
	_, err := env.ctrl.ResolveKeepCopies(ctx, KeepCopiesRequest{
		Folder:            env.folder,
		Path:              "doc.txt",
		Reviewed:          reviewed,
		ExpectedHeadToken: token,
		Copies: []CopyTarget{
			{HeadID: envA.ID, DestinationPath: "doc_a.txt"},
			{HeadID: envB.ID, DestinationPath: "existing.txt"},
		},
		IdempotencyKey: "coll-key",
	})
	if err == nil {
		t.Fatal("expected error on destination collision")
	}
	var ctrlErr *ControlError
	if !errors.As(err, &ctrlErr) || ctrlErr.Code != "DESTINATION_COLLISION" {
		t.Fatalf("expected DESTINATION_COLLISION error, got: %v", err)
	}

	// Verify existing file is preserved completely intact
	got, err := os.ReadFile(collisionPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(existingContent) {
		t.Fatalf("existing file modified: got %q, want %q", got, existingContent)
	}
}

func TestKeepCopiesPartialResumptionNoDuplicateCopies(t *testing.T) {
	stepCount := 0
	opts := Options{
		FaultHook: func(name string) error {
			if name == "control.keep_copies.step" {
				stepCount++
				if stepCount == 1 {
					// Simulate crash right after first copy step
					return errors.New("injected crash after step 1")
				}
			}
			return nil
		},
	}
	env := setupTestEnv(t, opts)
	ctx := context.Background()

	envA, envB := createConflict(t, env, "doc.txt", []byte("version A"), []byte("version B"), false, false)
	reviewed := []history.VersionID{envA.ID, envB.ID}
	token := history.HeadToken(reviewed)

	req := KeepCopiesRequest{
		Folder:            env.folder,
		Path:              "doc.txt",
		Reviewed:          reviewed,
		ExpectedHeadToken: token,
		Copies: []CopyTarget{
			{HeadID: envA.ID, DestinationPath: "copy_a.txt"},
			{HeadID: envB.ID, DestinationPath: "copy_b.txt"},
		},
		IdempotencyKey: "partial-kc-key",
	}

	// First attempt: fails at step 1 fault hook
	_, err := env.ctrl.ResolveKeepCopies(ctx, req)
	if err == nil {
		t.Fatal("expected fault failure")
	}

	// Verify copy_a.txt was created
	copyAHist, err := env.ctrl.History(ctx, env.folder, "copy_a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(copyAHist) != 1 {
		t.Fatalf("copy_a.txt versions = %d, want 1", len(copyAHist))
	}
	copyAVersionID := copyAHist[0].ID

	// Second attempt: disable fault hook, resume with same idempotency key
	env.ctrl.options.FaultHook = nil
	res, err := env.ctrl.ResolveKeepCopies(ctx, req)
	if err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if !res.Completed {
		t.Fatal("expected completed = true")
	}

	// Verify copy_a.txt still has EXACTLY 1 version (no duplicate version minted!)
	copyAHistAfter, err := env.ctrl.History(ctx, env.folder, "copy_a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(copyAHistAfter) != 1 {
		t.Fatalf("copy_a.txt versions after resume = %d, want 1 (duplicate copy detected!)", len(copyAHistAfter))
	}
	if copyAHistAfter[0].ID != copyAVersionID {
		t.Fatalf("copy_a version changed: %v vs %v", copyAHistAfter[0].ID, copyAVersionID)
	}

	// Verify copy_b.txt exists
	copyBHist, err := env.ctrl.History(ctx, env.folder, "copy_b.txt")
	if err != nil || len(copyBHist) != 1 {
		t.Fatalf("copy_b.txt versions = %d, want 1", len(copyBHist))
	}

	// Verify both copies exist on disk
	contentA, err := os.ReadFile(filepath.Join(env.rootDir, "copy_a.txt"))
	if err != nil || string(contentA) != "version A" {
		t.Fatalf("copy_a content = %q, want 'version A'", contentA)
	}
	contentB, err := os.ReadFile(filepath.Join(env.rootDir, "copy_b.txt"))
	if err != nil || string(contentB) != "version B" {
		t.Fatalf("copy_b content = %q, want 'version B'", contentB)
	}
}

func TestExport(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	content := []byte("payload to export byte for byte")
	m := fileManifest(content, false)
	if err := env.db.InstallChunk(ctx, m.Chunks[0].Digest, m.Chunks[0].Length, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	v, err := env.db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           env.folder,
		Path:             "export.txt",
		Basis:            nil,
		Kind:             history.KindFile,
		Manifest:         m,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := env.ctrl.Export(ctx, env.folder, v.ID, &buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), content) {
		t.Fatalf("exported bytes = %q, want %q", buf.Bytes(), content)
	}
}

func TestHistoryAvailability(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	content := []byte("history file")
	m := fileManifest(content, false)
	if err := env.db.InstallChunk(ctx, m.Chunks[0].Digest, m.Chunks[0].Length, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	v1, err := env.db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           env.folder,
		Path:             "history.txt",
		Basis:            nil,
		Kind:             history.KindFile,
		Manifest:         m,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}

	items, err := env.ctrl.History(ctx, env.folder, "history.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("history items = %d, want 1", len(items))
	}
	if items[0].ID != v1.ID || items[0].ContentState != repository.ContentReady || !items[0].IsHead {
		t.Fatalf("unexpected history item: %+v", items[0])
	}
}

func TestMembershipExportImportPreview(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Setup initial membership (rev 1) with authorA and authorB
	m1 := protocol.Membership{
		Folder:   env.folder,
		Revision: 1,
		Active: []protocol.ActiveMember{
			{Device: env.authorA, KeyPin: testDigest('A')},
			{Device: env.authorB, KeyPin: testDigest('B')},
		},
	}
	app1, err := env.db.ApproveMembership(ctx, m1)
	if err != nil {
		t.Fatal(err)
	}

	// Export membership
	exportRes, err := env.ctrl.MembershipExport(ctx, env.folder)
	if err != nil {
		t.Fatal(err)
	}
	if exportRes.Membership.Revision != 1 || exportRes.Digest != app1.Digest {
		t.Fatalf("export mismatch: rev=%d digest=%x", exportRes.Membership.Revision, exportRes.Digest)
	}

	// Preview next membership (rev 2) adding authorC
	authorC := testID('C')
	m2 := protocol.Membership{
		Folder:      env.folder,
		Revision:    2,
		PriorDigest: app1.Digest,
		Active: []protocol.ActiveMember{
			{Device: env.authorA, KeyPin: testDigest('A')},
			{Device: env.authorB, KeyPin: testDigest('B')},
			{Device: authorC, KeyPin: testDigest('C')},
		},
	}
	preview, err := env.ctrl.MembershipPreview(ctx, env.folder, m2)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.ValidTransition || !preview.PriorDigestMatches {
		t.Fatalf("preview should be valid: %+v", preview)
	}
	if len(preview.AddedActive) != 1 || preview.AddedActive[0] != authorC {
		t.Fatalf("expected authorC added, got: %v", preview.AddedActive)
	}

	// Import without approve
	impNoApp, err := env.ctrl.MembershipImport(ctx, env.folder, m2, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if impNoApp.Revision != 2 {
		t.Fatalf("expected revision 2, got %d", impNoApp.Revision)
	}

	// Still rev 1 in db
	curM, _, err := env.db.GetMembership(ctx, env.folder)
	if err != nil {
		t.Fatal(err)
	}
	if curM.Revision != 1 {
		t.Fatalf("expected revision 1 still in db, got %d", curM.Revision)
	}

	// Import with approve
	app2, err := env.ctrl.MembershipImport(ctx, env.folder, m2, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if app2.Revision != 2 {
		t.Fatalf("expected revision 2, got %d", app2.Revision)
	}

	// PeerList
	peers, err := env.ctrl.PeerList(ctx, env.folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(peers.Active) != 3 {
		t.Fatalf("expected 3 active peers, got %d", len(peers.Active))
	}
}

func TestRetireMemberPreviewAndExecute(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Initial membership rev 1 with authorA and authorB
	m1 := protocol.Membership{
		Folder:   env.folder,
		Revision: 1,
		Active: []protocol.ActiveMember{
			{Device: env.authorA, KeyPin: testDigest('A')},
			{Device: env.authorB, KeyPin: testDigest('B')},
		},
	}
	if _, err := env.db.ApproveMembership(ctx, m1); err != nil {
		t.Fatal(err)
	}

	// Author a version from authorB
	bContent := []byte("authored by B")
	bManifest := fileManifest(bContent, false)
	if err := env.db.InstallChunk(ctx, bManifest.Chunks[0].Digest, bManifest.Chunks[0].Length, bytes.NewReader(bContent)); err != nil {
		t.Fatal(err)
	}
	bEnv := history.Envelope{
		ID:               history.VersionID{Folder: env.folder, Author: env.authorB, Counter: 1},
		Path:             "b_file.txt",
		Vector:           []history.ClockEntry{{Author: env.authorB, Counter: 1}},
		Kind:             history.KindFile,
		Manifest:         bManifest,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := env.db.ImportMetadata(ctx, bEnv); err != nil {
		t.Fatal(err)
	}

	// Preview retirement of authorB
	prev, err := env.ctrl.RetireMemberPreview(ctx, env.folder, env.authorB)
	if err != nil {
		t.Fatal(err)
	}
	if prev.CurrentRevision != 1 || prev.NextRevision != 2 {
		t.Fatalf("unexpected revision preview: %+v", prev)
	}
	if prev.AcceptedVersionsCount != 1 {
		t.Fatalf("expected 1 accepted version, got %d", prev.AcceptedVersionsCount)
	}
	if len(prev.SurvivingMembers) != 1 || prev.SurvivingMembers[0] != env.authorA {
		t.Fatalf("expected surviving member authorA, got: %v", prev.SurvivingMembers)
	}

	// Execute retirement with idempotency key
	key := "retire-key-test"
	req := RetireMemberRequest{
		Folder:         env.folder,
		TargetDevice:   env.authorB,
		IdempotencyKey: key,
	}
	res1, err := env.ctrl.RetireMemberExecute(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if res1.ApprovedRevision != 2 || res1.Replay {
		t.Fatalf("unexpected res1: %+v", res1)
	}
	if len(res1.NextMembership.Active) != 1 || res1.NextMembership.Active[0].Device != env.authorA {
		t.Fatalf("expected active authorA: %+v", res1.NextMembership.Active)
	}
	if len(res1.NextMembership.Retired) != 1 || res1.NextMembership.Retired[0].Device != env.authorB {
		t.Fatalf("expected retired authorB: %+v", res1.NextMembership.Retired)
	}

	// Replay execution with same key
	res2, err := env.ctrl.RetireMemberExecute(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Replay {
		t.Fatalf("expected res2 to be replay: %+v", res2)
	}
	if res2.ApprovedRevision != res1.ApprovedRevision || res2.ApprovedDigest != res1.ApprovedDigest {
		t.Fatalf("replay result differed: res1=%+v, res2=%+v", res1, res2)
	}

	// Check PeerList
	peers, err := env.ctrl.PeerList(ctx, env.folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(peers.Active) != 1 || peers.Active[0].Device != env.authorA {
		t.Fatalf("peer list active mismatch: %+v", peers.Active)
	}
	if len(peers.Retired) != 1 || peers.Retired[0].Device != env.authorB {
		t.Fatalf("peer list retired mismatch: %+v", peers.Retired)
	}
}

func TestEnrollPreviewAndBootstrap(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Initial membership rev 1 with authorA
	m1 := protocol.Membership{
		Folder:   env.folder,
		Revision: 1,
		Active: []protocol.ActiveMember{
			{Device: env.authorA, KeyPin: testDigest('A')},
		},
	}
	if _, err := env.db.ApproveMembership(ctx, m1); err != nil {
		t.Fatal(err)
	}

	// Create files on disk in rootDir
	file1Path := filepath.Join(env.rootDir, "hello.txt")
	if err := os.WriteFile(file1Path, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}
	file2Path := filepath.Join(env.rootDir, "conflict.txt")
	if err := os.WriteFile(file2Path, []byte("disk content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Repo has projection for conflict.txt with different content
	diffContent := []byte("repo content")
	diffManifest := fileManifest(diffContent, false)
	if err := env.db.InstallChunk(ctx, diffManifest.Chunks[0].Digest, diffManifest.Chunks[0].Length, bytes.NewReader(diffContent)); err != nil {
		t.Fatal(err)
	}
	diffEnv, err := env.db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           env.folder,
		Path:             "conflict.txt",
		Kind:             history.KindFile,
		Manifest:         diffManifest,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = diffEnv

	// EnrollPreview
	preview, err := env.ctrl.EnrollPreview(ctx, env.folder, env.rootDir)
	if err != nil {
		t.Fatal(err)
	}
	if preview.LocalFilesCount != 2 {
		t.Fatalf("expected 2 local files, got %d", preview.LocalFilesCount)
	}
	foundConflict := false
	for _, p := range preview.ConflictingPaths {
		if p == "conflict.txt" {
			foundConflict = true
		}
	}
	if !foundConflict {
		t.Fatalf("expected conflict.txt in conflicting paths, got: %v", preview.ConflictingPaths)
	}

	// EnrollBootstrap
	boot, err := env.ctrl.EnrollBootstrap(ctx, env.folder, env.rootDir)
	if err != nil {
		t.Fatal(err)
	}
	if boot.CapturedCount == 0 {
		t.Fatalf("expected captured count > 0, got %d", boot.CapturedCount)
	}
}

func TestStorageControlOperations(t *testing.T) {
	ctx := context.Background()
	env := setupTestEnv(t)

	// 1. StorageUsage
	usageRes, err := env.ctrl.StorageUsage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if usageRes.Usage.StateFilesystem.TotalBytes == 0 {
		t.Fatal("expected non-zero state filesystem total bytes")
	}

	// 2. RetentionPreview with default policy
	prevRes, err := env.ctrl.RetentionPreview(ctx, RetentionPreviewRequest{Folder: env.folder})
	if err != nil {
		t.Fatal(err)
	}
	if prevRes.Preview.FolderID != env.folder {
		t.Fatalf("preview folder ID mismatch: %v", prevRes.Preview.FolderID)
	}

	// 3. RetentionChange
	changeRes, err := env.ctrl.RetentionChange(ctx, RetentionChangeRequest{
		Folder:        env.folder,
		RetentionDays: 14,
		MinSuperseded: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if changeRes.Policy.RetentionDays != 14 || changeRes.Policy.MinSuperseded != 10 {
		t.Fatalf("unexpected policy after change: %+v", changeRes.Policy)
	}

	// 4. GCPreview with custom policy
	zero := 0
	gcPrevRes, err := env.ctrl.GCPreview(ctx, GCPreviewRequest{
		Folder:        env.folder,
		RetentionDays: &zero,
		MinSuperseded: &zero,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = gcPrevRes

	// 5. GCRun with idempotency key
	key := "gc-test-key-1"
	gcRunRes1, err := env.ctrl.GCRun(ctx, GCRunRequest{
		Folder:         env.folder,
		RetentionDays:  &zero,
		MinSuperseded:  &zero,
		IdempotencyKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gcRunRes1.Replay {
		t.Fatal("first run should not be replay")
	}

	// Replay GCRun
	gcRunRes2, err := env.ctrl.GCRun(ctx, GCRunRequest{
		Folder:         env.folder,
		RetentionDays:  &zero,
		MinSuperseded:  &zero,
		IdempotencyKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !gcRunRes2.Replay {
		t.Fatal("second run should be replay")
	}

	// 6. ReclaimRecoveryCopies
	reclaimRes, err := env.ctrl.ReclaimRecoveryCopies(ctx, ReclaimRecoveryRequest{Folder: env.folder})
	if err != nil {
		t.Fatal(err)
	}
	if reclaimRes.Folder != env.folder {
		t.Fatalf("reclaim folder mismatch: %v", reclaimRes.Folder)
	}
}

type mockRepairClient struct {
	replication.PeerClient
	data map[history.Digest][]byte
}

func (m *mockRepairClient) Chunk(ctx context.Context, req replication.ChunkRequest, exp history.Chunk) ([]byte, error) {
	if b, ok := m.data[exp.Digest]; ok {
		return b, nil
	}
	return nil, errors.New("chunk not found on mock peer")
}

func TestStorageCheckAndRepairControlOperations(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	// Approve membership with authorA (local) and authorB (peer)
	m1 := protocol.Membership{
		Folder:   env.folder,
		Revision: 1,
		Active: []protocol.ActiveMember{
			{Device: env.authorA, KeyPin: testDigest('A')},
			{Device: env.authorB, KeyPin: testDigest('B')},
		},
	}
	if _, err := env.db.ApproveMembership(ctx, m1); err != nil {
		t.Fatal(err)
	}

	content := []byte("clean content for integrity and repair testing")
	digest := sha256.Sum256(content)
	if err := os.WriteFile(filepath.Join(env.rootDir, "test.txt"), content, 0o600); err != nil {
		t.Fatal(err)
	}

	scanRes, err := env.ws.Scan(ctx, env.folder)
	if err != nil || len(scanRes.Captured) != 1 {
		t.Fatalf("scan: %+v, %v", scanRes, err)
	}
	v1 := scanRes.Captured[0]

	// 1. StorageIntegrityCheck on clean state
	check1, err := env.ctrl.StorageIntegrityCheck(ctx, StorageCheckRequest{Folder: env.folder})
	if err != nil {
		t.Fatal(err)
	}
	if check1.TotalChunksChecked != 1 || check1.CleanChunks != 1 || len(check1.CorruptChunks) != 0 {
		t.Fatalf("unexpected clean check result: %+v", check1)
	}

	// 2. Inject corruption into the chunk file on disk
	chunkPath := filepath.Join(env.stateDir, "objects", "sha256", fmt.Sprintf("%02x", digest[0]), fmt.Sprintf("%x", digest[1:]))
	if err := os.WriteFile(chunkPath, []byte("bad corrupt bytes!"), 0o600); err != nil {
		t.Fatal(err)
	}

	// 3. StorageIntegrityCheck with AutoQuarantine
	check2, err := env.ctrl.StorageIntegrityCheck(ctx, StorageCheckRequest{Folder: env.folder, AutoQuarantine: true})
	if err != nil {
		t.Fatal(err)
	}
	if check2.TotalChunksChecked != 1 || len(check2.CorruptChunks) != 1 {
		t.Fatalf("expected 1 corrupt chunk, got %+v", check2)
	}
	if check2.CorruptChunks[0].Digest != digest {
		t.Fatalf("expected corrupt chunk digest %x, got %x", digest, check2.CorruptChunks[0].Digest)
	}
	if len(check2.CorruptChunks[0].AffectedVersions) != 1 || check2.CorruptChunks[0].AffectedVersions[0] != v1.ID {
		t.Fatalf("expected affected version %v, got %v", v1.ID, check2.CorruptChunks[0].AffectedVersions)
	}

	// Verify version availability is now corrupt
	avail, err := env.db.ContentAvailability(ctx, v1.ID)
	if err != nil || avail != repository.ContentCorrupt {
		t.Fatalf("expected ContentCorrupt, got %v (%v)", avail, err)
	}

	// 4. StorageRepair using mock peer
	mockPeer := &mockRepairClient{
		data: map[history.Digest][]byte{digest: content},
	}
	repairPeers := []replication.RepairPeer{
		{DeviceID: env.authorB, Client: mockPeer},
	}

	key := "repair-test-key-1"
	repairRes1, err := env.ctrl.StorageRepair(ctx, StorageRepairRequest{
		Folder:         env.folder,
		VersionID:      v1.ID,
		Peers:          repairPeers,
		IdempotencyKey: key,
	})
	if err != nil {
		t.Fatalf("StorageRepair: %v", err)
	}
	if repairRes1.Status != "repaired" || repairRes1.RepairedChunks != 1 || repairRes1.Replay {
		t.Fatalf("unexpected repair result: %+v", repairRes1)
	}

	// 5. Replay with idempotency key
	repairRes2, err := env.ctrl.StorageRepair(ctx, StorageRepairRequest{
		Folder:         env.folder,
		VersionID:      v1.ID,
		Peers:          repairPeers,
		IdempotencyKey: key,
	})
	if err != nil {
		t.Fatalf("StorageRepair replay: %v", err)
	}
	if !repairRes2.Replay || repairRes2.RepairedChunks != 1 {
		t.Fatalf("expected replay, got %+v", repairRes2)
	}

	// 6. Verify restored availability and chunk content
	availRestored, err := env.db.ContentAvailability(ctx, v1.ID)
	if err != nil || availRestored != repository.ContentReady {
		t.Fatalf("expected ContentReady, got %v (%v)", availRestored, err)
	}

	restoredData, err := os.ReadFile(chunkPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restoredData, content) {
		t.Fatalf("restored data mismatch: got %q, want %q", restoredData, content)
	}

	// Final check should be clean
	check3, err := env.ctrl.StorageIntegrityCheck(ctx, StorageCheckRequest{Folder: env.folder})
	if err != nil {
		t.Fatal(err)
	}
	if check3.CleanChunks != 1 || len(check3.CorruptChunks) != 0 {
		t.Fatalf("expected clean check, got %+v", check3)
	}
}
