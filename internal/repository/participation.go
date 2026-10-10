package repository

import (
	"context"
	"sync"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
)

// A folder's live exchanges share this barrier. Ending local participation
// cancels them, waits for their release, and excludes new exchanges until the
// durable marker and workspace change commit. Other folders keep running.
type folderExchange struct {
	mu       sync.Mutex
	stopping bool
	active   map[*byte]context.CancelFunc
	drained  chan struct{}
}

func (db *DB) exchange(folder history.ID) *folderExchange {
	db.exchangeMu.Lock()
	defer db.exchangeMu.Unlock()
	if db.exchanges == nil {
		db.exchanges = make(map[history.ID]*folderExchange)
	}
	g := db.exchanges[folder]
	if g == nil {
		g = &folderExchange{active: make(map[*byte]context.CancelFunc)}
		db.exchanges[folder] = g
	}
	return g
}

func (db *DB) BeginFolderExchange(ctx context.Context, folder history.ID) (context.Context, func(), error) {
	g := db.exchange(folder)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopping {
		return ctx, nil, ErrFolderLeft
	}
	left, err := db.FolderLeft(ctx, folder)
	if err != nil {
		return ctx, nil, err
	}
	if left {
		return ctx, nil, ErrFolderLeft
	}
	child, cancel := context.WithCancel(ctx)
	key := new(byte)
	g.active[key] = cancel
	var once sync.Once
	release := func() {
		once.Do(func() {
			cancel()
			g.mu.Lock()
			delete(g.active, key)
			if len(g.active) == 0 && g.drained != nil {
				close(g.drained)
				g.drained = nil
			}
			g.mu.Unlock()
		})
	}
	return child, release, nil
}

func (db *DB) EndFolderExchanges(ctx context.Context, folder history.ID, commit func(context.Context) error) error {
	g := db.exchange(folder)
	g.mu.Lock()
	if g.stopping {
		g.mu.Unlock()
		return ErrFolderLeft
	}
	g.stopping = true
	for _, cancel := range g.active {
		cancel()
	}
	var drained <-chan struct{}
	if len(g.active) != 0 {
		g.drained = make(chan struct{})
		drained = g.drained
	}
	g.mu.Unlock()
	defer func() { g.mu.Lock(); g.stopping = false; g.mu.Unlock() }()
	if drained != nil {
		select {
		case <-drained:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return commit(ctx)
}

// MarkDeviceRemoved stops local activity but keeps its root visible so the
// owner can read who refused it and explicitly Leave. No file is changed.
func (db *DB) MarkDeviceRemoved(ctx context.Context, folder, by history.ID, reportedOnly ...bool) error {
	reported := len(reportedOnly) > 0 && reportedOnly[0]
	return db.markDeviceRemoved(ctx, folder, nil, by, reported)
}

// MarkDeviceRemovedFromPeer atomically ignores a late refusal from a peer
// this device has already retired. Retirement and the marker share db.mu.
func (db *DB) MarkDeviceRemovedFromPeer(ctx context.Context, folder, peer, by history.ID, reportedOnly bool) error {
	return db.markDeviceRemoved(ctx, folder, &peer, by, reportedOnly)
}

func (db *DB) markDeviceRemoved(ctx context.Context, folder history.ID, peer *history.ID, by history.ID, reportedOnly bool) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if peer != nil {
		var retired int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM folders f JOIN membership_entries e ON e.folder_id=f.folder_id AND e.revision=f.membership_revision WHERE f.folder_id=? AND e.device_id=? AND e.state='retired'`, folder[:], peer[:]).Scan(&retired); err != nil {
			return err
		} else if retired != 0 {
			return nil
		}
	}
	reason := "DEVICE_REMOVED"
	if reportedOnly {
		reason = "DEVICE_REMOVED_REPORTED"
	}
	if _, err = tx.ExecContext(ctx, `UPDATE folders SET pause_reason=CASE WHEN removed_by IS NULL THEN ? ELSE pause_reason END, removed_by=COALESCE(removed_by,?), paused=1 WHERE folder_id=?`, reason, by[:], folder[:]); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE durable_work_tasks SET state='canceled', updated_ns=? WHERE folder_id=? AND state NOT IN ('completed','canceled')`, time.Now().UnixNano(), folder[:]); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) RemovedBy(ctx context.Context, folder history.ID) (history.ID, error) {
	var raw []byte
	err := db.db.QueryRowContext(ctx, `SELECT removed_by FROM folders WHERE folder_id=?`, folder[:]).Scan(&raw)
	var by history.ID
	copy(by[:], raw)
	return by, err
}
