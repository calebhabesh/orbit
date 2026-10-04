package repository

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"

	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
)

// NamedPage is a live keyset page of folder/device labels. It is navigation,
// never a review token. A rename or concurrent insert is seen on the next refresh.
func (db *DB) NamedPage(ctx context.Context, q tc.Query, local string) ([]tc.NamedItem, string, error) {
	scope := sha256.Sum256([]byte(q.Kind + "\x00" + q.Folder + "\x00" + q.ID + "\x00" + q.Name))
	type cursor struct {
		Scope string
		After string
	}
	c := cursor{Scope: hex.EncodeToString(scope[:])}
	if q.Cursor != "" {
		if len(q.Cursor) > 512 {
			return nil, "", ErrInvalidCursor
		}
		b, err := base64.RawURLEncoding.DecodeString(q.Cursor)
		var old cursor
		if err != nil || json.Unmarshal(b, &old) != nil || old.Scope != c.Scope {
			return nil, "", ErrInvalidCursor
		}
		id, err := hex.DecodeString(old.After)
		if err != nil || len(id) != 32 {
			return nil, "", ErrInvalidCursor
		}
		c.After = old.After
	}
	limit := int(q.Limit)
	if limit <= 0 || limit > int(tc.MaxPage) {
		limit = int(tc.MaxPage)
	}
	var sql string
	var args []any
	if q.Kind == "folders" {
		sql = `SELECT lower(hex(folder_id)),COALESCE(display_name,''),root_path,'' FROM folders
WHERE root_path IS NOT NULL AND lower(hex(folder_id))>? AND (?='' OR lower(hex(folder_id))=?)
AND (?='' OR display_name=? OR root_path=? OR root_path LIKE ? ESCAPE '\') ORDER BY folder_id LIMIT ?`
		namePath := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q.Name)
		args = []any{c.After, q.Folder, q.Folder, q.Name, q.Name, q.Name, "%/" + namePath, limit + 1}
	} else {
		sql = `WITH names(id,name,pin) AS (
SELECT lower(hex(device_id)),CASE WHEN COALESCE(display_name,'')!='' THEN display_name WHEN is_local=1 THEN 'This device' ELSE 'device-'||substr(lower(hex(device_id)),1,8) END,COALESCE(lower(hex(key_pin)),'') FROM devices
UNION ALL SELECT ?,'This device','' WHERE ?!='' AND NOT EXISTS(SELECT 1 FROM devices WHERE lower(hex(device_id))=?)
) SELECT id,name,'',pin FROM names WHERE id>? AND (?='' OR id=?) AND (?='' OR name=?) ORDER BY id LIMIT ?`
		args = []any{local, local, local, c.After, q.ID, q.ID, q.Name, q.Name, limit + 1}
	}
	rows, err := db.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := make([]tc.NamedItem, 0, limit+1)
	for rows.Next() {
		var item tc.NamedItem
		if err := rows.Scan(&item.ID, &item.Name, &item.Root, &item.KeyPin); err != nil {
			return nil, "", err
		}
		if item.Name == "" {
			item.Name = filepath.Base(item.Root)
			if item.Name == "." || item.Name == "/" {
				item.Name = item.ID
			}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		c.After = items[len(items)-1].ID
		b, _ := json.Marshal(c)
		next = base64.RawURLEncoding.EncodeToString(b)
	}
	return items, next, nil
}
