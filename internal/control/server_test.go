package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/workspace"
)

func setupTestServer(t *testing.T) (*Server, string, *Controller) {
	ctx := context.Background()
	stateDir := t.TempDir()
	if err := os.Chmod(stateDir, 0700); err != nil {
		t.Fatal(err)
	}

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ws := workspace.New(db, workspace.Options{})
	ctrl := New(db, ws)

	srv, err := NewServer(ctrl, stateDir)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		_ = srv.Serve(listener)
	}()
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	baseURL := fmt.Sprintf("http://%s", listener.Addr().String())
	return srv, baseURL, ctrl
}

func TestServerSecurityAndBootstrapFlow(t *testing.T) {
	srv, baseURL, ctrl := setupTestServer(t)
	ctx := context.Background()

	// Register a test folder
	root := filepath.Join(t.TempDir(), "ws-root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	var folder history.ID
	folder[0] = 0x11
	var author history.ID
	author[0] = 0x22
	_ = ctrl.db.EnsureFolder(ctx, folder, author, 1)
	_, _ = ctrl.RegisterFolder(ctx, folder, root)

	client := &http.Client{}

	// 1. Unauthenticated sensitive read MUST fail with 401 Unauthorized
	resp, err := client.Get(baseURL + "/api/v1/folders")
	if err != nil {
		t.Fatalf("get folders: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated sensitive read, got %d", resp.StatusCode)
	}

	// 2. DNS rebinding protection: Host header mismatch MUST fail with 400 Bad Request
	req, _ := http.NewRequest("GET", baseURL+"/api/v1/health", nil)
	req.Host = "evil.attacker.com"
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("do invalid host req: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for external Host header, got %d", resp.StatusCode)
	}

	// 3. Browser cross-origin protection: Origin header from external site MUST fail with 403 Forbidden
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/folders/pause", strings.NewReader(`{"folder":"`+fullID(folder)+`"}`))
	req.Header.Set("Origin", "http://evil.com")
	req.Header.Set("Authorization", "Bearer "+srv.CLIToken())
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("do cross-origin req: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for cross-origin browser request, got %d", resp.StatusCode)
	}

	// 4. CLI Authentication via Bearer token MUST succeed on sensitive reads & mutations
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/folders", nil)
	req.Header.Set("Authorization", "Bearer "+srv.CLIToken())
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("get folders with CLI token: %v", err)
	}
	var folders []repository.FolderRecord
	if err := json.NewDecoder(resp.Body).Decode(&folders); err != nil {
		t.Fatalf("decode folders: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(folders) != 1 {
		t.Errorf("expected 200 OK and 1 folder, got status=%d len=%d", resp.StatusCode, len(folders))
	}

	// 5. Browser Bootstrap flow:
	// a. Generate bootstrap token
	bootToken := srv.GenerateBootstrapToken()

	// b. Exchange bootstrap token at /api/v1/auth/bootstrap
	bootBody, _ := json.Marshal(BootstrapRequest{Token: bootToken})
	resp, err = client.Post(baseURL+"/api/v1/auth/bootstrap", "application/json", bytes.NewReader(bootBody))
	if err != nil {
		t.Fatalf("post bootstrap: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for bootstrap, got %d", resp.StatusCode)
	}

	var bootRes BootstrapResult
	_ = json.NewDecoder(resp.Body).Decode(&bootRes)
	resp.Body.Close()

	if bootRes.SessionID == "" || bootRes.CSRFToken == "" {
		t.Fatalf("empty session or csrf token: %+v", bootRes)
	}

	// Check that cookie was set
	cookies := resp.Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "orbit_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value != bootRes.SessionID || !sessionCookie.HttpOnly {
		t.Fatalf("invalid session cookie: %+v", sessionCookie)
	}

	// c. Bootstrap token is ONE-USE: replay MUST fail with 401 Unauthorized
	respReplay, err := client.Post(baseURL+"/api/v1/auth/bootstrap", "application/json", bytes.NewReader(bootBody))
	if err != nil {
		t.Fatalf("post bootstrap replay: %v", err)
	}
	respReplay.Body.Close()
	if respReplay.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for replayed one-use bootstrap token, got %d", respReplay.StatusCode)
	}

	// 6. Browser Session Read: reading with session cookie MUST succeed without CSRF token
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/doctor", nil)
	req.AddCookie(sessionCookie)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("get doctor with session: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for session read, got %d", resp.StatusCode)
	}

	// 7. Browser Session Mutation WITHOUT CSRF token MUST fail with 403 Forbidden
	pauseBody := []byte(`{"folder":"` + fullID(folder) + `","reason":"TEST_PAUSE"}`)
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/folders/pause", bytes.NewReader(pauseBody))
	req.AddCookie(sessionCookie)
	// Notice: NO X-CSRF-Token header!
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("do mutation without csrf: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for mutating request without CSRF token, got %d", resp.StatusCode)
	}

	// 8. Browser Session Mutation WITH valid CSRF token MUST succeed!
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/folders/pause", bytes.NewReader(pauseBody))
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", bootRes.CSRFToken)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("do mutation with csrf: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for mutation with valid CSRF, got %d", resp.StatusCode)
	}

	// Verify mutation actually occurred in Controller!
	flist, _ := ctrl.Folders(ctx)
	if !flist[0].Paused || flist[0].PauseReason != "TEST_PAUSE" {
		t.Errorf("expected folder to be paused, got: %+v", flist[0])
	}
}
