//go:build ignore

// This executable is a disposable VM's init, never a release fault endpoint.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"

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

// Exhaust only the dedicated guest disk. Physical devices and host folders
// never participate in this experiment.
func fullDiskExperiment(db *repository.DB, ws *workspace.Workspace, hook string, baseID, nextID history.VersionID) {
	if !strings.HasPrefix(hook, "enospc.") {
		panic("invalid disk-full experiment")
	}
	if hook == "enospc.fsync" {
		out, err := exec.Command("/init", "--fsync-worker").CombinedOutput()
		fmt.Print(string(out))
		must(err)
		must(db.VerifyVersionContent(ctx, baseID))
		fmt.Println("FILESYNC_ENOSPC_VERIFY_OK hook=enospc.fsync protected_hashes=true syscall_fault=true")
		return
	}
	if hook == "enospc.checkpoint" {
		// Grow committed WAL before filling the disk, then force the main database
		// to allocate blocks while checkpointing that committed record.
		must(db.PutInstallationValue(ctx, "checkpoint-growth", bytes.Repeat([]byte("W"), 2*1024*1024)))
	}
	filler, err := os.OpenFile("/disk/filler", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	must(err)
	buffer := make([]byte, 64*1024)
	for {
		_, err = filler.Write(buffer)
		if err != nil {
			break
		}
	}
	if !errors.Is(err, syscall.ENOSPC) {
		panic(fmt.Sprintf("disk fill error: %v", err))
	}
	_ = filler.Sync()
	must(filler.Close())
	var operationErr error
	switch hook {
	case "enospc.object":
		_, operationErr = db.StoreFile(ctx, bytes.NewReader(bytes.Repeat([]byte("D"), 2*1024*1024)), false)
	case "enospc.sqlite":
		operationErr = db.PutInstallationValue(ctx, "failed-growth", bytes.Repeat([]byte("S"), 2*1024*1024))
	case "enospc.checkpoint":
		operationErr = db.Checkpoint(ctx)
	case "enospc.staging":
		operationErr = ws.Apply(ctx, nextID)
	default:
		panic("unknown disk-full case")
	}
	if operationErr == nil {
		panic("disk-full operation unexpectedly succeeded")
	}
	if !errors.Is(operationErr, syscall.ENOSPC) && !strings.Contains(strings.ToLower(operationErr.Error()), "full") && !strings.Contains(strings.ToLower(operationErr.Error()), "space") {
		panic(fmt.Sprintf("operation failed for another reason: %v", operationErr))
	}
	fmt.Printf("FILESYNC_ENOSPC_ERROR hook=%s error=%v\n", hook, operationErr)
	must(os.Remove("/disk/filler"))
	if hook == "enospc.staging" {
		must(ws.Recover(ctx, folder))
	}
	must(db.VerifyVersionContent(ctx, baseID))
	if hook == "enospc.staging" {
		data, err := os.ReadFile("/disk/root/note.txt")
		must(err)
		if !bytes.Equal(data, original) {
			panic("disk-full staging changed working bytes")
		}
		must(db.VerifyVersionContent(ctx, nextID))
	}
	must(db.Close())
	db, err = repository.Open(ctx, "/disk/state")
	must(err)
	must(db.VerifyVersionContent(ctx, baseID))
	must(db.Checkpoint(ctx))
	if hook == "enospc.checkpoint" {
		data, err := db.InstallationValue(ctx, "checkpoint-growth")
		must(err)
		if len(data) != 2*1024*1024 {
			panic("committed WAL record lost")
		}
	}
	fmt.Printf("FILESYNC_ENOSPC_VERIFY_OK hook=%s protected_hashes=true reopened=true\n", hook)
}

func fsyncWorker() {
	marker, err := os.ReadFile("/disk/.filesync-disposable")
	must(err)
	if string(marker) != "filesync disposable reset VM\n" {
		panic("unmarked fsync-fault disk")
	}
	db, err := repository.Open(ctx, "/disk/state")
	must(err)
	// Precreate the shard so the fault reaches the incoming-file flush.
	digest := sha256.Sum256(bytes.Repeat([]byte("F"), 1024*1024))
	must(os.MkdirAll(fmt.Sprintf("/disk/state/objects/sha256/%02x", digest[0]), 0700))
	// The filter is confined to this VM child process. Real fsync/fdatasync
	// syscalls return ENOSPC; no production hook fakes a post-success error.
	must(unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0))
	filter := []unix.SockFilter{
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 0},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jt: 2, K: unix.SYS_FSYNC},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jt: 1, K: unix.SYS_FDATASYNC},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ERRNO | uint32(syscall.ENOSPC)},
	}
	program := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	_, _, errno := unix.RawSyscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER, unix.SECCOMP_FILTER_FLAG_TSYNC, uintptr(unsafe.Pointer(&program)))
	if errno != 0 {
		panic(errno)
	}
	_, err = db.StoreFile(ctx, bytes.NewReader(bytes.Repeat([]byte("F"), 1024*1024)), false)
	if !errors.Is(err, syscall.ENOSPC) || !strings.Contains(err.Error(), "flush incoming chunk") {
		panic(fmt.Sprintf("fsync syscall fault not observed: %v", err))
	}
	fmt.Printf("FILESYNC_ENOSPC_ERROR hook=enospc.fsync error=%v\n", err)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--fsync-worker" {
		fsyncWorker()
		return
	}
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
	publication := strings.HasPrefix(hook, "publication.") || hook == "enospc.staging"
	if hook == "enospc.staging" {
		successor = bytes.Repeat([]byte("R"), 2*1024*1024)
	}
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
	case "enospc":
		fullDiskExperiment(db, ws, hook, baseID, nextID)
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
