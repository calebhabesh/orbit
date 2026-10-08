package launcher

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/control"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/state"
	_ "modernc.org/sqlite"
)

var (
	ErrInvalidState = errors.New("invalid existing state directory")
)

// DiscoverState returns the explicit state directory, or the default one.
func DiscoverState(explicitDir string) (string, error) {
	if explicitDir != "" {
		clean, err := filepath.Abs(explicitDir)
		if err != nil {
			return "", fmt.Errorf("resolve state path: %w", err)
		}
		return clean, nil
	}

	return config.DefaultStateDir(), nil
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
