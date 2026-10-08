package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"golang.org/x/sys/unix"
)

// AdoptionWalk is private durable continuation, never a user supplied cursor.
// Directory offsets are observations, not snapshot guarantees; commit repeats
// enumeration and compares the complete generation before registering.
type AdoptionWalk struct {
	RootMode uint32              `json:"root_mode"`
	Preview  tc.RootPreview      `json:"preview"`
	Stack    []AdoptionDirectory `json:"stack"`
	Sum      [32]byte            `json:"sum"`
	File     string              `json:"file"`
	Offset   int64               `json:"offset"`
	Hash     []byte              `json:"hash"`
	Stat     unix.Stat_t         `json:"stat"`
	Missing  bool                `json:"missing"`
}
type AdoptionDirectory struct {
	Path   string
	Offset int64
}

func (w *Workspace) BeginAdoption(ctx context.Context, path string) (AdoptionWalk, error) {
	a := AdoptionWalk{}
	abs, err := filepath.Abs(path)
	if err != nil {
		return a, err
	}
	state, err := filepath.EvalSymlinks(w.repo.StateDir())
	if err != nil {
		return a, err
	}
	if overlaps(abs, state) {
		return a, errors.New("root overlaps private state")
	}
	regs, err := w.repo.RegisteredFolders(ctx)
	if err != nil {
		return a, err
	}
	for _, reg := range regs {
		if overlaps(abs, reg.Path) {
			return a, errors.New("root overlaps registered folder")
		}
	}
	fd, st, err := openAbsoluteDirectory(abs)
	if errors.Is(err, unix.ENOENT) {
		fd, st, err = openAbsoluteDirectory(filepath.Dir(abs))
		a.Missing = true
	}
	if err != nil {
		return a, err
	}
	defer unix.Close(fd)
	a.RootMode = st.Mode
	a.Preview = tc.RootPreview{Root: abs, Device: tc.Uint(st.Dev), Inode: tc.Uint(st.Ino), Issues: []tc.Issue{}}
	if a.Missing {
		a.Preview.Complete = true
	} else {
		a.Stack = []AdoptionDirectory{{Path: ""}}
	}
	return a, nil
}
func (a *AdoptionWalk) add(value any) {
	b, _ := json.Marshal(value)
	d := sha256.Sum256(b)
	for i := range d {
		a.Sum[i] ^= d[i]
	}
}
func (a *AdoptionWalk) issue(path, code string, unsupported bool) {
	if unsupported {
		a.Preview.Unsupported++
	} else {
		a.Preview.Unreadable++
	}
	if len(a.Preview.Issues) < tc.MaxPage {
		a.Preview.Issues = append(a.Preview.Issues, tc.Issue{Path: path, Code: code})
	}
	a.add(struct{ Path, Code string }{path, code})
}
func adoptionStat(st unix.Stat_t) any {
	return struct {
		Dev          uint64
		Ino          uint64
		Mode         uint32
		Size         int64
		Mtime, Ctime int64
		Links        uint64
	}{uint64(st.Dev), st.Ino, st.Mode, st.Size, st.Mtim.Nano(), st.Ctim.Nano(), uint64(st.Nlink)}
}
func (w *Workspace) AdoptionSlice(ctx context.Context, a *AdoptionWalk) error {
	path := a.Preview.Root
	if a.Missing {
		path = filepath.Dir(path)
	}
	fd, st, err := openAbsoluteDirectory(path)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if tc.Uint(st.Dev) != a.Preview.Device || tc.Uint(st.Ino) != a.Preview.Inode || st.Mode != a.RootMode {
		return ErrRootUnavailable
	}
	if a.Missing {
		var s unix.Stat_t
		if err := unix.Fstatat(fd, filepath.Base(a.Preview.Root), &s, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
			return ErrRootUnavailable
		}
		return nil
	}
	deadline := time.Now().Add(5 * time.Second)
	entries, readBytes := 0, int64(0)
	buffer := make([]byte, 128<<10)
	for (a.File != "" || len(a.Stack) > 0) && entries < 10000 && readBytes < 32<<20 && len(a.Preview.Issues) < tc.MaxPage && time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if a.File != "" {
			ffd, err := safeOpen(fd, a.File, unix.O_RDONLY|unix.O_NONBLOCK, 0)
			if err != nil {
				a.issue(a.File, "UNREADABLE", false)
				a.File = ""
				continue
			}
			f := os.NewFile(uintptr(ffd), a.File)
			var now unix.Stat_t
			err = unix.Fstat(ffd, &now)
			if err != nil || generationStat(now) != generationStat(a.Stat) {
				f.Close()
				a.issue(a.File, "UNSTABLE_FILE", false)
				a.File = ""
				continue
			}
			h := sha256.New()
			if len(a.Hash) > 0 {
				if err = h.(encoding.BinaryUnmarshaler).UnmarshalBinary(a.Hash); err != nil {
					f.Close()
					return err
				}
			}
			_, err = f.Seek(a.Offset, io.SeekStart)
			if err != nil {
				f.Close()
				return err
			}
			n, e := f.Read(buffer)
			if n > 0 {
				h.Write(buffer[:n])
				a.Offset += int64(n)
				readBytes += int64(n)
			}
			err = unix.Fstat(ffd, &now)
			f.Close()
			if err != nil || generationStat(now) != generationStat(a.Stat) {
				a.issue(a.File, "UNSTABLE_FILE", false)
				a.File = ""
				continue
			}
			if e != nil && !errors.Is(e, io.EOF) {
				a.issue(a.File, "UNREADABLE", false)
				a.File = ""
				continue
			}
			if errors.Is(e, io.EOF) {
				a.add(struct {
					Path   string
					Stat   any
					Digest string
				}{a.File, adoptionStat(a.Stat), hex.EncodeToString(h.Sum(nil))})
				a.Preview.Files++
				a.Preview.Bytes += tc.Uint(a.Offset)
				a.File = ""
				a.Hash = nil
				a.Offset = 0
			} else {
				a.Hash, _ = h.(encoding.BinaryMarshaler).MarshalBinary()
			}
			continue
		}
		i := len(a.Stack) - 1
		frame := a.Stack[i]
		dfd := fd
		if frame.Path != "" {
			dfd, err = safeOpen(fd, frame.Path, unix.O_RDONLY|unix.O_DIRECTORY, 0)
		} else {
			dfd, err = unix.Openat(fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
		}
		if err != nil {
			a.issue(frame.Path, "INCOMPLETE_SUBTREE", false)
			a.Stack = a.Stack[:i]
			continue
		}
		dir := os.NewFile(uintptr(dfd), frame.Path)
		_, err = dir.Seek(frame.Offset, io.SeekStart)
		if err != nil {
			dir.Close()
			return err
		}
		dents := make([]byte, 4096)
		n, e := unix.Getdents(dfd, dents)
		if n == 0 {
			dir.Close()
			a.Stack = a.Stack[:i]
			if e != nil {
				a.issue(frame.Path, "INCOMPLETE_SUBTREE", false)
			}
			continue
		}
		if n < 20 {
			dir.Close()
			return errors.New("short directory record")
		}
		length := int(binary.NativeEndian.Uint16(dents[16:18]))
		if length < 20 || length > n {
			dir.Close()
			return errors.New("invalid directory record")
		}
		a.Stack[i].Offset = int64(binary.NativeEndian.Uint64(dents[8:16]))
		raw := dents[19:length]
		if z := bytes.IndexByte(raw, 0); z >= 0 {
			raw = raw[:z]
		}
		name := string(raw)
		if name == "." || name == ".." {
			dir.Close()
			continue
		}
		p := filepath.Join(frame.Path, name)
		entries++
		if frame.Path == "" && name == scratchName {
			dir.Close()
			continue
		}
		var s unix.Stat_t
		err = unix.Fstatat(dfd, name, &s, unix.AT_SYMLINK_NOFOLLOW)
		dir.Close()
		if err != nil {
			a.issue(p, "UNREADABLE", false)
			continue
		}
		if history.ValidatePath(p) != nil || uint64(s.Dev) != uint64(a.Preview.Device) {
			a.issue(p, "UNSUPPORTED_PATH_OR_MOUNT", true)
			continue
		}
		switch s.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
			a.Preview.Directories++
			a.add(struct {
				Path     string
				Dev, Ino uint64
				Mode     uint32
			}{p, uint64(s.Dev), s.Ino, s.Mode})
			if len(a.Stack) >= 256 {
				a.issue(p, "DEPTH_LIMIT", true)
			} else {
				a.Stack = append(a.Stack, AdoptionDirectory{Path: p})
			}
		case unix.S_IFREG:
			if s.Nlink != 1 {
				a.issue(p, "HARD_LINK_UNSUPPORTED", true)
				continue
			}
			a.File = p
			a.Stat = s
			a.Offset = 0
			a.Hash = nil
		default:
			a.issue(p, "UNSUPPORTED_OBJECT", true)
		}
	}
	a.Preview.Complete = a.File == "" && len(a.Stack) == 0
	return nil
}
func generationStat(s unix.Stat_t) string { b, _ := json.Marshal(adoptionStat(s)); return string(b) }
func (a AdoptionWalk) Generation() string {
	b, _ := json.Marshal(struct {
		RootMode                                    uint32
		Root                                        string
		Dev, Ino                                    tc.Uint
		Missing                                     bool
		Sum                                         [32]byte
		Files, Dirs, Bytes, Unsupported, Unreadable tc.Uint
	}{a.RootMode, a.Preview.Root, a.Preview.Device, a.Preview.Inode, a.Missing, a.Sum, a.Preview.Files, a.Preview.Directories, a.Preview.Bytes, a.Preview.Unsupported, a.Preview.Unreadable})
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:])
}

// CreateAdoptionRoot rechecks the reviewed parent descriptor and flushes mkdir.
func (w *Workspace) CreateAdoptionRoot(a AdoptionWalk) error {
	if !a.Missing {
		return nil
	}
	fd, st, err := openAbsoluteDirectory(filepath.Dir(a.Preview.Root))
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if tc.Uint(st.Dev) != a.Preview.Device || tc.Uint(st.Ino) != a.Preview.Inode {
		return ErrRootUnavailable
	}
	if err = unix.Mkdirat(fd, filepath.Base(a.Preview.Root), 0o755); err != nil {
		return err
	}
	return unix.Fsync(fd)
}
