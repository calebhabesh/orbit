package replication

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
	"github.com/calebhabesh/orbit/internal/workspace"
	"github.com/calebhabesh/orbit/model"
)

func TestWANW15DurableBoundariesMatchIndependentModel(t *testing.T) {
	for _, boundary := range []string{HookChunkVerified, HookBeforeReady, HookAfterReady, HookBeforeReceipt, HookAfterReceipt} {
		t.Run(boundary, func(t *testing.T) {
			f, _, manager, _, _ := quicFixture(t)
			ctx := context.Background()
			want := append(bytes.Repeat([]byte("A"), int(history.ChunkSize)), bytes.Repeat([]byte("B"), int(history.ChunkSize))...)
			if e := os.WriteFile(filepath.Join(f.senderRoot, "protected.bin"), want, 0600); e != nil {
				t.Fatal(e)
			}
			if _, e := f.senderWork.Scan(ctx, f.folder); e != nil {
				t.Fatal(e)
			}
			original, e := f.senderRepo.Heads(ctx, f.folder, "protected.bin")
			if e != nil || len(original) != 1 {
				t.Fatal(original, e)
			}
			oracle := model.NewWANTransfer("version-1", "sender", "expected-pin", []string{"chunk-1", "chunk-2"})
			oracle.AcceptMetadata(true)
			oracle.Switch("quic", "expected-pin")
			chunks := 0
			trace := []string{"approved metadata", "authenticated quic"}
			reached := false
			hook := func(name string) error {
				trace = append(trace, name)
				switch name {
				case HookChunkVerified:
					chunks++
					oracle.Chunk(fmt.Sprintf("chunk-%d", chunks), true)
				case HookAfterReady:
					if !oracle.CommitReady(true) {
						return errors.New("model refused readiness")
					}
				case HookAfterReceipt:
					if !oracle.SendReceipt(true) {
						return errors.New("model refused receipt")
					}
				}
				if name == boundary {
					reached = true
					return errors.New("W15 marked boundary loss")
				}
				return nil
			}
			t.Cleanup(func() {
				if t.Failed() {
					data, _ := json.Marshal(trace)
					t.Log("replay trace", string(data))
				}
			})
			if _, e = f.newSyncer(TransferOptions{Workers: 1, Hook: hook}).Sync(ctx); e == nil || !reached {
				t.Fatal("boundary not reached", e)
			}
			ready, e := f.receiverRepo.ContentReady(ctx, original[0].ID)
			if e != nil || ready != oracle.Ready {
				t.Fatal("model readiness mismatch", ready, oracle.Ready, e)
			}
			progress, e := f.senderRepo.PeerProgress(ctx, f.folder)
			if e != nil {
				t.Fatal(e)
			}
			receipt := false
			for _, p := range progress {
				receipt = receipt || p.Receipt
			}
			if receipt != oracle.Receipt {
				t.Fatal("model receipt mismatch", receipt, oracle.Receipt)
			}
			w15GC(t, f)
			trace = append(trace, "aggressive GC while transfer/ready state remains protected")
			manager.NetworkChanged()
			oracle.Restart()
			trace = append(trace, "generation retired")
			host, iface := quicHost(t)
			listener, e := net.Listen("tcp", net.JoinHostPort(host, "0"))
			if e != nil {
				t.Fatal(e)
			}
			server := NewServer(f.senderRepo, f.senderID).HTTPServer()
			done := make(chan struct{})
			go func() { defer close(done); _ = server.ServeTLS(listener, "", "") }()
			defer func() { _ = server.Close(); <-done }()
			target := network.Target{Device: f.senderID.DeviceID, Pin: f.senderID.KeyPin, Purpose: network.PeerData}
			if e = manager.SetCandidates(target, []protocol.NetworkCandidate{{Transport: "tcp", Address: listener.Addr().String(), Scope: "lan", Interface: iface}}, 2, time.Now().Add(time.Minute), false); e != nil {
				t.Fatal(e)
			}
			f.client, e = NewRoutedClient(ctx, network.LogicalOrigin(target), f.receiverID, f.senderID.Leaf, target, manager)
			if e != nil {
				t.Fatal(e)
			}
			oracle.Switch("tcp", "expected-pin")
			// Retry is the same immutable version with verified chunks retained.
			result, e := f.newSyncer(TransferOptions{Workers: 1}).Sync(ctx)
			if e != nil {
				t.Fatal(e)
			}
			if manager.Observe(target).Route != "direct" {
				t.Fatal("TCP switch not exercised")
			}
			if boundary == HookChunkVerified && result.ChunksReused != 1 {
				t.Fatal("lost durable chunk", result)
			}
			after, e := f.receiverRepo.Heads(ctx, f.folder, "protected.bin")
			left, _ := json.Marshal(original)
			right, _ := json.Marshal(after)
			if e != nil || !bytes.Equal(left, right) || f.receiverRepo.VerifyManifest(after[0].Manifest) != nil {
				t.Fatal("changed immutable head/hash", e)
			}
			got, e := os.ReadFile(filepath.Join(f.receiverRoot, "protected.bin"))
			if e != nil || !bytes.Equal(got, want) {
				t.Fatal("wrong protected bytes", e)
			}
			progress, e = f.senderRepo.PeerProgress(ctx, f.folder)
			if e != nil || len(progress) != 1 || !progress[0].Receipt {
				t.Fatal("lost endpoint receipt", e, progress)
			}
			t.Logf("trace=%v; reused=%d fetched=%d; exact heads/author/manifests/working bytes/receipt", trace, result.ChunksReused, result.ChunksFetched)
		})
	}
}

// Switch the authenticated transport without changing the expected device/pin.
func w15SwitchTCP(t *testing.T, f *syncFixture, manager *network.ConnectionManager) {
	t.Helper()
	host, iface := quicHost(t)
	l, e := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if e != nil {
		t.Fatal(e)
	}
	server := NewServer(f.senderRepo, f.senderID).HTTPServer()
	done := make(chan struct{})
	go func() { defer close(done); _ = server.ServeTLS(l, "", "") }()
	t.Cleanup(func() { _ = server.Close(); <-done })
	manager.NetworkChanged()
	target := network.Target{Device: f.senderID.DeviceID, Pin: f.senderID.KeyPin, Purpose: network.PeerData}
	if e = manager.SetCandidates(target, []protocol.NetworkCandidate{{Transport: "tcp", Address: l.Addr().String(), Scope: "lan", Interface: iface}}, 2, time.Now().Add(time.Minute), false); e != nil {
		t.Fatal(e)
	}
	f.client, e = NewRoutedClient(f.ctx, network.LogicalOrigin(target), f.receiverID, f.senderID.Leaf, target, manager)
	if e != nil {
		t.Fatal(e)
	}
}

func w15GC(t *testing.T, f *syncFixture) {
	t.Helper()
	root := filepath.Dir(f.receiverRepo.StateDir())
	if e := os.WriteFile(filepath.Join(root, testkit.Marker), []byte("W15 disposable GC fixture\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := testkit.ValidateDestructiveTarget(root, f.receiverRepo.StateDir()); e != nil {
		t.Fatal(e)
	}
	if _, e := f.receiverRepo.RunGC(f.ctx, f.folder, &repository.RetentionPolicy{}, time.Now().Add(365*24*time.Hour)); e != nil {
		t.Fatal(e)
	}
}

func TestWANW15RouteSwitchPublicationGCConflictAndRetirement(t *testing.T) {
	t.Run("publication", func(t *testing.T) {
		f, _, manager, _, _ := quicFixture(t)
		want := bytes.Repeat([]byte("protected staged bytes\n"), 100000)
		if e := os.WriteFile(filepath.Join(f.senderRoot, "staged.bin"), want, 0600); e != nil {
			t.Fatal(e)
		}
		if _, e := f.senderWork.Scan(f.ctx, f.folder); e != nil {
			t.Fatal(e)
		}
		interrupted := false
		f.receiverWork = workspace.New(f.receiverRepo, workspace.Options{FaultHook: func(name string) error {
			if name == workspace.HookFilesystemTransition {
				interrupted = true
				return errors.New("marked post-rename publication interruption")
			}
			return nil
		}})
		if _, e := f.newSyncer(TransferOptions{}).Sync(f.ctx); e == nil || !interrupted {
			t.Fatal("publication hook not reached", e)
		}
		w15GC(t, f)
		w15SwitchTCP(t, f, manager)
		f.receiverWork = workspace.New(f.receiverRepo, workspace.Options{})
		if e := f.receiverWork.Recover(f.ctx, f.folder); e != nil {
			t.Fatal(e)
		}
		if _, e := f.newSyncer(TransferOptions{}).Sync(f.ctx); e != nil {
			t.Fatal(e)
		}
		got, e := os.ReadFile(filepath.Join(f.receiverRoot, "staged.bin"))
		if e != nil || !bytes.Equal(got, want) {
			t.Fatal("publication lost protected bytes", e)
		}
		a, e := f.senderRepo.Heads(f.ctx, f.folder, "staged.bin")
		if e != nil {
			t.Fatal(e)
		}
		b, e := f.receiverRepo.Heads(f.ctx, f.folder, "staged.bin")
		if e != nil {
			t.Fatal(e)
		}
		left, _ := json.Marshal(a)
		right, _ := json.Marshal(b)
		if !bytes.Equal(left, right) || len(b) != 1 || f.receiverRepo.VerifyManifest(b[0].Manifest) != nil {
			t.Fatal("publication changed version/hash")
		}
		scan, e := f.receiverWork.Scan(f.ctx, f.folder)
		if e != nil || len(scan.Captured) != 0 {
			t.Fatal("publication recovery fabricated capture", e)
		}
		t.Log("QUIC publication interrupted after filesystem transition → protected aggressive GC → TCP recovery; exact bytes/heads/authors/hash and no scan feedback")
	})
	t.Run("conflict and retirement", func(t *testing.T) {
		f, senderManager, receiverManager, _, _ := quicFixture(t)
		for _, n := range []struct {
			root string
			work *workspace.Workspace
			data string
		}{{f.senderRoot, f.senderWork, "offline sender edit"}, {f.receiverRoot, f.receiverWork, "offline receiver edit"}} {
			if e := os.WriteFile(filepath.Join(n.root, "conflict.txt"), []byte(n.data), 0600); e != nil {
				t.Fatal(e)
			}
			if _, e := n.work.Scan(f.ctx, f.folder); e != nil {
				t.Fatal(e)
			}
		}
		if _, e := f.newSyncer(TransferOptions{}).Sync(f.ctx); e != nil {
			t.Fatal(e)
		}
		w15GC(t, f)
		w15SwitchTCP(t, f, receiverManager)
		if _, e := f.newSyncer(TransferOptions{}).Sync(f.ctx); e != nil {
			t.Fatal(e)
		}
		target := network.Target{Device: f.receiverID.DeviceID, Pin: f.receiverID.KeyPin, Purpose: network.PeerData}
		reverse, e := NewRoutedClient(f.ctx, network.LogicalOrigin(target), f.senderID, f.receiverID.Leaf, target, senderManager)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = NewSyncer(f.senderRepo, f.senderWork, reverse, f.senderID.DeviceID, f.receiverID.DeviceID, f.folder, f.approved, TransferOptions{}).Sync(f.ctx); e != nil {
			t.Fatal(e)
		}
		a, e := f.senderRepo.Heads(f.ctx, f.folder, "conflict.txt")
		if e != nil {
			t.Fatal(e)
		}
		b, e := f.receiverRepo.Heads(f.ctx, f.folder, "conflict.txt")
		if e != nil {
			t.Fatal(e)
		}
		left, _ := json.Marshal(a)
		right, _ := json.Marshal(b)
		if !bytes.Equal(left, right) || len(a) != 2 {
			t.Fatal("lost conflicting heads")
		}
		for _, h := range b {
			if f.senderRepo.VerifyManifest(h.Manifest) != nil || f.receiverRepo.VerifyManifest(h.Manifest) != nil {
				t.Fatal("GC lost conflict bytes")
			}
		}
		for _, n := range []struct {
			root string
			data string
		}{{f.senderRoot, "offline sender edit"}, {f.receiverRoot, "offline receiver edit"}} {
			got, e := os.ReadFile(filepath.Join(n.root, "conflict.txt"))
			if e != nil || string(got) != n.data {
				t.Fatal("overwrote conflicting working candidate", e)
			}
		}
		versions, e := f.senderRepo.VersionsByAuthor(f.ctx, f.folder, f.receiverID.DeviceID)
		if e != nil {
			t.Fatal(e)
		}
		snapshot := protocol.RetirementSnapshot{Folder: f.folder, ConfigurationRev: 1, RetiredDevice: f.receiverID.DeviceID, AcceptedByRetiree: versions}
		digest, e := protocol.RetirementSnapshotDigest(snapshot)
		if e != nil {
			t.Fatal(e)
		}
		next := protocol.Membership{Folder: f.folder, Revision: 2, PriorDigest: f.approved.Digest, Active: []protocol.ActiveMember{{Device: f.senderID.DeviceID, KeyPin: f.senderID.KeyPin}}, Retired: []protocol.RetiredMember{{Device: f.receiverID.DeviceID, RetiredAt: 2, SnapshotDigest: digest}}}
		if _, e = f.senderRepo.ApproveMembership(f.ctx, next, snapshot); e != nil {
			t.Fatal(e)
		}
		// Reusing a previously authenticated TCP pool must recheck current folder authority.
		_, e = f.newSyncer(TransferOptions{}).Sync(f.ctx)
		var refused *WireError
		if !errors.As(e, &refused) || refused.Body.Code != "UNAUTHORIZED" {
			t.Fatal("retired device did not receive exact authorization refusal", e)
		}
		senderManager.NetworkChanged()
		for _, h := range a {
			if f.senderRepo.VerifyManifest(h.Manifest) != nil {
				t.Fatal("retirement destroyed protected content")
			}
		}
		t.Log("offline conflicts converge through QUIC/TCP with two exact heads and both working candidates; GC preserves bytes; cached authenticated TCP denies retired device")
	})
}
