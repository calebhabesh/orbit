package replication

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/network"
)

// R01 route accounting: real pulls through the production manager over each
// route class. Each check is this device's (the receiver's) observation.

const r01Size = 3 << 20

// r01Pull writes random bytes on the sender and pulls them to the receiver.
func r01Pull(t *testing.T, f *syncFixture, name string) {
	t.Helper()
	data := make([]byte, r01Size)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.senderRoot, name), data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := f.senderWork.Scan(ctx, f.folder); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSyncer(f.receiverRepo, f.receiverWork, f.client, f.receiverID.DeviceID, f.senderID.DeviceID, f.folder, f.approved, TransferOptions{}).Sync(ctx); err != nil {
		t.Fatal(err)
	}
}

// r01Check asserts that all of the receiver's counted traffic toward the
// sender is in one class and covers at least the pulled bytes.
func r01Check(t *testing.T, m *network.ConnectionManager, sender history.ID, want string) {
	t.Helper()
	device := hex.EncodeToString(sender[:])
	var found *network.PeerRouteUsage
	for _, u := range m.RouteUsage().Window(time.Unix(0, 0)) {
		if u.Device == device && u.Purpose == string(network.PeerData) {
			found = &u
		}
	}
	if found == nil {
		t.Fatal("no route usage for the sender")
	}
	for _, class := range network.RouteClasses {
		got := found.Routes[class]
		if class == want {
			if got.Requests == 0 || got.Received < r01Size || got.Sent == 0 {
				t.Fatalf("%s undercounted: %+v", class, got)
			}
			t.Logf("%s: requests=%d sent=%d received=%d (pulled %d)", class, got.Requests, got.Sent, got.Received, r01Size)
		} else if got != (network.RouteTotals{}) {
			t.Fatalf("traffic misclassified as %s: %+v (want only %s)", class, got, want)
		}
	}
}

func TestFinalR01RouteAccountingLANTCP(t *testing.T) {
	f, m := routedFixture(t)
	r01Pull(t, f, "lan.bin")
	r01Check(t, m, f.senderID.DeviceID, network.RouteLAN)
}

// A public peer address cannot be produced without a namespace; the dialer
// reports a documentation-range remote address over the real loopback socket,
// which exercises the manager's classification of the dialed peer address.
type r01PublicConn struct{ net.Conn }

func (r01PublicConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 443}
}

func TestFinalR01RouteAccountingWANTCP(t *testing.T) {
	f := newSyncFixture(t)
	dial := (&net.Dialer{Timeout: 3 * time.Second}).DialContext
	m := network.NewManager(network.ManagerOptions{DialContext: func(ctx context.Context, n, a string) (net.Conn, error) {
		c, err := dial(ctx, n, a)
		if err != nil {
			return nil, err
		}
		return r01PublicConn{c}, nil
	}})
	t.Cleanup(func() { _ = m.Close() })
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
	r01Pull(t, f, "wan.bin")
	r01Check(t, m, f.senderID.DeviceID, network.RouteWANTCP)
}

func TestFinalR01RouteAccountingQUIC(t *testing.T) {
	f, _, b, _, _ := quicFixture(t)
	r01Pull(t, f, "quic.bin")
	r01Check(t, b, f.senderID.DeviceID, network.RouteQUIC)
}

func TestFinalR01RouteAccountingRelay(t *testing.T) {
	f := newSyncFixture(t)
	r := connectProductionRelay(t, f)
	r01Pull(t, f, "relay.bin")
	r01Check(t, r.b, f.senderID.DeviceID, network.RouteRelay)
}

// TestFinalR01OneGiBPull pulls 1 GiB over the LAN fixture with route
// accounting on and records duration, requests and counted bytes. Opt-in:
// ORBIT_FINAL_R01_GIB=1 (about 1 GiB of disposable disk per side).
func TestFinalR01OneGiBPull(t *testing.T) {
	if os.Getenv("ORBIT_FINAL_R01_GIB") != "1" {
		t.Skip("set ORBIT_FINAL_R01_GIB=1")
	}
	f, m := routedFixture(t)
	const size = 1 << 30
	file, err := os.Create(filepath.Join(f.senderRoot, "gib.bin"))
	if err != nil {
		t.Fatal(err)
	}
	block := make([]byte, 4<<20)
	for written := 0; written < size; written += len(block) {
		if _, err = rand.Read(block); err != nil {
			t.Fatal(err)
		}
		if _, err = file.Write(block); err != nil {
			t.Fatal(err)
		}
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = f.senderWork.Scan(ctx, f.folder); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err = NewSyncer(f.receiverRepo, f.receiverWork, f.client, f.receiverID.DeviceID, f.senderID.DeviceID, f.folder, f.approved, TransferOptions{}).Sync(ctx); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	usage := m.RouteUsage().Window(time.Unix(0, 0))
	if len(usage) != 1 {
		t.Fatalf("usage: %+v", usage)
	}
	lan := usage[0].Routes[network.RouteLAN]
	if lan.Received < size {
		t.Fatalf("undercounted 1 GiB pull: %+v", lan)
	}
	t.Logf("R01 1 GiB pull: %s (%.0f MiB/s); lan requests=%d sent=%d received=%d (+%.4f%% over file bytes)",
		elapsed.Round(time.Millisecond), float64(size)/(1<<20)/elapsed.Seconds(), lan.Requests, lan.Sent, lan.Received, float64(lan.Received-size)*100/size)
}
