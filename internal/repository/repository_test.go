package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSQLitePersistsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	first, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.PutInstallationValue(ctx, "restart-probe", []byte("durable")); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	value, err := second.InstallationValue(ctx, "restart-probe")
	if err != nil {
		t.Fatal(err)
	}
	if string(value) != "durable" {
		t.Fatalf("persisted value = %q, want durable", value)
	}
}

func TestOpenRefusesNewerSchema(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.sqlite")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", CurrentSchema+1)); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = Open(ctx, dir)
	if !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("Open error = %v, want ErrIncompatibleSchema", err)
	}
}

func TestOpenPinsRequiredPragmas(t *testing.T) {
	db, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for pragma, want := range map[string]string{
		"foreign_keys": "1",
		"journal_mode": "wal",
		"synchronous":  "2",
	} {
		var got string
		if err := db.db.QueryRow("PRAGMA " + pragma).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("PRAGMA %s = %q, want %q", pragma, got, want)
		}
	}
}
