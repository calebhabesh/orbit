package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
)

type Transfer struct {
	ID            string
	Folder        history.ID
	Peer          history.ID
	Version       history.VersionID
	State         string
	ReservedBytes uint64
	Attempts      int
	LastError     string
}

func EnvelopeDigest(envelope history.Envelope) history.Digest { return envelopeDigest(envelope) }

func (db *DB) VersionIDs(ctx context.Context, folder history.ID) ([]history.VersionID, error) {
	rows, err := db.db.QueryContext(ctx, `SELECT author_id,counter FROM versions WHERE folder_id=? ORDER BY author_id,counter`, folder[:])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []history.VersionID
	for rows.Next() {
		var authorRaw, counterRaw []byte
		if err := rows.Scan(&authorRaw, &counterRaw); err != nil {
			return nil, err
		}
		var author history.ID
		copy(author[:], authorRaw)
		counter, err := decodeUint(counterRaw)
		if err != nil {
			return nil, err
		}
		result = append(result, history.VersionID{Folder: folder, Author: author, Counter: counter})
	}
	return result, rows.Err()
}

// Membership returns the exact locally approved handshake for a folder.
func (db *DB) Membership(ctx context.Context, folder history.ID) (ApprovedMembership, error) {
	var revisionRaw, digestRaw []byte
	if err := db.db.QueryRowContext(ctx, `SELECT membership_revision,membership_digest FROM folders WHERE folder_id=?`, folder[:]).Scan(&revisionRaw, &digestRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ApprovedMembership{}, ErrFolderUnknown
		}
		return ApprovedMembership{}, err
	}
	revision, err := decodeUint(revisionRaw)
	if err != nil || len(digestRaw) != len(history.Digest{}) {
		return ApprovedMembership{}, ErrMembershipMismatch
	}
	var digest history.Digest
	copy(digest[:], digestRaw)
	return ApprovedMembership{Revision: revision, Digest: digest}, nil
}

// VerifiedChunk reports availability only after rehashing the installed
// immutable object. A stale SQLite object row never counts as progress.
func (db *DB) VerifiedChunk(ctx context.Context, chunk history.Chunk) (bool, error) {
	var lengthRaw []byte
	var verified int
	err := db.db.QueryRowContext(ctx, `SELECT length,verified FROM objects WHERE digest=?`, chunk.Digest[:]).Scan(&lengthRaw, &verified)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	length, err := decodeUint(lengthRaw)
	if err != nil || verified != 1 || length != chunk.Length {
		return false, nil
	}
	if err := db.verifyObjectFile(chunk.Digest, chunk.Length); err != nil {
		if errors.Is(err, ErrContentMissing) || errors.Is(err, ErrContentMismatch) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// BeginTransfer creates or resumes one stable peer/version transfer and its
// manifest positions. Replays must describe the same immutable manifest.
func (db *DB) BeginTransfer(ctx context.Context, transfer Transfer, manifest *history.Manifest) error {
	if transfer.ID == "" || transfer.Peer == (history.ID{}) || transfer.Version.Folder != transfer.Folder || manifest == nil {
		return errors.New("invalid transfer identity or manifest")
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO transfers(transfer_id,folder_id,state,reserved_bytes,peer_id,version_author,version_counter,attempts,updated_ns)
		VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(transfer_id) DO UPDATE SET updated_ns=excluded.updated_ns`, transfer.ID, transfer.Folder[:], "receiving", encodeUint(transfer.ReservedBytes), transfer.Peer[:], transfer.Version.Author[:], encodeUint(transfer.Version.Counter), 0, time.Now().UnixNano())
	if err != nil {
		return err
	}
	var folderRaw, peerRaw, authorRaw, counterRaw []byte
	if err := tx.QueryRowContext(ctx, `SELECT folder_id,peer_id,version_author,version_counter FROM transfers WHERE transfer_id=?`, transfer.ID).Scan(&folderRaw, &peerRaw, &authorRaw, &counterRaw); err != nil {
		return err
	}
	if string(folderRaw) != string(transfer.Folder[:]) || string(peerRaw) != string(transfer.Peer[:]) || string(authorRaw) != string(transfer.Version.Author[:]) || string(counterRaw) != string(encodeUint(transfer.Version.Counter)) {
		return errors.New("transfer identity was reused with different parameters")
	}
	for position, chunk := range manifest.Chunks {
		result, err := tx.ExecContext(ctx, `INSERT INTO transfer_chunks(transfer_id,position,digest,length,verified) VALUES(?,?,?,?,0)
			ON CONFLICT(transfer_id,position) DO UPDATE SET digest=excluded.digest,length=excluded.length
			WHERE transfer_chunks.digest=excluded.digest AND transfer_chunks.length=excluded.length`, transfer.ID, position, chunk.Digest[:], encodeUint(chunk.Length))
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return errors.New("transfer manifest changed during resume")
		}
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM transfer_chunks WHERE transfer_id=?`, transfer.ID).Scan(&count); err != nil {
		return err
	}
	if count != len(manifest.Chunks) {
		return errors.New("transfer manifest length changed during resume")
	}
	if transfer.ReservedBytes > 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO reservations(reservation_id,bytes,purpose,created_ns) VALUES(?,?,?,?)
			ON CONFLICT(reservation_id) DO NOTHING`, transfer.ID, encodeUint(transfer.ReservedBytes), "peer transfer", time.Now().UnixNano())
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) MarkTransferChunkVerified(ctx context.Context, transferID string, position int, chunk history.Chunk) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var lengthRaw []byte
	var verified int
	if err := tx.QueryRowContext(ctx, `SELECT length,verified FROM objects WHERE digest=?`, chunk.Digest[:]).Scan(&lengthRaw, &verified); err != nil {
		return err
	}
	length, err := decodeUint(lengthRaw)
	if err != nil || verified != 1 || length != chunk.Length {
		return ErrContentMismatch
	}
	result, err := tx.ExecContext(ctx, `UPDATE transfer_chunks SET verified=1 WHERE transfer_id=? AND position=? AND digest=? AND length=?`, transferID, position, chunk.Digest[:], encodeUint(chunk.Length))
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return errors.New("transfer chunk does not match persisted manifest")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO content_pins(digest,owner_kind,owner_key,created_ns) VALUES(?,?,?,?) ON CONFLICT DO NOTHING`, chunk.Digest[:], "transfer", transferID, time.Now().UnixNano()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE transfers SET updated_ns=? WHERE transfer_id=?`, time.Now().UnixNano(), transferID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return db.callHook(HookTransferProgress)
}

func (db *DB) TransferVerifiedPositions(ctx context.Context, transferID string) (map[int]bool, error) {
	rows, err := db.db.QueryContext(ctx, `SELECT position FROM transfer_chunks WHERE transfer_id=? AND verified=1 ORDER BY position`, transferID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[int]bool{}
	for rows.Next() {
		var position int
		if err := rows.Scan(&position); err != nil {
			return nil, err
		}
		result[position] = true
	}
	return result, rows.Err()
}

func (db *DB) CompleteTransfer(ctx context.Context, transferID string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var incomplete int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM transfer_chunks WHERE transfer_id=? AND verified=0`, transferID).Scan(&incomplete); err != nil {
		return err
	}
	if incomplete != 0 {
		return errors.New("cannot complete transfer with unverified chunks")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE transfers SET state='complete',reserved_bytes=?,last_error=NULL,updated_ns=? WHERE transfer_id=?`, encodeUint(0), time.Now().UnixNano(), transferID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM content_pins WHERE owner_kind='transfer' AND owner_key=?`, transferID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM reservations WHERE reservation_id=?`, transferID); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) FailTransfer(ctx context.Context, transferID string, cause error) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	_, err := db.db.ExecContext(ctx, `UPDATE transfers SET state='retry',attempts=attempts+1,last_error=?,updated_ns=? WHERE transfer_id=?`, message, time.Now().UnixNano(), transferID)
	return err
}

type VersionStatus struct {
	ID            history.VersionID
	Path          string
	MetadataKnown bool
	Stored        bool
	Applied       bool
	Conflict      bool
	Blocked       bool
	ContentState  string
}

func (db *DB) VersionStatus(ctx context.Context, id history.VersionID) (VersionStatus, error) {
	status := VersionStatus{ID: id}
	envelope, state, err := db.envelopeAndState(ctx, db.db, id)
	if errors.Is(err, sql.ErrNoRows) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	status.MetadataKnown, status.Path, status.ContentState = true, envelope.Path, state
	status.Stored = state == "ready"
	status.Applied, err = db.WorkingApplied(ctx, id)
	if err != nil {
		return status, err
	}
	h, err := loadHistoryReadOnly(ctx, db, id.Folder, envelope.Path)
	if err != nil {
		return status, err
	}
	status.Conflict = len(h.Heads(id.Folder, envelope.Path)) > 1
	var block sql.NullString
	err = db.db.QueryRowContext(ctx, `SELECT block_reason FROM path_projections WHERE folder_id=? AND path=?`, id.Folder[:], envelope.Path).Scan(&block)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return status, err
	}
	status.Blocked = block.Valid && block.String != ""
	return status, nil
}

func loadHistoryReadOnly(ctx context.Context, db *DB, folder history.ID, paths ...string) (*history.History, error) {
	return loadHistoryQuery(ctx, db.db, folder, paths...)
}

// UnappliedSingleHeads returns paths that have exactly one causally maximal head,
// are content-ready, are not yet working-applied, and are not blocked by structural
// conflicts. Conflicting and structurally blocked paths are left unapplied.
func (db *DB) UnappliedSingleHeads(ctx context.Context, folder history.ID) ([]history.VersionID, error) {
	h, err := loadHistoryReadOnly(ctx, db, folder)
	if err != nil {
		return nil, err
	}
	structural := h.StructuralConflicts(folder)
	structurallyBlocked := map[string]bool{}
	for _, sc := range structural {
		structurallyBlocked[sc.AncestorPath] = true
		structurallyBlocked[sc.DescendantPath] = true
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
	var result []history.VersionID
	for _, path := range paths {
		if structurallyBlocked[path] {
			continue
		}
		heads := h.Heads(folder, path)
		if len(heads) != 1 {
			continue
		}
		ready, err := db.ContentReady(ctx, heads[0].ID)
		if err != nil || !ready {
			continue
		}
		applied, err := db.WorkingApplied(ctx, heads[0].ID)
		if err != nil {
			return nil, err
		}
		if !applied {
			result = append(result, heads[0].ID)
		}
	}
	return result, nil
}

func (db *DB) RecordPeerReceipt(ctx context.Context, folder, peer history.ID, version history.VersionID, now time.Time) error {
	return db.RecordPeerReceiptWithOptions(ctx, folder, peer, version, true, now)
}

func (db *DB) RecordPeerReceiptWithOptions(ctx context.Context, folder, peer history.ID, version history.VersionID, direct bool, now time.Time) error {
	if version.Folder != folder {
		return errors.New("receipt folder mismatch")
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	var present int
	if err := db.db.QueryRowContext(ctx, `SELECT 1 FROM versions WHERE folder_id=? AND author_id=? AND counter=?`, folder[:], version.Author[:], encodeUint(version.Counter)).Scan(&present); err != nil {
		return fmt.Errorf("receipt names unknown version: %w", err)
	}
	directInt := 0
	if direct {
		directInt = 1
	}
	_, err := db.db.ExecContext(ctx, `INSERT INTO peer_progress(folder_id,peer_id,version_author,version_counter,receipt,direct,last_contact_ns)
		VALUES(?,?,?,?,1,?,?) ON CONFLICT(folder_id,peer_id,version_author,version_counter)
		DO UPDATE SET receipt=1,direct=excluded.direct,last_contact_ns=excluded.last_contact_ns`, folder[:], peer[:], version.Author[:], encodeUint(version.Counter), directInt, now.UnixNano())
	if err != nil {
		return err
	}
	if _, err := db.db.ExecContext(ctx, `INSERT INTO peer_contacts(folder_id,peer_id,last_contact_ns) VALUES(?,?,?)
		ON CONFLICT(folder_id,peer_id) DO UPDATE SET last_contact_ns=excluded.last_contact_ns`, folder[:], peer[:], now.UnixNano()); err != nil {
		return err
	}
	return db.callHook(HookReceiptRecorded)
}

func (db *DB) RecordPeerStatus(ctx context.Context, folder, peer history.ID, version history.VersionID, status string, now time.Time) error {
	return db.RecordPeerStatusWithOptions(ctx, folder, peer, version, status, true, now)
}

func (db *DB) RecordPeerStatusWithOptions(ctx context.Context, folder, peer history.ID, version history.VersionID, status string, direct bool, now time.Time) error {
	directInt := 0
	if direct {
		directInt = 1
	}
	_, err := db.db.ExecContext(ctx, `INSERT INTO peer_progress(folder_id,peer_id,version_author,version_counter,receipt,direct,remote_status,last_contact_ns)
		VALUES(?,?,?,?,0,?,?,?) ON CONFLICT(folder_id,peer_id,version_author,version_counter)
		DO UPDATE SET remote_status=excluded.remote_status,direct=excluded.direct,last_contact_ns=excluded.last_contact_ns`, folder[:], peer[:], version.Author[:], encodeUint(version.Counter), directInt, status, now.UnixNano())
	if err != nil {
		return err
	}
	_, err = db.db.ExecContext(ctx, `INSERT INTO peer_contacts(folder_id,peer_id,last_contact_ns) VALUES(?,?,?)
		ON CONFLICT(folder_id,peer_id) DO UPDATE SET last_contact_ns=excluded.last_contact_ns`, folder[:], peer[:], now.UnixNano())
	return err
}

type PeerProgress struct {
	Peer        history.ID        `json:"peer"`
	Version     history.VersionID `json:"version"`
	Receipt     bool              `json:"receipt"`
	Direct      bool              `json:"direct"`
	RemoteState string            `json:"remote_state"`
	LastContact time.Time         `json:"last_contact"`
}

func (db *DB) PeerProgress(ctx context.Context, folder history.ID) ([]PeerProgress, error) {
	rows, err := db.db.QueryContext(ctx, `SELECT peer_id,version_author,version_counter,receipt,COALESCE(direct,1),COALESCE(remote_status,''),COALESCE(last_contact_ns,0)
		FROM peer_progress WHERE folder_id=? AND version_author IS NOT NULL ORDER BY peer_id,version_author,version_counter`, folder[:])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PeerProgress
	for rows.Next() {
		var item PeerProgress
		var peerRaw, authorRaw, counterRaw []byte
		var receipt, direct int
		var contact int64
		if err := rows.Scan(&peerRaw, &authorRaw, &counterRaw, &receipt, &direct, &item.RemoteState, &contact); err != nil {
			return nil, err
		}
		copy(item.Peer[:], peerRaw)
		copy(item.Version.Author[:], authorRaw)
		item.Version.Folder = folder
		item.Version.Counter, err = decodeUint(counterRaw)
		if err != nil {
			return nil, err
		}
		item.Receipt = receipt == 1
		item.Direct = direct == 1
		if contact > 0 {
			item.LastContact = time.Unix(0, contact)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
