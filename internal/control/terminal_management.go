package control

import (
	"context"
	"encoding/hex"
	"fmt"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/protocol"
)

func (c *Controller) terminalSetups(ctx context.Context, q tc.Query) (tc.Result, error) {
	r := terminalResult()
	var err error
	r.Items, r.Cursor, err = c.db.PendingSetupPage(ctx, q)
	return r, err
}

func (c *Controller) terminalFolderManagement(ctx context.Context, q tc.Query) (tc.Result, error) {
	r := terminalResult()
	folder := terminalID(q.Folder)
	reg, err := c.db.Root(ctx, folder)
	if err != nil {
		return r, err
	}
	membership, _, err := c.db.GetMembership(ctx, folder)
	if err != nil {
		return r, err
	}
	digest, err := protocol.MembershipDigest(membership)
	if err != nil {
		return r, err
	}
	info := &tc.FolderManagement{Root: reg.Path, Paused: reg.Paused, MembershipDigest: hex.EncodeToString(digest[:]), Revision: tc.Uint(membership.Revision), Members: []tc.NamedItem{}}
	// Membership is protocol-bounded; no filesystem or path inventory is copied.
	for _, member := range membership.Active {
		name, err := c.db.GetDeviceDisplayName(ctx, member.Device)
		if err != nil {
			return r, err
		}
		if name == "" {
			name = fmt.Sprintf("Device %x", member.Device[:4])
		}
		info.Members = append(info.Members, tc.NamedItem{ID: hex.EncodeToString(member.Device[:]), Name: name, KeyPin: hex.EncodeToString(member.KeyPin[:])})
	}
	r, err = c.terminalStatus(ctx, tc.Query{Version: tc.Version, Kind: "status", Folder: q.Folder, Limit: 20})
	r.FolderManagement = info
	return r, err
}
