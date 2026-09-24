package control

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/state"
)

// Folders returns all folders in the database.
func (c *Controller) Folders(ctx context.Context) ([]repository.FolderRecord, error) {
	return c.db.Folders(ctx)
}

// RegisterFolder validates and registers a workspace root directory for a folder.
func (c *Controller) RegisterFolder(ctx context.Context, folder history.ID, root string) (repository.RootRegistration, error) {
	if folder == ([32]byte{}) {
		return repository.RootRegistration{}, errors.New("folder ID cannot be zero")
	}
	if root == "" {
		return repository.RootRegistration{}, errors.New("root path cannot be empty")
	}
	if c.options.LocalDevice != ([32]byte{}) {
		if err := c.db.EnsureFolder(ctx, folder, c.options.LocalDevice, 1); err != nil {
			return repository.RootRegistration{}, err
		}
	}
	return c.ws.Register(ctx, folder, root)
}

// PauseFolder marks a folder as paused.
func (c *Controller) PauseFolder(ctx context.Context, folder history.ID, reason string) error {
	return c.ws.Pause(ctx, folder, reason)
}

// ResumeFolder unpauses a folder.
func (c *Controller) ResumeFolder(ctx context.Context, folder history.ID) error {
	return c.ws.Resume(ctx, folder)
}

// UnregisterFolder unregisters a folder root, preserving all user files on disk
// and without authoring or replicating any deletion tombstones.
func (c *Controller) UnregisterFolder(ctx context.Context, folder history.ID) error {
	return c.ws.Unregister(ctx, folder)
}

// RevalidateRoot checks that a folder's root directory is mounted, matches device/inode,
// and contains a valid private scratch registration marker.
func (c *Controller) RevalidateRoot(ctx context.Context, folder history.ID) error {
	return c.ws.Revalidate(ctx, folder)
}

// Backup creates a transactionally consistent copy of the SQLite database.
func (c *Controller) Backup(ctx context.Context, targetPath string) (*BackupResult, error) {
	if targetPath == "" {
		ts := time.Now().UTC().Format("20060102-150405")
		targetPath = filepath.Join(c.db.StateDir(), fmt.Sprintf("backup-%s.sqlite", ts))
	}
	targetPath, err := filepath.Abs(targetPath)
	if err != nil {
		return nil, err
	}
	if err := c.db.Backup(ctx, targetPath); err != nil {
		return nil, err
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		return nil, err
	}
	return &BackupResult{
		BackupPath: targetPath,
		SizeBytes:  info.Size(),
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// CheckMigration inspects the database schema version against binary CurrentSchema.
func (c *Controller) CheckMigration(ctx context.Context) (*MigrationCheckResult, error) {
	ver, err := c.db.UserVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("read database user_version: %w", err)
	}

	res := &MigrationCheckResult{
		CurrentDatabaseSchema: ver,
		BinarySchema:          repository.CurrentSchema,
	}

	if ver == repository.CurrentSchema {
		res.Status = "up_to_date"
		res.Action = "database schema is up to date"
	} else if ver < repository.CurrentSchema {
		res.Status = "migration_needed"
		res.Action = "run filesync serve or filesync init to apply pending migrations"
	} else {
		res.Status = "incompatible"
		res.Action = "upgrade filesync binary to match newer database schema"
	}
	return res, nil
}

// RecoveryInspection inspects journals, quarantine, and tasks requiring recovery.
func (c *Controller) RecoveryInspection(ctx context.Context) (*RecoveryInspectionResult, error) {
	result := &RecoveryInspectionResult{}

	// 1. Pending publications
	registered, err := c.db.RegisteredFolders(ctx)
	if err != nil {
		return nil, err
	}
	for _, reg := range registered {
		pubs, err := c.db.Publications(ctx, reg.Folder)
		if err == nil {
			for _, p := range pubs {
				if p.Phase != "COMMITTED" {
					result.PendingPublications++
					result.Details = append(result.Details, fmt.Sprintf("folder %s: publication %s (%s) phase=%s", shortID(reg.Folder), p.OperationID, p.Path, p.Phase))
				}
			}
		}
	}

	// 2. Exhausted tasks
	tasks, err := c.db.ListDurableTasks(ctx, repository.TaskFilter{State: "exhausted"})
	if err == nil {
		result.ExhaustedTasks = len(tasks)
		for _, t := range tasks {
			result.Details = append(result.Details, fmt.Sprintf("exhausted task %s (%s) error=%s: %s", t.ID, t.Kind, t.ErrorCode, t.LastError))
		}
	}

	// 3. Reclaimable recovery copies
	for _, reg := range registered {
		reclaim, err := c.ReclaimRecoveryCopies(ctx, ReclaimRecoveryRequest{Folder: reg.Folder})
		if err == nil && reclaim.ReclaimedCount > 0 {
			result.ReclaimableRecovery += reclaim.ReclaimedCount
			result.Details = append(result.Details, fmt.Sprintf("folder %s: %d recovery files (%d bytes) reclaimable", shortID(reg.Folder), reclaim.ReclaimedCount, reclaim.ReclaimedBytes))
		}
	}

	return result, nil
}

// ResetIdentity creates a fresh device identity and keypair, updating configuration
// without reusing rolled-back author counters.
func (c *Controller) ResetIdentity(ctx context.Context, targetStateDir string) (*ResetIdentityResult, error) {
	if targetStateDir == "" {
		targetStateDir = c.db.StateDir()
	}

	// Read existing config to obtain old device ID
	cfgPath := filepath.Join(targetStateDir, "config.json")
	var oldID history.ID
	if raw, err := os.ReadFile(cfgPath); err == nil {
		var cfg map[string]any
		if json.Unmarshal(raw, &cfg) == nil {
			if devStr, ok := cfg["device_id"].(string); ok {
				if d, err := hex.DecodeString(devStr); err == nil && len(d) == 32 {
					copy(oldID[:], d)
				}
			}
		}
	}

	// Generate fresh 32-byte device ID
	var newID history.ID
	if _, err := rand.Read(newID[:]); err != nil {
		return nil, fmt.Errorf("generate new device id: %w", err)
	}

	// Generate and save fresh TLS identity
	ident, err := replication.LoadOrCreateIdentity(targetStateDir, newID, time.Now())
	if err != nil {
		return nil, fmt.Errorf("generate new TLS identity: %w", err)
	}

	// Update config.json
	cfgData := map[string]any{
		"device_id": hex.EncodeToString(newID[:]),
		"version":   1,
	}
	updatedRaw, _ := json.MarshalIndent(cfgData, "", "  ")
	if err := os.WriteFile(cfgPath, updatedRaw, 0o600); err != nil {
		return nil, fmt.Errorf("write updated config: %w", err)
	}

	return &ResetIdentityResult{
		OldDeviceID: oldID,
		NewDeviceID: newID,
		NewKeyPin:   ident.KeyPin,
		Message:     "fresh device identity generated successfully; counters reset",
		Action:      "re-enroll new device ID in folder memberships on peers",
	}, nil
}

// RecordEvent records an operational event log.
func (c *Controller) RecordEvent(ctx context.Context, entry repository.EventLogEntry) error {
	return c.db.RecordEvent(ctx, entry)
}

// ListEvents queries recent operational event logs.
func (c *Controller) ListEvents(ctx context.Context, limit int) ([]repository.EventLogEntry, error) {
	return c.db.ListEvents(ctx, limit)
}

// Metrics aggregates operational metrics.
func (c *Controller) Metrics(ctx context.Context) (*OperationalMetrics, error) {
	usage, _ := c.db.DetailedStorageUsage(ctx)

	metrics := &OperationalMetrics{
		StagingBytes:    usage.Usage.Incoming,
		QuarantineBytes: usage.Usage.Quarantine,
		ObjectBytes:     usage.Usage.Objects,
		MetadataBytes:   usage.Usage.Metadata,
		RetryCauses:     make(map[string]uint64),
	}

	tasks, _ := c.db.ListDurableTasks(ctx, repository.TaskFilter{Limit: 1000})
	for _, t := range tasks {
		if t.State == "queued" || t.State == "running" || t.State == "retry" {
			metrics.QueueDepth++
		}
		if t.ErrorCode != "" {
			metrics.RetryCauses[t.ErrorCode]++
		}
	}

	// Count conflicts across all folders
	folders, _ := c.db.Folders(ctx)
	for _, f := range folders {
		conflicts, structural, err := c.Conflicts(ctx, f.Folder)
		if err == nil {
			metrics.ActiveConflicts += len(conflicts) + len(structural)
		}
	}

	return metrics, nil
}

// Preflight runs upgrade preflight checks on the active controller.
func (c *Controller) Preflight(ctx context.Context) (*PreflightResult, error) {
	return RunPreflight(ctx, c.db.StateDir())
}

// RunPreflight runs upgrade preflight checks against stateDir.
func RunPreflight(ctx context.Context, stateDir string) (*PreflightResult, error) {
	if stateDir == "" {
		return nil, errors.New("state directory cannot be empty")
	}

	result := &PreflightResult{
		Status:         "ready",
		StateDir:       stateDir,
		BinarySchema:   repository.CurrentSchema,
		IntegrityClean: true,
	}

	// 1. Validate state directory
	if err := state.ValidateDirectory(stateDir); err != nil {
		result.Status = "blocked"
		result.Issues = append(result.Issues, fmt.Sprintf("invalid state directory: %v", err))
		result.NextSteps = append(result.NextSteps, "ensure state directory exists with mode 0700 owned by current user")
		return result, nil
	}

	// 2. Check free disk space
	var stat unix.Statfs_t
	if err := unix.Statfs(stateDir, &stat); err == nil {
		available := stat.Bavail * uint64(stat.Bsize)
		result.FreeSpaceBytes = available
		if available < 512*1024*1024 { // 512 MiB reserve
			result.Issues = append(result.Issues, fmt.Sprintf("free disk space (%d MiB) is below required 512 MiB reserve", available/(1024*1024)))
			result.Status = "warning"
			result.NextSteps = append(result.NextSteps, "free disk space on the partition hosting state directory")
		}
	}

	// 3. Check if agent is currently running
	lock, err := state.Acquire(stateDir)
	if err != nil {
		if errors.Is(err, state.ErrLocked) {
			result.AgentRunning = true
			result.Status = "warning"
			result.Issues = append(result.Issues, "file-sync background agent is currently running (lock held)")
			result.NextSteps = append(result.NextSteps, "stop background service ('systemctl --user stop filesync.service' or 'filesync stop') before applying binary upgrade")
		} else {
			result.Status = "blocked"
			result.Issues = append(result.Issues, fmt.Sprintf("cannot inspect agent lock: %v", err))
			return result, nil
		}
	} else {
		_ = lock.Close()
		result.AgentRunning = false
	}

	// 4. Inspect SQLite database
	dbPath := filepath.Join(stateDir, "metadata.sqlite")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		result.Status = "blocked"
		result.Issues = append(result.Issues, "metadata.sqlite does not exist in state directory")
		result.NextSteps = append(result.NextSteps, "initialize device first ('filesync init')")
		return result, nil
	}

	// Open read-only to check schema and integrity
	dsn := "file:" + dbPath + "?mode=ro&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		result.Status = "blocked"
		result.Issues = append(result.Issues, fmt.Sprintf("open database: %v", err))
		return result, nil
	}
	defer db.Close()

	var userVersion int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&userVersion); err != nil {
		result.Status = "blocked"
		result.Issues = append(result.Issues, fmt.Sprintf("query user_version: %v", err))
		return result, nil
	}
	result.DatabaseSchema = userVersion

	if userVersion > repository.CurrentSchema {
		result.Status = "blocked"
		result.Issues = append(result.Issues, fmt.Sprintf("database schema (%d) is newer than binary schema (%d); binary cannot run against newer schema (Invariant I20)", userVersion, repository.CurrentSchema))
		result.NextSteps = append(result.NextSteps, "upgrade filesync binary to match newer database schema")
	} else if userVersion < repository.CurrentSchema {
		result.NextSteps = append(result.NextSteps, fmt.Sprintf("pending migration: database schema will be updated from %d to %d upon restart", userVersion, repository.CurrentSchema))
	}

	var integrityResult string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&integrityResult); err != nil || integrityResult != "ok" {
		result.IntegrityClean = false
		result.Status = "blocked"
		result.Issues = append(result.Issues, fmt.Sprintf("database integrity check failed: %s", integrityResult))
		result.NextSteps = append(result.NextSteps, "repair or restore database from consistent backup before upgrade")
	}

	if result.Status == "ready" {
		result.NextSteps = append(result.NextSteps, "environment is ready for upgrade; create consistent backup ('filesync maintenance backup') and proceed")
	}

	return result, nil
}

// RestoreBackup restores a consistent SQLite backup into the controller's stateDir.
func (c *Controller) RestoreBackup(ctx context.Context, backupPath string) (*RestoreBackupResult, error) {
	return RestoreBackup(ctx, c.db.StateDir(), backupPath)
}

// RestoreBackup restores a consistent SQLite backup into stateDir, resetting causal identity (Invariant I08).
func RestoreBackup(ctx context.Context, stateDir string, backupPath string) (*RestoreBackupResult, error) {
	if stateDir == "" {
		return nil, errors.New("state directory cannot be empty")
	}
	if backupPath == "" {
		return nil, errors.New("backup path cannot be empty")
	}

	backupAbs, err := filepath.Abs(backupPath)
	if err != nil {
		return nil, fmt.Errorf("resolve backup path: %w", err)
	}

	info, err := os.Stat(backupAbs)
	if err != nil {
		return nil, fmt.Errorf("read backup file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("backup path %s is not a regular file", backupAbs)
	}

	// 1. Verify directory and acquire lock (agent must NOT be running)
	if err := state.ValidateDirectory(stateDir); err != nil {
		return nil, err
	}
	lock, err := state.Acquire(stateDir)
	if err != nil {
		if errors.Is(err, state.ErrLocked) {
			return nil, errors.New("cannot restore backup while agent is running; stop service first ('systemctl --user stop filesync.service' or 'filesync stop')")
		}
		return nil, fmt.Errorf("acquire agent lock: %w", err)
	}
	defer lock.Close()

	// 2. Validate backup SQLite file
	dsn := "file:" + backupAbs + "?mode=ro&_pragma=busy_timeout(5000)"
	backupDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open backup file: %w", err)
	}
	var backupVersion int
	if err := backupDB.QueryRowContext(ctx, "PRAGMA user_version").Scan(&backupVersion); err != nil {
		backupDB.Close()
		return nil, fmt.Errorf("read backup user_version: %w", err)
	}
	if backupVersion > repository.CurrentSchema {
		backupDB.Close()
		return nil, fmt.Errorf("backup schema (%d) is newer than current binary schema (%d)", backupVersion, repository.CurrentSchema)
	}
	var integrity string
	if err := backupDB.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&integrity); err != nil || integrity != "ok" {
		backupDB.Close()
		return nil, fmt.Errorf("backup integrity check failed: %s", integrity)
	}
	backupDB.Close()

	// 3. Remove existing WAL and SHM files
	_ = os.Remove(filepath.Join(stateDir, "metadata.sqlite-wal"))
	_ = os.Remove(filepath.Join(stateDir, "metadata.sqlite-shm"))

	// 4. Copy backup to metadata.sqlite via temporary file
	targetDB := filepath.Join(stateDir, "metadata.sqlite")
	tmpTarget := filepath.Join(stateDir, "metadata.sqlite.restore-tmp")
	srcFile, err := os.Open(backupAbs)
	if err != nil {
		return nil, fmt.Errorf("open backup source: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(tmpTarget, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create restore temporary file: %w", err)
	}
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		dstFile.Close()
		_ = os.Remove(tmpTarget)
		return nil, fmt.Errorf("copy backup data: %w", err)
	}
	if err := dstFile.Sync(); err != nil {
		dstFile.Close()
		_ = os.Remove(tmpTarget)
		return nil, fmt.Errorf("sync restore file: %w", err)
	}
	dstFile.Close()

	if err := os.Rename(tmpTarget, targetDB); err != nil {
		_ = os.Remove(tmpTarget)
		return nil, fmt.Errorf("replace metadata.sqlite: %w", err)
	}

	// 5. CRITICAL: Reset Identity to preserve Invariant I08 (causal counter monotonicity)
	cfgPath := filepath.Join(stateDir, "config.json")
	var oldID history.ID
	if raw, err := os.ReadFile(cfgPath); err == nil {
		var cfg map[string]any
		if json.Unmarshal(raw, &cfg) == nil {
			if devStr, ok := cfg["device_id"].(string); ok {
				if d, err := hex.DecodeString(devStr); err == nil && len(d) == 32 {
					copy(oldID[:], d)
				}
			}
		}
	}

	var newID history.ID
	if _, err := rand.Read(newID[:]); err != nil {
		return nil, fmt.Errorf("generate new device id: %w", err)
	}

	ident, err := replication.LoadOrCreateIdentity(stateDir, newID, time.Now())
	if err != nil {
		return nil, fmt.Errorf("generate fresh TLS identity: %w", err)
	}

	cfgData := map[string]any{
		"device_id": hex.EncodeToString(newID[:]),
		"version":   1,
	}
	updatedRaw, _ := json.MarshalIndent(cfgData, "", "  ")
	if err := os.WriteFile(cfgPath, updatedRaw, 0o600); err != nil {
		return nil, fmt.Errorf("write updated config.json: %w", err)
	}

	// Update restored database with new identity and reset next_counter to 0
	restoreDSN := "file:" + targetDB + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(0)"
	postDB, err := sql.Open("sqlite", restoreDSN)
	if err != nil {
		return nil, fmt.Errorf("open restored database for identity update: %w", err)
	}
	defer postDB.Close()

	zeroCounter := make([]byte, 8)
	_, _ = postDB.ExecContext(ctx, "UPDATE folders SET local_author=?, next_counter=?", newID[:], zeroCounter)

	return &RestoreBackupResult{
		Status:      "success",
		BackupPath:  backupAbs,
		OldDeviceID: oldID,
		NewDeviceID: newID,
		NewKeyPin:   ident.KeyPin,
		Message:     "backup successfully restored and causal identity safely reset",
		Action:      "re-enroll new device ID in folder memberships with peers (Invariant I08)",
	}, nil
}
