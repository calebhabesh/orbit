package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
)

var (
	ErrExpiredReplay       = errors.New("idempotency record has expired")
	ErrIdempotencyConflict = errors.New("idempotency key was previously used with different parameters")
)

type ContentAvailability string

const (
	ContentReady       ContentAvailability = "ready"
	ContentPending     ContentAvailability = "pending"
	ContentUnavailable ContentAvailability = "unavailable"
	ContentExpired     ContentAvailability = "expired"
	ContentCorrupt     ContentAvailability = "corrupt"
)

type ControlOpRecord struct {
	Status      string             `json:"status"` // "in_progress", "completed", "failed"
	Action      string             `json:"action"` // "select", "merge", "restore", "keep_copies"
	Error       string             `json:"error,omitempty"`
	ResolvedID  *history.VersionID `json:"resolved_id,omitempty"`
	Envelope    *history.Envelope  `json:"envelope,omitempty"`
	Applied     bool               `json:"applied,omitempty"`
	Copies      []CopyStepResult   `json:"copies,omitempty"`
	CompletedAt *time.Time         `json:"completed_at,omitempty"`
	Payload     json.RawMessage    `json:"payload,omitempty"`
}

type CopyStepResult struct {
	HeadID          history.VersionID `json:"head_id"`
	DestinationPath string            `json:"destination_path"`
	VersionID       history.VersionID `json:"version_id"`
	Completed       bool              `json:"completed"`
}

type ResolutionVersionRequest struct {
	Folder            history.ID
	Path              string
	Reviewed          []history.VersionID
	ExpectedHeadToken history.Digest
	Kind              history.Kind
	Manifest          *history.Manifest
	AuthoredRevision  uint64
	DisplayTime       string
}

// CreateResolutionVersion creates a new resolution or restore version whose parents are the reviewed heads,
// advancing the local author counter atomically in a single transaction.
func (db *DB) CreateResolutionVersion(ctx context.Context, request ResolutionVersionRequest) (history.Envelope, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if err := db.checkMetadataBudget(ctx); err != nil {
		return history.Envelope{}, err
	}
	if err := db.checkFreeSpaceReserve(ctx); err != nil {
		return history.Envelope{}, err
	}
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return history.Envelope{}, err
	}
	defer tx.Rollback()

	var authorRaw, counterRaw, revRaw []byte
	if err := tx.QueryRowContext(ctx, `SELECT local_author,next_counter,membership_revision FROM folders WHERE folder_id=?`, request.Folder[:]).Scan(&authorRaw, &counterRaw, &revRaw); errors.Is(err, sql.ErrNoRows) {
		return history.Envelope{}, ErrFolderUnknown
	} else if err != nil {
		return history.Envelope{}, err
	}

	var author history.ID
	if len(authorRaw) != len(author) {
		return history.Envelope{}, errors.New("invalid stored author")
	}
	copy(author[:], authorRaw)

	counter, err := decodeUint(counterRaw)
	if err != nil {
		return history.Envelope{}, err
	}
	if counter == math.MaxUint64 {
		return history.Envelope{}, history.ErrCounterOverflow
	}
	counter++

	authoredRevision := request.AuthoredRevision
	if authoredRevision == 0 {
		authoredRevision, _ = decodeUint(revRaw)
	}

	h, err := loadHistory(ctx, tx, request.Folder)
	if err != nil {
		return history.Envelope{}, err
	}

	parents, vector, err := h.PlanResolutionCapture(request.Folder, author, request.Path, request.Reviewed, request.ExpectedHeadToken, counter)
	if err != nil {
		return history.Envelope{}, err
	}

	envelope := history.Envelope{
		ID:               history.VersionID{Folder: request.Folder, Author: author, Counter: counter},
		Path:             request.Path,
		Parents:          parents,
		Vector:           vector,
		Kind:             request.Kind,
		Manifest:         cloneManifest(request.Manifest),
		AuthoredRevision: authoredRevision,
		DisplayTime:      request.DisplayTime,
	}

	if err := h.Accept(envelope); err != nil {
		return history.Envelope{}, err
	}

	if envelope.Kind == history.KindFile {
		if err := db.VerifyManifest(envelope.Manifest); err != nil {
			return history.Envelope{}, err
		}
		if err := ensureManifestObjects(ctx, tx, envelope.Manifest); err != nil {
			return history.Envelope{}, err
		}
	}

	if err := insertEnvelope(ctx, tx, envelope, "ready"); err != nil {
		return history.Envelope{}, err
	}

	result, err := tx.ExecContext(ctx, `UPDATE folders SET next_counter=? WHERE folder_id=? AND next_counter=?`, encodeUint(counter), request.Folder[:], counterRaw)
	if err != nil {
		return history.Envelope{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return history.Envelope{}, errors.New("local counter changed concurrently")
	}

	if err := addObjectReferences(ctx, tx, envelope); err != nil {
		return history.Envelope{}, err
	}

	if err := db.callHook(HookBeforeVersionCommit); err != nil {
		return history.Envelope{}, err
	}
	if err := tx.Commit(); err != nil {
		return history.Envelope{}, fmt.Errorf("commit resolution version: %w", err)
	}
	if err := db.callHook(HookAfterVersionCommit); err != nil {
		return history.Envelope{}, err
	}

	return envelope, nil
}

type CopyVersionRequest struct {
	Folder           history.ID
	Path             string
	Kind             history.Kind
	Manifest         *history.Manifest
	AuthoredRevision uint64
	DisplayTime      string
}

// CreateCopyVersion creates a new file or directory version at a new path without prior ancestry,
// leaving workspace publication to safely install it on disk.
func (db *DB) CreateCopyVersion(ctx context.Context, request CopyVersionRequest) (history.Envelope, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return history.Envelope{}, err
	}
	defer tx.Rollback()

	var authorRaw, counterRaw, revRaw []byte
	if err := tx.QueryRowContext(ctx, `SELECT local_author,next_counter,membership_revision FROM folders WHERE folder_id=?`, request.Folder[:]).Scan(&authorRaw, &counterRaw, &revRaw); errors.Is(err, sql.ErrNoRows) {
		return history.Envelope{}, ErrFolderUnknown
	} else if err != nil {
		return history.Envelope{}, err
	}

	var author history.ID
	if len(authorRaw) != len(author) {
		return history.Envelope{}, errors.New("invalid stored author")
	}
	copy(author[:], authorRaw)

	counter, err := decodeUint(counterRaw)
	if err != nil {
		return history.Envelope{}, err
	}
	if counter == math.MaxUint64 {
		return history.Envelope{}, history.ErrCounterOverflow
	}
	counter++

	authoredRevision := request.AuthoredRevision
	if authoredRevision == 0 {
		authoredRevision, _ = decodeUint(revRaw)
	}

	h, err := loadHistory(ctx, tx, request.Folder)
	if err != nil {
		return history.Envelope{}, err
	}

	envelope := history.Envelope{
		ID:               history.VersionID{Folder: request.Folder, Author: author, Counter: counter},
		Path:             request.Path,
		Parents:          nil,
		Vector:           []history.ClockEntry{{Author: author, Counter: counter}},
		Kind:             request.Kind,
		Manifest:         cloneManifest(request.Manifest),
		AuthoredRevision: authoredRevision,
		DisplayTime:      request.DisplayTime,
	}

	if err := h.Accept(envelope); err != nil {
		return history.Envelope{}, err
	}

	if envelope.Kind == history.KindFile {
		if err := db.VerifyManifest(envelope.Manifest); err != nil {
			return history.Envelope{}, err
		}
		if err := ensureManifestObjects(ctx, tx, envelope.Manifest); err != nil {
			return history.Envelope{}, err
		}
	}

	if err := insertEnvelope(ctx, tx, envelope, "ready"); err != nil {
		return history.Envelope{}, err
	}

	result, err := tx.ExecContext(ctx, `UPDATE folders SET next_counter=? WHERE folder_id=? AND next_counter=?`, encodeUint(counter), request.Folder[:], counterRaw)
	if err != nil {
		return history.Envelope{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return history.Envelope{}, errors.New("local counter changed concurrently")
	}

	if err := addObjectReferences(ctx, tx, envelope); err != nil {
		return history.Envelope{}, err
	}

	if err := db.callHook(HookBeforeVersionCommit); err != nil {
		return history.Envelope{}, err
	}
	if err := tx.Commit(); err != nil {
		return history.Envelope{}, fmt.Errorf("commit copy version: %w", err)
	}
	if err := db.callHook(HookAfterVersionCommit); err != nil {
		return history.Envelope{}, err
	}

	return envelope, nil
}

// GetControlOperation returns the stored control operation record if found, or nil if not found.
// If found and expired, it returns ErrExpiredReplay.
func (db *DB) GetControlOperation(ctx context.Context, key string) (*ControlOpRecord, history.Digest, int64, error) {
	var digestRaw []byte
	var generation int64
	var resultRaw []byte
	var expiresNS int64
	err := db.db.QueryRowContext(ctx, `SELECT request_digest, expected_generation, result, expires_ns FROM control_operations WHERE operation_key=?`, key).Scan(&digestRaw, &generation, &resultRaw, &expiresNS)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, history.Digest{}, 0, nil
	}
	if err != nil {
		return nil, history.Digest{}, 0, err
	}
	if expiresNS > 0 && time.Now().UnixNano() > expiresNS {
		return nil, history.Digest{}, 0, ErrExpiredReplay
	}
	var digest history.Digest
	copy(digest[:], digestRaw)
	var record ControlOpRecord
	if len(resultRaw) > 0 {
		if err := json.Unmarshal(resultRaw, &record); err != nil {
			return nil, digest, generation, fmt.Errorf("corrupt control operation record: %w", err)
		}
	}
	return &record, digest, generation, nil
}

// PutControlOperation saves or updates a control operation record.
func (db *DB) PutControlOperation(ctx context.Context, key string, digest history.Digest, generation int64, record *ControlOpRecord, expiresAt time.Time) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	var resultRaw []byte
	if record != nil {
		var err error
		resultRaw, err = json.Marshal(record)
		if err != nil {
			return err
		}
	}
	var expiresNS int64
	if !expiresAt.IsZero() {
		expiresNS = expiresAt.UnixNano()
	}
	_, err := db.db.ExecContext(ctx, `INSERT INTO control_operations(operation_key, request_digest, expected_generation, result, expires_ns) VALUES(?,?,?,?,?) ON CONFLICT(operation_key) DO UPDATE SET request_digest=excluded.request_digest, expected_generation=excluded.expected_generation, result=excluded.result, expires_ns=excluded.expires_ns`, key, digest[:], generation, resultRaw, expiresNS)
	return err
}

// PruneControlOperations removes expired control operation records.
func (db *DB) PruneControlOperations(ctx context.Context, cutoff time.Time) (int64, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	res, err := db.db.ExecContext(ctx, `DELETE FROM control_operations WHERE expires_ns > 0 AND expires_ns <= ?`, cutoff.UnixNano())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SetRetention records or updates retention expiration for a version.
func (db *DB) SetRetention(ctx context.Context, id history.VersionID, retainUntil time.Time, pin bool) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	var retainNS int64
	if !retainUntil.IsZero() {
		retainNS = retainUntil.UnixNano()
	}
	pinInt := 0
	if pin {
		pinInt = 1
	}
	_, err := db.db.ExecContext(ctx, `INSERT INTO retention_records(folder_id, author_id, counter, retain_until_ns, explicit_pin) VALUES(?,?,?,?,?) ON CONFLICT(folder_id, author_id, counter) DO UPDATE SET retain_until_ns=excluded.retain_until_ns, explicit_pin=excluded.explicit_pin`, id.Folder[:], id.Author[:], encodeUint(id.Counter), retainNS, pinInt)
	return err
}

// ExpireContent marks a version's content as unavailable and sets its retention expiration in the past.
func (db *DB) ExpireContent(ctx context.Context, id history.VersionID) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE versions SET content_state='unavailable' WHERE folder_id=? AND author_id=? AND counter=?`, id.Folder[:], id.Author[:], encodeUint(id.Counter)); err != nil {
		return err
	}
	pastNS := time.Now().Add(-time.Hour).UnixNano()
	if _, err := tx.ExecContext(ctx, `INSERT INTO retention_records(folder_id, author_id, counter, retain_until_ns, explicit_pin) VALUES(?,?,?,?,0) ON CONFLICT(folder_id, author_id, counter) DO UPDATE SET retain_until_ns=excluded.retain_until_ns, explicit_pin=0`, id.Folder[:], id.Author[:], encodeUint(id.Counter), pastNS); err != nil {
		return err
	}
	return tx.Commit()
}

// ContentAvailability reports whether a version's content is ready, pending, unavailable, or expired.
func (db *DB) ContentAvailability(ctx context.Context, id history.VersionID) (ContentAvailability, error) {
	var kind int
	var state string
	err := db.db.QueryRowContext(ctx, `SELECT kind, content_state FROM versions WHERE folder_id=? AND author_id=? AND counter=?`, id.Folder[:], id.Author[:], encodeUint(id.Counter)).Scan(&kind, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("version not found: %v", id)
	}
	if err != nil {
		return "", err
	}
	if history.Kind(kind) != history.KindFile {
		return ContentReady, nil
	}
	if state == "pending" {
		return ContentPending, nil
	}

	// Check if any chunk is in quarantine
	var quarantinedCount int
	_ = db.db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM manifest_chunks mc
		JOIN quarantined_chunks qc ON qc.digest=mc.digest AND qc.repaired=0
		WHERE mc.folder_id=? AND mc.author_id=? AND mc.counter=?
	`, id.Folder[:], id.Author[:], encodeUint(id.Counter)).Scan(&quarantinedCount)
	if quarantinedCount > 0 {
		return ContentCorrupt, nil
	}

	var retainNS sql.NullInt64
	_ = db.db.QueryRowContext(ctx, `SELECT retain_until_ns FROM retention_records WHERE folder_id=? AND author_id=? AND counter=?`, id.Folder[:], id.Author[:], encodeUint(id.Counter)).Scan(&retainNS)
	isExpired := retainNS.Valid && retainNS.Int64 > 0 && time.Now().UnixNano() >= retainNS.Int64

	if state == "unavailable" {
		if isExpired {
			return ContentExpired, nil
		}
		return ContentUnavailable, nil
	}

	// state is "ready", verify objects exist on disk
	envelope, _, err := db.envelopeAndState(ctx, db.db, id)
	if err != nil {
		return "", err
	}
	if envelope.Manifest != nil {
		for _, chunk := range envelope.Manifest.Chunks {
			if _, err := os.Stat(db.objectPath(chunk.Digest)); err != nil {
				if isExpired {
					return ContentExpired, nil
				}
				return ContentUnavailable, nil
			}
		}
	}
	return ContentReady, nil
}

// PathActiveInRepository reports whether any non-tombstone causal heads exist for path.
func (db *DB) PathActiveInRepository(ctx context.Context, folder history.ID, path string) (bool, error) {
	h, err := loadHistoryReadOnly(ctx, db, folder)
	if err != nil {
		return false, err
	}
	heads := h.Heads(folder, path)
	for _, head := range heads {
		if head.Kind != history.KindTombstone {
			return true, nil
		}
	}
	return false, nil
}

// PathHistory returns all historical envelopes for path sorted by counter descending.
func (db *DB) PathHistory(ctx context.Context, folder history.ID, path string) ([]history.Envelope, error) {
	rows, err := db.db.QueryContext(ctx, `SELECT author_id, counter FROM versions WHERE folder_id=? AND path=? ORDER BY rowid DESC`, folder[:], path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []history.VersionID
	for rows.Next() {
		var a, c []byte
		if err := rows.Scan(&a, &c); err != nil {
			return nil, err
		}
		var author history.ID
		copy(author[:], a)
		counter, _ := decodeUint(c)
		ids = append(ids, history.VersionID{Folder: folder, Author: author, Counter: counter})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var envelopes []history.Envelope
	for _, id := range ids {
		env, _, err := db.envelopeAndState(ctx, db.db, id)
		if err != nil {
			return nil, err
		}
		envelopes = append(envelopes, env)
	}
	return envelopes, nil
}

// Heads returns the current causal heads for path in folder.
func (db *DB) Heads(ctx context.Context, folder history.ID, path string) ([]history.Envelope, error) {
	h, err := loadHistoryReadOnly(ctx, db, folder)
	if err != nil {
		return nil, err
	}
	return h.Heads(folder, path), nil
}
