package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/rendezvous"
)

type w13Kit struct {
	t                        *testing.T
	dir                      string
	authority, service       string
	authorityPub, servicePub string
}

func newKit(t *testing.T) *w13Kit {
	k := &w13Kit{t: t, dir: t.TempDir()}
	if err := os.Chmod(k.dir, 0700); err != nil {
		t.Fatal(err)
	}
	k.authority, k.authorityPub = k.keygen("authority.key")
	k.service, k.servicePub = k.keygen("service.key")
	return k
}
func (k *w13Kit) path(name string) string { return filepath.Join(k.dir, name) }
func (k *w13Kit) run(args ...string) (string, error) {
	var out, errOut bytes.Buffer
	err := run(args, &out, &errOut)
	if err != nil {
		errOut.WriteString(err.Error())
	}
	return out.String() + errOut.String(), err
}
func (k *w13Kit) keygen(name string) (string, string) {
	k.t.Helper()
	out, err := k.run("keygen", "--out", k.path(name))
	if err != nil {
		k.t.Fatal(err, out)
	}
	return k.path(name), strings.TrimSpace(out)
}
func (k *w13Kit) template(name, origin string, epoch int, stun []string) string {
	k.t.Helper()
	b, _ := json.Marshal(map[string]any{"version": "1", "operator": "W13 rehearsal operator", "service_key": k.servicePub, "epoch": strconv.Itoa(epoch), "origins": []string{"https://" + origin, "wss://" + origin}, "stun": stun, "privacy": "Rehearsal only. Addresses and connection metadata are held in memory for at most ten minutes; no folder data is stored."})
	if err := os.WriteFile(k.path(name), b, 0600); err != nil {
		k.t.Fatal(err)
	}
	return k.path(name)
}
func (k *w13Kit) sign(template, environment, out string, extra ...string) (string, error) {
	return k.run(append([]string{"profile", "sign", "--authority-key", k.authority, "--template", template, "--environment", environment, "--valid-for", "720h", "--out", k.path(out)}, extra...)...)
}
func (k *w13Kit) certificate(name, host string, notAfter time.Time) (string, string) {
	k.t.Helper()
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host}, NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{host}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		k.t.Fatal(err)
	}
	pkcs8, _ := x509.MarshalPKCS8PrivateKey(key)
	cert, keyPath := k.path(name+".pem"), k.path(name+".key")
	_ = os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644)
	_ = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}), 0600)
	return cert, keyPath
}
func (k *w13Kit) config(name string, cfg map[string]any) string {
	k.t.Helper()
	b, _ := json.Marshal(cfg)
	if err := os.WriteFile(k.path(name), b, 0600); err != nil {
		k.t.Fatal(err)
	}
	return k.path(name)
}

func TestW13OperatorToolingRefusesUnsafeProfiles(t *testing.T) {
	k := newKit(t)
	if info, err := os.Stat(k.authority); err != nil || info.Mode().Perm() != 0600 || len(k.authorityPub) != 64 {
		t.Fatal("authority key not private", err)
	}
	if out, err := k.run("keygen", "--out", k.authority); err == nil {
		t.Fatal("keygen replaced an existing key", out)
	}
	private := k.template("private.json", "10.20.30.40:8443", 1, []string{"10.20.30.40:3478"})
	if out, err := k.sign(private, "release", "release.json"); err == nil || !strings.Contains(out, "public HTTPS") {
		t.Fatal("release profile accepted private origins", err, out)
	}
	out, err := k.sign(private, "self_hosted", "epoch1.json")
	if err != nil || !strings.Contains(out, "epoch:       1") || !strings.Contains(out, "digest:") {
		t.Fatal("self-hosted profile not signed", err, out)
	}
	if out, err = k.run("profile", "verify", "--profile", k.path("epoch1.json"), "--authority", k.authorityPub); err != nil {
		t.Fatal(err, out)
	}
	if out, err = k.run("profile", "verify", "--profile", k.path("epoch1.json"), "--authority", k.servicePub); err == nil {
		t.Fatal("verify accepted another authority", out)
	}
	if out, err = k.sign(private, "self_hosted", "rollback.json", "--previous", k.path("epoch1.json")); err == nil || !strings.Contains(out, "rollback") {
		t.Fatal("same epoch signed over previous", out)
	}
	next := k.template("next.json", "10.20.30.40:8443", 2, []string{})
	if out, err = k.sign(next, "development", "env.json", "--previous", k.path("epoch1.json")); err == nil {
		t.Fatal("environment change accepted under --previous", out)
	}
	if out, err = k.sign(next, "self_hosted", "epoch2.json", "--previous", k.path("epoch1.json")); err != nil {
		t.Fatal(err, out)
	}
	if out, err = k.sign(next, "self_hosted", "epoch2.json"); err == nil {
		t.Fatal("signing replaced an existing selection", out)
	}
}

func TestW13ServeCheckValidatesOperatorInputs(t *testing.T) {
	k := newKit(t)
	host := "10.20.30.40"
	template := k.template("profile-template.json", host+":8443", 1, []string{host + ":3478"})
	if out, err := k.sign(template, "self_hosted", "profile.json"); err != nil {
		t.Fatal(err, out)
	}
	cert, key := k.certificate("tls", host, time.Now().Add(24*time.Hour))
	wrongCert, wrongKey := k.certificate("wrong", "10.20.30.41", time.Now().Add(24*time.Hour))
	otherService, _ := k.keygen("other-service.key")
	base := map[string]any{"listen": "127.0.0.1:0", "profile": k.path("profile.json"), "service_key": k.service, "tls_cert": cert, "tls_key": key, "stun_listen": host + ":3478", "metrics_listen": "127.0.0.1:9464", "relay_bps": 1 << 20, "relay_device_bps": 1 << 19, "relay_session_bytes": 1 << 30}
	with := func(changes map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range base {
			out[k] = v
		}
		for k, v := range changes {
			if v == nil {
				delete(out, k)
			} else {
				out[k] = v
			}
		}
		return out
	}
	out, err := k.run("serve", "--config", k.config("good.json", base), "--check")
	if err != nil || !strings.Contains(out, "configuration valid for https://"+host+":8443") || !strings.Contains(out, "warning: serving a self_hosted profile") || !strings.Contains(out, "TLS certificate expires") {
		t.Fatal("valid configuration refused", err, out)
	}
	if err = os.Chmod(key, 0640); err != nil {
		t.Fatal(err)
	}
	if out, err = k.run("serve", "--config", k.path("good.json"), "--check"); err == nil {
		t.Fatal("group-readable TLS key accepted", out)
	}
	_ = os.Chmod(key, 0600)
	for name, changes := range map[string]map[string]any{
		"unknown field":            {"debug": true},
		"wrong host":               {"tls_cert": wrongCert, "tls_key": wrongKey},
		"mismatched key pair":      {"tls_key": wrongKey},
		"service key mismatch":     {"service_key": otherService},
		"STUN outside profile":     {"stun_listen": host + ":3479"},
		"public metrics":           {"metrics_listen": "0.0.0.0:9464"},
		"zero relay ceiling":       {"relay_bps": 0},
		"device above global":      {"relay_device_bps": 2 << 20},
		"missing profile":          {"profile": nil},
		"overlap without key":      {"overlap_profile": k.path("profile.json")},
		"origin outside profile":   {"origin": "https://10.20.30.41:8443"},
		"STUN bind without listen": {"stun_listen": nil, "stun_bind": "10.0.0.5:3478"},
		"STUN bind without port":   {"stun_bind": "10.0.0.5"},
	} {
		if out, err = k.run("serve", "--config", k.config(strings.ReplaceAll(name, " ", "-")+".json", with(changes)), "--check"); err == nil {
			t.Fatalf("%s accepted: %s", name, out)
		}
	}
	expiredCert, expiredKey := k.certificate("expired", host, time.Now().Add(-time.Minute))
	if out, err = k.run("serve", "--config", k.config("expired.json", with(map[string]any{"tls_cert": expiredCert, "tls_key": expiredKey})), "--check"); err == nil {
		t.Fatal("expired certificate accepted", out)
	}
	// Behind 1:1 NAT the reviewed public STUN address differs from the socket.
	if out, err = k.run("serve", "--config", k.config("nat.json", with(map[string]any{"stun_bind": "10.0.0.5:3478"})), "--check"); err != nil {
		t.Fatal("NAT STUN bind refused", out)
	}
	// Flags override the private config without rewriting it.
	if out, err = k.run("serve", "--config", k.path("good.json"), "--metrics-listen", "192.0.2.1:9464", "--check"); err == nil {
		t.Fatal("flag override not applied", out)
	}
}

func TestW13MetricsAreSanitized(t *testing.T) {
	k := newKit(t)
	host := "10.20.30.40"
	if out, err := k.sign(k.template("t.json", host+":8443", 1, []string{}), "development", "profile.json"); err != nil {
		t.Fatal(err, out)
	}
	cert, key := k.certificate("tls", host, time.Now().Add(90*24*time.Hour))
	p, err := prepare(serveConfig{Listen: "127.0.0.1:0", Profile: k.path("profile.json"), ServiceKey: k.service, TLSCert: cert, TLSKey: key, RelayBPS: 1 << 20, RelayDeviceBPS: 1 << 20, RelaySessionBytes: 1 << 20}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer p.service.Close()
	raw, _ := net.Listen("tcp", "127.0.0.1:0")
	defer raw.Close()
	text := p.metricsText(rendezvous.BoundedListener(raw), nil, time.Now())
	for _, want := range []string{"orbit_net_profile_valid{epoch=\"1\"} 1", "orbit_net_connections_active 0", "orbit_net_refusals_total{reason=\"quota\"} 0", "orbit_net_relay_limit_bytes_per_second 1048576"} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics missing %q:\n%s", want, text)
		}
	}
	for _, leak := range []string{host, "127.0.0.1", k.servicePub, k.authorityPub} {
		if strings.Contains(text, leak) {
			t.Fatalf("metrics leaked %q", leak)
		}
	}
}

// A restart after the rotation window must keep serving the current epoch: an
// overlap epoch that has only expired is skipped with a warning, while any
// other overlap fault still refuses to start.
func TestOverlapEpochExpiryDoesNotStopService(t *testing.T) {
	k := newKit(t)
	host := "10.20.30.40"
	if out, err := k.sign(k.template("t1.json", host+":8443", 1, []string{}), "development", "e1.json", "--valid-for", "1h"); err != nil {
		t.Fatal(err, out)
	}
	if out, err := k.sign(k.template("t2.json", host+":8443", 2, []string{}), "development", "e2.json", "--previous", k.path("e1.json")); err != nil {
		t.Fatal(err, out)
	}
	cert, key := k.certificate("tls", host, time.Now().Add(90*24*time.Hour))
	cfg := serveConfig{Listen: "127.0.0.1:0", Profile: k.path("e2.json"), ServiceKey: k.service, OverlapProfile: k.path("e1.json"), OverlapServiceKey: k.service, TLSCert: cert, TLSKey: key, RelayBPS: 1 << 20, RelayDeviceBPS: 1 << 20, RelaySessionBytes: 1 << 20}
	p, err := prepare(cfg, time.Now())
	if err != nil {
		t.Fatal("overlap during rotation", err)
	}
	p.service.Close()
	p, err = prepare(cfg, time.Now().Add(2*time.Hour))
	if err != nil {
		t.Fatal("expired overlap stopped the current epoch", err)
	}
	defer p.service.Close()
	if !strings.Contains(strings.Join(p.warnings, "\n"), "overlap profile epoch 1 expired") {
		t.Fatal("missing expired-overlap warning", p.warnings)
	}
	other, _ := k.keygen("other.key")
	cfg.OverlapServiceKey = other
	if _, err = prepare(cfg, time.Now()); err == nil {
		t.Fatal("overlap with the wrong service key accepted")
	}
}
