package terminal_test

// E01 daemon lifecycle and service defaults (F01, F04, F12 startup, F14, EG3).
// Every service manager here is a stand-in on PATH inside a marked disposable
// root, except TestOnboardingE01F01RealUserManagerTransientUnit, which is
// opt-in and runs a uniquely named transient unit over disposable state.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/app"
	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
	"github.com/calebhabesh/orbit/internal/testkit"
)

// holdPort8080 holds 127.0.0.1:8080 for the test when it is free; when another
// process already holds it (as on the trial PC) the condition exists anyway.
func holdPort8080(t *testing.T) {
	t.Helper()
	if l, err := net.Listen("tcp", "127.0.0.1:8080"); err == nil {
		t.Cleanup(func() { l.Close() })
		t.Log("test holds 127.0.0.1:8080")
	} else {
		t.Logf("127.0.0.1:8080 already held on this host: %v", err)
	}
}

func packagedExecStart(t *testing.T) []string {
	t.Helper()
	unit, err := os.ReadFile("../../packaging/systemd/orbit.service")
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(string(unit), "\n") {
		if v, ok := strings.CutPrefix(line, "ExecStart="); ok {
			return strings.Fields(v)
		}
	}
	t.Fatal("packaged unit has no ExecStart")
	return nil
}

// F01: the packaged unit's arguments start a daemon while 127.0.0.1:8080 is
// taken, and status reaches it through control.addr.
func TestOnboardingE01F01PackagedUnitStartsWithPort8080Taken(t *testing.T) {
	base := testkit.NewDisposable(t)
	_ = os.Chmod(base, 0700)
	execStart := packagedExecStart(t)
	for _, a := range execStart {
		if strings.HasPrefix(a, "--control-listen=") && a != "--control-listen=127.0.0.1:0" {
			t.Fatalf("packaged unit hardcodes %s", a)
		}
	}
	holdPort8080(t)
	binary := buildOrbitBinary(t, base)
	state := filepath.Join(base, "state")
	if _, err := app.Initialize(context.Background(), state, app.SystemDependencies()); err != nil {
		t.Fatal(err)
	}
	args := []string{}
	for _, a := range execStart[1:] {
		args = append(args, strings.ReplaceAll(a, "%h/.local/state/orbit", state))
	}
	e00StopOnCleanup(t, base, state)
	cmd := exec.Command(binary, args...)
	cmd.Env = []string{"HOME=" + base, "PATH=" + os.Getenv("PATH")}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	addrPath := filepath.Join(state, "control.addr")
	var addr []byte
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline) && len(addr) == 0; time.Sleep(100 * time.Millisecond) {
		addr, _ = os.ReadFile(addrPath)
	}
	if len(addr) == 0 || strings.HasSuffix(strings.TrimSpace(string(addr)), ":8080") {
		t.Fatalf("daemon published control address %q", addr)
	}
	// The daemon holds the state lock, so status must use the live endpoint.
	out, err := e00Run(t, cmd.Env, 15*time.Second, binary, "status", "--state", state, "--json")
	if err != nil {
		t.Fatalf("orbit status could not reach the packaged-argument daemon: %v\n%s", err, out)
	}
	t.Logf("control.addr %s; status connected", strings.TrimSpace(string(addr)))
}

// F01 upgrade: a unit written by an earlier `orbit service enable` (fixed
// 8080) is rewritten on the next service action; an edited unit is kept.
func TestOnboardingE01F01LegacyUserUnitMigrates(t *testing.T) {
	base := testkit.NewDisposable(t)
	home := filepath.Join(base, "home")
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USER", "e01-disposable")
	e01Stubs(t, base, stubHost{target: "graphical.target"})
	// Start fails fast; only the migration before it is under test.
	if err := os.WriteFile(filepath.Join(base, "fail-start"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(base, "state")
	if err := control.InstallUserUnit(state, "/opt/orbit/bin/orbit"); err != nil {
		t.Fatal(err)
	}
	unitPath := filepath.Join(unitDir, "orbit.service")
	current, _ := os.ReadFile(unitPath)
	if !strings.Contains(string(current), "--control-listen=127.0.0.1:0 ") {
		t.Fatalf("enable installs a fixed control port:\n%s", current)
	}
	legacy := strings.Replace(string(current), "127.0.0.1:0 ", "127.0.0.1:8080 ", 1)
	for _, tc := range []struct {
		name, content string
		migrated      bool
	}{{"generated", legacy, true}, {"edited", legacy + "# owner edit\n", false}} {
		if err := os.WriteFile(unitPath, []byte(tc.content), 0644); err != nil {
			t.Fatal(err)
		}
		_, _ = control.StartService(context.Background(), state, nil, nil)
		after, _ := os.ReadFile(unitPath)
		if tc.migrated && string(after) != string(current) {
			t.Errorf("%s legacy unit not migrated:\n%s", tc.name, after)
		}
		if !tc.migrated && string(after) != tc.content {
			t.Errorf("%s unit was rewritten", tc.name)
		}
	}
}

type stubHost struct {
	target        string // systemctl get-default
	graphical     bool   // user graphical-session.target active
	linger        bool
	noUserManager bool
}

// e01Stubs installs systemctl/loginctl/sudo stand-ins on PATH. The service
// stand-in really runs `orbit serve` for start/restart (so ownership is
// observable), stops only the process it started, and fails start while
// $base/fail-start exists. loginctl and sudo record every call; any
// enable-linger or sudo use is a test failure.
func e01Stubs(t *testing.T, base string, h stubHost) []string {
	t.Helper()
	bin := filepath.Join(base, "stub-bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	calls := filepath.Join(base, "calls.log")
	reachable := "echo running"
	if h.noUserManager {
		reachable = `echo "Failed to connect to bus: No medium found" >&2; exit 1`
	}
	graphical := "inactive"
	if h.graphical {
		graphical = "active"
	}
	systemctl := `#!/bin/sh
echo "systemctl $*" >> "` + calls + `"
B="` + base + `"
alive() { [ -f "$B/mainpid" ] && kill -0 "$(cat "$B/mainpid")" 2>/dev/null; }
stopunit() {
  if alive; then p=$(cat "$B/mainpid"); kill -TERM "$p"; i=0; while kill -0 "$p" 2>/dev/null && [ $i -lt 300 ]; do i=$((i+1)); sleep .05; done; fi
  rm -f "$B/mainpid"
}
startunit() {
  if [ -f "$B/fail-start" ]; then echo "Job for orbit.service failed" >&2; exit 1; fi
  if alive; then exit 0; fi
  "$ORBIT_E01_BIN" serve --state="$ORBIT_E01_STATE" --control-listen=127.0.0.1:0 --no-watch >> "$B/journal" 2>&1 &
  echo "$!" > "$B/mainpid"
  i=0; while [ $i -lt 300 ]; do if [ -s "$ORBIT_E01_STATE/control.addr" ] && [ "$(cat "$ORBIT_E01_STATE/.agent.pid" 2>/dev/null)" = "$(cat "$B/mainpid")" ]; then exit 0; fi; i=$((i+1)); sleep .05; done; exit 1
}
[ "$1" = "--user" ] && shift
case "$1" in
  get-default) echo "` + h.target + `" ;;
  is-system-running) ` + reachable + ` ;;
  is-enabled) echo disabled; exit 1 ;;
  is-active)
    case "$2" in
      graphical-session.target) echo ` + graphical + ` ;;
      *) if [ -f "$B/fail-start" ]; then echo failed; exit 3; elif alive; then echo active; else echo inactive; exit 3; fi ;;
    esac ;;
  show) case "$*" in *MainPID*) if alive; then cat "$B/mainpid"; else echo 0; fi ;; *) echo "" ;; esac ;;
  start) startunit ;;
  restart) stopunit; startunit ;;
  stop) stopunit ;;
  *) exit 0 ;;
esac
`
	linger := "no"
	if h.linger {
		linger = "yes"
	}
	loginctl := `#!/bin/sh
echo "loginctl $*" >> "` + calls + `"
case "$*" in *enable-linger*) echo "FORBIDDEN enable-linger" >> "` + calls + `"; exit 1 ;; esac
echo Linger=` + linger + `
`
	sudo := "#!/bin/sh\necho \"FORBIDDEN sudo $*\" >> \"" + calls + "\"\nexit 1\n"
	for name, body := range map[string]string{"systemctl": systemctl, "loginctl": loginctl, "sudo": sudo} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Cleanup(func() {
		b, _ := os.ReadFile(calls)
		if strings.Contains(string(b), "FORBIDDEN") {
			t.Errorf("privileged command attempted:\n%s", b)
		}
	})
	return os.Environ()
}

// EG3 fixtures: desktop (local and over SSH), headless with lingering off/on,
// and no user manager. Lingering is checked, never enabled.
func TestOnboardingE01EG3HostClassFixtures(t *testing.T) {
	for _, c := range []struct {
		name           string
		host           stubHost
		class, suggest string
		lingerGuidance bool
	}{
		{"desktop local", stubHost{target: "graphical.target", graphical: true}, "desktop", "login", false},
		{"desktop over ssh", stubHost{target: "graphical.target"}, "desktop", "login", false},
		{"desktop booted multi-user, session active", stubHost{target: "multi-user.target", graphical: true}, "desktop", "login", false},
		{"headless linger off", stubHost{target: "multi-user.target"}, "headless", "login", true},
		{"headless linger on", stubHost{target: "multi-user.target", linger: true}, "headless", "unattended", false},
		{"no user manager", stubHost{target: "graphical.target", noUserManager: true}, "unknown", "manual", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			base := testkit.NewDisposable(t)
			t.Setenv("HOME", base)
			t.Setenv("USER", "e01-disposable")
			e01Stubs(t, base, c.host)
			h := control.ProbeHostStartup(context.Background())
			t.Logf("%+v", h)
			if h.Class != c.class || h.Suggested != c.suggest {
				t.Fatalf("class %q suggested %q, want %q %q", h.Class, h.Suggested, c.class, c.suggest)
			}
			if h.LingerCommand != "sudo loginctl enable-linger e01-disposable" {
				t.Fatalf("linger command %q", h.LingerCommand)
			}
			if c.lingerGuidance != strings.Contains(h.Note, h.LingerCommand) {
				t.Fatalf("linger guidance %v in note %q", c.lingerGuidance, h.Note)
			}
		})
	}
}

// EG3 through control: a fresh device's settings review proposes the host
// default; saved settings keep the owner's choice; re-checking after the owner
// enables lingering proposes unattended.
func TestOnboardingE01EG3FreshSettingsUseHostDefault(t *testing.T) {
	f := fresh(t)
	t.Setenv("HOME", f.root)
	t.Setenv("USER", "e01-disposable")
	e01Stubs(t, f.root, stubHost{target: "multi-user.target"})
	c := terminalClient(t, f)
	// A unit serving another state keeps this state manual and says why.
	if err := control.InstallUserUnit(filepath.Join(f.root, "other-state"), "/opt/orbit/bin/orbit"); err != nil {
		t.Fatal(err)
	}
	if r := querySettings(t, c); r.Settings.Startup != "manual" || !strings.Contains(r.Host.Note, "different state") {
		t.Fatalf("foreign unit: startup %q note %q", r.Settings.Startup, r.Host.Note)
	}
	if err := os.Remove(filepath.Join(f.root, ".config", "systemd", "user", "orbit.service")); err != nil {
		t.Fatal(err)
	}
	// No unit and a non-default state: still manual (enable would install one).
	if r := querySettings(t, c); r.Settings.Startup != "manual" {
		t.Fatalf("no unit, non-default state: startup %q", r.Settings.Startup)
	}
	if err := control.InstallUserUnit(f.state, "/opt/orbit/bin/orbit"); err != nil {
		t.Fatal(err)
	}
	r := querySettings(t, c)
	if r.Host == nil || r.Host.Class != "headless" || r.Settings.Startup != "login" || !strings.Contains(r.Host.Note, "sudo loginctl enable-linger") {
		t.Fatalf("headless linger-off review: settings %+v host %+v", r.Settings, r.Host)
	}
	// The owner runs the shown command; a re-check now proposes unattended.
	e01Stubs(t, f.root, stubHost{target: "multi-user.target", linger: true})
	if r = querySettings(t, c); r.Settings.Startup != "unattended" {
		t.Fatalf("after lingering, startup %q", r.Settings.Startup)
	}
	s := *r.Settings
	s.Startup = "manual"
	m := tc.Mutation{Version: tc.Version, Kind: "settings", OperationID: strings.Repeat("e", 64), Settings: &tc.SettingsIntent{Review: *r.Review, Settings: s}}
	if res, err := c.Mutate(context.Background(), m); err != nil || res.Error != nil {
		t.Fatalf("save settings: %v %+v", err, res.Error)
	}
	if r = querySettings(t, c); r.Settings.Startup != "manual" {
		t.Fatalf("saved choice replaced by host default: %q", r.Settings.Startup)
	}
}

func e01Service(t *testing.T, env []string, binary, state string, args ...string) map[string]any {
	t.Helper()
	out, err := e00Run(t, env, 90*time.Second, binary, append(args, "--state", state, "--json")...)
	if err != nil {
		t.Fatalf("orbit %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("orbit %s output: %v\n%s", strings.Join(args, " "), err, out)
	}
	return v
}

func e01Owner(t *testing.T, env []string, binary, state string) (owner, unit string, running bool) {
	t.Helper()
	st := e01Service(t, env, binary, state, "service", "status")
	owner, _ = st["owner"].(string)
	unit, _ = st["unit_state"].(string)
	running, _ = st["currently_running"].(bool)
	return
}

func e01OperationCompleted(t *testing.T, r map[string]any, action string) {
	t.Helper()
	op, _ := r["operation"].(map[string]any)
	if op == nil || op["state"] != "completed" || r["error"] != nil {
		t.Fatalf("orbit service %s: %v", action, r)
	}
}

// F04 + F14: a terminal-started daemon beside a failing unit is reported as
// such, logs to a file, and `orbit service start/stop/restart` hand over or
// succeed with the owner named at each step, keeping identity and settings.
func TestOnboardingE01F04F14OwnerAndHandover(t *testing.T) {
	base := testkit.NewDisposable(t)
	_ = os.Chmod(base, 0700)
	binary := buildOrbitBinary(t, base)
	home := filepath.Join(base, "home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(base, "state")
	t.Setenv("HOME", home)
	t.Setenv("USER", "e01-disposable")
	t.Setenv("ORBIT_E01_BIN", binary)
	t.Setenv("ORBIT_E01_STATE", state)
	env := e01Stubs(t, base, stubHost{target: "graphical.target", graphical: true})
	if err := control.InstallUserUnit(state, binary); err != nil {
		t.Fatal(err)
	}
	e00StopOnCleanup(t, base, state)
	t.Cleanup(func() {
		if b, err := os.ReadFile(filepath.Join(base, "mainpid")); err == nil {
			_ = exec.Command("kill", "-TERM", strings.TrimSpace(string(b))).Run()
		}
	})
	failStart := filepath.Join(base, "fail-start")
	terminalDaemon := func() {
		t.Helper()
		if err := os.WriteFile(failStart, nil, 0600); err != nil {
			t.Fatal(err)
		}
		if out, err := e00Run(t, env, 60*time.Second, binary, "launch", "--state", state, "--no-browser", "--json"); err != nil {
			t.Fatalf("launch: %v\n%s", err, out)
		}
	}

	// 1. The unit fails, so the launcher falls back to a detached daemon.
	terminalDaemon()
	owner, unit, running := e01Owner(t, env, binary, state)
	t.Logf("after launch with failing unit: owner=%q unit=%q running=%v", owner, unit, running)
	if !running || owner != control.OwnerTerminal || unit != "failed" {
		t.Fatalf("status: owner=%q unit=%q running=%v", owner, unit, running)
	}
	human, err := e00Run(t, env, 30*time.Second, binary, "status", "--state", state)
	if err != nil || !strings.Contains(human, "running (terminal)") || !strings.Contains(human, "service failed") {
		t.Fatalf("orbit status header does not name the owner/failing unit: %v\n%s", err, human)
	}
	pid, _ := os.ReadFile(filepath.Join(state, ".agent.pid"))
	for _, fd := range []string{"1", "2"} {
		target, err := os.Readlink(filepath.Join("/proc", strings.TrimSpace(string(pid)), "fd", fd))
		if err != nil || target != filepath.Join(state, "daemon.log") {
			t.Fatalf("detached daemon fd %s -> %q (%v)", fd, target, err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(base, "calls.log")); !strings.Contains(string(b), "systemctl --user stop orbit.service") {
		t.Fatal("failed unit was not stopped before the detached fallback (it would crash-loop)")
	}

	// Committed work before the handover: a reviewed settings change.
	c := &controlclient.Client{StateDir: state}
	r, err := c.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "settings"})
	if err != nil {
		t.Fatal(err)
	}
	s := *r.Settings
	s.DataBudget += 4096
	if res, err := c.Mutate(context.Background(), tc.Mutation{Version: tc.Version, Kind: "settings", OperationID: strings.Repeat("1", 64), Settings: &tc.SettingsIntent{Review: *r.Review, Settings: s}}); err != nil || res.Error != nil {
		t.Fatalf("settings: %v %+v", err, res.Error)
	}
	cfgBefore, _ := config.Load(state)

	// 2. `orbit service start` hands the terminal daemon over to the unit.
	if err := os.Remove(failStart); err != nil {
		t.Fatal(err)
	}
	e01OperationCompleted(t, e01Service(t, env, binary, state, "service", "start"), "start (handover)")
	if owner, unit, running = e01Owner(t, env, binary, state); owner != control.OwnerService || unit != "active" || !running {
		t.Fatalf("after handover: owner=%q unit=%q running=%v", owner, unit, running)
	}
	if b, _ := os.ReadFile(filepath.Join(base, "journal")); len(b) == 0 {
		t.Fatal("service daemon output did not reach the journal stand-in")
	}
	cfgAfter, _ := config.Load(state)
	settings, _ := config.LoadRuntimeSettings(state)
	if cfgAfter.DeviceID != cfgBefore.DeviceID || settings.DataBudget != s.DataBudget {
		t.Fatal("handover changed identity or lost committed settings")
	}

	// 3. stop, start, restart each succeed with the owner named.
	e01OperationCompleted(t, e01Service(t, env, binary, state, "service", "stop"), "stop")
	if owner, _, running = e01Owner(t, env, binary, state); running || owner != "" {
		t.Fatalf("after stop: owner=%q running=%v", owner, running)
	}
	e01OperationCompleted(t, e01Service(t, env, binary, state, "service", "start"), "start")
	if owner, _, _ = e01Owner(t, env, binary, state); owner != control.OwnerService {
		t.Fatalf("after start: owner=%q", owner)
	}
	before, _ := os.ReadFile(filepath.Join(state, ".agent.instance"))
	e01OperationCompleted(t, e01Service(t, env, binary, state, "service", "restart"), "restart")
	after, _ := os.ReadFile(filepath.Join(state, ".agent.instance"))
	if owner, _, running = e01Owner(t, env, binary, state); owner != control.OwnerService || !running || string(before) == string(after) {
		t.Fatalf("after restart: owner=%q running=%v new instance=%v", owner, running, string(before) != string(after))
	}
	e01OperationCompleted(t, e01Service(t, env, binary, state, "service", "stop"), "stop")

	// 4. `orbit service stop` also stops a terminal-started daemon (F14).
	terminalDaemon()
	if owner, _, _ = e01Owner(t, env, binary, state); owner != control.OwnerTerminal {
		t.Fatalf("second launch owner %q", owner)
	}
	e01OperationCompleted(t, e01Service(t, env, binary, state, "service", "stop"), "stop (terminal daemon)")
	if _, _, running = e01Owner(t, env, binary, state); running {
		t.Fatal("orbit service stop left the terminal daemon running")
	}
	if cfg, _ := config.Load(state); cfg.DeviceID != cfgBefore.DeviceID {
		t.Fatal("identity changed")
	}
	b, _ := os.ReadFile(filepath.Join(base, "calls.log"))
	t.Logf("service manager calls:\n%s", strings.TrimSpace(string(b)))
}

// F01 against a real user manager: a uniquely named transient unit runs the
// packaged arguments over disposable state while 8080 is taken; status
// connects and the journal holds daemon output. Opt in with
// ORBIT_E01_REAL_USER_MANAGER=1; it never touches orbit.service.
func TestOnboardingE01F01RealUserManagerTransientUnit(t *testing.T) {
	if os.Getenv("ORBIT_E01_REAL_USER_MANAGER") != "1" {
		t.Skip("opt in with ORBIT_E01_REAL_USER_MANAGER=1 (runs a transient user unit over disposable state)")
	}
	base := testkit.NewDisposable(t)
	_ = os.Chmod(base, 0700)
	holdPort8080(t)
	binary := buildOrbitBinary(t, base)
	state := filepath.Join(base, "state")
	if _, err := app.Initialize(context.Background(), state, app.SystemDependencies()); err != nil {
		t.Fatal(err)
	}
	var id [4]byte
	_, _ = rand.Read(id[:])
	unit := "orbit-e01-" + hex.EncodeToString(id[:])
	args := []string{"--user", "--unit=" + unit, "--collect", "--property=SyslogIdentifier=" + unit, binary}
	for _, a := range packagedExecStart(t)[1:] {
		args = append(args, strings.ReplaceAll(a, "%h/.local/state/orbit", state))
	}
	if out, err := exec.Command("systemd-run", args...).CombinedOutput(); err != nil {
		t.Fatalf("systemd-run: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("systemctl", "--user", "stop", unit+".service").Run()
		if err := testkit.ValidateDestructiveTarget(base, state); err == nil {
			_ = app.StopAgent(state, 10*time.Second)
		}
	})
	var out string
	err := errors.New("not attempted")
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if out, err = e00Run(t, os.Environ(), 15*time.Second, binary, "status", "--state", state, "--json"); err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("status through control.addr: %v\n%s", err, out)
	}
	active, _ := exec.Command("systemctl", "--user", "is-active", unit+".service").Output()
	addr, _ := os.ReadFile(filepath.Join(state, "control.addr"))
	var journal []byte
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline) && len(strings.TrimSpace(string(journal))) == 0; time.Sleep(200 * time.Millisecond) {
		journal, _ = exec.Command("journalctl", "--user", "-u", unit+".service", "--no-pager", "-o", "cat").Output()
	}
	t.Logf("unit %s is-active=%s control.addr=%s\njournal:\n%s", unit, strings.TrimSpace(string(active)), strings.TrimSpace(string(addr)), strings.TrimSpace(string(journal)))
	if strings.TrimSpace(string(active)) != "active" || len(strings.TrimSpace(string(journal))) == 0 {
		t.Fatal("transient unit not active or journal empty")
	}
}
