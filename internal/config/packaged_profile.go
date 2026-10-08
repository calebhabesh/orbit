package config

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/state"
)

// Outcomes of AdoptPackagedProfile.
const (
	PackagedAdopted   = "adopted"   // Automatic awaiting a profile now uses the packaged one
	PackagedUpdated   = "updated"   // same operator/privacy text, newer epoch applied
	PackagedCompleted = "completed" // an interrupted profile/policy write was finished
	PackagedReview    = "review"    // newer epoch changes operator/privacy text; owner reviews
)

// AdoptPackagedProfile runs at daemon start under exclusive state ownership.
// It only acts for the Automatic mode the owner already chose, with the
// packaged release selection (BuiltinProfile pins its authority): an Automatic
// install still waiting for a profile adopts the packaged one, and a newer
// packaged epoch whose operator and privacy text are unchanged replaces the
// stored one. Manual, Local-only
// and self-hosted installs, other authorities and expired packaged profiles
// are left untouched. Writes go profile, then policy, then routes; a later
// start completes an interruption between them.
func AdoptPackagedProfile(dir string, packaged network.ProfileSelection, now uint64) (string, error) {
	if packaged.Environment != "release" || packaged.Validate(now) != nil {
		return "", nil
	}
	digest, err := packaged.Digest()
	if err != nil {
		return "", err
	}
	policy, err := LoadNetworkPolicy(dir)
	if err != nil || policy.Mode != "automatic" {
		return "", err
	}
	stored, err := LoadNetworkProfile(dir, 0) // expiry is irrelevant to adoption
	outcome := ""
	switch {
	case errors.Is(err, os.ErrNotExist):
		if !policy.AwaitingProfile && policy.Profile != digest {
			return "", nil
		}
		if err = SaveNetworkProfile(dir, packaged, now); err != nil {
			return "", err
		}
		outcome = PackagedAdopted
	case err != nil:
		return "", nil // invalid stored selection: reported by status, never overwritten here
	case stored.Environment != "release" || stored.Authority != packaged.Authority:
		return "", nil
	default:
		sd, e := stored.Digest()
		if e != nil {
			return "", e
		}
		switch {
		case sd == digest && policy.Profile == digest:
			return "", nil
		case sd == digest:
			outcome = PackagedCompleted
		case packaged.Profile.Epoch <= stored.HighestEpoch || policy.Profile != sd:
			return "", nil
		case stored.Profile.Operator != packaged.Profile.Operator || stored.Profile.Privacy != packaged.Profile.Privacy:
			return PackagedReview, nil
		default:
			if err = SaveNetworkProfile(dir, packaged, now); err != nil {
				return "", err
			}
			outcome = PackagedUpdated
		}
	}
	policy.Profile = digest
	policy.AwaitingProfile = false
	policy.Generation++
	if err = SaveNetworkPolicy(dir, policy); err != nil {
		return "", err
	}
	return outcome, RebindPeerRoutes(dir, digest)
}

// AutomaticOfferFile records that the owner of a manual install declined the
// one-time offer to review Automatic with the packaged profile.
const AutomaticOfferFile = "network-offer.json"

type automaticOffer struct {
	Declined bool `json:"declined"`
}

func AutomaticOfferDeclined(dir string) (bool, error) {
	b, err := state.ReadPrivate(dir, AutomaticOfferFile, 1024)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var o automaticOffer
	if err = json.Unmarshal(b, &o); err != nil {
		return false, err
	}
	return o.Declined, nil
}

func DeclineAutomaticOffer(dir string) error {
	b, _ := json.Marshal(automaticOffer{Declined: true})
	return WritePrivate(dir, AutomaticOfferFile, b)
}
