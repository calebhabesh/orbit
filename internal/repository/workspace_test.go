package repository

import (
	"context"
	"testing"

	"github.com/calebhabesh/file-sync/internal/history"
)

func TestPublicationCannotCommitBeforeFilesystemPublished(t *testing.T) {
	ctx := context.Background()
	db := openTestRepository(t, Options{})
	folder, local, remote := repositoryID('F'), repositoryID('A'), repositoryID('B')
	if err := db.EnsureFolder(ctx, folder, local, 1); err != nil {
		t.Fatal(err)
	}
	envelope := history.Envelope{ID: history.VersionID{Folder: folder, Author: remote, Counter: 1}, Path: "empty", Vector: []history.ClockEntry{{Author: remote, Counter: 1}}, Kind: history.KindDirectory, AuthoredRevision: 1}
	if err := db.ImportMetadata(ctx, envelope); err != nil {
		t.Fatal(err)
	}
	publication := Publication{OperationID: "test-op", Folder: folder, Path: "empty", Intended: envelope.ID, Kind: history.KindDirectory}
	if err := db.PreparePublication(ctx, publication); err != nil {
		t.Fatal(err)
	}
	if err := db.CommitPublication(ctx, publication.OperationID); err == nil {
		t.Fatal("prepared journal committed an applied basis")
	}
	if applied, err := db.WorkingApplied(ctx, envelope.ID); err != nil || applied {
		t.Fatalf("applied=%v err=%v", applied, err)
	}
}
