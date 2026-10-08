package network

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/calebhabesh/orbit/internal/history"
)

type retiringTestConn struct {
	net.Conn
	retired *atomic.Bool
}

func (c *retiringTestConn) Retired() bool { return c.retired.Load() }
func TestWANW04ManagerRetiresPooledRelayBeforeNewRequest(t *testing.T) {
	var handled atomic.Int32
	s, trust, target := managerServer(t, func(w http.ResponseWriter, r *http.Request) { handled.Add(1); _, _ = w.Write([]byte("synthetic")) })
	target.Profile = history.Digest{1}
	var retired atomic.Bool
	m := NewManager(ManagerOptions{})
	defer m.Close()
	if err := m.SetRelay(target, func(ctx context.Context, _ Target) (net.Conn, error) {
		c, err := (&net.Dialer{}).DialContext(ctx, "tcp", strings.TrimPrefix(s.URL, "https://"))
		if err != nil {
			return nil, err
		}
		return &retiringTestConn{c, &retired}, nil
	}); err != nil {
		t.Fatal(err)
	}
	rt, err := m.Transport(context.Background(), target, trust)
	if err != nil {
		t.Fatal(err)
	}
	res := round(t, rt, LogicalOrigin(target))
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	if m.Observe(target).Route != "relay" {
		t.Fatal("missing relay observation")
	}
	retired.Store(true)
	if _, err = rt.RoundTrip(mustRequest(LogicalOrigin(target))); !errors.Is(err, ErrStale) {
		t.Fatal("retired tunnel reused", err)
	}
	if handled.Load() != 1 {
		t.Fatal("new HTTP bytes entered retired tunnel")
	}
}
func TestWANW04ManagerRelayIsolationAndPendingCancel(t *testing.T) {
	_, trust, target := managerServer(t, func(http.ResponseWriter, *http.Request) {})
	target.Profile = history.Digest{1}
	m := NewManager(ManagerOptions{})
	defer m.Close()
	started := make(chan struct{})
	if err := m.SetRelay(target, func(ctx context.Context, _ Target) (net.Conn, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	rt, err := m.Transport(context.Background(), target, trust)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", LogicalOrigin(target), nil)
	done := make(chan error, 1)
	go func() { _, err := rt.RoundTrip(req); done <- err }()
	<-started
	cancel()
	if err = <-done; err == nil {
		t.Fatal("pending relay ignored cancel")
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = m.IncomingListener(Purpose("control")); err == nil {
		t.Fatal("owner control virtual listener exposed")
	}
	lData, _ := m.IncomingListener(PeerData)
	lEnroll, _ := m.IncomingListener(Enrollment)
	if lData == lEnroll {
		t.Fatal("purpose queues shared")
	}
}
