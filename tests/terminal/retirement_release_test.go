package terminal_test

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/replication"
)

func TestTerminalT13RetiredIdentityApprovalHasRecoveryAction(t *testing.T) {
	f := newEnrollmentFixture(t)
	ctx := context.Background()
	wire := prepareEnrollment(t, f)
	pending, err := f.client.Submit(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	approveEnrollment(t, f, pending, wire)
	retired, err := f.owner.ctrl.RetireMemberExecute(ctx, control.RetireMemberRequest{Folder: f.folder, TargetDevice: f.joiner.device})
	if err != nil {
		t.Fatal(err)
	}
	invite, err := f.control.Mutate(ctx, tc.Mutation{Version: tc.Version, Kind: "invite", OperationID: enrollmentRandom(t), Invite: &tc.InviteIntent{
		Folder: hex.EncodeToString(f.folder[:]), ExpectedMembership: hex.EncodeToString(retired.ApprovedDigest[:]), ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano),
	}})
	if err != nil {
		t.Fatal(err)
	}
	client, err := replication.NewEnrollmentClient(*invite.Invitation, f.joinerID)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	wire, err = client.Prepare(ctx, enrollmentRandom(t), "Retired device", "")
	if err != nil {
		t.Fatal(err)
	}
	pending, err = client.Submit(ctx, wire)
	if err != nil || pending.State != "pending_approval" {
		t.Fatalf("request submission is not membership admission: %+v %v", pending, err)
	}
	mutation := tc.Mutation{Version: tc.Version, Kind: "approval", OperationID: enrollmentRandom(t), Approval: &tc.ApprovalIntent{
		Request: pending.Request, Folder: wire.Folder, Requester: wire.Requester, KeyPin: wire.RequesterPin,
		TranscriptDigest: pending.TranscriptDigest, ExpectedMembership: wire.PriorMembership, Decision: "approve",
	}}
	for i := 0; i < 2; i++ {
		_, err = f.control.Mutate(ctx, mutation)
		if err == nil || !strings.Contains(err.Error(), "RETIRED_MEMBER_REVIVAL") || !strings.Contains(err.Error(), "fresh device identity") {
			t.Fatalf("retired approval needs an explicit fresh-identity recovery action: %v", err)
		}
	}
	membership, approved, err := f.owner.db.GetMembership(ctx, f.folder)
	if err != nil || approved.Digest != retired.ApprovedDigest || len(membership.Active) != 1 || len(membership.Retired) != 1 {
		t.Fatalf("rejected approval changed membership: %+v %v", membership, err)
	}
	// Two real prepare/submit journeys consume the shared per-IP token budget.
	// Let it replenish before asserting signed requester status, as in T03/T05.
	time.Sleep(13 * time.Second)
	status, err := client.Status(ctx, pending.Request)
	if err != nil || status.State != "pending_approval" || status.MembershipHex != "" {
		t.Fatalf("rejected approval admitted data access: %+v %v", status, err)
	}
	// The owner can still dismiss the pending request without reviving the identity.
	mutation.OperationID = enrollmentRandom(t)
	mutation.Approval.Decision = "decline"
	if _, err := f.control.Mutate(ctx, mutation); err != nil {
		t.Fatalf("retirement must not block request dismissal: %v", err)
	}
	requests, err := f.control.Query(ctx, tc.Query{Version: tc.Version, Kind: "requests", Folder: wire.Folder})
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range requests.Requests {
		if request.ID == pending.Request && request.State == "declined" {
			return
		}
	}
	t.Fatalf("retired request was not declined: %+v", requests.Requests)
}

func TestTerminalT13ReplacementBootstrapsRetiredHistory(t *testing.T) {
	f := newEnrollmentFixture(t)
	ctx := context.Background()
	ownerRoot := filepath.Join(f.owner.root, "source")
	if err := os.Mkdir(ownerRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.ws.Register(ctx, f.folder, ownerRoot); err != nil {
		t.Fatal(err)
	}
	retiredVersion := t08Remote(t, f.owner, f.folder, "retained", "recorded retired-author bytes", 91)
	if err := f.owner.ws.Apply(ctx, retiredVersion.ID); err != nil {
		t.Fatal(err)
	}
	retired, err := f.owner.ctrl.RetireMemberExecute(ctx, control.RetireMemberRequest{Folder: f.folder, TargetDevice: retiredVersion.ID.Author})
	if err != nil {
		t.Fatal(err)
	}
	invite, err := f.control.Mutate(ctx, tc.Mutation{Version: tc.Version, Kind: "invite", OperationID: enrollmentRandom(t), Invite: &tc.InviteIntent{
		Folder: hex.EncodeToString(f.folder[:]), ExpectedMembership: hex.EncodeToString(retired.ApprovedDigest[:]), ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano),
	}})
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(f.joiner.root, "replacement")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "existing"), []byte("replacement existing bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	plan, _ := setupReview(t, f.joiner, root, "join")
	mutation := tc.Mutation{Version: tc.Version, Kind: "join", OperationID: enrollmentRandom(t), Join: &tc.JoinIntent{
		Invitation: *invite.Invitation, Attempt: enrollmentRandom(t), DeviceName: plan.DeviceName, FolderName: plan.FolderName,
		Root: plan.Root, Preview: plan.Preview, Settings: plan.Settings,
	}}
	pending, err := f.joiner.ctrl.TerminalMutate(ctx, mutation)
	if err != nil || pending.Operation.Phase != "awaiting_approval" {
		t.Fatalf("replacement request: %+v %v", pending.Error, err)
	}
	approveSetupRequest(t, f, pending.Join.Request)
	// Simulate an older/interrupted bootstrap which installed the approved
	// membership without its retirement artifacts, then reopen the real owner.
	approvedMembership, _, err := f.owner.db.GetMembership(ctx, f.folder)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.joiner.db.EnsureFolder(ctx, f.folder, f.joiner.device, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := f.joiner.db.ApproveMembership(ctx, approvedMembership); err != nil {
		t.Fatal(err)
	}
	f.joiner.close()
	f.joiner.open()
	time.Sleep(25 * time.Second)
	result, err := f.joiner.ctrl.TerminalMutate(ctx, mutation)
	if err != nil || result.State != "completed" || !result.Readiness.Ready() {
		t.Fatalf("ordinary replacement cannot import approved retired history: %+v %v", result.Error, err)
	}
	for path, expected := range map[string]string{"retained": "recorded retired-author bytes", "existing": "replacement existing bytes"} {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || string(data) != expected {
			t.Fatalf("replacement lost %s: %v", path, err)
		}
	}
	heads, err := f.joiner.db.Heads(ctx, f.folder, "retained")
	if err != nil || len(heads) != 1 || heads[0].ID != retiredVersion.ID {
		t.Fatalf("replacement changed original author/version: %+v %v", heads, err)
	}
	replay, err := f.joiner.ctrl.TerminalMutate(ctx, mutation)
	if err != nil || replay.Join.Request != pending.Join.Request || replay.Join.Attempt != pending.Join.Attempt {
		t.Fatalf("replacement replay changed identity: %+v %v", replay.Error, err)
	}
}
