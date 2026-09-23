package faults

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

var p04Local = []byte("captured local bytes")
var p04Remote = []byte("verified remote bytes")

func TestP04PublicationKillRestartBoundaries(t *testing.T) {
	for _, hook := range []string{
		repository.HookPublicationPrepared,
		workspace.HookStageFlushed,
		repository.HookPublicationStaged,
		repository.HookPublicationIntent,
		workspace.HookBeforeExchange,
		workspace.HookFilesystemTransition,
		workspace.HookRecoveryNamed,
		workspace.HookPublicationDirFlushed,
		repository.HookPublicationRenamed,
		repository.HookPublicationCommit,
	} {
		t.Run(hook, func(t *testing.T) {
			ctx := context.Background()
			disposable := testkit.NewDisposable(t)
			state := filepath.Join(disposable, "state")
			root := filepath.Join(disposable, "root")
			if err := os.Mkdir(state, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := testkit.ValidateDestructiveTarget(disposable, root); err != nil {
				t.Fatal(err)
			}
			localID, remoteID, err := prepareP04(ctx, state, root)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestP04BoundaryHelper$")
			cmd.Env = append(os.Environ(), "FILESYNC_P04_HELPER=1", "FILESYNC_P04_STATE="+state, "FILESYNC_P04_HOOK="+hook)
			err = cmd.Run()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ProcessState.Success() || exitErr.ExitCode() != -1 {
				t.Fatalf("helper was not SIGKILLed at %s: %v", hook, err)
			}
			db, err := repository.Open(ctx, state)
			if err != nil {
				t.Fatal(err)
			}
			work := workspace.New(db, workspace.Options{})
			if err := work.Recover(ctx, faultFolder); err != nil {
				db.Close()
				t.Fatal(err)
			}
			if err := db.VerifyVersionContent(ctx, localID); err != nil {
				db.Close()
				t.Fatalf("local protected content lost: %v", err)
			}
			if err := db.VerifyVersionContent(ctx, remoteID); err != nil {
				db.Close()
				t.Fatalf("remote protected content lost: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(root, "note.txt"))
			if err != nil {
				db.Close()
				t.Fatal(err)
			}
			if !bytes.Equal(data, p04Local) && !bytes.Equal(data, p04Remote) {
				db.Close()
				t.Fatalf("unexpected published bytes %q", data)
			}
			if bytes.Equal(data, p04Local) {
				if err := work.Apply(ctx, remoteID); err != nil {
					db.Close()
					t.Fatal(err)
				}
			} else {
				applied, err := db.WorkingApplied(ctx, remoteID)
				if err != nil || !applied {
					db.Close()
					t.Fatalf("remote not applied after recovery: applied=%v err=%v", applied, err)
				}
			}
			if err := work.Recover(ctx, faultFolder); err != nil {
				db.Close()
				t.Fatal(err)
			}
			result, err := work.Scan(ctx, faultFolder)
			if err != nil {
				db.Close()
				t.Fatal(err)
			}
			if len(result.Captured) != 0 {
				db.Close()
				t.Fatalf("scan fabricated echo: %+v", result.Captured)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestP04NewFileKillRestartBoundaries(t *testing.T) {
	for _, hook := range []string{repository.HookPublicationPrepared, workspace.HookStageFlushed, repository.HookPublicationStaged, repository.HookPublicationIntent, workspace.HookBeforeExchange, workspace.HookFilesystemTransition, workspace.HookPublicationDirFlushed, repository.HookPublicationRenamed, repository.HookPublicationCommit} {
		t.Run(hook, func(t *testing.T) {
			ctx := context.Background()
			disposable := testkit.NewDisposable(t)
			state, root := filepath.Join(disposable, "state"), filepath.Join(disposable, "root")
			if err := os.Mkdir(state, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := testkit.ValidateDestructiveTarget(disposable, root); err != nil {
				t.Fatal(err)
			}
			id, err := prepareP04NewFile(ctx, state, root, "note.txt")
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestP04BoundaryHelper$")
			cmd.Env = append(os.Environ(), "FILESYNC_P04_HELPER=1", "FILESYNC_P04_STATE="+state, "FILESYNC_P04_HOOK="+hook)
			err = cmd.Run()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != -1 {
				t.Fatalf("helper not SIGKILLed: %v", err)
			}
			db, err := repository.Open(ctx, state)
			if err != nil {
				t.Fatal(err)
			}
			work := workspace.New(db, workspace.Options{})
			if err := work.Recover(ctx, faultFolder); err != nil {
				db.Close()
				t.Fatal(err)
			}
			applied, err := db.WorkingApplied(ctx, id)
			if err != nil {
				db.Close()
				t.Fatal(err)
			}
			if !applied {
				if err := work.Apply(ctx, id); err != nil {
					db.Close()
					t.Fatal(err)
				}
			}
			data, err := os.ReadFile(filepath.Join(root, "note.txt"))
			if err != nil || !bytes.Equal(data, p04Remote) {
				db.Close()
				t.Fatalf("data=%q err=%v", data, err)
			}
			result, err := work.Scan(ctx, faultFolder)
			if err != nil || len(result.Captured) != 0 {
				db.Close()
				t.Fatalf("echo scan=%+v err=%v", result, err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func prepareP04NewFile(ctx context.Context, state, root, path string) (history.VersionID, error) {
	db, err := repository.Open(ctx, state)
	if err != nil {
		return history.VersionID{}, err
	}
	defer db.Close()
	if err := db.EnsureFolder(ctx, faultFolder, faultLocal, 1); err != nil {
		return history.VersionID{}, err
	}
	work := workspace.New(db, workspace.Options{})
	if _, err := work.Register(ctx, faultFolder, root); err != nil {
		return history.VersionID{}, err
	}
	if _, err := work.Scan(ctx, faultFolder); err != nil {
		return history.VersionID{}, err
	}
	manifest, err := db.StoreFile(ctx, bytes.NewReader(p04Remote), false)
	if err != nil {
		return history.VersionID{}, err
	}
	envelope := history.Envelope{ID: history.VersionID{Folder: faultFolder, Author: faultRemote, Counter: 1}, Path: path, Vector: []history.ClockEntry{{Author: faultRemote, Counter: 1}}, Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1}
	if err := db.ImportMetadata(ctx, envelope); err != nil {
		return history.VersionID{}, err
	}
	if err := db.MarkContentReady(ctx, envelope.ID); err != nil {
		return history.VersionID{}, err
	}
	return envelope.ID, nil
}

func TestP04ParentCreationKillDoesNotAuthorScaffold(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	state, root := filepath.Join(disposable, "state"), filepath.Join(disposable, "root")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := testkit.ValidateDestructiveTarget(disposable, root); err != nil {
		t.Fatal(err)
	}
	id, err := prepareP04NewFile(ctx, state, root, "nested/note.txt")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestP04BoundaryHelper$")
	cmd.Env = append(os.Environ(), "FILESYNC_P04_HELPER=1", "FILESYNC_P04_STATE="+state, "FILESYNC_P04_HOOK="+workspace.HookParentCreated)
	err = cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != -1 {
		t.Fatalf("helper not SIGKILLed: %v", err)
	}
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	work := workspace.New(db, workspace.Options{})
	if err := work.Recover(ctx, faultFolder); err != nil {
		t.Fatal(err)
	}
	result, err := work.Scan(ctx, faultFolder)
	if err != nil {
		t.Fatal(err)
	}
	for _, captured := range result.Captured {
		if captured.Path == "nested" {
			t.Fatalf("scaffold authored as explicit directory: %+v", captured)
		}
	}
	if err := work.Apply(ctx, id); err != nil {
		t.Fatal(err)
	}
	result, err = work.Scan(ctx, faultFolder)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Captured) != 0 {
		t.Fatalf("scan after retry authored echo: %+v", result.Captured)
	}
}

func prepareP04(ctx context.Context, state, root string) (history.VersionID, history.VersionID, error) {
	db, err := repository.Open(ctx, state)
	if err != nil {
		return history.VersionID{}, history.VersionID{}, err
	}
	defer db.Close()
	if err := db.EnsureFolder(ctx, faultFolder, faultLocal, 1); err != nil {
		return history.VersionID{}, history.VersionID{}, err
	}
	work := workspace.New(db, workspace.Options{})
	if _, err := work.Register(ctx, faultFolder, root); err != nil {
		return history.VersionID{}, history.VersionID{}, err
	}
	if err := os.WriteFile(filepath.Join(root, "note.txt"), p04Local, 0o600); err != nil {
		return history.VersionID{}, history.VersionID{}, err
	}
	result, err := work.Scan(ctx, faultFolder)
	if err != nil {
		return history.VersionID{}, history.VersionID{}, err
	}
	if len(result.Captured) != 1 {
		return history.VersionID{}, history.VersionID{}, errors.New("local scan did not capture exactly one version")
	}
	manifest, err := db.StoreFile(ctx, bytes.NewReader(p04Remote), false)
	if err != nil {
		return history.VersionID{}, history.VersionID{}, err
	}
	remote := history.Envelope{ID: history.VersionID{Folder: faultFolder, Author: faultRemote, Counter: 1}, Path: "note.txt", Vector: []history.ClockEntry{{Author: faultRemote, Counter: 1}}, Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1}
	if err := db.ImportMetadata(ctx, remote); err != nil {
		return history.VersionID{}, history.VersionID{}, err
	}
	if err := db.MarkContentReady(ctx, remote.ID); err != nil {
		return history.VersionID{}, history.VersionID{}, err
	}
	return result.Captured[0].ID, remote.ID, nil
}

func TestP04BoundaryHelper(t *testing.T) {
	if os.Getenv("FILESYNC_P04_HELPER") != "1" {
		return
	}
	ctx := context.Background()
	hook := os.Getenv("FILESYNC_P04_HOOK")
	db, err := repository.OpenWithOptions(ctx, os.Getenv("FILESYNC_P04_STATE"), repository.Options{FaultHook: func(name string) error {
		if name == hook {
			_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
		}
		return nil
	}})
	if err != nil {
		os.Exit(80)
	}
	work := workspace.New(db, workspace.Options{FaultHook: func(name string) error {
		if name == hook {
			_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
		}
		return nil
	}})
	if err := work.Apply(ctx, history.VersionID{Folder: faultFolder, Author: faultRemote, Counter: 1}); err != nil {
		os.Exit(81)
	}
	os.Exit(82)
}

func TestP04DirectoryTombstoneKillRestart(t *testing.T) {
	for _, kind := range []string{"directory", "tombstone"} {
		for _, hook := range []string{repository.HookPublicationPrepared, repository.HookPublicationIntent, workspace.HookFilesystemTransition, repository.HookPublicationRenamed, repository.HookPublicationCommit} {
			t.Run(kind+"/"+hook, func(t *testing.T) {
				ctx := context.Background()
				disposable := testkit.NewDisposable(t)
				state, root := filepath.Join(disposable, "state"), filepath.Join(disposable, "root")
				if err := os.Mkdir(state, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := testkit.ValidateDestructiveTarget(disposable, root); err != nil {
					t.Fatal(err)
				}
				id, err := prepareP04Structural(ctx, state, root, kind)
				if err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command(os.Args[0], "-test.run=^TestP04StructuralHelper$")
				cmd.Env = append(os.Environ(), "FILESYNC_P04_STRUCT_HELPER=1", "FILESYNC_P04_STATE="+state, "FILESYNC_P04_HOOK="+hook)
				err = cmd.Run()
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != -1 {
					t.Fatalf("helper not SIGKILLed: %v", err)
				}
				db, err := repository.Open(ctx, state)
				if err != nil {
					t.Fatal(err)
				}
				work := workspace.New(db, workspace.Options{})
				if err := work.Recover(ctx, faultFolder); err != nil {
					db.Close()
					t.Fatal(err)
				}
				applied, err := db.WorkingApplied(ctx, id)
				if err != nil {
					db.Close()
					t.Fatal(err)
				}
				if !applied {
					if err := work.Apply(ctx, id); err != nil {
						db.Close()
						t.Fatal(err)
					}
				}
				_, statErr := os.Lstat(filepath.Join(root, "empty"))
				if kind == "directory" && statErr != nil {
					db.Close()
					t.Fatal(statErr)
				}
				if kind == "tombstone" && !errors.Is(statErr, os.ErrNotExist) {
					db.Close()
					t.Fatalf("deleted directory remains: %v", statErr)
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func prepareP04Structural(ctx context.Context, state, root, kind string) (history.VersionID, error) {
	db, err := repository.Open(ctx, state)
	if err != nil {
		return history.VersionID{}, err
	}
	defer db.Close()
	if err := db.EnsureFolder(ctx, faultFolder, faultLocal, 1); err != nil {
		return history.VersionID{}, err
	}
	work := workspace.New(db, workspace.Options{})
	if _, err := work.Register(ctx, faultFolder, root); err != nil {
		return history.VersionID{}, err
	}
	if kind == "tombstone" {
		if err := os.Mkdir(filepath.Join(root, "empty"), 0o700); err != nil {
			return history.VersionID{}, err
		}
	}
	if _, err := work.Scan(ctx, faultFolder); err != nil {
		return history.VersionID{}, err
	}
	versionKind := history.KindDirectory
	if kind == "tombstone" {
		versionKind = history.KindTombstone
	}
	envelope := history.Envelope{ID: history.VersionID{Folder: faultFolder, Author: faultRemote, Counter: 1}, Path: "empty", Vector: []history.ClockEntry{{Author: faultRemote, Counter: 1}}, Kind: versionKind, AuthoredRevision: 1}
	if err := db.ImportMetadata(ctx, envelope); err != nil {
		return history.VersionID{}, err
	}
	return envelope.ID, nil
}

func TestP04StructuralHelper(t *testing.T) {
	if os.Getenv("FILESYNC_P04_STRUCT_HELPER") != "1" {
		return
	}
	ctx := context.Background()
	hook := os.Getenv("FILESYNC_P04_HOOK")
	db, err := repository.OpenWithOptions(ctx, os.Getenv("FILESYNC_P04_STATE"), repository.Options{FaultHook: func(name string) error {
		if name == hook {
			_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
		}
		return nil
	}})
	if err != nil {
		os.Exit(80)
	}
	work := workspace.New(db, workspace.Options{FaultHook: func(name string) error {
		if name == hook {
			_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
		}
		return nil
	}})
	if err := work.Apply(ctx, history.VersionID{Folder: faultFolder, Author: faultRemote, Counter: 1}); err != nil {
		os.Exit(81)
	}
	os.Exit(82)
}
