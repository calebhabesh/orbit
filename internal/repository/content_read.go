package repository

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
)

// VersionRead pins an immutable exact version for the entire response, including
// range reads. It holds one verified chunk at a time. Close is mandatory.
// A live stream pin has no timer: wall-clock expiry must not delete bytes from
// a slow response. Startup removes abandoned stream pins after GC recovery.
type VersionRead struct {
	db         *DB
	ctx        context.Context
	Envelope   history.Envelope
	leaseID    string
	offset     int64
	chunkIndex int
	chunk      []byte
	closeOnce  sync.Once
	closed     bool
}

func (db *DB) OpenVersionRead(ctx context.Context, id history.VersionID) (*VersionRead, error) {
	db.mu.Lock()
	env, state, err := db.envelopeAndState(ctx, db.db, id)
	if err != nil {
		db.mu.Unlock()
		return nil, err
	}
	if env.Kind != history.KindFile || env.Manifest == nil {
		db.mu.Unlock()
		return nil, ErrNotAFile
	}
	if state != "ready" {
		db.mu.Unlock()
		return nil, ErrNotReady
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		db.mu.Unlock()
		return nil, err
	}
	leaseID := "stream-" + hex.EncodeToString(raw)
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		db.mu.Unlock()
		return nil, err
	}
	fail := func(err error) (*VersionRead, error) { tx.Rollback(); db.mu.Unlock(); return nil, err }
	now := time.Now()
	if _, err := tx.ExecContext(ctx, `INSERT INTO read_leases(lease_id,folder_id,version_author,version_counter,expires_ns,created_ns) VALUES(?,?,?,?,?,?)`, leaseID, id.Folder[:], id.Author[:], encodeUint(id.Counter), now.Add(5*time.Minute).UnixNano(), now.UnixNano()); err != nil {
		return fail(err)
	}
	seen := map[history.Digest]bool{}
	for _, chunk := range env.Manifest.Chunks {
		if seen[chunk.Digest] {
			continue
		}
		seen[chunk.Digest] = true
		var verified, intents int
		err := tx.QueryRowContext(ctx, `SELECT verified,(SELECT COUNT(*) FROM gc_intents WHERE digest=objects.digest) FROM objects WHERE digest=?`, chunk.Digest[:]).Scan(&verified, &intents)
		if err != nil || verified != 1 {
			return fail(ErrContentMissing)
		}
		if intents != 0 {
			return fail(ErrGCIntentActive)
		}
		if _, err := os.Stat(db.objectPath(chunk.Digest)); err != nil {
			return fail(ErrContentMissing)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO read_lease_chunks(lease_id,chunk_digest) VALUES(?,?)`, leaseID, chunk.Digest[:]); err != nil {
			return fail(err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO content_pins(digest,owner_kind,owner_key,created_ns) VALUES(?,'stream',?,?)`, chunk.Digest[:], leaseID, now.UnixNano()); err != nil {
			return fail(err)
		}
	}
	if err := tx.Commit(); err != nil {
		db.mu.Unlock()
		return nil, err
	}
	db.mu.Unlock()
	read := &VersionRead{db: db, ctx: ctx, Envelope: env, leaseID: leaseID, chunkIndex: -1}
	// Validate the whole manifest before headers. Each chunk is checked again
	// before it is emitted so mutation/corruption never substitutes bytes.
	if err := read.verify(); err != nil {
		read.Close()
		return nil, err
	}
	return read, nil
}

func (r *VersionRead) loadChunk(index int) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	if r.chunkIndex == index {
		return nil
	}
	chunk := r.Envelope.Manifest.Chunks[index]
	f, err := os.Open(r.db.objectPath(chunk.Digest))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrContentMissing, err)
	}
	if cap(r.chunk) < int(chunk.Length)+1 {
		r.chunk = make([]byte, int(chunk.Length)+1)
	} else {
		r.chunk = r.chunk[:int(chunk.Length)+1]
	}
	n, err := io.ReadFull(f, r.chunk)
	f.Close()
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return err
	}
	r.chunk = r.chunk[:n]
	if uint64(n) != chunk.Length || sha256.Sum256(r.chunk) != chunk.Digest {
		r.chunkIndex = -1
		_, _ = r.db.QuarantineChunk(context.Background(), chunk.Digest, "corrupt exact-version read")
		return ErrContentMismatch
	}
	r.chunkIndex = index
	return nil
}

func (r *VersionRead) verify() error {
	whole := sha256.New()
	for i := range r.Envelope.Manifest.Chunks {
		if err := r.loadChunk(i); err != nil {
			return err
		}
		whole.Write(r.chunk)
	}
	if !equalHash(whole, r.Envelope.Manifest.Digest) {
		return ErrContentMismatch
	}
	// Do not keep a preflight buffer eligible for emission after verification.
	r.chunkIndex = -1
	return nil
}

func (r *VersionRead) Read(p []byte) (int, error) {
	if r.closed {
		return 0, os.ErrClosed
	}
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) == 0 {
		return 0, nil
	}
	if uint64(r.offset) >= r.Envelope.Manifest.Size {
		return 0, io.EOF
	}
	index := int(uint64(r.offset) / history.ChunkSize)
	if err := r.loadChunk(index); err != nil {
		return 0, err
	}
	offset := int(uint64(r.offset) % history.ChunkSize)
	n := copy(p, r.chunk[offset:])
	r.offset += int64(n)
	return n, nil
}

func (r *VersionRead) Seek(offset int64, whence int) (int64, error) {
	if r.closed {
		return 0, os.ErrClosed
	}
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += r.offset
	case io.SeekEnd:
		offset += int64(r.Envelope.Manifest.Size)
	default:
		return 0, errors.New("invalid seek origin")
	}
	if offset < 0 || uint64(offset) > r.Envelope.Manifest.Size {
		return 0, errors.New("invalid content offset")
	}
	r.offset = offset
	return offset, nil
}

func (r *VersionRead) Close() error {
	var result error
	r.closeOnce.Do(func() {
		r.closed = true
		r.chunk = nil
		r.db.mu.Lock()
		defer r.db.mu.Unlock()
		tx, err := r.db.db.BeginTx(context.Background(), nil)
		if err != nil {
			result = err
			return
		}
		defer tx.Rollback()
		if _, err = tx.Exec(`DELETE FROM content_pins WHERE owner_kind='stream' AND owner_key=?`, r.leaseID); err != nil {
			result = err
			return
		}
		if _, err = tx.Exec(`DELETE FROM read_leases WHERE lease_id=?`, r.leaseID); err != nil {
			result = err
			return
		}
		result = tx.Commit()
	})
	return result
}

func (db *DB) clearAbandonedReads(ctx context.Context) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM content_pins WHERE owner_kind='stream'`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM read_leases WHERE lease_id LIKE 'stream-%'`); err != nil {
		return err
	}
	return tx.Commit()
}

// CheckLocalWorkspaceAccess requires a registered local workspace and, when
// membership is configured, the local author in its current active revision.
func (db *DB) CheckLocalWorkspaceAccess(ctx context.Context, folder history.ID) error {
	var allowed int
	err := db.db.QueryRowContext(ctx, `SELECT (NOT EXISTS(SELECT 1 FROM membership_entries e WHERE e.folder_id=f.folder_id AND e.revision=f.membership_revision) OR EXISTS(SELECT 1 FROM membership_entries e JOIN membership_revisions r ON r.folder_id=e.folder_id AND r.revision=e.revision WHERE e.folder_id=f.folder_id AND e.revision=f.membership_revision AND e.device_id=f.local_author AND e.state='active' AND r.approved=1)) FROM folders f WHERE folder_id=? AND root_path IS NOT NULL`, folder[:]).Scan(&allowed)
	if err != nil || allowed != 1 {
		return ErrUnauthorized
	}
	return nil
}
