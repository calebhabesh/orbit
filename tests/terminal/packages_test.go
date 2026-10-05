package terminal_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/calebhabesh/file-sync/internal/control"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/controlclient"
	"github.com/calebhabesh/file-sync/internal/testkit"
)

func TestTerminalT12BareEntryAndLegacyDiscovery(t *testing.T) {
	base := testkit.NewDisposable(t)
	binary := buildOrbitBinary(t, base)
	home := filepath.Join(base, "home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".filesync")
	run := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Env = append(os.Environ(), "HOME="+home, "XDG_STATE_HOME="+filepath.Join(home, ".local/state"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v %s", args, err, out)
		}
		return out
	}
	run("init", "--state", dir)
	before := mustRead(t, filepath.Join(dir, "config.json"))
	for _, args := range [][]string{nil, {"--json"}, {"tui", "--json"}, {"status", "--json"}} {
		out := run(args...)
		if bytes.Contains(out, []byte("\x1b")) || bytes.Contains(out, []byte("bootstrap")) {
			t.Fatalf("pipe opened UI: %s", out)
		}
		if len(args) > 0 && !bytes.Contains(out, []byte(dir)) {
			t.Fatalf("legacy state not discovered: %s", out)
		}
	}
	if !bytes.Equal(before, mustRead(t, filepath.Join(dir, "config.json"))) {
		t.Fatal("entry changed identity")
	}
	if _, err := os.Stat(filepath.Join(home, ".local/state/filesync/config.json")); !os.IsNotExist(err) {
		t.Fatal("created competing default state")
	}
}

func TestTerminalT12LegacySettingsExactlyOnce(t *testing.T) {
	f := fresh(t)
	folder := f.folder("preserved")
	if err := os.WriteFile(filepath.Join(f.root, "preserved", "note"), []byte("legacy captured bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	scan, err := f.ws.Scan(context.Background(), folder)
	if err != nil || len(scan.Captured) != 1 {
		t.Fatal("capture", err)
	}
	f.close()
	before := mustRead(t, filepath.Join(f.state, "config.json"))
	if err := testkit.ValidateDestructiveTarget(f.root, f.state); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.state, "limits.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.state, "runtime.json")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	c := &controlclient.Client{StateDir: f.state}
	r := querySettings(t, c)
	if r.Error == nil || r.Error.Code != "LIMITS_REVIEW_REQUIRED" {
		t.Fatal("missing deliberate adoption", r)
	}
	m := tc.Mutation{Version: tc.Version, Kind: "settings", OperationID: strings.Repeat("c", 64), Settings: &tc.SettingsIntent{Review: *r.Review, Settings: config.DefaultRuntimeSettings()}}
	first, err := c.Mutate(context.Background(), m)
	if err != nil || first.Operation.State != "completed" {
		t.Fatal("adopt", err)
	}
	replay, err := c.Mutate(context.Background(), m)
	if err != nil || replay.Operation.ID != first.Operation.ID || replay.Operation.Fingerprint != first.Operation.Fingerprint {
		t.Fatal("adoption replay changed", err)
	}
	if !bytes.Equal(before, mustRead(t, filepath.Join(f.state, "config.json"))) {
		t.Fatal("identity changed")
	}
	f.open()
	heads, err := f.db.Heads(context.Background(), folder, "note")
	if err != nil || len(heads) != 1 || heads[0].ID != scan.Captured[0].ID {
		t.Fatal("history changed", err)
	}
	if string(mustRead(t, filepath.Join(f.root, "preserved", "note"))) != "legacy captured bytes" {
		t.Fatal("root changed")
	}
	if err := os.WriteFile(filepath.Join(f.root, "preserved", "note"), []byte("new edit after adoption"), 0600); err != nil {
		t.Fatal(err)
	}
	next, err := f.ws.Scan(context.Background(), folder)
	if err != nil || len(next.Captured) != 1 || next.Captured[0].ID.Author != scan.Captured[0].ID.Author || next.Captured[0].ID.Counter <= scan.Captured[0].ID.Counter {
		t.Fatal("adoption reused author counter", err)
	}

}

func TestTerminalT12BareRealPTY(t *testing.T) {
	base := testkit.NewDisposable(t)
	binary := buildOrbitBinary(t, base)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", "../../scripts/terminal_pty_test.py", "--binary", binary, "--bare")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bare PTY: %v %s", err, out)
	} else {
		t.Log(string(out))
	}
}

// An older live daemon retains exclusive ownership; failed negotiation never
// falls back to directly opening its state or submits an unsupported mutation.
func TestTerminalT12MixedCapabilitiesRefuseBeforeAction(t *testing.T) {
	for _, test := range []struct {
		name, kind, code string
		version          string
		caps             []string
	}{
		{"old-control", "status", "UNSUPPORTED_CAPABILITY", "1", nil},
		{"new-control", "status", "INCOMPATIBLE_VERSION", "2", []string{tc.Capability}},
		{"old-content", "conflicts", "UNSUPPORTED_CAPABILITY", "1", []string{tc.Capability}},
		{"old-management", "setups", "UNSUPPORTED_CAPABILITY", "1", []string{tc.Capability}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := fresh(t)
			f.server.Close()
			f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Orbit-Device", hex.EncodeToString(f.device[:]))
				if r.Header.Get("Authorization") != "Bearer "+f.token {
					t.Error("missing owner credential")
				}
				var q tc.Query
				if err := json.NewDecoder(r.Body).Decode(&q); err != nil || q.Kind != "capabilities" {
					t.Error("action dispatched before capability refusal")
				}
				_ = json.NewEncoder(w).Encode(tc.Result{Version: test.version, Capabilities: test.caps})
			}))
			c := terminalClient(t, f)
			q := tc.Query{Version: tc.Version, Kind: test.kind}
			if test.kind == "conflicts" {
				q.Folder = strings.Repeat("1", 64)
				q.Path = "note"
			}
			_, err := c.Query(context.Background(), q)
			var failure *control.ControlError
			if !errors.As(err, &failure) || failure.Code != test.code || failure.Action == "" {
				t.Fatalf("incorrect next action: %v", err)
			}
		})
	}
}
