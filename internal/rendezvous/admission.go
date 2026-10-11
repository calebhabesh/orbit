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

// Admission is immutable once built; reload installs a new one.
type Admission struct {
	listed map[actor]bool // nil: open admission
}

// OpenAdmission serves every device.
func OpenAdmission() *Admission { return &Admission{} }

// NewAllowlist validates and builds an allowlist. An empty list would admit
// no device at all, so it is refused as a likely mistake.
func NewAllowlist(devices []AllowedDevice) (*Admission, error) {
	if len(devices) == 0 || len(devices) > MaxAllowedDevices {
		return nil, errors.New("allowlist needs 1 to 64 devices")
	}
	listed := map[actor]bool{}
	for _, d := range devices {
		if p.NetworkHex(d.Device, 32) != nil || p.NetworkHex(d.Pin, 32) != nil {
			return nil, errors.New("allowlist entries need a 64-hex device ID and key pin")
		}
		a := actor{d.Device, d.Pin}
		if listed[a] {
			return nil, errors.New("allowlist repeats device " + d.Device)
		}
		listed[a] = true
	}
	return &Admission{listed: listed}, nil
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
