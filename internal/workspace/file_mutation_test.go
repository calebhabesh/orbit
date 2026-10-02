package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOrbitMutationWorkspace(t *testing.T) {
	ctx := context.Background()
	work, db, folder, root := testWorkspace(t)

	// 1. Import
	impRes, err := work.ImportFile(ctx, ImportRequest{
		Folder:      folder,
		Path:        "imported/doc.txt",
		Source:      strings.NewReader("sample workspace import"),
		Size:        uint64(len("sample workspace import")),
		OperationID: "op-ws-imp-1",
	})
	if err != nil {
		t.Fatalf("ImportFile failed: %v", err)
	}
	if !impRes.Completed {
		t.Fatal("expected import to complete")
	}
	content, err := os.ReadFile(filepath.Join(root, "imported", "doc.txt"))
	if err != nil || string(content) != "sample workspace import" {
		t.Fatalf("unexpected content on disk: %q, err: %v", string(content), err)
	}

	// 2. CreateDirectory
	mkdirRes, err := work.CreateDirectory(ctx, CreateDirRequest{
		Folder:      folder,
		Path:        "new_folder/sub",
		OperationID: "op-ws-mkdir-1",
	})
	if err != nil {
		t.Fatalf("CreateDirectory failed: %v", err)
	}
	if !mkdirRes.Completed {
		t.Fatal("expected CreateDirectory to complete")
	}
	st, err := os.Stat(filepath.Join(root, "new_folder", "sub"))
	if err != nil || !st.IsDir() {
		t.Fatalf("expected directory at new_folder/sub, err: %v", err)
	}

	// 3. Move
	moveRes, err := work.Move(ctx, MoveRequest{
		Folder:      folder,
		SourcePath:  "imported/doc.txt",
		DestPath:    "new_folder/sub/moved.txt",
		OperationID: "op-ws-move-1",
	})
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}
	if !moveRes.Completed || moveRes.SourceRetained {
		t.Fatalf("unexpected MoveResult: %+v", moveRes)
	}
	if _, err := os.Stat(filepath.Join(root, "imported", "doc.txt")); !os.IsNotExist(err) {
		t.Fatal("expected source to be unlinked")
	}
	movedContent, err := os.ReadFile(filepath.Join(root, "new_folder", "sub", "moved.txt"))
	if err != nil || string(movedContent) != "sample workspace import" {
		t.Fatalf("unexpected moved content: %q", string(movedContent))
	}

	// 4. Delete
	delRes, err := work.Delete(ctx, DeleteRequest{
		Folder:      folder,
		Path:        "new_folder",
		Recursive:   true,
		OperationID: "op-ws-del-1",
	})
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if !delRes.Completed || delRes.DeletedCount < 2 {
		t.Fatalf("unexpected DeleteResult: %+v", delRes)
	}
	if _, err := os.Stat(filepath.Join(root, "new_folder")); !os.IsNotExist(err) {
		t.Fatal("expected new_folder to be removed")
	}

	_ = db
}
