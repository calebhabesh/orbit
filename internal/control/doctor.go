package control

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"golang.org/x/sys/unix"
)

func shortID(id history.ID) string {
	return hex.EncodeToString(id[:4])
}

func fullID(id history.ID) string {
	return hex.EncodeToString(id[:])
}

type CheckStatus string

const (
	StatusOk   CheckStatus = "OK"
	StatusWarn CheckStatus = "WARN"
	StatusFail CheckStatus = "FAIL"
)

type DoctorCheck struct {
	Name        string      `json:"name"`
	Category    string      `json:"category"`
	Status      CheckStatus `json:"status"`
	Message     string      `json:"message"`
	Remediation string      `json:"remediation,omitempty"`
}

type DoctorReport struct {
	OverallStatus CheckStatus   `json:"overall_status"`
	Checks        []DoctorCheck `json:"checks"`
	GeneratedAt   string        `json:"generated_at"`
}

const (
	FreeSpaceReserveTargetBytes = 512 * 1024 * 1024 // 512 MiB
	MetadataWALSoftCapBytes     = 256 * 1024 * 1024 // 256 MiB
	MaxActiveMembersBaseline    = 16
)

func (c *Controller) Doctor(ctx context.Context) (*DoctorReport, error) {
	var checks []DoctorCheck

	// 1. Identity & permissions
	checks = append(checks, c.checkIdentityPermissions()...)

	// 2. State directory & database permissions
	checks = append(checks, c.checkStatePermissions()...)

	// 3. Root availability for registered folders
	rootChecks, err := c.checkRootAvailability(ctx)
	if err != nil {
		checks = append(checks, DoctorCheck{
			Name:        "root_query",
			Category:    "roots",
			Status:      StatusFail,
			Message:     fmt.Sprintf("failed to query registered roots: %v", err),
			Remediation: "inspect database integrity",
		})
	} else {
		checks = append(checks, rootChecks...)
	}

	// 4. Free space & storage limits
	checks = append(checks, c.checkStorageCapacity()...)

	// 5. Membership status & limits
	memChecks, err := c.checkMembershipStatus(ctx)
	if err != nil {
		checks = append(checks, DoctorCheck{
			Name:        "membership_query",
			Category:    "membership",
			Status:      StatusFail,
			Message:     fmt.Sprintf("failed to inspect membership: %v", err),
			Remediation: "inspect database integrity",
		})
	} else {
		checks = append(checks, memChecks...)
	}

	// 6. Protocol & schema compatibility
	checks = append(checks, c.checkProtocolCompatibility(ctx)...)

	// 7. Pending recovery items
	recChecks, err := c.checkPendingRecovery(ctx)
	if err != nil {
		checks = append(checks, DoctorCheck{
			Name:        "recovery_query",
			Category:    "recovery",
			Status:      StatusFail,
			Message:     fmt.Sprintf("failed to inspect recovery status: %v", err),
			Remediation: "inspect database integrity",
		})
	} else {
		checks = append(checks, recChecks...)
	}

	overall := StatusOk
	for _, ch := range checks {
		if ch.Status == StatusFail {
			overall = StatusFail
			break
		} else if ch.Status == StatusWarn && overall != StatusFail {
			overall = StatusWarn
		}
	}

	return &DoctorReport{
		OverallStatus: overall,
		Checks:        checks,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (c *Controller) checkIdentityPermissions() []DoctorCheck {
	var checks []DoctorCheck
	stateDir := c.db.StateDir()
	keyPath := filepath.Join(stateDir, "identity", "peer-identity.pem")
	info, err := os.Stat(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		checks = append(checks, DoctorCheck{
			Name:        "identity_key",
			Category:    "permissions",
			Status:      StatusWarn,
			Message:     "identity key file not found",
			Remediation: "run filesync init to generate device identity and keypair",
		})
	} else if err != nil {
		checks = append(checks, DoctorCheck{
			Name:        "identity_key",
			Category:    "permissions",
			Status:      StatusFail,
			Message:     fmt.Sprintf("failed to stat identity key: %v", err),
			Remediation: "ensure read access to state directory",
		})
	} else {
		perm := info.Mode().Perm()
		if perm != 0o600 && perm != 0o400 {
			checks = append(checks, DoctorCheck{
				Name:        "identity_key",
				Category:    "permissions",
				Status:      StatusFail,
				Message:     fmt.Sprintf("identity key has insecure permissions (%04o, expected 0600)", perm),
				Remediation: fmt.Sprintf("chmod 0600 %s", keyPath),
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:     "identity_key",
				Category: "permissions",
				Status:   StatusOk,
				Message:  "identity key permissions are secure (0600)",
			})
		}
	}
	return checks
}

func (c *Controller) checkStatePermissions() []DoctorCheck {
	var checks []DoctorCheck
	stateDir := c.db.StateDir()

	dirInfo, err := os.Stat(stateDir)
	if err != nil {
		checks = append(checks, DoctorCheck{
			Name:        "state_directory",
			Category:    "permissions",
			Status:      StatusFail,
			Message:     fmt.Sprintf("state directory %s inaccessible: %v", stateDir, err),
			Remediation: fmt.Sprintf("mkdir -p %s && chmod 0700 %s", stateDir, stateDir),
		})
	} else {
		perm := dirInfo.Mode().Perm()
		if perm&0o077 != 0 {
			checks = append(checks, DoctorCheck{
				Name:        "state_directory",
				Category:    "permissions",
				Status:      StatusWarn,
				Message:     fmt.Sprintf("state directory %s has group/world permissions (%04o, expected 0700)", stateDir, perm),
				Remediation: fmt.Sprintf("chmod 0700 %s", stateDir),
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:     "state_directory",
				Category: "permissions",
				Status:   StatusOk,
				Message:  "state directory permissions are secure (0700)",
			})
		}
	}

	dbPath := filepath.Join(stateDir, "metadata.sqlite")
	dbInfo, err := os.Stat(dbPath)
	if err == nil {
		perm := dbInfo.Mode().Perm()
		if perm&0o077 != 0 {
			checks = append(checks, DoctorCheck{
				Name:        "metadata_database",
				Category:    "permissions",
				Status:      StatusWarn,
				Message:     fmt.Sprintf("database %s has group/world permissions (%04o, expected 0600)", dbPath, perm),
				Remediation: fmt.Sprintf("chmod 0600 %s", dbPath),
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:     "metadata_database",
				Category: "permissions",
				Status:   StatusOk,
				Message:  "metadata database permissions are secure (0600)",
			})
		}
	}

	return checks
}

func (c *Controller) checkRootAvailability(ctx context.Context) ([]DoctorCheck, error) {
	var checks []DoctorCheck
	registered, err := c.db.RegisteredFolders(ctx)
	if err != nil {
		return nil, err
	}

	if len(registered) == 0 {
		checks = append(checks, DoctorCheck{
			Name:     "folder_roots",
			Category: "roots",
			Status:   StatusOk,
			Message:  "no folders registered on this device",
		})
		return checks, nil
	}

	for _, reg := range registered {
		name := fmt.Sprintf("root_%s", shortID(reg.Folder))
		if reg.Paused {
			checks = append(checks, DoctorCheck{
				Name:        name,
				Category:    "roots",
				Status:      StatusWarn,
				Message:     fmt.Sprintf("folder %s is paused (%s)", shortID(reg.Folder), reg.PauseReason),
				Remediation: fmt.Sprintf("run filesync folders resume --folder %s", fullID(reg.Folder)),
			})
			continue
		}

		err := c.ws.Revalidate(ctx, reg.Folder)
		if err != nil {
			checks = append(checks, DoctorCheck{
				Name:        name,
				Category:    "roots",
				Status:      StatusFail,
				Message:     fmt.Sprintf("workspace root %q failed validation: %v", reg.Path, err),
				Remediation: fmt.Sprintf("remount workspace filesystem or run filesync safety root-revalidate --folder %s", fullID(reg.Folder)),
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:     name,
				Category: "roots",
				Status:   StatusOk,
				Message:  fmt.Sprintf("workspace root %q is mounted and valid", reg.Path),
			})
		}
	}

	return checks, nil
}

func (c *Controller) checkStorageCapacity() []DoctorCheck {
	var checks []DoctorCheck
	stateDir := c.db.StateDir()

	var stat unix.Statfs_t
	if err := unix.Statfs(stateDir, &stat); err == nil {
		availBytes := stat.Bavail * uint64(stat.Bsize)
		if availBytes < FreeSpaceReserveTargetBytes {
			checks = append(checks, DoctorCheck{
				Name:        "free_disk_space",
				Category:    "storage",
				Status:      StatusFail,
				Message:     fmt.Sprintf("state partition has only %d MiB free space (reserve target is %d MiB)", availBytes/(1024*1024), FreeSpaceReserveTargetBytes/(1024*1024)),
				Remediation: "free disk space on the state partition or run filesync storage gc run",
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:     "free_disk_space",
				Category: "storage",
				Status:   StatusOk,
				Message:  fmt.Sprintf("state partition has sufficient free space (%d MiB available)", availBytes/(1024*1024)),
			})
		}
	}

	walPath := filepath.Join(stateDir, "metadata.sqlite-wal")
	if walInfo, err := os.Stat(walPath); err == nil {
		walSize := uint64(walInfo.Size())
		if walSize > MetadataWALSoftCapBytes {
			checks = append(checks, DoctorCheck{
				Name:        "wal_file_size",
				Category:    "storage",
				Status:      StatusWarn,
				Message:     fmt.Sprintf("SQLite WAL file size is %d MiB (soft cap is %d MiB)", walSize/(1024*1024), MetadataWALSoftCapBytes/(1024*1024)),
				Remediation: "run filesync maintenance backup to checkpoint WAL into main database",
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:     "wal_file_size",
				Category: "storage",
				Status:   StatusOk,
				Message:  fmt.Sprintf("SQLite WAL file is within soft cap (%d MiB)", walSize/(1024*1024)),
			})
		}
	}

	return checks
}

func (c *Controller) checkMembershipStatus(ctx context.Context) ([]DoctorCheck, error) {
	var checks []DoctorCheck
	folders, err := c.db.Folders(ctx)
	if err != nil {
		return nil, err
	}

	for _, f := range folders {
		name := fmt.Sprintf("membership_%s", shortID(f.Folder))
		pl, err := c.PeerList(ctx, f.Folder)
		if err != nil {
			checks = append(checks, DoctorCheck{
				Name:        name,
				Category:    "membership",
				Status:      StatusFail,
				Message:     fmt.Sprintf("failed to get peer list for folder %s: %v", shortID(f.Folder), err),
				Remediation: "check membership records",
			})
			continue
		}

		if len(pl.Active) > MaxActiveMembersBaseline {
			checks = append(checks, DoctorCheck{
				Name:        name,
				Category:    "membership",
				Status:      StatusWarn,
				Message:     fmt.Sprintf("folder %s has %d active members (baseline limit is %d)", shortID(f.Folder), len(pl.Active), MaxActiveMembersBaseline),
				Remediation: fmt.Sprintf("retire inactive members with filesync peers retire --folder %s", fullID(f.Folder)),
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:     name,
				Category: "membership",
				Status:   StatusOk,
				Message:  fmt.Sprintf("folder %s has %d active members and revision %d", shortID(f.Folder), len(pl.Active), pl.Revision),
			})
		}
	}

	return checks, nil
}

func (c *Controller) checkProtocolCompatibility(ctx context.Context) []DoctorCheck {
	var checks []DoctorCheck
	checks = append(checks, DoctorCheck{
		Name:     "protocol_version",
		Category: "protocol",
		Status:   StatusOk,
		Message:  "protocol version 1 is supported",
	})

	userVer, err := c.db.UserVersion(ctx)
	if err != nil {
		checks = append(checks, DoctorCheck{
			Name:        "schema_version",
			Category:    "protocol",
			Status:      StatusFail,
			Message:     fmt.Sprintf("failed to query database schema version: %v", err),
			Remediation: "verify SQLite database file integrity",
		})
	} else if userVer < repository.CurrentSchema {
		checks = append(checks, DoctorCheck{
			Name:        "schema_version",
			Category:    "protocol",
			Status:      StatusWarn,
			Message:     fmt.Sprintf("database schema %d is older than binary schema %d", userVer, repository.CurrentSchema),
			Remediation: "run filesync serve or filesync init to apply migrations",
		})
	} else {
		checks = append(checks, DoctorCheck{
			Name:     "schema_version",
			Category: "protocol",
			Status:   StatusOk,
			Message:  fmt.Sprintf("database schema version %d matches binary", userVer),
		})
	}

	return checks
}

func (c *Controller) checkPendingRecovery(ctx context.Context) ([]DoctorCheck, error) {
	var checks []DoctorCheck

	// Check exhausted durable tasks
	tasks, err := c.db.ListDurableTasks(ctx, repository.TaskFilter{State: "exhausted"})
	if err != nil {
		return nil, err
	}
	if len(tasks) > 0 {
		checks = append(checks, DoctorCheck{
			Name:        "exhausted_tasks",
			Category:    "recovery",
			Status:      StatusWarn,
			Message:     fmt.Sprintf("%d durable work tasks exhausted retries", len(tasks)),
			Remediation: "run filesync work retry --all or inspect error causes with filesync work list --state exhausted",
		})
	} else {
		checks = append(checks, DoctorCheck{
			Name:     "exhausted_tasks",
			Category: "recovery",
			Status:   StatusOk,
			Message:  "no exhausted durable work tasks",
		})
	}

	// Check uncommitted publication journals
	registered, err := c.db.RegisteredFolders(ctx)
	if err != nil {
		return nil, err
	}
	totalPendingPubs := 0
	for _, reg := range registered {
		pubs, err := c.db.Publications(ctx, reg.Folder)
		if err == nil {
			for _, p := range pubs {
				if p.Phase != "COMMITTED" {
					totalPendingPubs++
				}
			}
		}
	}
	if totalPendingPubs > 0 {
		checks = append(checks, DoctorCheck{
			Name:        "pending_publications",
			Category:    "recovery",
			Status:      StatusWarn,
			Message:     fmt.Sprintf("%d unfinalized publication journals pending", totalPendingPubs),
			Remediation: "run filesync work scan to reconcile and complete publication journals",
		})
	} else {
		checks = append(checks, DoctorCheck{
			Name:     "pending_publications",
			Category: "recovery",
			Status:   StatusOk,
			Message:  "zero pending publication journals",
		})
	}

	return checks, nil
}
