package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
)

func TestOrbitBrowse_PendingStructuralAndStrictPaths(t *testing.T) {
	db, folder, _, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	installTestVersion(t, db, folder, "日本/empty", history.KindDirectory, nil)
	old := installTestVersion(t, db, folder, "collision", history.KindFile, []byte("old"))
	// Distinct author introduces a pending child beneath a known file.
	author := history.ID{99}
	data := []byte("not received")
	digest := sha256.Sum256(data)
	pending := history.Envelope{ID: history.VersionID{Folder: folder, Author: author, Counter: 1}, Path: "collision/pending.txt", Kind: history.KindFile, AuthoredRevision: 1, Vector: []history.ClockEntry{{Author: author, Counter: 1}}, Manifest: &history.Manifest{Size: uint64(len(data)), Digest: digest, Chunks: []history.Chunk{{Digest: digest, Length: uint64(len(data))}}}}
	if err := db.ImportMetadata(ctx, pending); err != nil {
		t.Fatal(err)
	}
	root, err := db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	res, err := db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{DirPath: "collision"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].ContentState != "pending" || res.Items[0].StructuralConflict == "" {
		t.Fatalf("pending structural row: %+v", res)
	}
	details, err := db.FileDetails(ctx, folder, pending.Path)
	if err != nil {
		t.Fatal(err)
	}
	if details.WorkingState != "conflict" || details.Heads[0].CASAvailable {
		t.Fatalf("pending details: %+v", details)
	}
	unicode, err := db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{DirPath: "日本"})
	if err != nil || len(unicode.Items) != 1 || !unicode.Items[0].IsDir {
		t.Fatalf("unicode explicit directory: %+v %v", unicode, err)
	}
	for _, path := range []string{"/日本", "日本/../日本", "日本//empty", "日本/", "../collision", ".filesync-internal"} {
		if _, err := db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{DirPath: path}); !errors.Is(err, ErrInvalidDirectoryPath) {
			t.Fatalf("accepted invalid path %q: %v", path, err)
		}
		if _, err := db.FileDetails(ctx, folder, path); !errors.Is(err, ErrInvalidDirectoryPath) {
			t.Fatalf("accepted invalid detail path %q: %v", path, err)
		}
	}
	before, err := db.headsUnlocked(ctx, folder, "collision")
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 || before[0] != old.ID {
		t.Fatal("browse authored a directory")
	}
	installTestVersion(t, db, folder, "new.txt", history.KindFile, []byte("new"))
	if _, err := db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{Cursor: root.NextCursor}); !errors.Is(err, ErrStaleCursor) {
		t.Fatalf("received/captured change did not stale cursor: %v", err)
	}
}

func TestOrbitBrowse_CursorValidationAndStableTies(t *testing.T) {
	db, folder, _, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	for _, path := range []string{"a.txt", "A.txt", "b.txt", "B.txt"} {
		installTestVersion(t, db, folder, path, history.KindFile, []byte("same size"))
	}
	for _, sort := range []string{"size", "mtime", "kind", "name"} {
		seen := map[string]bool{}
		cursor := ""
		for {
			res, err := db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{Limit: 1, SortBy: sort, SortDir: "desc", Cursor: cursor})
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range res.Items {
				if seen[item.Path] {
					t.Fatalf("repeated %s in %s", item.Path, sort)
				}
				seen[item.Path] = true
			}
			cursor = res.NextCursor
			if cursor == "" {
				break
			}
		}
		if len(seen) != 4 {
			t.Fatal(seen)
		}
	}
	res, err := db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	cur, err := decodeCursor(res.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	cur.Offset = -1
	raw, _ := json.Marshal(cur)
	if _, err := db.BrowseWorkspaceDirectory(ctx, folder, BrowseOptions{Cursor: base64.RawURLEncoding.EncodeToString(raw)}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("negative cursor accepted: %v", err)
	}
	if _, err := db.SearchWorkspace(ctx, folder, SearchOptions{Query: strings.Repeat("x", 257)}); err == nil {
		t.Fatal("unbounded search query")
	}
	if _, err := db.SearchWorkspace(ctx, folder, SearchOptions{Query: "same", Cursor: res.NextCursor}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatal("browse cursor reused for search")
	}
}

func TestOrbitReadLease_GCStreamingExpiryReleaseAndRestart(t *testing.T) {
	db, folder, _, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	payload := bytes.Repeat([]byte("0123456789abcdef"), int(history.ChunkSize)/16)
	manifest, err := db.StoreFile(ctx, io.LimitReader(&repeatReader{data: payload}, 32*int64(history.ChunkSize)), false)
	if err != nil {
		t.Fatal(err)
	}
	env, err := db.CreateLocalVersion(ctx, LocalVersionRequest{Folder: folder, Path: "large.bin", Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	installTestVersion(t, db, folder, "large.bin", history.KindFile, []byte("successor"))
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	read, err := db.OpenVersionRead(ctx, env.ID)
	if err != nil {
		t.Fatal(err)
	}
	// TTL expiry of the durable read record cannot expire a live response pin.
	if _, err := db.PruneExpiredReadLeases(ctx, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	policy := RetentionPolicy{RetentionDays: 0, MinSuperseded: 0}
	report, err := db.RunGC(ctx, folder, &policy, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if report.UnlinkedObjects != 0 {
		t.Fatal("GC unlinked active read", report)
	}
	hash := sha256.New()
	n, err := io.Copy(hash, read)
	if err != nil || n != int64(manifest.Size) || !equalHash(hash, manifest.Digest) {
		t.Fatalf("stream bytes: %d %v", n, err)
	}
	runtime.ReadMemStats(&after)
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("32 MiB repeated-chunk read+GC total allocations: %d bytes", allocated)
	if allocated > 8<<20 {
		t.Fatalf("whole-file allocation: %d", allocated)
	}
	if err := read.Close(); err != nil {
		t.Fatal(err)
	}
	if err := read.Close(); err != nil {
		t.Fatal(err)
	}
	report, err = db.RunGC(ctx, folder, &policy, time.Now().Add(time.Hour))
	if err != nil || report.UnlinkedObjects != 1 {
		t.Fatalf("released bytes not collectible: %+v %v", report, err)
	}
	if _, err := db.OpenVersionRead(ctx, env.ID); err == nil {
		t.Fatal("substituted successor for expired history")
	}
	// Open a new stream, simulate process abandonment by closing the repository,
	// then reopen and verify only abandoned stream pins are released.
	current := installTestVersion(t, db, folder, "restart.bin", history.KindFile, []byte("restart"))
	abandoned, err := db.OpenVersionRead(ctx, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	stateDir := db.StateDir()
	db.Close()
	reopened, err := Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var pins int
	if err := reopened.db.QueryRow(`SELECT COUNT(*) FROM content_pins WHERE owner_kind='stream'`).Scan(&pins); err != nil {
		t.Fatal(err)
	}
	if pins != 0 {
		t.Fatalf("abandoned pins retained: %d", pins)
	}
	_ = abandoned // A terminated process cannot call Close on its old repository.
}

type repeatReader struct {
	data   []byte
	offset int
}

func (r *repeatReader) Read(p []byte) (int, error) {
	if r.offset == len(r.data) {
		r.offset = 0
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

func TestOrbitContent_CorruptionCancellationAndIntent(t *testing.T) {
	db, folder, _, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	env := installTestVersion(t, db, folder, "bytes.txt", history.KindFile, []byte("correct content"))
	chunk := env.Manifest.Chunks[0]
	_, err := db.db.Exec(`INSERT INTO gc_intents(digest,generation,state) VALUES(?,1,'intent')`, chunk.Digest[:])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.OpenVersionRead(ctx, env.ID); !errors.Is(err, ErrGCIntentActive) {
		t.Fatalf("admitted intent: %v", err)
	}
	if err := db.AcquireReadLease(ctx, ReadLeaseRecord{LeaseID: "unsafe", Folder: folder, VersionAuthor: env.ID.Author, VersionCounter: env.ID.Counter, ExpiresNS: time.Now().Add(time.Minute).UnixNano(), ChunkDigests: []history.Digest{chunk.Digest}}); !errors.Is(err, ErrGCIntentActive) {
		t.Fatalf("raw lease admitted intent: %v", err)
	}
	db.db.Exec(`DELETE FROM gc_intents`)
	cancelled, cancel := context.WithCancel(ctx)
	read, err := db.OpenVersionRead(cancelled, env.ID)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := read.Read(make([]byte, 1)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	read.Close()
	if err := os.WriteFile(db.objectPath(chunk.Digest), []byte("corrupt content"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.OpenVersionRead(ctx, env.ID); !errors.Is(err, ErrContentMismatch) {
		t.Fatalf("corrupt read succeeded: %v", err)
	}
	state, err := db.ContentAvailability(ctx, env.ID)
	if err != nil || state == ContentReady {
		t.Fatalf("corruption not diagnosed: %s %v", state, err)
	}
	var pins int
	db.db.QueryRow(`SELECT COUNT(*) FROM content_pins WHERE owner_kind='stream'`).Scan(&pins)
	if pins != 0 {
		t.Fatal("failed/canceled read leaked pins")
	}
}

func TestOrbitBrowse_HistoryPaginationAndConditionalDeletedAvailability(t *testing.T) {
	db, folder, _, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	first := installTestVersion(t, db, folder, "history.txt", history.KindFile, []byte("first"))
	installTestVersion(t, db, folder, "history.txt", history.KindFile, []byte("second"))
	installTestVersion(t, db, folder, "history.txt", history.KindTombstone, nil)
	page, err := db.BrowsePathHistory(ctx, folder, "history.txt", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Kind != history.KindTombstone || !page.Items[0].IsHead || page.NextCursor == "" {
		t.Fatal(page)
	}
	next, err := db.BrowsePathHistory(ctx, folder, "history.txt", page.NextCursor, 1)
	if err != nil || next.Items[0].Kind != history.KindFile {
		t.Fatalf("history second page %+v %v", next, err)
	}
	if _, err := db.BrowsePathHistory(ctx, folder, "another", page.NextCursor, 1); !errors.Is(err, ErrInvalidCursor) {
		t.Fatal("path cursor was reused")
	}
	details, err := db.FileDetails(ctx, folder, "history.txt")
	if err != nil || details.Kind != history.KindTombstone {
		t.Fatalf("deleted details %+v %v", details, err)
	}
	deleted, err := db.BrowseDeletedFiles(ctx, folder, "", 10)
	if err != nil || len(deleted.Items) != 1 || !deleted.Items[0].CASAvailable {
		t.Fatalf("prior bytes %+v %v", deleted, err)
	}
	policy := RetentionPolicy{RetentionDays: 0, MinSuperseded: 0}
	if _, err := db.RunGC(ctx, folder, &policy, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	deleted, err = db.BrowseDeletedFiles(ctx, folder, "", 10)
	if err != nil || deleted.Items[0].CASAvailable {
		t.Fatalf("metadata implied byte retention %+v %v", deleted, err)
	}
	if _, err := db.OpenVersionRead(ctx, first.ID); err == nil {
		t.Fatal("read substituted expired history")
	}
	if _, err := db.BrowsePathHistory(ctx, folder, "history.txt", page.NextCursor, 1); !errors.Is(err, ErrStaleCursor) {
		t.Fatal("GC did not invalidate availability cursor", err)
	}
}
