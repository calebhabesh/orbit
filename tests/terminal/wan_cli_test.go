package terminal_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/app"
	"github.com/calebhabesh/file-sync/internal/config"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/controlclient"
	"github.com/calebhabesh/file-sync/internal/testkit"
)

type w06CLI struct {
	t          *testing.T
	binary, ca string
}

func (c w06CLI) call(input string, args ...string) (tc.Result, string, error) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.binary, args...)
	cmd.Env = append(os.Environ(), "SSL_CERT_FILE="+c.ca)
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	var r tc.Result
	_ = json.Unmarshal(stdout.Bytes(), &r)
	return r, stdout.String() + stderr.String(), err
}
func (c w06CLI) ok(args ...string) tc.Result {
	c.t.Helper()
	r, out, err := c.call("", args...)
	if err != nil {
		c.t.Fatalf("CLI %v: %v\n%s", args, err, out)
	}
	return r
}
func w06Wait(t *testing.T, dir, operation string) tc.Result {
	t.Helper()
	deadline := time.Now().Add(120 * time.Second)
	var r tc.Result
	for time.Now().Before(deadline) {
		var err error
		r, err = (&controlclient.Client{StateDir: dir}).Query(context.Background(), tc.Query{Version: tc.Version, Kind: "operation", ID: operation})
		if err != nil {
			t.Fatal(err)
		}
		if r.State == "completed" {
			return r
		}
		if r.Error != nil && !r.Error.Retryable {
			t.Fatalf("operation failed: %+v", r.Error)
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("operation did not complete: %+v", r)
	return r
}
func TestWANW06BinaryReviewedRelayJourney(t *testing.T) { w06BinaryReviewedRelayJourney(t, "", false) }

func w06BinaryReviewedRelayJourney(t *testing.T, optionalListen string, firstFolderOnly bool, optionalUDPListen ...string) {
	w06BinaryJourneyWithFollowup(t, optionalListen, firstFolderOnly, optionalUDPListen, nil)
}

func w06BinaryJourneyWithFollowup(t *testing.T, optionalListen string, firstFolderOnly bool, optionalUDPListen []string, followup func(w06CLI, []string, string, string, tc.Invitation, *atomic.Int64)) {
	base := testkit.NewDisposable(t)
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	var serviceRequests atomic.Int64
	service, selection, origin, roots, _ := w05Service(t, func() { serviceRequests.Add(1) })
	conn, err := tls.Dial("tcp", strings.TrimPrefix(origin, "https://"), &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13})
	if err != nil {
		t.Fatal(err)
	}
	ca := filepath.Join(base, "service-ca.pem")
	err = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: conn.ConnectionState().PeerCertificates[0].Raw}), 0600)
	_ = conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	cli := w06CLI{t: t, binary: buildOrbitBinary(t, base), ca: ca}
	profile := filepath.Join(base, "profile.json")
	data, _ := json.Marshal(selection)
	if err = os.WriteFile(profile, data, 0600); err != nil {
		t.Fatal(err)
	}
	states := []string{filepath.Join(base, "a"), filepath.Join(base, "b")}
	for _, dir := range states {
		dir := dir
		t.Cleanup(func() {
			if err := testkit.ValidateDestructiveTarget(base, dir); err != nil {
				t.Error(err)
				return
			}
			_ = app.StopAgent(dir, 10*time.Second)
		})
		review := dir + "-network.json"
		cli.ok("network", "preview", "--state", dir, "--mode", "self_hosted", "--profile-file", profile, "--review-file", review, "--json")
		cli.ok("network", "apply", "--state", dir, "--review-file", review, "--json")
		if optionalListen != "" {
			settings := map[string]any{"interfaces": []string{}, "listen": optionalListen, "disabled": false, "udp_disabled": true}
			if len(optionalUDPListen) > 0 {
				settings["udp_disabled"] = false
				settings["udp_listen"] = optionalUDPListen[0]
			}
			b, _ := json.Marshal(settings)
			if err = config.WritePrivate(dir, "direct-network.json", b); err != nil {
				t.Fatal(err)
			}
		}
	}
	rootA, rootB := filepath.Join(base, "documents-a"), filepath.Join(base, "documents-b")
	if err = os.Mkdir(rootA, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(rootA, "from-a.txt"), []byte("verified A bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	setupFile := filepath.Join(base, "create.json")
	cli.ok("setup", "--state", states[0], "--root", rootA, "--label", "Laptop", "--name", "Documents", "--preview", "--review-file", setupFile, "--json")
	created := cli.ok("setup", "--state", states[0], "--request-file", setupFile, "--timeout", "0", "--json")
	w06Wait(t, states[0], created.Operation.ID)

	// Retained invite vocabulary follows the reviewed policy and labels v3
	// correctly; its JSON remains a deliberate invitation transfer API.
	_, legacyTransfer, e := cli.call("", "invite", "create", "--state", states[0], "--folder", created.Join.Folder, "--json")
	if e != nil {
		t.Fatalf("legacy routed invitation failed: %v", e)
	}
	var legacy struct {
		InvitationCode string `json:"invitation_code"`
	}
	if e = json.Unmarshal([]byte(legacyTransfer), &legacy); e != nil || !strings.HasPrefix(legacy.InvitationCode, "orbit-invitation:v3:") {
		t.Fatal("legacy invitation did not explicitly label v3")
	}
	beforeA, _ := config.Load(states[0])
	beforeB, _ := config.Load(states[1])
	inviteReview, invFile := filepath.Join(base, "invite-review.json"), filepath.Join(base, "invitation.json")
	cli.ok("devices", "invite", "--state", states[0], "--folder", "Documents", "--preview", "--review-file", inviteReview, "--json")
	invResult, out, err := cli.call("", "devices", "invite", "--state", states[0], "--request-file", inviteReview, "--out", invFile, "--json")
	if err != nil {
		t.Fatal(err, out)
	}
	if invResult.Invitation != nil {
		t.Fatal("normal JSON exposed capability")
	}
	data, err = os.ReadFile(invFile)
	if err != nil {
		t.Fatal(err)
	}
	var inv tc.Invitation
	if err = json.Unmarshal(data, &inv); err != nil {
		t.Fatal(err)
	}
	if inv.Version != "3" || inv.Route == nil {
		t.Fatal("expected v3 logical invitation")
	}
	if strings.Contains(out, inv.Capability) {
		t.Fatal("capability leaked")
	}
	info, _ := os.Stat(invFile)
	if info.Mode().Perm() != 0600 {
		t.Fatal("invitation is not private")
	}

	// Invalid trust and expired fresh authorization fail without exposing the secret
	// or creating an apply artifact. Root/name inputs remain available for retry.
	for _, kind := range []string{"wrong-pin", "expired"} {
		bad := inv
		if kind == "wrong-pin" {
			bad.KeyPin = strings.Repeat("0", 64)
			route := *bad.Route
			route.Pin = bad.KeyPin
			bad.Route = &route
		} else {
			bad.ExpiresAt = time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano)
		}
		path := filepath.Join(base, kind+".json")
		b, _ := json.Marshal(bad)
		if e := os.WriteFile(path, b, 0600); e != nil {
			t.Fatal(e)
		}
		rejected := filepath.Join(base, kind+"-review.json")
		_, printed, e := cli.call("", "join", "--state", states[1], "--root", rootB, "--label", "Pi", "--preview", "--invitation-file", path, "--review-file", rejected, "--json")
		if e == nil || strings.Contains(printed, inv.Capability) {
			t.Fatal("bad invitation accepted or disclosed", kind, e, printed)
		}
		if _, e = os.Stat(rejected); !os.IsNotExist(e) {
			t.Fatal("bad invitation wrote apply artifact")
		}
	}
	joinFile := filepath.Join(base, "join.json")
	cli.ok("join", "--state", states[1], "--root", rootB, "--label", "Pi", "--name", "Documents", "--invitation-file", invFile, "--preview", "--review-file", joinFile, "--json")
	joined := cli.ok("join", "--state", states[1], "--request-file", joinFile, "--timeout", "0", "--json")
	var pending tc.Result
	deadline := time.Now().Add(70 * time.Second)
	for time.Now().Before(deadline) {
		pending = cli.ok("devices", "requests", "--state", states[0], "--json")
		if len(pending.Requests) == 1 {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if len(pending.Requests) != 1 || pending.Requests[0].Label != "Pi" || pending.Requests[0].VerificationCode == "" {
		t.Fatalf("exact approval missing: %+v", pending.Requests)
	}
	// Closing/relaunching the CLI retains its request and root without approval.
	resumed := cli.ok("join", "--state", states[1], "--request-file", joinFile, "--timeout", "0", "--json")
	if resumed.Operation.ID != joined.Operation.ID || resumed.Join.Root != rootB || resumed.State == "completed" {
		t.Fatal("delayed approval did not retain operation")
	}
	approvalFile := filepath.Join(base, "approve.json")
	cli.ok("devices", "requests", "show", "--state", states[0], "--device", "Pi", "--review-file", approvalFile, "--json")
	cli.ok("devices", "approve", "--state", states[0], "--review-file", approvalFile, "--json")
	w06Wait(t, states[1], joined.Operation.ID)
	waitBytes := func(path, want string) {
		t.Helper()
		until := time.Now().Add(90 * time.Second)
		for time.Now().Before(until) {
			b, _ := os.ReadFile(path)
			if string(b) == want {
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Fatalf("missing verified bytes %s", path)
	}
	waitBytes(filepath.Join(rootB, "from-a.txt"), "verified A bytes")
	if err = os.WriteFile(filepath.Join(rootB, "from-b.txt"), []byte("verified B bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	waitBytes(filepath.Join(rootA, "from-b.txt"), "verified B bytes")
	afterA, _ := config.Load(states[0])
	afterB, _ := config.Load(states[1])
	if afterA.DeviceID != beforeA.DeviceID || afterB.DeviceID != beforeB.DeviceID {
		t.Fatal("identity changed")
	}
	status := cli.ok("network", "status", "--state", states[1], "--json")
	if status.Network == nil || len(status.Network.Observations) == 0 {
		t.Fatal("missing route observations")
	}
	for _, o := range status.Network.Observations {
		if o.Route != "relay" {
			t.Fatalf("expected actual relay observation: %+v", o)
		}
	}
	// Exact head identities and authenticated inviter certificate remain observable.
	historyResult, err := (&controlclient.Client{StateDir: states[1]}).Query(context.Background(), tc.Query{Version: tc.Version, Kind: "history", Folder: inv.Folder, Path: "from-a.txt", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(historyResult.Versions) != 1 || historyResult.Versions[0].Version.Author != inv.Inviter {
		t.Fatalf("incorrect heads: %+v", historyResult.Versions)
	}
	routes, err := config.LoadPeerRoutes(states[1])
	if err != nil || len(routes) != 1 || routes[0].CertificateDER != inv.CertificateDER || routes[0].Pin != inv.KeyPin {
		t.Fatal("inviter trust was not persisted", err)
	}

	if followup != nil {
		followup(cli, states, rootA, rootB, inv, &serviceRequests)
	}
	if firstFolderOnly {
		t.Log("W08 occupied optional listeners: actual fresh CLI invitation/request/exact approval and verified two-way bytes through reviewed relay; no manual peer IPs; persistent inviter certificate and author identity")
		return
	}

	// A second folder reuses the same identities and operator but has its own
	// invitation, approval artifact and independently reviewed receiving root.
	secondA, secondB := filepath.Join(base, "archive-a"), filepath.Join(base, "archive-b")
	if err = os.Mkdir(secondA, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(secondA, "archive.txt"), []byte("separate folder"), 0600); err != nil {
		t.Fatal(err)
	}
	create2 := filepath.Join(base, "create2.json")
	cli.ok("setup", "--state", states[0], "--root", secondA, "--label", "Laptop", "--name", "Archive", "--preview", "--review-file", create2, "--json")
	r2 := cli.ok("setup", "--state", states[0], "--request-file", create2, "--timeout", "0", "--json")
	w06Wait(t, states[0], r2.Operation.ID)
	invite2 := filepath.Join(base, "invitation2.json")
	cli.ok("devices", "invite", "--state", states[0], "--folder", "Archive", "--out", invite2, "--json")
	join2 := filepath.Join(base, "join2.json")
	// Exercise the private stdin path on production CLI controls.
	input, e := os.ReadFile(invite2)
	if e != nil {
		t.Fatal(e)
	}
	_, printed, e := cli.call(string(input), "join", "--state", states[1], "--root", secondB, "--label", "Pi", "--name", "Archive", "--invitation-stdin", "--preview", "--review-file", join2, "--json")
	if e != nil {
		t.Fatal(e, printed)
	}
	joined2, printed, e := cli.call("", "join", "--state", states[1], "--request-file", join2, "--timeout", "0", "--json")
	if e != nil && (joined2.Error == nil || !joined2.Error.Retryable || joined2.Operation == nil) {
		t.Fatal(e, printed)
	}
	deadline = time.Now().Add(70 * time.Second)
	for time.Now().Before(deadline) {
		pending = cli.ok("devices", "requests", "--state", states[0], "--json")
		if len(pending.Requests) == 1 {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if len(pending.Requests) != 1 {
		t.Fatal("second folder approval missing")
	}
	cli.guided("approve\n", "devices", "approve", "--state", states[0], "--request", pending.Requests[0].ID, "--json")
	w06Wait(t, states[1], joined2.Operation.ID)
	waitBytes(filepath.Join(secondB, "archive.txt"), "separate folder")
	afterA, _ = config.Load(states[0])
	afterB, _ = config.Load(states[1])
	if afterA.DeviceID != beforeA.DeviceID || afterB.DeviceID != beforeB.DeviceID {
		t.Fatal("second folder changed identities")
	}
	// A service outage is separate from local root/capture readiness.
	if err = service.Close(); err != nil {
		t.Fatal(err)
	}
	offlineRoot := filepath.Join(base, "offline-root")
	offlineReview := filepath.Join(base, "offline-create.json")
	cli.ok("setup", "--state", states[0], "--root", offlineRoot, "--name", "Offline", "--label", "Laptop", "--preview", "--review-file", offlineReview, "--json")
	offline := cli.ok("setup", "--state", states[0], "--request-file", offlineReview, "--timeout", "0", "--json")
	w06Wait(t, states[0], offline.Operation.ID)
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status = cli.ok("network", "status", "--state", states[0], "--json")
		if !status.Network.Ready {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if status.Network.Ready || status.Network.Code != "SERVICE_UNAVAILABLE" {
		t.Fatal("service outage claimed ready", status.Network)
	}
	t.Log("Local signed service fixture; production binaries create/invite/join/relaunch/approve and transfer both directions without init/serve/manual peer addresses")
}

func TestWANW06BinaryLocalCaptureAndPrivateInputs(t *testing.T) {
	base := testkit.NewDisposable(t)
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	cli := w06CLI{t: t, binary: buildOrbitBinary(t, base)}
	dir := filepath.Join(base, "state")
	root := filepath.Join(base, "local")
	review := filepath.Join(base, "setup.json")
	t.Cleanup(func() {
		if err := testkit.ValidateDestructiveTarget(base, dir); err != nil {
			t.Error(err)
			return
		}
		_ = app.StopAgent(dir, 10*time.Second)
	})
	cli.ok("setup", "--state", dir, "--root", root, "--label", "Offline laptop", "--name", "Local", "--preview", "--review-file", review, "--json")
	data, err := os.ReadFile(review)
	if err != nil {
		t.Fatal(err)
	}
	var m tc.Mutation
	if err = json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m.Setup.Network == nil || m.Setup.Network.Mode != "automatic" || !m.Setup.Network.AwaitingProfile {
		t.Fatal("fresh default was not reviewed Automatic")
	}
	r := cli.ok("setup", "--state", dir, "--request-file", review, "--timeout", "0", "--json")
	w06Wait(t, dir, r.Operation.ID)
	status := cli.ok("network", "status", "--state", dir, "--json")
	if status.Network.Ready || status.Network.Code != "PROFILE_MISSING_OR_EXPIRED" {
		t.Fatal("missing hosted default claimed ready", status.Network)
	}
	// A plain non-TTY mutation never waits for hidden prompts or exposes argv secrets.
	_, out, err := cli.call("", "setup", "--state", dir, "--root", root)
	if err == nil || !strings.Contains(out, "reviewed --request-file") {
		t.Fatal("non-TTY setup did not require review", err, out)
	}
	secret := "synthetic-private-capability-DO-NOT-PRINT"
	_, out, err = cli.call("", "join", "--state", dir, "--invitation", secret)
	if err == nil || strings.Contains(out, secret) {
		t.Fatal("argv capability accepted/leaked", err, out)
	}
	public := filepath.Join(base, "public-invitation.json")
	if err = os.WriteFile(public, []byte(secret), 0644); err != nil {
		t.Fatal(err)
	}
	_, out, err = cli.call("", "join", "--state", dir, "--root", filepath.Join(base, "join"), "--preview", "--invitation-file", public, "--json")
	if err == nil || strings.Contains(out, secret) {
		t.Fatal("public invitation accepted/leaked", err, out)
	}
	localReview := filepath.Join(base, "local-policy.json")
	cli.ok("network", "preview", "--state", dir, "--mode", "local_only", "--review-file", localReview, "--json")
	cli.ok("network", "apply", "--state", dir, "--review-file", localReview, "--json")
	status = cli.ok("network", "status", "--state", dir, "--json")
	if status.Network.Policy.Mode != "local_only" || status.Network.Ready || status.Network.Code != "LOCAL_ONLY" {
		t.Fatal(status.Network)
	}
	// A new root containing unsupported objects cannot receive an apply artifact.
	bad := filepath.Join(base, "unsupported")
	if err = os.Mkdir(bad, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink("elsewhere", filepath.Join(bad, "link")); err != nil {
		t.Fatal(err)
	}
	rejected := filepath.Join(base, "rejected.json")
	_, out, err = cli.call("", "setup", "--state", dir, "--root", bad, "--preview", "--review-file", rejected, "--json")
	if err == nil {
		t.Fatal("unsupported preview accepted", out)
	}
	if _, err = os.Stat(rejected); !os.IsNotExist(err) {
		t.Fatal("incomplete review wrote apply artifact")
	}
}

// This is the line-oriented CLI in a real PTY, separate from W07's keyboard TUI.
func (c w06CLI) guided(answers string, args ...string) tc.Result {
	c.t.Helper()
	script := `import os, pty, subprocess, sys, select, time, json
master, slave = pty.openpty()
p = subprocess.Popen(sys.argv[1:], stdin=slave, stdout=slave, stderr=slave, close_fds=True)
os.close(slave)
os.write(master, os.environ['ORBIT_W06_ANSWERS'].encode())
data = b''
end = time.monotonic()+90
while time.monotonic()<end:
    ready,_,_ = select.select([master],[],[],0.2)
    if ready:
        try: data += os.read(master,65536)
        except OSError: break
    if p.poll() is not None: break
# PTY EOF can precede child reaping (notably under race instrumentation).
# Wait only for the remainder of the existing deadline, not a new timeout.
try:
    status = p.wait(timeout=max(0.001, end-time.monotonic()))
except subprocess.TimeoutExpired:
    p.kill()
    p.wait()
    raise RuntimeError('guided CLI timeout')
os.close(master)
sys.stdout.buffer.write(data)
sys.exit(status)`
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", append([]string{"-c", script, c.binary}, args...)...)
	cmd.Env = append(os.Environ(), "SSL_CERT_FILE="+c.ca, "ORBIT_W06_ANSWERS="+answers)
	out, err := cmd.CombinedOutput()
	if err != nil {
		c.t.Fatalf("guided CLI %v: %v\n%s", args, err, out)
	}
	// JSON is the last output line after the deliberate human review on stderr.
	var r tc.Result
	for _, line := range bytes.Split(out, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if at := bytes.IndexByte(line, '{'); at >= 0 {
			line = line[at:]
			if e := json.Unmarshal(line, &r); e == nil {
				return r
			}
		}
	}
	c.t.Fatalf("guided CLI missing result: %s", out)
	return r
}
func TestWANW06BinaryGuidedFirstDevice(t *testing.T) {
	base := testkit.NewDisposable(t)
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	cli := w06CLI{t: t, binary: buildOrbitBinary(t, base)}
	dir := filepath.Join(base, "state")
	root := filepath.Join(base, "notes")
	t.Cleanup(func() {
		if err := testkit.ValidateDestructiveTarget(base, dir); err != nil {
			t.Error(err)
			return
		}
		_ = app.StopAgent(dir, 10*time.Second)
	})
	result := cli.guided(strings.Repeat("\n", 9)+"yes\n", "setup", "--state", dir, "--root", root, "--label", "Laptop", "--name", "Notes", "--timeout", "0", "--json")
	if result.Operation == nil {
		t.Fatal("missing guided setup operation")
	}
	w06Wait(t, dir, result.Operation.ID)
	policy, err := config.LoadNetworkPolicy(dir)
	if err != nil || policy.Mode != "automatic" || !policy.AwaitingProfile {
		t.Fatal(policy, err)
	}
}
