package terminal_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/controlclient"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/testkit"
)

// This child uses the complete production daemon lifecycle, scheduler, listeners
// and watcher. All firewall/address mutations are restricted to an owned namespace.
func TestWANW11WholeDaemonRoamingAndMixedProgress(t *testing.T) {
	if os.Getenv("ORBIT_W11_NAMESPACE") != "isolated-marked-namespace" {
		t.Skip("requires marked disposable namespace runner")
	}
	if err := testkit.ValidateNetworkNamespace(os.Getenv("TMPDIR"), os.Getenv("ORBIT_W11_PARENT_NETNS")); err != nil {
		t.Fatal(err)
	}
	_, selection, origin, roots, _ := w05Service(t)
	a, b := newW05Node(t, selection, origin, roots), newW05Node(t, selection, origin, roots)
	folder := w05Create(t, a, "source")
	inv := w05Invite(t, a, folder, "")
	join := w05Join(t, b, inv, "join")
	pending, err := w05MutateRetry(t, b, join)
	if err != nil || pending.Error != nil {
		t.Fatal(pending, err)
	}
	w05Approve(t, a, pending.Join.Request)
	time.Sleep(26 * time.Second)
	completed, err := w05MutateRetry(t, b, join)
	if err != nil || completed.Error != nil || !completed.Readiness.Ready() {
		t.Fatal(completed, err)
	}
	rootA, rootB := filepath.Join(a.f.root, "source"), join.Join.Root
	a.stop()
	b.stop()
	a.f.close()
	b.f.close()
	base := testkit.NewDisposable(t)
	if err = os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	conn, err := tls.Dial("tcp", strings.TrimPrefix(origin, "https://"), &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13})
	if err != nil {
		t.Fatal(err)
	}
	ca := filepath.Join(base, "ca.pem")
	err = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: conn.ConnectionState().PeerCertificates[0].Raw}), 0600)
	conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []*w05Node{a, b} {
		policy, e := config.LoadNetworkPolicy(n.f.state)
		if e != nil {
			t.Fatal(e)
		}
		policy.LANAdvertising = true
		policy.Generation++
		policy.Timing = protocol.RouteTiming{ProbeMS: 10000, CooldownMS: 20000, PollMS: 500, QuietMS: 2000}
		if os.Getenv("ORBIT_W11_DEFAULT_TIMING") == "1" {
			policy.Timing = protocol.RouteTiming{}
		}
		if e = config.SaveNetworkPolicy(n.f.state, policy); e != nil {
			t.Fatal(e)
		}
		settings := config.DefaultRuntimeSettings()
		settings.Concurrency = 2
		settings.BandwidthBytesPerSecond = 1 << 20
		if e = config.SaveRuntimeSettings(n.f.state, settings); e != nil {
			t.Fatal(e)
		}
		if e = config.WritePrivate(n.f.state, "direct-network.json", []byte(`{"interfaces":["orbit-w11"],"listen":"0.0.0.0:0","disabled":false,"udp_listen":"0.0.0.0:0","udp_disabled":false}`)); e != nil {
			t.Fatal(e)
		}
	}
	// Preserve service/control traffic while blocking every peer-data address.
	_, servicePort, _ := net.SplitHostPort(strings.TrimPrefix(origin, "https://"))
	run := func(command string, args ...string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, e := exec.CommandContext(ctx, command, args...).CombinedOutput()
		if e != nil {
			t.Fatal(command, args, string(out), e)
		}
	}
	block := func() {
		run("iptables", "-N", "ORBIT_W11")
		run("iptables", "-A", "ORBIT_W11", "-d", "127.0.0.0/8", "-j", "RETURN")
		run("iptables", "-A", "ORBIT_W11", "-p", "tcp", "--dport", servicePort, "-j", "RETURN")
		run("iptables", "-A", "ORBIT_W11", "-p", "tcp", "--sport", servicePort, "-j", "RETURN")
		run("iptables", "-A", "ORBIT_W11", "-d", "224.0.0.0/4", "-j", "RETURN")
		run("iptables", "-A", "ORBIT_W11", "-p", "tcp", "-j", "REJECT")
		run("iptables", "-A", "ORBIT_W11", "-p", "udp", "-j", "DROP")
		run("iptables", "-I", "OUTPUT", "1", "-j", "ORBIT_W11")
		run("ip6tables", "-N", "ORBIT_W11")
		run("ip6tables", "-A", "ORBIT_W11", "-d", "::1/128", "-j", "RETURN")
		run("ip6tables", "-A", "ORBIT_W11", "-p", "tcp", "-j", "REJECT")
		run("ip6tables", "-A", "ORBIT_W11", "-p", "udp", "-j", "DROP")
		run("ip6tables", "-I", "OUTPUT", "1", "-j", "ORBIT_W11")
	}
	unblock := func() {
		for _, cmd := range []string{"iptables", "ip6tables"} {
			run(cmd, "-D", "OUTPUT", "-j", "ORBIT_W11")
			run(cmd, "-F", "ORBIT_W11")
			run(cmd, "-X", "ORBIT_W11")
		}
	}
	block()
	pa, pb := startW05Process(t, a.f, ca, ""), startW05Process(t, b.f, ca, "")
	t.Cleanup(func() {
		if t.Failed() {
			for _, p := range []*w05Process{pa, pb} {
				data, _ := os.ReadFile(p.log)
				t.Log("daemon failure transcript", string(data))
				_ = os.WriteFile(filepath.Join(os.Getenv("TMPDIR"), filepath.Base(p.log)), data, 0600)
			}
		}
	})
	query := func(n *w05Node) tc.NetworkObservation {
		t.Helper()
		r, e := (&controlclient.Client{StateDir: n.f.state}).Query(context.Background(), tc.Query{Version: tc.Version, Kind: "network_status"})
		if e != nil || r.Network == nil {
			t.Fatal(r, e)
		}
		if len(r.Network.Observations) == 0 {
			return tc.NetworkObservation{}
		}
		return r.Network.Observations[0]
	}
	waitRoute := func(want string) tc.NetworkObservation {
		t.Helper()
		start := time.Now()
		for time.Since(start) < 90*time.Second {
			obs := query(b)
			if obs.Route == want {
				t.Logf("whole daemon route %s after %s generation=%d", want, time.Since(start), obs.Generation)
				return obs
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("route absent", want, query(a), query(b))
		return tc.NetworkObservation{}
	}
	waitBytes := func(path string, want []byte) {
		t.Helper()
		start := time.Now()
		for time.Since(start) < 120*time.Second {
			data, _ := os.ReadFile(path)
			if bytes.Equal(data, want) {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("missing verified bytes", filepath.Base(path), query(b))
	}
	write := func(name string, data []byte) {
		t.Helper()
		if e := os.WriteFile(filepath.Join(rootA, name), data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	write("relay-start.txt", []byte("initial relay bytes"))
	waitBytes(filepath.Join(rootB, "relay-start.txt"), []byte("initial relay bytes"))
	initial := waitRoute("relay")
	unblock()
	direct := waitRoute("quic")
	// Native address/default-route changes occur while a multi-chunk version is in flight.
	large := make([]byte, 16*int(history.ChunkSize))
	for i := range large {
		large[i] = byte((i/int(history.ChunkSize) + i) % 251)
	}
	write("large.bin", large)
	time.Sleep(3 * time.Second)
	run("ip", "addr", "del", "10.23.45.1/24", "dev", "orbit-w11")
	// Keep the directory's address alive on another interface so only peer scope roams.
	run("ip", "addr", "add", "10.23.45.1/32", "dev", "lo")
	run("ip", "addr", "add", "10.23.45.9/24", "dev", "orbit-w11")
	run("ip", "route", "add", "default", "via", "10.23.45.2", "dev", "orbit-w11", "metric", "20")
	start := time.Now()
	for time.Since(start) < 15*time.Second {
		obs := query(b)
		if obs.Generation > direct.Generation {
			t.Logf("whole daemon address/default-route generation rebuild after %s", time.Since(start))
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if query(b).Generation <= direct.Generation {
		t.Fatal("whole daemon generation did not change")
	}
	// Ongoing small captures contend with real large bytes and a second peer pull.
	for i := 0; i < 8; i++ {
		write(fmt.Sprintf("small-%02d.txt", i), []byte(fmt.Sprintf("small version %d", i)))
		time.Sleep(100 * time.Millisecond)
	}
	block()
	waitRoute("relay")
	waitBytes(filepath.Join(rootB, "large.bin"), large)
	for i := 0; i < 8; i++ {
		waitBytes(filepath.Join(rootB, fmt.Sprintf("small-%02d.txt", i)), []byte(fmt.Sprintf("small version %d", i)))
	}
	unblock()
	restored := waitRoute("quic")
	if restored.Pin != initial.Pin || restored.Device != initial.Device {
		t.Fatal("roaming changed peer trust")
	}
	for _, n := range []*w05Node{a, b} {
		if _, err := os.Stat(filepath.Join(n.f.state, "w11-receipt-loss")); err != nil {
			t.Fatal("whole-daemon receipt boundary was not exercised", err)
		}
	}
	pa.stop(t, false)
	pb.stop(t, false)
	a.f.open()
	b.f.open()
	for _, name := range []string{"large.bin", "relay-start.txt", "small-00.txt", "small-07.txt"} {
		left, e := a.f.db.Heads(context.Background(), folder, name)
		if e != nil {
			t.Fatal(e)
		}
		right, e := b.f.db.Heads(context.Background(), folder, name)
		if e != nil {
			t.Fatal(e)
		}
		l, _ := json.Marshal(left)
		r, _ := json.Marshal(right)
		if !bytes.Equal(l, r) || len(left) != 1 || left[0].ID.Author != a.id.DeviceID || a.f.db.VerifyManifest(left[0].Manifest) != nil || b.f.db.VerifyManifest(right[0].Manifest) != nil {
			t.Fatal("changed version/head/author/hash", name)
		}
	}
	progress, err := a.f.db.PeerProgress(context.Background(), folder)
	if err != nil {
		t.Fatal(err)
	}
	hasReceipt := false
	for _, p := range progress {
		if p.Receipt {
			hasReceipt = true
		}
	}
	if !hasReceipt {
		t.Fatal("no durable receipt after daemon route recovery")
	}
	cfgA, _ := config.Load(a.f.state)
	cfgB, _ := config.Load(b.f.state)
	if cfgA.DeviceID != inv.Inviter || cfgB.DeviceID != fmt.Sprintf("%x", b.id.DeviceID) {
		t.Fatal("identity changed")
	}
	t.Logf("whole production daemons: relay→QUIC→address/default-route change during %d-byte transfer→relay→QUIC; exact two-way enrollment and heads/authors/manifests/bytes; finite 1 MiB/s budget; service port=%s", len(large), strconv.Quote(servicePort))
}

func startW11DaemonSampling(t *testing.T) func() {
	if os.Getenv("ORBIT_W11_NAMESPACE") != "isolated-marked-namespace" {
		return func() {}
	}
	root := os.Getenv("TMPDIR")
	if err := testkit.ValidateNetworkNamespace(root, os.Getenv("ORBIT_W11_PARENT_NETNS")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, fmt.Sprintf("daemon-metrics-%d.jsonl", os.Getpid()))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	done, stop := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		defer f.Close()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		encoder := json.NewEncoder(f)
		for {
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			fds, _ := os.ReadDir("/proc/self/fd")
			status, _ := os.ReadFile("/proc/self/status")
			var rss uint64
			for _, line := range strings.Split(string(status), "\n") {
				fields := strings.Fields(line)
				if len(fields) > 1 && fields[0] == "VmRSS:" {
					rss, _ = strconv.ParseUint(fields[1], 10, 64)
				}
			}
			var usage syscall.Rusage
			_ = syscall.Getrusage(syscall.RUSAGE_SELF, &usage)
			_ = encoder.Encode(map[string]any{"time_ns": time.Now().UnixNano(), "pid": os.Getpid(), "goroutines": runtime.NumGoroutine(), "fds": len(fds), "heap_bytes": mem.HeapAlloc, "rss_bytes": rss * 1024, "cpu_user_us": usage.Utime.Sec*1000000 + usage.Utime.Usec, "cpu_system_us": usage.Stime.Sec*1000000 + usage.Stime.Usec})
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()
	return func() { close(stop); <-done }
}
