package network

import (
	"context"
	"errors"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const NetworkPollInterval = 2 * time.Second
const NetworkChangeQuietPeriod = 5 * time.Second

// NetworkSnapshot contains only routing observations, never persisted authority.
// Sorting prevents an OS enumeration reorder from replacing live connections.
func NetworkSnapshot(names []string) (string, error) {
	interfaces, err := SelectedInterfaces(names)
	if err != nil && err.Error() != "INTERFACE_UNAVAILABLE" {
		return "", err
	}
	var rows []string
	for _, iface := range interfaces {
		for _, prefix := range iface.Prefixes {
			rows = append(rows, iface.Name+"/"+strconv.Itoa(iface.Index)+"/"+prefix.String())
		}
	}
	for _, path := range []string{"/proc/net/route", "/proc/net/ipv6_route"} {
		f, err := os.Open(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		raw, err := io.ReadAll(io.LimitReader(f, 65537))
		_ = f.Close()
		if err != nil {
			return "", err
		}
		if len(raw) > 65536 {
			return "", ErrBackpressure
		}
		for _, line := range strings.Split(string(raw), "\n") {
			fields := strings.Fields(line)
			if path == "/proc/net/route" && len(fields) >= 11 && fields[1] == "00000000" && fields[7] == "00000000" {
				// Ignore volatile reference/use counters.
				rows = append(rows, strings.Join([]string{fields[0], fields[2], fields[3], fields[6]}, "/"))
			}
			if path == "/proc/net/ipv6_route" && len(fields) == 10 && fields[0] == strings.Repeat("0", 32) && fields[1] == "00" {
				rows = append(rows, strings.Join([]string{fields[9], fields[4], fields[5], fields[8]}, "/"))
			}
		}
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n"), nil
}

// WatchNetwork runs one joined worker with coalesced snapshots. A change must be
// observed twice; repeated flapping cannot accumulate callbacks or goroutines.
// Sampling errors retain the last valid snapshot instead of retiring good routes.
func WatchNetwork(ctx context.Context, snapshot func() (string, error), changed func(), interval time.Duration) func() {
	return WatchNetworkTiming(ctx, snapshot, changed, interval, NetworkChangeQuietPeriod)
}

func WatchNetworkTiming(ctx context.Context, snapshot func() (string, error), changed func(), interval, quiet time.Duration) func() {
	if quiet <= 0 {
		quiet = NetworkChangeQuietPeriod
	}
	if interval <= 0 {
		interval = NetworkPollInterval
	}
	life, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	initial, initialErr := snapshot()
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		current, candidate, valid := initial, initial, initialErr == nil
		var lastChange time.Time
		for {
			select {
			case <-life.Done():
				return
			case <-ticker.C:
				next, err := snapshot()
				if err != nil {
					continue
				}
				if !valid {
					current, candidate, valid = next, next, true
					continue
				}
				if next == current {
					candidate = next
					continue
				}
				if next != candidate {
					candidate = next
					continue
				}
				if time.Since(lastChange) < quiet {
					continue
				}
				current = next
				lastChange = time.Now()
				changed()
			}
		}
	}()
	return func() { cancel(); <-done }
}

// ReplaceQUIC changes optional socket ownership on a network generation. Old
// requests can fail and resume verified chunks; no HTTP request is replayed here.
func (m *ConnectionManager) ReplaceQUIC(e *QUICEndpoint) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrClosed
	}
	old := m.quicEndpoint
	m.quicEndpoint = e
	m.generation++
	m.observations = map[Target]Observation{}
	m.iceFailures = map[Target]error{}
	m.pruneLocked()
	m.mu.Unlock()
	if old != nil && old != e {
		_ = old.Close()
	}
	return nil
}
