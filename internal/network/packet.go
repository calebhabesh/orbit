package network

import (
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"
)

const QUICPacketMTU = 1200

var ErrDatagramSize = errors.New("DATAGRAM_SIZE")

// PacketPair supplies complete datagrams, never a byte stream. Pion's established
// ICE Conn implements this interface; W10 owns establishing and replacing pairs.
type PacketPair interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	SetDeadline(time.Time) error
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}

// PairPacketConn freezes numeric addresses for a generation. Only quic.Transport
// reads this adapter. A full-size scratch read detects truncation before copying.
type PairPacketConn struct {
	pair          PacketPair
	local, remote net.UDPAddr
	readMu        sync.Mutex
	buffer        [65536]byte
}

func NewPairPacketConn(pair PacketPair) (*PairPacketConn, error) {
	if pair == nil || pair.LocalAddr() == nil || pair.RemoteAddr() == nil {
		return nil, ErrNoRoute
	}
	localAddress, e := netip.ParseAddrPort(pair.LocalAddr().String())
	if e != nil || localAddress.Port() == 0 {
		return nil, ErrNoRoute
	}
	remoteAddress, e := netip.ParseAddrPort(pair.RemoteAddr().String())
	if e != nil || remoteAddress.Port() == 0 {
		return nil, ErrNoRoute
	}
	local, remote := net.UDPAddrFromAddrPort(localAddress), net.UDPAddrFromAddrPort(remoteAddress)
	return &PairPacketConn{pair: pair, local: *local, remote: *remote}, nil
}
func (c *PairPacketConn) current() bool {
	local, remote := c.pair.LocalAddr(), c.pair.RemoteAddr()
	return local != nil && remote != nil && local.String() == c.local.String() && remote.String() == c.remote.String()
}
func (c *PairPacketConn) ReadFrom(b []byte) (int, net.Addr, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if !c.current() {
		return 0, nil, ErrStale
	}
	n, e := c.pair.Read(c.buffer[:])
	if e != nil {
		return 0, nil, e
	}
	if !c.current() {
		return 0, nil, ErrStale
	}
	if n > QUICPacketMTU || n > len(b) {
		return 0, nil, ErrDatagramSize
	}
	copy(b, c.buffer[:n])
	return n, c.remoteAddr(), nil
}
func (c *PairPacketConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if !c.current() {
		return 0, ErrStale
	}
	if addr == nil || addr.String() != c.remote.String() {
		return 0, ErrNoRoute
	}
	if len(b) > QUICPacketMTU {
		return 0, ErrDatagramSize
	}
	n, e := c.pair.Write(b)
	if e == nil && n != len(b) {
		e = io.ErrShortWrite
	}
	return n, e
}
func (c *PairPacketConn) remoteAddr() net.Addr {
	a := c.remote
	a.IP = append(net.IP(nil), a.IP...)
	return &a
}
func (c *PairPacketConn) LocalAddr() net.Addr {
	a := c.local
	a.IP = append(net.IP(nil), a.IP...)
	return &a
}
func (c *PairPacketConn) Close() error                       { return c.pair.Close() }
func (c *PairPacketConn) SetDeadline(t time.Time) error      { return c.pair.SetDeadline(t) }
func (c *PairPacketConn) SetReadDeadline(t time.Time) error  { return c.pair.SetReadDeadline(t) }
func (c *PairPacketConn) SetWriteDeadline(t time.Time) error { return c.pair.SetWriteDeadline(t) }
