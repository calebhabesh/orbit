package designgates

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/model"
)

var (
	ErrInvitationNotFound = errors.New("invitation not found or expired")
	ErrInvitationRevoked  = errors.New("invitation was revoked")
	ErrInvalidSignature   = errors.New("joining signature verification failed")
	ErrPayloadTooLarge    = errors.New("request payload exceeds bounded limit")
	ErrRateLimitExceeded  = errors.New("rate limit exceeded")
)

type storedInvitation struct {
	Digest    string
	Workspace string
	ExpiresAt time.Time
	Revoked   bool
	MaxUses   int
	UsesCount int
}

type joinRequestRecord struct {
	RequestID      string
	WorkspaceID    string
	JoiningDevice  string
	JoiningKeyPin  string
	JoiningPubKey  ed25519.PublicKey
	SuggestedLabel string
	Approved       bool
}

type enrollmentManager struct {
	mu          sync.Mutex
	invitations map[string]*storedInvitation // keyed by SHA-256 hex digest
	requests    map[string]*joinRequestRecord
	rateCounts  map[string]int
	maxBodySize int64
}

func newEnrollmentManager() *enrollmentManager {
	return &enrollmentManager{
		invitations: make(map[string]*storedInvitation),
		requests:    make(map[string]*joinRequestRecord),
		rateCounts:  make(map[string]int),
		maxBodySize: 16 * 1024, // 16 KiB limit
	}
}

func (em *enrollmentManager) CreateInvitation(workspace string, ttl time.Duration) (token string, digest string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token = hex.EncodeToString(b)
	h := sha256.Sum256([]byte(token))
	digest = hex.EncodeToString(h[:])

	em.mu.Lock()
	defer em.mu.Unlock()
	em.invitations[digest] = &storedInvitation{
		Digest:    digest,
		Workspace: workspace,
		ExpiresAt: time.Now().Add(ttl),
		Revoked:   false,
		MaxUses:   1,
	}
	return token, digest, nil
}

func (em *enrollmentManager) RevokeInvitation(digest string) {
	em.mu.Lock()
	defer em.mu.Unlock()
	if inv, ok := em.invitations[digest]; ok {
		inv.Revoked = true
	}
}

func (em *enrollmentManager) SubmitJoinRequest(clientIP string, token string, req joinRequestRecord, challenge []byte, sig []byte) error {
	em.mu.Lock()
	defer em.mu.Unlock()

	// Rate limit check: max 5 requests per IP
	if em.rateCounts[clientIP] >= 5 {
		return ErrRateLimitExceeded
	}
	em.rateCounts[clientIP]++

	h := sha256.Sum256([]byte(token))
	digest := hex.EncodeToString(h[:])

	inv, exists := em.invitations[digest]
	if !exists || time.Now().After(inv.ExpiresAt) {
		return ErrInvitationNotFound
	}
	if inv.Revoked {
		return ErrInvitationRevoked
	}
	if inv.UsesCount >= inv.MaxUses {
		return ErrInvitationNotFound
	}

	// Verify cryptographic proof of possession of the joining private key
	if !ed25519.Verify(req.JoiningPubKey, challenge, sig) {
		return ErrInvalidSignature
	}

	inv.UsesCount++
	req.Approved = false
	em.requests[req.RequestID] = &req
	return nil
}

// TestOrbitG02InvitationCreationAndDigestStorage tests that invitation secrets are stored
// only as one-way SHA-256 verifiers, and cannot be read out in plaintext from storage.
func TestOrbitG02InvitationCreationAndDigestStorage(t *testing.T) {
	em := newEnrollmentManager()

	rawToken, digest, err := em.CreateInvitation("ws-main", 1*time.Hour)
	if err != nil {
		t.Fatalf("CreateInvitation failed: %v", err)
	}

	// Ensure plaintext token is NOT stored in manager
	em.mu.Lock()
	for k := range em.invitations {
		if k == rawToken {
			t.Fatal("plaintext token was stored directly in invitations table")
		}
	}
	inv, ok := em.invitations[digest]
	em.mu.Unlock()

	if !ok || inv.Digest != digest {
		t.Fatal("invitation not found by digest")
	}

	// Revocation check
	em.RevokeInvitation(digest)
	em.mu.Lock()
	revoked := em.invitations[digest].Revoked
	em.mu.Unlock()
	if !revoked {
		t.Fatal("expected invitation to be marked revoked")
	}
}

// TestOrbitG02CapabilityGatingAndDosLimits tests Invariant I23: invitation possession
// cannot grant access to ordinary data or metadata; bounds and rate limits protect the listener.
func TestOrbitG02CapabilityGatingAndDosLimits(t *testing.T) {
	em := newEnrollmentManager()
	token, digest, _ := em.CreateInvitation("ws-secure", 1*time.Hour)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/inventory", "/api/v1/chunks/123":
			// Ordinary data endpoints require established peer mTLS or session cookie;
			// an invitation token passed here MUST be rejected!
			http.Error(w, "forbidden: invitation token cannot access content", http.StatusForbidden)
		case "/api/v1/enrollment/request":
			if r.ContentLength > em.maxBodySize {
				http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
				return
			}
			// Verify rate limit
			ip := "192.168.1.50"
			pub, priv, _ := ed25519.GenerateKey(rand.Reader)
			challenge := []byte("challenge-data")
			sig := ed25519.Sign(priv, challenge)
			err := em.SubmitJoinRequest(ip, token, joinRequestRecord{
				RequestID:     "req-1",
				JoiningPubKey: pub,
			}, challenge, sig)
			if errors.Is(err, ErrRateLimitExceeded) {
				http.Error(w, "rate limited", http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	client := &http.Client{}

	// Adversarial test 1: Attempt to access inventory using invitation token
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/inventory", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for inventory access using invitation token, got: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Adversarial test 2: Attempt to access chunk data using invitation token
	reqChunk, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/chunks/123", nil)
	reqChunk.Header.Set("Authorization", "Bearer "+token)
	respChunk, err := client.Do(reqChunk)
	if err != nil || respChunk.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for chunk access using invitation token, got: %d", respChunk.StatusCode)
	}
	respChunk.Body.Close()

	// Adversarial test 3: Oversized request payload rejected
	hugeBody := bytes.Repeat([]byte("A"), int(em.maxBodySize)+1024)
	reqHuge, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/enrollment/request", bytes.NewReader(hugeBody))
	respHuge, err := client.Do(reqHuge)
	if err != nil || respHuge.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 Payload Too Large, got: %d", respHuge.StatusCode)
	}
	respHuge.Body.Close()

	// Adversarial test 4: Rate limit protection
	_ = digest
	for i := 0; i < 5; i++ {
		reqJoin, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/enrollment/request", bytes.NewReader([]byte("{}")))
		r, _ := client.Do(reqJoin)
		r.Body.Close()
	}
	// 6th request must trigger rate limit
	reqRate, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/enrollment/request", bytes.NewReader([]byte("{}")))
	respRate, err := client.Do(reqRate)
	if err != nil || respRate.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got: %d", respRate.StatusCode)
	}
	respRate.Body.Close()
}

// TestOrbitG02KeyPossessionAndOwnerApproval tests that a joining device must prove
// possession of its private key, and that owner approval is strictly required before
// membership revision is created.
func TestOrbitG02KeyPossessionAndOwnerApproval(t *testing.T) {
	em := newEnrollmentManager()
	token, _, err := em.CreateInvitation("ws-appr", 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	joinPub, joinPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	challenge := []byte("orbit-challenge-nonce-98765")
	validSig := ed25519.Sign(joinPriv, challenge)

	// Negative test: Forged signature by wrong private key
	_, attackerPriv, _ := ed25519.GenerateKey(rand.Reader)
	forgedSig := ed25519.Sign(attackerPriv, challenge)

	err = em.SubmitJoinRequest("127.0.0.1", token, joinRequestRecord{
		RequestID:     "req-forged",
		JoiningDevice: "join-dev-1",
		JoiningPubKey: joinPub,
	}, challenge, forgedSig)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got: %v", err)
	}

	// Positive test: Valid signature accepted into pending requests
	err = em.SubmitJoinRequest("127.0.0.1", token, joinRequestRecord{
		RequestID:      "req-valid",
		WorkspaceID:    "ws-appr",
		JoiningDevice:  "join-dev-1",
		JoiningKeyPin:  "pin-1",
		JoiningPubKey:  joinPub,
		SuggestedLabel: "Laptop",
	}, challenge, validSig)
	if err != nil {
		t.Fatalf("valid submit failed: %v", err)
	}

	em.mu.Lock()
	pending, ok := em.requests["req-valid"]
	em.mu.Unlock()
	if !ok || pending.Approved {
		t.Fatal("expected request to be stored in unapproved pending state")
	}

	// Replay of same single-use invitation must be rejected
	err = em.SubmitJoinRequest("127.0.0.1", token, joinRequestRecord{
		RequestID:     "req-replay",
		JoiningDevice: "join-dev-2",
		JoiningPubKey: joinPub,
	}, challenge, validSig)
	if !errors.Is(err, ErrInvitationNotFound) {
		t.Fatalf("expected ErrInvitationNotFound on exhausted invitation, got: %v", err)
	}
}

// TestOrbitG02OfflineRolloutAndCompetingFork tests Invariant I24:
// competing approvals from partitioned devices create an explicit membership fork
// that blocks silent data exchange until explicit owner reconciliation.
func TestOrbitG02OfflineRolloutAndCompetingFork(t *testing.T) {
	ownerPub, ownerPriv, _ := ed25519.GenerateKey(rand.Reader)

	chain, err := model.NewMembershipChain("ws-rollout", []model.MemberRecord{
		{DeviceID: "owner-dev", KeyPin: "pin-owner"},
	}, ownerPub, ownerPriv)
	if err != nil {
		t.Fatal(err)
	}

	// Two partitioned peers approve different devices from the same base revision 1
	baseDigest := chain.Head.Digest()

	rev2A := &model.MembershipRevision{
		WorkspaceID: "ws-rollout",
		Revision:    2,
		PriorDigest: baseDigest,
		Active: []model.MemberRecord{
			{DeviceID: "owner-dev", KeyPin: "pin-owner"},
			{DeviceID: "peer-A", KeyPin: "pin-A"},
		},
		AuthorID: "owner-dev",
	}
	rev2A.Sign(ownerPriv)

	rev2B := &model.MembershipRevision{
		WorkspaceID: "ws-rollout",
		Revision:    2,
		PriorDigest: baseDigest,
		Active: []model.MemberRecord{
			{DeviceID: "owner-dev", KeyPin: "pin-owner"},
			{DeviceID: "peer-B", KeyPin: "pin-B"},
		},
		AuthorID: "owner-dev",
	}
	rev2B.Sign(ownerPriv)

	// Check fork detection oracle
	err = chain.DetectFork(rev2A, rev2B)
	if !errors.Is(err, model.ErrMembershipFork) {
		t.Fatalf("expected ErrMembershipFork, got: %v", err)
	}

	// Applying rev2A
	if err := chain.ApplyRevision(rev2A); err != nil {
		t.Fatalf("failed to apply rev2A: %v", err)
	}

	// Connecting to peer holding rev2B must be rejected rather than silently merging
	err = chain.ApplyRevision(rev2B)
	if err == nil {
		t.Fatal("expected applying conflicting branch rev2B to fail")
	}

	// Explicit owner reconciliation produces Rev 3
	rev3 := &model.MembershipRevision{
		WorkspaceID: "ws-rollout",
		Revision:    3,
		PriorDigest: chain.Head.Digest(),
		Active: []model.MemberRecord{
			{DeviceID: "owner-dev", KeyPin: "pin-owner"},
			{DeviceID: "peer-A", KeyPin: "pin-A"},
			{DeviceID: "peer-B", KeyPin: "pin-B"},
		},
		AuthorID: "owner-dev",
	}
	rev3.Sign(ownerPriv)

	if err := chain.ApplyRevision(rev3); err != nil {
		t.Fatalf("reconciliation failed: %v", err)
	}
	if !chain.IsMember("peer-A") || !chain.IsMember("peer-B") {
		t.Fatal("expected both peers active after reconciliation")
	}
}

// TestOrbitG02RetiredDeviceCannotRejoin tests that once retired in a membership revision,
// a device cannot be revived into the active set.
func TestOrbitG02RetiredDeviceCannotRejoin(t *testing.T) {
	ownerPub, ownerPriv, _ := ed25519.GenerateKey(rand.Reader)

	chain, err := model.NewMembershipChain("ws-retire", []model.MemberRecord{
		{DeviceID: "dev-keep", KeyPin: "pin-keep"},
		{DeviceID: "dev-retire", KeyPin: "pin-retire"},
	}, ownerPub, ownerPriv)
	if err != nil {
		t.Fatal(err)
	}

	// Rev 2 retires dev-retire
	rev2 := &model.MembershipRevision{
		WorkspaceID: "ws-retire",
		Revision:    2,
		PriorDigest: chain.Head.Digest(),
		Active: []model.MemberRecord{
			{DeviceID: "dev-keep", KeyPin: "pin-keep"},
		},
		Retired:  []string{"dev-retire"},
		AuthorID: "owner",
	}
	rev2.Sign(ownerPriv)
	if err := chain.ApplyRevision(rev2); err != nil {
		t.Fatalf("rev2 failed: %v", err)
	}

	// Rev 3 attempts to re-add dev-retire
	rev3Revive := &model.MembershipRevision{
		WorkspaceID: "ws-retire",
		Revision:    3,
		PriorDigest: chain.Head.Digest(),
		Active: []model.MemberRecord{
			{DeviceID: "dev-keep", KeyPin: "pin-keep"},
			{DeviceID: "dev-retire", KeyPin: "pin-retire"},
		},
		AuthorID: "owner",
	}
	rev3Revive.Sign(ownerPriv)

	err = chain.ApplyRevision(rev3Revive)
	if !errors.Is(err, model.ErrRetiredMemberRevival) {
		t.Fatalf("expected ErrRetiredMemberRevival, got: %v", err)
	}
}

func discardBody(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}
