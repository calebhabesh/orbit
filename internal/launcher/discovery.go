package launcher

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/state"
	_ "modernc.org/sqlite"
)

var (
	ErrMultipleStatesFound = errors.New("multiple state directories found; specify which to use with --state")
	ErrInvalidState        = errors.New("invalid existing state directory")
)

type StateCandidate struct {
	Path        string
	Exists      bool
	Initialized bool
	HasDB       bool
	HasLock     bool
}

// DiscoverState discovers the state directory without guessing when multiple states exist.
func DiscoverState(explicitDir string) (string, error) {
	if explicitDir != "" {
		clean, err := filepath.Abs(explicitDir)
		if err != nil {
			return "", fmt.Errorf("resolve state path: %w", err)
		}
		return clean, nil
	}

	// Candidate 1: Default XDG state dir (~/.local/state/filesync or $XDG_STATE_HOME/filesync)
	defaultDir := config.DefaultStateDir()

	// Candidate 2: Legacy state dir (~/.filesync)
	var legacyDir string
	if home, err := os.UserHomeDir(); err == nil {
		legacyDir = filepath.Join(home, ".filesync")
	}

	candDefault := inspectCandidate(defaultDir)
	candLegacy := inspectCandidate(legacyDir)

	// If both have state (initialized or DB exists or lock held)
	if candDefault.HasState() && candLegacy.HasState() && defaultDir != legacyDir {
		return "", fmt.Errorf("%w: default state (%s) and legacy state (%s) both exist",
			ErrMultipleStatesFound, defaultDir, legacyDir)
	}

	if candLegacy.HasState() {
		return legacyDir, nil
	}
	return defaultDir, nil
}

func inspectCandidate(dir string) StateCandidate {
	if dir == "" {
		return StateCandidate{}
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return StateCandidate{Path: dir, Exists: false}
	}
	_, cfgErr := os.Stat(filepath.Join(dir, "config.json"))
	_, dbErr := os.Stat(filepath.Join(dir, "metadata.sqlite"))
	_, lockErr := os.Stat(filepath.Join(dir, ".agent.lock"))

	return StateCandidate{
		Path:        dir,
		Exists:      true,
		Initialized: cfgErr == nil,
		HasDB:       dbErr == nil,
		HasLock:     lockErr == nil,
	}
}

func (c StateCandidate) HasState() bool {
	return c.Exists && (c.Initialized || c.HasDB || c.HasLock)
}

// ValidateExistingState verifies that an existing state directory is valid
// and prevents running or initializing over corrupted or incompatible state.
func ValidateExistingState(stateDir string) error {
	info, err := os.Stat(stateDir)
	if errors.Is(err, os.ErrNotExist) {
		// Clean non-existent directory is safe
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect state directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("state path %s is not a directory", stateDir)
	}

	// 1. Directory permissions & ownership
	if err := state.ValidateDirectory(stateDir); err != nil {
		return err
	}

	// 2. Check config.json if present
	cfgPath := filepath.Join(stateDir, "config.json")
	if _, err := os.Stat(cfgPath); err == nil {
		if _, err := config.Load(stateDir); err != nil {
			return fmt.Errorf("%w in %s: %v", ErrInvalidState, stateDir, err)
		}
	}

	// 3. Check startup recovery consistency if metadata.sqlite exists
	dbPath := filepath.Join(stateDir, "metadata.sqlite")
	if _, err := os.Stat(dbPath); err == nil {
		if err := control.VerifyRecoveryConsistency(stateDir); err != nil {
			return fmt.Errorf("%w in %s: %v", ErrInvalidState, stateDir, err)
		}
		// Also verify schema version is not newer than CurrentSchema
		dbDSN := "file:" + dbPath + "?mode=ro&_pragma=busy_timeout(5000)"
		db, err := sql.Open("sqlite", dbDSN)
		if err != nil {
			return fmt.Errorf("%w in %s: %v", ErrInvalidState, stateDir, err)
		}
		var userVersion int
		err = db.QueryRow("PRAGMA user_version").Scan(&userVersion)
		_ = db.Close()
		if err != nil {
			return fmt.Errorf("%w in %s: query schema version: %v", ErrInvalidState, stateDir, err)
		}
		if userVersion > repository.CurrentSchema {
			return fmt.Errorf("%w in %s: database schema version %d is newer than supported %d", ErrInvalidState, stateDir, userVersion, repository.CurrentSchema)
		}
	}

	return nil
}
