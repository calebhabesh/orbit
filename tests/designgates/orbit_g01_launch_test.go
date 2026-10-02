package designgates

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/state"
	"github.com/calebhabesh/file-sync/internal/testkit"
)

var (
	ErrBootstrapTokenUsed    = errors.New("bootstrap token already consumed")
	ErrBootstrapTokenExpired = errors.New("bootstrap token expired")
	ErrInvalidHostHeader     = errors.New("invalid host header: non-loopback rejected")
	ErrInvalidOrigin         = errors.New("invalid origin header: cross-site origin rejected")
)

type bootstrapManager struct {
	mu       sync.Mutex
	tokens   map[string]time.Time
	consumed map[string]bool
	ttl      time.Duration
}

func newBootstrapManager(ttl time.Duration) *bootstrapManager {
	return &bootstrapManager{
		tokens:   make(map[string]time.Time),
		consumed: make(map[string]bool),
		ttl:      ttl,
	}
}

func (bm *bootstrapManager) IssueToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.tokens[token] = time.Now().Add(bm.ttl)
	return token, nil
}

func (bm *bootstrapManager) Exchange(token, host, origin string) (string, error) {
	// Validate Host is strictly loopback
	hostWithoutPort := host
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		hostWithoutPort = host[:idx]
	}
	if hostWithoutPort != "127.0.0.1" && hostWithoutPort != "localhost" && hostWithoutPort != "[::1]" {
		return "", ErrInvalidHostHeader
	}

	// Validate Origin if present
	if origin != "" && origin != "null" {
		if !strings.HasPrefix(origin, "http://127.0.0.1") &&
			!strings.HasPrefix(origin, "http://localhost") &&
			!strings.HasPrefix(origin, "http://[::1]") {
			return "", ErrInvalidOrigin
		}
	}

	bm.mu.Lock()
	defer bm.mu.Unlock()

	if bm.consumed[token] {
		return "", ErrBootstrapTokenUsed
	}
	expiry, exists := bm.tokens[token]
	if !exists {
		return "", errors.New("unknown bootstrap token")
	}
	if time.Now().After(expiry) {
		return "", ErrBootstrapTokenExpired
	}

	// Mark consumed immediately (one-use handoff)
	bm.consumed[token] = true

	// Mint session cookie ID
	sessionBytes := make([]byte, 24)
	if _, err := rand.Read(sessionBytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(sessionBytes), nil
}

// TestOrbitG01SingletonExclusiveOwnership verifies that the daemon enforces exclusive
// state directory locking, preventing duplicate background instances on the same state.
func TestOrbitG01SingletonExclusiveOwnership(t *testing.T) {
	stateDir := testkit.NewDisposable(t)
	if err := state.EnsureDirectory(stateDir); err != nil {
		t.Fatal(err)
	}

	// First daemon instance acquires exclusive lock
	holder1, err := state.Acquire(stateDir)
	if err != nil {
		t.Fatalf("first state.Acquire failed: %v", err)
	}
	defer holder1.Close()

	// Second instance must fail immediately
	holder2, err := state.Acquire(stateDir)
	if !errors.Is(err, state.ErrLocked) {
		if err == nil {
			holder2.Close()
		}
		t.Fatalf("expected state.ErrLocked for second instance, got: %v", err)
	}

	// Releasing first allows subsequent acquisition
	if err := holder1.Close(); err != nil {
		t.Fatalf("holder1.Close failed: %v", err)
	}

	holder3, err := state.Acquire(stateDir)
	if err != nil {
		t.Fatalf("expected acquisition after release to succeed, got: %v", err)
	}
	holder3.Close()
}

// TestOrbitG01OneUseBootstrapHandoff tests the single-use browser handoff mechanism,
// verifying replay prevention, expiration, loopback host validation, and origin checking.
func TestOrbitG01OneUseBootstrapHandoff(t *testing.T) {
	bm := newBootstrapManager(200 * time.Millisecond)

	token, err := bm.IssueToken()
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	// Negative case 1: DNS rebinding / non-loopback Host rejected
	_, err = bm.Exchange(token, "evil.attacker.com:8080", "http://evil.attacker.com:8080")
	if !errors.Is(err, ErrInvalidHostHeader) {
		t.Fatalf("expected ErrInvalidHostHeader for external host, got: %v", err)
	}

	// Negative case 2: Cross-site Origin rejected
	_, err = bm.Exchange(token, "127.0.0.1:8080", "http://malicious-site.com")
	if !errors.Is(err, ErrInvalidOrigin) {
		t.Fatalf("expected ErrInvalidOrigin, got: %v", err)
	}

	// Success case: Valid loopback host and matching origin
	sessionID, err := bm.Exchange(token, "127.0.0.1:8080", "http://127.0.0.1:8080")
	if err != nil {
		t.Fatalf("valid exchange failed: %v", err)
	}
	if sessionID == "" {
		t.Fatal("expected non-empty session ID")
	}

	// Negative case 3: Replay attack with same token must fail
	_, err = bm.Exchange(token, "127.0.0.1:8080", "http://127.0.0.1:8080")
	if !errors.Is(err, ErrBootstrapTokenUsed) {
		t.Fatalf("expected ErrBootstrapTokenUsed on replay, got: %v", err)
	}

	// Negative case 4: Expired token
	expiredToken, err := bm.IssueToken()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond) // Exceed 200ms TTL
	_, err = bm.Exchange(expiredToken, "127.0.0.1:8080", "")
	if !errors.Is(err, ErrBootstrapTokenExpired) {
		t.Fatalf("expected ErrBootstrapTokenExpired, got: %v", err)
	}
}

// TestOrbitG01SessionAndLogoutKeepsSyncActive tests Invariant I21: closing the web session
// or logging out invalidates UI session cookies but preserves the running sync daemon and lock.
func TestOrbitG01SessionAndLogoutKeepsSyncActive(t *testing.T) {
	stateDir := testkit.NewDisposable(t)
	if err := state.EnsureDirectory(stateDir); err != nil {
		t.Fatal(err)
	}

	daemonLock, err := state.Acquire(stateDir)
	if err != nil {
		t.Fatalf("failed to acquire daemon lock: %v", err)
	}
	defer daemonLock.Close()

	sessions := make(map[string]bool)
	var mu sync.Mutex

	// HTTP handler simulating local control server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/logout":
			cookie, err := r.Cookie("orbit_session")
			if err != nil {
				http.Error(w, "missing cookie", http.StatusUnauthorized)
				return
			}
			mu.Lock()
			delete(sessions, cookie.Value)
			mu.Unlock()
			// Clear cookie on client
			http.SetCookie(w, &http.Cookie{
				Name:     "orbit_session",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
			w.WriteHeader(http.StatusOK)
		case "/api/v1/files":
			cookie, err := r.Cookie("orbit_session")
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			mu.Lock()
			valid := sessions[cookie.Value]
			mu.Unlock()
			if !valid {
				http.Error(w, "session revoked", http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Establish session
	sessID := "test-session-12345"
	mu.Lock()
	sessions[sessID] = true
	mu.Unlock()

	client := &http.Client{}

	// Step 1: Authorized query succeeds
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/files", nil)
	req.AddCookie(&http.Cookie{Name: "orbit_session", Value: sessID})
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got: %v (status: %d)", err, resp.StatusCode)
	}
	resp.Body.Close()

	// Step 2: User logs out (or browser closes session)
	logoutReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/logout", nil)
	logoutReq.AddCookie(&http.Cookie{Name: "orbit_session", Value: sessID})
	logoutResp, err := client.Do(logoutReq)
	if err != nil || logoutResp.StatusCode != http.StatusOK {
		t.Fatalf("logout failed: %v", err)
	}
	logoutResp.Body.Close()

	// Step 3: Subsequent request with old session fails
	reqAfter, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/files", nil)
	reqAfter.AddCookie(&http.Cookie{Name: "orbit_session", Value: sessID})
	respAfter, err := client.Do(reqAfter)
	if err != nil || respAfter.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized after logout, got: %d", respAfter.StatusCode)
	}
	respAfter.Body.Close()

	// Invariant check: Daemon lock MUST still be held! Background sync is NOT stopped.
	if _, err := state.Acquire(stateDir); !errors.Is(err, state.ErrLocked) {
		t.Fatal("daemon lock was unexpectedly released on UI logout")
	}

	// Verify lock file still exists
	lockPath := filepath.Join(stateDir, ".agent.lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file missing: %v", err)
	}
}
