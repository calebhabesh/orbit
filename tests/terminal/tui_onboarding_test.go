package terminal_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
)

func TestTerminalT10BlockedSetupPagesAndFolderManagement(t *testing.T) {
	f := fresh(t)
	ctx := context.Background()
	for i := 0; i < 43; i++ {
		id := fmt.Sprintf("%064x", i+1)
		r := tc.Result{Operation: &tc.Operation{ID: id, Kind: "join", State: "blocked", Phase: "awaiting_approval"}, Join: &tc.JoinRecord{Root: "/synthetic/root"}}
		record := repository.TerminalRecord{Mutation: tc.Mutation{OperationID: id, Kind: "join"}, Result: r}
		if err := f.db.SaveTerminalRecord(ctx, "operation/"+id, record, true); err != nil {
			t.Fatal(err)
		}
	}
	// Test the owning bounded repository page without manufacturing recoverable
	// setup jobs; actual resume jobs are supplied by the process acceptance test.
	q := tc.Query{Version: tc.Version, Kind: "setups", Limit: 7}
	seen := map[string]bool{}
	for {
		items, cursor, err := f.db.PendingSetupPage(ctx, q)
		if err != nil || len(items) > 7 {
			t.Fatal("bounded setup query", err)
		}
		for _, it := range items {
			if seen[it.ID] {
				t.Fatal("repeated setup")
			}
			seen[it.ID] = true
		}
		if cursor == "" {
			break
		}
		q.Cursor = cursor
	}
	if len(seen) != 43 {
		t.Fatal("blocked setup omitted")
	}
	if err := (tc.Query{Version: tc.Version, Kind: "setups", Folder: strings.Repeat("a", 64)}).Validate(); err == nil {
		t.Fatal("setup cursor/query accepted another selector")
	}
	q.Cursor = "invalid"
	if _, _, err := f.db.PendingSetupPage(ctx, q); err == nil {
		t.Fatal("invalid cursor accepted")
	}
	if err := (tc.Query{Version: tc.Version, Kind: "folder_management"}).Validate(); err == nil {
		t.Fatal("unscoped management accepted")
	}
}
func TestTerminalT10RealPTYOnboarding(t *testing.T) {
	base := testkit.NewDisposable(t)
	binary := filepath.Join(base, "orbit")
	build := exec.Command("go", "build", "-o", binary, "./cmd/orbit")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", "../../scripts/terminal_onboarding_pty_test.py", "--binary", binary)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("PTY onboarding: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "two-process-onboarding") {
		t.Fatal("no scenarios executed")
	}
	t.Log(string(out))
}

func TestTerminalT10FolderForkLagAndConservativePreview(t *testing.T) {
	f := fresh(t)
	ctx := context.Background()
	root := filepath.Join(f.root, "Notes")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("protected folder-management bytes")
	os.WriteFile(filepath.Join(root, "keep"), original, 0600)
	plan, _ := setupReview(t, f, root, "setup")
	created, err := f.ctrl.TerminalMutate(ctx, tc.Mutation{Version: tc.Version, Kind: "setup", OperationID: enrollmentRandom(t), Setup: &plan})
	if err != nil {
		t.Fatal(err)
	}
	folder := mustID(t, created.Join.Folder)
	client := terminalClient(t, f)
	current, approved, err := f.db.GetMembership(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	extra := history.ID{77}
	next := current
	next.Revision++
	next.PriorDigest = approved.Digest
	next.Active = append(next.Active, protocol.ActiveMember{Device: extra, KeyPin: history.Digest{88}})
	if _, err = f.db.ApproveMembership(ctx, next); err != nil {
		t.Fatal(err)
	}
	query := tc.Query{Version: tc.Version, Kind: "folder_management", Folder: created.Join.Folder}
	res, err := client.Query(ctx, query)
	if err != nil || res.FolderManagement.Revision != 2 || len(res.FolderManagement.Members) != 2 {
		t.Fatal("membership details", err)
	}
	// A peer still at revision 1 cannot be inferred current from local membership.
	res.Observations = nil
	if err = client.ManageFolder(ctx, created.Join.Folder, "pause", "", ""); err != nil {
		t.Fatal(err)
	}
	res, err = client.Query(ctx, query)
	if err != nil || !res.FolderManagement.Paused || res.Readiness.Ready() {
		t.Fatal("pause readiness", err)
	}
	if err = client.ManageFolder(ctx, created.Join.Folder, "resume", "", ""); err != nil {
		t.Fatal(err)
	}
	preview, err := client.RetirementPreview(ctx, created.Join.Folder, hex.EncodeToString(extra[:]))
	if err != nil || preview.CurrentRevision != 2 || preview.NextRevision != 3 || preview.Disclaimer == "" {
		t.Fatal("conservative retirement preview", err)
	}
	still, _, _ := f.db.GetMembership(ctx, folder)
	if still.Revision != 2 {
		t.Fatal("preview retired a device")
	}
	fork := next
	fork.Active = append(append([]protocol.ActiveMember(nil), next.Active...), protocol.ActiveMember{Device: history.ID{99}, KeyPin: history.Digest{100}})
	if _, err = f.db.ApproveMembership(ctx, fork); err == nil {
		t.Fatal("fork accepted")
	}
	if err = f.ws.Pause(ctx, folder, "MEMBERSHIP_FORK"); err != nil {
		t.Fatal(err)
	}
	res, err = client.Query(ctx, query)
	if err != nil || res.Readiness.MembershipCurrent || res.Readiness.Ready() {
		t.Fatal("fork hidden", err)
	}
	hasFork := false
	for _, a := range res.Attention {
		if a.Code == "MEMBERSHIP_FORK" {
			hasFork = true
		}
	}
	if !hasFork {
		t.Fatal("fork attention absent")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "keep")); !bytes.Equal(b, original) {
		t.Fatal("management changed bytes")
	}
}

func TestTerminalT10SetupRetryRetainsNetworkRestartAndExactIntent(t *testing.T) {
	f := fresh(t)
	ctx := context.Background()
	root := filepath.Join(f.root, "network-review")
	os.Mkdir(root, 0700)
	os.WriteFile(filepath.Join(root, "keep"), []byte("before restart"), 0600)
	s, err := config.LoadRuntimeSettings(f.state)
	if err != nil {
		t.Fatal(err)
	}
	s.PeerListen = "127.0.0.1:0"
	s.EnrollmentListen = "127.0.0.1:0"
	plan := tc.SetupIntent{DeviceName: "Laptop", FolderName: "Notes", Root: root, Settings: s}
	r, err := f.ctrl.TerminalQuery(ctx, tc.Query{Version: tc.Version, Kind: "root_preview", Name: "adopt", Path: root, RootPlan: &plan})
	if err != nil {
		t.Fatal(err)
	}
	plan.Preview = *r.Review
	m := tc.Mutation{Version: tc.Version, Kind: "adopt", OperationID: enrollmentRandom(t), Setup: &plan}
	// Deliberately preexisting pretty-printed CLI request must be equivalent.
	b, _ := json.MarshalIndent(m, "", "  ")
	os.WriteFile(filepath.Join(f.state, "setup-request-"+m.OperationID+".json"), b, 0600)
	client := terminalClient(t, f)
	calls := 0
	client.RestartDaemon = func(context.Context) error {
		calls++
		if calls == 1 {
			return errors.New("simulated lifecycle interruption")
		}
		return nil
	}
	first, err := client.Setup(ctx, m)
	if err == nil || first.Operation == nil {
		t.Fatal("missing interrupted admitted operation")
	}
	retry, err := client.Setup(ctx, m)
	if err != nil || retry.Operation.ID != first.Operation.ID || retry.Join.Folder != first.Join.Folder || calls != 2 {
		t.Fatal("lost restart intent on exact retry", err)
	}
	_, err = client.Setup(ctx, m)
	if err != nil || calls != 2 {
		t.Fatal("completed restart was repeated", err)
	}
	changed := m
	p := plan
	p.DeviceName = "Different"
	changed.Setup = &p
	if _, err = client.Setup(ctx, changed); err == nil || !strings.Contains(err.Error(), "IDEMPOTENCY_CONFLICT") {
		t.Fatal("private retry intent overwritten")
	}
	for _, name := range []string{"setup-request-" + m.OperationID + ".json", "setup-network-" + m.OperationID + ".json"} {
		st, err := os.Stat(filepath.Join(f.state, name))
		if err != nil || st.Mode().Perm()&0077 != 0 {
			t.Fatal("nonprivate recovery record")
		}
	}
}
