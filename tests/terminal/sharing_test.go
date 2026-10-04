package terminal_test

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/replication"
)

func TestTerminalT05ScopedKnownDeviceSharing(t *testing.T) {
	f := newEnrollmentFixture(t)
	ctx := context.Background()
	wire, err := f.client.Prepare(ctx, enrollmentRandom(t), "Known", "")
	if err != nil {
		t.Fatal(err)
	}
	first, err := f.client.Submit(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	approveSetupRequest(t, f, first.Request)
	folder := mustID(t, enrollmentRandom(t))
	if err = f.owner.db.EnsureFolder(ctx, folder, f.owner.device, 1); err != nil {
		t.Fatal(err)
	}
	app, err := f.owner.db.ApproveMembership(ctx, protocol.Membership{Folder: folder, Revision: 1, Active: []protocol.ActiveMember{{Device: f.owner.device, KeyPin: f.ownerID.KeyPin}}})
	if err != nil {
		t.Fatal(err)
	}
	m := tc.Mutation{Version: "1", Kind: "share", OperationID: enrollmentRandom(t), Invite: &tc.InviteIntent{Folder: hex.EncodeToString(folder[:]), Device: hex.EncodeToString(f.joiner.device[:]), ExpectedMembership: hex.EncodeToString(app.Digest[:]), ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)}}
	r, err := f.control.Mutate(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := f.control.Mutate(ctx, m)
	if err != nil || replay.Invitation.Capability != r.Invitation.Capability {
		t.Fatal("share retry lost invitation")
	}
	changed := m
	copyIntent := *m.Invite
	copyIntent.Device = enrollmentRandom(t)
	changed.Invite = &copyIntent
	if _, err = f.control.Mutate(ctx, changed); err == nil {
		t.Fatal("changed replay accepted")
	}
	attacker := fresh(t)
	id, _ := replication.LoadOrCreateIdentity(attacker.state, attacker.device, time.Now())
	wrong, err := replication.NewEnrollmentClient(*r.Invitation, id)
	if err != nil {
		t.Fatal(err)
	}
	defer wrong.Close()
	if _, err = wrong.Prepare(ctx, enrollmentRandom(t), "Known", ""); err == nil {
		t.Fatal("alias impersonation accepted")
	}
	second, err := replication.NewEnrollmentClient(*r.Invitation, f.joinerID)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	secondWire, err := second.Prepare(ctx, enrollmentRandom(t), "Known", "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := second.Submit(ctx, secondWire)
	if err != nil || result.Request == first.Request {
		t.Fatalf("second scoped attempt collided: %v", err)
	}
	time.Sleep(13 * time.Second) // replenish real per-IP admission before identical retry
	again, err := second.Submit(ctx, secondWire)
	if err != nil || again.Request != result.Request {
		t.Fatal("request retry failed")
	}
	// Approval remains separate; a retained first-folder approval cannot admit it.
	obs, err := f.control.Query(ctx, tc.Query{Version: "1", Kind: "requests"})
	if err != nil || len(obs.Requests) != 2 {
		t.Fatal("retained requests missing")
	}
	oldFolder := f.folder
	f.folder = folder
	approveSetupRequest(t, f, result.Request)
	f.folder = oldFolder
	membership, _, err := f.owner.db.GetMembership(ctx, folder)
	if err != nil || len(membership.Active) != 2 {
		t.Fatal("independent membership absent")
	}
	pin, err := f.owner.db.DeviceKeyPin(ctx, f.joiner.device)
	if err != nil || pin != f.joinerID.KeyPin {
		t.Fatal("persistent key changed")
	}
}

func TestTerminalT05MembershipPinAndSequentialGate(t *testing.T) {
	f := newEnrollmentFixture(t)
	ctx := context.Background()
	wire, err := f.client.Prepare(ctx, enrollmentRandom(t), "Known", "")
	if err != nil {
		t.Fatal(err)
	}
	pending, err := f.client.Submit(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	approveSetupRequest(t, f, pending.Request)
	current, app, err := f.owner.db.GetMembership(ctx, f.folder)
	if err != nil {
		t.Fatal(err)
	}
	from := app
	for i := 0; i < 2; i++ {
		extra := fresh(t)
		id, err := replication.LoadOrCreateIdentity(extra.state, extra.device, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		current.PriorDigest = app.Digest
		current.Revision++
		current.Active = append(current.Active, protocol.ActiveMember{Device: extra.device, KeyPin: id.KeyPin})
		app, err = f.owner.db.ApproveMembership(ctx, current)
		if err != nil {
			t.Fatal(err)
		}
	}
	client, err := replication.NewClient(f.peerURL, f.joinerID, f.ownerID.Leaf, f.ownerID.KeyPin)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	response, err := client.MembershipGet(ctx, replication.MembershipGetRequest{ProtocolVersion: "1", DeviceID: hex.EncodeToString(f.joiner.device[:]), FolderID: hex.EncodeToString(f.folder[:]), FromRevision: "2", ExpectedDigest: hex.EncodeToString(from.Digest[:])})
	if err != nil || response.Membership.Revision != 3 || response.Membership.PriorDigest != from.Digest {
		t.Fatalf("skipped predecessor: %v", err)
	}
	req := replication.MembershipGetRequest{ProtocolVersion: "1", DeviceID: hex.EncodeToString(f.joiner.device[:]), FolderID: hex.EncodeToString(f.folder[:]), FromRevision: "2", ExpectedDigest: strings.Repeat("a", 64)}
	if _, err = client.MembershipGet(ctx, req); err == nil || !strings.Contains(err.Error(), "MEMBERSHIP_FORK") {
		t.Fatalf("fork not surfaced: %v", err)
	}
	// The production syncer installs both missing revisions before data exchange.
	if err = f.joiner.db.EnsureFolder(ctx, f.folder, f.joiner.device, 1); err != nil {
		t.Fatal(err)
	}
	original, _, err := f.owner.db.GetMembership(ctx, f.folder, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.joiner.db.ApproveMembership(ctx, original); err != nil {
		t.Fatal(err)
	}
	syncer := replication.NewSyncer(f.joiner.db, nil, client, f.joiner.device, f.owner.device, f.folder, from, replication.TransferOptions{})
	if _, err = syncer.Sync(ctx); err != nil {
		t.Fatalf("sequential sync rollout: %v", err)
	}
	adopted, err := f.joiner.db.Membership(ctx, f.folder)
	if err != nil || adopted != app {
		t.Fatal("did not durably install full chain")
	}
	// A different certificate claiming a real member ID must not fetch membership.
	fake := fresh(t)
	fakeID, _ := replication.LoadOrCreateIdentity(fake.state, fake.device, time.Now())
	forged, err := replication.NewClient(f.peerURL, fakeID, f.ownerID.Leaf, f.ownerID.KeyPin)
	if err != nil {
		t.Fatal(err)
	}
	defer forged.CloseIdleConnections()
	if _, err = forged.MembershipGet(ctx, replication.MembershipGetRequest{ProtocolVersion: "1", DeviceID: hex.EncodeToString(f.joiner.device[:]), FolderID: hex.EncodeToString(f.folder[:])}); err == nil {
		t.Fatal("membership fetched with wrong pin")
	}
	// Exact-revision data requests remain denied while configuration is behind.
	if _, err = client.Inventory(ctx, replication.InventoryRequest{ProtocolVersion: "1", DeviceID: hex.EncodeToString(f.joiner.device[:]), FolderID: hex.EncodeToString(f.folder[:]), Revision: "2", MembershipDigest: hex.EncodeToString(from.Digest[:]), Cursor: "0", PageSize: "128"}); err == nil {
		t.Fatal("data transferred before rollout")
	}
}

func TestTerminalT05ForkRecoveryPreservesOriginalGroup(t *testing.T) {
	f := fresh(t)
	ctx := context.Background()
	root := filepath.Join(f.root, "original")
	os.Mkdir(root, 0700)
	os.WriteFile(filepath.Join(root, "keep"), []byte("captured before fork"), 0600)
	p, _ := setupReview(t, f, root, "setup")
	r, err := f.ctrl.TerminalMutate(ctx, tc.Mutation{Version: "1", Kind: "setup", OperationID: enrollmentRandom(t), Setup: &p})
	if err != nil {
		t.Fatal(err)
	}
	folder := mustID(t, r.Join.Folder)
	current, app, err := f.db.GetMembership(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	next := current
	next.Revision++
	next.PriorDigest = app.Digest
	if _, err = f.db.ApproveMembership(ctx, next); err != nil {
		t.Fatal(err)
	}
	fork := next
	fork.Active = append(append([]protocol.ActiveMember(nil), next.Active...), protocol.ActiveMember{Device: mustID(t, enrollmentRandom(t)), KeyPin: history.Digest(mustID(t, enrollmentRandom(t)))})
	if _, err = f.db.ApproveMembership(ctx, fork); err == nil {
		t.Fatal("competing branch accepted")
	}
	// Recovery does not rewrite causal history or membership: pause the old group,
	// inspect/export exact content, then review a separate root as a new group.
	if err = f.ws.Pause(ctx, folder, "MEMBERSHIP_FORK"); err != nil {
		t.Fatal(err)
	}
	recovered := filepath.Join(f.root, "recovered")
	os.Mkdir(recovered, 0700)
	bytes, err := os.ReadFile(filepath.Join(root, "keep"))
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(recovered, "keep"), bytes, 0600)
	plan, _ := setupReview(t, f, recovered, "adopt")
	rebuilt, err := f.ctrl.TerminalMutate(ctx, tc.Mutation{Version: "1", Kind: "adopt", OperationID: enrollmentRandom(t), Setup: &plan})
	if err != nil || rebuilt.Join.Folder == r.Join.Folder {
		t.Fatal("recovery group failed")
	}
	oldHeads, _ := f.db.Heads(ctx, folder, "keep")
	newHeads, _ := f.db.Heads(ctx, mustID(t, rebuilt.Join.Folder), "keep")
	if len(oldHeads) != 1 || len(newHeads) != 1 || oldHeads[0].Manifest.Digest != newHeads[0].Manifest.Digest {
		t.Fatal("recovery discarded captured bytes")
	}
	reg, _ := f.db.Root(ctx, folder)
	if !reg.Paused {
		t.Fatal("old group resumed implicitly")
	}
}

func TestTerminalT05CompetingReviewedApprovals(t *testing.T) {
	f := newEnrollmentFixture(t)
	ctx := context.Background()
	firstWire, err := f.client.Prepare(ctx, enrollmentRandom(t), "first", "")
	if err != nil {
		t.Fatal(err)
	}
	first, err := f.client.Submit(ctx, firstWire)
	if err != nil {
		t.Fatal(err)
	}
	extra := fresh(t)
	identity, err := replication.LoadOrCreateIdentity(extra.state, extra.device, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	m := f.mutation
	m.OperationID = enrollmentRandom(t)
	invitation, err := f.control.Mutate(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	client, err := replication.NewEnrollmentClient(*invitation.Invitation, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	secondWire, err := client.Prepare(ctx, enrollmentRandom(t), "second", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.Submit(ctx, secondWire)
	if err != nil {
		t.Fatal(err)
	}
	approveEnrollment(t, f, first, firstWire)
	_, err = f.control.Mutate(ctx, tc.Mutation{Version: "1", Kind: "approval", OperationID: enrollmentRandom(t), Approval: &tc.ApprovalIntent{Request: second.Request, Folder: secondWire.Folder, Requester: secondWire.Requester, KeyPin: secondWire.RequesterPin, TranscriptDigest: second.TranscriptDigest, ExpectedMembership: secondWire.PriorMembership, Decision: "approve"}})
	if err == nil || !strings.Contains(err.Error(), "STALE_VIEW") {
		t.Fatalf("competing approval accepted: %v", err)
	}
	members, _, err := f.owner.db.GetMembership(ctx, f.folder)
	if err != nil || members.Revision != 2 || len(members.Active) != 2 {
		t.Fatal("stale approval changed membership")
	}
}

func TestTerminalT05TargetedShareRejectsRekey(t *testing.T) {
	f := newEnrollmentFixture(t)
	ctx := context.Background()
	wire, err := f.client.Prepare(ctx, enrollmentRandom(t), "known", "")
	if err != nil {
		t.Fatal(err)
	}
	first, err := f.client.Submit(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	approveEnrollment(t, f, first, wire)
	other := mustID(t, enrollmentRandom(t))
	f.owner.db.EnsureFolder(ctx, other, f.owner.device, 1)
	app, err := f.owner.db.ApproveMembership(ctx, protocol.Membership{Folder: other, Revision: 1, Active: []protocol.ActiveMember{{Device: f.owner.device, KeyPin: f.ownerID.KeyPin}}})
	if err != nil {
		t.Fatal(err)
	}
	shared, err := f.control.Mutate(ctx, tc.Mutation{Version: "1", Kind: "share", OperationID: enrollmentRandom(t), Invite: &tc.InviteIntent{Folder: hex.EncodeToString(other[:]), Device: hex.EncodeToString(f.joiner.device[:]), ExpectedMembership: hex.EncodeToString(app.Digest[:]), ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)}})
	if err != nil {
		t.Fatal(err)
	}
	wrongState := fresh(t)
	wrongID, err := replication.LoadOrCreateIdentity(wrongState.state, f.joiner.device, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := replication.NewEnrollmentClient(*shared.Invitation, wrongID)
	if err != nil {
		t.Fatal(err)
	}
	defer wrong.Close()
	changed, err := wrong.Prepare(ctx, enrollmentRandom(t), "known", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = wrong.Submit(ctx, changed); err == nil {
		t.Fatal("targeted device rekeyed without approval")
	}
	rs, err := f.control.Query(ctx, tc.Query{Version: "1", Kind: "requests", Folder: hex.EncodeToString(other[:])})
	if err != nil || len(rs.Requests) != 0 {
		t.Fatal("wrong pin left pending request")
	}
}
