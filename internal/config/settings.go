package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	SettingsFormatVersion = 1
	SettingsFilename      = "settings.json"
	MaxDeviceLabelLength  = 256
	MaxWorkspaceNameLen   = 256
	MaxSettingsFileSize   = 64 * 1024 // 64 KiB limit
)

var (
	ErrInvalidSettings = errors.New("invalid product settings")
)

// ProductSettings represents Orbit UI and presentation preferences,
// stored independently in settings.json to preserve strict config.json validation.
type ProductSettings struct {
	FormatVersion    int               `json:"format_version"`
	DeviceLabel      string            `json:"device_label"`
	DefaultWorkspace string            `json:"default_workspace,omitempty"`
	WorkspaceNames   map[string]string `json:"workspace_names,omitempty"`
	Theme            string            `json:"theme,omitempty"` // "dark", "light", "system"
	DarkTheme        bool              `json:"dark_theme,omitempty"`
}

// DefaultSettings returns initial product settings.
func DefaultSettings() ProductSettings {
	return ProductSettings{
		FormatVersion:  SettingsFormatVersion,
		DeviceLabel:    "",
		WorkspaceNames: make(map[string]string),
		Theme:          "dark",
		DarkTheme:      true,
	}
}

// ValidateSettings checks that settings fields conform to bounds and version rules.
func ValidateSettings(s ProductSettings) error {
	if s.FormatVersion != SettingsFormatVersion {
		return fmt.Errorf("%w: unsupported format_version %d (supported: %d)", ErrInvalidSettings, s.FormatVersion, SettingsFormatVersion)
	}
	if len(s.DeviceLabel) > MaxDeviceLabelLength {
		return fmt.Errorf("%w: device_label exceeds %d characters", ErrInvalidSettings, MaxDeviceLabelLength)
	}
	for wsID, name := range s.WorkspaceNames {
		if len(wsID) > 64 {
			return fmt.Errorf("%w: workspace ID %q exceeds 64 characters", ErrInvalidSettings, wsID)
		}
		if len(name) > MaxWorkspaceNameLen {
			return fmt.Errorf("%w: workspace name for %q exceeds %d characters", ErrInvalidSettings, wsID, MaxWorkspaceNameLen)
		}
	}
	if s.Theme != "" && s.Theme != "dark" && s.Theme != "light" && s.Theme != "system" {
		return fmt.Errorf("%w: invalid theme %q (must be dark, light, or system)", ErrInvalidSettings, s.Theme)
	}
	return nil
}

// LoadSettings loads product display settings from stateDir/settings.json.
// If settings.json does not exist, it returns DefaultSettings() without error.
// If settings.json is corrupted or invalid, it returns DefaultSettings() without breaking daemon startup (G05).
func LoadSettings(stateDir string) (ProductSettings, error) {
	path := filepath.Join(stateDir, SettingsFilename)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return DefaultSettings(), fmt.Errorf("inspect settings: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > MaxSettingsFileSize {
		// Non-regular file, insecure permissions, or oversized file: fallback to defaults
		return DefaultSettings(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultSettings(), nil
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var s ProductSettings
	if err := decoder.Decode(&s); err != nil {
		// Corrupt settings file falls back to defaults without breaking startup
		return DefaultSettings(), nil
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return DefaultSettings(), nil
	}

	if s.FormatVersion != SettingsFormatVersion {
		return DefaultSettings(), nil
	}
	if s.WorkspaceNames == nil {
		s.WorkspaceNames = make(map[string]string)
	}
	if s.Theme == "" {
		if s.DarkTheme {
			s.Theme = "dark"
		} else {
			s.Theme = "light"
		}
	}

	return s, nil
}

// LoadSettingsStrict loads settings and returns an error if the file is invalid or corrupt.
func LoadSettingsStrict(stateDir string) (ProductSettings, error) {
	path := filepath.Join(stateDir, SettingsFilename)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return ProductSettings{}, fmt.Errorf("inspect settings: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return ProductSettings{}, fmt.Errorf("settings %q must be a private regular file (mode 0600)", path)
	}
	if info.Size() > MaxSettingsFileSize {
		return ProductSettings{}, fmt.Errorf("settings file exceeds maximum size (%d bytes)", MaxSettingsFileSize)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ProductSettings{}, fmt.Errorf("read settings: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var s ProductSettings
	if err := decoder.Decode(&s); err != nil {
		return ProductSettings{}, fmt.Errorf("decode settings: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return ProductSettings{}, errors.New("settings contain trailing data")
	}

	if err := ValidateSettings(s); err != nil {
		return ProductSettings{}, err
	}

	if s.WorkspaceNames == nil {
		s.WorkspaceNames = make(map[string]string)
	}
	return s, nil
}

// SaveSettings atomically writes product display preferences to stateDir/settings.json with mode 0600.
func SaveSettings(stateDir string, s ProductSettings) error {
	if err := ValidateSettings(s); err != nil {
		return err
	}
	if s.WorkspaceNames == nil {
		s.WorkspaceNames = make(map[string]string)
	}
	if s.Theme == "dark" {
		s.DarkTheme = true
	} else if s.Theme == "light" {
		s.DarkTheme = false
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	data = append(data, '\n')

	temporary, err := os.CreateTemp(stateDir, ".settings-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary settings: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure temporary settings: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write settings: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("flush settings: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close settings: %w", err)
	}

	targetPath := filepath.Join(stateDir, SettingsFilename)
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		return fmt.Errorf("install settings: %w", err)
	}
	if dirFile, err := os.Open(stateDir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
	return nil
}

// UpdateDeviceLabel updates the human device label in settings.json without rotating identity.
func UpdateDeviceLabel(stateDir, label string) error {
	s, err := LoadSettingsStrict(stateDir)
	if err != nil {
		s = DefaultSettings()
	}
	s.DeviceLabel = label
	return SaveSettings(stateDir, s)
}

// UpdateWorkspaceName updates the human name for a workspace in settings.json.
func UpdateWorkspaceName(stateDir, workspaceID, name string) error {
	s, err := LoadSettingsStrict(stateDir)
	if err != nil {
		s = DefaultSettings()
	}
	if s.WorkspaceNames == nil {
		s.WorkspaceNames = make(map[string]string)
	}
	s.WorkspaceNames[workspaceID] = name
	return SaveSettings(stateDir, s)
}

// UpdateDefaultWorkspace sets the default workspace in settings.json.
func UpdateDefaultWorkspace(stateDir, workspaceID string) error {
	s, err := LoadSettingsStrict(stateDir)
	if err != nil {
		s = DefaultSettings()
	}
	s.DefaultWorkspace = workspaceID
	return SaveSettings(stateDir, s)
}
