package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sort"

	"github.com/calebhabesh/file-sync/internal/history"
)

const MaxActiveMembers = 16

type ActiveMember struct {
	Device history.ID
	KeyPin history.Digest
}

type RetiredMember struct {
	Device         history.ID
	RetiredAt      uint64
	SnapshotDigest history.Digest
}

type Membership struct {
	Folder      history.ID
	Revision    uint64
	PriorDigest history.Digest
	Active      []ActiveMember
	Retired     []RetiredMember
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
	out.WriteString("filesync-membership-v1\x00")
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
