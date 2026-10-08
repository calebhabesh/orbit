package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/state"
)

// WritePrivate installs a flushed owner-only record, including its directory entry.
func WritePrivate(dir, name string, data []byte) error {
	f, err := os.CreateTemp(dir, ".runtime-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), filepath.Join(dir, name)); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func DefaultRuntimeSettings() tc.Settings {
	l := DefaultStorageLimits()
	return tc.Settings{DataBudget: tc.Uint(l.DataBudgetBytes), MetadataBudget: tc.Uint(l.MetadataBudgetBytes), ReserveBytes: tc.Uint(l.FreeSpaceReserveBytes), Concurrency: 4, Startup: "manual"}
}
func ValidateRuntimeSettings(s tc.Settings) error {
	if s.RetentionSeconds != 0 {
		return errors.New("UNSUPPORTED_CAPABILITY: retention remains per-folder; use reviewed retention controls")
	}
	if uint64(s.BandwidthBytesPerSecond) > math.MaxInt64 {
		return errors.New("bandwidth exceeds supported signed range")
	}

	if s.DataBudget == 0 || s.MetadataBudget == 0 || s.ReserveBytes == 0 || s.Concurrency == 0 || s.Concurrency > 32 {
		return errors.New("finite positive budgets and concurrency 1–32 required")
	}
	if s.Startup != "manual" && s.Startup != "login" && s.Startup != "unattended" {
		return errors.New("invalid startup mode")
	}
	for _, address := range []string{s.PeerListen, s.EnrollmentListen} {
		if address != "" {
			host, port, err := net.SplitHostPort(address)
			number, e := strconv.ParseUint(port, 10, 16)
			if err != nil || e != nil || number > 65535 || net.ParseIP(host) == nil {
				return fmt.Errorf("listener requires numeric address: %q", address)
			}
		}
	}
	for _, address := range []string{s.AdvertisedPeer, s.AdvertisedEnrollment} {
		if address != "" {
			host, port, err := net.SplitHostPort(address)
			ip := net.ParseIP(host)
			number, e := strconv.ParseUint(port, 10, 16)
			if e != nil || number == 0 || err != nil || ip == nil || ip.IsUnspecified() || ip.IsLoopback() {
				return errors.New("advertised address requires reachable numeric nonloopback host")
			}
		}
	}
	return nil
}

// HasRuntimeSettings reports whether reviewed runtime settings were saved.
func HasRuntimeSettings(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, "runtime.json"))
	return err == nil
}
func LoadRuntimeSettings(dir string) (tc.Settings, error) {
	s := DefaultRuntimeSettings()
	b, err := state.ReadPrivate(dir, "runtime.json", tc.MaxMetadata)
	if errors.Is(err, os.ErrNotExist) {
		l, e := LoadStorageLimits(dir)
		if e != nil {
			return s, e
		}
		s.DataBudget = tc.Uint(l.DataBudgetBytes)
		s.MetadataBudget = tc.Uint(l.MetadataBudgetBytes)
		s.ReserveBytes = tc.Uint(l.FreeSpaceReserveBytes)
		return s, nil
	}
	if err != nil {
		return s, err
	}
	info, err := os.Lstat(filepath.Join(dir, "runtime.json"))
	if err != nil {
		return s, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return s, errors.New("runtime.json requires private regular file")
	}
	if err = tc.Decode(b, &s); err != nil {
		return s, err
	}
	return s, ValidateRuntimeSettings(s)
}
func SaveRuntimeSettings(dir string, s tc.Settings) error {
	if err := ValidateRuntimeSettings(s); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return WritePrivate(dir, "runtime.json", b)
}
