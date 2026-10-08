package config

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	FormatVersion = 1
	filename      = "config.json"
)

type Config struct {
	FormatVersion int       `json:"format_version"`
	DeviceID      string    `json:"device_id"`
	CreatedAt     time.Time `json:"created_at"`
}

func DefaultStateDir() string {
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		return filepath.Join(stateHome, "orbit")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".orbit-state"
	}
	return filepath.Join(home, ".local", "state", "orbit")
}

func Initialize(stateDir string, now func() time.Time, random io.Reader) (Config, error) {
	cfg, err := Load(stateDir)
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}
	if now == nil || random == nil {
		return Config{}, errors.New("clock and randomness dependencies are required")
	}
	rawID := make([]byte, 32)
	if _, err := io.ReadFull(random, rawID); err != nil {
		return Config{}, fmt.Errorf("create device identity: %w", err)
	}
	cfg = Config{
		FormatVersion: FormatVersion,
		DeviceID:      hex.EncodeToString(rawID),
		CreatedAt:     now().UTC(),
	}
	if err := Save(stateDir, cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Save atomically writes a validated configuration file to stateDir with mode 0600.
func Save(stateDir string, cfg Config) error {
	path := filepath.Join(stateDir, filename)
	if cfg.FormatVersion != FormatVersion {
		return fmt.Errorf("unsupported configuration format %d (supported: %d)", cfg.FormatVersion, FormatVersion)
	}
	decodedID, err := hex.DecodeString(cfg.DeviceID)
	if err != nil || len(decodedID) != 32 {
		return errors.New("configuration has an invalid device identity")
	}
	if cfg.CreatedAt.IsZero() {
		return errors.New("configuration has no creation time")
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode configuration: %w", err)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(stateDir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary configuration: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure temporary configuration: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write configuration: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("flush configuration: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close configuration: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("install configuration: %w", err)
	}
	if dirFile, err := os.Open(stateDir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
	return nil
}

func Load(stateDir string) (Config, error) {
	path := filepath.Join(stateDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read configuration: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return Config{}, fmt.Errorf("inspect configuration: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return Config{}, fmt.Errorf("configuration %q must be a private regular file (mode 0600)", path)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode configuration: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return Config{}, errors.New("configuration contains trailing data")
	}
	if cfg.FormatVersion != FormatVersion {
		return Config{}, fmt.Errorf("unsupported configuration format %d (supported: %d)", cfg.FormatVersion, FormatVersion)
	}
	decodedID, err := hex.DecodeString(cfg.DeviceID)
	if err != nil || len(decodedID) != 32 {
		return Config{}, errors.New("configuration has an invalid device identity")
	}
	if cfg.CreatedAt.IsZero() {
		return Config{}, errors.New("configuration has no creation time")
	}
	return cfg, nil
}
