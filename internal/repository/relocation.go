package repository

import (
	"context"
	"errors"
	"path/filepath"
)

// RelocateRoot changes only local registration and setup presentation. Causal
// versions, projections, counters, membership and registration identity survive.
func (db *DB) RelocateRoot(ctx context.Context, old, next RootRegistration) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if old.Folder != next.Folder || old.RegistrationID != next.RegistrationID || !filepath.IsAbs(next.Path) {
		return errors.New("invalid relocation registration")
	}
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT root_path FROM folders WHERE root_path IS NOT NULL AND folder_id!=?`, old.Folder[:])
	if err != nil {
		return err
	}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			return err
		}
		if pathsOverlap(next.Path, path) {
			rows.Close()
			return errors.New("destination overlaps another registered workspace")
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	paused := 0
	if old.Paused {
		paused = 1
	}
	result, err := tx.ExecContext(ctx, `UPDATE folders SET root_path=?,root_device=?,root_inode=?,paused=?,pause_reason=? WHERE folder_id=? AND root_path=? AND registration_id=?`, next.Path, int64(next.Device), int64(next.Inode), paused, old.PauseReason, old.Folder[:], old.Path, old.RegistrationID[:])
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrStaleGeneration
	}
	if _, err := tx.ExecContext(ctx, `UPDATE setup_state SET root_path=? WHERE default_folder_id=?`, next.Path, old.Folder[:]); err != nil {
		return err
	}
	return tx.Commit()
}
