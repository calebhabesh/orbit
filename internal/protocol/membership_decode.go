package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"

	"github.com/calebhabesh/file-sync/internal/history"
)

// DecodeMembership accepts only the bounded canonical v1 byte representation.
func DecodeMembership(data []byte) (Membership, error) {
	var m Membership
	domain := []byte("filesync-membership-v1\x00")
	if len(data) > len(domain)+76+history.MaxIdentities*72 || !bytes.HasPrefix(data, domain) {
		return m, errors.New("invalid membership encoding")
	}
	r := bytes.NewReader(data[len(domain):])
	if _, err := io.ReadFull(r, m.Folder[:]); err != nil {
		return m, err
	}
	if err := binary.Read(r, binary.BigEndian, &m.Revision); err != nil {
		return m, err
	}
	if _, err := io.ReadFull(r, m.PriorDigest[:]); err != nil {
		return m, err
	}
	var count uint16
	if err := binary.Read(r, binary.BigEndian, &count); err != nil {
		return m, err
	}
	if count == 0 || count > MaxActiveMembers {
		return m, errors.New("invalid active member count")
	}
	for i := 0; i < int(count); i++ {
		var a ActiveMember
		if _, err := io.ReadFull(r, a.Device[:]); err != nil {
			return m, err
		}
		if _, err := io.ReadFull(r, a.KeyPin[:]); err != nil {
			return m, err
		}
		m.Active = append(m.Active, a)
	}
	if err := binary.Read(r, binary.BigEndian, &count); err != nil {
		return m, err
	}
	if int(count)+len(m.Active) > history.MaxIdentities {
		return m, errors.New("invalid retired member count")
	}
	for i := 0; i < int(count); i++ {
		var a RetiredMember
		if _, err := io.ReadFull(r, a.Device[:]); err != nil {
			return m, err
		}
		if err := binary.Read(r, binary.BigEndian, &a.RetiredAt); err != nil {
			return m, err
		}
		if _, err := io.ReadFull(r, a.SnapshotDigest[:]); err != nil {
			return m, err
		}
		m.Retired = append(m.Retired, a)
	}
	canonical, err := EncodeMembership(m)
	if err != nil {
		return m, err
	}
	if r.Len() != 0 || !bytes.Equal(canonical, data) {
		return m, errors.New("noncanonical membership encoding")
	}
	return m, nil
}
