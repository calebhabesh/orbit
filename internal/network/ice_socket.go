package network

import (
	"errors"
	"net"
	"net/netip"
	"sync"
	"time"

	p "github.com/calebhabesh/orbit/internal/protocol"
	"github.com/pion/stun/v4"
	"github.com/pion/transport/v5"
)

// iceNet keeps Pion the sole raw socket reader, but bounds kernel buffers and
// source addresses before peer-reflexive candidate allocation inside Pion.
type iceNet struct {
	transport.Net
	servers map[string]bool
}

func (n *iceNet) ListenUDP(kind string, addr *net.UDPAddr) (transport.UDPConn, error) {
	c, e := n.Net.ListenUDP(kind, addr)
	if e != nil {
		return nil, e
	}
	if e = c.SetReadBuffer(64 << 10); e != nil && !errors.Is(e, transport.ErrNotSupported) {
		_ = c.Close()
		return nil, e
	}
	if e = c.SetWriteBuffer(64 << 10); e != nil && !errors.Is(e, transport.ErrNotSupported) {
		_ = c.Close()
		return nil, e
	}
	return &iceSocket{UDPConn: c, servers: n.servers, sources: map[string]bool{}}, nil
}

type iceSocket struct {
	transport.UDPConn
	mu      sync.Mutex
	servers map[string]bool
	sources map[string]bool
	second  int64
	checks  int
}

func (c *iceSocket) allowed(b []byte, addr net.Addr) bool {
	if addr == nil || len(b) > QUICPacketMTU {
		return false
	}
	a, e := netip.ParseAddrPort(addr.String())
	if e != nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.servers[addr.String()] {
		if !p.AllowedAddress(a.Addr(), false) {
			return false
		}
		if !c.sources[addr.String()] {
			if len(c.sources) >= 32 {
				return false
			}
			c.sources[addr.String()] = true
		}
	}
	if stun.IsMessage(b) {
		now := time.Now().Unix()
		if now != c.second {
			c.second = now
			c.checks = 0
		}
		if c.checks >= 100 {
			return false
		}
		c.checks++
	}
	return true
}
func (c *iceSocket) ReadFrom(b []byte) (int, net.Addr, error) {
	for {
		n, a, e := c.UDPConn.ReadFrom(b)
		if e != nil {
			return n, a, e
		}
		if c.allowed(b[:n], a) {
			return n, a, nil
		}
	}
}
func (c *iceSocket) ReadFromUDP(b []byte) (int, *net.UDPAddr, error) {
	for {
		n, a, e := c.UDPConn.ReadFromUDP(b)
		if e != nil {
			return n, a, e
		}
		if c.allowed(b[:n], a) {
			return n, a, nil
		}
	}
}
func (c *iceSocket) ReadMsgUDP(b, oob []byte) (int, int, int, *net.UDPAddr, error) {
	for {
		n, on, flags, a, e := c.UDPConn.ReadMsgUDP(b, oob)
		if e != nil {
			return n, on, flags, a, e
		}
		if c.allowed(b[:n], a) {
			return n, on, flags, a, nil
		}
	}
}
