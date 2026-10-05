package control

import (
	"context"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
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

	// 1. Daemon & local control lifecycle
	checks = append(checks, c.checkDaemonLocalControl()...)

	// 2. Identity & permissions (including TLS cert)
	checks = append(checks, c.checkIdentityPermissions()...)

	// 3. State directory & database permissions
	checks = append(checks, c.checkStatePermissions()...)

	// 4. Root availability for registered folders & blocked paths
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

	// 5. Free space & storage limits
	checks = append(checks, c.checkStorageCapacity(ctx)...)

	// 6. Network reachability & tooling
	checks = append(checks, c.checkNetworkReachability(ctx)...)

	// 7. Folder approvals & membership revisions/forks
	checks = append(checks, c.checkFolderApprovalAndRevisions(ctx)...)

	// 8. Membership status & limits
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

	// 9. Service & systemd tooling
	checks = append(checks, c.checkServiceTooling(ctx)...)

	// 10. Protocol & schema compatibility
	checks = append(checks, c.checkProtocolCompatibility(ctx)...)

	// 11. Pending recovery items
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

func (c *Controller) checkDaemonLocalControl() []DoctorCheck {
	var checks []DoctorCheck
	stateDir := c.db.StateDir()
	addrPath := filepath.Join(stateDir, "control.addr")
	tokenPath := filepath.Join(stateDir, "control.token")

	if c.options.StoppedAdapter {
		checks = append(checks, DoctorCheck{
			Name:        "daemon_lifecycle",
			Category:    "daemon",
			Status:      StatusOk,
			Message:     "running in stopped-state adapter (daemon inactive; start with 'orbit service start' or 'filesync serve' if background sync desired)",
			Remediation: "run 'orbit service start' (managed) or 'filesync serve' (manual) to start the background daemon",
		})
		return checks
	}

	addrBytes, err := os.ReadFile(addrPath)
	if err != nil {
		checks = append(checks, DoctorCheck{
			Name:        "daemon_lifecycle",
			Category:    "daemon",
			Status:      StatusOk,
			Message:     "background daemon inactive (start with 'orbit service start' or 'filesync serve' if background sync desired)",
			Remediation: "run 'orbit service start' (managed) or 'filesync serve' (manual) to start the background daemon",
		})
		return checks
	}

	checks = append(checks, DoctorCheck{
		Name:     "daemon_control_addr",
		Category: "daemon",
		Status:   StatusOk,
		Message:  fmt.Sprintf("daemon is active on %s", strings.TrimSpace(string(addrBytes))),
	})

	tokenInfo, err := os.Stat(tokenPath)
	if err != nil {
		checks = append(checks, DoctorCheck{
			Name:        "daemon_control_token",
			Category:    "daemon",
			Status:      StatusWarn,
			Message:     "control credential token not found while control address is active",
			Remediation: "restart the background daemon with 'orbit service restart' or supervised 'filesync serve'",
		})
	} else {
		perm := tokenInfo.Mode().Perm()
		if perm != 0o600 && perm != 0o400 {
			checks = append(checks, DoctorCheck{
				Name:        "daemon_control_token",
				Category:    "daemon",
				Status:      StatusFail,
				Message:     fmt.Sprintf("control token has insecure permissions (%04o, expected 0600)", perm),
				Remediation: fmt.Sprintf("chmod 0600 %s", tokenPath),
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:     "daemon_control_token",
				Category: "daemon",
				Status:   StatusOk,
				Message:  "control credential token permissions are secure (0600)",
			})
		}
	}
	return checks
}

func (c *Controller) checkIdentityPermissions() []DoctorCheck {
	var checks []DoctorCheck
	stateDir := c.db.StateDir()
	keyPath := filepath.Join(stateDir, "identity", "peer-identity.pem")
	certPath := filepath.Join(stateDir, "identity", "peer-certificate.pem")
	info, err := os.Stat(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		checks = append(checks, DoctorCheck{
			Name:        "identity_key",
			Category:    "permissions",
			Status:      StatusWarn,
			Message:     "identity key file not found",
			Remediation: "run filesync init or orbit setup to generate device identity and keypair",
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

	certBytes, err := os.ReadFile(certPath)
	if err == nil {
		block, _ := pem.Decode(certBytes)
		if block != nil && block.Type == "CERTIFICATE" {
			cert, parseErr := x509.ParseCertificate(block.Bytes)
			if parseErr != nil {
				checks = append(checks, DoctorCheck{
					Name:        "identity_cert",
					Category:    "permissions",
					Status:      StatusFail,
					Message:     fmt.Sprintf("corrupt TLS identity certificate: %v", parseErr),
					Remediation: "re-enroll device or regenerate identity",
				})
			} else {
				now := time.Now()
				if now.After(cert.NotAfter) {
					checks = append(checks, DoctorCheck{
						Name:        "identity_cert",
						Category:    "permissions",
						Status:      StatusFail,
						Message:     fmt.Sprintf("TLS certificate expired at %s", cert.NotAfter.Format(time.RFC3339)),
						Remediation: "generate fresh certificate with 'filesync init' or re-enroll",
					})
				} else if now.Add(7 * 24 * time.Hour).After(cert.NotAfter) {
					checks = append(checks, DoctorCheck{
						Name:        "identity_cert",
						Category:    "permissions",
						Status:      StatusWarn,
						Message:     fmt.Sprintf("TLS certificate expires soon (%s)", cert.NotAfter.Format(time.RFC3339)),
						Remediation: "renew identity before expiry",
					})
				} else {
					checks = append(checks, DoctorCheck{
						Name:     "identity_cert",
						Category: "permissions",
						Status:   StatusOk,
						Message:  fmt.Sprintf("TLS identity certificate is valid (expires %s)", cert.NotAfter.Format(time.RFC3339)),
					})
				}
			}
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
				Remediation: fmt.Sprintf("run orbit folders resume %s", fullID(reg.Folder)),
			})
			continue
		}

		if _, statErr := os.Stat(reg.Path); errors.Is(statErr, os.ErrNotExist) {
			checks = append(checks, DoctorCheck{
				Name:        name,
				Category:    "roots",
				Status:      StatusFail,
				Message:     fmt.Sprintf("workspace root %q does not exist", reg.Path),
				Remediation: fmt.Sprintf("mount workspace filesystem or run orbit folders relocate %s", fullID(reg.Folder)),
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
				Remediation: fmt.Sprintf("remount workspace filesystem or run orbit folders relocate %s", fullID(reg.Folder)),
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:     name,
				Category: "roots",
				Status:   StatusOk,
				Message:  fmt.Sprintf("workspace root %q is mounted and valid", reg.Path),
			})
		}

		// Check blocked paths
		blocked, bErr := c.db.BlockedPaths(ctx, reg.Folder)
		if bErr == nil && len(blocked) > 0 {
			checks = append(checks, DoctorCheck{
				Name:        fmt.Sprintf("blocked_paths_%s", shortID(reg.Folder)),
				Category:    "roots",
				Status:      StatusWarn,
				Message:     fmt.Sprintf("folder %s has %d blocked paths (unsupported object or permission error)", shortID(reg.Folder), len(blocked)),
				Remediation: "remove unsupported filesystem objects (fifos, sockets) or adjust file permissions",
			})
		}
	}

	return checks, nil
}

func (c *Controller) checkStorageCapacity(ctx context.Context) []DoctorCheck {
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
				Remediation: "free disk space on the state partition or run orbit storage gc run",
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
				Remediation: "checkpoint WAL or run filesync maintenance backup",
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

	// Check data and metadata budgets
	if usage, err := c.db.DetailedStorageUsage(ctx); err == nil {
		if usage.DataBudgetBytes > 0 && usage.ObjectBytes > usage.DataBudgetBytes {
			checks = append(checks, DoctorCheck{
				Name:        "data_budget",
				Category:    "storage",
				Status:      StatusFail,
				Message:     fmt.Sprintf("stored objects (%d MiB) exceed configured data budget (%d MiB)", usage.ObjectBytes/(1024*1024), usage.DataBudgetBytes/(1024*1024)),
				Remediation: "run orbit storage gc run or increase data budget in settings",
			})
		} else if usage.DataBudgetBytes > 0 {
			checks = append(checks, DoctorCheck{
				Name:     "data_budget",
				Category: "storage",
				Status:   StatusOk,
				Message:  fmt.Sprintf("stored objects (%d MiB) within data budget (%d MiB)", usage.ObjectBytes/(1024*1024), usage.DataBudgetBytes/(1024*1024)),
			})
		}

		if usage.MetadataBudgetBytes > 0 && usage.MetadataBytes > usage.MetadataBudgetBytes {
			checks = append(checks, DoctorCheck{
				Name:        "metadata_budget",
				Category:    "storage",
				Status:      StatusWarn,
				Message:     fmt.Sprintf("metadata storage (%d MiB) exceeds configured metadata budget (%d MiB)", usage.MetadataBytes/(1024*1024), usage.MetadataBudgetBytes/(1024*1024)),
				Remediation: "run checkpoint or increase metadata budget in settings",
			})
		}
	}

	return checks
}

func (c *Controller) checkNetworkReachability(ctx context.Context) []DoctorCheck {
	var checks []DoctorCheck
	settings, err := config.LoadRuntimeSettings(c.db.StateDir())
	if err != nil {
		settings = tc.Settings{}
	}

	// 1. Advertised peer address check
	if settings.AdvertisedPeer != "" {
		host, _, sErr := net.SplitHostPort(settings.AdvertisedPeer)
		if sErr != nil {
			host = settings.AdvertisedPeer
		}
		ip := net.ParseIP(host)
		if ip != nil && ip.IsLoopback() {
			folders, _ := c.db.Folders(ctx)
			hasRemotePeers := false
			for _, f := range folders {
				pl, err := c.PeerList(ctx, f.Folder)
				if err == nil && len(pl.Active) > 1 {
					hasRemotePeers = true
					break
				}
			}
			if hasRemotePeers {
				checks = append(checks, DoctorCheck{
					Name:        "advertised_peer_loopback",
					Category:    "network",
					Status:      StatusWarn,
					Message:     fmt.Sprintf("advertised peer address %q is loopback while remote peers exist; peers cannot connect", settings.AdvertisedPeer),
					Remediation: "configure LAN or Tailscale IP in runtime settings with 'orbit config'",
				})
			}
		}
	}

	// 2. Peer contact freshness
	folders, _ := c.db.Folders(ctx)
	for _, f := range folders {
		progress, err := c.db.PeerProgress(ctx, f.Folder)
		if err == nil {
			for _, p := range progress {
				if !p.LastContact.IsZero() && time.Since(p.LastContact) > 24*time.Hour {
					checks = append(checks, DoctorCheck{
						Name:        fmt.Sprintf("peer_contact_%s", shortID(p.Peer)),
						Category:    "network",
						Status:      StatusWarn,
						Message:     fmt.Sprintf("peer %s has not contacted this device since %s (>24h ago)", shortID(p.Peer), p.LastContact.Format(time.RFC3339)),
						Remediation: "verify peer device is online and network connectivity is active",
					})
				}
			}
		}
	}

	// 3. Network tooling availability: check tailscale if configured
	if strings.Contains(strings.ToLower(settings.AdvertisedPeer), "tailscale") || strings.Contains(strings.ToLower(settings.PeerListen), "100.") {
		if _, err := exec.LookPath("tailscale"); err != nil {
			checks = append(checks, DoctorCheck{
				Name:        "tailscale_tooling",
				Category:    "network",
				Status:      StatusWarn,
				Message:     "tailscale binary not found in PATH but Tailscale network address is configured",
				Remediation: "install Tailscale package or configure LAN addresses",
			})
		}
	}

	if len(checks) == 0 {
		checks = append(checks, DoctorCheck{
			Name:     "network_configuration",
			Category: "network",
			Status:   StatusOk,
			Message:  "network configuration is healthy",
		})
	}

	return checks
}

func (c *Controller) checkFolderApprovalAndRevisions(ctx context.Context) []DoctorCheck {
	var checks []DoctorCheck

	// Check pending enrollment requests
	pendingCount := 0
	reqResult, err := c.terminalRequests(ctx, tc.Query{Version: tc.Version, Kind: "requests", Limit: tc.MaxPage})
	if err == nil {
		for _, req := range reqResult.Requests {
			if req.State == "pending" || req.State == "pending_approval" {
				pendingCount++
			}
		}
	}
	folders, _ := c.db.Folders(ctx)
	for _, f := range folders {
		reqs, _ := c.db.ListEnrollmentRequests(ctx, f.Folder, "pending")
		pendingCount += len(reqs)
	}
	if pendingCount > 0 {
		checks = append(checks, DoctorCheck{
			Name:        "pending_enrollment_requests",
			Category:    "membership",
			Status:      StatusWarn,
			Message:     fmt.Sprintf("%d pending enrollment request(s) awaiting approval", pendingCount),
			Remediation: "run 'orbit devices requests' to review and approve pending requests",
		})
	}

	// Check membership forks
	hasAnyFork := false
	for _, f := range folders {
		if hasFork, _ := c.db.HasMembershipFork(ctx, f.Folder); hasFork {
			hasAnyFork = true
			checks = append(checks, DoctorCheck{
				Name:        fmt.Sprintf("membership_fork_%s", shortID(f.Folder)),
				Category:    "membership",
				Status:      StatusFail,
				Message:     fmt.Sprintf("folder %s has a membership fork with competing revisions", shortID(f.Folder)),
				Remediation: fmt.Sprintf("review competing membership revisions for folder %s", fullID(f.Folder)),
			})
		}
	}
	if !hasAnyFork && len(checks) == 0 {
		checks = append(checks, DoctorCheck{
			Name:     "membership_revisions",
			Category: "membership",
			Status:   StatusOk,
			Message:  "membership revisions and approvals are consistent",
		})
	}

	return checks
}

func (c *Controller) checkServiceTooling(ctx context.Context) []DoctorCheck {
	var checks []DoctorCheck
	st, err := CheckServiceStatus(ctx, c.db.StateDir(), c.db)
	if err != nil {
		checks = append(checks, DoctorCheck{
			Name:        "service_status",
			Category:    "service",
			Status:      StatusWarn,
			Message:     fmt.Sprintf("unable to inspect service status: %v", err),
			Remediation: "inspect systemd user session or file permissions",
		})
		return checks
	}

	settings, err := config.LoadRuntimeSettings(c.db.StateDir())
	if err != nil {
		settings = tc.Settings{}
	}

	if settings.Startup == "unattended" {
		if !st.LingeringEnabled {
			checks = append(checks, DoctorCheck{
				Name:        "unattended_linger",
				Category:    "service",
				Status:      StatusWarn,
				Message:     "unattended startup configured but systemd user lingering is not enabled (service will stop on logout)",
				Remediation: "run 'loginctl enable-linger' as root or user to permit background sync after logout",
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:     "unattended_linger",
				Category: "service",
				Status:   StatusOk,
				Message:  "user lingering is verified for unattended startup",
			})
		}
	}

	if (settings.Startup == "login" || settings.Startup == "unattended") && !st.SystemdAvailable {
		checks = append(checks, DoctorCheck{
			Name:        "systemd_tooling",
			Category:    "service",
			Status:      StatusWarn,
			Message:     "systemctl not found in PATH; systemd user services are unavailable on this host",
			Remediation: "install systemd or switch to manual startup with 'orbit service'",
		})
	}

	if len(checks) == 0 {
		checks = append(checks, DoctorCheck{
			Name:     "service_configuration",
			Category: "service",
			Status:   StatusOk,
			Message:  "service configuration is normal",
		})
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
