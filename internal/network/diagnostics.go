package network

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/url"
	"time"

	"bytes"
	p "github.com/calebhabesh/orbit/internal/protocol"
	"github.com/pion/stun/v4"
	"net/netip"
)

// ProbeResult deliberately contains no addresses, credentials, or raw errors.
// Success describes the named probe only, never file storage or NAT type.
type ProbeResult struct {
	Kind       string `json:"kind"`
	Code       string `json:"code"`
	ObservedAt string `json:"observed_at"`
}

func ProbeCode(ctx context.Context, err error) string {
	if ctx.Err() != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "TIMEOUT"
		}
		return "CANCELLED"
	}
	if err == nil {
		return "VERIFIED"
	}
	var service *ServiceError
	if errors.As(err, &service) {
		return service.Code
	}
	var cert *tls.CertificateVerificationError
	var unknown x509.UnknownAuthorityError
	var hostname x509.HostnameError
	if errors.As(err, &cert) || errors.As(err, &unknown) || errors.As(err, &hostname) {
		return "TLS_IDENTITY_FAILED"
	}
	if errors.Is(err, ErrBackpressure) {
		return "QUOTA_EXCEEDED"
	}
	switch err.Error() {
	case "peer public key pin mismatch", "peer certificate does not match approved key pin":
		return "IDENTITY_MISMATCH"
	case "IDENTITY_MISMATCH", "PROFILE_UNTRUSTED", "PROFILE_EXPIRED", "QUOTA_EXCEEDED", "PEER_OFFLINE", "STALE_GENERATION":
		return err.Error()
	}
	return "UNAVAILABLE"
}
func probeResult(ctx context.Context, kind string, err error) ProbeResult {
	return ProbeResult{kind, ProbeCode(ctx, err), time.Now().UTC().Format(time.RFC3339Nano)}
}

// ProbeService uses the existing checked resolver/socket admission and TLS roots.
// It creates no registration or control channel. Each stage has a finite deadline.
func (c *ServiceClient) ProbeService(ctx context.Context) []ProbeResult {
	out := []ProbeResult{}
	work, done, err := c.begin(ctx)
	if err != nil {
		return []ProbeResult{probeResult(ctx, "service_tls", err)}
	}
	defer done()
	u, _ := url.Parse(c.origin)
	port := u.Port()
	if port == "" {
		port = "443"
	}
	dial, cancel := context.WithTimeout(work, 3*time.Second)
	conn, err := c.transport.DialContext(dial, "tcp", net.JoinHostPort(u.Hostname(), port))
	resultCtx := dial
	if ctx.Err() != nil {
		resultCtx = ctx
	}
	out = append(out, probeResult(resultCtx, "service_dns_tcp", err))
	cancel()
	if err != nil {
		return append(out, ProbeResult{Kind: "service_tls", Code: "NOT_TESTED"})
	}
	defer conn.Close()
	trust := c.transport.TLSClientConfig.Clone()
	trust.ServerName = u.Hostname()
	socket := tls.Client(conn, trust)
	handshake, stop := context.WithTimeout(work, 3*time.Second)
	err = socket.HandshakeContext(handshake)
	resultCtx = handshake
	if ctx.Err() != nil {
		resultCtx = ctx
	}
	out = append(out, probeResult(resultCtx, "service_tls", err))
	stop()
	return out
}

// CandidateSummary reads current leases without resolving, dialing or renewing.
type CandidateSummary struct{ LAN, Public, Expired int }

func (m *ConnectionManager) Candidates(t Target) CandidateSummary {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out CandidateSummary
	count := func(l candidateLease, public bool) {
		if l.invalidated || !m.now().Before(l.expires) {
			out.Expired += len(l.candidates)
			return
		}
		if public {
			out.Public += len(l.candidates)
		} else {
			out.LAN += len(l.candidates)
		}
	}
	if l, ok := m.public[t]; ok {
		count(l, true)
	}
	for k, l := range m.lan {
		if k.target == t {
			count(l, false)
		}
	}
	return out
}

// ProbeTCP performs only pinned TLS handshakes, with no HTTP or file operation.
// Direct and relay are separate probes; neither falls back to the other.
func (m *ConnectionManager) ProbeTCP(ctx context.Context, t Target, trust *tls.Config, relay bool) ProbeResult {
	kind := "direct_tls"
	if relay {
		kind = "relay_inner_tls"
	}
	m.mu.Lock()
	r, ok := m.routes[t]
	closed := m.closed
	if ok && !closed {
		m.tlsDialWG.Add(1)
	}
	m.mu.Unlock()
	if !ok || closed {
		return probeResult(ctx, kind, ErrNoRoute)
	}
	defer m.tlsDialWG.Done()
	life, stopLife := context.WithCancel(ctx)
	stop := context.AfterFunc(m.lifetime, stopLife)
	defer func() { stop(); stopLife() }()
	ctx = life
	validated, err := NewTransport(t, trust, func(context.Context, Target) (net.Conn, error) { return nil, ErrNoRoute })
	if err != nil {
		return probeResult(ctx, kind, err)
	}
	defer validated.CloseIdleConnections()
	bound := 3 * time.Second
	if relay {
		// Relay setup includes paced offer/reserve/attach on both devices.
		// A direct socket's 3 s bound can expire during legitimate budget
		// refill; use the existing handshake bound inside doctor's 20 s cap.
		bound = InnerHandshakeTimeout
	}
	work, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	var conn net.Conn
	if relay {
		if r.stream == nil {
			return ProbeResult{Kind: kind, Code: "NOT_TESTED"}
		}
		r.address = ""
		conn, err = m.dialTLSCandidates(context.WithValue(work, relayProbeKey{}, true), t, r, validated.transport.TLSClientConfig, nil)
	} else {
		r.stream = nil
		conn, err = m.dialTLSCandidates(work, t, r, validated.transport.TLSClientConfig, m.directCandidates(work, t))
	}
	if conn != nil {
		conn.Close()
	}
	return probeResult(work, kind, err)
}

// ProbeUDP verifies a STUN response, not reachability to a peer or a NAT class.
func ProbeUDP(ctx context.Context, servers, interfaces []string) ProbeResult {
	if len(servers) == 0 {
		return ProbeResult{Kind: "udp_stun", Code: "NOT_TESTED"}
	}
	if len(servers) > p.NetworkMaxSTUN {
		return probeResult(ctx, "udp_stun", ErrBackpressure)
	}
	// Interface policy cannot be silently bypassed by an ordinary UDP dial.
	// Bind a selected permitted source address when explicit interfaces exist.
	var local net.Addr
	if len(interfaces) > 0 {
		selected, err := SelectedInterfaces(interfaces)
		if err != nil || len(selected) == 0 {
			return probeResult(ctx, "udp_stun", ErrNoRoute)
		}
		for _, i := range selected {
			for _, prefix := range i.Prefixes {
				if prefix.Addr().Is4() {
					local = &net.UDPAddr{IP: net.IP(prefix.Addr().AsSlice())}
					break
				}
			}
			if local != nil {
				break
			}
		}
		if local == nil {
			return probeResult(ctx, "udp_stun", ErrNoRoute)
		}
	}
	work, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for _, server := range servers {
		addr, err := netip.ParseAddrPort(server)
		if err != nil || addr.Port() == 0 || !p.AllowedAddress(addr.Addr(), true) {
			return probeResult(work, "udp_stun", ErrNoRoute)
		}
		attempt, stop := context.WithTimeout(work, 750*time.Millisecond)
		conn, err := (&net.Dialer{LocalAddr: local}).DialContext(attempt, "udp", server)
		if err == nil {
			deadline, _ := attempt.Deadline()
			_ = conn.SetDeadline(deadline)
			closer := context.AfterFunc(attempt, func() { _ = conn.Close() })
			message, e := stun.Build(stun.BindingRequest, stun.TransactionID, stun.Fingerprint)
			if e == nil {
				_, e = conn.Write(message.Raw)
			}
			buffer := make([]byte, 512)
			if e == nil {
				n, readErr := conn.Read(buffer)
				e = readErr
				if e == nil {
					response := &stun.Message{Raw: buffer[:n]}
					e = response.Decode()
					if e == nil && (response.Type != stun.BindingSuccess || !bytes.Equal(response.TransactionID[:], message.TransactionID[:])) {
						e = ErrICEGather
					}
					var mapped stun.XORMappedAddress
					if e == nil {
						e = mapped.GetFrom(response)
					}
				}
			}
			closer()
			_ = conn.Close()
			err = e
		}
		if err == nil {
			stop()
			return probeResult(work, "udp_stun", nil)
		}
		stop()
		if work.Err() != nil {
			return probeResult(work, "udp_stun", work.Err())
		}
	}
	return probeResult(work, "udp_stun", ErrICEGather)
}
