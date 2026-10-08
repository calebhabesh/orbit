// Package terminal_test holds opt-in terminal baseline reproductions. These
// assert the approved behavior, so known gaps deliberately fail until their
// owning packet promotes them to ordinary passing regressions.
package terminal_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/app"
	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/control"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/replication"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/state"
	"github.com/calebhabesh/orbit/internal/testkit"
	"github.com/calebhabesh/orbit/internal/workspace"
)

func baselineOnly(t *testing.T) {
	t.Helper()
	if os.Getenv("ORBIT_TERMINAL_BASELINE") != "1" {
		t.Skip("deliberately failing baseline; opt in with ORBIT_TERMINAL_BASELINE=1")
	}
}

type fixture struct {
	t      *testing.T
	root   string
	state  string
	device history.ID
	db     *repository.DB
	ws     *workspace.Workspace
	ctrl   *control.Controller
	server *httptest.Server
	token  string
	lock   *state.Lock
}

func fresh(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, root: testkit.NewDisposable(t)}
	f.state = filepath.Join(f.root, "state")
	cfg, err := app.Initialize(context.Background(), f.state, app.SystemDependencies())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.device.UnmarshalText([]byte(cfg.DeviceID)); err != nil {
		t.Fatal(err)
	}
	f.open()
	t.Cleanup(f.close)
	return f
}

func (f *fixture) open() {
	f.t.Helper()
	var err error
	f.lock, err = state.Acquire(f.state)
	if err != nil {
		f.t.Fatal(err)
	}
	f.db, err = repository.Open(context.Background(), f.state)
	if err != nil {
		f.t.Fatal(err)
	}
	f.ws = workspace.New(f.db, workspace.Options{})
	f.ctrl = control.New(f.db, f.ws, control.Options{LocalDevice: f.device})
	s, err := control.NewServer(f.ctrl, f.state)
	if err != nil {
		f.t.Fatal(err)
	}
	f.token = s.CLIToken()
	f.server = httptest.NewServer(s.Handler())
}

func (f *fixture) close() {
	if f.server != nil {
		f.server.Close()
		f.server = nil
	}
	if f.db != nil {
		_ = f.db.Close()
		f.db = nil
	}
	if f.lock != nil {
		_ = f.lock.Close()
		f.lock = nil
	}
}

// Never print request/response bodies: invitation and local credentials may
// occur in them. Callers log only the sanitized assertion and status code.
func api(t *testing.T, endpoint, token, method, path string, in, out any) int {
	t.Helper()
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, endpoint+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal("HTTP fixture request failed")
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
			t.Fatalf("HTTP %d: response did not decode", resp.StatusCode)
		}
	}
	return resp.StatusCode
}

func (f *fixture) call(path string, in, out any) int {
	f.t.Helper()
	return api(f.t, f.server.URL, f.token, http.MethodPost, path, in, out)
}

func (f *fixture) folder(name string) history.ID {
	f.t.Helper()
	var result control.StartSetupResult
	status := f.call("/api/v1/setup/start", control.StartSetupRequest{RootPath: filepath.Join(f.root, name), WorkspaceName: name}, &result)
	if status != http.StatusOK || result.Phase != "completed" {
		f.t.Fatalf("fixture setup failed: HTTP %d", status)
	}
	return result.FolderID
}

func (f *fixture) invitation(folder history.ID) control.CreateInvitationResult {
	f.t.Helper()
	var result control.CreateInvitationResult
	if status := f.call("/api/v1/invitations", control.CreateInvitationRequest{Folder: folder}, &result); status != http.StatusOK {
		f.t.Fatalf("fixture invitation failed: HTTP %d", status)
	}
	return result
}

func signed(t *testing.T, private ed25519.PrivateKey, token string, folder history.ID) control.SubmitJoinRequestPayload {
	t.Helper()
	public := private.Public().(ed25519.PublicKey)
	challenge := []byte("synthetic-terminal-baseline-challenge")
	return control.SubmitJoinRequestPayload{Token: token, PublicKey: hex.EncodeToString(public), JoiningDevice: sha256.Sum256(public), Challenge: hex.EncodeToString(challenge), Signature: hex.EncodeToString(ed25519.Sign(private, challenge)), TargetFolder: folder, SuggestedLabel: "Synthetic joining device"}
}

func privateKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return private
}

func TestTerminalT00WrongFolderCapability(t *testing.T) {
	baselineOnly(t)
	f := fresh(t)
	a, b := f.folder("a"), f.folder("b")
	inv := f.invitation(a)
	var result control.SubmitJoinRequestResult
	status := api(t, f.server.URL, "", http.MethodPost, "/api/v1/enrollment/request", signed(t, privateKey(t), inv.Token, b), &result)
	if status == http.StatusOK {
		record, err := f.db.GetEnrollmentRequest(context.Background(), result.RequestID)
		if err != nil || record.Folder != b || record.Status != "pending" {
			t.Fatal("fixture did not record the mismatched request")
		}
		t.Error("scope gap: folder A capability accepted and consumed for folder B; request remains pending (no data-access claim)")
	}
}

func TestTerminalT00SameKeySecondFolder(t *testing.T) {
	baselineOnly(t)
	f := fresh(t)
	a, b := f.folder("a"), f.folder("b")
	key := privateKey(t)
	first, second := f.invitation(a), f.invitation(b)
	var result control.SubmitJoinRequestResult
	if status := api(t, f.server.URL, "", http.MethodPost, "/api/v1/enrollment/request", signed(t, key, first.Token, a), &result); status != http.StatusOK {
		t.Fatalf("first request failed: HTTP %d", status)
	}
	var failure control.ControlError
	status := api(t, f.server.URL, "", http.MethodPost, "/api/v1/enrollment/request", signed(t, key, second.Token, b), &failure)
	if status != http.StatusOK {
		if !strings.Contains(failure.Message, "UNIQUE constraint failed: enrollment_requests.request_id") {
			t.Fatalf("unexpected second request failure: HTTP %d code=%s", status, failure.Code)
		}
		invRecord, err := f.db.GetInvitation(context.Background(), second.Digest)
		if err != nil || invRecord.UsesCount != 1 {
			t.Fatal("fixture could not inspect consumed second invitation")
		}
		t.Errorf("request identity gap: same persistent key's second-folder request returns HTTP %d, global request_id UNIQUE collision; second token already consumed", status)
	}
}

func TestTerminalT00InviterVerification(t *testing.T) {
	baselineOnly(t)
	f := fresh(t)
	var folder history.ID
	folder[0] = 1
	// This self-signed TLS server is deliberately unrelated to any enrolled
	// inviter. Count disclosure without logging the synthetic capability.
	disclosed := make(chan bool, 1)
	remote := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload control.SubmitJoinRequestPayload
		_ = json.NewDecoder(r.Body).Decode(&payload)
		disclosed <- payload.Token == "synthetic-capability"
		_ = json.NewEncoder(w).Encode(control.SubmitJoinRequestResult{RequestID: "synthetic-request", Status: "pending"})
	}))
	defer remote.Close()
	var result control.JoinFlowSubmitResult
	status := f.call("/api/v1/orbit/setup/join/submit", control.JoinFlowSubmitRequest{InvitationToken: "synthetic-capability", TargetFolder: hex.EncodeToString(folder[:]), RemoteEndpoint: remote.URL, RootPath: filepath.Join(f.root, "joining")}, &result)
	select {
	case sent := <-disclosed:
		if sent {
			t.Errorf("identity gap: invitation secret disclosed to unrelated self-signed HTTPS server; submit HTTP %d", status)
		}
	default:
		if status == http.StatusOK {
			t.Fatal("fixture got success without remote submission")
		}
	}
}

func TestTerminalT00JoinPersistence(t *testing.T) {
	baselineOnly(t)
	inviter, joiner := fresh(t), fresh(t)
	folder := inviter.folder("shared")
	inv := inviter.invitation(folder)
	root := filepath.Join(joiner.root, "joining")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	var submitted control.JoinFlowSubmitResult
	if status := joiner.call("/api/v1/orbit/setup/join/submit", control.JoinFlowSubmitRequest{InvitationToken: inv.Token, TargetFolder: hex.EncodeToString(folder[:]), RemoteEndpoint: inviter.server.URL, RootPath: root}, &submitted); status != http.StatusOK {
		t.Fatalf("join submit failed: HTTP %d", status)
	}
	identityBefore, err := os.ReadFile(filepath.Join(joiner.state, "identity", "peer-identity.pem"))
	if err != nil {
		t.Fatal(err)
	}
	joiner.close()
	joiner.open()
	st, err := joiner.db.GetSetupState(context.Background())
	if err != nil || st == nil || st.Phase != "joining" || st.RootPath != root {
		t.Fatal("joining root/phase did not survive reopen")
	}
	identityAfter, err := os.ReadFile(filepath.Join(joiner.state, "identity", "peer-identity.pem"))
	if err != nil || !bytes.Equal(identityBefore, identityAfter) {
		t.Fatal("joining identity changed after reopen")
	}
	var resumed control.ResumeSetupResult
	status := joiner.call("/api/v1/setup/resume", control.ResumeSetupRequest{}, &resumed)
	t.Logf("reopened joining workflow: resume HTTP %d, completed=%v", status, resumed.Completed)
	request, err := inviter.db.GetEnrollmentRequest(context.Background(), submitted.RequestID)
	if err != nil || request.Status != "pending" {
		t.Fatal("fixture request is no longer awaiting approval")
	}
	if status != http.StatusOK {
		t.Fatalf("existing-root resume fixture failed: HTTP %d", status)
	}
	if resumed.Completed {
		t.Error("durable onboarding gap: reopened pending join resumes as completed without querying exact request/endpoint; inviter still pending")
	}
}

func TestTerminalT00RootPreview(t *testing.T) {
	baselineOnly(t)
	f := fresh(t)
	root := filepath.Join(f.root, "existing")
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "document"), bytes.Repeat([]byte("x"), 4096), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("nested/document", filepath.Join(root, "unsupported-link")); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if status := f.call("/api/v1/setup/preview-root", control.PreviewCreateRootRequest{Path: root}, &result); status != http.StatusOK {
		t.Fatalf("preview failed: HTTP %d", status)
	}
	t.Logf("preview immediate rows=%v; synthetic tree has 1 regular file, 4096 bytes and 1 unsupported symlink", result["preexisting_rows"])
	// Field spelling will be frozen in T01; today's result has none of these
	// observations under any name, rather than a mismatch to a frozen schema.
	for _, name := range []string{"total_bytes", "unsupported_items", "incomplete", "review_generation", "available_capacity"} {
		if _, ok := result[name]; !ok {
			t.Errorf("root review gap: no %s observation", name)
		}
	}
}

func TestTerminalT00JoinReadiness(t *testing.T) {
	baselineOnly(t)
	inviter, joiner := fresh(t), fresh(t)
	folder := inviter.folder("shared")
	inv := inviter.invitation(folder)
	root := filepath.Join(joiner.root, "joining")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(root, "unsupported-link")); err != nil {
		t.Fatal(err)
	}
	var submitted control.JoinFlowSubmitResult
	if status := joiner.call("/api/v1/orbit/setup/join/submit", control.JoinFlowSubmitRequest{InvitationToken: inv.Token, TargetFolder: hex.EncodeToString(folder[:]), RemoteEndpoint: inviter.server.URL, RootPath: root}, &submitted); status != http.StatusOK {
		t.Fatalf("join submit failed: HTTP %d", status)
	}
	var approved control.ApproveEnrollmentResult
	if status := inviter.call("/api/v1/enrollment/approve", control.ApproveEnrollmentRequest{RequestID: submitted.RequestID, Folder: folder}, &approved); status != http.StatusOK {
		t.Fatalf("join approve failed: HTTP %d", status)
	}
	var completed control.JoinFlowCompleteResult
	status := joiner.call("/api/v1/orbit/setup/join/complete", control.JoinFlowCompleteRequest{RequestID: submitted.RequestID, RemoteEndpoint: inviter.server.URL, TargetFolder: hex.EncodeToString(folder[:]), RootPath: root}, &completed)
	scan, err := joiner.ws.Scan(context.Background(), folder)
	if err != nil || len(scan.Issues) != 1 || scan.Issues[0].Code != "UNSUPPORTED_OBJECT" {
		t.Fatal("fixture did not produce the intended scan issue")
	}
	if _, err := os.Lstat(filepath.Join(root, "unsupported-link")); err != nil {
		t.Fatal("preexisting unsupported entry lost")
	}
	if status == http.StatusOK && completed.Completed && completed.Status == "ready" {
		t.Error("readiness gap: join reports ready/completed despite UNSUPPORTED_OBJECT scan issue; existing link preserved")
	}
}

func TestTerminalT00PeerEnrollmentRoute(t *testing.T) {
	baselineOnly(t)
	f := fresh(t)
	identity, err := replication.LoadOrCreateIdentity(f.state, f.device, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	peer := replication.NewServer(f.db, identity)
	server := httptest.NewUnstartedServer(peer)
	server.TLS = identity.ServerTLSConfig()
	server.StartTLS()
	defer server.Close()
	tlsConfig, err := identity.ClientTLSConfig(identity.Leaf, identity.KeyPin)
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	resp, err := client.Post(server.URL+"/api/v1/enrollment/request", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal("pinned mutual TLS fixture request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		t.Error("transport gap: actual peer handler returns HTTP 404 for enrollment even with pinned mutual TLS; unknown requesters also require client certificates")
	}
}

func TestTerminalT00InvitationEndpoint(t *testing.T) {
	baselineOnly(t)
	f := fresh(t)
	if err := os.WriteFile(filepath.Join(f.state, "control.addr"), []byte(strings.TrimPrefix(f.server.URL, "http://")), 0600); err != nil {
		t.Fatal(err)
	}
	inv := f.invitation(f.folder("shared"))
	u, err := url.Parse(inv.InvitationCode)
	if err != nil {
		t.Fatal("invitation parse failed")
	}
	endpoint := u.Query().Get("endpoint")
	if endpoint == strings.TrimPrefix(f.server.URL, "http://") {
		t.Error("transport gap: invitation defaults to loopback owner-control address (not reachable from another host)")
	}
	if u.Query().Get("key_pin") == "" && u.Query().Get("certificate") == "" {
		t.Error("identity gap: current invitation carries no inviting key/certificate binding")
	}
}

func TestTerminalT00DetailObservation(t *testing.T) {
	baselineOnly(t)
	f := fresh(t)
	folder := f.folder("shared")
	if err := os.WriteFile(filepath.Join(f.root, "shared", "document"), []byte("synthetic saved content"), 0600); err != nil {
		t.Fatal(err)
	}
	scan, err := f.ws.Scan(context.Background(), folder)
	if err != nil || len(scan.Captured) != 1 {
		t.Fatal("fixture did not capture document")
	}
	var peer history.ID
	peer[0] = 7
	if err := f.db.RecordPeerReceiptWithOptions(context.Background(), folder, peer, scan.Captured[0].ID, false, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	progress, err := f.db.PeerProgress(context.Background(), folder)
	if err != nil || len(progress) != 1 || progress[0].Direct {
		t.Fatal("fixture did not record indirect observation")
	}
	var details map[string]any
	path := "/api/v1/browse/details?folder=" + hex.EncodeToString(folder[:]) + "&path=document"
	if status := api(t, f.server.URL, f.token, http.MethodGet, path, nil, &details); status != http.StatusOK {
		t.Fatalf("detail request failed: HTTP %d", status)
	}
	peers, ok := details["peers"].([]any)
	if !ok || len(peers) != 1 {
		t.Fatal("detail result lost entire peer record")
	}
	summary := peers[0].(map[string]any)
	if _, ok := summary["direct"]; !ok {
		t.Error("status gap: detail API drops persisted direct=false observation while retaining receipt and last_contact_ns")
	}
}

func TestTerminalT00CLI(t *testing.T) {
	baselineOnly(t)
	root := testkit.NewDisposable(t)
	binary := filepath.Join(root, "orbit")
	build := exec.Command("go", "build", "-o", binary, "./cmd/orbit")
	build.Dir = filepath.Join("..", "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fixture binary: %v: %s", err, output)
	}
	stateDir := filepath.Join(root, "state")
	// Exact default launcher arguments. Keep the child handle for safe graceful
	// shutdown; never stop a discovered/personal daemon or invoke user systemd.
	cmd := exec.Command(binary, "serve", "--state="+stateDir, "--control-listen=127.0.0.1:0", "--allow-init")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		if err := testkit.ValidateDestructiveTarget(root, stateDir); err != nil {
			t.Error(err)
			return
		}
		// Process is the exact direct child handle started above. Validate its
		// executable and state arguments before signaling it.
		args, err := os.ReadFile(filepath.Join("/proc", strings.TrimSpace(string(mustRead(t, filepath.Join(stateDir, ".agent.pid")))), "cmdline"))
		if err != nil || !bytes.Contains(args, []byte(binary+"\x00")) || !bytes.Contains(args, []byte("--state="+stateDir+"\x00")) {
			t.Error("refusing shutdown: child identity validation failed")
			return
		}
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("disposable child did not shut down gracefully")
		}
	})
	endpoint := ""
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(filepath.Join(stateDir, "control.addr"))
		if err == nil {
			endpoint = "http://" + strings.TrimSpace(string(b))
			resp, err := (&http.Client{Timeout: 200 * time.Millisecond}).Get(endpoint + "/api/v1/health")
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					break
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if endpoint == "" {
		t.Fatal("child never produced control address")
	}
	token := strings.TrimSpace(string(mustRead(t, filepath.Join(stateDir, "control.token"))))
	t.Run("FiniteInitialization", func(t *testing.T) {
		limits, err := config.LoadStorageLimits(stateDir)
		if err != nil {
			t.Fatal(err)
		}
		if limits.DataBudgetBytes == 0 || limits.MetadataBudgetBytes == 0 || limits.FreeSpaceReserveBytes == 0 {
			t.Error("initialization gap: default launcher serve --allow-init creates no limits.json; all loaded budgets/reserve are zero")
		}
	})
	t.Run("LiveAdapters", func(t *testing.T) {
		for _, args := range [][]string{
			{"setup", "--preview", "--state", stateDir, "--root", filepath.Join(root, "adopt"), "--json"},
			{"join", "--state", stateDir, "--root", filepath.Join(root, "join"), "--timeout", "0"},
			{"conflicts", "--state", stateDir, "--folder", strings.Repeat("1", 64), "--json"},
			{"restore", "--state", stateDir, "--folder", strings.Repeat("1", 64), "--path", "document", "--source", strings.Repeat("2", 64) + ":1", "--preview", "--json"},
			{"conflicts", "merge", "--state", stateDir, "--folder", strings.Repeat("1", 64), "--path", "document", "--content", "synthetic merge", "--json"},
		} {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			output, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
			cancel()
			if err != nil && strings.Contains(string(output), "state directory is already owned by another agent") {
				t.Errorf("adapter gap: live-daemon orbit %s enters stopped-state lock and fails", args[0])
			} else if err != nil {
				t.Fatalf("fixture orbit %s failed for another reason: %s", args[0], output)
			}
		}
	})
	t.Run("Status", func(t *testing.T) {
		var setup control.StartSetupResult
		status := api(t, endpoint, token, http.MethodPost, "/api/v1/setup/start", control.StartSetupRequest{RootPath: filepath.Join(root, "shared")}, &setup)
		if status != http.StatusOK {
			t.Fatalf("live HTTP setup failed: %d", status)
		}
		output, err := exec.Command(binary, "status", "--state", stateDir, "--json").Output()
		if err != nil {
			t.Fatal(err)
		}
		var result map[string]any
		if err := json.Unmarshal(output, &result); err != nil {
			t.Fatal(err)
		}
		if result["daemon_running"] != true {
			t.Fatal("status did not observe running daemon")
		}
		for _, field := range []string{"setup", "folders", "attention"} {
			if _, ok := result[field]; !ok {
				t.Errorf("status gap: running-daemon JSON omits %s despite live HTTP setup succeeding", field)
			}
		}
	})
	t.Run("DeletedCommand", func(t *testing.T) {
		output, err := exec.Command(binary, "deleted", "--state", stateDir, "--json").CombinedOutput()
		if err != nil && strings.Contains(string(output), "unknown orbit command") {
			t.Error("command gap: orbit deleted is unavailable")
		} else if err != nil {
			t.Fatalf("fixture failed for another reason: %s", output)
		}
	})
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
