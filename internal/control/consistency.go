package control

import (
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/repository"
	_ "modernc.org/sqlite"
)

var (
	ErrIncompleteRecovery = errors.New("incomplete identity recovery detected: state inconsistent")
)

// ConsistencyReport describes startup consistency verification details.
type ConsistencyReport struct {
	Consistent       bool   `json:"consistent"`
	ConfigDeviceID   string `json:"config_device_id"`
	CertDevicePrefix string `json:"cert_device_prefix,omitempty"`
	DBLocalAuthor    string `json:"db_local_author,omitempty"`
	ErrorReason      string `json:"error_reason,omitempty"`
}

// VerifyRecoveryConsistency verifies that config.json, peer-identity.pem, and metadata.sqlite folders.local_author
// all agree on the local device identity (Invariant I20).
// Returns nil if state is consistent or uninitialized.
// Returns ErrIncompleteRecovery if a crashed/partial identity transition is detected.
func VerifyRecoveryConsistency(stateDir string) error {
	rep, err := CheckRecoveryConsistency(stateDir)
	if err != nil {
		return err
	}
	if !rep.Consistent {
		return fmt.Errorf("%w: %s", ErrIncompleteRecovery, rep.ErrorReason)
	}
	return nil
}

// CheckRecoveryConsistency performs a read-only inspection of identity consistency.
func CheckRecoveryConsistency(stateDir string) (*ConsistencyReport, error) {
	return checkRecoveryConsistency(context.Background(), stateDir, nil)
}

func checkRecoveryConsistency(ctx context.Context, stateDir string, owner *repository.DB) (*ConsistencyReport, error) {
	cfg, err := config.Load(stateDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Uninitialized state is trivially consistent
			return &ConsistencyReport{Consistent: true}, nil
		}
		return nil, fmt.Errorf("read config for consistency check: %w", err)
	}

	report := &ConsistencyReport{
		Consistent:     true,
		ConfigDeviceID: cfg.DeviceID,
	}

	// 1. Check TLS certificate identity in stateDir/identity/peer-identity.pem or stateDir/peer-identity.pem
	certPaths := []string{
		filepath.Join(stateDir, "identity", "peer-identity.pem"),
		filepath.Join(stateDir, "peer-identity.pem"),
	}
	var certPEM []byte
	for _, p := range certPaths {
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			certPEM = data
			break
		}
	}

	if len(certPEM) > 0 {
		rest := certPEM
		for len(rest) > 0 {
			var block *pem.Block
			block, rest = pem.Decode(rest)
			if block == nil {
				break
			}
			if block.Type == "CERTIFICATE" {
				cert, err := x509.ParseCertificate(block.Bytes)
				if err == nil && cert != nil {
					cn := cert.Subject.CommonName
					const prefix = "orbit-device-"
					if strings.HasPrefix(cn, prefix) {
						certDevHex := strings.TrimPrefix(cn, prefix)
						report.CertDevicePrefix = certDevHex
						if !strings.HasPrefix(cfg.DeviceID, certDevHex) {
							report.Consistent = false
							report.ErrorReason = fmt.Sprintf("TLS certificate identity (%s) does not match config device (%s)", certDevHex, cfg.DeviceID[:len(certDevHex)])
							return report, nil
						}
					}
				}
			}
		}
	}

	// 2. Check metadata.sqlite folders.local_author
	if owner != nil {
		folders, err := owner.Folders(ctx)
		if err != nil {
			return nil, fmt.Errorf("read owned folder identities: %w", err)
		}
		for _, folder := range folders {
			checkLocalAuthor(report, folder.LocalAuthor[:], cfg.DeviceID)
			if !report.Consistent {
				break
			}
		}
		return report, nil
	}
	dbPath := filepath.Join(stateDir, "metadata.sqlite")
	if _, err := os.Stat(dbPath); err == nil {
		dbDSN := "file:" + dbPath + "?mode=ro&_pragma=busy_timeout(5000)"
		db, err := sql.Open("sqlite", dbDSN)
		if err != nil {
			return nil, fmt.Errorf("open database for consistency check: %w", err)
		}
		defer db.Close()

		rows, err := db.QueryContext(ctx, "SELECT local_author FROM folders")
		if err != nil {
			// An empty pre-migration database has no identity rows yet.
			if strings.Contains(err.Error(), "no such table: folders") {
				return report, nil
			}
			return nil, fmt.Errorf("read folder identities: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var rawAuthor []byte
			if err := rows.Scan(&rawAuthor); err != nil {
				return nil, err
			}
			checkLocalAuthor(report, rawAuthor, cfg.DeviceID)
			if !report.Consistent {
				return report, nil
			}
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	return report, nil
}

func checkLocalAuthor(report *ConsistencyReport, rawAuthor []byte, device string) {
	authorHex := hex.EncodeToString(rawAuthor)
	report.DBLocalAuthor = authorHex
	if len(rawAuthor) != 32 || authorHex != device {
		report.Consistent = false
		report.ErrorReason = fmt.Sprintf("database local_author (%s) does not match config device (%s)", authorHex, device)
	}
}
