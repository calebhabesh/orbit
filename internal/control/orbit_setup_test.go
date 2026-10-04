package control_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

func TestOrbitSetup_InspectSetup(t *testing.T) {
	stateDir := testkit.NewDisposable(t)
	ctx := context.Background()

	// 1. Uninitialized setup
	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := workspace.New(db, workspace.Options{})
	ctrl := control.New(db, ws)

	insp, err := ctrl.InspectSetup(ctx)
	if err != nil {
		t.Fatalf("InspectSetup failed: %v", err)
	}
	if insp.Initialized {
		t.Errorf("expected Initialized=false before config.Save")
	}
	if insp.SetupCompleted {
		t.Errorf("expected SetupCompleted=false initially")
	}
	if insp.RegisteredCount != 0 {
		t.Errorf("expected RegisteredCount=0, got %d", insp.RegisteredCount)
	}

	// 2. Initialized device config
	rawID := make([]byte, 32)
	rand.Read(rawID)
	devID := hex.EncodeToString(rawID)
	if err := config.Save(stateDir, config.Config{FormatVersion: 1, DeviceID: devID, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	insp2, err := ctrl.InspectSetup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !insp2.Initialized {
		t.Errorf("expected Initialized=true after config.Save")
	}
	if insp2.DeviceID != devID {
		t.Errorf("expected DeviceID=%s, got %s", devID, insp2.DeviceID)
	}
	if insp2.SuggestedRoot == "" {
		t.Errorf("expected non-empty SuggestedRoot")
	}
}

func TestOrbitSetup_PreviewCreateAndJoinRoot(t *testing.T) {
	ctrl, db, stateDir, cleanup := setupTestController(t)
	defer cleanup()
	ctx := context.Background()

	// Case 1: Empty path
	_, err := ctrl.PreviewCreateRoot(ctx, control.PreviewCreateRootRequest{Path: ""})
	if err == nil {
		t.Fatal("expected error for empty root path")
	}

	// Case 2: State directory
	prev, err := ctrl.PreviewCreateRoot(ctx, control.PreviewCreateRootRequest{Path: stateDir})
	if err != nil {
		t.Fatal(err)
	}
	if !prev.Disallowed {
		t.Errorf("expected state directory to be disallowed")
	}

	// Case 3: System root directory
	prevSys, err := ctrl.PreviewCreateRoot(ctx, control.PreviewCreateRootRequest{Path: "/etc"})
	if err != nil {
		t.Fatal(err)
	}
	if !prevSys.Disallowed {
		t.Errorf("expected /etc to be disallowed")
	}

	// Case 4: Non-existent personal directory
	personalDir := filepath.Join(filepath.Dir(stateDir), "MyOrbitFolder")
	prevPersonal, err := ctrl.PreviewCreateRoot(ctx, control.PreviewCreateRootRequest{Path: personalDir})
	if err != nil {
		t.Fatal(err)
	}
	if prevPersonal.Disallowed {
		t.Errorf("expected personal directory to be allowed, got disallowed: %s", prevPersonal.Reason)
	}
	if prevPersonal.Exists {
		t.Errorf("expected exists=false for new directory")
	}
	if !prevPersonal.Writable {
		t.Errorf("expected writable=true in test directory")
	}

	// Case 5: Preexisting files in directory
	existingDir := filepath.Join(filepath.Dir(stateDir), "ExistingFilesFolder")
	if err := os.MkdirAll(existingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(existingDir, "note1.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(existingDir, "note2.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	prevExist, err := ctrl.PreviewCreateRoot(ctx, control.PreviewCreateRootRequest{Path: existingDir})
	if err != nil {
		t.Fatal(err)
	}
	if !prevExist.Exists {
		t.Errorf("expected exists=true")
	}
	if prevExist.IsEmpty {
		t.Errorf("expected is_empty=false")
	}
	if prevExist.PreexistingRows != 2 {
		t.Errorf("expected 2 preexisting rows, got %d", prevExist.PreexistingRows)
	}
	if len(prevExist.ExistingSamples) != 2 {
		t.Errorf("expected 2 samples, got %d", len(prevExist.ExistingSamples))
	}

	// Case 6: Overlapping roots rejection
	var fID history.ID
	rand.Read(fID[:])
	if _, err := ctrl.RegisterFolder(ctx, fID, existingDir); err != nil {
		t.Fatal(err)
	}

	// Attempt to register the exact same directory
	prevDup, err := ctrl.PreviewCreateRoot(ctx, control.PreviewCreateRootRequest{Path: existingDir})
	if err != nil {
		t.Fatal(err)
	}
	if !prevDup.Disallowed {
		t.Errorf("expected duplicate root registration to be disallowed")
	}

	// Attempt to register child directory
	childDir := filepath.Join(existingDir, "subfolder")
	prevChild, err := ctrl.PreviewCreateRoot(ctx, control.PreviewCreateRootRequest{Path: childDir})
	if err != nil {
		t.Fatal(err)
	}
	if !prevChild.Disallowed {
		t.Errorf("expected nested child root to be disallowed")
	}

	// Case 7: PreviewJoinRoot
	var joinFolderID history.ID
	rand.Read(joinFolderID[:])
	joinPath := filepath.Join(filepath.Dir(stateDir), "JoinFolder")
	joinPrev, err := ctrl.PreviewJoinRoot(ctx, control.PreviewJoinRootRequest{
		Path:     joinPath,
		FolderID: joinFolderID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if joinPrev.Disallowed {
		t.Errorf("expected join root to be allowed: %s", joinPrev.Reason)
	}
	if joinPrev.FolderID != joinFolderID {
		t.Errorf("expected folder ID match")
	}
	_ = db
}

func TestOrbitSetup_StartAndResumeSetup_PreservesPreexistingFiles(t *testing.T) {
	ctrl, db, stateDir, cleanup := setupTestController(t)
	defer cleanup()
	ctx := context.Background()

	// 1. Prepare directory with preexisting user files (Invariant I22)
	rootPath := filepath.Join(filepath.Dir(stateDir), "UserDocuments")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	fileA := filepath.Join(rootPath, "document.pdf")
	fileB := filepath.Join(rootPath, "notes.txt")
	contentA := []byte("%PDF-1.4 simulated user content")
	contentB := []byte("important thoughts and meetings")
	if err := os.WriteFile(fileA, contentA, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, contentB, 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. Execute StartSetup
	setupRes, err := ctrl.StartSetup(ctx, control.StartSetupRequest{
		RootPath:      rootPath,
		DeviceLabel:   "Laptop-Primary",
		WorkspaceName: "Documents Workspace",
	})
	if err != nil {
		t.Fatalf("StartSetup failed: %v", err)
	}

	if setupRes.Phase != "completed" {
		t.Errorf("expected phase 'completed', got %q", setupRes.Phase)
	}
	if setupRes.FolderID == ([32]byte{}) {
		t.Fatal("expected non-zero FolderID")
	}

	// 3. Verify Invariant I22: Preexisting user files are strictly PRESERVED on disk
	readA, err := os.ReadFile(fileA)
	if err != nil || string(readA) != string(contentA) {
		t.Fatalf("preexisting fileA corrupted or missing: %v", err)
	}
	readB, err := os.ReadFile(fileB)
	if err != nil || string(readB) != string(contentB) {
		t.Fatalf("preexisting fileB corrupted or missing: %v", err)
	}

	// 4. Verify capture: Captured versions exist in database
	hasCaptured, err := db.HasAnyCapturedVersions(ctx)
	if err != nil || !hasCaptured {
		t.Fatalf("expected versions captured in database: err=%v, hasCaptured=%v", err, hasCaptured)
	}

	// 5. Verify settings updated
	settings, err := config.LoadSettings(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if settings.DeviceLabel != "Laptop-Primary" {
		t.Errorf("expected device label Laptop-Primary, got %q", settings.DeviceLabel)
	}
	folderHex := hex.EncodeToString(setupRes.FolderID[:])
	if settings.DefaultWorkspace != folderHex {
		t.Errorf("expected default workspace %s, got %s", folderHex, settings.DefaultWorkspace)
	}
	if settings.WorkspaceNames[folderHex] != "Documents Workspace" {
		t.Errorf("expected workspace name 'Documents Workspace', got %q", settings.WorkspaceNames[folderHex])
	}

	// 6. Verify InspectSetup reflects completion
	insp, err := ctrl.InspectSetup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !insp.SetupCompleted {
		t.Errorf("expected SetupCompleted=true")
	}
	if insp.RegisteredCount != 1 {
		t.Errorf("expected RegisteredCount=1, got %d", insp.RegisteredCount)
	}

	// 7. Verify ResumeSetup on completed setup returns completed
	resumed, err := ctrl.ResumeSetup(ctx, control.ResumeSetupRequest{})
	if err != nil {
		t.Fatalf("ResumeSetup failed: %v", err)
	}
	if !resumed.Completed {
		t.Errorf("expected resumed.Completed=true")
	}
}

func TestOrbitSetup_DirectoryPicker(t *testing.T) {
	ctrl, _, stateDir, cleanup := setupTestController(t)
	defer cleanup()
	ctx := context.Background()

	// Create test directory tree
	baseDir := filepath.Join(filepath.Dir(stateDir), "PickerTest")
	subDirs := []string{"Alpha", "Beta", "Gamma", "Delta", "Epsilon"}
	for _, s := range subDirs {
		if err := os.MkdirAll(filepath.Join(baseDir, s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Add a regular file (should be ignored by directory picker)
	_ = os.WriteFile(filepath.Join(baseDir, "file.txt"), []byte("data"), 0o644)

	// Page 1: limit 2, offset 0
	page1, err := ctrl.BrowseDirectories(ctx, control.DirectoryPickerRequest{
		Path:   baseDir,
		Limit:  2,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("BrowseDirectories page 1 failed: %v", err)
	}
	if page1.TotalEntries != 5 {
		t.Errorf("expected TotalEntries=5, got %d", page1.TotalEntries)
	}
	if len(page1.Entries) != 2 {
		t.Errorf("expected 2 entries on page 1, got %d", len(page1.Entries))
	}
	if !page1.HasMore {
		t.Errorf("expected HasMore=true for page 1")
	}
	if !page1.Writable {
		t.Errorf("expected Writable=true")
	}

	// Page 2: limit 2, offset 2
	page2, err := ctrl.BrowseDirectories(ctx, control.DirectoryPickerRequest{
		Path:   baseDir,
		Limit:  2,
		Offset: 2,
	})
	if err != nil {
		t.Fatalf("BrowseDirectories page 2 failed: %v", err)
	}
	if len(page2.Entries) != 2 {
		t.Errorf("expected 2 entries on page 2, got %d", len(page2.Entries))
	}
	if !page2.HasMore {
		t.Errorf("expected HasMore=true for page 2")
	}

	// Page 3: limit 2, offset 4
	page3, err := ctrl.BrowseDirectories(ctx, control.DirectoryPickerRequest{
		Path:   baseDir,
		Limit:  2,
		Offset: 4,
	})
	if err != nil {
		t.Fatalf("BrowseDirectories page 3 failed: %v", err)
	}
	if len(page3.Entries) != 1 {
		t.Errorf("expected 1 entry on page 3, got %d", len(page3.Entries))
	}
	if page3.HasMore {
		t.Errorf("expected HasMore=false for page 3")
	}
}

func TestOrbitSetup_OpenLocalFolder(t *testing.T) {
	// Never launch the developer's real desktop against a disposable test root.
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	ctrl, _, stateDir, cleanup := setupTestController(t)
	defer cleanup()
	ctx := context.Background()

	// Register a folder
	var fID history.ID
	rand.Read(fID[:])
	regDir := filepath.Join(filepath.Dir(stateDir), "RegisteredFolder")
	if err := os.MkdirAll(regDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.RegisterFolder(ctx, fID, regDir); err != nil {
		t.Fatal(err)
	}

	// Negative case 1: Request opening an unregistered path outside workspace
	_, err := ctrl.OpenLocalFolder(ctx, control.OpenFolderRequest{Path: "/tmp"})
	if err == nil {
		t.Fatal("expected error opening unregistered path")
	}

	// A registered folder in an explicitly headless test must fail cleanly.
	_, err = ctrl.OpenLocalFolder(ctx, control.OpenFolderRequest{Folder: hex.EncodeToString(fID[:])})
	ctrlErr, ok := err.(*control.ControlError)
	if !ok || ctrlErr.Code != "DESKTOP_HELPER_UNAVAILABLE" {
		t.Fatalf("expected headless desktop helper refusal, got %v", err)
	}
}

func TestOrbitSetup_InterruptedSetupResume(t *testing.T) {
	ctrl, db, stateDir, cleanup := setupTestController(t)
	defer cleanup()
	ctx := context.Background()

	// 1. Simulate an interrupted setup: root directory exists with files,
	// setup_state record is written with completed=false and phase="registering_folder"
	var folderID history.ID
	rand.Read(folderID[:])
	rootPath := filepath.Join(filepath.Dir(stateDir), "InterruptedFolder")
	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	sampleFile := filepath.Join(rootPath, "draft.txt")
	sampleContent := []byte("important document in progress")
	if err := os.WriteFile(sampleFile, sampleContent, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := db.SaveSetupState(ctx, repository.SetupStateRecord{
		Phase:           "registering_folder",
		RootPath:        rootPath,
		DefaultFolderID: &folderID,
		Completed:       false,
		UpdatedNS:       time.Now().UnixNano(),
	}); err != nil {
		t.Fatal(err)
	}

	// 2. Verify InspectSetup reports setup is not completed and in registering_folder phase
	insp, err := ctrl.InspectSetup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if insp.SetupCompleted {
		t.Errorf("expected SetupCompleted=false for interrupted setup")
	}
	if insp.CurrentPhase != "registering_folder" {
		t.Errorf("expected CurrentPhase=registering_folder, got %s", insp.CurrentPhase)
	}

	// 3. Resume setup
	resumeRes, err := ctrl.ResumeSetup(ctx, control.ResumeSetupRequest{})
	if err != nil {
		t.Fatalf("ResumeSetup failed: %v", err)
	}
	if !resumeRes.Completed {
		t.Errorf("expected Completed=true after resume")
	}
	if resumeRes.Phase != "completed" {
		t.Errorf("expected Phase=completed, got %s", resumeRes.Phase)
	}
	if resumeRes.FolderID != folderID {
		t.Errorf("expected FolderID=%x, got %x", folderID, resumeRes.FolderID)
	}

	// 4. Verify post-resume state: folder is registered, file was captured without data loss (Invariant I22)
	registered, err := db.RegisteredFolders(ctx)
	if err != nil || len(registered) != 1 {
		t.Fatalf("expected 1 registered folder, got %d (%v)", len(registered), err)
	}
	if registered[0].Folder != folderID {
		t.Errorf("expected registered folder %x, got %x", folderID, registered[0].Folder)
	}

	hasCaptured, err := db.HasAnyCapturedVersions(ctx)
	if err != nil || !hasCaptured {
		t.Errorf("expected captured versions after resumed scan")
	}

	// Verify file content remains intact
	readBack, err := os.ReadFile(sampleFile)
	if err != nil || string(readBack) != string(sampleContent) {
		t.Errorf("file content modified during resume: %v", err)
	}
}
