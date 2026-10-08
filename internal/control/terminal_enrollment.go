package control

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/calebhabesh/orbit/internal/config"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/replication"
	"github.com/calebhabesh/orbit/internal/repository"
)

func terminalID(s string) history.ID {
	var id history.ID
	b, _ := hex.DecodeString(s)
	copy(id[:], b)
	return id
}

// Invitation capabilities are a domain-separated HMAC PRF of a random operation
// identity and fingerprint under the persistent private key. Replay can reproduce
// explicit transfer without storing raw capabilities in the inviter's database.
func invitationCapability(id replication.Identity, m tc.Mutation, fingerprint string) (string, error) {
	key, ok := id.Certificate.PrivateKey.(ed25519.PrivateKey)
	if !ok {
		return "", terminalError("IDENTITY_MISMATCH")
	}
	mac := hmac.New(sha256.New, key.Seed())
	mac.Write([]byte("orbit-invitation-capability-v2\x00"))
	mac.Write([]byte(m.OperationID))
	mac.Write([]byte(fingerprint))
	return hex.EncodeToString(mac.Sum(nil)), nil
}
func (c *Controller) terminalEnrollmentMutation(ctx context.Context, m tc.Mutation) (tc.Result, error) {
	fingerprint, err := m.Fingerprint()
	if err != nil {
		return tc.Result{}, err
	}
	c.terminalMu.Lock()
	defer c.terminalMu.Unlock()
	id, err := replication.LoadOrCreateIdentity(c.db.StateDir(), c.options.LocalDevice, c.options.Now())
	if err != nil {
		return tc.Result{}, err
	}
	r := terminalResult()
	r.Operation = &tc.Operation{ID: m.OperationID, Fingerprint: fingerprint, Kind: m.Kind, State: "completed", Phase: "completed", CommittedEffects: []tc.Effect{}}
	var capability string
	if m.Kind == "invite" || m.Kind == "share" {
		capability, err = invitationCapability(id, m, fingerprint)
		if err != nil {
			return r, err
		}
	}
	// Accepted local mutations finish independently of a disconnected waiter.
	owned, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	err = c.db.EnrollmentTransaction(owned, func(t *repository.EnrollmentTx) error {
		var old repository.TerminalRecord
		if err := t.Get("operation/"+m.OperationID, &old); err == nil {
			var e error
			r, e = c.replayTerminal(old, fingerprint)
			return e
		} else if !errors.Is(err, repository.ErrOperationNotFound) {
			return err
		}
		now := c.options.Now()
		switch m.Kind {
		case "invite", "share":
			p := m.Invite
			var targetPin string
			if m.Kind == "share" {
				if p.Device == "" {
					return terminalError("INVALID_REQUEST")
				}
				pin, err := t.DeviceKeyPin(terminalID(p.Device))
				if err != nil {
					return terminalError("UNAUTHORIZED")
				}
				targetPin = hex.EncodeToString(pin[:])
			} else if p.Device != "" {
				return terminalError("INVALID_REQUEST")
			}
			settings, err := config.LoadRuntimeSettings(c.db.StateDir())
			if err != nil {
				return err
			}
			policy, err := config.LoadNetworkPolicy(c.db.StateDir())
			if err != nil {
				return err
			}
			routed := policy.Mode == "automatic" || policy.Mode == "self_hosted"
			if !routed && (settings.AdvertisedEnrollment == "" || settings.AdvertisedPeer == "" || settings.EnrollmentListen == "" || settings.PeerListen == "") {
				return terminalError("NETWORK_REVIEW_REQUIRED")
			}
			membership, digest, err := t.Membership(terminalID(p.Folder))
			if err != nil {
				return err
			}
			if hex.EncodeToString(digest[:]) != p.ExpectedMembership {
				return terminalError("STALE_VIEW")
			}
			active := false
			for _, a := range membership.Active {
				if a.Device == id.DeviceID && a.KeyPin == id.KeyPin {
					active = true
				}
			}
			if !active {
				return terminalError("UNAUTHORIZED")
			}
			expires, err := time.Parse(time.RFC3339Nano, p.ExpiresAt)
			if err != nil || !now.Before(expires) || expires.After(now.Add(24*time.Hour)) {
				return terminalError("INVALID_REQUEST")
			}
			inv := tc.Invitation{Version: tc.Version, Folder: p.Folder, Inviter: hex.EncodeToString(id.DeviceID[:]), CertificateDER: base64.StdEncoding.EncodeToString(id.Leaf.Raw), KeyPin: hex.EncodeToString(id.KeyPin[:]), EnrollmentEndpoint: "https://" + settings.AdvertisedEnrollment, PeerEndpoint: "https://" + settings.AdvertisedPeer, ExpiresAt: p.ExpiresAt}
			if routed {
				if c.options.Relay == nil {
					return terminalError("SERVICE_UNAVAILABLE")
				}
				selection, e := config.LoadNetworkProfile(c.db.StateDir(), uint64(now.Unix()))
				if e != nil {
					return e
				}
				profile, e := selection.Digest()
				if e != nil {
					return e
				}
				if policy.Profile != profile {
					return terminalError("PROFILE_MISMATCH")
				}
				inv.Version = "3"
				inv.EnrollmentEndpoint = ""
				inv.PeerEndpoint = ""
				inv.Route = &protocol.EnrollmentRoute{Device: inv.Inviter, Pin: inv.KeyPin, Profile: profile, Purpose: "enrollment"}
				inv.Profile = &selection.Profile
			}
			b, _ := hex.DecodeString(capability)
			d := sha256.Sum256(b)
			if err := t.Put("invite/"+hex.EncodeToString(d[:]), replication.EnrollmentInvitation{Invitation: inv, Digest: hex.EncodeToString(d[:]), TargetDevice: p.Device, TargetPin: targetPin}); err != nil {
				return err
			}
			r.Invitation = &inv
			r.Operation.CommittedEffects = append(r.Operation.CommittedEffects, tc.Effect{State: "invitation_created"})
		case "approval":
			p := m.Approval
			var record replication.EnrollmentRecord
			if err := t.Get("request/"+p.Request, &record); err != nil {
				return err
			}
			w := record.Wire
			if w.Folder != p.Folder || w.Requester != p.Requester || w.RequesterPin != p.KeyPin || record.Digest != p.TranscriptDigest || w.PriorMembership != p.ExpectedMembership {
				return terminalError("STALE_VIEW")
			}
			if record.Result.State != "pending_approval" || now.Unix() >= record.Expires {
				return terminalError("EXPIRED_REPLAY")
			}
			var inv replication.EnrollmentInvitation
			if err := t.Get("invite/"+record.TokenDigest, &inv); err != nil {
				return err
			}
			if inv.Revoked {
				return terminalError("INVITATION_INVALID")
			}
			membership, digest, err := t.Membership(terminalID(p.Folder))
			if err != nil {
				return err
			}
			if hex.EncodeToString(digest[:]) != p.ExpectedMembership {
				return terminalError("STALE_VIEW")
			}
			active := false
			for _, a := range membership.Active {
				if a.Device == id.DeviceID && a.KeyPin == id.KeyPin {
					active = true
				}
				if a.Device == terminalID(p.Requester) {
					return terminalError("ALREADY_MEMBER")
				}
			}
			if !active {
				return terminalError("UNAUTHORIZED")
			}
			if p.Decision == "approve" {
				for _, retired := range membership.Retired {
					if retired.Device == terminalID(p.Requester) {
						return RetiredMemberRevivalError("")
					}
				}
				membership.PriorDigest = digest
				membership.Revision++
				membership.Active = append(membership.Active, protocol.ActiveMember{Device: terminalID(p.Requester), KeyPin: history.Digest(terminalID(p.KeyPin))})
				if err := t.Approve(membership); err != nil {
					return err
				}
				bytes, err := protocol.EncodeMembership(membership)
				if err != nil {
					return err
				}
				record.Result.MembershipHex = hex.EncodeToString(bytes)
				record.Result.State = "approved"
				if record.Routed != nil {
					d, e := protocol.MembershipDigest(membership)
					if e != nil {
						return e
					}
					approval := protocol.RoutedEnrollmentApproval{Version: "3", Request: p.Request, TranscriptDigest: record.Digest, PriorMembership: p.ExpectedMembership, MembershipDigest: hex.EncodeToString(d[:])}
					bytes, e := approval.Canonical()
					if e != nil {
						return e
					}
					record.Approval = &approval
					record.ApprovalSignature = hex.EncodeToString(ed25519.Sign(id.Certificate.PrivateKey.(ed25519.PrivateKey), bytes))
				}

			} else {
				record.Result.State = "declined"
			}
			if err := t.Put("request/"+p.Request, record); err != nil {
				return err
			}
			r.Requests = append(r.Requests, enrollmentObservation(record))
			r.Operation.CommittedEffects = append(r.Operation.CommittedEffects, tc.Effect{State: "enrollment_" + record.Result.State})
		default:
			return terminalError("UNSUPPORTED_CAPABILITY")
		}
		saved := repository.TerminalRecord{Mutation: m, Result: r, Owner: hex.EncodeToString(id.DeviceID[:]), CreatedAt: now.UTC().Format(time.RFC3339Nano), CompletedAt: now.UTC().Format(time.RFC3339Nano)}
		return t.Put("operation/"+m.OperationID, saved)
	})
	if err != nil {
		return r, err
	}
	if m.Kind == "approval" && m.Approval.Decision == "approve" {
		if err = c.callHook("terminal.enrollment.approved"); err != nil {
			return r, err
		}
		record, found, err := c.enrollmentV2Record(owned, m.Approval.Request)
		if err != nil {
			return r, err
		}
		if found && record.Routed != nil {
			if err = c.persistLogicalRoute(record.Wire.Folder, record.Routed.Transcript.Requester, record.Routed.CertificateDER); err != nil {
				return r, err
			}
		}
		if found && record.Wire.RequesterEndpoint != "" {
			if err = c.persistRequesterEndpoint(record); err != nil {
				return r, err
			}
		}
		if label := printableLabel(record.Wire.Label); found && label != "" {
			// List the joiner by the name it chose (F11), unless the owner
			// already named that device here.
			device := history.ID(terminalID(record.Wire.Requester))
			if name, e := c.db.GetDeviceDisplayName(owned, device); e == nil && name == "" {
				if err = c.db.SetDeviceDisplayName(owned, device, label); err != nil {
					return r, err
				}
			}
		}
	}
	if r.Invitation != nil {
		copy := *r.Invitation
		copy.Capability = capability
		r.Invitation = &copy
		if m.Invite != nil && m.Invite.ShortCode && copy.Version == "3" {
			if client := c.options.NetworkService; client != nil {
				text, e := tc.InvitationCode(copy)
				if e != nil {
					return r, e
				}
				expires, _ := time.Parse(time.RFC3339Nano, copy.ExpiresAt)
				s, e := client.StartPairing(ctx, m.OperationID, []byte(text), expires)
				if e == nil {
					r.Pairing = &s
				} else {
					r.Pairing = &network.PairingStatus{State: "fallback", Error: "Use the long invitation or file; the service could not create a short code."}
				}
			}
		}
	}
	return r, nil
}
func enrollmentObservation(record replication.EnrollmentRecord) tc.EnrollmentRequest {
	w := record.Wire
	out := record.Result
	return tc.EnrollmentRequest{ID: out.Request, Folder: w.Folder, Requester: w.Requester, KeyPin: w.RequesterPin, Label: w.Label, TranscriptDigest: record.Digest, VerificationCode: out.VerificationCode, ExpectedMembership: w.PriorMembership, State: out.State}
}
func (c *Controller) terminalRequests(ctx context.Context, q tc.Query) (tc.Result, error) {
	r := terminalResult()
	err := c.db.EnrollmentTransaction(ctx, func(t *repository.EnrollmentTx) error {
		limit := int(q.Limit)
		if limit == 0 {
			limit = tc.MaxPage
		}
		rs, err := t.EnrollmentRequestPage(q.Folder, q.Cursor, limit+1)
		if err != nil {
			return err
		}
		keys := make([]string, 0, len(rs))
		for k := range rs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if q.Cursor != "" && key <= "request/"+q.Cursor {
				continue
			}
			b := rs[key]
			var record replication.EnrollmentRecord
			if err := protocol.DecodeStrict(b, &record); err != nil {
				return err
			}
			if q.Folder != "" && q.Folder != record.Wire.Folder {
				continue
			}
			if record.Result.State == "pending_approval" && c.options.Now().Unix() >= record.Expires {
				record.Result.State = "expired"
			}
			if len(r.Requests) == limit {
				r.Cursor = r.Requests[len(r.Requests)-1].ID
				break
			}
			r.Requests = append(r.Requests, enrollmentObservation(record))
		}
		return nil
	})
	return r, err
}

// printableLabel keeps a remote device's chosen name displayable: the label
// arrives from the joining device, so control and other non-printable runes
// are dropped before it is shown in listings.
func printableLabel(label string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, label))
}
