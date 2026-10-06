package terminal_test

import (
	"context"
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
)

func TestWANW12BinaryDoctorPrivacyAndPTY(t *testing.T) {
	w06BinaryJourneyWithFollowup(t, "", true, nil, func(cli w06CLI, states []string, rootA, rootB string, inv tc.Invitation, requests *atomic.Int64) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "python3", "../../scripts/terminal_network_pty_test.py", "--binary", cli.binary, "--root", filepath.Dir(cli.binary), "--state", states[1])
		cmd.Env = append(os.Environ(), "SSL_CERT_FILE="+cli.ca)
		out, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("W12 PTY: %v\n%s", e, out)
		}
		t.Log(string(out))
		var doctor tc.Result
		until := time.Now().Add(35 * time.Second)
		for {
			doctor = cli.ok("network", "doctor", "--state", states[1], "--device", inv.Inviter, "--json")
			verified := false
			quota := false
			for _, p := range doctor.Network.Probes {
				if p.Kind == "relay_inner_tls" {
					verified = p.Code == "VERIFIED"
					quota = p.Code == "QUOTA_EXCEEDED"
				}
			}
			if verified || !quota || time.Now().After(until) {
				break
			}
			time.Sleep(5 * time.Second)
		}
		seen := map[string]string{}
		for _, p := range doctor.Network.Probes {
			seen[p.Kind] = p.Code
		}
		if seen["service_tls"] != "VERIFIED" || seen["directory"] != "VERIFIED" || (seen["relay_inner_tls"] != "VERIFIED" && seen["relay_inner_tls"] != "QUOTA_EXCEEDED") || seen["udp_stun"] != "NOT_TESTED" {
			t.Fatal(seen)
		}
		status := cli.ok("status", "--state", states[1], "--json")
		if status.Network == nil || status.Network.Code != "SERVICE_READY" || status.Readiness == nil || !status.Readiness.Ready() {
			t.Fatal("separate readiness", status)
		}
		// Stop only this dedicated inviter; service health is independent of its presence.
		if e = app.StopAgent(states[0], 10*time.Second); e != nil {
			t.Fatal(e)
		}
		time.Sleep(1100 * time.Millisecond)
		doctor = cli.ok("network", "doctor", "--state", states[1], "--device", inv.Inviter, "--json")
		seen = map[string]string{}
		for _, p := range doctor.Network.Probes {
			seen[p.Kind] = p.Code
		}
		if seen["service_tls"] != "VERIFIED" || seen["relay_inner_tls"] == "VERIFIED" {
			t.Fatal("healthy service vs offline peer", seen)
		}
		before, _ := config.Load(states[1])
		policyReview := filepath.Join(filepath.Dir(cli.binary), "w12-local-review.json")
		cli.ok("network", "preview", "--state", states[1], "--mode", "local_only", "--lan-advertising=false", "--review-file", policyReview, "--json")
		cli.ok("network", "apply", "--state", states[1], "--review-file", policyReview, "--json")
		cli.ok("network", "apply", "--state", states[1], "--review-file", policyReview, "--json")
		count := requests.Load()
		for range 3 {
			result := cli.ok("network", "doctor", "--state", states[1], "--device", inv.Inviter, "--json")
			if result.Network.Policy.Mode != "local_only" || result.Network.RestartRequired {
				t.Fatal(result.Network)
			}
			for _, p := range result.Network.Probes {
				if p.Kind != "direct_tls" && p.Code != "DISABLED_BY_POLICY" {
					t.Fatal(p)
				}
			}
		}
		time.Sleep(1100 * time.Millisecond)
		if requests.Load() != count {
			t.Fatal("privacy mode contacted service", count, requests.Load())
		}
		if e = app.StopAgent(states[1], 10*time.Second); e != nil {
			t.Fatal(e)
		}
		cli.ok("network", "preview", "--state", states[1], "--review-file", policyReview+"-restart", "--json")
		result := cli.ok("network", "status", "--state", states[1], "--json")
		if result.Network.Policy.Mode != "local_only" || result.Network.Policy.Profile != "" || result.Network.Code != "LOCAL_ONLY" {
			t.Fatal(result.Network)
		}
		after, _ := config.Load(states[1])
		if after.DeviceID != before.DeviceID {
			t.Fatal("privacy reset identity")
		}
		if b, e := os.ReadFile(filepath.Join(rootB, "from-a.txt")); e != nil || strings.TrimSpace(string(b)) != "verified A bytes" {
			t.Fatal("privacy lost file", e)
		}
		t.Log("actual CLI/PTY doctor, healthy service/offline peer, reviewed local-only/replay/restart, zero service requests, preserved identity/files")
	})
}
