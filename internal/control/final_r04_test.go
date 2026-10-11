package control_test

import (
	"context"
	"strings"
	"testing"

	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/workspace"
)

// R04: Automatic with no stored and no packaged profile says so, and short
// pairing codes explain that this device has no service.
func TestFinalR04NoServiceConfigured(t *testing.T) {
	restore := network.OverrideBuiltinProfile(nil)
	defer restore()
	_, db, _, cleanup := setupTestController(t)
	defer cleanup()
	c := control.New(db, workspace.New(db, workspace.Options{}), control.Options{})
	ctx := context.Background()
	status, err := c.TerminalQuery(ctx, tc.Query{Version: tc.Version, Kind: "network_status"})
	if err != nil || status.Network.Builtin != nil || status.Network.AutomaticOffer {
		t.Fatal("public build offers a packaged service", status.Network, err)
	}
	in := tc.NetworkIntent{Policy: tc.NetworkPolicy{Mode: "automatic", AwaitingProfile: true, LANAdvertising: true}}
	if r, e := w14Apply(t, c, in); e != nil || r.State != "completed" {
		t.Fatal(r, e)
	}
	status, err = c.TerminalQuery(ctx, tc.Query{Version: tc.Version, Kind: "network_status"})
	if err != nil || status.Network.Code != "NO_SERVICE_CONFIGURED" || status.Network.Ready || status.Network.Action != tc.NoServiceAction {
		t.Fatal("automatic without a service", status.Network, err)
	}
	r, err := c.TerminalMutate(ctx, tc.Mutation{Version: tc.Version, Kind: "pairing", OperationID: strings.Repeat("ab", 32), Pairing: &tc.PairingIntent{Code: "ABCD-EFGH", Profile: strings.Repeat("d", 64)}})
	code := ""
	if r.Error != nil {
		code = r.Error.Code
	} else if err != nil {
		code = err.Error()
	}
	if !strings.Contains(code, "NO_SERVICE_CONFIGURED") {
		t.Fatal("short code without a service", code, r.Error, err)
	}
}
