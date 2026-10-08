package network

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"testing"

	"github.com/calebhabesh/orbit/internal/history"
)

func TestWANW01TransportTrustAndPlaintext(t *testing.T) {
	leaf := &x509.Certificate{RawSubjectPublicKeyInfo: []byte("synthetic SPKI")}
	pin := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	var device history.ID
	device[0] = 1
	target := Target{Device: device, Pin: pin, Purpose: PeerData}
	upstream := errors.New("replication rejected this cert")
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: x509.NewCertPool(), ServerName: "peer.orbit.invalid", VerifyConnection: func(tls.ConnectionState) error { return upstream }}
	called := false
	rt, e := NewTransport(target, cfg, func(context.Context, Target) (net.Conn, error) {
		called = true
		return nil, errors.New("no dial expected")
	})
	if e != nil {
		t.Fatal(e)
	}
	defer rt.CloseIdleConnections()
	request, _ := http.NewRequest("POST", "http://peer.orbit.invalid/peer/v1/hello", nil)
	if _, e = rt.RoundTrip(request); e == nil || called {
		t.Fatal("plaintext reached dialer")
	}
	if e = rt.transport.TLSClientConfig.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}}); !errors.Is(e, upstream) {
		t.Fatal("bypassed replication verifier")
	}
	if rt.transport.Proxy != nil || rt.transport.TLSClientConfig.RootCAs == cfg.RootCAs || rt.transport.TLSClientConfig == cfg {
		t.Fatal("trust clone/proxy")
	}
	cfg.InsecureSkipVerify = true
	if _, e = NewTransport(target, cfg, func(context.Context, Target) (net.Conn, error) { return nil, nil }); e == nil {
		t.Fatal("permissive TLS accepted")
	}
}
