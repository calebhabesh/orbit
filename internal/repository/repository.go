// Package repository owns durable metadata and immutable managed content.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
	_ "modernc.org/sqlite"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/history"
)

const CurrentSchema = 13

var (
	ErrIncompatibleSchema     = errors.New("metadata schema is newer than this binary")
	ErrMetadataBudgetExceeded = errors.New("metadata budget exceeded")
	ErrStorageExhausted       = errors.New("filesystem free-space reserve exhausted")
	ErrGCIntentActive         = errors.New("GC intent is active on object")
	ErrCleanupSuspended       = errors.New("cleanup suspended due to active configuration change or pending maintenance")
	ErrContentCorrupt         = errors.New("content is corrupt and has been quarantined")
)

// FaultHook runs at a named durability boundary. It is nil in production.
type FaultHook func(name string) error

const (
	HookObjectFlushed       = "object.flushed"
	HookObjectInstalled     = "object.installed"
	HookObjectRecorded      = "object.recorded"
	HookBeforeVersionCommit = "sql.version.before_commit"
	HookAfterVersionCommit  = "sql.version.after_commit"
	HookBeforeReadyCommit   = "sql.readiness.before_commit"
	HookAfterReadyCommit    = "sql.readiness.after_commit"
	HookBeforeCheckpoint    = "sql.checkpoint.before"
	HookAfterCheckpoint     = "sql.checkpoint.after"
	HookPublicationPrepared = "publication.prepared"
	HookPublicationStaged   = "publication.staged"
	HookPublicationIntent   = "publication.intent"
	HookPublicationRenamed  = "publication.renamed"
	HookPublicationFlushed  = "publication.flushed"
	HookPublicationCommit   = "publication.committed"
	HookTransferProgress    = "transfer.progress"
	HookReceiptRecorded     = "receipt.recorded"
	HookGCIntent            = "gc.intent"
	HookGCUnlink            = "gc.unlink"
	HookGCFinalization      = "gc.finalization"
	HookChunkQuarantined    = "integrity.chunk.quarantined"
	HookIntegrityScanned    = "integrity.scanned"
	HookRepairInstalled     = "repair.installed"
)

type Options struct {
	FaultHook             FaultHook
	BudgetBytes           uint64
	MetadataBudgetBytes   uint64
	FreeSpaceReserveBytes uint64
}

type DB struct {
	db                    *sql.DB
	stateDir              string
	hook                  FaultHook
	budgetBytes           uint64
	metadataBudgetBytes   uint64
	freeSpaceReserveBytes uint64
	mu                    sync.Mutex
}

func Open(ctx context.Context, stateDir string) (*DB, error) {
	return OpenWithOptions(ctx, stateDir, Options{})
}

func OpenWithOptions(ctx context.Context, stateDir string, options Options) (*DB, error) {
	limits, err := config.LoadStorageLimits(stateDir)
	if err != nil {
		return nil, fmt.Errorf("load storage limits: %w", err)
	}
	if options.BudgetBytes == 0 {
		options.BudgetBytes = limits.DataBudgetBytes
	}
	if options.MetadataBudgetBytes == 0 {
		options.MetadataBudgetBytes = limits.MetadataBudgetBytes
	}
	if options.FreeSpaceReserveBytes == 0 {
		options.FreeSpaceReserveBytes = limits.FreeSpaceReserveBytes
	}
	for _, dir := range []string{"objects/sha256", "incoming", "quarantine", "operations"} {
		if err := os.MkdirAll(filepath.Join(stateDir, dir), 0o700); err != nil {
			return nil, fmt.Errorf("create repository directory %s: %w", dir, err)
		}
	}
	path := filepath.Join(stateDir, "metadata.sqlite")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create metadata database: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return nil, fmt.Errorf("secure metadata database: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close metadata database file: %w", err)
	}

	// The driver sorts _pragma values alphabetically. Setting journal_mode in
	// the DSN would therefore access WAL before locking_mode on reopened state.
	dsn := (&url.URL{Scheme: "file", Path: path}).String() + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=locking_mode(EXCLUSIVE)&_pragma=synchronous(FULL)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open metadata database: %w", err)
	}
	// Set EXCLUSIVE before the first WAL access: the existing single owner and
	// serialized connection can keep the volatile WAL index in heap memory.
	// This avoids mmap-backed -shm allocation faults when storage is exhausted.
	// Live readers use authenticated control; raw SQLite inspection requires stop.
	// The serialized connection is the mutation/reference boundary and ensures
	// every connection in use has the required DSN PRAGMAs.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	if _, err := sqlDB.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("enable metadata WAL: %w", err)
	}
	metadataBudget := options.MetadataBudgetBytes
	if metadataBudget == 0 {
		metadataBudget = 256 * 1024 * 1024
	}
	db := &DB{
		db:                    sqlDB,
		stateDir:              stateDir,
		hook:                  options.FaultHook,
		budgetBytes:           options.BudgetBytes,
		metadataBudgetBytes:   metadataBudget,
		freeSpaceReserveBytes: options.FreeSpaceReserveBytes,
	}
	if err := db.migrate(ctx); err != nil {
		sqlDB.Close()
		return nil, err
	}
	if err := db.verifyPragmas(ctx); err != nil {
		sqlDB.Close()
		return nil, err
	}
	if err := db.indexInstalledObjects(ctx); err != nil {
		sqlDB.Close()
		return nil, err
	}
	if err := db.reconcileGCIntents(ctx); err != nil {
		sqlDB.Close()
		return nil, err
	}
	if err := db.clearAbandonedReads(ctx); err != nil {
		sqlDB.Close()
		return nil, err
	}
	if _, err := db.PruneExpiredReadLeases(ctx, time.Now()); err != nil {
		sqlDB.Close()
		return nil, err
	}
	if _, err := db.RecoverInFlightDurableTasks(ctx); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) Close() error { return db.db.Close() }

func (db *DB) callHook(name string) error {
	if db.hook == nil {
		return nil
	}
	if err := db.hook(name); err != nil {
		return fmt.Errorf("fault hook %s: %w", name, err)
	}
	return nil
}

func (db *DB) verifyPragmas(ctx context.Context) error {
	for pragma, want := range map[string]string{"foreign_keys": "1", "locking_mode": "exclusive", "journal_mode": "wal", "synchronous": "2"} {
		var got string
		if err := db.db.QueryRowContext(ctx, "PRAGMA "+pragma).Scan(&got); err != nil {
			return fmt.Errorf("read SQLite %s: %w", pragma, err)
		}
		if strings.ToLower(got) != want {
			return fmt.Errorf("SQLite %s=%q, require %q", pragma, got, want)
		}
	}
	return nil
}

func (db *DB) migrate(ctx context.Context) error {
	var version int
	if err := db.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read metadata schema version: %w", err)
	}
	if version > CurrentSchema {
		return fmt.Errorf("%w: database=%d binary=%d", ErrIncompatibleSchema, version, CurrentSchema)
	}
	for version < CurrentSchema {
		next := version + 1
		if err := migrations[next](ctx, db.db); err != nil {
			return fmt.Errorf("apply metadata migration %d: %w", next, err)
		}
		version = next
	}
	return nil
}

// InitSchemaV5ForTest initializes an empty database with schema version 5 for legacy testing.
func InitSchemaV5ForTest(ctx context.Context, db *sql.DB) error {
	for v := 1; v <= 5; v++ {
		if err := migrations[v](ctx, db); err != nil {
			return err
		}
	}
	return nil
}

var migrations = map[int]func(context.Context, *sql.DB) error{
	1: func(ctx context.Context, db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `CREATE TABLE installation_metadata (key TEXT PRIMARY KEY, value BLOB NOT NULL) STRICT; PRAGMA user_version = 1;`); err != nil {
			return err
		}
		return tx.Commit()
	},
	2: func(ctx context.Context, db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, schemaV2); err != nil {
			return err
		}
		return tx.Commit()
	},
	3: func(ctx context.Context, db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, schemaV3); err != nil {
			return err
		}
		return tx.Commit()
	},
	4: func(ctx context.Context, db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, schemaV4); err != nil {
			return err
		}
		return tx.Commit()
	},
	5: func(ctx context.Context, db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, schemaV5); err != nil {
			return err
		}
		return tx.Commit()
	},
	6: func(ctx context.Context, db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, schemaV6); err != nil {
			return err
		}
		return tx.Commit()
	},
	7: func(ctx context.Context, db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, schemaV7); err != nil {
			return err
		}
		return tx.Commit()
	},
	8: func(ctx context.Context, db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, schemaV8); err != nil {
			return err
		}
		return tx.Commit()
	},
	9: func(ctx context.Context, db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, schemaV9); err != nil {
			return err
		}
		return tx.Commit()
	},
	10: func(ctx context.Context, db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, schemaV10); err != nil {
			return err
		}
		return tx.Commit()
	},
	11: func(ctx context.Context, db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, schemaV11); err != nil {
			return err
		}
		return tx.Commit()
	},
	12: func(ctx context.Context, db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `CREATE TABLE browse_generation (id INTEGER PRIMARY KEY CHECK(id=1), generation INTEGER NOT NULL) STRICT;
INSERT INTO browse_generation VALUES(1,1);
PRAGMA user_version=12;
CREATE INDEX read_lease_expiry ON read_leases(expires_ns);
CREATE INDEX version_parent_lookup ON version_parents(folder_id,parent_author,parent_counter);`); err != nil {
			return err
		}
		for _, table := range []string{"versions", "version_parents", "path_projections", "workspace_scaffolds", "folders"} {
			for _, action := range []string{"INSERT", "UPDATE", "DELETE"} {
				query := fmt.Sprintf("CREATE TRIGGER browse_%s_%s AFTER %s ON %s BEGIN UPDATE browse_generation SET generation=generation+1 WHERE id=1; END", table, action, action, table)
				if _, err := tx.ExecContext(ctx, query); err != nil {
					return err
				}
			}
		}
		return tx.Commit()
	},
	13: func(ctx context.Context, db *sql.DB) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, schemaV13); err != nil {
			return err
		}
		return tx.Commit()
	},
}

const schemaV2 = `
CREATE TABLE devices (device_id BLOB PRIMARY KEY CHECK(length(device_id)=32), key_pin BLOB CHECK(key_pin IS NULL OR length(key_pin)=32), display_name TEXT, is_local INTEGER NOT NULL CHECK(is_local IN (0,1))) STRICT;
CREATE TABLE folders (folder_id BLOB PRIMARY KEY CHECK(length(folder_id)=32), local_author BLOB NOT NULL CHECK(length(local_author)=32), next_counter BLOB NOT NULL CHECK(length(next_counter)=8), membership_revision BLOB NOT NULL CHECK(length(membership_revision)=8), membership_digest BLOB CHECK(membership_digest IS NULL OR length(membership_digest)=32), root_path TEXT, root_device INTEGER, root_inode INTEGER, registration_id BLOB) STRICT;
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
CREATE TABLE path_projections (folder_id BLOB NOT NULL, path TEXT NOT NULL, working_basis BLOB, publication_generation INTEGER NOT NULL DEFAULT 0, block_reason TEXT, applied_author BLOB, applied_counter BLOB, PRIMARY KEY(folder_id,path), FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE TABLE transfers (transfer_id TEXT PRIMARY KEY, folder_id BLOB NOT NULL, state TEXT NOT NULL, reserved_bytes BLOB NOT NULL, FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE TABLE publication_journal (operation_id TEXT PRIMARY KEY, folder_id BLOB NOT NULL, path TEXT NOT NULL, intended_author BLOB NOT NULL, intended_counter BLOB NOT NULL, prior_basis BLOB, stage_path TEXT, recovery_path TEXT, phase TEXT NOT NULL, FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE TABLE peer_progress (folder_id BLOB NOT NULL, peer_id BLOB NOT NULL, version_author BLOB, version_counter BLOB, receipt INTEGER NOT NULL DEFAULT 0, remote_status TEXT, last_contact_ns INTEGER, inventory_cursor TEXT, PRIMARY KEY(folder_id,peer_id,version_author,version_counter), FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE TABLE retention_records (folder_id BLOB NOT NULL, author_id BLOB NOT NULL, counter BLOB NOT NULL, retain_until_ns INTEGER, explicit_pin INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(folder_id,author_id,counter), FOREIGN KEY(folder_id,author_id,counter) REFERENCES versions(folder_id,author_id,counter)) STRICT;
CREATE TABLE gc_intents (digest BLOB PRIMARY KEY, generation INTEGER NOT NULL, state TEXT NOT NULL, FOREIGN KEY(digest) REFERENCES objects(digest)) STRICT;
CREATE TABLE control_operations (operation_key TEXT PRIMARY KEY, request_digest BLOB NOT NULL, expected_generation INTEGER, result BLOB, expires_ns INTEGER NOT NULL) STRICT;
PRAGMA user_version = 2;`

const schemaV3 = `
ALTER TABLE folders ADD COLUMN scan_generation INTEGER NOT NULL DEFAULT 0;
ALTER TABLE folders ADD COLUMN bootstrap_complete INTEGER NOT NULL DEFAULT 0 CHECK(bootstrap_complete IN (0,1));
ALTER TABLE path_projections ADD COLUMN observed_kind INTEGER CHECK(observed_kind IS NULL OR observed_kind BETWEEN 1 AND 3);
ALTER TABLE path_projections ADD COLUMN observed_digest BLOB CHECK(observed_digest IS NULL OR length(observed_digest)=32);
ALTER TABLE path_projections ADD COLUMN observed_executable INTEGER CHECK(observed_executable IS NULL OR observed_executable IN (0,1));
CREATE TABLE projection_basis (folder_id BLOB NOT NULL, path TEXT NOT NULL, position INTEGER NOT NULL, author_id BLOB NOT NULL CHECK(length(author_id)=32), counter BLOB NOT NULL CHECK(length(counter)=8), PRIMARY KEY(folder_id,path,position), FOREIGN KEY(folder_id,path) REFERENCES path_projections(folder_id,path) ON DELETE CASCADE, FOREIGN KEY(folder_id,author_id,counter) REFERENCES versions(folder_id,author_id,counter)) STRICT;
CREATE TABLE workspace_scaffolds (folder_id BLOB NOT NULL, path TEXT NOT NULL, pending INTEGER NOT NULL DEFAULT 0 CHECK(pending IN (0,1)), PRIMARY KEY(folder_id,path), FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
CREATE TABLE deletion_proposals (folder_id BLOB NOT NULL, token TEXT NOT NULL, generation INTEGER NOT NULL, path TEXT NOT NULL, PRIMARY KEY(folder_id,path), FOREIGN KEY(folder_id) REFERENCES folders(folder_id)) STRICT;
ALTER TABLE publication_journal ADD COLUMN intended_kind INTEGER CHECK(intended_kind IS NULL OR intended_kind BETWEEN 1 AND 3);
ALTER TABLE publication_journal ADD COLUMN observed_device INTEGER;
ALTER TABLE publication_journal ADD COLUMN observed_inode INTEGER;
ALTER TABLE publication_journal ADD COLUMN observed_size INTEGER;
ALTER TABLE publication_journal ADD COLUMN observed_mtime_ns INTEGER;
ALTER TABLE publication_journal ADD COLUMN observed_ctime_ns INTEGER;
PRAGMA user_version = 3;`

const schemaV4 = `
CREATE TABLE inventory_snapshots (
 token BLOB PRIMARY KEY CHECK(length(token)=32),
 folder_id BLOB NOT NULL CHECK(length(folder_id)=32),
 peer_id BLOB NOT NULL CHECK(length(peer_id)=32),
 created_ns INTEGER NOT NULL,
 expires_ns INTEGER NOT NULL,
 FOREIGN KEY(folder_id) REFERENCES folders(folder_id) ON DELETE CASCADE
) STRICT;
CREATE INDEX inventory_snapshots_owner ON inventory_snapshots(folder_id,peer_id,expires_ns);
CREATE TABLE inventory_snapshot_entries (
 token BLOB NOT NULL,
 position INTEGER NOT NULL,
 author_id BLOB NOT NULL CHECK(length(author_id)=32),
 counter BLOB NOT NULL CHECK(length(counter)=8),
 path TEXT NOT NULL,
 kind INTEGER NOT NULL CHECK(kind BETWEEN 1 AND 3),
 content_state TEXT NOT NULL CHECK(content_state IN ('pending','ready','unavailable')),
 envelope_digest BLOB NOT NULL CHECK(length(envelope_digest)=32),
 PRIMARY KEY(token,position),
 FOREIGN KEY(token) REFERENCES inventory_snapshots(token) ON DELETE CASCADE
) STRICT;
PRAGMA user_version = 4;`

const schemaV5 = `
ALTER TABLE transfers ADD COLUMN peer_id BLOB CHECK(peer_id IS NULL OR length(peer_id)=32);
ALTER TABLE transfers ADD COLUMN version_author BLOB CHECK(version_author IS NULL OR length(version_author)=32);
ALTER TABLE transfers ADD COLUMN version_counter BLOB CHECK(version_counter IS NULL OR length(version_counter)=8);
ALTER TABLE transfers ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE transfers ADD COLUMN last_error TEXT;
ALTER TABLE transfers ADD COLUMN updated_ns INTEGER NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX transfers_version ON transfers(folder_id,peer_id,version_author,version_counter);
CREATE TABLE transfer_chunks (
 transfer_id TEXT NOT NULL,
 position INTEGER NOT NULL,
 digest BLOB NOT NULL CHECK(length(digest)=32),
 length BLOB NOT NULL CHECK(length(length)=8),
 verified INTEGER NOT NULL DEFAULT 0 CHECK(verified IN (0,1)),
 PRIMARY KEY(transfer_id,position),
 FOREIGN KEY(transfer_id) REFERENCES transfers(transfer_id) ON DELETE CASCADE
) STRICT;
CREATE TABLE peer_contacts (
 folder_id BLOB NOT NULL CHECK(length(folder_id)=32),
 peer_id BLOB NOT NULL CHECK(length(peer_id)=32),
 last_contact_ns INTEGER NOT NULL,
 PRIMARY KEY(folder_id,peer_id),
 FOREIGN KEY(folder_id) REFERENCES folders(folder_id) ON DELETE CASCADE
) STRICT;
PRAGMA user_version = 5;`

const schemaV6 = `
CREATE TABLE retirement_snapshots (
 folder_id BLOB NOT NULL CHECK(length(folder_id)=32),
 revision BLOB NOT NULL CHECK(length(revision)=8),
 retired_device BLOB NOT NULL CHECK(length(retired_device)=32),
 snapshot_digest BLOB NOT NULL CHECK(length(snapshot_digest)=32),
 canonical_snapshot BLOB NOT NULL,
 PRIMARY KEY(folder_id, revision, retired_device),
 FOREIGN KEY(folder_id, revision) REFERENCES membership_revisions(folder_id, revision) ON DELETE CASCADE
) STRICT;
CREATE TABLE retirement_snapshot_entries (
 folder_id BLOB NOT NULL CHECK(length(folder_id)=32),
 revision BLOB NOT NULL CHECK(length(revision)=8),
 retired_device BLOB NOT NULL CHECK(length(retired_device)=32),
 counter BLOB NOT NULL CHECK(length(counter)=8),
 envelope_digest BLOB NOT NULL CHECK(length(envelope_digest)=32),
 PRIMARY KEY(folder_id, revision, retired_device, counter),
 FOREIGN KEY(folder_id, revision, retired_device) REFERENCES retirement_snapshots(folder_id, revision, retired_device) ON DELETE CASCADE
) STRICT;
CREATE TABLE resumable_maintenance (
 maintenance_id TEXT PRIMARY KEY,
 folder_id BLOB NOT NULL CHECK(length(folder_id)=32),
 target_device BLOB NOT NULL CHECK(length(target_device)=32),
 phase TEXT NOT NULL,
 state BLOB,
 updated_ns INTEGER NOT NULL,
 FOREIGN KEY(folder_id) REFERENCES folders(folder_id) ON DELETE CASCADE
) STRICT;
ALTER TABLE peer_progress ADD COLUMN direct INTEGER NOT NULL DEFAULT 1 CHECK(direct IN (0,1));
PRAGMA user_version = 6;`

const schemaV7 = `
CREATE TABLE folder_retention (
	folder_id BLOB PRIMARY KEY CHECK(length(folder_id)=32),
	retention_days INTEGER NOT NULL DEFAULT 30,
	min_superseded INTEGER NOT NULL DEFAULT 20,
	FOREIGN KEY(folder_id) REFERENCES folders(folder_id) ON DELETE CASCADE
) STRICT;
PRAGMA user_version = 7;`

const schemaV8 = `
CREATE TABLE quarantined_chunks (
	digest BLOB PRIMARY KEY CHECK(length(digest)=32),
	quarantine_path TEXT NOT NULL,
	length BLOB NOT NULL CHECK(length(length)=8),
	reason TEXT NOT NULL,
	quarantined_ns INTEGER NOT NULL,
	repaired INTEGER NOT NULL DEFAULT 0 CHECK(repaired IN (0,1))
) STRICT;
CREATE TABLE peer_integrity_incidents (
	peer_id BLOB NOT NULL CHECK(length(peer_id)=32),
	folder_id BLOB NOT NULL CHECK(length(folder_id)=32),
	chunk_digest BLOB NOT NULL CHECK(length(chunk_digest)=32),
	incident_ns INTEGER NOT NULL,
	error_reason TEXT NOT NULL,
	PRIMARY KEY(peer_id, folder_id, chunk_digest, incident_ns)
) STRICT;
PRAGMA user_version = 8;`

const schemaV9 = `
ALTER TABLE path_projections ADD COLUMN observed_size INTEGER;
ALTER TABLE path_projections ADD COLUMN observed_mtime_ns INTEGER;
ALTER TABLE path_projections ADD COLUMN observed_ctime_ns INTEGER;
ALTER TABLE path_projections ADD COLUMN observed_inode INTEGER;
ALTER TABLE path_projections ADD COLUMN last_scanned_ns INTEGER NOT NULL DEFAULT 0;
CREATE TABLE durable_work_tasks (
	task_id TEXT PRIMARY KEY,
	folder_id BLOB NOT NULL CHECK(length(folder_id)=32),
	task_kind TEXT NOT NULL CHECK(task_kind IN ('scan','sync','repair','gc')),
	peer_id BLOB CHECK(peer_id IS NULL OR length(peer_id)=32),
	target_path TEXT,
	version_author BLOB CHECK(version_author IS NULL OR length(version_author)=32),
	version_counter BLOB CHECK(version_counter IS NULL OR length(version_counter)=8),
	state TEXT NOT NULL CHECK(state IN ('queued','running','retry','exhausted','completed','canceled')),
	attempts INTEGER NOT NULL DEFAULT 0,
	max_attempts INTEGER NOT NULL DEFAULT 5,
	last_error TEXT,
	error_code TEXT,
	retry_after_ns INTEGER NOT NULL DEFAULT 0,
	created_ns INTEGER NOT NULL,
	updated_ns INTEGER NOT NULL,
	file_size INTEGER NOT NULL DEFAULT 0,
	age_counter INTEGER NOT NULL DEFAULT 0,
	FOREIGN KEY(folder_id) REFERENCES folders(folder_id) ON DELETE CASCADE
) STRICT;
CREATE INDEX durable_work_state ON durable_work_tasks(state, folder_id);
PRAGMA user_version = 9;`

const schemaV10 = `
ALTER TABLE folders ADD COLUMN paused INTEGER NOT NULL DEFAULT 0 CHECK(paused IN (0,1));
ALTER TABLE folders ADD COLUMN pause_reason TEXT;
CREATE TABLE event_logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	timestamp_ns INTEGER NOT NULL,
	operation_id TEXT NOT NULL,
	folder_id BLOB,
	version_author BLOB,
	version_counter BLOB,
	peer_id BLOB,
	phase TEXT NOT NULL,
	error_code TEXT,
	duration_ns INTEGER
) STRICT;
CREATE INDEX event_logs_ts ON event_logs(timestamp_ns);
PRAGMA user_version = 10;`

const schemaV11 = `
ALTER TABLE folders ADD COLUMN display_name TEXT;
CREATE TABLE invitations (
	digest BLOB PRIMARY KEY CHECK(length(digest)=32),
	folder_id BLOB NOT NULL CHECK(length(folder_id)=32),
	created_ns INTEGER NOT NULL,
	expires_ns INTEGER NOT NULL,
	max_uses INTEGER NOT NULL DEFAULT 1,
	uses_count INTEGER NOT NULL DEFAULT 0,
	revoked INTEGER NOT NULL DEFAULT 0 CHECK(revoked IN (0,1)),
	FOREIGN KEY(folder_id) REFERENCES folders(folder_id) ON DELETE CASCADE
) STRICT;
CREATE TABLE enrollment_requests (
	request_id TEXT PRIMARY KEY,
	folder_id BLOB NOT NULL CHECK(length(folder_id)=32),
	device_id BLOB NOT NULL CHECK(length(device_id)=32),
	public_key BLOB NOT NULL CHECK(length(public_key)=32),
	key_pin BLOB NOT NULL CHECK(length(key_pin)=32),
	suggested_label TEXT NOT NULL,
	status TEXT NOT NULL CHECK(status IN ('pending','approved','declined')),
	created_ns INTEGER NOT NULL,
	updated_ns INTEGER NOT NULL,
	FOREIGN KEY(folder_id) REFERENCES folders(folder_id) ON DELETE CASCADE
) STRICT;
CREATE INDEX enrollment_requests_folder ON enrollment_requests(folder_id, status);
CREATE TABLE setup_state (
	id INTEGER PRIMARY KEY CHECK(id=1),
	phase TEXT NOT NULL,
	root_path TEXT,
	default_folder_id BLOB CHECK(default_folder_id IS NULL OR length(default_folder_id)=32),
	completed INTEGER NOT NULL DEFAULT 0 CHECK(completed IN (0,1)),
	updated_ns INTEGER NOT NULL
) STRICT;
CREATE TABLE operation_progress (
	operation_id TEXT PRIMARY KEY,
	kind TEXT NOT NULL,
	phase TEXT NOT NULL,
	progress_numerator INTEGER NOT NULL DEFAULT 0,
	progress_denominator INTEGER NOT NULL DEFAULT 0,
	details TEXT,
	error_message TEXT,
	retryable INTEGER NOT NULL DEFAULT 0 CHECK(retryable IN (0,1)),
	canceled INTEGER NOT NULL DEFAULT 0 CHECK(canceled IN (0,1)),
	created_ns INTEGER NOT NULL,
	updated_ns INTEGER NOT NULL
) STRICT;
CREATE TABLE read_leases (
	lease_id TEXT PRIMARY KEY,
	folder_id BLOB NOT NULL CHECK(length(folder_id)=32),
	version_author BLOB NOT NULL CHECK(length(version_author)=32),
	version_counter BLOB NOT NULL CHECK(length(version_counter)=8),
	expires_ns INTEGER NOT NULL,
	created_ns INTEGER NOT NULL,
	FOREIGN KEY(folder_id) REFERENCES folders(folder_id) ON DELETE CASCADE
) STRICT;
CREATE TABLE read_lease_chunks (
	lease_id TEXT NOT NULL,
	chunk_digest BLOB NOT NULL CHECK(length(chunk_digest)=32),
	PRIMARY KEY(lease_id, chunk_digest),
	FOREIGN KEY(lease_id) REFERENCES read_leases(lease_id) ON DELETE CASCADE
) STRICT;
CREATE INDEX read_lease_chunks_digest ON read_lease_chunks(chunk_digest);
PRAGMA user_version = 11;`

const schemaV13 = `
CREATE TABLE file_mutations (
	operation_id TEXT PRIMARY KEY,
	folder_id BLOB NOT NULL CHECK(length(folder_id)=32),
	action TEXT NOT NULL CHECK(action IN ('import','mkdir','move','delete')),
	source_path TEXT,
	dest_path TEXT,
	reviewed_token TEXT,
	overwrite INTEGER NOT NULL DEFAULT 0 CHECK(overwrite IN (0,1)),
	phase TEXT NOT NULL CHECK(phase IN ('PLANNED','STAGED','INSTALLED','SOURCE_VERIFIED','COMPLETED','ABORTED')),
	stage_path TEXT,
	recovery_path TEXT,
	source_retained INTEGER NOT NULL DEFAULT 0 CHECK(source_retained IN (0,1)),
	details TEXT,
	created_ns INTEGER NOT NULL,
	updated_ns INTEGER NOT NULL,
	FOREIGN KEY(folder_id) REFERENCES folders(folder_id) ON DELETE CASCADE
) STRICT;
CREATE INDEX file_mutations_folder ON file_mutations(folder_id, phase);
CREATE TABLE file_mutation_entries (
	operation_id TEXT NOT NULL,
	position INTEGER NOT NULL,
	source_path TEXT NOT NULL,
	dest_path TEXT,
	kind INTEGER NOT NULL,
	phase TEXT NOT NULL CHECK(phase IN ('PENDING','COMPLETED','SKIPPED_RETAINED','FAILED')),
	error_message TEXT,
	PRIMARY KEY(operation_id, position),
	FOREIGN KEY(operation_id) REFERENCES file_mutations(operation_id) ON DELETE CASCADE
) STRICT;
PRAGMA user_version = 13;`

func (db *DB) reconcileGCIntents(ctx context.Context) error {
	rows, err := db.db.QueryContext(ctx, `SELECT digest, state FROM gc_intents`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type intentRow struct {
		digest history.Digest
		state  string
	}
	var list []intentRow
	for rows.Next() {
		var raw []byte
		var state string
		if err := rows.Scan(&raw, &state); err != nil {
			return err
		}
		var d history.Digest
		copy(d[:], raw)
		list = append(list, intentRow{digest: d, state: state})
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, item := range list {
		objPath := db.objectPath(item.digest)
		_, statErr := os.Lstat(objPath)
		if statErr == nil {
			// Object file still exists on disk.
			// D4: crash after intent retains extra bytes safely. Cancel intent.
			_, _ = db.db.ExecContext(ctx, `DELETE FROM gc_intents WHERE digest=?`, item.digest[:])
		} else if errors.Is(statErr, os.ErrNotExist) {
			// Object file was unlinked before crash.
			var refCount int
			_ = db.db.QueryRowContext(ctx, `SELECT count(*) FROM object_references WHERE digest=?`, item.digest[:]).Scan(&refCount)
			var pinCount int
			_ = db.db.QueryRowContext(ctx, `SELECT count(*) FROM content_pins WHERE digest=?`, item.digest[:]).Scan(&pinCount)
			if refCount == 0 && pinCount == 0 {
				// Safely finalize unreferenced object.
				_, _ = db.db.ExecContext(ctx, `DELETE FROM gc_intents WHERE digest=?`, item.digest[:])
				_, _ = db.db.ExecContext(ctx, `DELETE FROM objects WHERE digest=?`, item.digest[:])
			}
		}
	}
	return nil
}

func (db *DB) checkMetadataBudget(ctx context.Context) error {
	if db.metadataBudgetBytes == 0 {
		return nil
	}
	var size uint64
	for _, name := range []string{"metadata.sqlite", "metadata.sqlite-wal", "metadata.sqlite-shm"} {
		info, err := os.Stat(filepath.Join(db.stateDir, name))
		if err == nil && info.Mode().IsRegular() {
			size += uint64(info.Size())
		}
	}
	if size >= db.metadataBudgetBytes {
		return ErrMetadataBudgetExceeded
	}
	return nil
}

func (db *DB) checkFreeSpaceReserve(ctx context.Context) error {
	if db.freeSpaceReserveBytes == 0 {
		return nil
	}
	var stat unix.Statfs_t
	if err := unix.Statfs(db.stateDir, &stat); err == nil {
		available := stat.Bavail * uint64(stat.Bsize)
		if available < db.freeSpaceReserveBytes {
			return ErrStorageExhausted
		}
	}
	return nil
}

func (db *DB) PutInstallationValue(ctx context.Context, key string, value []byte) error {
	_, err := db.db.ExecContext(ctx, `INSERT INTO installation_metadata (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}
func (db *DB) InstallationValue(ctx context.Context, key string) ([]byte, error) {
	var value []byte
	err := db.db.QueryRowContext(ctx, `SELECT value FROM installation_metadata WHERE key=?`, key).Scan(&value)
	return value, err
}

func (db *DB) Checkpoint(ctx context.Context) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if err := db.callHook(HookBeforeCheckpoint); err != nil {
		return err
	}
	var busy, logFrames, checkpointed int
	if err := db.db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(FULL)").Scan(&busy, &logFrames, &checkpointed); err != nil {
		return fmt.Errorf("checkpoint WAL: %w", err)
	}
	if busy != 0 {
		return fmt.Errorf("checkpoint WAL: database busy (%d of %d frames checkpointed)", checkpointed, logFrames)
	}
	return db.callHook(HookAfterCheckpoint)
}

// Backup creates a consistent SQLite backup including committed WAL contents.
func (db *DB) Backup(ctx context.Context, destination string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("backup destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	quoted := strings.ReplaceAll(destination, "'", "''")
	if _, err := db.db.ExecContext(ctx, "VACUUM INTO '"+quoted+"'"); err != nil {
		return fmt.Errorf("create SQLite backup: %w", err)
	}
	return nil
}

func (db *DB) UserVersion(ctx context.Context) (int, error) {
	var version int
	if err := db.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

// ExecRaw executes a raw SQL statement on the underlying database connection.
func (db *DB) ExecRaw(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.db.ExecContext(ctx, query, args...)
}

// QueryRowRaw queries a single row on the underlying database connection.
func (db *DB) QueryRowRaw(ctx context.Context, query string, args ...any) *sql.Row {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.db.QueryRowContext(ctx, query, args...)
}
