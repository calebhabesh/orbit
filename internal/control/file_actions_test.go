package control

import (
	"context"
	"strings"
	"testing"
)

func TestOrbitMutationControl(t *testing.T) {
	ctx := context.Background()
	env := setupTestEnv(t)

	// 1. Controller ImportFile
	content := "controller import data"
	impRes, err := env.ctrl.ImportFile(ctx, ImportFileRequest{
		Folder:         env.folder,
		Path:           "docs/test.txt",
		IdempotencyKey: "ctrl-imp-1",
	}, strings.NewReader(content), uint64(len(content)))
	if err != nil {
		t.Fatalf("ImportFile failed: %v", err)
	}
	if !impRes.Completed {
		t.Fatal("expected import to complete")
	}

	// 2. Controller CreateDir
	mkdirRes, err := env.ctrl.CreateDir(ctx, CreateDirRequest{
		Folder:         env.folder,
		Path:           "projects/sub",
		IdempotencyKey: "ctrl-mkdir-1",
	})
	if err != nil {
		t.Fatalf("CreateDir failed: %v", err)
	}
	if !mkdirRes.Completed {
		t.Fatal("expected CreateDir to complete")
	}

	// 3. Controller MoveFile
	moveRes, err := env.ctrl.MoveFile(ctx, MoveFileRequest{
		Folder:         env.folder,
		SourcePath:     "docs/test.txt",
		DestPath:       "projects/sub/test.txt",
		IdempotencyKey: "ctrl-move-1",
	})
	if err != nil {
		t.Fatalf("MoveFile failed: %v", err)
	}
	if !moveRes.Completed || moveRes.SourceRetained {
		t.Fatalf("unexpected move result: %+v", moveRes)
	}

	// 4. Controller DeleteFile
	delRes, err := env.ctrl.DeleteFile(ctx, DeleteFileRequest{
		Folder:         env.folder,
		Path:           "projects",
		Recursive:      true,
		IdempotencyKey: "ctrl-del-1",
	})
	if err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}
	if !delRes.Completed || delRes.DeletedCount < 2 {
		t.Fatalf("unexpected delete result: %+v", delRes)
	}
}
