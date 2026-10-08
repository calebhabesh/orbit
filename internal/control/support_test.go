package control

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/workspace"
)

func TestSupportExport(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ws := workspace.New(db, workspace.Options{})
	ctrl := New(db, ws)

	// Register a folder with a sensitive path
	root := filepath.Join(t.TempDir(), "super_secret_project_directory")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var folder history.ID
	folder[0] = 0xAA
	var author history.ID
	author[0] = 0xBB
	_ = db.EnsureFolder(ctx, folder, author, 1)
	_, _ = ws.Register(ctx, folder, root)

	// 1. Preview
	preview, err := ctrl.PreviewSupportExport(ctx, true)
	if err != nil {
		t.Fatalf("preview support: %v", err)
	}
	if len(preview.Categories) == 0 || preview.EstimatedSize == 0 {
		t.Errorf("empty preview: %+v", preview)
	}

	// 2. Export with path redaction
	archivePath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	res, err := ctrl.ExportSupportBundle(ctx, SupportExportRequest{
		DestinationPath: archivePath,
		RedactPaths:     true,
	})
	if err != nil {
		t.Fatalf("export support bundle: %v", err)
	}
	if res.TotalFiles != 9 {
		t.Errorf("expected 9 archive files, got %d", res.TotalFiles)
	}
	if !res.RedactedPaths || res.RedactionCount == 0 {
		t.Errorf("expected paths to be redacted, got count=%d", res.RedactionCount)
	}

	// 3. Inspect archive contents
	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	archiveFiles := make(map[string][]byte)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read tar file %s: %v", hdr.Name, err)
		}
		archiveFiles[hdr.Name] = data
	}

	// Verify all 7 files are in archive
	for _, expectedFile := range []string{"doctor.json", "system.json", "config.json", "membership.json", "storage.json", "work.json", "events.json"} {
		data, exists := archiveFiles[expectedFile]
		if !exists {
			t.Errorf("missing %s in archive", expectedFile)
			continue
		}
		content := string(data)
		// Crucial security checks:
		if strings.Contains(content, "super_secret_project_directory") {
			t.Errorf("file %s contained unredacted sensitive path!", expectedFile)
		}
		if strings.Contains(content, "BEGIN EC PRIVATE KEY") || strings.Contains(content, "BEGIN PRIVATE KEY") || strings.Contains(content, "BEGIN OPENSSH PRIVATE KEY") {
			t.Errorf("file %s contained private key material!", expectedFile)
		}
	}
}
