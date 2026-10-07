package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/calebhabesh/file-sync/internal/state"
)

// Alert thresholds mirror the Monitoring table in docs/orbit-net-operator.md.
const (
	alertProfileWarn   = 30 * 24 * time.Hour
	alertCertWarn      = 14 * 24 * time.Hour
	alertSustained     = 10 * time.Minute
	alertSampleGap     = 5 * time.Minute // a longer gap restarts sustained streaks
	alertEgressRatio   = 0.8
	alertDownSamples   = 2 // consecutive failed checks before Down fires
	alertMaxConfig     = 4 << 10
	alertMaxState      = 64 << 10
	alertMaxMetrics    = 256 << 10
	alertHTTPTimeout   = 10 * time.Second
	alertConditionDown = "down"
)

type alertConfig struct {
	MetricsURL string `json:"metrics_url"` // loopback http origin serving /healthz and /metrics
	NotifyURL  string `json:"notify_url"`  // ntfy-compatible topic URL; treat as a secret
	Name       string `json:"name"`        // short service label shown in notifications
}

// alertState is the checker's memory between timer runs. Counters are the raw
// previous sample; Since records when a sustained condition's streak began.
type alertState struct {
	SampledAt int64              `json:"sampled_at,omitempty"`
	Counters  map[string]float64 `json:"counters,omitempty"`
	Failures  int                `json:"failures,omitempty"`
	Firing    map[string]bool    `json:"firing,omitempty"`
	Since     map[string]int64   `json:"since,omitempty"`
	Latched   map[string]bool    `json:"latched,omitempty"`
	Details   map[string]string  `json:"details,omitempty"`
}

type alertEvent struct {
	condition string
	firing    bool
	detail    string
}

func alert(args []string, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("alert", flag.ContinueOnError)
	flags.SetOutput(errOut)
	configPath := flags.String("config", "", "owner-only alert.json")
	statePath := flags.String("state", "", "owner-only checker state file (created if absent)")
	metricsOverride := flags.String("metrics-url", "", "synthetic exercise: check this loopback origin instead of metrics_url")
	test := flags.Bool("test", false, "send one test notification and exit")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *configPath == "" || (*statePath == "" && !*test) || flags.NArg() != 0 {
		return errors.New("alert requires --config FILE and --state FILE (or --test)")
	}
	cfg, err := loadAlertConfig(*configPath)
	if err != nil {
		return err
	}
	if *metricsOverride != "" {
		if err = checkLoopbackHTTP(*metricsOverride); err != nil {
			return err
		}
		cfg.MetricsURL = *metricsOverride
	}
	client := &http.Client{Timeout: alertHTTPTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if *test {
		if err = notify(client, cfg, "test", "Test notification from orbit-net alert. Delivery works.", "default"); err != nil {
			return err
		}
		fmt.Fprintln(out, "test notification sent")
		return nil
	}
	st, err := loadAlertState(*statePath)
	if err != nil {
		return err
	}
	events := evaluateAlerts(client, cfg, &st, time.Now())
	var failed []string
	for _, e := range events {
		title, priority := "RESOLVED "+e.condition, "default"
		if e.firing {
			title, priority = "FIRING "+e.condition, "high"
		}
		if err := notify(client, cfg, title, e.detail, priority); err != nil {
			// Undo the transition so the next run retries the notification.
			st.Firing[e.condition] = !e.firing
			failed = append(failed, e.condition)
			continue
		}
		fmt.Fprintf(out, "%s: %s\n", title, e.detail)
	}
	if err = saveAlertState(*statePath, st); err != nil {
		return err
	}
	if len(failed) > 0 {
		return fmt.Errorf("notification delivery failed for %s", strings.Join(failed, ", "))
	}
	return nil
}

func loadAlertConfig(path string) (alertConfig, error) {
	var cfg alertConfig
	data, err := readPrivatePath(path, alertMaxConfig)
	if err != nil {
		return cfg, fmt.Errorf("read alert config: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&cfg); err != nil {
		return cfg, errors.New("alert config must be JSON with metrics_url, notify_url and name")
	}
	if err = checkLoopbackHTTP(cfg.MetricsURL); err != nil {
		return cfg, err
	}
	notifyURL, err := url.Parse(cfg.NotifyURL)
	if err != nil || notifyURL.Host == "" || notifyURL.User != nil || (notifyURL.Scheme != "https" && !(notifyURL.Scheme == "http" && isLoopbackHost(notifyURL.Hostname()))) {
		return cfg, errors.New("notify_url must be an https URL without credentials")
	}
	if cfg.Name == "" || len(cfg.Name) > 64 || strings.ContainsAny(cfg.Name, "\r\n") {
		return cfg, errors.New("name must be 1-64 characters on one line")
	}
	return cfg, nil
}

func checkLoopbackHTTP(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || !isLoopbackHost(u.Hostname()) || u.Port() == "" || (u.Path != "" && u.Path != "/") || u.User != nil || u.RawQuery != "" {
		return errors.New("metrics_url must be a numeric loopback http origin such as http://127.0.0.1:9464")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func loadAlertState(path string) (alertState, error) {
	st := alertState{}
	data, err := readPrivatePath(path, alertMaxState)
	if errors.Is(err, os.ErrNotExist) {
		if err = state.ValidateDirectory(filepath.Dir(path)); err != nil {
			return st, err
		}
		data, err = []byte("{}"), nil
	}
	if err != nil {
		return st, fmt.Errorf("read alert state: %w", err)
	}
	if err = json.Unmarshal(data, &st); err != nil {
		return st, errors.New("alert state is corrupt; remove it to start fresh")
	}
	for _, m := range []*map[string]bool{&st.Firing, &st.Latched} {
		if *m == nil {
			*m = map[string]bool{}
		}
	}
	if st.Since == nil {
		st.Since = map[string]int64{}
	}
	if st.Counters == nil {
		st.Counters = map[string]float64{}
	}
	if st.Details == nil {
		st.Details = map[string]string{}
	}
	return st, nil
}

func saveAlertState(path string, st alertState) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".alert-state-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// evaluateAlerts takes one sample and returns the firing/resolved transitions.
func evaluateAlerts(client *http.Client, cfg alertConfig, st *alertState, now time.Time) []alertEvent {
	desired := map[string]string{} // condition -> detail, for conditions active now
	origin := strings.TrimSuffix(cfg.MetricsURL, "/")
	healthy, healthDetail := checkHealth(client, origin+"/healthz")
	var samples map[string]float64
	if healthy {
		var err error
		if samples, err = fetchMetrics(client, origin+"/metrics"); err != nil {
			healthy, healthDetail = false, err.Error()
		}
	}
	if !healthy {
		st.Failures++
		if st.Failures >= alertDownSamples || st.Firing[alertConditionDown] {
			desired[alertConditionDown] = cfg.Name + " health check failing: " + healthDetail
		}
		// Keep metric-based conditions as they were; nothing new is known.
		for c, f := range st.Firing {
			if f && c != alertConditionDown {
				desired[c] = st.Details[c]
			}
		}
		return transitions(st, desired)
	}
	st.Failures = 0
	elapsed := time.Duration(0)
	if st.SampledAt != 0 {
		elapsed = now.Sub(time.Unix(st.SampledAt, 0))
	}
	continuous := st.SampledAt != 0 && elapsed > 0 && elapsed <= alertSampleGap

	if v, ok := newestProfileExpiry(samples); ok && time.Duration(v)*time.Second < alertProfileWarn {
		desired["profile_expiring"] = fmt.Sprintf("%s newest profile epoch expires in %s; sign and serve the next epoch", cfg.Name, humanDuration(v))
	}
	if v, ok := samples["orbit_net_certificate_expiry_seconds"]; ok && time.Duration(v)*time.Second < alertCertWarn {
		desired["certificate_expiring"] = fmt.Sprintf("%s TLS certificate expires in %s; check the renewal timer", cfg.Name, humanDuration(v))
	}

	// A rejected reload latches until a later successful reload or a restart.
	failures, reloads := samples["orbit_net_certificate_reload_failures_total"], samples["orbit_net_certificate_reloads_total"]
	prevFailures, hadFailures := st.Counters["orbit_net_certificate_reload_failures_total"]
	prevReloads := st.Counters["orbit_net_certificate_reloads_total"]
	if hadFailures && failures > prevFailures {
		st.Latched["reload_rejected"] = true
	}
	if failures < prevFailures || reloads > prevReloads && !(hadFailures && failures > prevFailures) {
		st.Latched["reload_rejected"] = false
	}
	if st.Latched["reload_rejected"] {
		desired["reload_rejected"] = cfg.Name + " rejected a certificate reload; the previous certificate is still served"
	}

	rising := func(names ...string) bool {
		for _, n := range names {
			prev, ok := st.Counters[n]
			if ok && samples[n] > prev {
				return true
			}
		}
		return false
	}
	egress := false
	if limit := samples["orbit_net_relay_limit_bytes_per_second"]; continuous && limit > 0 {
		prev, ok := st.Counters["orbit_net_relay_bytes_total"]
		if delta := samples["orbit_net_relay_bytes_total"] - prev; ok && delta > 0 {
			egress = delta/elapsed.Seconds() >= alertEgressRatio*limit
		}
	}
	sustained := []struct {
		condition, detail string
		active            bool
	}{
		{"saturation", "connections or quota refusals rising for 10 minutes", rising(`orbit_net_connections_refused_total`, `orbit_net_refusals_total{reason="quota"}`)},
		{"stranded_devices", "untrusted/expired refusals rising for 10 minutes; check the last profile or trust change", rising(`orbit_net_refusals_total{reason="untrusted"}`, `orbit_net_refusals_total{reason="expired"}`)},
		{"egress", fmt.Sprintf("relay traffic at or above %.0f%% of the aggregate limit for 10 minutes", alertEgressRatio*100), egress},
	}
	for _, s := range sustained {
		if !continuous || !s.active {
			delete(st.Since, s.condition)
			continue
		}
		if _, ok := st.Since[s.condition]; !ok {
			st.Since[s.condition] = st.SampledAt
		}
		if now.Sub(time.Unix(st.Since[s.condition], 0)) >= alertSustained {
			desired[s.condition] = cfg.Name + " " + s.detail
		}
	}
	st.SampledAt = now.Unix()
	st.Counters = map[string]float64{}
	for n, v := range samples {
		if strings.HasSuffix(strings.SplitN(n, "{", 2)[0], "_total") {
			st.Counters[n] = v
		}
	}
	return transitions(st, desired)
}

func transitions(st *alertState, desired map[string]string) []alertEvent {
	var events []alertEvent
	names := map[string]bool{}
	for c := range desired {
		names[c] = true
	}
	for c, f := range st.Firing {
		if f {
			names[c] = true
		}
	}
	sorted := make([]string, 0, len(names))
	for c := range names {
		sorted = append(sorted, c)
	}
	sort.Strings(sorted)
	for _, c := range sorted {
		detail, want := desired[c]
		switch {
		case want && !st.Firing[c]:
			st.Firing[c], st.Details[c] = true, detail
			events = append(events, alertEvent{c, true, detail})
		case !want && st.Firing[c]:
			events = append(events, alertEvent{c, false, "cleared: " + st.Details[c]})
			st.Firing[c] = false
			delete(st.Details, c)
		}
	}
	return events
}

func checkHealth(client *http.Client, u string) (bool, string) {
	resp, err := client.Get(u)
	if err != nil {
		return false, "no response from the metrics listener (service stopped?)"
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("/healthz returned %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return true, ""
}

func fetchMetrics(client *http.Client, u string) (map[string]float64, error) {
	resp, err := client.Get(u)
	if err != nil {
		return nil, errors.New("metrics unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("/metrics returned %d", resp.StatusCode)
	}
	return parseMetrics(io.LimitReader(resp.Body, alertMaxMetrics))
}

// parseMetrics reads the Prometheus text this service writes: one sample per
// line as name{labels} value, keyed by the exact series text.
func parseMetrics(r io.Reader) (map[string]float64, error) {
	samples := map[string]float64{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.LastIndexByte(line, ' ')
		if i <= 0 {
			return nil, errors.New("malformed metrics line")
		}
		v, err := strconv.ParseFloat(line[i+1:], 64)
		if err != nil {
			return nil, errors.New("malformed metrics value")
		}
		samples[line[:i]] = v
	}
	return samples, scanner.Err()
}

func newestProfileExpiry(samples map[string]float64) (float64, bool) {
	newest, value, found := -1, 0.0, false
	for series, v := range samples {
		rest, ok := strings.CutPrefix(series, `orbit_net_profile_expiry_seconds{epoch="`)
		if !ok {
			continue
		}
		epoch, err := strconv.Atoi(strings.TrimSuffix(rest, `"}`))
		if err == nil && epoch > newest {
			newest, value, found = epoch, v, true
		}
	}
	return value, found
}

func humanDuration(seconds float64) string {
	if seconds <= 0 {
		return "0 days (already expired)"
	}
	if days := seconds / 86400; days >= 1 {
		return fmt.Sprintf("%.0f days", days)
	}
	return fmt.Sprintf("%.0f hours", seconds/3600)
}

func notify(client *http.Client, cfg alertConfig, title, body, priority string) error {
	req, err := http.NewRequest(http.MethodPost, cfg.NotifyURL, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Title", "orbit-net "+cfg.Name+": "+title)
	req.Header.Set("Priority", priority)
	req.Header.Set("Tags", "satellite")
	resp, err := client.Do(req)
	if err != nil {
		// The URL embeds the secret topic; never echo it.
		return errors.New("notification endpoint unreachable")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("notification endpoint returned %d", resp.StatusCode)
	}
	return nil
}
