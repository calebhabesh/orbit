package network

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"sync"
	"syscall"
	"time"

	p "github.com/calebhabesh/orbit/internal/protocol"
	"golang.org/x/sys/unix"
)

const DiscoveryPort = 22027
const MaxDiscoveryInterfaces = 8

// DirectSettings is advanced local configuration. Empty selection means up,
// multicast-capable nonloopback interfaces; explicit names narrow that set.
type DirectSettings struct {
	Interfaces  []string `json:"interfaces"`
	Listen      string   `json:"listen"`
	Disabled    bool     `json:"disabled"`
	UDPListen   string   `json:"udp_listen"`
	UDPDisabled bool     `json:"udp_disabled"`
}
type LocalInterface struct {
	Index    int
	Name     string
	Prefixes []netip.Prefix
}

func SelectedInterfaces(names []string) ([]LocalInterface, error) {
	if len(names) > MaxDiscoveryInterfaces {
		return nil, ErrBackpressure
	}
	selected := map[string]bool{}
	for _, name := range names {
		if name == "" || selected[name] {
			return nil, errors.New("INVALID_INTERFACE")
		}
		selected[name] = true
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []LocalInterface
	for _, iface := range interfaces {
		if len(selected) > 0 && !selected[iface.Name] {
			continue
		}
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		local := LocalInterface{Index: iface.Index, Name: iface.Name}
		for _, a := range addrs {
			prefix, err := netip.ParsePrefix(a.String())
			if err == nil && p.AllowedAddress(prefix.Addr(), true) {
				local.Prefixes = append(local.Prefixes, prefix)
			}
		}
		if len(local.Prefixes) > 0 && len(out) < MaxDiscoveryInterfaces {
			out = append(out, local)
		}
	}
	if len(selected) > 0 && len(out) != len(selected) {
		return out, errors.New("INTERFACE_UNAVAILABLE")
	}
	return out, nil
}

// GatherCandidates uses actual bound TCP ports and interface addresses. LAN
// records never enter public service announcements. No observed source port is used.
func GatherCandidates(interfaces []LocalInterface, bound net.Addr, public bool) []p.NetworkCandidate {
	if bound == nil {
		return nil
	}
	host, port, err := net.SplitHostPort(bound.String())
	if err != nil {
		return nil
	}
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil || n == 0 {
		return nil
	}
	bindIP, _ := netip.ParseAddr(host)
	var out []p.NetworkCandidate
	for _, iface := range interfaces {
		for _, prefix := range iface.Prefixes {
			ip := prefix.Addr()
			if bindIP.IsValid() && !bindIP.IsUnspecified() && ip != bindIP {
				continue
			}
			// An IPv4-only wildcard cannot serve IPv6.
			if bindIP.Is4() && !ip.Is4() {
				continue
			}
			c := p.NetworkCandidate{Transport: "tcp", Address: netip.AddrPortFrom(ip, uint16(n)).String(), Scope: "public"}
			if bound.Network() == "udp" || bound.Network() == "udp4" || bound.Network() == "udp6" {
				c.Transport = "udp"
			}
			if !public {
				c.Scope = "lan"
				c.Interface = iface.Name
			}
			if c.Validate(public) == nil && len(out) < p.NetworkMaxCandidates {
				out = append(out, c)
			}
		}
	}
	return out
}

// OptionalDirectListener is deliberately independent of mandatory manual
// listeners. Failure is returned as a route limitation to the daemon.
func OptionalDirectListener(settings DirectSettings) (net.Listener, error) {
	if settings.Disabled {
		return nil, nil
	}
	address := settings.Listen
	if address == "" {
		address = ":0"
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if host != "" {
		ip := net.ParseIP(host)
		if ip == nil || ip.IsLoopback() || ip.IsMulticast() {
			return nil, errors.New("INVALID_DIRECT_LISTENER")
		}
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	return boundedDirectListener(listener), nil
}

type LANDiscovery struct {
	conn        *net.UDPConn
	interfaces  []LocalInterface
	manager     *ConnectionManager
	known       func() []Target
	key         ed25519.PrivateKey
	device, pin string
	candidates  []p.NetworkCandidate
	genMu       sync.Mutex
	generation  uint64
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func NewLANDiscovery(ctx context.Context, manager *ConnectionManager, device, pin string, cert tls.Certificate, interfaces []LocalInterface, candidates []p.NetworkCandidate, known func() []Target) (*LANDiscovery, error) {
	key, ok := cert.PrivateKey.(ed25519.PrivateKey)
	if !ok || manager == nil || known == nil || len(interfaces) == 0 || len(interfaces) > MaxDiscoveryInterfaces {
		return nil, errors.New("LOCAL_DISCOVERY_UNAVAILABLE")
	}
	lc := net.ListenConfig{Control: func(_, _ string, raw syscall.RawConn) error {
		var inner error
		err := raw.Control(func(fd uintptr) {
			inner = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
			if inner == nil {
				inner = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_PKTINFO, 1)
			}
		})
		if err != nil {
			return err
		}
		return inner
	}}
	packet, err := lc.ListenPacket(ctx, "udp4", ":"+strconv.Itoa(DiscoveryPort))
	if err != nil {
		return nil, err
	}
	conn := packet.(*net.UDPConn)
	raw, err := conn.SyscallConn()
	if err != nil {
		conn.Close()
		return nil, err
	}
	group := [4]byte{239, 255, 79, 66}
	var joined []LocalInterface
	for _, iface := range interfaces {
		hasV4 := false
		for _, prefix := range iface.Prefixes {
			if prefix.Addr().Is4() && prefix.Addr().IsPrivate() {
				hasV4 = true
			}
		}
		if !hasV4 {
			continue
		}
		var inner error
		err = raw.Control(func(fd uintptr) {
			inner = unix.SetsockoptIPMreqn(int(fd), unix.IPPROTO_IP, unix.IP_ADD_MEMBERSHIP, &unix.IPMreqn{Multiaddr: group, Ifindex: int32(iface.Index)})
		})
		if err == nil && inner == nil {
			joined = append(joined, iface)
		}
	}
	if len(joined) == 0 {
		conn.Close()
		return nil, errors.New("LOCAL_DISCOVERY_UNAVAILABLE")
	}
	life, cancel := context.WithCancel(ctx)
	d := &LANDiscovery{conn: conn, interfaces: joined, manager: manager, known: known, key: key, device: device, pin: pin, candidates: append([]p.NetworkCandidate(nil), candidates...), generation: uint64(time.Now().UnixNano()), ctx: life, cancel: cancel}
	d.wg.Add(2)
	go d.receive()
	go d.announce()
	return d, nil
}
func (d *LANDiscovery) Close() error { d.cancel(); d.conn.Close(); d.wg.Wait(); return nil }

// nextGeneration follows the wall clock so records signed for multicast and for
// peer exchange stay ordered for receivers that hear both.
func (d *LANDiscovery) nextGeneration() uint64 {
	d.genMu.Lock()
	defer d.genMu.Unlock()
	d.generation = max(d.generation+1, uint64(time.Now().UnixNano()))
	return d.generation
}

// sign returns the signed record and its encoding for one interface, or false
// when the interface has no candidates or the record exceeds a datagram.
func (d *LANDiscovery) sign(iface LocalInterface, generation uint64) (p.LANAnnouncement, []byte, bool) {
	var candidates []p.NetworkCandidate
	for _, c := range d.candidates {
		if c.Interface == iface.Name && len(candidates) < 4 {
			candidates = append(candidates, c)
		}
	}
	if len(candidates) == 0 {
		return p.LANAnnouncement{}, nil, false
	}
	a := p.LANAnnouncement{Version: "1", Device: d.device, Pin: d.pin, Key: hex.EncodeToString(d.key.Public().(ed25519.PublicKey)), Generation: p.NetworkUint(generation), Expires: p.NetworkUint(time.Now().Unix() + 600), Capabilities: lanCapabilities(candidates), Candidates: candidates}
	b, err := a.Canonical()
	if err != nil {
		return p.LANAnnouncement{}, nil, false
	}
	a.Signature = hex.EncodeToString(ed25519.Sign(d.key, b))
	b, _ = json.Marshal(a)
	// Drop complete oversized records; never truncate or fragment metadata.
	return a, b, len(b) <= MaxDiscoveryDatagramBytes
}

// Records signs this device's current per-interface LAN records for a peer
// exchange; they are the same records multicast would carry.
func (d *LANDiscovery) Records() []p.LANAnnouncement {
	generation := d.nextGeneration()
	out := []p.LANAnnouncement{}
	for _, iface := range d.interfaces {
		if a, _, ok := d.sign(iface, generation); ok && len(out) < p.LANExchangeMaxRecords {
			out = append(out, a)
		}
	}
	return out
}

// AcceptPeer installs LAN records that an approved peer sent over its pinned
// session. Records must carry that session's pin and a known peer-data
// identity. Each candidate must fall inside a selected local interface prefix
// and must not be one of this device's own addresses (identical container
// bridges on both hosts); the rest are dropped, never dialed.
func (d *LANDiscovery) AcceptPeer(records []p.LANAnnouncement, pin [32]byte) int {
	own := map[netip.Addr]bool{}
	for _, iface := range d.interfaces {
		for _, prefix := range iface.Prefixes {
			own[prefix.Addr()] = true
		}
	}
	type scoped struct {
		candidates []p.NetworkCandidate
		generation uint64
		expires    int64
	}
	installed := 0
	now := uint64(time.Now().Unix())
	byDevice := map[string]map[int]*scoped{}
	for _, a := range records {
		if a.Pin != hex.EncodeToString(pin[:]) || a.Device == d.device || a.Verify(now) != nil {
			continue
		}
		for _, c := range a.Candidates {
			addr, err := netip.ParseAddrPort(c.Address)
			if err != nil || own[addr.Addr()] {
				continue
			}
			for _, iface := range d.interfaces {
				inside := false
				for _, prefix := range iface.Prefixes {
					inside = inside || prefix.Contains(addr.Addr())
				}
				if !inside {
					continue
				}
				if byDevice[a.Device] == nil {
					byDevice[a.Device] = map[int]*scoped{}
				}
				lease := byDevice[a.Device][iface.Index]
				if lease == nil {
					lease = &scoped{expires: int64(a.Expires)}
					byDevice[a.Device][iface.Index] = lease
				}
				lease.generation = max(lease.generation, uint64(a.Generation))
				lease.expires = min(lease.expires, int64(a.Expires))
				duplicate := false
				for _, known := range lease.candidates {
					duplicate = duplicate || (known.Transport == c.Transport && known.Address == c.Address)
				}
				if !duplicate && len(lease.candidates) < MaxPeerLANCandidates {
					local := c
					local.Interface = iface.Name
					lease.candidates = append(lease.candidates, local)
				}
				break
			}
		}
	}
	for _, t := range d.known() {
		if t.Purpose != PeerData || t.Pin != pin {
			continue
		}
		for index, lease := range byDevice[hex.EncodeToString(t.Device[:])] {
			if d.manager.SetPeerLANCandidates(t, lease.candidates, lease.generation, time.Unix(lease.expires, 0), index) == nil {
				installed += len(lease.candidates)
			}
		}
	}
	return installed
}

func (d *LANDiscovery) announce() {
	defer d.wg.Done()
	raw, err := d.conn.SyscallConn()
	if err != nil {
		return
	}
	announcements := 0
	for d.ctx.Err() == nil {
		generation := d.nextGeneration()
		for _, iface := range d.interfaces {
			_, b, ok := d.sign(iface, generation)
			if !ok {
				continue
			}
			var inner error
			err = raw.Control(func(fd uintptr) {
				inner = unix.SetsockoptIPMreqn(int(fd), unix.IPPROTO_IP, unix.IP_MULTICAST_IF, &unix.IPMreqn{Ifindex: int32(iface.Index)})
				if inner == nil {
					inner = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_MULTICAST_TTL, 1)
				}
			})
			if err != nil || inner != nil {
				continue
			}
			_ = d.conn.SetWriteDeadline(time.Now().Add(time.Second))
			_, _ = d.conn.WriteToUDP(b, &net.UDPAddr{IP: net.IPv4(239, 255, 79, 66), Port: DiscoveryPort})
		}
		// Discovery has one finite timer and no per-device goroutines. Frequent
		// first-contact retries are local metadata only, with bounded packet size.
		announcements++
		delay := ReannounceInterval + time.Duration(time.Now().UnixNano()%int64(15*time.Second))
		if announcements < 3 {
			delay = 5 * time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-d.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
func (d *LANDiscovery) receive() {
	defer d.wg.Done()
	buffer := make([]byte, MaxDiscoveryDatagramBytes+1)
	oob := make([]byte, 128)
	window := time.Now()
	count := 0
	for {
		n, on, flags, source, err := d.conn.ReadMsgUDP(buffer, oob)
		if err != nil {
			return
		}
		if time.Since(window) >= time.Second {
			window = time.Now()
			count = 0
		}
		count++
		if count > 20 || n > MaxDiscoveryDatagramBytes || flags&(unix.MSG_TRUNC|unix.MSG_CTRUNC) != 0 {
			continue
		}
		index := 0
		messages, err := unix.ParseSocketControlMessage(oob[:on])
		if err != nil {
			continue
		}
		for _, msg := range messages {
			if msg.Header.Level == unix.IPPROTO_IP && msg.Header.Type == unix.IP_PKTINFO {
				if len(msg.Data) >= 12 {
					index = int(binary.NativeEndian.Uint32(msg.Data[:4]))
				}
			}
		}
		var iface LocalInterface
		for _, i := range d.interfaces {
			if i.Index == index {
				iface = i
			}
		}
		_ = d.Accept(buffer[:n], source, iface)
	}
}

// Accept is also the scoped harness entry point. Unknown identities are discarded
// before signature work and never retained in a nearby-device cache.
func (d *LANDiscovery) Accept(b []byte, source *net.UDPAddr, iface LocalInterface) error {
	if len(b) > MaxDiscoveryDatagramBytes || source == nil || iface.Index == 0 {
		return errors.New("INVALID_LAN_ANNOUNCEMENT")
	}
	var a p.LANAnnouncement
	if err := p.NetworkDecode(b, &a); err != nil {
		return err
	}
	if a.Device == d.device {
		return ErrNoRoute
	}
	var targets []Target
	for _, t := range d.known() {
		if hex.EncodeToString(t.Device[:]) == a.Device && hex.EncodeToString(t.Pin[:]) == a.Pin && t.Purpose == PeerData {
			targets = append(targets, t)
		}
	}
	if len(targets) == 0 {
		return ErrNoRoute
	}
	sourceIP, ok := netip.AddrFromSlice(source.IP)
	if !ok {
		return errors.New("INVALID_SOURCE")
	}
	sourceIP = sourceIP.Unmap()
	contains := func(ip netip.Addr) bool {
		for _, prefix := range iface.Prefixes {
			if prefix.Contains(ip) {
				return true
			}
		}
		return false
	}
	if !sourceIP.IsPrivate() || !contains(sourceIP) {
		return errors.New("INTERFACE_SCOPE_MISMATCH")
	}
	if err := a.Verify(uint64(time.Now().Unix())); err != nil {
		return err
	}
	for _, c := range a.Candidates {
		addr, _ := netip.ParseAddrPort(c.Address)
		if !contains(addr.Addr()) {
			return errors.New("INTERFACE_SCOPE_MISMATCH")
		}
	}
	for _, t := range targets {
		if err := d.manager.SetCandidates(t, a.Candidates, uint64(a.Generation), time.Unix(int64(a.Expires), 0), false, iface.Index); err != nil {
			return err
		}
	}
	return nil
}

// LocalEndpoint prevents local-only invitation input from creating an internet
// dial. The transferred inviter pin is still required by replication before any
// capability disclosure. Discovery itself never approves enrollment.
func LocalEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	ip, err := netip.ParseAddr(u.Hostname())
	if err != nil || !ip.IsPrivate() || !p.AllowedAddress(ip, true) {
		return false
	}
	interfaces, err := SelectedInterfaces(nil)
	if err != nil {
		return false
	}
	for _, iface := range interfaces {
		for _, prefix := range iface.Prefixes {
			if prefix.Contains(ip) {
				return true
			}
		}
	}
	return false
}

// LocalListener filters remote and destination scope before TLS admission. It
// retains the original finite authenticated peer server and owns no authority.
func LocalListener(listener net.Listener, interfaces []LocalInterface) net.Listener {
	return &localListener{Listener: listener, interfaces: interfaces}
}

type localListener struct {
	net.Listener
	mu         sync.RWMutex
	interfaces []LocalInterface
}

func (l *localListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		remoteHost, _, _ := net.SplitHostPort(c.RemoteAddr().String())
		localHost, _, _ := net.SplitHostPort(c.LocalAddr().String())
		remote, e1 := netip.ParseAddr(remoteHost)
		local, e2 := netip.ParseAddr(localHost)
		allowed := false
		if e1 == nil && e2 == nil && remote.IsPrivate() && local.IsPrivate() {
			l.mu.RLock()
			for _, iface := range l.interfaces {
				for _, prefix := range iface.Prefixes {
					if prefix.Addr() == local && prefix.Contains(remote) {
						allowed = true
					}
				}
			}
		}
		if e1 == nil && e2 == nil && remote.IsPrivate() && local.IsPrivate() {
			l.mu.RUnlock()
		}
		if allowed {
			return c, nil
		}
		c.Close()
	}
}

func lanCapabilities(candidates []p.NetworkCandidate) []string {
	caps := []string{"direct_https_v1"}
	for _, c := range candidates {
		if c.Transport == "udp" {
			return append(caps, "quic_http3_v1")
		}
	}
	return caps
}
func OptionalQUICSocket(settings DirectSettings) (net.PacketConn, error) {
	if settings.Disabled || settings.UDPDisabled {
		return nil, nil
	}
	address := settings.UDPListen
	if address == "" {
		address = ":0"
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if host != "" && net.ParseIP(host) == nil {
		return nil, errors.New("INVALID_LISTEN")
	}
	return net.ListenPacket("udp", address)
}

// CombineDirectCandidates keeps both transports represented within the existing
// finite advertisement budget, including the four-candidate LAN record limit.
func CombineDirectCandidates(tcp, udp []p.NetworkCandidate) []p.NetworkCandidate {
	var out []p.NetworkCandidate
	for i := 0; len(out) < p.NetworkMaxCandidates && (i < len(tcp) || i < len(udp)); i++ {
		if i < len(udp) {
			out = append(out, udp[i])
		}
		if i < len(tcp) && len(out) < p.NetworkMaxCandidates {
			out = append(out, tcp[i])
		}
	}
	return out
}

type localPacketConn struct {
	net.PacketConn
	interfaces []LocalInterface
}

func LocalPacketConn(c net.PacketConn, interfaces []LocalInterface) net.PacketConn {
	return &localPacketConn{c, interfaces}
}
func (c *localPacketConn) ReadFrom(b []byte) (int, net.Addr, error) {
	for {
		n, a, err := c.PacketConn.ReadFrom(b)
		if err != nil {
			return n, a, err
		}
		host, _, _ := net.SplitHostPort(a.String())
		localHost, _, _ := net.SplitHostPort(c.LocalAddr().String())
		local, _ := netip.ParseAddr(localHost)
		remote, e := netip.ParseAddr(host)
		if e == nil && remote.IsPrivate() {
			for _, iface := range c.interfaces {
				for _, prefix := range iface.Prefixes {
					if prefix.Addr() == local && prefix.Contains(remote) {
						return n, a, nil
					}
				}
			}
		}
	}
}

// Local-only sockets bind a selected concrete address, so filtering never relies
// on a wildcard destination or on a sender's interface claim.
func OptionalLocalQUICSocket(settings DirectSettings, interfaces []LocalInterface) (net.PacketConn, error) {
	if settings.Disabled || settings.UDPDisabled {
		return nil, nil
	}
	address := settings.UDPListen
	if address == "" {
		address = ":0"
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	requested := net.ParseIP(host)
	for _, iface := range interfaces {
		for _, prefix := range iface.Prefixes {
			ip := prefix.Addr()
			if host == "" || (requested != nil && requested.IsUnspecified()) || host == ip.String() {
				settings.UDPListen = net.JoinHostPort(ip.String(), port)
				c, err := OptionalQUICSocket(settings)
				if c == nil || err != nil {
					return c, err
				}
				return LocalPacketConn(c, interfaces), nil
			}
		}
	}
	return nil, ErrNoRoute
}

// RefreshLocalListener replaces the allowed interface scope after roaming.
func RefreshLocalListener(listener net.Listener, interfaces []LocalInterface) {
	if local, ok := listener.(*localListener); ok {
		local.mu.Lock()
		local.interfaces = interfaces
		local.mu.Unlock()
	}
}
