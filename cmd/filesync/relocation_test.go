package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/calebhabesh/file-sync/internal/app"
	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/state"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

func TestRelocationCLIStoppedAndLive(t *testing.T) {
	ctx := context.Background()
	stateDir := testkit.NewDisposable(t)
	parent := testkit.NewDisposable(t)
	cfg, err := app.Initialize(ctx, stateDir, app.SystemDependencies())
	if err != nil {
		t.Fatal(err)
	}
	local, _ := parseID(cfg.DeviceID)
	folder := history.ID{77}
	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	w := workspace.New(db, workspace.Options{})
	ctrl := control.New(db, w, control.Options{LocalDevice: local})
	if _, err := ctrl.RegisterFolder(ctx, folder, root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "note"), []byte("preserved"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}
	db.Close()
	execute := func(from, to string) {
		var stdout, stderr bytes.Buffer
		err := run([]string{"orbit", "folders", "relocate", "--state", stateDir, "--folder", hex.EncodeToString(folder[:]), "--from", from, "--to", to, "--json"}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("CLI: %v %s", err, stderr.String())
		}
		var result workspace.RelocationResult
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Path != to {
			t.Fatalf("result=%s %v", stdout.String(), err)
		}
		data, err := os.ReadFile(filepath.Join(to, "note"))
		if err != nil || string(data) != "preserved" {
			t.Fatalf("bytes=%q %v", data, err)
		}
	}
	first := filepath.Join(parent, "first")
	execute(root, first)
	lock, err := state.Acquire(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	db, err = repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctrl = control.New(db, workspace.New(db, workspace.Options{}))
	srv, err := control.NewServer(ctrl, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()
	if err := os.WriteFile(filepath.Join(stateDir, "control.addr"), []byte(httpSrv.URL), 0o600); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(parent, "second")
	execute(first, second)
	reg, err := db.Root(ctx, folder)
	if err != nil || reg.Path != second {
		t.Fatalf("root=%+v %v", reg, err)
	}
}
