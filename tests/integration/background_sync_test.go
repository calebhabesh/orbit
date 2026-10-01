package integration_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/testkit"
)

// Exercise the normal CLI daemon, with no injected client factory or manual
// scan/sync calls. Configuration persists across receiver restarts.
func TestBackgroundSyncFromPersistedPeerEndpoints(t *testing.T) {
	root := testkit.NewDisposable(t)
	binary := buildBinary(t, root)
	folder := strings.Repeat("a", 64)
	states := []string{filepath.Join(root, "a"), filepath.Join(root, "b")}
	roots := []string{filepath.Join(root, "files-a"), filepath.Join(root, "files-b")}
	run := func(args ...string) []byte {
		t.Helper()
		out, err := exec.Command(binary, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v %s", args, err, out)
		}
		return out
	}
	for i := range states {
		if err := os.Mkdir(roots[i], 0700); err != nil {
			t.Fatal(err)
		}
		run("init", "--state", states[i])
		run("register", "--state", states[i], "--folder", folder, "--root", roots[i])
	}
	a, pinA, certA := parseIdentityOutput(t, run("identity", "--state", states[0], "--certificate"))
	b, pinB, _ := parseIdentityOutput(t, run("identity", "--state", states[1], "--certificate"))
	run("pair-approve", "--state", states[0], "--folder", folder, "--peer-device", b, "--peer-key-pin", pinB)
	run("pair-approve", "--state", states[1], "--folder", folder, "--peer-device", a, "--peer-key-pin", pinA)
	if err := os.WriteFile(filepath.Join(states[1], "a.pem"), certA, 0600); err != nil {
		t.Fatal(err)
	}
	start := func(state string) (*exec.Cmd, string) {
		t.Helper()
		cmd := exec.Command(binary, "serve", "--state", state, "--peer-listen", "127.0.0.1:0", "--sync-interval", "100ms")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return cmd, readListenerURL(t, stdout)
	}
	_, url := start(states[0])
	data, _ := json.Marshal(map[string]any{"format_version": 1, "peers": []any{map[string]string{
		"folder": folder, "device": a, "url": url, "certificate": "a.pem",
	}}})
	if err := os.WriteFile(filepath.Join(states[1], "peers.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	receiver, _ := start(states[1])
	waitForBytes := func(want []byte) {
		t.Helper()
		deadline := time.Now().Add(12 * time.Second)
		for time.Now().Before(deadline) {
			got, err := os.ReadFile(filepath.Join(roots[1], "note.txt"))
			if err == nil && bytes.Equal(got, want) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("ordinary edit did not arrive through background sync")
	}
	first := []byte("ordinary watched edit\n")
	if err := os.WriteFile(filepath.Join(roots[0], "note.txt"), first, 0600); err != nil {
		t.Fatal(err)
	}
	waitForBytes(first)
	_ = receiver.Process.Kill()
	_ = receiver.Wait()
	second := []byte("edit while receiver offline\n")
	if err := os.WriteFile(filepath.Join(roots[0], "note.txt"), second, 0600); err != nil {
		t.Fatal(err)
	}
	start(states[1])
	waitForBytes(second)

	// Continue producing ordinary small edits until a distinct-chunk archive has
	// actually arrived. Queue scoring alone is not a transfer-progress oracle.
	stopEdits := make(chan struct{})
	editsDone := make(chan struct{})
	var editCount atomic.Int64
	go func() {
		defer close(editsDone)
		for {
			select {
			case <-stopEdits:
				return
			default:
			}
			value := []byte(strings.Repeat("small edit ", 100) + time.Now().String())
			temporary := filepath.Join(roots[0], "small.incoming")
			if os.WriteFile(temporary, value, 0600) == nil && os.Rename(temporary, filepath.Join(roots[0], "small.txt")) == nil {
				editCount.Add(1)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	t.Cleanup(func() { close(stopEdits); <-editsDone })
	large := make([]byte, 16*1024*1024)
	for i := range large {
		large[i] = byte(i ^ (i >> 12) ^ (i >> 20))
	}
	if err := os.WriteFile(filepath.Join(roots[0], "large.bin"), large, 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		got, err := os.ReadFile(filepath.Join(roots[1], "large.bin"))
		if err == nil && bytes.Equal(got, large) {
			if editCount.Load() < 2 {
				t.Fatal("archive transfer did not overlap repeated ordinary edits")
			}
			t.Logf("16-MiB archive arrived while %d ordinary small edits were produced", editCount.Load())
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("large-file transfer did not progress under continuing small-file edits")
}
