package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// E09: the monthly relay budget alerts once at 80% and once at 100%, each a
// transition, and resolves when a new month resets the count.
func TestOnboardingE09RelayBudgetAlerts(t *testing.T) {
	f := newAlertFixture(t)
	start := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	f.set(`orbit_net_relay_month_limit_bytes`, 1000)
	f.set(`orbit_net_relay_month_bytes`, 100)
	steps := []struct {
		used float64
		want []string
	}{
		{100, nil},
		{810, []string{"FIRING relay_budget_80"}},
		{900, nil},
		{1000, []string{"FIRING relay_budget_spent", "RESOLVED relay_budget_80"}},
		{1200, nil},
		{0, []string{"RESOLVED relay_budget_spent"}}, // new month
	}
	for i, s := range steps {
		f.set(`orbit_net_relay_month_bytes`, s.used)
		got := f.step(start.Add(time.Duration(i) * 5 * time.Minute))
		for j, e := range got {
			got[j] = map[byte]string{'+': "FIRING ", '-': "RESOLVED "}[e[0]] + e[1:]
		}
		if strings.Join(sorted(got), ",") != strings.Join(sorted(s.want), ",") {
			t.Fatalf("step %d (used %v): %v, want %v", i, s.used, got, s.want)
		}
	}
	if d := f.st.Details["relay_budget_spent"]; d != "" && !strings.Contains(d, "direct connections unaffected") {
		t.Fatalf("spent detail %q", d)
	}
}

func sorted(s []string) []string {
	out := append([]string(nil), s...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// E09: serve's configuration defaults to a finite budget, rejects a negative one, and
// keeps the count in $STATE_DIRECTORY/relay-month.json across restarts.
func TestOnboardingE09ServeBudgetConfigAndRestart(t *testing.T) {
	if defaultRelayMonthBytes <= 0 || defaultRelayMonthBytes > 10<<40 {
		t.Fatalf("default relay_month_bytes %d not finite and below 10 TiB", defaultRelayMonthBytes)
	}
	k := newKit(t)
	host := "10.20.30.40"
	if out, err := k.sign(k.template("t.json", host+":8443", 1, []string{}), "development", "profile.json"); err != nil {
		t.Fatal(err, out)
	}
	cert, key := k.certificate("tls", host, time.Now().Add(90*24*time.Hour))
	cfg := serveConfig{Listen: "127.0.0.1:0", Profile: k.path("profile.json"), ServiceKey: k.service, TLSCert: cert, TLSKey: key, RelayBPS: 1 << 20, RelayDeviceBPS: 1 << 20, RelaySessionBytes: 1 << 20, RelayMonthBytes: -1}
	if _, err := prepare(cfg, time.Now()); err == nil || !strings.Contains(err.Error(), "relay_month_bytes") {
		t.Fatalf("negative budget accepted: %v", err)
	}
	state := t.TempDir()
	if err := os.Chmod(state, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STATE_DIRECTORY", state)
	cfg.RelayMonthBytes = 1 << 20
	p, err := prepare(cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	m := p.service.Metrics()
	if m.RelayMonthLimit != 1<<20 {
		t.Fatalf("budget limit %d", m.RelayMonthLimit)
	}
	_ = p.service.Close()
	path := filepath.Join(state, "relay-month.json")
	// Simulate a month's use recorded by an earlier run, then restart.
	month := time.Now().UTC().Format("2006-01")
	if err = os.WriteFile(path, []byte(fmt.Sprintf(`{"version":1,"month":%q,"bytes":%d}`, month, 1<<20)), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err = prepare(cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer p.service.Close()
	if m = p.service.Metrics(); m.RelayMonthBytes != 1<<20 {
		t.Fatalf("restart lost the month's count: %d", m.RelayMonthBytes)
	}
}
