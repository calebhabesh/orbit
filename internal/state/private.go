package state

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// PrivateFile refuses symlinks, foreign ownership and public local credentials.
func ReadPrivate(dir, name string, max int64) ([]byte, error) {
	if err := ValidateDirectory(dir); err != nil {
		return nil, err
	}
	fd, err := unix.Open(filepath.Join(dir, name), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > max {
		return nil, errors.New("local control files must be bounded owner-only regular files")
	}
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if len(b) > int(max) {
		return nil, errors.New("local control file too large")
	}
	return b, err
}
