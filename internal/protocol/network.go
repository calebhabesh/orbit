package protocol

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

const NetworkMaxBytes = 64 << 10
const NetworkMaxCandidates = 16
const NetworkMaxOfferCandidates = 8
const NetworkMaxCertificateBytes = 4096
const NetworkMaxPrivacyBytes = 4096
const NetworkMaxOrigins = 4
const NetworkMaxSTUN = 4

// NetworkUint preserves every uint64 exactly across JSON implementations.
type NetworkUint uint64

func (n NetworkUint) MarshalJSON() ([]byte, error) {
	return json.Marshal(strconv.FormatUint(uint64(n), 10))
}
func (n *NetworkUint) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := parseUint(s, true)
	if err != nil {
		return err
	}
	*n = NetworkUint(v)
	return nil
}
func NetworkHex(s string, size int) error {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != size || hex.EncodeToString(b) != s || bytes.Equal(b, make([]byte, size)) {
		return errors.New("INVALID_ENCODING")
	}
	return nil
}

// NetworkDecode rejects unknown/duplicate keys using the shared strict codec.
// Validation of required fields and semantic bounds follows decoding.
func NetworkDecode(data []byte, out any) error {
	if len(data) > NetworkMaxBytes {
		return errors.New("PAYLOAD_TOO_LARGE")
	}
	if err := DecodeStrict(data, out); err != nil {
		return err
	}
	var shape any
	if err := json.Unmarshal(data, &shape); err != nil {
		return err
	}
	return networkShape(shape, reflect.TypeOf(out).Elem())
}

// Encoding/json matches struct fields without regard to case and accepts null.
// Network messages require every exact wire key and reject null/alias spellings.
func networkShape(v any, t reflect.Type) error {
	if v == nil {
		return errors.New("INVALID_ENCODING")
	}
	if t.Kind() == reflect.Pointer {
		return networkShape(v, t.Elem())
	}
	switch t.Kind() {
	case reflect.Struct:
		m, ok := v.(map[string]any)
		if !ok {
			return errors.New("INVALID_ENCODING")
		}
		if len(m) > t.NumField() {
			return errors.New("INVALID_ENCODING")
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			x, ok := m[name]
			if !ok {
				// Only the explicitly versioned ICE extension is optional.
				if t == reflect.TypeOf(NetworkOffer{}) && name == "ice" && len(m) == t.NumField()-1 {
					continue
				}
				return errors.New("INVALID_ENCODING")
			}
			if e := networkShape(x, f.Type); e != nil {
				return e
			}
		}
	case reflect.Slice:
		a, ok := v.([]any)
		if !ok {
			return errors.New("INVALID_ENCODING")
		}
		for _, x := range a {
			if e := networkShape(x, t.Elem()); e != nil {
				return e
			}
		}
	}
	return nil
}
func NetworkCanonical(kind string, fields ...string) []byte {
	var b bytes.Buffer
	for _, s := range append([]string{"orbit-network-v1", kind}, fields...) {
		_ = binary.Write(&b, binary.BigEndian, uint32(len(s)))
		b.WriteString(s)
	}
	return b.Bytes()
}
func networkText(s string, max int) bool {
	return utf8.ValidString(s) && len(s) <= max && !strings.ContainsAny(s, "\x00\r\n")
}

// ServiceOrigin is a canonical TLS origin, never a path or owner-control URL.
// DNS answers must ALSO be checked at dial time (W03); syntax is not SSRF proof.
func ServiceOrigin(s string, private bool) error {
	u, err := url.Parse(s)
	if err != nil || len(s) > 256 || (u.Scheme != "https" && u.Scheme != "wss") || u.Host == "" || u.User != nil || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || strings.ToLower(s) != s || strings.Contains(s, "%") {
		return errors.New("INVALID_ORIGIN")
	}
	h := u.Hostname()
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".") {
		return errors.New("INVALID_ORIGIN")
	}
	if u.Port() != "" {
		p, e := strconv.ParseUint(u.Port(), 10, 16)
		if e != nil || p == 0 || strconv.FormatUint(p, 10) != u.Port() || p == 443 {
			return errors.New("INVALID_ORIGIN")
		}
	}
	if ip, e := netip.ParseAddr(h); e == nil {
		if !AllowedAddress(ip, private) {
			return errors.New("INVALID_ORIGIN")
		}
	} else {
		if len(h) > 253 || !strings.Contains(h, ".") {
			return errors.New("INVALID_ORIGIN")
		}
		for _, label := range strings.Split(h, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return errors.New("INVALID_ORIGIN")
			}
			for _, c := range label {
				if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
					return errors.New("INVALID_ORIGIN")
				}
			}
		}
	}
	return nil
}

var excludedPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2001::/32"), netip.MustParsePrefix("2002::/16"),
}

func AllowedAddress(ip netip.Addr, private bool) bool {
	if !ip.IsValid() || ip.Is4In6() || ip.Zone() != "" || !ip.IsGlobalUnicast() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if ip.IsPrivate() {
		return private
	}
	for _, p := range excludedPublic {
		if p.Contains(ip) {
			return false
		}
	}
	// Public IPv6 candidates must be in the currently allocated global range.
	return ip.Is4() || netip.MustParsePrefix("2000::/3").Contains(ip)
}

type NetworkCandidate struct {
	Transport string `json:"transport"` // tcp or udp
	Address   string `json:"address"`   // canonical numeric AddrPort; no DNS
	Scope     string `json:"scope"`     // public or lan
	Interface string `json:"interface"` // required for lan, empty for public
}

func (c NetworkCandidate) Validate(publication bool) error {
	a, e := netip.ParseAddrPort(c.Address)
	if e != nil || a.String() != c.Address || a.Port() == 0 || (c.Transport != "tcp" && c.Transport != "udp") {
		return errors.New("INVALID_CANDIDATE")
	}
	switch c.Scope {
	case "public":
		if c.Interface != "" || !AllowedAddress(a.Addr(), false) {
			return errors.New("INVALID_CANDIDATE")
		}
	case "lan":
		if publication || !networkText(c.Interface, 64) || c.Interface == "" || !a.Addr().IsPrivate() || !AllowedAddress(a.Addr(), true) {
			return errors.New("INVALID_CANDIDATE")
		}
	default:
		return errors.New("INVALID_CANDIDATE")
	}
	return nil
}

type NetworkProfile struct {
	ServiceKey string      `json:"service_key"` // online relay credential signer, profile-authenticated
	Version    string      `json:"version"`
	Operator   string      `json:"operator"`
	Authority  string      `json:"authority"` // raw Ed25519 public key, hex
	Epoch      NetworkUint `json:"epoch"`
	Expires    NetworkUint `json:"expires"`
	Origins    []string    `json:"origins"`
	STUN       []string    `json:"stun"` // numeric AddrPort, no URI or credentials
	Privacy    string      `json:"privacy"`
	Signature  string      `json:"signature"`
}

func (p NetworkProfile) Canonical(private bool) ([]byte, error) {
	if p.Version != "1" || !networkText(p.Operator, 128) || p.Operator == "" || p.Epoch == 0 || p.Expires == 0 || !networkText(p.Privacy, NetworkMaxPrivacyBytes) || p.Privacy == "" || len(p.Origins) == 0 || len(p.Origins) > NetworkMaxOrigins || p.STUN == nil || len(p.STUN) > NetworkMaxSTUN {
		return nil, errors.New("INVALID_PROFILE")
	}
	if e := NetworkHex(p.ServiceKey, 32); e != nil {
		return nil, e
	}
	if e := NetworkHex(p.Authority, 32); e != nil {
		return nil, e
	}
	f := []string{p.Version, p.Operator, p.Authority, p.ServiceKey, strconv.FormatUint(uint64(p.Epoch), 10), strconv.FormatUint(uint64(p.Expires), 10), strconv.Itoa(len(p.Origins))}
	seen := map[string]bool{}
	for _, s := range p.Origins {
		if e := ServiceOrigin(s, private); e != nil || seen[s] {
			return nil, errors.New("INVALID_PROFILE")
		}
		seen[s] = true
		f = append(f, s)
	}
	f = append(f, strconv.Itoa(len(p.STUN)))
	for _, s := range p.STUN {
		a, e := netip.ParseAddrPort(s)
		if e != nil || a.Port() == 0 || a.String() != s || !AllowedAddress(a.Addr(), private) || seen[s] {
			return nil, errors.New("INVALID_PROFILE")
		}
		seen[s] = true
		f = append(f, s)
	}
	f = append(f, p.Privacy)
	return NetworkCanonical("profile", f...), nil
}
func (p NetworkProfile) Verify(authority ed25519.PublicKey, now uint64, minEpoch uint64, private bool) error {
	b, e := p.Canonical(private)
	if e != nil {
		return e
	}
	if len(authority) != ed25519.PublicKeySize || hex.EncodeToString(authority) != p.Authority || uint64(p.Expires) <= now || uint64(p.Epoch) < minEpoch {
		return errors.New("PROFILE_UNTRUSTED")
	}
	sig, e := hex.DecodeString(p.Signature)
	if e != nil || NetworkHex(p.Signature, 64) != nil || !ed25519.Verify(authority, b, sig) {
		return errors.New("INVALID_SIGNATURE")
	}
	return nil
}
func (p NetworkProfile) Digest(private bool) (string, error) {
	b, e := p.Canonical(private)
	if e != nil {
		return "", e
	}
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:]), nil
}

// NetworkProof is used on every authenticated service operation. Payload is a
// digest of separately validated canonical bytes; certificate DER never grants
// folder authority. Fields remain required even for no-target operations; their
// target/session/role/generation use the explicit empty/0 sentinels below.
type NetworkProof struct {
	Version        string      `json:"version"`
	Kind           string      `json:"kind"`
	Profile        string      `json:"profile"`
	Origin         string      `json:"origin"`
	Sender         string      `json:"sender"`
	SenderPin      string      `json:"sender_pin"`
	Target         string      `json:"target"`
	TargetPin      string      `json:"target_pin"`
	Purpose        string      `json:"purpose"`
	Challenge      string      `json:"challenge"`
	Operation      string      `json:"operation"`
	Session        string      `json:"session"`
	Generation     NetworkUint `json:"generation"`
	Role           string      `json:"role"`
	Expires        NetworkUint `json:"expires"`
	Payload        string      `json:"payload"`
	CertificateDER string      `json:"certificate_der"`
	Signature      string      `json:"signature"`
}

func (p NetworkProof) Canonical(private bool) ([]byte, error) {
	if p.Version != "1" || p.Expires == 0 || (p.Purpose != "enrollment" && p.Purpose != "peer_data") {
		return nil, errors.New("INVALID_PROOF")
	}
	if e := ServiceOrigin(p.Origin, private); e != nil {
		return nil, e
	}
	for _, s := range []string{p.Profile, p.Sender, p.SenderPin, p.Challenge, p.Operation, p.Payload} {
		if e := NetworkHex(s, 32); e != nil {
			return nil, e
		}
	}
	switch p.Kind {
	case "authenticate", "announce", "pairing":
		if p.Target != "" || p.TargetPin != "" || p.Session != "" || p.Role != "" {
			return nil, errors.New("INVALID_PROOF")
		}
	case "lookup":
		if p.Session != "" || p.Role != "" {
			return nil, errors.New("INVALID_PROOF")
		}
		fallthrough
	case "offer", "accept", "reserve", "attach", "release":
		for _, s := range []string{p.Target, p.TargetPin} {
			if e := NetworkHex(s, 32); e != nil {
				return nil, e
			}
		}
		if p.Kind != "lookup" {
			if NetworkHex(p.Session, 32) != nil || (p.Role != "initiator" && p.Role != "responder") {
				return nil, errors.New("INVALID_PROOF")
			}
		}
	default:
		return nil, errors.New("UNSUPPORTED_CAPABILITY")
	}
	return NetworkCanonical(p.Kind, p.Version, p.Profile, p.Origin, p.Sender, p.SenderPin, p.Target, p.TargetPin, p.Purpose, p.Challenge, p.Operation, p.Session, strconv.FormatUint(uint64(p.Generation), 10), p.Role, strconv.FormatUint(uint64(p.Expires), 10), p.Payload), nil
}

// NetworkCertificate checks DER/SPKI/key encoding with the existing certificate
// representation. DeviceIDs are independent random IDs, never derived from keys.
func NetworkCertificate(encoded, pin string, now uint64) (*x509.Certificate, ed25519.PublicKey, error) {
	if len(encoded) > base64.StdEncoding.EncodedLen(NetworkMaxCertificateBytes) {
		return nil, nil, errors.New("PAYLOAD_TOO_LARGE")
	}
	der, e := base64.StdEncoding.Strict().DecodeString(encoded)
	if e != nil || base64.StdEncoding.EncodeToString(der) != encoded || len(der) > NetworkMaxCertificateBytes {
		return nil, nil, errors.New("INVALID_CERTIFICATE")
	}
	c, e := x509.ParseCertificate(der)
	if e != nil {
		return nil, nil, errors.New("INVALID_CERTIFICATE")
	}
	key, ok := c.PublicKey.(ed25519.PublicKey)
	d := sha256.Sum256(c.RawSubjectPublicKeyInfo)
	if !ok || hex.EncodeToString(d[:]) != pin || c.NotBefore.Unix() > int64(now) || c.NotAfter.Unix() <= int64(now) || now > uint64(^uint64(0)>>1) {
		return nil, nil, errors.New("IDENTITY_MISMATCH")
	}
	return c, key, nil
}
func (p NetworkProof) Verify(private bool, now uint64) error {
	b, e := p.Canonical(private)
	if e != nil {
		return e
	}
	if uint64(p.Expires) <= now || uint64(p.Expires)-now > 60 {
		return errors.New("CHALLENGE_EXPIRED")
	}
	_, key, e := NetworkCertificate(p.CertificateDER, p.SenderPin, now)
	if e != nil {
		return e
	}
	sig, e := hex.DecodeString(p.Signature)
	if e != nil || NetworkHex(p.Signature, 64) != nil || !ed25519.Verify(key, b, sig) {
		return errors.New("INVALID_SIGNATURE")
	}
	return nil
}

type NetworkAnnouncement struct {
	Generation   NetworkUint        `json:"generation"`
	Expires      NetworkUint        `json:"expires"`
	Candidates   []NetworkCandidate `json:"candidates"`
	Capabilities []string           `json:"capabilities"`
	Relay        bool               `json:"relay"`
}

func (a NetworkAnnouncement) Canonical(now uint64, public bool) ([]byte, error) {
	if a.Generation == 0 || uint64(a.Expires) <= now || uint64(a.Expires)-now > 600 || a.Candidates == nil || len(a.Candidates) > NetworkMaxCandidates || a.Capabilities == nil || len(a.Capabilities) > 5 {
		return nil, errors.New("INVALID_ANNOUNCEMENT")
	}
	f := []string{strconv.FormatUint(uint64(a.Generation), 10), strconv.FormatUint(uint64(a.Expires), 10), strconv.Itoa(len(a.Candidates))}
	seen := map[string]bool{}
	for _, c := range a.Candidates {
		if e := c.Validate(public); e != nil {
			return nil, e
		}
		s := c.Transport + "/" + c.Address + "/" + c.Scope + "/" + c.Interface
		if seen[s] {
			return nil, errors.New("INVALID_CANDIDATE")
		}
		seen[s] = true
		f = append(f, c.Transport, c.Address, c.Scope, c.Interface)
	}
	f = append(f, strconv.Itoa(len(a.Capabilities)))
	seen = map[string]bool{}
	for _, s := range a.Capabilities {
		if seen[s] || (s != "direct_https_v1" && s != "relay_inner_tls_v1" && s != "enrollment_v3" && s != "quic_ice_v1" && s != "quic_http3_v1") {
			return nil, errors.New("UNSUPPORTED_CAPABILITY")
		}
		seen[s] = true
		f = append(f, s)
	}
	f = append(f, strconv.FormatBool(a.Relay))
	return NetworkCanonical("announcement", f...), nil
}

// RelayAttachment is a service-signed one-use credential, bound to BOTH pins
// and one service restart epoch. Attachment expiry does not expire an active TLS
// tunnel. The broker cannot dial an arbitrary URL because none is represented.
type RelayAttachment struct {
	Profile    string      `json:"profile"`
	Origin     string      `json:"origin"`
	Epoch      string      `json:"epoch"`
	Session    string      `json:"session"`
	Device     string      `json:"device"`
	Pin        string      `json:"pin"`
	Partner    string      `json:"partner"`
	PartnerPin string      `json:"partner_pin"`
	Purpose    string      `json:"purpose"`
	Role       string      `json:"role"`
	Expires    NetworkUint `json:"expires"`
	Signature  string      `json:"signature"`
}

func (t RelayAttachment) Canonical(private bool) ([]byte, error) {
	for _, s := range []string{t.Profile, t.Epoch, t.Session, t.Device, t.Pin, t.Partner, t.PartnerPin} {
		if e := NetworkHex(s, 32); e != nil {
			return nil, e
		}
	}
	if e := ServiceOrigin(t.Origin, private); e != nil {
		return nil, e
	}
	if t.Expires == 0 || (t.Purpose != "peer_data" && t.Purpose != "enrollment") || (t.Role != "initiator" && t.Role != "responder") {
		return nil, errors.New("INVALID_ATTACHMENT")
	}
	return NetworkCanonical("relay-attachment", t.Profile, t.Origin, t.Epoch, t.Session, t.Device, t.Pin, t.Partner, t.PartnerPin, t.Purpose, t.Role, strconv.FormatUint(uint64(t.Expires), 10)), nil
}
func (t RelayAttachment) Verify(key ed25519.PublicKey, now uint64, private bool) error {
	b, e := t.Canonical(private)
	if e != nil {
		return e
	}
	if uint64(t.Expires) <= now || uint64(t.Expires)-now > 30 {
		return errors.New("ATTACHMENT_EXPIRED")
	}
	sig, e := hex.DecodeString(t.Signature)
	if e != nil || NetworkHex(t.Signature, 64) != nil || len(key) != 32 || !ed25519.Verify(key, b, sig) {
		return errors.New("INVALID_SIGNATURE")
	}
	return nil
}

func NetworkDigest(b []byte) string { d := sha256.Sum256(b); return fmt.Sprintf("%x", d) }

// These envelopes freeze operation shapes. Service implementations must verify
// the proof's payload digest against each payload's canonical bytes before
// consuming a challenge, lease, slot or token. All arrays are present, even empty.
type NetworkChallengeRequest struct {
	Version        string `json:"version"`
	Profile        string `json:"profile"`
	Sender         string `json:"sender"`
	SenderPin      string `json:"sender_pin"`
	CertificateDER string `json:"certificate_der"`
}
type NetworkChallengeResult struct {
	Version   string      `json:"version"`
	Profile   string      `json:"profile"`
	Origin    string      `json:"origin"`
	Challenge string      `json:"challenge"`
	Expires   NetworkUint `json:"expires"`
}
type NetworkAnnounceRequest struct {
	Proof        NetworkProof        `json:"proof"`
	Announcement NetworkAnnouncement `json:"announcement"`
}
type NetworkLookupRequest struct {
	Proof NetworkProof `json:"proof"`
}
type NetworkLookupResult struct {
	Version string `json:"version"`
	Found   bool   `json:"found"`
	// Empty arrays on absent route; at most one current signed record on found.
	Proofs        []NetworkProof        `json:"proofs"`
	Announcements []NetworkAnnouncement `json:"announcements"`
}
type NetworkOffer struct {
	ICE              *NetworkICE        `json:"ice,omitempty"`
	SenderGeneration NetworkUint        `json:"sender_generation"`
	TargetGeneration NetworkUint        `json:"target_generation"`
	Candidates       []NetworkCandidate `json:"candidates"`
}

func (o NetworkOffer) Canonical() ([]byte, error) {
	if o.SenderGeneration == 0 || o.TargetGeneration == 0 || o.Candidates == nil || len(o.Candidates) > NetworkMaxOfferCandidates {
		return nil, errors.New("INVALID_OFFER")
	}
	f := []string{strconv.FormatUint(uint64(o.SenderGeneration), 10), strconv.FormatUint(uint64(o.TargetGeneration), 10), strconv.Itoa(len(o.Candidates))}
	seen := map[string]bool{}
	for _, c := range o.Candidates {
		if e := c.Validate(false); e != nil {
			return nil, e
		}
		s := c.Transport + "/" + c.Address + "/" + c.Scope + "/" + c.Interface
		if seen[s] {
			return nil, errors.New("INVALID_CANDIDATE")
		}
		seen[s] = true
		f = append(f, c.Transport, c.Address, c.Scope, c.Interface)
	}
	if o.ICE != nil {
		b, err := o.ICE.Canonical()
		if err != nil || len(o.Candidates) != 0 {
			return nil, errors.New("INVALID_ICE")
		}
		f = append(f, string(b))
		return NetworkCanonical("ice-offer-v1", f...), nil
	}
	return NetworkCanonical("offer", f...), nil
}

type NetworkOfferRequest struct {
	Proof NetworkProof `json:"proof"`
	Offer NetworkOffer `json:"offer"`
}
type NetworkRelayRequest struct {
	Proof NetworkProof `json:"proof"`
}
type NetworkRelayResult struct {
	Version     string            `json:"version"`
	Attachments []RelayAttachment `json:"attachments"` // exactly one for requesting leg
}
type NetworkResult struct {
	Version   string `json:"version"`
	State     string `json:"state"` // accepted, replayed, released
	Operation string `json:"operation"`
}
type NetworkFailure struct {
	Version    string      `json:"version"`
	Code       string      `json:"code"`
	Retryable  bool        `json:"retryable"`
	RetryAfter NetworkUint `json:"retry_after"`
}

// Stable bounded codes contain no secrets, candidate addresses or folder names.
const (
	NetworkInvalidRequest      = "INVALID_REQUEST"
	NetworkUnsupported         = "UNSUPPORTED_CAPABILITY"
	NetworkProfileMissing      = "PROFILE_MISSING"
	NetworkProfileExpired      = "PROFILE_EXPIRED"
	NetworkProfileUntrusted    = "PROFILE_UNTRUSTED"
	NetworkIdentityMismatch    = "IDENTITY_MISMATCH"
	NetworkPurposeMismatch     = "PURPOSE_MISMATCH"
	NetworkChallengeExpired    = "CHALLENGE_EXPIRED"
	NetworkReplay              = "REPLAY_REJECTED"
	NetworkStaleGeneration     = "STALE_GENERATION"
	NetworkIdempotencyConflict = "IDEMPOTENCY_CONFLICT"
	NetworkQuota               = "QUOTA_EXCEEDED"
	// NetworkRelayBudget refuses relay data because the operator's monthly
	// relay allowance is spent; it resets at the next UTC month (E09).
	NetworkRelayBudget        = "RELAY_BUDGET"
	NetworkUnavailable        = "ROUTE_UNAVAILABLE"
	NetworkServiceUnavailable = "SERVICE_UNAVAILABLE"
	NetworkCanceled           = "CANCELED"
)

func (p NetworkProof) Intent(private bool) ([]byte, error) {
	bound := p
	bound.Payload = NetworkDigest(nil) // intent can be built before its digest is assigned
	if _, e := bound.Canonical(private); e != nil {
		return nil, e
	}
	return NetworkCanonical(p.Kind+"-intent", p.Profile, p.Origin, p.Sender, p.SenderPin, p.Target, p.TargetPin, p.Purpose, p.Operation, p.Session, strconv.FormatUint(uint64(p.Generation), 10), p.Role), nil
}
func (p NetworkProof) VerifyPayload(canonical []byte) error {
	if len(canonical) > NetworkMaxBytes || NetworkDigest(canonical) != p.Payload {
		return errors.New("PAYLOAD_MISMATCH")
	}
	return nil
}
func (r NetworkAnnounceRequest) Verify(private bool, now uint64) error {
	if r.Proof.Kind != "announce" || r.Proof.Generation != r.Announcement.Generation {
		return errors.New("INVALID_PROOF")
	}
	payload, e := r.Announcement.Canonical(now, true)
	if e != nil {
		return e
	}
	if e = r.Proof.VerifyPayload(payload); e != nil {
		return e
	}
	return r.Proof.Verify(private, now)
}
func (r NetworkOfferRequest) Verify(private bool, now uint64) error {
	if err := r.VerifyICE(); err != nil {
		return err
	}
	if (r.Proof.Kind != "offer" && r.Proof.Kind != "accept") || r.Proof.Generation != r.Offer.SenderGeneration {
		return errors.New("INVALID_PROOF")
	}
	payload, e := r.Offer.Canonical()
	if e != nil {
		return e
	}
	if e = r.Proof.VerifyPayload(payload); e != nil {
		return e
	}
	return r.Proof.Verify(private, now)
}

// Relay token attachment requires proof from exactly the token's named leg.
// Service admission additionally requires stored reservation/partner acceptance,
// current epoch and a fresh issued proof challenge; this is not a folder gate.
func (t RelayAttachment) Bind(p NetworkProof, profile, origin, epoch string) error {
	if p.Kind != "attach" || t.Profile != profile || t.Origin != origin || t.Epoch != epoch || p.Profile != t.Profile || p.Origin != t.Origin || p.Session != t.Session || p.Sender != t.Device || p.SenderPin != t.Pin || p.Target != t.Partner || p.TargetPin != t.PartnerPin || p.Purpose != t.Purpose || p.Role != t.Role {
		return errors.New("PURPOSE_MISMATCH")
	}
	return nil
}

type NetworkRelayAttachRequest struct {
	Proof      NetworkProof    `json:"proof"`
	Attachment RelayAttachment `json:"attachment"`
}

func (r NetworkRelayAttachRequest) Verify(key ed25519.PublicKey, profile, origin, epoch string, now uint64, private bool) error {
	if e := r.Attachment.Bind(r.Proof, profile, origin, epoch); e != nil {
		return e
	}
	if e := r.Attachment.Verify(key, now, private); e != nil {
		return e
	}
	intent, e := r.Proof.Intent(private)
	if e != nil {
		return e
	}
	if e = r.Proof.VerifyPayload(intent); e != nil {
		return e
	}
	return r.Proof.Verify(private, now)
}
