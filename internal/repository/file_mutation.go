package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
)

var (
	ErrFileMutationNotFound = errors.New("file mutation record not found")
)

// FileMutationRecord tracks durable multi-phase file operations (G03/O09).
type FileMutationRecord struct {
	OperationID    string
	Folder         history.ID
	Action         string // "import", "mkdir", "move", "delete"
	SourcePath     string
	DestPath       string
	ReviewedToken  string
	Overwrite      bool
	Phase          string // "PLANNED", "STAGED", "INSTALLED", "SOURCE_VERIFIED", "COMPLETED", "ABORTED"
	StagePath      string
	RecoveryPath   string
	SourceRetained bool
	Details        string
	CreatedNS      int64
	UpdatedNS      int64
}

// FileMutationEntry tracks individual entry progress in batch/directory operations.
type FileMutationEntry struct {
	OperationID  string
	Position     int
	SourcePath   string
	DestPath     string
	Kind         history.Kind
	Phase        string // "PENDING", "COMPLETED", "SKIPPED_RETAINED", "FAILED"
	ErrorMessage string
}

// RecordFileMutation inserts or updates a file mutation journal record.
func (db *DB) RecordFileMutation(ctx context.Context, m FileMutationRecord) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	now := time.Now().UnixNano()
	created := m.CreatedNS
	if created == 0 {
		created = now
	}
	updated := now

	overwriteVal := 0
	if m.Overwrite {
		overwriteVal = 1
	}
	retainedVal := 0
	if m.SourceRetained {
		retainedVal = 1
	}

	_, err := db.db.ExecContext(ctx,
		`INSERT INTO file_mutations(
			operation_id, folder_id, action, source_path, dest_path,
			reviewed_token, overwrite, phase, stage_path, recovery_path,
			source_retained, details, created_ns, updated_ns
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(operation_id) DO UPDATE SET
			phase = excluded.phase,
			stage_path = excluded.stage_path,
			recovery_path = excluded.recovery_path,
			source_retained = excluded.source_retained,
			details = excluded.details,
			updated_ns = excluded.updated_ns`,
		m.OperationID, m.Folder[:], m.Action, m.SourcePath, m.DestPath,
		m.ReviewedToken, overwriteVal, m.Phase, m.StagePath, m.RecoveryPath,
		retainedVal, m.Details, created, updated,
	)
	return err
}

// SetFileMutationPhase updates the phase and timestamp of an existing mutation.
func (db *DB) SetFileMutationPhase(ctx context.Context, opID, phase string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	now := time.Now().UnixNano()
	res, err := db.db.ExecContext(ctx, `UPDATE file_mutations SET phase=?, updated_ns=? WHERE operation_id=?`, phase, now, opID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrFileMutationNotFound
	}
	return nil
}

// SetFileMutationSourceRetained flags whether the source file was retained due to a concurrent race.
func (db *DB) SetFileMutationSourceRetained(ctx context.Context, opID string, retained bool) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	val := 0
	if retained {
		val = 1
	}
	now := time.Now().UnixNano()
	_, err := db.db.ExecContext(ctx, `UPDATE file_mutations SET source_retained=?, updated_ns=? WHERE operation_id=?`, val, now, opID)
	return err
}

// GetFileMutation retrieves a mutation record by operation ID.
func (db *DB) GetFileMutation(ctx context.Context, opID string) (*FileMutationRecord, error) {
	var m FileMutationRecord
	var folderRaw []byte
	var overwriteVal, retainedVal int
	var srcPath, dstPath, revToken, stgPath, recPath, details sql.NullString

	err := db.db.QueryRowContext(ctx,
		`SELECT operation_id, folder_id, action, source_path, dest_path,
		        reviewed_token, overwrite, phase, stage_path, recovery_path,
		        source_retained, details, created_ns, updated_ns
		 FROM file_mutations WHERE operation_id=?`, opID,
	).Scan(
		&m.OperationID, &folderRaw, &m.Action, &srcPath, &dstPath,
		&revToken, &overwriteVal, &m.Phase, &stgPath, &recPath,
		&retainedVal, &details, &m.CreatedNS, &m.UpdatedNS,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrFileMutationNotFound
	}
	if err != nil {
		return nil, err
	}

	copy(m.Folder[:], folderRaw)
	m.SourcePath = srcPath.String
	m.DestPath = dstPath.String
	m.ReviewedToken = revToken.String
	m.StagePath = stgPath.String
	m.RecoveryPath = recPath.String
	m.Details = details.String
	m.Overwrite = overwriteVal == 1
	m.SourceRetained = retainedVal == 1
	return &m, nil
}

// ListIncompleteFileMutations retrieves all mutations that have not reached COMPLETED or ABORTED.
func (db *DB) ListIncompleteFileMutations(ctx context.Context, folder history.ID) ([]FileMutationRecord, error) {
	rows, err := db.db.QueryContext(ctx,
		`SELECT operation_id, folder_id, action, source_path, dest_path,
		        reviewed_token, overwrite, phase, stage_path, recovery_path,
		        source_retained, details, created_ns, updated_ns
		 FROM file_mutations
		 WHERE folder_id=? AND phase NOT IN ('COMPLETED', 'ABORTED')
		 ORDER BY created_ns ASC`, folder[:],
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []FileMutationRecord
	for rows.Next() {
		var m FileMutationRecord
		var folderRaw []byte
		var overwriteVal, retainedVal int
		var srcPath, dstPath, revToken, stgPath, recPath, details sql.NullString

		if err := rows.Scan(
			&m.OperationID, &folderRaw, &m.Action, &srcPath, &dstPath,
			&revToken, &overwriteVal, &m.Phase, &stgPath, &recPath,
			&retainedVal, &details, &m.CreatedNS, &m.UpdatedNS,
		); err != nil {
			return nil, err
		}

		copy(m.Folder[:], folderRaw)
		m.SourcePath = srcPath.String
		m.DestPath = dstPath.String
		m.ReviewedToken = revToken.String
		m.StagePath = stgPath.String
		m.RecoveryPath = recPath.String
		m.Details = details.String
		m.Overwrite = overwriteVal == 1
		m.SourceRetained = retainedVal == 1
		list = append(list, m)
	}
	return list, rows.Err()
}

// RemoveFileMutation deletes a completed or aborted mutation record.
func (db *DB) RemoveFileMutation(ctx context.Context, opID string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, err := db.db.ExecContext(ctx, `DELETE FROM file_mutations WHERE operation_id=? AND phase IN ('COMPLETED', 'ABORTED')`, opID)
	return err
}

// RecordFileMutationEntries inserts a batch of child entries for directory operations.
func (db *DB) RecordFileMutationEntries(ctx context.Context, opID string, entries []FileMutationEntry) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO file_mutation_entries(operation_id, position, source_path, dest_path, kind, phase, error_message)
		 VALUES(?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(operation_id, position) DO UPDATE SET
			phase = excluded.phase,
			error_message = excluded.error_message`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range entries {
		var errMsg sql.NullString
		if e.ErrorMessage != "" {
			errMsg = sql.NullString{String: e.ErrorMessage, Valid: true}
		}
		var dstPath sql.NullString
		if e.DestPath != "" {
			dstPath = sql.NullString{String: e.DestPath, Valid: true}
		}
		if _, err := stmt.ExecContext(ctx, opID, e.Position, e.SourcePath, dstPath, int(e.Kind), e.Phase, errMsg); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetFileMutationEntryPhase updates an entry's phase and optional error message.
func (db *DB) SetFileMutationEntryPhase(ctx context.Context, opID string, pos int, phase, errMsg string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	var errVal sql.NullString
	if errMsg != "" {
		errVal = sql.NullString{String: errMsg, Valid: true}
	}
	_, err := db.db.ExecContext(ctx,
		`UPDATE file_mutation_entries SET phase=?, error_message=? WHERE operation_id=? AND position=?`,
		phase, errVal, opID, pos,
	)
	return err
}

// GetFileMutationEntries retrieves all entries for a given operation.
func (db *DB) GetFileMutationEntries(ctx context.Context, opID string) ([]FileMutationEntry, error) {
	rows, err := db.db.QueryContext(ctx,
		`SELECT position, source_path, dest_path, kind, phase, error_message
		 FROM file_mutation_entries WHERE operation_id=? ORDER BY position ASC`, opID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []FileMutationEntry
	for rows.Next() {
		var e FileMutationEntry
		e.OperationID = opID
		var dstPath, errMsg sql.NullString
		var kindInt int
		if err := rows.Scan(&e.Position, &e.SourcePath, &dstPath, &kindInt, &e.Phase, &errMsg); err != nil {
			return nil, err
		}
		e.DestPath = dstPath.String
		e.Kind = history.Kind(kindInt)
		e.ErrorMessage = errMsg.String
		list = append(list, e)
	}
	return list, rows.Err()
}
