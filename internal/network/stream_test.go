package network

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func streamPair(t *testing.T) (net.Conn, net.Conn, *websocket.Conn, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	accepted := make(chan net.Conn, 1)
	h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" {
			http.Error(w, "browser forbidden", 403)
			return
		}
		ws, e := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
		if e != nil {
			return
		}
		accepted <- BinaryStream(ctx, ws)
	}))
	ws, _, e := websocket.Dial(ctx, "wss"+h.URL[5:], &websocket.DialOptions{HTTPClient: h.Client(), CompressionMode: websocket.CompressionDisabled})
	if e != nil {
		t.Fatal(e)
	}
	a := BinaryStream(ctx, ws)
	b := <-accepted
	t.Cleanup(func() { cancel(); a.Close(); b.Close(); h.Close() })
	return a, b, ws, cancel
}
func TestWANW01StreamBoundsAndDeadlines(t *testing.T) {
	a, b, _, _ := streamPair(t)
	// HTTP's aborted background read can be reset without killing the TLS stream.
	if e := b.SetReadDeadline(time.Now().Add(-time.Second)); e != nil {
		t.Fatal(e)
	}
	if _, e := b.Read(make([]byte, 1)); !errors.Is(e, os.ErrDeadlineExceeded) {
		t.Fatalf("deadline %v", e)
	}
	b.SetReadDeadline(time.Time{})
	data := bytes.Repeat([]byte("x"), StreamBufferBytes*3+1)
	done := make(chan error, 1)
	go func() { _, e := a.Write(data); done <- e }()
	got := make([]byte, len(data))
	if _, e := io.ReadFull(b, got); e != nil || !bytes.Equal(got, data) {
		t.Fatalf("split writes: %v", e)
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}
func TestWANW01StreamInvalidFrames(t *testing.T) {
	for _, test := range []struct {
		name  string
		typ   websocket.MessageType
		n     int
		valid bool
	}{{"maximum", websocket.MessageBinary, StreamBufferBytes, true}, {"one-over", websocket.MessageBinary, StreamBufferBytes + 1, false}, {"text", websocket.MessageText, 1, false}} {
		t.Run(test.name, func(t *testing.T) {
			_, b, ws, _ := streamPair(t)
			done := make(chan error, 1)
			go func() { done <- ws.Write(context.Background(), test.typ, bytes.Repeat([]byte("x"), test.n)) }()
			b.SetReadDeadline(time.Now().Add(time.Second))
			_, e := io.ReadAll(io.LimitReader(b, int64(test.n)))
			if test.valid && e != nil {
				t.Fatal(e)
			}
			if !test.valid && e == nil {
				t.Fatal("invalid frame accepted")
			}
			b.Close()
			<-done
		})
	}
}
func TestWANW01WSSSlowReceiverCancellation(t *testing.T) {
	a, _, _, cancel := streamPair(t)
	var writes atomic.Int64
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, StreamBufferBytes)
		for i := 0; i < 4096; i++ {
			if _, e := a.Write(buf); e != nil {
				done <- e
				return
			}
			writes.Add(1)
		}
		done <- nil
	}()
	time.Sleep(100 * time.Millisecond)
	if writes.Load() == 4096 {
		t.Fatal("128 MiB buffered without receiver")
	}
	cancel()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("canceled write succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("write not canceled")
	}
	t.Logf("slow receiver blocked after %d bounded frames; cancellation joined writer", writes.Load())
}
func TestWANW01ForwardBackpressureCleanup(t *testing.T) {
	source, a := net.Pipe()
	b, receiver := net.Pipe()
	defer source.Close()
	defer receiver.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Forward(ctx, a, b, time.Second) }()
	sent := make(chan error, 1)
	go func() { _, e := source.Write(make([]byte, StreamBufferBytes*4)); sent <- e }()
	select {
	case <-sent:
		t.Fatal("unread receiver did not exert backpressure")
	case <-time.After(30 * time.Millisecond):
	}
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("pumps not joined")
	}
	select {
	case <-sent:
	case <-time.After(time.Second):
		t.Fatal("leg not closed")
	}
}
func TestWANW01ListenerCancel(t *testing.T) {
	l := NewStreamListener()
	a, b := net.Pipe()
	defer b.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := l.Offer(ctx, a); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	l.Close()
	if _, e := l.Accept(); !errors.Is(e, net.ErrClosed) {
		t.Fatal(e)
	}
}

func TestWANW01BrokerBrowserOrigin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	broker := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		ws, e := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
		if e == nil {
			ws.CloseNow()
		}
	}))
	defer broker.Close()
	headers := http.Header{}
	headers.Set("Origin", broker.URL)
	ws, response, e := websocket.Dial(ctx, "wss"+broker.URL[5:], &websocket.DialOptions{HTTPClient: broker.Client(), HTTPHeader: headers})
	if ws != nil {
		ws.CloseNow()
	}
	if e == nil || response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatal("same-origin browser reached stream")
	}
}
