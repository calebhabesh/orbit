package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
)

type EventLogEntry struct {
	ID             int64       `json:"id,omitempty"`
	TimestampNS    int64       `json:"timestamp_ns"`
	OperationID    string      `json:"operation_id"`
	FolderID       *history.ID `json:"folder_id,omitempty"`
	VersionAuthor  *history.ID `json:"version_author,omitempty"`
	VersionCounter *uint64     `json:"version_counter,omitempty"`
	PeerID         *history.ID `json:"peer_id,omitempty"`
	Phase          string      `json:"phase"`
	ErrorCode      string      `json:"error_code,omitempty"`
	DurationNS     int64       `json:"duration_ns,omitempty"`
}

func (db *DB) RecordEvent(ctx context.Context, entry EventLogEntry) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if entry.TimestampNS == 0 {
		entry.TimestampNS = time.Now().UnixNano()
	}
	var folderRaw, authorRaw, counterRaw, peerRaw []byte
	if entry.FolderID != nil {
		folderRaw = entry.FolderID[:]
	}
	if entry.VersionAuthor != nil {
		authorRaw = entry.VersionAuthor[:]
	}
	if entry.VersionCounter != nil {
		counterRaw = encodeUint(*entry.VersionCounter)
	}
	if entry.PeerID != nil {
		peerRaw = entry.PeerID[:]
	}
	_, err := db.db.ExecContext(ctx, `INSERT INTO event_logs(timestamp_ns, operation_id, folder_id, version_author, version_counter, peer_id, phase, error_code, duration_ns) VALUES(?,?,?,?,?,?,?,?,?)`,
		entry.TimestampNS, entry.OperationID, folderRaw, authorRaw, counterRaw, peerRaw, entry.Phase, entry.ErrorCode, entry.DurationNS)
	if err != nil {
		return err
	}
	// Bound the log table: keep at most 10,000 entries
	_, _ = db.db.ExecContext(ctx, `DELETE FROM event_logs WHERE id NOT IN (SELECT id FROM event_logs ORDER BY id DESC LIMIT 10000)`)
	return nil
}

func (db *DB) ListEvents(ctx context.Context, limit int) ([]EventLogEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := db.db.QueryContext(ctx, `SELECT id, timestamp_ns, operation_id, folder_id, version_author, version_counter, peer_id, phase, error_code, duration_ns FROM event_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []EventLogEntry
	for rows.Next() {
		var entry EventLogEntry
		var folderRaw, authorRaw, counterRaw, peerRaw []byte
		var errCode sql.NullString
		if err := rows.Scan(&entry.ID, &entry.TimestampNS, &entry.OperationID, &folderRaw, &authorRaw, &counterRaw, &peerRaw, &entry.Phase, &errCode, &entry.DurationNS); err != nil {
			return nil, err
		}
		if len(folderRaw) == 32 {
			var fid history.ID
			copy(fid[:], folderRaw)
			entry.FolderID = &fid
		}
		if len(authorRaw) == 32 {
			var aid history.ID
			copy(aid[:], authorRaw)
			entry.VersionAuthor = &aid
		}
		if len(counterRaw) == 8 {
			c, _ := decodeUint(counterRaw)
			entry.VersionCounter = &c
		}
		if len(peerRaw) == 32 {
			var pid history.ID
			copy(pid[:], peerRaw)
			entry.PeerID = &pid
		}
		if errCode.Valid {
			entry.ErrorCode = errCode.String
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}
