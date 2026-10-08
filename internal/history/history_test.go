package history

import (
	"crypto/sha256"
	"errors"
	"math"
	"strings"
	"testing"
)

func testID(label byte) ID {
	var id ID
	for i := range id {
		id[i] = label
	}
	return id
}
func testDigest(label byte) Digest {
	var digest Digest
	for i := range digest {
		digest[i] = label
	}
	return digest
}
func fileManifest(content string, executable bool) *Manifest {
	digest := sha256.Sum256([]byte(content))
	manifest := &Manifest{Size: uint64(len(content)), Digest: digest, Executable: executable}
	if content != "" {
		manifest.Chunks = []Chunk{{Digest: digest, Length: uint64(len(content))}}
	}
	return manifest
}
func envelope(folder, author ID, counter uint64, path string, kind Kind, parents []Envelope, manifest *Manifest) Envelope {
	parentIDs := make([]VersionID, len(parents))
	for i := range parents {
		parentIDs[i] = parents[i].ID
	}
	// Test helpers use label-ordered authors and parents.
	vector, err := deriveVector(parents, author, counter)
	if err != nil {
		panic(err)
	}
	return Envelope{ID: VersionID{folder, author, counter}, Path: path, Parents: parentIDs, Vector: vector, Kind: kind, Manifest: manifest, AuthoredRevision: 1}
}

func TestRequiredCausalFixtures(t *testing.T) {
	folder, a, b, c := testID('F'), testID('A'), testID('B'), testID('C')
	path := "notes/plan.txt"
	h := New()
	a1 := envelope(folder, a, 1, path, KindFile, nil, fileManifest("same", false))
	b1 := envelope(folder, b, 1, path, KindFile, nil, fileManifest("same", false))
	c1 := envelope(folder, c, 1, path, KindFile, nil, fileManifest("c", false))
	for _, event := range []Envelope{b1, a1, c1} {
		if err := h.Accept(event); err != nil {
			t.Fatal(err)
		}
	}
	if got := h.Heads(folder, path); len(got) != 3 {
		t.Fatalf("offline heads=%d, want 3", len(got))
	}
	a2 := envelope(folder, a, 2, path, KindFile, []Envelope{a1, b1}, fileManifest("resolved", false))
	if err := h.Accept(a2); err != nil {
		t.Fatal(err)
	}
	heads := h.Heads(folder, path)
	if len(heads) != 2 || heads[0].ID != a2.ID || heads[1].ID != c1.ID {
		t.Fatalf("resolution heads=%v, want A2/C1", heads)
	}
	if relation, _ := Compare(a1, b1); relation != Concurrent {
		t.Fatalf("equal-byte edits relation=%v, want concurrent", relation)
	}

	deletePath := "old.txt"
	tombstone := envelope(folder, a, 3, deletePath, KindTombstone, nil, nil)
	edit := envelope(folder, b, 2, deletePath, KindFile, nil, fileManifest("kept", false))
	if err := h.Accept(tombstone); err != nil {
		t.Fatal(err)
	}
	if err := h.Accept(edit); err != nil {
		t.Fatal(err)
	}
	if len(h.Heads(folder, deletePath)) != 2 {
		t.Fatal("delete/edit conflict did not preserve both heads")
	}
}

func TestExecutableOnlyEditAndEmptyFileAreVersions(t *testing.T) {
	folder, author := testID('F'), testID('A')
	h := New()
	empty := envelope(folder, author, 1, "empty", KindFile, nil, fileManifest("", false))
	if err := h.Accept(empty); err != nil {
		t.Fatal(err)
	}
	plain := envelope(folder, author, 2, "script", KindFile, nil, fileManifest("echo", false))
	executable := envelope(folder, author, 3, "script", KindFile, []Envelope{plain}, fileManifest("echo", true))
	if err := h.Accept(plain); err != nil {
		t.Fatal(err)
	}
	if err := h.Accept(executable); err != nil {
		t.Fatal(err)
	}
	if heads := h.Heads(folder, "script"); len(heads) != 1 || !heads[0].Manifest.Executable {
		t.Fatal("mode-only successor was not retained")
	}
}

func TestDuplicateIdentityMissingParentMalformedVectorAndOverflowRejected(t *testing.T) {
	folder, a, b := testID('F'), testID('A'), testID('B')
	h := New()
	a1 := envelope(folder, a, 1, "x", KindFile, nil, fileManifest("a", false))
	if err := h.Accept(a1); err != nil {
		t.Fatal(err)
	}
	if err := h.Accept(a1); err != nil {
		t.Fatalf("idempotent duplicate: %v", err)
	}
	changed := a1
	changed.Kind = KindTombstone
	changed.Manifest = nil
	if !errors.Is(h.Accept(changed), ErrDuplicateID) {
		t.Fatalf("changed duplicate error=%v", h.Accept(changed))
	}
	missing := envelope(folder, b, 1, "x", KindFile, nil, fileManifest("b", false))
	missing.Parents = []VersionID{{Folder: folder, Author: b, Counter: 99}}
	missing.Vector = []ClockEntry{{Author: b, Counter: 1}}
	if !errors.Is(h.Accept(missing), ErrMissingParent) {
		t.Fatalf("missing parent error=%v", h.Accept(missing))
	}
	bad := envelope(folder, b, 1, "x", KindFile, nil, fileManifest("b", false))
	bad.Vector = []ClockEntry{{Author: a, Counter: 4}, {Author: b, Counter: 1}}
	if !errors.Is(h.Accept(bad), ErrInvalidEnvelope) {
		t.Fatalf("invented vector error=%v", h.Accept(bad))
	}
	if _, _, err := h.PlanOrdinaryCapture(folder, a, "x", []VersionID{a1.ID}, 0); !errors.Is(err, ErrCounterOverflow) {
		t.Fatalf("zero counter error=%v", err)
	}
	max := a1
	max.ID.Counter = math.MaxUint64
	max.Vector = []ClockEntry{{Author: a, Counter: math.MaxUint64}}
	max.Path = "max"
	if err := h.Accept(max); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.PlanOrdinaryCapture(folder, a, "max", []VersionID{max.ID}, math.MaxUint64); !errors.Is(err, ErrCounterOverflow) {
		t.Fatalf("overflow error=%v", err)
	}
}

func TestFolderWideCounterGapDoesNotFakeSamePathAncestry(t *testing.T) {
	folder, author := testID('F'), testID('A')
	h := New()
	// Counter 2 can legitimately be the first event at this path because
	// counter 1 may have been used elsewhere. It cannot later dominate a newly
	// presented counter-1 event at the same path without an explicit parent.
	a2 := envelope(folder, author, 2, "x", KindFile, nil, fileManifest("two", false))
	if err := h.Accept(a2); err != nil {
		t.Fatal(err)
	}
	a1 := envelope(folder, author, 1, "x", KindFile, nil, fileManifest("one", false))
	if err := h.Accept(a1); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("late same-author sibling error=%v", err)
	}
}

func TestOrdinaryCaptureUsesBasisAndBlocksSameAuthorStaleBasis(t *testing.T) {
	folder, a, b := testID('F'), testID('A'), testID('B')
	h := New()
	a1 := envelope(folder, a, 1, "x", KindFile, nil, fileManifest("a1", false))
	b1 := envelope(folder, b, 1, "x", KindFile, nil, fileManifest("b1", false))
	if err := h.Accept(a1); err != nil {
		t.Fatal(err)
	}
	if err := h.Accept(b1); err != nil {
		t.Fatal(err)
	}
	parents, vector, err := h.PlanOrdinaryCapture(folder, a, "x", []VersionID{a1.ID}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(parents) != 1 || parents[0] != a1.ID || len(vector) != 1 {
		t.Fatalf("ordinary plan adopted remote head: parents=%v vector=%v", parents, vector)
	}
	a2 := Envelope{ID: VersionID{folder, a, 2}, Path: "x", Parents: parents, Vector: vector, Kind: KindFile, Manifest: fileManifest("a2", false), AuthoredRevision: 1}
	if err := h.Accept(a2); err != nil {
		t.Fatal(err)
	}
	heads := h.Heads(folder, "x")
	if len(heads) != 2 || heads[0].ID != a2.ID || heads[1].ID != b1.ID {
		t.Fatalf("ordinary capture heads=%v, want local successor and unreviewed remote head", heads)
	}
	if _, _, err := h.PlanOrdinaryCapture(folder, a, "x", []VersionID{a1.ID}, 3); !errors.Is(err, ErrStaleBasis) {
		t.Fatalf("stale basis error=%v", err)
	}
}

func TestReviewedResolutionAndStructuralConflict(t *testing.T) {
	folder, a, b := testID('F'), testID('A'), testID('B')
	h := New()
	a1 := envelope(folder, a, 1, "x", KindFile, nil, fileManifest("a", false))
	b1 := envelope(folder, b, 1, "x", KindFile, nil, fileManifest("b", false))
	for _, e := range []Envelope{a1, b1} {
		if err := h.Accept(e); err != nil {
			t.Fatal(err)
		}
	}
	ids := []VersionID{a1.ID, b1.ID}
	selected := a1.ID
	plan, err := h.PlanResolution(ResolutionRequest{folder, "x", ids, HeadToken(ids), ResolutionSelect, &selected})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Parents) != 2 {
		t.Fatal("resolution did not cover reviewed heads")
	}
	if _, err := h.PlanResolution(ResolutionRequest{folder, "x", ids, HeadToken([]VersionID{a1.ID}), ResolutionSelect, &selected}); !errors.Is(err, ErrStaleView) {
		t.Fatalf("stale token error=%v", err)
	}
	restorePlan, err := h.PlanResolution(ResolutionRequest{folder, "x", ids, HeadToken(ids), ResolutionRestore, &selected})
	if err != nil {
		t.Fatal(err)
	}
	if len(restorePlan.Parents) != 2 || *restorePlan.Selected != selected {
		t.Fatal("restore plan invalid")
	}
	missingID := VersionID{folder, a, 999}
	if _, err := h.PlanResolution(ResolutionRequest{folder, "x", ids, HeadToken(ids), ResolutionRestore, &missingID}); err == nil {
		t.Fatal("expected error for missing historical version in restore")
	}

	capturedParents, capturedVector, err := h.PlanResolutionCapture(folder, a, "x", ids, HeadToken(ids), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(capturedParents) != 2 {
		t.Fatalf("captured parents = %d, want 2", len(capturedParents))
	}
	if capturedVector[0].Counter != 2 && capturedVector[1].Counter != 2 {
		t.Fatalf("captured vector does not advance counter: %v", capturedVector)
	}
	if _, _, err := h.PlanResolutionCapture(folder, a, "x", ids, HeadToken([]VersionID{a1.ID}), 2); !errors.Is(err, ErrStaleView) {
		t.Fatalf("stale token error=%v, want ErrStaleView", err)
	}
	blocker := envelope(folder, a, 2, "parent", KindFile, nil, fileManifest("file", false))
	child := envelope(folder, b, 2, "parent/child", KindFile, nil, fileManifest("child", false))
	if err := h.Accept(blocker); err != nil {
		t.Fatal(err)
	}
	if err := h.Accept(child); err != nil {
		t.Fatal(err)
	}
	if conflicts := h.StructuralConflicts(folder); len(conflicts) != 1 || conflicts[0].Descendant != child.ID {
		t.Fatalf("structural conflicts=%v", conflicts)
	}
}

func TestPathManifestAndRetirementValidation(t *testing.T) {
	for _, path := range []string{"", "/absolute", "a//b", "a/../b", `a\b`, ".orbit/stage", strings.Repeat("x", 256)} {
		if err := ValidatePath(path); err == nil {
			t.Errorf("path %q accepted", path)
		}
	}
	policy := RetirementPolicy{MembershipMatches: true, Active: map[ID]bool{testID('A'): true}, Retired: map[ID]map[uint64]Digest{testID('B'): {2: testDigest('x')}}}
	if err := policy.Admit(VersionID{testID('F'), testID('B'), 2}, testDigest('x'), true); err != nil {
		t.Fatal(err)
	}
	if err := policy.Admit(VersionID{testID('F'), testID('B'), 3}, testDigest('x'), true); !errors.Is(err, ErrMembership) {
		t.Fatalf("unknown retired event error=%v", err)
	}
	policy.MembershipMatches = false
	if err := policy.Admit(VersionID{testID('F'), testID('A'), 1}, Digest{}, true); !errors.Is(err, ErrMembership) {
		t.Fatalf("mismatch error=%v", err)
	}
}
