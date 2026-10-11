package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// R03: one command produces a complete, owner-only, self-validating service.
func TestFinalR03InitCreatesCheckedService(t *testing.T) {
	base := t.TempDir()
	_ = os.Chmod(base, 0700)
	dir := filepath.Join(base, "svc")
	k := &w13Kit{t: t, dir: base}
	out, err := k.run("init", "--dir", dir, "--origin", "https://10.20.30.40:8443", "--stun", "10.20.30.40:3478")
	if err != nil {
		t.Fatal(err, out)
	}
	for _, want := range []string{"orbit network set --mode self_hosted --profile-file profile.json --service-roots service-ca.pem", "orbit-net devices add --config " + filepath.Join(dir, "serve.json"), "offline/authority.key", "8443/tcp", "3478/udp"} {
		if !strings.Contains(out, want) {
			t.Fatalf("init output missing %q:\n%s", want, out)
		}
	}
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if want := os.FileMode(0600); info.IsDir() {
			want = 0700
			if info.Mode().Perm() != want {
				t.Errorf("%s mode %v", path, info.Mode().Perm())
			}
		} else if info.Mode().Perm() != want {
			t.Errorf("%s mode %v", path, info.Mode().Perm())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "serve.json")
	if out, err = k.run("serve", "--config", cfg, "--check"); err == nil || !strings.Contains(out, "allowed_devices_file") {
		t.Fatal("service checked clean with no allowed device", out)
	}
	if out, err = k.run("devices", "add", "--config", cfg, "device="+r02Hex(5)+" pin="+r02Hex(6)); err != nil {
		t.Fatal(err, out)
	}
	if out, err = k.run("serve", "--config", cfg, "--check"); err != nil || !strings.Contains(out, "self_hosted") || !strings.Contains(out, "admission allowlist (1 devices, 0 by member sponsorship)") {
		t.Fatal("initialized service rejected", err, out)
	}
	if out, err = k.run("profile", "verify", "--profile", filepath.Join(dir, "profile.json")); err != nil || !strings.Contains(out, "environment: self_hosted") || !strings.Contains(out, "stun:        10.20.30.40:3478") {
		t.Fatal("profile", err, out)
	}
	// Tamper: a changed profile byte and a loosened key both fail the check.
	profile := filepath.Join(dir, "profile.json")
	original, _ := os.ReadFile(profile)
	_ = os.WriteFile(profile, []byte(strings.Replace(string(original), "Self-hosted Orbit service", "Self-hosted Orbit servicf", 1)), 0600)
	if out, err = k.run("serve", "--config", cfg, "--check"); err == nil {
		t.Fatal("tampered profile accepted", out)
	}
	_ = os.WriteFile(profile, original, 0600)
	_ = os.Chmod(filepath.Join(dir, "service.key"), 0644)
	if out, err = k.run("serve", "--config", cfg, "--check"); err == nil {
		t.Fatal("group-readable service key accepted", out)
	}
	_ = os.Chmod(filepath.Join(dir, "service.key"), 0600)
	if out, err = k.run("serve", "--config", cfg, "--check"); err != nil {
		t.Fatal("restored service rejected", out)
	}
	// Never overwrite: a second init into the same directory refuses.
	if out, err = k.run("init", "--dir", dir, "--origin", "https://10.20.30.40:8443"); err == nil {
		t.Fatal("init overwrote an existing service", out)
	}
}

func TestFinalR03InitRefusesBadInputs(t *testing.T) {
	base := t.TempDir()
	_ = os.Chmod(base, 0700)
	k := &w13Kit{t: t, dir: base}
	for name, args := range map[string][]string{
		"http origin":      {"--origin", "http://10.0.0.1:8443"},
		"path in origin":   {"--origin", "https://10.0.0.1:8443/x"},
		"STUN name":        {"--origin", "https://10.0.0.1:8443", "--stun", "stun.example:3478"},
		"STUN bind alone":  {"--origin", "https://10.0.0.1:8443", "--stun-bind", "10.0.0.1:3478"},
		"cert without key": {"--origin", "https://10.0.0.1:8443", "--tls-cert", "x.pem"},
		"short validity":   {"--origin", "https://10.0.0.1:8443", "--valid-for", "1h"},
		"missing origin":   {},
	} {
		dir := filepath.Join(base, strings.ReplaceAll(name, " ", "-"))
		if out, err := k.run(append([]string{"init", "--dir", dir}, args...)...); err == nil {
			t.Errorf("%s accepted: %s", name, out)
		}
	}
	// A DNS origin on 443 drops the port in the signed origins.
	dir := filepath.Join(base, "dns")
	if out, err := k.run("init", "--dir", dir, "--origin", "https://orbit.example.net"); err != nil {
		t.Fatal(err, out)
	}
	out, _ := k.run("profile", "verify", "--profile", filepath.Join(dir, "profile.json"))
	if !strings.Contains(out, "origin:      https://orbit.example.net\n") || !strings.Contains(out, "origin:      wss://orbit.example.net\n") {
		t.Fatal(out)
	}
}
