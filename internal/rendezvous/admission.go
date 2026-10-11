package rendezvous

import (
	"errors"

	p "github.com/calebhabesh/orbit/internal/protocol"
)

// RG1 service admission. Open admission serves every device (the W03
// behaviour). An allowlist serves listed (device, pin) pairs fully. Unlisted
// devices get only the pairing path toward a listed device, and only within
// their own small share of challenges, controls and directory records. That
// path is: enrollment-purpose announce, control, lookup, offer, accept,
// reserve, release and attach whose target is a listed device, plus claiming a
// pairing mailbox a listed device created. Everything else is refused with
// NOT_ADMITTED before any allocation or rate state.
//
// Member sponsorship (optional) removes the per-device operator step: a served
// device sends a signed "sponsor" operation for each peer in its orbits, and
// the service lists that peer too. Peers are already pinned by authenticated
// orbit membership, so a listed device vouches only for devices its owner
// admitted to an orbit. The operator can revoke a sponsored device.

const (
	MaxAllowedDevices     = 64
	MaxUnlistedChallenges = 32
	MaxUnlistedControls   = 16
	MaxUnlistedRecords    = 16
)

// AllowedDevice is one allowlist entry: a device ID and its key pin, both
// 64 lowercase hex characters.
type AllowedDevice struct {
	Device string `json:"device"`
	Pin    string `json:"pin"`
}

// SponsoredDevice is a device a listed device vouched for (member
// sponsorship): Sponsor is the vouching device and Added the Unix time.
type SponsoredDevice struct {
	Device  string `json:"device"`
	Pin     string `json:"pin"`
	Sponsor string `json:"sponsor"`
	Added   uint64 `json:"added"`
}

// SponsorStore keeps the sponsored devices across restarts. The service is
// its only writer; it saves the whole list before admitting a new device.
type SponsorStore interface {
	SaveSponsored([]SponsoredDevice) error
}

// Admission is immutable once built; reload and sponsorship install a new one.
type Admission struct {
	listed     map[actor]bool // nil: open admission
	static     map[string]bool
	sponsoring bool
	sponsored  []SponsoredDevice
	revoked    map[string]bool
}

// OpenAdmission serves every device.
func OpenAdmission() *Admission { return &Admission{} }

// NewAllowlist validates and builds an allowlist. An empty list would admit
// no device at all, so it is refused as a likely mistake.
func NewAllowlist(devices []AllowedDevice) (*Admission, error) {
	if len(devices) == 0 || len(devices) > MaxAllowedDevices {
		return nil, errors.New("allowlist needs 1 to 64 devices")
	}
	listed, static := map[actor]bool{}, map[string]bool{}
	for _, d := range devices {
		if p.NetworkHex(d.Device, 32) != nil || p.NetworkHex(d.Pin, 32) != nil {
			return nil, errors.New("allowlist entries need a 64-hex device ID and key pin")
		}
		a := actor{d.Device, d.Pin}
		if listed[a] {
			return nil, errors.New("allowlist repeats device " + d.Device)
		}
		listed[a] = true
		static[d.Device] = true
	}
	return &Admission{listed: listed, static: static}, nil
}

// WithSponsorship lets listed devices vouch for the other devices in their
// orbits, which then get full service like listed ones. sponsored is the
// stored list; entries for revoked or statically listed devices, malformed
// entries and any beyond the 64-device limit are dropped (the caller saves the
// pruned list). Revoked devices cannot be sponsored again.
func (a *Admission) WithSponsorship(sponsored []SponsoredDevice, revoked []string) (*Admission, error) {
	if a.Open() {
		return nil, errors.New("sponsorship needs allowlist admission")
	}
	if len(revoked) > MaxRevokedDevices {
		return nil, errors.New("at most 256 revoked devices")
	}
	next := &Admission{listed: map[actor]bool{}, static: a.static, sponsoring: true, sponsored: []SponsoredDevice{}, revoked: map[string]bool{}}
	for _, id := range revoked {
		if p.NetworkHex(id, 32) != nil {
			return nil, errors.New("revoked entries need a 64-hex device ID")
		}
		next.revoked[id] = true
	}
	for x := range a.listed {
		if a.static[x.device] {
			next.listed[x] = true
		}
	}
	seen := map[string]bool{}
	for _, d := range sponsored {
		if p.NetworkHex(d.Device, 32) != nil || p.NetworkHex(d.Pin, 32) != nil || p.NetworkHex(d.Sponsor, 32) != nil || next.static[d.Device] || next.revoked[d.Device] || seen[d.Device] || len(next.listed) >= MaxAllowedDevices {
			continue
		}
		seen[d.Device] = true
		next.listed[actor{d.Device, d.Pin}] = true
		next.sponsored = append(next.sponsored, d)
	}
	return next, nil
}

// MaxRevokedDevices bounds the operator's revocation list.
const MaxRevokedDevices = 256

// Sponsoring reports whether listed devices may vouch for others.
func (a *Admission) Sponsoring() bool { return a != nil && a.sponsoring }

// Sponsored returns a copy of the sponsored devices.
func (a *Admission) Sponsored() []SponsoredDevice {
	if a == nil {
		return nil
	}
	return append([]SponsoredDevice(nil), a.sponsored...)
}

// sponsor admits t on behalf of the listed device by. It returns the same
// policy when t is already listed, or a refusal code.
func (a *Admission) sponsor(t actor, by string, now uint64) (*Admission, string) {
	switch {
	case a.listed[t]:
		return a, ""
	case a.revoked[t.device]:
		return nil, p.NetworkNotAdmitted
	case a.static[t.device]:
		// The operator listed this device with another key pin.
		return nil, p.NetworkIdentityMismatch
	}
	next := &Admission{listed: map[actor]bool{}, static: a.static, sponsoring: true, sponsored: []SponsoredDevice{}, revoked: a.revoked}
	for x := range a.listed {
		if x.device != t.device {
			next.listed[x] = true
		}
	}
	// A device that made a new identity replaces its earlier sponsored pin.
	for _, d := range a.sponsored {
		if d.Device != t.device {
			next.sponsored = append(next.sponsored, d)
		}
	}
	if len(next.listed) >= MaxAllowedDevices {
		return nil, p.NetworkQuota
	}
	next.listed[t] = true
	next.sponsored = append(next.sponsored, SponsoredDevice{Device: t.device, Pin: t.pin, Sponsor: by, Added: now})
	return next, ""
}

// Open reports whether every device is admitted.
func (a *Admission) Open() bool { return a == nil || a.listed == nil }

// Size is the number of listed devices (0 when open).
func (a *Admission) Size() int {
	if a == nil {
		return 0
	}
	return len(a.listed)
}

func (a *Admission) listedActor(x actor) bool { return a.Open() || a.listed[x] }

// admits decides one verified signed operation. Pairing mailbox ownership is
// checked separately in pairing.
func (a *Admission) admits(q p.NetworkProof) bool {
	if a.listedActor(who(q)) {
		return true
	}
	if q.Purpose != "enrollment" {
		return false
	}
	switch q.Kind {
	case "announce", "authenticate", "pairing":
		return true
	case "lookup", "offer", "accept", "reserve", "release", "attach":
		return a.listed[target(q)]
	}
	return false
}

// SetAdmission installs a reloaded admission policy. Directory records,
// controls and sessions that the new policy no longer admits are withdrawn at
// once; their sockets close outside the service lock.
func (s *Service) SetAdmission(a *Admission) {
	if a == nil {
		a = OpenAdmission()
	}
	var closing []controlConn
	s.mu.Lock()
	s.admission = a
	if !a.Open() {
		for k := range s.records {
			if !a.listedActor(k.actor) && k.purpose != "enrollment" {
				delete(s.records, k)
			}
		}
		for k, c := range s.controls {
			if !a.listedActor(k.actor) && k.purpose != "enrollment" {
				delete(s.controls, k)
				closing = append(closing, c.conn)
			}
		}
		for id, v := range s.sessions {
			if !a.admits(v.proof) || (v.proof.Purpose != "enrollment" && !a.listedActor(target(v.proof))) {
				s.dropSession(id)
			}
		}
	}
	s.mu.Unlock()
	for _, c := range closing {
		_ = c.CloseNow()
	}
}

// Admission reports the installed policy.
func (s *Service) Admission() *Admission {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.admission
}

// unlisted* count entries held by actors the policy does not list.
func (s *Service) unlistedChallenges() int {
	n := 0
	for _, c := range s.challenges {
		if !s.admission.listedActor(c.who) {
			n++
		}
	}
	return n
}
func (s *Service) unlistedControls() int {
	n := 0
	for k := range s.controls {
		if !s.admission.listedActor(k.actor) {
			n++
		}
	}
	return n
}
func (s *Service) unlistedRecords() int {
	n := 0
	for k := range s.records {
		if !s.admission.listedActor(k.actor) {
			n++
		}
	}
	return n
}
