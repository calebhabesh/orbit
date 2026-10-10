package repository

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
)

var ErrRetirementChanged = errors.New("retirement review changed; sync surviving devices and review again")

type RetirementProposal struct {
	Initiator  history.ID                  `json:"initiator"`
	DeviceName string                      `json:"device_name"`
	Membership protocol.Membership         `json:"membership"`
	Snapshot   protocol.RetirementSnapshot `json:"snapshot"`
}

// Only one exactly reviewed removal is allowed: no enrollment, rekeying,
// revival, changed older retirement, or removal of the approving device.
func validateRetirement(current protocol.Membership, p RetirementProposal) error {
	next, snap := p.Membership, p.Snapshot
	prior, err := protocol.MembershipDigest(current)
	if err != nil {
		return err
	}
	digest, err := protocol.RetirementSnapshotDigest(snap)
	if err != nil {
		return err
	}
	if next.Folder != current.Folder || next.Revision != current.Revision+1 || next.PriorDigest != prior || snap.Folder != current.Folder || snap.ConfigurationRev != current.Revision || len(next.Active) != len(current.Active)-1 || len(next.Active) == 0 || len(next.Retired) != len(current.Retired)+1 || p.Initiator == snap.RetiredDevice {
		return ErrRetirementChanged
	}
	found, initiator := false, false
	for _, member := range current.Active {
		if member.Device == snap.RetiredDevice {
			found = true
			continue
		}
		if !slices.Contains(next.Active, member) {
			return ErrRetirementChanged
		}
		if member.Device == p.Initiator {
			initiator = true
		}
	}
	if !found || !initiator {
		return ErrRetirementChanged
	}
	for _, member := range current.Retired {
		if !slices.Contains(next.Retired, member) {
			return ErrRetirementChanged
		}
	}
	if !slices.Contains(next.Retired, protocol.RetiredMember{Device: snap.RetiredDevice, RetiredAt: next.Revision, SnapshotDigest: digest}) {
		return ErrRetirementChanged
	}
	_, err = protocol.MembershipDigest(next)
	return err
}

func checkRetirementVersionsTx(ctx context.Context, tx *sql.Tx, p RetirementProposal) error {
	rows, err := tx.QueryContext(ctx, `SELECT counter,envelope_digest FROM versions WHERE folder_id=? AND author_id=? ORDER BY counter`, p.Snapshot.Folder[:], p.Snapshot.RetiredDevice[:])
	if err != nil {
		return err
	}
	defer rows.Close()
	var actual []protocol.RetiredVersion
	for rows.Next() {
		var counter, digest []byte
		if err := rows.Scan(&counter, &digest); err != nil {
			return err
		}
		n, err := decodeUint(counter)
		if err != nil {
			return err
		}
		var d history.Digest
		copy(d[:], digest)
		actual = append(actual, protocol.RetiredVersion{Counter: n, EnvelopeDigest: d})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !slices.Equal(actual, p.Snapshot.AcceptedByRetiree) {
		return ErrRetirementChanged
	}
	return nil
}

// PrepareRetirement records the single owner's explicit intent received from
// a pinned surviving member. It grants no data access and changes no membership.
func (db *DB) PrepareRetirement(ctx context.Context, p RetirementProposal) error {
	current, app, err := db.GetMembership(ctx, p.Membership.Folder)
	if err != nil {
		return err
	}
	digest, err := protocol.MembershipDigest(p.Membership)
	if err != nil {
		return err
	}
	if app.Digest == digest {
		return nil
	}
	if err := validateRetirement(current, p); err != nil {
		return err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var prior []byte
	if err = tx.QueryRowContext(ctx, `SELECT membership_digest FROM folders WHERE folder_id=? AND left_ns IS NULL AND removed_by IS NULL`, current.Folder[:]).Scan(&prior); err != nil {
		return err
	}
	if !bytes.Equal(prior, app.Digest[:]) {
		return ErrRetirementChanged
	}
	if err = checkRetirementVersionsTx(ctx, tx, p); err != nil {
		return err
	}
	var oldRaw []byte
	err = tx.QueryRowContext(ctx, `SELECT proposal FROM retirement_reviews WHERE folder_id=?`, current.Folder[:]).Scan(&oldRaw)
	if err == nil {
		var old RetirementProposal
		if err = protocol.DecodeStrict(oldRaw, &old); err != nil {
			return err
		}
		if old.Membership.PriorDigest == app.Digest && old.Initiator != p.Initiator {
			return ErrMembershipFork
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO retirement_reviews(folder_id,digest,proposal) VALUES(?,?,?) ON CONFLICT(folder_id) DO UPDATE SET digest=excluded.digest,proposal=excluded.proposal`, current.Folder[:], digest[:], raw); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) RetirementReview(ctx context.Context, folder history.ID) (RetirementProposal, error) {
	var raw []byte
	var p RetirementProposal
	err := db.db.QueryRowContext(ctx, `SELECT proposal FROM retirement_reviews WHERE folder_id=?`, folder[:]).Scan(&raw)
	if err != nil {
		return p, err
	}
	return p, protocol.DecodeStrict(raw, &p)
}

// CommitRetirement accepts only a previously reviewed exact successor, and
// rechecks the accepted retiree set in the same transaction as membership.
func (db *DB) CommitRetirement(ctx context.Context, p RetirementProposal) (ApprovedMembership, error) {
	digest, err := protocol.MembershipDigest(p.Membership)
	if err != nil {
		return ApprovedMembership{}, err
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return ApprovedMembership{}, err
	}
	defer tx.Rollback()
	var current, review, reviewRaw []byte
	if err = tx.QueryRowContext(ctx, `SELECT membership_digest FROM folders WHERE folder_id=?`, p.Membership.Folder[:]).Scan(&current); err != nil {
		return ApprovedMembership{}, err
	}
	if bytes.Equal(current, digest[:]) {
		return ApprovedMembership{Revision: p.Membership.Revision, Digest: digest}, nil
	}
	if !bytes.Equal(current, p.Membership.PriorDigest[:]) {
		return ApprovedMembership{}, ErrMembershipFork
	}
	if err = tx.QueryRowContext(ctx, `SELECT digest,proposal FROM retirement_reviews WHERE folder_id=?`, p.Membership.Folder[:]).Scan(&review, &reviewRaw); err != nil || !bytes.Equal(review, digest[:]) {
		return ApprovedMembership{}, ErrRetirementChanged
	}
	var prepared RetirementProposal
	if err = protocol.DecodeStrict(reviewRaw, &prepared); err != nil {
		return ApprovedMembership{}, err
	}
	if prepared.Initiator != p.Initiator || prepared.DeviceName != p.DeviceName {
		return ApprovedMembership{}, ErrRetirementChanged
	}
	if err = checkRetirementVersionsTx(ctx, tx, p); err != nil {
		return ApprovedMembership{}, err
	}
	approved, err := approveMembershipTx(ctx, tx, p.Membership, p.Snapshot)
	if err != nil {
		return approved, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO retirement_actors(folder_id,device_id,initiator) VALUES(?,?,?)`, p.Membership.Folder[:], p.Snapshot.RetiredDevice[:], p.Initiator[:]); err != nil {
		return approved, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE durable_work_tasks SET state='canceled' WHERE folder_id=? AND peer_id=? AND state NOT IN ('completed','canceled')`, p.Membership.Folder[:], p.Snapshot.RetiredDevice[:]); err != nil {
		return approved, err
	}
	return approved, tx.Commit()
}

func (db *DB) RetirementActor(ctx context.Context, folder, device history.ID) (history.ID, error) {
	var raw []byte
	err := db.db.QueryRowContext(ctx, `SELECT initiator FROM retirement_actors WHERE folder_id=? AND device_id=?`, folder[:], device[:]).Scan(&raw)
	var actor history.ID
	copy(actor[:], raw)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return actor, err
}
