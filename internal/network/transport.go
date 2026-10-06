// Package network owns routes and bounded transport adapters. Replication owns
// peer TLS trust and folder authority; routing never supplies those permissions.
package network

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
)

type Purpose string

const (
	PeerData   Purpose = "peer_data"
	Enrollment Purpose = "enrollment"
)

type Target struct {
	Device  history.ID
	Pin     history.Digest
	Purpose Purpose
	// Profile is a reviewed profile digest, empty only for explicit manual routes.
	Profile history.Digest
}

func (t Target) Validate() error {
	if t.Device == (history.ID{}) || t.Pin == (history.Digest{}) || (t.Purpose != PeerData && t.Purpose != Enrollment) {
		return errors.New("INVALID_TARGET")
	}
	return nil
}

type Observation struct {
	Target     Target
	Route      string
	Code       string
	ObservedAt time.Time
	Generation uint64
}
type Manager interface {
	Transport(context.Context, Target, *tls.Config) (http.RoundTripper, error)
	Observe(Target) Observation
	Close() error
}

// StreamDialer receives only an already-bound logical target. It must not dial
// an HTTP request's URL. Its lifetime belongs to the owning manager/pool.
type StreamDialer func(context.Context, Target) (net.Conn, error)

// NewTransport demonstrates the common direct/relay HTTP seam. TLS policy comes
// from replication; this adapter clones it and pins ALPN to HTTP/1.1 for WSS.
// Target binding is checked in addition to (never instead of) supplied TLS trust.
// W02 owns manager integration and lifetime shutdown.
func NewTransport(target Target, trust *tls.Config, dial StreamDialer) (*Transport, error) {
	if err := target.Validate(); err != nil {
		return nil, err
	}
	if trust == nil || trust.InsecureSkipVerify || trust.VerifyConnection == nil || trust.RootCAs == nil || trust.ServerName == "" || trust.MinVersion < tls.VersionTLS13 || dial == nil {
		return nil, errors.New("INVALID_TLS_TRUST")
	}
	cfg := trust.Clone()
	cfg.NextProtos = []string{"http/1.1"}
	cfg.RootCAs = trust.RootCAs.Clone()
	verify := trust.VerifyConnection
	cfg.VerifyConnection = func(state tls.ConnectionState) error {
		if e := verify(state); e != nil {
			return e
		}
		if len(state.PeerCertificates) == 0 || sha256.Sum256(state.PeerCertificates[0].RawSubjectPublicKeyInfo) != target.Pin {
			return errors.New("IDENTITY_MISMATCH")
		}
		return nil
	}
	headerTimeout := 15 * time.Second
	if target.Purpose == Enrollment {
		headerTimeout = 5 * time.Second
	}
	return &Transport{transport: &http.Transport{
		Proxy: nil, TLSClientConfig: cfg, DisableCompression: true,
		MaxIdleConns: 4, MaxIdleConnsPerHost: 2, MaxConnsPerHost: 2,
		IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: headerTimeout,
		MaxResponseHeaderBytes: 16 << 10, TLSHandshakeTimeout: 10 * time.Second,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) { return dial(ctx, target) },
	}}, nil
}
func HTTPClient(rt http.RoundTripper) *http.Client {
	return &http.Client{Transport: rt, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("REDIRECT_FORBIDDEN") }}
}

// Transport keeps the inner TLS requirement at the public RoundTripper seam.
// A raw http.Transport would accept an http:// request and disclose plaintext.
type Transport struct {
	transport         *http.Transport
	quic              *quicPool
	routeMu           sync.Mutex
	candidateRevision uint64
	tcpUntil          time.Time
	routeManager      *ConnectionManager
	routeTarget       Target
	route             manualRoute
	routeTrust        *tls.Config
}

func (t *Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r == nil || r.URL == nil || r.URL.Scheme != "https" || r.URL.User != nil {
		return nil, errors.New("TLS_REQUIRED")
	}
	if t.quic != nil && t.routeManager != nil {
		ready, useQUIC, err := t.selectRoute(r.Context())
		if err != nil {
			return nil, err
		}
		if !useQUIC {
			if ready != nil {
				defer ready.closeUnused()
				r = r.Clone(context.WithValue(r.Context(), preparedTLSKey{}, ready))
			}
			return t.transport.RoundTrip(r)
		}
	}
	if t.quic != nil {
		response, selected, err := t.quic.roundTrip(r)
		if selected {
			return response, err
		}
		if r.Context().Err() != nil {
			return nil, r.Context().Err()
		}
	}
	return t.transport.RoundTrip(r)
}
func (t *Transport) CloseIdleConnections() {
	t.transport.CloseIdleConnections()
	if t.quic != nil {
		t.quic.close()
	}
}

// ServiceError is bounded route backpressure/offline state, never a file receipt.
type ServiceError struct{ Code string }

func (e *ServiceError) Error() string { return e.Code }
