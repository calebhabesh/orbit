package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/testkit"
)

// R01: the rollup survives a restart and is owner-only.
func TestFinalR01RouteUsageSurvivesRestart(t *testing.T) {
	dir := testkit.NewDisposable(t)
	_ = os.Chmod(dir, 0700)
	if records, err := LoadRouteUsage(dir); err != nil || records != nil {
		t.Fatal("missing record", records, err)
	}
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	device := "0101010101010101010101010101010101010101010101010101010101010101"
	before := network.NewRouteUsage(func() time.Time { return now })
	if err := before.Restore([]network.RouteUsageRecord{{Start: now.Unix(), Device: device, Purpose: "peer_data", Route: network.RouteRelay, Requests: 3, Sent: 30, Received: 3000}}); err != nil {
		t.Fatal(err)
	}
	records, _ := before.Records()
	if err := SaveRouteUsage(dir, records); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, RouteUsageFile))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("route usage file not owner-only", info, err)
	}
	loaded, err := LoadRouteUsage(dir)
	if err != nil {
		t.Fatal(err)
	}
	after := network.NewRouteUsage(func() time.Time { return now })
	if err = after.Restore(loaded); err != nil {
		t.Fatal(err)
	}
	got := after.Window(now.Add(-time.Hour))
	if len(got) != 1 || got[0].Routes[network.RouteRelay] != (network.RouteTotals{Requests: 3, Sent: 30, Received: 3000}) {
		t.Fatalf("restart lost route usage: %+v", got)
	}
	if err = os.WriteFile(filepath.Join(dir, RouteUsageFile), []byte(`{"version":"1","records":[],"extra":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadRouteUsage(dir); err == nil {
		t.Fatal("unknown field accepted")
	}
}
