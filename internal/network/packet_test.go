package network

import (
	"bytes"
	"errors"
	ice "github.com/pion/ice/v4"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

// Compile the selected Pion API against the W09 adapter contract.
var _ PacketPair = (*ice.Conn)(nil)

type udpPairFixture struct {
	*net.UDPConn
	mu     sync.Mutex
	remote *net.UDPAddr
}

func (p *udpPairFixture) RemoteAddr() net.Addr {
	p.mu.Lock()
	defer p.mu.Unlock()
	a := *p.remote
	return &a
}
func (p *udpPairFixture) Read(b []byte) (int, error) {
	n, _, e := p.UDPConn.ReadFromUDP(b)
	return n, e
}
func (p *udpPairFixture) Write(b []byte) (int, error) {
	p.mu.Lock()
	a := *p.remote
	p.mu.Unlock()
	return p.UDPConn.WriteToUDP(b, &a)
}
func pairPackets(t *testing.T) (*PairPacketConn, *PairPacketConn, *udpPairFixture) {
	t.Helper()
	a, e := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if e != nil {
		t.Fatal(e)
	}
	b, e := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if e != nil {
		t.Fatal(e)
	}
	pa := &udpPairFixture{UDPConn: a, remote: b.LocalAddr().(*net.UDPAddr)}
	pb := &udpPairFixture{UDPConn: b, remote: a.LocalAddr().(*net.UDPAddr)}
	ca, e := NewPairPacketConn(pa)
	if e != nil {
		t.Fatal(e)
	}
	cb, e := NewPairPacketConn(pb)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { ca.Close(); cb.Close() })
	return ca, cb, pa
}
func TestWANW09PacketBoundariesMTUAddressesAndPairChange(t *testing.T) {
	a, b, p := pairPackets(t)
	for _, payload := range [][]byte{[]byte("one"), []byte("two"), bytes.Repeat([]byte{42}, QUICPacketMTU)} {
		if _, e := a.WriteTo(payload, a.remoteAddr()); e != nil {
			t.Fatal(e)
		}
		buf := make([]byte, QUICPacketMTU)
		n, addr, e := b.ReadFrom(buf)
		if e != nil || !bytes.Equal(payload, buf[:n]) || addr.String() != a.LocalAddr().String() {
			t.Fatal(n, addr, e)
		}
	}
	if _, e := a.WriteTo(make([]byte, QUICPacketMTU+1), a.remoteAddr()); !errors.Is(e, ErrDatagramSize) {
		t.Fatal(e)
	}
	if _, e := a.WriteTo([]byte("x"), a.LocalAddr()); !errors.Is(e, ErrNoRoute) {
		t.Fatal(e)
	}
	a.WriteTo([]byte("too big for destination buffer"), a.remoteAddr())
	if n, _, e := b.ReadFrom(make([]byte, 2)); n != 0 || !errors.Is(e, ErrDatagramSize) {
		t.Fatal("truncation", n, e)
	}
	// Bypass outbound guard only in the synthetic fixture to test oversized input.
	p.Write(make([]byte, QUICPacketMTU+1))
	if n, _, e := b.ReadFrom(make([]byte, 2000)); n != 0 || !errors.Is(e, ErrDatagramSize) {
		t.Fatal("MTU input", n, e)
	}
	p.mu.Lock()
	p.remote = &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}
	p.mu.Unlock()
	if _, e := a.WriteTo([]byte("x"), a.remoteAddr()); !errors.Is(e, ErrStale) {
		t.Fatal("pair rebuild required", e)
	}
	frozen := a.LocalAddr().(*net.UDPAddr)
	frozen.Port = 1
	if a.LocalAddr().(*net.UDPAddr).Port == 1 {
		t.Fatal("mutable frozen address")
	}
}
func TestWANW09PacketDeadlineCancelClose(t *testing.T) {
	a, b, _ := pairPackets(t)
	a.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
	if _, _, e := a.ReadFrom(make([]byte, 1200)); !errors.Is(e, os.ErrDeadlineExceeded) {
		t.Fatal("read deadline", e)
	}
	a.SetReadDeadline(time.Time{})
	b.WriteTo([]byte("resumed"), b.remoteAddr())
	if _, _, e := a.ReadFrom(make([]byte, 1200)); e != nil {
		t.Fatal("reversible deadline", e)
	}
	a.SetWriteDeadline(time.Now().Add(-time.Second))
	if _, e := a.WriteTo([]byte("x"), a.remoteAddr()); !errors.Is(e, os.ErrDeadlineExceeded) {
		t.Fatal("write deadline", e)
	}
	a.SetDeadline(time.Time{})
	done := make(chan error, 1)
	go func() { _, _, e := a.ReadFrom(make([]byte, 1200)); done <- e }()
	a.Close()
	select {
	case e := <-done:
		if !errors.Is(e, net.ErrClosed) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("close did not join read")
	}
}
