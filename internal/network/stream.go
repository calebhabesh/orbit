package network

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const StreamBufferBytes = 32 << 10

// BinaryStream restores a finite frame limit AFTER NetConn, which otherwise
// disables it. Writes split into bounded messages; reads use one fixed frame
// buffer and never allocate from a caller-supplied message size. Compression must be disabled at Dial/Accept.
func BinaryStream(ctx context.Context, ws *websocket.Conn) net.Conn {
	c := websocket.NetConn(ctx, ws, websocket.MessageBinary)
	ws.SetReadLimit(StreamBufferBytes)
	s := &binaryStream{Conn: c, ws: ws, ctx: ctx, reads: make(chan streamRead), ack: make(chan struct{}), pumpDone: make(chan struct{}), done: make(chan struct{}), changed: make(chan struct{}), failed: make(chan struct{})}
	s.stopMu.Lock()
	s.stop = context.AfterFunc(ctx, func() { _ = s.Close() })
	s.stopMu.Unlock()
	go s.readPump()
	return s
}

type streamRead struct {
	data []byte
	err  error
}
type binaryStream struct {
	failed      chan struct{}
	failOnce    sync.Once
	ctx         context.Context
	pumpDone    chan struct{}
	stop        func() bool
	stopMu      sync.Mutex
	terminalErr error
	writeMu     sync.Mutex
	net.Conn
	ws           *websocket.Conn
	reads        chan streamRead
	ack          chan struct{}
	done         chan struct{}
	once         sync.Once
	readMu       sync.Mutex
	pending      []byte
	pendingErr   error
	deadlineMu   sync.Mutex
	readDeadline time.Time
	changed      chan struct{}
}

// One reusable read buffer: the pump cannot refill it until the caller consumes
// its final byte. HTTP's temporary past read deadline never cancels the actual
// WebSocket Reader; cancel/Close still closes the connection and pump.
func (s *binaryStream) readPump() {
	defer close(s.pumpDone)
	buf := make([]byte, StreamBufferBytes+1)
	for {
		typ, r, e := s.ws.Reader(s.ctx)
		n := 0
		if e == nil {
			if typ != websocket.MessageBinary {
				e = errors.New("INVALID_FRAME_TYPE")
			} else {
				n, e = io.ReadFull(r, buf)
				if n > StreamBufferBytes {
					n = 0
					e = errors.New("FRAME_TOO_LARGE")
				} else if errors.Is(e, io.EOF) || errors.Is(e, io.ErrUnexpectedEOF) {
					e = nil
				}
			}
		}
		if e != nil {
			s.failOnce.Do(func() { close(s.failed) })
			_ = s.ws.CloseNow()
		}
		if e == nil && n == 0 {
			continue
		}
		select {
		case s.reads <- streamRead{buf[:n], e}:
		case <-s.done:
			return
		}
		select {
		case <-s.ack:
		case <-s.done:
			return
		}
		if e != nil {
			return
		}
	}
}
func (s *binaryStream) Read(p []byte) (int, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	if len(p) == 0 {
		return 0, nil
	}
	for {
		if s.terminalErr != nil {
			return 0, s.terminalErr
		}
		s.deadlineMu.Lock()
		deadline, changed := s.readDeadline, s.changed
		s.deadlineMu.Unlock()
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return 0, os.ErrDeadlineExceeded
		}
		if len(s.pending) > 0 || s.pendingErr != nil {
			n := copy(p, s.pending)
			s.pending = s.pending[n:]
			var e error
			if len(s.pending) == 0 {
				e = s.pendingErr
				s.terminalErr = s.pendingErr
				s.pendingErr = nil
				select {
				case s.ack <- struct{}{}:
				case <-s.done:
				}
			}
			return n, e
		}
		var timer *time.Timer
		var timeout <-chan time.Time
		if !deadline.IsZero() {
			timer = time.NewTimer(time.Until(deadline))
			timeout = timer.C
		}
		select {
		case r := <-s.reads:
			s.pending = r.data
			s.pendingErr = r.err
		case <-changed:
		case <-timeout:
			if timer != nil {
				timer.Stop()
			}
			return 0, os.ErrDeadlineExceeded
		case <-s.done:
			if timer != nil {
				timer.Stop()
			}
			return 0, net.ErrClosed
		}
		if timer != nil {
			timer.Stop()
		}
	}
}
func (s *binaryStream) SetReadDeadline(t time.Time) error {
	s.deadlineMu.Lock()
	defer s.deadlineMu.Unlock()
	s.readDeadline = t
	close(s.changed)
	s.changed = make(chan struct{})
	return nil
}
func (s *binaryStream) SetDeadline(t time.Time) error {
	_ = s.SetReadDeadline(t)
	return s.Conn.SetWriteDeadline(t)
}
func (s *binaryStream) Write(p []byte) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n := 0
	for len(p) > 0 {
		size := min(len(p), StreamBufferBytes)
		m, err := s.Conn.Write(p[:size])
		n += m
		if err != nil {
			return n, err
		}
		if m != size {
			return n, io.ErrShortWrite
		}
		p = p[size:]
	}
	return n, nil
}
func (s *binaryStream) Close() error {
	s.once.Do(func() {
		close(s.done)
		s.failOnce.Do(func() { close(s.failed) })
		s.stopMu.Lock()
		if s.stop != nil {
			s.stop()
		}
		s.stopMu.Unlock()
		_ = s.ws.CloseNow()
		_ = s.Conn.Close()
	})
	<-s.pumpDone
	return nil
}

// Forward streams two already-admitted legs. It never dials a URL or interprets
// payload bytes. Two fixed buffers and socket backpressure bound userspace data.
// Idle deadlines apply independently to reads/writes; context/one-leg failure
// closes both legs and joins both pumps before return. Quotas/admission are W04.
func Forward(ctx context.Context, a, b net.Conn, idle time.Duration) error {
	if idle <= 0 {
		return errors.New("INVALID_IDLE_LIMIT")
	}
	defer a.Close()
	defer b.Close()
	stop := context.AfterFunc(ctx, func() { _ = a.Close(); _ = b.Close() })
	defer stop()
	done := make(chan error, 2)
	pump := func(dst, src net.Conn) {
		buf := make([]byte, StreamBufferBytes)
		for {
			_ = src.SetReadDeadline(time.Now().Add(idle))
			n, err := src.Read(buf)
			if n > 0 {
				_ = dst.SetWriteDeadline(time.Now().Add(idle))
				m, e := dst.Write(buf[:n])
				if e != nil {
					done <- e
					return
				}
				if m != n {
					done <- io.ErrShortWrite
					return
				}
			}
			if err != nil {
				done <- err
				return
			}
		}
	}
	go pump(a, b)
	go pump(b, a)
	err := <-done
	_ = a.Close()
	_ = b.Close()
	<-done
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

// StreamListener feeds only the selected peer OR enrollment HTTP server. A
// manager must supply a separately owned listener for each purpose.
type StreamListener struct {
	queue chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func NewStreamListener() *StreamListener {
	return &StreamListener{queue: make(chan net.Conn), done: make(chan struct{})}
}
func (l *StreamListener) Offer(ctx context.Context, c net.Conn) error {
	select {
	case <-l.done:
		_ = c.Close()
		return net.ErrClosed
	default:
	}
	select {
	case l.queue <- listenerConn{c}:
		return nil
	case <-ctx.Done():
		_ = c.Close()
		return ctx.Err()
	case <-l.done:
		_ = c.Close()
		return net.ErrClosed
	}
}
func (l *StreamListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.queue:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}
func (l *StreamListener) Close() error   { l.once.Do(func() { close(l.done) }); return nil }
func (l *StreamListener) Addr() net.Addr { return streamAddr{} }

type streamAddr struct{}

func (streamAddr) Network() string { return "orbit-stream" }
func (streamAddr) String() string  { return "orbit-relay:0" }

// All relay enrollment legs share a conservative logical address bucket. Never
// present an untrusted forwarded address as the end-device IP. W05 adds per-key
// admission while retaining global limits.
type listenerConn struct{ net.Conn }

func (listenerConn) RemoteAddr() net.Addr { return streamAddr{} }

// Failed signals terminal socket/frame failure, including before a partner arrives.
func (s *binaryStream) Failed() <-chan struct{} { return s.failed }
