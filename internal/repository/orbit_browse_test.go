package repository

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/testkit"
)

func setupTestDB(t *testing.T) (*DB, history.ID, history.ID, func()) {
	t.Helper()
	stateDir := testkit.NewDisposable(t)
	ctx := context.Background()

	db, err := Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}

	var folderID, authorID history.ID
	rand.Read(folderID[:])
	rand.Read(authorID[:])

	// Register folder and setup author
	if err := db.EnsureFolder(ctx, folderID, authorID, 1); err != nil {
		t.Fatal(err)
	}
	reg := RootRegistration{
		Folder:         folderID,
		Path:           stateDir + "/root",
		RegistrationID: folderID,
	}
	if err := db.RegisterRoot(ctx, reg); err != nil {
		t.Fatal(err)
	}

	cleanup := func() {
		_ = db.Close()
	}
	return db, folderID, authorID, cleanup
}

func installTestVersion(t *testing.T, db *DB, folder history.ID, path string, kind history.Kind, content []byte) history.Envelope {
	t.Helper()
	ctx := context.Background()

	var manifest *history.Manifest
	if kind == history.KindFile {
		m, err := db.StoreFile(ctx, strings.NewReader(string(content)), false)
		if err != nil {
			t.Fatalf("StoreFile %s failed: %v", path, err)
		}
		manifest = m
	}

	heads, err := db.headsUnlocked(ctx, folder, path)
	if err != nil {
		t.Fatal(err)
	}
	env, err := db.CreateLocalVersion(ctx, LocalVersionRequest{
		Basis:            heads,
		Folder:           folder,
		Path:             path,
		Kind:             kind,
		Manifest:         manifest,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("CreateLocalVersion %s failed: %v", path, err)
	}

	return env
}

// TestOrbitBrowse_HierarchicalDirectoryQueries tests Invariants I25, I19, I09:
// immediate child queries, implicit ancestors, empty scaffolds, unusual names, tombstones,
// sorting, pagination, and cursor freshness.
func TestOrbitBrowse_HierarchicalDirectoryQueries(t *testing.T) {
	db, folder, _, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// 1. Install root files
	installTestVersion(t, db, folder, "readme.txt", history.KindFile, []byte("Orbit Orbit"))
	installTestVersion(t, db, folder, "root.go", history.KindFile, []byte("package main"))
	installTestVersion(t, db, folder, "space file (1).txt", history.KindFile, []byte("spaces and parens"))
	installTestVersion(t, db, folder, "unicode_日本語_🚀.md", history.KindFile, []byte("# 日本語 Rocket"))
	installTestVersion(t, db, folder, "dots.and.more.dots.tar.gz", history.KindFile, []byte("binary dots"))

	// 2. Install files in nested implicit directories
	installTestVersion(t, db, folder, "docs/getting-started.md", history.KindFile, []byte("getting started"))
	installTestVersion(t, db, folder, "docs/api/v1.md", history.KindFile, []byte("api v1"))
	installTestVersion(t, db, folder, "docs/api/v2.md", history.KindFile, []byte("api v2"))
	installTestVersion(t, db, folder, "src/main.go", history.KindFile, []byte("func main() {}"))
	installTestVersion(t, db, folder, "src/internal/util.go", history.KindFile, []byte("func util() {}"))

	// 3. Install explicit empty scaffold directories
	if err := db.MarkScaffold(ctx, folder, "empty_dir"); err != nil {
		t.Fatalf("MarkScaffold empty_dir failed: %v", err)
	}
	if err := db.CompleteScaffold(ctx, folder, "empty_dir"); err != nil {
		t.Fatalf("CompleteScaffold empty_dir failed: %v", err)
	}
	if err := db.MarkScaffold(ctx, folder, "docs/empty_sub"); err != nil {
		t.Fatalf("MarkScaffold docs/empty_sub failed: %v", err)
	}
	if err := db.CompleteScaffold(ctx, folder, "docs/empty_sub"); err != nil {
		t.Fatalf("CompleteScaffold docs/empty_sub failed: %v", err)
	}

	// 4. Install a tombstone file (deleted file)
	installTestVersion(t, db, folder, "deleted_file.txt", history.KindFile, []byte("will be deleted"))
	// Delete it
	installTestVersion(t, db, folder, "deleted_file.txt", history.KindTombstone, nil)

	// --- A. Query Root Directory ("") ---
	rootRes, err := db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{})
	if err != nil {
		t.Fatalf("BrowseWorkspaceDirectory root failed: %v", err)
	}

	if rootRes.DirPath != "" {
		t.Errorf("expected root DirPath '', got %q", rootRes.DirPath)
	}
	if rootRes.ParentPath != "" {
		t.Errorf("expected root ParentPath '', got %q", rootRes.ParentPath)
	}

	// Expected directories in root: "docs", "empty_dir", "src"
	// Expected files in root: "dots.and.more.dots.tar.gz", "readme.txt", "root.go", "space file (1).txt", "unicode_日本語_🚀.md"
	// "deleted_file.txt" MUST NOT appear
	// Nested files MUST NOT appear directly in root
	dirMap := make(map[string]BrowseItem)
	fileMap := make(map[string]BrowseItem)
	for _, it := range rootRes.Items {
		if it.IsDir {
			dirMap[it.Name] = it
		} else {
			fileMap[it.Name] = it
		}
	}

	expectedDirs := []string{"docs", "empty_dir", "src"}
	for _, d := range expectedDirs {
		if _, ok := dirMap[d]; !ok {
			t.Errorf("expected directory %q in root browse, but was missing", d)
		}
	}

	expectedFiles := []string{"dots.and.more.dots.tar.gz", "readme.txt", "root.go", "space file (1).txt", "unicode_日本語_🚀.md"}
	for _, f := range expectedFiles {
		if _, ok := fileMap[f]; !ok {
			t.Errorf("expected file %q in root browse, but was missing", f)
		}
	}

	if _, ok := fileMap["deleted_file.txt"]; ok {
		t.Error("tombstoned file 'deleted_file.txt' must NOT appear in active directory browse")
	}
	if _, ok := fileMap["getting-started.md"]; ok {
		t.Error("nested file 'getting-started.md' must NOT appear in root directory browse")
	}

	// Invariant: Directories must be sorted before files by default
	seenFile := false
	for _, it := range rootRes.Items {
		if !it.IsDir {
			seenFile = true
		} else if seenFile {
			t.Errorf("found directory %s after file in default sort order", it.Name)
		}
	}

	// --- B. Query Subdirectory "docs" ---
	docsRes, err := db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{DirPath: "docs"})
	if err != nil {
		t.Fatalf("BrowseWorkspaceDirectory docs failed: %v", err)
	}
	if docsRes.ParentPath != "" {
		t.Errorf("expected docs ParentPath '', got %q", docsRes.ParentPath)
	}

	docsSubdirs := make(map[string]bool)
	docsFiles := make(map[string]bool)
	for _, it := range docsRes.Items {
		if it.IsDir {
			docsSubdirs[it.Name] = true
		} else {
			docsFiles[it.Name] = true
		}
	}

	if !docsSubdirs["api"] {
		t.Error("expected implicit subdir 'api' in docs/")
	}
	if !docsSubdirs["empty_sub"] {
		t.Error("expected scaffold subdir 'empty_sub' in docs/")
	}
	if !docsFiles["getting-started.md"] {
		t.Error("expected direct child file 'getting-started.md' in docs/")
	}
	if docsFiles["v1.md"] {
		t.Error("nested grandchild 'v1.md' should NOT appear directly in docs/")
	}

	// --- C. Query Deep Subdirectory "docs/api" ---
	apiRes, err := db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{DirPath: "docs/api"})
	if err != nil {
		t.Fatalf("BrowseWorkspaceDirectory docs/api failed: %v", err)
	}
	if apiRes.ParentPath != "docs" {
		t.Errorf("expected docs/api ParentPath 'docs', got %q", apiRes.ParentPath)
	}
	if len(apiRes.Items) != 2 {
		t.Fatalf("expected 2 items in docs/api, got %d", len(apiRes.Items))
	}

	// --- D. Query Empty Scaffold Directory ---
	emptyRes, err := db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{DirPath: "empty_dir"})
	if err != nil {
		t.Fatalf("BrowseWorkspaceDirectory empty_dir failed: %v", err)
	}
	if len(emptyRes.Items) != 0 {
		t.Fatalf("expected 0 items in empty scaffold dir, got %d", len(emptyRes.Items))
	}

	// --- E. Error Handling: Non-existent and Invalid Directories ---
	_, err = db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{DirPath: "non_existent_folder"})
	if err == nil {
		t.Fatal("expected ErrDirectoryNotFound for non_existent_folder, got nil")
	}

	_, err = db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{DirPath: "../secret"})
	if err == nil {
		t.Fatal("expected ErrInvalidDirectoryPath for traversal '../secret', got nil")
	}

	// --- F. Sorting Options ---
	// Sort by size desc
	sizeRes, err := db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{
		SortBy:  "size",
		SortDir: "desc",
	})
	if err != nil {
		t.Fatalf("BrowseWorkspaceDirectory sort size desc failed: %v", err)
	}
	var prevSize uint64 = ^uint64(0)
	for _, it := range sizeRes.Items {
		if !it.IsDir {
			if it.Size > prevSize {
				t.Errorf("size sort desc failed: item %s size %d > prev %d", it.Name, it.Size, prevSize)
			}
			prevSize = it.Size
		}
	}

	// --- G. Pagination and Cursor Validation ---
	p1, err := db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{
		Limit: 3,
	})
	if err != nil {
		t.Fatalf("Browse page 1 failed: %v", err)
	}
	if len(p1.Items) != 3 {
		t.Fatalf("expected 3 items on page 1, got %d", len(p1.Items))
	}
	if p1.NextCursor == "" {
		t.Fatal("expected non-empty NextCursor on page 1")
	}

	p2, err := db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{
		Cursor: p1.NextCursor,
		Limit:  3,
	})
	if err != nil {
		t.Fatalf("Browse page 2 failed: %v", err)
	}
	if len(p2.Items) != 3 {
		t.Fatalf("expected 3 items on page 2, got %d", len(p2.Items))
	}
	// Verify no overlap between page 1 and page 2
	for _, item1 := range p1.Items {
		for _, item2 := range p2.Items {
			if item1.Path == item2.Path {
				t.Errorf("page 1 and page 2 overlap on item %s", item1.Path)
			}
		}
	}

	// --- H. Stale Cursor Rejection ---
	// Advance scan generation
	if _, err := db.BeginScan(ctx, folder); err != nil {
		t.Fatalf("BeginScan failed: %v", err)
	}
	_, err = db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{
		Cursor: p1.NextCursor,
	})
	if err == nil {
		t.Fatal("expected ErrStaleCursor after scan generation advance, got nil")
	}
}

// TestOrbitSearch_BoundedWorkspaceSearch tests Invariant I09, I13:
// indexed substring search across files and directories, case-insensitivity, and tombstones exclusion.
func TestOrbitSearch_BoundedWorkspaceSearch(t *testing.T) {
	db, folder, _, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	installTestVersion(t, db, folder, "config.json", history.KindFile, []byte(`{"orbit": true}`))
	installTestVersion(t, db, folder, "docs/config-guide.md", history.KindFile, []byte("# Config Guide"))
	installTestVersion(t, db, folder, "src/config/parser.go", history.KindFile, []byte("package config"))
	installTestVersion(t, db, folder, "unrelated.txt", history.KindFile, []byte("unrelated"))

	// Tombstoned config file
	installTestVersion(t, db, folder, "deleted-config.bak", history.KindFile, []byte("old config"))
	installTestVersion(t, db, folder, "deleted-config.bak", history.KindTombstone, nil)

	// 1. Search for "config"
	res, err := db.SearchWorkspace(ctx, folder, SearchOptions{
		Query: "config",
	})
	if err != nil {
		t.Fatalf("SearchWorkspace failed: %v", err)
	}

	if res.TotalFound != 4 {
		t.Errorf("expected 4 results including implicit directory for 'config', got %d", res.TotalFound)
	}

	paths := make(map[string]bool)
	for _, it := range res.Items {
		paths[it.Path] = true
	}
	if !paths["config.json"] || !paths["docs/config-guide.md"] || !paths["src/config/parser.go"] {
		t.Errorf("missing expected search result in %v", paths)
	}
	if paths["deleted-config.bak"] {
		t.Error("tombstoned file deleted-config.bak must NOT appear in search results")
	}

	// 2. Case-insensitive search ("CONFIG")
	resUpper, err := db.SearchWorkspace(ctx, folder, SearchOptions{
		Query: "CONFIG",
	})
	if err != nil {
		t.Fatalf("SearchWorkspace upper failed: %v", err)
	}
	if resUpper.TotalFound != 4 {
		t.Errorf("expected case-insensitive match for 'CONFIG' (4 results), got %d", resUpper.TotalFound)
	}

	// 3. Paging with limit
	paged, err := db.SearchWorkspace(ctx, folder, SearchOptions{
		Query: "config",
		Limit: 2,
	})
	if err != nil {
		t.Fatalf("SearchWorkspace paged failed: %v", err)
	}
	if len(paged.Items) != 2 {
		t.Errorf("expected 2 items with limit=2, got %d", len(paged.Items))
	}
	if !paged.HasMore {
		t.Error("expected HasMore=true when results exceed limit")
	}
}

// TestOrbitBrowse_FileDetailsAndHistory tests Invariant I18, I25:
// complete technical metadata, DAG heads, CAS chunk availability, and deleted files index.
func TestOrbitBrowse_FileDetailsAndHistory(t *testing.T) {
	db, folder, _, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// 1. Active file with content installed
	installTestVersion(t, db, folder, "report.pdf", history.KindFile, []byte("PDF content header"))

	details, err := db.FileDetails(ctx, folder, "report.pdf")
	if err != nil {
		t.Fatalf("FileDetails report.pdf failed: %v", err)
	}

	if details.Name != "report.pdf" {
		t.Errorf("expected Name report.pdf, got %s", details.Name)
	}
	if details.IsDir {
		t.Error("expected IsDir=false for file")
	}
	if details.Kind != history.KindFile {
		t.Errorf("expected KindFile, got %v", details.Kind)
	}
	if len(details.Heads) != 1 {
		t.Fatalf("expected 1 head, got %d", len(details.Heads))
	}
	head := details.Heads[0]
	if !head.CASAvailable {
		t.Error("expected CASAvailable=true since chunks were installed")
	}
	if head.ContentState != "ready" {
		t.Errorf("expected ContentState 'ready', got %s", head.ContentState)
	}

	// 2. Version history
	// Install second version
	installTestVersion(t, db, folder, "report.pdf", history.KindFile, []byte("PDF content header updated v2"))
	hist, err := db.FilePathHistory(ctx, folder, "report.pdf")
	if err != nil {
		t.Fatalf("FilePathHistory failed: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(hist))
	}
	if !hist[0].IsHead {
		t.Error("expected latest version to be head")
	}
	if !hist[0].CASAvailable || !hist[1].CASAvailable {
		t.Error("expected both versions to have CASAvailable=true")
	}

	// 3. Tombstone & Deleted Files
	installTestVersion(t, db, folder, "report.pdf", history.KindTombstone, nil)

	deletedRes, err := db.BrowseDeletedFiles(ctx, folder, "", 10)
	if err != nil {
		t.Fatalf("BrowseDeletedFiles failed: %v", err)
	}
	if deletedRes.TotalItems != 1 {
		t.Fatalf("expected 1 deleted file, got %d", deletedRes.TotalItems)
	}
	delItem := deletedRes.Items[0]
	if delItem.Path != "report.pdf" {
		t.Errorf("expected deleted path 'report.pdf', got %s", delItem.Path)
	}
	if !delItem.CASAvailable {
		t.Error("expected CASAvailable=true for prior version of deleted file")
	}
}

// TestOrbitBrowse_ScalingTenThousandFiles tests Invariant I13:
// search and first-page queries stay bounded on a generated 10,000-file workspace.
// Query latency, response sizes, and memory usage are recorded.
func TestOrbitBrowse_ScalingTenThousandFiles(t *testing.T) {
	db, folder, author, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	t.Log("Generating 10,000 files across 100 directories...")
	startGen := time.Now()

	// 100 directories, each with 100 files
	emptyChunkDigest := sha256.Sum256([]byte("dummy content"))
	_ = db.InstallChunk(ctx, emptyChunkDigest, 13, bytes.NewReader([]byte("dummy content")))

	// Use raw transactions for batch inserting 10,000 files quickly in the test fixture
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	for d := 0; d < 100; d++ {
		dirName := fmt.Sprintf("folder_%02d", d)
		_, _ = tx.ExecContext(ctx, `INSERT INTO workspace_scaffolds(folder_id, path, pending) VALUES(?,?,0)`, folder[:], dirName)
		for f := 0; f < 100; f++ {
			counter := uint64(d*100 + f + 1)
			filePath := fmt.Sprintf("%s/file_%02d.txt", dirName, f)

			// Insert versions
			_, err = tx.ExecContext(ctx, `
				INSERT INTO versions(folder_id, author_id, counter, path, kind, authored_revision, display_time, file_size, file_digest, content_state, acquired_ns, envelope_digest)
				VALUES(?,?,?,?,1,X'0000000000000001','2026-10-01T12:00:00Z',X'000000000000000D',?,'ready',?,?)
			`, folder[:], author[:], encodeUint(counter), filePath, emptyChunkDigest[:], time.Now().UnixNano(), emptyChunkDigest[:])
			if err != nil {
				t.Fatalf("insert version: %v", err)
			}

			// Insert manifest_chunk
			_, err = tx.ExecContext(ctx, `
				INSERT INTO manifest_chunks(folder_id, author_id, counter, position, digest, length)
				VALUES(?,?,?,0,?,X'000000000000000D')
			`, folder[:], author[:], encodeUint(counter), emptyChunkDigest[:])
			if err != nil {
				t.Fatalf("insert manifest chunk: %v", err)
			}

			// Insert path_projection
			_, err = tx.ExecContext(ctx, `
				INSERT INTO path_projections(folder_id, path, publication_generation, applied_author, applied_counter, observed_kind, observed_digest, observed_size, observed_mtime_ns)
				VALUES(?,?,1,?,?,1,?,13,1000000000)
			`, folder[:], filePath, author[:], encodeUint(counter), emptyChunkDigest[:])
			if err != nil {
				t.Fatalf("insert projection: %v", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	t.Logf("Generated 10,000 files in %v", time.Since(startGen))

	var memStart runtime.MemStats
	runtime.ReadMemStats(&memStart)

	// 1. Measure First-Page Root Directory Browse
	t1 := time.Now()
	rootRes, err := db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{
		Limit: 50,
	})
	durRoot := time.Since(t1)
	if err != nil {
		t.Fatalf("Root browse failed: %v", err)
	}
	if len(rootRes.Items) != 50 {
		t.Errorf("expected 50 items on page 1, got %d", len(rootRes.Items))
	}
	t.Logf("Root first-page browse (50 items) on 10,000 files took: %v (items: %d, total: %d)",
		durRoot, len(rootRes.Items), rootRes.TotalItems)

	// 2. Measure Deep Subdirectory Browse (inside folder_42)
	t2 := time.Now()
	dirRes, err := db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{
		DirPath: "folder_42",
		Limit:   50,
	})
	durDir := time.Since(t2)
	if err != nil {
		t.Fatalf("Subdir browse failed: %v", err)
	}
	if len(dirRes.Items) != 50 {
		t.Errorf("expected 50 items in folder_42, got %d", len(dirRes.Items))
	}
	t.Logf("Deep subdir browse (50 items) took: %v (items: %d, total: %d)",
		durDir, len(dirRes.Items), dirRes.TotalItems)

	// 3. Measure Workspace Substring Search
	t3 := time.Now()
	searchRes, err := db.SearchWorkspace(ctx, folder, SearchOptions{
		Query: "file_42",
		Limit: 50,
	})
	durSearch := time.Since(t3)
	if err != nil {
		t.Fatalf("Workspace search failed: %v", err)
	}
	if len(searchRes.Items) != 50 {
		t.Errorf("expected 50 search items, got %d", len(searchRes.Items))
	}
	t.Logf("Workspace search for 'file_42' on 10,000 files took: %v (matched: %d, total found: %d)",
		durSearch, len(searchRes.Items), searchRes.TotalFound)

	var memEnd runtime.MemStats
	runtime.ReadMemStats(&memEnd)
	allocMB := float64(memEnd.TotalAlloc-memStart.TotalAlloc) / 1024 / 1024
	response, err := json.Marshal(rootRes)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Root page JSON bytes: %d", len(response))
	t.Logf("Memory allocated during scaling queries: %.2f MB", allocMB)
}
