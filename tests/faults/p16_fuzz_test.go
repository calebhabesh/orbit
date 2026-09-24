package faults

import (
	"testing"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
)

// FuzzProtocolEnvelopeDecode tests that unmarshaling wire envelopes
// never panics regardless of malformed or adversarial JSON input.
func FuzzProtocolEnvelopeDecode(f *testing.F) {
	// Seed 1: valid envelope JSON
	seed1 := []byte(`{
		"id": {"folder": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "author": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "counter": 1},
		"path": "test.txt",
		"parents": [],
		"vector": [{"author": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "counter": 1}],
		"kind": "file",
		"authored_revision": 1
	}`)
	// Seed 2: minimal empty object
	seed2 := []byte(`{}`)
	// Seed 3: array instead of object
	seed3 := []byte(`[1, 2, 3]`)
	// Seed 4: malformed json
	seed4 := []byte(`{"id": {"folder": "truncated`)

	f.Add(seed1)
	f.Add(seed2)
	f.Add(seed3)
	f.Add(seed4)

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = protocol.DecodeEnvelope(data)
	})
}

// FuzzPathSanitization tests that path validation never panics and
// consistently rejects any path with directory traversal (..) or invalid bytes.
func FuzzPathSanitization(f *testing.F) {
	seeds := []string{
		"valid/relative/path.txt",
		"file.txt",
		"../escape",
		"dir/../../escape",
		"/absolute/path",
		"",
		"a/./b",
		"a//b",
		"bad\x00byte",
		"long/" + string(make([]byte, 300)),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, path string) {
		err := history.ValidatePath(path)
		// If path contains "..", ValidatePath MUST reject it
		for _, part := range splitPath(path) {
			if part == ".." && err == nil {
				t.Fatalf("path with .. was accepted: %q", path)
			}
		}
	})
}

func splitPath(p string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			if i > start {
				parts = append(parts, p[start:i])
			}
			start = i + 1
		}
	}
	if start < len(p) {
		parts = append(parts, p[start:])
	}
	return parts
}
