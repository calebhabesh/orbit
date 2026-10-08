package terminal_test

// E00 onboarding baseline reproductions for the 2026-10-08 owner trial
// findings that need real processes. Each test asserts approved behavior and
// deliberately fails on the current implementation; ordinary runs skip them.
// Opt in with ORBIT_ONBOARDING_BASELINE=1. All state, HOME and service-manager
// stand-ins live in marked disposable roots; no real user unit, systemd
// manager, personal folder or deployed service is touched.

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/app"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
	"github.com/calebhabesh/orbit/internal/testkit"
)

func onboardingBaseline(t *testing.T) {
	t.Helper()
	if os.Getenv("ORBIT_ONBOARDING_BASELINE") != "1" {
		t.Skip("deliberately failing E00 baseline; opt in with ORBIT_ONBOARDING_BASELINE=1")
	}
}

// e00Run runs the built CLI with an explicit environment and returns combined
// output. Outputs here never contain invitation capabilities.
func e00Run(t *testing.T, env []string, timeout time.Duration, binary string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

func e00StopOnCleanup(t *testing.T, base, dir string) {
	t.Cleanup(func() {
		if err := testkit.ValidateDestructiveTarget(base, dir); err != nil {
			t.Error(err)
			return
		}
		_ = app.StopAgent(dir, 10*time.Second)
	})
}

// F01: the packaged user unit must start while another process holds
// 127.0.0.1:8080 (the PC and Pi each had an unrelated Java app there).
func TestOnboardingE00F01PackagedUnitStartsWithPort8080Taken(t *testing.T) {
	onboardingBaseline(t)
	base := testkit.NewDisposable(t)
	_ = os.Chmod(base, 0700)
	unit, err := os.ReadFile("../../packaging/systemd/orbit.service")
	if err != nil {
		t.Fatal(err)
	}
	var execStart []string
	for line := range strings.SplitSeq(string(unit), "\n") {
		if v, ok := strings.CutPrefix(line, "ExecStart="); ok {
			execStart = strings.Fields(v)
		}
	}
	if len(execStart) < 2 {
		t.Fatal("packaged unit has no ExecStart")
	}
	t.Logf("packaged ExecStart: %s", strings.Join(execStart, " "))
	// Hold the port when it is free; when it is already held (as on the dev PC)
	// the condition exists without this test owning it.
	if l, e := net.Listen("tcp", "127.0.0.1:8080"); e == nil {
		defer l.Close()
		t.Log("test holds 127.0.0.1:8080")
	} else {
		t.Logf("127.0.0.1:8080 already held on this host: %v", e)
	}
	binary := buildOrbitBinary(t, base)
	state := filepath.Join(base, "state")
	if _, err = app.Initialize(context.Background(), state, app.SystemDependencies()); err != nil {
		t.Fatal(err)
	}
	args := []string{}
	for _, a := range execStart[1:] {
		args = append(args, strings.ReplaceAll(a, "%h/.local/state/orbit", state))
	}
	e00StopOnCleanup(t, base, state)
	out, err := e00Run(t, []string{"HOME=" + base, "PATH=" + os.Getenv("PATH")}, 4*time.Second, binary, args...)
	if err == nil || errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "killed") {
		t.Log("daemon stayed up with the packaged arguments")
	} else {
		t.Errorf("packaged unit arguments exit while 8080 is taken: %v\n%s", err, strings.TrimSpace(out))
	}
	for _, a := range execStart {
		if strings.HasPrefix(a, "--control-listen=") && a != "--control-listen=127.0.0.1:0" {
			t.Errorf("packaged unit hardcodes %s (approved: 127.0.0.1:0, clients read control.addr)", a)
		}
	}
}

// F03: the advice printed for exhausted work must run while the daemon runs.
func TestOnboardingE00F03ExhaustedWorkAdviceRunsWithDaemon(t *testing.T) {
	onboardingBaseline(t)
	base := testkit.NewDisposable(t)
	_ = os.Chmod(base, 0700)
	binary := buildOrbitBinary(t, base)
	state := filepath.Join(base, "state")
	env := []string{"HOME=" + base, "PATH=" + os.Getenv("PATH")}
	e00StopOnCleanup(t, base, state)
	if out, err := e00Run(t, env, 30*time.Second, binary, "launch", "--state", state, "--no-browser", "--json"); err != nil {
		t.Fatalf("launch: %v\n%s", err, out)
	}
	// The exact advice text from control attention (terminal_status.go).
	advice := "orbit engine work retry --task 1011680dc086cb3e4ab83c1b6177ad38"
	out, err := e00Run(t, env, 30*time.Second, binary, append(strings.Fields(advice)[1:], "--state", state)...)
	t.Logf("%s => %v: %s", advice, err, strings.TrimSpace(out))
	if strings.Contains(out, "already owned by another agent") {
		t.Errorf("attention advice cannot run while the daemon runs: %s", strings.TrimSpace(out))
	}
}

// e00ServiceStub puts systemctl/loginctl stand-ins first on PATH. They model a
// user manager whose orbit.service is enabled but failing (crash loop on the
// taken port, as on the trial PC) and record every invocation.
func e00ServiceStub(t *testing.T, base string) (env []string, calls string) {
	t.Helper()
	bin := filepath.Join(base, "stub-bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	calls = filepath.Join(base, "systemctl-calls.log")
	systemctl := `#!/bin/sh
echo "$*" >> "` + calls + `"
case "$*" in
  "--user is-system-running") echo running; exit 0 ;;
  "--user is-enabled orbit.service") echo enabled; exit 0 ;;
  "--user is-active orbit.service") echo failed; exit 3 ;;
  *"show -p MainPID"*) echo 0; exit 0 ;;
  *"show"*) echo "ActiveState=failed"; echo "SubState=failed"; echo "Result=exit-code"; exit 0 ;;
  *) exit 0 ;;
esac
`
	loginctl := "#!/bin/sh\necho Linger=no\n"
	for name, body := range map[string]string{"systemctl": systemctl, "loginctl": loginctl} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return []string{"HOME=" + base, "USER=e00-disposable", "PATH=" + bin + ":" + os.Getenv("PATH")}, calls
}

// F04 + F14: a terminal-spawned daemon with an enabled-but-failing unit.
// Approved: status names who runs the daemon and shows the failing unit;
// `orbit service start/stop` hand over or succeed instead of failing; the
// detached daemon's output is retained somewhere (journal or file).
func TestOnboardingE00F04F14TerminalDaemonAndFailingUnit(t *testing.T) {
	onboardingBaseline(t)
	base := testkit.NewDisposable(t)
	_ = os.Chmod(base, 0700)
	binary := buildOrbitBinary(t, base)
	state := filepath.Join(base, "state")
	env, calls := e00ServiceStub(t, base)
	unitDir := filepath.Join(base, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0700); err != nil {
		t.Fatal(err)
	}
	unit := "[Service]\nExecStart=" + binary + " serve --state=" + state + " --control-listen=127.0.0.1:8080\n"
	if err := os.WriteFile(filepath.Join(unitDir, "orbit.service"), []byte(unit), 0600); err != nil {
		t.Fatal(err)
	}
	e00StopOnCleanup(t, base, state)
	// The TUI and `orbit launch` share launcher.EnsureDaemon (detached serve).
	if out, err := e00Run(t, env, 30*time.Second, binary, "launch", "--state", state, "--no-browser", "--json"); err != nil {
		t.Fatalf("launch: %v\n%s", err, out)
	}

	status, err := e00Run(t, env, 30*time.Second, binary, "service", "status", "--state", state, "--json")
	if err != nil {
		t.Fatalf("service status: %v\n%s", err, status)
	}
	t.Logf("service status: %s", strings.TrimSpace(status))
	var st map[string]any
	if err = json.Unmarshal([]byte(status), &st); err != nil {
		t.Fatal(err)
	}
	if st["currently_running"] == true && st["enabled_on_login"] == true {
		if _, ok := st["owner"]; !ok {
			t.Error("F04: status reports running + login-enabled with no daemon owner (approved: service/terminal/manual)")
		}
		if !strings.Contains(status, "failed") {
			t.Error("F04: failing unit not shown as failing")
		}
	}

	pid, err := os.ReadFile(filepath.Join(state, ".agent.pid"))
	if err != nil {
		t.Fatal(err)
	}
	var logs []string
	for _, fd := range []string{"1", "2"} {
		target, e := os.Readlink(filepath.Join("/proc", strings.TrimSpace(string(pid)), "fd", fd))
		if e != nil {
			t.Fatal(e)
		}
		logs = append(logs, target)
	}
	t.Logf("F14: detached daemon stdout/stderr: %v", logs)
	if logs[0] == "/dev/null" && logs[1] == "/dev/null" {
		t.Error("F14: terminal-spawned daemon output goes to /dev/null (approved: start through orbit.service so output reaches the journal)")
	}

	for _, action := range []string{"stop", "start"} {
		out, err := e00Run(t, env, 60*time.Second, binary, "service", action, "--state", state, "--json")
		t.Logf("F14: orbit service %s => %v: %s", action, err, strings.TrimSpace(out))
		if err != nil || strings.Contains(out, "MANUAL_DAEMON_RUNNING") || strings.Contains(out, "IO_ERROR") {
			t.Errorf("F14: orbit service %s failed with a terminal-spawned daemon running", action)
		}
	}
	if b, e := os.ReadFile(calls); e == nil {
		t.Logf("systemctl stand-in calls:\n%s", strings.TrimSpace(string(b)))
	}
}

// F15: a review file in an ordinary 0750 home directory reports the file's
// directory and the fix, not a "state directory" error.
func TestOnboardingE00F15ReviewFileInGroupReadableHome(t *testing.T) {
	onboardingBaseline(t)
	base := testkit.NewDisposable(t)
	_ = os.Chmod(base, 0700)
	binary := buildOrbitBinary(t, base)
	home := filepath.Join(base, "home")
	if err := os.Mkdir(home, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(home, 0750); err != nil {
		t.Fatal(err)
	}
	state, root := filepath.Join(base, "state"), filepath.Join(base, "Orbit")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	env := []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
	e00StopOnCleanup(t, base, state)
	review := filepath.Join(home, "r.json")
	out, err := e00Run(t, env, 60*time.Second, binary, "setup", "--state", state, "--root", root, "--label", "PC", "--name", "Orbit", "--preview", "--review-file", review, "--json")
	t.Logf("setup --review-file ~/r.json (0750 home) => %v: %s", err, strings.TrimSpace(out))
	if err == nil {
		if fi, e := os.Stat(review); e == nil && fi.Mode().Perm() == 0600 {
			t.Log("review file written privately")
			return
		}
	}
	if strings.Contains(out, "state directory") {
		t.Error("review-file failure is described as a state directory problem")
	}
	if !strings.Contains(out, home) || !strings.Contains(out, "mkdir -m 700") {
		t.Error("review-file failure does not name the file's directory and the fix")
	}
}

// F08 (CLI half), F10, F11, F16 over a disposable local routed service
// (w05Service; production orbit-net admission limits) and two real daemons.
// Never run against the deployed VPS.
func TestOnboardingE00RelayJoinWaitNamesAndModes(t *testing.T) {
	onboardingBaseline(t)
	base := testkit.NewDisposable(t)
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	var serviceRequests atomic.Int64
	_, selection, origin, roots, _ := w05Service(t, func() { serviceRequests.Add(1) })
	conn, err := tls.Dial("tcp", strings.TrimPrefix(origin, "https://"), &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13})
	if err != nil {
		t.Fatal(err)
	}
	ca := filepath.Join(base, "service-ca.pem")
	err = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: conn.ConnectionState().PeerCertificates[0].Raw}), 0600)
	_ = conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	cli := w06CLI{t: t, binary: buildOrbitBinary(t, base), ca: ca}
	profile := filepath.Join(base, "profile.json")
	data, _ := json.Marshal(selection)
	if err = os.WriteFile(profile, data, 0600); err != nil {
		t.Fatal(err)
	}
	stateA, stateB := filepath.Join(base, "a"), filepath.Join(base, "b")
	for _, dir := range []string{stateA, stateB} {
		e00StopOnCleanup(t, base, dir)
	}
	review := stateA + "-network.json"
	cli.ok("network", "preview", "--state", stateA, "--mode", "self_hosted", "--profile-file", profile, "--review-file", review, "--json")
	cli.ok("network", "apply", "--state", stateA, "--review-file", review, "--json")

	rootA, rootB := filepath.Join(base, "orbit-a"), filepath.Join(base, "orbit-b")
	if err = os.Mkdir(rootA, 0700); err != nil {
		t.Fatal(err)
	}
	shared := filepath.Join(rootA, "shared.txt")
	if err = os.WriteFile(shared, []byte("ordinary 0644 source"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(shared, 0644); err != nil {
		t.Fatal(err)
	}
	setupFile := filepath.Join(base, "create.json")
	cli.ok("setup", "--state", stateA, "--root", rootA, "--label", "PC", "--name", "Orbit", "--preview", "--review-file", setupFile, "--json")
	created := cli.ok("setup", "--state", stateA, "--request-file", setupFile, "--timeout", "0", "--json")
	w06Wait(t, stateA, created.Operation.ID)
	inviteReview, invFile := filepath.Join(base, "invite-review.json"), filepath.Join(base, "invitation.json")
	cli.ok("devices", "invite", "--state", stateA, "--folder", "Orbit", "--preview", "--review-file", inviteReview, "--json")
	if _, out, e := cli.call("", "devices", "invite", "--state", stateA, "--request-file", inviteReview, "--out", invFile, "--json"); e != nil {
		t.Fatal(e, out)
	}

	// F08 (CLI half): a fresh joiner's reviewed plan for a routed invitation.
	var inv tc.Invitation
	if b, e := os.ReadFile(invFile); e != nil || json.Unmarshal(b, &inv) != nil || inv.Route == nil {
		t.Fatal("routed invitation unreadable", e)
	}
	freshFile := filepath.Join(base, "join-fresh.json")
	cli.ok("join", "--state", stateB, "--root", rootB, "--label", "Pi", "--name", "Orbit", "--invitation-file", invFile, "--preview", "--review-file", freshFile, "--json")
	var plan tc.Mutation
	if b, e := os.ReadFile(freshFile); e != nil || json.Unmarshal(b, &plan) != nil || plan.Join == nil {
		t.Fatal("fresh join review unreadable", e)
	}
	planned := tc.NetworkPolicy{}
	if plan.Join.Network != nil {
		planned = *plan.Join.Network
	}
	t.Logf("F08: fresh routed join plan: mode %q, profile set %v, awaiting profile %v; inviter's operator profile preselected %v",
		planned.Mode, planned.Profile != "", planned.AwaitingProfile, planned.Profile == inv.Route.Profile)
	fresh, freshOut, e := cli.call("", "join", "--state", stateB, "--request-file", freshFile, "--timeout", "0", "--json")
	freshCode := ""
	if fresh.Error != nil {
		freshCode = fresh.Error.Code + ": " + fresh.Error.Message
	}
	t.Logf("F08: submitting the fresh plan => err=%v %s", e, freshCode)
	_ = freshOut
	if planned.Profile != inv.Route.Profile || e != nil {
		t.Errorf("F08: fresh join does not preselect the routed invitation's operator (approved: Automatic/inviter operator offered for review, then the join proceeds)")
	}
	// Continue the journey with an explicitly reviewed policy on the joiner.
	reviewB := stateB + "-network.json"
	cli.ok("network", "preview", "--state", stateB, "--mode", "self_hosted", "--profile-file", profile, "--review-file", reviewB, "--json")
	cli.ok("network", "apply", "--state", stateB, "--review-file", reviewB, "--json")
	joinFile := filepath.Join(base, "join.json")
	cli.ok("join", "--state", stateB, "--root", rootB, "--label", "Pi", "--name", "Orbit", "--invitation-file", invFile, "--preview", "--review-file", joinFile, "--json")
	joined := cli.ok("join", "--state", stateB, "--request-file", joinFile, "--timeout", "0", "--json")

	// F10: wait unapproved, polling as the TUI progress screen does (3 s).
	waitStart := time.Now()
	codes := map[string]int{}
	firstLimited := time.Duration(0)
	for time.Since(waitStart) < 150*time.Second {
		q, err := (&controlclient.Client{StateDir: stateB}).Query(context.Background(), tc.Query{Version: tc.Version, Kind: "operation", ID: joined.Operation.ID})
		if err != nil {
			t.Fatal(err)
		}
		if q.Error != nil {
			codes[q.Error.Code]++
			if q.Error.Code == "RATE_LIMITED" && firstLimited == 0 {
				firstLimited = time.Since(waitStart)
			}
		}
		time.Sleep(3 * time.Second)
	}
	t.Logf("F10: joiner errors during %s unapproved wait: %v; first RATE_LIMITED after %s; local service requests %d", time.Since(waitStart).Round(time.Second), codes, firstLimited.Round(time.Second), serviceRequests.Load())
	if codes["RATE_LIMITED"] > 0 {
		t.Errorf("F10: waiting joiner shown RATE_LIMITED %d times", codes["RATE_LIMITED"])
	}

	approvalFile := filepath.Join(base, "approve.json")
	var pending tc.Result
	for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline); time.Sleep(time.Second) {
		if pending = cli.ok("devices", "requests", "--state", stateA, "--json"); len(pending.Requests) == 1 {
			break
		}
	}
	if len(pending.Requests) != 1 {
		t.Fatalf("request never reached the inviter: %+v", pending.Requests)
	}
	cli.ok("devices", "requests", "show", "--state", stateA, "--device", "Pi", "--review-file", approvalFile, "--json")
	cli.ok("devices", "approve", "--state", stateA, "--review-file", approvalFile, "--json")
	w06Wait(t, stateB, joined.Operation.ID)
	received := filepath.Join(rootB, "shared.txt")
	for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
		if b, _ := os.ReadFile(received); string(b) == "ordinary 0644 source" {
			break
		}
	}
	fi, err := os.Stat(received)
	if err != nil {
		t.Fatal("F16: file never received", err)
	}
	// By design (persistence: safe local permissions); E04 documents it.
	t.Logf("F16: source mode 0644, received mode %#o", fi.Mode().Perm())
	if fi.Mode().Perm() != 0600 {
		t.Errorf("F16: received mode %#o, documented safe local permission is 0600", fi.Mode().Perm())
	}

	// F11: the inviter lists the joiner by the name it chose ("Pi").
	text, err := e00Run(t, append(os.Environ(), "SSL_CERT_FILE="+ca), 30*time.Second, cli.binary, "devices", "--state", stateA)
	t.Logf("F11: inviter `orbit devices`:\n%s", strings.TrimSpace(text))
	if err != nil || !strings.Contains(text, "Pi") || strings.Contains(text, "Device ") {
		t.Errorf("F11: inviter's device list does not show the joiner's chosen name")
	}
	asJSON, _ := e00Run(t, append(os.Environ(), "SSL_CERT_FILE="+ca), 30*time.Second, cli.binary, "devices", "--state", stateA, "--json")
	if !strings.Contains(asJSON, `"Pi"`) {
		t.Errorf("F11: inviter's JSON device list lacks the joiner's chosen name")
	}
}
