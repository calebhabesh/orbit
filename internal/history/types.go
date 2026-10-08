// Package history implements Orbit's pure causal-history rules. It owns no
// filesystem, database, transport, clock, or user-interface behavior.
package history

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	ChunkSize       = uint64(1 << 20)
	MaxFileSize     = uint64(64) << 30
	MaxPathBytes    = 4096
	MaxPathSegments = 128
	MaxSegmentBytes = 255
	MaxHeads        = 64
	MaxParents      = 64
	MaxIdentities   = 64
)

var (
	ErrInvalidEnvelope = errors.New("invalid version envelope")
	ErrMissingParent   = errors.New("missing parent")
	ErrDuplicateID     = errors.New("version identity has different immutable fields")
	ErrCounterOverflow = errors.New("author counter overflow")
	ErrStaleBasis      = errors.New("working basis does not cover latest same-author event")
	ErrStaleView       = errors.New("reviewed head set is stale")
	ErrMembership      = errors.New("version is not admitted by membership state")
)

type ID [32]byte

func (id ID) MarshalText() ([]byte, error) {
	return []byte(hex.EncodeToString(id[:])), nil
}

func (id *ID) UnmarshalText(text []byte) error {
	raw, err := hex.DecodeString(string(text))
	if err != nil || len(raw) != 32 {
		return errors.New("ID must be 64 hexadecimal characters")
	}
	copy(id[:], raw)
	return nil
}

type Digest [32]byte

func (d Digest) MarshalText() ([]byte, error) {
	return []byte(hex.EncodeToString(d[:])), nil
}

func (d *Digest) UnmarshalText(text []byte) error {
	raw, err := hex.DecodeString(string(text))
	if err != nil || len(raw) != 32 {
		return errors.New("digest must be 64 hexadecimal characters")
	}
	copy(d[:], raw)
	return nil
}

type VersionID struct {
	Folder  ID
	Author  ID
	Counter uint64
}

func CompareVersionID(a, b VersionID) int {
	if value := bytes.Compare(a.Folder[:], b.Folder[:]); value != 0 {
		return value
	}
	if value := bytes.Compare(a.Author[:], b.Author[:]); value != 0 {
		return value
	}
	return compareUint64(a.Counter, b.Counter)
}

type ClockEntry struct {
	Author  ID
	Counter uint64
}

type Kind uint8

const (
	KindFile Kind = iota + 1
	KindDirectory
	KindTombstone
)

type Chunk struct {
	Digest Digest
	Length uint64
}

type Manifest struct {
	Size       uint64
	Digest     Digest
	Chunks     []Chunk
	Executable bool
}

type Envelope struct {
	ID               VersionID
	Path             string
	Parents          []VersionID
	Vector           []ClockEntry
	Kind             Kind
	Manifest         *Manifest
	AuthoredRevision uint64
	DisplayTime      string
}

type Relation uint8

const (
	Equal Relation = iota
	Before
	After
	Concurrent
)

func Compare(left, right Envelope) (Relation, error) {
	if left.ID.Folder != right.ID.Folder || left.Path != right.Path {
		return Equal, fmt.Errorf("%w: causal comparison requires the same folder and path", ErrInvalidEnvelope)
	}
	if err := validateVectorShape(left.Vector); err != nil {
		return Equal, err
	}
	if err := validateVectorShape(right.Vector); err != nil {
		return Equal, err
	}
	leftGreater, rightGreater := false, false
	i, j := 0, 0
	for i < len(left.Vector) || j < len(right.Vector) {
		var comparison int
		switch {
		case i == len(left.Vector):
			comparison = 1
		case j == len(right.Vector):
			comparison = -1
		default:
			comparison = bytes.Compare(left.Vector[i].Author[:], right.Vector[j].Author[:])
		}
		switch {
		case comparison < 0:
			leftGreater = left.Vector[i].Counter > 0
			i++
		case comparison > 0:
			rightGreater = right.Vector[j].Counter > 0
			j++
		default:
			leftGreater = leftGreater || left.Vector[i].Counter > right.Vector[j].Counter
			rightGreater = rightGreater || right.Vector[j].Counter > left.Vector[i].Counter
			i++
			j++
		}
	}
	switch {
	case leftGreater && rightGreater:
		return Concurrent, nil
	case leftGreater:
		return After, nil
	case rightGreater:
		return Before, nil
	default:
		return Equal, nil
	}
}

func validateVectorShape(vector []ClockEntry) error {
	if len(vector) == 0 || len(vector) > MaxIdentities {
		return fmt.Errorf("%w: vector size is invalid", ErrInvalidEnvelope)
	}
	for i, entry := range vector {
		if entry.Author == (ID{}) || entry.Counter == 0 || (i > 0 && bytes.Compare(vector[i-1].Author[:], entry.Author[:]) >= 0) {
			return fmt.Errorf("%w: vector is not sorted, unique, and nonzero", ErrInvalidEnvelope)
		}
	}
	return nil
}

func ValidatePath(path string) error {
	if path == "" || len(path) > MaxPathBytes || !utf8.ValidString(path) || strings.HasPrefix(path, "/") || strings.ContainsRune(path, '\x00') || strings.Contains(path, `\`) {
		return fmt.Errorf("%w: invalid relative path", ErrInvalidEnvelope)
	}
	segments := strings.Split(path, "/")
	if len(segments) > MaxPathSegments {
		return fmt.Errorf("%w: path has too many segments", ErrInvalidEnvelope)
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || len(segment) > MaxSegmentBytes || segment == ".orbit" || strings.HasPrefix(segment, ".orbit-") {
			return fmt.Errorf("%w: invalid or reserved path segment", ErrInvalidEnvelope)
		}
	}
	return nil
}

func validateManifest(kind Kind, manifest *Manifest) error {
	if kind != KindFile {
		if manifest != nil {
			return fmt.Errorf("%w: non-file version has a manifest", ErrInvalidEnvelope)
		}
		return nil
	}
	if manifest == nil || manifest.Size > MaxFileSize {
		return fmt.Errorf("%w: file manifest is absent or oversized", ErrInvalidEnvelope)
	}
	if manifest.Size == 0 {
		empty := sha256.Sum256(nil)
		if len(manifest.Chunks) != 0 || manifest.Digest != empty {
			return fmt.Errorf("%w: malformed empty-file manifest", ErrInvalidEnvelope)
		}
		return nil
	}
	expectedChunks := (manifest.Size + ChunkSize - 1) / ChunkSize
	if uint64(len(manifest.Chunks)) != expectedChunks {
		return fmt.Errorf("%w: chunk count does not match file size", ErrInvalidEnvelope)
	}
	var total uint64
	for index, chunk := range manifest.Chunks {
		want := ChunkSize
		if index == len(manifest.Chunks)-1 {
			want = manifest.Size - total
		}
		if chunk.Length != want || total > math.MaxUint64-chunk.Length {
			return fmt.Errorf("%w: invalid chunk length", ErrInvalidEnvelope)
		}
		total += chunk.Length
	}
	if total != manifest.Size {
		return fmt.Errorf("%w: chunks do not cover file size", ErrInvalidEnvelope)
	}
	return nil
}

func HeadToken(ids []VersionID) Digest {
	ordered := append([]VersionID(nil), ids...)
	sort.Slice(ordered, func(i, j int) bool { return CompareVersionID(ordered[i], ordered[j]) < 0 })
	hash := sha256.New()
	hash.Write([]byte("orbit-head-set-v1\x00"))
	var encoded [8]byte
	for _, id := range ordered {
		hash.Write(id.Folder[:])
		hash.Write(id.Author[:])
		binary.BigEndian.PutUint64(encoded[:], id.Counter)
		hash.Write(encoded[:])
	}
	var digest Digest
	copy(digest[:], hash.Sum(nil))
	return digest
}

func compareUint64(a, b uint64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// RetirementPolicy is the causal layer's immutable admission view. Membership
// revision construction and operator approval remain outside this package.
type RetirementPolicy struct {
	MembershipMatches bool
	Active            map[ID]bool
	Retired           map[ID]map[uint64]Digest
}

func (policy RetirementPolicy) Admit(id VersionID, envelopeDigest Digest, parentsAdmitted bool) error {
	if !policy.MembershipMatches {
		return fmt.Errorf("%w: membership digest mismatch", ErrMembership)
	}
	if !parentsAdmitted {
		return fmt.Errorf("%w: rejected or unknown ancestry", ErrMembership)
	}
	if policy.Active[id.Author] {
		return nil
	}
	versions, retired := policy.Retired[id.Author]
	if !retired || versions[id.Counter] != envelopeDigest {
		return fmt.Errorf("%w: identity/counter/digest absent from retirement snapshot", ErrMembership)
	}
	return nil
}
