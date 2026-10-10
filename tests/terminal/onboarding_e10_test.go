package terminal_test

import (
	"context"
	"testing"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
)

// E10 trial finding: a device that was only switched off left one
// EXHAUSTED_WORK item per periodic sync (50 on the owner's PC), which also
// kept the TUI off Files. Syncs that could not reach the device (offline,
// service busy or restarting) are not attention: they resolve when a later
// sync succeeds, and a long absence is OFFLINE. Other sync
// failures show once per folder, device and cause.
func TestOnboardingE10OfflinePeerSyncsAreNotAttention(t *testing.T) {
	f := fresh(t)
	ctx := context.Background()
	folder := f.folder("e10")
	peer := history.ID{7}
	current, approved, err := f.db.GetMembership(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	current.Revision++
	current.PriorDigest = approved.Digest
	current.Active = append(current.Active, protocol.ActiveMember{Device: peer, KeyPin: history.Digest{7}})
	if _, err := f.db.ApproveMembership(ctx, current); err != nil {
		t.Fatal(err)
	}
	exhaust := func(code string) string {
		t.Helper()
		id, err := f.db.EnqueueDurableTask(ctx, repository.DurableTask{Folder: folder, Peer: &peer, Kind: "sync", MaxAttempts: 5})
		if err != nil {
			t.Fatal(err)
		}
		if err = f.db.UpdateDurableTaskState(ctx, id, "exhausted", 5, "sync failed", code, 0); err != nil {
			t.Fatal(err)
		}
		return id
	}
	for _, code := range []string{protocol.NetworkUnavailable, protocol.NetworkUnavailable, protocol.NetworkServiceUnavailable, protocol.NetworkQuota, protocol.NetworkStaleGeneration, "NETWORK_BUSY"} {
		exhaust(code)
	}
	first := exhaust("IO_ERROR")
	exhaust("IO_ERROR")
	res, err := terminalClient(t, f).Query(ctx, tc.Query{Version: tc.Version, Kind: "attention", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	var exhausted []tc.Attention
	for _, a := range res.Attention {
		if a.Code == "EXHAUSTED_WORK" {
			exhausted = append(exhausted, a)
		}
	}
	if len(exhausted) != 1 || exhausted[0].OperationID != first {
		t.Fatalf("want one IO_ERROR item for the first failed sync %s; got %+v", first, exhausted)
	}
}
