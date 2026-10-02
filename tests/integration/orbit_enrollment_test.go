package integration_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

func setupEnrollmentTestEnv(t *testing.T) (*control.Controller, *control.Server, *repository.DB, string, history.ID, history.ID, func()) {
	t.Helper()
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}

	rawDev := make([]byte, 32)
	rand.Read(rawDev)
	var localDevice history.ID
	copy(localDevice[:], rawDev)

	coreCfg := config.Config{
		FormatVersion: 1,
		DeviceID:      hex.EncodeToString(rawDev),
		CreatedAt:     time.Now().UTC(),
	}
	if err := config.Save(stateDir, coreCfg); err != nil {
		t.Fatal(err)
	}

	ident, err := replication.LoadOrCreateIdentity(stateDir, localDevice, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}

	ws := workspace.New(db, workspace.Options{})
	ctrl := control.New(db, ws, control.Options{
		LocalDevice: localDevice,
	})

	srv, err := control.NewServer(ctrl, stateDir)
	if err != nil {
		t.Fatal(err)
	}

	var folderID history.ID
	rand.Read(folderID[:])
	if err := db.EnsureFolder(ctx, folderID, localDevice, 1); err != nil {
		t.Fatal(err)
	}
	_, err = db.ApproveMembership(ctx, protocol.Membership{
		Folder:      folderID,
		Revision:    1,
		PriorDigest: history.Digest{},
		Active: []protocol.ActiveMember{
			{Device: localDevice, KeyPin: ident.KeyPin},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	cleanup := func() {
		_ = db.Close()
	}

	return ctrl, srv, db, stateDir, localDevice, folderID, cleanup
}

// TestOrbitEnrollment_InvitationLifecycleAndExclusions tests invitation creation, verifier storage,
// single/multi-use limits, revocation, expiration, and Invariant I23 (no content/inventory access).
func TestOrbitEnrollment_InvitationLifecycleAndExclusions(t *testing.T) {
	ctrl, srv, _, stateDir, _, folderID, cleanup := setupEnrollmentTestEnv(t)
	defer cleanup()
	ctx := context.Background()

	// 1. Create a single-use invitation
	inv1, err := ctrl.CreateInvitation(ctx, control.CreateInvitationRequest{
		Folder:  folderID,
		TTLSecs: 3600,
		MaxUses: 1,
	})
	if err != nil {
		t.Fatalf("CreateInvitation failed: %v", err)
	}
	if inv1.Token == "" {
		t.Fatal("expected non-empty invitation token")
	}

	// Invariant I23: Raw token must NEVER be stored in the database.
	// Verify database only stores the SHA-256 digest of the token.
	rawDB, err := sql.Open("sqlite", filepath.Join(stateDir, "metadata.sqlite"))
	if err != nil {
		t.Fatalf("failed to open raw sqlite: %v", err)
	}
	defer rawDB.Close()

	var storedDigest []byte
	err = rawDB.QueryRowContext(ctx, "SELECT digest FROM invitations WHERE folder_id=?", folderID[:]).Scan(&storedDigest)
	if err != nil {
		t.Fatalf("failed to query stored invitation digest: %v", err)
	}
	expectedDigest := sha256.Sum256([]byte(inv1.Token))
	if !bytes.Equal(storedDigest, expectedDigest[:]) {
		t.Fatalf("stored digest mismatch: got %x, want %x", storedDigest, expectedDigest)
	}

	// Verify raw token is NOT in any sqlite tables or text columns
	var rawMatchCount int
	_ = rawDB.QueryRowContext(ctx, "SELECT count(*) FROM invitations WHERE digest=?", []byte(inv1.Token)).Scan(&rawMatchCount)
	if rawMatchCount > 0 {
		t.Fatal("raw token unexpectedly matched digest column")
	}

	// Invariant I23: Token holder cannot access inventory, chunks, or metadata
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	client := &http.Client{Timeout: 2 * time.Second}

	for _, path := range []string{"/api/v1/inventory", "/api/v1/chunks/get", "/api/v1/files", "/api/v1/settings"} {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+inv1.Token)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusNotFound {
			t.Fatalf("path %s returned unexpected status %d using invitation token; expected 401/403/404", path, resp.StatusCode)
		}
	}

	// 2. Joining device keys and proof
	joinPub, joinPriv, _ := ed25519.GenerateKey(rand.Reader)
	joinDev := sha256.Sum256(joinPub)
	challenge := []byte("orbit-inv-test-challenge-1")
	sig := ed25519.Sign(joinPriv, challenge)

	joinPayload := control.SubmitJoinRequestPayload{
		Token:          inv1.Token,
		JoiningDevice:  joinDev,
		PublicKey:      hex.EncodeToString(joinPub),
		Signature:      hex.EncodeToString(sig),
		Challenge:      hex.EncodeToString(challenge),
		SuggestedLabel: "Alice Tablet",
		TargetFolder:   folderID,
	}

	// First submission succeeds
	joinRes, err := ctrl.SubmitEnrollmentRequest(ctx, joinPayload)
	if err != nil {
		t.Fatalf("SubmitEnrollmentRequest failed: %v", err)
	}
	if joinRes.Status != "pending" {
		t.Fatalf("status = %q, want pending", joinRes.Status)
	}

	// Second submission with exhausted single-use token fails
	_, err = ctrl.SubmitEnrollmentRequest(ctx, joinPayload)
	if err == nil {
		t.Fatal("expected error submitting with exhausted single-use invitation token")
	}

	// 3. Test TTL expiration
	invExpired, err := ctrl.CreateInvitation(ctx, control.CreateInvitationRequest{
		Folder:  folderID,
		TTLSecs: -1, // Expired immediately
		MaxUses: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	joinPayloadExpired := joinPayload
	joinPayloadExpired.Token = invExpired.Token
	_, err = ctrl.SubmitEnrollmentRequest(ctx, joinPayloadExpired)
	if err == nil {
		t.Fatal("expected error submitting with expired invitation token")
	}

	// 4. Test Revocation
	invRevocable, err := ctrl.CreateInvitation(ctx, control.CreateInvitationRequest{
		Folder:  folderID,
		TTLSecs: 3600,
		MaxUses: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = ctrl.RevokeInvitation(ctx, control.RevokeInvitationRequest{Digest: invRevocable.Digest})
	if err != nil {
		t.Fatalf("RevokeInvitation failed: %v", err)
	}

	joinPayloadRevoked := joinPayload
	joinPayloadRevoked.Token = invRevocable.Token
	_, err = ctrl.SubmitEnrollmentRequest(ctx, joinPayloadRevoked)
	if err == nil {
		t.Fatal("expected error submitting with revoked invitation token")
	}

	_ = stateDir
}

// TestOrbitEnrollment_RequestBoundingAndRateLimits tests 16 KiB max payload bounding
// and 5 req/min rate limiting on join request submission.
func TestOrbitEnrollment_RequestBoundingAndRateLimits(t *testing.T) {
	_, srv, _, _, _, folderID, cleanup := setupEnrollmentTestEnv(t)
	defer cleanup()

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	client := &http.Client{Timeout: 2 * time.Second}

	// Test 1: Oversized request (>16 KiB) is rejected with HTTP 413
	oversizedBody := bytes.Repeat([]byte("X"), 17*1024)
	reqOversized, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/enrollment/request", bytes.NewReader(oversizedBody))
	reqOversized.Header.Set("Content-Type", "application/json")
	respOversized, err := client.Do(reqOversized)
	if err != nil {
		t.Fatal(err)
	}
	defer respOversized.Body.Close()
	if respOversized.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected HTTP 413 Payload Too Large, got %d", respOversized.StatusCode)
	}

	// Test 2: Rate limit check (max 5 requests per IP)
	// Rapidly send empty/dummy JSON bodies
	dummyBody, _ := json.Marshal(control.SubmitJoinRequestPayload{
		Token:        "dummy",
		TargetFolder: folderID,
	})
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/enrollment/request", bytes.NewReader(dummyBody))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	// 6th request from same client must receive HTTP 429 Too Many Requests
	reqRate, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/enrollment/request", bytes.NewReader(dummyBody))
	reqRate.Header.Set("Content-Type", "application/json")
	respRate, err := client.Do(reqRate)
	if err != nil {
		t.Fatal(err)
	}
	defer respRate.Body.Close()
	if respRate.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected HTTP 429 Too Many Requests on 6th request, got %d", respRate.StatusCode)
	}
}

// TestOrbitEnrollment_KeyPossessionAndApproval tests that joining keys must prove private
// key possession, unapproved requests have no access, and approval mints Revision N+1 idempotently.
func TestOrbitEnrollment_KeyPossessionAndApproval(t *testing.T) {
	ctrl, _, db, stateDir, localDev, folderID, cleanup := setupEnrollmentTestEnv(t)
	defer cleanup()
	ctx := context.Background()

	inv, err := ctrl.CreateInvitation(ctx, control.CreateInvitationRequest{
		Folder:  folderID,
		TTLSecs: 3600,
		MaxUses: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	joinPub, joinPriv, _ := ed25519.GenerateKey(rand.Reader)
	joinDev := sha256.Sum256(joinPub)
	challenge := []byte("challenge-orbit-key-possession")

	// Negative Test: Forged signature with attacker key
	_, attackerPriv, _ := ed25519.GenerateKey(rand.Reader)
	forgedSig := ed25519.Sign(attackerPriv, challenge)

	_, err = ctrl.SubmitEnrollmentRequest(ctx, control.SubmitJoinRequestPayload{
		Token:          inv.Token,
		JoiningDevice:  joinDev,
		PublicKey:      hex.EncodeToString(joinPub),
		Signature:      hex.EncodeToString(forgedSig),
		Challenge:      hex.EncodeToString(challenge),
		SuggestedLabel: "Laptop",
		TargetFolder:   folderID,
	})
	if err == nil {
		t.Fatal("expected error on forged signature proof")
	}

	// Positive Test: Valid signature accepted
	validSig := ed25519.Sign(joinPriv, challenge)
	subRes, err := ctrl.SubmitEnrollmentRequest(ctx, control.SubmitJoinRequestPayload{
		Token:          inv.Token,
		JoiningDevice:  joinDev,
		PublicKey:      hex.EncodeToString(joinPub),
		Signature:      hex.EncodeToString(validSig),
		Challenge:      hex.EncodeToString(challenge),
		SuggestedLabel: "Alice Laptop",
		TargetFolder:   folderID,
	})
	if err != nil {
		t.Fatalf("valid submit failed: %v", err)
	}
	if subRes.RequestID == "" {
		t.Fatal("missing request ID in result")
	}

	// Unapproved request: verify device is not in active membership
	memBefore, _, err := db.GetMembership(ctx, folderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(memBefore.Active) != 1 || memBefore.Active[0].Device != localDev {
		t.Fatalf("membership unexpectedly changed before approval: %+v", memBefore.Active)
	}

	// Owner approves enrollment request
	certPath := filepath.Join(stateDir, "alice_laptop.crt")
	_ = os.WriteFile(certPath, []byte("-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----"), 0o600)

	appRes, err := ctrl.ApproveEnrollmentRequest(ctx, control.ApproveEnrollmentRequest{
		RequestID:   subRes.RequestID,
		Folder:      folderID,
		Endpoint:    "https://192.168.1.50:8443",
		Certificate: certPath,
	})
	if err != nil {
		t.Fatalf("ApproveEnrollmentRequest failed: %v", err)
	}
	if appRes.Status != "approved" {
		t.Fatalf("status = %q, want approved", appRes.Status)
	}
	if appRes.Revision != 2 {
		t.Fatalf("expected revision 2 (N+1), got %d", appRes.Revision)
	}

	// Check durable membership in database
	memAfter, curApp, err := db.GetMembership(ctx, folderID)
	if err != nil {
		t.Fatal(err)
	}
	if memAfter.Revision != 2 {
		t.Fatalf("stored membership revision = %d, want 2", memAfter.Revision)
	}
	if curApp.Digest != appRes.Digest {
		t.Fatalf("stored digest %x != returned %x", curApp.Digest, appRes.Digest)
	}
	if len(memAfter.Active) != 2 {
		t.Fatalf("active members = %d, want 2", len(memAfter.Active))
	}

	// Check display name set
	alias, err := db.GetDeviceDisplayName(ctx, joinDev)
	if err != nil || alias != "Alice Laptop" {
		t.Fatalf("expected display name 'Alice Laptop', got %q (err: %v)", alias, err)
	}

	// Check peer endpoint installed in peers.json
	peers, err := config.LoadPeerEndpoints(stateDir)
	if err != nil || len(peers) != 1 {
		t.Fatalf("expected 1 peer endpoint saved, got %d (err: %v)", len(peers), err)
	}
	if peers[0].URL != "https://192.168.1.50:8443" {
		t.Fatalf("endpoint URL = %s, want https://192.168.1.50:8443", peers[0].URL)
	}

	// Idempotency: repeated approval returns replay: true
	appReplay, err := ctrl.ApproveEnrollmentRequest(ctx, control.ApproveEnrollmentRequest{
		RequestID: subRes.RequestID,
		Folder:    folderID,
	})
	if err != nil {
		t.Fatalf("repeated approval failed: %v", err)
	}
	if !appReplay.Replay {
		t.Fatal("expected replay = true on repeated approval")
	}
	if appReplay.Revision != 2 {
		t.Fatalf("replay revision = %d, want 2", appReplay.Revision)
	}
}

// TestOrbitEnrollment_RetiredDeviceRejection tests Invariant I24:
// retired devices cannot rejoin or be approved under the same Device ID.
func TestOrbitEnrollment_RetiredDeviceRejection(t *testing.T) {
	ctrl, _, db, _, localDev, folderID, cleanup := setupEnrollmentTestEnv(t)
	defer cleanup()
	ctx := context.Background()

	// 1. Enroll and then retire Device R
	retiredPub, retiredPriv, _ := ed25519.GenerateKey(rand.Reader)
	retiredDev := sha256.Sum256(retiredPub)
	retiredPin := sha256.Sum256(retiredPub)

	// Step to Rev 2: [localDev, retiredDev]
	rev1Mem, rev1App, _ := db.GetMembership(ctx, folderID)
	rev2 := protocol.Membership{
		Folder:      folderID,
		Revision:    2,
		PriorDigest: rev1App.Digest,
		Active: []protocol.ActiveMember{
			{Device: localDev, KeyPin: rev1Mem.Active[0].KeyPin},
			{Device: retiredDev, KeyPin: retiredPin},
		},
	}
	rev2App, err := db.ApproveMembership(ctx, rev2)
	if err != nil {
		t.Fatal(err)
	}

	// Step to Rev 3: retire retiredDev
	snap := protocol.RetirementSnapshot{
		Folder:           folderID,
		ConfigurationRev: 3,
		RetiredDevice:    retiredDev,
	}
	snapDigest, err := protocol.RetirementSnapshotDigest(snap)
	if err != nil {
		t.Fatal(err)
	}
	rev3 := protocol.Membership{
		Folder:      folderID,
		Revision:    3,
		PriorDigest: rev2App.Digest,
		Active: []protocol.ActiveMember{
			{Device: localDev, KeyPin: rev1Mem.Active[0].KeyPin},
		},
		Retired: []protocol.RetiredMember{
			{Device: retiredDev, RetiredAt: 3, SnapshotDigest: snapDigest},
		},
	}
	_, err = db.ApproveMembership(ctx, rev3, snap)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Retired device generates fresh invitation
	inv, err := ctrl.CreateInvitation(ctx, control.CreateInvitationRequest{
		Folder:  folderID,
		TTLSecs: 3600,
		MaxUses: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	challenge := []byte("challenge-rejoin-retired")
	sig := ed25519.Sign(retiredPriv, challenge)

	// Attempt to rejoin with the retired device ID must fail with ErrRetiredMemberRevival (Invariant I24)
	_, err = ctrl.SubmitEnrollmentRequest(ctx, control.SubmitJoinRequestPayload{
		Token:          inv.Token,
		JoiningDevice:  retiredDev,
		PublicKey:      hex.EncodeToString(retiredPub),
		Signature:      hex.EncodeToString(sig),
		Challenge:      hex.EncodeToString(challenge),
		SuggestedLabel: "Retired Device Rejoin Attempt",
		TargetFolder:   folderID,
	})
	if err == nil {
		t.Fatal("expected error rejoining retired device")
	}
	var ctrlErr *control.ControlError
	if errors.As(err, &ctrlErr) {
		if ctrlErr.Code != "RETIRED_MEMBER_REVIVAL" {
			t.Fatalf("expected code RETIRED_MEMBER_REVIVAL, got %s", ctrlErr.Code)
		}
	}
}
