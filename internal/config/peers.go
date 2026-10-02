package config

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
)

const (
	MaxPeerEndpoints = 64
	PeersFilename    = "peers.json"
)

var (
	ErrInvalidPeerEndpoint = errors.New("invalid peer endpoint")
)

type PeerEndpoint struct {
	Folder      string `json:"folder"`
	Device      string `json:"device"`
	URL         string `json:"url"`
	Certificate string `json:"certificate"`
}

// ValidatePeerEndpoints validates a slice of peer endpoints according to canonical rules.
func ValidatePeerEndpoints(peers []PeerEndpoint, stateDir string) ([]PeerEndpoint, error) {
	if len(peers) > MaxPeerEndpoints {
		return nil, fmt.Errorf("%w: endpoint count %d exceeds maximum %d", ErrInvalidPeerEndpoint, len(peers), MaxPeerEndpoints)
	}
	validated := make([]PeerEndpoint, len(peers))
	seen := map[string]bool{}

	for i, peer := range peers {
		for name, id := range map[string]string{"folder": peer.Folder, "device": peer.Device} {
			decoded, err := hex.DecodeString(id)
			if err != nil || len(decoded) != 32 || id != hex.EncodeToString(decoded) {
				return nil, fmt.Errorf("%w: peer %s must be canonical 32-byte hex", ErrInvalidPeerEndpoint, name)
			}
		}
		u, err := url.Parse(peer.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return nil, fmt.Errorf("%w: peer URL %q must be an HTTPS origin without userinfo, path, query, or fragment", ErrInvalidPeerEndpoint, peer.URL)
		}
		peer.URL = "https://" + u.Host

		key := peer.Folder + ":" + peer.Device
		if seen[key] {
			return nil, fmt.Errorf("%w: duplicate folder/peer endpoint for folder %s, device %s", ErrInvalidPeerEndpoint, peer.Folder, peer.Device)
		}
		seen[key] = true

		if peer.Certificate == "" {
			return nil, fmt.Errorf("%w: peer certificate path is required", ErrInvalidPeerEndpoint)
		}
		if stateDir != "" && !filepath.IsAbs(peer.Certificate) {
			peer.Certificate = filepath.Join(stateDir, peer.Certificate)
		}
		validated[i] = peer
	}
	return validated, nil
}

// Peer endpoints locate already approved members; they never grant membership.
// Relative certificate paths are resolved against the private state directory.
func LoadPeerEndpoints(stateDir string) ([]PeerEndpoint, error) {
	path := filepath.Join(stateDir, PeersFilename)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 64*1024 {
		return nil, errors.New("peers.json must be a private regular file at most 64 KiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var document struct {
		FormatVersion int            `json:"format_version"`
		Peers         []PeerEndpoint `json:"peers"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode peers.json: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF || document.FormatVersion != 1 || len(document.Peers) > MaxPeerEndpoints {
		return nil, errors.New("invalid peer configuration version, trailing data or endpoint count")
	}
	return ValidatePeerEndpoints(document.Peers, stateDir)
}

// SavePeerEndpoints atomically writes validated peer endpoints to stateDir/peers.json with mode 0600.
func SavePeerEndpoints(stateDir string, peers []PeerEndpoint) error {
	validated, err := ValidatePeerEndpoints(peers, stateDir)
	if err != nil {
		return err
	}

	// For serialization, store relative certificate paths if inside stateDir
	persisted := make([]PeerEndpoint, len(validated))
	for i, p := range validated {
		persisted[i] = p
		if rel, err := filepath.Rel(stateDir, p.Certificate); err == nil && !filepath.IsAbs(rel) && rel != ".." && len(rel) > 0 && rel[0] != '.' {
			persisted[i].Certificate = rel
		}
	}

	document := struct {
		FormatVersion int            `json:"format_version"`
		Peers         []PeerEndpoint `json:"peers"`
	}{
		FormatVersion: 1,
		Peers:         persisted,
	}

	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode peers.json: %w", err)
	}
	data = append(data, '\n')

	temporary, err := os.CreateTemp(stateDir, ".peers-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary peers: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure temporary peers: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write peers: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("flush peers: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close peers: %w", err)
	}

	targetPath := filepath.Join(stateDir, PeersFilename)
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		return fmt.Errorf("install peers: %w", err)
	}
	if dirFile, err := os.Open(stateDir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
	return nil
}

// SetPeerEndpoint adds or updates an endpoint for a (folder, device) pair.
func SetPeerEndpoint(stateDir string, endpoint PeerEndpoint) error {
	existing, err := LoadPeerEndpoints(stateDir)
	if err != nil {
		return err
	}

	updated := false
	for i, p := range existing {
		if p.Folder == endpoint.Folder && p.Device == endpoint.Device {
			existing[i] = endpoint
			updated = true
			break
		}
	}
	if !updated {
		existing = append(existing, endpoint)
	}
	return SavePeerEndpoints(stateDir, existing)
}

// RemovePeerEndpoint removes an endpoint matching (folder, device).
func RemovePeerEndpoint(stateDir, folderID, deviceID string) error {
	existing, err := LoadPeerEndpoints(stateDir)
	if err != nil {
		return err
	}
	remaining := make([]PeerEndpoint, 0, len(existing))
	for _, p := range existing {
		if p.Folder == folderID && p.Device == deviceID {
			continue
		}
		remaining = append(remaining, p)
	}
	return SavePeerEndpoints(stateDir, remaining)
}
