package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type alertFixture struct {
	t                     *testing.T
	mu                    sync.Mutex
	healthy               bool
	metrics               map[string]float64
	posts                 []string
	notifyStatus          int
	metricsSrv, notifySrv *httptest.Server
	cfg                   alertConfig
	st                    alertState
}

func newAlertFixture(t *testing.T) *alertFixture {
	f := &alertFixture{t: t, healthy: true, notifyStatus: http.StatusOK, metrics: map[string]float64{
		`orbit_net_profile_expiry_seconds{epoch="1"}`:  60 * 86400,
		`orbit_net_certificate_expiry_seconds`:         60 * 86400,
		`orbit_net_certificate_reloads_total`:          0,
		`orbit_net_certificate_reload_failures_total`:  0,
		`orbit_net_connections_refused_total`:          0,
		`orbit_net_refusals_total{reason="quota"}`:     0,
		`orbit_net_refusals_total{reason="untrusted"}`: 0,
		`orbit_net_refusals_total{reason="expired"}`:   0,
		`orbit_net_relay_bytes_total`:                  0,
		`orbit_net_relay_limit_bytes_per_second`:       4 << 20,
		`orbit_net_build_info{version="test"}`:         1,
	}}
	f.metricsSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case "/healthz":
			if !f.healthy {
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprintln(w, "no valid profile")
				return
			}
			fmt.Fprintln(w, "ok")
		case "/metrics":
			fmt.Fprintln(w, "# HELP x y\n# TYPE x gauge")
			for k, v := range f.metrics {
				fmt.Fprintf(w, "%s %v\n", k, v)
			}
		}
	}))
	f.notifySrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.posts = append(f.posts, r.Header.Get("Title")+"|"+r.Header.Get("Priority")+"|"+string(body))
		w.WriteHeader(f.notifyStatus)
	}))
	t.Cleanup(f.metricsSrv.Close)
	t.Cleanup(f.notifySrv.Close)
	f.cfg = alertConfig{MetricsURL: f.metricsSrv.URL, NotifyURL: f.notifySrv.URL + "/topic", Name: "test"}
	f.st, _ = loadAlertStateFromBytes(t, "{}")
	return f
}

func loadAlertStateFromBytes(t *testing.T, s string) (alertState, error) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := dir + "/state.json"
	if err := os.WriteFile(path, []byte(s), 0600); err != nil {
		t.Fatal(err)
	}
	return loadAlertState(path)
}

func (f *alertFixture) set(series string, v float64) {
	f.mu.Lock()
	f.metrics[series] = v
	f.mu.Unlock()
}
func (f *alertFixture) add(series string, v float64) {
	f.mu.Lock()
	f.metrics[series] += v
	f.mu.Unlock()
}

// step evaluates at now and returns "+cond"/"-cond" transitions.
func (f *alertFixture) step(now time.Time) []string {
	f.t.Helper()
	var got []string
	for _, e := range evaluateAlerts(http.DefaultClient, f.cfg, &f.st, now) {
		sign := "-"
		if e.firing {
			sign = "+"
		}
		got = append(got, sign+e.condition)
	}
	return got
}

func expectEvents(t *testing.T, label string, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s: events %v, want %v", label, got, want)
	}
}

func TestW16AlertDownFiresAfterConsecutiveFailuresAndRecovers(t *testing.T) {
	f := newAlertFixture(t)
	now := time.Unix(1_800_000_000, 0)
	expectEvents(t, "baseline", f.step(now))
	f.healthy = false
	expectEvents(t, "first failure is tolerated", f.step(now.Add(time.Minute)))
	expectEvents(t, "second failure fires", f.step(now.Add(2*time.Minute)), "+down")
	expectEvents(t, "still down, no repeat", f.step(now.Add(3*time.Minute)))
	f.healthy = true
	expectEvents(t, "recovery", f.step(now.Add(4*time.Minute)), "-down")

	f.metricsSrv.Close() // unreachable listener, as when the unit is stopped
	f.step(now.Add(5 * time.Minute))
	expectEvents(t, "unreachable fires", f.step(now.Add(6*time.Minute)), "+down")
}

func TestW16AlertExpiryThresholdsUseNewestEpoch(t *testing.T) {
	f := newAlertFixture(t)
	now := time.Unix(1_800_000_000, 0)
	// An old overlap epoch about to expire must not alert while the newest is healthy.
	f.set(`orbit_net_profile_expiry_seconds{epoch="0"}`, 3600)
	expectEvents(t, "old overlap epoch", f.step(now))
	f.set(`orbit_net_profile_expiry_seconds{epoch="1"}`, 29*86400)
	f.set(`orbit_net_certificate_expiry_seconds`, 13*86400)
	expectEvents(t, "both expiring", f.step(now.Add(time.Minute)), "+certificate_expiring", "+profile_expiring")
	f.set(`orbit_net_profile_expiry_seconds{epoch="2"}`, 90*86400)
	f.set(`orbit_net_certificate_expiry_seconds`, 89*86400)
	expectEvents(t, "rotated", f.step(now.Add(2*time.Minute)), "-certificate_expiring", "-profile_expiring")
}

func TestW16AlertReloadRejectedLatchesUntilSuccessfulReload(t *testing.T) {
	f := newAlertFixture(t)
	now := time.Unix(1_800_000_000, 0)
	f.step(now)
	f.add(`orbit_net_certificate_reload_failures_total`, 1)
	expectEvents(t, "rejected", f.step(now.Add(time.Minute)), "+reload_rejected")
	expectEvents(t, "latched", f.step(now.Add(2*time.Minute)))
	f.add(`orbit_net_certificate_reloads_total`, 1)
	expectEvents(t, "good reload", f.step(now.Add(3*time.Minute)), "-reload_rejected")
}

func TestW16AlertSustainedConditionsNeedTenMinutes(t *testing.T) {
	f := newAlertFixture(t)
	now := time.Unix(1_800_000_000, 0)
	f.step(now)
	for i := 1; i <= 9; i++ {
		f.add(`orbit_net_refusals_total{reason="quota"}`, 3)
		f.add(`orbit_net_refusals_total{reason="expired"}`, 1)
		f.add(`orbit_net_relay_bytes_total`, 60*float64(4<<20)) // 100% of limit
		expectEvents(t, fmt.Sprintf("minute %d", i), f.step(now.Add(time.Duration(i)*time.Minute)))
	}
	f.add(`orbit_net_refusals_total{reason="quota"}`, 3)
	f.add(`orbit_net_refusals_total{reason="expired"}`, 1)
	f.add(`orbit_net_relay_bytes_total`, 60*float64(4<<20))
	expectEvents(t, "minute 10", f.step(now.Add(10*time.Minute)), "+egress", "+saturation", "+stranded_devices")
	// Low egress and no refusals in the next interval clear all three.
	f.add(`orbit_net_relay_bytes_total`, 1)
	expectEvents(t, "quiet", f.step(now.Add(11*time.Minute)), "-egress", "-saturation", "-stranded_devices")

	// A sampling gap (missed timer) restarts the streak instead of spanning it.
	g := newAlertFixture(t)
	g.step(now)
	for i := 1; i <= 5; i++ {
		g.add(`orbit_net_connections_refused_total`, 1)
		g.step(now.Add(time.Duration(i) * time.Minute))
	}
	g.add(`orbit_net_connections_refused_total`, 1)
	expectEvents(t, "after gap", g.step(now.Add(20*time.Minute)))
	// A service restart resets counters; that is not a rise.
	g.set(`orbit_net_connections_refused_total`, 0)
	expectEvents(t, "reset", g.step(now.Add(21*time.Minute)))
}

func TestW16AlertCommandDeliversAndRetriesFailedNotifications(t *testing.T) {
	f := newAlertFixture(t)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	cfgPath, statePath := dir+"/alert.json", dir+"/state.json"
	data, _ := json.Marshal(f.cfg)
	if err := os.WriteFile(cfgPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	var out, errOut strings.Builder
	runAlert := func(extra ...string) error {
		out.Reset()
		return run(append([]string{"alert", "--config", cfgPath, "--state", statePath}, extra...), &out, &errOut)
	}
	if err := run([]string{"alert", "--config", cfgPath, "--test"}, &out, &errOut); err != nil || len(f.posts) != 1 || !strings.Contains(f.posts[0], "test") {
		t.Fatal("test notification", err, f.posts)
	}
	if err := runAlert(); err != nil {
		t.Fatal(err)
	}
	// Synthetic exercise: point at an unused loopback port, as the live drill does.
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	f.notifyStatus = http.StatusBadGateway
	_ = runAlert("--metrics-url", deadURL)
	if err := runAlert("--metrics-url", deadURL); err == nil || strings.Contains(err.Error(), "/topic") {
		t.Fatal("failed delivery must error without echoing the secret URL", err)
	}
	f.notifyStatus = http.StatusOK
	if err := runAlert("--metrics-url", deadURL); err != nil || !strings.Contains(out.String(), "FIRING down") {
		t.Fatal("retried firing", err, out.String())
	}
	if err := runAlert(); err != nil || !strings.Contains(out.String(), "RESOLVED down") {
		t.Fatal("recovery", err, out.String())
	}
	last := f.posts[len(f.posts)-1]
	if !strings.HasPrefix(last, "orbit-net test: RESOLVED down|default|") || !strings.Contains(f.posts[len(f.posts)-2], "FIRING down|high|") {
		t.Fatal("notification format", f.posts)
	}
	info, err := os.Stat(statePath)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("state must be owner-only", err)
	}

	// Configuration refusals.
	for name, cfg := range map[string]string{
		"public metrics":  `{"metrics_url":"http://192.0.2.1:9464","notify_url":"https://ntfy.sh/x","name":"a"}`,
		"plain notify":    `{"metrics_url":"http://127.0.0.1:9464","notify_url":"http://ntfy.sh/x","name":"a"}`,
		"credentials":     `{"metrics_url":"http://127.0.0.1:9464","notify_url":"https://u:p@ntfy.sh/x","name":"a"}`,
		"unknown field":   `{"metrics_url":"http://127.0.0.1:9464","notify_url":"https://ntfy.sh/x","name":"a","x":1}`,
		"multi-line name": `{"metrics_url":"http://127.0.0.1:9464","notify_url":"https://ntfy.sh/x","name":"a\nb"}`,
	} {
		if err := os.WriteFile(cfgPath, []byte(cfg), 0600); err != nil {
			t.Fatal(err)
		}
		if err := runAlert(); err == nil {
			t.Fatal(name, "accepted")
		}
	}
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfgPath, 0644); err != nil {
		t.Fatal(err)
	}
	if err := runAlert(); err == nil {
		t.Fatal("group-readable config with a secret topic accepted")
	}
}
