package terminal_test

// E00 onboarding baseline reproductions for the 2026-10-08 owner trial
// findings that need real processes. Each test asserts approved behavior and
// deliberately fails on the current implementation; ordinary runs skip them.
// Opt in with ORBIT_ONBOARDING_BASELINE=1. E01 promoted F01 and F04/F14 into
// ordinary regressions in onboarding_e01_test.go; E02 promoted F03; E03 promoted F15; E04 promoted F08, F11 and F16 (onboarding_e04_test.go). All state, HOME and service-manager
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

// F10 (E07) over a disposable local routed service
// (w05Service; production orbit-net admission limits) and two real daemons.
// Never run against the deployed VPS.
func TestOnboardingE00F10RelayJoinWait(t *testing.T) {
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

}
