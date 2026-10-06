package network

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"

	"github.com/calebhabesh/file-sync/internal/protocol"
)

// ProfileSelection is durable reviewed trust, never inferred from a downloaded
// profile. Development selections cannot be used as release defaults. Root CAs
// are supplied independently to the client; this object cannot disable TLS.
type ProfileSelection struct {
	Profile      protocol.NetworkProfile `json:"profile"`
	Authority    string                  `json:"authority"`
	HighestEpoch protocol.NetworkUint    `json:"highest_epoch"`
	Environment  string                  `json:"environment"` // release, self_hosted, development
}

func (s ProfileSelection) Validate(now uint64) error {
	if s.Environment != "release" && s.Environment != "self_hosted" && s.Environment != "development" {
		return errors.New("PROFILE_UNTRUSTED")
	}
	key, err := hex.DecodeString(s.Authority)
	if err != nil || s.HighestEpoch == 0 || s.Profile.Epoch != s.HighestEpoch {
		return errors.New("PROFILE_UNTRUSTED")
	}
	return s.Profile.Verify(ed25519.PublicKey(key), now, uint64(s.HighestEpoch), s.Environment != "release")
}
func (s ProfileSelection) Private() bool           { return s.Environment != "release" }
func (s ProfileSelection) Digest() (string, error) { return s.Profile.Digest(s.Private()) }

// ReviewProfile updates only under the independently reviewed authority. The
// caller owns exclusive state access and persists this together with its review.
func ReviewProfile(old *ProfileSelection, p protocol.NetworkProfile, authority, environment string, now uint64) (ProfileSelection, error) {
	return ReviewProfileChange(old, nil, p, authority, environment, now, false)
}

// ErrOperatorChange marks a review that names a different authority or
// environment without the explicit replacement the owner must confirm.
var ErrOperatorChange = errors.New("PROFILE_OPERATOR_CHANGE")

// ReviewProfileChange also admits a deliberately reviewed operator switch
// (replace). floors holds the highest epoch ever accepted per authority, so a
// switch back to an earlier operator cannot roll its profile back either.
func ReviewProfileChange(old *ProfileSelection, floors map[string]uint64, p protocol.NetworkProfile, authority, environment string, now uint64, replace bool) (ProfileSelection, error) {
	s := ProfileSelection{Profile: p, Authority: authority, HighestEpoch: p.Epoch, Environment: environment}
	switched := old != nil && (old.Authority != authority || old.Environment != environment)
	if switched && !replace {
		return s, ErrOperatorChange
	}
	if old != nil && !switched && p.Epoch < old.HighestEpoch {
		return s, errors.New("PROFILE_UNTRUSTED")
	}
	if floor, ok := floors[authority]; ok && uint64(p.Epoch) < floor {
		return s, errors.New("PROFILE_UNTRUSTED")
	}
	if err := s.Validate(now); err != nil {
		return s, err
	}
	if old != nil && !switched && p.Epoch == old.HighestEpoch {
		a, _ := old.Digest()
		b, _ := s.Digest()
		if a != b {
			return s, errors.New("PROFILE_UNTRUSTED")
		}
	}
	return s, nil
}
