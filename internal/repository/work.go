package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
)

var (
	ErrQueueFull    = errors.New("queue bound of 1024 tasks exceeded")
	ErrTaskNotFound = errors.New("durable task not found")
)

const MaxQueueCapacity = 1024

type DurableTask struct {
	ID             string      `json:"id"`
	Folder         history.ID  `json:"folder"`
	Kind           string      `json:"kind"` // "scan", "sync", "repair", "gc"
	Peer           *history.ID `json:"peer,omitempty"`
	TargetPath     string      `json:"target_path,omitempty"`
	VersionAuthor  *history.ID `json:"version_author,omitempty"`
	VersionCounter *uint64     `json:"version_counter,omitempty"`
	State          string      `json:"state"` // "queued", "running", "retry", "exhausted", "completed", "canceled"
	Attempts       int         `json:"attempts"`
	MaxAttempts    int         `json:"max_attempts"`
	LastError      string      `json:"last_error,omitempty"`
	ErrorCode      string      `json:"error_code,omitempty"`
	RetryAfterNS   int64       `json:"retry_after_ns"`
	CreatedNS      int64       `json:"created_ns"`
	UpdatedNS      int64       `json:"updated_ns"`
	FileSize       uint64      `json:"file_size"`
	AgeCounter     int         `json:"age_counter"`
}

type TaskFilter struct {
	Folder history.ID
	State  string
	Kind   string
	Limit  int
}

func (db *DB) EnqueueDurableTask(ctx context.Context, task DurableTask) (string, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	var activeCount int
	if err := db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM durable_work_tasks WHERE state IN ('queued', 'running', 'retry')`).Scan(&activeCount); err != nil {
		return "", err
	}

	// Check coalescing: if an equivalent active task exists, return its ID without adding another row.
	if task.Kind == "scan" && task.TargetPath == "" {
		var existingID string
		err := db.db.QueryRowContext(ctx, `SELECT task_id FROM durable_work_tasks WHERE folder_id=? AND task_kind='scan' AND (target_path IS NULL OR target_path='') AND state IN ('queued', 'running', 'retry') LIMIT 1`, task.Folder[:]).Scan(&existingID)
		if err == nil {
			return existingID, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	} else if task.Kind == "sync" && task.Peer != nil {
		var existingID string
		err := db.db.QueryRowContext(ctx, `SELECT task_id FROM durable_work_tasks WHERE folder_id=? AND task_kind='sync' AND peer_id=? AND state IN ('queued', 'running', 'retry') LIMIT 1`, task.Folder[:], (*task.Peer)[:]).Scan(&existingID)
		if err == nil {
			return existingID, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}

	if activeCount >= MaxQueueCapacity {
		return "", ErrQueueFull
	}
	if err := db.checkMetadataBudget(ctx); err != nil {
		return "", err
	}

	if task.ID == "" {
		var raw [16]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return "", err
		}
		task.ID = hex.EncodeToString(raw[:])
	}
	now := time.Now().UnixNano()
	if task.CreatedNS == 0 {
		task.CreatedNS = now
	}
	task.UpdatedNS = now
	if task.State == "" {
		task.State = "queued"
	}
	if task.MaxAttempts <= 0 {
		task.MaxAttempts = 5
	}

	var peerRaw, authorRaw, counterRaw any
	if task.Peer != nil {
		peerRaw = (*task.Peer)[:]
	}
	if task.VersionAuthor != nil {
		authorRaw = (*task.VersionAuthor)[:]
	}
	if task.VersionCounter != nil {
		counterRaw = encodeUint(*task.VersionCounter)
	}

	_, err := db.db.ExecContext(ctx, `INSERT INTO durable_work_tasks(
		task_id, folder_id, task_kind, peer_id, target_path,
		version_author, version_counter, state, attempts, max_attempts,
		last_error, error_code, retry_after_ns, created_ns, updated_ns,
		file_size, age_counter
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		task.ID, task.Folder[:], task.Kind, peerRaw, task.TargetPath,
		authorRaw, counterRaw, task.State, task.Attempts, task.MaxAttempts,
		task.LastError, task.ErrorCode, task.RetryAfterNS, task.CreatedNS, task.UpdatedNS,
		int64(task.FileSize), task.AgeCounter,
	)
	if err != nil {
		return "", err
	}
	return task.ID, nil
}

func (db *DB) GetDurableTask(ctx context.Context, taskID string) (DurableTask, error) {
	row := db.db.QueryRowContext(ctx, `SELECT
		task_id, folder_id, task_kind, peer_id, target_path,
		version_author, version_counter, state, attempts, max_attempts,
		COALESCE(last_error, ''), COALESCE(error_code, ''), retry_after_ns,
		created_ns, updated_ns, file_size, age_counter
		FROM durable_work_tasks WHERE task_id=?`, taskID)
	return scanDurableTask(row)
}

func (db *DB) UpdateDurableTaskState(ctx context.Context, taskID string, state string, attempts int, lastError, errorCode string, retryAfterNS int64) error {
	if len(lastError) > 2048 {
		lastError = strings.ToValidUTF8(lastError[:2048], "?")
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.db.ExecContext(ctx, `UPDATE durable_work_tasks SET
		state=?, attempts=?, last_error=?, error_code=?, retry_after_ns=?, updated_ns=?
		WHERE task_id=?`,
		state, attempts, lastError, errorCode, retryAfterNS, time.Now().UnixNano(), taskID)
	return err
}

func (db *DB) IncrementDurableTaskAge(ctx context.Context, taskIDs []string) error {
	if len(taskIDs) == 0 {
		return nil
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `UPDATE durable_work_tasks SET age_counter = age_counter + 1, updated_ns = ? WHERE task_id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().UnixNano()
	for _, id := range taskIDs {
		if _, err := stmt.ExecContext(ctx, now, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) RecoverInFlightDurableTasks(ctx context.Context) (int, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	res, err := db.db.ExecContext(ctx, `UPDATE durable_work_tasks SET state='queued', updated_ns=? WHERE state='running'`, time.Now().UnixNano())
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

func (db *DB) ListDurableTasks(ctx context.Context, filter TaskFilter) ([]DurableTask, error) {
	query := `SELECT
		task_id, folder_id, task_kind, peer_id, target_path,
		version_author, version_counter, state, attempts, max_attempts,
		COALESCE(last_error, ''), COALESCE(error_code, ''), retry_after_ns,
		created_ns, updated_ns, file_size, age_counter
		FROM durable_work_tasks WHERE 1=1`
	var args []any
	if filter.Folder != (history.ID{}) {
		query += ` AND folder_id=?`
		args = append(args, filter.Folder[:])
	}
	if filter.State == "active" {
		query += ` AND state IN ('queued','running','retry')`
	} else if filter.State != "" {
		query += ` AND state=?`
		args = append(args, filter.State)
	}
	if filter.Kind != "" {
		query += ` AND task_kind=?`
		args = append(args, filter.Kind)
	}
	query += ` ORDER BY created_ns ASC`
	if filter.Limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, filter.Limit)
	}

	rows, err := db.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []DurableTask
	for rows.Next() {
		task, err := scanDurableTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (db *DB) CountQueuedDurableTasks(ctx context.Context) (int, error) {
	var count int
	err := db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM durable_work_tasks WHERE state IN ('queued', 'running', 'retry')`).Scan(&count)
	return count, err
}

func (db *DB) RetryDurableTask(ctx context.Context, taskID string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	res, err := db.db.ExecContext(ctx, `UPDATE durable_work_tasks SET state='queued', attempts=0, last_error=NULL, error_code=NULL, retry_after_ns=0, updated_ns=? WHERE task_id=?`, time.Now().UnixNano(), taskID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

func (db *DB) RetryAllExhaustedTasks(ctx context.Context, folder *history.ID) (int, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	query := `UPDATE durable_work_tasks SET state='queued', attempts=0, last_error=NULL, error_code=NULL, retry_after_ns=0, updated_ns=? WHERE state IN ('exhausted', 'retry')`
	var args []any
	args = append(args, time.Now().UnixNano())
	if folder != nil {
		query += ` AND folder_id=?`
		args = append(args, (*folder)[:])
	}
	res, err := db.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	return int(affected), err
}

func (db *DB) CancelDurableTask(ctx context.Context, taskID string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	res, err := db.db.ExecContext(ctx, `UPDATE durable_work_tasks SET state='canceled', updated_ns=? WHERE task_id=? AND state NOT IN ('completed', 'canceled')`, time.Now().UnixNano(), taskID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDurableTask(s rowScanner) (DurableTask, error) {
	var task DurableTask
	var folderRaw, peerRaw, authorRaw, counterRaw []byte
	var targetPath sql.NullString
	var fileSize int64

	err := s.Scan(
		&task.ID, &folderRaw, &task.Kind, &peerRaw, &targetPath,
		&authorRaw, &counterRaw, &task.State, &task.Attempts, &task.MaxAttempts,
		&task.LastError, &task.ErrorCode, &task.RetryAfterNS,
		&task.CreatedNS, &task.UpdatedNS, &fileSize, &task.AgeCounter,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return task, ErrTaskNotFound
	}
	if err != nil {
		return task, err
	}
	copy(task.Folder[:], folderRaw)
	if targetPath.Valid {
		task.TargetPath = targetPath.String
	}
	if len(peerRaw) == len(history.ID{}) {
		var p history.ID
		copy(p[:], peerRaw)
		task.Peer = &p
	}
	if len(authorRaw) == len(history.ID{}) {
		var a history.ID
		copy(a[:], authorRaw)
		task.VersionAuthor = &a
	}
	if len(counterRaw) > 0 {
		c, err := decodeUint(counterRaw)
		if err == nil {
			task.VersionCounter = &c
		}
	}
	if fileSize > 0 {
		task.FileSize = uint64(fileSize)
	}
	return task, nil
}

// PruneFinishedTasks deletes durable work tasks in completed or canceled state older than cutoff.
// Pending tasks (queued, running, retry) and diagnostic tasks (exhausted) are strictly preserved (Invariant I28).
func (db *DB) PruneFinishedTasks(ctx context.Context, cutoff time.Time) (int64, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	res, err := db.db.ExecContext(ctx, `DELETE FROM durable_work_tasks WHERE state IN ('completed', 'canceled') AND updated_ns <= ?`, cutoff.UnixNano())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
