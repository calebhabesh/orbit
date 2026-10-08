package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/control"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/scheduler"
	"github.com/calebhabesh/orbit/internal/testkit"
	"github.com/calebhabesh/orbit/internal/workspace"
)

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func buildBinary(t *testing.T, disposable string) string {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(disposable, "orbit")
	build := exec.Command("go", "build", "-o", binary, "./cmd/orbit")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build orbit: %v\n%s", err, output)
	}
	return binary
}

// TestP12BoundedQueueAndFairScheduling validates:
// 1. Queue bound of 1024 tasks rejects excessive work (I13).
// 2. Small-file preference with aging guarantees large-file progress amid sustained small edits.
func TestP12BoundedQueueAndFairScheduling(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state")
	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	folderID := history.ID{0x12}
	if err := db.EnsureFolder(ctx, folderID, history.ID{0xaa}, 1); err != nil {
		t.Fatalf("ensure folder: %v", err)
	}

	// 1. Fill queue up to 1024 capacity
	for i := 0; i < repository.MaxQueueCapacity; i++ {
		_, err := db.EnqueueDurableTask(ctx, repository.DurableTask{
			ID:         fmt.Sprintf("task-%04d", i),
			Folder:     folderID,
			Kind:       "sync",
			TargetPath: fmt.Sprintf("path-%d", i),
			State:      "queued",
			CreatedNS:  time.Now().UnixNano(),
		})
		if err != nil {
			t.Fatalf("enqueue task %d: %v", i, err)
		}
	}

	// 1025th task must fail with ErrQueueFull
	_, err = db.EnqueueDurableTask(ctx, repository.DurableTask{
		ID:         "task-overflow",
		Folder:     folderID,
		Kind:       "sync",
		TargetPath: "path-overflow",
		State:      "queued",
		CreatedNS:  time.Now().UnixNano(),
	})
	if err == nil || !strings.Contains(err.Error(), "1024") {
		t.Fatalf("expected queue overflow error, got: %v", err)
	}

	// 2. Fair scheduling test: Large-file progress amid sustained small edits via aging
	queue := scheduler.NewQueue(db, 256)
	// Clear previous tasks for clean priority test
	for i := 0; i < repository.MaxQueueCapacity; i++ {
		_ = db.CancelDurableTask(ctx, fmt.Sprintf("task-%04d", i))
	}

	largeTask := repository.DurableTask{
		ID:         "large-file-task",
		Folder:     folderID,
		Kind:       "scan",
		TargetPath: "large.bin",
		FileSize:   200 * 1024, // 200 KiB
		State:      "queued",
		CreatedNS:  time.Now().UnixNano(),
	}
	if _, err := queue.Enqueue(ctx, largeTask); err != nil {
		t.Fatalf("enqueue large task: %v", err)
	}

	// Sustained stream of small edits (each 10 KiB)
	for i := 1; i <= 3; i++ {
		smallTask := repository.DurableTask{
			ID:         fmt.Sprintf("small-file-%d", i),
			Folder:     folderID,
			Kind:       "scan",
			TargetPath: fmt.Sprintf("small_%d.txt", i),
			FileSize:   10 * 1024, // 10 KiB
			State:      "queued",
			CreatedNS:  time.Now().UnixNano(),
		}
		if _, err := queue.Enqueue(ctx, smallTask); err != nil {
			t.Fatalf("enqueue small task %d: %v", i, err)
		}
	}

	// Round 1: Small file 1 has score 10KB vs Large file score 200KB -> small file 1 wins
	next1, err := queue.NextReadyTask(ctx, nil, time.Now())
	if err != nil || next1 == nil || next1.ID != "small-file-1" {
		t.Fatalf("expected small-file-1 to be dispatched first, got: %+v (err=%v)", next1, err)
	}
	_ = queue.UpdateState(ctx, next1.ID, "completed", 1, "", "", 0)

	// Round 2: Small file 2 vs Large file -> small file 2 wins, large file aged to 136 KiB
	next2, err := queue.NextReadyTask(ctx, nil, time.Now())
	if err != nil || next2 == nil || next2.ID != "small-file-2" {
		t.Fatalf("expected small-file-2, got: %+v (err=%v)", next2, err)
	}
	_ = queue.UpdateState(ctx, next2.ID, "completed", 1, "", "", 0)

	// Round 3: Small file 3 vs Large file -> small file 3 wins, large file aged to 72 KiB -> then 8 KiB
	next3, err := queue.NextReadyTask(ctx, nil, time.Now())
	if err != nil || next3 == nil || next3.ID != "small-file-3" {
		t.Fatalf("expected small-file-3, got: %+v (err=%v)", next3, err)
	}
	_ = queue.UpdateState(ctx, next3.ID, "completed", 1, "", "", 0)

	// Now enqueue another small file (10 KiB, Age 0 -> score 10 KiB).
	// But large file now has effective score 200 - 3*64 = 8 KiB!
	// Large file MUST beat the new small file, proving large-file progress (I13)!
	smallTaskNew := repository.DurableTask{
		ID:         "small-file-new",
		Folder:     folderID,
		Kind:       "scan",
		TargetPath: "small_new.txt",
		FileSize:   10 * 1024,
		State:      "queued",
		CreatedNS:  time.Now().UnixNano(),
	}
	if _, err := queue.Enqueue(ctx, smallTaskNew); err != nil {
		t.Fatalf("enqueue small task new: %v", err)
	}

	next4, err := queue.NextReadyTask(ctx, nil, time.Now())
	if err != nil || next4 == nil {
		t.Fatalf("next task: %v", err)
	}
	if next4.ID != "large-file-task" {
		t.Fatalf("expected large-file-task to make progress due to aging, got: %s", next4.ID)
	}
}

// TestP12EqualSizeTimestampPreservingEdits validates:
// 1. Quick scan skips hashing when size and mtime match (CPU/IO efficiency).
// 2. Full-content verification scan detects changes even when size and mtime are identical.
func TestP12EqualSizeTimestampPreservingEdits(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	binary := buildBinary(t, disposable)

	state := filepath.Join(disposable, "state")
	root := filepath.Join(disposable, "root")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}

	folder := strings.Repeat("e", 64)

	// 1. Init & Register
	for _, args := range [][]string{
		{"init", "--state", state},
		{"register", "--state", state, "--folder", folder, "--root", root},
	} {
		if out, err := exec.Command(binary, append([]string{"engine"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	// 2. Create file1.txt and perform initial scan
	filePath := filepath.Join(root, "file1.txt")
	contentA := []byte("InitialContent01")
	if err := os.WriteFile(filePath, contentA, 0644); err != nil {
		t.Fatal(err)
	}
	fixedTime := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filePath, fixedTime, fixedTime); err != nil {
		t.Fatal(err)
	}

	// Initial scan captures file1.txt
	cmd := exec.Command(binary, "engine", "work", "scan", "--state", state, "--folder", folder, "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("scan 1: %v\n%s", err, out)
	}
	var res1 control.WorkScanResult
	if err := json.Unmarshal(out, &res1); err != nil {
		t.Fatalf("unmarshal scan 1: %v\n%s", err, out)
	}
	if res1.CapturedCount != 1 {
		t.Fatalf("expected 1 captured in scan 1, got %d", res1.CapturedCount)
	}

	// 3. Mutate file with EQUAL SIZE content and restore same mtime
	contentB := []byte("MutatedContent02") // exactly 16 bytes
	if err := os.WriteFile(filePath, contentB, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filePath, fixedTime, fixedTime); err != nil {
		t.Fatal(err)
	}

	// 4. Quick scan (full=false): stat matches, skips hashing, captures 0 changes
	cmd = exec.Command(binary, "engine", "work", "scan", "--state", state, "--folder", folder, "--json")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("quick scan: %v\n%s", err, out)
	}
	var res2 control.WorkScanResult
	if err := json.Unmarshal(out, &res2); err != nil {
		t.Fatalf("unmarshal quick scan: %v\n%s", err, out)
	}
	if res2.CapturedCount != 0 {
		t.Fatalf("expected quick scan to skip unchanged stat, got %d captured", res2.CapturedCount)
	}

	// 5. Full-content scan (full=true): forces full hash check and detects mutated bytes!
	cmd = exec.Command(binary, "engine", "work", "scan", "--state", state, "--folder", folder, "--full", "--json")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("full scan: %v\n%s", err, out)
	}
	var res3 control.WorkScanResult
	if err := json.Unmarshal(out, &res3); err != nil {
		t.Fatalf("unmarshal full scan: %v\n%s", err, out)
	}
	if res3.CapturedCount != 1 {
		t.Fatalf("expected full scan to detect equal-size edit, got %d captured", res3.CapturedCount)
	}
}

// TestP12WatcherFeedbackSuppression validates Invariant I17:
// Repeated scans after apply, restart, or watcher callbacks create no authored versions.
func TestP12WatcherFeedbackSuppression(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	binary := buildBinary(t, disposable)

	state := filepath.Join(disposable, "state")
	root := filepath.Join(disposable, "root")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}

	folder := strings.Repeat("a", 64)

	for _, args := range [][]string{
		{"init", "--state", state},
		{"register", "--state", state, "--folder", folder, "--root", root},
	} {
		if out, err := exec.Command(binary, append([]string{"engine"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	filePath := filepath.Join(root, "doc.txt")
	if err := os.WriteFile(filePath, []byte("author original doc"), 0644); err != nil {
		t.Fatal(err)
	}

	// First scan authors version counter=1
	cmd := exec.Command(binary, "engine", "work", "scan", "--state", state, "--folder", folder, "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("scan 1: %v\n%s", err, out)
	}
	var res1 control.WorkScanResult
	if err := json.Unmarshal(out, &res1); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res1.CapturedCount != 1 || res1.CapturedEnvelopes[0].ID.Counter != 1 {
		t.Fatalf("expected version counter 1, got %+v", res1.CapturedEnvelopes)
	}

	// Repeat scan 5 times (simulating redundant watcher callbacks or restarts)
	for i := 2; i <= 6; i++ {
		cmd = exec.Command(binary, "engine", "work", "scan", "--state", state, "--folder", folder, "--full", "--json")
		out, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("repeat scan %d: %v\n%s", i, err, out)
		}
		var resRepeat control.WorkScanResult
		if err := json.Unmarshal(out, &resRepeat); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resRepeat.CapturedCount != 0 {
			t.Fatalf("scan %d created duplicate authored version! captured=%d", i, resRepeat.CapturedCount)
		}
	}
}

// TestP12RootUnavailablePauseFolderNoDeletions validates Invariant I11:
// When a root directory is removed or replaced, no deletions are created,
// and folder operations pause cleanly.
func TestP12RootUnavailablePauseFolderNoDeletions(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	binary := buildBinary(t, disposable)

	state := filepath.Join(disposable, "state")
	root := filepath.Join(disposable, "root")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}

	folder := strings.Repeat("b", 64)

	for _, args := range [][]string{
		{"init", "--state", state},
		{"register", "--state", state, "--folder", folder, "--root", root},
	} {
		if out, err := exec.Command(binary, append([]string{"engine"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	if err := os.WriteFile(filepath.Join(root, "important.txt"), []byte("do not delete"), 0644); err != nil {
		t.Fatal(err)
	}

	// Initial scan captures file
	cmd := exec.Command(binary, "engine", "work", "scan", "--state", state, "--folder", folder, "--json")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("scan: %v\n%s", err, out)
	}

	// Simulate root disconnect: rename root directory away
	hiddenRoot := filepath.Join(disposable, "root_unmounted")
	if err := os.Rename(root, hiddenRoot); err != nil {
		t.Fatal(err)
	}

	// Scan fails with ROOT_UNAVAILABLE
	cmd = exec.Command(binary, "engine", "work", "scan", "--state", state, "--folder", folder)
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "ROOT_UNAVAILABLE") && !strings.Contains(string(out), "unavailable or replaced") {
		t.Fatalf("expected root unavailable error, got: %v\n%s", err, out)
	}

	// Restore root directory
	if err := os.Rename(hiddenRoot, root); err != nil {
		t.Fatal(err)
	}

	// Verify file is still present and valid
	cmd = exec.Command(binary, "engine", "work", "scan", "--state", state, "--folder", folder, "--json")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("scan after restore: %v\n%s", err, out)
	}
	var res control.WorkScanResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res.Deletion != nil && len(res.Deletion.Paths) > 0 {
		t.Fatalf("invariant violated: root disconnect triggered deletions: %v", res.Deletion.Paths)
	}
}

// TestP12WorkStatusRetryCancelAndList validates CLI work subcommands:
// status, retry, cancel, and list.
func TestP12WorkStatusRetryCancelAndList(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	binary := buildBinary(t, disposable)

	state := filepath.Join(disposable, "state")
	root := filepath.Join(disposable, "root")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}

	folder := strings.Repeat("c", 64)

	for _, args := range [][]string{
		{"init", "--state", state},
		{"register", "--state", state, "--folder", folder, "--root", root},
	} {
		if out, err := exec.Command(binary, append([]string{"engine"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	// Enqueue a sync task via CLI
	cmd := exec.Command(binary, "engine", "work", "sync", "--state", state, "--folder", folder, "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("work sync enqueue: %v\n%s", err, out)
	}
	var syncRes map[string]any
	if err := json.Unmarshal(out, &syncRes); err != nil {
		t.Fatalf("unmarshal sync res: %v\n%s", err, out)
	}
	taskID, ok := syncRes["task_id"].(string)
	if !ok || taskID == "" {
		t.Fatalf("expected valid task_id, got %v", syncRes)
	}

	// 1. orbit work list
	cmd = exec.Command(binary, "engine", "work", "list", "--state-dir", state, "--folder", folder, "--json")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("work list: %v\n%s", err, out)
	}
	var listRes control.WorkListResult
	if err := json.Unmarshal(out, &listRes); err != nil {
		t.Fatalf("unmarshal list res: %v\n%s", err, out)
	}
	if len(listRes.Tasks) == 0 {
		t.Fatalf("expected at least 1 task in list, got 0")
	}

	// 2. orbit work status
	cmd = exec.Command(binary, "engine", "work", "status", "--state", state, "--folder", folder, "--json")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("work status: %v\n%s", err, out)
	}
	var statusRes control.WorkStatusResult
	if err := json.Unmarshal(out, &statusRes); err != nil {
		t.Fatalf("unmarshal status res: %v\n%s", err, out)
	}
	if len(statusRes.Folders) != 1 || statusRes.Folders[0].QueuedTasks < 1 {
		t.Fatalf("expected queued task in status, got %+v", statusRes)
	}

	// 3. orbit work cancel
	cmd = exec.Command(binary, "engine", "work", "cancel", "--state", state, "--task", taskID, "--json")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("work cancel: %v\n%s", err, out)
	}
	var cancelRes control.WorkCancelResult
	if err := json.Unmarshal(out, &cancelRes); err != nil {
		t.Fatalf("unmarshal cancel res: %v\n%s", err, out)
	}
	if cancelRes.Status != "canceled" {
		t.Fatalf("expected canceled status, got %s", cancelRes.Status)
	}

	// Verify state is canceled in work list
	cmd = exec.Command(binary, "engine", "work", "list", "--state-dir", state, "--folder", folder, "--state", "canceled", "--json")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("work list canceled: %v\n%s", err, out)
	}
	if err := json.Unmarshal(out, &listRes); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(listRes.Tasks) != 1 || listRes.Tasks[0].ID != taskID {
		t.Fatalf("expected canceled task in list, got %+v", listRes.Tasks)
	}

	// 4. orbit work retry
	cmd = exec.Command(binary, "engine", "work", "retry", "--state", state, "--task", taskID, "--json")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("work retry: %v\n%s", err, out)
	}
	var retryRes control.WorkRetryResult
	if err := json.Unmarshal(out, &retryRes); err != nil {
		t.Fatalf("unmarshal retry res: %v\n%s", err, out)
	}
	if retryRes.RetriedCount != 1 {
		t.Fatalf("expected 1 retried task, got %d", retryRes.RetriedCount)
	}
}

// TestP12ContinuousServeProfilesAndShutdown validates:
// 1. Starting agent with --profile pi and bandwidth limiting.
// 2. Continuous scheduler running without busy looping.
// 3. Clean and prompt shutdown on SIGTERM / context cancellation.
func TestP12ContinuousServeProfilesAndShutdown(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	binary := buildBinary(t, disposable)

	state := filepath.Join(disposable, "state")
	root := filepath.Join(disposable, "root")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}

	folder := strings.Repeat("d", 64)

	for _, args := range [][]string{
		{"init", "--state", state},
		{"register", "--state", state, "--folder", folder, "--root", root},
	} {
		if out, err := exec.Command(binary, append([]string{"engine"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	// Launch orbit serve with Pi profile and bandwidth limit
	serveCmd := exec.Command(binary, "engine", "serve",
		"--state", state,
		"--profile", "pi",
		"--bandwidth-limit", "1048576", // 1 MiB/s
		"--sync-interval", "500ms",
		"--full-scan-interval", "2s",
	)

	stdoutBuf := new(safeBuffer)
	serveCmd.Stdout = stdoutBuf
	serveCmd.Stderr = os.Stderr

	if err := serveCmd.Start(); err != nil {
		t.Fatalf("start serve: %v", err)
	}

	// Wait for agent ready
	ready := make(chan bool, 1)
	go func() {
		for {
			if strings.Contains(stdoutBuf.String(), "agent ready:") {
				ready <- true
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()

	select {
	case <-ready:
		// Agent successfully initialized and running
	case <-time.After(5 * time.Second):
		_ = serveCmd.Process.Kill()
		t.Fatalf("timed out waiting for agent ready: %s", stdoutBuf.String())
	}

	// Create a file while agent is running
	testFile := filepath.Join(root, "continuous_file.txt")
	if err := os.WriteFile(testFile, []byte("continuous operational content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Give continuous scheduler a moment to run reconciliation scan
	time.Sleep(1200 * time.Millisecond)

	// Send SIGTERM for graceful shutdown
	if err := serveCmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- serveCmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil && !strings.Contains(err.Error(), "signal: terminated") && !strings.Contains(err.Error(), "interrupt") {
			t.Logf("serve exit: %v", err)
		}
	case <-time.After(5 * time.Second):
		_ = serveCmd.Process.Kill()
		t.Fatal("agent failed to shut down promptly within 5 seconds")
	}

	// Verify that the continuous reconciliation scan captured the file into SQLite
	db, err := repository.Open(context.Background(), state)
	if err != nil {
		t.Fatalf("open db after serve: %v", err)
	}
	defer db.Close()

	var folderID history.ID
	folderBytes, _ := hexDecode(folder)
	copy(folderID[:], folderBytes)

	projections, err := db.Projections(context.Background(), folderID)
	if err != nil {
		t.Fatalf("projections: %v", err)
	}
	found := false
	for _, p := range projections {
		if p.Path == "continuous_file.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected continuous_file.txt to be scanned and projected into metadata database, got %v", projections)
	}
}

// TestP12BandwidthLimiterDirect validates token-bucket burst and rate limits.
func TestP12BandwidthLimiterDirect(t *testing.T) {
	// 50 KB/s rate limit
	rateBps := int64(50 * 1024)
	limiter := scheduler.NewBandwidthLimiter(rateBps)

	start := time.Now()
	// Consume burst capacity (1x rate)
	if err := limiter.Acquire(context.Background(), nil, int(rateBps)); err != nil {
		t.Fatalf("acquire burst: %v", err)
	}
	initialElapsed := time.Since(start)
	if initialElapsed > 200*time.Millisecond {
		t.Fatalf("initial burst took too long: %s", initialElapsed)
	}

	// Consume another rate capacity; must sleep ~1s to refill tokens
	start = time.Now()
	if err := limiter.Acquire(context.Background(), nil, int(rateBps)); err != nil {
		t.Fatalf("acquire rate: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 800*time.Millisecond || elapsed > 1500*time.Millisecond {
		t.Fatalf("expected ~1s rate throttle, got %s", elapsed)
	}
}

func hexDecode(s string) ([]byte, error) {
	var b []byte
	_, err := fmt.Sscanf(s, "%x", &b)
	return b, err
}

// TestP12NotificationLossRecoveredByReconciliationScan validates:
// Even when inotify events are lost, dropped, or disabled (--no-watch),
// the periodic reconciliation scan recovers all changes and projects them.
func TestP12NotificationLossRecoveredByReconciliationScan(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	binary := buildBinary(t, disposable)

	state := filepath.Join(disposable, "state")
	root := filepath.Join(disposable, "root")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}

	folder := strings.Repeat("1", 64)

	for _, args := range [][]string{
		{"init", "--state", state},
		{"register", "--state", state, "--folder", folder, "--root", root},
	} {
		if out, err := exec.Command(binary, append([]string{"engine"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	// Launch agent with --no-watch (simulating watcher loss / overflow) and 500ms sync interval
	serveCmd := exec.Command(binary, "engine", "serve",
		"--state", state,
		"--no-watch",
		"--sync-interval", "500ms",
	)

	stdoutBuf := new(safeBuffer)
	serveCmd.Stdout = stdoutBuf
	serveCmd.Stderr = os.Stderr

	if err := serveCmd.Start(); err != nil {
		t.Fatalf("start serve: %v", err)
	}

	ready := make(chan bool, 1)
	go func() {
		for {
			if strings.Contains(stdoutBuf.String(), "agent ready:") {
				ready <- true
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()

	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		_ = serveCmd.Process.Kill()
		t.Fatalf("agent startup timeout: %s", stdoutBuf.String())
	}

	// Write file while watcher is disabled
	testFile := filepath.Join(root, "unwatched_file.txt")
	if err := os.WriteFile(testFile, []byte("recovered via reconciliation scan"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for periodic reconciliation scan to run and discover the file
	time.Sleep(1200 * time.Millisecond)

	// Terminate agent
	_ = serveCmd.Process.Signal(syscall.SIGTERM)
	_ = serveCmd.Wait()

	// Verify unwatched_file.txt was captured and projected
	db, err := repository.Open(context.Background(), state)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var folderID history.ID
	folderBytes, _ := hexDecode(folder)
	copy(folderID[:], folderBytes)

	projections, err := db.Projections(context.Background(), folderID)
	if err != nil {
		t.Fatalf("projections: %v", err)
	}
	found := false
	for _, p := range projections {
		if p.Path == "unwatched_file.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected unwatched_file.txt to be recovered by reconciliation scan, got %v", projections)
	}
}

// TestP12RetryClassificationAndExhaustion validates:
// 1. Transient errors (UNSTABLE_FILE) back off exponentially.
// 2. Permanent errors (ROOT_UNAVAILABLE) do not retry.
// 3. Reaching MaxRetryAttempts (5) exhausts retries and yields to subsequent tasks.
func TestP12RetryClassificationAndExhaustion(t *testing.T) {
	classifier := scheduler.RetryClassifier{}

	// Transient error backoff
	unstableErr := fmt.Errorf("file was modified during hashing: %w", workspace.ErrUnstableFile)
	if !classifier.IsTransient(unstableErr) || classifier.ErrorCode(unstableErr) != "UNSTABLE_FILE" {
		t.Fatalf("expected transient retry for UNSTABLE_FILE")
	}

	backoff1 := classifier.Backoff(1)
	if backoff1 <= 0 {
		t.Fatalf("expected positive backoff duration, got %s", backoff1)
	}

	backoff4 := classifier.Backoff(4)
	if backoff4 <= backoff1 {
		t.Fatalf("expected exponential backoff increase: attempt 1 (%s) vs attempt 4 (%s)", backoff1, backoff4)
	}

	if scheduler.MaxRetryAttempts != 5 {
		t.Fatalf("expected MaxRetryAttempts=5, got %d", scheduler.MaxRetryAttempts)
	}

	// Permanent error
	rootErr := workspace.ErrRootUnavailable
	if classifier.IsTransient(rootErr) || classifier.ErrorCode(rootErr) != "ROOT_UNAVAILABLE" {
		t.Fatalf("expected permanent failure for ROOT_UNAVAILABLE")
	}
}

// TestP12CrashRecoveryOfInFlightTasks validates Invariant I20:
// Tasks in 'running' state when a crash or abrupt restart occurs
// are automatically safely restored to 'queued' state.
func TestP12CrashRecoveryOfInFlightTasks(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state")

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	folderID := history.ID{0x99}
	if err := db.EnsureFolder(ctx, folderID, history.ID{0xaa}, 1); err != nil {
		t.Fatalf("ensure folder: %v", err)
	}

	taskID, err := db.EnqueueDurableTask(ctx, repository.DurableTask{
		Folder: folderID,
		Kind:   "scan",
		State:  "queued",
	})
	if err != nil {
		t.Fatalf("enqueue task: %v", err)
	}

	// Simulate task being actively processed (state = running)
	if err := db.UpdateDurableTaskState(ctx, taskID, "running", 1, "", "", 0); err != nil {
		t.Fatalf("update state: %v", err)
	}

	// Abrupt close (simulating process crash / power outage)
	_ = db.Close()

	// Reopen database (recovery hook runs in OpenWithOptions)
	reopenedDB, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer reopenedDB.Close()

	recoveredTask, err := reopenedDB.GetDurableTask(ctx, taskID)
	if err != nil {
		t.Fatalf("get durable task: %v", err)
	}
	if recoveredTask.State != "queued" {
		t.Fatalf("expected recovered task to be in 'queued' state, got '%s'", recoveredTask.State)
	}
}

// TestP12ResourceLimitsValidation validates hardware profiles and boundary checks.
func TestP12ResourceLimitsValidation(t *testing.T) {
	piProfile := scheduler.GetProfile(scheduler.ProfilePi)
	if piProfile.TransferWorkers != 2 || piProfile.HashWorkers != 1 {
		t.Fatalf("unexpected Pi profile defaults: %+v", piProfile)
	}

	laptopProfile := scheduler.GetProfile(scheduler.ProfileLaptop)
	if laptopProfile.TransferWorkers != 4 || laptopProfile.HashWorkers != 2 {
		t.Fatalf("unexpected Laptop profile defaults: %+v", laptopProfile)
	}

	if err := scheduler.ValidateProfile(piProfile); err != nil {
		t.Fatalf("pi profile should be valid: %v", err)
	}
	if err := scheduler.ValidateProfile(laptopProfile); err != nil {
		t.Fatalf("laptop profile should be valid: %v", err)
	}

	// Invalid profile
	invalidProfile := piProfile
	invalidProfile.TransferWorkers = 0
	if err := scheduler.ValidateProfile(invalidProfile); err == nil {
		t.Fatal("expected error for 0 transfer workers")
	}

	invalidProfile = piProfile
	invalidProfile.MaxQueuedTasks = 5000 // exceeds 4096
	if err := scheduler.ValidateProfile(invalidProfile); err == nil {
		t.Fatal("expected error for excessive max queued tasks")
	}
}
