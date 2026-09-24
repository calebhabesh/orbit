package repository_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
)

func id(val byte) history.ID {
	var res history.ID
	for i := range res {
		res[i] = val
	}
	return res
}

func digest(val byte) history.Digest {
	var res history.Digest
	for i := range res {
		res[i] = val
	}
	return res
}

func TestMembershipApprovalAndRetrieval(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	db, err := repository.Open(ctx, disposable)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder := id('F')
	nodeA := id('A')
	nodeB := id('B')
	nodeC := id('C')

	if err := db.EnsureFolder(ctx, folder, nodeA, 1); err != nil {
		t.Fatal(err)
	}

	rev1 := protocol.Membership{
		Folder:   folder,
		Revision: 1,
		Active: []protocol.ActiveMember{
			{Device: nodeA, KeyPin: digest('a')},
			{Device: nodeB, KeyPin: digest('b')},
		},
	}
	app1, err := db.ApproveMembership(ctx, rev1)
	if err != nil {
		t.Fatalf("approve rev1: %v", err)
	}

	active, retired, r, d, err := db.PeerMembers(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if r != 1 || d != app1.Digest || len(active) != 2 || len(retired) != 0 {
		t.Fatalf("unexpected peer members at rev1: r=%d active=%d retired=%d", r, len(active), len(retired))
	}

	// Advance to revision 2 adding nodeC
	rev2 := protocol.Membership{
		Folder:      folder,
		Revision:    2,
		PriorDigest: app1.Digest,
		Active: []protocol.ActiveMember{
			{Device: nodeA, KeyPin: digest('a')},
			{Device: nodeB, KeyPin: digest('b')},
			{Device: nodeC, KeyPin: digest('c')},
		},
	}
	app2, err := db.ApproveMembership(ctx, rev2)
	if err != nil {
		t.Fatalf("approve rev2: %v", err)
	}

	// Verify querying historic revision 1
	hist1, histApp1, err := db.GetMembership(ctx, folder, 1)
	if err != nil {
		t.Fatalf("get historic rev1: %v", err)
	}
	if hist1.Revision != 1 || histApp1.Digest != app1.Digest || len(hist1.Active) != 2 {
		t.Fatalf("historic rev1 mismatch: %+v", hist1)
	}

	// Verify current membership is revision 2
	cur, curApp, err := db.GetMembership(ctx, folder)
	if err != nil {
		t.Fatalf("get current membership: %v", err)
	}
	if cur.Revision != 2 || curApp.Digest != app2.Digest || len(cur.Active) != 3 {
		t.Fatalf("current membership mismatch: %+v", cur)
	}
}

func TestRetirementSnapshotStorageAndAdmission(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	db, err := repository.Open(ctx, disposable)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder := id('F')
	nodeA := id('A')
	nodeB := id('B')

	if err := db.EnsureFolder(ctx, folder, nodeA, 1); err != nil {
		t.Fatal(err)
	}

	rev1 := protocol.Membership{
		Folder:   folder,
		Revision: 1,
		Active: []protocol.ActiveMember{
			{Device: nodeA, KeyPin: digest('a')},
			{Device: nodeB, KeyPin: digest('b')},
		},
	}
	app1, err := db.ApproveMembership(ctx, rev1)
	if err != nil {
		t.Fatal(err)
	}

	// Create envelope B1 authored by nodeB
	emptyDigest := sha256.Sum256(nil)
	b1 := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: nodeB, Counter: 1},
		Path:             "file.txt",
		Kind:             history.KindFile,
		Manifest:         &history.Manifest{Size: 0, Digest: emptyDigest},
		AuthoredRevision: 1,
		Vector:           []history.ClockEntry{{Author: nodeB, Counter: 1}},
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	b1Digest := repository.EnvelopeDigest(b1)

	b3Original := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: nodeB, Counter: 3},
		Path:             "file3.txt",
		Kind:             history.KindFile,
		Manifest:         &history.Manifest{Size: 0, Digest: emptyDigest},
		AuthoredRevision: 1,
		Vector:           []history.ClockEntry{{Author: nodeB, Counter: 3}},
		DisplayTime:      "2026-09-20T00:00:00Z",
	}
	b3OriginalDigest := repository.EnvelopeDigest(b3Original)

	// Build retirement snapshot for nodeB at revision 1
	snapshot := protocol.RetirementSnapshot{
		Folder:           folder,
		ConfigurationRev: 1,
		RetiredDevice:    nodeB,
		AcceptedByRetiree: []protocol.RetiredVersion{
			{Counter: 1, EnvelopeDigest: b1Digest},
			{Counter: 3, EnvelopeDigest: b3OriginalDigest},
		},
	}
	snapshotDigest, err := protocol.RetirementSnapshotDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}

	// Approve revision 2 with nodeB retired
	rev2 := protocol.Membership{
		Folder:      folder,
		Revision:    2,
		PriorDigest: app1.Digest,
		Active: []protocol.ActiveMember{
			{Device: nodeA, KeyPin: digest('a')},
		},
		Retired: []protocol.RetiredMember{
			{Device: nodeB, RetiredAt: 2, SnapshotDigest: snapshotDigest},
		},
	}
	_, err = db.ApproveMembership(ctx, rev2, snapshot)
	if err != nil {
		t.Fatalf("approve rev2 with snapshot: %v", err)
	}

	// Retrieve snapshot
	storedSnap, err := db.GetRetirementSnapshot(ctx, folder, 2, nodeB)
	if err != nil {
		t.Fatalf("get stored snapshot: %v", err)
	}
	if len(storedSnap.AcceptedByRetiree) != 2 || storedSnap.AcceptedByRetiree[0].Counter != 1 {
		t.Fatalf("unexpected stored snapshot: %+v", storedSnap)
	}

	// 1. Import B1 (in snapshot): should SUCCEED
	if err := db.ImportMetadata(ctx, b1); err != nil {
		t.Fatalf("expected B1 to be admitted from snapshot, got error: %v", err)
	}

	// 2. Import B2 (counter 2, NOT in snapshot): should FAIL with ErrRetiredAuthorVersionRejected
	b2 := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: nodeB, Counter: 2},
		Path:             "file.txt",
		Parents:          []history.VersionID{b1.ID},
		Kind:             history.KindFile,
		Manifest:         &history.Manifest{Size: 0, Digest: emptyDigest},
		AuthoredRevision: 1,
		Vector:           []history.ClockEntry{{Author: nodeB, Counter: 2}},
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := db.ImportMetadata(ctx, b2); err != repository.ErrRetiredAuthorVersionRejected {
		t.Fatalf("expected ErrRetiredAuthorVersionRejected for B2, got: %v", err)
	}

	// 3. Import B3 with mutated digest (tampered envelope): should FAIL with ErrRetiredAuthorVersionRejected
	tamperedB3 := b3Original
	tamperedB3.DisplayTime = "2099-01-01T00:00:00Z" // changes envelope digest
	if err := db.ImportMetadata(ctx, tamperedB3); err != repository.ErrRetiredAuthorVersionRejected {
		t.Fatalf("expected rejection for tampered envelope digest, got: %v", err)
	}

	// 4. Import from unknown author (not in membership): should return ErrUnauthorized
	unknownEnv := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: id('X'), Counter: 1},
		Path:             "unknown.txt",
		Kind:             history.KindFile,
		Manifest:         &history.Manifest{Size: 0, Digest: emptyDigest},
		AuthoredRevision: 1,
		Vector:           []history.ClockEntry{{Author: id('X'), Counter: 1}},
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := db.ImportMetadata(ctx, unknownEnv); err != repository.ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized for unknown author, got: %v", err)
	}

	// 5. AuthorizePeer checks
	// Active nodeA succeeds
	if err := db.AuthorizePeer(ctx, folder, nodeA, digest('a'), 2, rev2Digest(t, rev2)); err != nil {
		t.Fatalf("active peer authorize: %v", err)
	}
	// Retired nodeB fails with ErrUnauthorized
	if err := db.AuthorizePeer(ctx, folder, nodeB, digest('b'), 2, rev2Digest(t, rev2)); err != repository.ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized for retired peer, got: %v", err)
	}
	// Stale revision 1 fails with ErrMembershipMismatch
	if err := db.AuthorizePeer(ctx, folder, nodeA, digest('a'), 1, app1.Digest); err != repository.ErrMembershipMismatch {
		t.Fatalf("expected ErrMembershipMismatch for stale revision, got: %v", err)
	}
}

func rev2Digest(t *testing.T, m protocol.Membership) history.Digest {
	t.Helper()
	d, err := protocol.MembershipDigest(m)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestResumableMaintenanceLifecycle(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	db, err := repository.Open(ctx, disposable)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder := id('F')
	nodeA := id('A')
	nodeB := id('B')

	if err := db.EnsureFolder(ctx, folder, nodeA, 1); err != nil {
		t.Fatal(err)
	}

	maintID := "maint-123"
	payload := []byte("resumable-state-payload")
	now := time.Now()

	if err := db.SaveResumableMaintenance(ctx, maintID, folder, nodeB, "PROPOSED", payload, now); err != nil {
		t.Fatalf("save maintenance: %v", err)
	}

	gotID, gotTarget, gotPhase, gotState, err := db.GetResumableMaintenance(ctx, folder)
	if err != nil {
		t.Fatalf("get maintenance: %v", err)
	}
	if gotID != maintID || gotTarget != nodeB || gotPhase != "PROPOSED" || string(gotState) != string(payload) {
		t.Fatalf("maintenance mismatch: id=%s target=%x phase=%s", gotID, gotTarget, gotPhase)
	}

	if err := db.DeleteResumableMaintenance(ctx, maintID); err != nil {
		t.Fatalf("delete maintenance: %v", err)
	}

	if _, _, _, _, err := db.GetResumableMaintenance(ctx, folder); err == nil {
		t.Fatal("expected error after deleting maintenance, got nil")
	}
}

func TestPeerProgressDirectVsIndirect(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	db, err := repository.Open(ctx, disposable)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder := id('F')
	nodeA := id('A')
	nodeB := id('B')
	nodeC := id('C')

	if err := db.EnsureFolder(ctx, folder, nodeA, 1); err != nil {
		t.Fatal(err)
	}

	_, err = db.ApproveMembership(ctx, protocol.Membership{
		Folder:   folder,
		Revision: 1,
		Active: []protocol.ActiveMember{
			{Device: nodeA, KeyPin: digest('a')},
			{Device: nodeB, KeyPin: digest('b')},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("hello")
	chunkDigest := sha256.Sum256(data)
	if err := db.InstallChunk(ctx, chunkDigest, uint64(len(data)), bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}

	v1, err := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder: folder,
		Path:   "hello.txt",
		Kind:   history.KindFile,
		Manifest: &history.Manifest{
			Size:   uint64(len(data)),
			Digest: chunkDigest,
			Chunks: []history.Chunk{{Digest: chunkDigest, Length: uint64(len(data))}},
		},
		DisplayTime: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	// Direct receipt from nodeB
	if err := db.RecordPeerReceiptWithOptions(ctx, folder, nodeB, v1.ID, true, now); err != nil {
		t.Fatal(err)
	}

	// Indirect receipt from nodeC (forwarded via B)
	if err := db.RecordPeerReceiptWithOptions(ctx, folder, nodeC, v1.ID, false, now); err != nil {
		t.Fatal(err)
	}

	progress, err := db.PeerProgress(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) != 2 {
		t.Fatalf("expected 2 progress records, got %d", len(progress))
	}

	byPeer := map[history.ID]repository.PeerProgress{}
	for _, p := range progress {
		byPeer[p.Peer] = p
	}

	if !byPeer[nodeB].Direct {
		t.Fatal("expected nodeB progress to be direct")
	}
	if byPeer[nodeC].Direct {
		t.Fatal("expected nodeC progress to be indirect")
	}
}
