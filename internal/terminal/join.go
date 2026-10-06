package terminal

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/controlclient"
)

func (m *model) parseInvitation() tea.Cmd {
	f := m.flow
	raw := strings.TrimSpace(f.fields[len(f.fields)-1].input.Value())
	f.task = "parse_invitation"
	f.work = func(ctx context.Context) (tc.Result, error) {
		var inv tc.Invitation
		var b []byte
		var err error
		var code bool
		b, code, err = tc.DecodeInvitationCode(raw)
		if err == nil && !code {
			b, err = controlclient.PrivateInvitation(ctx, raw)
		}
		if err == nil {
			err = tc.Decode(b, &inv)
		}
		if err == nil {
			err = inv.ExpandPackaged()
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
			err = inv.Validate()
		}
		return tc.Result{Invitation: &inv}, err
	}
	return m.invalidate()
}
