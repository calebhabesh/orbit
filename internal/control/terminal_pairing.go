package control

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/replication"
)

type pairingRecord struct {
	fingerprint string
	result      tc.Result
	err         error
	expires     time.Time
}

func (c *Controller) terminalPairing(ctx context.Context, m tc.Mutation, fingerprint string) (tc.Result, error) {
	// A separate lane avoids holding the main control lock while the inviter
	// answers. Results are private, bounded, and lost on daemon restart.
	c.pairMu.Lock()
	defer c.pairMu.Unlock()
	r := terminalResult()
	if c.pairResults == nil {
		c.pairResults = map[string]pairingRecord{}
	}
	for id, v := range c.pairResults {
		if !c.options.Now().Before(v.expires) {
			delete(c.pairResults, id)
		}
	}
	if old, ok := c.pairResults[m.OperationID]; ok {
		if old.fingerprint != fingerprint {
			return r, terminalError("IDEMPOTENCY_CONFLICT")
		}
		return old.result, old.err
	}
	if len(c.pairResults) >= 8 {
		return r, terminalError("RATE_LIMITED")
	}
	client, closeClient, err := c.pairingClient(m.Pairing.Profile)
	if err != nil {
		return r, err
	}
	defer closeClient()
	plain, device, pin, err := client.JoinPairing(ctx, m.Pairing.Code)
	if err == nil {
		var inv tc.Invitation
		inv, _, err = tc.DecodeInvitationText(plain)
		if err == nil {
			err = inv.Validate()
		}
		if err == nil && (inv.Version != "3" || inv.Inviter != device || inv.KeyPin != pin || inv.Route.Profile != m.Pairing.Profile) {
			err = terminalError("IDENTITY_MISMATCH")
		}
		if err == nil {
			r.Invitation = &inv
		}
	}
	clear(plain)
	if err != nil {
		err = pairingControlError(err)
	}
	c.pairResults[m.OperationID] = pairingRecord{fingerprint, r, err, c.options.Now().Add(10 * time.Minute)}
	return r, err
}

func (c *Controller) pairingClient(digest string) (*network.ServiceClient, func(), error) {
	if c.options.NetworkService != nil && c.options.NetworkPolicy != nil && c.options.NetworkPolicy.Profile == digest {
		return c.options.NetworkService, func() {}, nil
	}
	selection, err := config.LoadNetworkProfile(c.db.StateDir(), uint64(c.options.Now().Unix()))
	if err != nil || profileDigest(selection) != digest {
		builtin, ok := network.BuiltinProfile()
		if !ok || profileDigest(builtin) != digest {
			return nil, nil, terminalError("PROFILE_OPERATOR_MISMATCH")
		}
		selection = builtin
	}
	id, err := replication.LoadOrCreateIdentity(c.db.StateDir(), c.options.LocalDevice, c.options.Now())
	if err != nil {
		return nil, nil, err
	}
	roots, _, err := config.LoadServiceRoots(c.db.StateDir(), c.options.Now())
	if err != nil {
		return nil, nil, err
	}
	if selection.Environment == "release" {
		roots = nil
	}
	origin := ""
	for _, o := range selection.Profile.Origins {
		if strings.HasPrefix(o, "https://") {
			origin = o
			break
		}
	}
	client, err := network.NewServiceClient(network.ServiceClientOptions{Selection: selection, Origin: origin, Device: hex.EncodeToString(id.DeviceID[:]), Certificate: id.Certificate, Roots: roots})
	if err != nil {
		return nil, nil, err
	}
	return client, func() { _ = client.Close() }, nil
}
func profileDigest(s network.ProfileSelection) string { d, _ := s.Digest(); return d }

func pairingControlError(err error) error {
	code := err.Error()
	var ce *ControlError
	if errors.As(err, &ce) {
		return ce
	}
	action := "Ask the inviting device for a new code; both devices must use the same Orbit operator."
	switch code {
	case "PAIRING_CODE_INVALID":
		action = "Enter all eight characters shown on the inviting device."
	case "PAIRING_WRONG_CODE":
		action = "The code did not match. Ask the inviting device for a new code."
	case "PAIRING_ALREADY_USED":
		action = "This code was already claimed. Ask the inviting device for a new code."
	case "UNSUPPORTED_CAPABILITY":
		action = "This service needs an update; use a long invitation or private invitation file."
	case "QUOTA_EXCEEDED":
		action = "The service is busy. Wait a minute and request a new code."
	default:
		code = "PAIRING_UNAVAILABLE"
	}
	return &ControlError{Code: code, Message: code, Action: action}
}
