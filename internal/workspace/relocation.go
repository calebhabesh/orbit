package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"golang.org/x/sys/unix"
)

const HookRelocationPrepared = "relocation.prepared"
const HookRelocationCopied = "relocation.copied"
const HookRelocationMoved = "relocation.moved"
const HookRelocationCommitted = "relocation.committed"

type ioContextKey struct{}

// Nested workspace calls inherit their caller's gate; unrelated callers wait
// while relocation holds it exclusively. Transfers may continue storing CAS.
func (w *Workspace) enterIO(ctx context.Context) (context.Context, func()) {
	if ctx.Value(ioContextKey{}) == w {
		return ctx, func() {}
	}
	w.ioGate.RLock()
	return context.WithValue(ctx, ioContextKey{}, w), w.ioGate.RUnlock
}

type RelocationResult struct {
	Path           string `json:"path"`
	SourceRetained bool   `json:"source_retained"`
	SourcePath     string `json:"source_path"`
}
type relocationJournal struct {
	Old   repository.RootRegistration
	Next  repository.RootRegistration
	Stage string
}

func (w *Workspace) relocationPath(folder history.ID) string {
	return filepath.Join(w.repo.StateDir(), "relocation-"+hex.EncodeToString(folder[:])+".json")
}
func syncDirectory(path string) error {
	fd, _, err := openAbsoluteDirectory(path)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return unix.Fsync(fd)
}
func (w *Workspace) writeRelocation(folder history.ID, j relocationJournal) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(w.repo.StateDir(), ".relocation-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(f.Name(), w.relocationPath(folder)); err != nil {
		return err
	}
	return syncDirectory(w.repo.StateDir())
}
func (w *Workspace) clearRelocation(folder history.ID) error {
	if err := os.Remove(w.relocationPath(folder)); err != nil {
		return err
	}
	return syncDirectory(w.repo.StateDir())
}

// recoverRelocation never guesses from matching bytes: device, inode and the
// private registration marker must match the journal's expected directory.
func (w *Workspace) recoverRelocation(ctx context.Context, folder history.ID) error {
	w.relocationMu.Lock()
	defer w.relocationMu.Unlock()
	data, err := os.ReadFile(w.relocationPath(folder))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var j relocationJournal
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	if j.Old.Folder != folder || j.Next.Folder != folder {
		return errors.New("invalid relocation journal")
	}
	reg, err := w.repo.Root(ctx, folder)
	if err != nil {
		return err
	}
	if reg.Path == j.Next.Path {
		root, err := w.openRootRegistered(ctx, folder)
		if err != nil {
			return err
		}
		root.close()
		return w.clearRelocation(folder)
	}
	if reg.Path != j.Old.Path {
		return errors.New("relocation registration changed unexpectedly")
	}
	// Probe destination using the full existing root verifier without changing DB.
	if w.matchesRegistration(j.Next) {
		if err := syncDirectory(filepath.Dir(j.Old.Path)); err != nil {
			return err
		}
		if err := syncDirectory(filepath.Dir(j.Next.Path)); err != nil {
			return err
		}
		if err := w.repo.RelocateRoot(ctx, j.Old, j.Next); err != nil {
			return err
		}
		return w.clearRelocation(folder)
	}
	if w.matchesRegistration(j.Old) {
		// Rename did not happen. Keep partial copy directories for explicit cleanup.
		if err := w.repo.RelocateRoot(ctx, j.Old, j.Old); err != nil {
			return err
		}
		return w.clearRelocation(folder)
	}
	return fmt.Errorf("%w: interrupted relocation needs attention", ErrRootUnavailable)
}
func (w *Workspace) matchesRegistration(reg repository.RootRegistration) bool {
	root, err := w.openRegistration(reg)
	if err != nil {
		return false
	}
	root.close()
	return true
}

// Relocate requires an unused destination and existing parent. Across devices
// the old tree stays intact, including any editor writes racing the copy.
func (w *Workspace) Relocate(ctx context.Context, folder history.ID, expectedPath, destination string) (*RelocationResult, error) {
	w.ioGate.Lock()
	defer w.ioGate.Unlock()
	ctx = context.WithValue(ctx, ioContextKey{}, w)
	if err := w.recoverRelocation(ctx, folder); err != nil {
		return nil, err
	}
	old, err := w.repo.Root(ctx, folder)
	if err != nil {
		return nil, err
	}
	dest, err := filepath.Abs(destination)
	if err != nil || destination == "" {
		return nil, errors.New("destination path is required")
	}
	dest = filepath.Clean(dest)
	if old.Path == dest {
		result := &RelocationResult{Path: dest, SourcePath: expectedPath}
		// A lost cross-drive response must still report the retained original.
		if expectedPath != dest {
			fd, stat, err := openAbsoluteDirectory(expectedPath)
			if err == nil {
				unix.Close(fd)
				original := old
				original.Path, original.Device, original.Inode = expectedPath, uint64(stat.Dev), stat.Ino
				result.SourceRetained = w.matchesRegistration(original)
			}
		}
		return result, nil
	}
	if old.Path != expectedPath {
		return nil, errors.New("folder location changed; refresh and review again")
	}
	state, err := filepath.Abs(w.repo.StateDir())
	if err != nil {
		return nil, err
	}
	if overlaps(old.Path, dest) || overlaps(state, dest) {
		return nil, errors.New("destination overlaps the current folder or private state directory")
	}
	regs, err := w.repo.RegisteredFolders(ctx)
	if err != nil {
		return nil, err
	}
	for _, reg := range regs {
		if reg.Folder != folder && overlaps(reg.Path, dest) {
			return nil, errors.New("destination overlaps another workspace")
		}
	}
	parent, ps, err := openAbsoluteDirectory(filepath.Dir(dest))
	if err != nil {
		return nil, fmt.Errorf("destination parent must exist without symlinks: %w", err)
	}
	defer unix.Close(parent)
	var ds unix.Stat_t
	if err := unix.Fstatat(parent, filepath.Base(dest), &ds, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
		return nil, errors.New("destination already exists or cannot be inspected; choose an unused folder name")
	}
	if err := w.Recover(ctx, folder); err != nil {
		return nil, err
	}
	root, err := w.openRootRegistered(ctx, folder)
	if err != nil {
		return nil, err
	}
	defer root.close()
	next := old
	next.Path = dest
	j := relocationJournal{Old: old, Next: next}
	cross := uint64(ps.Dev) != old.Device
	if cross {
		// Stage under a private, exclusively created sibling. Unsupported entries
		// fail without modifying either the source or final destination.
		stage, err := os.MkdirTemp(filepath.Dir(dest), ".orbit-relocation-")
		if err != nil {
			return nil, err
		}
		j.Stage = stage
		stageFD, st, err := openAbsoluteDirectory(stage)
		if err != nil {
			return nil, err
		}
		defer unix.Close(stageFD)
		before, err := relocationTree(ctx, root.fd, stageFD, true)
		if err != nil {
			return nil, fmt.Errorf("copy incomplete (original retained; staging %s): %w", stage, err)
		}
		if err := w.callHook(HookRelocationCopied); err != nil {
			return nil, err
		}
		after, err := relocationTree(ctx, root.fd, -1, false)
		if err != nil {
			return nil, err
		}
		copied, err := relocationTree(ctx, stageFD, -1, false)
		if err != nil {
			return nil, err
		}
		if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(before, copied) {
			return nil, fmt.Errorf("files changed while copying; original retained, retry after closing editors (staging %s)", stage)
		}
		var rs unix.Stat_t
		if err := unix.Fstat(root.fd, &rs); err != nil {
			return nil, err
		}
		if err := unix.Fchmod(stageFD, rs.Mode&0o777); err != nil {
			return nil, err
		}
		if err := unix.Fsync(stageFD); err != nil {
			return nil, err
		}
		next.Device = uint64(st.Dev)
		next.Inode = st.Ino
		j.Next = next
	}
	if err := w.writeRelocation(folder, j); err != nil {
		return nil, err
	}
	if err := w.repo.PauseFolder(ctx, folder, "RELOCATING"); err != nil {
		return nil, err
	}
	if err := w.callHook(HookRelocationPrepared); err != nil {
		return nil, err
	}
	source := old.Path
	if cross {
		source = j.Stage
	}
	sourceParent, _, err := openAbsoluteDirectory(filepath.Dir(source))
	if err != nil {
		return nil, err
	}
	defer unix.Close(sourceParent)
	// Check the source name still identifies the directory we opened.
	var ss unix.Stat_t
	if err := unix.Fstatat(sourceParent, filepath.Base(source), &ss, unix.AT_SYMLINK_NOFOLLOW); err != nil || uint64(ss.Dev) != next.Device || ss.Ino != next.Inode {
		return nil, ErrRootUnavailable
	}
	if err := unix.Renameat2(sourceParent, filepath.Base(source), parent, filepath.Base(dest), unix.RENAME_NOREPLACE); err != nil {
		return nil, err
	}
	if err := unix.Fsync(sourceParent); err != nil {
		return nil, err
	}
	if err := unix.Fsync(parent); err != nil {
		return nil, err
	}
	if !w.matchesRegistration(next) {
		return nil, ErrRootUnavailable
	}
	if err := w.callHook(HookRelocationMoved); err != nil {
		return nil, err
	}
	if err := w.repo.RelocateRoot(ctx, old, next); err != nil {
		return nil, err
	}
	if err := w.callHook(HookRelocationCommitted); err != nil {
		return nil, err
	}
	if err := w.clearRelocation(folder); err != nil {
		return nil, err
	}
	return &RelocationResult{Path: dest, SourceRetained: cross, SourcePath: old.Path}, nil
}

type treeEntry struct {
	Path   string
	Mode   uint32
	Size   int64
	Digest [32]byte
}

func relocationTree(ctx context.Context, source, destination int, copyFiles bool) ([]treeEntry, error) {
	var entries []treeEntry
	var walk func(int, int, string) error
	walk = func(src, dst int, prefix string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		fd, err := unix.Openat(src, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
		if err != nil {
			return err
		}
		dir := os.NewFile(uintptr(fd), prefix)
		names, err := dir.ReadDir(-1)
		dir.Close()
		if err != nil {
			return err
		}
		sort.Slice(names, func(i, j int) bool { return names[i].Name() < names[j].Name() })
		for _, name := range names {
			if err := ctx.Err(); err != nil {
				return err
			}
			n := name.Name()
			path := filepath.Join(prefix, n)
			var stat unix.Stat_t
			if err := unix.Fstatat(src, n, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
				return err
			}
			e := treeEntry{Path: path, Mode: stat.Mode & 0o777, Size: stat.Size}
			switch stat.Mode & unix.S_IFMT {
			case unix.S_IFDIR:
				e.Size = 0
				e.Mode |= unix.S_IFDIR
				entries = append(entries, e)
				s, err := unix.Openat2(src, n, &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV})
				if err != nil {
					return err
				}
				d := -1
				if copyFiles {
					if err := unix.Mkdirat(dst, n, 0o700); err != nil {
						unix.Close(s)
						return err
					}
					d, err = unix.Openat(dst, n, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
					if err != nil {
						unix.Close(s)
						return err
					}
				}
				err = walk(s, d, path)
				unix.Close(s)
				if copyFiles {
					if err == nil {
						err = unix.Fchmod(d, stat.Mode&0o777)
					}
					if err == nil {
						err = unix.Fsync(d)
					}
					unix.Close(d)
				}
				if err != nil {
					return err
				}
			case unix.S_IFREG:
				e.Mode |= unix.S_IFREG
				s, err := unix.Openat2(src, n, &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_NONBLOCK | unix.O_CLOEXEC, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV})
				if err != nil {
					return err
				}
				input := os.NewFile(uintptr(s), path)
				hash := sha256.New()
				var out *os.File
				writer := io.Writer(hash)
				if copyFiles {
					d, err := unix.Openat(dst, n, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
					if err != nil {
						input.Close()
						return err
					}
					out = os.NewFile(uintptr(d), path)
					writer = io.MultiWriter(hash, out)
				}
				count, err := io.Copy(writer, input)
				input.Close()
				if out != nil {
					if err == nil {
						err = out.Chmod(os.FileMode(stat.Mode & 0o777))
					}
					if err == nil {
						err = out.Sync()
					}
					ce := out.Close()
					if err == nil {
						err = ce
					}
				}
				if err != nil {
					return err
				}
				if count != stat.Size {
					return ErrUnstableFile
				}
				copy(e.Digest[:], hash.Sum(nil))
				entries = append(entries, e)
			default:
				return fmt.Errorf("%w: %s", ErrUnsupportedEntry, path)
			}
		}
		return nil
	}
	err := walk(source, destination, "")
	return entries, err
}
