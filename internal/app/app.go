package app

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/state"
)

// Dependencies makes identity creation deterministic in tests without weakening
// production randomness or introducing a global clock.
type Dependencies struct {
	Now    func() time.Time
	Random io.Reader
}

func SystemDependencies() Dependencies {
	return Dependencies{Now: time.Now, Random: rand.Reader}
}

func Initialize(ctx context.Context, stateDir string, deps Dependencies) (config.Config, error) {
	if err := state.EnsureDirectory(stateDir); err != nil {
		return config.Config{}, err
	}
	lock, err := state.Acquire(stateDir)
	if err != nil {
		return config.Config{}, err
	}
	defer lock.Close()

	cfg, err := config.Initialize(stateDir, deps.Now, deps.Random)
	if err != nil {
		return config.Config{}, err
	}
	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		return config.Config{}, err
	}
	if err := db.Close(); err != nil {
		return config.Config{}, fmt.Errorf("close metadata database: %w", err)
	}
	return cfg, nil
}

func Serve(ctx context.Context, stateDir string, ready io.Writer) error {
	if err := state.ValidateDirectory(stateDir); err != nil {
		return err
	}
	lock, err := state.Acquire(stateDir)
	if err != nil {
		return err
	}
	defer lock.Close()

	cfg, err := config.Load(stateDir)
	if err != nil {
		return err
	}
	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		return err
	}
	defer db.Close()

	fmt.Fprintf(ready, "agent ready: device=%s schema=%d\n", cfg.DeviceID, repository.CurrentSchema)
	<-ctx.Done()
	return nil
}
