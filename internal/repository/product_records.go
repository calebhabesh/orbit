package repository

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
)

var (
	ErrInvitationNotFound = errors.New("invitation not found")
	ErrInvitationRevoked  = errors.New("invitation was revoked")
	ErrInvitationExpired  = errors.New("invitation expired or uses exhausted")
	ErrRequestNotFound    = errors.New("enrollment request not found")
	ErrLeaseNotFound      = errors.New("read lease not found")
	ErrLeaseExpired       = errors.New("read lease expired")
	ErrOperationNotFound  = errors.New("operation progress record not found")
)

// --- Folder & Device Display Names ---

// SetFolderDisplayName sets an owner-local display alias for a folder/workspace.
func (db *DB) SetFolderDisplayName(ctx context.Context, folder history.ID, name string) error {
	res, err := db.db.ExecContext(ctx, `UPDATE folders SET display_name=? WHERE folder_id=?`, name, folder[:])
	if err != nil {
		return fmt.Errorf("update folder display name: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("folder %s not found", folder)
	}
	return nil
}

// GetFolderDisplayName retrieves the local display name for a folder/workspace.
func (db *DB) GetFolderDisplayName(ctx context.Context, folder history.ID) (string, error) {
	var name sql.NullString
	if err := db.db.QueryRowContext(ctx, `SELECT display_name FROM folders WHERE folder_id=?`, folder[:]).Scan(&name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("folder %s not found", folder)
		}
		return "", err
	}
	if name.Valid {
		return name.String, nil
	}
	return "", nil
}

// SetDeviceDisplayName sets a display alias for a known device.
func (db *DB) SetDeviceDisplayName(ctx context.Context, device history.ID, name string) error {
	res, err := db.db.ExecContext(ctx, `UPDATE devices SET display_name=? WHERE device_id=?`, name, device[:])
	if err != nil {
		return fmt.Errorf("update device display name: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// Insert device row if not present
		_, err = db.db.ExecContext(ctx, `INSERT INTO devices(device_id, display_name, is_local) VALUES(?,?,0)`, device[:], name)
		if err != nil {
			return fmt.Errorf("insert device with display name: %w", err)
		}
	}
	return nil
}

// GetDeviceDisplayName retrieves the display alias for a device.
func (db *DB) GetDeviceDisplayName(ctx context.Context, device history.ID) (string, error) {
	var name sql.NullString
	if err := db.db.QueryRowContext(ctx, `SELECT display_name FROM devices WHERE device_id=?`, device[:]).Scan(&name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	if name.Valid {
		return name.String, nil
	}
	return "", nil
}

// GetDeviceDisplayNames retrieves all known device display aliases indexed by hex device ID.
func (db *DB) GetDeviceDisplayNames(ctx context.Context) (map[string]string, error) {
	rows, err := db.db.QueryContext(ctx, `SELECT device_id, display_name FROM devices WHERE display_name IS NOT NULL AND display_name != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := make(map[string]string)
	for rows.Next() {
		var dev []byte
		var name string
		if err := rows.Scan(&dev, &name); err != nil {
			return nil, err
		}
		res[hex.EncodeToString(dev)] = name
	}
	return res, rows.Err()
}

// --- Invitations (G02) ---

type InvitationRecord struct {
	Digest    history.Digest
	Folder    history.ID
	CreatedNS int64
	ExpiresNS int64
	MaxUses   int
	UsesCount int
	Revoked   bool
}

// CreateInvitation persists a workspace invitation verifier digest.
func (db *DB) CreateInvitation(ctx context.Context, inv InvitationRecord) error {
	revoked := 0
	if inv.Revoked {
		revoked = 1
	}
	if inv.MaxUses <= 0 {
		inv.MaxUses = 1
	}
	_, err := db.db.ExecContext(ctx,
		`INSERT INTO invitations(digest, folder_id, created_ns, expires_ns, max_uses, uses_count, revoked) VALUES(?,?,?,?,?,?,?)`,
		inv.Digest[:], inv.Folder[:], inv.CreatedNS, inv.ExpiresNS, inv.MaxUses, inv.UsesCount, revoked,
	)
	if err != nil {
		return fmt.Errorf("insert invitation: %w", err)
	}
	return nil
}

// GetInvitation retrieves an invitation by its token digest.
func (db *DB) GetInvitation(ctx context.Context, digest history.Digest) (*InvitationRecord, error) {
	var rawDigest, rawFolder []byte
	var createdNS, expiresNS int64
	var maxUses, usesCount, revokedInt int

	err := db.db.QueryRowContext(ctx,
		`SELECT digest, folder_id, created_ns, expires_ns, max_uses, uses_count, revoked FROM invitations WHERE digest=?`,
		digest[:],
	).Scan(&rawDigest, &rawFolder, &createdNS, &expiresNS, &maxUses, &usesCount, &revokedInt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvitationNotFound
		}
		return nil, err
	}

	rec := &InvitationRecord{
		CreatedNS: createdNS,
		ExpiresNS: expiresNS,
		MaxUses:   maxUses,
		UsesCount: usesCount,
		Revoked:   revokedInt == 1,
	}
	copy(rec.Digest[:], rawDigest)
	copy(rec.Folder[:], rawFolder)
	return rec, nil
}

// RevokeInvitation invalidates an invitation digest permanently.
func (db *DB) RevokeInvitation(ctx context.Context, digest history.Digest) error {
	res, err := db.db.ExecContext(ctx, `UPDATE invitations SET revoked=1 WHERE digest=?`, digest[:])
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvitationNotFound
	}
	return nil
}

// ConsumeInvitation validates that an invitation is active, unexpired, and has remaining uses,
// and increments its usage count atomically.
func (db *DB) ConsumeInvitation(ctx context.Context, digest history.Digest, now time.Time, scope ...history.ID) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var folderRaw []byte
	var expiresNS int64
	var maxUses, usesCount, revokedInt int
	err = tx.QueryRowContext(ctx,
		`SELECT folder_id, expires_ns, max_uses, uses_count, revoked FROM invitations WHERE digest=?`,
		digest[:],
	).Scan(&folderRaw, &expiresNS, &maxUses, &usesCount, &revokedInt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvitationNotFound
		}
		return err
	}

	if len(scope) > 0 && !bytes.Equal(folderRaw, scope[0][:]) {
		return ErrUnauthorized
	}
	if revokedInt == 1 {
		return ErrInvitationRevoked
	}
	if now.UnixNano() >= expiresNS || usesCount >= maxUses {
		return ErrInvitationExpired
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE invitations SET uses_count = uses_count + 1 WHERE digest=?`,
		digest[:],
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ListInvitations lists all invitations for a workspace.
func (db *DB) ListInvitations(ctx context.Context, folder history.ID) ([]InvitationRecord, error) {
	rows, err := db.db.QueryContext(ctx,
		`SELECT digest, folder_id, created_ns, expires_ns, max_uses, uses_count, revoked FROM invitations WHERE folder_id=? ORDER BY created_ns DESC`,
		folder[:],
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []InvitationRecord
	for rows.Next() {
		var rawDigest, rawFolder []byte
		var createdNS, expiresNS int64
		var maxUses, usesCount, revokedInt int
		if err := rows.Scan(&rawDigest, &rawFolder, &createdNS, &expiresNS, &maxUses, &usesCount, &revokedInt); err != nil {
			return nil, err
		}
		rec := InvitationRecord{
			CreatedNS: createdNS,
			ExpiresNS: expiresNS,
			MaxUses:   maxUses,
			UsesCount: usesCount,
			Revoked:   revokedInt == 1,
		}
		copy(rec.Digest[:], rawDigest)
		copy(rec.Folder[:], rawFolder)
		result = append(result, rec)
	}
	return result, rows.Err()
}

// --- Enrollment Requests (G02) ---

type EnrollmentRequestRecord struct {
	RequestID      string     `json:"request_id"`
	Folder         history.ID `json:"folder"`
	DeviceID       history.ID `json:"device_id"`
	PublicKey      [32]byte   `json:"public_key"`
	KeyPin         [32]byte   `json:"key_pin"`
	SuggestedLabel string     `json:"suggested_label"`
	Status         string     `json:"status"` // "pending", "approved", "declined"
	CreatedNS      int64      `json:"created_ns"`
	UpdatedNS      int64      `json:"updated_ns"`
}

// RecordEnrollmentRequest saves a new join request proving private key possession.
func (db *DB) RecordEnrollmentRequest(ctx context.Context, req EnrollmentRequestRecord) error {
	if req.Status == "" {
		req.Status = "pending"
	}
	_, err := db.db.ExecContext(ctx,
		`INSERT INTO enrollment_requests(request_id, folder_id, device_id, public_key, key_pin, suggested_label, status, created_ns, updated_ns) VALUES(?,?,?,?,?,?,?,?,?)`,
		req.RequestID, req.Folder[:], req.DeviceID[:], req.PublicKey[:], req.KeyPin[:], req.SuggestedLabel, req.Status, req.CreatedNS, req.UpdatedNS,
	)
	if err != nil {
		return fmt.Errorf("insert enrollment request: %w", err)
	}
	return nil
}

// GetEnrollmentRequest retrieves an enrollment request by ID.
func (db *DB) GetEnrollmentRequest(ctx context.Context, requestID string) (*EnrollmentRequestRecord, error) {
	var rawFolder, rawDevice, rawPub, rawPin []byte
	var label, status string
	var createdNS, updatedNS int64

	err := db.db.QueryRowContext(ctx,
		`SELECT folder_id, device_id, public_key, key_pin, suggested_label, status, created_ns, updated_ns FROM enrollment_requests WHERE request_id=?`,
		requestID,
	).Scan(&rawFolder, &rawDevice, &rawPub, &rawPin, &label, &status, &createdNS, &updatedNS)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRequestNotFound
		}
		return nil, err
	}

	rec := &EnrollmentRequestRecord{
		RequestID:      requestID,
		SuggestedLabel: label,
		Status:         status,
		CreatedNS:      createdNS,
		UpdatedNS:      updatedNS,
	}
	copy(rec.Folder[:], rawFolder)
	copy(rec.DeviceID[:], rawDevice)
	copy(rec.PublicKey[:], rawPub)
	copy(rec.KeyPin[:], rawPin)
	return rec, nil
}

// UpdateEnrollmentRequestStatus updates the approval status of a join request.
func (db *DB) UpdateEnrollmentRequestStatus(ctx context.Context, requestID string, status string) error {
	now := time.Now().UnixNano()
	res, err := db.db.ExecContext(ctx,
		`UPDATE enrollment_requests SET status=?, updated_ns=? WHERE request_id=?`,
		status, now, requestID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrRequestNotFound
	}
	return nil
}

// ListEnrollmentRequests queries enrollment requests for a workspace with optional status filter.
func (db *DB) ListEnrollmentRequests(ctx context.Context, folder history.ID, statusFilter string) ([]EnrollmentRequestRecord, error) {
	var query string
	var args []any
	if statusFilter != "" {
		query = `SELECT request_id, folder_id, device_id, public_key, key_pin, suggested_label, status, created_ns, updated_ns FROM enrollment_requests WHERE folder_id=? AND status=? ORDER BY created_ns DESC`
		args = []any{folder[:], statusFilter}
	} else {
		query = `SELECT request_id, folder_id, device_id, public_key, key_pin, suggested_label, status, created_ns, updated_ns FROM enrollment_requests WHERE folder_id=? ORDER BY created_ns DESC`
		args = []any{folder[:]}
	}

	rows, err := db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []EnrollmentRequestRecord
	for rows.Next() {
		var reqID, label, status string
		var rawFolder, rawDevice, rawPub, rawPin []byte
		var createdNS, updatedNS int64
		if err := rows.Scan(&reqID, &rawFolder, &rawDevice, &rawPub, &rawPin, &label, &status, &createdNS, &updatedNS); err != nil {
			return nil, err
		}
		rec := EnrollmentRequestRecord{
			RequestID:      reqID,
			SuggestedLabel: label,
			Status:         status,
			CreatedNS:      createdNS,
			UpdatedNS:      updatedNS,
		}
		copy(rec.Folder[:], rawFolder)
		copy(rec.DeviceID[:], rawDevice)
		copy(rec.PublicKey[:], rawPub)
		copy(rec.KeyPin[:], rawPin)
		result = append(result, rec)
	}
	return result, rows.Err()
}

// --- Setup State (O02/O03) ---

type SetupStateRecord struct {
	Phase           string
	RootPath        string
	DefaultFolderID *history.ID
	Completed       bool
	UpdatedNS       int64
}

// GetSetupState returns the current setup progress.
func (db *DB) GetSetupState(ctx context.Context) (*SetupStateRecord, error) {
	var phase, rootPath sql.NullString
	var rawFolder []byte
	var completedInt int
	var updatedNS int64

	err := db.db.QueryRowContext(ctx,
		`SELECT phase, root_path, default_folder_id, completed, updated_ns FROM setup_state WHERE id=1`,
	).Scan(&phase, &rootPath, &rawFolder, &completedInt, &updatedNS)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	rec := &SetupStateRecord{
		Completed: completedInt == 1,
		UpdatedNS: updatedNS,
	}
	if phase.Valid {
		rec.Phase = phase.String
	}
	if rootPath.Valid {
		rec.RootPath = rootPath.String
	}
	if len(rawFolder) == 32 {
		var fid history.ID
		copy(fid[:], rawFolder)
		rec.DefaultFolderID = &fid
	}
	return rec, nil
}

// SaveSetupState updates or inserts the singleton setup state.
func (db *DB) SaveSetupState(ctx context.Context, s SetupStateRecord) error {
	completedInt := 0
	if s.Completed {
		completedInt = 1
	}
	var rawFolder any
	if s.DefaultFolderID != nil {
		rawFolder = s.DefaultFolderID[:]
	}
	now := s.UpdatedNS
	if now == 0 {
		now = time.Now().UnixNano()
	}

	_, err := db.db.ExecContext(ctx,
		`INSERT INTO setup_state(id, phase, root_path, default_folder_id, completed, updated_ns) VALUES(1,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET phase=excluded.phase, root_path=excluded.root_path, default_folder_id=excluded.default_folder_id, completed=excluded.completed, updated_ns=excluded.updated_ns`,
		s.Phase, s.RootPath, rawFolder, completedInt, now,
	)
	return err
}

// --- Operation Progress (O02/O09/O10) ---

type OperationProgressRecord struct {
	OperationID         string
	Kind                string
	Phase               string
	ProgressNumerator   int64
	ProgressDenominator int64
	Details             string
	ErrorMessage        string
	Retryable           bool
	Canceled            bool
	CreatedNS           int64
	UpdatedNS           int64
}

// RecordOperationProgress inserts or updates a long-running operation progress record.
func (db *DB) RecordOperationProgress(ctx context.Context, rec OperationProgressRecord) error {
	retryable := 0
	if rec.Retryable {
		retryable = 1
	}
	canceled := 0
	if rec.Canceled {
		canceled = 1
	}
	if rec.CreatedNS == 0 {
		rec.CreatedNS = time.Now().UnixNano()
	}
	if rec.UpdatedNS == 0 {
		rec.UpdatedNS = time.Now().UnixNano()
	}

	_, err := db.db.ExecContext(ctx,
		`INSERT INTO operation_progress(operation_id, kind, phase, progress_numerator, progress_denominator, details, error_message, retryable, canceled, created_ns, updated_ns)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(operation_id) DO UPDATE SET
			phase=excluded.phase,
			progress_numerator=excluded.progress_numerator,
			progress_denominator=excluded.progress_denominator,
			details=excluded.details,
			error_message=excluded.error_message,
			retryable=excluded.retryable,
			canceled=excluded.canceled,
			updated_ns=excluded.updated_ns`,
		rec.OperationID, rec.Kind, rec.Phase, rec.ProgressNumerator, rec.ProgressDenominator, rec.Details, rec.ErrorMessage, retryable, canceled, rec.CreatedNS, rec.UpdatedNS,
	)
	return err
}

// GetOperationProgress retrieves the progress of an operation.
func (db *DB) GetOperationProgress(ctx context.Context, opID string) (*OperationProgressRecord, error) {
	var kind, phase, details, errMsg sql.NullString
	var num, den, createdNS, updatedNS int64
	var retryableInt, canceledInt int

	err := db.db.QueryRowContext(ctx,
		`SELECT kind, phase, progress_numerator, progress_denominator, details, error_message, retryable, canceled, created_ns, updated_ns FROM operation_progress WHERE operation_id=?`,
		opID,
	).Scan(&kind, &phase, &num, &den, &details, &errMsg, &retryableInt, &canceledInt, &createdNS, &updatedNS)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrOperationNotFound
		}
		return nil, err
	}

	rec := &OperationProgressRecord{
		OperationID:         opID,
		ProgressNumerator:   num,
		ProgressDenominator: den,
		Retryable:           retryableInt == 1,
		Canceled:            canceledInt == 1,
		CreatedNS:           createdNS,
		UpdatedNS:           updatedNS,
	}
	if kind.Valid {
		rec.Kind = kind.String
	}
	if phase.Valid {
		rec.Phase = phase.String
	}
	if details.Valid {
		rec.Details = details.String
	}
	if errMsg.Valid {
		rec.ErrorMessage = errMsg.String
	}
	return rec, nil
}

// CancelOperationProgress marks an operation as canceled.
func (db *DB) CancelOperationProgress(ctx context.Context, opID string) error {
	now := time.Now().UnixNano()
	res, err := db.db.ExecContext(ctx,
		`UPDATE operation_progress SET canceled=1, updated_ns=? WHERE operation_id=?`,
		now, opID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrOperationNotFound
	}
	return nil
}

// --- Read Leases & GC Protection (G03/O02/O07 - Invariant I25) ---

type ReadLeaseRecord struct {
	LeaseID        string
	Folder         history.ID
	VersionAuthor  history.ID
	VersionCounter uint64
	ExpiresNS      int64
	CreatedNS      int64
	ChunkDigests   []history.Digest
}

// AcquireReadLease creates a read lease and registers GC content pins for all chunks (Invariant I25).
func (db *DB) AcquireReadLease(ctx context.Context, lease ReadLeaseRecord) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	var leases int
	if err := db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM read_leases`).Scan(&leases); err != nil {
		return err
	}
	if leases >= 128 {
		return fmt.Errorf("read lease limit reached")
	}

	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if lease.CreatedNS == 0 {
		lease.CreatedNS = time.Now().UnixNano()
	}

	// 1. Insert read lease
	counterRaw := encodeUint(lease.VersionCounter)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO read_leases(lease_id, folder_id, version_author, version_counter, expires_ns, created_ns) VALUES(?,?,?,?,?,?)`,
		lease.LeaseID, lease.Folder[:], lease.VersionAuthor[:], counterRaw, lease.ExpiresNS, lease.CreatedNS,
	)
	if err != nil {
		return fmt.Errorf("insert read lease: %w", err)
	}

	// 2. Check availability/intents at the same serialized boundary as GC.
	seen := map[history.Digest]bool{}
	for _, chunk := range lease.ChunkDigests {
		if seen[chunk] {
			continue
		}
		seen[chunk] = true
		var verified, intents int
		if err := tx.QueryRowContext(ctx, `SELECT verified,(SELECT COUNT(*) FROM gc_intents WHERE digest=objects.digest) FROM objects WHERE digest=?`, chunk[:]).Scan(&verified, &intents); err != nil {
			return ErrContentMissing
		}
		if verified != 1 {
			return ErrContentMissing
		}
		if intents != 0 {
			return ErrGCIntentActive
		}
		if _, err := os.Stat(db.objectPath(chunk)); err != nil {
			return ErrContentMissing
		}

		_, err = tx.ExecContext(ctx,
			`INSERT INTO read_lease_chunks(lease_id, chunk_digest) VALUES(?,?)`,
			lease.LeaseID, chunk[:],
		)
		if err != nil {
			return fmt.Errorf("insert read lease chunk: %w", err)
		}
		// Pin chunk in content_pins so GC sweeps ignore it
		_, err = tx.ExecContext(ctx,
			`INSERT INTO content_pins(digest, owner_kind, owner_key, created_ns) VALUES(?,?,?,?) ON CONFLICT(digest, owner_kind, owner_key) DO NOTHING`,
			chunk[:], "read_lease", lease.LeaseID, lease.CreatedNS,
		)
		if err != nil {
			return fmt.Errorf("pin chunk for read lease: %w", err)
		}
	}

	return tx.Commit()
}

// ReleaseReadLease explicitly removes a read lease and unpins its chunks.
func (db *DB) ReleaseReadLease(ctx context.Context, leaseID string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Unpin from content_pins
	_, err = tx.ExecContext(ctx,
		`DELETE FROM content_pins WHERE owner_kind='read_lease' AND owner_key=?`,
		leaseID,
	)
	if err != nil {
		return err
	}

	// Delete from read_leases (cascades to read_lease_chunks)
	res, err := tx.ExecContext(ctx, `DELETE FROM read_leases WHERE lease_id=?`, leaseID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrLeaseNotFound
	}

	return tx.Commit()
}

// PruneExpiredReadLeases removes all read leases that have expired.
func (db *DB) PruneExpiredReadLeases(ctx context.Context, now time.Time) (int, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.pruneExpiredReadLeases(ctx, now)
}

func (db *DB) pruneExpiredReadLeases(ctx context.Context, now time.Time) (int, error) {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx,
		`SELECT lease_id FROM read_leases WHERE expires_ns <= ?`,
		now.UnixNano(),
	)
	if err != nil {
		return 0, err
	}
	var expiredIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			expiredIDs = append(expiredIDs, id)
		}
	}
	rows.Close()

	for _, id := range expiredIDs {
		_, _ = tx.ExecContext(ctx, `DELETE FROM content_pins WHERE owner_kind='read_lease' AND owner_key=?`, id)
		_, _ = tx.ExecContext(ctx, `DELETE FROM read_leases WHERE lease_id=?`, id)
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(expiredIDs), nil
}

// IsChunkLeased returns true if any unexpired read lease holds chunkDigest.
func (db *DB) IsChunkLeased(ctx context.Context, chunk history.Digest, now time.Time) (bool, error) {
	var count int
	err := db.db.QueryRowContext(ctx,
		`SELECT count(*) FROM read_lease_chunks c JOIN read_leases l ON c.lease_id = l.lease_id WHERE c.chunk_digest=? AND l.expires_ns > ?`,
		chunk[:], now.UnixNano(),
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// HasAnyCapturedVersions reports whether any version records exist in the database.
func (db *DB) HasAnyCapturedVersions(ctx context.Context) (bool, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	var exists int
	err := db.db.QueryRowContext(ctx, "SELECT 1 FROM versions LIMIT 1").Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check versions: %w", err)
	}
	return true, nil
}

// PruneExpiredInvitations deletes invitations where expires_ns <= cutoff or that are revoked/exhausted and created <= cutoff.
func (db *DB) PruneExpiredInvitations(ctx context.Context, cutoff time.Time) (int64, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	res, err := db.db.ExecContext(ctx, `DELETE FROM invitations WHERE expires_ns <= ? OR (revoked = 1 AND created_ns <= ?) OR (uses_count >= max_uses AND created_ns <= ?)`, cutoff.UnixNano(), cutoff.UnixNano(), cutoff.UnixNano())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// PruneTerminalEnrollmentRequests deletes enrollment requests in approved, declined, or expired states older than cutoff.
// Pending enrollment requests are strictly preserved (Invariant I28).
func (db *DB) PruneTerminalEnrollmentRequests(ctx context.Context, cutoff time.Time) (int64, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	res, err := db.db.ExecContext(ctx, `DELETE FROM enrollment_requests WHERE status IN ('approved', 'declined', 'expired') AND updated_ns <= ?`, cutoff.UnixNano())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// PruneFinishedOperations deletes operation progress records that are completed or failed older than cutoff.
// Active operations are strictly preserved.
func (db *DB) PruneFinishedOperations(ctx context.Context, cutoff time.Time) (int64, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	res, err := db.db.ExecContext(ctx, `DELETE FROM operation_progress WHERE (phase IN ('COMPLETED', 'FAILED') OR canceled = 1) AND updated_ns <= ?`, cutoff.UnixNano())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
