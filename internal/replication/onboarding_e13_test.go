package replication

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
)

func TestOnboardingE13ManualSyncPersistsDeviceRemoval(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	path := filepath.Join(f.receiverRoot, "keep.txt")
	if err := os.WriteFile(path, []byte("unsent local bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot := protocol.RetirementSnapshot{Folder: f.folder, ConfigurationRev: 1, RetiredDevice: f.receiverID.DeviceID}
	digest, err := protocol.RetirementSnapshotDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	next := protocol.Membership{Folder: f.folder, Revision: 2, PriorDigest: f.approved.Digest, Active: []protocol.ActiveMember{{Device: f.senderID.DeviceID, KeyPin: f.senderID.KeyPin}}, Retired: []protocol.RetiredMember{{Device: f.receiverID.DeviceID, RetiredAt: 2, SnapshotDigest: digest}}}
	proposal := repository.RetirementProposal{Initiator: f.senderID.DeviceID, DeviceName: "Receiver", Membership: next, Snapshot: snapshot}
	if err = f.senderRepo.PrepareRetirement(ctx, proposal); err != nil {
		t.Fatal(err)
	}
	if _, err = f.senderRepo.CommitRetirement(ctx, proposal); err != nil {
		t.Fatal(err)
	}
	syncer := NewSyncer(f.receiverRepo, f.receiverWork, f.client, f.receiverID.DeviceID, f.senderID.DeviceID, f.folder, f.approved, TransferOptions{})
	_, err = syncer.Sync(ctx)
	var refusal *WireError
	if !errors.As(err, &refusal) || refusal.Body.Code != DeviceRemovedCode {
		t.Fatal("expected authenticated removal", err)
	}
	if by, err := f.receiverRepo.RemovedBy(ctx, f.folder); err != nil || by != f.senderID.DeviceID {
		t.Fatal("manual sync did not retain removal actor", by, err)
	}
	if _, err := f.receiverRepo.EnqueueDurableTask(ctx, repository.DurableTask{Folder: f.folder, Kind: "scan"}); !errors.Is(err, repository.ErrFolderLeft) {
		t.Fatal("removed Orbit accepted work", err)
	}
	if root, err := f.receiverRepo.Root(ctx, f.folder); err != nil || root.Path != f.receiverRoot {
		t.Fatal("removal hid retained root", root, err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "unsent local bytes" {
		t.Fatal("removal changed working bytes", err)
	}
	dir := f.receiverRepo.StateDir()
	if err := f.receiverRepo.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := repository.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if ended, err := reopened.FolderLeft(ctx, f.folder); err != nil || !ended {
		t.Fatal("manual removal lost across restart", ended, err)
	}
}

func TestOnboardingE13LateRemovedPeerRefusalCannotEndLocalParticipation(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	active, release, err := f.receiverRepo.BeginFolderExchange(ctx, f.folder)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	done := make(chan error, 1)
	go func() {
		done <- RecordDeviceRemoval(ctx, f.receiverRepo, f.folder, f.senderID.DeviceID, &WireError{Body: ErrorResponse{Code: DeviceRemovedCode}})
	}()
	select {
	case <-active.Done():
	case <-time.After(time.Second):
		t.Fatal("removal did not begin draining exchanges")
	}
	// Remove the reporting peer while its refusal waits for an admitted exchange.
	snapshot := protocol.RetirementSnapshot{Folder: f.folder, ConfigurationRev: 1, RetiredDevice: f.senderID.DeviceID}
	digest, err := protocol.RetirementSnapshotDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	next := protocol.Membership{Folder: f.folder, Revision: 2, PriorDigest: f.approved.Digest, Active: []protocol.ActiveMember{{Device: f.receiverID.DeviceID, KeyPin: f.receiverID.KeyPin}}, Retired: []protocol.RetiredMember{{Device: f.senderID.DeviceID, RetiredAt: 2, SnapshotDigest: digest}}}
	proposal := repository.RetirementProposal{Initiator: f.receiverID.DeviceID, Membership: next, Snapshot: snapshot}
	if err = f.receiverRepo.PrepareRetirement(ctx, proposal); err != nil {
		t.Fatal(err)
	}
	if _, err = f.receiverRepo.CommitRetirement(ctx, proposal); err != nil {
		t.Fatal(err)
	}
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("late refusal did not finish")
	}
	if ended, err := f.receiverRepo.FolderLeft(ctx, f.folder); err != nil || ended {
		t.Fatal("retired peer ended local participation", ended, err)
	}
	// The repository guard also ignores a directly replayed late refusal.
	if err := f.receiverRepo.MarkDeviceRemovedFromPeer(ctx, f.folder, f.senderID.DeviceID, f.senderID.DeviceID, true); err != nil {
		t.Fatal(err)
	}
	if by, err := f.receiverRepo.RemovedBy(ctx, f.folder); err != nil || by != (history.ID{}) {
		t.Fatal("late refusal stored removal marker", by, err)
	}
	if _, err := f.receiverRepo.EnqueueDurableTask(ctx, repository.DurableTask{Folder: f.folder, Kind: "scan"}); err != nil {
		t.Fatal("late refusal stopped local scans", err)
	}
}

func TestOnboardingE13LeftOrbitRefusesEveryDataEndpointAndMembership(t *testing.T) {
	f := newPeerFixture(t)
	if err := f.serverRepo.LeaveFolder(f.ctx, f.folder, time.Now()); err != nil {
		t.Fatal(err)
	}
	h := f.handshake()
	dev := hex.EncodeToString(f.clientID.DeviceID[:])
	calls := []func() error{
		func() error { _, e := f.client.Hello(f.ctx, f.hello(f.clientID.DeviceID, h)); return e },
		func() error { _, e := f.client.Inventory(f.ctx, f.inventory("", "0", 1)); return e },
		func() error {
			_, e := f.client.Versions(f.ctx, VersionsRequest{ProtocolVersion: ProtocolVersion, DeviceID: dev, FolderID: h.FolderID, Revision: h.Revision, MembershipDigest: h.MembershipDigest, Versions: []VersionIDWire{{AuthorID: dev, Counter: "1"}}})
			return e
		},
		func() error {
			_, e := f.client.Chunk(f.ctx, ChunkRequest{ProtocolVersion: ProtocolVersion, DeviceID: dev, FolderID: h.FolderID, Revision: h.Revision, MembershipDigest: h.MembershipDigest, AuthorID: dev, Counter: "1", ChunkIndex: "0"}, history.Chunk{})
			return e
		},
		func() error {
			_, e := f.client.Receipts(f.ctx, ReceiptsRequest{ProtocolVersion: ProtocolVersion, DeviceID: dev, FolderID: h.FolderID, Revision: h.Revision, MembershipDigest: h.MembershipDigest, Versions: []VersionIDWire{{AuthorID: dev, Counter: "1"}}})
			return e
		},
		func() error {
			_, e := f.client.Names(f.ctx, NamesRequest{ProtocolVersion: ProtocolVersion, DeviceID: dev, FolderID: h.FolderID, Revision: h.Revision, MembershipDigest: h.MembershipDigest, Devices: []NameWire{}})
			return e
		},
		func() error {
			_, e := f.client.MembershipGet(f.ctx, MembershipGetRequest{ProtocolVersion: ProtocolVersion, DeviceID: dev, FolderID: h.FolderID})
			return e
		},
		func() error {
			_, e := f.client.Status(f.ctx, StatusRequest{ProtocolVersion: ProtocolVersion, DeviceID: dev, FolderID: h.FolderID, Revision: h.Revision, MembershipDigest: h.MembershipDigest, Versions: []VersionIDWire{{AuthorID: dev, Counter: "1"}}})
			return e
		},
	}
	for i, call := range calls {
		e := call()
		var wire *WireError
		if !errors.As(e, &wire) || wire.Body.Code != PeerLeftCode || wire.Body.Retryable {
			t.Fatalf("endpoint %d still works or retries: %v", i, e)
		}
	}
	// Authentication remains mandatory before revealing the participation state.
	attacker, e := NewClient(f.baseURL, f.attackerID, f.serverID.Leaf, f.serverID.KeyPin)
	if e != nil {
		t.Fatal(e)
	}
	defer attacker.CloseIdleConnections()
	_, e = attacker.Hello(f.ctx, f.hello(f.attackerID.DeviceID, h))
	var wire *WireError
	if !errors.As(e, &wire) || wire.Body.Code != "UNAUTHORIZED" {
		t.Fatal("left state disclosed to unauthorized identity", e)
	}
	// Another Orbit with the same two devices still exchanges normally.
	other := fixedID('G')
	if e = f.serverRepo.EnsureFolder(f.ctx, other, f.serverID.DeviceID, 1); e != nil {
		t.Fatal(e)
	}
	m := f.membership
	m.Folder = other
	app, e := f.serverRepo.ApproveMembership(f.ctx, m)
	if e != nil {
		t.Fatal(e)
	}
	h.FolderID = hex.EncodeToString(other[:])
	h.MembershipDigest = hex.EncodeToString(app.Digest[:])
	if _, e = f.client.Hello(f.ctx, f.hello(f.clientID.DeviceID, h)); e != nil {
		t.Fatal("Leave broke another Orbit", e)
	}
}

func TestOnboardingE13RetirementPrepareCommitAndLateHistory(t *testing.T) {
	f := newPeerFixture(t)
	ctx := context.Background()
	third := fixedID('C')
	current := f.membership
	current.Revision = 2
	current.PriorDigest = f.digest
	current.Active = append(current.Active, protocol.ActiveMember{Device: third, KeyPin: history.Digest(fixedID('c'))})
	app, err := f.serverRepo.ApproveMembership(ctx, current)
	if err != nil {
		t.Fatal(err)
	}
	snap := protocol.RetirementSnapshot{Folder: f.folder, ConfigurationRev: 2, RetiredDevice: third}
	sd, err := protocol.RetirementSnapshotDigest(snap)
	if err != nil {
		t.Fatal(err)
	}
	next := current
	next.Revision = 3
	next.PriorDigest = app.Digest
	next.Active = append([]protocol.ActiveMember(nil), f.membership.Active...)
	next.Retired = []protocol.RetiredMember{{Device: third, RetiredAt: 3, SnapshotDigest: sd}}
	p := repository.RetirementProposal{Initiator: f.clientID.DeviceID, DeviceName: "Pi", Membership: next, Snapshot: snap}
	req := RetirementRequest{ProtocolVersion: ProtocolVersion, DeviceID: hex.EncodeToString(f.clientID.DeviceID[:]), Action: "commit", Proposal: p}
	if _, err = f.client.Retirement(ctx, req); err == nil {
		t.Fatal("unprepared retirement committed")
	}
	req.Action = "prepare"
	if _, err = f.client.Retirement(ctx, req); err != nil {
		t.Fatal(err)
	}
	// A new retiree version after review cannot silently be dropped.
	env := history.Envelope{ID: history.VersionID{Folder: f.folder, Author: third, Counter: 1}, Path: "late", Kind: history.KindDirectory, Vector: []history.ClockEntry{{Author: third, Counter: 1}}, AuthoredRevision: 2, DisplayTime: time.Now().UTC().Format(time.RFC3339Nano)}
	if err = f.serverRepo.ImportMetadata(ctx, env); err != nil {
		t.Fatal(err)
	}
	req.Action = "commit"
	if _, err = f.client.Retirement(ctx, req); err == nil {
		t.Fatal("late history dropped")
	}
	now, _ := f.serverRepo.Membership(ctx, f.folder)
	if now.Digest != app.Digest {
		t.Fatal("failed commit changed membership")
	}
	// Fresh exact review succeeds and repeats idempotently.
	p.Snapshot.AcceptedByRetiree, err = f.serverRepo.VersionsByAuthor(ctx, f.folder, third)
	if err != nil {
		t.Fatal(err)
	}
	sd, _ = protocol.RetirementSnapshotDigest(p.Snapshot)
	p.Membership.Retired[0].SnapshotDigest = sd
	req.Proposal = p
	req.Action = "prepare"
	if _, err = f.client.Retirement(ctx, req); err != nil {
		t.Fatal(err)
	}
	req.Action = "commit"
	for i := 0; i < 2; i++ {
		if _, err = f.client.Retirement(ctx, req); err != nil {
			t.Fatal("commit/replay", err)
		}
	}
	// A later review for a different retiree cannot change attribution of
	// the completed removal. Manual retirements have no invented actor.
	app, _ = f.serverRepo.Membership(ctx, f.folder)
	secondSnapshot := protocol.RetirementSnapshot{Folder: f.folder, ConfigurationRev: 3, RetiredDevice: f.clientID.DeviceID}
	secondDigest, _ := protocol.RetirementSnapshotDigest(secondSnapshot)
	second := protocol.Membership{Folder: f.folder, Revision: 4, PriorDigest: app.Digest, Active: []protocol.ActiveMember{{Device: f.serverID.DeviceID, KeyPin: f.serverID.KeyPin}}, Retired: append(append([]protocol.RetiredMember(nil), p.Membership.Retired...), protocol.RetiredMember{Device: f.clientID.DeviceID, RetiredAt: 4, SnapshotDigest: secondDigest})}
	if err = f.serverRepo.PrepareRetirement(ctx, repository.RetirementProposal{Initiator: f.serverID.DeviceID, Membership: second, Snapshot: secondSnapshot}); err != nil {
		t.Fatal(err)
	}
	ending := f.server.removalError(ctx, f.folder, third).(*peerRemovalError)
	if ending.by != f.clientID.DeviceID {
		t.Fatal("later review changed removal actor", ending.by)
	}
	unknown := f.server.removalError(ctx, f.folder, fixedID('Z')).(*peerRemovalError)
	if unknown.by != (history.ID{}) {
		t.Fatal("invented actor for a legacy removal", unknown.by)
	}
}
