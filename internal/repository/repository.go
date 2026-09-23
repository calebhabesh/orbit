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

	_ "modernc.org/sqlite"
)

const CurrentSchema = 5

var ErrIncompatibleSchema = errors.New("metadata schema is newer than this binary")

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
)

type Options struct {
	FaultHook   FaultHook
	BudgetBytes uint64
}

type DB struct {
	db          *sql.DB
	stateDir    string
	hook        FaultHook
	budgetBytes uint64
	mu          sync.Mutex
}

func Open(ctx context.Context, stateDir string) (*DB, error) {
	return OpenWithOptions(ctx, stateDir, Options{})
}

func OpenWithOptions(ctx context.Context, stateDir string, options Options) (*DB, error) {
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

	dsn := (&url.URL{Scheme: "file", Path: path}).String() + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open metadata database: %w", err)
	}
	// The serialized connection is the mutation/reference boundary and ensures
	// every connection in use has the required DSN PRAGMAs.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	db := &DB{db: sqlDB, stateDir: stateDir, hook: options.FaultHook, budgetBytes: options.BudgetBytes}
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
	for pragma, want := range map[string]string{"foreign_keys": "1", "journal_mode": "wal", "synchronous": "2"} {
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
