package network

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"time"

	p "github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/pion/ice/v4"
	"github.com/pion/stun/v4"
	"github.com/pion/transport/v5"
	"github.com/pion/transport/v5/stdnet"
)

const ICEGatherTimeout = 3 * time.Second
const ICECheckTimeout = 5 * time.Second

var ErrICEGather = errors.New("ICE_NO_CANDIDATES")
var ErrICEQUIC = errors.New("ICE_QUIC_UNAVAILABLE")
var ErrICEChecks = errors.New("ICE_CHECKS_FAILED")

// ICESession owns one bounded agent. QUIC reads only the established Conn.
// Credentials remain in memory and are cleared when the session is closed.
type ICESession struct {
	agent       *ice.Agent
	mu          sync.Mutex
	local       p.NetworkICE
	closed      bool
	changed     chan struct{}
	changedOnce sync.Once
	selected    string
}

func NewICESession(ctx context.Context, servers []string, interfaces []string, networks ...transport.Net) (*ICESession, error) {
	if len(servers) > p.NetworkMaxSTUN || len(interfaces) > 8 || len(networks) > 1 {
		return nil, ErrBackpressure
	}
	urls := make([]*stun.URI, 0, len(servers))
	for _, s := range servers {
		addr, err := netip.ParseAddrPort(s)
		if err != nil || addr.Port() == 0 || !p.AllowedAddress(addr.Addr(), true) {
			return nil, ErrNoRoute
		}
		urls = append(urls, &stun.URI{Scheme: stun.SchemeTypeSTUN, Host: addr.Addr().String(), Port: int(addr.Port()), Proto: stun.ProtoTypeUDP})
	}
	var rawNet transport.Net
	if len(networks) > 0 {
		rawNet = networks[0]
	}
	if rawNet == nil {
		n, err := stdnet.NewNet()
		if err != nil {
			return nil, err
		}
		rawNet = n
	}
	approved := map[string]bool{}
	for _, s := range servers {
		approved[s] = true
	}
	opts := []ice.AgentOption{ice.WithNet(&iceNet{Net: rawNet, servers: approved}),
		ice.WithUrls(urls), ice.WithNetworkTypes([]ice.NetworkType{ice.NetworkTypeUDP4, ice.NetworkTypeUDP6}),
		ice.WithCandidateTypes([]ice.CandidateType{ice.CandidateTypeHost, ice.CandidateTypeServerReflexive}),
		ice.WithMulticastDNSMode(ice.MulticastDNSModeDisabled), ice.WithSTUNGatherTimeout(ICEGatherTimeout), ice.WithMaxBindingRequests(7),
		ice.WithDisconnectedTimeout(5 * time.Second), ice.WithFailedTimeout(5 * time.Second),
		ice.WithIPFilter(func(ip net.IP) bool {
			addr, ok := netip.AddrFromSlice(ip)
			return ok && p.AllowedAddress(addr.Unmap(), true)
		}),
		ice.WithRemoteIPFilter(func(ip net.IP) bool {
			addr, ok := netip.AddrFromSlice(ip)
			return ok && p.AllowedAddress(addr.Unmap(), false)
		}),
	}
	// Limit the addresses before opening sockets; a multi-address interface cannot
	// cause unlimited gathering. Selected interface intent also applies to ICE.
	var filterMu sync.Mutex
	addresses := map[string]bool{}
	opts = append(opts, ice.WithIPFilter(func(ip net.IP) bool {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok || !p.AllowedAddress(addr.Unmap(), true) {
			return false
		}
		filterMu.Lock()
		defer filterMu.Unlock()
		if addresses[ip.String()] {
			return true
		}
		if len(addresses) >= 2 {
			return false
		}
		addresses[ip.String()] = true
		return true
	}), ice.WithInterfaceFilter(func(name string) bool {
		if len(interfaces) == 0 {
			return true
		}
		for _, i := range interfaces {
			if i == name {
				return true
			}
		}
		return false
	}))
	return newICESession(ctx, opts)
}
func newICESession(ctx context.Context, opts []ice.AgentOption) (*ICESession, error) {
	agent, err := ice.NewAgentWithOptions(opts...)
	if err != nil {
		return nil, err
	}
	s := &ICESession{agent: agent, changed: make(chan struct{})}
	stop := context.AfterFunc(ctx, func() { _ = agent.Close() })
	defer stop()
	failed := true
	defer func() {
		if failed {
			_ = s.Close()
		}
	}()
	_ = agent.OnSelectedCandidatePairChange(func(local, remote ice.Candidate) { s.pairChanged(local.Marshal() + "/" + remote.Marshal()) })
	_ = agent.OnConnectionStateChange(func(state ice.ConnectionState) {
		if state == ice.ConnectionStateDisconnected || state == ice.ConnectionStateFailed || state == ice.ConnectionStateClosed {
			s.invalidate()
		}
	})
	complete := make(chan struct{})
	var once sync.Once
	_ = agent.OnCandidate(func(c ice.Candidate) {
		if c == nil {
			once.Do(func() { close(complete) })
		}
	})
	if err = agent.GatherCandidates(); err != nil {
		return nil, err
	}
	work, cancel := context.WithTimeout(ctx, ICEGatherTimeout+time.Second)
	defer cancel()
	select {
	case <-complete:
	case <-work.Done():
		return nil, ErrICEGather
	}
	candidates, err := agent.GetLocalCandidates()
	if err != nil {
		return nil, err
	}
	s.local = p.NetworkICE{Mode: "offer", Candidates: []string{}}
	s.local.Ufrag, s.local.Password, err = agent.GetLocalUserCredentials()
	if err != nil {
		return nil, err
	}
	for _, c := range candidates {
		// Strip related topology from srflx candidates; no private host addresses go
		// through the service. Pion still owns the underlying local candidate.
		wire := c.Marshal()
		if c.Type() == ice.CandidateTypeServerReflexive {
			parsed, e := ice.NewCandidateServerReflexive(&ice.CandidateServerReflexiveConfig{Network: "udp", Address: c.Address(), Port: c.Port(), Component: c.Component(), Foundation: c.Foundation(), Priority: c.Priority(), RelAddr: "0.0.0.0", RelPort: 0})
			if e != nil {
				continue
			}
			wire = parsed.Marshal()
		}
		if p.ValidateICECandidate(wire) == nil {
			s.local.Candidates = append(s.local.Candidates, wire)
		}
	}
	if _, err = s.local.Canonical(); err != nil {
		return nil, ErrICEGather
	}
	failed = false
	return s, nil
}
func (s *ICESession) Description(mode string) p.NetworkICE {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.local
	d.Mode = mode
	d.Candidates = append([]string{}, d.Candidates...)
	return d
}
func (s *ICESession) Connect(ctx context.Context, remote p.NetworkICE, controlling bool) (*PairPacketConn, error) {
	if _, err := remote.Canonical(); err != nil || remote.Mode == "request" {
		return nil, ErrNoRoute
	}
	for _, wire := range remote.Candidates {
		c, err := ice.UnmarshalCandidate(wire)
		if err != nil {
			return nil, err
		}
		if err = s.agent.AddRemoteCandidate(c); err != nil {
			return nil, err
		}
	}
	work, cancel := context.WithTimeout(ctx, ICECheckTimeout)
	defer cancel()
	var conn *ice.Conn
	var err error
	if controlling {
		conn, err = s.agent.Dial(work, remote.Ufrag, remote.Password)
	} else {
		conn, err = s.agent.Accept(work, remote.Ufrag, remote.Password)
	}
	if err != nil {
		return nil, ErrICEChecks
	}
	packet, err := NewPairPacketConn(conn)
	if err != nil {
		return nil, err
	}
	select {
	case <-s.changed:
		return nil, ErrStale
	default:
	}
	return packet, nil
}
func (s *ICESession) pairChanged(pair string) {
	s.mu.Lock()
	changed := s.selected != "" && s.selected != pair
	s.selected = pair
	s.mu.Unlock()
	if changed {
		s.invalidate()
	}
}
func (s *ICESession) invalidate() { s.changedOnce.Do(func() { close(s.changed) }) }
func (s *ICESession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.local = p.NetworkICE{}
	s.mu.Unlock()
	s.invalidate()
	return s.agent.Close()
}
