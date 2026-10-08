package repository_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
)

func TestConflictsAndStructuralConflicts(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	db, err := repository.Open(ctx, disposable)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder := testID('F')
	authorA := testID('A')
	authorB := testID('B')

	if err := db.EnsureFolder(ctx, folder, authorA, 1); err != nil {
		t.Fatal(err)
	}

	contentA := []byte("hello from A")
	manifestA, err := db.StoreFile(ctx, strings.NewReader(string(contentA)), false)
	if err != nil {
		t.Fatal(err)
	}

	contentB := []byte("hello from B (different)")
	manifestB, err := db.StoreFile(ctx, strings.NewReader(string(contentB)), false)
	if err != nil {
		t.Fatal(err)
	}

	contentSame := []byte("hello equal content")
	manifestSameA, err := db.StoreFile(ctx, strings.NewReader(string(contentSame)), false)
	if err != nil {
		t.Fatal(err)
	}
	manifestSameB, err := db.StoreFile(ctx, strings.NewReader(string(contentSame)), false)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Edit-Edit conflict on "edit.txt"
	// Author A creates A1
	envA1, err := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "edit.txt",
		Kind:             history.KindFile,
		Manifest:         manifestA,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Author B creates B1 on "edit.txt" imported via ImportMetadata
	envB1 := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: authorB, Counter: 1},
		Path:             "edit.txt",
		Vector:           []history.ClockEntry{{Author: authorB, Counter: 1}},
		Kind:             history.KindFile,
		Manifest:         manifestB,
		AuthoredRevision: 1,
	}
	if err := db.ImportMetadata(ctx, envB1); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkContentReady(ctx, envB1.ID); err != nil {
		t.Fatal(err)
	}

	// 2. Edit-Delete conflict on "del.txt"
	// A creates A2 on "del.txt"
	_, err = db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "del.txt",
		Kind:             history.KindFile,
		Manifest:         manifestA,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	// B creates B2 tombstone on "del.txt"
	envDelB := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: authorB, Counter: 2},
		Path:             "del.txt",
		Vector:           []history.ClockEntry{{Author: authorB, Counter: 2}},
		Kind:             history.KindTombstone,
		AuthoredRevision: 1,
	}
	if err := db.ImportMetadata(ctx, envDelB); err != nil {
		t.Fatal(err)
	}

	// 3. Delete-Delete conflict on "both_del.txt"
	_, err = db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "both_del.txt",
		Kind:             history.KindTombstone,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	envBDelB := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: authorB, Counter: 3},
		Path:             "both_del.txt",
		Vector:           []history.ClockEntry{{Author: authorB, Counter: 3}},
		Kind:             history.KindTombstone,
		AuthoredRevision: 1,
	}
	if err := db.ImportMetadata(ctx, envBDelB); err != nil {
		t.Fatal(err)
	}

	// 4. Equal-content conflict on "equal.txt"
	_, err = db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "equal.txt",
		Kind:             history.KindFile,
		Manifest:         manifestSameA,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	envEqB := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: authorB, Counter: 4},
		Path:             "equal.txt",
		Vector:           []history.ClockEntry{{Author: authorB, Counter: 4}},
		Kind:             history.KindFile,
		Manifest:         manifestSameB,
		AuthoredRevision: 1,
	}
	if err := db.ImportMetadata(ctx, envEqB); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkContentReady(ctx, envEqB.ID); err != nil {
		t.Fatal(err)
	}

	// 5. Structural conflict: Ancestor tombstone vs Descendant child file
	// A creates tombstone for "sub"
	_, err = db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "sub",
		Kind:             history.KindTombstone,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	// B creates file "sub/child.txt"
	envChild := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: authorB, Counter: 5},
		Path:             "sub/child.txt",
		Vector:           []history.ClockEntry{{Author: authorB, Counter: 5}},
		Kind:             history.KindFile,
		Manifest:         manifestA,
		AuthoredRevision: 1,
	}
	if err := db.ImportMetadata(ctx, envChild); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkContentReady(ctx, envChild.ID); err != nil {
		t.Fatal(err)
	}

	// Publish envA1 so that it is marked working-applied
	pub := repository.Publication{
		OperationID: "op1",
		Folder:      folder,
		Path:        "edit.txt",
		Intended:    envA1.ID,
		Kind:        history.KindFile,
	}
	if err := db.PreparePublication(ctx, pub); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPublicationPhase(ctx, pub.OperationID, "FILESYSTEM_PUBLISHED"); err != nil {
		t.Fatal(err)
	}
	if err := db.CommitPublication(ctx, pub.OperationID); err != nil {
		t.Fatal(err)
	}

	// Test db.Conflicts()
	conflicts, err := db.Conflicts(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	conflictMap := map[string]repository.ConflictSet{}
	for _, c := range conflicts {
		conflictMap[c.Path] = c
	}

	if c, ok := conflictMap["edit.txt"]; !ok {
		t.Error("missing conflict for edit.txt")
	} else {
		if c.ConflictKind != "edit-edit" {
			t.Errorf("edit.txt kind=%q, want edit-edit", c.ConflictKind)
		}
		if len(c.Heads) != 2 {
			t.Errorf("edit.txt heads len=%d, want 2", len(c.Heads))
		}
		if c.Applied == nil || *c.Applied != envA1.ID {
			t.Errorf("edit.txt applied=%v, want %v", c.Applied, envA1.ID)
		}
	}

	if c, ok := conflictMap["del.txt"]; !ok {
		t.Error("missing conflict for del.txt")
	} else {
		if c.ConflictKind != "edit-delete" {
			t.Errorf("del.txt kind=%q, want edit-delete", c.ConflictKind)
		}
		if len(c.Heads) != 2 {
			t.Errorf("del.txt heads len=%d, want 2", len(c.Heads))
		}
		if c.Applied == nil || c.Applied.Counter != 2 {
			t.Errorf("del.txt applied=%v, want counter 2", c.Applied)
		}
	}

	if c, ok := conflictMap["both_del.txt"]; !ok {
		t.Error("missing conflict for both_del.txt")
	} else {
		if c.ConflictKind != "delete-delete" {
			t.Errorf("both_del.txt kind=%q, want delete-delete", c.ConflictKind)
		}
		if len(c.Heads) != 2 {
			t.Errorf("both_del.txt heads len=%d, want 2", len(c.Heads))
		}
		if c.Applied == nil || c.Applied.Counter != 3 {
			t.Errorf("both_del.txt applied=%v, want counter 3", c.Applied)
		}
	}

	if c, ok := conflictMap["equal.txt"]; !ok {
		t.Error("missing conflict for equal.txt")
	} else {
		if c.ConflictKind != "equal-content" {
			t.Errorf("equal.txt kind=%q, want equal-content", c.ConflictKind)
		}
		if len(c.Heads) != 2 {
			t.Errorf("equal.txt heads len=%d, want 2", len(c.Heads))
		}
		if c.Applied == nil || c.Applied.Counter != 4 {
			t.Errorf("equal.txt applied=%v, want counter 4", c.Applied)
		}
	}

	// Test db.StructuralConflicts()
	structural, err := db.StructuralConflicts(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(structural) != 1 {
		t.Fatalf("structural conflicts len=%d, want 1", len(structural))
	}
	if structural[0].AncestorPath != "sub" || structural[0].DescendantPath != "sub/child.txt" {
		t.Errorf("structural conflict mismatch: ancestor=%s, descendant=%s", structural[0].AncestorPath, structural[0].DescendantPath)
	}

	// Test UnappliedSingleHeads:
	// "sub/child.txt" is a single head, but it is in a structural conflict with "sub",
	// so UnappliedSingleHeads must NOT return it!
	unapplied, err := db.UnappliedSingleHeads(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range unapplied {
		env, err := db.Envelope(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if env.Path == "sub" || env.Path == "sub/child.txt" {
			t.Errorf("UnappliedSingleHeads returned structurally conflicted path: %s", env.Path)
		}
		if env.Path == "edit.txt" || env.Path == "del.txt" || env.Path == "both_del.txt" || env.Path == "equal.txt" {
			t.Errorf("UnappliedSingleHeads returned conflicting path: %s", env.Path)
		}
	}
}

func TestScaffoldRepositoryMethods(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	db, err := repository.Open(ctx, disposable)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	folder := testID('F')
	author := testID('A')
	if err := db.EnsureFolder(ctx, folder, author, 1); err != nil {
		t.Fatal(err)
	}

	// Mark and complete a scaffold
	if err := db.MarkScaffold(ctx, folder, "scaffold/dir"); err != nil {
		t.Fatal(err)
	}
	// While pending: IsScaffold returns false
	if isScaffold, err := db.IsScaffold(ctx, folder, "scaffold/dir"); err != nil || isScaffold {
		t.Fatalf("pending scaffold IsScaffold=%v, err=%v, want false", isScaffold, err)
	}
	if err := db.CompleteScaffold(ctx, folder, "scaffold/dir"); err != nil {
		t.Fatal(err)
	}
	// After completion: IsScaffold returns true
	if isScaffold, err := db.IsScaffold(ctx, folder, "scaffold/dir"); err != nil || !isScaffold {
		t.Fatalf("completed scaffold IsScaffold=%v, err=%v, want true", isScaffold, err)
	}

	// HasActiveDescendantProjections: initially false
	hasDesc, err := db.HasActiveDescendantProjections(ctx, folder, "scaffold/dir")
	if err != nil || hasDesc {
		t.Fatalf("HasActiveDescendantProjections=%v, err=%v, want false", hasDesc, err)
	}

	// Create a local child file
	manifest, err := db.StoreFile(ctx, strings.NewReader("child content"), false)
	if err != nil {
		t.Fatal(err)
	}
	childEnv, err := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "scaffold/dir/child.txt",
		Kind:             history.KindFile,
		Manifest:         manifest,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Now HasActiveDescendantProjections should be true
	hasDesc, err = db.HasActiveDescendantProjections(ctx, folder, "scaffold/dir")
	if err != nil || !hasDesc {
		t.Fatalf("HasActiveDescendantProjections after child creation=%v, err=%v, want true", hasDesc, err)
	}

	// Publish directory version for "scaffold/dir" -> should upgrade scaffold to explicit directory
	dirEnv, err := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "scaffold/dir",
		Kind:             history.KindDirectory,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}

	pub := repository.Publication{
		OperationID: "op-dir",
		Folder:      folder,
		Path:        "scaffold/dir",
		Intended:    dirEnv.ID,
		Kind:        history.KindDirectory,
	}
	if err := db.PreparePublication(ctx, pub); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPublicationPhase(ctx, pub.OperationID, "FILESYSTEM_PUBLISHED"); err != nil {
		t.Fatal(err)
	}
	if err := db.CommitPublication(ctx, pub.OperationID); err != nil {
		t.Fatal(err)
	}

	// Now IsScaffold must be false, because it was upgraded to an explicit directory!
	if isScaffold, err := db.IsScaffold(ctx, folder, "scaffold/dir"); err != nil || isScaffold {
		t.Fatalf("after explicit directory publication IsScaffold=%v, err=%v, want false", isScaffold, err)
	}

	_ = childEnv
}

func testID(b byte) history.ID {
	var id history.ID
	for i := range id {
		id[i] = b
	}
	return id
}
