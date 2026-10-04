package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/calebhabesh/file-sync/internal/app"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTerminalT02RuntimeCLIReviewedReplay(t *testing.T) {
	root := testkit.NewDisposable(t)
	dir := filepath.Join(root, "state")
	if _, err := app.Initialize(context.Background(), dir, app.SystemDependencies()); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := handleOrbit([]string{"settings", "runtime", "--state", dir}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var reviewed tc.Result
	if err := json.Unmarshal(stdout.Bytes(), &reviewed); err != nil {
		t.Fatal(err)
	}
	s := *reviewed.Settings
	s.PeerListen = "127.0.0.1:0"
	s.DataBudget += 4096
	m := tc.Mutation{Version: tc.Version, Kind: "settings", OperationID: strings.Repeat("a", 64), Settings: &tc.SettingsIntent{Review: *reviewed.Review, Settings: s}}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	request := filepath.Join(root, "request.json")
	if err := os.WriteFile(request, b, 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		stdout.Reset()
		if err := handleOrbit([]string{"settings", "runtime", "--state", dir, "--request-file", request}, &stdout, &stderr); err != nil {
			t.Fatal(err)
		}
		var r tc.Result
		if err := json.Unmarshal(stdout.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		if r.Operation.State != "completed" || len(r.Operation.CommittedEffects) != 1 {
			t.Fatal("CLI retry changed effect")
		}
	}
	stdout.Reset()
	if err := handleOrbit([]string{"settings", "runtime", "--state", dir, "--operation", m.OperationID}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var inspected tc.Result
	if err := json.Unmarshal(stdout.Bytes(), &inspected); err != nil {
		t.Fatal(err)
	}
	if inspected.Operation.ID != m.OperationID {
		t.Fatal("operation inspection lost identity")
	}
}
