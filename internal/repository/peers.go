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

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
)

var (
	ErrUnauthorized       = errors.New("peer is not authorized for this folder")
	ErrMembershipMismatch = errors.New("membership revision or digest mismatch")
	ErrSnapshotExpired    = errors.New("inventory snapshot expired or unknown")
	ErrSnapshotLimit      = errors.New("too many open inventory snapshots")
)

type ApprovedMembership struct {
	Revision uint64
	Digest   history.Digest
}

// ApproveMembership durably installs an exact owner-reviewed membership
// revision. Existing revisions are immutable and replay-idempotent.
func (db *DB) ApproveMembership(ctx context.Context, membership protocol.Membership) (ApprovedMembership, error) {
	digest, err := protocol.MembershipDigest(membership)
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
			return ApprovedMembership{Revision: membership.Revision, Digest: digest}, nil
		}
		if membership.Revision != currentRevision+1 || !bytes.Equal(currentDigestRaw, membership.PriorDigest[:]) {
			return ApprovedMembership{}, ErrMembershipMismatch
		}
	} else if membership.Revision != currentRevision || membership.PriorDigest != (history.Digest{}) {
		return ApprovedMembership{}, ErrMembershipMismatch
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
	}
	if _, err := tx.ExecContext(ctx, `UPDATE folders SET membership_revision=?,membership_digest=? WHERE folder_id=?`, encodeUint(membership.Revision), digest[:], membership.Folder[:]); err != nil {
		return ApprovedMembership{}, err
	}
	if err := tx.Commit(); err != nil {
		return ApprovedMembership{}, err
	}
	return ApprovedMembership{Revision: membership.Revision, Digest: digest}, nil
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
	page.Done = true
	return page, rows.Err()
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
	if err := db.verifyObjectFile(chunk.Digest, chunk.Length); err != nil {
		return nil, history.Chunk{}, err
	}
	data, err := os.ReadFile(db.objectPath(chunk.Digest))
	if err != nil {
		return nil, history.Chunk{}, err
	}
	if uint64(len(data)) != chunk.Length {
		return nil, history.Chunk{}, fmt.Errorf("%w: chunk length changed", ErrContentMismatch)
	}
	return data, chunk, nil
}
