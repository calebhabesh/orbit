// Package rendezvous owns ephemeral authenticated network coordination. It has
// no repository, folder, inventory or owner-control dependency.
package rendezvous

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/calebhabesh/orbit/internal/network"
	p "github.com/calebhabesh/orbit/internal/protocol"
)

const MaxRecords = 128
const MaxSessions = 128
const MaxUnknownOffers = 2
const MaxConnections = 256

type actor struct{ device, pin string }
type routeKey struct {
	actor
	purpose string
}
type challenge struct {
	who     actor
	expires uint64
}
type record struct {
	proof        p.NetworkProof
	announcement p.NetworkAnnouncement
}
type replay struct {
	semantic string
	expires  uint64
	result   any
}
type bucket struct {
	tokens float64
	at     time.Time
}
type session struct {
	proof       p.NetworkProof
	offer       p.NetworkOffer
	accepted    bool
	expires     uint64
	attachments map[string]p.RelayAttachment
	relay       *relaySession
}
type control struct {
	conn    controlConn
	events  chan p.NetworkOfferRequest
	profile string
}

// MaxServedProfiles bounds rotation overlap: the current profile plus one
// next/previous epoch under the same authority, environment and origins.
const MaxServedProfiles = 2

// ServedProfile pairs a reviewed selection with its online signing key.
type ServedProfile struct {
	Selection  network.ProfileSelection
	ServiceKey ed25519.PrivateKey
}
type servedProfile struct {
	selection network.ProfileSelection
	key       ed25519.PrivateKey
}

type Options struct {
	Selection  network.ProfileSelection
	Origin     string // HTTPS control origin in the reviewed profile
	ServiceKey ed25519.PrivateKey
	// Overlap admits clients that reviewed an adjacent epoch during rotation.
	// It must share authority, environment, HTTPS origin and relay origin.
	Overlap []ServedProfile
	Now     func() time.Time
	Relay   RelayLimits
	// RelayBudget, when set, is the monthly relay egress allowance (E09).
	RelayBudget *RelayBudget
}

type Service struct {
	mailboxes                          map[string]*mailbox
	pairSources                        map[string]bucket
	mu                                 sync.Mutex
	selection                          network.ProfileSelection
	origin, digest, epoch, relayOrigin string
	key                                ed25519.PrivateKey
	now                                func() time.Time
	closed                             bool
	challenges                         map[string]challenge
	records                            map[routeKey]record
	operations                         map[string]replay
	sessions                           map[string]*session
	controls                           map[routeKey]*control
	rates                              map[actor]bucket
	sources                            map[string]bucket
	global                             bucket
	active                             chan struct{}
	controlWG                          sync.WaitGroup
	pending                            map[controlConn]bool
	cancel                             context.CancelFunc
	sweepWG                            sync.WaitGroup
	controlSlots                       chan struct{}
	relaySlots                         chan struct{}
	relayLimits                        RelayLimits
	bandwidth                          *byteLimiter
	deviceBandwidth                    map[actor]*byteLimiter
	served                             map[string]*servedProfile
	stats                              counters
	budget                             *RelayBudget
}

func New(opts Options) (*Service, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if len(opts.Overlap) > MaxServedProfiles-1 {
		return nil, errors.New("PROFILE_UNTRUSTED")
	}
	served := map[string]*servedProfile{}
	digest, relay := "", ""
	for i, candidate := range append([]ServedProfile{{opts.Selection, opts.ServiceKey}}, opts.Overlap...) {
		sp, d, r, err := serveProfile(candidate, opts.Origin, opts.Now)
		if err != nil {
			return nil, err
		}
		if i == 0 {
			digest, relay = d, r
		} else if r != relay || sp.selection.Authority != opts.Selection.Authority || sp.selection.Environment != opts.Selection.Environment || served[d] != nil {
			return nil, errors.New("PROFILE_UNTRUSTED")
		}
		served[d] = sp
	}
	opts.Selection = served[digest].selection
	limits, err := opts.Relay.defaults()
	if err != nil {
		return nil, err
	}
	service := &Service{served: served, relaySlots: make(chan struct{}, 2*(network.MaxServiceDataTunnels+MaxUnknownOffers)), relayLimits: limits, bandwidth: newByteLimiter(limits.ServiceBytesPerSecond), deviceBandwidth: map[actor]*byteLimiter{}, selection: opts.Selection, origin: opts.Origin, digest: digest, epoch: nonce(), relayOrigin: relay, key: append(ed25519.PrivateKey{}, opts.ServiceKey...), now: opts.Now, budget: opts.RelayBudget,
		challenges: map[string]challenge{}, records: map[routeKey]record{}, operations: map[string]replay{}, sessions: map[string]*session{}, controls: map[routeKey]*control{}, rates: map[actor]bucket{}, sources: map[string]bucket{}, active: make(chan struct{}, network.MaxServiceControls), pending: map[controlConn]bool{}, controlSlots: make(chan struct{}, network.MaxServiceControls)}
	lifetime, cancel := context.WithCancel(context.Background())
	service.cancel = cancel
	service.sweepWG.Add(1)
	go func() {
		defer service.sweepWG.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-lifetime.Done():
				return
			case <-ticker.C:
				service.mu.Lock()
				service.expire(uint64(service.now().Unix()))
				service.mu.Unlock()
			}
		}
	}()
	return service, nil
}

// serveProfile validates one served epoch. Expired overlap profiles are refused
// at startup; live expiry is enforced per request.
func serveProfile(candidate ServedProfile, origin string, now func() time.Time) (*servedProfile, string, string, error) {
	if err := candidate.Selection.Validate(uint64(now().Unix())); err != nil {
		return nil, "", "", err
	}
	if len(candidate.ServiceKey) != ed25519.PrivateKeySize || hex.EncodeToString(candidate.ServiceKey.Public().(ed25519.PublicKey)) != candidate.Selection.Profile.ServiceKey {
		return nil, "", "", errors.New("PROFILE_UNTRUSTED")
	}
	found := false
	relay := ""
	for _, o := range candidate.Selection.Profile.Origins {
		if o == origin && strings.HasPrefix(o, "https://") {
			found = true
		}
		if strings.HasPrefix(o, "wss://") && strings.TrimPrefix(o, "wss://") == strings.TrimPrefix(origin, "https://") {
			relay = o
		}
	}
	if !found {
		return nil, "", "", errors.New("PROFILE_UNTRUSTED")
	}
	// Copy profile slices; caller mutation cannot replace approved origins.
	selection := candidate.Selection
	selection.Profile.Origins = append([]string{}, selection.Profile.Origins...)
	selection.Profile.STUN = append([]string{}, selection.Profile.STUN...)
	digest, err := selection.Digest()
	if err != nil {
		return nil, "", "", err
	}
	return &servedProfile{selection: selection, key: append(ed25519.PrivateKey{}, candidate.ServiceKey...)}, digest, relay, nil
}

// profile returns a served epoch that is still valid. Unknown digests are
// untrusted; a known but expired epoch reports expiry.
func (s *Service) profile(digest string, now uint64) (*servedProfile, string) {
	sp := s.served[digest]
	if sp == nil {
		return nil, p.NetworkProfileUntrusted
	}
	if s.closed || sp.selection.Validate(now) != nil {
		return nil, p.NetworkProfileExpired
	}
	return sp, ""
}

// live reports whether any served epoch still admits new work.
func (s *Service) live(now uint64) bool {
	for _, sp := range s.served {
		if !s.closed && sp.selection.Validate(now) == nil {
			return true
		}
	}
	return false
}
func nonce() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func who(q p.NetworkProof) actor    { return actor{q.Sender, q.SenderPin} }
func target(q p.NetworkProof) actor { return actor{q.Target, q.TargetPin} }
func opKey(q p.NetworkProof) string {
	return strings.Join([]string{q.Profile, q.Sender, q.SenderPin, q.Purpose, q.Kind, q.Operation}, "/")
}
func semantic(q p.NetworkProof) string {
	return p.NetworkDigest(p.NetworkCanonical("semantic", q.Profile, q.Origin, q.Sender, q.SenderPin, q.Target, q.TargetPin, q.Purpose, q.Kind, q.Operation, q.Session, stringUint(q.Generation), q.Role, q.Payload))
}
func stringUint(n p.NetworkUint) string {
	b, _ := n.MarshalJSON()
	return strings.Trim(string(b), "\"")
}
func consume(b bucket, now time.Time, burst, rate float64) (bucket, bool) {
	if b.at.IsZero() {
		b = bucket{burst, now}
	}
	elapsed := now.Sub(b.at).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * rate
		if b.tokens > burst {
			b.tokens = burst
		}
		b.at = now
	}
	if b.tokens < 1 {
		return b, false
	}
	b.tokens--
	return b, true
}
func (s *Service) expire(now uint64) {
	for name, m := range s.mailboxes {
		if now >= m.expires {
			delete(s.mailboxes, name)
		}
	}
	s.expireBandwidth()
	for id, v := range s.sessions {
		if _, code := s.profile(v.proof.Profile, now); code != "" {
			s.dropSession(id)
		}
	}
	for k, v := range s.challenges {
		if now >= v.expires {
			delete(s.challenges, k)
		}
	}
	for k, v := range s.records {
		if _, code := s.profile(v.proof.Profile, now); code != "" || now >= uint64(v.announcement.Expires) {
			delete(s.records, k)
		}
	}
	for k, v := range s.operations {
		if now >= v.expires {
			delete(s.operations, k)
		}
	}
	for k, v := range s.sessions {
		if now >= v.expires {
			s.dropSession(k)
		}
	}
	for k, v := range s.rates {
		if s.now().Sub(v.at) > time.Minute {
			delete(s.rates, k)
		}
	}
	for k, v := range s.sources {
		if s.now().Sub(v.at) > time.Minute {
			delete(s.sources, k)
		}
	}
}
func (s *Service) admit(q p.NetworkProof, canonical []byte, now uint64) string {
	return s.admitOrigin(q, canonical, now, s.origin)
}
func (s *Service) admitOrigin(q p.NetworkProof, canonical []byte, now uint64, origin string) string {
	if _, code := s.profile(q.Profile, now); code != "" {
		return code
	}
	if q.Origin != origin {
		return p.NetworkProfileUntrusted
	}
	if q.Verify(s.selection.Private(), now) != nil || q.VerifyPayload(canonical) != nil {
		return p.NetworkIdentityMismatch
	}
	c, ok := s.challenges[q.Challenge]
	if !ok || c.who != who(q) || c.expires != uint64(q.Expires) {
		return p.NetworkReplay
	}
	// A cryptographically valid proof is one-use even on quota refusal.
	// Otherwise denied requests strand both challenge slots for a minute.
	delete(s.challenges, q.Challenge)
	b, exists := s.rates[who(q)]
	if !exists && len(s.rates) >= 1024 {
		return p.NetworkQuota
	}
	var allowed bool
	b, allowed = consume(b, s.now(), network.MetadataBurst, 1)
	s.rates[who(q)] = b
	if !allowed {
		return p.NetworkQuota
	}
	// Consume even on semantic failure: each signed exchange needs fresh proof.
	delete(s.challenges, q.Challenge)
	return ""
}
func (s *Service) cached(q p.NetworkProof, now uint64) (any, string) {
	if old, ok := s.operations[opKey(q)]; ok {
		if old.semantic != semantic(q) {
			return nil, p.NetworkIdempotencyConflict
		}
		return old.result, ""
	}
	if len(s.operations) >= network.MaxReplayEntries {
		return nil, p.NetworkQuota
	}
	return nil, ""
}
func (s *Service) remember(q p.NetworkProof, result any, now uint64) {
	s.operations[opKey(q)] = replay{semantic(q), now + 300, result}
}
func result(q p.NetworkProof, state string) p.NetworkResult {
	return p.NetworkResult{Version: "1", State: state, Operation: q.Operation}
}

func (s *Service) challenge(r p.NetworkChallengeRequest, now uint64) (any, string) {
	if _, code := s.profile(r.Profile, now); code != "" {
		return nil, code
	}
	if r.Version != "1" || p.NetworkHex(r.Sender, 32) != nil || p.NetworkHex(r.SenderPin, 32) != nil {
		return nil, p.NetworkInvalidRequest
	}
	if _, _, err := p.NetworkCertificate(r.CertificateDER, r.SenderPin, now); err != nil {
		return nil, p.NetworkIdentityMismatch
	}
	a := actor{r.Sender, r.SenderPin}
	count := 0
	for _, c := range s.challenges {
		if c.who == a {
			count++
		}
	}
	if count >= 2 || len(s.challenges) >= 128 {
		return nil, p.NetworkQuota
	}
	n := nonce()
	s.challenges[n] = challenge{a, now + 60}
	return p.NetworkChallengeResult{Version: "1", Profile: r.Profile, Origin: s.origin, Challenge: n, Expires: p.NetworkUint(now + 60)}, ""
}
func (s *Service) announce(r p.NetworkAnnounceRequest, now uint64) (any, string) {
	payload, err := r.Announcement.Canonical(now, true)
	if err != nil || r.Proof.Kind != "announce" || r.Proof.Generation != r.Announcement.Generation {
		return nil, p.NetworkInvalidRequest
	}
	if code := s.admit(r.Proof, payload, now); code != "" {
		return nil, code
	}
	if old, code := s.cached(r.Proof, now); old != nil || code != "" {
		return old, code
	}
	key := routeKey{who(r.Proof), r.Proof.Purpose}
	if old, ok := s.records[key]; ok && old.announcement.Generation >= r.Announcement.Generation {
		return nil, p.NetworkStaleGeneration
	}
	if _, ok := s.records[key]; !ok && len(s.records) >= MaxRecords {
		return nil, p.NetworkQuota
	}
	s.records[key] = record{r.Proof, r.Announcement}
	out := result(r.Proof, "accepted")
	s.remember(r.Proof, out, now)
	return out, ""
}
func (s *Service) intent(q p.NetworkProof, kind string, now uint64) string {
	if q.Kind != kind || q.Generation != 0 {
		return p.NetworkInvalidRequest
	}
	payload, err := q.Intent(s.selection.Private())
	if err != nil {
		return p.NetworkInvalidRequest
	}
	return s.admit(q, payload, now)
}
func (s *Service) lookup(q p.NetworkProof, now uint64) (any, string) {
	if code := s.intent(q, "lookup", now); code != "" {
		return nil, code
	}
	out := p.NetworkLookupResult{Version: "1", Proofs: []p.NetworkProof{}, Announcements: []p.NetworkAnnouncement{}}
	if r, ok := s.records[routeKey{target(q), q.Purpose}]; ok {
		out.Found = true
		out.Proofs = append(out.Proofs, r.proof)
		out.Announcements = append(out.Announcements, r.announcement)
	}
	return out, ""
}
func (s *Service) offer(r p.NetworkOfferRequest, now uint64) (any, string) {
	q := r.Proof
	if err := r.Verify(s.selection.Private(), now); err != nil {
		return nil, p.NetworkInvalidRequest
	}
	// Public service never exchanges LAN interfaces or local candidates.
	for _, c := range r.Offer.Candidates {
		if c.Validate(true) != nil {
			return nil, p.NetworkInvalidRequest
		}
	}
	b, _ := r.Offer.Canonical()
	if code := s.admit(q, b, now); code != "" {
		return nil, code
	}
	if old, code := s.cached(q, now); old != nil || code != "" {
		return old, code
	}
	if who(q) == target(q) {
		return nil, p.NetworkPurposeMismatch
	}
	// Unknown enrollment initiators need an authenticated control channel,
	// not a public directory announcement. Acceptance binds their exact live
	// offered session/generation below; data still requires both announcements.
	if q.Kind != "accept" || q.Purpose != "enrollment" {
		dest, ok := s.records[routeKey{target(q), q.Purpose}]
		if !ok {
			return nil, p.NetworkUnavailable
		}
		if dest.announcement.Generation != r.Offer.TargetGeneration {
			return nil, p.NetworkStaleGeneration
		}
	}
	if q.Purpose == "peer_data" {
		src, ok := s.records[routeKey{who(q), q.Purpose}]
		if !ok || src.announcement.Generation != r.Offer.SenderGeneration {
			return nil, p.NetworkStaleGeneration
		}
	}
	out := result(q, "accepted")
	switch q.Kind {
	case "offer":
		if q.Role != "initiator" {
			return nil, p.NetworkPurposeMismatch
		}
		if _, ok := s.sessions[q.Session]; ok {
			return nil, p.NetworkIdempotencyConflict
		}
		if len(s.sessions) >= MaxSessions {
			return nil, p.NetworkQuota
		}
		if q.Purpose == "enrollment" {
			n := 0
			for _, v := range s.sessions {
				if v.proof.Purpose == "enrollment" {
					n++
				}
			}
			if n >= MaxUnknownOffers {
				return nil, p.NetworkQuota
			}
		}
		c, ok := s.controls[routeKey{target(q), q.Purpose}]
		if !ok {
			return nil, p.NetworkUnavailable
		}
		select {
		case c.events <- r:
		default:
			return nil, p.NetworkQuota
		}
		s.sessions[q.Session] = &session{proof: q, offer: r.Offer, expires: now + 30, attachments: map[string]p.RelayAttachment{}}
	case "accept":
		v, ok := s.sessions[q.Session]
		if !ok || q.Role != "responder" || who(q) != target(v.proof) || target(q) != who(v.proof) || q.Purpose != v.proof.Purpose {
			return nil, p.NetworkPurposeMismatch
		}
		if (r.Offer.ICE == nil) != (v.offer.ICE == nil) || (v.offer.ICE != nil && (v.offer.ICE.Mode != "offer" || r.Offer.ICE.Mode != "accept")) {
			return nil, p.NetworkPurposeMismatch
		}
		if r.Offer.SenderGeneration != v.offer.TargetGeneration || r.Offer.TargetGeneration != v.offer.SenderGeneration {
			return nil, p.NetworkStaleGeneration
		}
		if v.accepted {
			return nil, p.NetworkIdempotencyConflict
		}
		c, ok := s.controls[routeKey{target(q), q.Purpose}]
		if !ok {
			return nil, p.NetworkUnavailable
		}
		select {
		case c.events <- r:
		default:
			return nil, p.NetworkQuota
		}
		v.accepted = true
	default:
		return nil, p.NetworkInvalidRequest
	}
	s.remember(q, out, now)
	return out, ""
}
func (s *Service) reservation(q p.NetworkProof, now uint64) (any, string) {
	if code := s.intent(q, q.Kind, now); code != "" {
		return nil, code
	}
	if old, code := s.cached(q, now); old != nil || code != "" {
		return old, code
	}
	v, ok := s.sessions[q.Session]
	if !ok || q.Purpose != v.proof.Purpose || !((q.Role == "initiator" && who(q) == who(v.proof) && target(q) == target(v.proof)) || (q.Role == "responder" && who(q) == target(v.proof) && target(q) == who(v.proof))) {
		return nil, p.NetworkPurposeMismatch
	}
	if q.Kind == "release" {
		s.dropSession(q.Session)
		out := result(q, "released")
		s.remember(q, out, now)
		return out, ""
	}
	if v.offer.ICE != nil {
		return nil, p.NetworkPurposeMismatch
	}
	if q.Kind != "reserve" || !v.accepted || s.relayOrigin == "" {
		return nil, p.NetworkUnavailable
	}
	sp, code := s.profile(q.Profile, now)
	if code != "" {
		return nil, code
	}
	t, exists := v.attachments[q.Role]
	if !exists {
		t = p.RelayAttachment{Profile: q.Profile, Origin: s.relayOrigin, Epoch: s.epoch, Session: q.Session, Device: q.Sender, Pin: q.SenderPin, Partner: q.Target, PartnerPin: q.TargetPin, Purpose: q.Purpose, Role: q.Role, Expires: p.NetworkUint(v.expires)}
		b, err := t.Canonical(s.selection.Private())
		if err != nil {
			return nil, p.NetworkInvalidRequest
		}
		t.Signature = hex.EncodeToString(ed25519.Sign(sp.key, b))
		v.attachments[q.Role] = t
	}
	out := p.NetworkRelayResult{Version: "1", Attachments: []p.RelayAttachment{t}}
	s.remember(q, out, now)
	return out, ""
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	headerSize := len(r.Method) + len(r.RequestURI) + len(r.Proto) + len(r.Host) + 16
	for name, values := range r.Header {
		for _, value := range values {
			headerSize += len(name) + len(value) + 4
		}
	}
	if headerSize > network.MaxNetworkHeaderBytes {
		s.failure(w, p.NetworkInvalidRequest)
		return
	}
	slots := s.active
	if r.Method == "GET" && r.URL.Path == "/network/v1/relay" {
		slots = s.relaySlots
	}
	if r.Method == "GET" && r.URL.Path == "/network/v1/control" {
		slots = s.controlSlots
	}
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	default:
		s.failure(w, p.NetworkQuota)
		return
	}
	// TLS terminates here: forwarded headers are neither needed nor accepted.
	if r.TLS == nil || r.Host != strings.TrimPrefix(s.origin, "https://") || nonemptyHeader(r.Header, "Origin") || r.URL.RawQuery != "" || len(r.Header.Values("Forwarded")) != 0 || hasForwarded(r.Header) {
		s.failure(w, p.NetworkInvalidRequest)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/network/v1/relay" {
		s.serveRelay(w, r)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/network/v1/control" {
		s.serveControl(w, r)
		return
	}
	if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Content-Encoding") != "" {
		s.failure(w, p.NetworkInvalidRequest)
		return
	}
	http.NewResponseController(w).SetReadDeadline(time.Now().Add(5 * time.Second))
	http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Second))
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, p.NetworkMaxBytes))
	if err != nil {
		s.failure(w, p.NetworkInvalidRequest)
		return
	}
	var request any
	switch r.URL.Path {
	case "/network/v1/challenge":
		request = &p.NetworkChallengeRequest{}
	case "/network/v1/announce":
		request = &p.NetworkAnnounceRequest{}
	case "/network/v1/pairing":
		request = &p.PairingRequest{}
	case "/network/v1/offer", "/network/v1/accept":
		request = &p.NetworkOfferRequest{}
	case "/network/v1/authenticate", "/network/v1/lookup", "/network/v1/reserve", "/network/v1/release":
		request = &p.NetworkLookupRequest{}
	default:
		s.failure(w, p.NetworkUnsupported)
		return
	}
	if p.NetworkDecode(body, request) != nil {
		s.failure(w, p.NetworkInvalidRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := uint64(s.now().Unix())
	s.expire(now)
	if !s.live(now) {
		s.failure(w, p.NetworkProfileExpired)
		return
	}
	b, allowed := consume(s.global, s.now(), 128, 128)
	s.global = b
	if !allowed {
		s.failure(w, p.NetworkQuota)
		return
	}
	var out any
	code := ""
	switch v := request.(type) {
	case *p.PairingRequest:
		out, code = s.pairing(*v, r, now)
	case *p.NetworkChallengeRequest:
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		rate, exists := s.sources[host]
		if !exists && len(s.sources) >= 128 {
			code = p.NetworkQuota
			break
		}
		rate, allowed = consume(rate, s.now(), 128, 2)
		s.sources[host] = rate
		if !allowed {
			code = p.NetworkQuota
			break
		}
		out, code = s.challenge(*v, now)
	case *p.NetworkAnnounceRequest:
		out, code = s.announce(*v, now)
	case *p.NetworkOfferRequest:
		if v.Proof.Kind != strings.TrimPrefix(r.URL.Path, "/network/v1/") {
			code = p.NetworkInvalidRequest
			break
		}
		out, code = s.offer(*v, now)
	case *p.NetworkLookupRequest:
		kind := strings.TrimPrefix(r.URL.Path, "/network/v1/")
		if v.Proof.Kind != kind {
			code = p.NetworkInvalidRequest
			break
		}
		switch kind {
		case "lookup":
			out, code = s.lookup(v.Proof, now)
		case "authenticate":
			code = s.intent(v.Proof, kind, now)
			out = result(v.Proof, "accepted")
		default:
			out, code = s.reservation(v.Proof, now)
		}
	}
	if code != "" {
		s.failure(w, code)
		return
	}
	writeJSON(w, out)
}
func nonemptyHeader(h http.Header, name string) bool {
	for _, value := range h.Values(name) {
		if value != "" {
			return true
		}
	}
	return false
}

func hasForwarded(h http.Header) bool {
	for k := range h {
		if strings.HasPrefix(strings.ToLower(k), "x-forwarded-") {
			return true
		}
	}
	return false
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Service) failure(w http.ResponseWriter, code string) {
	s.stats.refused(code)
	status := http.StatusForbidden
	retry := false
	switch code {
	case p.NetworkInvalidRequest, p.NetworkUnsupported:
		status = 400
	case p.NetworkQuota:
		status = 429
		retry = true
		w.Header().Set("Retry-After", "1")
	case p.NetworkStaleGeneration, p.NetworkIdempotencyConflict:
		status = 409
	case p.NetworkUnavailable, p.NetworkServiceUnavailable:
		status = 503
		retry = true
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	after := p.NetworkUint(0)
	if retry {
		after = 1
	}
	_ = json.NewEncoder(w).Encode(p.NetworkFailure{Version: "1", Code: code, Retryable: retry, RetryAfter: after})
}
