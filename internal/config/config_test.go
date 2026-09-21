package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInitializeIsDeterministicAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.FixedZone("test", -4*60*60))
	random := bytes.NewReader(bytes.Repeat([]byte{0xab}, 32))

	first, err := Initialize(dir, func() time.Time { return now }, random)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Initialize(dir, func() time.Time { return time.Time{} }, bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("second initialization changed config: first=%+v second=%+v", first, second)
	}
	if want := now.UTC(); first.CreatedAt != want {
		t.Fatalf("created_at = %v, want %v", first.CreatedAt, want)
	}
	info, err := os.Stat(filepath.Join(dir, filename))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("config mode = %04o, want 0600", got)
	}
}

func TestLoadRejectsUnknownAndIncompatibleConfiguration(t *testing.T) {
	for name, contents := range map[string]string{
		"unknown field":        `{"format_version":1,"device_id":"` + string(bytes.Repeat([]byte{'0'}, 64)) + `","created_at":"2026-09-20T00:00:00Z","extra":true}`,
		"incompatible version": `{"format_version":2,"device_id":"` + string(bytes.Repeat([]byte{'0'}, 64)) + `","created_at":"2026-09-20T00:00:00Z"}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, filename), []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(dir); err == nil {
				t.Fatal("Load succeeded, want an error")
			}
		})
	}
}
