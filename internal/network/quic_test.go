package network

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

func testQUICEndpoint(t *testing.T, h http.Handler) (*QUICEndpoint, *tls.Config) {
	t.Helper()
	s, trust, _ := managerServer(t, func(http.ResponseWriter, *http.Request) {})
	t.Cleanup(s.Close)
	server := s.TLS.Clone()
	server.ClientAuth = tls.RequireAnyClientCert
	trust.Certificates = server.Certificates
	trust.NextProtos = []string{"h3"}
	socket, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	e, err := NewQUICEndpoint(socket, server, h)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	return e, trust
}
func testQUICDial(t *testing.T, e *QUICEndpoint, trust *tls.Config) *quic.Conn {
	t.Helper()
	socket, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	transport := &quic.Transport{Conn: socket}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := transport.Dial(ctx, e.LocalAddr(), trust, peerQUICConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.CloseWithError(0, ""); transport.Close(); socket.Close() })
	return conn
}
func TestWANW09HTTP3HeaderAndBodyDeadlines(t *testing.T) {
	var handled atomic.Int32
	e, trust := testQUICEndpoint(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handled.Add(1)
		_, err := io.Copy(io.Discard, r.Body)
		if err != nil {
			http.Error(w, "body timeout", 408)
		}
	}))
	conn := testQUICDial(t, e, trust)
	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// A HEADERS frame with no length cannot reach the HTTP handler.
	start := time.Now()
	stream.Write([]byte{1})
	stream.SetReadDeadline(time.Now().Add(7 * time.Second))
	_, err = stream.Read(make([]byte, 128))
	if err == nil || time.Since(start) > 6*time.Second || handled.Load() != 0 {
		t.Fatal("preparse header deadline", err, time.Since(start), handled.Load())
	}
	client := (&http3.Transport{MaxResponseHeaderBytes: 16 << 10}).NewClientConn(conn)
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	req, _ := http.NewRequest("POST", "https://example.com/", reader)
	start = time.Now()
	res, err := client.RoundTrip(req)
	if err != nil {
		t.Fatal("body response", err)
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != 408 || time.Since(start) > 17*time.Second {
		t.Fatal("body deadline", res.StatusCode, time.Since(start))
	}
	t.Log("actual header and body bounds", 5*time.Second, 15*time.Second)
}
func TestWANW09HTTP3StreamingMemoryCancelAndJoinedClose(t *testing.T) {
	var sent atomic.Int64
	e, trust := testQUICEndpoint(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buffer := bytes.Repeat([]byte{42}, 32<<10)
		for i := 0; i < 1024; i++ {
			n, err := w.Write(buffer)
			sent.Add(int64(n))
			if err != nil {
				return
			}
		}
	}))
	runtime.GC()
	var before, during runtime.MemStats
	runtime.ReadMemStats(&before)
	conn := testQUICDial(t, e, trust)
	client := (&http3.Transport{MaxResponseHeaderBytes: 16 << 10}).NewClientConn(conn)
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://example.com/", nil)
	res, err := client.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	// Slow reader only consumes a prefix; receive flow-control stays fixed while
	// the server attempts a 32-MiB response with one 32-KiB copy buffer.
	prefix := make([]byte, 64<<10)
	if _, err = io.ReadFull(res.Body, prefix); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	runtime.ReadMemStats(&during)
	if sent.Load() >= 32<<20 {
		t.Fatal("stream bypassed backpressure", sent.Load())
	}
	if during.HeapAlloc > before.HeapAlloc+12<<20 {
		t.Fatal("unbounded streaming heap", before.HeapAlloc, during.HeapAlloc)
	}
	cancel()
	if _, err = res.Body.Read(make([]byte, 1024)); err == nil { // buffered bytes may drain before cancellation surfaces
		_, err = io.Copy(io.Discard, res.Body)
	}
	res.Body.Close()
	if err == nil {
		t.Fatal("request cancellation ignored")
	}
	done := make(chan error, 1)
	go func() { done <- e.Close() }()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("endpoint workers did not join")
	}
	t.Logf("32-MiB offered; sent before cancel=%d; heap before/during=%d/%d; goroutines=%d", sent.Load(), before.HeapAlloc, during.HeapAlloc, runtime.NumGoroutine())
	if !errors.Is(conn.Context().Err(), context.Canceled) { // peer close may be asynchronous; local close completes ownership
		conn.CloseWithError(0, "")
	}
}
func TestWANW09OptionalUDPDisabledAndCollision(t *testing.T) {
	c, err := OptionalQUICSocket(DirectSettings{UDPDisabled: true})
	if c != nil || err != nil {
		t.Fatal(c, err)
	}
	occupied, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	c, err = OptionalQUICSocket(DirectSettings{UDPListen: occupied.LocalAddr().String()})
	if err == nil || c != nil {
		t.Fatal("occupied UDP must fail only its optional route")
	}
}

func TestWANW09QUICAdmissionAndALPN(t *testing.T) {
	baseline := runtime.NumGoroutine()
	e, trust := testQUICEndpoint(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	socket, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	transport := &quic.Transport{Conn: socket}
	defer transport.Close()
	defer socket.Close()
	var conns []*quic.Conn
	for i := 0; i < MaxActivePeerSlots; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		c, err := transport.Dial(ctx, e.LocalAddr(), trust, peerQUICConfig())
		cancel()
		if err != nil {
			t.Fatal(i, err)
		}
		conns = append(conns, c)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	extra, err := transport.Dial(ctx, e.LocalAddr(), trust, peerQUICConfig())
	cancel()
	if extra != nil {
		extra.CloseWithError(0, "")
	}
	if err == nil {
		t.Fatal("33rd incoming session admitted")
	}
	if len(e.slots) > MaxActivePeerSlots {
		t.Fatal("admission cap", len(e.slots))
	}
	for _, c := range conns {
		c.CloseWithError(0, "")
	}
	deadline := time.Now().Add(time.Second)
	for len(e.slots) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(e.slots) != 0 {
		t.Fatal("session slots leaked", len(e.slots))
	}
	cfg := trust.Clone()
	cfg.NextProtos = []string{"http/1.1"}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	_, err = transport.Dial(ctx, e.LocalAddr(), cfg, peerQUICConfig())
	cancel()
	if err == nil {
		t.Fatal("non-H3 ALPN accepted")
	}
	e.Close()
	transport.Close()
	socket.Close()
	deadline = time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > baseline+2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	t.Logf("incoming cap=%d; goroutines before/after=%d/%d", MaxActivePeerSlots, baseline, runtime.NumGoroutine())
	if runtime.NumGoroutine() > baseline+2 {
		t.Fatal("QUIC workers remain after close")
	}
}

func TestWANW09HTTP3ResponseHeaderAndWriteBounds(t *testing.T) {
	type writeResult struct {
		elapsed time.Duration
		err     error
	}
	written := make(chan writeResult, 1)
	e, trust := testQUICEndpoint(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/headers":
			w.Header().Set("X-Oversized", string(bytes.Repeat([]byte{42}, 17<<10)))
			w.Write([]byte("bounded"))
		case "/stall":
			<-r.Context().Done()
		case "/write":
			start := time.Now()
			buffer := bytes.Repeat([]byte{42}, 32<<10)
			for i := 0; i < 1024; i++ {
				if _, err := w.Write(buffer); err != nil {
					written <- writeResult{time.Since(start), err}
					return
				}
			}
			written <- writeResult{time.Since(start), nil}
		}
	}))
	conn := testQUICDial(t, e, trust)
	client := (&http3.Transport{MaxResponseHeaderBytes: 16 << 10}).NewClientConn(conn)
	req, _ := http.NewRequest("POST", "https://example.com/headers", nil)
	if res, err := client.RoundTrip(req); err == nil {
		res.Body.Close()
		t.Fatal("16-KiB response header cap ignored")
	}
	pool := &quicPool{conn: conn, client: client}
	req, _ = http.NewRequest("POST", "https://example.com/stall", nil)
	start := time.Now()
	res, selected, err := pool.roundTrip(req)
	if res != nil {
		res.Body.Close()
	}
	if !selected || err == nil || time.Since(start) < 14*time.Second || time.Since(start) > 17*time.Second {
		t.Fatal("response header timeout", err, time.Since(start))
	}
	req, _ = http.NewRequest("POST", "https://example.com/write", nil)
	res, err = client.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	select {
	case result := <-written:
		var timeout net.Error
		if result.elapsed < 29*time.Second || result.elapsed > 32*time.Second || !errors.As(result.err, &timeout) || !timeout.Timeout() {
			t.Fatal("HTTP3 response write deadline", result.elapsed, result.err)
		}
		t.Log("16-KiB response header refusal; actual 15-second response-header and 30-second response-write deadlines", result.elapsed)
	case <-time.After(33 * time.Second):
		t.Fatal("response write did not reach its finite deadline")
	}
}
