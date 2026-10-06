package control_test

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/network"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

func TestWANW12LocalOnlyDoctorIsPassiveAndTyped(t *testing.T) {
	_, db, dir, cleanup := setupTestController(t)
	defer cleanup()
	policy := tc.NetworkPolicy{Mode: "local_only", Generation: 2}
	if e := config.SaveNetworkPolicy(dir, policy); e != nil {
		t.Fatal(e)
	}
	manager := network.NewManager(network.ManagerOptions{})
	defer manager.Close()
	c := control.New(db, workspace.New(db, workspace.Options{}), control.Options{Network: manager, NetworkPolicy: &policy})
	r, e := c.TerminalQuery(context.Background(), tc.Query{Version: tc.Version, Kind: "network_status"})
	if e != nil || r.Network.Code != "LOCAL_ONLY" || len(r.Network.Probes) != 0 || r.Network.Action == "" {
		t.Fatal(r.Network, e)
	}
	r, e = c.TerminalQuery(context.Background(), tc.Query{Version: tc.Version, Kind: "network_doctor"})
	if e != nil || len(r.Network.Probes) != 6 {
		t.Fatal(r.Network, e)
	}
	for _, probe := range r.Network.Probes {
		if probe.Kind == "direct_tls" {
			if probe.Code != "NOT_TESTED" {
				t.Fatal(probe)
			}
		} else if probe.Code != "DISABLED_BY_POLICY" {
			t.Fatal(probe)
		}
	}
}
func TestWANW12InternetDisableClosesBeforeCompletionAndRetainsIntent(t *testing.T) {
	_, db, dir, cleanup := setupTestController(t)
	defer cleanup()
	active := tc.NetworkPolicy{Mode: "automatic", AwaitingProfile: true, Generation: 2}
	if e := config.SaveNetworkPolicy(dir, active); e != nil {
		t.Fatal(e)
	}
	cfg, _ := config.Load(dir)
	manager := network.NewManager(network.ManagerOptions{})
	defer manager.Close()
	stopped := false
	c := control.New(db, workspace.New(db, workspace.Options{}), control.Options{Network: manager, NetworkPolicy: &active, StopInternet: func() error { stopped = true; return manager.Close() }})
	p, e := c.TerminalQuery(context.Background(), tc.Query{Version: tc.Version, Kind: "network_preview", NetworkPlan: &tc.NetworkIntent{Policy: tc.NetworkPolicy{Mode: "local_only", LANAdvertising: true}}})
	if e != nil {
		t.Fatal(e)
	}
	op := randomFolderID(t)
	mutation := tc.Mutation{Version: tc.Version, Kind: "network", OperationID: hex.EncodeToString(op[:]), Network: &tc.NetworkIntent{Policy: p.Network.Policy, Review: *p.Review}}
	r, e := c.TerminalMutate(context.Background(), mutation)
	if e != nil || r.State != "completed" || !stopped {
		t.Fatal(r, e, stopped)
	}
	r, e = c.TerminalQuery(context.Background(), tc.Query{Version: tc.Version, Kind: "network_doctor"})
	if e != nil || r.Network.Code != "NETWORK_RESTART_REQUIRED" || len(r.Network.Probes) != 0 {
		t.Fatal(r.Network, e)
	}
	after, _ := config.Load(dir)
	if cfg.DeviceID != after.DeviceID {
		t.Fatal("identity changed")
	}
	c = control.New(db, workspace.New(db, workspace.Options{}), control.Options{})
	r, e = c.TerminalMutate(context.Background(), mutation)
	if e != nil || r.State != "completed" {
		t.Fatal("restart replay", r, e)
	}
	policy, e := config.LoadNetworkPolicy(dir)
	if e != nil || policy.Mode != "local_only" || policy.Generation != 3 {
		t.Fatal(policy, e)
	}
}
func TestWANW12RestartRequiredHasNextAction(t *testing.T) {
	_, db, dir, cleanup := setupTestController(t)
	defer cleanup()
	active := tc.NetworkPolicy{Mode: "manual", Generation: 1}
	if e := config.SaveNetworkPolicy(dir, tc.NetworkPolicy{Mode: "local_only", Generation: 2}); e != nil {
		t.Fatal(e)
	}
	c := control.New(db, workspace.New(db, workspace.Options{}), control.Options{NetworkPolicy: &active, Now: func() time.Time { return time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC) }})
	r, e := c.TerminalQuery(context.Background(), tc.Query{Version: tc.Version, Kind: "network_status"})
	if e != nil || !r.Network.RestartRequired || r.Network.Action == "" {
		t.Fatal(r.Network, e)
	}
}
