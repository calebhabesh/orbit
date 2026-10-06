package control_test

import (
	"context"
	"encoding/hex"
	"errors"
	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/workspace"
	"testing"

	"github.com/calebhabesh/file-sync/internal/config"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
)

func TestWANW06PolicyReviewReplayAndStaleness(t *testing.T) {
	c, _, dir, cleanup := setupTestController(t)
	defer cleanup()
	ctx := context.Background()
	before, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	query := tc.Query{Version: tc.Version, Kind: "network_preview", NetworkPlan: &tc.NetworkIntent{Policy: tc.NetworkPolicy{Mode: "local_only"}}}
	first, err := c.TerminalQuery(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.TerminalQuery(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	op := randomFolderID(t)
	m := tc.Mutation{Version: tc.Version, OperationID: hex.EncodeToString(op[:]), Kind: "network", Network: &tc.NetworkIntent{Policy: first.Network.Policy, Review: *first.Review}}
	r, err := c.TerminalMutate(ctx, m)
	if err != nil || r.State != "completed" {
		t.Fatal(r, err)
	}
	replay, err := c.TerminalMutate(ctx, m)
	if err != nil || replay.Operation.ID != r.Operation.ID {
		t.Fatal(replay, err)
	}
	id := randomFolderID(t)
	stale := tc.Mutation{Version: tc.Version, OperationID: hex.EncodeToString(id[:]), Kind: "network", Network: &tc.NetworkIntent{Policy: second.Network.Policy, Review: *second.Review}}
	if _, err = c.TerminalMutate(ctx, stale); err == nil {
		t.Fatal("stale policy review accepted")
	}
	changed := *m.Network
	changed.Policy.Mode = "manual"
	m.Network = &changed
	if _, err = c.TerminalMutate(ctx, m); err == nil {
		t.Fatal("changed operation accepted")
	}
	after, err := config.Load(dir)
	if err != nil || before.DeviceID != after.DeviceID {
		t.Fatal("policy changed identity", err)
	}
	policy, err := config.LoadNetworkPolicy(dir)
	if err != nil || policy.Mode != "local_only" || policy.Generation != 2 {
		t.Fatal(policy, err)
	}
	query.NetworkPlan.Policy = tc.NetworkPolicy{Mode: "manual", LANAdvertising: true}
	if _, err = c.TerminalQuery(ctx, query); err == nil {
		t.Fatal("unimplemented advertising accepted")
	}
}

func TestWANW06AcceptedPolicyRecoversBeforeActivation(t *testing.T) {
	c, db, dir, cleanup := setupTestController(t)
	defer cleanup()
	ctx := context.Background()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	id, err := hex.DecodeString(cfg.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	var local history.ID
	copy(local[:], id)
	injected := errors.New("disposable interruption after durable policy acceptance")
	c = control.New(db, workspace.New(db, workspace.Options{}), control.Options{LocalDevice: local, FaultHook: func(name string) error {
		if name == "terminal.network.accepted" {
			return injected
		}
		return nil
	}})
	preview, err := c.TerminalQuery(ctx, tc.Query{Version: tc.Version, Kind: "network_preview", NetworkPlan: &tc.NetworkIntent{Policy: tc.NetworkPolicy{Mode: "local_only"}}})
	if err != nil {
		t.Fatal(err)
	}
	op := randomFolderID(t)
	m := tc.Mutation{Version: tc.Version, Kind: "network", OperationID: hex.EncodeToString(op[:]), Network: &tc.NetworkIntent{Policy: preview.Network.Policy, Review: *preview.Review}}
	accepted, err := c.TerminalMutate(ctx, m)
	if !errors.Is(err, injected) || accepted.Operation.State != "running" {
		t.Fatal(accepted, err)
	}
	policy, err := config.LoadNetworkPolicy(dir)
	if err != nil || policy.Mode != "manual" {
		t.Fatal("effect happened before durable boundary", policy, err)
	}
	recovered := control.New(db, workspace.New(db, workspace.Options{}), control.Options{LocalDevice: local})
	if err = recovered.RecoverTerminalOperations(ctx); err != nil {
		t.Fatal(err)
	}
	policy, err = config.LoadNetworkPolicy(dir)
	if err != nil || policy.Mode != "local_only" {
		t.Fatal(policy, err)
	}
	replay, err := recovered.TerminalMutate(ctx, m)
	if err != nil || replay.Operation.State != "completed" || replay.Operation.ID != accepted.Operation.ID {
		t.Fatal(replay, err)
	}
}
