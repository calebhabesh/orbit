package protocol

import (
	"os"
	"strings"
	"testing"

	"github.com/calebhabesh/file-sync/internal/history"
)

func TestGoldenEnvelopeDecodesAndValidates(t *testing.T) {
	data, err := os.ReadFile("../../schemas/fixtures/version-envelope-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := DecodeEnvelope(data)
	if err != nil {
		t.Fatal(err)
	}
	h := history.New()
	if err := h.Accept(envelope); err != nil {
		t.Fatal(err)
	}
}
func TestEnvelopeDecoderRejectsUnknownDuplicateAndNoncanonicalFields(t *testing.T) {
	data, err := os.ReadFile("../../schemas/fixtures/version-envelope-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	cases := [][]byte{[]byte(strings.Replace(string(data), `"path": "empty.txt"`, `"path": "empty.txt", "path": "other"`, 1)), []byte(strings.Replace(string(data), `"path": "empty.txt"`, `"unknown": true, "path": "empty.txt"`, 1)), []byte(strings.Replace(string(data), `"counter": "1"`, `"counter": "01"`, 1)), append(append([]byte(nil), data...), []byte(` {}`)...)}
	for _, candidate := range cases {
		if _, err := DecodeEnvelope(candidate); err == nil {
			t.Fatalf("malformed envelope accepted: %s", candidate)
		}
	}
}
