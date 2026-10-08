package control_test

import (
	"context"
	"encoding/hex"
	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/workspace"
	"testing"
)

func TestWANW11TimingReviewIsExactDurableAndRestartRequired(t *testing.T) {
	c, db, dir, cleanup := setupTestController(t)
	defer cleanup()
	ctx := context.Background()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	idBytes, err := hex.DecodeString(cfg.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	var local history.ID
	copy(local[:], idBytes)
	active, err := config.LoadNetworkPolicy(dir)
	if err != nil {
		t.Fatal(err)
	}
	c = control.New(db, workspace.New(db, workspace.Options{}), control.Options{LocalDevice: local, NetworkPolicy: &active})
	timing := protocol.RouteTiming{HeadStartMS: 1500, CycleMS: 15000, ProbeMS: 10000, CooldownMS: 60000, PollMS: 500, QuietMS: 2000}
	in := tc.NetworkIntent{Policy: tc.NetworkPolicy{Mode: "manual", Timing: timing}}
	preview, err := c.TerminalQuery(ctx, tc.Query{Version: tc.Version, Kind: "network_preview", NetworkPlan: &in})
	if err != nil {
		t.Fatal(err)
	}
	id := randomFolderID(t)
	m := tc.Mutation{Version: tc.Version, Kind: "network", OperationID: hex.EncodeToString(id[:]), Network: &tc.NetworkIntent{Policy: preview.Network.Policy, Review: *preview.Review}}
	changed := *m.Network
	changed.Policy.Timing.HeadStartMS = 2000
	bad := m
	bad.Network = &changed
	if _, err = c.TerminalMutate(ctx, bad); err == nil {
		t.Fatal("unreviewed timing accepted")
	}
	result, err := c.TerminalMutate(ctx, m)
	if err != nil || result.State != "completed" {
		t.Fatal(result, err)
	}
	replay, err := c.TerminalMutate(ctx, m)
	if err != nil || replay.Operation.ID != m.OperationID {
		t.Fatal(replay, err)
	}
	saved, err := config.LoadNetworkPolicy(dir)
	if err != nil || saved.Timing != timing {
		t.Fatal(saved, err)
	}
	current, err := config.Load(dir)
	if err != nil || current.DeviceID != cfg.DeviceID {
		t.Fatal("identity changed", err)
	}
	// The active controller still has legacy/default timing until daemon restart.
	status, err := c.TerminalQuery(ctx, tc.Query{Version: tc.Version, Kind: "network_status"})
	if err != nil {
		t.Fatal(err)
	}
	if status.Network.Policy.Timing != timing || !status.Network.RestartRequired || status.Network.ActivePolicy.Timing != (protocol.RouteTiming{}) {
		t.Fatal(status.Network)
	}
	in.Policy.Timing.PollMS = 1
	if _, err = c.TerminalQuery(ctx, tc.Query{Version: tc.Version, Kind: "network_preview", NetworkPlan: &in}); err == nil {
		t.Fatal("unbounded watcher accepted")
	}
}
