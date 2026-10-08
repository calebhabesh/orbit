package terminalcontract

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/protocol"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Invitation code prefixes. The legacy envelope uses control version 1 inside
// its v2 code; routed v3 is additive. Neither is relabeled.
const (
	InvitationCodeV2 = "orbit-invitation:v2:"
	InvitationCodeV3 = "orbit-invitation:v3:"
)

// ErrNewerInvitation reports an invitation format from a newer Orbit release.
// ErrIncompleteInvitation reports a pasted code that does not decode to a
// whole invitation: usually cut short by copying part of a wrapped line (F07).
var ErrIncompleteInvitation = errors.New("INVITATION_INCOMPLETE: the invitation code is incomplete or damaged")

var ErrNewerInvitation = errors.New("UNSUPPORTED_INVITATION_VERSION: this invitation was created by a newer Orbit; update Orbit on this device or ask for an invitation from the same version")

// ErrProfileNotPackaged reports a compact code whose service profile this
// build does not carry.
var ErrProfileNotPackaged = errors.New("PROFILE_NOT_PACKAGED: this short invitation code names a service profile that is not packaged in this Orbit version; update Orbit on the older device, or transfer the invitation file instead")

// InvitationCode encodes an invitation for pasting on one line. A routed
// invitation whose profile is this build's packaged release profile omits the
// profile; the receiver restores it from its own packaged copy and the route
// digest still binds the exact bytes. Invitation files keep the full form.
func InvitationCode(inv Invitation) (string, error) {
	prefix := InvitationCodeV2
	if inv.Version == "3" {
		prefix = InvitationCodeV3
		if inv.Route != nil {
			if packaged, ok := network.BuiltinProfile(); ok {
				if d, err := packaged.Digest(); err == nil && d == inv.Route.Profile {
					inv.Profile = nil
				}
			}
		}
	}
	b, err := json.Marshal(inv)
	if err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// DecodeInvitationCode returns the JSON inside a pasted code. ok is false when
// the text is not an invitation code (for example raw JSON or a file path).
func DecodeInvitationCode(text string) (b []byte, ok bool, err error) {
	code := strings.TrimSpace(text)
	for _, prefix := range []string{InvitationCodeV2, InvitationCodeV3} {
		if strings.HasPrefix(code, prefix) {
			b, err = base64.RawURLEncoding.DecodeString(strings.TrimPrefix(code, prefix))
			if err != nil {
				err = ErrIncompleteInvitation
			}
			return b, true, err
		}
	}
	if rest, found := strings.CutPrefix(code, "orbit-invitation:v"); found {
		if n, e := strconv.Atoi(strings.SplitN(rest, ":", 2)[0]); e == nil && n > 3 {
			return nil, true, ErrNewerInvitation
		}
	}
	return nil, false, nil
}

// ExpandPackaged restores the profile omitted from a compact routed code.
func (i *Invitation) ExpandPackaged() error {
	if i.Version != "3" || i.Profile != nil || i.Route == nil {
		return nil
	}
	packaged, ok := network.BuiltinProfile()
	if !ok {
		return ErrProfileNotPackaged
	}
	if d, err := packaged.Digest(); err != nil || d != i.Route.Profile {
		return ErrProfileNotPackaged
	}
	p := packaged.Profile
	i.Profile = &p
	return nil
}

func validEndpoint(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Path == "" || u.Path == "/")
}

// Validate checks a transferred invitation's structure and certificate pin.
// Trust still requires deliberate transfer; expiry and current membership are
// rechecked by the inviter's durable admission transaction, not this codec.
func (i Invitation) Validate() error {
	if i.Version == "3" {
		data, err := json.Marshal(i)
		if err != nil || len(data) > protocol.RoutedInvitationMaxBytes {
			return fmt.Errorf("PAYLOAD_TOO_LARGE")
		}
		expires, err := time.Parse(time.RFC3339Nano, i.ExpiresAt)
		if err != nil || i.Route == nil || i.Profile == nil || i.EnrollmentEndpoint != "" || i.PeerEndpoint != "" || i.Inviter != i.Route.Device || i.KeyPin != i.Route.Pin {
			return fmt.Errorf("INVALID_REQUEST: routed invitation")
		}
		// Codec checks shape, not lifetime; a persisted accepted attempt must be
		// inspectable after the original capability expires.
		routed := i.RoutedInvitation()
		return routed.Validate(uint64(expires.Unix()-1), true)
	}
	if n, err := strconv.Atoi(i.Version); err == nil && n > 3 {
		return ErrNewerInvitation
	}
	if i.Route != nil || i.Profile != nil {
		return fmt.Errorf("UNSUPPORTED_CAPABILITY")
	}
	if i.Version != Version || !validID(i.Folder) || !validID(i.Inviter) || !validID(i.KeyPin) || !validID(i.Capability) || !validEndpoint(i.EnrollmentEndpoint) || !validEndpoint(i.PeerEndpoint) {
		return fmt.Errorf("INVALID_REQUEST: invitation")
	}
	if _, err := time.Parse(time.RFC3339Nano, i.ExpiresAt); err != nil {
		return err
	}
	b, err := base64.StdEncoding.DecodeString(i.CertificateDER)
	if err != nil {
		return err
	}
	cert, err := x509.ParseCertificate(b)
	if err != nil {
		return err
	}
	pin := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	if hex.EncodeToString(pin[:]) != i.KeyPin {
		return fmt.Errorf("IDENTITY_MISMATCH")
	}
	return nil
}

func (i Invitation) RoutedInvitation() protocol.RoutedInvitation {
	expires, _ := time.Parse(time.RFC3339Nano, i.ExpiresAt)
	r := protocol.RoutedInvitation{Version: "3", Folder: i.Folder, CertificateDER: i.CertificateDER, Capability: i.Capability, Expires: protocol.NetworkUint(expires.Unix())}
	if i.Route != nil {
		r.InviterRoute = *i.Route
	}
	if i.Profile != nil {
		r.Profile = *i.Profile
	}
	return r
}

// JoinPolicy proposes the joining device's connection policy (F08). A routed
// (v3) invitation selects the inviter's operator: Automatic when it is this
// build's packaged profile, self-hosted otherwise; that profile travels in the
// invitation and is reviewed with the join. Any other invitation keeps the
// current policy, except that a device with no folders yet (fresh) is offered
// Automatic when a current packaged profile exists. The result names the next
// generation whenever it differs from current.
func JoinPolicy(inv Invitation, current NetworkPolicy, builtin *BuiltinProfile, fresh bool) NetworkPolicy {
	p := current
	switch {
	case inv.Route != nil:
		p.Mode = "self_hosted"
		if builtin != nil && !builtin.Expired && builtin.Digest == inv.Route.Profile {
			p.Mode = "automatic"
		}
		p.Profile, p.LANAdvertising, p.AwaitingProfile = inv.Route.Profile, true, false
	case fresh && current.Mode == "manual" && builtin != nil && !builtin.Expired:
		p.Mode, p.Profile, p.LANAdvertising, p.AwaitingProfile = "automatic", builtin.Digest, true, false
	}
	if p != current {
		p.Generation = current.Generation + 1
	}
	return p
}

// DecodeInvitationText decodes a pasted code or invitation JSON and checks its
// shape. Every failure of a pasted code that is not expiry, a newer version
// or an unpackaged profile is reported as incomplete or damaged (F07), never
// as a generic INVALID_REQUEST.
func DecodeInvitationText(b []byte) (Invitation, bool, error) {
	var inv Invitation
	decoded, code, err := DecodeInvitationCode(string(b))
	if err != nil {
		return inv, code, err
	}
	if code {
		b = decoded
	}
	if err = Decode(b, &inv); err != nil {
		if code {
			return inv, code, ErrIncompleteInvitation
		}
		return inv, code, err
	}
	return inv, code, inv.ExpandPackaged()
}

// ShapeError maps a structural Validate failure of a decoded invitation to
// ErrIncompleteInvitation; identity, version and other classes pass through.
func ShapeError(err error) error {
	if err != nil && strings.HasPrefix(err.Error(), "INVALID_REQUEST") {
		return ErrIncompleteInvitation
	}
	return err
}
