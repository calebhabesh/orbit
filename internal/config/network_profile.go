package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/calebhabesh/file-sync/internal/network"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/state"
)

func LoadNetworkProfile(dir string, now uint64) (network.ProfileSelection, error) {
	var s network.ProfileSelection
	b, err := state.ReadPrivate(dir, "network-profile.json", protocol.NetworkMaxBytes)
	if err != nil {
		return s, err
	}
	if err = protocol.NetworkDecode(b, &s); err != nil {
		return s, err
	}
	return s, s.Validate(now)
}

// ProfileFloorsFile retains the highest epoch accepted from every authority
// this device has used, so an operator switch cannot be used to roll back.
const ProfileFloorsFile = "network-profile-floors.json"

type profileFloors struct {
	Floors map[string]protocol.NetworkUint `json:"floors"`
}

// LoadProfileFloors returns the per-authority rollback floors. A missing file
// is empty: before W14 only one authority could ever have been accepted, and
// its floor is the stored selection's highest epoch.
func LoadProfileFloors(dir string) (map[string]uint64, error) {
	out := map[string]uint64{}
	b, err := state.ReadPrivate(dir, ProfileFloorsFile, protocol.NetworkMaxBytes)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	var f profileFloors
	if err = json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	for k, v := range f.Floors {
		if protocol.NetworkHex(k, 32) != nil {
			return nil, errors.New("PROFILE_UNTRUSTED")
		}
		out[k] = uint64(v)
	}
	return out, nil
}

// SaveNetworkProfile requires the same exclusive private state ownership as
// policy apply. It neither activates networking nor changes identities/history.
func SaveNetworkProfile(dir string, s network.ProfileSelection, now uint64) error {
	return SaveNetworkProfileChange(dir, s, now, false)
}

// SaveNetworkProfileChange additionally accepts a reviewed operator switch.
// The floors file is written first, so an interruption can only leave a floor
// at least as high as the selection that is actually stored.
func SaveNetworkProfileChange(dir string, s network.ProfileSelection, now uint64, replace bool) error {
	if err := state.ValidateDirectory(dir); err != nil {
		return err
	}
	if err := s.Validate(now); err != nil {
		return err
	}
	floors, err := LoadProfileFloors(dir)
	if err != nil {
		return err
	}
	old, err := state.ReadPrivate(dir, "network-profile.json", protocol.NetworkMaxBytes)
	if err == nil {
		var prior network.ProfileSelection
		if err = protocol.NetworkDecode(old, &prior); err != nil {
			return err
		}
		if _, err = network.ReviewProfileChange(&prior, floors, s.Profile, s.Authority, s.Environment, now, replace); err != nil {
			return err
		}
		if uint64(prior.HighestEpoch) > floors[prior.Authority] {
			floors[prior.Authority] = uint64(prior.HighestEpoch)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	} else if _, err = network.ReviewProfileChange(nil, floors, s.Profile, s.Authority, s.Environment, now, replace); err != nil {
		return err
	}
	if uint64(s.HighestEpoch) > floors[s.Authority] {
		floors[s.Authority] = uint64(s.HighestEpoch)
	}
	pf := profileFloors{Floors: map[string]protocol.NetworkUint{}}
	for k, v := range floors {
		pf.Floors[k] = protocol.NetworkUint(v)
	}
	fb, err := json.Marshal(pf)
	if err != nil {
		return err
	}
	if err = WritePrivate(dir, ProfileFloorsFile, fb); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".network-profile-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(f.Name(), filepath.Join(dir, "network-profile.json")); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
