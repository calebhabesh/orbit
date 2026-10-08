package terminalcontract

import (
	"errors"
	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/protocol"
)

// W06 supplies policy/status controls; W12/W14 retain expanded diagnostics and
// migration ownership. Capabilities describe implemented controls, not reachability.
const NetworkDiagnosticsCapability = "network_diagnostics_v1"
const NetworkCapability = "network_control_v1"
const RoutedEnrollmentCapability = "enrollment_v3"
const ShortPairingCapability = "short_pairing_v1"

// PackagedProfileCapability: the daemon reports its built-in release profile,
// accepts its digest in reviewed setup/network intents, reviews operator
// replacement and decodes compact invitation codes (W14).
const PackagedProfileCapability = "packaged_profile_v1"

// MaxServiceRoots bounds reviewed self-hosted PEM trust in a network intent.
const MaxServiceRoots = 16 << 10

type NetworkPolicy struct {
	Timing          protocol.RouteTiming `json:"timing,omitempty,omitzero"`
	AwaitingProfile bool                 `json:"awaiting_profile,omitempty"`
	Mode            string               `json:"mode"`    // automatic, local_only, manual, self_hosted
	Profile         string               `json:"profile"` // reviewed digest; empty for manual/local_only
	LANAdvertising  bool                 `json:"lan_advertising"`
	Generation      Uint                 `json:"generation"`
}

func (p NetworkPolicy) Validate() error {
	if err := p.Timing.Validate(); err != nil {
		return err
	}
	switch p.Mode {
	case "automatic", "self_hosted":
		if p.Profile == "" && p.Mode == "automatic" && p.AwaitingProfile {
			return nil
		}
		if p.AwaitingProfile {
			return errors.New("INVALID_REQUEST")
		}
		return protocol.NetworkHex(p.Profile, 32)
	case "local_only", "manual":
		if p.Mode == "manual" && p.LANAdvertising {
			return errors.New("UNSUPPORTED_CAPABILITY")
		}
		if p.Profile != "" || p.AwaitingProfile {
			return errors.New("INVALID_REQUEST")
		}
		return nil
	default:
		return errors.New("INVALID_REQUEST")
	}
}

type NetworkQuery struct {
	Device  string `json:"device"` // empty = bounded page of known peers
	Purpose string `json:"purpose"`
	Cursor  string `json:"cursor"`
	Limit   Uint   `json:"limit"`
}
type NetworkObservation struct {
	Freshness         string `json:"freshness"`
	Action            string `json:"action"`
	LANCandidates     Uint   `json:"lan_candidates"`
	PublicCandidates  Uint   `json:"public_candidates"`
	ExpiredCandidates Uint   `json:"expired_candidates"`
	UDPCode           string `json:"udp_code"`
	Device            string `json:"device"`
	Pin               string `json:"pin"`
	Purpose           string `json:"purpose"`
	Route             string `json:"route"` // direct, quic, relay, finding, unavailable, not_tested
	Code              string `json:"code"`
	ObservedAt        string `json:"observed_at"`
	Generation        Uint   `json:"generation"`
}
type NetworkPreview struct {
	Policy  NetworkPolicy `json:"policy"`
	Review  Review        `json:"review"`
	Effects []Effect      `json:"effects"`
}
type NetworkApply struct {
	Operation string        `json:"operation"`
	Policy    NetworkPolicy `json:"policy"`
	Review    Review        `json:"review"`
}

// PrivateNetworkSetup is stored ONLY under existing exclusive private state
// ownership. It extends reviewed setup intent; transient candidates, tokens,
// leases/ICE passwords and observations are regenerated rather than persisted.
type PrivateNetworkSetup struct {
	Version    string                              `json:"version"`
	Operation  string                              `json:"operation"`
	Attempt    string                              `json:"attempt"`
	Root       string                              `json:"root"`
	Phase      string                              `json:"phase"`
	Policy     NetworkPolicy                       `json:"policy"`
	Invitation protocol.RoutedInvitation           `json:"invitation"` // secret, private only
	Transcript protocol.RoutedEnrollmentTranscript `json:"transcript"`
	Request    string                              `json:"request"`
}

func (q NetworkQuery) Validate() error {
	if q.Device != "" && !validID(q.Device) {
		return errors.New("INVALID_REQUEST")
	}
	if q.Purpose != "peer_data" && q.Purpose != "enrollment" {
		return errors.New("PURPOSE_MISMATCH")
	}
	if q.Limit == 0 || q.Limit > MaxPage || len(q.Cursor) > 256 {
		return errors.New("INVALID_REQUEST")
	}
	return nil
}
func (a NetworkApply) Validate() error {
	if !validID(a.Operation) || !validID(a.Review.Token) || a.Review.Generation == "" || a.Review.ExpiresAt == "" {
		return errors.New("INVALID_REQUEST")
	}
	return a.Policy.Validate()
}

// NetworkIntent uses the normal durable mutation ledger. The authority is an
// independently reviewed trust input, never inferred from the profile itself.
type NetworkIntent struct {
	Policy      NetworkPolicy            `json:"policy"`
	Review      Review                   `json:"review"`
	Profile     *protocol.NetworkProfile `json:"profile,omitempty"`
	Authority   string                   `json:"authority"`
	Environment string                   `json:"environment"`
	// ServiceRoots is reviewed PEM trust for a self-hosted/development service.
	// A profile review replaces it; release profiles use system roots only.
	ServiceRoots string `json:"service_roots,omitempty"`
	// ReplaceOperator confirms a reviewed switch to a different authority or
	// environment. Per-authority rollback floors still apply.
	ReplaceOperator bool `json:"replace_operator,omitempty"`
	// DeclineAutomaticOffer records that a manual install's owner reviewed and
	// declined the one-time offer to use Automatic with the packaged profile.
	DeclineAutomaticOffer bool `json:"decline_automatic_offer,omitempty"`
}

// BuiltinProfile describes the packaged release profile. It is public signed
// data; selecting it still requires a reviewed setup or network intent.
type BuiltinProfile struct {
	Digest   string `json:"digest"`
	Operator string `json:"operator"`
	Privacy  string `json:"privacy"`
	Epoch    Uint   `json:"epoch"`
	Expires  string `json:"expires"`
	Expired  bool   `json:"expired"`
}
type NetworkStatus struct {
	GeneratedAt     string                `json:"generated_at"`
	ProfileExpires  string                `json:"profile_expires"`
	ProfileState    string                `json:"profile_state"`
	ServiceTrust    string                `json:"service_trust,omitempty"` // system or custom:<sha256>
	Action          string                `json:"action"`
	Probes          []network.ProbeResult `json:"probes"`
	Policy          NetworkPolicy         `json:"policy"`
	ActivePolicy    NetworkPolicy         `json:"active_policy"`
	RestartRequired bool                  `json:"restart_required"`
	Ready           bool                  `json:"ready"`
	Code            string                `json:"code"`
	Operator        string                `json:"operator"`
	Privacy         string                `json:"privacy"`
	Observations    []NetworkObservation  `json:"observations"`
	// W14 packaged defaults. ProfileUpdate is "available" when the packaged
	// profile is a newer epoch of the selected authority that needs review
	// (changed operator/privacy text); same-text updates apply at startup.
	// AutomaticOffer marks a manual install that has not yet reviewed
	// Automatic with the packaged profile.
	Builtin        *BuiltinProfile `json:"builtin,omitempty"`
	ProfileUpdate  string          `json:"profile_update,omitempty"`
	AutomaticOffer bool            `json:"automatic_offer,omitempty"`
}
