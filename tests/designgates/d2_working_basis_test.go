package designgates

import (
	"fmt"
	"sort"
	"testing"
)

type modelVersion struct {
	id      string
	author  string
	parents []string
	bytes   string
	vector  map[string]uint64
}

type historyModel struct {
	versions map[string]modelVersion
	heads    map[string]bool
}

func newHistoryModel() *historyModel {
	return &historyModel{versions: map[string]modelVersion{}, heads: map[string]bool{}}
}

func (m *historyModel) add(id, author, bytes string, parents ...string) modelVersion {
	vector := map[string]uint64{}
	for _, parentID := range parents {
		parent, ok := m.versions[parentID]
		if !ok {
			panic("missing parent " + parentID)
		}
		for actor, counter := range parent.vector {
			if counter > vector[actor] {
				vector[actor] = counter
			}
		}
	}
	next := vector[author] + 1
	for _, version := range m.versions {
		if version.author == author && version.vector[author] >= next {
			next = version.vector[author] + 1
		}
	}
	version := modelVersion{id: id, author: author, parents: append([]string(nil), parents...), bytes: bytes, vector: vector}
	version.vector[author] = next
	m.versions[id] = version
	for headID := range m.heads {
		if dominates(version.vector, m.versions[headID].vector) {
			delete(m.heads, headID)
		}
	}
	m.heads[id] = true
	return version
}

func (m *historyModel) ordinaryCaptureAllowed(author string, workingBasis []string) (bool, string) {
	basis := map[string]bool{}
	for _, id := range workingBasis {
		basis[id] = true
	}
	var latest *modelVersion
	for _, version := range m.versions {
		if version.author == author && (latest == nil || version.vector[author] > latest.vector[author]) {
			copy := version
			latest = &copy
		}
	}
	if latest == nil {
		return true, "first local event"
	}
	for id := range basis {
		if id == latest.id || dominates(m.versions[id].vector, latest.vector) {
			return true, "basis contains latest same-author lineage"
		}
	}
	return false, fmt.Sprintf("latest same-author event %s is absent from working basis", latest.id)
}

func dominates(left, right map[string]uint64) bool {
	strict := false
	actors := map[string]bool{}
	for actor := range left {
		actors[actor] = true
	}
	for actor := range right {
		actors[actor] = true
	}
	for actor := range actors {
		if left[actor] < right[actor] {
			return false
		}
		if left[actor] > right[actor] {
			strict = true
		}
	}
	return strict
}

func (m *historyModel) headIDs() []string {
	ids := make([]string, 0, len(m.heads))
	for id := range m.heads {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func TestD2WorkingBasisDoesNotImplicitlyResolveReceivedHead(t *testing.T) {
	m := newHistoryModel()
	m.add("A1", "A", "local")
	m.add("B1", "B", "remote")
	if allowed, reason := m.ordinaryCaptureAllowed("A", []string{"A1"}); !allowed {
		t.Fatalf("ordinary edit from A1 unexpectedly blocked: %s", reason)
	}
	m.add("A2", "A", "next-local", "A1")
	want := []string{"A2", "B1"}
	if got := m.headIDs(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("heads = %v, want %v", got, want)
	}
	t.Log("A2 extends the working basis A1, not all known heads; unreviewed B1 remains concurrent")
}

func TestD2SameAuthorStaleBasisBlocksCandidate(t *testing.T) {
	m := newHistoryModel()
	m.add("A1", "A", "first")
	m.add("A2", "A", "second", "A1")
	allowed, reason := m.ordinaryCaptureAllowed("A", []string{"A1"})
	if allowed {
		t.Fatal("stale same-author basis was allowed to mint a sibling")
	}
	t.Logf("candidate bytes are preserved for review and event creation is blocked: %s", reason)
}

func TestD2ResolutionCoversOnlyReviewedHeads(t *testing.T) {
	m := newHistoryModel()
	m.add("A1", "A", "a")
	m.add("B1", "B", "b")
	m.add("C1", "C", "c")
	m.add("A2", "A", "resolved-a-b", "A1", "B1")
	want := []string{"A2", "C1"}
	if got := m.headIDs(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("heads = %v, want %v", got, want)
	}
}

func TestD2EqualBytesDoNotEraseAncestry(t *testing.T) {
	m := newHistoryModel()
	a := m.add("A1", "A", "same")
	b := m.add("B1", "B", "same")
	if dominates(a.vector, b.vector) || dominates(b.vector, a.vector) {
		t.Fatal("equal contents incorrectly affected causal comparison")
	}
	if got := len(m.headIDs()); got != 2 {
		t.Fatalf("head count = %d, want 2", got)
	}
}
