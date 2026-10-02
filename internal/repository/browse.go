package repository

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/calebhabesh/file-sync/internal/history"
)

var (
	ErrStaleCursor          = errors.New("cursor snapshot generation is stale")
	ErrInvalidCursor        = errors.New("cursor format is invalid")
	ErrInvalidDirectoryPath = errors.New("invalid directory path")
	ErrDirectoryNotFound    = errors.New("directory not found")
	ErrPathNotFound         = errors.New("path not found")
	ErrNotAFile             = errors.New("requested path is not a regular file")
)

// BrowseItem represents a direct child entry within a workspace directory.
type BrowseItem struct {
	WorkingState       string       `json:"working_state"`
	Path               string       `json:"path"`
	Name               string       `json:"name"`
	IsDir              bool         `json:"is_dir"`
	Kind               history.Kind `json:"kind"`
	Size               uint64       `json:"size"`
	MtimeNS            int64        `json:"mtime_ns"`
	Inode              uint64       `json:"inode"`
	Executable         bool         `json:"executable"`
	BlockReason        string       `json:"block_reason,omitempty"`
	ContentState       string       `json:"content_state,omitempty"` // ready, pending, unavailable, corrupt
	HasConflict        bool         `json:"has_conflict,omitempty"`
	StructuralConflict string       `json:"structural_conflict,omitempty"`
}

// BrowseOptions specifies directory query and pagination options.
type BrowseOptions struct {
	DirPath            string `json:"dir_path"`
	SortBy             string `json:"sort_by,omitempty"`  // "name", "size", "mtime", "kind" (default: name)
	SortDir            string `json:"sort_dir,omitempty"` // "asc", "desc" (default: asc)
	Limit              int    `json:"limit,omitempty"`    // default: 50, max: 200
	Cursor             string `json:"cursor,omitempty"`
	SnapshotGeneration int64  `json:"snapshot_generation,omitempty"`
}

// BrowseResult contains immediate child items and pagination state.
type BrowseResult struct {
	DirPath            string       `json:"dir_path"`
	ParentPath         string       `json:"parent_path"`
	Items              []BrowseItem `json:"items"`
	NextCursor         string       `json:"next_cursor,omitempty"`
	TotalItems         int          `json:"total_items"`
	SnapshotGeneration int64        `json:"snapshot_generation"`
}

type cursorPayload struct {
	Gen     int64  `json:"gen"`
	DirPath string `json:"dir_path"`
	Offset  int    `json:"offset"`
	SortBy  string `json:"sort_by"`
	SortDir string `json:"sort_dir"`
	Limit   int    `json:"limit"`
}

func encodeCursor(gen int64, dirPath string, offset int, sortBy, sortDir string, limit int) string {
	p := cursorPayload{
		Gen:     gen,
		DirPath: dirPath,
		Offset:  offset,
		SortBy:  sortBy,
		SortDir: sortDir,
		Limit:   limit,
	}
	data, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeCursor(cur string) (*cursorPayload, error) {
	if len(cur) > 8192 {
		return nil, ErrInvalidCursor
	}
	data, err := base64.RawURLEncoding.DecodeString(cur)
	if err != nil {
		return nil, ErrInvalidCursor
	}
	var p cursorPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, ErrInvalidCursor
	}
	return &p, nil
}

// queryKnown uses folder indexes and SQL pagination: only the requested page is
// materialized in Go. Substring search scans locally known paths within a folder;
// it is not a full-text index or a remote workspace census.
const knownPathsSQL = `WITH RECURSIVE
heads AS (
 SELECT v.* FROM versions v WHERE folder_id=?
 AND NOT EXISTS (SELECT 1 FROM version_parents p WHERE p.folder_id=v.folder_id AND p.parent_author=v.author_id AND p.parent_counter=v.counter)
),
raw(path,kind,sizehex,mtime,inode,executable,block,state) AS (
 SELECT path,kind,COALESCE(hex(file_size),'0000000000000000'),0,0,COALESCE(executable,0),'',content_state FROM heads WHERE kind!=3
 UNION ALL
 SELECT path,COALESCE(observed_kind,1),printf('%016X',COALESCE(observed_size,0)),COALESCE(observed_mtime_ns,0),COALESCE(observed_inode,0),COALESCE(observed_executable,0),COALESCE(block_reason,''),'unknown'
 FROM path_projections WHERE folder_id=? AND observed_kind!=3
 AND (NOT EXISTS(SELECT 1 FROM heads WHERE heads.path=path_projections.path) OR EXISTS(SELECT 1 FROM heads WHERE heads.path=path_projections.path AND kind!=3))
 UNION ALL SELECT path,2,'0000000000000000',0,0,0,'','unknown' FROM workspace_scaffolds WHERE folder_id=?
),
ancestors(path,rest) AS (
 SELECT '',path FROM raw WHERE instr(path,'/')>0
 UNION
 SELECT CASE WHEN path='' THEN substr(rest,1,instr(rest,'/')-1) ELSE path||'/'||substr(rest,1,instr(rest,'/')-1) END,substr(rest,instr(rest,'/')+1) FROM ancestors WHERE instr(rest,'/')>0
),
entries AS (
 SELECT path,MAX(kind=2) AS isdir,MAX(sizehex) AS sizehex,MAX(mtime) AS mtime,MAX(inode) AS inode,MAX(executable) AS executable,MAX(block) AS block,
 CASE WHEN MAX(state='unavailable') THEN 'unavailable' WHEN MAX(state='pending') THEN 'pending' WHEN MAX(state='ready') THEN 'ready' ELSE 'unknown' END AS state
 FROM (SELECT * FROM raw UNION ALL SELECT path,2,'0000000000000000',0,0,0,'','unknown' FROM ancestors WHERE path!='') GROUP BY path
)
`

func (db *DB) browseGeneration(ctx context.Context, folder history.ID) (int64, error) {
	var generation int64
	err := db.db.QueryRowContext(ctx, `SELECT generation FROM browse_generation WHERE id=1 AND EXISTS(SELECT 1 FROM folders WHERE folder_id=?)`, folder[:]).Scan(&generation)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrFolderUnknown
	}
	return generation, err
}

func validateBrowsePath(path string, root bool) error {
	if root && path == "" {
		return nil
	}
	if err := history.ValidatePath(path); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidDirectoryPath, err)
	}
	return nil
}

func pageLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func (db *DB) queryKnown(ctx context.Context, folder history.ID, where, order string, args []any, limit, offset int) ([]BrowseItem, int, error) {
	baseArgs := []any{folder[:], folder[:], folder[:]}
	var total int
	if err := db.db.QueryRowContext(ctx, knownPathsSQL+`SELECT COUNT(*) FROM entries WHERE `+where, append(baseArgs, args...)...).Scan(&total); err != nil {
		return nil, 0, err
	}
	params := append(append([]any{}, baseArgs...), folder[:])
	params = append(params, args...)
	params = append(params, limit, offset)
	rows, err := db.db.QueryContext(ctx, knownPathsSQL+`SELECT path,isdir,sizehex,mtime,inode,executable,block,state,EXISTS(SELECT 1 FROM path_projections p WHERE p.folder_id=? AND p.path=entries.path AND observed_kind IS NOT NULL) FROM entries WHERE `+where+` ORDER BY `+order+` LIMIT ? OFFSET ?`, params...)
	if err != nil {
		return nil, 0, err
	}
	items := []BrowseItem{}
	for rows.Next() {
		var it BrowseItem
		var sizehex string
		var observed bool
		if err := rows.Scan(&it.Path, &it.IsDir, &sizehex, &it.MtimeNS, &it.Inode, &it.Executable, &it.BlockReason, &it.ContentState, &observed); err != nil {
			rows.Close()
			return nil, 0, err
		}
		it.WorkingState = "unobserved"
		if observed {
			it.WorkingState = "observed"
		}
		it.Name = filepath.Base(it.Path)
		it.Kind = history.KindFile
		if it.IsDir {
			it.Kind = history.KindDirectory
		}
		it.Size, _ = strconv.ParseUint(sizehex, 16, 64)
		items = append(items, it)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, err
	}
	for i := range items {
		var err error
		items[i].HasConflict, items[i].StructuralConflict, err = db.pathConflict(ctx, folder, items[i].Path)
		if items[i].ContentState == "pending" {
			items[i].WorkingState = "pending"
		}
		if items[i].HasConflict || items[i].StructuralConflict != "" {
			items[i].WorkingState = "conflict"
		}
		if items[i].BlockReason != "" {
			items[i].WorkingState = "blocked"
		}
		if err != nil {
			return nil, 0, err
		}
	}
	return items, total, nil
}

// pathConflict inspects just the selected path and its ancestors/descendants,
// without loading the workspace history into an in-memory reference model.
func (db *DB) pathConflict(ctx context.Context, folder history.ID, path string) (bool, string, error) {
	heads, err := db.headsUnlocked(ctx, folder, path)
	if err != nil {
		return false, "", err
	}
	structural := ""
	for ancestor := path; ancestor != "." && ancestor != ""; ancestor = filepath.Dir(ancestor) {
		var found int
		err := db.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM versions v WHERE folder_id=? AND path=? AND kind!=2 AND NOT EXISTS(SELECT 1 FROM version_parents p WHERE p.folder_id=v.folder_id AND p.parent_author=v.author_id AND p.parent_counter=v.counter)) AND EXISTS(SELECT 1 FROM versions v WHERE folder_id=? AND path>=? AND path<? AND kind!=3 AND NOT EXISTS(SELECT 1 FROM version_parents p WHERE p.folder_id=v.folder_id AND p.parent_author=v.author_id AND p.parent_counter=v.counter))`, folder[:], ancestor, folder[:], ancestor+"/", ancestor+"0").Scan(&found)
		if err != nil {
			return false, "", err
		}
		if found != 0 {
			structural = "incompatible ancestor and descendant history"
			break
		}
	}
	return len(heads) > 1, structural, nil
}

func (db *DB) BrowseWorkspaceDirectory(ctx context.Context, folder history.ID, options BrowseOptions) (*BrowseResult, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if err := validateBrowsePath(options.DirPath, true); err != nil {
		return nil, err
	}
	generation, err := db.browseGeneration(ctx, folder)
	if err != nil {
		return nil, err
	}
	dir := options.DirPath
	offset := 0
	sortBy := options.SortBy
	sortDir := options.SortDir
	limit := pageLimit(options.Limit)
	if options.SnapshotGeneration != 0 && options.SnapshotGeneration != generation {
		return nil, ErrStaleCursor
	}
	if options.Cursor != "" {
		cur, err := decodeCursor(options.Cursor)
		if err != nil {
			return nil, err
		}
		if cur.DirPath != hex.EncodeToString(folder[:])+":"+dir || cur.Offset < 0 || cur.Offset > 1000000 || cur.Limit < 1 || cur.Limit > 200 {
			return nil, ErrInvalidCursor
		}
		if cur.Gen != generation {
			return nil, ErrStaleCursor
		}
		if (sortBy != "" && sortBy != cur.SortBy) || (sortDir != "" && sortDir != cur.SortDir) || (options.Limit > 0 && limit != cur.Limit) {
			return nil, ErrInvalidCursor
		}
		offset = cur.Offset
		sortBy = cur.SortBy
		sortDir = cur.SortDir
		limit = cur.Limit
	}
	if sortBy == "" {
		sortBy = "name"
	}
	if sortDir == "" {
		sortDir = "asc"
	}
	sorts := map[string]string{"name": "lower(path)", "size": "sizehex", "mtime": "mtime", "kind": "isdir"}
	column, ok := sorts[sortBy]
	if !ok || (sortDir != "asc" && sortDir != "desc") {
		return nil, ErrInvalidCursor
	}
	if dir != "" {
		var isdir int
		err := db.db.QueryRowContext(ctx, knownPathsSQL+`SELECT isdir FROM entries WHERE path=?`, folder[:], folder[:], folder[:], dir).Scan(&isdir)
		if errors.Is(err, sql.ErrNoRows) || isdir == 0 {
			return nil, ErrDirectoryNotFound
		}
		if err != nil {
			return nil, err
		}
	}
	where := `instr(path,'/')=0`
	var args []any
	if dir != "" {
		where = `path>=? AND path<? AND instr(substr(path,?),'/')=0`
		args = []any{dir + "/", dir + "0", utf8.RuneCountInString(dir) + 2}
	}
	items, total, err := db.queryKnown(ctx, folder, where, "isdir DESC,"+column+" "+sortDir+",path ASC", args, limit, offset)
	if err != nil {
		return nil, err
	}
	next := ""
	if offset+len(items) < total {
		next = encodeCursor(generation, hex.EncodeToString(folder[:])+":"+dir, offset+len(items), sortBy, sortDir, limit)
	}
	parent := filepath.Dir(dir)
	if parent == "." {
		parent = ""
	}
	return &BrowseResult{DirPath: dir, ParentPath: parent, Items: items, TotalItems: total, NextCursor: next, SnapshotGeneration: generation}, nil
}

type SearchOptions struct {
	Query  string `json:"query"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}
type SearchResultItem = BrowseItem
type SearchResult struct {
	Query              string             `json:"query"`
	Items              []SearchResultItem `json:"items"`
	TotalFound         int                `json:"total_found"`
	HasMore            bool               `json:"has_more"`
	NextCursor         string             `json:"next_cursor,omitempty"`
	SnapshotGeneration int64              `json:"snapshot_generation"`
}

func (db *DB) SearchWorkspace(ctx context.Context, folder history.ID, options SearchOptions) (*SearchResult, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if len(options.Query) > 256 || !utf8.ValidString(options.Query) || options.Offset < 0 || options.Offset > 10000 {
		return nil, ErrInvalidCursor
	}
	generation, err := db.browseGeneration(ctx, folder)
	if err != nil {
		return nil, err
	}
	query := strings.TrimSpace(options.Query)
	limit := pageLimit(options.Limit)
	offset := options.Offset
	if options.Cursor != "" {
		cur, err := decodeCursor(options.Cursor)
		if err != nil {
			return nil, err
		}
		if cur.DirPath != hex.EncodeToString(folder[:])+":search:"+query || cur.Offset < 0 || cur.Offset > 1000000 || cur.Limit < 1 || cur.Limit > 200 {
			return nil, ErrInvalidCursor
		}
		if cur.Gen != generation {
			return nil, ErrStaleCursor
		}
		offset = cur.Offset
		limit = cur.Limit
	}
	items := []BrowseItem{}
	total := 0
	if query != "" {
		items, total, err = db.queryKnown(ctx, folder, `instr(lower(path),lower(?))>0`, "path ASC", []any{query}, limit, offset)
		if err != nil {
			return nil, err
		}
	}
	next := ""
	if offset+len(items) < total {
		next = encodeCursor(generation, hex.EncodeToString(folder[:])+":search:"+query, offset+len(items), "name", "asc", limit)
	}
	return &SearchResult{Query: query, Items: items, TotalFound: total, HasMore: next != "", NextCursor: next, SnapshotGeneration: generation}, nil
}

// FileVersionDetail describes one version DAG head for a path.
type FileVersionDetail struct {
	AuthorID     string `json:"author_id"`
	AuthorName   string `json:"author_name,omitempty"`
	Counter      uint64 `json:"counter"`
	CounterStr   string `json:"counter_str"`
	FileDigest   string `json:"file_digest,omitempty"`
	FileSize     uint64 `json:"file_size"`
	DisplayTime  string `json:"display_time"`
	ContentState string `json:"content_state"`
	CASAvailable bool   `json:"cas_available"` // locally present; verified again on read
}

// PeerProgressSummary describes peer replication progress for a version.
type PeerProgressSummary struct {
	VersionAuthor  string `json:"version_author"`
	VersionCounter uint64 `json:"version_counter"`
	PeerID         string `json:"peer_id"`
	PeerName       string `json:"peer_name,omitempty"`
	Receipt        bool   `json:"receipt"`
	RemoteStatus   string `json:"remote_status,omitempty"`
	LastContactNS  int64  `json:"last_contact_ns"`
}

// FileDetails contains comprehensive technical and human metadata for an exact path.
type FileDetails struct {
	Path               string                `json:"path"`
	Name               string                `json:"name"`
	IsDir              bool                  `json:"is_dir"`
	Kind               history.Kind          `json:"kind"`
	Size               uint64                `json:"size"`
	MtimeNS            int64                 `json:"mtime_ns"`
	Inode              uint64                `json:"inode"`
	Executable         bool                  `json:"executable"`
	BlockReason        string                `json:"block_reason,omitempty"`
	WorkingState       string                `json:"working_state"` // clean, modified, saving, saved, syncing, conflict, blocked
	ContentState       string                `json:"content_state"`
	Heads              []FileVersionDetail   `json:"heads"`
	HasConflict        bool                  `json:"has_conflict"`
	StructuralConflict string                `json:"structural_conflict,omitempty"`
	Peers              []PeerProgressSummary `json:"peers,omitempty"`
}

// FileDetails reports technical metadata, DAG heads, CAS chunk availability, and working copy state.
func (db *DB) FileDetails(ctx context.Context, folder history.ID, path string) (*FileDetails, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	cleanPath := path
	if err := validateBrowsePath(cleanPath, false); err != nil {
		return nil, err
	}
	if _, err := db.browseGeneration(ctx, folder); err != nil {
		return nil, err
	}
	// 1. Query path_projections
	var p Projection
	var hasProj bool
	proj, err := loadProjection(ctx, db.db, folder, cleanPath)
	if err == nil {
		p = proj
		hasProj = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	items, _, err := db.queryKnown(ctx, folder, "path=?", "path ASC", []any{cleanPath}, 1, 0)
	if err != nil {
		return nil, err
	}
	isDir := false
	if len(items) > 0 {
		isDir = items[0].IsDir
	} else if !hasProj {
		heads, err := db.headsUnlocked(ctx, folder, cleanPath)
		if err != nil {
			return nil, err
		}
		if len(heads) == 0 {
			return nil, ErrPathNotFound
		}
		env, _, err := db.envelopeAndState(ctx, db.db, heads[0])
		if err != nil {
			return nil, err
		}
		p.Kind = env.Kind
	}

	name := filepath.Base(cleanPath)
	kind := history.KindFile
	if isDir {
		kind = history.KindDirectory
	} else if p.Kind == history.KindTombstone {
		kind = history.KindTombstone
	}

	// 3. Query DAG heads for this path
	headIDs, err := db.headsUnlocked(ctx, folder, cleanPath)
	if err != nil {
		return nil, err
	}
	var heads []FileVersionDetail
	contentState := "unknown"

	for _, hID := range headIDs {
		env, state, envErr := db.envelopeAndState(ctx, db.db, hID)
		if envErr != nil {
			return nil, envErr
		}
		casAvail := false
		if env.Manifest != nil {
			casAvail = state == "ready" && db.checkManifestCASAvailable(env.Manifest)
		}
		authorName, _ := db.getDeviceDisplayNameUnlocked(ctx, hID.Author)
		detail := FileVersionDetail{
			AuthorID:   hex.EncodeToString(hID.Author[:]),
			AuthorName: authorName,
			Counter:    hID.Counter,
			CounterStr: strconv.FormatUint(hID.Counter, 10),

			DisplayTime:  env.DisplayTime,
			ContentState: string(state),
			CASAvailable: casAvail,
		}
		if env.Manifest != nil {
			detail.FileDigest = hex.EncodeToString(env.Manifest.Digest[:])
			detail.FileSize = env.Manifest.Size
		}
		heads = append(heads, detail)
		if contentState == "unknown" || (state != "ready" && contentState == "ready") {
			contentState = string(state)
		}
	}

	// 4. Conflicts
	hasConflict := len(heads) > 1
	_, structConflict, err := db.pathConflict(ctx, folder, cleanPath)
	if err != nil {
		return nil, err
	}
	workingState := "unobserved"
	if hasProj {
		workingState = "observed"
	}
	if contentState != "ready" {
		workingState = "pending"
	}
	if hasConflict || structConflict != "" {
		workingState = "conflict"
	}
	if p.BlockReason != "" {
		workingState = "blocked"
	}

	if !hasProj && len(items) > 0 {
		p.ObservedSize = items[0].Size
	}
	// 6. Peers progress
	var peers []PeerProgressSummary
	if len(heads) > 0 {
		primaryHead := headIDs[0]
		pRows, pErr := db.db.QueryContext(ctx, `
			SELECT peer_id, receipt, COALESCE(remote_status, ''), COALESCE(last_contact_ns, 0), COALESCE(d.display_name,'')
			FROM peer_progress LEFT JOIN devices d ON d.device_id=peer_progress.peer_id
			WHERE folder_id=? AND version_author=? AND version_counter=?
		`, folder[:], primaryHead.Author[:], encodeUint(primaryHead.Counter))
		if pErr == nil {
			defer pRows.Close()
			for pRows.Next() {
				var peerIDRaw []byte
				var receiptInt int
				var remoteStatus, pName string
				var lastContact int64
				if err := pRows.Scan(&peerIDRaw, &receiptInt, &remoteStatus, &lastContact, &pName); err == nil {
					var peerID history.ID
					copy(peerID[:], peerIDRaw)
					peers = append(peers, PeerProgressSummary{
						VersionAuthor:  hex.EncodeToString(primaryHead.Author[:]),
						VersionCounter: primaryHead.Counter,
						PeerID:         hex.EncodeToString(peerIDRaw),
						PeerName:       pName,
						Receipt:        receiptInt == 1,
						RemoteStatus:   remoteStatus,
						LastContactNS:  lastContact,
					})
				}
			}
		}
	}

	return &FileDetails{
		Path:               cleanPath,
		Name:               name,
		IsDir:              isDir,
		Kind:               kind,
		Size:               p.ObservedSize,
		MtimeNS:            p.ObservedMtimeNS,
		Inode:              p.ObservedInode,
		Executable:         p.Executable,
		BlockReason:        p.BlockReason,
		WorkingState:       workingState,
		ContentState:       contentState,
		Heads:              heads,
		HasConflict:        hasConflict,
		StructuralConflict: structConflict,
		Peers:              peers,
	}, nil
}

func (db *DB) headsUnlocked(ctx context.Context, folder history.ID, path string) ([]history.VersionID, error) {
	rows, err := db.db.QueryContext(ctx, `
		SELECT author_id, counter FROM versions v
		WHERE folder_id=? AND path=?
		  AND NOT EXISTS (
			SELECT 1 FROM version_parents vp
			WHERE vp.folder_id=v.folder_id AND vp.parent_author=v.author_id AND vp.parent_counter=v.counter
		  )
		ORDER BY counter DESC,author_id
	`, folder[:], path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []history.VersionID
	for rows.Next() {
		var a, c []byte
		if err := rows.Scan(&a, &c); err != nil {
			return nil, err
		}
		var author history.ID
		copy(author[:], a)
		counter, _ := decodeUint(c)
		result = append(result, history.VersionID{Folder: folder, Author: author, Counter: counter})
	}
	return result, rows.Err()
}

func (db *DB) getDeviceDisplayNameUnlocked(ctx context.Context, device history.ID) (string, error) {
	var name sql.NullString
	if err := db.db.QueryRowContext(ctx, `SELECT display_name FROM devices WHERE device_id=?`, device[:]).Scan(&name); err != nil {
		return "", nil
	}
	if name.Valid {
		return name.String, nil
	}
	return "", nil
}

func (db *DB) checkManifestCASAvailable(manifest *history.Manifest) bool {
	if manifest == nil {
		return false
	}
	if manifest.Size == 0 {
		return true
	}
	for _, chunk := range manifest.Chunks {
		p := db.objectPath(chunk.Digest)
		if fi, err := os.Stat(p); err != nil || uint64(fi.Size()) != chunk.Length {
			return false
		}
	}
	return true
}

// DeletedFileItem represents a tombstoned file and restore availability.
type DeletedFileItem struct {
	Path           string `json:"path"`
	Name           string `json:"name"`
	DeletedAt      string `json:"deleted_at"`
	DeletedBy      string `json:"deleted_by"`
	DeletedCounter uint64 `json:"deleted_counter"`
	LastActiveSize uint64 `json:"last_active_size"`
	CASAvailable   bool   `json:"cas_available"` // prior content bytes present in local CAS
}

// DeletedFilesResult contains paginated deleted files.
type DeletedFilesResult struct {
	Items      []DeletedFileItem `json:"items"`
	NextCursor string            `json:"next_cursor,omitempty"`
	TotalItems int               `json:"total_items"`
}

// BrowseDeletedFiles lists tombstoned files and whether their pre-deletion bytes remain available in CAS.
func (db *DB) BrowseDeletedFiles(ctx context.Context, folder history.ID, cursor string, limit int) (*DeletedFilesResult, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	generation, err := db.browseGeneration(ctx, folder)
	if err != nil {
		return nil, err
	}
	limit = pageLimit(limit)
	offset := 0
	binding := hex.EncodeToString(folder[:]) + ":deleted"
	if cursor != "" {
		cur, err := decodeCursor(cursor)
		if err != nil {
			return nil, err
		}
		if cur.DirPath != binding || cur.Offset < 0 || cur.Offset > 1000000 || cur.Limit < 1 || cur.Limit > 200 {
			return nil, ErrInvalidCursor
		}
		if cur.Gen != generation {
			return nil, ErrStaleCursor
		}
		offset = cur.Offset
		limit = cur.Limit
	}
	const deleted = ` FROM versions v WHERE folder_id=? AND kind=3 AND NOT EXISTS(SELECT 1 FROM version_parents p WHERE p.folder_id=v.folder_id AND p.parent_author=v.author_id AND p.parent_counter=v.counter) AND NOT EXISTS(SELECT 1 FROM versions a WHERE a.folder_id=v.folder_id AND a.path=v.path AND a.kind!=3 AND NOT EXISTS(SELECT 1 FROM version_parents p WHERE p.folder_id=a.folder_id AND p.parent_author=a.author_id AND p.parent_counter=a.counter))`
	var total int
	if err := db.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT path)`+deleted, folder[:]).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := db.db.QueryContext(ctx, `SELECT path,author_id,counter,display_time`+deleted+` GROUP BY path ORDER BY path LIMIT ? OFFSET ?`, folder[:], limit, offset)
	if err != nil {
		return nil, err
	}
	items := []DeletedFileItem{}
	for rows.Next() {
		var it DeletedFileItem
		var author, counter []byte
		if err := rows.Scan(&it.Path, &author, &counter, &it.DeletedAt); err != nil {
			rows.Close()
			return nil, err
		}
		it.Name = filepath.Base(it.Path)
		it.DeletedBy = hex.EncodeToString(author)
		it.DeletedCounter, _ = decodeUint(counter)
		items = append(items, it)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range items {
		var author, counter, size []byte
		err := db.db.QueryRowContext(ctx, `SELECT author_id,counter,file_size FROM versions WHERE folder_id=? AND path=? AND kind=1 ORDER BY acquired_ns DESC,author_id,counter DESC LIMIT 1`, folder[:], items[i].Path).Scan(&author, &counter, &size)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		items[i].LastActiveSize, _ = decodeUint(size)
		var a history.ID
		copy(a[:], author)
		c, _ := decodeUint(counter)
		env, state, err := db.envelopeAndState(ctx, db.db, history.VersionID{Folder: folder, Author: a, Counter: c})
		if err != nil {
			return nil, err
		}
		items[i].CASAvailable = state == "ready" && db.checkManifestCASAvailable(env.Manifest)
	}
	next := ""
	if offset+len(items) < total {
		next = encodeCursor(generation, binding, offset+len(items), "name", "asc", limit)
	}
	return &DeletedFilesResult{Items: items, TotalItems: total, NextCursor: next}, nil
}

// FileHistoryItem reports a historical version and whether its bytes are available in CAS.
type FileHistoryItem struct {
	AuthorID     string       `json:"author_id"`
	AuthorName   string       `json:"author_name,omitempty"`
	Counter      uint64       `json:"counter"`
	CounterStr   string       `json:"counter_str"`
	Kind         history.Kind `json:"kind"`
	DisplayTime  string       `json:"display_time"`
	FileSize     uint64       `json:"file_size"`
	FileDigest   string       `json:"file_digest,omitempty"`
	ContentState string       `json:"content_state"`
	CASAvailable bool         `json:"cas_available"`
	IsHead       bool         `json:"is_head"`
}

type FileHistoryResult struct {
	Items              []FileHistoryItem `json:"items"`
	NextCursor         string            `json:"next_cursor,omitempty"`
	TotalItems         int               `json:"total_items"`
	SnapshotGeneration int64             `json:"snapshot_generation"`
}

// FilePathHistory is the compatibility first-page query; new callers paginate
// with BrowsePathHistory. Ordering is deterministic presentation, not causality.
func (db *DB) FilePathHistory(ctx context.Context, folder history.ID, path string) ([]FileHistoryItem, error) {
	result, err := db.BrowsePathHistory(ctx, folder, path, "", 200)
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

func (db *DB) BrowsePathHistory(ctx context.Context, folder history.ID, path, cursor string, limit int) (*FileHistoryResult, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	cleanPath := path
	if err := validateBrowsePath(cleanPath, false); err != nil {
		return nil, err
	}
	generation, err := db.browseGeneration(ctx, folder)
	if err != nil {
		return nil, err
	}
	limit = pageLimit(limit)
	offset := 0
	binding := hex.EncodeToString(folder[:]) + ":history:" + path
	if cursor != "" {
		cur, err := decodeCursor(cursor)
		if err != nil {
			return nil, err
		}
		if cur.DirPath != binding || cur.Offset < 0 || cur.Offset > 1000000 || cur.Limit < 1 || cur.Limit > 200 {
			return nil, ErrInvalidCursor
		}
		if cur.Gen != generation {
			return nil, ErrStaleCursor
		}
		offset = cur.Offset
		limit = cur.Limit
	}
	var total int
	if err := db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM versions WHERE folder_id=? AND path=?`, folder[:], path).Scan(&total); err != nil {
		return nil, err
	}
	if err := history.ValidatePath(cleanPath); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDirectoryPath, err)
	}

	// 1. Get all historical versions
	rows, err := db.db.QueryContext(ctx, `SELECT author_id, counter FROM versions WHERE folder_id=? AND path=? ORDER BY counter DESC,author_id LIMIT ? OFFSET ?`, folder[:], cleanPath, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []history.VersionID
	for rows.Next() {
		var a, c []byte
		if err := rows.Scan(&a, &c); err != nil {
			return nil, err
		}
		var author history.ID
		copy(author[:], a)
		counter, _ := decodeUint(c)
		ids = append(ids, history.VersionID{Folder: folder, Author: author, Counter: counter})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows.Close()
	// 2. Identify heads
	headIDs, err := db.headsUnlocked(ctx, folder, cleanPath)
	if err != nil {
		return nil, err
	}
	headMap := make(map[history.VersionID]bool, len(headIDs))
	for _, h := range headIDs {
		headMap[h] = true
	}

	items := []FileHistoryItem{}
	for _, id := range ids {
		env, state, err := db.envelopeAndState(ctx, db.db, id)
		if err != nil {
			return nil, err
		}
		authorName, _ := db.getDeviceDisplayNameUnlocked(ctx, id.Author)
		casAvail := false
		var size uint64
		digestHex := ""
		if env.Manifest != nil {
			casAvail = state == "ready" && db.checkManifestCASAvailable(env.Manifest)
			size = env.Manifest.Size
			digestHex = hex.EncodeToString(env.Manifest.Digest[:])
		}

		items = append(items, FileHistoryItem{
			AuthorID:     hex.EncodeToString(id.Author[:]),
			AuthorName:   authorName,
			Counter:      id.Counter,
			CounterStr:   strconv.FormatUint(id.Counter, 10),
			Kind:         env.Kind,
			DisplayTime:  env.DisplayTime,
			FileSize:     size,
			FileDigest:   digestHex,
			ContentState: string(state),
			CASAvailable: casAvail,
			IsHead:       headMap[id],
		})
	}

	next := ""
	if offset+len(items) < total {
		next = encodeCursor(generation, binding, offset+len(items), "counter", "desc", limit)
	}
	return &FileHistoryResult{Items: items, NextCursor: next, TotalItems: total, SnapshotGeneration: generation}, nil
}

// GetVersionManifest retrieves the verified manifest and envelope for an exact version ID.
func (db *DB) GetVersionManifest(ctx context.Context, id history.VersionID) (*history.Manifest, history.Envelope, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	env, _, err := db.envelopeAndState(ctx, db.db, id)
	if err != nil {
		return nil, history.Envelope{}, err
	}
	if env.Kind != history.KindFile {
		return nil, env, ErrNotAFile
	}
	if env.Manifest == nil {
		return nil, env, fmt.Errorf("version manifest is missing")
	}
	return env.Manifest, env, nil
}
