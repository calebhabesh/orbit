package model

// WANAdmission is an independent symbolic oracle. It deliberately does not import
// wire validators, the transport manager, or service implementations. Time is
// supplied in integer seconds. Authentication/profile facts come from a test's
// separately verified crypto inputs. Folder authority is NEVER an output.
type WANIdentity struct{ Device, Pin string }
type WANChallenge struct {
	Who   WANIdentity
	Until uint64
}
type WANLease struct {
	Generation, Until uint64
	Payload           string
}
type WANOperation struct {
	Payload string
	Until   uint64
}
type WANAdmission struct {
	Challenges  map[string]WANChallenge
	Operations  map[string]WANOperation
	Routes      map[WANIdentity]WANLease
	Attachments map[string]uint64
	Spent       map[string]uint64
	Epoch       string
}

func NewWANAdmission(epoch string) *WANAdmission {
	return &WANAdmission{Challenges: map[string]WANChallenge{}, Operations: map[string]WANOperation{}, Routes: map[WANIdentity]WANLease{}, Attachments: map[string]uint64{}, Spent: map[string]uint64{}, Epoch: epoch}
}
func (m *WANAdmission) Expire(now uint64) {
	for k, until := range m.Spent {
		if now >= until {
			delete(m.Spent, k)
		}
	}
	for k, until := range m.Attachments {
		if now >= until {
			delete(m.Attachments, k)
		}
	}
	for k, v := range m.Challenges {
		if now >= v.Until {
			delete(m.Challenges, k)
		}
	}
	for k, v := range m.Operations {
		if now >= v.Until {
			delete(m.Operations, k)
		}
	}
	for k, v := range m.Routes {
		if now >= v.Until {
			delete(m.Routes, k)
		}
	}
}
func (m *WANAdmission) Challenge(nonce string, who WANIdentity, now uint64) string {
	m.Expire(now)
	if nonce == "" || who.Device == "" || who.Pin == "" {
		return "invalid"
	}
	if _, ok := m.Challenges[nonce]; ok {
		return "replay"
	}
	if len(m.Challenges) >= 128 {
		return "capacity"
	}
	m.Challenges[nonce] = WANChallenge{who, now + 60}
	return "accepted"
}

// Operation includes semantic bindings but excludes fresh challenge/expiry.
// Retry of a delivered mutation requires fresh authentication and returns its
// previous result. Changed payload under the same actor/operation fails.
func (m *WANAdmission) Announce(who WANIdentity, nonce, operation, payload string, generation, lease, now uint64, authenticated, profile bool) string {
	m.Expire(now)
	if !authenticated || !profile {
		return "unauthorized"
	}
	c, ok := m.Challenges[nonce]
	if !ok || c.Who != who {
		return "replay"
	}
	if generation == 0 || lease <= now || lease-now > 600 {
		return "invalid"
	}
	k := who.Device + "/" + who.Pin + "/" + operation
	if old, ok := m.Operations[k]; ok {
		if old.Payload != payload {
			return "conflict"
		}
		delete(m.Challenges, nonce)
		return "replayed"
	}
	if old, ok := m.Routes[who]; ok && generation <= old.Generation {
		return "stale"
	}
	if _, ok := m.Routes[who]; !ok && len(m.Routes) >= 128 {
		return "capacity"
	}
	if len(m.Operations) >= 1024 {
		return "capacity"
	}
	delete(m.Challenges, nonce)
	m.Operations[k] = WANOperation{payload, now + 300}
	m.Routes[who] = WANLease{generation, lease, payload}
	return "accepted"
}
func (m *WANAdmission) Lookup(who WANIdentity, authenticated bool, now uint64) (WANLease, bool) {
	m.Expire(now)
	if !authenticated {
		return WANLease{}, false
	}
	v, ok := m.Routes[who]
	return v, ok
}
func (m *WANAdmission) Attach(session, role, epoch, purpose string, who, partner WANIdentity, now, expires uint64, signature, accepted bool) string {
	m.Expire(now)
	if !signature || !accepted || epoch != m.Epoch || (purpose != "enrollment" && purpose != "peer_data") || who.Device == "" || who.Pin == "" || partner.Device == "" || partner.Pin == "" || (role != "initiator" && role != "responder") {
		return "unauthorized"
	}
	if expires <= now || expires-now > 30 {
		return "expired"
	}
	k := session + "/" + role
	if _, ok := m.Spent[k]; ok {
		return "replay"
	}
	if len(m.Attachments) >= 128 || len(m.Spent) >= 1024 {
		return "capacity"
	}
	m.Attachments[k] = now + 3600
	m.Spent[k] = expires
	return "attached"
}
func (m *WANAdmission) Release(session string) {
	delete(m.Attachments, session+"/initiator")
	delete(m.Attachments, session+"/responder")
}
func (m *WANAdmission) Restart(epoch string) { *m = *NewWANAdmission(epoch) }

// WANEnrollment separates routability from durable explicit folder approval.
// Snapshot copies simulate persistence/restart without depending on SQL logic.
type WANJoin struct {
	Folder, Attempt, Inviter, Requester, Pin, Prior, Digest string
	Until                                                   uint64
}
type WANEnrollment struct {
	Pending      map[string]WANJoin
	Approved     map[string]WANJoin
	Retired      map[string]bool
	Revoked      map[string]bool
	Capabilities map[string]string
}

func NewWANEnrollment() *WANEnrollment {
	return &WANEnrollment{map[string]WANJoin{}, map[string]WANJoin{}, map[string]bool{}, map[string]bool{}, map[string]string{}}
}
func (j WANJoin) ID() string { return j.Folder + "/" + j.Inviter + "/" + j.Requester + "/" + j.Attempt }
func (m *WANEnrollment) Submit(j WANJoin, capability string, now uint64, pinned, proof bool) string {
	if !pinned || !proof || m.Retired[j.Requester] || m.Revoked[capability] {
		return "unauthorized"
	}
	if now >= j.Until {
		return "expired"
	}
	k := j.ID()
	if old, ok := m.Capabilities[k]; ok && old != capability {
		return "unauthorized"
	}
	if old, ok := m.Pending[k]; ok {
		if old != j {
			return "conflict"
		}
		return "replayed"
	}
	if old, ok := m.Approved[k]; ok {
		if old != j {
			return "conflict"
		}
		return "approved"
	}
	if len(m.Pending) >= 128 {
		return "capacity"
	}
	m.Pending[k] = j
	m.Capabilities[k] = capability
	return "pending"
}
func (m *WANEnrollment) Approve(request, folder, requester, pin, prior, digest, capability string, now uint64) string {
	j, ok := m.Pending[request]
	if !ok {
		return "unknown"
	}
	if m.Capabilities[request] != capability || m.Retired[j.Requester] || m.Revoked[capability] || now >= j.Until {
		return "unauthorized"
	}
	if j.Folder != folder || j.Requester != requester || j.Pin != pin || j.Prior != prior || j.Digest != digest {
		return "stale"
	}
	m.Approved[request] = j
	delete(m.Pending, request)
	return "approved"
}
func (m *WANEnrollment) FolderAccess(j WANJoin) bool {
	old, ok := m.Approved[j.ID()]
	return ok && old == j && !m.Retired[j.Requester]
}

// WANConnection models one logical peer/purpose cycle. Candidate completions
// carry generation+attempt; a verified old request may drain but cannot publish
// new route state. Mutation IDs and receipts are outside route policy.
type WANAttempt struct {
	Generation uint64
	Kind       string
}
type WANConnection struct {
	Generation uint64
	Attempts   map[string]WANAttempt
	Route      string
	Closed     bool
}

func NewWANConnection() *WANConnection {
	return &WANConnection{Generation: 1, Attempts: map[string]WANAttempt{}}
}
func (m *WANConnection) Start(id, kind string) bool {
	if m.Closed || len(m.Attempts) >= 3 || id == "" || (kind != "direct" && kind != "relay") {
		return false
	}
	if _, ok := m.Attempts[id]; ok {
		return false
	}
	n := 0
	for _, a := range m.Attempts {
		if a.Kind == kind {
			n++
		}
	}
	if kind == "direct" && n >= 2 || kind == "relay" && n >= 1 {
		return false
	}
	m.Attempts[id] = WANAttempt{m.Generation, kind}
	return true
}
func (m *WANConnection) Complete(id string, generation uint64, pinned bool) bool {
	a, ok := m.Attempts[id]
	if !ok || generation != m.Generation || a.Generation != generation || m.Closed {
		return false
	}
	delete(m.Attempts, id)
	if !pinned {
		return false
	}
	if m.Route != "direct" || a.Kind == "direct" {
		m.Route = a.Kind
	}
	return true
}
func (m *WANConnection) Change() {
	if m.Closed {
		return
	}
	m.Generation++
	m.Attempts = map[string]WANAttempt{}
	m.Route = ""
}
func (m *WANConnection) Cancel() { m.Closed = true; m.Attempts = map[string]WANAttempt{}; m.Route = "" }
