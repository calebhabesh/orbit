package config

import (
	"encoding/json"
	"errors"
	"os"

	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/state"
)

// LoadNetworkPolicy is an additive migration scaffold. Missing intent means the
// legacy manual policy, with no write, announcement, identity or peer migration.
// Reviewed automatic/profile controls and their activation belong to W03/W06.
func LoadNetworkPolicy(dir string) (tc.NetworkPolicy, error) {
	policy := tc.NetworkPolicy{Mode: "manual", Generation: 1}
	b, err := state.ReadPrivate(dir, "network.json", tc.MaxMetadata)
	if errors.Is(err, os.ErrNotExist) {
		return policy, nil
	}
	if err != nil {
		return policy, err
	}
	if err = tc.Decode(b, &policy); err != nil {
		return policy, err
	}
	if err = policy.Validate(); err != nil {
		return policy, err
	}
	if policy.Generation == 0 {
		return policy, errors.New("INVALID_NETWORK_GENERATION")
	}
	return policy, nil
}

// SaveNetworkPolicy is called under exclusive controller/state ownership.
func SaveNetworkPolicy(dir string, policy tc.NetworkPolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	if policy.Generation == 0 {
		return errors.New("UNSUPPORTED_CAPABILITY")
	}
	prior, err := LoadNetworkPolicy(dir)
	if err != nil {
		return err
	}
	if policy.Generation < prior.Generation || (policy.Generation == prior.Generation && policy != prior) {
		return errors.New("STALE_VIEW")
	}
	b, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	return WritePrivate(dir, "network.json", b)
}
