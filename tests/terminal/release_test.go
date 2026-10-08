package terminal_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
	"github.com/calebhabesh/orbit/internal/testkit"
)

func TestTerminalT13OperationObservesPublicationWithoutReauthoring(t *testing.T) {
	ctx := context.Background()
	f := fresh(t)
	folder := f.folder("operation-observation")
	base := t08Capture(t, f, folder, "operation-observation", "doc", "original")
	t08Capture(t, f, folder, "operation-observation", "doc", "edited")
	client := terminalClient(t, f)
	review := t08Review(t, client, folder, "doc", &base.ID, "")
	mutation := t08Mutation(review, "restore", base.ID, "7a")
	fault := control.New(f.db, f.ws, control.Options{LocalDevice: f.device, FaultHook: func(name string) error {
		if name == "terminal.content.committed" {
			return errors.New("stop after durable resolution before publication")
		}
		return nil
	}})
	if _, err := fault.TerminalMutate(ctx, mutation); err == nil {
		t.Fatal("publication boundary not reached")
	}
	query := tc.Query{Version: tc.Version, Kind: "operation", ID: mutation.OperationID}
	before, err := client.Query(ctx, query)
	if err != nil || before.Operation.State == "completed" || len(before.Effects) != 1 {
		t.Fatalf("premature publication: %+v %v", before, err)
	}
	heads, err := f.db.Heads(ctx, folder, "doc")
	if err != nil || len(heads) != 1 {
		t.Fatalf("committed heads=%v err=%v", heads, err)
	}
	if err := f.ws.Apply(ctx, heads[0].ID); err != nil {
		t.Fatal(err)
	}
	after, err := client.Query(ctx, query)
	if err != nil || after.Operation.State != "completed" || len(after.Effects) != 1 || after.Effects[0].State != "applied" || *after.Effects[0].Version != *before.Effects[0].Version {
		t.Fatalf("stale publication observation: %+v %v", after, err)
	}
	unchanged, err := f.db.Heads(ctx, folder, "doc")
	if err != nil || len(unchanged) != 1 || unchanged[0].ID != heads[0].ID {
		t.Fatalf("query reauthored durable effect: %v %v", unchanged, err)
	}
}

func TestTerminalT13UnavailableRootKeepsCauseAfterAutomaticPause(t *testing.T) {
	f := fresh(t)
	folder := f.folder("root-cause")
	root := filepath.Join(f.root, "root-cause")
	if err := os.Rename(root, root+"-missing"); err != nil {
		t.Fatal(err)
	}
	if err := f.ws.Revalidate(context.Background(), folder); err == nil {
		t.Fatal("missing root was accepted")
	}
	client := terminalClient(t, f)
	r, err := client.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "attention", Folder: hex.EncodeToString(folder[:]), Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, attention := range r.Attention {
		if attention.Code == "ROOT_UNAVAILABLE" {
			return
		}
	}
	t.Fatalf("automatic pause hid the root cause: %+v", r.Attention)
}

// This Linux process experiment measures live CLI/editor costs separately from
// fixture construction in the Go parent. Sampling can miss brief peaks; values
// are observations, not a constant-memory proof or hardware throughput target.
func TestTerminalT13LargeDirectoryAndStreamedMergeResources(t *testing.T) {
	f := fresh(t)
	folder := f.folder("release")
	root := filepath.Join(f.root, "release")
	const entries = 1024
	for i := 0; i < entries; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("entry-%04d", i)), []byte("small fixture\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	scan, err := f.ws.Scan(context.Background(), folder)
	if err != nil || len(scan.Captured) != entries {
		t.Fatalf("large-directory capture=%d %v", len(scan.Captured), err)
	}
	// Bound the deleted-files terminal response over hundreds of real tombstones.
	for i := 0; i < entries/2; i++ {
		if err := os.Remove(filepath.Join(root, fmt.Sprintf("entry-%04d", i))); err != nil {
			t.Fatal(err)
		}
	}
	scan, err = f.ws.Scan(context.Background(), folder)
	if err != nil || len(scan.Captured) != 0 || scan.Deletion == nil || len(scan.Deletion.Paths) != entries/2 {
		t.Fatalf("mass-delete preview=%+v %v", scan.Deletion, err)
	}
	if deleted, err := f.ws.ApproveDeletions(context.Background(), folder, scan.Deletion.Token); err != nil || len(deleted) != entries/2 {
		t.Fatalf("reviewed mass-delete=%d %v", len(deleted), err)
	}
	for _, mib := range []int{8, 32} {
		t08Capture(t, f, folder, "release", fmt.Sprintf("merge-%d", mib), strings.Repeat("L", mib<<20))
	}
	_ = terminalClient(t, f)
	binary := buildOrbitBinary(t, f.root)
	f.close()
	daemon := exec.Command(binary, "serve", "--state", f.state, "--control-listen", "127.0.0.1:0", "--no-watch", "--sync-interval", "1h")
	if err := daemon.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := testkit.ValidateDestructiveTarget(f.root, f.state); err != nil {
			t.Fatal(err)
		}
		_ = daemon.Process.Signal(syscall.SIGTERM)
		_ = daemon.Wait()
	}()
	client := &controlclient.Client{StateDir: f.state}
	until := time.Now().Add(15 * time.Second)
	for {
		pid, _ := os.ReadFile(filepath.Join(f.state, ".agent.pid"))
		if strings.TrimSpace(string(pid)) == strconv.Itoa(daemon.Process.Pid) {
			if _, err := client.Query(context.Background(), tc.Query{Version: "1", Kind: "capabilities"}); err == nil {
				break
			}
		}
		if time.Now().After(until) {
			t.Fatal("daemon unavailable")
		}
		time.Sleep(25 * time.Millisecond)
	}
	run := func(args ...string) []byte {
		t.Helper()
		command := exec.Command(binary, args[1:]...)
		command.Dir = root
		var output bytes.Buffer
		command.Stdout = &output
		command.Stderr = &output
		started := time.Now()
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		var workers sync.WaitGroup
		samples, peakRSS, peakFD := 0, 0, 0
		workers.Add(1)
		go func() {
			defer workers.Done()
			ticker := time.NewTicker(5 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", command.Process.Pid))
					if err == nil {
						match := regexp.MustCompile(`VmRSS:\s+(\d+)`).FindSubmatch(data)
						if len(match) == 2 {
							rss, _ := strconv.Atoi(string(match[1]))
							if rss > peakRSS {
								peakRSS = rss
							}
							samples++
						}
					}
					files, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", command.Process.Pid))
					if err == nil && len(files) > peakFD {
						peakFD = len(files)
					}
				}
			}
		}()
		err := command.Wait()
		close(done)
		workers.Wait()
		usage := command.ProcessState.SysUsage().(*syscall.Rusage)
		t.Logf("resource command=%s input_fixture_files=%d samples=%d interval_ms=5 sampled_peak_rss_kib=%d child_ru_maxrss_kib=%d sampled_peak_fds=%d output_bytes=%d seconds=%.3f", args[1], entries, samples, peakRSS, usage.Maxrss, peakFD, output.Len(), time.Since(started).Seconds())
		if err != nil {
			t.Fatalf("CLI %v: %v %s", args, err, output.Bytes())
		}
		if bytes.Contains(output.Bytes(), []byte("\x1b")) {
			t.Fatal("terminal escapes in piped output")
		}
		return output.Bytes()
	}
	deleted := run("orbit", "deleted", "--folder", "release", "--state", f.state, "--limit", "20", "--json")
	var page tc.Result
	if err := json.Unmarshal(deleted, &page); err != nil || len(page.Versions) != 20 || page.Cursor == "" {
		t.Fatalf("bounded deleted page=%s %v", deleted, err)
	}
	run("orbit", "status", "--state", f.state, "--json")
	tool := filepath.Join(f.root, "stream editor.py")
	if err := os.WriteFile(tool, []byte("import os, sys\nremaining=int(sys.argv[1])\nwith open(sys.argv[2], 'wb') as f:\n while remaining:\n  n=min(1048576, remaining)\n  f.write(b'M'*n)\n  remaining-=n\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mib := range []int{8, 32} {
		path := fmt.Sprintf("merge-%d", mib)
		review := filepath.Join(f.root, path+"-review.json")
		run("orbit", "conflicts", "show", path, "--folder", "release", "--state", f.state, "--out", review, "--json")
		staged := run("orbit", "conflicts", "edit", path, "--folder", "release", "--state", f.state, "--review-file", review, "--tool", "python3 '"+tool+"' "+strconv.Itoa(mib<<20), "--json")
		var uploaded tc.Result
		if err := json.Unmarshal(staged, &uploaded); err != nil || uploaded.Upload == nil {
			t.Fatalf("staged upload: %v %s", err, staged)
		}
		run("orbit", "conflicts", "merge", path, "--folder", "release", "--state", f.state, "--review-file", review, "--session", uploaded.Upload.Session, "--upload", uploaded.Upload.ID, "--digest", uploaded.Upload.Digest, "--bytes", strconv.Itoa(mib<<20), "--json")
		file, err := os.Open(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.New()
		count, err := io.Copy(digest, file)
		file.Close()
		if err != nil || count != int64(mib<<20) || hex.EncodeToString(digest.Sum(nil)) != uploaded.Upload.Digest {
			t.Fatal("large merged bytes/hash differ")
		}
		t.Logf("verified streamed merge fixture_bytes=%d digest=%s", count, uploaded.Upload.Digest)
	}
}

func TestTerminalT13NativeHarnessRefusesUnsafeActions(t *testing.T) {
	command := exec.Command("python3", "-m", "unittest", "discover", "-s", "scripts/validation", "-p", "test_safety.py")
	command.Dir = "../.."
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("native harness safety: %v %s", err, out)
	}
}

func TestTerminalT13StatusKeepsRunningAndStartupSeparate(t *testing.T) {
	f := fresh(t)
	t.Setenv("PATH", t.TempDir())
	settings := config.DefaultRuntimeSettings()
	settings.Startup = "unattended"
	if err := config.SaveRuntimeSettings(f.state, settings); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"service", "status"} {
		result, err := f.ctrl.TerminalQuery(context.Background(), tc.Query{Version: "1", Kind: kind})
		if err != nil || result.Service == nil {
			t.Fatal(kind, err)
		}
		service := result.Service
		if !service.Running || service.Enabled || service.UnattendedVerified || service.Mode != "manual" {
			t.Fatalf("%s conflates desired startup with observed state: %+v", kind, service)
		}
	}
}

func TestTerminalT13ReadinessRetainsQuarantineAndConcurrentHeads(t *testing.T) {
	f := fresh(t)
	folder := f.folder("readiness")
	local := t08Capture(t, f, folder, "readiness", "doc", "captured local bytes")
	check := func(missing, conflicts tc.Uint) {
		t.Helper()
		var readiness tc.Readiness
		if err := f.db.OnboardingReadiness(context.Background(), folder, &readiness); err != nil {
			t.Fatal(err)
		}
		if readiness.MissingContent != missing || readiness.Conflicts != conflicts || readiness.PendingPublication != 0 {
			t.Fatalf("readiness=%+v want missing=%d conflicts=%d", readiness, missing, conflicts)
		}
	}
	check(0, 0)
	if _, err := f.db.QuarantineChunk(context.Background(), local.Manifest.Chunks[0].Digest, "isolated T13 quarantine observation"); err != nil {
		t.Fatal(err)
	}
	check(1, 0)
	t08Remote(t, f, folder, "doc", "independent remote bytes", 91)
	check(1, 1)
	// A quarantined captured object must never erase the protected working copy.
	if data, err := os.ReadFile(filepath.Join(f.root, "readiness", "doc")); err != nil || string(data) != "captured local bytes" {
		t.Fatal("quarantine changed working bytes")
	}
}
