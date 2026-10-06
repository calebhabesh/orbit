package config_test

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/replication"
)

func TestWANW05DurableRoutesPrivateBoundedAndLegacyPreserved(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	var device history.ID
	device[0] = 1
	id, err := replication.LoadOrCreateIdentity(dir, device, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{"format_version":1,"peers":[]}`)
	if err = config.WritePrivate(dir, config.PeersFilename, legacy); err != nil {
		t.Fatal(err)
	}
	route := config.PeerRoute{Folder: strings.Repeat("1", 64), Device: hex.EncodeToString(device[:]), Pin: hex.EncodeToString(id.KeyPin[:]), Profile: strings.Repeat("2", 64), CertificateDER: base64.StdEncoding.EncodeToString(id.Leaf.Raw)}
	for i := 0; i < config.MaxPeerEndpoints; i++ {
		route.Folder = fmt.Sprintf("%064x", i+1)
		if err = config.SetPeerRoute(dir, route); err != nil {
			t.Fatal(err)
		}
	}
	if err = config.SetPeerRoute(dir, route); err != nil {
		t.Fatal("identical route not idempotent", err)
	}
	route.Folder = strings.Repeat("f", 64)
	if err = config.SetPeerRoute(dir, route); err == nil {
		t.Fatal("over-limit routes persisted")
	}
	routes, err := config.LoadPeerRoutes(dir)
	if err != nil || len(routes) != config.MaxPeerEndpoints {
		t.Fatal("durable bound changed", err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "peer-routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	changed := routes[0]
	changed.Profile = strings.Repeat("3", 64)
	if err = config.SetPeerRoute(dir, changed); err == nil {
		t.Fatal("reviewed profile overwritten")
	}
	changed = routes[0]
	changed.Pin = strings.Repeat("3", 64)
	if err = config.SetPeerRoute(dir, changed); err == nil {
		t.Fatal("reviewed pin overwritten")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "peer-routes.json"))
	if string(before) != string(after) {
		t.Fatal("failed update changed durable routes")
	}
	info, err := os.Stat(filepath.Join(dir, "peer-routes.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("routes not private")
	}
	got, _ := os.ReadFile(filepath.Join(dir, config.PeersFilename))
	if string(got) != string(legacy) {
		t.Fatal("legacy peers rewritten")
	}
	if err = os.Chmod(filepath.Join(dir, "peer-routes.json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = config.LoadPeerRoutes(dir); err == nil {
		t.Fatal("public route file accepted")
	}
}

// Reviewing the next profile epoch keeps existing pairings routable; identities,
// pins and certificates are unchanged.
func TestWANW13RotationRebindsDurableRoutes(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	var device history.ID
	device[0] = 7
	id, err := replication.LoadOrCreateIdentity(dir, device, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = config.RebindPeerRoutes(dir, strings.Repeat("3", 64)); err != nil {
		t.Fatal("rebinding without routes", err)
	}
	route := config.PeerRoute{Folder: strings.Repeat("1", 64), Device: hex.EncodeToString(device[:]), Pin: hex.EncodeToString(id.KeyPin[:]), Profile: strings.Repeat("2", 64), CertificateDER: base64.StdEncoding.EncodeToString(id.Leaf.Raw)}
	if err = config.SetPeerRoute(dir, route); err != nil {
		t.Fatal(err)
	}
	if err = config.RebindPeerRoutes(dir, "not-a-digest"); err == nil {
		t.Fatal("invalid digest accepted")
	}
	if err = config.RebindPeerRoutes(dir, strings.Repeat("3", 64)); err != nil {
		t.Fatal(err)
	}
	routes, err := config.LoadPeerRoutes(dir)
	if err != nil || len(routes) != 1 {
		t.Fatal(err)
	}
	want := route
	want.Profile = strings.Repeat("3", 64)
	if routes[0] != want {
		t.Fatalf("rebind changed more than the profile: %+v", routes[0])
	}
	// A later enrollment under the new epoch reuses the same route.
	if err = config.SetPeerRoute(dir, want); err != nil {
		t.Fatal(err)
	}
}
