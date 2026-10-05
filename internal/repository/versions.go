package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
)

var (
	ErrFolderUnknown = errors.New("shared folder is not registered")
	ErrNotReady      = errors.New("version content is not durably ready")
)

func encodeUint(value uint64) []byte {
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], value)
	return raw[:]
}
func decodeUint(raw []byte) (uint64, error) {
	if len(raw) != 8 {
		return 0, errors.New("invalid stored uint64")
	}
	return binary.BigEndian.Uint64(raw), nil
}

func (db *DB) EnsureFolder(ctx context.Context, folder, localAuthor history.ID, membershipRevision uint64) error {
	if folder == (history.ID{}) || localAuthor == (history.ID{}) || membershipRevision == 0 {
		return errors.New("folder, author and membership revision must be nonzero")
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.db.ExecContext(ctx, `INSERT INTO folders(folder_id,local_author,next_counter,membership_revision) VALUES(?,?,?,?) ON CONFLICT(folder_id) DO NOTHING`, folder[:], localAuthor[:], encodeUint(0), encodeUint(membershipRevision))
	if err != nil {
		return err
	}
	var got []byte
	if err := db.db.QueryRowContext(ctx, `SELECT local_author FROM folders WHERE folder_id=?`, folder[:]).Scan(&got); err != nil {
		return err
	}
	if !bytes.Equal(got, localAuthor[:]) {
		return errors.New("folder already has a different local author")
	}
	return nil
}

type LocalVersionRequest struct {
	Folder           history.ID
	Path             string
	Basis            []history.VersionID
	Kind             history.Kind
	Manifest         *history.Manifest
	AuthoredRevision uint64
	DisplayTime      string
	SkipProjection   bool
}

// CreateLocalVersion allocates the folder-wide author counter and commits the
// envelope, manifest, object references and readiness in one transaction.
func (db *DB) CreateLocalVersion(ctx context.Context, request LocalVersionRequest) (history.Envelope, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if err := db.checkMetadataBudget(ctx); err != nil {
		return history.Envelope{}, err
	}
	if err := db.checkFreeSpaceReserve(ctx); err != nil {
		return history.Envelope{}, err
	}
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return history.Envelope{}, err
	}
	defer tx.Rollback()
	var authorRaw, counterRaw, revRaw []byte
	if err := tx.QueryRowContext(ctx, `SELECT local_author,next_counter,membership_revision FROM folders WHERE folder_id=?`, request.Folder[:]).Scan(&authorRaw, &counterRaw, &revRaw); errors.Is(err, sql.ErrNoRows) {
		return history.Envelope{}, ErrFolderUnknown
	} else if err != nil {
		return history.Envelope{}, err
	}
	var author history.ID
	if len(authorRaw) != len(author) {
		return history.Envelope{}, errors.New("invalid stored author")
	}
	copy(author[:], authorRaw)
	counter, err := decodeUint(counterRaw)
	if err != nil {
		return history.Envelope{}, err
	}
	if counter == math.MaxUint64 {
		return history.Envelope{}, history.ErrCounterOverflow
	}
	counter++
	// Folder-wide identity checks remain global even when causal state is per path.
	if err := rejectExistingVersionID(ctx, tx, history.VersionID{Folder: request.Folder, Author: author, Counter: counter}); err != nil {
		return history.Envelope{}, err
	}
	authoredRevision := request.AuthoredRevision
	if authoredRevision == 0 {
		authoredRevision, _ = decodeUint(revRaw)
	}
	var authorState string
	err = tx.QueryRowContext(ctx, `SELECT state FROM membership_entries WHERE folder_id=? AND revision=? AND device_id=?`, request.Folder[:], revRaw, author[:]).Scan(&authorState)
	if err == nil && authorState != "active" {
		return history.Envelope{}, errors.New("local author is retired or not active in this folder")
	}
	h, err := loadHistory(ctx, tx, request.Folder, request.Path)
	if err != nil {
		return history.Envelope{}, err
	}
	basis := request.Basis
	if len(basis) == 0 {
		proj, projErr := loadProjection(ctx, tx, request.Folder, request.Path)
		if projErr == nil && len(proj.Basis) > 0 {
			basis = proj.Basis
		} else {
			heads := h.Heads(request.Folder, request.Path)
			for _, head := range heads {
				basis = append(basis, head.ID)
			}
		}
	}
	parents, vector, err := h.PlanOrdinaryCapture(request.Folder, author, request.Path, basis, counter)
	if err != nil {
		return history.Envelope{}, err
	}
	envelope := history.Envelope{ID: history.VersionID{Folder: request.Folder, Author: author, Counter: counter}, Path: request.Path, Parents: parents, Vector: vector, Kind: request.Kind, Manifest: cloneManifest(request.Manifest), AuthoredRevision: authoredRevision, DisplayTime: request.DisplayTime}
	if err := h.Accept(envelope); err != nil {
		return history.Envelope{}, err
	}
	if envelope.Kind == history.KindFile {
		if err := db.VerifyManifest(envelope.Manifest); err != nil {
			return history.Envelope{}, err
		}
		if err := ensureManifestObjects(ctx, tx, envelope.Manifest); err != nil {
			return history.Envelope{}, err
		}
	}
	if err := insertEnvelope(ctx, tx, envelope, "ready"); err != nil {
		return history.Envelope{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE folders SET next_counter=? WHERE folder_id=? AND next_counter=?`, encodeUint(counter), request.Folder[:], counterRaw)
	if err != nil {
		return history.Envelope{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return history.Envelope{}, errors.New("local counter changed concurrently")
	}
	if err := addObjectReferences(ctx, tx, envelope); err != nil {
		return history.Envelope{}, err
	}
	if !request.SkipProjection {
		if err := setProjectionTx(ctx, tx, envelope.ID.Folder, envelope.Path, []history.VersionID{envelope.ID}, envelope.Kind, envelope.Manifest, author[:], encodeUint(counter), ""); err != nil {
			return history.Envelope{}, err
		}
	}
	if err := db.callHook(HookBeforeVersionCommit); err != nil {
		return history.Envelope{}, err
	}
	if err := tx.Commit(); err != nil {
		return history.Envelope{}, fmt.Errorf("commit local version: %w", err)
	}
	if err := db.callHook(HookAfterVersionCommit); err != nil {
		return history.Envelope{}, err
	}
	return envelope, nil
}

// ImportMetadata admits a validated immutable envelope. File metadata remains
// pending even when some chunks happen to exist until MarkContentReady verifies
// the complete ordered file.
func (db *DB) ImportMetadata(ctx context.Context, envelope history.Envelope) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if err := db.checkMetadataBudget(ctx); err != nil {
		return err
	}
	if err := db.checkFreeSpaceReserve(ctx); err != nil {
		return err
	}
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Check immutable identity across every path before loading path-local
	// causal state; narrowing history must never permit moving an existing ID.
	existing, _, existingErr := db.envelopeAndState(ctx, tx, envelope.ID)
	if existingErr == nil {
		if !sameEnvelope(existing, envelope) {
			return history.ErrDuplicateID
		}
		return nil
	}
	if !errors.Is(existingErr, sql.ErrNoRows) {
		return existingErr
	}
	h, err := loadHistory(ctx, tx, envelope.ID.Folder, envelope.Path)
	if errors.Is(err, ErrFolderUnknown) {
		return err
	}
	if err != nil {
		return err
	}
	if existing, ok := h.Envelope(envelope.ID); ok {
		if !sameEnvelope(existing, envelope) {
			return history.ErrDuplicateID
		}
		return nil
	}
	var revRaw []byte
	if err := tx.QueryRowContext(ctx, `SELECT membership_revision FROM folders WHERE folder_id=?`, envelope.ID.Folder[:]).Scan(&revRaw); err != nil {
		return err
	}
	var memberState string
	err = tx.QueryRowContext(ctx, `SELECT state FROM membership_entries WHERE folder_id=? AND revision=? AND device_id=?`, envelope.ID.Folder[:], revRaw, envelope.ID.Author[:]).Scan(&memberState)
	if errors.Is(err, sql.ErrNoRows) {
		var activeCount int
		_ = tx.QueryRowContext(ctx, `SELECT count(*) FROM membership_entries WHERE folder_id=? AND revision=?`, envelope.ID.Folder[:], revRaw).Scan(&activeCount)
		if activeCount > 0 {
			return ErrUnauthorized
		}
	} else if err != nil {
		return err
	} else if memberState == "retired" {
		fingerprint := EnvelopeDigest(envelope)
		var storedDigest []byte
		err := tx.QueryRowContext(ctx, `SELECT envelope_digest FROM retirement_snapshot_entries WHERE folder_id=? AND revision=? AND retired_device=? AND counter=?`,
			envelope.ID.Folder[:], revRaw, envelope.ID.Author[:], encodeUint(envelope.ID.Counter)).Scan(&storedDigest)
		if errors.Is(err, sql.ErrNoRows) || !bytes.Equal(storedDigest, fingerprint[:]) {
			return ErrRetiredAuthorVersionRejected
		} else if err != nil {
			return err
		}
	}
	if err := h.Accept(envelope); err != nil {
		return err
	}
	state := "ready"
	if envelope.Kind == history.KindFile {
		state = "pending"
	}
	if err := insertEnvelope(ctx, tx, envelope, state); err != nil {
		return err
	}
	if err := db.callHook(HookBeforeVersionCommit); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return db.callHook(HookAfterVersionCommit)
}

func (db *DB) MarkContentReady(ctx context.Context, id history.VersionID) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	envelope, state, err := db.envelopeAndState(ctx, db.db, id)
	if err != nil {
		return err
	}
	if state == "ready" {
		return nil
	}
	if envelope.Kind != history.KindFile {
		return errors.New("non-file metadata is already ready")
	}
	if err := db.VerifyManifest(envelope.Manifest); err != nil {
		return err
	}
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := ensureManifestObjects(ctx, tx, envelope.Manifest); err != nil {
		return err
	}
	if err := addObjectReferences(ctx, tx, envelope); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE versions SET content_state='ready' WHERE folder_id=? AND author_id=? AND counter=? AND content_state='pending'`, id.Folder[:], id.Author[:], encodeUint(id.Counter)); err != nil {
		return err
	}
	if err := db.callHook(HookBeforeReadyCommit); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return db.callHook(HookAfterReadyCommit)
}

func (db *DB) VersionsByAuthor(ctx context.Context, folder, author history.ID) ([]protocol.RetiredVersion, error) {
	rows, err := db.db.QueryContext(ctx, `SELECT counter, envelope_digest FROM versions WHERE folder_id=? AND author_id=? ORDER BY counter`, folder[:], author[:])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []protocol.RetiredVersion
	for rows.Next() {
		var counterRaw, digestRaw []byte
		if err := rows.Scan(&counterRaw, &digestRaw); err != nil {
			return nil, err
		}
		counter, err := decodeUint(counterRaw)
		if err != nil {
			return nil, err
		}
		var d history.Digest
		copy(d[:], digestRaw)
		list = append(list, protocol.RetiredVersion{Counter: counter, EnvelopeDigest: d})
	}
	return list, rows.Err()
}

func (db *DB) MetadataKnown(ctx context.Context, id history.VersionID) (bool, error) {
	var one int
	err := db.db.QueryRowContext(ctx, `SELECT 1 FROM versions WHERE folder_id=? AND author_id=? AND counter=?`, id.Folder[:], id.Author[:], encodeUint(id.Counter)).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
func (db *DB) ContentReady(ctx context.Context, id history.VersionID) (bool, error) {
	var state string
	err := db.db.QueryRowContext(ctx, `SELECT content_state FROM versions WHERE folder_id=? AND author_id=? AND counter=?`, id.Folder[:], id.Author[:], encodeUint(id.Counter)).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return state == "ready", err
}
func (db *DB) CanIssueDurableReceipt(ctx context.Context, id history.VersionID) (bool, error) {
	ready, err := db.ContentReady(ctx, id)
	if err != nil || !ready {
		return false, err
	}
	var quarantinedCount int
	err = db.db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM manifest_chunks mc
		JOIN quarantined_chunks qc ON qc.digest=mc.digest AND qc.repaired=0
		WHERE mc.folder_id=? AND mc.author_id=? AND mc.counter=?
	`, id.Folder[:], id.Author[:], encodeUint(id.Counter)).Scan(&quarantinedCount)
	if err != nil {
		return false, err
	}
	if quarantinedCount > 0 {
		return false, nil
	}
	return true, nil
}

// VerifyVersionContent rehashes every object and the ordered whole file. It is
// the recovery/integrity assertion used before content-consuming operations.
func (db *DB) VerifyVersionContent(ctx context.Context, id history.VersionID) error {
	envelope, state, err := db.envelopeAndState(ctx, db.db, id)
	if err != nil {
		return err
	}
	if state != "ready" {
		return ErrNotReady
	}
	if envelope.Kind != history.KindFile {
		return nil
	}
	if err := db.VerifyManifest(envelope.Manifest); err != nil {
		if errors.Is(err, ErrContentMismatch) && envelope.Manifest != nil {
			for _, ch := range envelope.Manifest.Chunks {
				if db.verifyObjectFile(ch.Digest, ch.Length) != nil {
					_, _ = db.QuarantineChunk(ctx, ch.Digest, "corrupt manifest chunk")
				}
			}
		}
		return err
	}
	return nil
}

func (db *DB) WorkingApplied(ctx context.Context, id history.VersionID) (bool, error) {
	var author, counter []byte
	err := db.db.QueryRowContext(ctx, `SELECT applied_author,applied_counter FROM path_projections p JOIN versions v ON v.folder_id=p.folder_id AND v.path=p.path WHERE v.folder_id=? AND v.author_id=? AND v.counter=?`, id.Folder[:], id.Author[:], encodeUint(id.Counter)).Scan(&author, &counter)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return bytes.Equal(author, id.Author[:]) && bytes.Equal(counter, encodeUint(id.Counter)), err
}

func ensureManifestObjects(ctx context.Context, tx *sql.Tx, manifest *history.Manifest) error {
	for _, chunk := range manifest.Chunks {
		var lengthRaw []byte
		var verified int
		err := tx.QueryRowContext(ctx, `SELECT length,verified FROM objects WHERE digest=?`, chunk.Digest[:]).Scan(&lengthRaw, &verified)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrContentMissing
		}
		if err != nil {
			return err
		}
		length, e := decodeUint(lengthRaw)
		if e != nil {
			return e
		}
		if length != chunk.Length || verified != 1 {
			return ErrContentMismatch
		}
	}
	return nil
}
func addObjectReferences(ctx context.Context, tx *sql.Tx, envelope history.Envelope) error {
	if envelope.Manifest == nil {
		return nil
	}
	for i, chunk := range envelope.Manifest.Chunks {
		if _, err := tx.ExecContext(ctx, `INSERT INTO object_references(folder_id,author_id,counter,position,digest) VALUES(?,?,?,?,?) ON CONFLICT DO NOTHING`, envelope.ID.Folder[:], envelope.ID.Author[:], encodeUint(envelope.ID.Counter), i, chunk.Digest[:]); err != nil {
			return err
		}
	}
	return nil
}

func insertEnvelope(ctx context.Context, tx *sql.Tx, e history.Envelope, state string) error {
	var size, digest any
	var executable any
	if e.Manifest != nil {
		size = encodeUint(e.Manifest.Size)
		digest = e.Manifest.Digest[:]
		if e.Manifest.Executable {
			executable = 1
		} else {
			executable = 0
		}
	}
	fingerprint := envelopeDigest(e)
	if _, err := tx.ExecContext(ctx, `INSERT INTO versions(folder_id,author_id,counter,path,kind,authored_revision,display_time,file_size,file_digest,executable,content_state,acquired_ns,envelope_digest) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, e.ID.Folder[:], e.ID.Author[:], encodeUint(e.ID.Counter), e.Path, int(e.Kind), encodeUint(e.AuthoredRevision), e.DisplayTime, size, digest, executable, state, time.Now().UnixNano(), fingerprint[:]); err != nil {
		return err
	}
	for i, p := range e.Parents {
		if _, err := tx.ExecContext(ctx, `INSERT INTO version_parents VALUES(?,?,?,?,?,?)`, e.ID.Folder[:], e.ID.Author[:], encodeUint(e.ID.Counter), i, p.Author[:], encodeUint(p.Counter)); err != nil {
			return err
		}
	}
	for i, v := range e.Vector {
		if _, err := tx.ExecContext(ctx, `INSERT INTO version_vectors VALUES(?,?,?,?,?,?)`, e.ID.Folder[:], e.ID.Author[:], encodeUint(e.ID.Counter), i, v.Author[:], encodeUint(v.Counter)); err != nil {
			return err
		}
	}
	if e.Manifest != nil {
		for i, c := range e.Manifest.Chunks {
			if _, err := tx.ExecContext(ctx, `INSERT INTO manifest_chunks VALUES(?,?,?,?,?,?)`, e.ID.Folder[:], e.ID.Author[:], encodeUint(e.ID.Counter), i, c.Digest[:], encodeUint(c.Length)); err != nil {
				return err
			}
		}
	}
	return nil
}

// SetAcquiredTime updates the acquired_ns timestamp for testing and retention reconciliation.
func (db *DB) SetAcquiredTime(ctx context.Context, id history.VersionID, t time.Time) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	_, err := db.db.ExecContext(ctx, `UPDATE versions SET acquired_ns=? WHERE folder_id=? AND author_id=? AND counter=?`,
		t.UnixNano(), id.Folder[:], id.Author[:], encodeUint(id.Counter))
	return err
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

const (
	envelopeLookupSQL  = `SELECT path,kind,authored_revision,display_time,file_size,file_digest,executable,content_state FROM versions WHERE folder_id=? AND author_id=? AND counter=?`
	envelopeParentsSQL = `SELECT parent_author,parent_counter FROM version_parents WHERE folder_id=? AND author_id=? AND counter=? ORDER BY position`
	envelopeVectorsSQL = `SELECT vector_author,vector_counter FROM version_vectors WHERE folder_id=? AND author_id=? AND counter=? ORDER BY position`
	envelopeChunksSQL  = `SELECT digest,length FROM manifest_chunks WHERE folder_id=? AND author_id=? AND counter=? ORDER BY position`
)

// A history read reuses four statement plans for its immutable envelopes.
// Scope plans to this read/transaction; never cache observations across queries.
type preparedHistoryQueries struct {
	queryer
	statements map[string]*sql.Stmt
}

func (p preparedHistoryQueries) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if stmt := p.statements[query]; stmt != nil {
		return stmt.QueryRowContext(ctx, args...)
	}
	return p.queryer.QueryRowContext(ctx, query, args...)
}
func (p preparedHistoryQueries) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if stmt := p.statements[query]; stmt != nil {
		return stmt.QueryContext(ctx, args...)
	}
	return p.queryer.QueryContext(ctx, query, args...)
}
func prepareHistoryQueries(ctx context.Context, q queryer) (queryer, func(), error) {
	preparer, ok := q.(interface {
		PrepareContext(context.Context, string) (*sql.Stmt, error)
	})
	if !ok {
		return q, func() {}, nil
	}
	p := preparedHistoryQueries{queryer: q, statements: make(map[string]*sql.Stmt, 4)}
	close := func() {
		for _, stmt := range p.statements {
			_ = stmt.Close()
		}
	}
	for _, query := range []string{envelopeLookupSQL, envelopeParentsSQL, envelopeVectorsSQL, envelopeChunksSQL} {
		stmt, err := preparer.PrepareContext(ctx, query)
		if err != nil {
			close()
			return nil, nil, err
		}
		p.statements[query] = stmt
	}
	return p, close, nil
}

func (db *DB) envelopeAndState(ctx context.Context, q queryer, id history.VersionID) (history.Envelope, string, error) {
	var e history.Envelope
	e.ID = id
	var kind int
	var revision, size, digest []byte
	var display, state string
	var executable sql.NullInt64
	err := q.QueryRowContext(ctx, envelopeLookupSQL, id.Folder[:], id.Author[:], encodeUint(id.Counter)).Scan(&e.Path, &kind, &revision, &display, &size, &digest, &executable, &state)
	if err != nil {
		return e, "", err
	}
	e.Kind = history.Kind(kind)
	e.AuthoredRevision, _ = decodeUint(revision)
	e.DisplayTime = display
	if e.Kind == history.KindFile {
		e.Manifest = &history.Manifest{Executable: executable.Int64 == 1}
		e.Manifest.Size, _ = decodeUint(size)
		copy(e.Manifest.Digest[:], digest)
	}
	parents, err := q.QueryContext(ctx, envelopeParentsSQL, id.Folder[:], id.Author[:], encodeUint(id.Counter))
	if err != nil {
		return e, "", err
	}
	for parents.Next() {
		var a, c []byte
		if err := parents.Scan(&a, &c); err != nil {
			parents.Close()
			return e, "", err
		}
		var author history.ID
		copy(author[:], a)
		counter, _ := decodeUint(c)
		e.Parents = append(e.Parents, history.VersionID{Folder: id.Folder, Author: author, Counter: counter})
	}
	parents.Close()
	vectors, err := q.QueryContext(ctx, envelopeVectorsSQL, id.Folder[:], id.Author[:], encodeUint(id.Counter))
	if err != nil {
		return e, "", err
	}
	for vectors.Next() {
		var a, c []byte
		if err := vectors.Scan(&a, &c); err != nil {
			vectors.Close()
			return e, "", err
		}
		var author history.ID
		copy(author[:], a)
		counter, _ := decodeUint(c)
		e.Vector = append(e.Vector, history.ClockEntry{Author: author, Counter: counter})
	}
	vectors.Close()
	if e.Manifest != nil {
		chunks, err := q.QueryContext(ctx, envelopeChunksSQL, id.Folder[:], id.Author[:], encodeUint(id.Counter))
		if err != nil {
			return e, "", err
		}
		for chunks.Next() {
			var d, l []byte
			if err := chunks.Scan(&d, &l); err != nil {
				chunks.Close()
				return e, "", err
			}
			var digest history.Digest
			copy(digest[:], d)
			length, _ := decodeUint(l)
			e.Manifest.Chunks = append(e.Manifest.Chunks, history.Chunk{Digest: digest, Length: length})
		}
		chunks.Close()
	}
	return e, state, nil
}

func rejectExistingVersionID(ctx context.Context, q queryer, id history.VersionID) error {
	var present int
	err := q.QueryRowContext(ctx, `SELECT 1 FROM versions WHERE folder_id=? AND author_id=? AND counter=?`, id.Folder[:], id.Author[:], encodeUint(id.Counter)).Scan(&present)
	if err == nil {
		return history.ErrDuplicateID
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

func loadHistory(ctx context.Context, tx *sql.Tx, folder history.ID, paths ...string) (*history.History, error) {
	var present int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM folders WHERE folder_id=?`, folder[:]).Scan(&present); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrFolderUnknown
	} else if err != nil {
		return nil, err
	}
	return loadHistoryQuery(ctx, tx, folder, paths...)
}

// Causality and ordinary capture are per path; structural/GC callers request
// full-folder history. The existing versions_path index bounds path queries.
func loadHistoryQuery(ctx context.Context, q queryer, folder history.ID, paths ...string) (*history.History, error) {
	query := `SELECT author_id,counter FROM versions WHERE folder_id=?`
	args := []any{folder[:]}
	if len(paths) > 0 {
		query += ` AND path=?`
		args = append(args, paths[0])
	}
	rows, err := q.QueryContext(ctx, query+` ORDER BY rowid`, args...)
	if err != nil {
		return nil, err
	}
	var ids []history.VersionID
	for rows.Next() {
		var a, c []byte
		if err := rows.Scan(&a, &c); err != nil {
			rows.Close()
			return nil, err
		}
		var author history.ID
		copy(author[:], a)
		counter, err := decodeUint(c)
		if err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, history.VersionID{Folder: folder, Author: author, Counter: counter})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	prepared := q
	if len(ids) > 1 {
		var close func()
		prepared, close, err = prepareHistoryQueries(ctx, q)
		if err != nil {
			return nil, err
		}
		defer close()
	}
	h := history.New()
	for _, id := range ids {
		e, _, err := (&DB{}).envelopeAndState(ctx, prepared, id)
		if err != nil {
			return nil, err
		}
		if err := h.Accept(e); err != nil {
			return nil, fmt.Errorf("stored history is invalid: %w", err)
		}
	}
	return h, nil
}

func cloneManifest(m *history.Manifest) *history.Manifest {
	if m == nil {
		return nil
	}
	copyM := *m
	copyM.Chunks = append([]history.Chunk(nil), m.Chunks...)
	return &copyM
}

func sameEnvelope(a, b history.Envelope) bool {
	if a.ID != b.ID || a.Path != b.Path || a.Kind != b.Kind || a.AuthoredRevision != b.AuthoredRevision || a.DisplayTime != b.DisplayTime || len(a.Parents) != len(b.Parents) || len(a.Vector) != len(b.Vector) || (a.Manifest == nil) != (b.Manifest == nil) {
		return false
	}
	for i := range a.Parents {
		if a.Parents[i] != b.Parents[i] {
			return false
		}
	}
	for i := range a.Vector {
		if a.Vector[i] != b.Vector[i] {
			return false
		}
	}
	if a.Manifest == nil {
		return true
	}
	if a.Manifest.Size != b.Manifest.Size || a.Manifest.Digest != b.Manifest.Digest || a.Manifest.Executable != b.Manifest.Executable || len(a.Manifest.Chunks) != len(b.Manifest.Chunks) {
		return false
	}
	for i := range a.Manifest.Chunks {
		if a.Manifest.Chunks[i] != b.Manifest.Chunks[i] {
			return false
		}
	}
	return true
}
func envelopeDigest(e history.Envelope) history.Digest {
	h := sha256.New()
	h.Write([]byte("filesync-envelope-db-v1\x00"))
	h.Write(e.ID.Folder[:])
	h.Write(e.ID.Author[:])
	h.Write(encodeUint(e.ID.Counter))
	h.Write([]byte(e.Path))
	h.Write([]byte{byte(e.Kind)})
	h.Write(encodeUint(e.AuthoredRevision))
	h.Write([]byte(e.DisplayTime))
	for _, p := range e.Parents {
		h.Write(p.Folder[:])
		h.Write(p.Author[:])
		h.Write(encodeUint(p.Counter))
	}
	for _, v := range e.Vector {
		h.Write(v.Author[:])
		h.Write(encodeUint(v.Counter))
	}
	if e.Manifest != nil {
		h.Write(encodeUint(e.Manifest.Size))
		h.Write(e.Manifest.Digest[:])
		for _, c := range e.Manifest.Chunks {
			h.Write(c.Digest[:])
			h.Write(encodeUint(c.Length))
		}
		if e.Manifest.Executable {
			h.Write([]byte{1})
		} else {
			h.Write([]byte{0})
		}
	}
	var result history.Digest
	copy(result[:], h.Sum(nil))
	return result
}
