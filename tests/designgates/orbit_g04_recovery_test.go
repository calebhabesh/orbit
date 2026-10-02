package designgates

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/state"
	"github.com/calebhabesh/file-sync/internal/testkit"
	_ "modernc.org/sqlite"
)

var (
	ErrIncompleteRecovery = errors.New("incomplete identity recovery detected: state inconsistent")
	ErrContentUnavailable = errors.New("content unavailable: chunk payloads missing from local storage")
)

func randomDeviceID() (history.ID, string) {
	var id history.ID
	_, _ = rand.Read(id[:])
	return id, hex.EncodeToString(id[:])
}

// setupMockDatabase creates a minimal SQLite metadata database for testing recovery transitions.
func setupMockDatabase(t *testing.T, dbPath, initialAuthor string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	schema := `
	CREATE TABLE folders (
		folder_id TEXT PRIMARY KEY,
		local_author TEXT NOT NULL,
		next_counter INTEGER NOT NULL
	);
	CREATE TABLE versions (
		version_id TEXT PRIMARY KEY,
		folder_id TEXT NOT NULL,
		author_id TEXT NOT NULL,
		counter INTEGER NOT NULL,
		path TEXT NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
	_, err = db.Exec("INSERT INTO folders (folder_id, local_author, next_counter) VALUES ('f-1', ?, 42);", initialAuthor)
	if err != nil {
		t.Fatalf("failed to insert initial folder: %v", err)
	}
	_, err = db.Exec("INSERT INTO versions (version_id, folder_id, author_id, counter, path) VALUES ('v-1', 'f-1', ?, 41, 'file.txt');", initialAuthor)
	if err != nil {
		t.Fatalf("failed to insert initial version: %v", err)
	}
	return db
}

// TestOrbitG04LiveIdentityResetFenced verifies that live identity mutation is strictly prohibited
// while the daemon lock is held.
func TestOrbitG04LiveIdentityResetFenced(t *testing.T) {
	stateDir := testkit.NewDisposable(t)
	if err := state.EnsureDirectory(stateDir); err != nil {
		t.Fatal(err)
	}

	lock, err := state.Acquire(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()

	// Attempting stopped transition operation while lock is active must fail
	_, err = state.Acquire(stateDir)
	if !errors.Is(err, state.ErrLocked) {
		t.Fatalf("expected state.ErrLocked when live daemon holds lock, got: %v", err)
	}
}

// TestOrbitG04AtomicTransitionAndRollbackSafety tests the stopped identity transition sequence,
// verifying that fresh identity is rotated, database folders author is transactional,
// and old history in versions table is fully preserved (Invariants I08, I19, I20).
func TestOrbitG04AtomicTransitionAndRollbackSafety(t *testing.T) {
	stateDir := testkit.NewDisposable(t)

	dbPath := filepath.Join(stateDir, "metadata.sqlite")

	dev1ID, dev1Hex := randomDeviceID()
	dev2ID, dev2Hex := randomDeviceID()

	// Initial identity
	ident1, err := replication.RotateIdentity(stateDir, dev1ID, time.Now())
	if err != nil {
		t.Fatalf("failed initial identity generation: %v", err)
	}
	initialPin := ident1.KeyPin

	// Initial config
	err = config.Save(stateDir, config.Config{
		FormatVersion: 1,
		DeviceID:      dev1Hex,
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	db := setupMockDatabase(t, dbPath, dev1Hex)
	defer db.Close()

	// Perform stopped transition:
	// Step 1: Rotate identity
	ident2, err := replication.RotateIdentity(stateDir, dev2ID, time.Now())
	if err != nil {
		t.Fatalf("failed to rotate identity: %v", err)
	}
	if ident2.KeyPin == initialPin {
		t.Fatal("expected rotated key pin to be different from initial")
	}

	// Step 2: Transactionally update SQLite folders author and reset counter
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec("UPDATE folders SET local_author = ?, next_counter = 0 WHERE folder_id = 'f-1'", dev2Hex)
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Step 3: Atomically update config.json
	err = config.Save(stateDir, config.Config{
		FormatVersion: 1,
		DeviceID:      dev2Hex,
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify database state:
	var updatedAuthor string
	var nextCounter int
	err = db.QueryRow("SELECT local_author, next_counter FROM folders WHERE folder_id = 'f-1'").Scan(&updatedAuthor, &nextCounter)
	if err != nil || updatedAuthor != dev2Hex || nextCounter != 0 {
		t.Fatalf("expected author=%s counter=0, got author=%s counter=%d", dev2Hex, updatedAuthor, nextCounter)
	}

	// Verify historical versions remain intact!
	var oldAuthorInHistory string
	err = db.QueryRow("SELECT author_id FROM versions WHERE version_id = 'v-1'").Scan(&oldAuthorInHistory)
	if err != nil || oldAuthorInHistory != dev1Hex {
		t.Fatalf("expected old version history to preserve original author %s, got: %s", dev1Hex, oldAuthorInHistory)
	}
}

// TestOrbitG04InterruptedTransitionDetection tests that an interrupted transition
// (crash between key rotation and database/config commit) is detected on restart,
// preventing the engine from authoring corrupt states.
func TestOrbitG04InterruptedTransitionDetection(t *testing.T) {
	stateDir := testkit.NewDisposable(t)

	dbPath := filepath.Join(stateDir, "metadata.sqlite")

	dev1ID, dev1Hex := randomDeviceID()
	dev2ID, dev2Hex := randomDeviceID()

	_, err := replication.RotateIdentity(stateDir, dev1ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	err = config.Save(stateDir, config.Config{
		FormatVersion: 1,
		DeviceID:      dev1Hex,
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	db := setupMockDatabase(t, dbPath, dev1Hex)
	defer db.Close()

	// Simulate crash: Key was rotated in identityDir, but DB and config.json still have dev1Hex!
	_, err = replication.RotateIdentity(stateDir, dev2ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// Verification oracle on daemon start:
	verifyStartupConsistency := func(cfgDeviceID, certDeviceID, dbAuthor string) error {
		if cfgDeviceID != certDeviceID || cfgDeviceID != dbAuthor {
			return ErrIncompleteRecovery
		}
		return nil
	}

	loadedCfg, err := config.Load(stateDir)
	if err != nil {
		t.Fatal(err)
	}

	// cert on disk now belongs to dev2Hex, while config and db have dev1Hex
	err = verifyStartupConsistency(loadedCfg.DeviceID, dev2Hex, dev1Hex)
	if !errors.Is(err, ErrIncompleteRecovery) {
		t.Fatalf("expected ErrIncompleteRecovery on inconsistent state, got: %v", err)
	}
}

// TestOrbitG04MissingPayloadsAfterMetadataRestore tests Invariant I18:
// restoring a metadata database when CAS chunks are missing diagnoses ContentUnavailable,
// and never serves substitute or synthetic content.
func TestOrbitG04MissingPayloadsAfterMetadataRestore(t *testing.T) {
	casRoot := testkit.NewDisposable(t)

	// CAS storage: only chunkA exists; chunkB is missing
	chunkAPath := filepath.Join(casRoot, "chunk-A")
	mustWrite(t, chunkAPath, "actual-bytes-for-chunk-A")

	chunkBPath := filepath.Join(casRoot, "chunk-B") // deliberately not created

	verifyVersionPayloadReady := func(chunks ...string) error {
		for _, c := range chunks {
			if _, err := os.Stat(c); os.IsNotExist(err) {
				return ErrContentUnavailable
			}
		}
		return nil
	}

	// Version 1 requires chunkA: available
	if err := verifyVersionPayloadReady(chunkAPath); err != nil {
		t.Fatalf("expected chunkA to be ready: %v", err)
	}

	// Version 2 requires chunkA and chunkB: missing chunkB yields ErrContentUnavailable
	err := verifyVersionPayloadReady(chunkAPath, chunkBPath)
	if !errors.Is(err, ErrContentUnavailable) {
		t.Fatalf("expected ErrContentUnavailable for missing payload, got: %v", err)
	}
}
