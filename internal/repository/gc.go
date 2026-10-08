package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"golang.org/x/sys/unix"
)

type RetentionPolicy struct {
	RetentionDays int `json:"retention_days"`
	MinSuperseded int `json:"min_superseded"`
}

func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{RetentionDays: 30, MinSuperseded: 20}
}

func (db *DB) GetRetentionPolicy(ctx context.Context, folder history.ID) (RetentionPolicy, error) {
	var days, min int
	err := db.db.QueryRowContext(ctx, `SELECT retention_days, min_superseded FROM folder_retention WHERE folder_id=?`, folder[:]).Scan(&days, &min)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultRetentionPolicy(), nil
	}
	if err != nil {
		return RetentionPolicy{}, err
	}
	return RetentionPolicy{RetentionDays: days, MinSuperseded: min}, nil
}

func (db *DB) SetRetentionPolicy(ctx context.Context, folder history.ID, policy RetentionPolicy) error {
	if policy.RetentionDays < 0 || policy.MinSuperseded < 0 {
		return errors.New("retention days and min superseded must be non-negative")
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.db.ExecContext(ctx, `INSERT INTO folder_retention(folder_id, retention_days, min_superseded) VALUES(?,?,?) ON CONFLICT(folder_id) DO UPDATE SET retention_days=excluded.retention_days, min_superseded=excluded.min_superseded`, folder[:], policy.RetentionDays, policy.MinSuperseded)
	return err
}

func (db *DB) IsCleanupSuspended(ctx context.Context, folder history.ID) (bool, string, error) {
	var pendingMaint int
	err := db.db.QueryRowContext(ctx, `SELECT count(*) FROM resumable_maintenance WHERE folder_id=? AND phase != 'completed'`, folder[:]).Scan(&pendingMaint)
	if err != nil {
		return false, "", err
	}
	if pendingMaint > 0 {
		return true, "active configuration maintenance in progress", nil
	}

	var unapprovedRevs int
	err = db.db.QueryRowContext(ctx, `SELECT count(*) FROM membership_revisions WHERE folder_id=? AND approved=0`, folder[:]).Scan(&unapprovedRevs)
	if err != nil {
		return false, "", err
	}
	if unapprovedRevs > 0 {
		return true, "unapproved membership revision pending", nil
	}

	return false, "", nil
}

func (db *DB) AcquireServeLease(ctx context.Context, digest history.Digest, leaseID string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	var verified int
	err := db.db.QueryRowContext(ctx, `SELECT verified FROM objects WHERE digest=?`, digest[:]).Scan(&verified)
	if errors.Is(err, sql.ErrNoRows) || verified != 1 {
		return ErrContentMissing
	} else if err != nil {
		return err
	}

	// D4: Active GC intent excludes serve lease
	var intentGen int
	err = db.db.QueryRowContext(ctx, `SELECT generation FROM gc_intents WHERE digest=?`, digest[:]).Scan(&intentGen)
	if err == nil {
		return ErrGCIntentActive
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	_, err = db.db.ExecContext(ctx, `INSERT INTO content_pins(digest, owner_kind, owner_key, created_ns) VALUES(?,?,?,?) ON CONFLICT(digest, owner_kind, owner_key) DO NOTHING`, digest[:], "serve", leaseID, time.Now().UnixNano())
	return err
}

func (db *DB) ReleaseServeLease(ctx context.Context, digest history.Digest, leaseID string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.db.ExecContext(ctx, `DELETE FROM content_pins WHERE digest=? AND owner_kind='serve' AND owner_key=?`, digest[:], leaseID)
	return err
}

type versionMeta struct {
	id          history.VersionID
	path        string
	acquiredNS  int64
	explicitPin bool
	chunks      []history.Digest
}

// ComputeProtectedChunks computes all chunk digests that are currently protected
// under heads, fallback content, retention policy, active pins, serve leases, and in-flight journals.
func (db *DB) ComputeProtectedChunks(ctx context.Context, folder history.ID, policy *RetentionPolicy, now time.Time) (map[history.Digest]bool, error) {
	protected := make(map[history.Digest]bool)

	// Always protect any chunk with an active content pin (serve leases, transfers, restore, etc.)
	pinRows, err := db.db.QueryContext(ctx, `SELECT DISTINCT digest FROM content_pins`)
	if err != nil {
		return nil, err
	}
	defer pinRows.Close()
	for pinRows.Next() {
		var raw []byte
		if err := pinRows.Scan(&raw); err != nil {
			return nil, err
		}
		var d history.Digest
		copy(d[:], raw)
		protected[d] = true
	}
	if err := pinRows.Err(); err != nil {
		return nil, err
	}

	// Always protect chunks referenced by in-flight publication journals
	pubRows, err := db.db.QueryContext(ctx, `SELECT mc.digest FROM publication_journal pj JOIN manifest_chunks mc ON mc.folder_id=pj.folder_id AND mc.author_id=pj.intended_author AND mc.counter=pj.intended_counter`)
	if err != nil {
		return nil, err
	}
	defer pubRows.Close()
	for pubRows.Next() {
		var raw []byte
		if err := pubRows.Scan(&raw); err != nil {
			return nil, err
		}
		var d history.Digest
		copy(d[:], raw)
		protected[d] = true
	}
	if err := pubRows.Err(); err != nil {
		return nil, err
	}

	var folders []history.ID
	if folder != (history.ID{}) {
		folders = []history.ID{folder}
	} else {
		fRows, err := db.db.QueryContext(ctx, `SELECT folder_id FROM folders`)
		if err != nil {
			return nil, err
		}
		defer fRows.Close()
		for fRows.Next() {
			var raw []byte
			if err := fRows.Scan(&raw); err != nil {
				return nil, err
			}
			var f history.ID
			copy(f[:], raw)
			folders = append(folders, f)
		}
		if err := fRows.Err(); err != nil {
			return nil, err
		}
	}

	for _, f := range folders {
		suspended, _, err := db.IsCleanupSuspended(ctx, f)
		if err != nil {
			return nil, err
		}
		if suspended {
			// Configuration change / maintenance: suspend cleanup by protecting all chunks in this folder!
			allRefRows, err := db.db.QueryContext(ctx, `SELECT DISTINCT digest FROM object_references WHERE folder_id=?`, f[:])
			if err != nil {
				return nil, err
			}
			for allRefRows.Next() {
				var raw []byte
				if err := allRefRows.Scan(&raw); err != nil {
					allRefRows.Close()
					return nil, err
				}
				var d history.Digest
				copy(d[:], raw)
				protected[d] = true
			}
			allRefRows.Close()
			continue
		}

		fPolicy := DefaultRetentionPolicy()
		if policy != nil {
			fPolicy = *policy
		} else if f != (history.ID{}) {
			fPolicy, _ = db.GetRetentionPolicy(ctx, f)
		}

		// Load DAG heads for folder f
		tx, err := db.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		h, err := loadHistory(ctx, tx, f)
		_ = tx.Rollback()
		if err != nil {
			if errors.Is(err, ErrFolderUnknown) {
				continue
			}
			return nil, err
		}

		// Query all file versions for folder f
		vRows, err := db.db.QueryContext(ctx, `
			SELECT v.author_id, v.counter, v.path, v.acquired_ns, COALESCE(rr.explicit_pin, 0)
			FROM versions v
			LEFT JOIN retention_records rr ON rr.folder_id=v.folder_id AND rr.author_id=v.author_id AND rr.counter=v.counter
			WHERE v.folder_id=? AND v.kind=1
		`, f[:])
		if err != nil {
			return nil, err
		}
		var versions []versionMeta
		for vRows.Next() {
			var authorRaw, counterRaw []byte
			var path string
			var acqNS int64
			var pin int
			if err := vRows.Scan(&authorRaw, &counterRaw, &path, &acqNS, &pin); err != nil {
				vRows.Close()
				return nil, err
			}
			var author history.ID
			copy(author[:], authorRaw)
			counter, _ := decodeUint(counterRaw)
			versions = append(versions, versionMeta{
				id:          history.VersionID{Folder: f, Author: author, Counter: counter},
				path:        path,
				acquiredNS:  acqNS,
				explicitPin: pin == 1,
			})
		}
		vRows.Close()

		// Fetch chunks for each version
		for i := range versions {
			cRows, err := db.db.QueryContext(ctx, `SELECT digest FROM manifest_chunks WHERE folder_id=? AND author_id=? AND counter=? ORDER BY position`, f[:], versions[i].id.Author[:], encodeUint(versions[i].id.Counter))
			if err != nil {
				return nil, err
			}
			for cRows.Next() {
				var dRaw []byte
				if err := cRows.Scan(&dRaw); err != nil {
					cRows.Close()
					return nil, err
				}
				var d history.Digest
				copy(d[:], dRaw)
				versions[i].chunks = append(versions[i].chunks, d)
			}
			cRows.Close()
		}

		// Collect paths and compute heads per path
		pathMap := make(map[string]bool)
		for _, v := range versions {
			pathMap[v.path] = true
		}

		headVersionSet := make(map[history.VersionID]bool)
		for path := range pathMap {
			heads := h.Heads(f, path)
			for _, head := range heads {
				headVersionSet[head.ID] = true
			}
		}

		// 1. Protect all current heads
		for _, v := range versions {
			if headVersionSet[v.id] {
				for _, ch := range v.chunks {
					protected[ch] = true
				}
			}
		}

		// 2. Pending-publication fallback
		// For any path where head has content_state == 'pending' or active journal exists,
		// protect the applied version in path_projections
		projRows, err := db.db.QueryContext(ctx, `
			SELECT p.path, p.applied_author, p.applied_counter
			FROM path_projections p
			WHERE p.folder_id=? AND p.applied_author IS NOT NULL AND p.applied_counter IS NOT NULL
		`, f[:])
		if err != nil {
			return nil, err
		}
		type projItem struct {
			path    string
			author  history.ID
			counter uint64
		}
		var projList []projItem
		for projRows.Next() {
			var ppath string
			var appAuthRaw, appCountRaw []byte
			if err := projRows.Scan(&ppath, &appAuthRaw, &appCountRaw); err != nil {
				projRows.Close()
				return nil, err
			}
			var appAuth history.ID
			copy(appAuth[:], appAuthRaw)
			appCount, _ := decodeUint(appCountRaw)
			projList = append(projList, projItem{path: ppath, author: appAuth, counter: appCount})
		}
		projRows.Close()

		for _, item := range projList {
			appID := history.VersionID{Folder: f, Author: item.author, Counter: item.counter}

			// Check if any head for ppath is pending
			heads := h.Heads(f, item.path)
			headPending := false
			for _, head := range heads {
				ready, _ := db.ContentReady(ctx, head.ID)
				if !ready {
					headPending = true
					break
				}
			}
			var journalCount int
			_ = db.db.QueryRowContext(ctx, `SELECT count(*) FROM publication_journal WHERE folder_id=? AND path=?`, f[:], item.path).Scan(&journalCount)

			if headPending || journalCount > 0 {
				// appID is fallback content! Protect its chunks
				for _, v := range versions {
					if v.id == appID {
						for _, ch := range v.chunks {
							protected[ch] = true
						}
					}
				}
			}
		}

		// 3. Historical retention policy
		// Group superseded versions by path
		supersededByPath := make(map[string][]versionMeta)
		for _, v := range versions {
			if !headVersionSet[v.id] {
				supersededByPath[v.path] = append(supersededByPath[v.path], v)
			}
		}

		retentionDuration := time.Duration(fPolicy.RetentionDays) * 24 * time.Hour
		for _, sList := range supersededByPath {
			// Deterministic tie-break sorting: acquired_ns DESC, counter DESC, author DESC
			sort.Slice(sList, func(i, j int) bool {
				if sList[i].acquiredNS != sList[j].acquiredNS {
					return sList[i].acquiredNS > sList[j].acquiredNS
				}
				if sList[i].id.Counter != sList[j].id.Counter {
					return sList[i].id.Counter > sList[j].id.Counter
				}
				for k := range sList[i].id.Author {
					if sList[i].id.Author[k] != sList[j].id.Author[k] {
						return sList[i].id.Author[k] > sList[j].id.Author[k]
					}
				}
				return false
			})

			for idx, sv := range sList {
				// Explicit pin protects
				if sv.explicitPin {
					for _, ch := range sv.chunks {
						protected[ch] = true
					}
					continue
				}

				// Arm 1: Newer than RetentionDays since local durable acquisition.
				// Clock jump backward: now.Before(acquiredTime) protects against rollback!
				acquiredTime := time.Unix(0, sv.acquiredNS)
				if now.Before(acquiredTime) || now.Sub(acquiredTime) < retentionDuration {
					for _, ch := range sv.chunks {
						protected[ch] = true
					}
					continue
				}

				// Arm 2: Among the MinSuperseded newest locally retained superseded versions
				// Clock jump forward: top MinSuperseded are protected regardless of how far in future!
				if idx < fPolicy.MinSuperseded {
					for _, ch := range sv.chunks {
						protected[ch] = true
					}
					continue
				}
			}
		}
	}

	return protected, nil
}

type RetentionPreview struct {
	FolderID         history.ID      `json:"folder_id"`
	Policy           RetentionPolicy `json:"policy"`
	TotalVersions    int             `json:"total_versions"`
	HeadVersions     int             `json:"head_versions"`
	RetainedVersions int             `json:"retained_versions"`
	ExpiredVersions  int             `json:"expired_versions"`
	ProtectedChunks  int             `json:"protected_chunks"`
	CandidateChunks  int             `json:"candidate_chunks"`
	ReclaimableBytes uint64          `json:"reclaimable_bytes"`
	Suspended        bool            `json:"suspended"`
	SuspendReason    string          `json:"suspend_reason,omitempty"`
}

func (db *DB) RetentionPreview(ctx context.Context, folder history.ID, policy *RetentionPolicy, now time.Time) (*RetentionPreview, error) {
	activePolicy := DefaultRetentionPolicy()
	if policy != nil {
		activePolicy = *policy
	} else if folder != (history.ID{}) {
		var err error
		activePolicy, err = db.GetRetentionPolicy(ctx, folder)
		if err != nil {
			return nil, err
		}
	}

	suspended, reason, err := db.IsCleanupSuspended(ctx, folder)
	if err != nil {
		return nil, err
	}

	preview := &RetentionPreview{
		FolderID:      folder,
		Policy:        activePolicy,
		Suspended:     suspended,
		SuspendReason: reason,
	}

	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	h, err := loadHistory(ctx, tx, folder)
	_ = tx.Rollback()
	if err != nil {
		return nil, err
	}

	vRows, err := db.db.QueryContext(ctx, `
		SELECT v.author_id, v.counter, v.path, v.acquired_ns, COALESCE(rr.explicit_pin, 0)
		FROM versions v
		LEFT JOIN retention_records rr ON rr.folder_id=v.folder_id AND rr.author_id=v.author_id AND rr.counter=v.counter
		WHERE v.folder_id=? AND v.kind=1
	`, folder[:])
	if err != nil {
		return nil, err
	}
	var versions []versionMeta
	for vRows.Next() {
		var authorRaw, counterRaw []byte
		var path string
		var acqNS int64
		var pin int
		if err := vRows.Scan(&authorRaw, &counterRaw, &path, &acqNS, &pin); err != nil {
			vRows.Close()
			return nil, err
		}
		var author history.ID
		copy(author[:], authorRaw)
		counter, _ := decodeUint(counterRaw)
		versions = append(versions, versionMeta{
			id:          history.VersionID{Folder: folder, Author: author, Counter: counter},
			path:        path,
			acquiredNS:  acqNS,
			explicitPin: pin == 1,
		})
	}
	vRows.Close()

	for i := range versions {
		cRows, err := db.db.QueryContext(ctx, `SELECT digest FROM manifest_chunks WHERE folder_id=? AND author_id=? AND counter=? ORDER BY position`, folder[:], versions[i].id.Author[:], encodeUint(versions[i].id.Counter))
		if err != nil {
			return nil, err
		}
		for cRows.Next() {
			var dRaw []byte
			if err := cRows.Scan(&dRaw); err != nil {
				cRows.Close()
				return nil, err
			}
			var d history.Digest
			copy(d[:], dRaw)
			versions[i].chunks = append(versions[i].chunks, d)
		}
		cRows.Close()
	}

	preview.TotalVersions = len(versions)

	pathMap := make(map[string]bool)
	for _, v := range versions {
		pathMap[v.path] = true
	}
	headVersionSet := make(map[history.VersionID]bool)
	for path := range pathMap {
		for _, head := range h.Heads(folder, path) {
			headVersionSet[head.ID] = true
		}
	}
	preview.HeadVersions = len(headVersionSet)

	protectedChunks, err := db.ComputeProtectedChunks(ctx, folder, &activePolicy, now)
	if err != nil {
		return nil, err
	}
	preview.ProtectedChunks = len(protectedChunks)

	// Check each version if retained or expired
	retentionDuration := time.Duration(activePolicy.RetentionDays) * 24 * time.Hour
	supersededByPath := make(map[string][]versionMeta)
	for _, v := range versions {
		if !headVersionSet[v.id] {
			supersededByPath[v.path] = append(supersededByPath[v.path], v)
		}
	}

	retainedSet := make(map[history.VersionID]bool)
	for _, sList := range supersededByPath {
		sort.Slice(sList, func(i, j int) bool {
			if sList[i].acquiredNS != sList[j].acquiredNS {
				return sList[i].acquiredNS > sList[j].acquiredNS
			}
			return sList[i].id.Counter > sList[j].id.Counter
		})

		for idx, sv := range sList {
			if suspended || sv.explicitPin {
				retainedSet[sv.id] = true
				continue
			}
			acquiredTime := time.Unix(0, sv.acquiredNS)
			if now.Before(acquiredTime) || now.Sub(acquiredTime) < retentionDuration || idx < activePolicy.MinSuperseded {
				retainedSet[sv.id] = true
				continue
			}
		}
	}

	preview.RetainedVersions = preview.HeadVersions + len(retainedSet)
	preview.ExpiredVersions = preview.TotalVersions - preview.RetainedVersions

	// Reclaimable candidate chunks: installed objects not in protectedChunks
	candRows, err := db.db.QueryContext(ctx, `SELECT digest, length FROM objects`)
	if err != nil {
		return nil, err
	}
	defer candRows.Close()
	for candRows.Next() {
		var raw, lenRaw []byte
		if err := candRows.Scan(&raw, &lenRaw); err != nil {
			return nil, err
		}
		var d history.Digest
		copy(d[:], raw)
		length, _ := decodeUint(lenRaw)
		if !protectedChunks[d] {
			preview.CandidateChunks++
			preview.ReclaimableBytes += length
		}
	}

	return preview, nil
}

type GCReport struct {
	FolderID         history.ID `json:"folder_id"`
	Candidates       int        `json:"candidates"`
	UnlinkedObjects  int        `json:"unlinked_objects"`
	ReclaimedBytes   uint64     `json:"reclaimed_bytes"`
	RemainingObjects int        `json:"remaining_objects"`
	RemainingBytes   uint64     `json:"remaining_bytes"`
	Suspended        bool       `json:"suspended"`
	SuspendReason    string     `json:"suspend_reason,omitempty"`
}

func (db *DB) GCPreview(ctx context.Context, folder history.ID, policy *RetentionPolicy, now time.Time) (*GCReport, error) {
	activePolicy := DefaultRetentionPolicy()
	if policy != nil {
		activePolicy = *policy
	} else if folder != (history.ID{}) {
		activePolicy, _ = db.GetRetentionPolicy(ctx, folder)
	}

	suspended, reason, err := db.IsCleanupSuspended(ctx, folder)
	if err != nil {
		return nil, err
	}

	protected, err := db.ComputeProtectedChunks(ctx, folder, &activePolicy, now)
	if err != nil {
		return nil, err
	}

	report := &GCReport{
		FolderID:      folder,
		Suspended:     suspended,
		SuspendReason: reason,
	}

	rows, err := db.db.QueryContext(ctx, `SELECT digest, length FROM objects WHERE verified=1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var dRaw, lenRaw []byte
		if err := rows.Scan(&dRaw, &lenRaw); err != nil {
			return nil, err
		}
		var d history.Digest
		copy(d[:], dRaw)
		length, _ := decodeUint(lenRaw)
		if !protected[d] && !suspended {
			report.Candidates++
			report.ReclaimedBytes += length
		} else {
			report.RemainingObjects++
			report.RemainingBytes += length
		}
	}
	return report, rows.Err()
}

// RunGC executes the crash-safe GC state machine:
// Compute candidates under generation -> acquire durable deletion intents in gc_intents ->
// unlink unreferenced objects -> flush directories -> finalize metadata and mark versions unavailable.
func (db *DB) RunGC(ctx context.Context, folder history.ID, policy *RetentionPolicy, now time.Time) (*GCReport, error) {
	activePolicy := DefaultRetentionPolicy()
	if policy != nil {
		activePolicy = *policy
	} else if folder != (history.ID{}) {
		activePolicy, _ = db.GetRetentionPolicy(ctx, folder)
	}

	suspended, reason, err := db.IsCleanupSuspended(ctx, folder)
	if err != nil {
		return nil, err
	}
	if suspended {
		return &GCReport{
			FolderID:      folder,
			Suspended:     true,
			SuspendReason: reason,
		}, nil
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	if _, err := db.pruneExpiredReadLeases(ctx, now); err != nil {
		return nil, err
	}
	// Session expiry releases only editor pins. Active exact-read response pins
	// retain their independent lifetime through this cleanup pass.
	if _, err := db.db.ExecContext(ctx, `DELETE FROM content_pins WHERE owner_kind='editor' AND owner_key IN (SELECT substr(key,length('terminal/v1/session/')+1) FROM installation_metadata WHERE key LIKE 'terminal/v1/session/%' AND ((json_extract(value,'$.session.state')!='active' AND COALESCE(json_extract(value,'$.building'),0)!=1) OR julianday(json_extract(value,'$.session.expires_at'))<=julianday(?)))`, now.UTC().Format(time.RFC3339Nano)); err != nil {
		return nil, err
	}

	protected, err := db.ComputeProtectedChunks(ctx, folder, &activePolicy, now)
	if err != nil {
		return nil, err
	}

	// 1. Identify candidate objects from objects table
	rows, err := db.db.QueryContext(ctx, `SELECT digest, length FROM objects WHERE verified=1`)
	if err != nil {
		return nil, err
	}
	type candidateObj struct {
		digest history.Digest
		length uint64
	}
	var candidates []candidateObj
	for rows.Next() {
		var dRaw, lenRaw []byte
		if err := rows.Scan(&dRaw, &lenRaw); err != nil {
			rows.Close()
			return nil, err
		}
		var d history.Digest
		copy(d[:], dRaw)
		length, _ := decodeUint(lenRaw)
		if !protected[d] {
			candidates = append(candidates, candidateObj{digest: d, length: length})
		}
	}
	rows.Close()

	report := &GCReport{FolderID: folder, Candidates: len(candidates)}
	if len(candidates) == 0 {
		remRows, err := db.db.QueryContext(ctx, `SELECT length FROM objects WHERE verified=1`)
		if err == nil {
			defer remRows.Close()
			for remRows.Next() {
				var lenRaw []byte
				if err := remRows.Scan(&lenRaw); err == nil {
					l, _ := decodeUint(lenRaw)
					report.RemainingBytes += l
					report.RemainingObjects++
				}
			}
		}
		return report, nil
	}

	// Generation token for this GC run
	generation := int(time.Now().UnixNano())

	// 2. Step 1 (Intent): Acquire durable deletion intents in SQLite
	for _, c := range candidates {
		// Ensure no active pin exists
		var pinCount int
		_ = db.db.QueryRowContext(ctx, `SELECT count(*) FROM content_pins WHERE digest=?`, c.digest[:]).Scan(&pinCount)
		if pinCount > 0 {
			continue
		}

		_, err := db.db.ExecContext(ctx, `INSERT INTO gc_intents(digest, generation, state) VALUES(?,?, 'intent') ON CONFLICT(digest) DO UPDATE SET generation=excluded.generation, state='intent'`, c.digest[:], generation)
		if err != nil {
			return nil, fmt.Errorf("acquire gc intent: %w", err)
		}
	}

	if err := db.callHook(HookGCIntent); err != nil {
		return nil, err
	}

	// 3. Step 2 (Unlink): Unlink unreferenced objects from disk and flush directories
	var unlinked []candidateObj
	for _, c := range candidates {
		// Re-check pins or intent cancellation
		var intentCount int
		err := db.db.QueryRowContext(ctx, `SELECT count(*) FROM gc_intents WHERE digest=?`, c.digest[:]).Scan(&intentCount)
		if err != nil || intentCount == 0 {
			continue // intent was cancelled by new reference or pin
		}
		var pinCount int
		_ = db.db.QueryRowContext(ctx, `SELECT count(*) FROM content_pins WHERE digest=?`, c.digest[:]).Scan(&pinCount)
		if pinCount > 0 {
			_, _ = db.db.ExecContext(ctx, `DELETE FROM gc_intents WHERE digest=?`, c.digest[:])
			continue
		}

		// Update state to 'unlinked'
		_, _ = db.db.ExecContext(ctx, `UPDATE gc_intents SET state='unlinked' WHERE digest=?`, c.digest[:])

		objPath := db.objectPath(c.digest)
		if err := os.Remove(objPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("unlink gc object: %w", err)
		}

		_ = syncDir(filepath.Dir(objPath))

		if err := db.callHook(HookGCUnlink); err != nil {
			return nil, err
		}

		unlinked = append(unlinked, c)
		report.UnlinkedObjects++
		report.ReclaimedBytes += c.length
	}

	// 4. Step 3 (Finalize): Finalize metadata in a single SQLite transaction
	if err := db.callHook(HookGCFinalization); err != nil {
		return nil, err
	}

	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	pastTime := time.Now().Add(-time.Hour)
	if now.Before(pastTime) {
		pastTime = now.Add(-time.Hour)
	}
	pastNS := pastTime.UnixNano()
	for _, u := range unlinked {
		// Delete references and intents
		if _, err := tx.ExecContext(ctx, `DELETE FROM object_references WHERE digest=?`, u.digest[:]); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM gc_intents WHERE digest=?`, u.digest[:]); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM objects WHERE digest=?`, u.digest[:]); err != nil {
			return nil, err
		}
	}

	// Update affected file versions whose chunks are no longer completely present in objects table
	// Mark content_state='unavailable' and record expired retention record
	updateQuery := `
		UPDATE versions
		SET content_state='unavailable'
		WHERE kind=1 AND content_state='ready' AND EXISTS (
			SELECT 1 FROM manifest_chunks mc
			WHERE mc.folder_id=versions.folder_id
			  AND mc.author_id=versions.author_id
			  AND mc.counter=versions.counter
			  AND NOT EXISTS (SELECT 1 FROM objects o WHERE o.digest=mc.digest)
		)
	`
	if _, err := tx.ExecContext(ctx, updateQuery); err != nil {
		return nil, fmt.Errorf("update unavailable versions: %w", err)
	}

	// Insert expired retention records for unavailable versions
	expiredRecordQuery := `
		INSERT INTO retention_records(folder_id, author_id, counter, retain_until_ns, explicit_pin)
		SELECT folder_id, author_id, counter, ?, 0
		FROM versions
		WHERE kind=1 AND content_state='unavailable'
		ON CONFLICT(folder_id, author_id, counter) DO UPDATE SET
			retain_until_ns=CASE WHEN retention_records.explicit_pin=1 THEN retention_records.retain_until_ns ELSE excluded.retain_until_ns END
	`
	if _, err := tx.ExecContext(ctx, expiredRecordQuery, pastNS); err != nil {
		return nil, fmt.Errorf("insert expired retention records: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("finalize gc metadata: %w", err)
	}

	// Compute remaining stats
	remRows, err := db.db.QueryContext(ctx, `SELECT length FROM objects WHERE verified=1`)
	if err == nil {
		defer remRows.Close()
		for remRows.Next() {
			var lenRaw []byte
			if err := remRows.Scan(&lenRaw); err == nil {
				l, _ := decodeUint(lenRaw)
				report.RemainingBytes += l
				report.RemainingObjects++
			}
		}
	}

	return report, nil
}

type FilesystemUsage struct {
	Path           string `json:"path"`
	TotalBytes     uint64 `json:"total_bytes"`
	FreeBytes      uint64 `json:"free_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
}

type FolderStorageUsage struct {
	FolderID         history.ID      `json:"folder_id"`
	RootPath         string          `json:"root_path"`
	WorkingRootBytes uint64          `json:"working_root_bytes"`
	StageBytes       uint64          `json:"stage_bytes"`
	RecoveryBytes    uint64          `json:"recovery_bytes"`
	Filesystem       FilesystemUsage `json:"filesystem"`
}

type DetailedStorageUsage struct {
	Usage
	MetadataBytes         uint64               `json:"metadata_bytes"`
	ObjectBytes           uint64               `json:"object_bytes"`
	StagingBytes          uint64               `json:"staging_bytes"`
	RecoveryBytes         uint64               `json:"recovery_bytes"`
	QuarantineBytes       uint64               `json:"quarantine_bytes"`
	TotalWorkingRootBytes uint64               `json:"total_working_root_bytes"`
	TotalManagedBytes     uint64               `json:"total_managed_bytes"`
	MetadataBudgetBytes   uint64               `json:"metadata_budget_bytes"`
	DataBudgetBytes       uint64               `json:"data_budget_bytes"`
	FreeSpaceReserveBytes uint64               `json:"free_space_reserve_bytes"`
	StateFilesystem       FilesystemUsage      `json:"state_filesystem"`
	Folders               []FolderStorageUsage `json:"folders"`
	Warnings              []string             `json:"warnings,omitempty"`
}

func (db *DB) DetailedStorageUsage(ctx context.Context) (DetailedStorageUsage, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	usage, err := db.usageUnlocked()
	if err != nil {
		return DetailedStorageUsage{}, err
	}
	usage.Reserved, err = db.reservedBytes(ctx)
	if err != nil {
		return DetailedStorageUsage{}, err
	}

	freeReserve := db.freeSpaceReserveBytes
	if freeReserve == 0 {
		freeReserve = 512 * 1024 * 1024
	}
	result := DetailedStorageUsage{
		Usage:                 usage,
		MetadataBudgetBytes:   db.metadataBudgetBytes,
		DataBudgetBytes:       db.budgetBytes,
		FreeSpaceReserveBytes: freeReserve,
		StateFilesystem:       getFilesystemUsage(db.stateDir),
	}

	// Folders usage
	fRows, err := db.db.QueryContext(ctx, `SELECT folder_id, root_path FROM folders WHERE root_path IS NOT NULL`)
	if err != nil {
		return result, err
	}
	defer fRows.Close()

	var totalStage uint64
	var totalRecovery uint64
	var totalWorkingRoot uint64

	for fRows.Next() {
		var fRaw []byte
		var root string
		if err := fRows.Scan(&fRaw, &root); err != nil {
			return result, err
		}
		var f history.ID
		copy(f[:], fRaw)

		fu := FolderStorageUsage{
			FolderID:   f,
			RootPath:   root,
			Filesystem: getFilesystemUsage(root),
		}

		// Calculate working root files bytes (excluding .orbit-internal)
		_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() && filepath.Base(p) == ".orbit-internal" {
				return filepath.SkipDir
			}
			if info.Mode().IsRegular() {
				fu.WorkingRootBytes += uint64(info.Size())
			}
			return nil
		})

		// Calculate scratch stage & recovery bytes beneath root/.orbit-internal
		internalDir := filepath.Join(root, ".orbit-internal")
		_ = filepath.Walk(internalDir, func(p string, info os.FileInfo, err error) error {
			if err != nil || !info.Mode().IsRegular() {
				return nil
			}
			rel, _ := filepath.Rel(internalDir, p)
			base := filepath.Base(p)
			if strings.HasPrefix(rel, "stage") || strings.HasPrefix(base, "stage-") {
				fu.StageBytes += uint64(info.Size())
			} else if strings.HasPrefix(rel, "recovery") || strings.HasPrefix(base, "recovery-") {
				fu.RecoveryBytes += uint64(info.Size())
			}
			return nil
		})

		totalWorkingRoot += fu.WorkingRootBytes
		totalStage += fu.StageBytes
		totalRecovery += fu.RecoveryBytes
		result.Folders = append(result.Folders, fu)
	}

	result.TotalWorkingRootBytes = totalWorkingRoot
	result.MetadataBytes = usage.Metadata
	result.ObjectBytes = usage.Objects
	result.StagingBytes = usage.Incoming + totalStage
	result.RecoveryBytes = totalRecovery
	result.QuarantineBytes = usage.Quarantine
	result.TotalManagedBytes = usage.Total() + totalStage + totalRecovery

	// Capacity checks covering state and root filesystems where separate
	if result.StateFilesystem.AvailableBytes < result.FreeSpaceReserveBytes {
		result.Warnings = append(result.Warnings, fmt.Sprintf("state filesystem free space (%d MiB) is below required %d MiB reserve", result.StateFilesystem.AvailableBytes/(1024*1024), result.FreeSpaceReserveBytes/(1024*1024)))
	}
	for _, folder := range result.Folders {
		if folder.Filesystem.AvailableBytes < result.FreeSpaceReserveBytes {
			result.Warnings = append(result.Warnings, fmt.Sprintf("sync folder %s free space (%d MiB) is below required %d MiB reserve", folder.RootPath, folder.Filesystem.AvailableBytes/(1024*1024), result.FreeSpaceReserveBytes/(1024*1024)))
		}
	}
	if result.MetadataBytes > result.MetadataBudgetBytes {
		result.Warnings = append(result.Warnings, fmt.Sprintf("metadata storage (%d MiB) exceeds soft budget (%d MiB)", result.MetadataBytes/(1024*1024), result.MetadataBudgetBytes/(1024*1024)))
	}

	return result, nil
}

func getFilesystemUsage(path string) FilesystemUsage {
	var stat unix.Statfs_t
	var total, free, avail uint64
	if err := unix.Statfs(path, &stat); err == nil {
		total = stat.Blocks * uint64(stat.Bsize)
		free = stat.Bfree * uint64(stat.Bsize)
		avail = stat.Bavail * uint64(stat.Bsize)
	}
	return FilesystemUsage{
		Path:           path,
		TotalBytes:     total,
		FreeBytes:      free,
		AvailableBytes: avail,
	}
}

type LifecyclePruneReport struct {
	TasksPruned              int64 `json:"tasks_pruned"`
	InvitationsPruned        int64 `json:"invitations_pruned"`
	EnrollmentRequestsPruned int64 `json:"enrollment_requests_pruned"`
	ReadLeasesPruned         int64 `json:"read_leases_pruned"`
	OperationsPruned         int64 `json:"operations_pruned"`
	ControlOpsPruned         int64 `json:"control_ops_pruned"`
	TotalPruned              int64 `json:"total_pruned"`
}

// PruneLifecycleRecords performs safe, bounded pruning of completed/expired lifecycle records
// older than cutoff, preserving pending work, causal DAG metadata, recovery journals, and content pins (Invariant I28).
func (db *DB) PruneLifecycleRecords(ctx context.Context, cutoff time.Time) (LifecyclePruneReport, error) {
	var report LifecyclePruneReport

	// 1. Prune finished tasks (preserves queued, running, retry, exhausted)
	tCount, err := db.PruneFinishedTasks(ctx, cutoff)
	if err != nil {
		return report, fmt.Errorf("prune finished tasks: %w", err)
	}
	report.TasksPruned = tCount

	// 2. Prune expired invitations
	invCount, err := db.PruneExpiredInvitations(ctx, cutoff)
	if err != nil {
		return report, fmt.Errorf("prune expired invitations: %w", err)
	}
	report.InvitationsPruned = invCount

	// 3. Prune terminal enrollment requests (preserves pending)
	enrCount, err := db.PruneTerminalEnrollmentRequests(ctx, cutoff)
	if err != nil {
		return report, fmt.Errorf("prune terminal enrollment requests: %w", err)
	}
	report.EnrollmentRequestsPruned = enrCount

	// 4. Prune expired read leases
	rlCount, err := db.PruneExpiredReadLeases(ctx, cutoff)
	if err != nil {
		return report, fmt.Errorf("prune expired read leases: %w", err)
	}
	report.ReadLeasesPruned = int64(rlCount)

	// 5. Prune finished operations (preserves in-progress)
	opCount, err := db.PruneFinishedOperations(ctx, cutoff)
	if err != nil {
		return report, fmt.Errorf("prune finished operations: %w", err)
	}
	report.OperationsPruned = opCount

	// 6. Prune control operations (expired idempotency records)
	ctrlCount, err := db.PruneControlOperations(ctx, cutoff)
	if err != nil {
		return report, fmt.Errorf("prune control operations: %w", err)
	}
	report.ControlOpsPruned = ctrlCount

	report.TotalPruned = report.TasksPruned + report.InvitationsPruned + report.EnrollmentRequestsPruned +
		report.ReadLeasesPruned + report.OperationsPruned + report.ControlOpsPruned

	return report, nil
}
