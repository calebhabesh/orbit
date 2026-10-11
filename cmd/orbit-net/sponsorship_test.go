package main

import (
	"encoding/json"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/rendezvous"
)

// Member sponsorship configuration, the devices command and reload.

func sponsoredFixture(t *testing.T, k *w13Kit, name string, devices ...rendezvous.SponsoredDevice) string {
	t.Helper()
	if err := (sponsorFile(k.path(name))).SaveSponsored(devices); err != nil {
		t.Fatal(err)
	}
	return k.path(name)
}

func TestSponsorshipConfigAndDevicesCommand(t *testing.T) {
	t.Setenv("STATE_DIRECTORY", "")
	k, base := r02Kit(t)
	owner := allowedDevice{Device: r02Hex(1), Pin: r02Hex(2), Label: "pc"}
	file := r02File(t, k, "allowed.json", owner)
	laptop := rendezvous.SponsoredDevice{Device: r02Hex(3), Pin: r02Hex(4), Sponsor: owner.Device, Added: 1760000000}
	sponsored := sponsoredFixture(t, k, "sponsored.json", laptop)
	cfg := k.config("serve.json", r02With(base, map[string]any{"admission": "allowlist", "allowed_devices_file": file, "sponsorship": true, "sponsored_devices_file": sponsored}))
	out, err := k.run("serve", "--config", cfg, "--check")
	if err != nil || !strings.Contains(out, "allowlist (2 devices, 1 by member sponsorship)") {
		t.Fatal("sponsorship check", err, out)
	}
	if out, err = k.run("devices", "list", "--config", cfg); err != nil || !strings.Contains(out, "1 sponsored device(s)") || !strings.Contains(out, "device="+laptop.Device+" pin="+laptop.Pin+" sponsor="+owner.Device) {
		t.Fatal("list sponsored", err, out)
	}
	// Removing a sponsored device revokes it, so its peers cannot re-admit it.
	if out, err = k.run("devices", "remove", "--config", cfg, laptop.Device); err != nil {
		t.Fatal("revoke sponsored", err, out)
	}
	if out, err = k.run("serve", "--config", cfg, "--check"); err != nil || !strings.Contains(out, "allowlist (1 devices, 0 by member sponsorship)") {
		t.Fatal("revoked device still admitted", err, out)
	}
	if out, err = k.run("devices", "remove", "--config", cfg, laptop.Device); err == nil {
		t.Fatal("revoked twice", out)
	}
	// Adding the device lists it statically and lifts the revocation.
	if out, err = k.run("devices", "add", "--config", cfg, laptop.Device, laptop.Pin); err != nil {
		t.Fatal("re-add", err, out)
	}
	f, _ := readAllowlist(file)
	if len(f.Revoked) != 0 || len(f.Devices) != 2 {
		t.Fatalf("allowlist after re-add %+v", f)
	}
	// The sponsored list defaults to the relay_state directory.
	if out, err = k.run("serve", "--config", k.config("default.json", r02With(base, map[string]any{"admission": "allowlist", "allowed_devices_file": file, "sponsorship": true, "relay_state": k.path("relay-month.json")})), "--check"); err != nil || !strings.Contains(out, "by member sponsorship") {
		t.Fatal("default sponsored path", err, out)
	}
	for name, changes := range map[string]map[string]any{
		"sponsorship while open": {"sponsorship": true, "relay_state": k.path("relay-month.json")},
		"no place to keep it":    {"admission": "allowlist", "allowed_devices_file": file, "sponsorship": true},
		"malformed sponsored":    {"admission": "allowlist", "allowed_devices_file": file, "sponsorship": true, "sponsored_devices_file": r02File(t, k, "not-sponsored.json", owner)},
	} {
		if out, err = k.run("serve", "--config", k.config(strings.ReplaceAll(name, " ", "-")+".json", r02With(base, changes)), "--check"); err == nil {
			t.Fatalf("%s accepted: %s", name, out)
		}
	}
}

// Reload keeps the running sponsored list, drops revoked devices from it and
// saves the result; toggling sponsorship needs a restart.
func TestSponsorshipReloadKeepsAndPrunes(t *testing.T) {
	k, base := r02Kit(t)
	owner := allowedDevice{Device: r02Hex(1), Pin: r02Hex(2)}
	file := r02File(t, k, "allowed.json", owner)
	laptop := rendezvous.SponsoredDevice{Device: r02Hex(3), Pin: r02Hex(4), Sponsor: owner.Device, Added: 1}
	phone := rendezvous.SponsoredDevice{Device: r02Hex(5), Pin: r02Hex(6), Sponsor: laptop.Device, Added: 2}
	sponsored := sponsoredFixture(t, k, "sponsored.json", laptop, phone)
	settings := r02With(base, map[string]any{"admission": "allowlist", "allowed_devices_file": file, "sponsorship": true, "sponsored_devices_file": sponsored})
	cfgPath := k.config("serve.json", settings)
	var cfg serveConfig
	b, _ := json.Marshal(settings)
	_ = json.Unmarshal(b, &cfg)
	p, err := prepare(cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer p.service.Close()
	p.configFile = cfgPath
	if got := p.service.Admission().Sponsored(); len(got) != 2 {
		t.Fatal("loaded", got)
	}
	f, _ := readAllowlist(file)
	f.Revoked = []string{phone.Device}
	_ = writeAllowlist(file, f)
	var log strings.Builder
	p.reloadAdmission(&log)
	if got := p.service.Admission().Sponsored(); len(got) != 1 || got[0].Device != laptop.Device {
		t.Fatal("reload", got, log.String())
	}
	stored, err := readSponsored(sponsored)
	if err != nil || len(stored) != 1 || stored[0].Device != laptop.Device {
		t.Fatal("pruned list not saved", stored, err)
	}
	if info, err := os.Stat(sponsored); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("sponsored file not owner-only", info, err)
	}
	k.config("serve.json", r02With(settings, map[string]any{"sponsorship": false, "sponsored_devices_file": ""}))
	p.reloadAdmission(&log)
	if !p.service.Admission().Sponsoring() || p.admissionFailures.Load() != 1 {
		t.Fatal("sponsorship toggled by reload", log.String())
	}
	raw, _ := net.Listen("tcp", "127.0.0.1:0")
	defer raw.Close()
	text := p.metricsText(rendezvous.BoundedListener(raw), nil, time.Now())
	for _, want := range []string{"orbit_net_admission_sponsorship 1", "orbit_net_admission_sponsored_devices 1", "orbit_net_admission_allowed_devices 2"} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, laptop.Device) {
		t.Fatal("metrics leaked a sponsored identity")
	}
}
