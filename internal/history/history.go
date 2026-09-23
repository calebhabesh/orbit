package history

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
)

type History struct {
	versions map[VersionID]Envelope
}

func New() *History { return &History{versions: make(map[VersionID]Envelope)} }

// Accept validates and retains an immutable envelope. Missing ancestry is a
// retryable condition; conflicting reuse of an identity is an integrity error.
func (h *History) Accept(candidate Envelope) error {
	if existing, ok := h.versions[candidate.ID]; ok {
		if envelopesEqual(existing, candidate) {
			return nil
		}
		return ErrDuplicateID
	}
	if err := h.validate(candidate); err != nil {
		return err
	}
	h.versions[candidate.ID] = cloneEnvelope(candidate)
	return nil
}

func (h *History) validate(candidate Envelope) error {
	if candidate.ID.Folder == (ID{}) || candidate.ID.Author == (ID{}) || candidate.ID.Counter == 0 || candidate.AuthoredRevision == 0 {
		return fmt.Errorf("%w: zero identity, counter, or membership revision", ErrInvalidEnvelope)
	}
	if err := ValidatePath(candidate.Path); err != nil {
		return err
	}
	if candidate.Kind < KindFile || candidate.Kind > KindTombstone {
		return fmt.Errorf("%w: unknown kind", ErrInvalidEnvelope)
	}
	if err := validateManifest(candidate.Kind, candidate.Manifest); err != nil {
		return err
	}
	if len(candidate.Parents) > MaxParents {
		return fmt.Errorf("%w: parent or vector limit exceeded", ErrInvalidEnvelope)
	}
	for i, parentID := range candidate.Parents {
		if parentID.Folder != candidate.ID.Folder || (i > 0 && CompareVersionID(candidate.Parents[i-1], parentID) >= 0) {
			return fmt.Errorf("%w: parents are not sorted unique same-folder identities", ErrInvalidEnvelope)
		}
	}
	if err := validateVectorShape(candidate.Vector); err != nil {
		return err
	}
	vector := make(map[ID]uint64, len(candidate.Vector))
	for _, entry := range candidate.Vector {
		vector[entry.Author] = entry.Counter
	}
	if vector[candidate.ID.Author] != candidate.ID.Counter {
		return fmt.Errorf("%w: author vector component differs from event counter", ErrInvalidEnvelope)
	}
	derived := make(map[ID]uint64)
	for _, parentID := range candidate.Parents {
		parent, ok := h.versions[parentID]
		if !ok {
			return fmt.Errorf("%w: %v", ErrMissingParent, parentID)
		}
		if parent.Path != candidate.Path {
			return fmt.Errorf("%w: parent is for another path", ErrInvalidEnvelope)
		}
		for _, entry := range parent.Vector {
			if entry.Counter > derived[entry.Author] {
				derived[entry.Author] = entry.Counter
			}
		}
	}
	if candidate.ID.Counter <= derived[candidate.ID.Author] {
		return fmt.Errorf("%w: author counter does not advance parents", ErrInvalidEnvelope)
	}
	derived[candidate.ID.Author] = candidate.ID.Counter
	if !vectorsEqualMap(candidate.Vector, derived) {
		return fmt.Errorf("%w: vector is not derived from explicit parents", ErrInvalidEnvelope)
	}

	for id, existing := range h.versions {
		if id.Folder != candidate.ID.Folder {
			continue
		}
		if id.Author == candidate.ID.Author && id.Counter == candidate.ID.Counter {
			return ErrDuplicateID
		}
		if existing.Path != candidate.Path || id.Author != candidate.ID.Author {
			continue
		}
		if !h.candidateReaches(candidate, id) {
			return fmt.Errorf("%w: same-author event does not explicitly descend from prior same-path event", ErrInvalidEnvelope)
		}
	}
	h.versions[candidate.ID] = cloneEnvelope(candidate)
	headCount := len(h.Heads(candidate.ID.Folder, candidate.Path))
	delete(h.versions, candidate.ID)
	if headCount > MaxHeads {
		return fmt.Errorf("%w: concurrent head limit exceeded", ErrInvalidEnvelope)
	}
	return nil
}

func (h *History) candidateReaches(candidate Envelope, target VersionID) bool {
	seen := map[VersionID]bool{}
	var visit func(VersionID) bool
	visit = func(id VersionID) bool {
		if id == target {
			return true
		}
		if seen[id] {
			return false
		}
		seen[id] = true
		envelope, ok := h.versions[id]
		if !ok {
			return false
		}
		for _, parent := range envelope.Parents {
			if visit(parent) {
				return true
			}
		}
		return false
	}
	for _, parent := range candidate.Parents {
		if visit(parent) {
			return true
		}
	}
	return false
}

func (h *History) Envelope(id VersionID) (Envelope, bool) {
	envelope, ok := h.versions[id]
	return cloneEnvelope(envelope), ok
}

func (h *History) Heads(folder ID, path string) []Envelope {
	var candidates []Envelope
	for _, envelope := range h.versions {
		if envelope.ID.Folder == folder && envelope.Path == path {
			candidates = append(candidates, envelope)
		}
	}
	result := make([]Envelope, 0, len(candidates))
	for i, candidate := range candidates {
		dominated := false
		for j, other := range candidates {
			if i == j {
				continue
			}
			relation, _ := Compare(candidate, other)
			if relation == Before {
				dominated = true
				break
			}
		}
		if !dominated {
			result = append(result, cloneEnvelope(candidate))
		}
	}
	sort.Slice(result, func(i, j int) bool { return CompareVersionID(result[i].ID, result[j].ID) < 0 })
	return result
}

// NormalizeHeads is the stable comparison surface used by fixtures and the
// independent oracle adapter. It deliberately excludes content availability
// and working-copy state.
func NormalizeHeads(heads []Envelope) []string {
	copyHeads := append([]Envelope(nil), heads...)
	sort.Slice(copyHeads, func(i, j int) bool { return CompareVersionID(copyHeads[i].ID, copyHeads[j].ID) < 0 })
	result := make([]string, len(copyHeads))
	for i, head := range copyHeads {
		var vector strings.Builder
		for index, entry := range head.Vector {
			if index > 0 {
				vector.WriteByte(',')
			}
			vector.WriteString(hex.EncodeToString(entry.Author[:]))
			fmt.Fprintf(&vector, "=%d", entry.Counter)
		}
		result[i] = fmt.Sprintf("%s:%d[%s]", hex.EncodeToString(head.ID.Author[:]), head.ID.Counter, vector.String())
	}
	return result
}

// PlanOrdinaryCapture returns exactly the working-basis parents. It rejects a
// stale basis rather than fabricating a same-author sibling or adopting remote heads.
func (h *History) PlanOrdinaryCapture(folder, author ID, path string, basis []VersionID, nextCounter uint64) ([]VersionID, []ClockEntry, error) {
	if nextCounter == 0 {
		return nil, nil, ErrCounterOverflow
	}
	parents, err := h.validatedBasis(folder, path, basis)
	if err != nil {
		return nil, nil, err
	}
	var latest *Envelope
	for _, envelope := range h.versions {
		if envelope.ID.Folder == folder && envelope.ID.Author == author && envelope.Path == path && (latest == nil || envelope.ID.Counter > latest.ID.Counter) {
			copy := envelope
			latest = &copy
		}
	}
	if latest != nil {
		covered := false
		for _, parent := range parents {
			relation, _ := Compare(parent, *latest)
			if parent.ID == latest.ID || relation == After {
				covered = true
				break
			}
		}
		if !covered {
			return nil, nil, fmt.Errorf("%w: latest local event counter %d", ErrStaleBasis, latest.ID.Counter)
		}
		if latest.ID.Counter == math.MaxUint64 || nextCounter <= latest.ID.Counter {
			return nil, nil, ErrCounterOverflow
		}
	}
	vector, err := deriveVector(parents, author, nextCounter)
	return append([]VersionID(nil), basis...), vector, err
}

type ResolutionAction uint8

const (
	ResolutionSelect ResolutionAction = iota + 1
	ResolutionManualMerge
)

type ResolutionRequest struct {
	Folder            ID
	Path              string
	Reviewed          []VersionID
	ExpectedHeadToken Digest
	Action            ResolutionAction
	Selected          *VersionID
}

type ResolutionPlan struct {
	Parents  []VersionID
	Action   ResolutionAction
	Selected *VersionID
}

func (h *History) PlanResolution(request ResolutionRequest) (ResolutionPlan, error) {
	heads := h.Heads(request.Folder, request.Path)
	if len(heads) == 0 {
		return ResolutionPlan{}, fmt.Errorf("%w: no heads to resolve", ErrInvalidEnvelope)
	}
	ids := make([]VersionID, len(heads))
	for i := range heads {
		ids[i] = heads[i].ID
	}
	if request.ExpectedHeadToken != HeadToken(ids) || !sameIDs(ids, request.Reviewed) {
		return ResolutionPlan{}, ErrStaleView
	}
	if request.Action != ResolutionSelect && request.Action != ResolutionManualMerge {
		return ResolutionPlan{}, fmt.Errorf("%w: unknown resolution action", ErrInvalidEnvelope)
	}
	if request.Action == ResolutionSelect {
		if request.Selected == nil || !containsID(ids, *request.Selected) {
			return ResolutionPlan{}, fmt.Errorf("%w: selected version was not reviewed", ErrInvalidEnvelope)
		}
	} else if request.Selected != nil {
		return ResolutionPlan{}, fmt.Errorf("%w: manual merge cannot select stored bytes", ErrInvalidEnvelope)
	}
	parents := append([]VersionID(nil), ids...)
	return ResolutionPlan{Parents: parents, Action: request.Action, Selected: request.Selected}, nil
}

type StructuralConflict struct {
	AncestorPath   string
	Ancestor       VersionID
	DescendantPath string
	Descendant     VersionID
}

func (h *History) StructuralConflicts(folder ID) []StructuralConflict {
	var conflicts []StructuralConflict
	paths := map[string]bool{}
	for _, envelope := range h.versions {
		if envelope.ID.Folder == folder {
			paths[envelope.Path] = true
		}
	}
	for path := range paths {
		for _, descendant := range h.Heads(folder, path) {
			if descendant.Kind == KindTombstone {
				continue
			}
			for ancestorPath := parentPath(descendant.Path); ancestorPath != ""; ancestorPath = parentPath(ancestorPath) {
				for _, ancestor := range h.Heads(folder, ancestorPath) {
					if ancestor.Kind == KindFile || ancestor.Kind == KindTombstone {
						conflicts = append(conflicts, StructuralConflict{ancestorPath, ancestor.ID, descendant.Path, descendant.ID})
					}
				}
			}
		}
	}
	sort.Slice(conflicts, func(i, j int) bool {
		a, b := conflicts[i], conflicts[j]
		if a.AncestorPath != b.AncestorPath {
			return a.AncestorPath < b.AncestorPath
		}
		if a.DescendantPath != b.DescendantPath {
			return a.DescendantPath < b.DescendantPath
		}
		return CompareVersionID(a.Descendant, b.Descendant) < 0
	})
	return conflicts
}

func (h *History) validatedBasis(folder ID, path string, ids []VersionID) ([]Envelope, error) {
	if len(ids) > MaxParents {
		return nil, fmt.Errorf("%w: too many basis versions", ErrInvalidEnvelope)
	}
	parents := make([]Envelope, len(ids))
	for i, id := range ids {
		if id.Folder != folder || (i > 0 && CompareVersionID(ids[i-1], id) >= 0) {
			return nil, fmt.Errorf("%w: basis is not sorted unique", ErrInvalidEnvelope)
		}
		var ok bool
		parents[i], ok = h.versions[id]
		if !ok {
			return nil, fmt.Errorf("%w: %v", ErrMissingParent, id)
		}
		if parents[i].Path != path {
			return nil, fmt.Errorf("%w: basis path mismatch", ErrInvalidEnvelope)
		}
	}
	return parents, nil
}

func deriveVector(parents []Envelope, author ID, counter uint64) ([]ClockEntry, error) {
	joined := map[ID]uint64{}
	for _, p := range parents {
		for _, e := range p.Vector {
			if e.Counter > joined[e.Author] {
				joined[e.Author] = e.Counter
			}
		}
	}
	if counter == 0 || counter <= joined[author] {
		return nil, ErrCounterOverflow
	}
	joined[author] = counter
	result := make([]ClockEntry, 0, len(joined))
	for id, value := range joined {
		result = append(result, ClockEntry{id, value})
	}
	sort.Slice(result, func(i, j int) bool { return bytes.Compare(result[i].Author[:], result[j].Author[:]) < 0 })
	return result, nil
}

func vectorsEqualMap(vector []ClockEntry, values map[ID]uint64) bool {
	if len(vector) != len(values) {
		return false
	}
	for _, entry := range vector {
		if values[entry.Author] != entry.Counter {
			return false
		}
	}
	return true
}
func containsID(ids []VersionID, id VersionID) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}
func sameIDs(a, b []VersionID) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]VersionID(nil), a...)
	bb := append([]VersionID(nil), b...)
	sort.Slice(aa, func(i, j int) bool { return CompareVersionID(aa[i], aa[j]) < 0 })
	sort.Slice(bb, func(i, j int) bool { return CompareVersionID(bb[i], bb[j]) < 0 })
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}
func parentPath(path string) string {
	index := strings.LastIndexByte(path, '/')
	if index < 0 {
		return ""
	}
	return path[:index]
}
func cloneEnvelope(e Envelope) Envelope {
	e.Parents = append([]VersionID(nil), e.Parents...)
	e.Vector = append([]ClockEntry(nil), e.Vector...)
	if e.Manifest != nil {
		m := *e.Manifest
		m.Chunks = append([]Chunk(nil), e.Manifest.Chunks...)
		e.Manifest = &m
	}
	return e
}
func envelopesEqual(a, b Envelope) bool {
	return a.ID == b.ID && a.Path == b.Path && a.Kind == b.Kind && a.AuthoredRevision == b.AuthoredRevision && a.DisplayTime == b.DisplayTime && sameIDsOrdered(a.Parents, b.Parents) && clockEqual(a.Vector, b.Vector) && manifestEqual(a.Manifest, b.Manifest)
}
func sameIDsOrdered(a, b []VersionID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func clockEqual(a, b []ClockEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func manifestEqual(a, b *Manifest) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Size != b.Size || a.Digest != b.Digest || a.Executable != b.Executable || len(a.Chunks) != len(b.Chunks) {
		return false
	}
	for i := range a.Chunks {
		if a.Chunks[i] != b.Chunks[i] {
			return false
		}
	}
	return true
}
