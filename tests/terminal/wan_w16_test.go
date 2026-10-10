package terminal_test

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/testkit"
)

// TestWANW16NativeRunnerRehearsal executes the delivered Python runner against
// production CLI/service binaries on private local roots. The archive is an
// explicitly synthetic harness fixture, not a release-package/WAN acceptance.
func TestWANW16NativeRunnerRehearsal(t *testing.T) {
	t.Setenv("ORBIT_DISABLE_PACKAGED_PROFILE", "1")
	base := testkit.NewDisposable(t)
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	ip := networkIP(t)
	o := &w13Operator{t: t, base: base, dir: filepath.Join(base, "operator"), ip: ip}
	for _, dir := range []string{o.dir, o.path("tls")} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	o.binary = w13Binary(t, base)
	o.port, o.udp, o.metricsPort = w13FreePort(t, ip, "tcp"), w13FreePort(t, ip, "udp"), w13FreePort(t, "127.0.0.1", "tcp")
	o.keygen("authority.key")
	service := o.keygen("service-1.key")
	o.sign("profile-1.json", 1, service, "2h", "")
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "W16 local rehearsal CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(3 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	o.caCert, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	o.caKey, o.serial = key, 1
	o.leaf(ip)
	roots := o.path("ca.pem")
	if err := os.WriteFile(roots, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	o.start(o.config("serve.json", nil))
	binary := buildOrbitBinary(t, base)
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	dist := filepath.Join(base, "dist")
	if err := os.Mkdir(dist, 0700); err != nil {
		t.Fatal(err)
	}
	arch := "amd64"
	if strings.Contains(execOutput(t, "uname", "-m"), "aarch64") {
		arch = "arm64"
	}
	name := "orbit-rehearsal-linux-" + arch + ".tar.gz"
	archive, err := os.Create(filepath.Join(dist, name))
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(archive)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "orbit", Mode: 0700, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	for _, close := range []func() error{tw.Close, gz.Close, archive.Close} {
		if err := close(); err != nil {
			t.Fatal(err)
		}
	}
	pack, err := os.ReadFile(filepath.Join(dist, name))
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"SHA256SUMS":            []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256(pack), name)),
		"release-manifest.json": []byte(`{"fixture":"W16 local rehearsal; no release acceptance"}`),
	} {
		if err := os.WriteFile(filepath.Join(dist, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// The CLI journey, then the keyboard TUI journey with a third device that is
	// reached through the namespace shell (here without a namespace).
	for _, variant := range []struct {
		name  string
		extra []string
	}{{"cli", nil}, {"tui-three-host", []string{"--journey", "tui", "--three-host"}}} {
		t.Run(variant.name, func(t *testing.T) {
			w16RehearsalRun(t, base, dist, ip, o.path("profile-1.json"), roots, variant.name, variant.extra)
		})
	}
}

func w16RehearsalRun(t *testing.T, base, dist, ip, profile, roots, name string, extra []string) {
	dir := filepath.Join(base, name)
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	third := ""
	if len(extra) > 0 {
		// Keep the socket directly in the marked, private fixture root. Nested
		// journey paths exceed Linux's Unix socket limit with a longer TMPDIR.
		socket := filepath.Join(base, "s.sock")
		shell := exec.Command("python3", "scripts/validation/wan_netns_shell.py", socket, "--idle", "900")
		shell.Dir = "../.."
		if err := shell.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = shell.Process.Kill(); _ = shell.Wait() })
		ready := false
		for i := 0; i < 100; i++ {
			if _, err := os.Stat(socket); err == nil {
				ready = true
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if !ready {
			t.Fatalf("namespace shell did not create its socket: %s", socket)
		}
		third = `,"third":{"host":"local","role":"Third","physical_network":"fixture-c","shell":"` + socket + `"}`
	}
	topology := filepath.Join(dir, "topology.json")
	// Labels merely exercise the parser. Actual report explicitly records rehearsal.
	if err := os.WriteFile(topology, []byte(`{"hosts":[{"host":"local","role":"Owner","physical_network":"fixture-a"},{"host":"local","role":"Joiner","physical_network":"fixture-b"}]`+third+`}`), 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "evidence")
	cmd := exec.Command("python3", append([]string{"scripts/validation/wan_native.py", "--dist", dist,
		"--topology", topology, "--route-targets", ip, "--output", out,
		"--rehearsal-profile", profile, "--rehearsal-roots", roots}, extra...)...)
	cmd.Dir = "../.."
	if output, err := cmd.CombinedOutput(); err != nil {
		if data, readErr := os.ReadFile(filepath.Join(out, "wan-native.json")); readErr == nil {
			t.Log(string(data))
		}
		t.Fatalf("local runner: %v\n%s", err, output)
	}
	var report struct {
		Success   bool   `json:"success"`
		Rehearsal bool   `json:"rehearsal"`
		Hosted    string `json:"hosted_default_acceptance"`
		WAN       string `json:"physical_wan_acceptance"`
		Hosts     []struct {
			Root string `json:"root"`
		} `json:"hosts"`
	}
	record, err := os.ReadFile(filepath.Join(out, "wan-native.json"))
	if err != nil || json.Unmarshal(record, &report) != nil || !report.Success || !report.Rehearsal || report.Hosted != "unexecuted" || report.WAN != "unexecuted" {
		t.Fatal("rehearsal evidence wrong", err, string(record))
	}
	if destination := os.Getenv("ORBIT_W16_REHEARSAL_REPORT"); destination != "" && name == "cli" {
		f, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := f.Write(record)
		closeErr := f.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatal(writeErr, closeErr)
		}
	}
	// Successful runs stop owned processes and remove only fresh marked roots;
	// failed runs retain private diagnostics. No personal/historical root is used.
	for _, host := range report.Hosts {
		t.Logf("cleaned private rehearsal root: %s", host.Root)
	}
}

func execOutput(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
