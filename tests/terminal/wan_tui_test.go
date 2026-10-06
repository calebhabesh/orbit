package terminal_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/testkit"
)

func TestWANW07RealPTYRelayOnboarding(t *testing.T) {
	base := testkit.NewDisposable(t)
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	service, selection, origin, roots, _ := w05Service(t)
	conn, err := tls.Dial("tcp", strings.TrimPrefix(origin, "https://"), &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13})
	if err != nil {
		t.Fatal(err)
	}
	ca := filepath.Join(base, "ca.pem")
	err = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: conn.ConnectionState().PeerCertificates[0].Raw}), 0600)
	conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(base, "profile.json")
	b, _ := json.Marshal(selection)
	if err = os.WriteFile(profile, b, 0600); err != nil {
		t.Fatal(err)
	}
	binary := buildOrbitBinary(t, base)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	outage := filepath.Join(base, "service-outage")
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if _, err := os.Stat(outage); err == nil {
					if testkit.ValidateDestructiveTarget(base, outage) == nil {
						_ = service.Close()
					}
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-done }()
	args := []string{"../../scripts/terminal_wan_pty_test.py", "--binary", binary, "--profile", profile, "--outage-marker", outage}
	if out := os.Getenv("ORBIT_W07_PTY_EVIDENCE"); out != "" {
		args = append(args, "--output", out)
	}
	cmd := exec.CommandContext(ctx, "python3", args...)
	cmd.Env = append(os.Environ(), "SSL_CERT_FILE="+ca)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("W07 PTY: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "W07-keyboard-relay-onboarding") {
		t.Fatal("no scenario executed")
	}
	t.Log(string(out))
}
