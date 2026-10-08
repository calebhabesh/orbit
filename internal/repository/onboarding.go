package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"

	"github.com/calebhabesh/orbit/internal/config"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"golang.org/x/sys/unix"
)

// AdoptionCapacity conservatively charges capture and duplicate staging; it
// credits no deduplication. Actual IO still uses normal reservation/admission.
func (db *DB) AdoptionCapacity(ctx context.Context, root string, size, entries uint64, s tc.Settings) (uint64, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if size > math.MaxUint64/2 || entries > math.MaxUint64/4096 {
		return 0, ErrBudgetExceeded
	}
	u, err := db.usageUnlocked()
	if err != nil {
		return 0, err
	}
	reserved, err := db.reservedBytes(ctx)
	if err != nil {
		return 0, err
	}
	data := size * 2
	metadata := entries*4096 + 65536
	if data > uint64(s.DataBudget) || u.Total() > uint64(s.DataBudget)-data || reserved > uint64(s.DataBudget)-data-u.Total() {
		return 0, ErrBudgetExceeded
	}
	if metadata > uint64(s.MetadataBudget) || u.Metadata > uint64(s.MetadataBudget)-metadata {
		return 0, ErrMetadataBudgetExceeded
	}
	var state, work unix.Statfs_t
	if err = unix.Statfs(db.stateDir, &state); err != nil {
		return 0, err
	}
	if err = unix.Statfs(root, &work); err != nil {
		if err = unix.Statfs(filepath.Dir(root), &work); err != nil {
			return 0, err
		}
	}
	stateAvail := state.Bavail * uint64(state.Bsize)
	workAvail := work.Bavail * uint64(work.Bsize)
	reserve := uint64(s.ReserveBytes)
	need := size + metadata
	if state.Fsid == work.Fsid {
		need = data + metadata
	}
	if stateAvail < reserve || need > stateAvail-reserve || reserved > stateAvail-reserve-need {
		return 0, ErrStorageExhausted
	}
	if workAvail < reserve || size > workAvail-reserve {
		return 0, ErrStorageExhausted
	}
	if workAvail < stateAvail {
		stateAvail = workAvail
	}
	return stateAvail - reserve, nil
}
func (db *DB) ReloadStorageLimits() error {
	s, err := config.LoadStorageLimits(db.stateDir)
	if err != nil {
		return err
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	db.budgetBytes = s.DataBudgetBytes
	db.metadataBudgetBytes = s.MetadataBudgetBytes
	db.freeSpaceReserveBytes = s.FreeSpaceReserveBytes
	return nil
}
func (db *DB) OnboardingReadiness(ctx context.Context, folder history.ID, r *tc.Readiness) error {
	h, err := loadHistoryReadOnly(ctx, db, folder)
	if err != nil {
		return err
	}
	r.Conflicts = tc.Uint(len(h.StructuralConflicts(folder)))
	rows, err := db.db.QueryContext(ctx, `SELECT DISTINCT path FROM versions WHERE folder_id=?`, folder[:])
	if err != nil {
		return err
	}
	var paths []string
	for rows.Next() {
		var p string
		if err = rows.Scan(&p); err != nil {
			rows.Close()
			return err
		}
		paths = append(paths, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// Only readiness dimensions are needed here. Load durable availability,
	// quarantine and applied observations together rather than rereading every
	// already loaded envelope through ContentAvailability/WorkingApplied.
	type observation struct{ ready, applied bool }
	observations := make(map[history.VersionID]observation)
	rows, err = db.db.QueryContext(ctx, `SELECT v.author_id,v.counter,v.kind,v.content_state,p.applied_author,p.applied_counter,
        EXISTS(SELECT 1 FROM manifest_chunks mc JOIN quarantined_chunks qc ON qc.digest=mc.digest AND qc.repaired=0
            WHERE mc.folder_id=v.folder_id AND mc.author_id=v.author_id AND mc.counter=v.counter)
        FROM versions v LEFT JOIN path_projections p ON p.folder_id=v.folder_id AND p.path=v.path WHERE v.folder_id=?`, folder[:])
	if err != nil {
		return err
	}
	for rows.Next() {
		var author, counter, appliedAuthor, appliedCounter []byte
		var kind, quarantined int
		var state string
		if err = rows.Scan(&author, &counter, &kind, &state, &appliedAuthor, &appliedCounter, &quarantined); err != nil {
			rows.Close()
			return err
		}
		var id history.VersionID
		id.Folder = folder
		copy(id.Author[:], author)
		id.Counter, err = decodeUint(counter)
		if err != nil {
			rows.Close()
			return err
		}
		observations[id] = observation{ready: history.Kind(kind) != history.KindFile || (state == "ready" && quarantined == 0),
			applied: bytes.Equal(author, appliedAuthor) && bytes.Equal(counter, appliedCounter)}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		heads := h.Heads(folder, p)
		if len(heads) > 1 {
			r.Conflicts++
		}
		for _, head := range heads {
			observed := observations[head.ID]
			if !observed.ready {
				r.MissingContent++
				continue
			}
			if head.Manifest != nil {
				if err = db.VerifyManifest(head.Manifest); err != nil {
					r.MissingContent++
					continue
				}
			}
			if !observed.applied && len(heads) == 1 {
				r.PendingPublication++
			}
		}
	}
	pubs, err := db.Publications(ctx, folder)
	if err != nil {
		return err
	}
	for _, p := range pubs {
		if p.Phase != "COMMITTED" {
			r.PendingPublication++
		}
	}
	var blocked uint64
	err = db.db.QueryRowContext(ctx, `SELECT count(*) FROM path_projections WHERE folder_id=? AND block_reason IS NOT NULL AND block_reason!=''`, folder[:]).Scan(&blocked)
	r.Uncaptured += tc.Uint(blocked)
	return err
}
func IsAdmissionError(err error) bool {
	return errors.Is(err, ErrBudgetExceeded) || errors.Is(err, ErrMetadataBudgetExceeded) || errors.Is(err, ErrStorageExhausted)
}

// SaveOnboarding writes job and safe operation together. Network responses cannot
// leave the signed request scrubbed while the operation still says unsent.
func (db *DB) SaveOnboarding(ctx context.Context, id string, job any, record TerminalRecord, newRecord bool) error {
	j, err := json.Marshal(job)
	if err != nil {
		return err
	}
	r, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if len(j) > tc.MaxMetadata || len(r) > tc.MaxMetadata {
		return ErrMetadataBudgetExceeded
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if newRecord {
		if err = db.checkMetadataBudget(ctx); err != nil {
			return err
		}
	}
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for key, value := range map[string][]byte{"setupjob/" + id: j, "operation/" + id: r} {
		if newRecord {
			_, err = tx.ExecContext(ctx, `INSERT INTO installation_metadata(key,value) VALUES(?,?)`, "terminal/v1/"+key, value)
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE installation_metadata SET value=? WHERE key=?`, value, "terminal/v1/"+key)
		}
		if err != nil {
			return err
		}
	}
	if newRecord {
		plan := record.Mutation.Setup
		token := ""
		if plan != nil {
			token = plan.Preview.Token
		} else {
			token = record.Mutation.Join.Preview.Token
		}
		result, err := tx.ExecContext(ctx, `UPDATE installation_metadata SET value=CAST(json_set(value,'$.used',?) AS BLOB) WHERE key=? AND json_extract(value,'$.used')=''`, id, "terminal/v1/rootreview/"+token)
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
	}
	return tx.Commit()
}
