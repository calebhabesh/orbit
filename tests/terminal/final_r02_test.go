package terminal_test

import (
	"context"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/rendezvous"
)

func r02Allowlist(t *testing.T, nodes ...*w05Node) *rendezvous.Admission {
	t.Helper()
	list := []rendezvous.AllowedDevice{}
	for _, n := range nodes {
		list = append(list, rendezvous.AllowedDevice{Device: hex.EncodeToString(n.id.DeviceID[:]), Pin: hex.EncodeToString(n.id.KeyPin[:])})
	}
	a, err := rendezvous.NewAllowlist(list)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestFinalR02UnlistedJoinerEnrollsThroughAllowlist: with only the inviter
// listed, a new device pairs through the service (routed v3 invitation,
// enrollment relay, approval). Its bootstrap needs peer data, which is refused
// until the operator lists it; the same join then resumes and both devices
// sync through the service.
func TestFinalR02UnlistedJoinerEnrollsThroughAllowlist(t *testing.T) {
	service, selection, origin, roots, _ := w05Service(t)
	a, b := newW05Node(t, selection, origin, roots), newW05Node(t, selection, origin, roots)
	service.SetAdmission(r02Allowlist(t, a))
	folder := w05Create(t, a, "source")
	inv := w05Invite(t, a, folder, "")
	m := w05Join(t, b, inv, "join")
	r, err := w05MutateRetry(t, b, m)
	if err != nil || r.Error != nil || r.Operation.Phase != "awaiting_approval" {
		t.Fatalf("unlisted join through allowlisted service: %v %+v", err, r.Error)
	}
	w05Approve(t, a, r.Join.Request)
	time.Sleep(26 * time.Second)
	r, err = b.f.ctrl.TerminalMutate(context.Background(), m)
	if err != nil || r.Error == nil || !r.Error.Retryable {
		t.Fatalf("unlisted bootstrap must wait, resumably, for the operator: %v %+v", err, r.Error)
	}
	status, err := b.f.ctrl.TerminalQuery(context.Background(), tc.Query{Version: tc.Version, Kind: "network_status"})
	if err != nil || status.Network == nil || status.Network.Code != "NOT_ADMITTED" {
		t.Fatal("unlisted member status", err, status.Network)
	}
	t.Logf("approved but unlisted: join %s (retryable); network code=%s action=%q", r.Error.Code, status.Network.Code, status.Network.Action)
	// Operator lists the new member and reloads; a daemon restart asks at once.
	service.SetAdmission(r02Allowlist(t, a, b))
	b.restart(t, nil)
	r, err = w05MutateRetry(t, b, m)
	if err != nil || r.Error != nil || !r.Readiness.Ready() {
		t.Fatalf("bootstrap after listing: %v %+v", err, r.Error)
	}
	w05Pull(t, b, a, folder)
	w05AssertFiles(t, a, b, folder, filepath.Join(a.f.root, "source"), m.Join.Root)
	if m := service.Metrics(); m.RefusedNotAdmitted == 0 || m.AllowlistSize != 2 {
		t.Fatalf("admission metrics: %+v", m)
	}
}
