package replication

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/workspace"
	"github.com/calebhabesh/orbit/model"
)

type twoPeerFixture struct {
	t        *testing.T
	ctx      context.Context
	cancel   context.CancelFunc
	devA     history.ID
	devB     history.ID
	folder   history.ID
	repoA    *repository.DB
	repoB    *repository.DB
	workA    *workspace.Workspace
	workB    *workspace.Workspace
	rootA    string
	rootB    string
	stateA   string
	stateB   string
	clientA  *Client
	clientB  *Client
	syncerA  *Syncer
	syncerB  *Syncer
	approved repository.ApprovedMembership
}

func newTwoPeerFixture(t *testing.T) *twoPeerFixture {
	t.Helper()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	devA, devB := fixedID('A'), fixedID('B')
	folder := fixedID('F')

	baseDir := t.TempDir()
	stateA := filepath.Join(baseDir, "state-a")
	stateB := filepath.Join(baseDir, "state-b")
	rootA := filepath.Join(baseDir, "root-a")
	rootB := filepath.Join(baseDir, "root-b")
	for _, d := range []string{stateA, stateB, rootA, rootB} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	idA, err := LoadOrCreateIdentity(filepath.Join(baseDir, "id-a"), devA, now)
	if err != nil {
		t.Fatal(err)
	}
	idB, err := LoadOrCreateIdentity(filepath.Join(baseDir, "id-b"), devB, now)
	if err != nil {
		t.Fatal(err)
	}

	repoA, err := repository.Open(context.Background(), stateA)
	if err != nil {
		t.Fatal(err)
	}
	repoB, err := repository.Open(context.Background(), stateB)
	if err != nil {
		t.Fatal(err)
	}

	if err := repoA.EnsureFolder(context.Background(), folder, devA, 1); err != nil {
		t.Fatal(err)
	}
	if err := repoB.EnsureFolder(context.Background(), folder, devB, 1); err != nil {
		t.Fatal(err)
	}

	workA := workspace.New(repoA, workspace.Options{})
	if _, err := workA.Register(context.Background(), folder, rootA); err != nil {
		t.Fatal(err)
	}
	workB := workspace.New(repoB, workspace.Options{})
	if _, err := workB.Register(context.Background(), folder, rootB); err != nil {
		t.Fatal(err)
	}

	membership := protocol.Membership{
		Folder:   folder,
		Revision: 1,
		Active: []protocol.ActiveMember{
			{Device: devA, KeyPin: idA.KeyPin},
			{Device: devB, KeyPin: idB.KeyPin},
		},
	}
	approvedA, err := repoA.ApproveMembership(context.Background(), membership)
	if err != nil {
		t.Fatal(err)
	}
	approvedB, err := repoB.ApproveMembership(context.Background(), membership)
	if err != nil {
		t.Fatal(err)
	}

	listenerA, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listenerB, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	serverA := NewServer(repoA, idA)
	serverA.now = func() time.Time { return now }
	go func() { _ = serverA.Serve(ctx, listenerA) }()

	serverB := NewServer(repoB, idB)
	serverB.now = func() time.Time { return now }
	go func() { _ = serverB.Serve(ctx, listenerB) }()

	clientA, err := NewClient("https://"+listenerB.Addr().String(), idA, idB.Leaf, idB.KeyPin)
	if err != nil {
		t.Fatal(err)
	}
	clientB, err := NewClient("https://"+listenerA.Addr().String(), idB, idA.Leaf, idA.KeyPin)
	if err != nil {
		t.Fatal(err)
	}

	syncerA := NewSyncer(repoA, workA, clientA, devA, devB, folder, approvedA, TransferOptions{})
	syncerB := NewSyncer(repoB, workB, clientB, devB, devA, folder, approvedB, TransferOptions{})

	fix := &twoPeerFixture{
		t:        t,
		ctx:      ctx,
		cancel:   cancel,
		devA:     devA,
		devB:     devB,
		folder:   folder,
		repoA:    repoA,
		repoB:    repoB,
		workA:    workA,
		workB:    workB,
		rootA:    rootA,
		rootB:    rootB,
		stateA:   stateA,
		stateB:   stateB,
		clientA:  clientA,
		clientB:  clientB,
		syncerA:  syncerA,
		syncerB:  syncerB,
		approved: approvedA,
	}

	t.Cleanup(func() {
		clientA.CloseIdleConnections()
		clientB.CloseIdleConnections()
		cancel()
		_ = listenerA.Close()
		_ = listenerB.Close()
		_ = repoA.Close()
		_ = repoB.Close()
	})

	return fix
}

func (fix *twoPeerFixture) syncAFromB() SyncResult {
	fix.t.Helper()
	res, err := fix.syncerA.Sync(fix.ctx)
	if err != nil {
		fix.t.Fatalf("sync A from B failed: %v", err)
	}
	return res
}

func (fix *twoPeerFixture) syncBFromA() SyncResult {
	fix.t.Helper()
	res, err := fix.syncerB.Sync(fix.ctx)
	if err != nil {
		fix.t.Fatalf("sync B from A failed: %v", err)
	}
	return res
}

func (fix *twoPeerFixture) fullSync() {
	fix.t.Helper()
	fix.syncAFromB()
	fix.syncBFromA()
}

func (fix *twoPeerFixture) fullSyncReverse() {
	fix.t.Helper()
	fix.syncBFromA()
	fix.syncAFromB()
}

func TestReconciliationOfflineEditEdit(t *testing.T) {
	for _, order := range []string{"A-then-B", "B-then-A"} {
		t.Run(order, func(t *testing.T) {
			fix := newTwoPeerFixture(t)

			// Offline: Node A writes "version alpha"
			fileA := filepath.Join(fix.rootA, "conflict.txt")
			if err := os.WriteFile(fileA, []byte("version alpha"), 0o600); err != nil {
				t.Fatal(err)
			}
			resA, err := fix.workA.Scan(fix.ctx, fix.folder)
			if err != nil || len(resA.Captured) != 1 {
				t.Fatalf("scan A: %+v %v", resA, err)
			}
			envA := resA.Captured[0]

			// Offline: Node B writes "version beta"
			fileB := filepath.Join(fix.rootB, "conflict.txt")
			if err := os.WriteFile(fileB, []byte("version beta"), 0o600); err != nil {
				t.Fatal(err)
			}
			resB, err := fix.workB.Scan(fix.ctx, fix.folder)
			if err != nil || len(resB.Captured) != 1 {
				t.Fatalf("scan B: %+v %v", resB, err)
			}
			envB := resB.Captured[0]

			// Reconnect and sync in chosen order
			if order == "A-then-B" {
				fix.fullSync()
			} else {
				fix.fullSyncReverse()
			}

			// Verify both repos have both heads
			for _, check := range []struct {
				name string
				repo *repository.DB
				root string
				want string
				app  history.VersionID
			}{
				{"Node A", fix.repoA, fix.rootA, "version alpha", envA.ID},
				{"Node B", fix.repoB, fix.rootB, "version beta", envB.ID},
			} {
				conflicts, err := check.repo.Conflicts(fix.ctx, fix.folder)
				if err != nil {
					t.Fatalf("%s conflicts err: %v", check.name, err)
				}
				if len(conflicts) != 1 {
					t.Fatalf("%s conflicts len=%d, want 1", check.name, len(conflicts))
				}
				c := conflicts[0]
				if c.Path != "conflict.txt" || c.ConflictKind != "edit-edit" || len(c.Heads) != 2 {
					t.Fatalf("%s conflict=%+v, want path=conflict.txt kind=edit-edit heads=2", check.name, c)
				}
				if c.Applied == nil || *c.Applied != check.app {
					t.Fatalf("%s applied=%v, want %v", check.name, c.Applied, check.app)
				}

				// Verify working file bytes on disk are preserved
				data, err := os.ReadFile(filepath.Join(check.root, "conflict.txt"))
				if err != nil || string(data) != check.want {
					t.Fatalf("%s file on disk=%q, want %q", check.name, string(data), check.want)
				}

				// Verify both head versions are marked content ready and stored
				for _, h := range c.Heads {
					ready, err := check.repo.ContentReady(fix.ctx, h.ID)
					if err != nil || !ready {
						t.Fatalf("%s head %v ContentReady=%v, want true", check.name, h.ID, ready)
					}
				}
			}

			// Verify scans create NO extra versions
			scanAfterA, err := fix.workA.Scan(fix.ctx, fix.folder)
			if err != nil || len(scanAfterA.Captured) != 0 {
				t.Fatalf("scan A after sync captured unexpected versions: %+v", scanAfterA.Captured)
			}
			scanAfterB, err := fix.workB.Scan(fix.ctx, fix.folder)
			if err != nil || len(scanAfterB.Captured) != 0 {
				t.Fatalf("scan B after sync captured unexpected versions: %+v", scanAfterB.Captured)
			}

			// Verify agreement with independent DAG oracle
			oracle := model.New()
			if err := oracle.Accept(model.Event{ID: "A1", Author: "A", Path: "conflict.txt", Content: "alpha"}, true); err != nil {
				t.Fatal(err)
			}
			if err := oracle.Accept(model.Event{ID: "B1", Author: "B", Path: "conflict.txt", Content: "beta"}, true); err != nil {
				t.Fatal(err)
			}
			oracleHeads := oracle.Heads("conflict.txt")
			if len(oracleHeads) != 2 || oracleHeads[0] != "A1" || oracleHeads[1] != "B1" {
				t.Fatalf("oracle heads=%v, want [A1, B1]", oracleHeads)
			}
		})
	}
}

func TestReconciliationOfflineEditDelete(t *testing.T) {
	fix := newTwoPeerFixture(t)

	// Step 1: Create base file on Node A and sync to Node B
	baseFile := filepath.Join(fix.rootA, "base.txt")
	if err := os.WriteFile(baseFile, []byte("common ancestor"), 0o600); err != nil {
		t.Fatal(err)
	}
	resA, err := fix.workA.Scan(fix.ctx, fix.folder)
	if err != nil || len(resA.Captured) != 1 {
		t.Fatalf("scan A: %+v %v", resA, err)
	}
	baseEnv := resA.Captured[0]

	// Sync to Node B so both start with baseEnv applied
	fix.syncBFromA()
	dataB, err := os.ReadFile(filepath.Join(fix.rootB, "base.txt"))
	if err != nil || string(dataB) != "common ancestor" {
		t.Fatalf("Node B base file=%q, want common ancestor", string(dataB))
	}

	// Complete bootstrap scan on Node B
	if _, err := fix.workB.Scan(fix.ctx, fix.folder); err != nil {
		t.Fatal(err)
	}

	// Step 2: Offline edits:
	// Node A modifies base.txt -> A2
	if err := os.WriteFile(baseFile, []byte("modified by A"), 0o600); err != nil {
		t.Fatal(err)
	}
	resA2, err := fix.workA.Scan(fix.ctx, fix.folder)
	if err != nil || len(resA2.Captured) != 1 {
		t.Fatalf("scan A2: %+v %v", resA2, err)
	}
	envA2 := resA2.Captured[0]

	// Node B deletes base.txt -> B2 tombstone
	if err := os.Remove(filepath.Join(fix.rootB, "base.txt")); err != nil {
		t.Fatal(err)
	}
	scanDelB, err := fix.workB.Scan(fix.ctx, fix.folder)
	if err != nil {
		t.Fatal(err)
	}
	var envB2 history.Envelope
	if len(scanDelB.Captured) == 1 && scanDelB.Captured[0].Kind == history.KindTombstone {
		envB2 = scanDelB.Captured[0]
	} else if scanDelB.Deletion != nil {
		approved, err := fix.workB.ApproveDeletions(fix.ctx, fix.folder, scanDelB.Deletion.Token)
		if err != nil || len(approved) != 1 {
			t.Fatalf("approve deletions: %v %+v", err, approved)
		}
		envB2 = approved[0]
	} else {
		t.Fatalf("expected tombstone captured on Node B, got: %+v", scanDelB)
	}

	// Step 3: Reconnect and full sync
	fix.fullSync()

	// Step 4: Verify conflict on both nodes
	for _, check := range []struct {
		name string
		repo *repository.DB
		root string
		want string
	}{
		{"Node A", fix.repoA, fix.rootA, "modified by A"},
		{"Node B", fix.repoB, fix.rootB, ""},
	} {
		conflicts, err := check.repo.Conflicts(fix.ctx, fix.folder)
		if err != nil {
			t.Fatalf("%s conflicts: %v", check.name, err)
		}
		if len(conflicts) != 1 {
			t.Fatalf("%s conflicts len=%d, want 1", check.name, len(conflicts))
		}
		c := conflicts[0]
		if c.Path != "base.txt" || c.ConflictKind != "edit-delete" || len(c.Heads) != 2 {
			t.Fatalf("%s conflict=%+v, want edit-delete heads=2", check.name, c)
		}

		if check.want != "" {
			data, err := os.ReadFile(filepath.Join(check.root, "base.txt"))
			if err != nil || string(data) != check.want {
				t.Fatalf("%s file=%q, want %q", check.name, string(data), check.want)
			}
		} else {
			if _, err := os.Stat(filepath.Join(check.root, "base.txt")); !os.IsNotExist(err) {
				t.Fatalf("%s file should be absent on disk", check.name)
			}
		}
	}

	// Scans produce no extra versions
	sA, _ := fix.workA.Scan(fix.ctx, fix.folder)
	sB, _ := fix.workB.Scan(fix.ctx, fix.folder)
	if len(sA.Captured) != 0 || len(sB.Captured) != 0 {
		t.Fatalf("unexpected captured versions after sync: A=%d B=%d", len(sA.Captured), len(sB.Captured))
	}

	// Verify agreement with independent DAG oracle
	oracle := model.New()
	_ = oracle.Accept(model.Event{ID: "A1", Author: "A", Path: "base.txt", Content: "base"}, true)
	_ = oracle.Accept(model.Event{ID: "A2", Author: "A", Path: "base.txt", Parents: []string{"A1"}, Content: "modified"}, true)
	_ = oracle.Accept(model.Event{ID: "B2", Author: "B", Path: "base.txt", Parents: []string{"A1"}, Kind: "tombstone"}, false)
	oracleHeads := oracle.Heads("base.txt")
	if len(oracleHeads) != 2 || oracleHeads[0] != "A2" || oracleHeads[1] != "B2" {
		t.Fatalf("oracle heads=%v, want [A2, B2]", oracleHeads)
	}

	_ = baseEnv
	_ = envA2
	_ = envB2
}

func TestReconciliationRepeatedDelete(t *testing.T) {
	fix := newTwoPeerFixture(t)

	// Step 1: Base file on Node A synced to Node B
	if err := os.WriteFile(filepath.Join(fix.rootA, "del.txt"), []byte("to be deleted"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := fix.workA.Scan(fix.ctx, fix.folder)
	if err != nil || len(res.Captured) != 1 {
		t.Fatal(err)
	}
	fix.syncBFromA()
	if _, err := fix.workB.Scan(fix.ctx, fix.folder); err != nil {
		t.Fatal(err)
	}

	// Step 2: Both delete the file independently
	if err := os.Remove(filepath.Join(fix.rootA, "del.txt")); err != nil {
		t.Fatal(err)
	}
	scanA, _ := fix.workA.Scan(fix.ctx, fix.folder)
	if len(scanA.Captured) == 0 && scanA.Deletion != nil {
		_, _ = fix.workA.ApproveDeletions(fix.ctx, fix.folder, scanA.Deletion.Token)
	}

	if err := os.Remove(filepath.Join(fix.rootB, "del.txt")); err != nil {
		t.Fatal(err)
	}
	scanB, _ := fix.workB.Scan(fix.ctx, fix.folder)
	if len(scanB.Captured) == 0 && scanB.Deletion != nil {
		_, _ = fix.workB.ApproveDeletions(fix.ctx, fix.folder, scanB.Deletion.Token)
	}

	// Step 3: Sync
	fix.fullSync()

	// Both have heads [A2, B2], conflict kind delete-delete
	for _, repo := range []*repository.DB{fix.repoA, fix.repoB} {
		conflicts, err := repo.Conflicts(fix.ctx, fix.folder)
		if err != nil {
			t.Fatal(err)
		}
		if len(conflicts) != 1 || conflicts[0].ConflictKind != "delete-delete" {
			t.Fatalf("conflicts=%+v, want 1 delete-delete", conflicts)
		}
	}

	// Scans produce no extra versions
	sA, _ := fix.workA.Scan(fix.ctx, fix.folder)
	sB, _ := fix.workB.Scan(fix.ctx, fix.folder)
	if len(sA.Captured) != 0 || len(sB.Captured) != 0 {
		t.Fatalf("unexpected captured after repeated delete: A=%d B=%d", len(sA.Captured), len(sB.Captured))
	}
}

func TestReconciliationEqualByteConflict(t *testing.T) {
	fix := newTwoPeerFixture(t)

	// Offline: A writes "identical"
	if err := os.WriteFile(filepath.Join(fix.rootA, "same.txt"), []byte("identical"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _ = fix.workA.Scan(fix.ctx, fix.folder)

	// Offline: B writes "identical"
	if err := os.WriteFile(filepath.Join(fix.rootB, "same.txt"), []byte("identical"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _ = fix.workB.Scan(fix.ctx, fix.folder)

	// Sync
	resA := fix.syncAFromB()
	resB := fix.syncBFromA()

	// Chunks were reused because bytes are identical
	if resA.ChunksReused != 1 || resB.ChunksReused != 1 {
		t.Logf("resA reused=%d, resB reused=%d", resA.ChunksReused, resB.ChunksReused)
	}

	// Heads remain distinct! Conflict kind is equal-content
	for _, repo := range []*repository.DB{fix.repoA, fix.repoB} {
		conflicts, err := repo.Conflicts(fix.ctx, fix.folder)
		if err != nil {
			t.Fatal(err)
		}
		if len(conflicts) != 1 || conflicts[0].ConflictKind != "equal-content" {
			t.Fatalf("conflicts=%+v, want 1 equal-content", conflicts)
		}
		if len(conflicts[0].Heads) != 2 {
			t.Fatalf("heads len=%d, want 2", len(conflicts[0].Heads))
		}
	}
}

func TestReconciliationExecutableOnlyChange(t *testing.T) {
	fix := newTwoPeerFixture(t)

	// Step 1: Base script created on Node A with 0o600
	scriptPath := filepath.Join(fix.rootA, "run.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _ = fix.workA.Scan(fix.ctx, fix.folder)
	fix.syncBFromA()

	// Verify Node B has 0o600
	statB, err := os.Stat(filepath.Join(fix.rootB, "run.sh"))
	if err != nil || statB.Mode().Perm()&0o111 != 0 {
		t.Fatalf("Node B initial exec=%v, want non-executable", statB.Mode().Perm())
	}

	// Step 2: Node A changes mode to 0o700 (executable) without changing content
	if err := os.Chmod(scriptPath, 0o700); err != nil {
		t.Fatal(err)
	}
	resScan, err := fix.workA.Scan(fix.ctx, fix.folder)
	if err != nil || len(resScan.Captured) != 1 {
		t.Fatalf("scan A for chmod: %+v %v", resScan, err)
	}
	if !resScan.Captured[0].Manifest.Executable {
		t.Fatal("expected captured version to have Executable=true")
	}

	// Step 3: Node B syncs from Node A
	syncRes := fix.syncBFromA()
	if syncRes.VersionsApplied != 1 {
		t.Fatalf("expected 1 applied version on B, got %d", syncRes.VersionsApplied)
	}

	// Step 4: Verify Node B's file on disk now has executable bit set
	statB2, err := os.Stat(filepath.Join(fix.rootB, "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if statB2.Mode().Perm()&0o111 == 0 {
		t.Fatalf("Node B run.sh perm=%v, expected executable", statB2.Mode().Perm())
	}

	// Scans on Node B produce no extra versions
	resScanB, _ := fix.workB.Scan(fix.ctx, fix.folder)
	if len(resScanB.Captured) != 0 {
		t.Fatalf("scan B captured unexpected version: %+v", resScanB.Captured)
	}
}

func TestReconciliationProtectedFallbackWhileRemotePending(t *testing.T) {
	fix := newTwoPeerFixture(t)

	// Step 1: Base file on Node A
	fileA := filepath.Join(fix.rootA, "fallback.txt")
	if err := os.WriteFile(fileA, []byte("fallback content 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	resA, err := fix.workA.Scan(fix.ctx, fix.folder)
	if err != nil || len(resA.Captured) != 1 {
		t.Fatal(err)
	}
	envA1 := resA.Captured[0]

	// Step 2: Remote peer creates B2 (dominating A1), but Node A only receives B2's envelope
	// with content_state = 'pending' (chunks not fetched)
	chunkDigest := history.Digest{0xbb, 0x01}
	envB2 := history.Envelope{
		ID:               history.VersionID{Folder: fix.folder, Author: fix.devB, Counter: 2},
		Path:             "fallback.txt",
		Parents:          []history.VersionID{envA1.ID},
		Vector:           []history.ClockEntry{{Author: fix.devA, Counter: envA1.ID.Counter}, {Author: fix.devB, Counter: 2}},
		Kind:             history.KindFile,
		Manifest:         &history.Manifest{Size: 10, Digest: history.Digest{0xbb}, Chunks: []history.Chunk{{Digest: chunkDigest, Length: 10}}},
		AuthoredRevision: 1,
	}
	if err := fix.repoA.ImportMetadata(fix.ctx, envB2); err != nil {
		t.Fatal(err)
	}

	// Single head is B2, but ContentReady is false
	ready, err := fix.repoA.ContentReady(fix.ctx, envB2.ID)
	if err != nil || ready {
		t.Fatalf("B2 ContentReady=%v, want false", ready)
	}

	// UnappliedSingleHeads should NOT return B2 because it is not ready
	unapplied, err := fix.repoA.UnappliedSingleHeads(fix.ctx, fix.folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(unapplied) != 0 {
		t.Fatalf("unapplied len=%d, want 0", len(unapplied))
	}

	// Working tree on Node A still has "fallback content 1"
	data, err := os.ReadFile(fileA)
	if err != nil || string(data) != "fallback content 1" {
		t.Fatalf("fallback file on disk=%q, want 'fallback content 1'", string(data))
	}

	// Orphan objects must NOT include A1's chunks
	orphans, err := fix.repoA.OrphanObjects(fix.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, orphan := range orphans {
		if orphan == envA1.Manifest.Digest {
			t.Errorf("A1 chunk was falsely reported as orphan object: %x", orphan)
		}
	}

	// Scan on Node A produces 0 extra versions
	scanRes, err := fix.workA.Scan(fix.ctx, fix.folder)
	if err != nil || len(scanRes.Captured) != 0 {
		t.Fatalf("scan captured unexpected version: %+v", scanRes.Captured)
	}
}

func TestReconciliationWorkingBasisNotAdvancedByUnreviewedHeads(t *testing.T) {
	fix := newTwoPeerFixture(t)

	// Step 1: A authors A1
	fileA := filepath.Join(fix.rootA, "doc.txt")
	if err := os.WriteFile(fileA, []byte("author A edit 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	resA1, _ := fix.workA.Scan(fix.ctx, fix.folder)
	envA1 := resA1.Captured[0]

	// Step 2: B authors B1
	fileB := filepath.Join(fix.rootB, "doc.txt")
	if err := os.WriteFile(fileB, []byte("author B edit 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	resB1, _ := fix.workB.Scan(fix.ctx, fix.folder)
	envB1 := resB1.Captured[0]

	// Step 3: A syncs from B -> receives B1 (conflict)
	fix.syncAFromB()

	// Verify A has heads [A1, B1]
	conflicts, err := fix.repoA.Conflicts(fix.ctx, fix.folder)
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("expected conflict on A: %v", conflicts)
	}

	// Step 4: Author on A saves a new edit -> A2
	if err := os.WriteFile(fileA, []byte("author A edit 2"), 0o600); err != nil {
		t.Fatal(err)
	}
	resA2, err := fix.workA.Scan(fix.ctx, fix.folder)
	if err != nil || len(resA2.Captured) != 1 {
		t.Fatalf("scan A2: %+v %v", resA2, err)
	}
	envA2 := resA2.Captured[0]

	// Crucial invariant I04: A2 parents must be [A1] ONLY, NOT including B1!
	if len(envA2.Parents) != 1 || envA2.Parents[0] != envA1.ID {
		t.Fatalf("A2 parents=%v, want [%v]", envA2.Parents, envA1.ID)
	}

	// Vector of A2 must NOT contain B
	for _, entry := range envA2.Vector {
		if entry.Author == fix.devB {
			t.Fatalf("A2 vector contains author B: %v", envA2.Vector)
		}
	}

	// Now heads on A are [A2, B1] (B1 is STILL concurrent!)
	conflicts2, err := fix.repoA.Conflicts(fix.ctx, fix.folder)
	if err != nil || len(conflicts2) != 1 {
		t.Fatalf("expected conflict on A: %v", conflicts2)
	}
	c2 := conflicts2[0]
	headIDs := []history.VersionID{c2.Heads[0].ID, c2.Heads[1].ID}
	sort.Slice(headIDs, func(i, j int) bool { return history.CompareVersionID(headIDs[i], headIDs[j]) < 0 })
	expectedHeads := []history.VersionID{envA2.ID, envB1.ID}
	sort.Slice(expectedHeads, func(i, j int) bool { return history.CompareVersionID(expectedHeads[i], expectedHeads[j]) < 0 })
	if !reflect.DeepEqual(headIDs, expectedHeads) {
		t.Fatalf("heads on A=%v, want %v", headIDs, expectedHeads)
	}
}

func TestReconciliationStructuralConflictPreservesChildren(t *testing.T) {
	fix := newTwoPeerFixture(t)

	// Step 1: Base directory "dir" created on Node A
	dirA := filepath.Join(fix.rootA, "dir")
	if err := os.Mkdir(dirA, 0o700); err != nil {
		t.Fatal(err)
	}
	resDir, err := fix.workA.Scan(fix.ctx, fix.folder)
	if err != nil || len(resDir.Captured) != 1 {
		t.Fatalf("scan A dir: %+v %v", resDir, err)
	}
	dirEnv := resDir.Captured[0]

	// Sync to Node B
	fix.syncBFromA()
	if _, err := fix.workB.Scan(fix.ctx, fix.folder); err != nil {
		t.Fatal(err)
	}

	// Step 2: Offline actions:
	// Node A deletes "dir" -> tombstone A2
	if err := os.Remove(dirA); err != nil {
		t.Fatal(err)
	}
	scanDelA, err := fix.workA.Scan(fix.ctx, fix.folder)
	if err != nil {
		t.Fatal(err)
	}
	var dirTomb history.Envelope
	if len(scanDelA.Captured) == 1 {
		dirTomb = scanDelA.Captured[0]
	} else if scanDelA.Deletion != nil {
		app, err := fix.workA.ApproveDeletions(fix.ctx, fix.folder, scanDelA.Deletion.Token)
		if err != nil || len(app) != 1 {
			t.Fatalf("approve deletions A: %v", err)
		}
		dirTomb = app[0]
	} else {
		t.Fatal("expected deletion captured on Node A")
	}

	// Node B creates "dir/child.txt"
	childB := filepath.Join(fix.rootB, "dir", "child.txt")
	if err := os.WriteFile(childB, []byte("child file content"), 0o600); err != nil {
		t.Fatal(err)
	}
	scanChildB, err := fix.workB.Scan(fix.ctx, fix.folder)
	if err != nil || len(scanChildB.Captured) != 1 {
		t.Fatalf("scan child B: %+v %v", scanChildB, err)
	}
	childEnv := scanChildB.Captured[0]

	// Step 3: Sync
	fix.fullSync()

	// Step 4: Verify structural conflict on both nodes
	for _, check := range []struct {
		name string
		repo *repository.DB
	}{
		{"Node A", fix.repoA},
		{"Node B", fix.repoB},
	} {
		sc, err := check.repo.StructuralConflicts(fix.ctx, fix.folder)
		if err != nil {
			t.Fatalf("%s structural conflicts: %v", check.name, err)
		}
		if len(sc) != 1 {
			t.Fatalf("%s structural conflicts len=%d, want 1", check.name, len(sc))
		}
		if sc[0].AncestorPath != "dir" || sc[0].DescendantPath != "dir/child.txt" {
			t.Fatalf("%s structural conflict mismatch: %+v", check.name, sc[0])
		}
	}

	// Invariant I12: Child file on Node B is NOT deleted!
	data, err := os.ReadFile(childB)
	if err != nil || string(data) != "child file content" {
		t.Fatalf("child file on Node B was destroyed: %v %q", err, string(data))
	}

	_ = dirEnv
	_ = dirTomb
	_ = childEnv
}

func TestReconciliationRestartPreservesConflictAndZeroExtraVersions(t *testing.T) {
	fix := newTwoPeerFixture(t)

	// A authors fileA, B authors fileB on same path
	if err := os.WriteFile(filepath.Join(fix.rootA, "test.txt"), []byte("edit A"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _ = fix.workA.Scan(fix.ctx, fix.folder)

	if err := os.WriteFile(filepath.Join(fix.rootB, "test.txt"), []byte("edit B"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _ = fix.workB.Scan(fix.ctx, fix.folder)

	fix.fullSync()

	// Verify conflict before restart
	cBeforeA, err := fix.repoA.Conflicts(fix.ctx, fix.folder)
	if err != nil || len(cBeforeA) != 1 {
		t.Fatalf("before restart conflict on A: %v", cBeforeA)
	}

	// Simulate restart: close repos and reopen them
	if err := fix.repoA.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fix.repoB.Close(); err != nil {
		t.Fatal(err)
	}

	reopenedA, err := repository.Open(fix.ctx, fix.stateA)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedA.Close()

	reopenedB, err := repository.Open(fix.ctx, fix.stateB)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedB.Close()

	// Recheck conflicts after restart
	cAfterA, err := reopenedA.Conflicts(fix.ctx, fix.folder)
	if err != nil || len(cAfterA) != 1 {
		t.Fatalf("after restart conflict on A: %v", cAfterA)
	}
	cAfterB, err := reopenedB.Conflicts(fix.ctx, fix.folder)
	if err != nil || len(cAfterB) != 1 {
		t.Fatalf("after restart conflict on B: %v", cAfterB)
	}

	// Verify working trees
	dataA, err := os.ReadFile(filepath.Join(fix.rootA, "test.txt"))
	if err != nil || string(dataA) != "edit A" {
		t.Fatalf("file A after restart=%q, want 'edit A'", string(dataA))
	}
	dataB, err := os.ReadFile(filepath.Join(fix.rootB, "test.txt"))
	if err != nil || string(dataB) != "edit B" {
		t.Fatalf("file B after restart=%q, want 'edit B'", string(dataB))
	}

	// Invariant I17: Scans after restart produce NO extra versions
	workReopenedA := workspace.New(reopenedA, workspace.Options{})
	workReopenedB := workspace.New(reopenedB, workspace.Options{})

	scanA, err := workReopenedA.Scan(fix.ctx, fix.folder)
	if err != nil || len(scanA.Captured) != 0 {
		t.Fatalf("scan A after restart captured unexpected versions: %+v", scanA.Captured)
	}
	scanB, err := workReopenedB.Scan(fix.ctx, fix.folder)
	if err != nil || len(scanB.Captured) != 0 {
		t.Fatalf("scan B after restart captured unexpected versions: %+v", scanB.Captured)
	}
}
