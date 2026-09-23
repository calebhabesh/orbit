package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/state"
	"github.com/calebhabesh/file-sync/internal/workspace"
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
	deviceID, err := decodeDeviceID(cfg.DeviceID)
	if err != nil {
		return config.Config{}, err
	}
	if _, err := replication.LoadOrCreateIdentity(stateDir, deviceID, deps.Now()); err != nil {
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

func Serve(ctx context.Context, stateDir, peerAddress string, ready io.Writer) error {
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
	deviceID, err := decodeDeviceID(cfg.DeviceID)
	if err != nil {
		return err
	}
	identity, err := replication.LoadOrCreateIdentity(stateDir, deviceID, time.Now())
	if err != nil {
		return err
	}

	if peerAddress == "" {
		fmt.Fprintf(ready, "agent ready: device=%s schema=%d peer-listener=disabled\n", cfg.DeviceID, repository.CurrentSchema)
		<-ctx.Done()
		return nil
	}
	listener, err := net.Listen("tcp", peerAddress)
	if err != nil {
		return fmt.Errorf("listen for peers: %w", err)
	}
	defer listener.Close()
	fmt.Fprintf(ready, "agent ready: device=%s schema=%d peer-listener=%s key-pin=%x\n", cfg.DeviceID, repository.CurrentSchema, listener.Addr(), identity.KeyPin)
	return replication.NewServer(db, identity).Serve(ctx, listener)
}

// WithWorkspace gives local CLI operations the same locked repository/workspace
// boundary used by the agent. The callback must not retain either handle.
func WithWorkspace(ctx context.Context, stateDir string, run func(config.Config, *repository.DB, *workspace.Workspace) error) error {
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
	return run(cfg, db, workspace.New(db, workspace.Options{}))
}

func decodeDeviceID(text string) (history.ID, error) {
	var id history.ID
	raw, err := hex.DecodeString(text)
	if err != nil || len(raw) != len(id) {
		return id, errors.New("configured device ID is invalid")
	}
	copy(id[:], raw)
	return id, nil
}
