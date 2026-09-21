package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const CurrentSchema = 1

var ErrIncompatibleSchema = errors.New("metadata schema is newer than this binary")

type DB struct {
	db *sql.DB
}

func Open(ctx context.Context, stateDir string) (*DB, error) {
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

	dsn := (&url.URL{Scheme: "file", Path: path}).String() +
		"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open metadata database: %w", err)
	}
	// P00 deliberately serializes access so every connection has the configured
	// PRAGMAs. P03 will introduce the bounded reader/writer connection policy.
	sqlDB.SetMaxOpenConns(1)
	db := &DB{db: sqlDB}
	if err := db.migrate(ctx); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) Close() error {
	return db.db.Close()
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
		if _, err := tx.ExecContext(ctx, `
			CREATE TABLE installation_metadata (
				key TEXT PRIMARY KEY,
				value BLOB NOT NULL
			) STRICT;
			PRAGMA user_version = 1;
		`); err != nil {
			return err
		}
		return tx.Commit()
	},
}

func (db *DB) PutInstallationValue(ctx context.Context, key string, value []byte) error {
	_, err := db.db.ExecContext(ctx, `
		INSERT INTO installation_metadata (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (db *DB) InstallationValue(ctx context.Context, key string) ([]byte, error) {
	var value []byte
	err := db.db.QueryRowContext(ctx, "SELECT value FROM installation_metadata WHERE key = ?", key).Scan(&value)
	return value, err
}
