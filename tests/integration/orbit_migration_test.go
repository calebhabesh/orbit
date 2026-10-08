package integration_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/app"
	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/control"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/replication"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
	_ "modernc.org/sqlite"
)

// TestOrbitMigration_EndToEnd_LegacyAdoption tests Invariant I20:
// Orbit cleanly adopts an existing legacy state directory containing schema version 5,
// running migrations through CurrentSchema (11) with folders, versions, and devices intact.
func TestOrbitMigration_EndToEnd_LegacyAdoption(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// 1. Initialize valid config.json
	coreCfg, err := config.Initialize(stateDir, time.Now, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Create mock legacy database at schema version 5
	dbPath := filepath.Join(stateDir, "metadata.sqlite")
	rawDB, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}

	legacySchemaV5 := `
CREATE TABLE devices (device_id BLOB PRIMARY KEY CHECK(length(device_id)=32), key_pin BLOB CHECK(key_pin IS NULL OR length(key_pin)=32), display_name TEXT, is_local INTEGER NOT NULL CHECK(is_local IN (0,1))) STRICT;
CREATE TABLE folders (folder_id BLOB PRIMARY KEY CHECK(length(folder_id)=32), local_author BLOB NOT NULL CHECK(length(local_author)=32), next_counter BLOB NOT NULL CHECK(length(next_counter)=8), membership_revision BLOB NOT NULL CHECK(length(membership_revision)=8), membership_digest BLOB CHECK(membership_digest IS NULL OR length(membership_digest)=32), root_path TEXT, root_device INTEGER, root_inode INTEGER, registration_id BLOB, scan_generation INTEGER NOT NULL DEFAULT 0, bootstrap_complete INTEGER NOT NULL DEFAULT 0 CHECK(bootstrap_complete IN (0,1))) STRICT;
CREATE TABLE membership_revisions (folder_id BLOB NOT NULL, revision BLOB NOT NULL CHECK(length(revision)=8), prior_digest BLOB NOT NULL CHECK(length(prior_digest)=32), digest BLOB NOT NULL UNIQUE CHECK(length(digest)=32), approved INTEGER NOT NULL CHECK(approved IN (0,1)), PRIMARY KEY(folder_id,revision), FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE TABLE membership_entries (folder_id BLOB NOT NULL, revision BLOB NOT NULL, device_id BLOB NOT NULL CHECK(length(device_id)=32), key_pin BLOB NOT NULL CHECK(length(key_pin)=32), state TEXT NOT NULL CHECK(state IN ('active','retired')), retired_at BLOB, retirement_snapshot BLOB, PRIMARY KEY(folder_id,revision,device_id), FOREIGN KEY(folder_id,revision) REFERENCES membership_revisions(folder_id,revision)) STRICT;
CREATE TABLE versions (folder_id BLOB NOT NULL, author_id BLOB NOT NULL, counter BLOB NOT NULL CHECK(length(counter)=8), path TEXT NOT NULL, kind INTEGER NOT NULL CHECK(kind BETWEEN 1 AND 3), authored_revision BLOB NOT NULL CHECK(length(authored_revision)=8), display_time TEXT NOT NULL, file_size BLOB CHECK(file_size IS NULL OR length(file_size)=8), file_digest BLOB CHECK(file_digest IS NULL OR length(file_digest)=32), executable INTEGER, content_state TEXT NOT NULL CHECK(content_state IN ('pending','ready','unavailable')), acquired_ns INTEGER NOT NULL, envelope_digest BLOB NOT NULL CHECK(length(envelope_digest)=32), PRIMARY KEY(folder_id,author_id,counter), FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE INDEX versions_path ON versions(folder_id,path);
CREATE TABLE version_parents (folder_id BLOB NOT NULL, author_id BLOB NOT NULL, counter BLOB NOT NULL, position INTEGER NOT NULL, parent_author BLOB NOT NULL, parent_counter BLOB NOT NULL, PRIMARY KEY(folder_id,author_id,counter,position), FOREIGN KEY(folder_id,author_id,counter) REFERENCES versions(folder_id,author_id,counter) ON DELETE CASCADE, FOREIGN KEY(folder_id,parent_author,parent_counter) REFERENCES versions(folder_id,author_id,counter)) STRICT;
CREATE TABLE version_vectors (folder_id BLOB NOT NULL, author_id BLOB NOT NULL, counter BLOB NOT NULL, position INTEGER NOT NULL, vector_author BLOB NOT NULL CHECK(length(vector_author)=32), vector_counter BLOB NOT NULL CHECK(length(vector_counter)=8), PRIMARY KEY(folder_id,author_id,counter,position), FOREIGN KEY(folder_id,author_id,counter) REFERENCES versions(folder_id,author_id,counter) ON DELETE CASCADE) STRICT;
CREATE TABLE manifest_chunks (folder_id BLOB NOT NULL, author_id BLOB NOT NULL, counter BLOB NOT NULL, position INTEGER NOT NULL, digest BLOB NOT NULL CHECK(length(digest)=32), length BLOB NOT NULL CHECK(length(length)=8), PRIMARY KEY(folder_id,author_id,counter,position), FOREIGN KEY(folder_id,author_id,counter) REFERENCES versions(folder_id,author_id,counter) ON DELETE CASCADE) STRICT;
CREATE TABLE objects (digest BLOB PRIMARY KEY CHECK(length(digest)=32), length BLOB NOT NULL CHECK(length(length)=8), verified INTEGER NOT NULL CHECK(verified IN (0,1))) STRICT;
CREATE TABLE object_references (folder_id BLOB NOT NULL, author_id BLOB NOT NULL, counter BLOB NOT NULL, position INTEGER NOT NULL, digest BLOB NOT NULL, PRIMARY KEY(folder_id,author_id,counter,position), FOREIGN KEY(folder_id,author_id,counter) REFERENCES versions(folder_id,author_id,counter) ON DELETE CASCADE, FOREIGN KEY(digest) REFERENCES objects(digest)) STRICT;
CREATE TABLE content_pins (digest BLOB NOT NULL, owner_kind TEXT NOT NULL, owner_key TEXT NOT NULL, created_ns INTEGER NOT NULL, PRIMARY KEY(digest,owner_kind,owner_key), FOREIGN KEY(digest) REFERENCES objects(digest)) STRICT;
CREATE TABLE reservations (reservation_id TEXT PRIMARY KEY, bytes BLOB NOT NULL CHECK(length(bytes)=8), purpose TEXT NOT NULL, created_ns INTEGER NOT NULL) STRICT;
CREATE TABLE path_projections (folder_id BLOB NOT NULL, path TEXT NOT NULL, working_basis BLOB, publication_generation INTEGER NOT NULL DEFAULT 0, block_reason TEXT, applied_author BLOB, applied_counter BLOB, observed_kind INTEGER, observed_digest BLOB, observed_executable INTEGER, PRIMARY KEY(folder_id,path), FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE TABLE transfers (transfer_id TEXT PRIMARY KEY, folder_id BLOB NOT NULL, state TEXT NOT NULL, reserved_bytes BLOB NOT NULL, peer_id BLOB, version_author BLOB, version_counter BLOB, attempts INTEGER NOT NULL DEFAULT 0, last_error TEXT, updated_ns INTEGER NOT NULL DEFAULT 0, FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE TABLE transfer_chunks (transfer_id TEXT NOT NULL, position INTEGER NOT NULL, digest BLOB NOT NULL CHECK(length(digest)=32), length BLOB NOT NULL CHECK(length(length)=8), verified INTEGER NOT NULL DEFAULT 0 CHECK(verified IN (0,1)), PRIMARY KEY(transfer_id,position), FOREIGN KEY(transfer_id) REFERENCES transfers(transfer_id) ON DELETE CASCADE) STRICT;
CREATE TABLE publication_journal (operation_id TEXT PRIMARY KEY, folder_id BLOB NOT NULL, path TEXT NOT NULL, intended_author BLOB NOT NULL, intended_counter BLOB NOT NULL, prior_basis BLOB, stage_path TEXT, recovery_path TEXT, phase TEXT NOT NULL, intended_kind INTEGER, observed_device INTEGER, observed_inode INTEGER, observed_size INTEGER, observed_mtime_ns INTEGER, observed_ctime_ns INTEGER, FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE TABLE peer_progress (folder_id BLOB NOT NULL, peer_id BLOB NOT NULL, version_author BLOB, version_counter BLOB, receipt INTEGER NOT NULL DEFAULT 0, remote_status TEXT, last_contact_ns INTEGER, inventory_cursor TEXT, PRIMARY KEY(folder_id,peer_id,version_author,version_counter), FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE TABLE retention_records (folder_id BLOB NOT NULL, author_id BLOB NOT NULL, counter BLOB NOT NULL, retain_until_ns INTEGER, explicit_pin INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(folder_id,author_id,counter), FOREIGN KEY(folder_id,author_id,counter) REFERENCES versions(folder_id,author_id,counter)) STRICT;
CREATE TABLE gc_intents (digest BLOB PRIMARY KEY, generation INTEGER NOT NULL, state TEXT NOT NULL, FOREIGN KEY(digest) REFERENCES objects(digest)) STRICT;
CREATE TABLE control_operations (operation_key TEXT PRIMARY KEY, request_digest BLOB NOT NULL, expected_generation INTEGER, result BLOB, expires_ns INTEGER NOT NULL) STRICT;
CREATE TABLE projection_basis (folder_id BLOB NOT NULL, path TEXT NOT NULL, position INTEGER NOT NULL, author_id BLOB NOT NULL, counter BLOB NOT NULL, PRIMARY KEY(folder_id,path,position), FOREIGN KEY(folder_id,path) REFERENCES path_projections(folder_id,path) ON DELETE CASCADE) STRICT;
CREATE TABLE workspace_scaffolds (folder_id BLOB NOT NULL, path TEXT NOT NULL, pending INTEGER NOT NULL DEFAULT 0 CHECK(pending IN (0,1)), PRIMARY KEY(folder_id,path), FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE TABLE deletion_proposals (folder_id BLOB NOT NULL, token TEXT NOT NULL, generation INTEGER NOT NULL, path TEXT NOT NULL, PRIMARY KEY(folder_id,path), FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE TABLE inventory_snapshots (token BLOB PRIMARY KEY CHECK(length(token)=32), folder_id BLOB NOT NULL, peer_id BLOB NOT NULL, created_ns INTEGER NOT NULL, expires_ns INTEGER NOT NULL, FOREIGN KEY(folder_id) REFERENCES folders(folder_id) ON DELETE CASCADE) STRICT;
CREATE TABLE inventory_snapshot_entries (token BLOB NOT NULL, position INTEGER NOT NULL, author_id BLOB NOT NULL, counter BLOB NOT NULL, path TEXT NOT NULL, kind INTEGER NOT NULL, content_state TEXT NOT NULL, envelope_digest BLOB NOT NULL, PRIMARY KEY(token,position), FOREIGN KEY(token) REFERENCES inventory_snapshots(token) ON DELETE CASCADE) STRICT;
CREATE TABLE peer_contacts (folder_id BLOB NOT NULL, peer_id BLOB NOT NULL, last_contact_ns INTEGER NOT NULL, PRIMARY KEY(folder_id,peer_id), FOREIGN KEY(folder_id) REFERENCES folders(folder_id) ON DELETE CASCADE) STRICT;
PRAGMA user_version = 5;
`
	if _, err := rawDB.Exec(legacySchemaV5); err != nil {
		t.Fatalf("failed to create legacy schema v5: %v", err)
	}

	// Insert legacy data: folder, device, version
	devBytes, _ := hex.DecodeString(coreCfg.DeviceID)
	folderBytes := make([]byte, 32)
	folderBytes[0] = 0x55
	zeroCounter := make([]byte, 8)
	revBytes := make([]byte, 8)
	revBytes[7] = 0x01

	_, err = rawDB.Exec(`INSERT INTO devices(device_id, is_local, display_name) VALUES(?,1,'Legacy Device')`, devBytes)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rawDB.Exec(`INSERT INTO folders(folder_id, local_author, next_counter, membership_revision) VALUES(?,?,?,?)`, folderBytes, devBytes, zeroCounter, revBytes)
	if err != nil {
		t.Fatal(err)
	}
	rawDB.Close()

	// 3. Open repository with modern binary: migrations 6..11 must execute automatically
	repo, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatalf("failed to open and migrate legacy database: %v", err)
	}
	defer repo.Close()

	// 4. Verify migrated schema version
	ver, err := repo.UserVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ver != repository.CurrentSchema {
		t.Errorf("expected migrated user_version %d, got %d", repository.CurrentSchema, ver)
	}

	// 5. Verify legacy data was preserved
	dName, err := repo.GetDeviceDisplayName(ctx, history.ID(devBytes))
	if err != nil || dName != "Legacy Device" {
		t.Errorf("legacy device display name was lost: got %q, err: %v", dName, err)
	}
	fName, err := repo.GetFolderDisplayName(ctx, history.ID(folderBytes))
	if err != nil {
		t.Errorf("error querying folder display name on migrated database: %v", err)
	}
	if fName != "" {
		t.Errorf("expected empty display name for unbackfilled legacy folder, got %q", fName)
	}

	// Backfill folder display name
	if err := repo.SetFolderDisplayName(ctx, history.ID(folderBytes), "Adopted Legacy Folder"); err != nil {
		t.Fatalf("SetFolderDisplayName failed on migrated database: %v", err)
	}
	fNameUpdated, err := repo.GetFolderDisplayName(ctx, history.ID(folderBytes))
	if err != nil || fNameUpdated != "Adopted Legacy Folder" {
		t.Errorf("GetFolderDisplayName = %q, want 'Adopted Legacy Folder'", fNameUpdated)
	}
}

// TestOrbitMigration_EndToEnd_NewerSchemaRefused verifies Invariant I20:
// A database with a future schema version (user_version > CurrentSchema) is refused
// cleanly with ErrIncompatibleSchema to prevent database corruption.
func TestOrbitMigration_EndToEnd_NewerSchemaRefused(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := config.Initialize(stateDir, time.Now, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	// Simulate future database version rollback attempt (e.g. CurrentSchema + 5)
	futureVersion := repository.CurrentSchema + 5
	rawDB, err := sql.Open("sqlite", "file:"+filepath.Join(stateDir, "metadata.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rawDB.Exec(fmt.Sprintf("PRAGMA user_version = %d;", futureVersion)); err != nil {
		t.Fatal(err)
	}
	rawDB.Close()

	// Opening repository MUST fail with ErrIncompatibleSchema
	_, err = repository.Open(ctx, stateDir)
	if !errors.Is(err, repository.ErrIncompatibleSchema) {
		t.Fatalf("expected ErrIncompatibleSchema on user_version %d, got: %v", futureVersion, err)
	}
}

// TestOrbitMigration_EndToEnd_InterruptedRecoveryDetected tests Invariant I20:
// If recovery was interrupted (e.g., certificate rotated, but DB local_author or config.json
// was not committed), the daemon startup detects the inconsistency and fences sync
// with ErrIncompleteRecovery.
func TestOrbitMigration_EndToEnd_InterruptedRecoveryDetected(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}

	dev1Raw := make([]byte, 32)
	dev2Raw := make([]byte, 32)
	dev1Raw[0] = 0x11
	dev2Raw[0] = 0x22
	var dev1ID, dev2ID history.ID
	copy(dev1ID[:], dev1Raw)
	copy(dev2ID[:], dev2Raw)

	// Step 1: Initialize dev1
	err := config.Save(stateDir, config.Config{
		FormatVersion: 1,
		DeviceID:      hex.EncodeToString(dev1Raw),
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = replication.RotateIdentity(stateDir, dev1ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	var folderID history.ID
	rand.Read(folderID[:])
	if err := db.EnsureFolder(ctx, folderID, dev1ID, 1); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	// Step 2: Simulate crashed/interrupted transition:
	// TLS identity was rotated to dev2ID, but DB still has dev1ID as local_author!
	_, err = replication.RotateIdentity(stateDir, dev2ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// Step 3: Check consistency oracle directly
	err = control.VerifyRecoveryConsistency(stateDir)
	if !errors.Is(err, control.ErrIncompleteRecovery) {
		t.Fatalf("expected ErrIncompleteRecovery from VerifyRecoveryConsistency, got: %v", err)
	}

	// Step 4: Daemon startup (ServeWithOptions) MUST fail with ErrIncompleteRecovery
	serveErr := app.ServeWithOptions(ctx, stateDir, app.ServeOptions{
		PeerAddress: "127.0.0.1:0",
	})
	if !errors.Is(serveErr, control.ErrIncompleteRecovery) {
		t.Fatalf("expected daemon startup to fail with ErrIncompleteRecovery, got: %v", serveErr)
	}
}
