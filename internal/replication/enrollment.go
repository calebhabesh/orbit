package replication

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
)

const EnrollmentMaxBytes = 16 << 10

type EnrollmentInvitation struct {
	Invitation   tc.Invitation `json:"invitation"` // capability always empty in durable records
	Digest       string        `json:"digest"`
	Revoked      bool          `json:"revoked"`
	Uses         int           `json:"uses"`
	TargetDevice string        `json:"target_device,omitempty"`
	TargetPin    string        `json:"target_pin,omitempty"`
}
type EnrollmentRecord struct {
	Routed            *protocol.RoutedEnrollmentRequest  `json:"routed,omitempty"`
	Approval          *protocol.RoutedEnrollmentApproval `json:"approval,omitempty"`
	ApprovalSignature string                             `json:"approval_signature,omitempty"`
	CreatedNS         int64                              `json:"created_ns"`
	Wire              protocol.TerminalEnrollmentWire    `json:"wire"` // token always empty in durable records
	Digest            string                             `json:"digest"`
	TokenDigest       string                             `json:"token_digest"`
	Expires           int64                              `json:"expires"`
	Result            protocol.TerminalEnrollmentResult  `json:"result"`
}
type enrollmentNonce struct {
	Routed       *protocol.RoutedChallengeRequest  `json:"routed,omitempty"`
	RoutedStatus *protocol.RoutedEnrollmentStatus  `json:"routed_status,omitempty"`
	Request      protocol.TerminalChallengeRequest `json:"request"` // token is verifier digest
	Result       protocol.TerminalChallengeResult  `json:"result"`
}

func enrollmentError(code string) error { return errors.New(code) }
func enrollmentID(s string) (history.ID, error) {
	var v history.ID
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 || hex.EncodeToString(b) != s || s == strings.Repeat("0", 64) {
		return v, enrollmentError("INVALID_REQUEST")
	}
	copy(v[:], b)
	return v, nil
}
func enrollmentRandom() (string, error) {
	var b [32]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}
func tokenVerifier(token string) (string, error) {
	b, err := enrollmentID(token)
	if err != nil {
		return "", err
	}
	d := sha256.Sum256(b[:])
	return hex.EncodeToString(d[:]), nil
}
func invitationActive(t *repository.EnrollmentTx, digest, folder string, now time.Time, allowUsed bool) (EnrollmentInvitation, error) {
	var inv EnrollmentInvitation
	if err := t.Get("invite/"+digest, &inv); err != nil {
		return inv, enrollmentError("INVITATION_INVALID")
	}
	expiry, err := time.Parse(time.RFC3339Nano, inv.Invitation.ExpiresAt)
	if err != nil || !now.Before(expiry) || inv.Revoked || (!allowUsed && inv.Uses >= 1) {
		return inv, enrollmentError("INVITATION_INVALID")
	}
	if folder != inv.Invitation.Folder {
		return inv, enrollmentError("FOLDER_MISMATCH")
	}
	f, err := enrollmentID(folder)
	if err != nil {
		return inv, err
	}
	m, _, err := t.Membership(f)
	if err != nil {
		return inv, err
	}
	active := false
	for _, a := range m.Active {
		if hex.EncodeToString(a.Device[:]) == inv.Invitation.Inviter && hex.EncodeToString(a.KeyPin[:]) == inv.Invitation.KeyPin {
			active = true
		}
	}
	if !active {
		return inv, enrollmentError("UNAUTHORIZED")
	}
	return inv, nil
}
func cleanNonces(t *repository.EnrollmentTx, prefix string, now time.Time) (int, error) {
	records, err := t.Records(prefix)
	if err != nil {
		return 0, err
	}
	n := 0
	for k, b := range records {
		var c enrollmentNonce
		if err := protocol.DecodeStrict(b, &c); err != nil {
			return 0, err
		}
		e, err := strconv.ParseInt(c.Result.ExpiresUnix, 10, 64)
		if err != nil {
			return 0, err
		}
		if now.Unix() >= e {
			if err := t.Delete(k); err != nil {
				return 0, err
			}
		} else {
			n++
		}
	}
	return n, nil
}
func enrollmentNonceCount(t *repository.EnrollmentTx, now time.Time) (int, error) {
	n, err := cleanNonces(t, "challenge/", now)
	if err != nil {
		return 0, err
	}
	m, err := cleanNonces(t, "status/", now)
	return n + m, err
}
func pendingCount(t *repository.EnrollmentTx, now time.Time) (int, error) {
	return t.PendingEnrollmentCount(now.Unix())
}

// EnrollmentServer mounts enrollment routes only. It never admits owner or data APIs.
type EnrollmentServer struct {
	repo     *repository.DB
	identity Identity
	now      func() time.Time
	admit    chan struct{}
	rateMu   sync.Mutex
	global   enrollmentBucket
	ips      map[string]enrollmentBucket
}
type enrollmentBucket struct {
	At     time.Time
	Tokens float64
}

func NewEnrollmentServer(db *repository.DB, id Identity) *EnrollmentServer {
	return &EnrollmentServer{repo: db, identity: id, now: time.Now, admit: make(chan struct{}, 8), ips: map[string]enrollmentBucket{}}
}
func (s *EnrollmentServer) HTTPServer() *http.Server {
	cfg := s.identity.ServerTLSConfig()
	cfg.ClientAuth = tls.NoClientCert
	return &http.Server{Handler: s, TLSConfig: cfg, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
}
func (s *EnrollmentServer) Serve(ctx context.Context, l net.Listener) error {
	h := s.HTTPServer()
	done := make(chan error, 1)
	go func() { done <- h.Serve(tls.NewListener(l, h.TLSConfig)) }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.Shutdown(c)
		err := <-done
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
func refill(b enrollmentBucket, now time.Time, rate, burst float64) enrollmentBucket {
	if b.At.IsZero() {
		return enrollmentBucket{now, burst}
	}
	elapsed := now.Sub(b.At).Seconds()
	if elapsed > 0 {
		b.Tokens += elapsed * rate
		if b.Tokens > burst {
			b.Tokens = burst
		}
		b.At = now
	}
	return b
}
func (s *EnrollmentServer) allow(ip string, now time.Time) bool {
	s.rateMu.Lock()
	defer s.rateMu.Unlock()
	for k, b := range s.ips {
		if now.Sub(b.At) >= time.Minute {
			delete(s.ips, k)
		}
	}
	if _, ok := s.ips[ip]; !ok && len(s.ips) >= 1024 {
		return false
	}
	s.global = refill(s.global, now, 64.0/60, 16)
	b := refill(s.ips[ip], now, 5.0/60, 5)
	if s.global.Tokens < 1 || b.Tokens < 1 {
		s.ips[ip] = b
		return false
	}
	s.global.Tokens--
	b.Tokens--
	s.ips[ip] = b
	return true
}
func (s *EnrollmentServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/enrollment/v3/challenge" && r.URL.Path != "/enrollment/v3/request" && r.URL.Path != "/enrollment/v3/status" && r.URL.Path != "/enrollment/v2/challenge" && r.URL.Path != "/enrollment/v2/request" && r.URL.Path != "/enrollment/v2/status" {
		http.NotFound(w, r)
		return
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		enrollmentHTTPError(w, enrollmentError("INVALID_REQUEST"))
		return
	}
	if !s.allow(ip, s.now()) {
		enrollmentHTTPError(w, enrollmentError("RATE_LIMITED"))
		return
	}
	select {
	case s.admit <- struct{}{}:
		defer func() { <-s.admit }()
	default:
		enrollmentHTTPError(w, enrollmentError("RATE_LIMITED"))
		return
	}
	if r.Method != http.MethodPost || r.Header.Get("Content-Encoding") != "" {
		enrollmentHTTPError(w, enrollmentError("INVALID_REQUEST"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, EnrollmentMaxBytes))
	if err != nil {
		enrollmentHTTPError(w, enrollmentError("PAYLOAD_TOO_LARGE"))
		return
	}
	var result any
	switch r.URL.Path {
	case "/enrollment/v3/challenge":
		var req protocol.RoutedChallengeRequest
		err = protocol.NetworkDecode(b, &req)
		if err == nil {
			result, err = s.routedChallenge(r.Context(), req)
		}
	case "/enrollment/v3/request":
		var req protocol.RoutedEnrollmentRequest
		err = protocol.NetworkDecode(b, &req)
		if err == nil {
			result, err = s.routedSubmit(r.Context(), req)
		}
	case "/enrollment/v3/status":
		var req protocol.RoutedEnrollmentStatus
		err = protocol.NetworkDecode(b, &req)
		if err == nil {
			result, err = s.routedStatus(r.Context(), req)
		}

	case "/enrollment/v2/challenge":
		var req protocol.TerminalChallengeRequest
		err = protocol.DecodeStrict(b, &req)
		if err == nil {
			result, err = s.challenge(r.Context(), req)
		}
	case "/enrollment/v2/request":
		var req protocol.TerminalEnrollmentWire
		err = protocol.DecodeStrict(b, &req)
		if err == nil {
			result, err = s.submit(r.Context(), req)
		}
	case "/enrollment/v2/status":
		var req protocol.TerminalEnrollmentStatusRequest
		err = protocol.DecodeStrict(b, &req)
		if err == nil {
			result, err = s.status(r.Context(), req)
		}
	}
	if err != nil {
		enrollmentHTTPError(w, err)
		return
	}
	out, err := json.Marshal(result)
	if err != nil || len(out) > EnrollmentMaxBytes {
		enrollmentHTTPError(w, enrollmentError("IO_ERROR"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}
func enrollmentHTTPError(w http.ResponseWriter, err error) {
	code := err.Error()
	status := http.StatusForbidden
	switch code {
	case "RATE_LIMITED":
		status = http.StatusTooManyRequests
		w.Header().Set("Retry-After", "60")
	case "PAYLOAD_TOO_LARGE":
		status = http.StatusRequestEntityTooLarge
	case "IDEMPOTENCY_CONFLICT":
		status = http.StatusConflict
	default:
		if code != "INVITATION_INVALID" && code != "FOLDER_MISMATCH" && code != "UNAUTHORIZED" && code != "STALE_VIEW" && code != "EXPIRED_REPLAY" {
			code = "INVALID_REQUEST"
			status = http.StatusBadRequest
		}
	}
	writeWireError(w, status, code, "enrollment rejected", code == "RATE_LIMITED", "inspect invitation or retry with backoff")
}
func (s *EnrollmentServer) challenge(ctx context.Context, q protocol.TerminalChallengeRequest) (protocol.TerminalChallengeResult, error) {
	var out protocol.TerminalChallengeResult
	if q.Version != "2" {
		return out, enrollmentError("INVALID_REQUEST")
	}
	for _, v := range []string{q.Folder, q.Attempt, q.Requester} {
		if _, err := enrollmentID(v); err != nil {
			return out, err
		}
	}
	digest, err := tokenVerifier(q.Token)
	if err != nil {
		return out, err
	}
	q.Token = digest
	err = s.repo.EnrollmentTransaction(ctx, func(t *repository.EnrollmentTx) error {
		inv, err := invitationActive(t, digest, q.Folder, s.now(), false)
		if err != nil {
			return err
		}
		if inv.Invitation.Version != tc.Version {
			return enrollmentError("UNSUPPORTED_CAPABILITY")
		}
		if inv.TargetDevice != "" && inv.TargetDevice != q.Requester {
			return enrollmentError("UNAUTHORIZED")
		}
		n, err := enrollmentNonceCount(t, s.now())
		if err != nil {
			return err
		}
		if n >= 128 {
			return enrollmentError("RATE_LIMITED")
		}
		f, _ := enrollmentID(q.Folder)
		_, prior, err := t.Membership(f)
		if err != nil {
			return err
		}
		nonce, err := enrollmentRandom()
		if err != nil {
			return err
		}
		expires := s.now().Add(time.Minute)
		ie, _ := time.Parse(time.RFC3339Nano, inv.Invitation.ExpiresAt)
		if ie.Before(expires) {
			expires = ie
		}
		out = protocol.TerminalChallengeResult{Version: "2", Challenge: nonce, ExpiresUnix: strconv.FormatInt(expires.Unix(), 10), PriorMembership: hex.EncodeToString(prior[:])}
		return t.Put("challenge/"+nonce, enrollmentNonce{Request: q, Result: out})
	})
	return out, err
}
func verifyEnrollmentWire(w protocol.TerminalEnrollmentWire) (protocol.TerminalEnrollmentTranscript, string, error) {
	tr, err := w.Transcript()
	if err != nil {
		return tr, "", err
	}
	der, err := base64.StdEncoding.DecodeString(w.CertificateDER)
	if err != nil {
		return tr, "", err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return tr, "", err
	}
	pub, ok := cert.PublicKey.(ed25519.PublicKey)
	if !ok || !pub.Equal(ed25519.PublicKey(tr.PublicKey[:])) || PublicKeyPin(cert) != history.Digest(tr.RequesterPin) {
		return tr, "", enrollmentError("UNAUTHORIZED")
	}
	bytes, err := tr.Canonical()
	if err != nil {
		return tr, "", err
	}
	sig, _ := hex.DecodeString(w.Signature)
	if !ed25519.Verify(pub, bytes, sig) {
		return tr, "", enrollmentError("UNAUTHORIZED")
	}
	d, err := tr.Digest()
	return tr, hex.EncodeToString(d[:]), err
}
func (s *EnrollmentServer) submit(ctx context.Context, w protocol.TerminalEnrollmentWire) (protocol.TerminalEnrollmentResult, error) {
	tr, digest, err := verifyEnrollmentWire(w)
	if err != nil {
		return protocol.TerminalEnrollmentResult{}, err
	}
	var out protocol.TerminalEnrollmentResult
	token, _ := tokenVerifier(w.Token)
	err = s.repo.EnrollmentTransaction(ctx, func(t *repository.EnrollmentTx) error {
		var existing EnrollmentRecord
		if err := t.Get("request/"+tr.RequestID(), &existing); err == nil {
			retryWire := w
			retryWire.Token = ""
			if existing.Digest != digest || existing.Wire != retryWire {
				return enrollmentError("IDEMPOTENCY_CONFLICT")
			}
			if s.now().Unix() >= existing.Expires {
				return enrollmentError("EXPIRED_REPLAY")
			}
			out = existing.Result
			return nil
		} else if !errors.Is(err, repository.ErrOperationNotFound) {
			return err
		}
		inv, err := invitationActive(t, token, w.Folder, s.now(), false)
		if err != nil {
			return err
		}
		i := inv.Invitation
		if i.Version != tc.Version {
			return enrollmentError("UNSUPPORTED_CAPABILITY")
		}
		if inv.TargetDevice != "" && (inv.TargetDevice != w.Requester || inv.TargetPin != w.RequesterPin) {
			return enrollmentError("UNAUTHORIZED")
		}
		if w.Inviter != i.Inviter || w.InviterPin != i.KeyPin || w.EnrollmentEndpoint != i.EnrollmentEndpoint || w.PeerEndpoint != i.PeerEndpoint {
			return enrollmentError("UNAUTHORIZED")
		}
		var nonce enrollmentNonce
		if err := t.Get("challenge/"+w.Challenge, &nonce); err != nil {
			return enrollmentError("UNAUTHORIZED")
		}
		if nonce.Request != (protocol.TerminalChallengeRequest{Version: "2", Folder: w.Folder, Token: token, Attempt: w.Attempt, Requester: w.Requester}) || nonce.Result.ExpiresUnix != w.ExpiresUnix || nonce.Result.PriorMembership != w.PriorMembership || s.now().Unix() >= int64(tr.ExpiresUnix) {
			return enrollmentError("UNAUTHORIZED")
		}
		_, prior, err := t.Membership(history.ID(tr.Folder))
		if err != nil {
			return err
		}
		if prior != history.Digest(tr.PriorMembership) {
			return enrollmentError("STALE_VIEW")
		}
		n, err := pendingCount(t, s.now())
		if err != nil {
			return err
		}
		if n >= 128 {
			return enrollmentError("RATE_LIMITED")
		}
		code, err := tr.VerificationCode()
		if err != nil {
			return err
		}
		out = protocol.TerminalEnrollmentResult{Version: "2", Request: tr.RequestID(), State: "pending_approval", TranscriptDigest: digest, VerificationCode: code, PeerEndpoint: w.PeerEndpoint}
		expires := s.now().Add(24 * time.Hour)
		ie, _ := time.Parse(time.RFC3339Nano, i.ExpiresAt)
		if ie.Before(expires) {
			expires = ie
		}
		w.Token = ""
		record := EnrollmentRecord{CreatedNS: s.now().UnixNano(), Wire: w, Digest: digest, TokenDigest: token, Expires: expires.Unix(), Result: out}
		if err := t.Put("request/"+out.Request, record); err != nil {
			return err
		}
		inv.Uses++
		if err := t.Put("invite/"+token, inv); err != nil {
			return err
		}
		return t.Delete("challenge/" + w.Challenge)
	})
	return out, err
}
func (s *EnrollmentServer) status(ctx context.Context, q protocol.TerminalEnrollmentStatusRequest) (any, error) {
	if q.Version != "2" {
		return nil, enrollmentError("INVALID_REQUEST")
	}
	req, err := enrollmentID(q.Request)
	if err != nil {
		return nil, err
	}
	var out any
	err = s.repo.EnrollmentTransaction(ctx, func(t *repository.EnrollmentTx) error {
		if q.Nonce == "" && q.Signature == "" && q.ExpiresUnix == "0" {
			n, err := enrollmentNonceCount(t, s.now())
			if err != nil {
				return err
			}
			if n >= 128 {
				return enrollmentError("RATE_LIMITED")
			}
			nonce, err := enrollmentRandom()
			if err != nil {
				return err
			}
			result := protocol.TerminalChallengeResult{Version: "2", Challenge: nonce, ExpiresUnix: strconv.FormatInt(s.now().Add(time.Minute).Unix(), 10)}
			out = result
			return t.Put("status/"+nonce, enrollmentNonce{Request: protocol.TerminalChallengeRequest{Requester: q.Request}, Result: result})
		}
		var nonce enrollmentNonce
		if err := t.Get("status/"+q.Nonce, &nonce); err != nil {
			return enrollmentError("UNAUTHORIZED")
		}
		e, err := strconv.ParseUint(q.ExpiresUnix, 10, 64)
		if err != nil || strconv.FormatUint(e, 10) != q.ExpiresUnix || nonce.Request.Requester != q.Request || nonce.Result.ExpiresUnix != q.ExpiresUnix || s.now().Unix() >= int64(e) {
			return enrollmentError("UNAUTHORIZED")
		}
		var record EnrollmentRecord
		if err := t.Get("request/"+q.Request, &record); err != nil {
			return enrollmentError("UNAUTHORIZED")
		}
		n, err := enrollmentID(q.Nonce)
		if err != nil {
			return err
		}
		msg, err := protocol.TerminalStatusTranscript([32]byte(req), [32]byte(n), e)
		if err != nil {
			return err
		}
		pub, err := hex.DecodeString(record.Wire.PublicKey)
		if err != nil {
			return err
		}
		sig, err := hex.DecodeString(q.Signature)
		if err != nil || hex.EncodeToString(sig) != q.Signature || !ed25519.Verify(ed25519.PublicKey(pub), msg, sig) {
			return enrollmentError("UNAUTHORIZED")
		}
		if s.now().Unix() >= record.Expires && record.Result.State == "pending_approval" {
			record.Result.State = "expired"
			if err := t.Put("request/"+q.Request, record); err != nil {
				return err
			}
		}
		out = record.Result
		return t.Delete("status/" + q.Nonce)
	})
	return out, err
}

// EnrollmentClient verifies the deliberately transferred trust anchor and SPKI
// before HTTP; redirects and proxy inheritance cannot disclose the capability.
type EnrollmentClient struct {
	invitation tc.Invitation
	identity   Identity
	http       *http.Client
	transport  interface{ CloseIdleConnections() }
}

func NewEnrollmentClient(inv tc.Invitation, id Identity) (*EnrollmentClient, error) {
	if inv.Version != tc.Version {
		return nil, enrollmentError("UNSUPPORTED_CAPABILITY")
	}
	if err := inv.Validate(); err != nil {
		return nil, err
	}
	der, _ := base64.StdEncoding.DecodeString(inv.CertificateDER)
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	pin, err := enrollmentID(inv.KeyPin)
	if err != nil {
		return nil, err
	}
	cfg, err := id.ClientTLSConfig(cert, history.Digest(pin))
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{TLSClientConfig: cfg, Proxy: nil, DisableCompression: true, DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext, ResponseHeaderTimeout: 5 * time.Second, MaxResponseHeaderBytes: 16 << 10}
	c := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("enrollment redirects forbidden") }}
	return &EnrollmentClient{inv, id, c, transport}, nil
}
func NewRoutedEnrollmentClient(ctx context.Context, inv tc.Invitation, id Identity, manager network.Manager) (*EnrollmentClient, error) {
	c, err := NewEnrollmentClient(inv, id)
	if err != nil {
		return nil, err
	}
	c.Close()
	device, err := enrollmentID(inv.Inviter)
	if err != nil {
		return nil, err
	}
	pin, err := enrollmentID(inv.KeyPin)
	if err != nil {
		return nil, err
	}
	der, _ := base64.StdEncoding.DecodeString(inv.CertificateDER)
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	trust, err := id.ClientTLSConfig(cert, history.Digest(pin))
	if err != nil {
		return nil, err
	}
	target := network.Target{Device: device, Pin: history.Digest(pin), Purpose: network.Enrollment}
	rt, err := manager.Transport(ctx, target, trust)
	if err != nil {
		return nil, err
	}
	c.http = network.HTTPClient(rt)
	c.http.Timeout = 10 * time.Second
	c.transport = c.http
	return c, nil
}

func (c *EnrollmentClient) Close() { c.transport.CloseIdleConnections() }
func (c *EnrollmentClient) post(ctx context.Context, path string, in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	if len(b) > EnrollmentMaxBytes {
		return enrollmentError("PAYLOAD_TOO_LARGE")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.invitation.EnrollmentEndpoint, "/")+path, strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return &EnrollmentConnectionError{Cause: connectionCause(err)}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, EnrollmentMaxBytes+1))
	if err != nil {
		return err
	}
	if len(data) > EnrollmentMaxBytes {
		return enrollmentError("PAYLOAD_TOO_LARGE")
	}
	if resp.StatusCode != http.StatusOK {
		var e ErrorResponse
		if protocol.DecodeStrict(data, &e) == nil {
			return fmt.Errorf("%s", e.Code)
		}
		return fmt.Errorf("enrollment HTTP %d", resp.StatusCode)
	}
	return protocol.DecodeStrict(data, out)
}

// Prepare returns the exact signed request to persist before Submit. A lost
// response is retried with these bytes, never a silently refreshed transcript.
func (c *EnrollmentClient) Prepare(ctx context.Context, attempt, label, endpoint string) (protocol.TerminalEnrollmentWire, error) {
	i := c.invitation
	expires, err := time.Parse(time.RFC3339Nano, i.ExpiresAt)
	if err != nil || !time.Now().Before(expires) {
		return protocol.TerminalEnrollmentWire{}, enrollmentError("INVITATION_INVALID")
	}
	var challenge protocol.TerminalChallengeResult
	err = c.post(ctx, "/enrollment/v2/challenge", protocol.TerminalChallengeRequest{Version: "2", Folder: i.Folder, Token: i.Capability, Attempt: attempt, Requester: hex.EncodeToString(c.identity.DeviceID[:])}, &challenge)
	if err != nil {
		return protocol.TerminalEnrollmentWire{}, err
	}
	private, ok := c.identity.Certificate.PrivateKey.(ed25519.PrivateKey)
	if !ok {
		return protocol.TerminalEnrollmentWire{}, enrollmentError("UNAUTHORIZED")
	}
	w := protocol.TerminalEnrollmentWire{Version: "2", Folder: i.Folder, Inviter: i.Inviter, InviterPin: i.KeyPin, Token: i.Capability, Attempt: attempt, Challenge: challenge.Challenge, Requester: hex.EncodeToString(c.identity.DeviceID[:]), RequesterPin: hex.EncodeToString(c.identity.KeyPin[:]), PublicKey: hex.EncodeToString(private.Public().(ed25519.PublicKey)), CertificateDER: base64.StdEncoding.EncodeToString(c.identity.Leaf.Raw), PriorMembership: challenge.PriorMembership, ExpiresUnix: challenge.ExpiresUnix, EnrollmentEndpoint: i.EnrollmentEndpoint, PeerEndpoint: i.PeerEndpoint, RequesterEndpoint: endpoint, Label: label, Signature: strings.Repeat("0", 128)}
	tr, err := w.Transcript()
	if err != nil {
		return w, err
	}
	b, err := tr.Canonical()
	if err != nil {
		return w, err
	}
	w.Signature = hex.EncodeToString(ed25519.Sign(private, b))
	return w, nil
}
func (c *EnrollmentClient) Submit(ctx context.Context, w protocol.TerminalEnrollmentWire) (protocol.TerminalEnrollmentResult, error) {
	var out protocol.TerminalEnrollmentResult
	err := c.post(ctx, "/enrollment/v2/request", w, &out)
	return out, err
}
func (c *EnrollmentClient) Status(ctx context.Context, request string) (protocol.TerminalEnrollmentResult, error) {
	var challenge protocol.TerminalChallengeResult
	var out protocol.TerminalEnrollmentResult
	q := protocol.TerminalEnrollmentStatusRequest{Version: "2", Request: request, ExpiresUnix: "0"}
	if err := c.post(ctx, "/enrollment/v2/status", q, &challenge); err != nil {
		return out, err
	}
	r, err := enrollmentID(request)
	if err != nil {
		return out, err
	}
	n, err := enrollmentID(challenge.Challenge)
	if err != nil {
		return out, err
	}
	expires, err := strconv.ParseUint(challenge.ExpiresUnix, 10, 64)
	if err != nil {
		return out, err
	}
	msg, err := protocol.TerminalStatusTranscript([32]byte(r), [32]byte(n), expires)
	if err != nil {
		return out, err
	}
	private, ok := c.identity.Certificate.PrivateKey.(ed25519.PrivateKey)
	if !ok {
		return out, enrollmentError("UNAUTHORIZED")
	}
	q.Nonce = challenge.Challenge
	q.ExpiresUnix = challenge.ExpiresUnix
	q.Signature = hex.EncodeToString(ed25519.Sign(private, msg))
	err = c.post(ctx, "/enrollment/v2/status", q, &out)
	if err == nil && (out.Version != "2" || out.Request != request || out.PeerEndpoint != c.invitation.PeerEndpoint) {
		err = enrollmentError("IDENTITY_MISMATCH")
	}
	if err == nil && out.State == "approved" {
		bytes, e := hex.DecodeString(out.MembershipHex)
		if e != nil {
			return out, e
		}
		membership, e := protocol.DecodeMembership(bytes)
		if e != nil {
			return out, e
		}
		inviter, requester := false, false
		for _, a := range membership.Active {
			if hex.EncodeToString(a.Device[:]) == c.invitation.Inviter && hex.EncodeToString(a.KeyPin[:]) == c.invitation.KeyPin {
				inviter = true
			}
			if a.Device == c.identity.DeviceID && a.KeyPin == c.identity.KeyPin {
				requester = true
			}
		}
		if hex.EncodeToString(membership.Folder[:]) != c.invitation.Folder || !inviter || !requester {
			err = enrollmentError("IDENTITY_MISMATCH")
		}
	}
	return out, err
}

// EnrollmentConnectionError reports that the inviting device could not be
// reached or did not prove its pinned identity. Cause is a short category
// (a service code, TIMEOUT or UNREACHABLE) that names no address or detail.
type EnrollmentConnectionError struct{ Cause string }

func (e *EnrollmentConnectionError) Error() string {
	return "could not reach the inviting device (" + e.Cause + ")"
}

// Retryable reports whether trying again later can help. An identity
// failure means the device at the other end is not the invited one.
func (e *EnrollmentConnectionError) Retryable() bool { return e.Cause != "IDENTITY_MISMATCH" }

func connectionCause(err error) string {
	var verify *tls.CertificateVerificationError
	if errors.Is(err, ErrPeerPinMismatch) || errors.As(err, &verify) {
		return "IDENTITY_MISMATCH"
	}
	var service *network.ServiceError
	if errors.As(err, &service) && service.Code != "" {
		return service.Code
	}
	var timeout interface{ Timeout() bool }
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
		return "TIMEOUT"
	}
	return "UNREACHABLE"
}
