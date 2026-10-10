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

	"github.com/calebhabesh/orbit/internal/app"
	"github.com/calebhabesh/orbit/internal/control"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/state"
	"github.com/calebhabesh/orbit/internal/testkit"
	"github.com/calebhabesh/orbit/internal/workspace"
)

func TestOnboardingE13ParticipationCLIStoppedAndLive(t *testing.T) {
	for _, live := range []bool{false, true} {
		t.Run(map[bool]string{false: "stopped", true: "live"}[live], func(t *testing.T) {
			ctx := context.Background()
			dir, parent := testkit.NewDisposable(t), testkit.NewDisposable(t)
			if err := os.Chmod(parent, 0700); err != nil {
				t.Fatal(err)
			}
			cfg, err := app.Initialize(ctx, dir, app.SystemDependencies())
			if err != nil {
				t.Fatal(err)
			}
			local, _ := parseID(cfg.DeviceID)
			folder, peer := history.ID{71}, history.ID{72}
			db, err := repository.Open(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(parent, "kept")
			if err = os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			ctrl := control.New(db, workspace.New(db, workspace.Options{}), control.Options{LocalDevice: local})
			if _, err = ctrl.RegisterFolder(ctx, folder, root); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(root, "note"), []byte("keep me"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = db.ApproveMembership(ctx, protocol.Membership{Folder: folder, Revision: 1, Active: []protocol.ActiveMember{{Device: local, KeyPin: history.Digest{1}}, {Device: peer, KeyPin: history.Digest{2}}}}); err != nil {
				t.Fatal(err)
			}
			if err = db.RenameDevice(ctx, peer, "Laptop", local); err != nil {
				t.Fatal(err)
			}
			if live {
				lock, err := state.Acquire(dir)
				if err != nil {
					t.Fatal(err)
				}
				defer lock.Close()
				defer db.Close()
				srv, err := control.NewServer(ctrl, dir)
				if err != nil {
					t.Fatal(err)
				}
				httpSrv := httptest.NewServer(srv.Handler())
				defer httpSrv.Close()
				if err = os.WriteFile(filepath.Join(dir, "control.addr"), []byte(httpSrv.URL), 0600); err != nil {
					t.Fatal(err)
				}
			} else if err = db.Close(); err != nil {
				t.Fatal(err)
			}
			invoke := func(args ...string) (string, error) {
				var out, stderr bytes.Buffer
				err := handleOrbit(append(args, "--state", dir, "--json"), &out, &stderr)
				return out.String(), err
			}
			id := hex.EncodeToString(folder[:])
			public := filepath.Join(parent, "public")
			if err := os.Mkdir(public, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(public, 0755); err != nil {
				t.Fatal(err)
			}
			for _, unsafe := range []string{filepath.Join(public, "review.json"), filepath.Join(root, "review.json")} {
				if out, err := invoke("remove-device", "Laptop", "--folder", id, "--review-file", unsafe); err == nil {
					t.Fatal("unsafe review location accepted", unsafe, out)
				}
				if _, err := os.Stat(unsafe); !os.IsNotExist(err) {
					t.Fatal("unsafe review file was written", unsafe, err)
				}
			}
			t.Chdir(parent)
			file := filepath.Join(parent, "review.json")
			if out, err := invoke("remove-device", "Laptop", "--folder", id, "--review-file", "review.json"); err != nil {
				t.Fatal(out, err)
			}
			info, err := os.Stat(file)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("review must be private", info, err)
			}
			original, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if out, err := invoke("remove-device", "Laptop", "--folder", id, "--review-file", "review.json"); err == nil {
				t.Fatal("review collision overwritten", out)
			}
			if current, err := os.ReadFile(file); err != nil || !bytes.Equal(current, original) {
				t.Fatal("collision changed exact review", err)
			}
			if out, err := invoke("remove-device", "--request-file", file, "--confirm-name", "wrong"); err == nil {
				t.Fatal("wrong name removed device", out)
			}
			for i := 0; i < 2; i++ {
				out, err := invoke("remove-device", "--request-file", "review.json", "--confirm-name", "Laptop")
				var result control.RemoveDeviceResult
				if err != nil || json.Unmarshal([]byte(out), &result) != nil || result.State != "completed" {
					t.Fatal(out, err)
				}
			}
			out, err := invoke("leave", "--folder", id)
			var preview map[string]any
			if err != nil || json.Unmarshal([]byte(out), &preview) != nil || preview["state"] != "preview" {
				t.Fatal(out, err)
			}
			if out, err = invoke("leave", "--folder", id, "--yes"); err != nil {
				t.Fatal(out, err)
			}
			data, err := os.ReadFile(filepath.Join(root, "note"))
			if err != nil || string(data) != "keep me" {
				t.Fatal("Leave changed bytes", string(data), err)
			}
		})
	}
}
