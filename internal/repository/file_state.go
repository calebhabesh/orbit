package repository

import (
	"bytes"
	"context"
	"database/sql"
	"errors"

	"github.com/calebhabesh/orbit/internal/history"
)

// Per-file sync states (gate EG2). Each is derived from this replica's own
// records, with the same rules as folder readiness; none says anything about
// other devices, whose reports are shown separately with their age. An edit
// made since the last scan has no record yet, so no label claims it: the
// detail view shows when the path was last checked instead.
const (
	SyncCaptured       = "captured"        // one head, content here, written to this working copy
	SyncWaitingPublish = "waiting_publish" // newest version is here but not yet written to the working copy
	SyncDownloading    = "downloading"     // version known; content still arriving
	SyncContentMissing = "content_missing" // version known; content unavailable or failed verification
	SyncConflict       = "conflict"        // concurrent heads or incompatible path structure
	SyncBlocked        = "blocked"         // unsupported or unreadable in this working copy
	SyncDeleted        = "deleted"         // the head is a deletion
)

type headState struct {
	author, counter []byte
	kind            history.Kind
	content         string
}

// fileSyncState labels one listed entry. Implicit parent directories, which
// have neither a version nor a projection, get no label.
func (db *DB) fileSyncState(ctx context.Context, folder history.ID, it BrowseItem) (string, error) {
	if it.BlockReason != "" {
		return SyncBlocked, nil
	}
	if it.HasConflict || it.StructuralConflict != "" {
		return SyncConflict, nil
	}
	heads, err := db.headStates(ctx, folder, it.Path)
	if err != nil {
		return "", err
	}
	var appliedAuthor, appliedCounter []byte
	err = db.db.QueryRowContext(ctx, `SELECT applied_author,applied_counter FROM path_projections WHERE folder_id=? AND path=?`, folder[:], it.Path).Scan(&appliedAuthor, &appliedCounter)
	projected := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if len(heads) == 0 {
		return "", nil
	}
	head := heads[0]
	if head.kind == history.KindTombstone {
		return SyncDeleted, nil
	}
	if head.kind == history.KindFile {
		quarantined, err := db.headQuarantined(ctx, folder, head)
		if err != nil {
			return "", err
		}
		switch {
		case head.content == "unavailable" || quarantined:
			return SyncContentMissing, nil
		case head.content == "pending":
			return SyncDownloading, nil
		}
	}
	if !projected || !bytes.Equal(appliedAuthor, head.author) || !bytes.Equal(appliedCounter, head.counter) {
		return SyncWaitingPublish, nil
	}
	return SyncCaptured, nil
}

func (db *DB) headStates(ctx context.Context, folder history.ID, path string) ([]headState, error) {
	rows, err := db.db.QueryContext(ctx, `SELECT author_id,counter,kind,content_state FROM versions v WHERE folder_id=? AND path=?
	 AND NOT EXISTS(SELECT 1 FROM version_parents p WHERE p.folder_id=v.folder_id AND p.parent_author=v.author_id AND p.parent_counter=v.counter)
	 ORDER BY counter DESC,author_id`, folder[:], path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var heads []headState
	for rows.Next() {
		var h headState
		var kind int
		if err := rows.Scan(&h.author, &h.counter, &kind, &h.content); err != nil {
			return nil, err
		}
		h.kind = history.Kind(kind)
		heads = append(heads, h)
	}
	return heads, rows.Err()
}

func (db *DB) headQuarantined(ctx context.Context, folder history.ID, h headState) (bool, error) {
	var found int
	err := db.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM manifest_chunks mc JOIN quarantined_chunks qc ON qc.digest=mc.digest AND qc.repaired=0
	 WHERE mc.folder_id=? AND mc.author_id=? AND mc.counter=?)`, folder[:], h.author, h.counter).Scan(&found)
	return found != 0, err
}
