package terminal_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
	"github.com/calebhabesh/orbit/internal/testkit"
)

// F10 over a disposable local routed service
// (w05Service; production orbit-net admission limits) and two real daemons.
// Never run against the deployed VPS.
func TestOnboardingE07F10RelayJoinWait(t *testing.T) {
	// The ordinary run waits long enough to pass the trial's first refusal
	// (48 s) several times over; ORBIT_E07_WAIT=30m is the acceptance run.
	wait := 100 * time.Second
	if v := os.Getenv("ORBIT_E07_WAIT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatal(err)
		}
		wait = d
	}
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
	blocked := 0
	for time.Since(waitStart) < wait {
		q, err := (&controlclient.Client{StateDir: stateB}).Query(context.Background(), tc.Query{Version: tc.Version, Kind: "operation", ID: joined.Operation.ID})
		if err != nil {
			t.Fatal(err)
		}
		if q.State == "blocked" {
			blocked++
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
	if len(codes) > 0 || blocked > 0 {
		t.Fatalf("F10: waiting joiner shown errors %v / blocked %d times", codes, blocked)
	}
	// Approval is noticed within one status interval (30–36 s) plus slack.
	approvalFile := filepath.Join(base, "approve.json")
	cli.ok("devices", "requests", "show", "--state", stateA, "--device", "Pi", "--review-file", approvalFile, "--json")
	approved := time.Now()
	cli.ok("devices", "approve", "--state", stateA, "--review-file", approvalFile, "--json")
	for {
		q, err := (&controlclient.Client{StateDir: stateB}).Query(context.Background(), tc.Query{Version: tc.Version, Kind: "operation", ID: joined.Operation.ID})
		if err != nil {
			t.Fatal(err)
		}
		if q.Operation != nil && q.Operation.Phase != "awaiting_approval" {
			t.Logf("F10: approval noticed after %s (phase %s)", time.Since(approved).Round(time.Second), q.Operation.Phase)
			break
		}
		if time.Since(approved) > 50*time.Second {
			t.Fatal("F10: approval not noticed within 50 s")
		}
		time.Sleep(time.Second)
	}
}
