package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"golang.org/x/sys/unix"
)

var (
	ErrContentMismatch = errors.New("content does not match declared digest or length")
	ErrContentMissing  = errors.New("required content is missing")
	ErrBudgetExceeded  = errors.New("repository storage budget exceeded")
)

func (db *DB) objectPath(digest history.Digest) string {
	hexDigest := hex.EncodeToString(digest[:])
	return filepath.Join(db.stateDir, "objects", "sha256", hexDigest[:2], hexDigest[2:])
}

// InstallChunk streams, verifies, flushes and atomically installs one immutable
// object. A duplicate installation verifies the existing object and never
// replaces it.
func (db *DB) InstallChunk(ctx context.Context, digest history.Digest, length uint64, source io.Reader) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if length > history.ChunkSize {
		return fmt.Errorf("%w: chunk too large", ErrContentMismatch)
	}
	if err := db.checkFreeSpaceReserve(ctx); err != nil {
		return err
	}
	// D4: new reference cancels active GC intent
	_, _ = db.db.ExecContext(ctx, `DELETE FROM gc_intents WHERE digest=?`, digest[:])
	if db.budgetBytes > 0 {
		usage, err := db.usageUnlocked()
		if err != nil {
			return err
		}
		reserved, err := db.reservedBytes(ctx)
		if err != nil {
			return err
		}
		used := usage.Total()
		if length > db.budgetBytes || used > db.budgetBytes-length || reserved > db.budgetBytes-used-length {
			return ErrBudgetExceeded
		}
	}
	target := db.objectPath(digest)
	if _, err := os.Lstat(target); err == nil {
		return db.verifyObjectFile(digest, length)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	shard := filepath.Dir(target)
	if err := os.Mkdir(shard, 0o700); err == nil {
		if err := syncDir(filepath.Dir(shard)); err != nil {
			return fmt.Errorf("flush object shard parent: %w", err)
		}
	} else if !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create object shard: %w", err)
	} else if info, statErr := os.Lstat(shard); statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("object shard is not a directory")
	}
	temporary, err := os.CreateTemp(filepath.Join(db.stateDir, "incoming"), "chunk-*")
	if err != nil {
		return fmt.Errorf("create incoming chunk: %w", err)
	}
	tempPath := temporary.Name()
	keep := false
	defer func() {
		temporary.Close()
		if !keep {
			os.Remove(tempPath)
		}
	}()
	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(temporary, hasher), io.LimitReader(source, int64(length)+1))
	if err != nil {
		return fmt.Errorf("write incoming chunk: %w", err)
	}
	if written != int64(length) || !equalHash(hasher, digest) {
		return ErrContentMismatch
	}
	var extra [1]byte
	if n, err := source.Read(extra[:]); n != 0 || (err != nil && !errors.Is(err, io.EOF)) {
		return ErrContentMismatch
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("flush incoming chunk: %w", err)
	}
	if err := db.callHook(HookObjectFlushed); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close incoming chunk: %w", err)
	}
	err = unix.Renameat2(unix.AT_FDCWD, tempPath, unix.AT_FDCWD, target, unix.RENAME_NOREPLACE)
	if errors.Is(err, syscall.EEXIST) {
		if err := db.verifyObjectFile(digest, length); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("install immutable object: %w", err)
	} else {
		keep = true
		if err := syncDir(filepath.Dir(target)); err != nil {
			return fmt.Errorf("flush object directory: %w", err)
		}
	}
	if err := db.callHook(HookObjectInstalled); err != nil {
		return err
	}
	result, err := db.db.ExecContext(ctx, `INSERT INTO objects(digest,length,verified) VALUES(?,?,1) ON CONFLICT(digest) DO UPDATE SET verified=1 WHERE length=excluded.length`, digest[:], encodeUint(length))
	if err != nil {
		return fmt.Errorf("record immutable object: %w", err)
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return ErrContentMismatch
	}
	return db.callHook(HookObjectRecorded)
}

func equalHash(hasher hash.Hash, digest history.Digest) bool {
	sum := hasher.Sum(nil)
	for i := range digest {
		if sum[i] != digest[i] {
			return false
		}
	}
	return true
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (db *DB) verifyObjectFile(digest history.Digest, length uint64) error {
	file, err := os.Open(db.objectPath(digest))
	if errors.Is(err, os.ErrNotExist) {
		return ErrContentMissing
	}
	if err != nil {
		return err
	}
	defer file.Close()
	hasher := sha256.New()
	n, err := io.Copy(hasher, file)
	if err != nil {
		return err
	}
	if uint64(n) != length || !equalHash(hasher, digest) {
		return ErrContentMismatch
	}
	return nil
}

// indexInstalledObjects makes a crash after rename but before the SQLite
// inventory update visible as an orphan. It never marks malformed bytes ready.
func (db *DB) indexInstalledObjects(ctx context.Context) error {
	root := filepath.Join(db.stateDir, "objects", "sha256")
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		directory, name := filepath.Split(relative)
		shard := filepath.Base(filepath.Clean(directory))
		hexDigest := shard + name
		decoded, err := hex.DecodeString(hexDigest)
		if err != nil || len(decoded) != sha256.Size || len(shard) != 2 {
			return nil
		}
		var digest history.Digest
		copy(digest[:], decoded)
		if err := db.verifyObjectFile(digest, uint64(info.Size())); err != nil {
			return nil
		}
		_, err = db.db.ExecContext(ctx, `INSERT INTO objects(digest,length,verified) VALUES(?,?,1) ON CONFLICT(digest) DO NOTHING`, digest[:], encodeUint(uint64(info.Size())))
		return err
	})
}

// StoreFile chunks a stream with bounded memory, installs each verified chunk,
// and returns its complete manifest. The whole-file digest is independent of
// chunk boundaries.
func (db *DB) StoreFile(ctx context.Context, source io.Reader, executable bool) (*history.Manifest, error) {
	whole := sha256.New()
	manifest := &history.Manifest{Executable: executable}
	buffer := make([]byte, history.ChunkSize)
	for {
		n, err := io.ReadFull(source, buffer)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("read file content: %w", err)
		}
		if n == 0 {
			break
		}
		if manifest.Size > history.MaxFileSize-uint64(n) {
			return nil, fmt.Errorf("file exceeds maximum size")
		}
		part := buffer[:n]
		whole.Write(part)
		digest := sha256.Sum256(part)
		if err := db.InstallChunk(ctx, digest, uint64(n), bytesReader(part)); err != nil {
			return nil, err
		}
		manifest.Chunks = append(manifest.Chunks, history.Chunk{Digest: digest, Length: uint64(n)})
		manifest.Size += uint64(n)
		if errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
	}
	copy(manifest.Digest[:], whole.Sum(nil))
	return manifest, nil
}

// bytesReader avoids retaining a reader past InstallChunk's synchronous call.
type sliceReader struct {
	value  []byte
	offset int
}

func bytesReader(value []byte) *sliceReader { return &sliceReader{value: value} }
func (r *sliceReader) Read(p []byte) (int, error) {
	if r.offset == len(r.value) {
		return 0, io.EOF
	}
	n := copy(p, r.value[r.offset:])
	r.offset += n
	return n, nil
}

func (db *DB) VerifyManifest(manifest *history.Manifest) error {
	if manifest == nil {
		return ErrContentMismatch
	}
	whole := sha256.New()
	var total uint64
	for _, chunk := range manifest.Chunks {
		file, err := os.Open(db.objectPath(chunk.Digest))
		if errors.Is(err, os.ErrNotExist) {
			return ErrContentMissing
		}
		if err != nil {
			return err
		}
		hasher := sha256.New()
		n, copyErr := io.Copy(io.MultiWriter(whole, hasher), file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if uint64(n) != chunk.Length || !equalHash(hasher, chunk.Digest) {
			return ErrContentMismatch
		}
		total += uint64(n)
	}
	if total != manifest.Size || !equalHash(whole, manifest.Digest) {
		return ErrContentMismatch
	}
	return nil
}

type Usage struct{ Metadata, Objects, Incoming, Quarantine, Operations, Reserved uint64 }

func (u Usage) Total() uint64 {
	return u.Metadata + u.Objects + u.Incoming + u.Quarantine + u.Operations + u.Reserved
}

func (db *DB) StorageUsage(ctx context.Context) (Usage, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	u, err := db.usageUnlocked()
	if err != nil {
		return Usage{}, err
	}
	u.Reserved, err = db.reservedBytes(ctx)
	return u, err
}
func (db *DB) usageUnlocked() (Usage, error) {
	var result Usage
	for _, item := range []struct {
		name   string
		target *uint64
	}{{"metadata.sqlite", &result.Metadata}, {"metadata.sqlite-wal", &result.Metadata}, {"metadata.sqlite-shm", &result.Metadata}, {"objects", &result.Objects}, {"incoming", &result.Incoming}, {"quarantine", &result.Quarantine}, {"operations", &result.Operations}} {
		path := filepath.Join(db.stateDir, item.name)
		err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				*item.target += uint64(info.Size())
			}
			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return Usage{}, err
		}
	}
	return result, nil
}
func (db *DB) reservedBytes(ctx context.Context) (uint64, error) {
	rows, err := db.db.QueryContext(ctx, `SELECT bytes FROM reservations`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var total uint64
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return 0, err
		}
		value, err := decodeUint(raw)
		if err != nil {
			return 0, err
		}
		if ^uint64(0)-total < value {
			return 0, ErrBudgetExceeded
		}
		total += value
	}
	return total, rows.Err()
}

func (db *DB) Reserve(ctx context.Context, id string, bytes uint64, purpose string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if id == "" || purpose == "" {
		return errors.New("reservation id and purpose are required")
	}
	if db.budgetBytes > 0 {
		usage, err := db.usageUnlocked()
		if err != nil {
			return err
		}
		reserved, err := db.reservedBytes(ctx)
		if err != nil {
			return err
		}
		used := usage.Total()
		if bytes > db.budgetBytes || used > db.budgetBytes-bytes || reserved > db.budgetBytes-used-bytes {
			return ErrBudgetExceeded
		}
	}
	_, err := db.db.ExecContext(ctx, `INSERT INTO reservations(reservation_id,bytes,purpose,created_ns) VALUES(?,?,?,?)`, id, encodeUint(bytes), purpose, time.Now().UnixNano())
	return err
}
func (db *DB) ReleaseReservation(ctx context.Context, id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.db.ExecContext(ctx, `DELETE FROM reservations WHERE reservation_id=?`, id)
	return err
}

func (db *DB) OrphanObjects(ctx context.Context) ([]history.Digest, error) {
	rows, err := db.db.QueryContext(ctx, `SELECT digest FROM objects o WHERE NOT EXISTS(SELECT 1 FROM object_references r WHERE r.digest=o.digest) AND NOT EXISTS(SELECT 1 FROM content_pins p WHERE p.digest=o.digest) ORDER BY digest`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []history.Digest
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var digest history.Digest
		copy(digest[:], raw)
		result = append(result, digest)
	}
	return result, rows.Err()
}

func (db *DB) Pin(ctx context.Context, digest history.Digest, kind, key string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	result, err := db.db.ExecContext(ctx, `INSERT INTO content_pins(digest,owner_kind,owner_key,created_ns) VALUES(?,?,?,?) ON CONFLICT DO NOTHING`, digest[:], kind, key, time.Now().UnixNano())
	if err != nil {
		return err
	}
	_, _ = result.RowsAffected()
	_, err = db.db.ExecContext(ctx, `DELETE FROM gc_intents WHERE digest=?`, digest[:])
	return err
}
func (db *DB) Unpin(ctx context.Context, digest history.Digest, kind, key string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.db.ExecContext(ctx, `DELETE FROM content_pins WHERE digest=? AND owner_kind=? AND owner_key=?`, digest[:], kind, key)
	return err
}
