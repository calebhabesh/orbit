package terminal_test

// E00 onboarding baseline reproductions for the 2026-10-08 owner trial
// findings that need real processes. Each test asserts approved behavior and
// deliberately fails on the current implementation; ordinary runs skip them.
// Opt in with ORBIT_ONBOARDING_BASELINE=1. E01 promoted F01 and F04/F14 into
// ordinary regressions in onboarding_e01_test.go; E02 promoted F03; E03 promoted F15. All state, HOME and service-manager
// stand-ins live in marked disposable roots; no real user unit, systemd
// manager, personal folder or deployed service is touched.

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

	"github.com/calebhabesh/orbit/internal/app"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
	"github.com/calebhabesh/orbit/internal/testkit"
)

func onboardingBaseline(t *testing.T) {
	t.Helper()
	if os.Getenv("ORBIT_ONBOARDING_BASELINE") != "1" {
		t.Skip("deliberately failing E00 baseline; opt in with ORBIT_ONBOARDING_BASELINE=1")
	}
}

// e00Run runs the built CLI with an explicit environment and returns combined
// output. Outputs here never contain invitation capabilities.
func e00Run(t *testing.T, env []string, timeout time.Duration, binary string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

func e00StopOnCleanup(t *testing.T, base, dir string) {
	t.Cleanup(func() {
		if err := testkit.ValidateDestructiveTarget(base, dir); err != nil {
			t.Error(err)
			return
		}
		_ = app.StopAgent(dir, 10*time.Second)
	})
}

// F08 (CLI half), F10, F11, F16 over a disposable local routed service
// (w05Service; production orbit-net admission limits) and two real daemons.
// Never run against the deployed VPS.
func TestOnboardingE00RelayJoinWaitNamesAndModes(t *testing.T) {
	onboardingBaseline(t)
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
	cli.ok("join", "--state", stateB, "--root", rootB, "--label", "Pi", "--name", "Orbit", "--invitation-file", invFile, "--preview", "--review-file", freshFile, "--json")
	var plan tc.Mutation
	if b, e := os.ReadFile(freshFile); e != nil || json.Unmarshal(b, &plan) != nil || plan.Join == nil {
		t.Fatal("fresh join review unreadable", e)
	}
	planned := tc.NetworkPolicy{}
	if plan.Join.Network != nil {
		planned = *plan.Join.Network
	}
	t.Logf("F08: fresh routed join plan: mode %q, profile set %v, awaiting profile %v; inviter's operator profile preselected %v",
		planned.Mode, planned.Profile != "", planned.AwaitingProfile, planned.Profile == inv.Route.Profile)
	fresh, freshOut, e := cli.call("", "join", "--state", stateB, "--request-file", freshFile, "--timeout", "0", "--json")
	freshCode := ""
	if fresh.Error != nil {
		freshCode = fresh.Error.Code + ": " + fresh.Error.Message
	}
	t.Logf("F08: submitting the fresh plan => err=%v %s", e, freshCode)
	_ = freshOut
	if planned.Profile != inv.Route.Profile || e != nil {
		t.Errorf("F08: fresh join does not preselect the routed invitation's operator (approved: Automatic/inviter operator offered for review, then the join proceeds)")
	}
	// Continue the journey with an explicitly reviewed policy on the joiner.
	reviewB := stateB + "-network.json"
	cli.ok("network", "preview", "--state", stateB, "--mode", "self_hosted", "--profile-file", profile, "--review-file", reviewB, "--json")
	cli.ok("network", "apply", "--state", stateB, "--review-file", reviewB, "--json")
	joinFile := filepath.Join(base, "join.json")
	cli.ok("join", "--state", stateB, "--root", rootB, "--label", "Pi", "--name", "Orbit", "--invitation-file", invFile, "--preview", "--review-file", joinFile, "--json")
	joined := cli.ok("join", "--state", stateB, "--request-file", joinFile, "--timeout", "0", "--json")

	// F10: wait unapproved, polling as the TUI progress screen does (3 s).
	waitStart := time.Now()
	codes := map[string]int{}
	firstLimited := time.Duration(0)
	for time.Since(waitStart) < 150*time.Second {
		q, err := (&controlclient.Client{StateDir: stateB}).Query(context.Background(), tc.Query{Version: tc.Version, Kind: "operation", ID: joined.Operation.ID})
		if err != nil {
			t.Fatal(err)
		}
		if q.Error != nil {
			codes[q.Error.Code]++
			if q.Error.Code == "RATE_LIMITED" && firstLimited == 0 {
				firstLimited = time.Since(waitStart)
			}
		}
		time.Sleep(3 * time.Second)
	}
	t.Logf("F10: joiner errors during %s unapproved wait: %v; first RATE_LIMITED after %s; local service requests %d", time.Since(waitStart).Round(time.Second), codes, firstLimited.Round(time.Second), serviceRequests.Load())
	if codes["RATE_LIMITED"] > 0 {
		t.Errorf("F10: waiting joiner shown RATE_LIMITED %d times", codes["RATE_LIMITED"])
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
}
