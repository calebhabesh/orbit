package rendezvous

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RelayBudgetFlush bounds how many forwarded bytes a crash can forget: the
// count is written at most this often, and again on Close (EG4).
const RelayBudgetFlush = 30 * time.Second

// RelayBudget is the operator's monthly relay egress allowance (E09). It
// counts the same bytes as orbit_net_relay_bytes_total: relay payload written
// to the receiving device, in both forwarding directions, without TLS or
// WebSocket framing. The count resets at the start of each UTC month and is
// kept in a private state file so it survives restarts.
type RelayBudget struct {
	mu      sync.Mutex
	limit   uint64
	path    string
	now     func() time.Time
	month   string
	used    uint64
	dirty   bool
	flushed time.Time
}

type relayBudgetFile struct {
	Version int    `json:"version"`
	Month   string `json:"month"`
	Bytes   uint64 `json:"bytes"`
}

func monthKey(t time.Time) string { return t.UTC().Format("2006-01") }

// NextMonth is when an exhausted budget resets: the next UTC month start.
func NextMonth(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month()+1, 1, 0, 0, 0, 0, time.UTC)
}

// OpenRelayBudget loads path (owner-only) or starts at zero. A file from an
// earlier month is ignored. now nil means time.Now.
func OpenRelayBudget(path string, limit uint64, now func() time.Time) (*RelayBudget, error) {
	if limit == 0 {
		return nil, errors.New("relay monthly budget must be finite and positive")
	}
	if now == nil {
		now = time.Now
	}
	b := &RelayBudget{limit: limit, path: path, now: now, month: monthKey(now())}
	if path == "" {
		return b, nil
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return b, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > 4096 {
		return nil, fmt.Errorf("relay budget state %s must be a private regular file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f relayBudgetFile
	if err = json.Unmarshal(data, &f); err != nil || f.Version != 1 {
		return nil, fmt.Errorf("relay budget state %s is unreadable", path)
	}
	if f.Month == b.month {
		b.used = f.Bytes
	}
	return b, nil
}

// rollLocked starts a new month when the UTC month changed.
func (b *RelayBudget) rollLocked() {
	if m := monthKey(b.now()); m != b.month {
		b.month, b.used, b.dirty = m, 0, true
	}
}

// Exhausted reports whether this month's allowance is spent.
func (b *RelayBudget) Exhausted() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollLocked()
	return b.used >= b.limit
}

// Add records forwarded bytes and writes the count at most every
// RelayBudgetFlush.
func (b *RelayBudget) Add(n uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollLocked()
	b.used += n
	b.dirty = true
	if b.now().Sub(b.flushed) >= RelayBudgetFlush {
		_ = b.flushLocked()
	}
}

// Snapshot returns the month, bytes used and the limit.
func (b *RelayBudget) Snapshot() (month string, used, limit uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollLocked()
	return b.month, b.used, b.limit
}

// Flush writes the count if it changed since the last write.
func (b *RelayBudget) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollLocked()
	return b.flushLocked()
}

func (b *RelayBudget) flushLocked() error {
	b.flushed = b.now()
	if b.path == "" || !b.dirty {
		return nil
	}
	data, _ := json.Marshal(relayBudgetFile{Version: 1, Month: b.month, Bytes: b.used})
	dir := filepath.Dir(b.path)
	f, err := os.CreateTemp(dir, ".relay-budget-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), b.path)
	}
	if err != nil {
		return err
	}
	if d, e := os.Open(dir); e == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	b.dirty = false
	return nil
}
