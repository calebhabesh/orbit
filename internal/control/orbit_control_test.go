package control_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

func setupTestController(t *testing.T) (*control.Controller, *repository.DB, string, func()) {
	t.Helper()
	stateDir := testkit.NewDisposable(t)
	if err := os.Chmod(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	rawID := make([]byte, 32)
	rand.Read(rawID)
	coreCfg := config.Config{
		FormatVersion: 1,
		DeviceID:      hex.EncodeToString(rawID),
		CreatedAt:     time.Now().UTC(),
	}
	if err := config.Save(stateDir, coreCfg); err != nil {
		t.Fatal(err)
	}

	if err := config.InitializeStorageLimits(stateDir); err != nil {
		t.Fatal(err)
	}
	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}

	ws := workspace.New(db, workspace.Options{})
	var localDevice history.ID
	copy(localDevice[:], rawID)

	ctrl := control.New(db, ws, control.Options{
		LocalDevice: localDevice,
	})

	cleanup := func() {
		_ = db.Close()
	}
	return ctrl, db, stateDir, cleanup
}

func TestOrbitSettings_Control_SettingsOperations(t *testing.T) {
	ctrl, _, stateDir, cleanup := setupTestController(t)
	defer cleanup()
	ctx := context.Background()

	// 1. Initial settings should be defaults
	getRes, err := ctrl.GetSettings(ctx)
	if err != nil {
		t.Fatalf("GetSettings failed: %v", err)
	}
	if getRes.Settings.FormatVersion != config.SettingsFormatVersion {
		t.Errorf("FormatVersion = %d, want %d", getRes.Settings.FormatVersion, config.SettingsFormatVersion)
	}

	// 2. Update settings
	devLabel := "Studio Workstation"
	wsName := "Code Projects"
	darkTheme := true
	folderIDHex := strings.Repeat("e", 64)

	updReq := control.UpdateSettingsRequest{
		DeviceLabel:      &devLabel,
		DefaultWorkspace: &folderIDHex,
		WorkspaceNames: map[string]string{
			folderIDHex: wsName,
		},
		DarkTheme: &darkTheme,
	}
	updRes, err := ctrl.UpdateSettings(ctx, updReq)
	if err != nil {
		t.Fatalf("UpdateSettings failed: %v", err)
	}
	if updRes.Settings.DeviceLabel != devLabel {
		t.Errorf("DeviceLabel = %q, want %q", updRes.Settings.DeviceLabel, devLabel)
	}

	// 3. Verify settings persisted to disk
	diskSettings, err := config.LoadSettingsStrict(stateDir)
	if err != nil {
		t.Fatalf("LoadSettingsStrict failed: %v", err)
	}
	if diskSettings.DeviceLabel != devLabel || diskSettings.WorkspaceNames[folderIDHex] != wsName {
		t.Errorf("disk settings mismatch: %+v", diskSettings)
	}
}

func TestOrbitSettings_Control_PeerEndpointsOperations(t *testing.T) {
	ctrl, _, _, cleanup := setupTestController(t)
	defer cleanup()
	ctx := context.Background()

	folderHex := strings.Repeat("1", 64)
	deviceHex := strings.Repeat("2", 64)

	// Set peer endpoint
	err := ctrl.SetPeerEndpoint(ctx, control.SetPeerEndpointRequest{
		Folder:      folderHex,
		Device:      deviceHex,
		URL:         "https://peer1.local:8443",
		Certificate: "peer1.pem",
	})
	if err != nil {
		t.Fatalf("SetPeerEndpoint failed: %v", err)
	}

	// List peer endpoints
	list, err := ctrl.ListPeerEndpoints(ctx)
	if err != nil {
		t.Fatalf("ListPeerEndpoints failed: %v", err)
	}
	if len(list.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(list.Peers))
	}
	if list.Peers[0].URL != "https://peer1.local:8443" {
		t.Errorf("peer URL = %s, want https://peer1.local:8443", list.Peers[0].URL)
	}

	// Remove peer endpoint
	err = ctrl.RemovePeerEndpoint(ctx, control.RemovePeerEndpointRequest{
		Folder: folderHex,
		Device: deviceHex,
	})
	if err != nil {
		t.Fatalf("RemovePeerEndpoint failed: %v", err)
	}
	listAfter, err := ctrl.ListPeerEndpoints(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listAfter.Peers) != 0 {
		t.Errorf("expected 0 peers after removal, got %d", len(listAfter.Peers))
	}
}

func TestOrbitSettings_Control_SetupOperations(t *testing.T) {
	ctrl, _, stateDir, cleanup := setupTestController(t)
	defer cleanup()
	ctx := context.Background()

	// Inspect setup
	insp, err := ctrl.InspectSetup(ctx)
	if err != nil {
		t.Fatalf("InspectSetup failed: %v", err)
	}
	if !insp.Initialized {
		t.Errorf("expected Initialized=true")
	}

	// Preview root outside stateDir
	newRoot := filepath.Join(filepath.Dir(stateDir), "SyncTestFolder")
	prev, err := ctrl.PreviewCreateRoot(ctx, control.PreviewCreateRootRequest{Path: newRoot})
	if err != nil {
		t.Fatalf("PreviewCreateRoot failed: %v", err)
	}
	if prev.Disallowed {
		t.Fatalf("expected root to be allowed, reason: %s", prev.Reason)
	}

	// Start setup
	setupRes, err := ctrl.StartSetup(ctx, control.StartSetupRequest{
		RootPath:      newRoot,
		DeviceLabel:   "Primary Device",
		WorkspaceName: "Main Workspace",
	})
	if err != nil {
		t.Fatalf("StartSetup failed: %v", err)
	}
	if setupRes.Phase != "completed" {
		t.Errorf("expected phase 'completed', got %q", setupRes.Phase)
	}

	// Check status
	st, err := ctrl.GetSetupStatus(ctx)
	if err != nil {
		t.Fatalf("GetSetupStatus failed: %v", err)
	}
	if !st.Completed {
		t.Errorf("expected setup completed=true")
	}
}

func TestOrbitSettings_Control_InvitationsAndEnrollment(t *testing.T) {
	ctrl, db, _, cleanup := setupTestController(t)
	defer cleanup()
	ctx := context.Background()

	var folderID, authorID history.ID
	rand.Read(folderID[:])
	rand.Read(authorID[:])

	if err := db.EnsureFolder(ctx, folderID, authorID, 1); err != nil {
		t.Fatalf("EnsureFolder failed: %v", err)
	}

	// 1. Create invitation
	invRes, err := ctrl.CreateInvitation(ctx, control.CreateInvitationRequest{
		Folder:  folderID,
		TTLSecs: 3600,
		MaxUses: 1,
	})
	if err != nil {
		t.Fatalf("CreateInvitation failed: %v", err)
	}
	if invRes.Token == "" {
		t.Fatal("empty invitation token")
	}

	// 2. Submit enrollment request with the token
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	joiningDev := sha256.Sum256(pub)
	challenge := []byte("orbit-challenge-test-12345")
	sig := ed25519.Sign(priv, challenge)

	subRes, err := ctrl.SubmitEnrollmentRequest(ctx, control.SubmitJoinRequestPayload{
		Token:          invRes.Token,
		JoiningDevice:  joiningDev,
		PublicKey:      hex.EncodeToString(pub),
		Signature:      hex.EncodeToString(sig),
		Challenge:      hex.EncodeToString(challenge),
		SuggestedLabel: "Bob Tablet",
		TargetFolder:   folderID,
	})
	if err != nil {
		t.Fatalf("SubmitEnrollmentRequest failed: %v", err)
	}
	if subRes.Status != "pending" {
		t.Errorf("status = %q, want 'pending'", subRes.Status)
	}

	// Reusing token must fail (single-use)
	_, err = ctrl.SubmitEnrollmentRequest(ctx, control.SubmitJoinRequestPayload{
		Token:          invRes.Token,
		JoiningDevice:  joiningDev,
		PublicKey:      hex.EncodeToString(pub),
		Signature:      hex.EncodeToString(sig),
		Challenge:      hex.EncodeToString(challenge),
		SuggestedLabel: "Bob Tablet",
		TargetFolder:   folderID,
	})
	if err == nil {
		t.Fatal("expected error on replaying consumed invitation token")
	}

	// 3. List requests
	listReqs, err := ctrl.ListEnrollmentRequests(ctx, folderID, "pending")
	if err != nil {
		t.Fatalf("ListEnrollmentRequests failed: %v", err)
	}
	if len(listReqs.Requests) != 1 {
		t.Fatalf("expected 1 pending request, got %d", len(listReqs.Requests))
	}

	// 4. Approve request
	apprRes, err := ctrl.ApproveEnrollmentRequest(ctx, control.ApproveEnrollmentRequest{
		RequestID: subRes.RequestID,
		Folder:    folderID,
	})
	if err != nil {
		t.Fatalf("ApproveEnrollmentRequest failed: %v", err)
	}
	if apprRes.Status != "approved" {
		t.Errorf("status = %q, want 'approved'", apprRes.Status)
	}
}

func TestOrbitSettings_Control_ReadLeasesAndGCProtection(t *testing.T) {
	ctrl, db, _, cleanup := setupTestController(t)
	defer cleanup()
	ctx := context.Background()

	var folderID, authorID history.ID
	rand.Read(folderID[:])
	cfg, err := config.Load(db.StateDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := authorID.UnmarshalText([]byte(cfg.DeviceID)); err != nil {
		t.Fatal(err)
	}

	if err := db.EnsureFolder(ctx, folderID, authorID, 1); err != nil {
		t.Fatalf("EnsureFolder failed: %v", err)
	}

	chunkData := []byte("read-lease-test-content")
	chunkDigest := sha256.Sum256(chunkData)
	if err := db.InstallChunk(ctx, chunkDigest, uint64(len(chunkData)), bytes.NewReader(chunkData)); err != nil {
		t.Fatalf("InstallChunk failed: %v", err)
	}

	if _, err := ctrl.RegisterFolder(ctx, folderID, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	manifest, err := db.StoreFile(ctx, bytes.NewReader(chunkData), false)
	if err != nil {
		t.Fatal(err)
	}
	version, err := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{Folder: folderID, Path: "lease.txt", Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Acquire read lease
	leaseRes, err := ctrl.AcquireReadLease(ctx, control.AcquireReadLeaseRequest{
		Folder:         folderID,
		VersionAuthor:  authorID,
		VersionCounter: version.ID.Counter,
		TTLSeconds:     60,
		ChunkDigests:   []history.Digest{chunkDigest},
	})
	if err != nil {
		t.Fatalf("AcquireReadLease failed: %v", err)
	}
	if leaseRes.LeaseID == "" {
		t.Fatal("empty lease ID")
	}
	if leaseRes.TotalBytesStr == "" {
		t.Fatal("empty total bytes decimal string")
	}

	// Release lease
	err = ctrl.ReleaseReadLease(ctx, control.ReleaseReadLeaseRequest{LeaseID: leaseRes.LeaseID})
	if err != nil {
		t.Fatalf("ReleaseReadLease failed: %v", err)
	}
}

func TestOrbitSettings_Control_HTTP_EndpointsAndRecoverySafety(t *testing.T) {
	ctrl, _, stateDir, cleanup := setupTestController(t)
	defer cleanup()

	srv, err := control.NewServer(ctrl, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	cliToken := srv.CLIToken()
	handler := srv.Handler()

	doReq := func(method, path string, body any) *httptest.ResponseRecorder {
		var reqBody []byte
		if body != nil {
			reqBody, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(reqBody))
		r.Host = "127.0.0.1"
		r.Header.Set("Authorization", "Bearer "+cliToken)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}

	// 1. GET /api/v1/settings
	rec := doReq("GET", "/api/v1/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/settings returned %d: %s", rec.Code, rec.Body.String())
	}

	// 2. POST /api/v1/settings
	lbl := "HTTP Living Room"
	rec = doReq("POST", "/api/v1/settings", control.UpdateSettingsRequest{
		DeviceLabel: &lbl,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/settings returned %d: %s", rec.Code, rec.Body.String())
	}

	// 3. GET /api/v1/recovery/status
	rec = doReq("GET", "/api/v1/recovery/status", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/recovery/status returned %d: %s", rec.Code, rec.Body.String())
	}
	var recovRes control.RecoveryInspectionResult
	if err := json.Unmarshal(rec.Body.Bytes(), &recovRes); err != nil {
		t.Fatalf("unmarshal recovery status: %v", err)
	}
	if !recovRes.Consistent {
		t.Errorf("expected recovery status consistent=true")
	}

	// 4. Live reset-identity MUST be blocked on running server
	rec = doReq("POST", "/api/v1/maintenance/reset-identity", nil)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected live reset-identity to be blocked, but returned 200 OK")
	}
	var blockedErr control.ControlError
	_ = json.Unmarshal(rec.Body.Bytes(), &blockedErr)
	if blockedErr.Code != "OPERATION_BLOCKED" {
		t.Errorf("expected error code OPERATION_BLOCKED, got %s", blockedErr.Code)
	}

	// 5. Live restore-backup MUST be blocked on running server
	rec = doReq("POST", "/api/v1/maintenance/restore-backup", control.RestoreBackupRequest{BackupPath: "/tmp/backup.sqlite"})
	if rec.Code == http.StatusOK {
		t.Fatalf("expected live restore-backup to be blocked, but returned 200 OK")
	}
	var restoreErr control.ControlError
	_ = json.Unmarshal(rec.Body.Bytes(), &restoreErr)
	if restoreErr.Code != "OPERATION_BLOCKED" {
		t.Errorf("expected error code OPERATION_BLOCKED, got %s", restoreErr.Code)
	}
}

func TestOrbitShell_DeletedFiles_HTTP(t *testing.T) {
	ctrl, db, stateDir, cleanup := setupTestController(t)
	defer cleanup()
	ctx := context.Background()

	var folder history.ID
	folder[0] = 0xAA
	folder[31] = 0xBB

	syncRoot := filepath.Join(filepath.Dir(stateDir), "test-root")
	if err := os.MkdirAll(syncRoot, 0700); err != nil {
		t.Fatal(err)
	}

	if _, err := ctrl.RegisterFolder(ctx, folder, syncRoot); err != nil {
		t.Fatal(err)
	}

	ws := workspace.New(db, workspace.Options{})

	// 1. Create a file in syncRoot and scan it
	activeFile := filepath.Join(syncRoot, "active.txt")
	if err := os.WriteFile(activeFile, []byte("active content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	deletedFile := filepath.Join(syncRoot, "will_delete.txt")
	if err := os.WriteFile(deletedFile, []byte("temporary content to delete\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := ws.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}

	// Verify both are active
	activeList, err := ctrl.Files(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(activeList) != 2 {
		t.Fatalf("expected 2 active files, got %d", len(activeList))
	}

	deletedList, err := ctrl.DeletedFiles(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(deletedList) != 0 {
		t.Fatalf("expected 0 deleted files initially, got %d", len(deletedList))
	}

	// 2. Delete will_delete.txt and rescan
	if err := os.Remove(deletedFile); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Scan(ctx, folder); err != nil {
		t.Fatal(err)
	}

	// Verify active files only has active.txt
	activeList, err = ctrl.Files(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(activeList) != 1 || activeList[0].Path != "active.txt" {
		t.Fatalf("expected 1 active file 'active.txt', got %v", activeList)
	}

	// Verify DeletedFiles contains will_delete.txt
	deletedList, err = ctrl.DeletedFiles(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(deletedList) != 1 || deletedList[0].Path != "will_delete.txt" {
		t.Fatalf("expected 1 deleted file 'will_delete.txt', got %v", deletedList)
	}

	// 3. Test HTTP endpoint GET /api/v1/files/deleted
	srv, err := control.NewServer(ctrl, stateDir)
	if err != nil {
		t.Fatal(err)
	}

	folderHex := hex.EncodeToString(folder[:])
	req := httptest.NewRequest("GET", "/api/v1/files/deleted?folder="+folderHex, nil)
	req.Host = "127.0.0.1"
	req.Header.Set("Authorization", "Bearer "+srv.CLIToken())

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/files/deleted returned %d: %s", rec.Code, rec.Body.String())
	}

	var httpDeleted []control.FileItem
	if err := json.NewDecoder(rec.Body).Decode(&httpDeleted); err != nil {
		t.Fatal(err)
	}
	if len(httpDeleted) != 1 || httpDeleted[0].Path != "will_delete.txt" {
		t.Fatalf("expected 1 deleted file from HTTP endpoint, got %v", httpDeleted)
	}
}

func TestOrbitEndpoints_ControlUnit(t *testing.T) {
	ctrl, _, stateDir, cleanup := setupTestController(t)
	defer cleanup()
	ctx := context.Background()

	folderHex := strings.Repeat("1", 64)
	deviceHex := strings.Repeat("2", 64)
	certPath := filepath.Join(stateDir, "peer.crt")
	_ = os.WriteFile(certPath, []byte("CERT"), 0o600)

	// Set endpoint
	err := ctrl.SetPeerEndpoint(ctx, control.SetPeerEndpointRequest{
		Folder:      folderHex,
		Device:      deviceHex,
		URL:         "https://peer.example.com:8443",
		Certificate: certPath,
	})
	if err != nil {
		t.Fatalf("SetPeerEndpoint failed: %v", err)
	}

	// List
	res, err := ctrl.ListPeerEndpoints(ctx)
	if err != nil || len(res.Peers) != 1 {
		t.Fatalf("ListPeerEndpoints failed: %+v (err: %v)", res, err)
	}

	// Remove
	err = ctrl.RemovePeerEndpoint(ctx, control.RemovePeerEndpointRequest{
		Folder: folderHex,
		Device: deviceHex,
	})
	if err != nil {
		t.Fatalf("RemovePeerEndpoint failed: %v", err)
	}

	res2, err := ctrl.ListPeerEndpoints(ctx)
	if err != nil || len(res2.Peers) != 0 {
		t.Fatalf("expected 0 endpoints, got %d", len(res2.Peers))
	}
}
