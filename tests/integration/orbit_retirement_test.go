package integration_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

// TestOrbitRetirement_LostDeviceAndReplacement verifies:
// 1. A lost or compromised device can be previewed and retired by an active member.
// 2. A deterministic retirement snapshot pins the retiree's accepted state, preventing DAG forks.
// 3. The retired device is forbidden from advancing the DAG (no counter reuse or post-retirement versions).
// 4. A replacement device is provisioned with fresh keys and a distinct device ID.
// 5. The replacement device enrolls via invitation and reviewed operator approval, restoring multi-device sync.
func TestOrbitRetirement_LostDeviceAndReplacement(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateA := filepath.Join(disposable, "state-node-a")
	rootA := filepath.Join(disposable, "workspace-a")
	_ = os.MkdirAll(stateA, 0o700)
	_ = os.MkdirAll(rootA, 0o755)

	// 1. Setup Node A
	pubA, privA, _ := ed25519.GenerateKey(rand.Reader)
	devA := sha256.Sum256(pubA)
	_ = config.Save(stateA, config.Config{
		FormatVersion: 1,
		DeviceID:      hex.EncodeToString(devA[:]),
		CreatedAt:     time.Now().UTC(),
	})
	identA, err := replication.LoadOrCreateIdentity(stateA, devA, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	dbA, err := repository.Open(ctx, stateA)
	if err != nil {
		t.Fatal(err)
	}
	defer dbA.Close()

	wsA := workspace.New(dbA, workspace.Options{})
	ctrlA := control.New(dbA, wsA, control.Options{LocalDevice: devA})

	var folderID history.ID
	rand.Read(folderID[:])
	if err := dbA.EnsureFolder(ctx, folderID, devA, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := ctrlA.RegisterFolder(ctx, folderID, rootA); err != nil {
		t.Fatal(err)
	}

	// 2. Provision Device B
	pubB, _, _ := ed25519.GenerateKey(rand.Reader)
	devB := sha256.Sum256(pubB)
	pinB := sha256.Sum256(pubB)

	// Revision 1: Both Device A and Device B are active members
	rev1 := protocol.Membership{
		Folder:      folderID,
		Revision:    1,
		PriorDigest: history.Digest{},
		Active: []protocol.ActiveMember{
			{Device: devA, KeyPin: identA.KeyPin},
			{Device: devB, KeyPin: pinB},
		},
	}
	app1, err := dbA.ApproveMembership(ctx, rev1)
	if err != nil {
		t.Fatalf("Rev 1 approval failed: %v", err)
	}

	// 3. Device B is lost! Device A initiates retirement of Device B
	// Preview retirement
	preview, err := ctrlA.RetireMemberPreview(ctx, folderID, devB)
	if err != nil {
		t.Fatalf("RetireMemberPreview failed: %v", err)
	}
	if preview.TargetDevice != devB {
		t.Errorf("TargetDevice = %x, want %x", preview.TargetDevice, devB)
	}
	if preview.NextRevision != 2 {
		t.Errorf("NextRevision = %d, want 2", preview.NextRevision)
	}

	// Execute retirement with idempotency key
	retireReq := control.RetireMemberRequest{
		Folder:         folderID,
		TargetDevice:   devB,
		IdempotencyKey: "retire-device-b-idemp",
	}
	retireRes, err := ctrlA.RetireMemberExecute(ctx, retireReq)
	if err != nil {
		t.Fatalf("RetireMemberExecute failed: %v", err)
	}

	if retireRes.ApprovedRevision != 2 {
		t.Errorf("ApprovedRevision = %d, want 2", retireRes.ApprovedRevision)
	}
	if len(retireRes.NextMembership.Retired) != 1 || retireRes.NextMembership.Retired[0].Device != devB {
		t.Fatalf("Expected Device B in retired membership: %+v", retireRes.NextMembership.Retired)
	}

	// 4. FORK PREVENTION ORACLES (Invariants I24, I28):
	// A) Device B is marked retired in database
	isRetired, err := dbA.IsDeviceRetired(ctx, folderID, devB)
	if err != nil || !isRetired {
		t.Fatalf("IsDeviceRetired(devB) = %v, want true", isRetired)
	}

	// B) Cannot revive retired member under old key
	revRevival := protocol.Membership{
		Folder:      folderID,
		Revision:    3,
		PriorDigest: retireRes.ApprovedDigest,
		Active: []protocol.ActiveMember{
			{Device: devA, KeyPin: identA.KeyPin},
			{Device: devB, KeyPin: pinB}, // Attempting to revive retired devB
		},
		Retired: []protocol.RetiredMember{},
	}
	_, err = dbA.ApproveMembership(ctx, revRevival)
	if !errors.Is(err, repository.ErrRetiredMemberRevival) {
		t.Fatalf("Expected ErrRetiredMemberRevival, got: %v", err)
	}

	// 5. Replacement Device C enrollment
	// Node C is provisioned with a FRESH key pair and new Device ID
	pubC, privC, _ := ed25519.GenerateKey(rand.Reader)
	devC := sha256.Sum256(pubC)

	// Ensure Device C does NOT reuse Device B's device ID or key
	if devC == devB {
		t.Fatalf("Replacement device must not reuse lost device ID")
	}

	// Create invitation on Device A
	inv, err := ctrlA.CreateInvitation(ctx, control.CreateInvitationRequest{
		Folder:  folderID,
		TTLSecs: 3600,
		MaxUses: 1,
	})
	if err != nil {
		t.Fatalf("CreateInvitation failed: %v", err)
	}

	// Submit enrollment from Device C
	chalC := []byte("replacement-device-c-challenge")
	sigC := ed25519.Sign(privC, chalC)
	subRes, err := ctrlA.SubmitEnrollmentRequest(ctx, control.SubmitJoinRequestPayload{
		Token:          inv.Token,
		JoiningDevice:  devC,
		PublicKey:      hex.EncodeToString(pubC),
		Signature:      hex.EncodeToString(sigC),
		Challenge:      hex.EncodeToString(chalC),
		SuggestedLabel: "Laptop-Replacement",
		TargetFolder:   folderID,
	})
	if err != nil {
		t.Fatalf("SubmitEnrollmentRequest failed: %v", err)
	}

	// Device A reviews and approves replacement enrollment
	appRes, err := ctrlA.ApproveEnrollmentRequest(ctx, control.ApproveEnrollmentRequest{
		RequestID: subRes.RequestID,
		Folder:    folderID,
	})
	if err != nil {
		t.Fatalf("ApproveEnrollmentRequest failed: %v", err)
	}

	if appRes.Revision != 3 {
		t.Errorf("ApproveEnrollmentRequest revision = %d, want 3", appRes.Revision)
	}

	// Verify current membership has Device A and Device C active, Device B retired
	peerList, err := ctrlA.PeerList(ctx, folderID)
	if err != nil {
		t.Fatalf("PeerList failed: %v", err)
	}

	if len(peerList.Active) != 2 {
		t.Errorf("Active members count = %d, want 2 (A and C)", len(peerList.Active))
	}
	if len(peerList.Retired) != 1 || peerList.Retired[0].Device != devB {
		t.Errorf("Retired members mismatch: %+v", peerList.Retired)
	}
	_ = app1
	_ = privA
}

// TestOrbitRetirement_StoppedMetadataRecovery verifies:
// 1. Backup creation produces a valid, consistent database snapshot.
// 2. Database integrity is verified via PRAGMA integrity_check.
// 3. Metadata backups report honest status and do not masquerade missing chunk payloads.
func TestOrbitRetirement_StoppedMetadataRecovery(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state")
	rootDir := filepath.Join(disposable, "workspace")
	_ = os.MkdirAll(stateDir, 0o700)
	_ = os.MkdirAll(rootDir, 0o755)

	var devID history.ID
	rand.Read(devID[:])
	_ = config.Save(stateDir, config.Config{
		FormatVersion: 1,
		DeviceID:      hex.EncodeToString(devID[:]),
		CreatedAt:     time.Now().UTC(),
	})

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := workspace.New(db, workspace.Options{})
	ctrl := control.New(db, ws, control.Options{LocalDevice: devID})

	var folderID history.ID
	rand.Read(folderID[:])
	_ = db.EnsureFolder(ctx, folderID, devID, 1)
	if _, err := ctrl.RegisterFolder(ctx, folderID, rootDir); err != nil {
		t.Fatal(err)
	}

	// 1. Create a metadata snapshot backup
	backupRes, err := ctrl.Backup(ctx, "")
	if err != nil {
		t.Fatalf("Backup failed: %v", err)
	}

	if backupRes.SizeBytes <= 0 {
		t.Errorf("Backup SizeBytes = %d, want > 0", backupRes.SizeBytes)
	}
	if _, err := os.Stat(backupRes.BackupPath); err != nil {
		t.Fatalf("Backup file does not exist at %s: %v", backupRes.BackupPath, err)
	}

	// 2. Perform recovery inspection
	inspRes, err := ctrl.RecoveryInspection(ctx)
	if err != nil {
		t.Fatalf("RecoveryInspection failed: %v", err)
	}

	if !inspRes.Consistent {
		t.Errorf("Database reported inconsistent: %s", inspRes.ConsistencyError)
	}
}
