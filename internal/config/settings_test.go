package config_test

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/testkit"
)

// TestOrbitSettings_LoadAndSaveRoundtrip tests saving and loading product settings.
func TestOrbitSettings_LoadAndSaveRoundtrip(t *testing.T) {
	stateDir := testkit.NewDisposable(t)

	// Missing settings file returns default settings without error
	defaults, err := config.LoadSettings(stateDir)
	if err != nil {
		t.Fatalf("unexpected error on missing settings: %v", err)
	}
	if defaults.FormatVersion != config.SettingsFormatVersion {
		t.Errorf("expected format version %d, got %d", config.SettingsFormatVersion, defaults.FormatVersion)
	}
	if !defaults.DarkTheme {
		t.Errorf("expected default DarkTheme=true")
	}

	// Save custom settings
	custom := config.ProductSettings{
		FormatVersion:    config.SettingsFormatVersion,
		DeviceLabel:      "Living Room Laptop",
		DefaultWorkspace: "ws-1",
		WorkspaceNames: map[string]string{
			"ws-1": "Personal Docs",
			"ws-2": "Shared Work",
		},
		Theme:     "dark",
		DarkTheme: true,
	}
	if err := config.SaveSettings(stateDir, custom); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	// Verify file permissions (0600)
	settingsPath := filepath.Join(stateDir, config.SettingsFilename)
	info, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected permissions 0600, got %o", info.Mode().Perm())
	}

	// Load settings and verify roundtrip
	loaded, err := config.LoadSettingsStrict(stateDir)
	if err != nil {
		t.Fatalf("LoadSettingsStrict failed: %v", err)
	}
	if loaded.DeviceLabel != custom.DeviceLabel {
		t.Errorf("DeviceLabel = %q, want %q", loaded.DeviceLabel, custom.DeviceLabel)
	}
	if loaded.DefaultWorkspace != custom.DefaultWorkspace {
		t.Errorf("DefaultWorkspace = %q, want %q", loaded.DefaultWorkspace, custom.DefaultWorkspace)
	}
	if loaded.WorkspaceNames["ws-1"] != "Personal Docs" || loaded.WorkspaceNames["ws-2"] != "Shared Work" {
		t.Errorf("WorkspaceNames mismatch: got %v", loaded.WorkspaceNames)
	}
}

// TestOrbitSettings_InvalidRejectedWithoutPartialConfig verifies that invalid settings
// are rejected without partial corruption.
func TestOrbitSettings_InvalidRejectedWithoutPartialConfig(t *testing.T) {
	stateDir := testkit.NewDisposable(t)

	// Save valid settings first
	valid := config.ProductSettings{
		FormatVersion: config.SettingsFormatVersion,
		DeviceLabel:   "Original Device",
		WorkspaceNames: map[string]string{
			"ws-1": "Valid WS",
		},
		Theme: "dark",
	}
	if err := config.SaveSettings(stateDir, valid); err != nil {
		t.Fatal(err)
	}

	// Attempt to save invalid settings (wrong format_version)
	invalidVersion := valid
	invalidVersion.FormatVersion = 99
	err := config.SaveSettings(stateDir, invalidVersion)
	if !errors.Is(err, config.ErrInvalidSettings) {
		t.Fatalf("expected ErrInvalidSettings on invalid version, got: %v", err)
	}

	// Attempt to save invalid theme
	invalidTheme := valid
	invalidTheme.Theme = "neon-rainbow"
	err = config.SaveSettings(stateDir, invalidTheme)
	if !errors.Is(err, config.ErrInvalidSettings) {
		t.Fatalf("expected ErrInvalidSettings on invalid theme, got: %v", err)
	}

	// Ensure the original settings were NOT modified or corrupted
	loaded, err := config.LoadSettingsStrict(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DeviceLabel != "Original Device" {
		t.Errorf("settings were modified despite failed SaveSettings: %v", loaded)
	}
}

// TestOrbitSettings_CorruptFallback verifies that a corrupted settings.json
// falls back to defaults without failing or breaking core config loading.
func TestOrbitSettings_CorruptFallback(t *testing.T) {
	stateDir := testkit.NewDisposable(t)

	rawID := make([]byte, 32)
	rawID[0] = 0x42
	coreCfg := config.Config{
		FormatVersion: config.FormatVersion,
		DeviceID:      hex.EncodeToString(rawID),
		CreatedAt:     time.Now().UTC(),
	}
	if err := config.Save(stateDir, coreCfg); err != nil {
		t.Fatal(err)
	}

	// Corrupt settings file
	settingsPath := filepath.Join(stateDir, config.SettingsFilename)
	if err := os.WriteFile(settingsPath, []byte("broken JSON {{{{"), 0o600); err != nil {
		t.Fatal(err)
	}

	// LoadSettings falls back to defaults
	settings, err := config.LoadSettings(stateDir)
	if err != nil {
		t.Fatalf("LoadSettings failed on corrupted file: %v", err)
	}
	if settings.FormatVersion != config.SettingsFormatVersion {
		t.Errorf("expected default format version, got %d", settings.FormatVersion)
	}

	// Core config loads cleanly regardless
	loadedCore, err := config.Load(stateDir)
	if err != nil {
		t.Fatalf("core config.Load failed due to corrupt settings: %v", err)
	}
	if loadedCore.DeviceID != coreCfg.DeviceID {
		t.Errorf("core device ID mismatch: %s vs %s", loadedCore.DeviceID, coreCfg.DeviceID)
	}
}

// TestOrbitSettings_LabelChangeDoesNotRotateIdentity verifies that updating product labels
// preserves the core device identity, keys, and format_version.
func TestOrbitSettings_LabelChangeDoesNotRotateIdentity(t *testing.T) {
	stateDir := testkit.NewDisposable(t)

	rawID := make([]byte, 32)
	rawID[0] = 0xAA
	coreCfg := config.Config{
		FormatVersion: config.FormatVersion,
		DeviceID:      hex.EncodeToString(rawID),
		CreatedAt:     time.Now().UTC(),
	}
	if err := config.Save(stateDir, coreCfg); err != nil {
		t.Fatal(err)
	}

	// Update device label
	if err := config.UpdateDeviceLabel(stateDir, "My Primary Workstation"); err != nil {
		t.Fatalf("UpdateDeviceLabel failed: %v", err)
	}

	// Update workspace name
	if err := config.UpdateWorkspaceName(stateDir, "ws-alpha", "Alpha Workspace"); err != nil {
		t.Fatalf("UpdateWorkspaceName failed: %v", err)
	}

	// Verify settings updated
	s, err := config.LoadSettingsStrict(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if s.DeviceLabel != "My Primary Workstation" {
		t.Errorf("DeviceLabel = %q, want 'My Primary Workstation'", s.DeviceLabel)
	}
	if s.WorkspaceNames["ws-alpha"] != "Alpha Workspace" {
		t.Errorf("WorkspaceNames['ws-alpha'] = %q, want 'Alpha Workspace'", s.WorkspaceNames["ws-alpha"])
	}

	// Core config MUST be untouched
	loadedCore, err := config.Load(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if loadedCore.DeviceID != coreCfg.DeviceID {
		t.Fatalf("identity was rotated! got %s, want %s", loadedCore.DeviceID, coreCfg.DeviceID)
	}
	if loadedCore.FormatVersion != 1 {
		t.Fatalf("format version changed: %d", loadedCore.FormatVersion)
	}
}

// TestOrbitSettings_DuplicateLabelsAllowed verifies that duplicate/conflicting labels
// across workspaces remain usable (owner-local aliases).
func TestOrbitSettings_DuplicateLabelsAllowed(t *testing.T) {
	stateDir := testkit.NewDisposable(t)

	settings := config.ProductSettings{
		FormatVersion: config.SettingsFormatVersion,
		DeviceLabel:   "Workstation",
		WorkspaceNames: map[string]string{
			"ws-1": "Projects",
			"ws-2": "Projects", // Duplicate label is permitted
		},
		Theme: "dark",
	}
	if err := config.SaveSettings(stateDir, settings); err != nil {
		t.Fatalf("SaveSettings failed with duplicate workspace names: %v", err)
	}

	loaded, err := config.LoadSettingsStrict(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.WorkspaceNames["ws-1"] != "Projects" || loaded.WorkspaceNames["ws-2"] != "Projects" {
		t.Errorf("duplicate workspace names not preserved: %v", loaded.WorkspaceNames)
	}
}
