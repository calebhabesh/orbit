package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/rendezvous"
	"github.com/calebhabesh/orbit/internal/testkit"
)

func r02Hex(b byte) string { return strings.Repeat(fmt.Sprintf("%02x", b), 32) }

func r02Kit(t *testing.T) (*w13Kit, map[string]any) {
	t.Helper()
	k := newKit(t)
	host := "10.20.30.40"
	if out, err := k.sign(k.template("t.json", host+":8443", 1, []string{}), "self_hosted", "profile.json"); err != nil {
		t.Fatal(err, out)
	}
	cert, key := k.certificate("tls", host, time.Now().Add(90*24*time.Hour))
	return k, map[string]any{"listen": "127.0.0.1:0", "profile": k.path("profile.json"), "service_key": k.service, "tls_cert": cert, "tls_key": key, "relay_bps": 1 << 20, "relay_device_bps": 1 << 19, "relay_session_bytes": 1 << 30, "relay_month_bytes": 1 << 30}
}

func r02With(base map[string]any, changes map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range changes {
		out[k] = v
	}
	return out
}

func r02File(t *testing.T, k *w13Kit, name string, devices ...allowedDevice) string {
	t.Helper()
	b, _ := json.Marshal(allowlistFile{Version: "1", Devices: devices})
	if err := os.WriteFile(k.path(name), b, 0600); err != nil {
		t.Fatal(err)
	}
	return k.path(name)
}

func TestFinalR02ServeCheckValidatesAdmission(t *testing.T) {
	k, base := r02Kit(t)
	one := allowedDevice{Device: r02Hex(1), Pin: r02Hex(2), Label: "laptop"}
	out, err := k.run("serve", "--config", k.config("inline.json", r02With(base, map[string]any{"admission": "allowlist", "allowed_devices": []allowedDevice{one}})), "--check")
	if err != nil || !strings.Contains(out, "admission allowlist (1 devices)") {
		t.Fatal("inline allowlist refused", err, out)
	}
	file := r02File(t, k, "allowed.json", one)
	if out, err = k.run("serve", "--config", k.config("file.json", r02With(base, map[string]any{"admission": "allowlist", "allowed_devices_file": file})), "--check"); err != nil || !strings.Contains(out, "admission allowlist (1 devices)") {
		t.Fatal("allowlist file refused", err, out)
	}
	if out, err = k.run("serve", "--config", k.config("open.json", base), "--check"); err != nil || !strings.Contains(out, "admission open") {
		t.Fatal("default open admission", err, out)
	}
	many := []allowedDevice{}
	for i := 0; i < rendezvous.MaxAllowedDevices+1; i++ {
		many = append(many, allowedDevice{Device: r02Hex(byte(i)), Pin: r02Hex(2)})
	}
	badFile := k.path("bad-field.json")
	_ = os.WriteFile(badFile, []byte(`{"version":"1","devices":[],"note":"x"}`), 0600)
	readable := r02File(t, k, "readable.json", one)
	_ = os.Chmod(readable, 0644)
	for name, changes := range map[string]map[string]any{
		"unknown admission":     {"admission": "closed"},
		"allowlist without any": {"admission": "allowlist"},
		"both sources":          {"admission": "allowlist", "allowed_devices": []allowedDevice{one}, "allowed_devices_file": file},
		"list while open":       {"allowed_devices": []allowedDevice{one}},
		"bad device hex":        {"admission": "allowlist", "allowed_devices": []allowedDevice{{Device: "abc", Pin: r02Hex(2)}}},
		"uppercase pin":         {"admission": "allowlist", "allowed_devices": []allowedDevice{{Device: r02Hex(1), Pin: strings.ToUpper(r02Hex(0xab))}}},
		"duplicate":             {"admission": "allowlist", "allowed_devices": []allowedDevice{one, one}},
		"over 64":               {"admission": "allowlist", "allowed_devices_file": r02File(t, k, "many.json", many...)},
		"label too long":        {"admission": "allowlist", "allowed_devices": []allowedDevice{{Device: r02Hex(1), Pin: r02Hex(2), Label: strings.Repeat("x", 65)}}},
		"unknown file field":    {"admission": "allowlist", "allowed_devices_file": badFile},
		"group-readable file":   {"admission": "allowlist", "allowed_devices_file": readable},
		"missing file":          {"admission": "allowlist", "allowed_devices_file": k.path("absent.json")},
	} {
		if out, err = k.run("serve", "--config", k.config(strings.ReplaceAll(name, " ", "-")+".json", r02With(base, changes)), "--check"); err == nil {
			t.Fatalf("%s accepted: %s", name, out)
		}
	}
}

func TestFinalR02DevicesCommandEditsAllowlist(t *testing.T) {
	k, base := r02Kit(t)
	file := k.path("allowed.json")
	cfg := k.config("serve.json", r02With(base, map[string]any{"admission": "allowlist", "allowed_devices_file": file}))
	// The identity line `orbit status` prints for a replica.
	line := "[active]  CalebLaptop          device=" + r02Hex(0x35) + " pin=" + r02Hex(0xb9) + " endpoint=no endpoint"
	if out, err := k.run("devices", "add", "--config", cfg, "--label", "laptop", line); err != nil || !strings.Contains(out, "1 allowed device(s)") {
		t.Fatal("add from identity line", err, out)
	}
	if out, err := k.run("devices", "add", "--config", cfg, r02Hex(0x36), r02Hex(0xba)); err != nil || !strings.Contains(out, "2 allowed device(s)") {
		t.Fatal("add bare pair", err, out)
	}
	if out, err := k.run("devices", "add", "--config", cfg, line); err == nil {
		t.Fatal("duplicate device added", out)
	}
	if out, err := k.run("devices", "add", "--config", cfg, "device=xyz pin=abc"); err == nil {
		t.Fatal("malformed identity added", out)
	}
	out, err := k.run("devices", "list", "--config", cfg)
	if err != nil || !strings.Contains(out, "device="+r02Hex(0x35)+" pin="+r02Hex(0xb9)+" laptop") {
		t.Fatal("list", err, out)
	}
	if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("allowlist file not owner-only", info, err)
	}
	if out, err = k.run("serve", "--config", cfg, "--check"); err != nil || !strings.Contains(out, "allowlist (2 devices)") {
		t.Fatal("edited allowlist rejected by serve", err, out)
	}
	if out, err = k.run("devices", "remove", "--config", cfg, r02Hex(0x36)); err != nil || !strings.Contains(out, "1 allowed device(s)") {
		t.Fatal("remove", err, out)
	}
	if out, err = k.run("devices", "remove", "--config", cfg, r02Hex(0x35)); err == nil {
		t.Fatal("removed the last device under allowlist admission", out)
	}
	if out, err = k.run("devices", "list", "--config", k.config("inline.json", base)); err == nil {
		t.Fatal("devices managed a config without allowed_devices_file", out)
	}
}

func TestFinalR02MetricsReportAdmission(t *testing.T) {
	k, _ := r02Kit(t)
	cert, key := k.certificate("tls2", "10.20.30.40", time.Now().Add(90*24*time.Hour))
	p, err := prepare(serveConfig{Listen: "127.0.0.1:0", Profile: k.path("profile.json"), ServiceKey: k.service, TLSCert: cert, TLSKey: key, RelayBPS: 1 << 20, RelayDeviceBPS: 1 << 20, RelaySessionBytes: 1 << 20, Admission: "allowlist", AllowedDevices: []allowedDevice{{Device: r02Hex(1), Pin: r02Hex(2)}}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer p.service.Close()
	raw, _ := net.Listen("tcp", "127.0.0.1:0")
	defer raw.Close()
	text := p.metricsText(rendezvous.BoundedListener(raw), nil, time.Now())
	for _, want := range []string{"orbit_net_admission_allowlist 1", "orbit_net_admission_allowed_devices 1", "orbit_net_refusals_total{reason=\"not_admitted\"} 0", "orbit_net_admission_reload_failures_total 0"} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, r02Hex(1)) || strings.Contains(text, r02Hex(2)) {
		t.Fatal("metrics leaked an allowlisted identity")
	}
}

// The real binary re-reads the allowlist on SIGHUP and keeps the previous
// policy when the new file is malformed.
func TestFinalR02BinarySIGHUPReloadsAllowlist(t *testing.T) {
	k, base := r02Kit(t)
	root := testkit.NewDisposable(t)
	_ = os.Chmod(root, 0700)
	binary := filepath.Join(root, "orbit-net")
	build := exec.Command("go", "build", "-trimpath", "-o", binary, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
	probe, _ := net.Listen("tcp", "127.0.0.1:0")
	metricsAddr := probe.Addr().String()
	_ = probe.Close()
	file := r02File(t, k, "allowed.json", allowedDevice{Device: r02Hex(1), Pin: r02Hex(2)})
	cfg := k.config("serve.json", r02With(base, map[string]any{"admission": "allowlist", "allowed_devices_file": file, "metrics_listen": metricsAddr, "relay_state": filepath.Join(k.dir, "relay-month.json")}))
	if err := testkit.ValidateDestructiveTarget(root, binary); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	cmd := exec.Command(binary, "serve", "--config", cfg)
	cmd.Stderr, cmd.Stdout = &logs, &logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Signal(syscall.SIGTERM); _ = cmd.Wait() }()
	gauge := func(name string) string {
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			resp, err := http.Get("http://" + metricsAddr + "/metrics")
			if err != nil {
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			for _, l := range strings.Split(string(body), "\n") {
				if strings.HasPrefix(l, name+" ") {
					return strings.TrimPrefix(l, name+" ")
				}
			}
		}
		t.Fatal("metric unavailable", name, logs.String())
		return ""
	}
	waitGauge := func(name, want string) {
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			if gauge(name) == want {
				return
			}
		}
		t.Fatalf("%s != %s\n%s", name, want, logs.String())
	}
	waitGauge("orbit_net_admission_allowed_devices", "1")
	r02File(t, k, "allowed.json", allowedDevice{Device: r02Hex(1), Pin: r02Hex(2)}, allowedDevice{Device: r02Hex(3), Pin: r02Hex(4)})
	_ = cmd.Process.Signal(syscall.SIGHUP)
	waitGauge("orbit_net_admission_allowed_devices", "2")
	_ = os.WriteFile(file, []byte(`{"version":"1","devices":[{"device":"bad"}]}`), 0600)
	_ = cmd.Process.Signal(syscall.SIGHUP)
	waitGauge("orbit_net_admission_reload_failures_total", "1")
	if gauge("orbit_net_admission_allowed_devices") != "2" {
		t.Fatal("malformed reload replaced the policy")
	}
	t.Log(strings.TrimSpace(logs.String()))
}

// TestFinalR02OldClientAgainstAllowlist runs prebuilt client programs (one
// unlisted device's production RelayRuntime, built from each tree) against a
// real allowlisted orbit-net and reports refused attempts per minute.
// Opt-in: ORBIT_FINAL_R02_CLIENTS="label=/path/client,..." and
// ORBIT_FINAL_R02_WINDOW (default 2m).
func TestFinalR02OldClientAgainstAllowlist(t *testing.T) {
	spec := os.Getenv("ORBIT_FINAL_R02_CLIENTS")
	if spec == "" {
		t.Skip("set ORBIT_FINAL_R02_CLIENTS")
	}
	window := 2 * time.Minute
	if v := os.Getenv("ORBIT_FINAL_R02_WINDOW"); v != "" {
		window, _ = time.ParseDuration(v)
	}
	var host string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		ip, _, _ := net.ParseCIDR(a.String())
		if ip != nil && ip.To4() != nil && ip.IsPrivate() && !ip.IsLoopback() {
			host = ip.String()
			break
		}
	}
	if host == "" {
		t.Skip("needs a private nonloopback interface")
	}
	root := testkit.NewDisposable(t)
	_ = os.Chmod(root, 0700)
	binary := filepath.Join(root, "orbit-net")
	build := exec.Command("go", "build", "-trimpath", "-o", binary, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
	for _, entry := range strings.Split(spec, ",") {
		label, client, _ := strings.Cut(entry, "=")
		t.Run(label, func(t *testing.T) {
			t.Parallel()
			k := newKit(t)
			free := func() string {
				l, _ := net.Listen("tcp", net.JoinHostPort(host, "0"))
				defer l.Close()
				return l.Addr().String()
			}
			listen := free()
			if out, err := k.sign(k.template("t.json", listen, 1, []string{}), "self_hosted", "profile.json"); err != nil {
				t.Fatal(err, out)
			}
			cert, key := k.certificate("tls", host, time.Now().Add(24*time.Hour))
			probe, _ := net.Listen("tcp", "127.0.0.1:0")
			metricsAddr := probe.Addr().String()
			_ = probe.Close()
			cfg := k.config("serve.json", map[string]any{"listen": listen, "profile": k.path("profile.json"), "service_key": k.service, "tls_cert": cert, "tls_key": key, "metrics_listen": metricsAddr, "relay_bps": 1 << 20, "relay_device_bps": 1 << 19, "relay_session_bytes": 1 << 30, "relay_month_bytes": 1 << 30, "relay_state": k.path("relay-month.json"), "admission": "allowlist", "allowed_devices": []allowedDevice{{Device: r02Hex(1), Pin: r02Hex(2)}}})
			if err := testkit.ValidateDestructiveTarget(root, binary); err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			service := exec.Command(binary, "serve", "--config", cfg)
			service.Stderr = &logs
			if err := service.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = service.Process.Signal(syscall.SIGTERM); _ = service.Wait() }()
			refusals := func() map[string]string {
				out := map[string]string{}
				for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
					resp, err := http.Get("http://" + metricsAddr + "/metrics")
					if err != nil {
						continue
					}
					body, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					for _, l := range strings.Split(string(body), "\n") {
						if strings.HasPrefix(l, "orbit_net_refusals_total{") {
							name, value, _ := strings.Cut(l, " ")
							out[strings.TrimSuffix(strings.TrimPrefix(name, "orbit_net_refusals_total{reason=\""), "\"}")] = value
						}
					}
					return out
				}
				t.Fatal("metrics unavailable", logs.String())
				return nil
			}
			refusals()
			cmd := exec.Command(client, "--selection", k.path("profile.json"), "--roots", cert, "--window", window.String())
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatal(err, string(output))
			}
			got := refusals()
			var n float64
			fmt.Sscan(got["not_admitted"], &n)
			t.Logf("R02 %s client, %s unlisted: refusals %v; not_admitted per minute=%.1f; client=%s", label, window, got, n/window.Minutes(), strings.TrimSpace(string(output)))
		})
	}
}
