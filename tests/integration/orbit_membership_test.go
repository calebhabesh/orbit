package integration_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

// TestOrbitMembership_SequentialRollout tests linear revision rollout across multiple revisions,
// checking that PriorDigest binds strictly to the hash of Revision N.
func TestOrbitMembership_SequentialRollout(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}

	devABytes := make([]byte, 32)
	rand.Read(devABytes)
	var devA history.ID
	copy(devA[:], devABytes)

	coreCfg := config.Config{
		FormatVersion: 1,
		DeviceID:      hex.EncodeToString(devABytes),
		CreatedAt:     time.Now().UTC(),
	}
	if err := config.Save(stateDir, coreCfg); err != nil {
		t.Fatal(err)
	}

	identA, err := replication.LoadOrCreateIdentity(stateDir, devA, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := workspace.New(db, workspace.Options{})
	ctrl := control.New(db, ws, control.Options{LocalDevice: devA})

	var folderID history.ID
	rand.Read(folderID[:])
	if err := db.EnsureFolder(ctx, folderID, devA, 1); err != nil {
		t.Fatal(err)
	}

	// Revision 1: Device A alone
	rev1 := protocol.Membership{
		Folder:      folderID,
		Revision:    1,
		PriorDigest: history.Digest{},
		Active: []protocol.ActiveMember{
			{Device: devA, KeyPin: identA.KeyPin},
		},
	}
	app1, err := db.ApproveMembership(ctx, rev1)
	if err != nil {
		t.Fatalf("Rev 1 approval failed: %v", err)
	}

	// Revision 2: Device B joins via invitation & enrollment
	invB, err := ctrl.CreateInvitation(ctx, control.CreateInvitationRequest{Folder: folderID, TTLSecs: 3600, MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	pubB, privB, _ := ed25519.GenerateKey(rand.Reader)
	devB := sha256.Sum256(pubB)
	chalB := []byte("join-b-challenge")
	sigB := ed25519.Sign(privB, chalB)

	subResB, err := ctrl.SubmitEnrollmentRequest(ctx, control.SubmitJoinRequestPayload{
		Token:          invB.Token,
		JoiningDevice:  devB,
		PublicKey:      hex.EncodeToString(pubB),
		Signature:      hex.EncodeToString(sigB),
		Challenge:      hex.EncodeToString(chalB),
		SuggestedLabel: "Device B",
		TargetFolder:   folderID,
	})
	if err != nil {
		t.Fatal(err)
	}

	appResB, err := ctrl.ApproveEnrollmentRequest(ctx, control.ApproveEnrollmentRequest{
		RequestID: subResB.RequestID,
		Folder:    folderID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if appResB.Revision != 2 {
		t.Fatalf("expected revision 2, got %d", appResB.Revision)
	}

	// Verify Rev 2 has PriorDigest = app1.Digest
	mem2, curApp2, err := db.GetMembership(ctx, folderID)
	if err != nil {
		t.Fatal(err)
	}
	if mem2.PriorDigest != app1.Digest {
		t.Fatalf("Rev 2 PriorDigest mismatch: got %x, want %x", mem2.PriorDigest, app1.Digest)
	}
	if curApp2.Digest != appResB.Digest {
		t.Fatalf("Rev 2 digest mismatch: got %x, want %x", curApp2.Digest, appResB.Digest)
	}

	// Revision 3: Device C joins
	invC, err := ctrl.CreateInvitation(ctx, control.CreateInvitationRequest{Folder: folderID, TTLSecs: 3600, MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	pubC, privC, _ := ed25519.GenerateKey(rand.Reader)
	devC := sha256.Sum256(pubC)
	chalC := []byte("join-c-challenge")
	sigC := ed25519.Sign(privC, chalC)

	subResC, err := ctrl.SubmitEnrollmentRequest(ctx, control.SubmitJoinRequestPayload{
		Token:          invC.Token,
		JoiningDevice:  devC,
		PublicKey:      hex.EncodeToString(pubC),
		Signature:      hex.EncodeToString(sigC),
		Challenge:      hex.EncodeToString(chalC),
		SuggestedLabel: "Device C",
		TargetFolder:   folderID,
	})
	if err != nil {
		t.Fatal(err)
	}

	appResC, err := ctrl.ApproveEnrollmentRequest(ctx, control.ApproveEnrollmentRequest{
		RequestID: subResC.RequestID,
		Folder:    folderID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if appResC.Revision != 3 {
		t.Fatalf("expected revision 3, got %d", appResC.Revision)
	}

	// Verify Rev 3 has PriorDigest = curApp2.Digest
	mem3, curApp3, err := db.GetMembership(ctx, folderID)
	if err != nil {
		t.Fatal(err)
	}
	if mem3.PriorDigest != curApp2.Digest {
		t.Fatalf("Rev 3 PriorDigest mismatch: got %x, want %x", mem3.PriorDigest, curApp2.Digest)
	}
	if len(mem3.Active) != 3 {
		t.Fatalf("active members count = %d, want 3", len(mem3.Active))
	}
	_ = curApp3
}

// TestOrbitMembership_CompetingAdministrationFork tests Invariant I24:
// two partitioned nodes approving different joins from base Revision N create an explicit fork
// that blocks silent merging until explicit owner reconciliation.
func TestOrbitMembership_CompetingAdministrationFork(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state-fork")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}

	var devA history.ID
	rand.Read(devA[:])
	_ = config.Save(stateDir, config.Config{FormatVersion: 1, DeviceID: hex.EncodeToString(devA[:]), CreatedAt: time.Now().UTC()})
	identA, _ := replication.LoadOrCreateIdentity(stateDir, devA, time.Now())

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := workspace.New(db, workspace.Options{})
	ctrl := control.New(db, ws, control.Options{LocalDevice: devA})

	var folderID history.ID
	rand.Read(folderID[:])
	_ = db.EnsureFolder(ctx, folderID, devA, 1)

	// Base Revision 1
	rev1 := protocol.Membership{
		Folder:      folderID,
		Revision:    1,
		PriorDigest: history.Digest{},
		Active: []protocol.ActiveMember{
			{Device: devA, KeyPin: identA.KeyPin},
		},
	}
	app1, err := db.ApproveMembership(ctx, rev1)
	if err != nil {
		t.Fatal(err)
	}

	// Branch 1: Node A approves Device B -> Revision 2A
	var devB history.ID
	var pinB history.Digest
	rand.Read(devB[:])
	rand.Read(pinB[:])

	rev2A := protocol.Membership{
		Folder:      folderID,
		Revision:    2,
		PriorDigest: app1.Digest,
		Active: []protocol.ActiveMember{
			{Device: devA, KeyPin: identA.KeyPin},
			{Device: devB, KeyPin: pinB},
		},
	}
	app2A, err := db.ApproveMembership(ctx, rev2A)
	if err != nil {
		t.Fatalf("Rev 2A approval failed: %v", err)
	}

	// Branch 2: Competing Revision 2B approved concurrently by a partitioned device
	var devC history.ID
	var pinC history.Digest
	rand.Read(devC[:])
	rand.Read(pinC[:])

	rev2B := protocol.Membership{
		Folder:      folderID,
		Revision:    2,
		PriorDigest: app1.Digest,
		Active: []protocol.ActiveMember{
			{Device: devA, KeyPin: identA.KeyPin},
			{Device: devC, KeyPin: pinC},
		},
	}

	// 1. DetectMembershipFork must catch competing revision
	forkErr := db.DetectMembershipFork(ctx, folderID, rev2B)
	if !errors.Is(forkErr, repository.ErrMembershipFork) {
		t.Fatalf("expected ErrMembershipFork, got: %v", forkErr)
	}

	// 2. ApproveMembership must also reject competing fork
	_, appForkErr := db.ApproveMembership(ctx, rev2B)
	if !errors.Is(appForkErr, repository.ErrMembershipFork) {
		t.Fatalf("expected ErrMembershipFork on ApproveMembership, got: %v", appForkErr)
	}

	// 3. Controller DetectMembershipFork API must return forked: true
	detectRes, err := ctrl.DetectMembershipFork(ctx, control.DetectForkRequest{
		Folder:    folderID,
		Candidate: rev2B,
	})
	if err != nil {
		t.Fatalf("DetectMembershipFork failed: %v", err)
	}
	if !detectRes.Forked {
		t.Fatal("expected forked = true in DetectMembershipFork result")
	}

	// 4. Explicit owner reconciliation flow:
	// Owner provides active members [devA, devB, devC] to unify the partition into Revision 3
	_ = db.SetDeviceDisplayName(ctx, devC, "Device C Reconciled")
	// Save devC pin into devices table so ReconcileMembership can look it up
	_, _ = db.ApproveMembership(ctx, protocol.Membership{
		Folder:      folderID,
		Revision:    2,
		PriorDigest: app1.Digest,
		Active:      rev2A.Active,
	}) // ensure devices table has devA and devB

	reconcileRes, err := ctrl.ReconcileMembership(ctx, control.ReconcileMembershipRequest{
		Folder: folderID,
		Active: []history.ID{devA, devB, devC},
	})
	if err != nil {
		t.Fatalf("ReconcileMembership failed: %v", err)
	}
	if reconcileRes.ApprovedRevision != 3 {
		t.Fatalf("expected reconciled revision 3, got %d", reconcileRes.ApprovedRevision)
	}

	// Check post-reconciliation state
	curMem, _, err := db.GetMembership(ctx, folderID)
	if err != nil {
		t.Fatal(err)
	}
	if curMem.Revision != 3 {
		t.Fatalf("current revision = %d, want 3", curMem.Revision)
	}
	if curMem.PriorDigest != app2A.Digest {
		t.Fatalf("prior digest = %x, want %x", curMem.PriorDigest, app2A.Digest)
	}
}

// TestOrbitMembership_OfflineCatchUp tests that an offline active peer reconnects,
// queries /peer/v1/membership/get, and catches up to the owner-approved revision.
// Also tests that retired devices are rejected with RETIRED_MEMBER.
func TestOrbitMembership_OfflineCatchUp(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)

	// Set up Node A (Owner / Online)
	dirA := filepath.Join(disposable, "node-a")
	_ = os.MkdirAll(dirA, 0o700)
	var devA history.ID
	rand.Read(devA[:])
	_ = config.Save(dirA, config.Config{FormatVersion: 1, DeviceID: hex.EncodeToString(devA[:]), CreatedAt: time.Now().UTC()})
	identA, _ := replication.LoadOrCreateIdentity(dirA, devA, time.Now())
	dbA, _ := repository.Open(ctx, dirA)
	defer dbA.Close()
	wsA := workspace.New(dbA, workspace.Options{})
	ctrlA := control.New(dbA, wsA, control.Options{LocalDevice: devA})

	// Set up Node B (Offline Peer)
	dirB := filepath.Join(disposable, "node-b")
	_ = os.MkdirAll(dirB, 0o700)
	var devB history.ID
	rand.Read(devB[:])
	_ = config.Save(dirB, config.Config{FormatVersion: 1, DeviceID: hex.EncodeToString(devB[:]), CreatedAt: time.Now().UTC()})
	identB, _ := replication.LoadOrCreateIdentity(dirB, devB, time.Now())
	dbB, _ := repository.Open(ctx, dirB)
	defer dbB.Close()
	wsB := workspace.New(dbB, workspace.Options{})
	ctrlB := control.New(dbB, wsB, control.Options{LocalDevice: devB})

	var folderID history.ID
	rand.Read(folderID[:])
	_ = dbA.EnsureFolder(ctx, folderID, devA, 1)
	_ = dbB.EnsureFolder(ctx, folderID, devB, 1)

	// Both nodes start agreeing on Revision 1: [devA, devB]
	rev1 := protocol.Membership{
		Folder:      folderID,
		Revision:    1,
		PriorDigest: history.Digest{},
		Active: []protocol.ActiveMember{
			{Device: devA, KeyPin: identA.KeyPin},
			{Device: devB, KeyPin: identB.KeyPin},
		},
	}
	app1A, err := dbA.ApproveMembership(ctx, rev1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = dbB.ApproveMembership(ctx, rev1)
	if err != nil {
		t.Fatal(err)
	}

	// Node A serves peer replication listener over TLS
	repServerA := replication.NewServer(dbA, identA)
	listenerA, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listenerA.Close()
	srvCtx, cancelSrv := context.WithCancel(ctx)
	defer cancelSrv()
	go func() { _ = repServerA.Serve(srvCtx, listenerA) }()

	// While Node B is offline, Node A approves Node C into Revision 2
	var devC history.ID
	var pinC history.Digest
	rand.Read(devC[:])
	rand.Read(pinC[:])
	rev2 := protocol.Membership{
		Folder:      folderID,
		Revision:    2,
		PriorDigest: app1A.Digest,
		Active: []protocol.ActiveMember{
			{Device: devA, KeyPin: identA.KeyPin},
			{Device: devB, KeyPin: identB.KeyPin},
			{Device: devC, KeyPin: pinC},
		},
	}
	_, err = dbA.ApproveMembership(ctx, rev2)
	if err != nil {
		t.Fatal(err)
	}

	// Node B comes online, connects to Node A using mutual TLS client, and queries /peer/v1/membership/get
	clientB, err := replication.NewClient("https://"+listenerA.Addr().String(), identB, identA.Leaf, identA.KeyPin)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	getResp, err := clientB.MembershipGet(ctx, replication.MembershipGetRequest{
		ProtocolVersion: replication.ProtocolVersion,
		FolderID:        hex.EncodeToString(folderID[:]),
		DeviceID:        hex.EncodeToString(devB[:]),
	})
	if err != nil {
		t.Fatalf("MembershipGet failed: %v", err)
	}
	if getResp.Membership.Revision != 2 {
		t.Fatalf("expected revision 2 from Node A, got %d", getResp.Membership.Revision)
	}

	// Node B installs the caught-up membership revision
	appCatchUp, err := ctrlB.CatchUpMembership(ctx, folderID, getResp.Membership, getResp.Snapshots...)
	if err != nil {
		t.Fatalf("CatchUpMembership failed: %v", err)
	}
	if appCatchUp.Revision != 2 {
		t.Fatalf("Node B caught up revision = %d, want 2", appCatchUp.Revision)
	}

	// Test Invariant I24: If a retired device queries /peer/v1/membership/get, it is forbidden
	// Retire Node B on Node A in Revision 3
	snapB := protocol.RetirementSnapshot{
		Folder:           folderID,
		ConfigurationRev: 3,
		RetiredDevice:    devB,
	}
	snapBDigest, _ := protocol.RetirementSnapshotDigest(snapB)
	rev3 := protocol.Membership{
		Folder:      folderID,
		Revision:    3,
		PriorDigest: appCatchUp.Digest,
		Active: []protocol.ActiveMember{
			{Device: devA, KeyPin: identA.KeyPin},
			{Device: devC, KeyPin: pinC},
		},
		Retired: []protocol.RetiredMember{
			{Device: devB, RetiredAt: 3, SnapshotDigest: snapBDigest},
		},
	}
	_, err = dbA.ApproveMembership(ctx, rev3, snapB)
	if err != nil {
		t.Fatal(err)
	}

	// Node B is now retired on Node A; querying membership update must return forbidden (Invariant I24)
	_, err = clientB.MembershipGet(ctx, replication.MembershipGetRequest{
		ProtocolVersion: replication.ProtocolVersion,
		FolderID:        hex.EncodeToString(folderID[:]),
		DeviceID:        hex.EncodeToString(devB[:]),
	})
	if err == nil {
		t.Fatal("expected error from MembershipGet when called by retired device")
	}
	_ = ctrlA
}

// TestOrbitMembership_RestartPreservesConsentAndRevisions tests that daemon shutdown
// and restart preserves all approved revisions, enrollment records, and product settings.
func TestOrbitMembership_RestartPreservesConsentAndRevisions(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state-restart")
	_ = os.MkdirAll(stateDir, 0o700)

	var devID history.ID
	rand.Read(devID[:])
	_ = config.Save(stateDir, config.Config{FormatVersion: 1, DeviceID: hex.EncodeToString(devID[:]), CreatedAt: time.Now().UTC()})
	ident, _ := replication.LoadOrCreateIdentity(stateDir, devID, time.Now())

	// Phase 1: Setup workspace and approve revisions
	db1, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	ws1 := workspace.New(db1, workspace.Options{})
	ctrl1 := control.New(db1, ws1, control.Options{LocalDevice: devID})

	var folderID history.ID
	rand.Read(folderID[:])
	_ = db1.EnsureFolder(ctx, folderID, devID, 1)

	rev1 := protocol.Membership{
		Folder:      folderID,
		Revision:    1,
		PriorDigest: history.Digest{},
		Active: []protocol.ActiveMember{
			{Device: devID, KeyPin: ident.KeyPin},
		},
	}
	app1, err := db1.ApproveMembership(ctx, rev1)
	if err != nil {
		t.Fatal(err)
	}

	// Add second device
	pub2, priv2, _ := ed25519.GenerateKey(rand.Reader)
	dev2 := sha256.Sum256(pub2)
	inv, _ := ctrl1.CreateInvitation(ctx, control.CreateInvitationRequest{Folder: folderID, TTLSecs: 3600, MaxUses: 1})
	chal := []byte("restart-test-chal")
	sig := ed25519.Sign(priv2, chal)

	subRes, err := ctrl1.SubmitEnrollmentRequest(ctx, control.SubmitJoinRequestPayload{
		Token:          inv.Token,
		JoiningDevice:  dev2,
		PublicKey:      hex.EncodeToString(pub2),
		Signature:      hex.EncodeToString(sig),
		Challenge:      hex.EncodeToString(chal),
		SuggestedLabel: "Device 2 Laptop",
		TargetFolder:   folderID,
	})
	if err != nil {
		t.Fatal(err)
	}

	appRes, err := ctrl1.ApproveEnrollmentRequest(ctx, control.ApproveEnrollmentRequest{
		RequestID: subRes.RequestID,
		Folder:    folderID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if appRes.Revision != 2 {
		t.Fatalf("expected rev 2, got %d", appRes.Revision)
	}

	// Update settings
	devLbl := "Owner Node"
	_, _ = ctrl1.UpdateSettings(ctx, control.UpdateSettingsRequest{DeviceLabel: &devLbl})

	// Close database and simulate restart
	if err := db1.Close(); err != nil {
		t.Fatal(err)
	}

	// Phase 2: Reopen after restart
	db2, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	ws2 := workspace.New(db2, workspace.Options{})
	ctrl2 := control.New(db2, ws2, control.Options{LocalDevice: devID})

	// Verify settings survived
	settings, err := ctrl2.GetSettings(ctx)
	if err != nil || settings.Settings.DeviceLabel != "Owner Node" {
		t.Fatalf("settings not preserved: %+v (err: %v)", settings, err)
	}

	// Verify membership survived
	memRestored, appRestored, err := db2.GetMembership(ctx, folderID)
	if err != nil {
		t.Fatalf("GetMembership failed after restart: %v", err)
	}
	if memRestored.Revision != 2 {
		t.Fatalf("restored revision = %d, want 2", memRestored.Revision)
	}
	if appRestored.Digest != appRes.Digest {
		t.Fatalf("restored digest = %x, want %x", appRestored.Digest, appRes.Digest)
	}
	if len(memRestored.Active) != 2 {
		t.Fatalf("restored active count = %d, want 2", len(memRestored.Active))
	}

	// Verify display name survived
	name, err := db2.GetDeviceDisplayName(ctx, dev2)
	if err != nil || name != "Device 2 Laptop" {
		t.Fatalf("device display name = %q, want 'Device 2 Laptop'", name)
	}

	_ = app1
}
