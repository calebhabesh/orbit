package terminal_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/testkit"
)

func TestTerminalT09NamedPages(t *testing.T) {
	f := fresh(t)
	ctx := context.Background()
	for i := 0; i < 45; i++ {
		f.folder(fmt.Sprintf("folder-%02d", i))
		if err := f.db.SetDeviceDisplayName(ctx, history.ID{byte(i + 1)}, fmt.Sprintf("device-%02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"folders", "devices"} {
		q := tc.Query{Version: tc.Version, Kind: kind, Limit: 7}
		seen := map[string]bool{}
		for {
			r, err := f.ctrl.TerminalQuery(ctx, q)
			if err != nil {
				t.Fatal(err)
			}
			if len(r.Items) > 7 || len(r.Items) == 0 {
				t.Fatal("invalid page")
			}
			for _, it := range r.Items {
				if seen[it.ID] {
					t.Fatal("repeated item")
				}
				seen[it.ID] = true
			}
			if r.Cursor == "" {
				break
			}
			wrong := q
			wrong.Cursor = r.Cursor
			wrong.Name = "another scope"
			if _, err := f.ctrl.TerminalQuery(ctx, wrong); err == nil {
				t.Fatal("cursor not scope bound")
			}
			q.Cursor = r.Cursor
		}
		if len(seen) < 45 {
			t.Fatal("page omitted items")
		}
	}
	q := tc.Query{Version: tc.Version, Kind: "folders", Limit: 7}
	r, err := f.ctrl.TerminalQuery(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	q.Folder = r.Items[0].ID
	r, err = f.ctrl.TerminalQuery(ctx, q)
	if err != nil || len(r.Items) != 1 {
		t.Fatal("selected folder query escaped context")
	}
	_, err = hex.DecodeString(r.Items[0].ID)
	if err != nil {
		t.Fatal("invalid item identity")
	}
}

func TestTerminalT09RealPTYLifetime(t *testing.T) {
	base := testkit.NewDisposable(t)
	binary := filepath.Join(base, "orbit")
	build := exec.Command("go", "build", "-o", binary, "./cmd/orbit")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", "../../scripts/terminal_pty_test.py", "--binary", binary)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PTY campaign: %v\n%s", err, out)
	} else {
		t.Log(string(out))
	}
}
