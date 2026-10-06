package replication

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/network"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/testkit"
)

func TestWANW11NativeQUICToTCPChunkAndReceiptRecovery(t *testing.T) {
	f, _, manager, senderEndpoint, _ := quicFixture(t)
	ctx := context.Background()
	for _, root := range []string{f.senderRepo.StateDir(), f.receiverRepo.StateDir()} {
		if err := os.WriteFile(filepath.Join(root, testkit.Marker), []byte("W11 disposable route-switch fixture\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	host, iface := quicHost(t)
	listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(f.senderRepo, f.senderID).HTTPServer()
	done := make(chan struct{})
	go func() { defer close(done); _ = server.ServeTLS(listener, "", "") }()
	defer func() { _ = server.Close(); <-done }()

	target := network.Target{Device: f.senderID.DeviceID, Pin: f.senderID.KeyPin, Purpose: network.PeerData}
	if err = manager.SetCandidates(target, []protocol.NetworkCandidate{
		{Transport: "udp", Address: senderEndpoint.LocalAddr().String(), Scope: "lan", Interface: iface},
		{Transport: "tcp", Address: listener.Addr().String(), Scope: "lan", Interface: iface},
	}, 2, time.Now().Add(time.Minute), false); err != nil {
		t.Fatal(err)
	}
	want := append(bytes.Repeat([]byte("A"), int(history.ChunkSize)), bytes.Repeat([]byte("B"), int(history.ChunkSize))...)
	path := "roaming.bin"
	if err = os.WriteFile(filepath.Join(f.senderRoot, path), want, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = f.senderWork.Scan(ctx, f.folder); err != nil {
		t.Fatal(err)
	}
	before, err := f.senderRepo.Heads(ctx, f.folder, path)
	if err != nil || len(before) != 1 {
		t.Fatal(before, err)
	}
	interrupted := false
	_, err = f.newSyncer(TransferOptions{Workers: 1, Hook: func(name string) error {
		if name == HookChunkVerified && !interrupted {
			interrupted = true
			return errors.New("marked chunk-boundary route loss")
		}
		return nil
	}}).Sync(ctx)
	if err == nil || !interrupted {
		t.Fatal("interruption missing", err)
	}
	if manager.Observe(network.Target{Device: f.senderID.DeviceID, Pin: f.senderID.KeyPin, Purpose: network.PeerData}).Route != "quic" {
		t.Fatal("initial native QUIC missing")
	}
	ready, err := f.receiverRepo.ContentReady(ctx, before[0].ID)
	if err != nil || ready {
		t.Fatal("premature ready", ready, err)
	}
	progress, err := f.senderRepo.PeerProgress(ctx, f.folder)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range progress {
		if p.Receipt {
			t.Fatal("premature receipt")
		}
	}
	manager.NetworkChanged()
	if err = manager.SetCandidates(target, []protocol.NetworkCandidate{{Transport: "tcp", Address: listener.Addr().String(), Scope: "lan", Interface: iface}}, 3, time.Now().Add(time.Minute), false); err != nil {
		t.Fatal(err)
	}
	f.client, err = NewRoutedClient(ctx, network.LogicalOrigin(target), f.receiverID, f.senderID.Leaf, target, manager)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	result, err := f.newSyncer(TransferOptions{Workers: 1, Hook: func(name string) error {
		if name == HookAfterReceipt {
			return errors.New("marked response loss after stored receipt")
		}
		return nil
	}}).Sync(ctx)
	if err == nil || result.ChunksReused != 1 {
		t.Fatal("TCP resume/receipt loss", result, err)
	}
	if manager.Observe(target).Route != "direct" {
		t.Fatal("TCP fallback missing", manager.Observe(target))
	}
	t.Logf("native QUIC to TCP after chunk-boundary interruption: %s; reused=%d fetched=%d", time.Since(started), result.ChunksReused, result.ChunksFetched)
	manager.NetworkChanged()
	if err = manager.SetCandidates(target, []protocol.NetworkCandidate{{Transport: "udp", Address: senderEndpoint.LocalAddr().String(), Scope: "lan", Interface: iface}}, 4, time.Now().Add(time.Minute), false); err != nil {
		t.Fatal(err)
	}
	f.client, err = NewRoutedClient(ctx, network.LogicalOrigin(target), f.receiverID, f.senderID.Leaf, target, manager)
	if err != nil {
		t.Fatal(err)
	}
	started = time.Now()
	if _, err = f.newSyncer(TransferOptions{Workers: 1}).Sync(ctx); err != nil {
		t.Fatal("receipt replay through QUIC", err)
	}
	if manager.Observe(target).Route != "quic" {
		t.Fatal("QUIC restored", manager.Observe(target))
	}
	after, err := f.receiverRepo.Heads(ctx, f.folder, path)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(before)
	current, _ := json.Marshal(after)
	if !bytes.Equal(original, current) || f.senderRepo.VerifyManifest(before[0].Manifest) != nil || f.receiverRepo.VerifyManifest(after[0].Manifest) != nil {
		t.Fatal("changed heads/author/hash")
	}
	got, err := os.ReadFile(filepath.Join(f.receiverRoot, path))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("working bytes", err)
	}
	progress, err = f.senderRepo.PeerProgress(ctx, f.folder)
	if err != nil || len(progress) != 1 || !progress[0].Receipt {
		t.Fatal("stored receipt lost", progress, err)
	}
	t.Logf("native TCP to QUIC receipt recovery: %s; original version/author, heads, manifests and working bytes preserved", time.Since(started))
}
