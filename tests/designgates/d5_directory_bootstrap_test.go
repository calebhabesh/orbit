package designgates

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
	"unicode/utf8"

	"github.com/calebhabesh/orbit/internal/testkit"
)

type observedKind string

const (
	kindFile      observedKind = "file"
	kindDirectory observedKind = "directory"
	kindTombstone observedKind = "tombstone"
)

func validateRelativePath(candidate string) error {
	if candidate == "" || !utf8.ValidString(candidate) || strings.IndexByte(candidate, 0) >= 0 {
		return errors.New("path must be non-empty UTF-8 without NUL")
	}
	if strings.HasPrefix(candidate, "/") || strings.HasSuffix(candidate, "/") || strings.Contains(candidate, "\\") {
		return errors.New("path must use relative slash-separated form")
	}
	if len([]byte(candidate)) > 4096 {
		return errors.New("path exceeds byte limit")
	}
	segments := strings.Split(candidate, "/")
	if len(segments) > 128 {
		return errors.New("path exceeds depth limit")
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || len([]byte(segment)) > 255 {
			return errors.New("invalid path segment")
		}
		if segment == ".orbit-internal" {
			return errors.New("reserved internal name")
		}
	}
	return nil
}

func TestD5CanonicalPathPolicy(t *testing.T) {
	valid := []string{"notes/today.txt", "café/exact-name", "control-\n-visible"}
	for _, candidate := range valid {
		if err := validateRelativePath(candidate); err != nil {
			t.Errorf("valid path %q rejected: %v", candidate, err)
		}
	}
	invalid := []string{"", "/absolute", "trailing/", "double//segment", "a/../b", "./a", "a\\b", ".orbit-internal/stage", "a/.orbit-internal/x", string([]byte{0xff})}
	for _, candidate := range invalid {
		if err := validateRelativePath(candidate); err == nil {
			t.Errorf("invalid path %q accepted", candidate)
		}
	}
}

type rootIdentity struct {
	device uint64
	inode  uint64
	marker string
}

func observeRootIdentity(root string) (rootIdentity, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return rootIdentity{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return rootIdentity{}, errors.New("root is not a real directory")
	}
	stat := info.Sys().(*syscall.Stat_t)
	markerPath := filepath.Join(root, ".orbit-internal", "registration")
	markerInfo, err := os.Lstat(markerPath)
	if err != nil {
		return rootIdentity{}, err
	}
	if !markerInfo.Mode().IsRegular() || markerInfo.Mode()&os.ModeSymlink != 0 || markerInfo.Sys().(*syscall.Stat_t).Nlink != 1 {
		return rootIdentity{}, errors.New("registration marker is not a private regular file")
	}
	marker, err := os.ReadFile(markerPath)
	if err != nil {
		return rootIdentity{}, err
	}
	return rootIdentity{device: uint64(stat.Dev), inode: stat.Ino, marker: string(marker)}, nil
}

func TestD5RootIdentityDetectsReplacementAndMarkerMismatch(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	root := filepath.Join(disposable, "root")
	if err := os.MkdirAll(filepath.Join(root, ".orbit-internal"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := testkit.ValidateDestructiveTarget(disposable, root); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, ".orbit-internal", "registration"), "random-registration-id")
	registered, err := observeRootIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	oldRoot := filepath.Join(disposable, "old-root")
	if err := os.Rename(root, oldRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".orbit-internal"), 0o700); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, ".orbit-internal", "registration"), "random-registration-id")
	replacement, err := observeRootIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	if registered == replacement {
		t.Fatal("root replacement with copied marker was not detected")
	}

	mustWrite(t, filepath.Join(root, ".orbit-internal", "registration"), "different-registration-id")
	mismatched, err := observeRootIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	if replacement == mismatched {
		t.Fatal("registration marker mismatch was not detected")
	}
}

type projectionModel struct {
	explicitDirectories map[string]bool
	scaffolds           map[string]bool
	children            map[string]observedKind
}

func newProjectionModel() *projectionModel {
	return &projectionModel{
		explicitDirectories: map[string]bool{},
		scaffolds:           map[string]bool{},
		children:            map[string]observedKind{},
	}
}

func (m *projectionModel) applyRemoteFile(name string) {
	for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
		if !m.explicitDirectories[parent] {
			m.scaffolds[parent] = true
		}
	}
	m.children[name] = kindFile
}

func (m *projectionModel) scanDirectory(name string) (createLocalVersion bool) {
	return !m.scaffolds[name]
}

func TestD5ProjectionScaffoldingDoesNotFabricateDirectoryVersions(t *testing.T) {
	model := newProjectionModel()
	model.applyRemoteFile("parent/child/file")
	for _, scaffold := range []string{"parent", "parent/child"} {
		if model.scanDirectory(scaffold) {
			t.Fatalf("scan fabricated a local version for scaffold %q", scaffold)
		}
	}
	model.explicitDirectories["empty"] = true
	if !model.scanDirectory("empty") {
		t.Fatal("explicit empty directory was suppressed")
	}
}

type structuralState struct {
	entries    map[string]observedKind
	generation uint64
}

func newStructuralState() *structuralState {
	return &structuralState{entries: map[string]observedKind{}, generation: 1}
}

func (s *structuralState) add(name string, kind observedKind) {
	s.entries[name] = kind
	s.generation++
}

func (s *structuralState) conflicts() []string {
	var conflicts []string
	for name, kind := range s.entries {
		if kind != kindFile && kind != kindTombstone {
			continue
		}
		prefix := name + "/"
		for candidate, candidateKind := range s.entries {
			if strings.HasPrefix(candidate, prefix) && candidateKind != kindTombstone {
				conflicts = append(conflicts, name+" blocks "+candidate)
			}
		}
	}
	sort.Strings(conflicts)
	return conflicts
}

func TestD5ParentDeleteOrFileVsChildIsStructuralConflict(t *testing.T) {
	for _, ancestorKind := range []observedKind{kindFile, kindTombstone} {
		t.Run(string(ancestorKind), func(t *testing.T) {
			state := newStructuralState()
			state.add("parent", ancestorKind)
			state.add("parent/new-child", kindFile)
			if got := state.conflicts(); len(got) != 1 {
				t.Fatalf("conflicts = %v, want one preserved structural conflict", got)
			}
			if state.entries["parent/new-child"] != kindFile {
				t.Fatal("child was destructively removed")
			}
		})
	}
}

func TestD5DirectoryDeletePreviewInvalidatedByNewChild(t *testing.T) {
	state := newStructuralState()
	state.add("parent", kindDirectory)
	previewGeneration := state.generation
	state.add("parent/new-child", kindFile)
	if previewGeneration == state.generation {
		t.Fatal("new child did not invalidate directory delete preview")
	}
}

type scanResult struct {
	bootstrap         bool
	rootVerified      bool
	completeSubtrees  map[string]bool
	observed          map[string]observedKind
	previouslyTracked []string
}

func (scan scanResult) inferredDeletions() []string {
	if scan.bootstrap || !scan.rootVerified {
		return nil
	}
	var deletions []string
	for _, name := range scan.previouslyTracked {
		if _, present := scan.observed[name]; present {
			continue
		}
		parent := path.Dir(name)
		if scan.completeSubtrees[parent] {
			deletions = append(deletions, name)
		}
	}
	sort.Strings(deletions)
	return deletions
}

func TestD5BootstrapAndIncompleteScansNeverInferDeletion(t *testing.T) {
	base := scanResult{
		rootVerified:      true,
		completeSubtrees:  map[string]bool{".": true},
		observed:          map[string]observedKind{},
		previouslyTracked: []string{"missing"},
	}
	bootstrap := base
	bootstrap.bootstrap = true
	if got := bootstrap.inferredDeletions(); len(got) != 0 {
		t.Fatalf("bootstrap inferred deletions: %v", got)
	}
	unavailable := base
	unavailable.rootVerified = false
	if got := unavailable.inferredDeletions(); len(got) != 0 {
		t.Fatalf("unavailable root inferred deletions: %v", got)
	}
	incomplete := base
	incomplete.completeSubtrees = map[string]bool{}
	if got := incomplete.inferredDeletions(); len(got) != 0 {
		t.Fatalf("incomplete subtree inferred deletions: %v", got)
	}
	if got := base.inferredDeletions(); fmt.Sprint(got) != "[missing]" {
		t.Fatalf("complete normal scan deletions = %v", got)
	}
}

func TestD5DivergentEnrollmentCreatesIndependentHistories(t *testing.T) {
	m := newHistoryModel()
	m.add("A1", "A", "existing-peer")
	m.add("B1", "B", "joining-root")
	if got := m.headIDs(); fmt.Sprint(got) != "[A1 B1]" {
		t.Fatalf("enrollment heads = %v", got)
	}
	if got := (scanResult{bootstrap: true, rootVerified: true, previouslyTracked: []string{"remote-only"}}).inferredDeletions(); len(got) != 0 {
		t.Fatalf("missing bootstrap path became deletion: %v", got)
	}
}
