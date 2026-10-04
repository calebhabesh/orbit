package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
)

// Terminal records use the existing typed BLOB metadata namespace. This is
// additive at schema 13; old binaries must not execute new terminal operations.
type TerminalRecord struct {
	BeforeInstance string      `json:"before_instance"`
	CompletedAt    string      `json:"completed_at"`
	Mutation       tc.Mutation `json:"mutation"`
	Result         tc.Result   `json:"result"`
	CreatedAt      string      `json:"created_at"`
	Owner          string      `json:"owner"`
}

func (db *DB) TerminalRecord(ctx context.Context, key string, out any) error {
	var b []byte
	err := db.db.QueryRowContext(ctx, `SELECT value FROM installation_metadata WHERE key=?`, "terminal/v1/"+key).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrOperationNotFound
	}
	if err != nil {
		return err
	}
	return tc.Decode(b, out)
}
func (db *DB) SaveTerminalRecord(ctx context.Context, key string, in any, newRecord bool) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	if len(b) > tc.MaxMetadata {
		return ErrMetadataBudgetExceeded
	}
	if newRecord {
		if err := db.checkMetadataBudget(ctx); err != nil {
			return err
		}
		_, err = db.db.ExecContext(ctx, `INSERT INTO installation_metadata(key,value) VALUES(?,?)`, "terminal/v1/"+key, b)
		return err
	}
	_, err = db.db.ExecContext(ctx, `UPDATE installation_metadata SET value=? WHERE key=?`, b, "terminal/v1/"+key)
	return err
}
func (db *DB) TerminalOperations(ctx context.Context) ([]TerminalRecord, error) {
	rows, err := db.db.QueryContext(ctx, `SELECT value FROM installation_metadata WHERE key LIKE 'terminal/v1/operation/%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TerminalRecord
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		var r TerminalRecord
		if err := tc.Decode(b, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReleaseContentOwner drops only session pins; active response pins are separate.
func (db *DB) ReleaseContentOwner(ctx context.Context, kind, key string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.db.ExecContext(ctx, `DELETE FROM content_pins WHERE owner_kind=? AND owner_key=?`, kind, key)
	return err
}
func (db *DB) TerminalSessions(ctx context.Context) ([]tc.EditorSession, error) {
	rows, err := db.db.QueryContext(ctx, `SELECT value FROM installation_metadata WHERE key LIKE 'terminal/v1/session/%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []tc.EditorSession{}
	for rows.Next() {
		var b []byte
		var r struct {
			Session tc.EditorSession `json:"session"`
		}
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &r); err != nil {
			return nil, err
		}
		out = append(out, r.Session)
	}
	return out, rows.Err()
}

// PendingSetupPage exposes identities/phases only, never capabilities or intents.
// SQL keysets bound decoded records and the response, including blocked jobs.
func (db *DB) PendingSetupPage(ctx context.Context, q tc.Query) ([]tc.NamedItem, string, error) {
	limit := int(q.Limit)
	if limit == 0 {
		limit = 20
	}
	if q.Cursor != "" && (len(q.Cursor) != 64 || strings.Trim(q.Cursor, "0123456789abcdef") != "") {
		return nil, "", errors.New("INVALID_REQUEST: setup cursor")
	}
	rows, err := db.db.QueryContext(ctx, `SELECT value FROM installation_metadata WHERE key LIKE 'terminal/v1/operation/%'
 AND json_extract(CAST(value AS TEXT),'$.mutation.kind') IN ('setup','adopt','join')
 AND json_extract(CAST(value AS TEXT),'$.result.operation.state') IN ('pending','running','blocked','partial')
 AND key > ? ORDER BY key LIMIT ?`, "terminal/v1/operation/"+q.Cursor, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := []tc.NamedItem{}
	cursor := ""
	for rows.Next() {
		if len(items) == limit {
			cursor = items[len(items)-1].ID
			break
		}
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, "", err
		}
		var saved TerminalRecord
		if err = tc.Decode(b, &saved); err != nil {
			return nil, "", err
		}
		root := ""
		if saved.Result.Join != nil {
			root = saved.Result.Join.Root
		}
		items = append(items, tc.NamedItem{ID: saved.Mutation.OperationID, Name: saved.Result.Operation.Phase, Root: root})
	}
	return items, cursor, rows.Err()
}
