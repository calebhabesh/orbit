package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sort"

	"github.com/calebhabesh/orbit/internal/history"
)

const MaxActiveMembers = 16

type ActiveMember struct {
	Device history.ID     `json:"device"`
	KeyPin history.Digest `json:"key_pin"`
}

type RetiredMember struct {
	Device         history.ID     `json:"device"`
	RetiredAt      uint64         `json:"retired_at"`
	SnapshotDigest history.Digest `json:"snapshot_digest"`
}

type Membership struct {
	Folder      history.ID      `json:"folder"`
	Revision    uint64          `json:"revision"`
	PriorDigest history.Digest  `json:"prior_digest"`
	Active      []ActiveMember  `json:"active"`
	Retired     []RetiredMember `json:"retired"`
}

// EncodeMembership returns the canonical membership byte stream specified by
// protocol v1. Entries are sorted by device ID; duplicate and overlapping
// identities are rejected.
func EncodeMembership(m Membership) ([]byte, error) {
	if m.Folder == (history.ID{}) || m.Revision == 0 || len(m.Active) == 0 || len(m.Active) > MaxActiveMembers || len(m.Active)+len(m.Retired) > history.MaxIdentities {
		return nil, errors.New("membership exceeds v1 limits or has zero required fields")
	}
	active := append([]ActiveMember(nil), m.Active...)
	retired := append([]RetiredMember(nil), m.Retired...)
	sort.Slice(active, func(i, j int) bool { return bytes.Compare(active[i].Device[:], active[j].Device[:]) < 0 })
	sort.Slice(retired, func(i, j int) bool { return bytes.Compare(retired[i].Device[:], retired[j].Device[:]) < 0 })
	seen := make(map[history.ID]bool, len(active)+len(retired))
	for _, member := range active {
		if member.Device == (history.ID{}) || member.KeyPin == (history.Digest{}) || seen[member.Device] {
			return nil, errors.New("membership has a zero or duplicate active member")
		}
		seen[member.Device] = true
	}
	for _, member := range retired {
		if member.Device == (history.ID{}) || member.RetiredAt == 0 || member.SnapshotDigest == (history.Digest{}) || seen[member.Device] {
			return nil, errors.New("membership has a zero, duplicate, or active-and-retired member")
		}
		seen[member.Device] = true
	}
	var out bytes.Buffer
	out.WriteString("orbit-membership-v1\x00")
	out.Write(m.Folder[:])
	_ = binary.Write(&out, binary.BigEndian, m.Revision)
	out.Write(m.PriorDigest[:])
	_ = binary.Write(&out, binary.BigEndian, uint16(len(active)))
	for _, member := range active {
		out.Write(member.Device[:])
		out.Write(member.KeyPin[:])
	}
	_ = binary.Write(&out, binary.BigEndian, uint16(len(retired)))
	for _, member := range retired {
		out.Write(member.Device[:])
		_ = binary.Write(&out, binary.BigEndian, member.RetiredAt)
		out.Write(member.SnapshotDigest[:])
	}
	return out.Bytes(), nil
}

func MembershipDigest(m Membership) (history.Digest, error) {
	encoded, err := EncodeMembership(m)
	if err != nil {
		return history.Digest{}, err
	}
	return sha256.Sum256(encoded), nil
}

type RetiredVersion struct {
	Counter        uint64
	EnvelopeDigest history.Digest
}

type RetirementSnapshot struct {
	Folder            history.ID
	ConfigurationRev  uint64
	RetiredDevice     history.ID
	AcceptedByRetiree []RetiredVersion
}

// EncodeRetirementSnapshot returns the canonical retirement snapshot byte stream
// specified by protocol v1. Versions are sorted by counter; duplicate counters are rejected.
func EncodeRetirementSnapshot(snapshot RetirementSnapshot) ([]byte, error) {
	if snapshot.Folder == (history.ID{}) || snapshot.ConfigurationRev == 0 || snapshot.RetiredDevice == (history.ID{}) {
		return nil, errors.New("retirement snapshot has zero required fields")
	}
	versions := append([]RetiredVersion(nil), snapshot.AcceptedByRetiree...)
	sort.Slice(versions, func(i, j int) bool { return versions[i].Counter < versions[j].Counter })
	for i := 1; i < len(versions); i++ {
		if versions[i-1].Counter == versions[i].Counter {
			return nil, errors.New("duplicate retired author counter")
		}
	}
	var out bytes.Buffer
	out.WriteString("orbit-retirement-v1\x00")
	out.Write(snapshot.Folder[:])
	_ = binary.Write(&out, binary.BigEndian, snapshot.ConfigurationRev)
	out.Write(snapshot.RetiredDevice[:])
	_ = binary.Write(&out, binary.BigEndian, uint64(len(versions)))
	for _, version := range versions {
		_ = binary.Write(&out, binary.BigEndian, version.Counter)
		out.Write(version.EnvelopeDigest[:])
	}
	return out.Bytes(), nil
}

func RetirementSnapshotDigest(snapshot RetirementSnapshot) (history.Digest, error) {
	encoded, err := EncodeRetirementSnapshot(snapshot)
	if err != nil {
		return history.Digest{}, err
	}
	return sha256.Sum256(encoded), nil
}

func DecodeRetirementSnapshot(data []byte) (RetirementSnapshot, error) {
	const prefix = "orbit-retirement-v1\x00"
	if len(data) < len(prefix)+32+8+32+8 {
		return RetirementSnapshot{}, errors.New("retirement snapshot data too short")
	}
	if string(data[:len(prefix)]) != prefix {
		return RetirementSnapshot{}, errors.New("invalid retirement snapshot prefix")
	}
	offset := len(prefix)
	var snapshot RetirementSnapshot
	copy(snapshot.Folder[:], data[offset:offset+32])
	offset += 32
	snapshot.ConfigurationRev = binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8
	copy(snapshot.RetiredDevice[:], data[offset:offset+32])
	offset += 32
	count := binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8
	if uint64(len(data)-offset) != count*40 {
		return RetirementSnapshot{}, errors.New("retirement snapshot payload length mismatch")
	}
	snapshot.AcceptedByRetiree = make([]RetiredVersion, count)
	for i := uint64(0); i < count; i++ {
		counter := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8
		var digest history.Digest
		copy(digest[:], data[offset:offset+32])
		offset += 32
		if i > 0 && counter <= snapshot.AcceptedByRetiree[i-1].Counter {
			return RetirementSnapshot{}, errors.New("retirement snapshot versions not strictly monotonically sorted")
		}
		snapshot.AcceptedByRetiree[i] = RetiredVersion{Counter: counter, EnvelopeDigest: digest}
	}
	return snapshot, nil
}
