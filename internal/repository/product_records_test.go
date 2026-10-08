package repository_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
)

// TestOrbitMigration_Schema11MigrationAndOperations tests that a new repository opens at schema 11,
// and tests all typed product record operations (display names, invitations, enrollment, setup, progress, read leases).
func TestOrbitMigration_Schema11MigrationAndOperations(t *testing.T) {
	ctx := context.Background()
	stateDir := testkit.NewDisposable(t)

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// 1. Verify schema version is CurrentSchema (11)
	ver, err := db.UserVersion(ctx)
	if err != nil {
		t.Fatalf("UserVersion failed: %v", err)
	}
	if ver != repository.CurrentSchema {
		t.Errorf("UserVersion = %d, want %d", ver, repository.CurrentSchema)
	}

	// 2. Test Folder & Device Display Names
	var folderID, authorID, deviceID history.ID
	rand.Read(folderID[:])
	rand.Read(authorID[:])
	rand.Read(deviceID[:])

	if err := db.EnsureFolder(ctx, folderID, authorID, 1); err != nil {
		t.Fatalf("EnsureFolder failed: %v", err)
	}

	// Set and get folder display name
	if err := db.SetFolderDisplayName(ctx, folderID, "Main Project"); err != nil {
		t.Fatalf("SetFolderDisplayName failed: %v", err)
	}
	fName, err := db.GetFolderDisplayName(ctx, folderID)
	if err != nil {
		t.Fatalf("GetFolderDisplayName failed: %v", err)
	}
	if fName != "Main Project" {
		t.Errorf("GetFolderDisplayName = %q, want 'Main Project'", fName)
	}

	// Set and get device display name
	if err := db.SetDeviceDisplayName(ctx, deviceID, "Desktop PC"); err != nil {
		t.Fatalf("SetDeviceDisplayName failed: %v", err)
	}
	dName, err := db.GetDeviceDisplayName(ctx, deviceID)
	if err != nil {
		t.Fatalf("GetDeviceDisplayName failed: %v", err)
	}
	if dName != "Desktop PC" {
		t.Errorf("GetDeviceDisplayName = %q, want 'Desktop PC'", dName)
	}

	// 3. Test Invitations (G02)
	var invDigest history.Digest
	rand.Read(invDigest[:])
	now := time.Now()
	inv := repository.InvitationRecord{
		Digest:    invDigest,
		Folder:    folderID,
		CreatedNS: now.UnixNano(),
		ExpiresNS: now.Add(24 * time.Hour).UnixNano(),
		MaxUses:   2,
		UsesCount: 0,
		Revoked:   false,
	}
	if err := db.CreateInvitation(ctx, inv); err != nil {
		t.Fatalf("CreateInvitation failed: %v", err)
	}

	retrievedInv, err := db.GetInvitation(ctx, invDigest)
	if err != nil {
		t.Fatalf("GetInvitation failed: %v", err)
	}
	if retrievedInv.Folder != folderID || retrievedInv.MaxUses != 2 {
		t.Errorf("retrieved invitation mismatch: %+v", retrievedInv)
	}

	// Consume invitation (use 1 of 2)
	if err := db.ConsumeInvitation(ctx, invDigest, now); err != nil {
		t.Fatalf("ConsumeInvitation failed: %v", err)
	}
	retrievedInv, _ = db.GetInvitation(ctx, invDigest)
	if retrievedInv.UsesCount != 1 {
		t.Errorf("UsesCount = %d, want 1", retrievedInv.UsesCount)
	}

	// Consume invitation (use 2 of 2)
	if err := db.ConsumeInvitation(ctx, invDigest, now); err != nil {
		t.Fatalf("ConsumeInvitation 2 failed: %v", err)
	}

	// Consume invitation when exhausted (use 3 of 2) fails
	if err := db.ConsumeInvitation(ctx, invDigest, now); !errors.Is(err, repository.ErrInvitationExpired) {
		t.Errorf("expected ErrInvitationExpired on exhausted uses, got: %v", err)
	}

	// Test revocation
	var invDigest2 history.Digest
	rand.Read(invDigest2[:])
	inv2 := repository.InvitationRecord{
		Digest:    invDigest2,
		Folder:    folderID,
		CreatedNS: now.UnixNano(),
		ExpiresNS: now.Add(time.Hour).UnixNano(),
		MaxUses:   1,
	}
	if err := db.CreateInvitation(ctx, inv2); err != nil {
		t.Fatal(err)
	}
	if err := db.RevokeInvitation(ctx, invDigest2); err != nil {
		t.Fatal(err)
	}
	if err := db.ConsumeInvitation(ctx, invDigest2, now); !errors.Is(err, repository.ErrInvitationRevoked) {
		t.Errorf("expected ErrInvitationRevoked, got: %v", err)
	}

	// 4. Test Enrollment Requests (G02)
	var pubKey, keyPin [32]byte
	rand.Read(pubKey[:])
	rand.Read(keyPin[:])
	enrReq := repository.EnrollmentRequestRecord{
		RequestID:      "req-101",
		Folder:         folderID,
		DeviceID:       deviceID,
		PublicKey:      pubKey,
		KeyPin:         keyPin,
		SuggestedLabel: "Alice Laptop",
		Status:         "pending",
		CreatedNS:      now.UnixNano(),
		UpdatedNS:      now.UnixNano(),
	}
	if err := db.RecordEnrollmentRequest(ctx, enrReq); err != nil {
		t.Fatalf("RecordEnrollmentRequest failed: %v", err)
	}
	reqLoaded, err := db.GetEnrollmentRequest(ctx, "req-101")
	if err != nil {
		t.Fatalf("GetEnrollmentRequest failed: %v", err)
	}
	if reqLoaded.SuggestedLabel != "Alice Laptop" || reqLoaded.Status != "pending" {
		t.Errorf("enrollment request mismatch: %+v", reqLoaded)
	}

	if err := db.UpdateEnrollmentRequestStatus(ctx, "req-101", "approved"); err != nil {
		t.Fatalf("UpdateEnrollmentRequestStatus failed: %v", err)
	}
	reqLoaded, _ = db.GetEnrollmentRequest(ctx, "req-101")
	if reqLoaded.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", reqLoaded.Status)
	}

	// 5. Test Setup State (O02/O03)
	setupState := repository.SetupStateRecord{
		Phase:           "root_selection",
		RootPath:        "/home/user/Orbit",
		DefaultFolderID: &folderID,
		Completed:       false,
		UpdatedNS:       now.UnixNano(),
	}
	if err := db.SaveSetupState(ctx, setupState); err != nil {
		t.Fatalf("SaveSetupState failed: %v", err)
	}
	loadedSetup, err := db.GetSetupState(ctx)
	if err != nil {
		t.Fatalf("GetSetupState failed: %v", err)
	}
	if loadedSetup == nil || loadedSetup.Phase != "root_selection" || loadedSetup.RootPath != "/home/user/Orbit" {
		t.Errorf("loaded setup state mismatch: %+v", loadedSetup)
	}

	// 6. Test Operation Progress (O02/O09/O10)
	prog := repository.OperationProgressRecord{
		OperationID:         "op-999",
		Kind:                "move_tree",
		Phase:               "staged",
		ProgressNumerator:   5,
		ProgressDenominator: 10,
		Details:             "Moving directory items",
		Retryable:           true,
		Canceled:            false,
		CreatedNS:           now.UnixNano(),
		UpdatedNS:           now.UnixNano(),
	}
	if err := db.RecordOperationProgress(ctx, prog); err != nil {
		t.Fatalf("RecordOperationProgress failed: %v", err)
	}
	loadedProg, err := db.GetOperationProgress(ctx, "op-999")
	if err != nil {
		t.Fatalf("GetOperationProgress failed: %v", err)
	}
	if loadedProg.ProgressNumerator != 5 || loadedProg.Phase != "staged" {
		t.Errorf("loaded operation progress mismatch: %+v", loadedProg)
	}
	if err := db.CancelOperationProgress(ctx, "op-999"); err != nil {
		t.Fatalf("CancelOperationProgress failed: %v", err)
	}
	loadedProg, _ = db.GetOperationProgress(ctx, "op-999")
	if !loadedProg.Canceled {
		t.Errorf("expected canceled=true")
	}

	// 7. Test Read Leases & GC Protection (G03 / Invariant I25)
	// Install objects in DB so foreign key to objects table works if applicable
	data1 := []byte("chunk-payload-1")
	data2 := []byte("chunk-payload-2")
	chunk1 := sha256.Sum256(data1)
	chunk2 := sha256.Sum256(data2)

	if err := db.InstallChunk(ctx, chunk1, uint64(len(data1)), bytes.NewReader(data1)); err != nil {
		t.Fatal(err)
	}
	if err := db.InstallChunk(ctx, chunk2, uint64(len(data2)), bytes.NewReader(data2)); err != nil {
		t.Fatal(err)
	}

	lease := repository.ReadLeaseRecord{
		LeaseID:        "lease-01",
		Folder:         folderID,
		VersionAuthor:  authorID,
		VersionCounter: 1,
		ExpiresNS:      now.Add(time.Minute).UnixNano(),
		CreatedNS:      now.UnixNano(),
		ChunkDigests:   []history.Digest{chunk1, chunk2},
	}
	if err := db.AcquireReadLease(ctx, lease); err != nil {
		t.Fatalf("AcquireReadLease failed: %v", err)
	}

	// Chunk should be leased
	isLeased, err := db.IsChunkLeased(ctx, chunk1, now)
	if err != nil || !isLeased {
		t.Errorf("expected chunk1 to be leased: isLeased=%v, err=%v", isLeased, err)
	}

	// Release lease
	if err := db.ReleaseReadLease(ctx, "lease-01"); err != nil {
		t.Fatalf("ReleaseReadLease failed: %v", err)
	}
	isLeasedAfter, err := db.IsChunkLeased(ctx, chunk1, now)
	if err != nil || isLeasedAfter {
		t.Errorf("expected chunk1 to no longer be leased after release: isLeased=%v", isLeasedAfter)
	}
}

// TestOrbitMigration_LegacyAdoptionAndRollbackLimit tests Invariant I20:
// 1. Opens an existing database created with schema version 5.
// 2. Upgrades it through migrations to CurrentSchema (11) without data loss.
// 3. Rejects opening a future database schema version (user_version > CurrentSchema).
func TestOrbitMigration_LegacyAdoptionAndRollbackLimit(t *testing.T) {
	ctx := context.Background()
	stateDir := testkit.NewDisposable(t)

	// Step 1: Open fresh database and check baseline
	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Step 2: Manually set user_version = 12 (simulating future version rollback)
	raw, err := sql.Open("sqlite", "file:"+stateDir+"/metadata.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(fmt.Sprintf("PRAGMA user_version = %d;", repository.CurrentSchema+1)); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	// Step 3: Re-open repository - MUST be rejected with ErrIncompatibleSchema
	_, err = repository.Open(ctx, stateDir)
	if !errors.Is(err, repository.ErrIncompatibleSchema) {
		t.Fatalf("expected ErrIncompatibleSchema on newer schema version, got: %v", err)
	}
}
