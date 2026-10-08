package control

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
)

func e08Remote(t *testing.T, env *testEnv, path string, counter uint64, kind history.Kind, content []byte, install bool, parents ...history.VersionID) history.Envelope {
	t.Helper()
	ctx := context.Background()
	var m *history.Manifest
	if kind == history.KindFile {
		m = fileManifest(content, false)
		if install {
			if err := env.db.InstallChunk(ctx, m.Chunks[0].Digest, m.Chunks[0].Length, bytes.NewReader(content)); err != nil {
				t.Fatal(err)
			}
		}
	}
	vector := []history.ClockEntry{{Author: env.authorB, Counter: counter}}
	for _, p := range parents {
		if p.Author != env.authorB {
			vector = append([]history.ClockEntry{{Author: p.Author, Counter: p.Counter}}, vector...)
		}
	}
	e := history.Envelope{ID: history.VersionID{Folder: env.folder, Author: env.authorB, Counter: counter}, Path: path, Parents: parents,
		Vector: vector, Kind: kind, Manifest: m, AuthoredRevision: 1, DisplayTime: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := env.db.ImportMetadata(ctx, e); err != nil {
		t.Fatal(err)
	}
	if install || kind != history.KindFile {
		if err := env.db.MarkContentReady(ctx, e.ID); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func e08Env(t *testing.T) *testEnv {
	t.Helper()
	env := setupTestEnv(t)
	if err := config.Save(env.stateDir, config.Config{FormatVersion: config.FormatVersion, DeviceID: hex.EncodeToString(env.authorA[:]), CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	return env
}

func e08Query(t *testing.T, env *testEnv, q tc.Query) tc.Result {
	t.Helper()
	q.Version = tc.Version
	q.Folder = fmt.Sprintf("%x", env.folder[:])
	r, err := env.ctrl.TerminalQuery(context.Background(), q)
	if err != nil {
		t.Fatalf("%s %q: %v", q.Kind, q.Path, err)
	}
	return r
}

func e08States(t *testing.T, env *testEnv, dir string) map[string]string {
	t.Helper()
	got := map[string]string{}
	for _, f := range e08Query(t, env, tc.Query{Kind: "files", Path: dir, Limit: 200}).Files {
		got[f.Path] = f.State
	}
	return got
}

// EG2: each state comes from a production capture, import, publication or
// integrity path and maps to exactly one label.
func TestOnboardingE08FileStatesFromProductionPaths(t *testing.T) {
	env := e08Env(t)
	ctx := context.Background()
	write := func(path, content string) {
		full := filepath.Join(env.rootDir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("captured.txt", "here")
	write("docs/notes.txt", "notes")
	write("edited.txt", "first")
	if err := os.MkdirAll(filepath.Join(env.rootDir, ".orbit-internal", "probe"), 0o700); err != nil {
		t.Fatal(err)
	}
	scan, err := env.ws.Scan(ctx, env.folder)
	if err != nil {
		t.Fatal(err)
	}
	// A remote update arrives over a local edit that no scan has captured:
	// publication refuses to overwrite it and records a block.
	var edited history.Envelope
	for _, e := range scan.Captured {
		if e.Path == "edited.txt" {
			edited = e
		}
	}
	write("edited.txt", "second, not yet captured")
	update := e08Remote(t, env, "edited.txt", 16, history.KindFile, []byte("remote update"), true, edited.ID)
	_ = env.ws.Apply(ctx, update.ID)

	waiting := e08Remote(t, env, "arrived.txt", 11, history.KindFile, []byte("from B"), true)
	e08Remote(t, env, "downloading.bin", 12, history.KindFile, []byte("not yet here"), false)
	missing := e08Remote(t, env, "missing.txt", 13, history.KindFile, []byte("will be quarantined"), true)
	if _, err := env.db.QuarantineChunk(ctx, missing.Manifest.Chunks[0].Digest, "fixture"); err != nil {
		t.Fatal(err)
	}
	createConflict(t, env, "both.txt", []byte("mine"), []byte("theirs"), false, false)
	gone := e08Remote(t, env, "gone.txt", 14, history.KindFile, []byte("short-lived"), true)
	e08Remote(t, env, "gone.txt", 15, history.KindTombstone, nil, false, gone.ID)

	want := map[string]string{
		"captured.txt":    repository.SyncCaptured,
		"docs":            repository.SyncCaptured,
		"arrived.txt":     repository.SyncWaitingPublish,
		"downloading.bin": repository.SyncDownloading,
		"missing.txt":     repository.SyncContentMissing,
		"both.txt":        repository.SyncConflict,
		"edited.txt":      repository.SyncBlocked,
	}
	got := e08States(t, env, "")
	for path, state := range want {
		if got[path] != state {
			t.Errorf("%s: state %q, want %q (all %v)", path, got[path], state, got)
		}
	}
	for path := range got {
		if _, ok := want[path]; !ok {
			t.Errorf("unexpected listed path %q (internal, deleted or unknown)", path)
		}
	}
	if s := e08States(t, env, "docs")["docs/notes.txt"]; s != repository.SyncCaptured {
		t.Errorf("docs/notes.txt: %q", s)
	}
	if d := e08Query(t, env, tc.Query{Kind: "file_details", Path: "gone.txt"}).File; d == nil || d.Entry.State != repository.SyncDeleted {
		t.Errorf("deleted details: %+v", d)
	}

	// Publication moves the remote version into the working copy.
	if err := env.ws.Apply(ctx, waiting.ID); err != nil {
		t.Fatal(err)
	}
	if s := e08States(t, env, "")["arrived.txt"]; s != repository.SyncCaptured {
		t.Errorf("arrived.txt after publication: %q", s)
	}
}

// EG2: another device's report keeps its own time; an old report is never
// shown as current.
func TestOnboardingE08ObservationKeepsItsAge(t *testing.T) {
	env := e08Env(t)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(env.rootDir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	scan, err := env.ws.Scan(ctx, env.folder)
	if err != nil || len(scan.Captured) == 0 {
		t.Fatalf("scan %v %+v", err, scan)
	}
	old := time.Now().Add(-3 * time.Hour)
	if err := env.db.RecordPeerStatus(ctx, env.folder, env.authorB, scan.Captured[0].ID, "APPLIED", old); err != nil {
		t.Fatal(err)
	}
	d := e08Query(t, env, tc.Query{Kind: "file_details", Path: "a.txt"}).File
	if d == nil || len(d.Observations) != 1 {
		t.Fatalf("details %+v", d)
	}
	o := d.Observations[0]
	at, err := time.Parse(time.RFC3339, o.ObservedAt)
	if err != nil || time.Since(at) < 2*time.Hour || o.Online || !o.Applied || !o.Stored {
		t.Fatalf("observation %+v", o)
	}
	if d.LastChecked == "" || len(d.Heads) != 1 {
		t.Fatalf("detail %+v", d)
	}
}

// 10,000 entries in one directory page through the files query in bounded
// pages; every entry appears exactly once.
func TestOnboardingE08PagesTenThousandEntries(t *testing.T) {
	env := e08Env(t)
	ctx := context.Background()
	m, err := env.db.StoreFile(ctx, bytes.NewReader([]byte("x")), false)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10000; i++ {
		if _, err := env.db.CreateLocalVersion(ctx, repository.LocalVersionRequest{Folder: env.folder, Path: fmt.Sprintf("big/f%05d", i), Kind: history.KindFile, Manifest: m, AuthoredRevision: 1}); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	cursor, pages := "", 0
	start := time.Now()
	for {
		r := e08Query(t, env, tc.Query{Kind: "files", Path: "big", Limit: 200, Cursor: cursor})
		pages++
		if len(r.Files) > 200 {
			t.Fatalf("page of %d", len(r.Files))
		}
		for _, f := range r.Files {
			if seen[f.Path] {
				t.Fatalf("%s twice", f.Path)
			}
			seen[f.Path] = true
		}
		if cursor = r.Cursor; cursor == "" {
			break
		}
	}
	t.Logf("paged 10,000 entries in %d pages, %v per page", pages, time.Since(start)/time.Duration(pages))
	if len(seen) != 10000 || pages != 50 {
		t.Fatalf("%d entries in %d pages", len(seen), pages)
	}
	s := e08Query(t, env, tc.Query{Kind: "files", Name: "f0999", Limit: 50}).Files
	if len(s) != 10 {
		t.Fatalf("search found %d", len(s))
	}
}
