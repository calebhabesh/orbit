package control

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The packaged install.sh writes ExecStart with systemd's %h home specifier;
// the selected state must match it after expansion, and only for that state.
func TestSelectedServiceAcceptsPackagedHomeSpecifier(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	packaged, err := os.ReadFile(filepath.Join("..", "..", "packaging", "systemd", "filesync.service"))
	if err != nil {
		t.Fatal(err)
	}
	unit := strings.ReplaceAll(string(packaged), "/usr/bin/filesync", filepath.Join(home, ".local", "bin", "filesync"))
	if !strings.Contains(unit, "--state=%h/") {
		t.Fatal("packaged unit no longer uses the %h specifier; update this regression")
	}
	if err := os.WriteFile(filepath.Join(unitDir, "filesync.service"), []byte(unit), 0o644); err != nil {
		t.Fatal(err)
	}
	selected := filepath.Join(home, ".local", "state", "filesync")
	if err := validateSelectedService(selected); err != nil {
		t.Fatalf("packaged unit for the selected state was refused: %v", err)
	}
	if err := validateSelectedService(selected + "/"); err != nil {
		t.Fatalf("equivalent selected path was refused: %v", err)
	}
	for _, other := range []string{filepath.Join(home, ".local", "state", "other"), filepath.Join(home, ".local", "state"), "/elsewhere/.local/state/filesync"} {
		if err := validateSelectedService(other); err == nil {
			t.Fatalf("unit for %s accepted for a different state %s", selected, other)
		}
	}
}

// Only the unit's own main process counts as service ownership of the state.
func TestStateOwnedByUnitRequiresUnitMainProcess(t *testing.T) {
	for _, c := range []struct {
		mainPID, agentPID string
		want              bool
	}{
		{"1234\n", "1234\n", true},
		{"0\n", "1234\n", false}, // unit inactive while a manual daemon holds the lock
		{"1235\n", "1234\n", false},
		{"", "1234\n", false},
		{"1234\n", "", false},
		{"1\n", "1\n", false},
	} {
		if got := stateOwnedByUnit(c.mainPID, c.agentPID); got != c.want {
			t.Errorf("stateOwnedByUnit(%q, %q) = %v, want %v", c.mainPID, c.agentPID, got, c.want)
		}
	}
}
