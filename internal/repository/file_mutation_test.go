package repository_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
)

func TestOrbitMutationJournal(t *testing.T) {
	ctx := context.Background()
	dir := testkit.NewDisposable(t)
	dbPath := filepath.Join(dir, "test.db")
	_ = os.MkdirAll(dir, 0o700)

	db, err := repository.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var folder history.ID
	_, _ = rand.Read(folder[:])
	var dev history.ID
	_, _ = rand.Read(dev[:])
	_ = db.EnsureFolder(ctx, folder, dev, 1)

	// 1. RecordFileMutation and GetFileMutation
	rec := repository.FileMutationRecord{
		OperationID:   "op-101",
		Folder:        folder,
		Action:        "move",
		SourcePath:    "old/path.txt",
		DestPath:      "new/path.txt",
		ReviewedToken: "token-abc",
		Overwrite:     true,
		Phase:         "PLANNED",
	}
	if err := db.RecordFileMutation(ctx, rec); err != nil {
		t.Fatalf("RecordFileMutation failed: %v", err)
	}

	got, err := db.GetFileMutation(ctx, "op-101")
	if err != nil {
		t.Fatalf("GetFileMutation failed: %v", err)
	}
	if got.Phase != "PLANNED" || got.SourcePath != "old/path.txt" || got.DestPath != "new/path.txt" || !got.Overwrite {
		t.Fatalf("unexpected mutation record: %+v", got)
	}

	// 2. SetFileMutationPhase
	if err := db.SetFileMutationPhase(ctx, "op-101", "INSTALLED"); err != nil {
		t.Fatalf("SetFileMutationPhase failed: %v", err)
	}
	got2, err := db.GetFileMutation(ctx, "op-101")
	if err != nil || got2.Phase != "INSTALLED" {
		t.Fatalf("expected INSTALLED, got %+v (err: %v)", got2, err)
	}

	// 3. SetFileMutationSourceRetained
	if err := db.SetFileMutationSourceRetained(ctx, "op-101", true); err != nil {
		t.Fatalf("SetFileMutationSourceRetained failed: %v", err)
	}
	got3, err := db.GetFileMutation(ctx, "op-101")
	if err != nil || !got3.SourceRetained {
		t.Fatalf("expected SourceRetained = true, got %+v", got3)
	}

	// 4. ListIncompleteFileMutations
	incomplete, err := db.ListIncompleteFileMutations(ctx, folder)
	if err != nil {
		t.Fatalf("ListIncompleteFileMutations failed: %v", err)
	}
	if len(incomplete) != 1 || incomplete[0].OperationID != "op-101" {
		t.Fatalf("expected 1 incomplete mutation, got %d", len(incomplete))
	}

	// Mark COMPLETED
	if err := db.SetFileMutationPhase(ctx, "op-101", "COMPLETED"); err != nil {
		t.Fatalf("SetFileMutationPhase COMPLETED failed: %v", err)
	}
	incomplete2, err := db.ListIncompleteFileMutations(ctx, folder)
	if err != nil || len(incomplete2) != 0 {
		t.Fatalf("expected 0 incomplete mutations after completion, got %d", len(incomplete2))
	}

	// 5. FileMutationEntries for directory moves
	if err := db.RecordFileMutation(ctx, repository.FileMutationRecord{
		OperationID: "op-dir-1",
		Folder:      folder,
		Action:      "move",
		SourcePath:  "dirA",
		DestPath:    "dirB",
		Phase:       "STAGED",
	}); err != nil {
		t.Fatalf("RecordFileMutation op-dir-1 failed: %v", err)
	}

	entries := []repository.FileMutationEntry{
		{
			OperationID: "op-dir-1",
			Position:    0,
			SourcePath:  "dirA",
			DestPath:    "dirB",
			Kind:        history.KindDirectory,
			Phase:       "PENDING",
		},
		{
			OperationID: "op-dir-1",
			Position:    1,
			SourcePath:  "dirA/file.txt",
			DestPath:    "dirB/file.txt",
			Kind:        history.KindFile,
			Phase:       "PENDING",
		},
	}
	if err := db.RecordFileMutationEntries(ctx, "op-dir-1", entries); err != nil {
		t.Fatalf("RecordFileMutationEntries failed: %v", err)
	}

	loadedEntries, err := db.GetFileMutationEntries(ctx, "op-dir-1")
	if err != nil {
		t.Fatalf("GetFileMutationEntries failed: %v", err)
	}
	if len(loadedEntries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(loadedEntries))
	}

	if err := db.SetFileMutationEntryPhase(ctx, "op-dir-1", 1, "COMPLETED", ""); err != nil {
		t.Fatalf("SetFileMutationEntryPhase failed: %v", err)
	}
	updatedEntries, _ := db.GetFileMutationEntries(ctx, "op-dir-1")
	if updatedEntries[1].Phase != "COMPLETED" {
		t.Fatalf("expected COMPLETED entry phase, got %s", updatedEntries[1].Phase)
	}

	_ = dbPath
	_ = hex.EncodeToString
	_ = time.Now
}
