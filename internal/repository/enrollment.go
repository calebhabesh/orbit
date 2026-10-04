package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
)

// EnrollmentTx is a bounded private record transaction. Capability use, nonce
// consumption, request admission and approval share SQLite's commit boundary.
// Callbacks must use only this handle, never another DB operation.
type EnrollmentTx struct {
	ctx context.Context
	tx  *sql.Tx
}

func (db *DB) EnrollmentTransaction(ctx context.Context, run func(*EnrollmentTx) error) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if err := db.checkMetadataBudget(ctx); err != nil {
		return err
	}
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := run(&EnrollmentTx{ctx, tx}); err != nil {
		return err
	}
	return tx.Commit()
}
func (t *EnrollmentTx) Get(key string, out any) error {
	var b []byte
	err := t.tx.QueryRowContext(t.ctx, `SELECT value FROM installation_metadata WHERE key=?`, "enrollment/v2/"+key).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrOperationNotFound
	}
	if err != nil {
		return err
	}
	return protocol.DecodeStrict(b, out)
}
func (t *EnrollmentTx) Put(key string, in any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	if len(b) > 16384 {
		return ErrMetadataBudgetExceeded
	}
	_, err = t.tx.ExecContext(t.ctx, `INSERT INTO installation_metadata(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, "enrollment/v2/"+key, b)
	return err
}
func (t *EnrollmentTx) Delete(key string) error {
	_, err := t.tx.ExecContext(t.ctx, `DELETE FROM installation_metadata WHERE key=?`, "enrollment/v2/"+key)
	return err
}
func (t *EnrollmentTx) Records(prefix string) (map[string]json.RawMessage, error) {
	rows, err := t.tx.QueryContext(t.ctx, `SELECT key,value FROM installation_metadata WHERE key LIKE ?`, "enrollment/v2/"+prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]json.RawMessage{}
	for rows.Next() {
		var k string
		var b []byte
		if err := rows.Scan(&k, &b); err != nil {
			return nil, err
		}
		out[strings.TrimPrefix(k, "enrollment/v2/")] = b
	}
	return out, rows.Err()
}
func (t *EnrollmentTx) Membership(folder history.ID) (protocol.Membership, history.Digest, error) {
	var rev, digest []byte
	if err := t.tx.QueryRowContext(t.ctx, `SELECT membership_revision,membership_digest FROM folders WHERE folder_id=?`, folder[:]).Scan(&rev, &digest); err != nil {
		return protocol.Membership{}, history.Digest{}, err
	}
	n, err := decodeUint(rev)
	if err != nil {
		return protocol.Membership{}, history.Digest{}, err
	}
	var d history.Digest
	copy(d[:], digest)
	var prior []byte
	if err := t.tx.QueryRowContext(t.ctx, `SELECT prior_digest FROM membership_revisions WHERE folder_id=? AND revision=?`, folder[:], rev).Scan(&prior); err != nil {
		return protocol.Membership{}, d, err
	}
	m := protocol.Membership{Folder: folder, Revision: n}
	copy(m.PriorDigest[:], prior)
	rows, err := t.tx.QueryContext(t.ctx, `SELECT device_id,key_pin,state,retired_at,retirement_snapshot FROM membership_entries WHERE folder_id=? AND revision=? ORDER BY device_id`, folder[:], rev)
	if err != nil {
		return m, d, err
	}
	defer rows.Close()
	for rows.Next() {
		var dev, pin, at, snap []byte
		var state string
		if err := rows.Scan(&dev, &pin, &state, &at, &snap); err != nil {
			return m, d, err
		}
		var id history.ID
		var p, s history.Digest
		copy(id[:], dev)
		copy(p[:], pin)
		copy(s[:], snap)
		if state == "active" {
			m.Active = append(m.Active, protocol.ActiveMember{Device: id, KeyPin: p})
		} else {
			v, err := decodeUint(at)
			if err != nil {
				return m, d, err
			}
			m.Retired = append(m.Retired, protocol.RetiredMember{Device: id, RetiredAt: v, SnapshotDigest: s})
		}
	}
	return m, d, rows.Err()
}
func (t *EnrollmentTx) Approve(m protocol.Membership) error {
	_, err := approveMembershipTx(t.ctx, t.tx, m)
	return err
}

// PendingEnrollmentCount expires pending records and counts in SQLite rather
// than allocating retained replay history in a network handler.
func (t *EnrollmentTx) PendingEnrollmentCount(now int64) (int, error) {
	_, err := t.tx.ExecContext(t.ctx, `UPDATE installation_metadata SET value=json_set(value,'$.result.state','expired') WHERE key LIKE 'enrollment/v2/request/%' AND json_extract(value,'$.result.state')='pending_approval' AND json_extract(value,'$.expires')<=?`, now)
	if err != nil {
		return 0, err
	}
	var count int
	err = t.tx.QueryRowContext(t.ctx, `SELECT count(*) FROM installation_metadata WHERE key LIKE 'enrollment/v2/request/%' AND json_extract(value,'$.result.state')='pending_approval'`).Scan(&count)
	return count, err
}
func (t *EnrollmentTx) EnrollmentRequestPage(folder, after string, limit int) (map[string]json.RawMessage, error) {
	if limit < 1 || limit > 201 {
		return nil, ErrMetadataBudgetExceeded
	}
	rows, err := t.tx.QueryContext(t.ctx, `SELECT key,value FROM installation_metadata WHERE key LIKE 'enrollment/v2/request/%' AND key>? AND (?='' OR json_extract(value,'$.wire.folder')=?) ORDER BY key LIMIT ?`, "enrollment/v2/request/"+after, folder, folder, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]json.RawMessage{}
	for rows.Next() {
		var k string
		var b []byte
		if err := rows.Scan(&k, &b); err != nil {
			return nil, err
		}
		out[strings.TrimPrefix(k, "enrollment/v2/")] = b
	}
	return out, rows.Err()
}

// DeviceKeyPin binds additional-folder sharing to an already recorded identity.
func (t *EnrollmentTx) DeviceKeyPin(device history.ID) (history.Digest, error) {
	var raw []byte
	err := t.tx.QueryRowContext(t.ctx, `SELECT key_pin FROM devices WHERE device_id=?`, device[:]).Scan(&raw)
	var pin history.Digest
	if err != nil {
		return pin, err
	}
	if len(raw) != len(pin) {
		return pin, ErrUnauthorized
	}
	copy(pin[:], raw)
	return pin, nil
}
