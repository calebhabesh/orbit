package control

import (
	"context"
	"encoding/hex"
	"testing"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/workspace"
)

// Only joins the setup worker still resumes, and only once they registered the
// root, hold their folder from the scheduler.
func TestJoiningFoldersNamesResumableJoinsOnly(t *testing.T) {
	ctx := context.Background()
	db, err := repository.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	save := func(id, kind string, folder byte, phase, state, code string) history.ID {
		var f history.ID
		f[0] = folder
		r := terminalResult()
		r.State = state
		r.Operation = &tc.Operation{ID: id, Kind: kind, State: state, Phase: phase, CommittedEffects: []tc.Effect{}}
		r.Join = &tc.JoinRecord{Operation: *r.Operation, Folder: hex.EncodeToString(f[:])}
		if code != "" {
			r.Error = &tc.Error{Code: code}
		}
		record := repository.TerminalRecord{Mutation: tc.Mutation{Version: tc.Version, OperationID: id, Kind: kind}, Result: r}
		if err := db.SaveTerminalRecord(ctx, "operation/"+id, record, true); err != nil {
			t.Fatal(err)
		}
		return f
	}
	running := save("join-running", "join", 1, "publishing", "running", "")
	blocked := save("join-blocked", "join", 2, "bootstrap_capture", "blocked", "SETUP_BLOCKED")
	save("join-done", "join", 3, "ready", "completed", "")
	save("join-stale", "join", 4, "content_pending", "blocked", "STALE_VIEW")
	save("join-expired", "join", 5, "awaiting_approval", "blocked", "EXPIRED_OR_DECLINED_ATTEMPT")
	save("setup-running", "setup", 6, "publishing", "running", "")
	save("join-awaiting", "join", 7, "awaiting_approval", "running", "")
	save("join-membership", "join", 8, "membership_received", "blocked", "SETUP_BLOCKED")
	joining, err := New(db, workspace.New(db, workspace.Options{})).JoiningFolders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(joining) != 2 || !joining[running] || !joining[blocked] {
		t.Fatalf("joining=%v", joining)
	}
}
