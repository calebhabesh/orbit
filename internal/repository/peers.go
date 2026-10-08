package repository

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
)

var (
	ErrUnauthorized                 = errors.New("peer is not authorized for this folder")
	ErrMembershipMismatch           = errors.New("membership revision or digest mismatch")
	ErrSnapshotExpired              = errors.New("inventory snapshot expired or unknown")
	ErrSnapshotLimit                = errors.New("too many open inventory snapshots")
	ErrRetiredAuthorVersionRejected = errors.New("retired-author version absent from approved snapshot")
	ErrMembershipFork               = errors.New("membership fork detected: concurrent conflicting revisions")
	ErrRetiredMemberRevival         = errors.New("cannot revive retired member")
)

type ApprovedMembership struct {
	Revision uint64
	Digest   history.Digest
}

// ApproveMembership durably installs an exact owner-reviewed membership
// revision. Existing revisions are immutable and replay-idempotent.
func (db *DB) ApproveMembership(ctx context.Context, membership protocol.Membership, snapshots ...protocol.RetirementSnapshot) (ApprovedMembership, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return ApprovedMembership{}, err
	}
	defer tx.Rollback()
	approved, err := approveMembershipTx(ctx, tx, membership, snapshots...)
	if err != nil {
		return approved, err
	}
	return approved, tx.Commit()
}

// approveMembershipTx preserves the membership rules for atomic enrollment approval.
func approveMembershipTx(ctx context.Context, tx *sql.Tx, membership protocol.Membership, snapshots ...protocol.RetirementSnapshot) (ApprovedMembership, error) {
	digest, err := protocol.MembershipDigest(membership)
	if err != nil {
		return ApprovedMembership{}, err
	}
	var currentRevisionRaw, currentDigestRaw []byte
	if err := tx.QueryRowContext(ctx, `SELECT membership_revision,membership_digest FROM folders WHERE folder_id=?`, membership.Folder[:]).Scan(&currentRevisionRaw, &currentDigestRaw); errors.Is(err, sql.ErrNoRows) {
		return ApprovedMembership{}, ErrFolderUnknown
	} else if err != nil {
		return ApprovedMembership{}, err
	}
	currentRevision, err := decodeUint(currentRevisionRaw)
	if err != nil {
		return ApprovedMembership{}, err
	}
	if len(currentDigestRaw) != 0 {
		if membership.Revision == currentRevision && bytes.Equal(currentDigestRaw, digest[:]) {
			// A joining/recovering replica may already have the approved revision
			// while still needing its hash-bound retirement artifacts. Hydration
			// stays in the same transaction and never changes approved membership.
			for _, member := range membership.Retired {
				if _, err := installRetirementSnapshotTx(ctx, tx, membership, member, snapshots); err != nil {
					return ApprovedMembership{}, err
				}
			}
			return ApprovedMembership{Revision: membership.Revision, Digest: digest}, nil
		}
		if membership.Revision == currentRevision && !bytes.Equal(currentDigestRaw, digest[:]) {
			return ApprovedMembership{}, ErrMembershipFork
		}
		if membership.Revision != currentRevision+1 {
			return ApprovedMembership{}, ErrMembershipMismatch
		}
		if !bytes.Equal(currentDigestRaw, membership.PriorDigest[:]) {
			var forkCount int
			_ = tx.QueryRowContext(ctx, `SELECT count(*) FROM membership_revisions WHERE folder_id=? AND prior_digest=?`, membership.Folder[:], membership.PriorDigest[:]).Scan(&forkCount)
			if forkCount > 0 {
				return ApprovedMembership{}, ErrMembershipFork
			}
			return ApprovedMembership{}, ErrMembershipMismatch
		}
	} else if membership.Revision == 0 {
		return ApprovedMembership{}, ErrMembershipMismatch
	}

	for _, member := range membership.Active {
		var retiredCount int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM membership_entries WHERE folder_id=? AND device_id=? AND state='retired'`, membership.Folder[:], member.Device[:]).Scan(&retiredCount); err != nil {
			return ApprovedMembership{}, err
		}
		if retiredCount > 0 {
			return ApprovedMembership{}, ErrRetiredMemberRevival
		}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO membership_revisions(folder_id,revision,prior_digest,digest,approved) VALUES(?,?,?,?,1)`, membership.Folder[:], encodeUint(membership.Revision), membership.PriorDigest[:], digest[:]); err != nil {
		return ApprovedMembership{}, err
	}
	for _, member := range membership.Active {
		if _, err := tx.ExecContext(ctx, `INSERT INTO devices(device_id,key_pin,is_local) VALUES(?,?,0) ON CONFLICT(device_id) DO UPDATE SET key_pin=excluded.key_pin WHERE devices.key_pin IS NULL OR devices.key_pin=excluded.key_pin`, member.Device[:], member.KeyPin[:]); err != nil {
			return ApprovedMembership{}, err
		}
		var storedPin []byte
		if err := tx.QueryRowContext(ctx, `SELECT key_pin FROM devices WHERE device_id=?`, member.Device[:]).Scan(&storedPin); err != nil || !bytes.Equal(storedPin, member.KeyPin[:]) {
			return ApprovedMembership{}, errors.New("device identity is already bound to another key")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO membership_entries(folder_id,revision,device_id,key_pin,state) VALUES(?,?,?,?, 'active')`, membership.Folder[:], encodeUint(membership.Revision), member.Device[:], member.KeyPin[:]); err != nil {
			return ApprovedMembership{}, err
		}
	}
	for _, member := range membership.Retired {
		if _, err := tx.ExecContext(ctx, `INSERT INTO membership_entries(folder_id,revision,device_id,key_pin,state,retired_at,retirement_snapshot) VALUES(?,?,?,?, 'retired',?,?)`, membership.Folder[:], encodeUint(membership.Revision), member.Device[:], make([]byte, 32), encodeUint(member.RetiredAt), member.SnapshotDigest[:]); err != nil {
			return ApprovedMembership{}, err
		}
		storedSnapshot, err := installRetirementSnapshotTx(ctx, tx, membership, member, snapshots)
		if err != nil {
			return ApprovedMembership{}, err
		}
		if !storedSnapshot && currentRevision > 0 {
			// Copy forward snapshot entries from prior revision if available.
			_, _ = tx.ExecContext(ctx, `INSERT INTO retirement_snapshots(folder_id,revision,retired_device,snapshot_digest,canonical_snapshot)
				SELECT folder_id,?,retired_device,snapshot_digest,canonical_snapshot FROM retirement_snapshots
				WHERE folder_id=? AND revision=? AND retired_device=?`,
				encodeUint(membership.Revision), membership.Folder[:], encodeUint(currentRevision), member.Device[:])
			_, _ = tx.ExecContext(ctx, `INSERT INTO retirement_snapshot_entries(folder_id,revision,retired_device,counter,envelope_digest)
				SELECT folder_id,?,retired_device,counter,envelope_digest FROM retirement_snapshot_entries
				WHERE folder_id=? AND revision=? AND retired_device=?`,
				encodeUint(membership.Revision), membership.Folder[:], encodeUint(currentRevision), member.Device[:])
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE folders SET membership_revision=?,membership_digest=? WHERE folder_id=?`, encodeUint(membership.Revision), digest[:], membership.Folder[:]); err != nil {
		return ApprovedMembership{}, err
	}
	return ApprovedMembership{Revision: membership.Revision, Digest: digest}, nil
}

func installRetirementSnapshotTx(ctx context.Context, tx *sql.Tx, membership protocol.Membership, member protocol.RetiredMember, snapshots []protocol.RetirementSnapshot) (bool, error) {
	for _, snapshot := range snapshots {
		if snapshot.Folder != membership.Folder || snapshot.RetiredDevice != member.Device {
			continue
		}
		digest, err := protocol.RetirementSnapshotDigest(snapshot)
		if err != nil {
			return false, err
		}
		if digest != member.SnapshotDigest {
			return false, fmt.Errorf("retirement snapshot digest mismatch for device %x", member.Device)
		}
		raw, err := protocol.EncodeRetirementSnapshot(snapshot)
		if err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO retirement_snapshots(folder_id,revision,retired_device,snapshot_digest,canonical_snapshot) VALUES(?,?,?,?,?) ON CONFLICT(folder_id,revision,retired_device) DO UPDATE SET canonical_snapshot=excluded.canonical_snapshot`,
			membership.Folder[:], encodeUint(membership.Revision), member.Device[:], digest[:], raw); err != nil {
			return false, err
		}
		for _, version := range snapshot.AcceptedByRetiree {
			if _, err := tx.ExecContext(ctx, `INSERT INTO retirement_snapshot_entries(folder_id,revision,retired_device,counter,envelope_digest) VALUES(?,?,?,?,?) ON CONFLICT(folder_id,revision,retired_device,counter) DO UPDATE SET envelope_digest=excluded.envelope_digest`,
				membership.Folder[:], encodeUint(membership.Revision), member.Device[:], encodeUint(version.Counter), version.EnvelopeDigest[:]); err != nil {
				return false, err
			}
		}
		return true, nil
	}
	return false, nil
}

// GetMembership reconstructs the approved membership for a folder.
func (db *DB) GetMembership(ctx context.Context, folder history.ID, targetRevision ...uint64) (protocol.Membership, ApprovedMembership, error) {
	var rev uint64
	var digestBytes []byte
	if len(targetRevision) > 0 && targetRevision[0] > 0 {
		rev = targetRevision[0]
		err := db.db.QueryRowContext(ctx, `SELECT digest FROM membership_revisions WHERE folder_id=? AND revision=?`, folder[:], encodeUint(rev)).Scan(&digestBytes)
		if errors.Is(err, sql.ErrNoRows) {
			return protocol.Membership{}, ApprovedMembership{}, ErrFolderUnknown
		} else if err != nil {
			return protocol.Membership{}, ApprovedMembership{}, err
		}
	} else {
		var revBytes []byte
		err := db.db.QueryRowContext(ctx, `SELECT membership_revision, membership_digest FROM folders WHERE folder_id=?`, folder[:]).Scan(&revBytes, &digestBytes)
		if errors.Is(err, sql.ErrNoRows) {
			return protocol.Membership{}, ApprovedMembership{}, ErrFolderUnknown
		} else if err != nil {
			return protocol.Membership{}, ApprovedMembership{}, err
		}
		var decodeErr error
		rev, decodeErr = decodeUint(revBytes)
		if decodeErr != nil {
			return protocol.Membership{}, ApprovedMembership{}, decodeErr
		}
	}
	var priorDigestBytes []byte
	err := db.db.QueryRowContext(ctx, `SELECT prior_digest FROM membership_revisions WHERE folder_id=? AND revision=?`, folder[:], encodeUint(rev)).Scan(&priorDigestBytes)
	if err != nil {
		return protocol.Membership{}, ApprovedMembership{}, err
	}
	membership := protocol.Membership{
		Folder:   folder,
		Revision: rev,
	}
	copy(membership.PriorDigest[:], priorDigestBytes)

	rows, err := db.db.QueryContext(ctx, `SELECT device_id, key_pin, state, COALESCE(retired_at, ?), COALESCE(retirement_snapshot, ?) FROM membership_entries WHERE folder_id=? AND revision=? ORDER BY device_id`, encodeUint(0), make([]byte, 32), folder[:], encodeUint(rev))
	if err != nil {
		return protocol.Membership{}, ApprovedMembership{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var devRaw, pinRaw, retiredAtRaw, snapshotRaw []byte
		var state string
		if err := rows.Scan(&devRaw, &pinRaw, &state, &retiredAtRaw, &snapshotRaw); err != nil {
			return protocol.Membership{}, ApprovedMembership{}, err
		}
		var dev history.ID
		var pin, snap history.Digest
		copy(dev[:], devRaw)
		copy(pin[:], pinRaw)
		copy(snap[:], snapshotRaw)
		retiredAt, _ := decodeUint(retiredAtRaw)

		if state == "active" {
			membership.Active = append(membership.Active, protocol.ActiveMember{Device: dev, KeyPin: pin})
		} else {
			membership.Retired = append(membership.Retired, protocol.RetiredMember{Device: dev, RetiredAt: retiredAt, SnapshotDigest: snap})
		}
	}
	if err := rows.Err(); err != nil {
		return protocol.Membership{}, ApprovedMembership{}, err
	}
	var d history.Digest
	copy(d[:], digestBytes)
	return membership, ApprovedMembership{Revision: rev, Digest: d}, nil
}

// DetectMembershipFork checks if candidate revision conflicts with local revisions.
func (db *DB) DetectMembershipFork(ctx context.Context, folder history.ID, candidate protocol.Membership) error {
	candidateDigest, err := protocol.MembershipDigest(candidate)
	if err != nil {
		return err
	}
	var currentRevisionRaw, currentDigestRaw []byte
	if err := db.db.QueryRowContext(ctx, `SELECT membership_revision,membership_digest FROM folders WHERE folder_id=?`, folder[:]).Scan(&currentRevisionRaw, &currentDigestRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrFolderUnknown
		}
		return err
	}
	currentRevision, err := decodeUint(currentRevisionRaw)
	if err != nil {
		return err
	}

	if candidate.Revision == currentRevision && !bytes.Equal(currentDigestRaw, candidateDigest[:]) {
		return fmt.Errorf("%w: competing revision %d with digest %x vs local %x", ErrMembershipFork, candidate.Revision, candidateDigest, currentDigestRaw)
	}

	var conflictingDigest []byte
	err = db.db.QueryRowContext(ctx, `SELECT digest FROM membership_revisions WHERE folder_id=? AND prior_digest=? AND revision=? AND digest!=?`, folder[:], candidate.PriorDigest[:], encodeUint(candidate.Revision), candidateDigest[:]).Scan(&conflictingDigest)
	if err == nil && len(conflictingDigest) > 0 {
		return fmt.Errorf("%w: competing revision %d with prior digest %x", ErrMembershipFork, candidate.Revision, candidate.PriorDigest)
	}
	return nil
}

// IsDeviceRetired reports whether a device ID was retired in any prior revision for this folder.
func (db *DB) IsDeviceRetired(ctx context.Context, folder history.ID, device history.ID) (bool, error) {
	var count int
	err := db.db.QueryRowContext(ctx, `SELECT count(*) FROM membership_entries WHERE folder_id=? AND device_id=? AND state='retired'`, folder[:], device[:]).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// IsActiveOrHistoricalMember reports whether a device has ever been an active member in this folder.
func (db *DB) IsActiveOrHistoricalMember(ctx context.Context, folder history.ID, device history.ID) (bool, error) {
	var count int
	err := db.db.QueryRowContext(ctx, `SELECT count(*) FROM membership_entries WHERE folder_id=? AND device_id=? AND state='active'`, folder[:], device[:]).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetMembershipRevisions returns all known membership revisions for a folder sorted by revision.
func (db *DB) GetMembershipRevisions(ctx context.Context, folder history.ID) ([]protocol.Membership, error) {
	rows, err := db.db.QueryContext(ctx, `SELECT revision FROM membership_revisions WHERE folder_id=? ORDER BY revision ASC`, folder[:])
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var revs []uint64
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		r, err := decodeUint(raw)
		if err != nil {
			return nil, err
		}
		revs = append(revs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var results []protocol.Membership
	for _, r := range revs {
		m, _, err := db.GetMembership(ctx, folder, r)
		if err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	return results, nil
}

// DeviceKeyPin retrieves the known key pin for a device.
func (db *DB) DeviceKeyPin(ctx context.Context, device history.ID) (history.Digest, error) {
	var pinRaw []byte
	err := db.db.QueryRowContext(ctx, `SELECT key_pin FROM devices WHERE device_id=?`, device[:]).Scan(&pinRaw)
	if err != nil {
		return history.Digest{}, err
	}
	var pin history.Digest
	copy(pin[:], pinRaw)
	return pin, nil
}

// GetRetirementSnapshot returns an approved retirement snapshot.
func (db *DB) GetRetirementSnapshot(ctx context.Context, folder history.ID, revision uint64, retiredDevice history.ID) (protocol.RetirementSnapshot, error) {
	var canonical []byte
	err := db.db.QueryRowContext(ctx, `SELECT canonical_snapshot FROM retirement_snapshots WHERE folder_id=? AND revision=? AND retired_device=?`, folder[:], encodeUint(revision), retiredDevice[:]).Scan(&canonical)
	if err != nil {
		return protocol.RetirementSnapshot{}, err
	}
	return protocol.DecodeRetirementSnapshot(canonical)
}

// ListRetirementSnapshots returns all retirement snapshots approved in a revision.
func (db *DB) ListRetirementSnapshots(ctx context.Context, folder history.ID, revision uint64) ([]protocol.RetirementSnapshot, error) {
	rows, err := db.db.QueryContext(ctx, `SELECT canonical_snapshot FROM retirement_snapshots WHERE folder_id=? AND revision=?`, folder[:], encodeUint(revision))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []protocol.RetirementSnapshot
	for rows.Next() {
		var canonical []byte
		if err := rows.Scan(&canonical); err != nil {
			return nil, err
		}
		snap, err := protocol.DecodeRetirementSnapshot(canonical)
		if err != nil {
			return nil, err
		}
		list = append(list, snap)
	}
	return list, rows.Err()
}

// PeerMembers returns active and retired members for a folder.
func (db *DB) PeerMembers(ctx context.Context, folder history.ID) ([]protocol.ActiveMember, []protocol.RetiredMember, uint64, history.Digest, error) {
	m, app, err := db.GetMembership(ctx, folder)
	if err != nil {
		return nil, nil, 0, history.Digest{}, err
	}
	return m.Active, m.Retired, app.Revision, app.Digest, nil
}

// SaveResumableMaintenance persists ongoing retirement maintenance state.
func (db *DB) SaveResumableMaintenance(ctx context.Context, id string, folder, targetDevice history.ID, phase string, state []byte, now time.Time) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.db.ExecContext(ctx, `INSERT INTO resumable_maintenance(maintenance_id, folder_id, target_device, phase, state, updated_ns)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(maintenance_id) DO UPDATE SET phase=excluded.phase, state=excluded.state, updated_ns=excluded.updated_ns`,
		id, folder[:], targetDevice[:], phase, state, now.UnixNano())
	return err
}

// GetResumableMaintenance returns active maintenance state for a folder.
func (db *DB) GetResumableMaintenance(ctx context.Context, folder history.ID) (string, history.ID, string, []byte, error) {
	var id, phase string
	var targetRaw, state []byte
	err := db.db.QueryRowContext(ctx, `SELECT maintenance_id, target_device, phase, state FROM resumable_maintenance WHERE folder_id=? ORDER BY updated_ns DESC LIMIT 1`, folder[:]).Scan(&id, &targetRaw, &phase, &state)
	if err != nil {
		return "", history.ID{}, "", nil, err
	}
	var target history.ID
	copy(target[:], targetRaw)
	return id, target, phase, state, nil
}

// DeleteResumableMaintenance removes a completed or aborted maintenance state.
func (db *DB) DeleteResumableMaintenance(ctx context.Context, id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.db.ExecContext(ctx, `DELETE FROM resumable_maintenance WHERE maintenance_id=?`, id)
	return err
}

// AuthorizePeer maps a presented key pin to the claimed active device and
// requires exact local membership agreement for every folder request.
func (db *DB) AuthorizePeer(ctx context.Context, folder, claimedDevice history.ID, pin history.Digest, revision uint64, digest history.Digest) error {
	var currentRevision, currentDigest, storedPin []byte
	err := db.db.QueryRowContext(ctx, `SELECT f.membership_revision,f.membership_digest,e.key_pin FROM folders f JOIN membership_entries e ON e.folder_id=f.folder_id AND e.revision=f.membership_revision WHERE f.folder_id=? AND e.device_id=? AND e.state='active'`, folder[:], claimedDevice[:]).Scan(&currentRevision, &currentDigest, &storedPin)
	if errors.Is(err, sql.ErrNoRows) || !bytes.Equal(storedPin, pin[:]) {
		return ErrUnauthorized
	}
	if err != nil {
		return err
	}
	storedRevision, err := decodeUint(currentRevision)
	if err != nil {
		return err
	}
	if storedRevision != revision || !bytes.Equal(currentDigest, digest[:]) {
		return ErrMembershipMismatch
	}
	return nil
}

type InventorySummary struct {
	ID             history.VersionID
	Path           string
	Kind           history.Kind
	ContentState   string
	EnvelopeDigest history.Digest
}

type InventoryPage struct {
	Token      [32]byte
	NextCursor uint64
	Done       bool
	Entries    []InventorySummary
}

func (db *DB) NewInventorySnapshot(ctx context.Context, folder, peer history.ID, now time.Time, lifetime time.Duration, maxOpen int) ([32]byte, error) {
	var token [32]byte
	if lifetime <= 0 || maxOpen <= 0 {
		return token, errors.New("snapshot lifetime and limit must be positive")
	}
	if _, err := io.ReadFull(rand.Reader, token[:]); err != nil {
		return token, err
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return token, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM inventory_snapshots WHERE expires_ns<=?`, now.UnixNano()); err != nil {
		return token, err
	}
	var open int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM inventory_snapshots WHERE folder_id=? AND peer_id=?`, folder[:], peer[:]).Scan(&open); err != nil {
		return token, err
	}
	if open >= maxOpen {
		return token, ErrSnapshotLimit
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO inventory_snapshots(token,folder_id,peer_id,created_ns,expires_ns) VALUES(?,?,?,?,?)`, token[:], folder[:], peer[:], now.UnixNano(), now.Add(lifetime).UnixNano()); err != nil {
		return token, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT author_id,counter,path,kind,content_state,envelope_digest FROM versions WHERE folder_id=? ORDER BY author_id,counter`, folder[:])
	if err != nil {
		return token, err
	}
	position := 0
	for rows.Next() {
		var author, counter, envelopeDigest []byte
		var path, state string
		var kind int
		if err := rows.Scan(&author, &counter, &path, &kind, &state, &envelopeDigest); err != nil {
			rows.Close()
			return token, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO inventory_snapshot_entries(token,position,author_id,counter,path,kind,content_state,envelope_digest) VALUES(?,?,?,?,?,?,?,?)`, token[:], position, author, counter, path, kind, state, envelopeDigest); err != nil {
			rows.Close()
			return token, err
		}
		position++
	}
	if err := rows.Close(); err != nil {
		return token, err
	}
	return token, tx.Commit()
}

func (db *DB) InventoryPage(ctx context.Context, token [32]byte, folder, peer history.ID, cursor uint64, limit int, now time.Time) (InventoryPage, error) {
	if limit <= 0 || limit > 128 {
		return InventoryPage{}, errors.New("inventory page limit is outside 1..128")
	}
	var expires int64
	var storedFolder, storedPeer []byte
	if err := db.db.QueryRowContext(ctx, `SELECT folder_id,peer_id,expires_ns FROM inventory_snapshots WHERE token=?`, token[:]).Scan(&storedFolder, &storedPeer, &expires); errors.Is(err, sql.ErrNoRows) {
		return InventoryPage{}, ErrSnapshotExpired
	} else if err != nil {
		return InventoryPage{}, err
	}
	if !bytes.Equal(storedFolder, folder[:]) || !bytes.Equal(storedPeer, peer[:]) {
		return InventoryPage{}, ErrUnauthorized
	}
	if now.UnixNano() >= expires {
		_, _ = db.db.ExecContext(ctx, `DELETE FROM inventory_snapshots WHERE token=?`, token[:])
		return InventoryPage{}, ErrSnapshotExpired
	}
	rows, err := db.db.QueryContext(ctx, `SELECT author_id,counter,path,kind,content_state,envelope_digest FROM inventory_snapshot_entries WHERE token=? AND position>=? ORDER BY position LIMIT ?`, token[:], cursor, limit+1)
	if err != nil {
		return InventoryPage{}, err
	}
	defer rows.Close()
	page := InventoryPage{Token: token, NextCursor: cursor}
	for rows.Next() {
		if len(page.Entries) == limit {
			page.Done = false
			return page, nil
		}
		var authorRaw, counterRaw, digestRaw []byte
		var entry InventorySummary
		entry.ID.Folder = folder
		var kind int
		if err := rows.Scan(&authorRaw, &counterRaw, &entry.Path, &kind, &entry.ContentState, &digestRaw); err != nil {
			return InventoryPage{}, err
		}
		copy(entry.ID.Author[:], authorRaw)
		entry.ID.Counter, err = decodeUint(counterRaw)
		if err != nil {
			return InventoryPage{}, err
		}
		entry.Kind = history.Kind(kind)
		copy(entry.EnvelopeDigest[:], digestRaw)
		page.Entries = append(page.Entries, entry)
		page.NextCursor++
	}
	if err := rows.Err(); err != nil {
		return InventoryPage{}, err
	}
	if err := rows.Close(); err != nil {
		return InventoryPage{}, err
	}
	page.Done = true
	if _, err := db.db.ExecContext(ctx, `DELETE FROM inventory_snapshots WHERE token=?`, token[:]); err != nil {
		return InventoryPage{}, err
	}
	return page, nil
}

// ReadAuthorizedChunk verifies the requested version is ready and the index is
// in that version's manifest before opening the content-addressed object.
func (db *DB) ReadAuthorizedChunk(ctx context.Context, id history.VersionID, index uint64) ([]byte, history.Chunk, error) {
	envelope, state, err := db.envelopeAndState(ctx, db.db, id)
	if err != nil {
		return nil, history.Chunk{}, err
	}
	if state != "ready" || envelope.Manifest == nil || index >= uint64(len(envelope.Manifest.Chunks)) {
		return nil, history.Chunk{}, ErrNotReady
	}
	chunk := envelope.Manifest.Chunks[index]

	leaseID := fmt.Sprintf("serve-%d-%x-%d", time.Now().UnixNano(), chunk.Digest[:4], index)
	if err := db.AcquireServeLease(ctx, chunk.Digest, leaseID); err != nil {
		return nil, history.Chunk{}, err
	}
	defer func() {
		_ = db.ReleaseServeLease(ctx, chunk.Digest, leaseID)
	}()

	if err := db.verifyObjectFile(chunk.Digest, chunk.Length); err != nil {
		if errors.Is(err, ErrContentMismatch) {
			_, _ = db.QuarantineChunk(ctx, chunk.Digest, "corrupt chunk on read")
		}
		return nil, history.Chunk{}, err
	}
	data, err := os.ReadFile(db.objectPath(chunk.Digest))
	if err != nil {
		return nil, history.Chunk{}, err
	}
	if uint64(len(data)) != chunk.Length {
		_, _ = db.QuarantineChunk(ctx, chunk.Digest, "chunk length changed")
		return nil, history.Chunk{}, fmt.Errorf("%w: chunk length changed", ErrContentMismatch)
	}
	return data, chunk, nil
}

// HasMembershipFork checks if there are concurrent competing membership revisions recorded for a folder,
// or if the folder was paused specifically due to a membership fork.
func (db *DB) HasMembershipFork(ctx context.Context, folder history.ID) (bool, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	var count int
	err := db.db.QueryRowContext(ctx, `
		SELECT count(*) FROM (
			SELECT prior_digest FROM membership_revisions
			WHERE folder_id=? AND prior_digest IS NOT NULL
			GROUP BY prior_digest HAVING count(*) > 1
		)`, folder[:]).Scan(&count)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	var paused int
	var pauseReason string
	err = db.db.QueryRowContext(ctx, `SELECT COALESCE(paused, 0), COALESCE(pause_reason, '') FROM folders WHERE folder_id=?`, folder[:]).Scan(&paused, &pauseReason)
	if err == nil && paused == 1 && pauseReason == "MEMBERSHIP_FORK" {
		return true, nil
	}
	return false, nil
}
