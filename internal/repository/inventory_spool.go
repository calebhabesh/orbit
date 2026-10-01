package repository

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// InventorySpool bounds memory while an authenticated inventory is received.
// It is transient transport state: restarting a session fetches a new snapshot.
// Its bytes count as incoming data and cannot exceed the metadata admission cap.
type InventorySpool struct {
	*os.File
	db *DB
}

func (db *DB) NewInventorySpool(ctx context.Context) (*InventorySpool, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if err := db.checkFreeSpaceReserve(ctx); err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(filepath.Join(db.stateDir, "incoming"), "inventory-*")
	if err != nil {
		return nil, err
	}
	return &InventorySpool{File: file, db: db}, nil
}

func (spool *InventorySpool) Write(data []byte) (int, error) {
	spool.db.mu.Lock()
	defer spool.db.mu.Unlock()
	ctx := context.Background()
	if err := spool.db.checkFreeSpaceReserve(ctx); err != nil {
		return 0, err
	}
	info, err := spool.Stat()
	if err != nil {
		return 0, err
	}
	bytes := uint64(len(data))
	if bytes > spool.db.metadataBudgetBytes || uint64(info.Size()) > spool.db.metadataBudgetBytes-bytes {
		return 0, ErrBudgetExceeded
	}
	if spool.db.budgetBytes > 0 {
		usage, err := spool.db.usageUnlocked()
		if err != nil {
			return 0, err
		}
		reserved, err := spool.db.reservedBytes(ctx)
		if err != nil {
			return 0, err
		}
		used := usage.Total()
		if bytes > spool.db.budgetBytes || used > spool.db.budgetBytes-bytes || reserved > spool.db.budgetBytes-used-bytes {
			return 0, ErrBudgetExceeded
		}
	}
	return spool.File.Write(data)
}

func (spool *InventorySpool) Close() error {
	return errors.Join(spool.File.Close(), os.Remove(spool.Name()))
}
