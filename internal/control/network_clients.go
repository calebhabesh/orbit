package control

import (
	"context"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
	"os"
	"sort"
	"strings"
	"time"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/replication"
)

// Only the live daemon injects a manager. Stopped-adapter admission/selection
// remains unchanged; later network controls cannot use a direct DB fallback.
func (c *Controller) enrollmentClient(ctx context.Context, inv tc.Invitation, id replication.Identity) (*replication.EnrollmentClient, error) {
	policy, err := config.LoadNetworkPolicy(c.db.StateDir())
	if err != nil {
		return nil, err
	}
	if policy.Mode == "local_only" {
		if inv.Version == "3" || !network.LocalEndpoint(inv.EnrollmentEndpoint) {
			return nil, terminalError("LOCAL_ONLY_ROUTE_UNAVAILABLE")
		}
	}
	if inv.Version == "3" {
		selection, err := config.LoadNetworkProfile(c.db.StateDir(), uint64(time.Now().Unix()))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) && policy.Mode == "manual" {
				return nil, &ControlError{Code: "NETWORK_REVIEW_REQUIRED", Message: "this invitation pairs through Orbit services, but this device uses manual connections", Action: "review Automatic with orbit network automatic, then resume the join"}
			}
			return nil, err
		}
		if err = profileMismatch(selection, inv); err != nil {
			return nil, err
		}
		if c.options.Network == nil || c.options.Relay == nil {
			return nil, terminalError("SERVICE_UNAVAILABLE")
		}
		target := network.Target{Device: terminalID(inv.Inviter), Pin: history.Digest(terminalID(inv.KeyPin)), Profile: history.Digest(terminalID(inv.Route.Profile)), Purpose: network.Enrollment}
		if err = c.options.Relay.Register(target); err != nil {
			return nil, err
		}
		return replication.NewV3EnrollmentClient(ctx, inv, id, selection, c.options.Network)
	}
	if c.options.Network == nil {
		return replication.NewEnrollmentClient(inv, id)
	}
	target := network.Target{Device: terminalID(inv.Inviter), Pin: history.Digest(terminalID(inv.KeyPin)), Purpose: network.Enrollment}
	if err := c.options.Network.SetManual(target, inv.EnrollmentEndpoint); err != nil {
		return nil, err
	}
	return replication.NewRoutedEnrollmentClient(ctx, inv, id, c.options.Network)
}
func (c *Controller) peerClient(ctx context.Context, endpoint string, id replication.Identity, cert *x509.Certificate, peer history.ID) (*replication.Client, error) {
	policy, err := config.LoadNetworkPolicy(c.db.StateDir())
	if err != nil {
		return nil, err
	}

	pin := replication.PublicKeyPin(cert)
	routes, err := config.LoadPeerRoutes(c.db.StateDir())
	if err != nil {
		return nil, err
	}
	for _, route := range routes {
		if route.Device == hex.EncodeToString(peer[:]) && route.Pin == hex.EncodeToString(pin[:]) {
			if c.options.Network == nil {
				return nil, terminalError("UNSUPPORTED_CAPABILITY")
			}
			target := network.Target{Device: peer, Pin: pin, Purpose: network.PeerData, Profile: history.Digest(terminalID(route.Profile))}
			if policy.Mode == "local_only" {
				err = c.options.Network.RegisterDirect(target, nil)
			} else if c.options.Relay != nil {
				err = c.options.Relay.Register(target)
			} else {
				err = c.options.Network.RegisterDirect(target, nil)
			}
			if err != nil {
				return nil, err
			}
			return replication.NewRoutedClient(ctx, network.LogicalOrigin(target), id, cert, target, c.options.Network)
		}
	}

	if policy.Mode == "local_only" && c.options.Network != nil && !network.LocalEndpoint(endpoint) {
		target := network.Target{Device: peer, Pin: pin, Purpose: network.PeerData}
		if err = c.options.Network.RegisterDirect(target, nil); err != nil {
			return nil, err
		}
		return replication.NewRoutedClient(ctx, network.LogicalOrigin(target), id, cert, target, c.options.Network)
	}
	if c.options.Network == nil {
		if policy.Mode == "local_only" {
			return nil, terminalError("LOCAL_ONLY_ROUTE_UNAVAILABLE")
		}
		return replication.NewClient(endpoint, id, cert, pin)
	}
	target := network.Target{Device: peer, Pin: pin, Purpose: network.PeerData}
	if err := c.options.Network.SetManual(target, endpoint); err != nil {
		return nil, err
	}
	return replication.NewRoutedClient(ctx, endpoint, id, cert, target, c.options.Network)
}

func (c *Controller) persistLogicalRoute(folder string, route protocol.EnrollmentRoute, certificate string) error {
	if route.Validate() != nil {
		return terminalError("INVALID_REQUEST")
	}
	if c.options.Relay == nil {
		return terminalError("UNSUPPORTED_CAPABILITY")
	}
	if err := config.SetPeerRoute(c.db.StateDir(), config.PeerRoute{Folder: folder, Device: route.Device, Pin: route.Pin, Profile: route.Profile, CertificateDER: certificate}); err != nil {
		return err
	}
	target := network.Target{Device: terminalID(route.Device), Pin: history.Digest(terminalID(route.Pin)), Profile: history.Digest(terminalID(route.Profile)), Purpose: network.PeerData}
	return c.options.Relay.Register(target)
}

// Approval commits its signed artifact and route reference together in SQLite.
// Repair the secondary route file after a crash between that commit and file
// installation; replaying owner approval is not required to restore reverse pull.
func (c *Controller) recoverApprovedRoutes(ctx context.Context) error {
	if c.options.Relay == nil {
		return nil
	}
	after := ""
	for {
		var records []replication.EnrollmentRecord
		err := c.db.EnrollmentTransaction(ctx, func(tx *repository.EnrollmentTx) error {
			page, e := tx.EnrollmentRequestPage("", after, 200)
			if e != nil {
				return e
			}
			keys := make([]string, 0, len(page))
			for key := range page {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				var record replication.EnrollmentRecord
				if e = protocol.DecodeStrict(page[key], &record); e != nil {
					return e
				}
				records = append(records, record)
				after = strings.TrimPrefix(key, "request/")
			}
			return nil
		})
		if err != nil {
			return err
		}
		for _, record := range records {
			if record.Routed != nil && record.Result.State == "approved" {
				if err = c.persistLogicalRoute(record.Wire.Folder, record.Routed.Transcript.Requester, record.Routed.CertificateDER); err != nil {
					return err
				}
			}
		}
		if len(records) < 200 {
			return nil
		}
	}
}

// profileMismatch explains why a routed invitation names a different service
// profile than the one this device reviewed. Pairing needs both devices on the
// same profile; the precise cause tells the owner which device to change.
func profileMismatch(local network.ProfileSelection, inv tc.Invitation) error {
	digest, err := local.Digest()
	if err != nil || inv.Route == nil || digest == inv.Route.Profile {
		return err
	}
	if inv.Profile == nil || inv.Profile.Authority != local.Authority {
		operator := "another operator"
		if inv.Profile != nil {
			operator = fmt.Sprintf("%q", inv.Profile.Operator)
		}
		return &ControlError{Code: "PROFILE_OPERATOR_MISMATCH", Message: fmt.Sprintf("the inviting device uses Orbit services from %s; this device uses %q", operator, local.Profile.Operator), Action: "use the same operator on both devices (orbit network set --profile-file on one of them), then create a new invitation"}
	}
	theirs, ours := uint64(inv.Profile.Epoch), uint64(local.Profile.Epoch)
	switch {
	case theirs > ours:
		return &ControlError{Code: "PROFILE_EPOCH_MISMATCH", Message: fmt.Sprintf("the inviting device uses service profile epoch %d; this device has epoch %d", theirs, ours), Action: "update Orbit on this device (or run orbit network update), then resume the join"}
	case theirs < ours:
		return &ControlError{Code: "PROFILE_EPOCH_MISMATCH", Message: fmt.Sprintf("the inviting device uses older service profile epoch %d; this device has epoch %d", theirs, ours), Action: "update Orbit on the inviting device (or run orbit network update there), then create a new invitation"}
	}
	return &ControlError{Code: "PROFILE_MISMATCH", Message: "the inviting device's service profile differs from this device's at the same epoch", Action: "stop and compare both devices' orbit network status; do not bypass verification"}
}
