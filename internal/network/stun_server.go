package network

import (
	"errors"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/stun/v4"
)

// STUNServer is a separate UDP service, never a peer/control HTTP handler.
// It returns only XOR-MAPPED-ADDRESS; no alternate servers or arbitrary targets.
type STUNServer struct {
	socket net.PacketConn
	done   chan struct{}
	once   sync.Once
	stats  STUNStats
}

// STUNStats are sanitized totals; no source address or prefix is retained.
type STUNStats struct {
	Answered, RateLimited, Invalid, TableFull atomic.Uint64
}

// Stats returns a snapshot of answered and dropped request totals.
func (s *STUNServer) Stats() (answered, rateLimited, invalid, tableFull uint64) {
	return s.stats.Answered.Load(), s.stats.RateLimited.Load(), s.stats.Invalid.Load(), s.stats.TableFull.Load()
}

type stunBucket struct {
	second int64
	count  int
}

func NewSTUNServer(socket net.PacketConn) (*STUNServer, error) {
	if socket == nil {
		return nil, errors.New("INVALID_STUN_SOCKET")
	}
	if udp, ok := socket.(*net.UDPConn); ok {
		if err := udp.SetReadBuffer(64 << 10); err != nil {
			return nil, err
		}
	}
	s := &STUNServer{socket: socket, done: make(chan struct{})}
	go s.run()
	return s, nil
}
func (s *STUNServer) run() {
	defer close(s.done)
	// All handling is sequential, with a bounded source table and no per-packet
	// goroutine or timer. Unknown sources are refused when the table is full.
	sources := map[netip.Prefix]stunBucket{}
	global := stunBucket{}
	lastSweep := int64(0)
	buffer := make([]byte, 1024)
	for {
		n, from, err := s.socket.ReadFrom(buffer)
		if err != nil {
			return
		}
		addr, err := netip.ParseAddrPort(from.String())
		if err != nil || n < 20 || n > 128 || !addr.Addr().IsGlobalUnicast() {
			s.stats.Invalid.Add(1)
			continue
		}
		now := time.Now().Unix()
		if now-lastSweep >= 60 {
			for key, b := range sources {
				if now-b.second >= 60 {
					delete(sources, key)
				}
			}
			lastSweep = now
		}
		bits := 64
		if addr.Addr().Is4() {
			bits = 24
		}
		prefix := netip.PrefixFrom(addr.Addr(), bits).Masked()
		b, known := sources[prefix]
		if !known && len(sources) >= 1024 {
			s.stats.TableFull.Add(1)
			continue
		}
		if b.second != now {
			b = stunBucket{second: now}
		}
		if global.second != now {
			global = stunBucket{second: now}
		}
		if b.count >= 10 || global.count >= 200 {
			s.stats.RateLimited.Add(1)
			continue
		}
		b.count++
		global.count++
		sources[prefix] = b
		message := &stun.Message{Raw: buffer[:n]}
		if message.Decode() != nil || message.Type != stun.BindingRequest || (len(message.Attributes) != 0 && !(len(message.Attributes) == 1 && message.Attributes[0].Type == stun.AttrFingerprint && stun.Fingerprint.Check(message) == nil)) {
			s.stats.Invalid.Add(1)
			continue
		}
		response, err := stun.Build(stun.NewTransactionIDSetter(message.TransactionID), stun.BindingSuccess, &stun.XORMappedAddress{IP: net.IP(addr.Addr().AsSlice()), Port: int(addr.Port())})
		// IPv4 response is 32 bytes; IPv6 is 44. Minimal requests are 20
		// bytes: hard amplification cap three, with no variable-size attributes.
		if err != nil || len(response.Raw) > 3*n || len(response.Raw) > 128 {
			s.stats.Invalid.Add(1)
			continue
		}
		_ = s.socket.SetWriteDeadline(time.Now().Add(time.Second))
		if _, err = s.socket.WriteTo(response.Raw, from); err == nil {
			s.stats.Answered.Add(1)
		}
	}
}
func (s *STUNServer) Close() error        { s.once.Do(func() { _ = s.socket.Close(); <-s.done }); return nil }
func (s *STUNServer) LocalAddr() net.Addr { return s.socket.LocalAddr() }
