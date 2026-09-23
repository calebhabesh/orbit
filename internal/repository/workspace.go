package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/calebhabesh/file-sync/internal/history"
)

var (
	ErrRootNotRegistered = errors.New("workspace root is not registered")
	ErrStaleGeneration   = errors.New("workspace generation is stale")
	ErrJournalUnknown    = errors.New("publication journal entry is unknown")
)

type RootRegistration struct {
	Folder            history.ID
	Path              string
	Device            uint64
	Inode             uint64
	RegistrationID    [32]byte
	ScanGeneration    uint64
	BootstrapComplete bool
}

func (db *DB) StateDir() string { return db.stateDir }

// CheckRootCandidate is a read-only preflight used before workspace creates
// its marker. RegisterRoot repeats these checks at the durable commit boundary.
func (db *DB) CheckRootCandidate(ctx context.Context, folder history.ID, path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return errors.New("root path must be absolute")
	}
	var existing sql.NullString
	if err := db.db.QueryRowContext(ctx, `SELECT root_path FROM folders WHERE folder_id=?`, folder[:]).Scan(&existing); errors.Is(err, sql.ErrNoRows) {
		return ErrFolderUnknown
	} else if err != nil {
		return err
	}
	if existing.Valid {
		return errors.New("folder already has a registered root")
	}
	rows, err := db.db.QueryContext(ctx, `SELECT root_path FROM folders WHERE root_path IS NOT NULL AND folder_id!=?`, folder[:])
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var other string
		if err := rows.Scan(&other); err != nil {
			return err
		}
		if pathsOverlap(filepath.Clean(path), filepath.Clean(other)) {
			return fmt.Errorf("workspace root overlaps registered root %q", other)
		}
	}
	return rows.Err()
}

func (db *DB) RegisterRoot(ctx context.Context, registration RootRegistration) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if registration.Path == "" || !filepath.IsAbs(registration.Path) || registration.RegistrationID == ([32]byte{}) {
		return errors.New("absolute root path and registration identity are required")
	}
	var existing sql.NullString
	if err := db.db.QueryRowContext(ctx, `SELECT root_path FROM folders WHERE folder_id=?`, registration.Folder[:]).Scan(&existing); errors.Is(err, sql.ErrNoRows) {
		return ErrFolderUnknown
	} else if err != nil {
		return err
	}
	if existing.Valid {
		return errors.New("folder already has a registered root")
	}
	rows, err := db.db.QueryContext(ctx, `SELECT folder_id,root_path FROM folders WHERE root_path IS NOT NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()
	clean := filepath.Clean(registration.Path)
	for rows.Next() {
		var folderRaw []byte
		var other string
		if err := rows.Scan(&folderRaw, &other); err != nil {
			return err
		}
		if string(folderRaw) == string(registration.Folder[:]) {
			continue
		}
		if pathsOverlap(clean, filepath.Clean(other)) {
			return fmt.Errorf("workspace root overlaps registered root %q", other)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	result, err := db.db.ExecContext(ctx, `UPDATE folders SET root_path=?,root_device=?,root_inode=?,registration_id=? WHERE folder_id=?`, clean, int64(registration.Device), int64(registration.Inode), registration.RegistrationID[:], registration.Folder[:])
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return err
		}
		return ErrFolderUnknown
	}
	return nil
}

func pathsOverlap(a, b string) bool {
	rel, err := filepath.Rel(a, b)
	if err == nil && !filepath.IsAbs(rel) && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))) {
		return true
	}
	rel, err = filepath.Rel(b, a)
	return err == nil && !filepath.IsAbs(rel) && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func (db *DB) Root(ctx context.Context, folder history.ID) (RootRegistration, error) {
	var result RootRegistration
	result.Folder = folder
	var device, inode int64
	var registration []byte
	var bootstrap int
	err := db.db.QueryRowContext(ctx, `SELECT root_path,root_device,root_inode,registration_id,scan_generation,bootstrap_complete FROM folders WHERE folder_id=? AND root_path IS NOT NULL`, folder[:]).Scan(&result.Path, &device, &inode, &registration, &result.ScanGeneration, &bootstrap)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrRootNotRegistered
	}
	if err != nil {
		return result, err
	}
	if len(registration) != len(result.RegistrationID) || device < 0 || inode < 0 {
		return result, errors.New("invalid stored root registration")
	}
	copy(result.RegistrationID[:], registration)
	result.Device, result.Inode = uint64(device), uint64(inode)
	result.BootstrapComplete = bootstrap == 1
	return result, nil
}

type Projection struct {
	Folder                history.ID
	Path                  string
	Basis                 []history.VersionID
	Kind                  history.Kind
	Digest                history.Digest
	Executable            bool
	PublicationGeneration uint64
	BlockReason           string
}

func (db *DB) Projection(ctx context.Context, folder history.ID, path string) (Projection, error) {
	return loadProjection(ctx, db.db, folder, path)
}

func (db *DB) Projections(ctx context.Context, folder history.ID) ([]Projection, error) {
	rows, err := db.db.QueryContext(ctx, `SELECT path FROM path_projections WHERE folder_id=? ORDER BY path`, folder[:])
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
	result := make([]Projection, 0, len(paths))
	for _, path := range paths {
		projection, err := loadProjection(ctx, db.db, folder, path)
		if err != nil {
			return nil, err
		}
		result = append(result, projection)
	}
	return result, nil
}

func loadProjection(ctx context.Context, q queryer, folder history.ID, path string) (Projection, error) {
	result := Projection{Folder: folder, Path: path}
	var kind sql.NullInt64
	var digest []byte
	var executable sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT publication_generation,COALESCE(block_reason,''),observed_kind,observed_digest,observed_executable FROM path_projections WHERE folder_id=? AND path=?`, folder[:], path).Scan(&result.PublicationGeneration, &result.BlockReason, &kind, &digest, &executable)
	if err != nil {
		return result, err
	}
	if kind.Valid {
		result.Kind = history.Kind(kind.Int64)
	}
	copy(result.Digest[:], digest)
	result.Executable = executable.Valid && executable.Int64 == 1
	rows, err := q.QueryContext(ctx, `SELECT author_id,counter FROM projection_basis WHERE folder_id=? AND path=? ORDER BY position`, folder[:], path)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var authorRaw, counterRaw []byte
		if err := rows.Scan(&authorRaw, &counterRaw); err != nil {
			return result, err
		}
		var author history.ID
		copy(author[:], authorRaw)
		counter, err := decodeUint(counterRaw)
		if err != nil {
			return result, err
		}
		result.Basis = append(result.Basis, history.VersionID{Folder: folder, Author: author, Counter: counter})
	}
	return result, rows.Err()
}

func setProjectionTx(ctx context.Context, tx *sql.Tx, folder history.ID, path string, basis []history.VersionID, kind history.Kind, manifest *history.Manifest, appliedAuthor, appliedCounter []byte, block string) error {
	var digest any
	var executable any
	if manifest != nil {
		digest = manifest.Digest[:]
		if manifest.Executable {
			executable = 1
		} else {
			executable = 0
		}
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO path_projections(folder_id,path,publication_generation,block_reason,applied_author,applied_counter,observed_kind,observed_digest,observed_executable) VALUES(?,?,1,NULL,?,?,?,?,?) ON CONFLICT(folder_id,path) DO UPDATE SET publication_generation=path_projections.publication_generation+1,block_reason=CASE WHEN path_projections.block_reason='LATE_EDITOR_CANDIDATE' THEN path_projections.block_reason ELSE excluded.block_reason END,applied_author=excluded.applied_author,applied_counter=excluded.applied_counter,observed_kind=excluded.observed_kind,observed_digest=excluded.observed_digest,observed_executable=excluded.observed_executable`, folder[:], path, appliedAuthor, appliedCounter, int(kind), digest, executable)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM projection_basis WHERE folder_id=? AND path=?`, folder[:], path); err != nil {
		return err
	}
	for position, id := range basis {
		if _, err := tx.ExecContext(ctx, `INSERT INTO projection_basis(folder_id,path,position,author_id,counter) VALUES(?,?,?,?,?)`, folder[:], path, position, id.Author[:], encodeUint(id.Counter)); err != nil {
			return err
		}
	}
	if block != "" {
		_, err = tx.ExecContext(ctx, `UPDATE path_projections SET block_reason=? WHERE folder_id=? AND path=?`, block, folder[:], path)
	}
	return err
}

func (db *DB) SetPathBlock(ctx context.Context, folder history.ID, path, reason string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.db.ExecContext(ctx, `INSERT INTO path_projections(folder_id,path,block_reason) VALUES(?,?,?) ON CONFLICT(folder_id,path) DO UPDATE SET block_reason=excluded.block_reason`, folder[:], path, reason)
	return err
}

func (db *DB) Envelope(ctx context.Context, id history.VersionID) (history.Envelope, error) {
	envelope, _, err := db.envelopeAndState(ctx, db.db, id)
	return envelope, err
}

func (db *DB) WriteVersion(ctx context.Context, id history.VersionID, destination io.Writer) (history.Envelope, error) {
	envelope, state, err := db.envelopeAndState(ctx, db.db, id)
	if err != nil {
		return envelope, err
	}
	if state != "ready" {
		return envelope, ErrNotReady
	}
	if envelope.Kind != history.KindFile {
		return envelope, nil
	}
	whole := sha256.New()
	var total uint64
	for _, chunk := range envelope.Manifest.Chunks {
		file, err := os.Open(db.objectPath(chunk.Digest))
		if err != nil {
			return envelope, err
		}
		part := sha256.New()
		written, copyErr := io.Copy(io.MultiWriter(destination, whole, part), io.LimitReader(file, int64(chunk.Length)+1))
		closeErr := file.Close()
		if copyErr != nil {
			return envelope, copyErr
		}
		if closeErr != nil {
			return envelope, closeErr
		}
		if written != int64(chunk.Length) || !equalHash(part, chunk.Digest) {
			return envelope, ErrContentMismatch
		}
		total += uint64(written)
	}
	if total != envelope.Manifest.Size || !equalHash(whole, envelope.Manifest.Digest) {
		return envelope, ErrContentMismatch
	}
	return envelope, nil
}

type Publication struct {
	OperationID  string
	Folder       history.ID
	Path         string
	Intended     history.VersionID
	Kind         history.Kind
	StagePath    string
	RecoveryPath string
	Phase        string
}

func (db *DB) PreparePublication(ctx context.Context, publication Publication) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.db.ExecContext(ctx, `INSERT INTO publication_journal(operation_id,folder_id,path,intended_author,intended_counter,stage_path,recovery_path,phase,intended_kind) VALUES(?,?,?,?,?,?,?,?,?)`, publication.OperationID, publication.Folder[:], publication.Path, publication.Intended.Author[:], encodeUint(publication.Intended.Counter), publication.StagePath, publication.RecoveryPath, "PREPARED", int(publication.Kind))
	if err != nil {
		return err
	}
	return db.callHook(HookPublicationPrepared)
}

func (db *DB) SetPublicationPhase(ctx context.Context, operation, phase string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	result, err := db.db.ExecContext(ctx, `UPDATE publication_journal SET phase=? WHERE operation_id=?`, phase, operation)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return ErrJournalUnknown
	}
	hooks := map[string]string{"STAGED": HookPublicationStaged, "REPLACEMENT_INTENT": HookPublicationIntent, "FILESYSTEM_PUBLISHED": HookPublicationRenamed}
	if hook := hooks[phase]; hook != "" {
		return db.callHook(hook)
	}
	return nil
}

func (db *DB) CommitPublication(ctx context.Context, operation string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var folderRaw, authorRaw, counterRaw []byte
	var path, phase string
	err = tx.QueryRowContext(ctx, `SELECT folder_id,path,intended_author,intended_counter,phase FROM publication_journal WHERE operation_id=?`, operation).Scan(&folderRaw, &path, &authorRaw, &counterRaw, &phase)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrJournalUnknown
	}
	if err != nil {
		return err
	}
	if phase != "FILESYSTEM_PUBLISHED" {
		return errors.New("publication cannot commit before filesystem-published phase")
	}
	var folder, author history.ID
	copy(folder[:], folderRaw)
	copy(author[:], authorRaw)
	counter, err := decodeUint(counterRaw)
	if err != nil {
		return err
	}
	id := history.VersionID{Folder: folder, Author: author, Counter: counter}
	envelope, state, err := db.envelopeAndState(ctx, tx, id)
	if err != nil {
		return err
	}
	if state != "ready" {
		return ErrNotReady
	}
	if err := setProjectionTx(ctx, tx, folder, path, []history.VersionID{id}, envelope.Kind, envelope.Manifest, author[:], encodeUint(counter), ""); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE publication_journal SET phase='COMMITTED' WHERE operation_id=?`, operation); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return db.callHook(HookPublicationCommit)
}

func (db *DB) Publications(ctx context.Context, folder history.ID) ([]Publication, error) {
	rows, err := db.db.QueryContext(ctx, `SELECT operation_id,path,intended_author,intended_counter,COALESCE(intended_kind,0),COALESCE(stage_path,''),COALESCE(recovery_path,''),phase FROM publication_journal WHERE folder_id=? ORDER BY operation_id`, folder[:])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Publication
	for rows.Next() {
		var item Publication
		item.Folder = folder
		var authorRaw, counterRaw []byte
		if err := rows.Scan(&item.OperationID, &item.Path, &authorRaw, &counterRaw, &item.Kind, &item.StagePath, &item.RecoveryPath, &item.Phase); err != nil {
			return nil, err
		}
		copy(item.Intended.Author[:], authorRaw)
		item.Intended.Folder = folder
		item.Intended.Counter, err = decodeUint(counterRaw)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (db *DB) RemovePublication(ctx context.Context, operation string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.db.ExecContext(ctx, `DELETE FROM publication_journal WHERE operation_id=? AND phase='COMMITTED'`, operation)
	return err
}

func (db *DB) MarkScaffold(ctx context.Context, folder history.ID, path string) error {
	_, err := db.db.ExecContext(ctx, `INSERT INTO workspace_scaffolds(folder_id,path,pending) VALUES(?,?,1) ON CONFLICT DO NOTHING`, folder[:], path)
	return err
}

func (db *DB) CompleteScaffold(ctx context.Context, folder history.ID, path string) error {
	_, err := db.db.ExecContext(ctx, `UPDATE workspace_scaffolds SET pending=0 WHERE folder_id=? AND path=?`, folder[:], path)
	return err
}

func (db *DB) RemoveScaffold(ctx context.Context, folder history.ID, path string) error {
	_, err := db.db.ExecContext(ctx, `DELETE FROM workspace_scaffolds WHERE folder_id=? AND path=? AND pending=1`, folder[:], path)
	return err
}

func (db *DB) RemoveAnyScaffold(ctx context.Context, folder history.ID, path string) error {
	_, err := db.db.ExecContext(ctx, `DELETE FROM workspace_scaffolds WHERE folder_id=? AND path=?`, folder[:], path)
	return err
}

func (db *DB) PendingScaffolds(ctx context.Context, folder history.ID) ([]string, error) {
	rows, err := db.db.QueryContext(ctx, `SELECT path FROM workspace_scaffolds WHERE folder_id=? AND pending=1 ORDER BY path`, folder[:])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}

func (db *DB) Scaffolds(ctx context.Context, folder history.ID) (map[string]bool, error) {
	rows, err := db.db.QueryContext(ctx, `SELECT path FROM workspace_scaffolds WHERE folder_id=?`, folder[:])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]bool{}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		result[path] = true
	}
	return result, rows.Err()
}

func (db *DB) BeginScan(ctx context.Context, folder history.ID) (uint64, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if _, err := db.db.ExecContext(ctx, `UPDATE folders SET scan_generation=scan_generation+1 WHERE folder_id=?`, folder[:]); err != nil {
		return 0, err
	}
	var generation uint64
	if err := db.db.QueryRowContext(ctx, `SELECT scan_generation FROM folders WHERE folder_id=?`, folder[:]).Scan(&generation); err != nil {
		return 0, err
	}
	_, err := db.db.ExecContext(ctx, `DELETE FROM deletion_proposals WHERE folder_id=?`, folder[:])
	return generation, err
}

func (db *DB) SaveDeletionProposal(ctx context.Context, folder history.ID, generation uint64, token string, paths []string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current uint64
	if err := tx.QueryRowContext(ctx, `SELECT scan_generation FROM folders WHERE folder_id=?`, folder[:]).Scan(&current); err != nil {
		return err
	}
	if current != generation {
		return ErrStaleGeneration
	}
	for _, path := range paths {
		if _, err := tx.ExecContext(ctx, `INSERT INTO deletion_proposals(folder_id,token,generation,path) VALUES(?,?,?,?)`, folder[:], token, int64(generation), path); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) DeletionProposal(ctx context.Context, folder history.ID, token string) (uint64, []string, error) {
	var current uint64
	if err := db.db.QueryRowContext(ctx, `SELECT scan_generation FROM folders WHERE folder_id=?`, folder[:]).Scan(&current); err != nil {
		return 0, nil, err
	}
	rows, err := db.db.QueryContext(ctx, `SELECT generation,path FROM deletion_proposals WHERE folder_id=? AND token=? ORDER BY path`, folder[:], token)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	var generation uint64
	var paths []string
	for rows.Next() {
		var itemGeneration uint64
		var path string
		if err := rows.Scan(&itemGeneration, &path); err != nil {
			return 0, nil, err
		}
		if generation == 0 {
			generation = itemGeneration
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		return 0, nil, err
	}
	if len(paths) == 0 || generation != current {
		return 0, nil, ErrStaleGeneration
	}
	return generation, paths, nil
}

func (db *DB) FinishDeletionProposal(ctx context.Context, folder history.ID, token string, bootstrapComplete bool) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM deletion_proposals WHERE folder_id=? AND token=?`, folder[:], token); err != nil {
		return err
	}
	if bootstrapComplete {
		if _, err := tx.ExecContext(ctx, `UPDATE folders SET bootstrap_complete=1 WHERE folder_id=?`, folder[:]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) MarkBootstrapComplete(ctx context.Context, folder history.ID) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.db.ExecContext(ctx, `UPDATE folders SET bootstrap_complete=1 WHERE folder_id=?`, folder[:])
	return err
}

func (db *DB) AbortPublication(ctx context.Context, operation string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.db.ExecContext(ctx, `DELETE FROM publication_journal WHERE operation_id=? AND phase!='COMMITTED'`, operation)
	return err
}

func SortVersionIDs(ids []history.VersionID) {
	sort.Slice(ids, func(i, j int) bool { return history.CompareVersionID(ids[i], ids[j]) < 0 })
}
