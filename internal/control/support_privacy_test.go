package control

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

func TestWANW12SupportRedactsNestedErrorsAndCredentials(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, e := repository.Open(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	ws := workspace.New(db, workspace.Options{})
	c := New(db, ws)
	secret := "synthetic-invitation-ice-relay-secret"
	root := filepath.Join(t.TempDir(), "private-filename.secret-extension")
	if e = os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	folder, author := history.ID{1}, history.ID{2}
	if e = db.EnsureFolder(ctx, folder, author, 1); e != nil {
		t.Fatal(e)
	}
	if _, e = ws.Register(ctx, folder, root); e != nil {
		t.Fatal(e)
	}
	if e = db.PauseFolder(ctx, folder, "permission "+root+" "+secret); e != nil {
		t.Fatal(e)
	}
	_, e = db.EnqueueDurableTask(ctx, repository.DurableTask{Folder: folder, Kind: "scan", State: "retry", TargetPath: "private-filename.secret-extension", LastError: "read " + root + " " + secret, ErrorCode: "failure " + secret, MaxAttempts: 3})
	if e != nil {
		t.Fatal(e)
	}
	if e = db.RecordEvent(ctx, repository.EventLogEntry{Phase: "failure " + secret, ErrorCode: "error " + secret}); e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(map[string]any{"format_version": 1, "device_id": strings.Repeat("a", 64), "created_at": "2026-10-06T00:00:00Z", "nested": map[string]string{"password": secret}})
	if e = os.WriteFile(filepath.Join(dir, "config.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	archive := filepath.Join(t.TempDir(), "bundle.tar.gz")
	result, e := c.ExportSupportBundle(ctx, SupportExportRequest{DestinationPath: archive, RedactPaths: true})
	if e != nil {
		t.Fatal(e)
	}
	f, e := os.Open(archive)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	gr, e := gzip.NewReader(f)
	if e != nil {
		t.Fatal(e)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		header, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		b, e := io.ReadAll(tr)
		if e != nil {
			t.Fatal(e)
		}
		for _, private := range []string{secret, "private-filename", "secret-extension", root, dir} {
			if strings.Contains(string(b), private) {
				t.Errorf("%s leaked seeded private text", header.Name)
			}
		}
	}
	if result.RedactionCount == 0 {
		t.Fatal("no redaction")
	}
	if _, e = c.ExportSupportBundle(ctx, SupportExportRequest{DestinationPath: archive, RedactPaths: true}); e == nil {
		t.Fatal("existing archive overwritten")
	}
}
