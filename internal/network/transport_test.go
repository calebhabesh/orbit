package network

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
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

type cancellationRoundTripper func(*http.Request) (*http.Response, error)

func (run cancellationRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return run(request)
}

type cancellationBody struct {
	io.Reader
	closed bool
}

func (body *cancellationBody) Close() error {
	body.closed = true
	return nil
}

// A response and cancellation can both become available to the HTTP adapter.
// A canceled peer request must not report success or leak the response body.
func TestWANW01TransportCancellationWinsCompletedResponse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := &cancellationBody{Reader: strings.NewReader("completed response")}
	underlying := &http.Transport{}
	underlying.RegisterProtocol("https", cancellationRoundTripper(func(request *http.Request) (*http.Response, error) {
		cancel()
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: request}, nil
	}))
	transport := &Transport{transport: underlying}
	defer transport.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, "POST", "https://peer.orbit.invalid/peer/v1/hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := HTTPClient(transport).Do(request)
	if response != nil {
		defer response.Body.Close()
	}
	if !errors.Is(err, context.Canceled) || response != nil {
		t.Fatalf("canceled request returned response=%v, error=%v", response != nil, err)
	}
	if !body.closed {
		t.Fatal("canceled response body was not closed")
	}
}
