package terminal_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/app"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
	"github.com/calebhabesh/orbit/internal/workspace"
)

// e02ExhaustedDaemon prepares disposable state holding one scan exhausted
// with ROOT_UNAVAILABLE and starts its daemon (detached: no user manager in
// this environment).
func e02ExhaustedDaemon(t *testing.T) (binary, state, task string, env []string) {
	t.Helper()
	base := testkit.NewDisposable(t)
	_ = os.Chmod(base, 0700)
	binary = buildOrbitBinary(t, base)
	state = filepath.Join(base, "state")
	ctx := context.Background()
	cfg, err := app.Initialize(ctx, state, app.SystemDependencies())
	if err != nil {
		t.Fatal(err)
	}
	var local history.ID
	if err = local.UnmarshalText([]byte(cfg.DeviceID)); err != nil {
		t.Fatal(err)
	}
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	// A registered folder gets the TUI past onboarding; the exhausted scan is
	// on a second, unregistered folder, so no startup scan supersedes it.
	shown, root := history.ID{0xe1}, filepath.Join(filepath.Dir(state), "Orbit")
	err = os.Mkdir(root, 0700)
	if err == nil {
		err = db.EnsureFolder(ctx, shown, local, 1)
	}
	if err == nil {
		_, err = db.ApproveMembership(ctx, protocol.Membership{Folder: shown, Revision: 1, Active: []protocol.ActiveMember{{Device: local, KeyPin: history.Digest{1}}}})
	}
	if err == nil {
		_, err = workspace.New(db, workspace.Options{}).Register(ctx, shown, root)
	}
	folder := history.ID{0xe2}
	if err == nil {
		err = db.EnsureFolder(ctx, folder, local, 1)
	}
	if err == nil {
		task, err = db.EnqueueDurableTask(ctx, repository.DurableTask{Folder: folder, Kind: "scan", MaxAttempts: 1})
	}
	if err == nil {
		err = db.UpdateDurableTaskState(ctx, task, "exhausted", 1, "root briefly missing (fixture)", "ROOT_UNAVAILABLE", 0)
	}
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	env = []string{"HOME=" + base, "PATH=" + os.Getenv("PATH"), "ORBIT_DISABLE_PACKAGED_PROFILE=1"}
	e00StopOnCleanup(t, base, state)
	if out, err := e00Run(t, env, 30*time.Second, binary, "launch", "--state", state, "--no-browser", "--json"); err != nil {
		t.Fatalf("launch: %v\n%s", err, out)
	}
	return binary, state, task, env
}

// F03: the retry advice shown for exhausted work runs while the daemon runs,
// both as advised (orbit retry) and through the retained engine command.
func TestOnboardingE02F03RetryAdviceRunsWithDaemon(t *testing.T) {
	binary, state, task, env := e02ExhaustedDaemon(t)
	out, err := e00Run(t, env, 30*time.Second, binary, "status", "--state", state, "--json")
	if err != nil {
		t.Fatalf("status: %v\n%s", err, out)
	}
	var status struct {
		Attention []struct{ Code, Action string } `json:"attention"`
	}
	_ = json.Unmarshal([]byte(out), &status)
	advice := ""
	for _, a := range status.Attention {
		if a.Code == "EXHAUSTED_WORK" {
			advice = a.Action
		}
	}
	if advice == "" {
		t.Fatalf("no EXHAUSTED_WORK attention:\n%s", out)
	}
	t.Logf("advice: %s", advice)
	if strings.Contains(advice, "orbit engine") || !strings.Contains(advice, "ROOT_UNAVAILABLE") {
		t.Fatalf("advice names an engine command or omits the failure: %q", advice)
	}
	quoted := regexp.MustCompile(`'(orbit [^']+)'`).FindStringSubmatch(advice)
	if quoted == nil {
		t.Fatalf("advice has no command: %q", advice)
	}
	for _, cmd := range [][]string{strings.Fields(quoted[1])[1:], {"engine", "work", "retry", "--task", task}} {
		out, err := e00Run(t, env, 30*time.Second, binary, append(cmd, "--state", state)...)
		t.Logf("orbit %s => %v: %s", strings.Join(cmd, " "), err, strings.TrimSpace(out))
		if err != nil || strings.Contains(out, "already owned by another agent") || !strings.Contains(out, "retried task "+task) {
			t.Fatalf("orbit %s did not retry through the running daemon", strings.Join(cmd, " "))
		}
	}
}

// F03 + F09 in a real terminal: the Attention section's EXHAUSTED_WORK row
// opens the retry review on Enter, and Enter re-queues the task through the
// running daemon.
func TestOnboardingE02F09RealPTYEnterOpensRetry(t *testing.T) {
	binary, state, task, env := e02ExhaustedDaemon(t)
	script := `
import os,pty,select,subprocess,sys,time,struct,fcntl,termios
sys.path.insert(0,sys.argv[3])
from terminal_vt import Screen
binary,state=sys.argv[1:3]
master,slave=pty.openpty()
fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',40,140,0,0))
env=dict(os.environ,TERM='xterm-256color')
p=subprocess.Popen([binary,'--state',state],stdin=slave,stdout=slave,stderr=slave,env=env,start_new_session=True)
os.close(slave)
screen=Screen(140,40)
def wait(text,limit=20):
 end=time.monotonic()+limit
 while time.monotonic()<end:
  r,_,_=select.select([master],[],[],0.1)
  if r:
   try: screen.feed(os.read(master,65536))
   except OSError: break
  if text in screen.text(): return
 raise SystemExit('timeout waiting for %r; screen:\n%s'%(text,screen.text()))
wait('Orbit')
time.sleep(1)
os.write(master,b'n'); wait('1 of 1')
os.write(master,b'\r'); wait('Retry work')
print('--- retry review ---'); print(screen.text())
os.write(master,b'\r'); wait('re-queued')
print('--- after Enter ---'); print(screen.text())
os.write(master,b'q')
p.wait(timeout=10)
`
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	scripts, _ := filepath.Abs("../../scripts")
	cmd := exec.CommandContext(ctx, "python3", "-c", script, binary, state, scripts)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("PTY: %v", err)
	}
	// The re-queued task left the exhausted state in the running daemon.
	status, err := e00Run(t, env, 30*time.Second, binary, "status", "--state", state, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(status, "attempt (ROOT_UNAVAILABLE)") && strings.Contains(status, task) && !strings.Contains(string(out), "re-queued") {
		t.Fatal("retry did not reach the daemon")
	}
}
