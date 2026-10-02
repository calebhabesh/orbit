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

func TestOrbitBrowse_CLIStoppedAndLiveParity(t *testing.T) {
	ctx := context.Background()
	stateDir := testkit.NewDisposable(t)
	root := testkit.NewDisposable(t)
	cfg, err := app.Initialize(ctx, stateDir, app.SystemDependencies())
	if err != nil {
		t.Fatal(err)
	}
	local, err := parseID(cfg.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	folder := history.ID{42}
	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	ctrl := control.New(db, workspace.New(db, workspace.Options{}), control.Options{LocalDevice: local})
	if _, err := ctrl.RegisterFolder(ctx, folder, root); err != nil {
		t.Fatal(err)
	}
	manifest, err := db.StoreFile(ctx, bytes.NewReader([]byte("notes")), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{Folder: folder, Path: "docs/notes.txt", Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	commands := [][]string{{"browse", "--path", "docs"}, {"search", "--query", "notes"}, {"details", "--path", "docs/notes.txt"}}
	execute := func(command []string) []byte {
		args := append([]string{"orbit"}, command...)
		args = append(args, "--state", stateDir, "--folder", hex.EncodeToString(folder[:]))
		var stdout, stderr bytes.Buffer
		if err := run(args, &stdout, &stderr); err != nil {
			t.Fatalf("%v: %v %s", command, err, stderr.String())
		}
		if !json.Valid(stdout.Bytes()) {
			t.Fatalf("invalid output: %s", stdout.String())
		}
		return append([]byte{}, stdout.Bytes()...)
	}
	stopped := make([][]byte, len(commands))
	for i, command := range commands {
		stopped[i] = execute(command)
	}
	// The actual state lock and HTTP server exercise live-daemon fallback.
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
	ctrl = control.New(db, workspace.New(db, workspace.Options{}), control.Options{LocalDevice: local})
	srv, err := control.NewServer(ctrl, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()
	if err := os.WriteFile(filepath.Join(stateDir, "control.addr"), []byte(httpSrv.URL), 0600); err != nil {
		t.Fatal(err)
	}
	for i, command := range commands {
		live := execute(command)
		if !bytes.Equal(live, stopped[i]) {
			t.Fatalf("live/stopped differ: %s vs %s", live, stopped[i])
		}
	}
}
