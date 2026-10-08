package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/calebhabesh/orbit/internal/history"
)

func TestScaffoldPrunedWhenChildDeleted(t *testing.T) {
	ctx := context.Background()
	work, db, folder, root := testWorkspace(t)

	authorB := testID('B')
	content := []byte("nested file content")
	manifest, err := db.StoreFile(ctx, strings.NewReader(string(content)), false)
	if err != nil {
		t.Fatal(err)
	}

	envFile := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: authorB, Counter: 1},
		Path:             "deep/nested/item.txt",
		Vector:           []history.ClockEntry{{Author: authorB, Counter: 1}},
		Kind:             history.KindFile,
		Manifest:         manifest,
		AuthoredRevision: 1,
	}
	if err := db.ImportMetadata(ctx, envFile); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkContentReady(ctx, envFile.ID); err != nil {
		t.Fatal(err)
	}

	// Apply the file
	if err := work.Apply(ctx, envFile.ID); err != nil {
		t.Fatalf("apply file: %v", err)
	}

	// Verify file and directories exist on disk
	filePath := filepath.Join(root, "deep", "nested", "item.txt")
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("expected file on disk: %v", err)
	}

	// Verify scaffolds are recorded in DB
	isScaffold, err := db.IsScaffold(ctx, folder, "deep/nested")
	if err != nil || !isScaffold {
		t.Fatalf("deep/nested IsScaffold=%v, want true", isScaffold)
	}
	isScaffold, err = db.IsScaffold(ctx, folder, "deep")
	if err != nil || !isScaffold {
		t.Fatalf("deep IsScaffold=%v, want true", isScaffold)
	}

	// Run scan: verify scaffolds are suppressed (no directory versions authored)
	scanRes, err := work.Scan(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	for _, env := range scanRes.Captured {
		if env.Kind == history.KindDirectory {
			t.Fatalf("scan authored directory version for scaffold: %s", env.Path)
		}
	}

	// Now author B deletes the file -> tombstone env
	envTombstone := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: authorB, Counter: 2},
		Path:             "deep/nested/item.txt",
		Parents:          []history.VersionID{envFile.ID},
		Vector:           []history.ClockEntry{{Author: authorB, Counter: 2}},
		Kind:             history.KindTombstone,
		AuthoredRevision: 1,
	}
	if err := db.ImportMetadata(ctx, envTombstone); err != nil {
		t.Fatal(err)
	}

	// Apply tombstone
	if err := work.Apply(ctx, envTombstone.ID); err != nil {
		t.Fatalf("apply tombstone: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected file to be gone, got err: %v", err)
	}

	// Verify empty scaffolds "deep/nested" and "deep" are pruned on disk
	nestedDir := filepath.Join(root, "deep", "nested")
	if _, err := os.Stat(nestedDir); !os.IsNotExist(err) {
		t.Errorf("expected deep/nested to be pruned on disk, stat err: %v", err)
	}
	deepDir := filepath.Join(root, "deep")
	if _, err := os.Stat(deepDir); !os.IsNotExist(err) {
		t.Errorf("expected deep to be pruned on disk, stat err: %v", err)
	}

	// Verify scaffolds removed from DB
	isScaffold, _ = db.IsScaffold(ctx, folder, "deep/nested")
	if isScaffold {
		t.Error("deep/nested should no longer be a scaffold")
	}
	isScaffold, _ = db.IsScaffold(ctx, folder, "deep")
	if isScaffold {
		t.Error("deep should no longer be a scaffold")
	}
}

func TestScaffoldNotPrunedIfHasOtherChildren(t *testing.T) {
	ctx := context.Background()
	work, db, folder, root := testWorkspace(t)

	authorB := testID('B')
	manifest1, err := db.StoreFile(ctx, strings.NewReader("child 1"), false)
	if err != nil {
		t.Fatal(err)
	}
	manifest2, err := db.StoreFile(ctx, strings.NewReader("child 2"), false)
	if err != nil {
		t.Fatal(err)
	}

	env1 := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: authorB, Counter: 1},
		Path:             "dir/nested/c1.txt",
		Vector:           []history.ClockEntry{{Author: authorB, Counter: 1}},
		Kind:             history.KindFile,
		Manifest:         manifest1,
		AuthoredRevision: 1,
	}
	env2 := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: authorB, Counter: 2},
		Path:             "dir/nested/c2.txt",
		Vector:           []history.ClockEntry{{Author: authorB, Counter: 2}},
		Kind:             history.KindFile,
		Manifest:         manifest2,
		AuthoredRevision: 1,
	}
	if err := db.ImportMetadata(ctx, env1); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkContentReady(ctx, env1.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.ImportMetadata(ctx, env2); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkContentReady(ctx, env2.ID); err != nil {
		t.Fatal(err)
	}

	if err := work.Apply(ctx, env1.ID); err != nil {
		t.Fatal(err)
	}
	if err := work.Apply(ctx, env2.ID); err != nil {
		t.Fatal(err)
	}

	// Delete c1.txt
	tomb1 := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: authorB, Counter: 3},
		Path:             "dir/nested/c1.txt",
		Parents:          []history.VersionID{env1.ID},
		Vector:           []history.ClockEntry{{Author: authorB, Counter: 3}},
		Kind:             history.KindTombstone,
		AuthoredRevision: 1,
	}
	if err := db.ImportMetadata(ctx, tomb1); err != nil {
		t.Fatal(err)
	}
	if err := work.Apply(ctx, tomb1.ID); err != nil {
		t.Fatal(err)
	}

	// "dir/nested" still contains "c2.txt", so it must NOT be pruned!
	nestedDir := filepath.Join(root, "dir", "nested")
	if _, err := os.Stat(nestedDir); err != nil {
		t.Errorf("dir/nested should not be pruned because c2.txt remains: %v", err)
	}
	c2Path := filepath.Join(root, "dir", "nested", "c2.txt")
	if _, err := os.Stat(c2Path); err != nil {
		t.Errorf("c2.txt should still exist: %v", err)
	}
}

func TestScaffoldNotPrunedIfUntrackedFilePresent(t *testing.T) {
	ctx := context.Background()
	work, db, folder, root := testWorkspace(t)

	authorB := testID('B')
	manifest, err := db.StoreFile(ctx, strings.NewReader("item"), false)
	if err != nil {
		t.Fatal(err)
	}

	envFile := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: authorB, Counter: 1},
		Path:             "holder/dir/item.txt",
		Vector:           []history.ClockEntry{{Author: authorB, Counter: 1}},
		Kind:             history.KindFile,
		Manifest:         manifest,
		AuthoredRevision: 1,
	}
	if err := db.ImportMetadata(ctx, envFile); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkContentReady(ctx, envFile.ID); err != nil {
		t.Fatal(err)
	}
	if err := work.Apply(ctx, envFile.ID); err != nil {
		t.Fatal(err)
	}

	// Create an untracked local file inside "holder/dir"
	untracked := filepath.Join(root, "holder", "dir", "untracked.txt")
	if err := os.WriteFile(untracked, []byte("user local note"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Delete item.txt
	tomb := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: authorB, Counter: 2},
		Path:             "holder/dir/item.txt",
		Parents:          []history.VersionID{envFile.ID},
		Vector:           []history.ClockEntry{{Author: authorB, Counter: 2}},
		Kind:             history.KindTombstone,
		AuthoredRevision: 1,
	}
	if err := db.ImportMetadata(ctx, tomb); err != nil {
		t.Fatal(err)
	}
	if err := work.Apply(ctx, tomb.ID); err != nil {
		t.Fatal(err)
	}

	// "holder/dir" has untracked.txt, so it must NOT be deleted!
	dirPath := filepath.Join(root, "holder", "dir")
	if _, err := os.Stat(dirPath); err != nil {
		t.Errorf("holder/dir should not be deleted because untracked file is inside: %v", err)
	}
	if _, err := os.Stat(untracked); err != nil {
		t.Errorf("untracked.txt should still exist: %v", err)
	}
}
