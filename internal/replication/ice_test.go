package replication

import (
	"bytes"
	"context"

	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/rendezvous"
	"github.com/calebhabesh/orbit/internal/testkit"
	"github.com/pion/stun/v4"
	"github.com/pion/transport/v5"
)

func TestWANW10IsolatedICEPeerSyncAndFallback(t *testing.T) {
	if os.Getenv("ORBIT_W10_PUBLIC_FIXTURE") != "isolated-marked-namespace" {
		t.Skip("requires marked disposable user/network namespace runner")
	}
	for _, fallback := range []bool{false, true} {
		t.Run(map[bool]string{false: "ICE", true: "failed ICE relay"}[fallback], func(t *testing.T) { verifyICEPeerSync(t, fallback) })
	}
}

type icePeerFixture struct {
	service  *rendezvous.Service
	f        *syncFixture
	runtimes []*network.RelayRuntime
	managers []*network.ConnectionManager
	peers    []network.Target
}

func verifyICEPeerSync(t *testing.T, fallback bool, nets ...transport.Net) *icePeerFixture {
	f := newSyncFixture(t)
	for _, db := range []string{f.senderRepo.StateDir(), f.receiverRepo.StateDir()} {
		if e := os.WriteFile(filepath.Join(db, testkit.Marker), []byte("W10 disposable namespace fixture\n"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	service, selection, origin, roots, _ := productionRelayService(t, func() []string {
		if len(nets) > 0 {
			return []string{"11.1.1.1:3478"}
		}
		return nil
	}()...)
	digest, _ := selection.Digest()
	raw, _ := hex.DecodeString(digest)
	var profile history.Digest
	copy(profile[:], raw)
	ta := network.Target{Device: f.receiverID.DeviceID, Pin: f.receiverID.KeyPin, Purpose: network.PeerData, Profile: profile}
	tb := network.Target{Device: f.senderID.DeviceID, Pin: f.senderID.KeyPin, Purpose: network.PeerData, Profile: profile}
	managers := []*network.ConnectionManager{network.NewManager(network.ManagerOptions{}), network.NewManager(network.ManagerOptions{})}
	ids := []Identity{f.senderID, f.receiverID}
	peers := []network.Target{ta, tb}
	var runtimes []*network.RelayRuntime
	var clients []*network.ServiceClient
	var serverWG sync.WaitGroup
	for n := range 2 {
		m := managers[n]
		t.Cleanup(func() { _ = m.Close() })
		id := ids[n]
		repo := f.senderRepo
		if n == 1 {
			repo = f.receiverRepo
		}
		c, e := network.NewServiceClient(network.ServiceClientOptions{Selection: selection, Origin: origin, Device: hex.EncodeToString(id.DeviceID[:]), Certificate: id.Certificate, Roots: roots})
		if e != nil {
			t.Fatal(e)
		}
		clients = append(clients, c)
		t.Cleanup(func() { _ = c.Close() })
		options := &network.ICEOptions{TLS: id.ServerTLSConfig(), Peer: NewServer(repo, id), Interfaces: []string{"orbit-w10"}}
		if len(nets) > 0 {
			options.Net = nets[n]
			options.Interfaces = nil
		} else if fallback {
			options.Interfaces = []string{"no-such-interface"}
		}
		r, e := network.NewRelayRuntime(f.ctx, c, m, 1, digest, options)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { _ = r.Close() })
		runtimes = append(runtimes, r)
		if e = r.Register(peers[n]); e != nil {
			t.Fatal(e)
		}
		server := NewServer(repo, id).HTTPServer()
		l, _ := m.IncomingListener(network.PeerData)
		serverWG.Go(func() { _ = server.ServeTLS(l, "", "") })
		t.Cleanup(func() { _ = server.Close() })
	}
	t.Cleanup(func() {
		for _, m := range managers {
			l, _ := m.IncomingListener(network.PeerData)
			_ = l.Close()
		}
		serverWG.Wait()
	})
	ctx, cancel := context.WithTimeout(f.ctx, 15*time.Second)
	defer cancel()
	for _, r := range runtimes {
		if e := r.WaitReady(ctx); e != nil {
			t.Fatal(e)
		}
	}

	if !fallback {
		if _, _, e := runtimes[1].ICE(f.ctx, tb); e != nil {
			t.Fatal("minimal signed ICE establishment", e)
		}
	}
	_ = f.listener.Close()
	var e error
	f.client, e = NewRoutedClient(f.ctx, network.LogicalOrigin(tb), f.receiverID, f.senderID.Leaf, tb, managers[1])
	if e != nil {
		t.Fatal(e)
	}
	verifyInterruptedResume(t, f)
	if !fallback {
		inventory := InventoryRequest{ProtocolVersion: ProtocolVersion, DeviceID: hex.EncodeToString(f.receiverID.DeviceID[:]), FolderID: strings.Repeat("f", 64), Revision: "1", MembershipDigest: hex.EncodeToString(f.approved.Digest[:]), PageSize: "1"}
		_, err := f.client.Inventory(f.ctx, inventory)
		var wire *WireError
		if !errors.As(err, &wire) || wire.Body.Code != "UNAUTHORIZED" {
			t.Fatal("ICE folder authority", err)
		}
		for _, path := range []string{"/control/v1/status", "/enrollment/v3/challenge"} {
			req, _ := http.NewRequest("POST", f.client.baseURL+path, strings.NewReader("{}"))
			res, err := f.client.http.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()
			if res.StatusCode != 404 {
				t.Fatal("ICE handler isolation", res.StatusCode)
			}
		}
		req, _ := http.NewRequest("POST", f.client.baseURL+"/peer/v1/hello", strings.NewReader(strings.Repeat("x", int(MaxMetadataBytes)+1)))
		res, err := f.client.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
		if res.StatusCode != 413 {
			t.Fatal("ICE body bound", res.StatusCode)
		}
		wrong := tb
		wrong.Pin = history.Digest{1}
		if err = managers[1].RegisterDirect(wrong, nil); err == nil {
			t.Fatal("ICE pin substitution")
		}
	}
	reverse, e := NewRoutedClient(f.ctx, network.LogicalOrigin(ta), f.senderID, f.receiverID.Leaf, ta, managers[0])
	if e != nil {
		t.Fatal(e)
	}
	want := bytes.Repeat([]byte("actual authenticated ICE reverse peer sync\n"), 35000)
	if e = os.WriteFile(filepath.Join(f.receiverRoot, "ice-reverse.txt"), want, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = f.receiverWork.Scan(f.ctx, f.folder); e != nil {
		t.Fatal(e)
	}
	reverseSync := NewSyncer(f.senderRepo, f.senderWork, reverse, f.senderID.DeviceID, f.receiverID.DeviceID, f.folder, f.approved, TransferOptions{})
	deadline := time.Now().Add(12 * time.Second)
	for {
		_, e = reverseSync.Sync(f.ctx)
		if e == nil {
			break
		}
		var serviceFailure *network.ServiceError
		if !errors.As(e, &serviceFailure) || serviceFailure.Code != "QUOTA_EXCEEDED" || time.Now().After(deadline) {
			t.Fatal(e)
		}
		t.Log("reviewed metadata quota refused fresh reverse relay; bounded five-second quiet refill")
		time.Sleep(5 * time.Second)
	}
	got, e := os.ReadFile(filepath.Join(f.senderRoot, "ice-reverse.txt"))
	if e != nil || !bytes.Equal(got, want) {
		t.Fatal("reverse bytes", e)
	}
	route := "quic"
	if fallback {
		route = "relay"
	}
	for n, m := range managers {
		if m.Observe(peers[n]).Route != route {
			t.Fatal("selected route", m.Observe(peers[n]))
		}
		if fallback && m.ICEFailure(peers[n]) == nil {
			t.Fatal("fallback lost typed ICE reason")
		}
		if !fallback && m.ICEFailure(peers[n]) != nil {
			t.Fatal("successful ICE retained stale failure")
		}
	}
	for _, path := range []string{"resume.bin", "ice-reverse.txt"} {
		a, e := f.senderRepo.Heads(f.ctx, f.folder, path)
		if e != nil {
			t.Fatal(e)
		}
		b, e := f.receiverRepo.Heads(f.ctx, f.folder, path)
		if e != nil {
			t.Fatal(e)
		}
		aa, _ := json.Marshal(a)
		bb, _ := json.Marshal(b)
		if !bytes.Equal(aa, bb) || len(a) != 1 || f.senderRepo.VerifyManifest(a[0].Manifest) != nil || f.receiverRepo.VerifyManifest(b[0].Manifest) != nil {
			t.Fatal("heads hash author", path)
		}
	}
	if !fallback {
		for n, r := range runtimes {
			if _, _, e = r.ICE(f.ctx, peers[n]); e != nil {
				t.Fatal("cached pair", e)
			}
		}
	}
	if fallback && len(nets) == 0 {
		// Give the service its documented one-token/second refill before
		// independently testing both roles outside the transfer journey.
		time.Sleep(2 * time.Second)
		// Neither peer has a selected interface. In either controlling role, fail
		// gathering before spending the coordination budget waiting for an offer.
		for n, r := range runtimes {
			deadline := time.Now().Add(12 * time.Second)
			for {
				started := time.Now()
				_, _, err := r.ICE(f.ctx, peers[n])
				var refusal *network.ServiceError
				if errors.As(err, &refusal) && refusal.Code == "QUOTA_EXCEEDED" && time.Now().Before(deadline) {
					t.Log("existing transfer exhausted metadata quota; bounded five-second quiet refill before empty-gather oracle")
					time.Sleep(5 * time.Second)
					continue
				}
				if !errors.Is(err, network.ErrICEGather) {
					t.Fatal("empty local gather did not fail before coordination", err)
				}
				if time.Since(started) > 2*time.Second {
					t.Fatal("empty local gather waited for remote offer")
				}
				break
			}
		}
	}
	t.Log("actual Pion path; selected", route, "two-way verified heads/hashes and interrupted chunk reuse")
	if len(nets) == 0 {
		liveFDs, _ := os.ReadDir("/proc/self/fd")
		liveGoroutines := runtime.NumGoroutine()
		for _, r := range runtimes {
			_ = r.Close()
		}
		for _, m := range managers {
			_ = m.Close()
		}
		for _, c := range clients {
			_ = c.Close()
		}
		closedFDs, _ := os.ReadDir("/proc/self/fd")
		t.Logf("native lifecycle samples: FDs live/closed=%d/%d goroutines live/closed=%d/%d; persistent repositories/service fixture remain open", len(liveFDs), len(closedFDs), liveGoroutines, runtime.NumGoroutine())
	}
	return &icePeerFixture{service, f, runtimes, managers, peers}
}

func TestWANW10IsolatedSTUNIPv6(t *testing.T) {
	if os.Getenv("ORBIT_W10_PUBLIC_FIXTURE") != "isolated-marked-namespace" {
		t.Skip("requires marked disposable user/network namespace runner")
	}
	ip := net.ParseIP("2606:4700:4700::1111")
	socket, e := net.ListenUDP("udp6", &net.UDPAddr{IP: ip})
	if e != nil {
		t.Fatal(e)
	}
	server, e := network.NewSTUNServer(socket)
	if e != nil {
		t.Fatal(e)
	}
	defer server.Close()
	client, e := net.ListenUDP("udp6", &net.UDPAddr{IP: ip})
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	request, e := stun.Build(stun.TransactionID, stun.BindingRequest)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = client.WriteTo(request.Raw, server.LocalAddr()); e != nil {
		t.Fatal(e)
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	buffer := make([]byte, 128)
	n, _, e := client.ReadFrom(buffer)
	if e != nil {
		t.Fatal(e)
	}
	message := &stun.Message{Raw: buffer[:n]}
	if e = message.Decode(); e != nil {
		t.Fatal(e)
	}
	var mapped stun.XORMappedAddress
	if e = mapped.GetFrom(message); e != nil {
		t.Fatal(e)
	}
	if !mapped.IP.Equal(ip) || mapped.Port != client.LocalAddr().(*net.UDPAddr).Port || message.TransactionID != request.TransactionID || n != 44 || len(request.Raw) != 20 {
		t.Fatal("IPv6 mapping/bound")
	}
	t.Log("actual UDP6 STUN codec in isolated namespace: 44/20-byte response/request; simulated public address, no native internet claim")
}
