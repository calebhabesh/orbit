package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"golang.org/x/sys/unix"
)

func testID(value byte) history.ID {
	var id history.ID
	for i := range id {
		id[i] = value
	}
	return id
}

func TestCaptureDetectsInPlaceAndSaveByRenameMutation(t *testing.T) {
	for _, pattern := range []string{"in-place", "save-by-rename"} {
		t.Run(pattern, func(t *testing.T) {
			ctx := context.Background()
			work, db, folder, root := testWorkspace(t)
			path := filepath.Join(root, "note.txt")
			if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}
			first, err := work.Scan(ctx, folder)
			if err != nil || len(first.Captured) != 1 {
				t.Fatalf("initial scan=%+v err=%v", first, err)
			}
			if err := os.WriteFile(path, []byte("updated!"), 0o600); err != nil {
				t.Fatal(err)
			}
			mutated := false
			work.hook = func(name string) error {
				if name != HookCaptureRead || mutated {
					return nil
				}
				mutated = true
				if pattern == "in-place" {
					return os.WriteFile(path, []byte("changed!"), 0o600)
				}
				temporary := filepath.Join(root, "replacement")
				if err := os.WriteFile(temporary, []byte("renamed!"), 0o600); err != nil {
					return err
				}
				return os.Rename(temporary, path)
			}
			result, err := work.Scan(ctx, folder)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Captured) != 1 || !mutated {
				t.Fatalf("scan=%+v mutated=%v", result, mutated)
			}
			projection, err := db.Projection(ctx, folder, "note.txt")
			if err != nil {
				t.Fatal(err)
			}
			if projection.Digest != sha256.Sum256([]byte("changed!")) && projection.Digest != sha256.Sum256([]byte("renamed!")) {
				t.Fatalf("captured stale bytes: %x", projection.Digest)
			}
			if err := db.VerifyVersionContent(ctx, first.Captured[0].ID); err != nil {
				t.Fatalf("protected original lost: %v", err)
			}
		})
	}
}

func TestApplyRejectsUnobservedLocalContentAndSymlink(t *testing.T) {
	ctx := context.Background()
	work, db, folder, root := testWorkspace(t)
	if _, err := work.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}
	remote := remoteFile(t, db, folder, "note.txt", []byte("remote"))
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("unscanned"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := work.Apply(ctx, remote); !errors.Is(err, ErrStructuralConflict) {
		t.Fatalf("apply err=%v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "unscanned" {
		t.Fatalf("working data=%q err=%v", data, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(root), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if err := work.Apply(ctx, remote); err == nil {
		t.Fatal("symlink target accepted")
	}
	outsideData, err := os.ReadFile(outside)
	if err != nil || string(outsideData) != "outside" {
		t.Fatalf("outside bytes=%q err=%v", outsideData, err)
	}
}

func TestExchangePreservesLateSaveByRenameCandidate(t *testing.T) {
	ctx := context.Background()
	work, db, folder, root := testWorkspace(t)
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("captured"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := work.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}
	remote := remoteFile(t, db, folder, "note.txt", []byte("remote"))
	work.hook = func(name string) error {
		if name != HookBeforeExchange {
			return nil
		}
		temporary := filepath.Join(root, "editor-save")
		if err := os.WriteFile(temporary, []byte("late editor"), 0o600); err != nil {
			return err
		}
		return os.Rename(temporary, path)
	}
	if err := work.Apply(ctx, remote); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "remote" {
		t.Fatalf("visible bytes=%q err=%v", data, err)
	}
	entries, err := os.ReadDir(filepath.Join(root, scratchName))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if len(entry.Name()) < len("recovery-") || entry.Name()[:len("recovery-")] != "recovery-" {
			continue
		}
		candidate, err := os.ReadFile(filepath.Join(root, scratchName, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if string(candidate) == "late editor" {
			found = true
		}
	}
	if !found {
		t.Fatal("late editor inode was not retained in recovery scratch")
	}
	projection, err := db.Projection(ctx, folder, "note.txt")
	if err != nil || projection.BlockReason != "LATE_EDITOR_CANDIDATE" {
		t.Fatalf("late editor not reported: projection=%+v err=%v", projection, err)
	}
}

func TestParentSymlinkSwapCannotPublishOutsideRoot(t *testing.T) {
	ctx := context.Background()
	work, db, folder, root := testWorkspace(t)
	parent := filepath.Join(root, "nested")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "note"), []byte("local"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := work.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}
	remote := remoteFile(t, db, folder, "nested/note", []byte("remote"))
	outside := filepath.Join(filepath.Dir(root), "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "note"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	work.hook = func(name string) error {
		if name != HookBeforeExchange {
			return nil
		}
		if err := os.Rename(parent, filepath.Join(root, "nested-old")); err != nil {
			return err
		}
		return os.Symlink(outside, parent)
	}
	if err := work.Apply(ctx, remote); err == nil {
		t.Fatal("parent swap was reported applied")
	}
	data, err := os.ReadFile(filepath.Join(outside, "note"))
	if err != nil || string(data) != "outside" {
		t.Fatalf("outside bytes=%q err=%v", data, err)
	}
	applied, err := db.WorkingApplied(ctx, remote)
	if err != nil || applied {
		t.Fatalf("remote applied=%v err=%v", applied, err)
	}
	if err := db.VerifyVersionContent(ctx, remote); err != nil {
		t.Fatalf("verified remote content lost: %v", err)
	}
}

func TestOpenDescriptorAfterExchangeWritesRecoveryCandidate(t *testing.T) {
	ctx := context.Background()
	work, db, folder, root := testWorkspace(t)
	path := filepath.Join(root, "note")
	if err := os.WriteFile(path, []byte("captured"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := work.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}
	remote := remoteFile(t, db, folder, "note", []byte("remote"))
	old, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	work.hook = func(name string) error {
		if name != HookFilesystemTransition {
			return nil
		}
		if _, err := old.Seek(0, 0); err != nil {
			return err
		}
		if _, err := old.Write([]byte("late edit")); err != nil {
			return err
		}
		return old.Sync()
	}
	if err := work.Apply(ctx, remote); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "remote" {
		t.Fatalf("visible=%q err=%v", data, err)
	}
	projection, err := db.Projection(ctx, folder, "note")
	if err != nil || projection.BlockReason != "LATE_EDITOR_CANDIDATE" {
		t.Fatalf("projection=%+v err=%v", projection, err)
	}
	entries, err := os.ReadDir(filepath.Join(root, scratchName))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "recovery-") {
			continue
		}
		candidate, err := os.ReadFile(filepath.Join(root, scratchName, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if string(candidate) == "late edit" {
			found = true
		}
	}
	if !found {
		t.Fatal("late descriptor bytes missing from recovery")
	}
}

func remoteFile(t *testing.T, db *repository.DB, folder history.ID, path string, content []byte) history.VersionID {
	t.Helper()
	ctx := context.Background()
	manifest, err := db.StoreFile(ctx, bytes.NewReader(content), false)
	if err != nil {
		t.Fatal(err)
	}
	remote := testID('B')
	envelope := history.Envelope{ID: history.VersionID{Folder: folder, Author: remote, Counter: 1}, Path: path, Vector: []history.ClockEntry{{Author: remote, Counter: 1}}, Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1}
	if err := db.ImportMetadata(ctx, envelope); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkContentReady(ctx, envelope.ID); err != nil {
		t.Fatal(err)
	}
	return envelope.ID
}

func testWorkspace(t *testing.T) (*Workspace, *repository.DB, history.ID, string) {
	t.Helper()
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	state := filepath.Join(disposable, "state")
	root := filepath.Join(disposable, "root")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	folder := testID('F')
	if err := db.EnsureFolder(ctx, folder, testID('A'), 1); err != nil {
		t.Fatal(err)
	}
	workspace := New(db, Options{})
	if _, err := workspace.Register(ctx, folder, root); err != nil {
		t.Fatal(err)
	}
	return workspace, db, folder, root
}

func TestBootstrapScanCapturesFilesAndDirectoryWithoutDeletion(t *testing.T) {
	ctx := context.Background()
	workspace, db, folder, root := testWorkspace(t)
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := workspace.Scan(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Captured) != 2 || first.Deletion != nil {
		t.Fatalf("first scan=%+v", first)
	}
	second, err := workspace.Scan(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Captured) != 0 || second.Deletion != nil {
		t.Fatalf("second scan=%+v", second)
	}
	for _, path := range []string{"empty", "note.txt"} {
		projection, err := db.Projection(ctx, folder, path)
		if err != nil || len(projection.Basis) != 1 {
			t.Fatalf("projection %s=%+v err=%v", path, projection, err)
		}
	}
}

func TestRootReplacementPausesScan(t *testing.T) {
	ctx := context.Background()
	workspace, _, folder, root := testWorkspace(t)
	moved := root + "-old"
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Scan(ctx, folder); !errors.Is(err, ErrRootUnavailable) {
		t.Fatalf("scan err=%v", err)
	}
}

func TestRootMarkerMismatchPausesScan(t *testing.T) {
	ctx := context.Background()
	work, _, folder, root := testWorkspace(t)
	marker := filepath.Join(root, scratchName, markerName)
	if err := os.WriteFile(marker, []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := work.Scan(ctx, folder); !errors.Is(err, ErrRootUnavailable) {
		t.Fatalf("scan err=%v", err)
	}
}

func TestRegistrationRejectsSymlinkedRootAndStateOverlap(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	state, root := filepath.Join(disposable, "state"), filepath.Join(disposable, "root")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	folder := testID('F')
	if err := db.EnsureFolder(ctx, folder, testID('A'), 1); err != nil {
		t.Fatal(err)
	}
	work := New(db, Options{})
	if _, err := work.Register(ctx, folder, disposable); err == nil {
		t.Fatal("state-overlapping root registered")
	}
	link := filepath.Join(disposable, "root-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if _, err := work.Register(ctx, folder, link); err == nil {
		t.Fatal("symlinked root registered")
	}
}

func TestOverlappingRootRegistrationLeavesNoScratch(t *testing.T) {
	ctx := context.Background()
	work, db, _, root := testWorkspace(t)
	other := testID('G')
	if err := db.EnsureFolder(ctx, other, testID('A'), 1); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := work.Register(ctx, other, child); err == nil {
		t.Fatal("overlapping root registered")
	}
	if _, err := os.Lstat(filepath.Join(child, scratchName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed registration left scratch: %v", err)
	}
}

func TestMassDeletionPreviewRequiresApproval(t *testing.T) {
	ctx := context.Background()
	workspace, db, folder, root := testWorkspace(t)
	workspace.deletionPolicy = MassDeletionPolicy{Absolute: 2}
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := workspace.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b"} {
		if err := testkit.ValidateDestructiveTarget(filepath.Dir(root), filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	result, err := workspace.Scan(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Captured) != 0 || result.Deletion == nil || len(result.Deletion.Paths) != 2 {
		t.Fatalf("scan=%+v", result)
	}
	if _, err := workspace.ApproveDeletions(ctx, folder, result.Deletion.Token); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b"} {
		projection, err := db.Projection(ctx, folder, name)
		if err != nil || projection.Kind != history.KindTombstone {
			t.Fatalf("%s=%+v err=%v", name, projection, err)
		}
	}
}

func TestMassDeletionPolicyUsesAbsoluteOrRatio(t *testing.T) {
	policy := MassDeletionPolicy{Absolute: 100, Ratio: 0.25, MinimumForRatio: 10}
	for _, test := range []struct {
		deleted, tracked int
		want             bool
	}{
		{1, 2, false}, {9, 20, false}, {10, 20, true}, {99, 1000, false}, {100, 1000, true}, {25, 100, true},
	} {
		if got := policy.RequiresApproval(test.deleted, test.tracked); got != test.want {
			t.Fatalf("deleted=%d tracked=%d got=%v want=%v", test.deleted, test.tracked, got, test.want)
		}
	}
}

func TestLargeFolderAbsoluteDeletionPreview(t *testing.T) {
	ctx := context.Background()
	work, _, folder, root := testWorkspace(t)
	work.deletionPolicy = MassDeletionPolicy{Absolute: 100, Ratio: 0.99, MinimumForRatio: 1000}
	for i := 0; i < 120; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("file-%03d", i)), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := work.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		path := filepath.Join(root, fmt.Sprintf("file-%03d", i))
		if err := testkit.ValidateDestructiveTarget(filepath.Dir(root), path); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	result, err := work.Scan(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if result.Deletion == nil || len(result.Deletion.Paths) != 100 || len(result.Captured) != 0 {
		t.Fatalf("large deletion scan=%+v", result)
	}
}

func TestDeletionPreviewIsStaleAfterRescanOrReappearance(t *testing.T) {
	ctx := context.Background()
	work, _, folder, root := testWorkspace(t)
	work.deletionPolicy = MassDeletionPolicy{Absolute: 1}
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := work.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}
	if err := testkit.ValidateDestructiveTarget(filepath.Dir(root), path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	first, err := work.Scan(ctx, folder)
	if err != nil || first.Deletion == nil {
		t.Fatalf("first preview=%+v err=%v", first, err)
	}
	if err := os.WriteFile(path, []byte("returned"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := work.ApproveDeletions(ctx, folder, first.Deletion.Token); !errors.Is(err, repository.ErrStaleGeneration) {
		t.Fatalf("reappearance approval err=%v", err)
	}
	if _, err := work.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}
	if _, err := work.ApproveDeletions(ctx, folder, first.Deletion.Token); !errors.Is(err, repository.ErrStaleGeneration) {
		t.Fatalf("rescan approval err=%v", err)
	}
}

func TestStructuralFileParentBlocksNestedPublication(t *testing.T) {
	ctx := context.Background()
	work, db, folder, root := testWorkspace(t)
	parent := filepath.Join(root, "parent")
	if err := os.WriteFile(parent, []byte("parent bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := work.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}
	remote := remoteFile(t, db, folder, "parent/child", []byte("child bytes"))
	if err := work.Apply(ctx, remote); !errors.Is(err, ErrStructuralConflict) {
		t.Fatalf("apply err=%v", err)
	}
	data, err := os.ReadFile(parent)
	if err != nil || string(data) != "parent bytes" {
		t.Fatalf("parent=%q err=%v", data, err)
	}
	projection, err := db.Projection(ctx, folder, "parent/child")
	if err != nil || projection.BlockReason != "STRUCTURAL_CONFLICT" {
		t.Fatalf("projection=%+v err=%v", projection, err)
	}
}

func TestInvalidNestedSymlinkCannotEscapeScan(t *testing.T) {
	ctx := context.Background()
	work, _, folder, root := testWorkspace(t)
	outside := filepath.Join(filepath.Dir(root), "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	result, err := work.Scan(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Captured) != 0 || len(result.Issues) == 0 {
		t.Fatalf("symlink scan=%+v", result)
	}
}

func TestHardLinkAndFIFOAreDiagnosedWithoutReading(t *testing.T) {
	ctx := context.Background()
	work, _, folder, root := testWorkspace(t)
	first := filepath.Join(root, "linked")
	if err := os.WriteFile(first, []byte("protected"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(first, filepath.Join(root, "second-link")); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filepath.Join(root, "pipe"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := work.Scan(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Captured) != 0 {
		t.Fatalf("unsupported objects captured: %+v", result.Captured)
	}
	codes := map[string]string{}
	for _, issue := range result.Issues {
		codes[issue.Path] = issue.Code
	}
	if codes["linked"] != "HARD_LINK_UNSUPPORTED" || codes["second-link"] != "HARD_LINK_UNSUPPORTED" || codes["pipe"] != "UNSUPPORTED_OBJECT" {
		t.Fatalf("issues=%v", codes)
	}
}

func TestIncompleteSubtreeCannotInferDeletion(t *testing.T) {
	ctx := context.Background()
	work, db, folder, root := testWorkspace(t)
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "note"), []byte("captured"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := work.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "sub"), filepath.Join(root, "sub-old")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(root), "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "sub")); err != nil {
		t.Fatal(err)
	}
	result, err := work.Scan(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Issues) == 0 {
		t.Fatalf("incomplete subtree not reported: %+v", result)
	}
	projection, err := db.Projection(ctx, folder, "sub/note")
	if err != nil || projection.Kind != history.KindFile {
		t.Fatalf("prior child wrongly deleted: %+v err=%v", projection, err)
	}
}

func TestApplyRemoteAndRestartScanDoesNotAuthorEcho(t *testing.T) {
	ctx := context.Background()
	workspace, db, folder, root := testWorkspace(t)
	if _, err := workspace.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}
	content := []byte("remote bytes")
	manifest, err := db.StoreFile(ctx, bytes.NewReader(content), true)
	if err != nil {
		t.Fatal(err)
	}
	remote := testID('B')
	envelope := history.Envelope{ID: history.VersionID{Folder: folder, Author: remote, Counter: 1}, Path: "nested/remote.txt", Vector: []history.ClockEntry{{Author: remote, Counter: 1}}, Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1}
	if err := db.ImportMetadata(ctx, envelope); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkContentReady(ctx, envelope.ID); err != nil {
		t.Fatal(err)
	}
	if err := workspace.Apply(ctx, envelope.ID); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "nested", "remote.txt"))
	if err != nil || string(data) != string(content) {
		t.Fatalf("data=%q err=%v", data, err)
	}
	result, err := workspace.Scan(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Captured) != 0 {
		t.Fatalf("scan authored echo: %+v", result.Captured)
	}
	if result.Deletion != nil {
		t.Fatalf("scan proposed deletion: %+v", result.Deletion)
	}
	projection, err := db.Projection(ctx, folder, envelope.Path)
	if err != nil || projection.Digest != sha256.Sum256(content) {
		t.Fatalf("projection=%+v err=%v", projection, err)
	}
}

func TestRemovedScaffoldCanBeRecreatedAsExplicitDirectory(t *testing.T) {
	ctx := context.Background()
	work, db, folder, root := testWorkspace(t)
	if _, err := work.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}
	remote := remoteFile(t, db, folder, "nested/file", []byte("remote"))
	if err := work.Apply(ctx, remote); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "nested", "file")
	parent := filepath.Join(root, "nested")
	if err := testkit.ValidateDestructiveTarget(filepath.Dir(root), child); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	if err := testkit.ValidateDestructiveTarget(filepath.Dir(root), parent); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	if _, err := work.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	result, err := work.Scan(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, envelope := range result.Captured {
		if envelope.Path == "nested" && envelope.Kind == history.KindDirectory {
			found = true
		}
	}
	if !found {
		t.Fatalf("explicit recreated directory not captured: %+v", result.Captured)
	}
}

func TestExecutableOnlyEditIsCapturedAndAppliedWithSafeMode(t *testing.T) {
	ctx := context.Background()
	work, db, folder, root := testWorkspace(t)
	path := filepath.Join(root, "tool")
	if err := os.WriteFile(path, []byte("script"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := work.Scan(ctx, folder)
	if err != nil || len(first.Captured) != 1 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	second, err := work.Scan(ctx, folder)
	if err != nil || len(second.Captured) != 1 || !second.Captured[0].Manifest.Executable {
		t.Fatalf("exec edit=%+v err=%v", second, err)
	}
	remote := remoteFile(t, db, folder, "remote-tool", []byte("remote script"))
	if err := work.Apply(ctx, remote); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, "remote-tool"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 != 0 {
		t.Fatalf("non-executable remote file gained execute bits: %04o", info.Mode().Perm())
	}
	manifest, err := db.StoreFile(ctx, bytes.NewReader([]byte("remote executable")), true)
	if err != nil {
		t.Fatal(err)
	}
	author := testID('C')
	envelope := history.Envelope{ID: history.VersionID{Folder: folder, Author: author, Counter: 1}, Path: "remote-exec", Vector: []history.ClockEntry{{Author: author, Counter: 1}}, Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1}
	if err := db.ImportMetadata(ctx, envelope); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkContentReady(ctx, envelope.ID); err != nil {
		t.Fatal(err)
	}
	if err := work.Apply(ctx, envelope.ID); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(filepath.Join(root, "remote-exec"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("executable remote mode=%04o, want 0700", info.Mode().Perm())
	}
}

func TestStageFailureAndBudgetRefusalPreserveCapturedBytes(t *testing.T) {
	ctx := context.Background()
	work, db, folder, root := testWorkspace(t)
	path := filepath.Join(root, "note")
	if err := os.WriteFile(path, []byte("captured"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := work.Scan(ctx, folder)
	if err != nil || len(first.Captured) != 1 {
		t.Fatalf("scan=%+v err=%v", first, err)
	}
	remote := remoteFile(t, db, folder, "note", []byte("remote"))
	work.hook = func(name string) error {
		if name == HookStageFlushed {
			return syscall.ENOSPC
		}
		return nil
	}
	if err := work.Apply(ctx, remote); !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("stage error=%v", err)
	}
	work.hook = nil
	if err := work.Recover(ctx, folder); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "captured" {
		t.Fatalf("working bytes=%q err=%v", data, err)
	}
	if err := db.VerifyVersionContent(ctx, first.Captured[0].ID); err != nil {
		t.Fatalf("captured object lost: %v", err)
	}
	usage, err := db.StorageUsage(ctx)
	if err != nil || usage.Reserved != 0 {
		t.Fatalf("unreleased stage reservation: %+v err=%v", usage, err)
	}
}

func TestStageBudgetRefusalPreservesPriorVersion(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	state, root := filepath.Join(disposable, "state"), filepath.Join(disposable, "root")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	folder := testID('F')
	if err := db.EnsureFolder(ctx, folder, testID('A'), 1); err != nil {
		t.Fatal(err)
	}
	work := New(db, Options{})
	if _, err := work.Register(ctx, folder, root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "note"), []byte("captured"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := work.Scan(ctx, folder)
	if err != nil || len(result.Captured) != 1 {
		t.Fatalf("scan=%+v err=%v", result, err)
	}
	remote := remoteFile(t, db, folder, "note", []byte("remote"))
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	limited, err := repository.OpenWithOptions(ctx, state, repository.Options{BudgetBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer limited.Close()
	work = New(limited, Options{})
	if err := work.Apply(ctx, remote); !errors.Is(err, repository.ErrBudgetExceeded) {
		t.Fatalf("budget refusal=%v", err)
	}
	if err := work.Recover(ctx, folder); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "note"))
	if err != nil || string(data) != "captured" {
		t.Fatalf("working bytes=%q err=%v", data, err)
	}
	if err := limited.VerifyVersionContent(ctx, result.Captured[0].ID); err != nil {
		t.Fatalf("prior content lost: %v", err)
	}
}
