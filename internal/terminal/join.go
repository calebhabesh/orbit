package terminal

import (
	"context"
	"encoding/base64"
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
		if strings.HasPrefix(raw, "orbit-invitation:v2:") {
			b, err = base64.RawURLEncoding.DecodeString(strings.TrimPrefix(raw, "orbit-invitation:v2:"))
		} else {
			b, err = controlclient.PrivateInvitation(ctx, raw)
		}
		if err == nil {
			err = tc.Decode(b, &inv)
		}
		if err == nil {
			err = inv.Validate()
		}
		if err == nil {
			expires, _ := time.Parse(time.RFC3339Nano, inv.ExpiresAt)
			if !time.Now().Before(expires) {
				err = errExpiredInvitation
			}
		}
		return tc.Result{Invitation: &inv}, err
	}
	return m.invalidate()
}
