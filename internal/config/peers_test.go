package config

import (
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
