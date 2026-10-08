package terminal_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/app"
	"github.com/calebhabesh/orbit/internal/config"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
	"github.com/calebhabesh/orbit/internal/testkit"
)

func TestWANW08BinaryLocalOnlyLANAfterApproval(t *testing.T) {
	verifyBinaryLocalOnlyTransport(t, false)
}
func TestWANW09BinaryLocalOnlyQUICAfterApproval(t *testing.T) {
	verifyBinaryLocalOnlyTransport(t, true)
}
func verifyBinaryLocalOnlyTransport(t *testing.T, quic bool) {
	var serviceRequests atomic.Int64
	service, selection, origin, roots, _ := w05Service(t, func() { serviceRequests.Add(1) })
	a, b := newW05Node(t, selection, origin, roots), newW05Node(t, selection, origin, roots)
	folder := w05Create(t, a, "source")
	inv := w05Invite(t, a, folder, "")
	mutation := w05Join(t, b, inv, "join")
	pending, err := w05MutateRetry(t, b, mutation)
	if err != nil || pending.Error != nil || pending.Join == nil || pending.Operation.Phase != "awaiting_approval" {
		t.Fatal("invitation admission", err, pending.Error)
	}
	if _, err = b.f.db.Membership(context.Background(), folder); err == nil {
		t.Fatal("discovery/relay granted pending folder access")
	}
	w05Approve(t, a, pending.Join.Request)
	// Preserve the real enrollment bucket; this is not a limiter bypass.
	time.Sleep(26 * time.Second)
	completed, err := w05MutateRetry(t, b, mutation)
	if err != nil || completed.Error != nil || !completed.Readiness.Ready() {
		t.Fatal("approved bootstrap", err, completed.Error)
	}
	rootA, rootB := filepath.Join(a.f.root, "source"), mutation.Join.Root
	a.stop()
	b.stop()
	a.f.close()
	b.f.close()
	base := testkit.NewDisposable(t)
	if err = os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	binary := buildOrbitBinary(t, base)
	ca := filepath.Join(base, "ca.pem")
	conn, err := tls.Dial("tcp", strings.TrimPrefix(origin, "https://"), &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: conn.ConnectionState().PeerCertificates[0].Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	start := func(n *w05Node) func() {
		t.Helper()
		cmd := exec.Command(binary, "serve", "--state", n.f.state, "--control-listen", "127.0.0.1:0", "--sync-interval", "5s", "--no-watch")
		cmd.Env = append(os.Environ(), "SSL_CERT_FILE="+ca)
		logPath := filepath.Join(n.f.state, "w09-daemon.log")
		logFile, logErr := os.Create(logPath)
		if logErr != nil {
			t.Fatal(logErr)
		}
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		t.Cleanup(func() {
			logFile.Close()
			if t.Failed() {
				data, _ := os.ReadFile(logPath)
				t.Log("daemon", string(data))
			}
		})
		if err = cmd.Start(); err != nil {
			t.Fatal(err)
		}
		stopped := false
		stop := func() {
			if stopped {
				return
			}
			stopped = true
			if err := testkit.ValidateDestructiveTarget(n.f.root, n.f.state); err != nil {
				t.Error(err)
				return
			}
			if err := app.StopAgent(n.f.state, 10*time.Second); err != nil {
				t.Error(err)
			}
			if err := cmd.Wait(); err != nil {
				t.Error(err)
			}
		}
		t.Cleanup(stop)
		until := time.Now().Add(15 * time.Second)
		for time.Now().Before(until) {
			var capabilities tc.Result
			// Startup polling must use live HTTP only: the stopped adapter could
			// acquire the state lock before this child and make its startup fail.
			e := (&controlclient.Client{StateDir: n.f.state}).Call(context.Background(), "POST", "/control/terminal/v1/query", tc.Query{Version: tc.Version, Kind: "capabilities"}, &capabilities)
			if e == nil {
				return stop
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("binary daemon not ready")
		return stop
	}
	waitBytes := func(path string, want []byte) {
		t.Helper()
		until := time.Now().Add(90 * time.Second)
		for time.Now().Before(until) {
			got, _ := os.ReadFile(path)
			if bytes.Equal(got, want) {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}

		for _, n := range []*w05Node{a, b} {
			result, e := (&controlclient.Client{StateDir: n.f.state}).Query(context.Background(), tc.Query{Version: tc.Version, Kind: "network_status"})
			data, _ := json.Marshal(result.Network)
			t.Log("network", e, string(data))
			log, _ := os.ReadFile(filepath.Join(n.f.state, "w09-daemon.log"))
			t.Log("daemon", string(log))
		}
		t.Fatal("binary verified bytes absent", filepath.Base(path))
	}
	verifyRoutes := func(expected string) {
		t.Helper()
		for _, n := range []*w05Node{a, b} {
			result, e := (&controlclient.Client{StateDir: n.f.state}).Query(context.Background(), tc.Query{Version: tc.Version, Kind: "network_status"})
			if e != nil || result.Network == nil || len(result.Network.Observations) != 1 || result.Network.Observations[0].Route != expected {
				t.Fatal("actual dated route", e, result.Network)
			}
			if result.Network.Observations[0].ObservedAt == "" {
				t.Fatal("undated observation")
			}
		}
	}
	// Local-only consumes the same approved logical peers and pins, with no
	// directory/STUN/relay work. No endpoint/IP is configured in either state.
	for _, n := range []*w05Node{a, b} {
		policy, _ := config.LoadNetworkPolicy(n.f.state)
		policy.Mode = "local_only"
		policy.Profile = ""
		policy.LANAdvertising = true
		policy.Generation++
		if err = config.SaveNetworkPolicy(n.f.state, policy); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []*w05Node{a, b} {
		settings, _ := json.Marshal(map[string]any{"interfaces": []string{}, "listen": "", "disabled": false, "udp_disabled": !quic, "udp_listen": "[::]:0"})
		if err = config.WritePrivate(n.f.state, "direct-network.json", settings); err != nil {
			t.Fatal(err)
		}
	}
	beforeLocalServiceRequests := serviceRequests.Load()
	stopA, stopB := start(a), start(b)
	wantA, wantB := []byte("local-only binary A captured bytes"), []byte("local-only binary B captured bytes")
	if err = os.WriteFile(filepath.Join(rootA, "binary-a.txt"), wantA, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(rootB, "binary-b.txt"), wantB, 0600); err != nil {
		t.Fatal(err)
	}
	waitBytes(filepath.Join(rootB, "binary-a.txt"), wantA)
	waitBytes(filepath.Join(rootA, "binary-b.txt"), wantB)
	if quic {
		verifyRoutes("quic")
	} else {
		verifyRoutes("direct")
	}
	if serviceRequests.Load() != beforeLocalServiceRequests {
		t.Fatal("local-only made public service requests")
	}
	// Actual directory/relay outage must leave daemon local capture alive.
	if err = service.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(rootA, "outage.txt"), []byte("local capture after actual service shutdown"), 0600); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(15 * time.Second)
	for time.Now().Before(until) {
		result, e := (&controlclient.Client{StateDir: a.f.state}).Query(context.Background(), tc.Query{Version: tc.Version, Kind: "history", Folder: inv.Folder, Path: "outage.txt", Limit: 20})
		if e == nil && len(result.Versions) == 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	stopA()
	stopB()
	a.f.open()
	b.f.open()
	w05AssertFiles(t, a, b, folder, rootA, rootB)
	for _, name := range []string{"binary-a.txt", "binary-b.txt"} {
		ah, e := a.f.db.Heads(context.Background(), folder, name)
		if e != nil {
			t.Fatal(e)
		}
		bh, e := b.f.db.Heads(context.Background(), folder, name)
		if e != nil {
			t.Fatal(e)
		}
		left, _ := json.Marshal(ah)
		right, _ := json.Marshal(bh)
		if !bytes.Equal(left, right) || len(ah) != 1 || a.f.db.VerifyManifest(ah[0].Manifest) != nil || b.f.db.VerifyManifest(bh[0].Manifest) != nil {
			t.Fatal("binary exact head/hash", name)
		}
	}
	cfgA, _ := config.Load(a.f.state)
	cfgB, _ := config.Load(b.f.state)
	if cfgA.DeviceID != inv.Inviter || cfgB.DeviceID != hex.EncodeToString(b.id.DeviceID[:]) {
		t.Fatal("binary identity changed")
	}
	heads, e := a.f.db.Heads(context.Background(), folder, "outage.txt")
	if e != nil || len(heads) != 1 || a.f.db.VerifyManifest(heads[0].Manifest) != nil {
		t.Fatal("outage local capture", e)
	}
	t.Log("Actual invitation/pending denial/exact approval; two production binaries discover known peers without configured IPs; two-way LAN bytes/heads/hashes; zero service requests in Local-only; actual service shutdown preserves capture. One host, no physical WAN claim.")
}

func TestWANW08BinaryOptionalCollisionFreshRelayOnboarding(t *testing.T) {
	occupied, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	w06BinaryReviewedRelayJourney(t, occupied.Addr().String(), true)
	base := testkit.NewDisposable(t)
	dir := filepath.Join(base, "manual-state")
	if _, err = app.Initialize(context.Background(), dir, app.SystemDependencies()); err != nil {
		t.Fatal(err)
	}
	binary := buildOrbitBinary(t, base)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "serve", "--state", dir, "--peer-listen", occupied.Addr().String(), "--no-watch")
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "listen for peers") {
		t.Fatal("explicit manual listener failure semantics changed", err, string(output))
	}
}

func TestWANW09BinaryOptionalUDPCollisionRelayOnboarding(t *testing.T) {
	tcp, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	udp, err := net.ListenPacket("udp", tcp.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	w06BinaryReviewedRelayJourney(t, tcp.Addr().String(), true, udp.LocalAddr().String())
}
