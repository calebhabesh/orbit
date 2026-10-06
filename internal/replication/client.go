package replication

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/network"
	"github.com/calebhabesh/file-sync/internal/protocol"
)

type WireError struct {
	Status int
	Body   ErrorResponse
}

func (err *WireError) Error() string {
	return fmt.Sprintf("peer error %s: %s", err.Body.Code, err.Body.Message)
}

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string, identity Identity, peerCertificate *x509.Certificate, peerPin history.Digest) (*Client, error) {
	tlsConfig, err := identity.ClientTLSConfig(peerCertificate, peerPin)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("invalid manual peer origin")
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig, Proxy: nil, DisableCompression: true,
		DialContext:  (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
		MaxIdleConns: 4, MaxIdleConnsPerHost: 2, MaxConnsPerHost: 4,
		IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: 15 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second, MaxResponseHeaderBytes: 16 << 10,
	}

	return &Client{baseURL: "https://" + u.Host, http: network.HTTPClient(transport)}, nil
}

// NewRoutedClient borrows a target-bound daemon pool; replication supplies trust
// and retains all request/chunk/receipt authorization and validation.
func NewRoutedClient(ctx context.Context, baseURL string, identity Identity, peerCertificate *x509.Certificate, target network.Target, manager network.Manager) (*Client, error) {
	if target.Purpose != network.PeerData {
		return nil, errors.New("PURPOSE_MISMATCH")
	}
	trust, err := identity.ClientTLSConfig(peerCertificate, target.Pin)
	if err != nil {
		return nil, err
	}
	rt, err := manager.Transport(ctx, target, trust)
	if err != nil {
		return nil, err
	}
	return &Client{baseURL: baseURL, http: network.HTTPClient(rt)}, nil
}

func (client *Client) CloseIdleConnections() { client.http.CloseIdleConnections() }

func (client *Client) Hello(ctx context.Context, request HelloRequest) (HelloResponse, error) {
	var response HelloResponse
	return response, client.postJSON(ctx, "/peer/v1/hello", request, &response)
}

func (client *Client) Inventory(ctx context.Context, request InventoryRequest) (InventoryResponse, error) {
	var response InventoryResponse
	return response, client.postJSON(ctx, "/peer/v1/inventory", request, &response)
}

func (client *Client) Versions(ctx context.Context, request VersionsRequest) (VersionsResponse, error) {
	var response VersionsResponse
	if err := client.postJSON(ctx, "/peer/v1/versions/get", request, &response); err != nil {
		return response, err
	}
	for _, raw := range response.Envelopes {
		if _, err := protocol.DecodeEnvelope(raw); err != nil {
			return VersionsResponse{}, fmt.Errorf("peer returned invalid envelope: %w", err)
		}
	}
	return response, nil
}

func (client *Client) Receipts(ctx context.Context, request ReceiptsRequest) (ReceiptsResponse, error) {
	var response ReceiptsResponse
	return response, client.postJSON(ctx, "/peer/v1/receipts", request, &response)
}

func (client *Client) Status(ctx context.Context, request StatusRequest) (StatusResponse, error) {
	var response StatusResponse
	return response, client.postJSON(ctx, "/peer/v1/status", request, &response)
}

func (client *Client) MembershipGet(ctx context.Context, request MembershipGetRequest) (MembershipGetResponse, error) {
	var response MembershipGetResponse
	return response, client.postJSON(ctx, "/peer/v1/membership/get", request, &response)
}

func (client *Client) Chunk(ctx context.Context, request ChunkRequest, expected history.Chunk) ([]byte, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+"/peer/v1/chunks/get", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := client.http.Do(httpRequest)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, MaxMetadataBytes+1))
		if readErr != nil {
			return nil, readErr
		}
		var wire ErrorResponse
		if err := protocol.DecodeStrict(body, &wire); err != nil {
			return nil, fmt.Errorf("peer returned HTTP %d with invalid error body", response.StatusCode)
		}
		return nil, &WireError{Status: response.StatusCode, Body: wire}
	}
	if expected.Length > history.ChunkSize || response.Header.Get("Content-Length") != strconv.FormatUint(expected.Length, 10) || response.Header.Get("X-FileSync-Chunk-SHA256") != fmt.Sprintf("%x", expected.Digest) {
		return nil, errors.New("chunk response metadata does not match requested manifest")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(expected.Length)+1))
	if err != nil {
		return nil, err
	}
	if uint64(len(body)) != expected.Length || sha256.Sum256(body) != expected.Digest {
		return nil, errors.New("chunk response content does not match requested manifest")
	}
	return body, nil
}

func (client *Client) postJSON(ctx context.Context, path string, input, output any) error {
	for attempt := 0; attempt < MaxRetryAttempts; attempt++ {
		err := client.postJSONOnce(ctx, path, input, output)
		var wire *WireError
		if !errors.As(err, &wire) || !wire.Body.Retryable || (wire.Status != http.StatusTooManyRequests && wire.Status != http.StatusServiceUnavailable) || attempt == MaxRetryAttempts-1 {
			return err
		}
		// Only explicit pre-admission backpressure is retried here. Causal
		// mutations/receipts remain idempotent, with a bounded retry budget.
		timer := time.NewTimer(InitialRetryBackoff << attempt)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return errors.New("peer metadata retry budget exhausted")
}

func (client *Client) postJSONOnce(ctx context.Context, path string, input, output any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	if int64(len(data)) > MaxMetadataBytes {
		return errors.New("request exceeds metadata limit")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, MaxMetadataBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if int64(len(body)) > MaxMetadataBytes {
		return errors.New("peer response exceeds metadata limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var wire ErrorResponse
		if err := protocol.DecodeStrict(body, &wire); err != nil {
			return fmt.Errorf("peer returned HTTP %d with invalid error body", response.StatusCode)
		}
		return &WireError{Status: response.StatusCode, Body: wire}
	}
	if err := protocol.DecodeStrict(body, output); err != nil {
		return fmt.Errorf("decode peer response: %w", err)
	}
	return nil
}
