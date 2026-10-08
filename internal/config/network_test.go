package config

import (
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"os"
	"path/filepath"
	"testing"
)

func TestWANW02LegacyNetworkPolicyIsManualWithoutMigration(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	p, err := LoadNetworkPolicy(dir)
	if err != nil || p.Mode != "manual" || p.Generation != 1 || p.LANAdvertising || p.Profile != "" {
		t.Fatal(p, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "network.json")); !os.IsNotExist(err) {
		t.Fatal("load mutated legacy state")
	}
	for _, data := range []string{`{"mode":"manual","profile":"","lan_advertising":false,"generation":"0"}`, `{"mode":"manual","profile":"","lan_advertising":false,"generation":"1","unknown":true}`} {
		if err := WritePrivate(dir, "network.json", []byte(data)); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadNetworkPolicy(dir); err == nil {
			t.Fatal("invalid policy accepted")
		}
	}
}

func TestWANW06PolicyGenerationCannotRollbackOrChange(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	selected := tc.NetworkPolicy{Mode: "local_only", Generation: 2}
	if err := SaveNetworkPolicy(dir, selected); err != nil {
		t.Fatal(err)
	}
	if err := SaveNetworkPolicy(dir, selected); err != nil {
		t.Fatal("identical recovery failed", err)
	}
	for _, bad := range []tc.NetworkPolicy{{Mode: "manual", Generation: 1}, {Mode: "manual", Generation: 2}} {
		if err := SaveNetworkPolicy(dir, bad); err == nil {
			t.Fatal("stale policy overwrote reviewed intent")
		}
	}
	actual, err := LoadNetworkPolicy(dir)
	if err != nil || actual != selected {
		t.Fatal(actual, err)
	}
}
