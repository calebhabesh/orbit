package control_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/workspace"
)

func w14Selection(t *testing.T, signer ed25519.PrivateKey, operator, env string, epoch uint64, privacy string) network.ProfileSelection {
	t.Helper()
	service, _, _ := ed25519.GenerateKey(rand.Reader)
	p := protocol.NetworkProfile{Version: "1", Operator: operator, Authority: hex.EncodeToString(signer.Public().(ed25519.PublicKey)), ServiceKey: hex.EncodeToString(service), Epoch: protocol.NetworkUint(epoch), Expires: protocol.NetworkUint(time.Now().Add(24 * time.Hour).Unix()), Origins: []string{"https://orbit.example.net:8443", "wss://orbit.example.net:8443"}, STUN: []string{}, Privacy: privacy}
	b, err := p.Canonical(env != "release")
	if err != nil {
		t.Fatal(err)
	}
	p.Signature = hex.EncodeToString(ed25519.Sign(signer, b))
	return network.ProfileSelection{Profile: p, Authority: p.Authority, HighestEpoch: p.Epoch, Environment: env}
}

func w14Apply(t *testing.T, c *control.Controller, in tc.NetworkIntent) (tc.Result, error) {
	t.Helper()
	p, err := c.TerminalQuery(context.Background(), tc.Query{Version: tc.Version, Kind: "network_preview", NetworkPlan: &in})
	if err != nil {
		return p, err
	}
	in.Policy, in.Review = p.Network.Policy, *p.Review
	op := make([]byte, 32)
	rand.Read(op)
	return c.TerminalMutate(context.Background(), tc.Mutation{Version: tc.Version, Kind: "network", OperationID: hex.EncodeToString(op), Network: &in})
}

// A manual install sees the one-time Automatic offer; reviewing the packaged
// digest installs the packaged profile, and a different operator needs an
// explicit replacement.
func TestWANW14PackagedOfferReviewAndOperatorReplacement(t *testing.T) {
	_, signer, _ := ed25519.GenerateKey(rand.Reader)
	packaged := w14Selection(t, signer, "Packaged operator", "release", 1, "Ephemeral routing metadata.")
	restore := network.OverrideBuiltinProfile(&packaged)
	defer restore()
	digest, _ := packaged.Digest()
	_, db, dir, cleanup := setupTestController(t)
	defer cleanup()
	c := control.New(db, workspace.New(db, workspace.Options{}), control.Options{})
	ctx := context.Background()

	status, err := c.TerminalQuery(ctx, tc.Query{Version: tc.Version, Kind: "network_status"})
	if err != nil || status.Network.Policy.Mode != "manual" || !status.Network.AutomaticOffer || status.Network.Builtin == nil || status.Network.Builtin.Digest != digest || !strings.Contains(status.Network.Action, "orbit network automatic") {
		t.Fatal("manual install lacks offer", status.Network, err)
	}

	// Declining keeps manual mode and ends the offer.
	if r, e := w14Apply(t, c, tc.NetworkIntent{Policy: status.Network.Policy, DeclineAutomaticOffer: true}); e != nil || r.State != "completed" {
		t.Fatal(r, e)
	}
	status, _ = c.TerminalQuery(ctx, tc.Query{Version: tc.Version, Kind: "network_status"})
	if status.Network.AutomaticOffer || status.Network.Policy.Mode != "manual" {
		t.Fatal("decline not retained", status.Network)
	}

	// Reviewing Automatic with the packaged digest shows and installs it.
	in := tc.NetworkIntent{Policy: tc.NetworkPolicy{Mode: "automatic", Profile: digest, LANAdvertising: true}}
	preview, err := c.TerminalQuery(ctx, tc.Query{Version: tc.Version, Kind: "network_preview", NetworkPlan: &in})
	if err != nil || preview.Network.Operator != "Packaged operator" || preview.Network.Privacy == "" {
		t.Fatal("packaged operator not reviewed", preview.Network, err)
	}
	if r, e := w14Apply(t, c, in); e != nil || r.State != "completed" {
		t.Fatal(r, e)
	}
	stored, err := config.LoadNetworkProfile(dir, uint64(time.Now().Unix()))
	if sd, _ := stored.Digest(); err != nil || sd != digest {
		t.Fatal("packaged profile not installed", err)
	}

	// A self-hosted profile from another authority is refused without the
	// explicit replacement, then accepted with it.
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	self := w14Selection(t, other, "Home server", "self_hosted", 1, "Self-hosted.")
	sd, _ := self.Digest()
	switchIntent := tc.NetworkIntent{Policy: tc.NetworkPolicy{Mode: "self_hosted", Profile: sd, LANAdvertising: true}, Profile: &self.Profile, Authority: self.Authority, Environment: "self_hosted"}
	if _, err = c.TerminalQuery(ctx, tc.Query{Version: tc.Version, Kind: "network_preview", NetworkPlan: &switchIntent}); err == nil || !strings.Contains(err.Error(), "PROFILE_OPERATOR_CHANGE") {
		t.Fatal("operator change not refused", err)
	}
	switchIntent.ReplaceOperator = true
	if r, e := w14Apply(t, c, switchIntent); e != nil || r.State != "completed" {
		t.Fatal(r, e)
	}
	stored, _ = config.LoadNetworkProfile(dir, uint64(time.Now().Unix()))
	if stored.Authority != self.Authority {
		t.Fatal("switch not stored")
	}
	// Switching back to the packaged operator is again an explicit review.
	back := tc.NetworkIntent{Policy: tc.NetworkPolicy{Mode: "automatic", Profile: digest, LANAdvertising: true}, ReplaceOperator: true}
	if r, e := w14Apply(t, c, back); e != nil || r.State != "completed" {
		t.Fatal(r, e)
	}
}

// A newer packaged epoch with changed text is offered for review; reviewing
// its digest updates the stored selection.
func TestWANW14PackagedUpdateNeedsReviewWhenTextChanges(t *testing.T) {
	_, signer, _ := ed25519.GenerateKey(rand.Reader)
	epoch1 := w14Selection(t, signer, "Packaged operator", "release", 1, "Ephemeral routing metadata.")
	epoch2 := w14Selection(t, signer, "Packaged operator", "release", 2, "Routing metadata kept for 7 days.")
	_, db, dir, cleanup := setupTestController(t)
	defer cleanup()
	d1, _ := epoch1.Digest()
	if err := config.SaveNetworkProfile(dir, epoch1, uint64(time.Now().Unix())); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveNetworkPolicy(dir, tc.NetworkPolicy{Mode: "automatic", Profile: d1, LANAdvertising: true, Generation: 2}); err != nil {
		t.Fatal(err)
	}
	restore := network.OverrideBuiltinProfile(&epoch2)
	defer restore()
	c := control.New(db, workspace.New(db, workspace.Options{}), control.Options{})
	status, err := c.TerminalQuery(context.Background(), tc.Query{Version: tc.Version, Kind: "network_status"})
	if err != nil || status.Network.ProfileUpdate != "available" || !strings.Contains(status.Network.Action, "orbit network update") {
		t.Fatal(status.Network, err)
	}
	d2, _ := epoch2.Digest()
	in := tc.NetworkIntent{Policy: status.Network.Policy}
	in.Policy.Profile = d2
	if r, e := w14Apply(t, c, in); e != nil || r.State != "completed" {
		t.Fatal(r, e)
	}
	status, _ = c.TerminalQuery(context.Background(), tc.Query{Version: tc.Version, Kind: "network_status"})
	if status.Network.ProfileUpdate != "" || status.Network.Policy.Profile != d2 {
		t.Fatal("update not applied", status.Network)
	}
}

func TestWANW14CapabilityAdvertised(t *testing.T) {
	_, db, _, cleanup := setupTestController(t)
	defer cleanup()
	c := control.New(db, workspace.New(db, workspace.Options{}), control.Options{})
	r, err := c.TerminalQuery(context.Background(), tc.Query{Version: tc.Version, Kind: "capabilities"})
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range r.Capabilities {
		if capability == tc.PackagedProfileCapability {
			return
		}
	}
	t.Fatal("packaged profile capability missing", r.Capabilities)
}
