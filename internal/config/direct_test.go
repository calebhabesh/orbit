package config

import (
	"os"
	"testing"
)

func TestWANW09DirectSettingsLegacyAndOptionalUDP(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"interfaces":[],"listen":"","disabled":false}`, `{"interfaces":[],"listen":"","disabled":false,"udp_listen":"[::]:0","udp_disabled":false}`} {
		if err := WritePrivate(dir, "direct-network.json", []byte(body)); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadDirectSettings(dir); err != nil {
			t.Fatal("compatible settings", err)
		}
	}
	for _, body := range []string{`{"interfaces":[],"listen":"","disabled":false,"udp_disabled":null}`, `{"interfaces":[],"listen":"","disabled":false,"udp_listen":"example.org:1234"}`, `{"interfaces":[],"listen":"","disabled":false,"udp_listen":"[::]:0","udp_listen":"[::]:1"}`} {
		if err := WritePrivate(dir, "direct-network.json", []byte(body)); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadDirectSettings(dir); err == nil {
			t.Fatal("invalid settings", body)
		}
	}
}
