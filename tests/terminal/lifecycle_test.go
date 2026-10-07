package terminal_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/app"
	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/controlclient"
	"github.com/calebhabesh/file-sync/internal/launcher"
	"github.com/calebhabesh/file-sync/internal/state"
	"github.com/calebhabesh/file-sync/internal/testkit"
)

func terminalClient(t *testing.T, f *fixture) *controlclient.Client {
	t.Helper()
	address := strings.TrimPrefix(f.server.URL, "http://")
	if err := os.WriteFile(filepath.Join(f.state, "control.addr"), []byte(address), 0600); err != nil {
		t.Fatal(err)
	}
	return &controlclient.Client{StateDir: f.state}
}
func querySettings(t *testing.T, c *controlclient.Client) tc.Result {
	t.Helper()
	r, err := c.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "settings"})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func settingsMutation(r tc.Result, id string) tc.Mutation {
	s := *r.Settings
	s.DataBudget += 1024
	return tc.Mutation{Version: tc.Version, Kind: "settings", OperationID: strings.Repeat(id, 64), Settings: &tc.SettingsIntent{Review: *r.Review, Settings: s}}
}

func TestTerminalT02LiveStoppedSettingsReplay(t *testing.T) {
	f := fresh(t)
	c := terminalClient(t, f)
	r := querySettings(t, c)
	m := settingsMutation(r, "1")
	first, err := c.Mutate(context.Background(), m)
	if err != nil || first.Operation == nil || first.Operation.State != "completed" {
		t.Fatalf("mutation: %v", err)
	}
	replay, err := c.Mutate(context.Background(), m)
	if err != nil || replay.Operation.Fingerprint != first.Operation.Fingerprint {
		t.Fatalf("replay: %v", err)
	}
	changed := m
	copySettings := *m.Settings
	copySettings.Settings.DataBudget++
	changed.Settings = &copySettings
	if _, err := c.Mutate(context.Background(), changed); err == nil || !strings.Contains(err.Error(), "IDEMPOTENCY_CONFLICT") {
		t.Fatal("changed retry accepted")
	}
	f.close()
	r = querySettings(t, c)
	if r.Settings.DataBudget != m.Settings.Settings.DataBudget {
		t.Fatal("stopped adapter lost settings")
	}
	replay, err = c.Mutate(context.Background(), m)
	if err != nil || replay.Operation.ID != m.OperationID {
		t.Fatalf("stopped replay: %v", err)
	}
}
func TestTerminalT02NoFallbackAfterLiveFailure(t *testing.T) {
	f := fresh(t)
	c := terminalClient(t, f)
	for _, test := range []string{"stale_endpoint", "bad_credential", "public_credential", "symlink_credential"} {
		t.Run(test, func(t *testing.T) {
			address := strings.TrimPrefix(f.server.URL, "http://")
			os.WriteFile(filepath.Join(f.state, "control.addr"), []byte(address), 0600)
			os.Remove(filepath.Join(f.state, "control.token"))
			os.WriteFile(filepath.Join(f.state, "control.token"), []byte(f.token), 0600)
			switch test {
			case "stale_endpoint":
				os.WriteFile(filepath.Join(f.state, "control.addr"), []byte("127.0.0.1:1"), 0600)
			case "bad_credential":
				os.WriteFile(filepath.Join(f.state, "control.token"), []byte(strings.Repeat("b", 64)), 0600)
			case "public_credential":
				os.Chmod(filepath.Join(f.state, "control.token"), 0644)
			case "symlink_credential":
				os.Remove(filepath.Join(f.state, "control.token"))
				os.Symlink(filepath.Join(f.state, "config.json"), filepath.Join(f.state, "control.token"))
			}
			called := false
			err := c.WithController(context.Background(), func() error { return c.Call(context.Background(), http.MethodGet, "/api/v1/settings", nil, new(any)) }, func(*control.Controller) error { called = true; return nil })
			if err == nil || called {
				t.Fatal("live failure opened stopped state")
			}
		})
	}
}
func TestTerminalT02EndpointAndBounds(t *testing.T) {
	for _, address := range []string{"http://example.com:80", "http://127.0.0.1:80/path", "http://user@127.0.0.1:80", "http://127.0.0.1:80?x=1", "http://0.0.0.0:80", "https://127.0.0.1:80"} {
		if _, err := controlclient.Endpoint(address); err == nil {
			t.Fatalf("unsafe endpoint accepted: %s", address)
		}
	}
	f := fresh(t)
	c := terminalClient(t, f)
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Orbit-Device", fmt.Sprintf("%x", f.device))
		io.WriteString(w, strings.Repeat("x", tc.MaxMetadata+1))
	}))
	defer big.Close()
	os.WriteFile(filepath.Join(f.state, "control.addr"), []byte(strings.TrimPrefix(big.URL, "http://")), 0600)
	if err := c.Call(context.Background(), http.MethodGet, "/api/v1/settings", nil, nil); err == nil || !strings.Contains(err.Error(), "PAYLOAD_TOO_LARGE") {
		t.Fatal("unbounded response accepted")
	}
	// A redirect cannot disclose the local bearer to a different listener.
	var reached atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Store(true) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer redirect.Close()
	os.WriteFile(filepath.Join(f.state, "control.addr"), []byte(strings.TrimPrefix(redirect.URL, "http://")), 0600)
	if err := c.Call(context.Background(), http.MethodGet, "/api/v1/settings", nil, nil); err == nil || reached.Load() {
		t.Fatal("redirect followed")
	}
}
func TestTerminalT02IdentityCancellationAndAuth(t *testing.T) {
	f := fresh(t)
	c := terminalClient(t, f)
	other := fresh(t)
	os.WriteFile(filepath.Join(f.state, "control.addr"), []byte(strings.TrimPrefix(other.server.URL, "http://")), 0600)
	os.WriteFile(filepath.Join(f.state, "control.token"), []byte(other.token), 0600)
	if _, err := c.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "settings"}); err == nil || !strings.Contains(err.Error(), "IDENTITY_MISMATCH") {
		t.Fatal("wrong selected daemon accepted")
	}
	c = terminalClient(t, f)
	os.WriteFile(filepath.Join(f.state, "control.token"), []byte(f.token), 0600)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Query(ctx, tc.Query{Version: tc.Version, Kind: "settings"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	querySettings(t, c)
	if status := api(t, f.server.URL, "", http.MethodPost, "/control/terminal/v1/query", tc.Query{Version: tc.Version, Kind: "settings"}, nil); status != http.StatusUnauthorized {
		t.Fatal("terminal namespace unauthenticated")
	}
}
func TestTerminalT02FiniteInitializerAndPartialRestart(t *testing.T) {
	root := testkit.NewDisposable(t)
	dir := filepath.Join(root, "state")
	ctx := context.Background()
	if err := state.EnsureDirectory(dir); err != nil {
		t.Fatal(err)
	}
	// Limits-only interruption is resumable before identity creation.
	if err := config.InitializeStorageLimits(dir); err != nil {
		t.Fatal(err)
	}
	cfg, err := app.Initialize(ctx, dir, app.SystemDependencies())
	if err != nil {
		t.Fatal(err)
	}
	identityBefore, err := os.ReadFile(filepath.Join(dir, "identity", "peer-identity.pem"))
	if err != nil {
		t.Fatal(err)
	}
	again, err := app.Initialize(ctx, dir, app.SystemDependencies())
	if err != nil || again.DeviceID != cfg.DeviceID {
		t.Fatal("reinitialization changed identity")
	}
	identityAfter, _ := os.ReadFile(filepath.Join(dir, "identity", "peer-identity.pem"))
	if string(identityBefore) != string(identityAfter) {
		t.Fatal("key replaced")
	}
	limits, err := config.LoadStorageLimits(dir)
	if err != nil || limits.DataBudgetBytes == 0 || limits.MetadataBudgetBytes == 0 || limits.FreeSpaceReserveBytes == 0 {
		t.Fatal("nonfinite initialization")
	}
	if err := testkit.ValidateDestructiveTarget(root, dir); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(dir, "config.json"))
	if _, err := app.Initialize(ctx, dir, app.SystemDependencies()); err == nil {
		t.Fatal("missing identity config regenerated over history")
	}
}
func TestTerminalT02LegacyLimitsReview(t *testing.T) {
	f := fresh(t)
	f.close()
	if err := testkit.ValidateDestructiveTarget(f.root, f.state); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(f.state, "limits.json"))
	c := &controlclient.Client{StateDir: f.state}
	r := querySettings(t, c)
	if r.State != "blocked" || r.Error == nil || r.Error.Code != "LIMITS_REVIEW_REQUIRED" {
		t.Fatal("legacy unlimited state hidden")
	}
	s := config.DefaultRuntimeSettings()
	m := tc.Mutation{Version: tc.Version, Kind: "settings", OperationID: strings.Repeat("2", 64), Settings: &tc.SettingsIntent{Review: *r.Review, Settings: s}}
	result, err := c.Mutate(context.Background(), m)
	if err != nil || result.Operation.State != "completed" {
		t.Fatalf("migration: %v", err)
	}
	limits, err := config.LoadStorageLimits(f.state)
	if err != nil || limits.DataBudgetBytes != uint64(s.DataBudget) {
		t.Fatal("reviewed limits not used")
	}
}
func TestTerminalT02LostResponseRecoveryAndExpiredReplay(t *testing.T) {
	f := fresh(t)
	c := terminalClient(t, f)
	m := settingsMutation(querySettings(t, c), "3")
	crashing := control.New(f.db, f.ws, control.Options{LocalDevice: f.device, FaultHook: func(name string) error {
		if name == "terminal.operation.accepted" {
			return errors.New("injected response loss")
		}
		return nil
	}})
	if _, err := crashing.TerminalMutate(context.Background(), m); err == nil {
		t.Fatal("fault did not fire")
	}
	f.close()
	f.open()
	if err := f.ctrl.RecoverTerminalOperations(context.Background()); err != nil {
		t.Fatal(err)
	}
	c = terminalClient(t, f)
	result, err := c.Mutate(context.Background(), m)
	if err != nil || result.Operation.State != "completed" {
		t.Fatalf("recovery: %v", err)
	}
	later := control.New(f.db, f.ws, control.Options{LocalDevice: f.device, Now: func() time.Time { return time.Now().Add(25 * time.Hour) }})
	if _, err := later.TerminalMutate(context.Background(), m); err == nil || !strings.Contains(err.Error(), "EXPIRED_REPLAY") {
		t.Fatal("expired replay executed")
	}
}
func TestTerminalT02StaleReviewNetworkSettings(t *testing.T) {
	f := fresh(t)
	c := terminalClient(t, f)
	old := querySettings(t, c)
	m := settingsMutation(old, "4")
	if _, err := c.Mutate(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	stale := settingsMutation(old, "5")
	if _, err := c.Mutate(context.Background(), stale); err == nil || !strings.Contains(err.Error(), "STALE_VIEW") {
		t.Fatal("stale settings review accepted")
	}
	r := querySettings(t, c)
	n := settingsMutation(r, "6")
	n.Settings.Settings.PeerListen = "127.0.0.1:0"
	n.Settings.Settings.AdvertisedPeer = "192.0.2.10:8443"
	result, err := c.Mutate(context.Background(), n)
	if err != nil || result.Error != nil {
		t.Fatalf("network settings: %v", err)
	}
	saved, err := config.LoadRuntimeSettings(f.state)
	if err != nil || saved.PeerListen != "127.0.0.1:0" {
		t.Fatal("network settings not persisted")
	}
	saved.AdvertisedPeer = "127.0.0.1:8443"
	if err := config.SaveRuntimeSettings(f.state, saved); err == nil {
		t.Fatal("loopback advertised to peers")
	}
}

// Helper executes real ownership, listeners, recovery and signal handling in a
// child process. No daemon is tied to the launch client's context.
func TestTerminalT02DaemonProcess(t *testing.T) {
	dir := os.Getenv("ORBIT_T02_CHILD_STATE")
	if dir == "" {
		t.Skip("subprocess entry only")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer cancel()
	err := app.ServeWithOptions(ctx, dir, app.ServeOptions{ControlAddress: "127.0.0.1:0", AllowInitialize: true, NoWatch: true, Ready: os.Stdout})
	if err != nil {
		t.Fatal(err)
	}
}
func TestTerminalT02ConcurrentProcessLaunchAndLifetime(t *testing.T) {
	root := testkit.NewDisposable(t)
	dir := filepath.Join(root, "state")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var starts atomic.Int32
	var processMu sync.Mutex
	var children []*exec.Cmd
	outputs := map[*exec.Cmd]*bytes.Buffer{}
	starter := func(context.Context, string, string) error {
		starts.Add(1)
		cmd := exec.Command(os.Args[0], "-test.run=^TestTerminalT02DaemonProcess$")
		cmd.Env = append(os.Environ(), "ORBIT_T02_CHILD_STATE="+dir)
		output := new(bytes.Buffer)
		cmd.Stdout = output
		cmd.Stderr = output
		if err := cmd.Start(); err != nil {
			return err
		}
		processMu.Lock()
		children = append(children, cmd)
		outputs[cmd] = output
		processMu.Unlock()
		return nil
	}
	defer func() {
		for _, cmd := range children {
			cmd.Process.Signal(syscall.SIGTERM)
			cmd.Wait()
		}
	}()
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() {
			_, err := launcher.EnsureDaemon(ctx, launcher.LaunchOptions{StateDir: dir, DaemonStarter: starter})
			results <- err
		}()
	}
	for i := 0; i < 8; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if starts.Load() != 1 {
		t.Fatalf("started %d daemons", starts.Load())
	}
	cancel()
	c := &controlclient.Client{StateDir: dir}
	r := querySettings(t, c)
	if r.Settings.DataBudget == 0 {
		t.Fatal("launcher fresh state has no budgets")
	}
	lock, err := state.Acquire(dir)
	if !errors.Is(err, state.ErrLocked) {
		if lock != nil {
			lock.Close()
		}
		t.Fatal("client exit stopped daemon")
	}
	if err := testkit.ValidateDestructiveTarget(root, dir); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range children {
		cmd.Process.Signal(syscall.SIGTERM)
		if err := cmd.Wait(); err != nil {
			t.Fatalf("daemon exit: %v %s", err, outputs[cmd].String())
		}
	}
	children = nil
	cfg, _ := config.Load(dir)
	s := config.DefaultRuntimeSettings()
	s.PeerListen = "127.0.0.1:0"
	if err := config.SaveRuntimeSettings(dir, s); err != nil {
		t.Fatal(err)
	}
	_, err = launcher.EnsureDaemon(context.Background(), launcher.LaunchOptions{StateDir: dir, DaemonStarter: starter})
	if err != nil {
		t.Fatal(err)
	}
	cfgAfter, _ := config.Load(dir)
	if cfgAfter.DeviceID != cfg.DeviceID {
		t.Fatal("restart changed identity")
	}
	querySettings(t, c)
	peerAddr, err := os.ReadFile(filepath.Join(dir, "peer.addr"))
	if err != nil {
		t.Fatal("configured peer listener not published")
	}
	conn, err := net.DialTimeout("tcp", strings.TrimSpace(string(peerAddr)), time.Second)
	if err != nil {
		t.Fatal("configured peer listener unreachable")
	}
	conn.Close()
	if err := app.StopAgent(dir, 3*time.Second); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range children {
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	children = nil
	// A stale PID file alone cannot signal a process after ownership was released.
	os.WriteFile(filepath.Join(dir, ".agent.pid"), []byte(fmt.Sprint(os.Getpid())), 0600)
	if err := app.StopAgent(dir, time.Second); err == nil {
		t.Fatal("stale stopped PID accepted")
	}
}
func TestTerminalT02MissingSystemdAndExplicitErrors(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", testkit.NewDisposable(t))
	t.Setenv("USER", "orbit-disposable")
	f := fresh(t)
	c := terminalClient(t, f)
	q := tc.Query{Version: tc.Version, Kind: "service"}
	r, err := c.Query(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	m := tc.Mutation{Version: tc.Version, Kind: "service", OperationID: strings.Repeat("7", 64), Service: &tc.ServiceIntent{Action: "enable", Mode: "login", Review: *r.Review}}
	result, err := c.Mutate(context.Background(), m)
	if err != nil || result.Error == nil || result.Error.Code != "SYSTEMD_UNAVAILABLE" {
		t.Fatalf("missing systemd result: %v", err)
	}
	if result.Operation.State != "failed" {
		t.Fatal("missing systemd reported completed")
	}
	replay, err := c.Mutate(context.Background(), m)
	if err != nil || replay.Operation.ID != m.OperationID || replay.Error.Code != "SYSTEMD_UNAVAILABLE" {
		t.Fatal("service failure replay changed")
	}
}

var _ tc.Client = (*controlclient.Client)(nil)

func TestTerminalT02ServiceProcessActionsAndSelection(t *testing.T) {
	root := testkit.NewDisposable(t)
	binary := filepath.Join(root, "filesync")
	build := exec.Command("go", "build", "-o", binary, "./cmd/filesync")
	build.Dir = filepath.Join("..", "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("fixture build: %v %s", err, output)
	}
	home := filepath.Join(root, "home")
	os.Mkdir(home, 0700)
	t.Setenv("HOME", home)
	t.Setenv("USER", "orbit-disposable")
	dir := filepath.Join(root, "state")
	if _, err := app.Initialize(context.Background(), dir, app.SystemDependencies()); err != nil {
		t.Fatal(err)
	}
	mockDir := filepath.Join(root, "mock")
	os.Mkdir(mockDir, 0700)
	t.Setenv("ORBIT_T02_BIN", binary)
	t.Setenv("ORBIT_T02_STATE", dir)
	t.Setenv("ORBIT_T02_ENABLED", filepath.Join(root, "enabled"))
	t.Setenv("ORBIT_T02_DISPATCHES", filepath.Join(root, "dispatches"))
	t.Setenv("ORBIT_T02_MAINPID", filepath.Join(root, "mainpid"))
	script := `#!/bin/sh
case "$2" in
 is-system-running) echo running ;;
 is-enabled) if [ -f "$ORBIT_T02_ENABLED" ]; then echo enabled; else echo disabled; exit 1; fi ;;
 daemon-reload) if [ "$ORBIT_T02_REJECT" = reload ]; then echo injected-reload-error >&2; exit 1; fi ;;
 enable) if [ "$ORBIT_T02_REJECT" = enable ]; then echo injected-enable-error >&2; exit 1; fi; echo enabled > "$ORBIT_T02_ENABLED" ;;
 disable) /bin/rm -f "$ORBIT_T02_ENABLED" ;;
 start|restart)
  echo "$2" >> "$ORBIT_T02_DISPATCHES"
  if [ "$2" = restart ]; then "$ORBIT_T02_BIN" stop --state="$ORBIT_T02_STATE" --timeout=3s >/dev/null 2>&1; fi
  "$ORBIT_T02_BIN" serve --state="$ORBIT_T02_STATE" --control-listen=127.0.0.1:0 --no-watch >/dev/null 2>&1 &
  echo "$!" > "$ORBIT_T02_MAINPID"
  i=0; while [ "$i" -lt 150 ]; do if [ -s "$ORBIT_T02_STATE/control.addr" ]; then exit 0; fi; i=$((i+1)); /bin/sleep .02; done; exit 1 ;;
 stop) "$ORBIT_T02_BIN" stop --state="$ORBIT_T02_STATE" --timeout=3s >/dev/null 2>&1 ;;
 show) if [ -f "$ORBIT_T02_MAINPID" ] && kill -0 "$(cat "$ORBIT_T02_MAINPID")" 2>/dev/null; then cat "$ORBIT_T02_MAINPID"; else echo 0; fi ;;
 *) exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(mockDir, "systemctl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))
	if err := control.InstallUserUnit(dir, binary); err != nil {
		t.Fatal(err)
	}
	defer app.StopAgent(dir, 3*time.Second)
	c := &controlclient.Client{StateDir: dir}
	action := func(name, id string) (tc.Result, error) {
		r, err := c.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "service"})
		if err != nil {
			return r, err
		}
		return c.Mutate(context.Background(), tc.Mutation{Version: tc.Version, Kind: "service", OperationID: strings.Repeat(id, 64), Service: &tc.ServiceIntent{Action: name, Mode: "login", Review: *r.Review}})
	}
	startReview, err := c.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "service"})
	if err != nil {
		t.Fatal(err)
	}
	startMutation := tc.Mutation{Version: tc.Version, Kind: "service", OperationID: strings.Repeat("8", 64), Service: &tc.ServiceIntent{Action: "start", Mode: "login", Review: *startReview.Review}}
	startsDone := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() { _, err := c.Mutate(context.Background(), startMutation); startsDone <- err }()
	}
	var unexpected error
	for i := 0; i < 8; i++ {
		err := <-startsDone
		if err != nil && !errors.Is(err, os.ErrNotExist) && !strings.Contains(err.Error(), "connect: connection refused") {
			unexpected = err
		}
	}
	if unexpected != nil {
		t.Fatal(unexpected)
	}
	// A competing stopped adapter or in-flight startup can hold the lock before
	// credentials/listener exist. That connection failure is returned safely;
	// replay the same ID after callers settle, never a fresh external operation.
	if _, err := c.Mutate(context.Background(), startMutation); err != nil {
		t.Fatal(err)
	}
	started, err := c.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "operation", ID: startMutation.OperationID})
	if err != nil || started.Operation.State != "completed" {
		t.Fatalf("stopped concurrent start: %v", err)
	}
	dispatches, err := os.ReadFile(filepath.Join(root, "dispatches"))
	if err != nil || strings.Count(string(dispatches), "start\n") != 1 {
		t.Fatal("external service start dispatched more than once")
	}
	service, err := c.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "service"})
	if err != nil || !service.Service.Running || service.Service.Enabled {
		t.Fatal("running confused with enabled")
	}
	enabled, err := action("enable", "9")
	if err != nil || enabled.Error != nil {
		t.Fatalf("enable: %v", err)
	}
	service, err = c.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "service"})
	if err != nil || !service.Service.Enabled || service.Service.UnattendedVerified {
		t.Fatal("enablement/unattended conflated")
	}
	disabled, err := action("disable", "a")
	if err != nil || disabled.Error != nil {
		t.Fatalf("disable: %v", err)
	}
	// Restart retains the selected identity and records the new daemon instance.
	beforeInstance, _ := os.ReadFile(filepath.Join(dir, ".agent.instance"))
	cfgBefore, _ := config.Load(dir)
	restarted, restartErr := action("restart", "e")
	if restartErr == nil && restarted.Operation.State != "completed" {
		t.Fatal("restart returned false completion")
	}
	restartDeadline := time.Now().Add(6 * time.Second)
	ready := false
	for time.Now().Before(restartDeadline) {
		instance, _ := os.ReadFile(filepath.Join(dir, ".agent.instance"))
		if len(instance) > 0 && string(instance) != string(beforeInstance) {
			var response any
			if c.Call(context.Background(), http.MethodGet, "/api/v1/settings", nil, &response) == nil {
				ready = true
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("restarted daemon never became ready")
	}
	cfgAfter, _ := config.Load(dir)
	if cfgAfter.DeviceID != cfgBefore.DeviceID {
		t.Fatal("service restart changed identity")
	}
	restartRecord, err := c.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "operation", ID: strings.Repeat("e", 64)})
	if err != nil || restartRecord.Operation.State != "completed" {
		t.Fatalf("lost restart response: %v", err)
	}
	// An explicit stop can lose its HTTP response while the owning process exits;
	// the same operation is recovered by observing stopped state, not reissuing stop.
	stopped, stopErr := action("stop", "b")
	if stopErr == nil && stopped.Operation.State != "completed" {
		t.Fatal("stop returned false completion")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		lock, err := state.Acquire(dir)
		if err == nil {
			lock.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	inspected, err := c.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "operation", ID: strings.Repeat("b", 64)})
	if err != nil || inspected.Operation.State != "completed" {
		t.Fatalf("lost stop response inspection: %v", err)
	}
	t.Setenv("ORBIT_T02_REJECT", "reload")
	failed, err := action("enable", "c")
	if err != nil || failed.Error == nil || failed.Operation.State != "failed" {
		t.Fatalf("daemon-reload error hidden: %v", err)
	}
	t.Setenv("ORBIT_T02_REJECT", "enable")
	failed, err = action("enable", "d")
	if err != nil || failed.Error == nil {
		t.Fatalf("explicit enable failure hidden: %v", err)
	}
	t.Setenv("ORBIT_T02_REJECT", "")
	// A daemon launched outside the unit owns the state: start is refused
	// rather than reported, and no external start is dispatched.
	manual := exec.Command(binary, "serve", "--state="+dir, "--control-listen=127.0.0.1:0", "--no-watch")
	if err := manual.Start(); err != nil {
		t.Fatal(err)
	}
	manualDeadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(manualDeadline) {
		if pid, _ := os.ReadFile(filepath.Join(dir, ".agent.pid")); strings.TrimSpace(string(pid)) == strconv.Itoa(manual.Process.Pid) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	dispatchesBefore, _ := os.ReadFile(filepath.Join(root, "dispatches"))
	var manualErr *control.ControlError
	if _, err := control.StartService(context.Background(), dir, nil); !errors.As(err, &manualErr) || manualErr.Code != "MANUAL_DAEMON_RUNNING" {
		t.Fatalf("start reported a manually launched daemon as the service: %v", err)
	}
	if _, err := control.RestartService(context.Background(), dir, nil); !errors.As(err, &manualErr) || manualErr.Code != "MANUAL_DAEMON_RUNNING" {
		t.Fatalf("restart reported a manually launched daemon as the service: %v", err)
	}
	if dispatchesAfter, _ := os.ReadFile(filepath.Join(root, "dispatches")); string(dispatchesAfter) != string(dispatchesBefore) {
		t.Fatal("start dispatched while a manual daemon owned the state")
	}
	if err := app.StopAgent(dir, 3*time.Second); err != nil {
		t.Fatal(err)
	}
	_ = manual.Wait()
	// A different selected state cannot replace or operate this user unit.
	other := filepath.Join(root, "other")
	os.Mkdir(other, 0700)
	before, _ := os.ReadFile(filepath.Join(home, ".config", "systemd", "user", "filesync.service"))
	if _, err := control.StartService(context.Background(), other, nil); err == nil {
		t.Fatal("unrelated unit started")
	}
	if err := control.InstallUserUnit(other, binary); err == nil {
		t.Fatal("existing unit overwritten")
	}
	after, _ := os.ReadFile(filepath.Join(home, ".config", "systemd", "user", "filesync.service"))
	if string(before) != string(after) {
		t.Fatal("personal service bytes changed")
	}
}

func TestTerminalT02CanceledMutationWaitingKeepsAcceptedWork(t *testing.T) {
	f := fresh(t)
	f.server.Close()
	admitted := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	f.ctrl = control.New(f.db, f.ws, control.Options{LocalDevice: f.device, FaultHook: func(name string) error {
		if name == "terminal.operation.accepted" {
			close(admitted)
			<-release
		}
		return nil
	}})
	server, err := control.NewServer(f.ctrl, f.state)
	if err != nil {
		t.Fatal(err)
	}
	f.server = httptest.NewServer(server.Handler())
	c := terminalClient(t, f)
	m := settingsMutation(querySettings(t, c), "f")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := c.Mutate(ctx, m); done <- err }()
	select {
	case <-admitted:
	case <-time.After(2 * time.Second):
		t.Fatal("operation not admitted")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting cancellation: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client did not cancel waiting")
	}
	once.Do(func() { close(release) })
	r, err := c.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "operation", ID: m.OperationID})
	if err != nil || r.Operation.State != "completed" {
		t.Fatalf("accepted work canceled: %v", err)
	}
	if querySettings(t, c).Settings.DataBudget != m.Settings.Settings.DataBudget {
		t.Fatal("accepted settings disappeared after canceled wait")
	}
}
func TestTerminalT02ConcurrentMutationReplay(t *testing.T) {
	f := fresh(t)
	c := terminalClient(t, f)
	m := settingsMutation(querySettings(t, c), "1")
	done := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() {
			r, err := c.Mutate(context.Background(), m)
			if err == nil && (r.Operation.State != "completed" || len(r.Operation.CommittedEffects) != 1) {
				err = errors.New("replay changed committed effects")
			}
			done <- err
		}()
	}
	for i := 0; i < 8; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

func TestTerminalT02ControlServeShutdown(t *testing.T) {
	f := fresh(t)
	for i := 0; i < 30; i++ {
		server, err := control.NewServer(f.ctrl, f.state)
		if err != nil {
			t.Fatal(err)
		}
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		served := make(chan error, 1)
		stopped := make(chan error, 1)
		go func() { served <- server.Serve(listener) }()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			stopped <- server.Shutdown(ctx)
		}()
		if err := <-stopped; err != nil {
			t.Fatal(err)
		}
		listener.Close()
		<-served
	}
}
