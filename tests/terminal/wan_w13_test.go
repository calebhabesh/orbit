package terminal_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/app"
	"github.com/calebhabesh/file-sync/internal/config"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"github.com/pion/stun/v4"
)

// w13Operator holds the disposable operator workspace: offline authority,
// online service keys, CA and the packaged executable.
type w13Operator struct {
	t                      *testing.T
	base, dir, binary, ip  string
	port, udp, metricsPort int
	caCert                 *x509.Certificate
	caKey                  ed25519.PrivateKey
	serial                 int64
}

func (o *w13Operator) path(name string) string { return filepath.Join(o.dir, name) }
func (o *w13Operator) origin() string          { return fmt.Sprintf("https://%s:%d", o.ip, o.port) }
func (o *w13Operator) run(args ...string) string {
	o.t.Helper()
	out, err := exec.Command(o.binary, args...).CombinedOutput()
	if err != nil {
		o.t.Fatalf("orbit-net %v: %v\n%s", args, err, out)
	}
	return string(out)
}
func (o *w13Operator) keygen(name string) string {
	o.t.Helper()
	return strings.TrimSpace(o.run("keygen", "--out", o.path(name)))
}
func (o *w13Operator) sign(out string, epoch int, servicePublic string, validFor string, previous string) {
	o.t.Helper()
	template := o.path(fmt.Sprintf("template-%d.json", epoch))
	b, _ := json.Marshal(map[string]any{"version": "1", "operator": "W13 disposable rehearsal operator", "service_key": servicePublic, "epoch": strconv.Itoa(epoch), "origins": []string{o.origin(), "wss" + strings.TrimPrefix(o.origin(), "https")}, "stun": []string{net.JoinHostPort(o.ip, strconv.Itoa(o.udp))}, "privacy": "Rehearsal only. The service sees device IDs, key pins, addresses and connection timing, kept in memory for at most ten minutes. It never sees file names or contents. No request logs are kept."})
	if err := os.WriteFile(template, b, 0600); err != nil {
		o.t.Fatal(err)
	}
	args := []string{"profile", "sign", "--authority-key", o.path("authority.key"), "--template", template, "--environment", "self_hosted", "--valid-for", validFor, "--out", o.path(out)}
	if previous != "" {
		args = append(args, "--previous", o.path(previous))
	}
	o.run(args...)
}

// leaf issues a CA-signed service certificate for host and installs it as the
// configured chain/key, as a certificate renewal hook would.
func (o *w13Operator) leaf(host string) *x509.Certificate {
	o.t.Helper()
	o.serial++
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	template := &x509.Certificate{SerialNumber: big.NewInt(o.serial), Subject: pkix.Name{CommonName: host}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(30 * 24 * time.Hour), IPAddresses: []net.IP{net.ParseIP(host)}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, o.caCert, pub, o.caKey)
	if err != nil {
		o.t.Fatal(err)
	}
	pkcs8, _ := x509.MarshalPKCS8PrivateKey(key)
	chain := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: o.caCert.Raw})...)
	for name, data := range map[string][]byte{"fullchain.pem": chain, "privkey.pem": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})} {
		if err = config.WritePrivate(filepath.Join(o.dir, "tls"), name, data); err != nil {
			o.t.Fatal(err)
		}
	}
	cert, _ := x509.ParseCertificate(der)
	return cert
}
func (o *w13Operator) config(name string, changes map[string]any) string {
	o.t.Helper()
	cfg := map[string]any{"listen": net.JoinHostPort(o.ip, strconv.Itoa(o.port)), "origin": o.origin(), "profile": o.path("profile-1.json"), "service_key": o.path("service-1.key"), "tls_cert": o.path("tls/fullchain.pem"), "tls_key": o.path("tls/privkey.pem"), "stun_listen": net.JoinHostPort(o.ip, strconv.Itoa(o.udp)), "metrics_listen": net.JoinHostPort("127.0.0.1", strconv.Itoa(o.metricsPort)), "relay_bps": 8 << 20, "relay_device_bps": 4 << 20, "relay_session_bytes": 1 << 30}
	for k, v := range changes {
		cfg[k] = v
	}
	b, _ := json.Marshal(cfg)
	if err := config.WritePrivate(o.dir, name, b); err != nil {
		o.t.Fatal(err)
	}
	return o.path(name)
}

type w13Service struct {
	cmd  *exec.Cmd
	log  string
	done chan error
}

func (o *w13Operator) start(configPath string) *w13Service {
	o.t.Helper()
	log := filepath.Join(o.base, "orbit-net-"+enrollmentRandom(o.t)[:8]+".log")
	out, err := os.Create(log)
	if err != nil {
		o.t.Fatal(err)
	}
	cmd := exec.Command(o.binary, "serve", "--config", configPath)
	cmd.Stdout, cmd.Stderr = out, out
	if err = cmd.Start(); err != nil {
		o.t.Fatal(err)
	}
	s := &w13Service{cmd: cmd, log: log, done: make(chan error, 1)}
	go func() { s.done <- cmd.Wait(); _ = out.Close() }()
	o.t.Cleanup(func() { s.stop(o.t, syscall.SIGKILL) })
	o.waitLog(s, "orbit-net: serving ")
	return s
}
func (s *w13Service) stop(t *testing.T, sig syscall.Signal) error {
	if s.cmd == nil {
		return nil
	}
	_ = s.cmd.Process.Signal(sig)
	select {
	case err := <-s.done:
		s.cmd = nil
		return err
	case <-time.After(20 * time.Second):
		_ = s.cmd.Process.Kill()
		t.Fatal("orbit-net did not stop within 20 seconds")
	}
	return nil
}
func (o *w13Operator) waitLog(s *w13Service, want string) string {
	o.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(s.log)
		if strings.Contains(string(data), want) {
			return string(data)
		}
		select {
		case err := <-s.done:
			s.cmd = nil
			o.t.Fatalf("orbit-net exited: %v\n%s", err, data)
		default:
		}
		time.Sleep(50 * time.Millisecond)
	}
	data, _ := os.ReadFile(s.log)
	o.t.Fatalf("orbit-net log missing %q:\n%s", want, data)
	return ""
}
func (o *w13Operator) metrics() string {
	o.t.Helper()
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics", o.metricsPort))
	if err != nil {
		o.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}
func w13Metric(t *testing.T, text, series string) float64 {
	t.Helper()
	m := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(series) + ` (\S+)$`).FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("metric %s missing:\n%s", series, text)
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	return v
}

// w13Process samples kernel-reported service resources; no in-process hooks.
func w13Process(t *testing.T, pid int) (fds int, rssKiB int) {
	t.Helper()
	entries, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", pid))
	if err != nil {
		t.Fatal(err)
	}
	status, _ := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	for _, line := range strings.Split(string(status), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			rssKiB, _ = strconv.Atoi(strings.Fields(line)[1])
		}
	}
	return len(entries), rssKiB
}

func w13FreePort(t *testing.T, ip, network string) int {
	t.Helper()
	if network == "udp" {
		c, err := net.ListenPacket("udp", net.JoinHostPort(ip, "0"))
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		return c.LocalAddr().(*net.UDPAddr).Port
	}
	l, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// w13Binary extracts the packaged archive named by ORBIT_NET_ARCHIVE, or builds
// the command for local runs. The archive layout is checked when supplied.
func w13Binary(t *testing.T, base string) string {
	t.Helper()
	binary := filepath.Join(base, "orbit-net")
	archive := os.Getenv("ORBIT_NET_ARCHIVE")
	if archive == "" {
		build := exec.Command("go", "build", "-o", binary, "./cmd/orbit-net")
		build.Dir = "../.."
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build orbit-net: %v\n%s", err, out)
		}
		return binary
	}
	f, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	seen := map[string]bool{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		name := h.Name[strings.Index(h.Name, "/")+1:]
		seen[name] = true
		if name == "bin/orbit-net" {
			data, _ := io.ReadAll(tr)
			if err = os.WriteFile(binary, data, 0700); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, want := range []string{"bin/orbit-net", "lib/systemd/system/orbit-net.service", "lib/systemd/system/orbit-net-alert.service", "lib/systemd/system/orbit-net-alert.timer", "share/doc/orbit-net/alert.example.json", "lib/sysusers.d/orbit-net.conf", "share/doc/orbit-net/orbit-net-operator.md", "share/doc/orbit-net/serve.example.json", "share/doc/orbit-net/profile-template.example.json", "share/doc/orbit-net/LICENSE", "share/doc/orbit-net/NOTICE"} {
		if !seen[want] {
			t.Fatalf("archive missing %s", want)
		}
	}
	return binary
}

func TestWANW13PackagedSelfHostRehearsal(t *testing.T) {
	base := testkit.NewDisposable(t)
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	var ip string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		host, _, _ := net.ParseCIDR(a.String())
		if host != nil && host.To4() != nil && host.IsPrivate() && !host.IsLoopback() {
			ip = host.String()
			break
		}
	}
	if ip == "" {
		t.Fatal("rehearsal requires a private nonloopback IPv4 interface")
	}
	o := &w13Operator{t: t, base: base, dir: filepath.Join(base, "operator"), ip: ip}
	for _, dir := range []string{o.dir, o.path("tls"), filepath.Join(base, "user")} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	o.binary = w13Binary(t, base)
	o.port, o.udp, o.metricsPort = w13FreePort(t, ip, "tcp"), w13FreePort(t, ip, "udp"), w13FreePort(t, "127.0.0.1", "tcp")
	t.Log(strings.TrimSpace(o.run("version")))

	// Operator provisioning: offline authority, online service key, private CA.
	authority := o.keygen("authority.key")
	service1 := o.keygen("service-1.key")
	o.sign("profile-1.json", 1, service1, "2h", "")
	if !strings.Contains(o.run("profile", "verify", "--profile", o.path("profile-1.json"), "--authority", authority), "epoch:       1") {
		t.Fatal("verify did not describe epoch 1")
	}
	caPub, caKey, _ := ed25519.GenerateKey(rand.Reader)
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "W13 disposable CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(365 * 24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caPub, caKey)
	if err != nil {
		t.Fatal(err)
	}
	o.caCert, _ = x509.ParseCertificate(caDER)
	o.caKey, o.serial = caKey, 1
	firstLeaf := o.leaf(ip)
	userCA := filepath.Join(base, "user", "service-ca.pem")
	if err = os.WriteFile(userCA, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0600); err != nil {
		t.Fatal(err)
	}
	cfg1 := o.config("serve-1.json", nil)
	if out := o.run("serve", "--config", cfg1, "--check"); !strings.Contains(out, "configuration valid for "+o.origin()) {
		t.Fatal("check did not validate", out)
	}
	svc := o.start(cfg1)
	pid := svc.cmd.Process.Pid
	if resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", o.metricsPort)); err != nil || resp.StatusCode != 200 {
		t.Fatal("health check failed", err)
	}
	if _, err = net.Dial("tcp", net.JoinHostPort(ip, strconv.Itoa(o.metricsPort))); err == nil {
		t.Fatal("metrics reachable on a nonloopback address")
	}

	// Devices use production binaries with no system trust for the private CA.
	emptyRoots := filepath.Join(base, "user", "no-system-roots.pem")
	_ = os.WriteFile(emptyRoots, nil, 0600)
	orbit := buildOrbitBinary(t, base)
	emptyDir := filepath.Join(base, "no-system-roots")
	if err = os.Mkdir(emptyDir, 0700); err != nil {
		t.Fatal(err)
	}
	cliEnv := append(os.Environ(), "SSL_CERT_FILE="+emptyRoots, "SSL_CERT_DIR="+emptyDir)
	call := func(args ...string) (tc.Result, string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, orbit, args...)
		cmd.Env = cliEnv
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		var r tc.Result
		_ = json.Unmarshal(stdout.Bytes(), &r)
		return r, stdout.String() + stderr.String(), err
	}
	ok := func(args ...string) tc.Result {
		t.Helper()
		r, out, err := call(args...)
		if err != nil {
			t.Fatalf("orbit %v: %v\n%s", args, err, out)
		}
		return r
	}
	states := []string{filepath.Join(base, "a"), filepath.Join(base, "b")}
	for _, dir := range states {
		dir := dir
		t.Cleanup(func() {
			if err := testkit.ValidateDestructiveTarget(base, dir); err == nil {
				_ = app.StopAgent(dir, 10*time.Second)
			}
		})
	}
	reviews := 0
	review := func(dir, profile string, roots bool) (tc.Result, string, error) {
		reviews++
		file := filepath.Join(base, "user", fmt.Sprintf("network-review-%d.json", reviews))
		args := []string{"network", "preview", "--state", dir, "--mode", "self_hosted", "--profile-file", o.path(profile), "--review-file", file, "--json"}
		if roots {
			args = append(args, "--service-roots", userCA)
		}
		r, out, err := call(args...)
		if err != nil {
			return r, out, err
		}
		return call("network", "apply", "--state", dir, "--review-file", file, "--json")
	}
	status := func(dir string) *tc.NetworkStatus {
		t.Helper()
		return ok("network", "status", "--state", dir, "--json").Network
	}
	waitReady := func(dir string, want bool) *tc.NetworkStatus {
		t.Helper()
		var s *tc.NetworkStatus
		deadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(deadline) {
			if s = status(dir); s.Ready == want {
				return s
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Fatalf("readiness %v not reached: %+v", want, s)
		return nil
	}

	// Without reviewed trust, normal verification refuses the private CA.
	if _, out, err := review(states[0], "profile-1.json", false); err != nil {
		t.Fatal(err, out)
	}
	time.Sleep(3 * time.Second)
	if s := status(states[0]); s.Ready || s.ServiceTrust != "system" {
		t.Fatalf("unverified service became ready: %+v", s)
	}
	if strings.Contains(o.metrics(), "orbit_net_controls 1") {
		t.Fatal("control admitted without verified trust")
	}
	for _, dir := range states {
		if _, out, err := review(dir, "profile-1.json", true); err != nil {
			t.Fatal(err, out)
		}
		if s := waitReady(dir, true); !strings.HasPrefix(s.ServiceTrust, "custom:") {
			t.Fatalf("custom trust not reported: %+v", s)
		}
	}

	// Pair the devices through the self-hosted relay with ordinary CLI steps.
	rootA, rootB := filepath.Join(base, "documents-a"), filepath.Join(base, "documents-b")
	if err = os.Mkdir(rootA, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(rootA, "from-a.txt"), []byte("W13 synthetic secret content A"), 0600); err != nil {
		t.Fatal(err)
	}
	setup := filepath.Join(base, "user", "create.json")
	ok("setup", "--state", states[0], "--root", rootA, "--label", "Laptop", "--name", "Documents", "--preview", "--review-file", setup, "--json")
	created := ok("setup", "--state", states[0], "--request-file", setup, "--timeout", "0", "--json")
	w06Wait(t, states[0], created.Operation.ID)
	inviteReview, invitation := filepath.Join(base, "user", "invite-review.json"), filepath.Join(base, "user", "invitation.json")
	ok("devices", "invite", "--state", states[0], "--folder", "Documents", "--preview", "--review-file", inviteReview, "--json")
	ok("devices", "invite", "--state", states[0], "--request-file", inviteReview, "--out", invitation, "--json")
	var inv tc.Invitation
	data, _ := os.ReadFile(invitation)
	if err = json.Unmarshal(data, &inv); err != nil {
		t.Fatal(err)
	}
	join := filepath.Join(base, "user", "join.json")
	ok("join", "--state", states[1], "--root", rootB, "--label", "Pi", "--name", "Documents", "--invitation-file", invitation, "--preview", "--review-file", join, "--json")
	joined := ok("join", "--state", states[1], "--request-file", join, "--timeout", "0", "--json")
	var pending tc.Result
	for deadline := time.Now().Add(70 * time.Second); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
		if pending = ok("devices", "requests", "--state", states[0], "--json"); len(pending.Requests) == 1 {
			break
		}
	}
	if len(pending.Requests) != 1 {
		t.Fatal("approval request did not arrive through the service")
	}
	approval := filepath.Join(base, "user", "approve.json")
	ok("devices", "requests", "show", "--state", states[0], "--device", "Pi", "--review-file", approval, "--json")
	ok("devices", "approve", "--state", states[0], "--review-file", approval, "--json")
	w06Wait(t, states[1], joined.Operation.ID)
	waitBytes := func(path, want string) {
		t.Helper()
		for deadline := time.Now().Add(120 * time.Second); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
			if b, _ := os.ReadFile(path); string(b) == want {
				return
			}
		}
		t.Fatalf("verified bytes did not arrive at %s", path)
	}
	transfer := func(from, to, name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(from, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		waitBytes(filepath.Join(to, name), content)
	}
	waitBytes(filepath.Join(rootB, "from-a.txt"), "W13 synthetic secret content A")
	transfer(rootB, rootA, "from-b.txt", "W13 synthetic secret content B")
	text := o.metrics()
	if w13Metric(t, text, "orbit_net_relay_bytes_total") == 0 || w13Metric(t, text, "orbit_net_controls") < 2 {
		t.Fatalf("relay path not exercised:\n%s", text)
	}
	cfgA, _ := config.Load(states[0])
	cfgB, _ := config.Load(states[1])
	logData, _ := os.ReadFile(svc.log)
	for _, secret := range []string{cfgA.DeviceID, cfgB.DeviceID, inv.KeyPin, inv.Capability, "from-a.txt", "synthetic secret", "Documents"} {
		if strings.Contains(text, secret) || strings.Contains(string(logData), secret) {
			t.Fatalf("operator output disclosed %q", secret)
		}
	}

	// Certificate rotation: SIGHUP installs a renewed chain without restart;
	// a wrong-host chain is refused and the valid one stays active.
	served := func() *big.Int {
		pool := x509.NewCertPool()
		pool.AddCert(o.caCert)
		conn, err := tls.Dial("tcp", net.JoinHostPort(ip, strconv.Itoa(o.port)), &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13})
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		return conn.ConnectionState().PeerCertificates[0].SerialNumber
	}
	if served().Cmp(firstLeaf.SerialNumber) != 0 {
		t.Fatal("initial certificate not served")
	}
	renewed := o.leaf(ip)
	_ = svc.cmd.Process.Signal(syscall.SIGHUP)
	o.waitLog(svc, "orbit-net: certificate reloaded")
	if served().Cmp(renewed.SerialNumber) != 0 {
		t.Fatal("renewed certificate not served after SIGHUP")
	}
	o.leaf("10.255.255.254")
	_ = svc.cmd.Process.Signal(syscall.SIGHUP)
	o.waitLog(svc, "certificate reload failed; keeping current certificate")
	if served().Cmp(renewed.SerialNumber) != 0 {
		t.Fatal("rejected certificate replaced the valid one")
	}
	text = o.metrics()
	if w13Metric(t, text, "orbit_net_certificate_reloads_total") != 1 || w13Metric(t, text, "orbit_net_certificate_reload_failures_total") != 1 {
		t.Fatal("reload counters wrong", text)
	}
	o.leaf(ip) // restore a valid chain for later restarts

	// Overload: a socket flood is capped before TLS and pre-auth requests hit
	// finite quotas; the process stays inside its declared FD/memory envelope.
	baseFDs, baseRSS := w13Process(t, pid)
	var flood []net.Conn
	for i := 0; i < 320; i++ {
		c, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(o.port)), 2*time.Second)
		if err == nil {
			flood = append(flood, c)
		}
	}
	time.Sleep(500 * time.Millisecond)
	peakFDs, peakRSS := w13Process(t, pid)
	text = o.metrics()
	active, refused := w13Metric(t, text, "orbit_net_connections_active"), w13Metric(t, text, "orbit_net_connections_refused_total")
	for _, c := range flood {
		_ = c.Close()
	}
	if active > 256 || refused == 0 || peakFDs > 256+64 || peakRSS > 256<<10 {
		t.Fatalf("overload bounds: active=%v refused=%v fds=%d rss=%dKiB", active, refused, peakFDs, peakRSS)
	}
	pool := x509.NewCertPool()
	pool.AddCert(o.caCert)
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13}, MaxConnsPerHost: 4}}
	quota := 0
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 60; j++ {
				body := fmt.Sprintf(`{"version":"1","profile":"%064x","sender":"%064x","sender_pin":"%064x","certificate_der":""}`, 1, j+1, j+1)
				resp, err := client.Post(o.origin()+"/network/v1/challenge", "application/json", strings.NewReader(body))
				if err == nil {
					if resp.StatusCode == 429 {
						mu.Lock()
						quota++
						mu.Unlock()
					}
					_ = resp.Body.Close()
				}
			}
		}()
	}
	wg.Wait()
	text = o.metrics()
	t.Logf("overload: baseline fds=%d rss=%dKiB; flood peak fds=%d rss=%dKiB active=%v refused=%v; pre-auth 429=%d untrusted=%v", baseFDs, baseRSS, peakFDs, peakRSS, active, refused, quota, w13Metric(t, text, `orbit_net_refusals_total{reason="untrusted"}`))
	if w13Metric(t, text, `orbit_net_refusals_total{reason="untrusted"}`) == 0 {
		t.Fatal("unknown-profile requests were not refused")
	}

	// STUN abuse: one source receives at most ten answers per second; malformed
	// and oversized datagrams receive nothing; responses never exceed 3x.
	udp, err := net.ListenPacket("udp", net.JoinHostPort(ip, "0"))
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	stunAddr := &net.UDPAddr{IP: net.ParseIP(ip), Port: o.udp}
	request := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	start := time.Now()
	for i := 0; i < 100; i++ {
		_, _ = udp.WriteTo(request.Raw, stunAddr)
	}
	_, _ = udp.WriteTo(make([]byte, 20), stunAddr)
	_, _ = udp.WriteTo(append(append([]byte{}, request.Raw...), make([]byte, 200)...), stunAddr)
	answers, largest := 0, 0
	buffer := make([]byte, 2048)
	for {
		_ = udp.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, _, err := udp.ReadFrom(buffer)
		if err != nil {
			break
		}
		answers++
		if n > largest {
			largest = n
		}
	}
	window := int(time.Since(start)/time.Second) + 1
	text = o.metrics()
	t.Logf("STUN: 100 binding requests + 2 malformed in %s; answers=%d largest=%dB; dropped rate=%v invalid=%v", time.Since(start).Round(time.Millisecond), answers, largest, w13Metric(t, text, `orbit_net_stun_dropped_total{reason="rate"}`), w13Metric(t, text, `orbit_net_stun_dropped_total{reason="invalid"}`))
	if answers == 0 || answers > 10*(window+1) || largest > 3*len(request.Raw) || w13Metric(t, text, `orbit_net_stun_dropped_total{reason="rate"}`) == 0 {
		t.Fatalf("STUN limits violated: answers=%d window=%ds largest=%d", answers, window, largest)
	}

	// The service recovers once the flood ends.
	transfer(rootA, rootB, "after-overload.txt", "still synchronized")

	// Profile rotation: sign epoch 2 with a new service key and serve both epochs.
	// Devices keep working on epoch 1, then each reviews epoch 2 independently.
	service2 := o.keygen("service-2.key")
	o.sign("profile-2.json", 2, service2, "2h", "profile-1.json")
	cfg2 := o.config("serve-2.json", map[string]any{"profile": o.path("profile-2.json"), "service_key": o.path("service-2.key"), "overlap_profile": o.path("profile-1.json"), "overlap_service_key": o.path("service-1.key")})
	o.run("serve", "--config", cfg2, "--check")
	if err = svc.stop(t, syscall.SIGTERM); err != nil {
		t.Fatal("graceful stop failed", err)
	}
	stopped, _ := os.ReadFile(svc.log)
	if !strings.Contains(string(stopped), "orbit-net: stopping;") {
		t.Fatal("shutdown not reported")
	}
	waitReady(states[0], false)
	svc = o.start(cfg2)
	pid = svc.cmd.Process.Pid
	text = o.metrics()
	if w13Metric(t, text, `orbit_net_profile_valid{epoch="1"}`) != 1 || w13Metric(t, text, `orbit_net_profile_valid{epoch="2"}`) != 1 {
		t.Fatal("overlap epochs not both served", text)
	}
	for _, dir := range states {
		waitReady(dir, true)
	}
	transfer(rootA, rootB, "after-restart.txt", "epoch one after restart")
	if _, out, err := review(states[1], "profile-2.json", true); err != nil {
		t.Fatal(err, out)
	}
	waitReady(states[1], true)
	transfer(rootB, rootA, "mixed-epoch.txt", "B on epoch two, A on epoch one")
	transfer(rootA, rootB, "mixed-epoch-reverse.txt", "A on epoch one, B on epoch two")
	if _, out, err := review(states[0], "profile-2.json", true); err != nil {
		t.Fatal(err, out)
	}
	waitReady(states[0], true)
	transfer(rootA, rootB, "epoch-two.txt", "both on epoch two")
	if _, out, err := review(states[0], "profile-1.json", true); err == nil {
		t.Fatal("device accepted rollback to an older epoch", out)
	}
	routes, err := config.LoadPeerRoutes(states[0])
	digest2 := status(states[0]).Policy.Profile
	if err != nil || len(routes) != 1 || routes[0].Profile != digest2 {
		t.Fatal("rotation did not rebind durable routes", err, routes)
	}

	// Per-epoch expiry: a short-lived epoch 3 expires while epoch 2 keeps serving.
	service3 := o.keygen("service-3.key")
	o.sign("profile-3.json", 3, service3, "25s", "profile-2.json")
	cfg3 := o.config("serve-3.json", map[string]any{"profile": o.path("profile-3.json"), "service_key": o.path("service-3.key"), "overlap_profile": o.path("profile-2.json"), "overlap_service_key": o.path("service-2.key")})
	if out := o.run("serve", "--config", cfg3, "--check"); !strings.Contains(out, "profile epoch 3 expires in") {
		t.Fatal("short profile not warned", out)
	}
	_ = svc.stop(t, syscall.SIGTERM)
	svc = o.start(cfg3)
	for _, dir := range states {
		waitReady(dir, true)
	}
	time.Sleep(27 * time.Second)
	text = o.metrics()
	if w13Metric(t, text, `orbit_net_profile_valid{epoch="3"}`) != 0 || w13Metric(t, text, `orbit_net_profile_valid{epoch="2"}`) != 1 {
		t.Fatal("epoch expiry not isolated", text)
	}
	if _, out, err := review(states[1], "profile-3.json", true); err == nil {
		t.Fatal("device reviewed an expired profile", out)
	}
	if out, err := exec.Command(o.binary, "serve", "--config", cfg3, "--check").CombinedOutput(); err == nil {
		t.Fatal("expired primary profile passed check", string(out))
	}
	transfer(rootB, rootA, "after-expiry.txt", "epoch two unaffected by epoch three expiry")

	// Rollback hazard: restoring a configuration that drops the epoch devices
	// already reviewed strands them; the runbook requires forward fixes.
	_ = svc.stop(t, syscall.SIGTERM)
	svc = o.start(cfg1)
	time.Sleep(5 * time.Second)
	if s := status(states[0]); s.Ready {
		t.Fatal("device on epoch 2 became ready against an epoch-1-only service")
	}
	if w13Metric(t, o.metrics(), `orbit_net_refusals_total{reason="untrusted"}`) == 0 {
		t.Fatal("stranded devices not visible to the operator")
	}
	_ = svc.stop(t, syscall.SIGTERM)
	svc = o.start(cfg2)
	for _, dir := range states {
		waitReady(dir, true)
	}

	// Disable: Local-only closes the device's service use; stopping the service
	// leaves local capture working and reports the outage.
	controls := w13Metric(t, o.metrics(), "orbit_net_controls")
	file := filepath.Join(base, "user", "local-only.json")
	ok("network", "preview", "--state", states[1], "--mode", "local_only", "--review-file", file, "--json")
	ok("network", "apply", "--state", states[1], "--review-file", file, "--json")
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
		if w13Metric(t, o.metrics(), "orbit_net_controls") < controls {
			break
		}
	}
	if w13Metric(t, o.metrics(), "orbit_net_controls") >= controls || status(states[1]).Code != "LOCAL_ONLY" {
		t.Fatal("local-only device retained a service control")
	}
	if err = svc.stop(t, syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if s := waitReady(states[0], false); s.Code != "SERVICE_UNAVAILABLE" {
		t.Fatalf("outage not reported: %+v", s)
	}
	t.Logf("W13 rehearsal: packaged orbit-net pid lifecycle, private-CA self-hosting via reviewed trust, relay pairing/transfer, SIGHUP certificate rotation, overload/STUN limits, epoch rotation with mixed-epoch devices, rollback refusal, per-epoch expiry, local-only disable and graceful shutdown")
}
