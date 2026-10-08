package terminal

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
	"github.com/calebhabesh/orbit/internal/pairing"
)

func (m *model) parseInvitation() tea.Cmd {
	f := m.flow
	raw := strings.TrimSpace(f.fields[len(f.fields)-1].input.Value())
	if code, err := pairing.Normalize(raw); err == nil {
		profile, _, _ := m.pairingOperator()
		if profile == "" {
			f.err = "Short codes need an Orbit operator. Select one with orbit network, or use a long invitation file."
			return nil
		}
		id, err := newID()
		if err != nil {
			return nil
		}
		w, _ := m.workflows()
		mutation := tc.Mutation{Version: tc.Version, Kind: "pairing", OperationID: id, Pairing: &tc.PairingIntent{Code: code, Profile: profile}}
		f.task = "parse_invitation"
		f.work = func(ctx context.Context) (tc.Result, error) { return w.Mutate(ctx, mutation) }
		return m.invalidate()
	}
	f.task = "parse_invitation"
	f.work = func(ctx context.Context) (tc.Result, error) {
		var inv tc.Invitation
		var b []byte
		var err error
		var code bool
		_, code, err = tc.DecodeInvitationCode(raw)
		b = []byte(raw)
		if err == nil && !code {
			b, err = controlclient.PrivateInvitation(ctx, raw)
		}
		if err == nil {
			inv, _, err = tc.DecodeInvitationText(b)
		}
		// Classify expired fresh input before its historical certificate check.
		// No transport or mutation runs for either rejected input.
		if err == nil {
			var expires time.Time
			expires, err = time.Parse(time.RFC3339Nano, inv.ExpiresAt)
			if err == nil && !time.Now().Before(expires) {
				err = errExpiredInvitation
			}
		}
		if err == nil {
			err = tc.ShapeError(inv.Validate())
		}
		return tc.Result{Invitation: &inv}, err
	}
	return m.invalidate()
}

func (m *model) pairingOperator() (digest, operator, privacy string) {
	f := m.flow
	if n := f.result.Network; n != nil && (n.Policy.Mode == "automatic" || n.Policy.Mode == "self_hosted") {
		return n.Policy.Profile, n.Operator, n.Privacy
	}
	if f.builtin != nil {
		return f.builtin.Digest, f.builtin.Operator, f.builtin.Privacy
	}
	return "", "", ""
}
