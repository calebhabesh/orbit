package designgates

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/testkit"
	_ "modernc.org/sqlite"
)

var (
	ErrUnsupportedSchemaVersion = errors.New("unsupported newer database schema version: rollback limit reached")
	ErrMandatoryCapUnsupported  = errors.New("mandatory capability unsupported by remote peer")
)

// ProductSettings represents Orbit UI and presentation preferences,
// kept strictly separate from the core engine's config.json.
type ProductSettings struct {
	FormatVersion    int               `json:"format_version"`
	DeviceLabel      string            `json:"device_label"`
	DefaultWorkspace string            `json:"default_workspace,omitempty"`
	WorkspaceNames   map[string]string `json:"workspace_names,omitempty"`
	DarkTheme        bool              `json:"dark_theme,omitempty"`
	TelemetryOptIn   bool              `json:"telemetry_opt_in,omitempty"`
}

// TestOrbitG05ProductSettingsSeparation tests that product preferences
// do not pollute or violate config.json strict validation.
func TestOrbitG05ProductSettingsSeparation(t *testing.T) {
	stateDir := testkit.NewDisposable(t)

	settingsPath := filepath.Join(stateDir, "settings.json")

	validHexID := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	coreCfg := config.Config{
		FormatVersion: 1,
		DeviceID:      validHexID,
		CreatedAt:     time.Now().UTC(),
	}
	if err := config.Save(stateDir, coreCfg); err != nil {
		t.Fatalf("failed to save config.json: %v", err)
	}

	// Product settings stored independently
	prodSettings := ProductSettings{
		FormatVersion:    1,
		DeviceLabel:      "Living Room Laptop",
		DefaultWorkspace: "ws-main",
		WorkspaceNames: map[string]string{
			"ws-main": "Personal Docs",
		},
		DarkTheme: true,
	}
	settingsBytes, err := json.MarshalIndent(prodSettings, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, settingsBytes, 0600); err != nil {
		t.Fatal(err)
	}

	// Load core config with strict parser
	loadedCore, err := config.Load(stateDir)
	if err != nil {
		t.Fatalf("strict config.Load failed: %v", err)
	}
	if loadedCore.DeviceID != coreCfg.DeviceID {
		t.Fatal("device ID mismatch")
	}

	// Corrupting settings.json does not prevent core config loading or daemon start
	_ = os.WriteFile(settingsPath, []byte("invalid-json{"), 0600)
	loadedCoreAfter, err := config.Load(stateDir)
	if err != nil || loadedCoreAfter.DeviceID != coreCfg.DeviceID {
		t.Fatal("corrupt settings.json broke core config.Load")
	}
}

// CapabilityNegotiator models protocol feature negotiation between mixed-version peers.
type CapabilityNegotiator struct {
	SupportedCaps map[string]bool
}

type HandshakeMessage struct {
	ClientVersion string   `json:"client_version"`
	Capabilities  []string `json:"capabilities"`
	MandatoryCaps []string `json:"mandatory_caps,omitempty"`
}

func (cn *CapabilityNegotiator) Negotiate(remote HandshakeMessage) (activeCaps []string, err error) {
	// Verify all remote mandatory capabilities are supported locally
	for _, m := range remote.MandatoryCaps {
		if !cn.SupportedCaps[m] {
			return nil, ErrMandatoryCapUnsupported
		}
	}
	for _, c := range remote.Capabilities {
		if cn.SupportedCaps[c] {
			activeCaps = append(activeCaps, c)
		}
	}
	return activeCaps, nil
}

// TestOrbitG05ProtocolCapabilityNegotiation tests that Orbit peers can negotiate capabilities
// with legacy peers without protocol failure.
func TestOrbitG05ProtocolCapabilityNegotiation(t *testing.T) {
	orbitNode := &CapabilityNegotiator{
		SupportedCaps: map[string]bool{
			"base_sync_v1":         true,
			"orbit_enrollment_v1":  true,
			"orbit_read_leases_v1": true,
		},
	}

	// Case 1: Legacy peer only supports base_sync_v1
	legacyHandshake := HandshakeMessage{
		ClientVersion: "orbit-1.0.0",
		Capabilities:  []string{"base_sync_v1"},
	}
	active, err := orbitNode.Negotiate(legacyHandshake)
	if err != nil {
		t.Fatalf("negotiation with legacy peer failed: %v", err)
	}
	if len(active) != 1 || active[0] != "base_sync_v1" {
		t.Fatalf("expected only base_sync_v1 active, got: %v", active)
	}

	// Case 2: Orbit peer supports all Orbit extensions
	orbitHandshake := HandshakeMessage{
		ClientVersion: "orbit-2.0.0",
		Capabilities:  []string{"base_sync_v1", "orbit_enrollment_v1", "orbit_read_leases_v1"},
	}
	activeOrbit, err := orbitNode.Negotiate(orbitHandshake)
	if err != nil {
		t.Fatalf("negotiation with orbit peer failed: %v", err)
	}
	if len(activeOrbit) != 3 {
		t.Fatalf("expected 3 active capabilities, got: %v", activeOrbit)
	}

	// Case 3: Future peer with unsupported mandatory capability
	futureHandshake := HandshakeMessage{
		ClientVersion: "orbit-9.0.0",
		Capabilities:  []string{"base_sync_v1", "future_quantum_crypto_v9"},
		MandatoryCaps: []string{"future_quantum_crypto_v9"},
	}
	_, err = orbitNode.Negotiate(futureHandshake)
	if !errors.Is(err, ErrMandatoryCapUnsupported) {
		t.Fatalf("expected ErrMandatoryCapUnsupported, got: %v", err)
	}
}

// TestOrbitG05LegacyStateAdoptionAndRollbackLimits tests Invariant I20:
// an existing valid metadata database is adopted cleanly, but an unsupported newer
// database schema version (future rollback limit) is rejected to preserve recoverable state.
func TestOrbitG05LegacyStateAdoptionAndRollbackLimits(t *testing.T) {
	stateDir := testkit.NewDisposable(t)

	dbPath := filepath.Join(stateDir, "metadata.sqlite")

	maxSupportedSchemaVersion := 5

	// Case 1: Legacy database at schema version 5
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("PRAGMA user_version = 5;")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	checkSchemaCompatibility := func(path string) error {
		d, err := sql.Open("sqlite", path)
		if err != nil {
			return err
		}
		defer d.Close()

		var v int
		if err := d.QueryRow("PRAGMA user_version;").Scan(&v); err != nil {
			return err
		}
		if v > maxSupportedSchemaVersion {
			return ErrUnsupportedSchemaVersion
		}
		return nil
	}

	if err := checkSchemaCompatibility(dbPath); err != nil {
		t.Fatalf("expected version 5 to be supported, got: %v", err)
	}

	// Case 2: Future database at schema version 99 (simulating rollback attempt)
	dbFuture, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = dbFuture.Exec("PRAGMA user_version = 99;")
	if err != nil {
		t.Fatal(err)
	}
	dbFuture.Close()

	err = checkSchemaCompatibility(dbPath)
	if !errors.Is(err, ErrUnsupportedSchemaVersion) {
		t.Fatalf("expected ErrUnsupportedSchemaVersion on newer user_version, got: %v", err)
	}
}
