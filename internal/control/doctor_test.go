package control

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/workspace"
)

func TestDoctorDiagnostics(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ws := workspace.New(db, workspace.Options{})
	ctrl := New(db, ws)

	report, err := ctrl.Doctor(ctx)
	if err != nil {
		t.Fatalf("doctor failed: %v", err)
	}

	if report.OverallStatus == "" {
		t.Errorf("expected non-empty overall status")
	}

	categoriesSeen := make(map[string]bool)
	for _, ch := range report.Checks {
		if ch.Name == "" || ch.Category == "" || ch.Status == "" || ch.Message == "" {
			t.Errorf("incomplete check: %+v", ch)
		}
		categoriesSeen[ch.Category] = true
	}

	for _, expectedCat := range []string{"permissions", "roots", "storage", "protocol", "recovery"} {
		if !categoriesSeen[expectedCat] {
			t.Errorf("missing doctor category: %s", expectedCat)
		}
	}

	// Register a real folder with workspace and check that Doctor sees it
	root := filepath.Join(t.TempDir(), "test-root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	var folder history.ID
	folder[0] = 0x42
	var localAuthor history.ID
	localAuthor[0] = 0x01
	if err := db.EnsureFolder(ctx, folder, localAuthor, 1); err != nil {
		t.Fatalf("ensure folder: %v", err)
	}
	if _, err := ws.Register(ctx, folder, root); err != nil {
		t.Fatalf("register folder: %v", err)
	}

	report2, err := ctrl.Doctor(ctx)
	if err != nil {
		t.Fatalf("doctor after register failed: %v", err)
	}

	foundRootCheck := false
	for _, ch := range report2.Checks {
		if ch.Category == "roots" && ch.Status == StatusOk {
			foundRootCheck = true
			break
		}
	}
	if !foundRootCheck {
		t.Errorf("expected root check to pass for registered root")
	}
}
