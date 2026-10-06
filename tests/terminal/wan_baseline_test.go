package terminal_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/replication"
)

// This passing baseline records missing automatic routing, rather than treating
// that future feature as a failure of the supported manual-network contract.
// The existing T04/T05 process journeys own ordinary sync and address refresh.
func TestWANW00ManualInvitationRequiresReachableAddress(t *testing.T) {
	f := newEnrollmentFixture(t)
	ctx := context.Background()
	root := filepath.Join(f.owner.root, "protected")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "captured.txt")
	want := []byte("captured before unavailable networking\n")
	if err := os.WriteFile(path, want, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.ws.Register(ctx, f.folder, root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.ws.Scan(ctx, f.folder); err != nil {
		t.Fatal(err)
	}
	heads, err := f.owner.db.Heads(ctx, f.folder, "captured.txt")
	if err != nil || len(heads) != 1 || f.owner.db.VerifyManifest(heads[0].Manifest) != nil {
		t.Fatal("protected captured version missing")
	}
	before, err := json.Marshal(heads)
	if err != nil {
		t.Fatal(err)
	}
	var saved = map[string][]byte{}
	for _, state := range []string{f.owner.state, f.joiner.state} {
		for _, name := range []string{"config.json", "identity/peer-identity.pem"} {
			p := filepath.Join(state, name)
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			saved[p] = b
		}
	}
	for _, field := range []string{"enrollment", "peer"} {
		inv := f.inv
		if field == "enrollment" {
			inv.EnrollmentEndpoint = ""
		} else {
			inv.PeerEndpoint = ""
		}
		c, err := replication.NewEnrollmentClient(inv, f.joinerID)
		if err == nil {
			c.Close()
			t.Fatalf("manual invitation accepted missing %s address", field)
		}
	}
	// Hold a kernel-assigned port without accepting TLS. This models an unusable
	// explicit address without risking a dial to a personal/remote service or a
	// reserve-close race with another process. The request must respect its bound.
	listener, err := net.Listen("tcp", net.JoinHostPort(networkIP(t), "0"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	inv := f.inv
	inv.EnrollmentEndpoint = "https://" + listener.Addr().String()
	c, err := replication.NewEnrollmentClient(inv, f.joinerID)
	if err != nil {
		t.Fatal("syntactically valid unreachable invitation rejected")
	}
	defer c.Close()
	bounded, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = c.Prepare(bounded, enrollmentRandom(t), "Synthetic joiner", "")
	if err == nil || bounded.Err() != context.DeadlineExceeded || time.Since(started) > 3*time.Second {
		t.Fatal("unreachable manual address did not fail within request bound")
	}
	for p, b := range saved {
		after, err := os.ReadFile(p)
		if err != nil || !bytes.Equal(b, after) {
			t.Fatal("network failure changed persistent identity")
		}
	}
	afterHeads, err := f.owner.db.Heads(ctx, f.folder, "captured.txt")
	if err != nil || len(afterHeads) != 1 || f.owner.db.VerifyManifest(afterHeads[0].Manifest) != nil {
		t.Fatal("network failure lost protected version content")
	}
	after, err := json.Marshal(afterHeads)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("network failure changed immutable head")
	}
	actual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, want) {
		t.Fatal("network failure changed local bytes")
	}
	t.Log("missing endpoint refused; unusable endpoint bounded; both identities, protected head/CAS and local bytes preserved")
}
