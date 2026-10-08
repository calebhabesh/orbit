package repository_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
)

func TestApprovedMembershipHydratesOnlyHashBoundRetirementSnapshot(t *testing.T) {
	ctx := context.Background()
	db, err := repository.Open(ctx, testkit.NewDisposable(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	folder, local, retired := id('F'), id('L'), id('R')
	if err := db.EnsureFolder(ctx, folder, local, 1); err != nil {
		t.Fatal(err)
	}
	envelope := history.Envelope{ID: history.VersionID{Folder: folder, Author: retired, Counter: 1}, Path: "retained", Kind: history.KindFile,
		Manifest: &history.Manifest{Digest: sha256.Sum256(nil)}, Vector: []history.ClockEntry{{Author: retired, Counter: 1}}, AuthoredRevision: 1}
	snapshot := protocol.RetirementSnapshot{Folder: folder, ConfigurationRev: 1, RetiredDevice: retired,
		AcceptedByRetiree: []protocol.RetiredVersion{{Counter: 1, EnvelopeDigest: repository.EnvelopeDigest(envelope)}}}
	snapshotDigest, err := protocol.RetirementSnapshotDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	membership := protocol.Membership{Folder: folder, Revision: 2, Active: []protocol.ActiveMember{{Device: local, KeyPin: digest('l')}},
		Retired: []protocol.RetiredMember{{Device: retired, RetiredAt: 2, SnapshotDigest: snapshotDigest}}}
	approved, err := db.ApproveMembership(ctx, membership)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ImportMetadata(ctx, envelope); !errors.Is(err, repository.ErrRetiredAuthorVersionRejected) {
		t.Fatalf("missing retirement artifact admitted history: %v", err)
	}
	wrong := snapshot
	wrong.AcceptedByRetiree = []protocol.RetiredVersion{{Counter: 1, EnvelopeDigest: digest('x')}}
	if _, err := db.ApproveMembership(ctx, membership, wrong); err == nil {
		t.Fatal("same-revision replay accepted a different retirement artifact")
	}
	if snapshots, err := db.ListRetirementSnapshots(ctx, folder, 2); err != nil || len(snapshots) != 0 {
		t.Fatalf("failed hydration left rows: %+v %v", snapshots, err)
	}
	for i := 0; i < 2; i++ {
		replay, err := db.ApproveMembership(ctx, membership, snapshot)
		if err != nil || replay != approved {
			t.Fatalf("hydration changed approved membership: %+v %v", replay, err)
		}
	}
	if err := db.ImportMetadata(ctx, envelope); err != nil {
		t.Fatalf("verified retirement artifact did not admit frozen history: %v", err)
	}
	unknown := envelope
	unknown.ID.Counter = 2
	unknown.Vector[0].Counter = 2
	if err := db.ImportMetadata(ctx, unknown); !errors.Is(err, repository.ErrRetiredAuthorVersionRejected) {
		t.Fatalf("hydration admitted unknown retired-author history: %v", err)
	}
	if _, err := db.ApproveMembership(ctx, membership, wrong); err == nil {
		t.Fatal("bad replay replaced a valid retirement artifact")
	}
	stored, err := db.GetRetirementSnapshot(ctx, folder, 2, retired)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := protocol.RetirementSnapshotDigest(stored)
	if err != nil || actual != snapshotDigest {
		t.Fatal("bad replay damaged protected retirement snapshot")
	}
}
