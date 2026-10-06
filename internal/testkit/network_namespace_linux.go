package testkit

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ValidateNetworkNamespace refuses host networking and requires a private marked
// root plus a network namespace owned by the current remapped user namespace.
func ValidateNetworkNamespace(root, parent string) error {
	canonical, err := filepath.EvalSymlinks(root)
	info, statErr := os.Stat(root)
	if err != nil || statErr != nil || canonical != root || !filepath.IsAbs(root) || info.Mode().Perm()&0077 != 0 {
		return errors.New("invalid private namespace root")
	}
	marker, err := os.Lstat(filepath.Join(root, Marker))
	if err != nil || !marker.Mode().IsRegular() {
		return errors.New("namespace marker missing")
	}
	current, err := os.Readlink("/proc/self/ns/net")
	if err != nil || parent == "" || current == parent {
		return errors.New("refusing parent network namespace")
	}
	data, err := os.ReadFile("/proc/self/uid_map")
	fields := strings.Fields(string(data))
	if err != nil || os.Geteuid() != 0 || len(fields) < 3 || (fields[0] == "0" && fields[1] == "0") {
		return errors.New("requires remapped user namespace")
	}
	fd, err := unix.Open("/proc/self/ns/net", unix.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	ownerFD, err := unix.IoctlRetInt(fd, 0xb701)
	if err != nil {
		return err
	}
	defer unix.Close(ownerFD)
	var owner unix.Stat_t
	if err = unix.Fstat(ownerFD, &owner); err != nil {
		return err
	}
	userInfo, err := os.Stat("/proc/self/ns/user")
	if err != nil {
		return err
	}
	if owner.Ino != userInfo.Sys().(*syscall.Stat_t).Ino {
		return errors.New("network namespace has foreign owner")
	}
	return nil
}
