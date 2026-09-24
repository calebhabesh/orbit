package repository

import (
	"context"

	"github.com/calebhabesh/file-sync/internal/history"
)

type ConflictHead struct {
	ID           history.VersionID    `json:"id"`
	Kind         history.Kind         `json:"kind"`
	Manifest     *history.Manifest    `json:"manifest,omitempty"`
	Vector       []history.ClockEntry `json:"vector"`
	Applied      bool                 `json:"applied"`
	ContentState string               `json:"content_state"`
}

type ConflictSet struct {
	Path         string             `json:"path"`
	Heads        []ConflictHead     `json:"heads"`
	HeadToken    history.Digest     `json:"head_token"`
	Applied      *history.VersionID `json:"applied,omitempty"`
	ConflictKind string             `json:"conflict_kind"`
}

// Conflicts inspects all paths in the folder and returns structured conflict sets
// for paths with multiple causal heads, separating them from displayed working bytes.
func (db *DB) Conflicts(ctx context.Context, folder history.ID) ([]ConflictSet, error) {
	h, err := loadHistoryReadOnly(ctx, db, folder)
	if err != nil {
		return nil, err
	}
	rows, err := db.db.QueryContext(ctx, `SELECT DISTINCT path FROM versions WHERE folder_id=? ORDER BY path`, folder[:])
	if err != nil {
		return nil, err
	}
	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			return nil, err
		}
		paths = append(paths, path)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var result []ConflictSet
	for _, path := range paths {
		heads := h.Heads(folder, path)
		if len(heads) <= 1 {
			continue
		}
		var headItems []ConflictHead
		var appliedID *history.VersionID
		tombstones := 0
		files := 0
		firstDigest := history.Digest{}
		equalDigests := true
		for _, head := range heads {
			applied, err := db.WorkingApplied(ctx, head.ID)
			if err != nil {
				return nil, err
			}
			if applied {
				copyID := head.ID
				appliedID = &copyID
			}
			envelope, state, err := db.envelopeAndState(ctx, db.db, head.ID)
			if err != nil {
				return nil, err
			}
			headItems = append(headItems, ConflictHead{
				ID:           head.ID,
				Kind:         head.Kind,
				Manifest:     envelope.Manifest,
				Vector:       envelope.Vector,
				Applied:      applied,
				ContentState: state,
			})
			if head.Kind == history.KindTombstone {
				tombstones++
			} else if head.Kind == history.KindFile {
				files++
				if head.Manifest != nil {
					if firstDigest == (history.Digest{}) {
						firstDigest = head.Manifest.Digest
					} else if head.Manifest.Digest != firstDigest {
						equalDigests = false
					}
				} else {
					equalDigests = false
				}
			}
		}
		conflictKind := "edit-edit"
		if tombstones == len(heads) {
			conflictKind = "delete-delete"
		} else if tombstones > 0 {
			conflictKind = "edit-delete"
		} else if files == len(heads) && equalDigests && len(heads) > 1 && firstDigest != (history.Digest{}) {
			conflictKind = "equal-content"
		}
		headIDs := make([]history.VersionID, len(heads))
		for i, h := range heads {
			headIDs[i] = h.ID
		}
		result = append(result, ConflictSet{
			Path:         path,
			Heads:        headItems,
			HeadToken:    history.HeadToken(headIDs),
			Applied:      appliedID,
			ConflictKind: conflictKind,
		})
	}
	return result, nil
}

// StructuralConflicts reports all ancestor/descendant structural conflicts
// present in the causally accepted versions for this folder.
func (db *DB) StructuralConflicts(ctx context.Context, folder history.ID) ([]history.StructuralConflict, error) {
	h, err := loadHistoryReadOnly(ctx, db, folder)
	if err != nil {
		return nil, err
	}
	return h.StructuralConflicts(folder), nil
}
