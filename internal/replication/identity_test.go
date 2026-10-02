package replication

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
)

func TestLoadOrCreateIdentityReusesExistingKey(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	var dev1 history.ID
	dev1[0] = 0x01
	id1, err := LoadOrCreateIdentity(dir, dev1, now)
	if err != nil {
		t.Fatalf("LoadOrCreateIdentity first: %v", err)
	}

	var dev2 history.ID
	dev2[0] = 0x02
	id2, err := LoadOrCreateIdentity(dir, dev2, now)
	if err != nil {
		t.Fatalf("LoadOrCreateIdentity second: %v", err)
	}

	// Normal load must reuse the existing key and pin
	if id1.KeyPin != id2.KeyPin {
		t.Errorf("expected LoadOrCreateIdentity to reuse key pin, got %x vs %x", id1.KeyPin, id2.KeyPin)
	}
}

func TestRotateIdentityGeneratesFreshKeyAndChangesPin(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	var dev1 history.ID
	dev1[0] = 0x01
	id1, err := LoadOrCreateIdentity(dir, dev1, now)
	if err != nil {
		t.Fatalf("initial identity: %v", err)
	}

	var dev2 history.ID
	dev2[0] = 0x02
	id2, err := RotateIdentity(dir, dev2, now)
	if err != nil {
		t.Fatalf("RotateIdentity: %v", err)
	}

	// Rotated identity must generate a fresh key and distinct key pin
	if id1.KeyPin == id2.KeyPin {
		t.Fatalf("RotateIdentity reused existing key pin: %x", id1.KeyPin)
	}
	if bytes.Equal(id1.Certificate.Certificate[0], id2.Certificate.Certificate[0]) {
		t.Fatalf("RotateIdentity reused existing certificate")
	}

	// Verify file mode is private 0600
	info, err := os.Stat(filepath.Join(dir, "identity", "peer-identity.pem"))
	if err != nil {
		t.Fatalf("stat identity file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("identity file perm = %04o, want 0600", got)
	}

	// Loading again should now return id2's key pin
	id3, err := LoadOrCreateIdentity(dir, dev2, now)
	if err != nil {
		t.Fatalf("load after rotate: %v", err)
	}
	if id3.KeyPin != id2.KeyPin {
		t.Errorf("subsequent load did not preserve rotated key: got %x, want %x", id3.KeyPin, id2.KeyPin)
	}
}
