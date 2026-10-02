package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPeerConfigurationRejectsUnsafeOrAmbiguousEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name, url, extra string
		mode             os.FileMode
		valid            bool
	}{
		{"valid", "https://127.0.0.1:8443", "", 0600, true},
		{"plaintext", "http://host", "", 0600, false},
		{"credentials", "https://user:password@host", "", 0600, false},
		{"path", "https://host/peer", "", 0600, false},
		{"query", "https://host?secret=x", "", 0600, false},
		{"unknown", "https://host", `,"other":true`, 0600, false},
		{"public", "https://host", "", 0644, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			data := fmt.Sprintf(`{"format_version":1,"peers":[{"folder":%q,"device":%q,"url":%q,"certificate":"peer.pem"}]%s}`, strings.Repeat("a", 64), strings.Repeat("b", 64), tc.url, tc.extra)
			if err := os.WriteFile(filepath.Join(root, "peers.json"), []byte(data), tc.mode); err != nil {
				t.Fatal(err)
			}
			peers, err := LoadPeerEndpoints(root)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%t err=%v", tc.valid, err)
			}
			if tc.valid && peers[0].Certificate != filepath.Join(root, "peer.pem") {
				t.Fatal("certificate not resolved against state")
			}
		})
	}
}

func TestOrbitPeerEndpoints_SaveAndLoadRoundtrip(t *testing.T) {
	root := t.TempDir()

	folder1 := strings.Repeat("a", 64)
	device1 := strings.Repeat("b", 64)
	device2 := strings.Repeat("c", 64)

	endpoints := []PeerEndpoint{
		{
			Folder:      folder1,
			Device:      device1,
			URL:         "https://192.168.1.50:8443",
			Certificate: "peer-device1.pem",
		},
		{
			Folder:      folder1,
			Device:      device2,
			URL:         "https://pi.local:8443",
			Certificate: filepath.Join(root, "peer-device2.pem"),
		},
	}

	if err := SavePeerEndpoints(root, endpoints); err != nil {
		t.Fatalf("SavePeerEndpoints failed: %v", err)
	}

	// Verify file permissions (0600)
	info, err := os.Stat(filepath.Join(root, PeersFilename))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected permissions 0600, got %o", info.Mode().Perm())
	}

	loaded, err := LoadPeerEndpoints(root)
	if err != nil {
		t.Fatalf("LoadPeerEndpoints failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(loaded))
	}
	if loaded[0].URL != "https://192.168.1.50:8443" {
		t.Errorf("loaded[0].URL = %s", loaded[0].URL)
	}
	if loaded[0].Certificate != filepath.Join(root, "peer-device1.pem") {
		t.Errorf("loaded[0].Certificate = %s", loaded[0].Certificate)
	}

	// Test SetPeerEndpoint (updating existing)
	updatedEndpoint := PeerEndpoint{
		Folder:      folder1,
		Device:      device1,
		URL:         "https://192.168.1.51:8443",
		Certificate: "peer-device1.pem",
	}
	if err := SetPeerEndpoint(root, updatedEndpoint); err != nil {
		t.Fatalf("SetPeerEndpoint failed: %v", err)
	}
	loadedAfterUpdate, err := LoadPeerEndpoints(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loadedAfterUpdate) != 2 {
		t.Fatalf("expected 2 endpoints after update, got %d", len(loadedAfterUpdate))
	}
	if loadedAfterUpdate[0].URL != "https://192.168.1.51:8443" {
		t.Errorf("URL after update = %s, want https://192.168.1.51:8443", loadedAfterUpdate[0].URL)
	}

	// Test RemovePeerEndpoint
	if err := RemovePeerEndpoint(root, folder1, device1); err != nil {
		t.Fatalf("RemovePeerEndpoint failed: %v", err)
	}
	loadedAfterRemove, err := LoadPeerEndpoints(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loadedAfterRemove) != 1 {
		t.Fatalf("expected 1 endpoint after remove, got %d", len(loadedAfterRemove))
	}
	if loadedAfterRemove[0].Device != device2 {
		t.Errorf("remaining device = %s, want %s", loadedAfterRemove[0].Device, device2)
	}
}

func TestOrbitPeerEndpoints_DuplicateAndInvalidRejection(t *testing.T) {
	root := t.TempDir()

	folder1 := strings.Repeat("a", 64)
	device1 := strings.Repeat("b", 64)

	duplicateEndpoints := []PeerEndpoint{
		{Folder: folder1, Device: device1, URL: "https://host1:8443", Certificate: "cert.pem"},
		{Folder: folder1, Device: device1, URL: "https://host2:8443", Certificate: "cert.pem"},
	}

	err := SavePeerEndpoints(root, duplicateEndpoints)
	if !errors.Is(err, ErrInvalidPeerEndpoint) {
		t.Fatalf("expected ErrInvalidPeerEndpoint on duplicate endpoint, got: %v", err)
	}

	invalidHex := []PeerEndpoint{
		{Folder: "short-hex", Device: device1, URL: "https://host:8443", Certificate: "cert.pem"},
	}
	err = SavePeerEndpoints(root, invalidHex)
	if !errors.Is(err, ErrInvalidPeerEndpoint) {
		t.Fatalf("expected ErrInvalidPeerEndpoint on invalid hex, got: %v", err)
	}
}
