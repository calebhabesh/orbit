package control_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/testkit"
)

func randomFolderID(t *testing.T) history.ID {
	t.Helper()
	var id history.ID
	if _, err := rand.Read(id[:]); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestTerminalContext_NoFolders(t *testing.T) {
	ctrl, _, _, cleanup := setupTestController(t)
	defer cleanup()
	ctx := context.Background()

	r, err := ctrl.TerminalQuery(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "context",
	})
	if err != nil {
		t.Fatalf("unexpected query error: %v", err)
	}
	if r.Error == nil || r.Error.Code != "ROOT_UNAVAILABLE" {
		t.Fatalf("expected ROOT_UNAVAILABLE, got %+v", r.Error)
	}
}

func TestTerminalContext_ExplicitFolderAndName(t *testing.T) {
	ctrl, db, _, cleanup := setupTestController(t)
	defer cleanup()
	rootA := testkit.NewDisposable(t)
	rootB := testkit.NewDisposable(t)
	ctx := context.Background()

	fA := randomFolderID(t)
	fB := randomFolderID(t)

	if _, err := ctrl.RegisterFolder(ctx, fA, rootA); err != nil {
		t.Fatal(err)
	}
	if err := db.SetFolderDisplayName(ctx, fA, "Documents"); err != nil {
		t.Fatal(err)
	}

	if _, err := ctrl.RegisterFolder(ctx, fB, rootB); err != nil {
		t.Fatal(err)
	}
	if err := db.SetFolderDisplayName(ctx, fB, "Photos"); err != nil {
		t.Fatal(err)
	}

	fAHex := hex.EncodeToString(fA[:])
	fBHex := hex.EncodeToString(fB[:])

	// 1. Query by explicit folder ID
	r1, err := ctrl.TerminalQuery(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "context",
		Folder:  fAHex,
	})
	if err != nil || r1.Error != nil {
		t.Fatalf("failed query by ID: err=%v, resError=%+v", err, r1.Error)
	}
	if r1.Context == nil || r1.Context.Folder != fAHex || r1.Context.FolderName != "Documents" || r1.Context.Root != rootA {
		t.Fatalf("unexpected context result: %+v", r1.Context)
	}
	if r1.Context.Generation == "" {
		t.Fatal("empty context generation")
	}

	// 2. Query by matching folder name
	r2, err := ctrl.TerminalQuery(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "context",
		Name:    "Photos",
	})
	if err != nil || r2.Error != nil {
		t.Fatalf("failed query by name: err=%v, resError=%+v", err, r2.Error)
	}
	if r2.Context == nil || r2.Context.Folder != fBHex || r2.Context.FolderName != "Photos" || r2.Context.Root != rootB {
		t.Fatalf("unexpected context result: %+v", r2.Context)
	}

	// 3. Query with conflicting folder ID and Name -> AMBIGUOUS_CONTEXT
	r3, err := ctrl.TerminalQuery(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "context",
		Folder:  fAHex,
		Name:    "Photos",
	})
	if err != nil {
		t.Fatal(err)
	}
	if r3.Error == nil || r3.Error.Code != "AMBIGUOUS_CONTEXT" {
		t.Fatalf("expected AMBIGUOUS_CONTEXT, got %+v", r3.Error)
	}
	if len(r3.Items) != 1 || r3.Items[0].ID != fAHex {
		t.Fatalf("expected candidate item with folder A, got %+v", r3.Items)
	}

	// 4. Query with nonexistent folder name -> FOLDER_NOT_FOUND
	r4, err := ctrl.TerminalQuery(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "context",
		Name:    "Music",
	})
	if err != nil {
		t.Fatal(err)
	}
	if r4.Error == nil || r4.Error.Code != "FOLDER_NOT_FOUND" {
		t.Fatalf("expected FOLDER_NOT_FOUND for missing name, got %+v", r4.Error)
	}
}

func TestTerminalContext_DuplicateNames(t *testing.T) {
	ctrl, db, _, cleanup := setupTestController(t)
	defer cleanup()
	root1 := testkit.NewDisposable(t)
	root2 := testkit.NewDisposable(t)
	ctx := context.Background()

	f1 := randomFolderID(t)
	f2 := randomFolderID(t)

	if _, err := ctrl.RegisterFolder(ctx, f1, root1); err != nil {
		t.Fatal(err)
	}
	if err := db.SetFolderDisplayName(ctx, f1, "Notes"); err != nil {
		t.Fatal(err)
	}

	if _, err := ctrl.RegisterFolder(ctx, f2, root2); err != nil {
		t.Fatal(err)
	}
	if err := db.SetFolderDisplayName(ctx, f2, "Notes"); err != nil {
		t.Fatal(err)
	}

	// Querying by duplicate name "Notes" must return AMBIGUOUS_CONTEXT with candidates
	r, err := ctrl.TerminalQuery(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "context",
		Name:    "Notes",
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Error == nil || r.Error.Code != "AMBIGUOUS_CONTEXT" {
		t.Fatalf("expected AMBIGUOUS_CONTEXT for duplicate name, got %+v", r.Error)
	}
	if len(r.Items) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(r.Items))
	}

	// Disambiguating by exact ID works
	f1Hex := hex.EncodeToString(f1[:])
	rDisambig, err := ctrl.TerminalQuery(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "context",
		Folder:  f1Hex,
	})
	if err != nil || rDisambig.Error != nil {
		t.Fatalf("disambiguation failed: err=%v, resError=%+v", err, rDisambig.Error)
	}
	if rDisambig.Context.Folder != f1Hex {
		t.Fatalf("expected folder %s, got %s", f1Hex, rDisambig.Context.Folder)
	}
}

func TestTerminalContext_CwdInference(t *testing.T) {
	ctrl, db, _, cleanup := setupTestController(t)
	defer cleanup()
	root := testkit.NewDisposable(t)
	outside := testkit.NewDisposable(t)
	ctx := context.Background()

	f := randomFolderID(t)
	if _, err := ctrl.RegisterFolder(ctx, f, root); err != nil {
		t.Fatal(err)
	}
	if err := db.SetFolderDisplayName(ctx, f, "Work"); err != nil {
		t.Fatal(err)
	}

	// 1. Cwd is root
	r1, err := ctrl.TerminalQuery(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "context",
		Cwd:     root,
	})
	if err != nil || r1.Error != nil {
		t.Fatalf("root cwd failed: err=%v, resError=%+v", err, r1.Error)
	}
	if r1.Context.Root != root {
		t.Fatalf("expected root %s, got %s", root, r1.Context.Root)
	}

	// 2. Cwd is subdirectory of root
	sub := filepath.Join(root, "docs", "specs")
	if err := os.MkdirAll(sub, 0700); err != nil {
		t.Fatal(err)
	}
	r2, err := ctrl.TerminalQuery(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "context",
		Cwd:     sub,
	})
	if err != nil || r2.Error != nil {
		t.Fatalf("subdir cwd failed: err=%v, resError=%+v", err, r2.Error)
	}
	if r2.Context.Root != root {
		t.Fatalf("expected root %s, got %s", root, r2.Context.Root)
	}

	// 3. Cwd is outside root -> AMBIGUOUS_CONTEXT
	r3, err := ctrl.TerminalQuery(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "context",
		Cwd:     outside,
	})
	if err != nil {
		t.Fatal(err)
	}
	if r3.Error == nil || r3.Error.Code != "AMBIGUOUS_CONTEXT" {
		t.Fatalf("expected AMBIGUOUS_CONTEXT for outside cwd, got %+v", r3.Error)
	}
	if len(r3.Items) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(r3.Items))
	}
}

func TestTerminalContext_PathValidationAndSymlink(t *testing.T) {
	ctrl, _, _, cleanup := setupTestController(t)
	defer cleanup()
	root := testkit.NewDisposable(t)
	ctx := context.Background()

	f := randomFolderID(t)
	if _, err := ctrl.RegisterFolder(ctx, f, root); err != nil {
		t.Fatal(err)
	}

	// 1. Reserved directory target -> INVALID_PATH
	r1, err := ctrl.TerminalQuery(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "context",
		Folder:  hex.EncodeToString(f[:]),
		Path:    ".orbit-scratch/secret.txt",
	})
	// The contract validator rejects reserved segments before the context lookup.
	if err == nil && (r1.Error == nil || r1.Error.Code != "INVALID_PATH") {
		t.Fatalf("expected INVALID_PATH for reserved path, got %+v", r1.Error)
	}
	if err != nil && !strings.Contains(err.Error(), "INVALID_PATH") {
		t.Fatalf("expected INVALID_PATH for reserved path, got %v", err)
	}

	// 2. Symlink inside root pointing outside root -> INVALID_PATH
	outsideTarget := filepath.Join(testkit.NewDisposable(t), "target.txt")
	os.WriteFile(outsideTarget, []byte("outside"), 0600)
	symlinkPath := filepath.Join(root, "symlink.txt")
	if err := os.Symlink(outsideTarget, symlinkPath); err != nil {
		t.Fatal(err)
	}

	r2, err := ctrl.TerminalQuery(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "context",
		Folder:  hex.EncodeToString(f[:]),
		Path:    "symlink.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if r2.Error == nil || r2.Error.Code != "INVALID_PATH" {
		t.Fatalf("expected INVALID_PATH for symlink, got %+v", r2.Error)
	}
}

func TestTerminalFoldersAndDevices(t *testing.T) {
	ctrl, db, _, cleanup := setupTestController(t)
	defer cleanup()
	root := testkit.NewDisposable(t)
	ctx := context.Background()

	// Query folders when empty
	rfEmpty, err := ctrl.TerminalQuery(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "folders",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rfEmpty.State != "empty" || len(rfEmpty.Items) != 0 {
		t.Fatalf("expected empty folders, got state=%s items=%d", rfEmpty.State, len(rfEmpty.Items))
	}

	// Register a folder
	f := randomFolderID(t)
	if _, err := ctrl.RegisterFolder(ctx, f, root); err != nil {
		t.Fatal(err)
	}
	if err := db.SetFolderDisplayName(ctx, f, "Projects"); err != nil {
		t.Fatal(err)
	}

	// Query folders now
	rf, err := ctrl.TerminalQuery(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "folders",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rf.State != "success" || len(rf.Items) != 1 {
		t.Fatalf("expected 1 folder, got state=%s items=%d", rf.State, len(rf.Items))
	}
	if rf.Items[0].Name != "Projects" || rf.Items[0].Root != root {
		t.Fatalf("unexpected folder item: %+v", rf.Items[0])
	}

	// Query devices
	rd, err := ctrl.TerminalQuery(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "devices",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rd.State != "success" || len(rd.Items) < 1 {
		t.Fatalf("expected at least 1 device (local), got state=%s items=%d", rd.State, len(rd.Items))
	}
}
