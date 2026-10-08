package replication

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/testkit"
)

func routedFixture(t *testing.T) (*syncFixture, *network.ConnectionManager) {
	t.Helper()
	f := newSyncFixture(t)
	// The interruption hook is non-destructive; explicitly mark the synthetic
	// fixture state nevertheless. No external path or process is a fault target.
	for _, db := range []string{f.senderRepo.StateDir(), f.receiverRepo.StateDir()} {
		if err := os.WriteFile(filepath.Join(db, testkit.Marker), []byte("W02 disposable synthetic state\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := network.NewManager(network.ManagerOptions{})
	t.Cleanup(func() { m.Close() })
	target := network.Target{Device: f.senderID.DeviceID, Pin: f.senderID.KeyPin, Purpose: network.PeerData}
	endpoint := "https://" + f.listener.Addr().String()
	if err := m.SetManual(target, endpoint); err != nil {
		t.Fatal(err)
	}
	client, err := NewRoutedClient(context.Background(), endpoint, f.receiverID, f.senderID.Leaf, target, m)
	if err != nil {
		t.Fatal(err)
	}
	f.client = client
	return f, m
}
func TestWANW02TwoWaySyncInterruptedChunkReuse(t *testing.T) {
	f, m := routedFixture(t)
	verifyInterruptedResume(t, f)
	ctx := context.Background()
	before, err := f.senderRepo.Heads(ctx, f.folder, "resume.bin")
	if err != nil {
		t.Fatal(err)
	}
	after, err := f.receiverRepo.Heads(ctx, f.folder, "resume.bin")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	if !bytes.Equal(a, b) {
		t.Fatal("normalized version/head differs after retry")
	}
	m.AdvanceGeneration()
	target := network.Target{Device: f.senderID.DeviceID, Pin: f.senderID.KeyPin, Purpose: network.PeerData}
	client, err := NewRoutedClient(ctx, "https://"+f.listener.Addr().String(), f.receiverID, f.senderID.Leaf, target, m)
	if err != nil {
		t.Fatal(err)
	}
	f.client = client
	if _, err := f.newSyncer(TransferOptions{}).Sync(ctx); err != nil {
		t.Fatal(err)
	}
	repeated, _ := f.receiverRepo.Heads(ctx, f.folder, "resume.bin")
	c, _ := json.Marshal(repeated)
	if !bytes.Equal(b, c) {
		t.Fatal("generation retry authored another version")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(f.receiverRepo, f.receiverID)
	server.now = f.server.now
	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.Serve(serveCtx, listener) }()
	defer func() { cancel(); listener.Close(); <-done }()
	reverse := network.NewManager(network.ManagerOptions{})
	defer reverse.Close()
	other := network.Target{Device: f.receiverID.DeviceID, Pin: f.receiverID.KeyPin, Purpose: network.PeerData}
	endpoint := "https://" + listener.Addr().String()
	if err := reverse.SetManual(other, endpoint); err != nil {
		t.Fatal(err)
	}
	reverseClient, err := NewRoutedClient(ctx, endpoint, f.senderID, f.receiverID.Leaf, other, reverse)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("two-way captured via manager")
	if err := os.WriteFile(filepath.Join(f.receiverRoot, "reverse.txt"), want, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.receiverWork.Scan(ctx, f.folder); err != nil {
		t.Fatal(err)
	}
	_, err = NewSyncer(f.senderRepo, f.senderWork, reverseClient, f.senderID.DeviceID, f.receiverID.DeviceID, f.folder, f.approved, TransferOptions{}).Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(f.senderRoot, "reverse.txt"))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("reverse transfer failed", err)
	}
	heads, _ := f.senderRepo.Heads(ctx, f.folder, "reverse.txt")
	remote, _ := f.receiverRepo.Heads(ctx, f.folder, "reverse.txt")
	a, _ = json.Marshal(heads)
	b, _ = json.Marshal(remote)
	if !bytes.Equal(a, b) || len(heads) != 1 || f.senderRepo.VerifyManifest(heads[0].Manifest) != nil {
		t.Fatal("two-way head/hash mismatch")
	}
	if m.Observe(target).Route != "direct" || reverse.Observe(other).Route != "direct" {
		t.Fatal("missing authenticated route observation")
	}
}
func TestWANW02RoutedPeerWrongPinAndUnknownRequester(t *testing.T) {
	f, m := routedFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	target := network.Target{Device: f.senderID.DeviceID, Pin: f.senderID.KeyPin, Purpose: network.PeerData}
	wrong := target
	wrong.Pin = history.Digest{1}
	if _, err := NewRoutedClient(ctx, "https://"+f.listener.Addr().String(), f.receiverID, f.senderID.Leaf, wrong, m); err == nil {
		t.Fatal("wrong pin accepted")
	}
	unknown, err := LoadOrCreateIdentity(t.TempDir(), fixedID('U'), f.server.now())
	if err != nil {
		t.Fatal(err)
	}
	isolated := network.NewManager(network.ManagerOptions{})
	defer isolated.Close()
	isolated.SetManual(target, "https://"+f.listener.Addr().String())
	client, err := NewRoutedClient(ctx, "https://"+f.listener.Addr().String(), unknown, f.senderID.Leaf, target, isolated)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Inventory(ctx, InventoryRequest{ProtocolVersion: ProtocolVersion, DeviceID: hex.EncodeToString(unknown.DeviceID[:]), FolderID: hex.EncodeToString(f.folder[:]), Revision: "1", MembershipDigest: hex.EncodeToString(f.approved.Digest[:]), PageSize: "1"})
	var wire *WireError
	if !errors.As(err, &wire) || wire.Body.Code != "UNAUTHORIZED" {
		t.Fatal("unknown requester did not fail authorization", err)
	}
}
