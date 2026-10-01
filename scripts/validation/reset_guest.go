//go:build ignore

// This executable is a disposable VM's init, never a release fault endpoint.
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/workspace"
	"golang.org/x/sys/unix"
)

var ctx = context.Background()
var folder, author, remote = history.ID{1}, history.ID{2}, history.ID{3}
var original = []byte("captured before abrupt reset\n")
var successor = []byte("verified successor after abrupt reset\n")

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func barrierFile(path string, data []byte) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	must(err)
	_, err = f.Write(data)
	must(err)
	must(f.Sync())
	must(f.Close())
	d, err := os.Open("/disk")
	must(err)
	must(d.Sync())
	must(d.Close())
}

func stopAt(name string) error {
	// Existing file, same-size overwrite, deliberately NO flush. This is a
	// negative control for loss of guest dirty caches, not protected user data.
	must(os.WriteFile("/disk/volatile", bytes.Repeat([]byte("N"), 4096), 0600))
	fmt.Println("FILESYNC_RESET_READY " + name)
	for {
		time.Sleep(time.Hour)
	}
}

func main() {
	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("FILESYNC_RESET_FAIL %v\n", err)
			for {
				time.Sleep(time.Hour)
			}
		}
	}()
	for _, p := range []string{"/proc", "/sys", "/dev", "/disk"} {
		must(os.MkdirAll(p, 0755))
	}
	must(unix.Mount("proc", "/proc", "proc", 0, ""))
	must(unix.Mount("sysfs", "/sys", "sysfs", 0, ""))
	must(unix.Mount("devtmpfs", "/dev", "devtmpfs", 0, ""))
	for i := 0; i < 100; i++ {
		if _, err := os.Stat("/dev/vda"); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	must(unix.Mount("/dev/vda", "/disk", "ext4", 0, ""))
	marker, err := os.ReadFile("/disk/.filesync-disposable")
	must(err)
	if string(marker) != "filesync disposable reset VM\n" {
		panic("unmarked VM disk")
	}
	cmdline, err := os.ReadFile("/proc/cmdline")
	must(err)
	mode, hook := "", ""
	for _, field := range strings.Fields(string(cmdline)) {
		if strings.HasPrefix(field, "filesync.mode=") {
			mode = strings.TrimPrefix(field, "filesync.mode=")
		}
		if strings.HasPrefix(field, "filesync.hook=") {
			hook = strings.TrimPrefix(field, "filesync.hook=")
		}
	}
	publication := strings.HasPrefix(hook, "publication.")
	options := repository.Options{}
	if mode == "mutate" {
		options.FaultHook = func(name string) error {
			if name == hook {
				return stopAt(name)
			}
			return nil
		}
	}
	db, err := repository.OpenWithOptions(ctx, "/disk/state", options)
	must(err)
	ws := workspace.New(db, workspace.Options{FaultHook: func(name string) error {
		if mode == "mutate" && name == hook {
			return stopAt(name)
		}
		return nil
	}})
	baseID := history.VersionID{Folder: folder, Author: author, Counter: 1}
	nextID := history.VersionID{Folder: folder, Author: remote, Counter: 1}
	switch mode {
	case "setup":
		must(db.EnsureFolder(ctx, folder, author, 1))
		must(os.MkdirAll("/disk/root", 0700))
		_, err := ws.Register(ctx, folder, "/disk/root")
		must(err)
		barrierFile("/disk/root/note.txt", original)
		report, err := ws.Scan(ctx, folder)
		must(err)
		if len(report.Captured) != 1 {
			panic("baseline was not captured")
		}
		if publication {
			manifest, err := db.StoreFile(ctx, bytes.NewReader(successor), false)
			must(err)
			envelope := history.Envelope{ID: nextID, Path: "note.txt", Parents: []history.VersionID{baseID}, Vector: []history.ClockEntry{{Author: author, Counter: 1}, {Author: remote, Counter: 1}}, Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1}
			must(db.ImportMetadata(ctx, envelope))
			must(db.MarkContentReady(ctx, nextID))
		}
		barrierFile("/disk/volatile", bytes.Repeat([]byte("O"), 4096))
		must(db.Close())
		unix.Sync()
		fmt.Println("FILESYNC_RESET_SETUP_OK")
	case "mutate":
		if hook == "dirty-cache-control" {
			_ = stopAt(hook)
		}
		if publication {
			must(ws.Apply(ctx, nextID))
		} else {
			manifest, err := db.StoreFile(ctx, bytes.NewReader(successor), false)
			must(err)
			_, err = db.CreateLocalVersion(ctx, repository.LocalVersionRequest{Folder: folder, Path: "note.txt", Basis: []history.VersionID{baseID}, Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1})
			must(err)
		}
		panic("requested boundary not reached: " + hook)
	case "verify":
		must(db.VerifyVersionContent(ctx, baseID))
		ids, err := db.VersionIDs(ctx, folder)
		must(err)
		for _, id := range ids {
			ready, err := db.ContentReady(ctx, id)
			must(err)
			if ready {
				must(db.VerifyVersionContent(ctx, id))
			}
		}
		if publication {
			must(ws.Recover(ctx, folder))
			must(ws.Apply(ctx, nextID))
			data, err := os.ReadFile("/disk/root/note.txt")
			must(err)
			if !bytes.Equal(data, successor) {
				panic("publication recovery bytes differ")
			}
		}
		if hook == repository.HookBeforeVersionCommit || hook == repository.HookAfterVersionCommit {
			known, err := db.MetadataKnown(ctx, history.VersionID{Folder: folder, Author: author, Counter: 2})
			must(err)
			if known != (hook == repository.HookAfterVersionCommit) {
				panic("version commit atomicity differs")
			}
		}
		volatile, err := os.ReadFile("/disk/volatile")
		must(err)
		if hook == "dirty-cache-control" && !bytes.Equal(volatile, bytes.Repeat([]byte("O"), 4096)) {
			panic("negative control: dirty guest cache was not discarded")
		}
		fmt.Printf("FILESYNC_RESET_VERIFY_OK hook=%s versions=%d dirty_cache_discarded=%t\n", hook, len(ids), bytes.Equal(volatile, bytes.Repeat([]byte("O"), 4096)))
	default:
		panic("unknown mode")
	}
	for {
		time.Sleep(time.Hour)
	}
}
