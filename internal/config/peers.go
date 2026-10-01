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

const MaxPeerEndpoints = 64

type PeerEndpoint struct {
	Folder      string `json:"folder"`
	Device      string `json:"device"`
	URL         string `json:"url"`
	Certificate string `json:"certificate"`
}

// Peer endpoints locate already approved members; they never grant membership.
// Relative certificate paths are resolved against the private state directory.
func LoadPeerEndpoints(stateDir string) ([]PeerEndpoint, error) {
	path := filepath.Join(stateDir, "peers.json")
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
	seen := map[string]bool{}
	for i := range document.Peers {
		peer := &document.Peers[i]
		for _, id := range []string{peer.Folder, peer.Device} {
			decoded, err := hex.DecodeString(id)
			if err != nil || len(decoded) != 32 || id != hex.EncodeToString(decoded) {
				return nil, errors.New("peer folder/device must be canonical 32-byte hex")
			}
		}
		u, err := url.Parse(peer.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return nil, errors.New("peer URL must be an HTTPS origin")
		}
		peer.URL = "https://" + u.Host
		key := peer.Folder + peer.Device
		if seen[key] {
			return nil, errors.New("duplicate folder/peer endpoint")
		}
		seen[key] = true
		if peer.Certificate == "" {
			return nil, errors.New("peer certificate path is required")
		}
		if !filepath.IsAbs(peer.Certificate) {
			peer.Certificate = filepath.Join(stateDir, peer.Certificate)
		}
	}
	return document.Peers, nil
}
