package replication

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/network"
	"github.com/calebhabesh/file-sync/internal/protocol"
)

func TestWANW08ProductionMulticastKnownPeerTwoWaySync(t *testing.T) {
	f, _ := routedFixture(t)
	interfaces, err := network.SelectedInterfaces(nil)
	if err != nil || len(interfaces) == 0 {
		t.Fatal("selected real private interface required", err)
	}
	// Restrict actual multicast to one eligible interface rather than all host
	// networks. This test uses disposable identities and no interface mutations.
	var selected []network.LocalInterface
	for _, iface := range interfaces {
		for _, prefix := range iface.Prefixes {
			if prefix.Addr().Is4() && prefix.Addr().IsPrivate() {
				selected = []network.LocalInterface{iface}
				break
			}
		}
		if len(selected) > 0 {
			break
		}
	}
	if len(selected) == 0 {
		t.Fatal("IPv4 multicast interface unavailable")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := network.NewManager(network.ManagerOptions{}), network.NewManager(network.ManagerOptions{})
	defer a.Close()
	defer b.Close()
	ta := network.Target{Device: f.senderID.DeviceID, Pin: f.senderID.KeyPin, Purpose: network.PeerData}
	tb := network.Target{Device: f.receiverID.DeviceID, Pin: f.receiverID.KeyPin, Purpose: network.PeerData}
	if err = a.RegisterDirect(tb, nil); err != nil {
		t.Fatal(err)
	}
	if err = b.RegisterDirect(ta, nil); err != nil {
		t.Fatal(err)
	}
	// Daemon-style automatic listeners, actual bound ports, no configured IPs.
	la, err := network.OptionalDirectListener(network.DirectSettings{})
	if err != nil {
		t.Fatal(err)
	}
	defer la.Close()
	lb, err := network.OptionalDirectListener(network.DirectSettings{})
	if err != nil {
		t.Fatal(err)
	}
	defer lb.Close()
	done := make(chan error, 2)
	go func() { done <- NewServer(f.senderRepo, f.senderID).Serve(ctx, la) }()
	go func() { done <- NewServer(f.receiverRepo, f.receiverID).Serve(ctx, lb) }()
	defer func() { cancel(); <-done; <-done }()
	da, err := network.NewLANDiscovery(ctx, a, hex.EncodeToString(f.senderID.DeviceID[:]), hex.EncodeToString(f.senderID.KeyPin[:]), f.senderID.Certificate, selected, network.GatherCandidates(selected, la.Addr(), false), a.KnownTargets)
	if err != nil {
		t.Fatal(err)
	}
	defer da.Close()
	db, err := network.NewLANDiscovery(ctx, b, hex.EncodeToString(f.receiverID.DeviceID[:]), hex.EncodeToString(f.receiverID.KeyPin[:]), f.receiverID.Certificate, selected, network.GatherCandidates(selected, lb.Addr(), false), b.KnownTargets)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	client, err := NewRoutedClient(ctx, network.LogicalOrigin(ta), f.receiverID, f.senderID.Leaf, ta, b)
	if err != nil {
		t.Fatal(err)
	}
	f.client = client
	want := []byte("production signed multicast discovery without a manually configured peer address")
	if err = os.WriteFile(filepath.Join(f.senderRoot, "lan.txt"), want, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = f.senderWork.Scan(ctx, f.folder); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		_, err = f.newSyncer(TransferOptions{}).Sync(ctx)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatal("production LAN sync", err)
	}
	got, err := os.ReadFile(filepath.Join(f.receiverRoot, "lan.txt"))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("LAN bytes", err)
	}
	// Current direct sessions and capture remain independent of directory services:
	// these managers have no service client, resolver or public source at all.
	verifyInterruptedResume(t, f)
	reverse, err := NewRoutedClient(ctx, network.LogicalOrigin(tb), f.senderID, f.receiverID.Leaf, tb, a)
	if err != nil {
		t.Fatal(err)
	}
	wantReverse := []byte("independent reverse pull through discovery")
	if err = os.WriteFile(filepath.Join(f.receiverRoot, "reverse.txt"), wantReverse, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = f.receiverWork.Scan(ctx, f.folder); err != nil {
		t.Fatal(err)
	}
	membership, err := f.senderRepo.Membership(ctx, f.folder)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = NewSyncer(f.senderRepo, f.senderWork, reverse, f.senderID.DeviceID, f.receiverID.DeviceID, f.folder, membership, TransferOptions{}).Sync(ctx); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(filepath.Join(f.senderRoot, "reverse.txt"))
	if err != nil || !bytes.Equal(got, wantReverse) {
		t.Fatal("reverse bytes", err)
	}
	for _, path := range []string{"lan.txt", "reverse.txt", "resume.bin"} {
		ah, e := f.senderRepo.Heads(ctx, f.folder, path)
		if e != nil {
			t.Fatal(e)
		}
		bh, e := f.receiverRepo.Heads(ctx, f.folder, path)
		if e != nil {
			t.Fatal(e)
		}
		left, _ := json.Marshal(ah)
		right, _ := json.Marshal(bh)
		if !bytes.Equal(left, right) || len(ah) != 1 || f.senderRepo.VerifyManifest(ah[0].Manifest) != nil || f.receiverRepo.VerifyManifest(bh[0].Manifest) != nil {
			t.Fatal("head/version/hash oracle", path)
		}
	}
	if a.Observe(tb).Route != "direct" || b.Observe(ta).Route != "direct" {
		t.Fatal("route selection", a.Observe(tb), b.Observe(ta))
	}
	// Report actual nonloopback IPv6 availability independently of IPv4 discovery.
	nativeV6 := false
	for _, prefix := range selected[0].Prefixes {
		if prefix.Addr().Is6() && prefix.Addr().IsPrivate() {
			listener, e := net.Listen("tcp6", net.JoinHostPort(prefix.Addr().String(), "0"))
			if e == nil {
				nativeV6 = true
				listener.Close()
			}
		}
	}
	t.Logf("real nonloopback local IPv6 listener availability=%t; public IPv6 internet reachability remains unexecuted", nativeV6)
}

func TestWANW08NativeLocalIPv6PinnedTransfer(t *testing.T) {
	f, _ := routedFixture(t)
	interfaces, err := network.SelectedInterfaces(nil)
	if err != nil {
		t.Fatal(err)
	}
	var address string
	var ifaceName string
	for _, iface := range interfaces {
		for _, prefix := range iface.Prefixes {
			if prefix.Addr().Is6() && prefix.Addr().IsPrivate() {
				address = prefix.Addr().String()
				ifaceName = iface.Name
				break
			}
		}
		if address != "" {
			break
		}
	}
	if address == "" {
		t.Skip("real nonloopback local IPv6 unavailable; no public IPv6 claim")
	}
	listener, err := net.Listen("tcp6", net.JoinHostPort(address, "0"))
	if err != nil {
		t.Fatal("actual IPv6 bind", err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- NewServer(f.senderRepo, f.senderID).Serve(ctx, listener) }()
	defer func() { cancel(); <-done }()
	manager := network.NewManager(network.ManagerOptions{})
	defer manager.Close()
	target := network.Target{Device: f.senderID.DeviceID, Pin: f.senderID.KeyPin, Purpose: network.PeerData}
	if err = manager.RegisterDirect(target, nil); err != nil {
		t.Fatal(err)
	}
	if err = manager.SetCandidates(target, []protocol.NetworkCandidate{{Transport: "tcp", Address: listener.Addr().String(), Scope: "lan", Interface: ifaceName}}, 1, time.Now().Add(time.Minute), false); err != nil {
		t.Fatal(err)
	}
	client, err := NewRoutedClient(ctx, network.LogicalOrigin(target), f.receiverID, f.senderID.Leaf, target, manager)
	if err != nil {
		t.Fatal(err)
	}
	f.client = client
	want := []byte("actual nonloopback IPv6 pinned mutual TLS transfer")
	if err = os.WriteFile(filepath.Join(f.senderRoot, "ipv6.txt"), want, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = f.senderWork.Scan(ctx, f.folder); err != nil {
		t.Fatal(err)
	}
	if _, err = f.newSyncer(TransferOptions{}).Sync(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(f.receiverRoot, "ipv6.txt"))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal(err)
	}
	heads, err := f.receiverRepo.Heads(ctx, f.folder, "ipv6.txt")
	if err != nil || len(heads) != 1 || heads[0].ID.Author != f.senderID.DeviceID || f.receiverRepo.VerifyManifest(heads[0].Manifest) != nil || manager.Observe(target).Route != "direct" {
		t.Fatal("IPv6 identity/version/hash/route", err)
	}
	t.Log("Native nonloopback local ULA IPv6 TCP/mTLS/HTTP/verified engine bytes executed; global/public IPv6 routing remains unexecuted.")
}
