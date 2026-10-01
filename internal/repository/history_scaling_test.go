package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/calebhabesh/file-sync/internal/history"
)

func BenchmarkCaptureDistinctPaths(b *testing.B) {
	for i := 0; i < b.N; i++ {
		db, err := Open(context.Background(), b.TempDir())
		if err != nil {
			b.Fatal(err)
		}
		folder, author := repositoryID('F'), repositoryID('A')
		if err := db.EnsureFolder(context.Background(), folder, author, 1); err != nil {
			b.Fatal(err)
		}
		for path := 0; path < 1000; path++ {
			_, err := db.CreateLocalVersion(context.Background(), LocalVersionRequest{Folder: folder, Path: fmt.Sprintf("directory-%04d", path), Kind: history.KindDirectory, AuthoredRevision: 1})
			if err != nil {
				b.Fatal(err)
			}
		}
		if err := db.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func TestImmutableIDCannotMoveToAnotherPath(t *testing.T) {
	db := openTestRepository(t, Options{})
	folder, author := repositoryID('F'), repositoryID('A')
	if err := db.EnsureFolder(context.Background(), folder, author, 1); err != nil {
		t.Fatal(err)
	}
	envelope, err := db.CreateLocalVersion(context.Background(), LocalVersionRequest{Folder: folder, Path: "first", Kind: history.KindDirectory, AuthoredRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	envelope.Path = "second"
	if err := db.ImportMetadata(context.Background(), envelope); !errors.Is(err, history.ErrDuplicateID) {
		t.Fatalf("cross-path identity reuse: %v", err)
	}
}
