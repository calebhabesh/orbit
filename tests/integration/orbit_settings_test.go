package integration_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/control"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
	"github.com/calebhabesh/orbit/internal/workspace"
)

// TestOrbitSettings_EndToEnd_SettingsAndRestart tests that:
// 1. Product preferences survive daemon restart.
// 2. Updating product labels NEVER rotates identity or mutates format_version: 1.
// 3. Invalid settings are rejected without corrupting or partially writing configuration.
// 4. Duplicate/conflicting workspace labels remain usable.
func TestOrbitSettings_EndToEnd_SettingsAndRestart(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state")

	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// 1. Initialize core configuration
	coreCfg, err := config.Initialize(stateDir, time.Now, rand.Reader)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	originalDeviceID := coreCfg.DeviceID

	// 2. Open repository and create controller
	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	ws := workspace.New(db, workspace.Options{})
	var localDevice history.ID
	rawDev, _ := hex.DecodeString(coreCfg.DeviceID)
	copy(localDevice[:], rawDev)
	ctrl := control.New(db, ws, control.Options{LocalDevice: localDevice})

	// 3. Save initial product settings
	folder1Hex := strings.Repeat("a", 64)
	folder2Hex := strings.Repeat("b", 64)
	lbl := "Laptop"
	theme := "dark"
	updRes, err := ctrl.UpdateSettings(ctx, control.UpdateSettingsRequest{
		DeviceLabel:      &lbl,
		DefaultWorkspace: &folder1Hex,
		WorkspaceNames: map[string]string{
			folder1Hex: "Work Docs",
			folder2Hex: "Work Docs", // Duplicate label is permitted (owner-local aliases)
		},
		Theme: &theme,
	})
	if err != nil {
		t.Fatalf("UpdateSettings failed: %v", err)
	}
	if updRes.Settings.DeviceLabel != "Laptop" {
		t.Errorf("DeviceLabel = %s, want 'Laptop'", updRes.Settings.DeviceLabel)
	}

	// 4. Reopen/restart: Close repository and simulate fresh daemon start
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Verify core config remains unchanged before and after restart
	loadedCore, err := config.Load(stateDir)
	if err != nil {
		t.Fatalf("core config.Load failed: %v", err)
	}
	if loadedCore.DeviceID != originalDeviceID {
		t.Fatalf("DeviceID rotated! got %s, want %s", loadedCore.DeviceID, originalDeviceID)
	}
	if loadedCore.FormatVersion != 1 {
		t.Errorf("FormatVersion = %d, want 1", loadedCore.FormatVersion)
	}

	// Reopen repository and reload settings
	dbRestarted, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatalf("Open after restart failed: %v", err)
	}
	defer dbRestarted.Close()

	wsRestarted := workspace.New(dbRestarted, workspace.Options{})
	ctrlRestarted := control.New(dbRestarted, wsRestarted, control.Options{LocalDevice: localDevice})

	getRes, err := ctrlRestarted.GetSettings(ctx)
	if err != nil {
		t.Fatalf("GetSettings after restart failed: %v", err)
	}
	if getRes.Settings.DeviceLabel != "Laptop" {
		t.Errorf("restarted DeviceLabel = %s, want 'Laptop'", getRes.Settings.DeviceLabel)
	}
	if getRes.Settings.DefaultWorkspace != folder1Hex {
		t.Errorf("restarted DefaultWorkspace = %s, want %s", getRes.Settings.DefaultWorkspace, folder1Hex)
	}
	if getRes.Settings.WorkspaceNames[folder1Hex] != "Work Docs" || getRes.Settings.WorkspaceNames[folder2Hex] != "Work Docs" {
		t.Errorf("restarted WorkspaceNames mismatch: %v", getRes.Settings.WorkspaceNames)
	}

	// 5. Invalid settings rejection without partial configuration
	invalidTheme := "unsupported-theme"
	_, err = ctrlRestarted.UpdateSettings(ctx, control.UpdateSettingsRequest{
		Theme: &invalidTheme,
	})
	if err == nil {
		t.Fatal("expected error on invalid theme")
	}

	// Verify settings on disk still retain valid theme
	rechecked, err := ctrlRestarted.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rechecked.Settings.Theme != "dark" {
		t.Errorf("theme was corrupted after invalid update: %s", rechecked.Settings.Theme)
	}
}

// TestOrbitSettings_EndToEnd_EndpointsEditing tests durable peer endpoints editing:
// 1. Setting and persisting endpoints to peers.json.
// 2. Reloading on restart.
// 3. Rejecting invalid or duplicate endpoints without corrupting state.
func TestOrbitSettings_EndToEnd_EndpointsEditing(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}

	coreCfg, err := config.Initialize(stateDir, time.Now, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ws := workspace.New(db, workspace.Options{})
	var localDevice history.ID
	rawDev, _ := hex.DecodeString(coreCfg.DeviceID)
	copy(localDevice[:], rawDev)
	ctrl := control.New(db, ws, control.Options{LocalDevice: localDevice})

	folderHex := strings.Repeat("c", 64)
	deviceHex := strings.Repeat("d", 64)

	// Set endpoint
	err = ctrl.SetPeerEndpoint(ctx, control.SetPeerEndpointRequest{
		Folder:      folderHex,
		Device:      deviceHex,
		URL:         "https://vps.example.com:8443",
		Certificate: "vps-cert.pem",
	})
	if err != nil {
		t.Fatalf("SetPeerEndpoint failed: %v", err)
	}

	// Verify persistence in peers.json
	peersOnDisk, err := config.LoadPeerEndpoints(stateDir)
	if err != nil {
		t.Fatalf("LoadPeerEndpoints failed: %v", err)
	}
	if len(peersOnDisk) != 1 || peersOnDisk[0].URL != "https://vps.example.com:8443" {
		t.Fatalf("peersOnDisk mismatch: %+v", peersOnDisk)
	}

	// Update existing endpoint (URL change)
	err = ctrl.SetPeerEndpoint(ctx, control.SetPeerEndpointRequest{
		Folder:      folderHex,
		Device:      deviceHex,
		URL:         "https://vps-new.example.com:8443",
		Certificate: "vps-cert.pem",
	})
	if err != nil {
		t.Fatalf("updating SetPeerEndpoint failed: %v", err)
	}

	listRes, err := ctrl.ListPeerEndpoints(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listRes.Peers) != 1 || listRes.Peers[0].URL != "https://vps-new.example.com:8443" {
		t.Errorf("updated endpoint mismatch: %+v", listRes.Peers)
	}

	// Reject invalid endpoint (plain HTTP)
	err = ctrl.SetPeerEndpoint(ctx, control.SetPeerEndpointRequest{
		Folder:      folderHex,
		Device:      strings.Repeat("e", 64),
		URL:         "http://insecure.example.com",
		Certificate: "cert.pem",
	})
	if err == nil {
		t.Fatal("expected error on non-HTTPS URL")
	}

	// Existing endpoint remains unaffected
	peersAfterInvalid, err := config.LoadPeerEndpoints(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(peersAfterInvalid) != 1 || peersAfterInvalid[0].URL != "https://vps-new.example.com:8443" {
		t.Errorf("peers corrupted after invalid update: %+v", peersAfterInvalid)
	}
}

// TestOrbitSettings_EndToEnd_FolderAuthorizationIsolated verifies Invariant I09:
// Knowing an endpoint URL or display name does NOT grant membership or chunk access.
// Peer authentication requires explicit approved membership entries.
func TestOrbitSettings_EndToEnd_FolderAuthorizationIsolated(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}

	coreCfg, err := config.Initialize(stateDir, time.Now, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var authorizedFolder, unauthorizedFolder history.ID
	var localAuthor history.ID
	rand.Read(authorizedFolder[:])
	rand.Read(unauthorizedFolder[:])
	copy(localAuthor[:], coreCfg.DeviceID[:32])

	// Enroll authorized folder only
	if err := db.EnsureFolder(ctx, authorizedFolder, localAuthor, 1); err != nil {
		t.Fatal(err)
	}

	// Configure peer endpoint for unauthorizedFolder in peers.json
	peerDevHex := strings.Repeat("f", 64)
	err = config.SetPeerEndpoint(stateDir, config.PeerEndpoint{
		Folder:      hex.EncodeToString(unauthorizedFolder[:]),
		Device:      peerDevHex,
		URL:         "https://peer.example.com:8443",
		Certificate: "peer.pem",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify that unauthorizedFolder is NOT registered or authorized in repository
	registered, err := db.RegisteredFolders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, reg := range registered {
		if reg.Folder == unauthorizedFolder {
			t.Fatalf("unauthorizedFolder %s must not be registered in database", unauthorizedFolder)
		}
	}

	// Server TLS handshake verification oracle:
	// A peer certificate with unauthorized folder cannot access workspace chunks or metadata
	var peerDevID history.ID
	rawPeerDev, _ := hex.DecodeString(peerDevHex)
	copy(peerDevID[:], rawPeerDev)

	// Peer has zero membership entries in database, so AuthorizePeer returns ErrUnauthorized
	err = db.AuthorizePeer(ctx, unauthorizedFolder, peerDevID, history.Digest{}, 1, history.Digest{})
	if !errors.Is(err, repository.ErrUnauthorized) {
		t.Fatalf("peer must not be authorized for unregistered folder: got %v, want ErrUnauthorized", err)
	}
}
