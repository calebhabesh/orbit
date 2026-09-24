package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
)

type QuarantinedChunk struct {
	Digest         history.Digest
	QuarantinePath string
	Length         uint64
	Reason         string
	QuarantinedNS  int64
	Repaired       bool
}

type CorruptChunkDetail struct {
	Digest           history.Digest      `json:"digest"`
	Length           uint64              `json:"length"`
	Reason           string              `json:"reason"`
	QuarantinePath   string              `json:"quarantine_path"`
	AffectedVersions []history.VersionID `json:"affected_versions"`
	AffectedPaths    []string            `json:"affected_paths"`
}

type MissingProtectedDetail struct {
	Digest           history.Digest      `json:"digest"`
	Length           uint64              `json:"length"`
	AffectedVersions []history.VersionID `json:"affected_versions"`
	AffectedPaths    []string            `json:"affected_paths"`
}

type ExpiredHistoricalDetail struct {
	Digest           history.Digest      `json:"digest"`
	Length           uint64              `json:"length"`
	AffectedVersions []history.VersionID `json:"affected_versions"`
}

type IntegrityCheckRequest struct {
	Folder         history.ID
	Path           string
	VersionID      *history.VersionID
	AutoQuarantine bool
	Limit          int
}

type IntegrityCheckResult struct {
	Folder             history.ID                `json:"folder,omitempty"`
	TotalChunksChecked int                       `json:"total_chunks_checked"`
	TotalFilesChecked  int                       `json:"total_files_checked"`
	CleanChunks        int                       `json:"clean_chunks"`
	CorruptChunks      []CorruptChunkDetail      `json:"corrupt_chunks"`
	MissingProtected   []MissingProtectedDetail  `json:"missing_protected"`
	ExpiredHistorical  []ExpiredHistoricalDetail `json:"expired_historical"`
	DurationNS         int64                     `json:"duration_ns"`
}

type VersionDiagnosis struct {
	ID            history.VersionID `json:"id"`
	Path          string            `json:"path"`
	Kind          history.Kind      `json:"kind"`
	Status        string            `json:"status"` // ready, pending, expired, missing_protected, corrupt
	Reason        string            `json:"reason"`
	CorruptChunks []history.Digest  `json:"corrupt_chunks,omitempty"`
	MissingChunks []history.Digest  `json:"missing_chunks,omitempty"`
}

// QuarantineChunk moves a corrupt chunk file into the quarantine directory,
// updates SQLite to mark verified=0 and records the quarantine entry,
// and marks all versions referencing this chunk as content_state='unavailable'.
// Returns all affected version IDs.
func (db *DB) QuarantineChunk(ctx context.Context, digest history.Digest, reason string) ([]history.VersionID, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	quarantineDir := filepath.Join(db.stateDir, "quarantine")
	if err := os.MkdirAll(quarantineDir, 0o700); err != nil {
		return nil, fmt.Errorf("create quarantine dir: %w", err)
	}

	srcPath := db.objectPath(digest)
	quarantineName := fmt.Sprintf("%x_%d", digest[:], time.Now().UnixNano())
	dstPath := filepath.Join(quarantineDir, quarantineName)

	var fileSize uint64
	if info, err := os.Lstat(srcPath); err == nil {
		fileSize = uint64(info.Size())
		if renameErr := os.Rename(srcPath, dstPath); renameErr == nil {
			_ = syncDir(filepath.Dir(srcPath))
			_ = syncDir(quarantineDir)
		} else {
			if copyErr := copyFile(srcPath, dstPath); copyErr != nil {
				return nil, fmt.Errorf("quarantine file copy: %w", copyErr)
			}
			_ = os.Remove(srcPath)
			_ = syncDir(filepath.Dir(srcPath))
			_ = syncDir(quarantineDir)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Mark object verified=0
	_, err = tx.ExecContext(ctx, `UPDATE objects SET verified=0 WHERE digest=?`, digest[:])
	if err != nil {
		return nil, err
	}

	// Record in quarantined_chunks
	nowNS := time.Now().UnixNano()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO quarantined_chunks(digest, quarantine_path, length, reason, quarantined_ns, repaired)
		VALUES(?,?,?,?,?,0)
		ON CONFLICT(digest) DO UPDATE SET
			quarantine_path=excluded.quarantine_path,
			reason=excluded.reason,
			quarantined_ns=excluded.quarantined_ns,
			repaired=0
	`, digest[:], dstPath, encodeUint(fileSize), reason, nowNS)
	if err != nil {
		return nil, err
	}

	// Find all affected versions
	rows, err := tx.QueryContext(ctx, `
		SELECT DISTINCT folder_id, author_id, counter
		FROM manifest_chunks
		WHERE digest=?
	`, digest[:])
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var affected []history.VersionID
	for rows.Next() {
		var fRaw, aRaw, cRaw []byte
		if err := rows.Scan(&fRaw, &aRaw, &cRaw); err != nil {
			return nil, err
		}
		var vid history.VersionID
		copy(vid.Folder[:], fRaw)
		copy(vid.Author[:], aRaw)
		vid.Counter, err = decodeUint(cRaw)
		if err != nil {
			return nil, err
		}
		affected = append(affected, vid)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Mark all affected versions as content_state='unavailable'
	for _, vid := range affected {
		_, err = tx.ExecContext(ctx, `
			UPDATE versions
			SET content_state='unavailable'
			WHERE folder_id=? AND author_id=? AND counter=? AND content_state='ready'
		`, vid.Folder[:], vid.Author[:], encodeUint(vid.Counter))
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if err := db.callHook(HookChunkQuarantined); err != nil {
		return affected, err
	}

	return affected, nil
}

// UnquarantineChunk marks a chunk repaired and verified, and restores
// content_state='ready' for any affected versions whose manifest chunks are all verified.
func (db *DB) UnquarantineChunk(ctx context.Context, digest history.Digest) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `UPDATE quarantined_chunks SET repaired=1 WHERE digest=?`, digest[:])
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `UPDATE objects SET verified=1 WHERE digest=?`, digest[:])
	if err != nil {
		return err
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT DISTINCT folder_id, author_id, counter
		FROM manifest_chunks
		WHERE digest=?
	`, digest[:])
	if err != nil {
		return err
	}
	defer rows.Close()

	type verID struct {
		f history.ID
		a history.ID
		c uint64
	}
	var vers []verID
	for rows.Next() {
		var fRaw, aRaw, cRaw []byte
		if err := rows.Scan(&fRaw, &aRaw, &cRaw); err != nil {
			return err
		}
		c, err := decodeUint(cRaw)
		if err != nil {
			return err
		}
		var v verID
		copy(v.f[:], fRaw)
		copy(v.a[:], aRaw)
		v.c = c
		vers = append(vers, v)
	}

	for _, v := range vers {
		var unreadyCount int
		err := tx.QueryRowContext(ctx, `
			SELECT count(*)
			FROM manifest_chunks mc
			WHERE mc.folder_id=? AND mc.author_id=? AND mc.counter=?
			  AND NOT EXISTS (
			      SELECT 1 FROM objects o
			      WHERE o.digest=mc.digest AND o.verified=1
			  )
		`, v.f[:], v.a[:], encodeUint(v.c)).Scan(&unreadyCount)
		if err == nil && unreadyCount == 0 {
			_, _ = tx.ExecContext(ctx, `
				UPDATE versions
				SET content_state='ready'
				WHERE folder_id=? AND author_id=? AND counter=? AND content_state='unavailable'
			`, v.f[:], v.a[:], encodeUint(v.c))
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return db.callHook(HookRepairInstalled)
}

// IsChunkQuarantined reports whether a chunk is currently in quarantine.
func (db *DB) IsChunkQuarantined(ctx context.Context, digest history.Digest) (bool, string, error) {
	var reason string
	err := db.db.QueryRowContext(ctx, `
		SELECT reason FROM quarantined_chunks WHERE digest=? AND repaired=0
	`, digest[:]).Scan(&reason)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, reason, nil
}

// ListQuarantinedChunks lists all quarantined chunks in descending order of quarantine time.
func (db *DB) ListQuarantinedChunks(ctx context.Context) ([]QuarantinedChunk, error) {
	rows, err := db.db.QueryContext(ctx, `
		SELECT digest, quarantine_path, length, reason, quarantined_ns, repaired
		FROM quarantined_chunks
		ORDER BY quarantined_ns DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []QuarantinedChunk
	for rows.Next() {
		var rawDigest, rawLen []byte
		var q QuarantinedChunk
		var repairedInt int
		if err := rows.Scan(&rawDigest, &q.QuarantinePath, &rawLen, &q.Reason, &q.QuarantinedNS, &repairedInt); err != nil {
			return nil, err
		}
		copy(q.Digest[:], rawDigest)
		q.Length, err = decodeUint(rawLen)
		if err != nil {
			return nil, err
		}
		q.Repaired = repairedInt == 1
		list = append(list, q)
	}
	return list, rows.Err()
}

// AffectedVersionsForChunk returns all version IDs that reference the given chunk.
func (db *DB) AffectedVersionsForChunk(ctx context.Context, digest history.Digest) ([]history.VersionID, error) {
	rows, err := db.db.QueryContext(ctx, `
		SELECT DISTINCT folder_id, author_id, counter
		FROM manifest_chunks
		WHERE digest=?
		ORDER BY folder_id, author_id, counter
	`, digest[:])
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []history.VersionID
	for rows.Next() {
		var fRaw, aRaw, cRaw []byte
		if err := rows.Scan(&fRaw, &aRaw, &cRaw); err != nil {
			return nil, err
		}
		var vid history.VersionID
		copy(vid.Folder[:], fRaw)
		copy(vid.Author[:], aRaw)
		var decodeErr error
		vid.Counter, decodeErr = decodeUint(cRaw)
		if decodeErr != nil {
			return nil, decodeErr
		}
		res = append(res, vid)
	}
	return res, rows.Err()
}

// AffectedPathsForChunk returns distinct paths across versions that reference the chunk.
func (db *DB) AffectedPathsForChunk(ctx context.Context, digest history.Digest) ([]string, error) {
	rows, err := db.db.QueryContext(ctx, `
		SELECT DISTINCT v.path
		FROM manifest_chunks mc
		JOIN versions v ON v.folder_id=mc.folder_id AND v.author_id=mc.author_id AND v.counter=mc.counter
		WHERE mc.digest=?
		ORDER BY v.path
	`, digest[:])
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

// CheckIntegrity performs a bounded, cancelable scan across managed chunks.
// Chunks are pinned during checking so concurrent GC cannot race or unlink them.
func (db *DB) CheckIntegrity(ctx context.Context, req IntegrityCheckRequest) (*IntegrityCheckResult, error) {
	startTime := time.Now()
	result := &IntegrityCheckResult{
		Folder: req.Folder,
	}

	// 1. Determine target chunk set to check
	type chunkTarget struct {
		digest history.Digest
		length uint64
	}
	var targets []chunkTarget

	if req.VersionID != nil {
		envelope, _, err := db.envelopeAndState(ctx, db.db, *req.VersionID)
		if err != nil {
			return nil, err
		}
		if envelope.Manifest != nil {
			result.TotalFilesChecked = 1
			for _, ch := range envelope.Manifest.Chunks {
				targets = append(targets, chunkTarget{digest: ch.Digest, length: ch.Length})
			}
		}
	} else if req.Folder != (history.ID{}) && req.Path != "" {
		rows, err := db.db.QueryContext(ctx, `
			SELECT DISTINCT mc.digest, mc.length
			FROM manifest_chunks mc
			JOIN versions v ON v.folder_id=mc.folder_id AND v.author_id=mc.author_id AND v.counter=mc.counter
			WHERE mc.folder_id=? AND v.path=?
		`, req.Folder[:], req.Path)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var dRaw, lRaw []byte
			if err := rows.Scan(&dRaw, &lRaw); err != nil {
				return nil, err
			}
			var ct chunkTarget
			copy(ct.digest[:], dRaw)
			var err error
			ct.length, err = decodeUint(lRaw)
			if err != nil {
				return nil, err
			}
			targets = append(targets, ct)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	} else if req.Folder != (history.ID{}) {
		rows, err := db.db.QueryContext(ctx, `
			SELECT DISTINCT mc.digest, mc.length
			FROM manifest_chunks mc
			WHERE mc.folder_id=?
		`, req.Folder[:])
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var dRaw, lRaw []byte
			if err := rows.Scan(&dRaw, &lRaw); err != nil {
				return nil, err
			}
			var ct chunkTarget
			copy(ct.digest[:], dRaw)
			var err error
			ct.length, err = decodeUint(lRaw)
			if err != nil {
				return nil, err
			}
			targets = append(targets, ct)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	} else {
		// All objects
		rows, err := db.db.QueryContext(ctx, `SELECT digest, length FROM objects WHERE verified=1`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var dRaw, lRaw []byte
			if err := rows.Scan(&dRaw, &lRaw); err != nil {
				return nil, err
			}
			var ct chunkTarget
			copy(ct.digest[:], dRaw)
			var err error
			ct.length, err = decodeUint(lRaw)
			if err != nil {
				return nil, err
			}
			targets = append(targets, ct)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	// Deduplicate targets by digest
	uniqueTargets := make([]chunkTarget, 0, len(targets))
	seen := make(map[history.Digest]bool, len(targets))
	for _, t := range targets {
		if !seen[t.digest] {
			seen[t.digest] = true
			uniqueTargets = append(uniqueTargets, t)
		}
	}

	if req.Limit > 0 && len(uniqueTargets) > req.Limit {
		uniqueTargets = uniqueTargets[:req.Limit]
	}

	scanID := fmt.Sprintf("scan-%d", startTime.UnixNano())
	now := time.Now()

	// Precompute protected chunks for classification of missing objects
	var policy *RetentionPolicy
	if req.Folder != (history.ID{}) {
		p, err := db.GetRetentionPolicy(ctx, req.Folder)
		if err == nil {
			policy = &p
		}
	}
	protectedMap, _ := db.ComputeProtectedChunks(ctx, req.Folder, policy, now)

	// 2. Iterate and verify each chunk with cancellation support
	for _, target := range uniqueTargets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result.TotalChunksChecked++

		// Pin chunk during checking so GC cannot race
		_ = db.Pin(ctx, target.digest, "integrity_check", scanID)

		filePath := db.objectPath(target.digest)
		file, err := os.Open(filePath)
		if errors.Is(err, os.ErrNotExist) {
			_ = db.Unpin(ctx, target.digest, "integrity_check", scanID)

			affected, _ := db.AffectedVersionsForChunk(ctx, target.digest)
			paths, _ := db.AffectedPathsForChunk(ctx, target.digest)

			// Check if quarantined
			if isQ, qReason, _ := db.IsChunkQuarantined(ctx, target.digest); isQ {
				result.CorruptChunks = append(result.CorruptChunks, CorruptChunkDetail{
					Digest:           target.digest,
					Length:           target.length,
					Reason:           qReason,
					AffectedVersions: affected,
					AffectedPaths:    paths,
				})
				continue
			}

			// Check if protected or expired
			if protectedMap != nil && protectedMap[target.digest] {
				result.MissingProtected = append(result.MissingProtected, MissingProtectedDetail{
					Digest:           target.digest,
					Length:           target.length,
					AffectedVersions: affected,
					AffectedPaths:    paths,
				})
			} else {
				result.ExpiredHistorical = append(result.ExpiredHistorical, ExpiredHistoricalDetail{
					Digest:           target.digest,
					Length:           target.length,
					AffectedVersions: affected,
				})
			}
			continue
		} else if err != nil {
			_ = db.Unpin(ctx, target.digest, "integrity_check", scanID)
			return nil, err
		}

		// Stream and hash file
		hasher := sha256.New()
		n, copyErr := io.Copy(hasher, file)
		_ = file.Close()

		if copyErr != nil || uint64(n) != target.length || !equalHash(hasher, target.digest) {
			reason := "checksum mismatch"
			if uint64(n) != target.length {
				reason = fmt.Sprintf("length mismatch: expected %d got %d", target.length, n)
			}

			affected, _ := db.AffectedVersionsForChunk(ctx, target.digest)
			paths, _ := db.AffectedPathsForChunk(ctx, target.digest)

			quarantinePath := ""
			if req.AutoQuarantine {
				_, _ = db.QuarantineChunk(ctx, target.digest, reason)
				quarantinePath = filepath.Join(db.stateDir, "quarantine")
			}

			result.CorruptChunks = append(result.CorruptChunks, CorruptChunkDetail{
				Digest:           target.digest,
				Length:           target.length,
				Reason:           reason,
				QuarantinePath:   quarantinePath,
				AffectedVersions: affected,
				AffectedPaths:    paths,
			})
		} else {
			result.CleanChunks++
		}

		_ = db.Unpin(ctx, target.digest, "integrity_check", scanID)
	}

	result.DurationNS = time.Since(startTime).Nanoseconds()
	_ = db.callHook(HookIntegrityScanned)
	return result, nil
}

// DiagnoseVersionAvailability inspects a version and returns a comprehensive diagnosis distinguishing:
// ready, pending, expired, missing_protected, and corrupt.
func (db *DB) DiagnoseVersionAvailability(ctx context.Context, id history.VersionID) (VersionDiagnosis, error) {
	envelope, state, err := db.envelopeAndState(ctx, db.db, id)
	if err != nil {
		return VersionDiagnosis{}, err
	}

	diag := VersionDiagnosis{
		ID:   id,
		Path: envelope.Path,
		Kind: envelope.Kind,
	}

	if envelope.Kind != history.KindFile {
		diag.Status = "ready"
		diag.Reason = "non-file versions require no content payloads"
		return diag, nil
	}

	if state == "pending" {
		diag.Status = "pending"
		diag.Reason = "remote content is pending transfer"
		return diag, nil
	}

	if envelope.Manifest == nil {
		diag.Status = "ready"
		diag.Reason = "empty file requires no chunks"
		return diag, nil
	}

	now := time.Now()
	var policy *RetentionPolicy
	if p, err := db.GetRetentionPolicy(ctx, id.Folder); err == nil {
		policy = &p
	}
	protectedMap, _ := db.ComputeProtectedChunks(ctx, id.Folder, policy, now)

	hasCorrupt := false
	hasMissing := false
	var corruptChunks []history.Digest
	var missingChunks []history.Digest

	for _, ch := range envelope.Manifest.Chunks {
		// Check quarantined table
		if isQ, _, _ := db.IsChunkQuarantined(ctx, ch.Digest); isQ {
			hasCorrupt = true
			corruptChunks = append(corruptChunks, ch.Digest)
			continue
		}

		file, err := os.Open(db.objectPath(ch.Digest))
		if errors.Is(err, os.ErrNotExist) {
			hasMissing = true
			missingChunks = append(missingChunks, ch.Digest)
			continue
		} else if err != nil {
			return diag, err
		}

		hasher := sha256.New()
		n, copyErr := io.Copy(hasher, file)
		_ = file.Close()

		if copyErr != nil || uint64(n) != ch.Length || !equalHash(hasher, ch.Digest) {
			hasCorrupt = true
			corruptChunks = append(corruptChunks, ch.Digest)
		}
	}

	if hasCorrupt {
		diag.Status = "corrupt"
		diag.Reason = fmt.Sprintf("%d chunk(s) corrupt or quarantined", len(corruptChunks))
		diag.CorruptChunks = corruptChunks
		return diag, nil
	}

	if hasMissing {
		diag.MissingChunks = missingChunks
		// Check if version is protected
		isProtected := false
		for _, ch := range missingChunks {
			if protectedMap != nil && protectedMap[ch] {
				isProtected = true
				break
			}
		}

		var retainNS sql.NullInt64
		_ = db.db.QueryRowContext(ctx, `SELECT retain_until_ns FROM retention_records WHERE folder_id=? AND author_id=? AND counter=?`, id.Folder[:], id.Author[:], encodeUint(id.Counter)).Scan(&retainNS)
		isExpired := retainNS.Valid && retainNS.Int64 > 0 && now.UnixNano() >= retainNS.Int64

		if isProtected || !isExpired {
			diag.Status = "missing_protected"
			diag.Reason = fmt.Sprintf("%d chunk(s) missing for protected version", len(missingChunks))
		} else {
			diag.Status = "expired"
			diag.Reason = "historical payload unlinked under retention policy"
		}
		return diag, nil
	}

	diag.Status = "ready"
	diag.Reason = "all chunks verified and present on disk"
	return diag, nil
}

// RecordPeerIntegrityIncident tracks a corruption incident from a remote peer for diagnostics.
func (db *DB) RecordPeerIntegrityIncident(ctx context.Context, peer, folder history.ID, chunk history.Digest, reason string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.db.ExecContext(ctx, `
		INSERT INTO peer_integrity_incidents(peer_id, folder_id, chunk_digest, incident_ns, error_reason)
		VALUES(?,?,?,?,?)
		ON CONFLICT(peer_id, folder_id, chunk_digest, incident_ns) DO NOTHING
	`, peer[:], folder[:], chunk[:], time.Now().UnixNano(), reason)
	return err
}

// PeerIntegrityIncidents returns the number of corruption incidents recorded for a peer in a folder.
func (db *DB) PeerIntegrityIncidents(ctx context.Context, peer, folder history.ID) (int, error) {
	var count int
	err := db.db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM peer_integrity_incidents
		WHERE peer_id=? AND folder_id=?
	`, peer[:], folder[:]).Scan(&count)
	return count, err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
