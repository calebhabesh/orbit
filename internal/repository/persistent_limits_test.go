package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/history"
)

func TestPersistentStorageBudgetSurvivesReopenAndPreservesCapturedVersion(t *testing.T) {
	ctx := context.Background()
	state := t.TempDir()
	limits := config.DefaultStorageLimits()
	limits.DataBudgetBytes = 1024 * 1024
	data, _ := json.Marshal(limits)
	if err := os.WriteFile(filepath.Join(state, "limits.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	db, err := Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	folder, author := history.ID{1}, history.ID{2}
	if err := db.EnsureFolder(ctx, folder, author, 1); err != nil {
		t.Fatal(err)
	}
	manifest, err := db.StoreFile(ctx, bytes.NewReader([]byte("protected prior capture")), false)
	if err != nil {
		t.Fatal(err)
	}
	first, err := db.CreateLocalVersion(ctx, LocalVersionRequest{Folder: folder, Path: "note", Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.StoreFile(ctx, bytes.NewReader(bytes.Repeat([]byte("X"), 1024*1024)), false)
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("persistent limit not applied: %v", err)
	}
	if err := db.VerifyVersionContent(ctx, first.ID); err != nil {
		t.Fatalf("prior captured version lost: %v", err)
	}
}
