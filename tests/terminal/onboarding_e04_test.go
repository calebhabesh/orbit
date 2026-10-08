package terminal_test

import (
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

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
	"github.com/calebhabesh/orbit/internal/testkit"
)

// F08 (CLI), F11, F16 over a disposable local routed service
// (w05Service; production orbit-net admission limits) and two real daemons.
// Never run against the deployed VPS.
func TestOnboardingE04FreshJoinDefaultsNamesAndModes(t *testing.T) {
	e04JoinDefaults(t, false)
}

func TestOnboardingE06ShortCodeJoinsAndTransfers(t *testing.T) {
	e04JoinDefaults(t, true)
}

func e04JoinDefaults(t *testing.T, short bool) {
	base := testkit.NewDisposable(t)
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	var serviceRequests atomic.Int64
	_, selection, origin, roots, _ := w05Service(t, func() { serviceRequests.Add(1) })
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
	stateA, stateB := filepath.Join(base, "a"), filepath.Join(base, "b")
	if short {
		// Full headless join with an isolated service-manager fixture. This starts
		// the real daemon, but never changes this host's units or lingering.
		home := filepath.Join(base, "home")
		if err := os.MkdirAll(home, 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("HOME", home)
		stateB = filepath.Join(home, ".local", "state", "orbit")
		t.Setenv("ORBIT_E01_BIN", cli.binary)
		t.Setenv("ORBIT_E01_STATE", stateB)
		e01Stubs(t, base, stubHost{target: "multi-user.target", linger: true})
	}

	for _, dir := range []string{stateA, stateB} {
		e00StopOnCleanup(t, base, dir)
	}
	review := stateA + "-network.json"
	cli.ok("network", "preview", "--state", stateA, "--mode", "self_hosted", "--profile-file", profile, "--review-file", review, "--json")
	cli.ok("network", "apply", "--state", stateA, "--review-file", review, "--json")

	rootA, rootB := filepath.Join(base, "orbit-a"), filepath.Join(base, "orbit-b")
	if err = os.Mkdir(rootA, 0700); err != nil {
		t.Fatal(err)
	}
	shared := filepath.Join(rootA, "shared.txt")
	if err = os.WriteFile(shared, []byte("ordinary 0644 source"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(shared, 0644); err != nil {
		t.Fatal(err)
	}
	setupFile := filepath.Join(base, "create.json")
	cli.ok("setup", "--state", stateA, "--root", rootA, "--label", "PC", "--name", "Orbit", "--preview", "--review-file", setupFile, "--json")
	created := cli.ok("setup", "--state", stateA, "--request-file", setupFile, "--timeout", "0", "--json")
	w06Wait(t, stateA, created.Operation.ID)
	inviteReview, invFile := filepath.Join(base, "invite-review.json"), filepath.Join(base, "invitation.json")
	cli.ok("devices", "invite", "--state", stateA, "--folder", "Orbit", "--preview", "--review-file", inviteReview, "--json")
	if _, out, e := cli.call("", "devices", "invite", "--state", stateA, "--request-file", inviteReview, "--out", invFile, "--json"); e != nil {
		t.Fatal(e, out)
	}

	// F08 (CLI half): a fresh joiner's reviewed plan for a routed invitation.
	var inv tc.Invitation
	if b, e := os.ReadFile(invFile); e != nil || json.Unmarshal(b, &inv) != nil || inv.Route == nil {
		t.Fatal("routed invitation unreadable", e)
	}
	freshFile := filepath.Join(base, "join-fresh.json")
	if short {
		// A self-hosted joiner reviews its operator before using its short code.
		reviewB := stateB + "-network.json"
		cli.ok("network", "preview", "--state", stateB, "--mode", "self_hosted", "--profile-file", profile, "--review-file", reviewB, "--json")
		cli.ok("network", "apply", "--state", stateB, "--review-file", reviewB, "--json")
		cmd := exec.Command(cli.binary, "devices", "invite", "--state", stateA, "--folder", "Orbit", "--code")
		cmd.Env = append(os.Environ(), "SSL_CERT_FILE="+ca)
		output, e := cmd.Output()
		if e != nil {
			t.Fatal(e)
		}
		code := strings.TrimSpace(string(output))
		if len(code) != 9 {
			t.Fatalf("expected short code; got %d characters", len(code))
		}
		digest, _ := selection.Digest()
		_, out, e := cli.call(code, "join", "--state", stateB, "--root", rootB, "--label", "Pi", "--name", "Orbit", "--invitation-stdin", "--pairing-profile", digest, "--preview", "--review-file", freshFile, "--json")
		if e != nil {
			t.Fatal(e, out)
		}
		t.Log("Short code resolved through production CLI and local service")
	} else {
		cli.ok("join", "--state", stateB, "--root", rootB, "--label", "Pi", "--name", "Orbit", "--invitation-file", invFile, "--preview", "--review-file", freshFile, "--json")
	}
	var plan tc.Mutation
	if b, e := os.ReadFile(freshFile); e != nil || json.Unmarshal(b, &plan) != nil || plan.Join == nil {
		t.Fatal("fresh join review unreadable", e)
	}
	planned := tc.NetworkPolicy{}
	if plan.Join.Network != nil {
		planned = *plan.Join.Network
	}
	if short && plan.Join.Settings.Startup != "unattended" {
		t.Fatalf("headless startup: %q", plan.Join.Settings.Startup)
	}
	t.Logf("F08: fresh routed join plan: mode %q, profile set %v, awaiting profile %v; inviter's operator profile preselected %v",
		planned.Mode, planned.Profile != "", planned.AwaitingProfile, planned.Profile == inv.Route.Profile)
	fresh, freshOut, e := cli.call("", "join", "--state", stateB, "--request-file", freshFile, "--timeout", "0", "--json")
	freshCode := ""
	if fresh.Error != nil {
		freshCode = fresh.Error.Code + ": " + fresh.Error.Message
	}
	t.Logf("F08: submitting the fresh plan => err=%v %s", e, freshCode)
	if planned.Profile != inv.Route.Profile || planned.Mode != "self_hosted" || e != nil {
		t.Fatalf("F08: fresh join does not use the routed invitation's operator: mode %q, err %v %s", planned.Mode, e, freshCode)
	}
	if fresh.Operation == nil {
		t.Fatalf("fresh join returned no operation: %s", freshOut)
	}
	joined := fresh
	// The joiner names what it waits for and the code to compare (E04).
	if _, out, _ := cli.call("", "join", "--state", stateB, "--operation", joined.Operation.ID); !strings.Contains(out, "phase=") {
		t.Logf("join progress: %s", out)
	}

	approvalFile := filepath.Join(base, "approve.json")
	var pending tc.Result
	for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline); time.Sleep(time.Second) {
		if pending = cli.ok("devices", "requests", "--state", stateA, "--json"); len(pending.Requests) == 1 {
			break
		}
	}
	if len(pending.Requests) != 1 {
		t.Fatalf("request never reached the inviter: %+v", pending.Requests)
	}
	cli.ok("devices", "requests", "show", "--state", stateA, "--device", "Pi", "--review-file", approvalFile, "--json")
	cli.ok("devices", "approve", "--state", stateA, "--review-file", approvalFile, "--json")
	w06Wait(t, stateB, joined.Operation.ID)
	received := filepath.Join(rootB, "shared.txt")
	for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
		if b, _ := os.ReadFile(received); string(b) == "ordinary 0644 source" {
			break
		}
	}
	fi, err := os.Stat(received)
	if err != nil {
		t.Fatal("F16: file never received", err)
	}
	// By design (persistence: safe local permissions); E04 documents it.
	t.Logf("F16: source mode 0644, received mode %#o", fi.Mode().Perm())
	if fi.Mode().Perm() != 0600 {
		t.Errorf("F16: received mode %#o, documented safe local permission is 0600", fi.Mode().Perm())
	}

	// F11: the inviter lists the joiner by the name it chose ("Pi").
	text, err := e00Run(t, append(os.Environ(), "SSL_CERT_FILE="+ca), 30*time.Second, cli.binary, "devices", "--state", stateA)
	t.Logf("F11: inviter `orbit devices`:\n%s", strings.TrimSpace(text))
	if err != nil || !strings.Contains(text, "Pi") || strings.Contains(text, "Device ") {
		t.Errorf("F11: inviter's device list does not show the joiner's chosen name")
	}
	asJSON, _ := e00Run(t, append(os.Environ(), "SSL_CERT_FILE="+ca), 30*time.Second, cli.binary, "devices", "--state", stateA, "--json")
	if !strings.Contains(asJSON, `"Pi"`) {
		t.Errorf("F11: inviter's JSON device list lacks the joiner's chosen name")
	}
	// The TUI's device list reads the same name through terminal control.
	devices, err := (&controlclient.Client{StateDir: stateA}).Query(context.Background(), tc.Query{Version: tc.Version, Kind: "devices", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	named := false
	for _, d := range devices.Items {
		named = named || d.Name == "Pi"
	}
	if !named {
		t.Errorf("F11: terminal device list lacks the joiner's chosen name: %+v", devices.Items)
	}
}
