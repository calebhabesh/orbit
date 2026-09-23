package faults

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
)

func faultID(label byte) history.ID {
	var id history.ID
	for i := range id {
		id[i] = label
	}
	return id
}

var faultFolder, faultLocal, faultRemote = faultID('F'), faultID('A'), faultID('B')
var faultBytes = []byte("durable boundary content")

func TestP03KillRestartBoundaries(t *testing.T) {
	for _, test := range []struct{ hook, scenario string }{
		{repository.HookObjectFlushed, "object"}, {repository.HookObjectInstalled, "object"},
		{repository.HookObjectRecorded, "object"},
		{repository.HookBeforeVersionCommit, "version"}, {repository.HookAfterVersionCommit, "version"},
		{repository.HookBeforeReadyCommit, "ready"}, {repository.HookAfterReadyCommit, "ready"},
	} {
		t.Run(test.hook, func(t *testing.T) {
			root := testkit.NewDisposable(t)
			state := filepath.Join(root, "state")
			if err := os.Mkdir(state, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := testkit.ValidateDestructiveTarget(root, state); err != nil {
				t.Fatal(err)
			}
			if err := prepareBoundaryState(state, test.scenario); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestP03BoundaryHelper$")
			cmd.Env = append(os.Environ(), "FILESYNC_P03_HELPER=1", "FILESYNC_P03_STATE="+state, "FILESYNC_P03_HOOK="+test.hook, "FILESYNC_P03_SCENARIO="+test.scenario)
			err := cmd.Run()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ProcessState.Success() {
				t.Fatalf("helper was not killed at %s: %v", test.hook, err)
			}
			db, err := repository.Open(context.Background(), state)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			switch test.scenario {
			case "object":
				orphans, err := db.OrphanObjects(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				want := 0
				if test.hook == repository.HookObjectInstalled || test.hook == repository.HookObjectRecorded {
					want = 1
				}
				if len(orphans) != want {
					t.Fatalf("orphans after restart=%d, want %d", len(orphans), want)
				}
			case "version":
				id := history.VersionID{Folder: faultFolder, Author: faultLocal, Counter: 1}
				known, err := db.MetadataKnown(context.Background(), id)
				if err != nil {
					t.Fatal(err)
				}
				want := test.hook == repository.HookAfterVersionCommit
				if known != want {
					t.Fatalf("metadata known=%v, want %v", known, want)
				}
				if known {
					if err := db.VerifyVersionContent(context.Background(), id); err != nil {
						t.Fatalf("committed protected content failed verification: %v", err)
					}
				}
				if !known {
					manifest, err := db.StoreFile(context.Background(), bytes.NewReader(faultBytes), false)
					if err != nil {
						t.Fatal(err)
					}
					created, err := db.CreateLocalVersion(context.Background(), repository.LocalVersionRequest{Folder: faultFolder, Path: "fault.txt", Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1})
					if err != nil {
						t.Fatal(err)
					}
					if created.ID.Counter != 1 {
						t.Fatalf("rolled-back counter advanced to %d", created.ID.Counter)
					}
				}
			case "ready":
				id := history.VersionID{Folder: faultFolder, Author: faultRemote, Counter: 1}
				ready, err := db.ContentReady(context.Background(), id)
				if err != nil {
					t.Fatal(err)
				}
				want := test.hook == repository.HookAfterReadyCommit
				if ready != want {
					t.Fatalf("ready after restart=%v, want %v", ready, want)
				}
				if ready {
					if err := db.VerifyVersionContent(context.Background(), id); err != nil {
						t.Fatalf("ready protected content failed verification: %v", err)
					}
				}
			}
		})
	}
}

func prepareBoundaryState(state, scenario string) error {
	if scenario == "object" {
		return nil
	}
	ctx := context.Background()
	db, err := repository.Open(ctx, state)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.EnsureFolder(ctx, faultFolder, faultLocal, 1); err != nil {
		return err
	}
	manifest, err := db.StoreFile(ctx, bytes.NewReader(faultBytes), false)
	if err != nil {
		return err
	}
	if scenario == "ready" {
		envelope := history.Envelope{ID: history.VersionID{Folder: faultFolder, Author: faultRemote, Counter: 1}, Path: "fault.txt", Vector: []history.ClockEntry{{Author: faultRemote, Counter: 1}}, Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1}
		return db.ImportMetadata(ctx, envelope)
	}
	return nil
}

func TestP03BoundaryHelper(t *testing.T) {
	if os.Getenv("FILESYNC_P03_HELPER") != "1" {
		return
	}
	ctx := context.Background()
	hook := os.Getenv("FILESYNC_P03_HOOK")
	db, err := repository.OpenWithOptions(ctx, os.Getenv("FILESYNC_P03_STATE"), repository.Options{FaultHook: func(name string) error {
		if name == hook {
			_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
		}
		return nil
	}})
	if err != nil {
		os.Exit(80)
	}
	switch os.Getenv("FILESYNC_P03_SCENARIO") {
	case "object":
		digest := sha256.Sum256(faultBytes)
		err = db.InstallChunk(ctx, digest, uint64(len(faultBytes)), bytes.NewReader(faultBytes))
	case "version":
		manifest, storeErr := db.StoreFile(ctx, bytes.NewReader(faultBytes), false)
		if storeErr != nil {
			err = storeErr
		} else {
			_, err = db.CreateLocalVersion(ctx, repository.LocalVersionRequest{Folder: faultFolder, Path: "fault.txt", Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1})
		}
	case "ready":
		err = db.MarkContentReady(ctx, history.VersionID{Folder: faultFolder, Author: faultRemote, Counter: 1})
	default:
		err = errors.New("unknown scenario")
	}
	if err != nil {
		os.Exit(81)
	}
	os.Exit(82)
}
