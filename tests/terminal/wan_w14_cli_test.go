package terminal_test

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/app"
	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/testkit"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b[()][A-Z0-9]|\x1b[=>]`)

// ptyRun drives a production binary in a real PTY with scripted answers and
// returns its combined output; it never waits beyond a finite deadline.
func ptyRun(t *testing.T, answers string, binary string, args ...string) (string, error) {
	t.Helper()
	script := `import os, pty, subprocess, sys, select, time, fcntl, termios, struct
master, slave = pty.openpty()
fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 40, 160, 0, 0))
p = subprocess.Popen(sys.argv[1:], stdin=slave, stdout=slave, stderr=slave, close_fds=True)
os.close(slave)
answers = os.environ['ORBIT_W14_ANSWERS']
if answers.startswith('@'):
    time.sleep(float(answers[1:answers.index(':')]))
    answers = answers[answers.index(':')+1:]
os.write(master, answers.encode())
data = b''
end = time.monotonic()+60
while time.monotonic()<end:
    ready,_,_ = select.select([master],[],[],0.2)
    if ready:
        try: data += os.read(master,65536)
        except OSError: break
    if p.poll() is not None: break
try:
    status = p.wait(timeout=max(0.001, end-time.monotonic()))
except subprocess.TimeoutExpired:
    p.kill(); p.wait(); raise RuntimeError('pty timeout')
os.close(master)
sys.stdout.buffer.write(data)
sys.exit(status)`
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", append([]string{"-c", script, binary}, args...)...)
	cmd.Env = append(os.Environ(), "ORBIT_W14_ANSWERS="+answers)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// An upgraded manual install with the real packaged profile: the offer is
// visible, the one-step command shows operator/privacy and asks once, refusing
// leaves manual mode, scripts must pass --yes, and decline ends the offer. A
// one-line invitation code is accepted by a receiver's stdin. Nothing here
// contacts the hosted service: the daemon stays manual throughout.
func TestWANW14OneStepNetworkAndInvitationCode(t *testing.T) {
	t.Setenv(network.DisablePackagedProfileEnv, "0")
	base := testkit.NewDisposable(t)
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	cli := w06CLI{t: t, binary: buildOrbitBinary(t, base)}
	dirs := []string{filepath.Join(base, "state-a"), filepath.Join(base, "state-b")}
	t.Cleanup(func() {
		for _, dir := range dirs {
			if err := testkit.ValidateDestructiveTarget(base, dir); err == nil {
				_ = app.StopAgent(dir, 10*time.Second)
			}
		}
	})
	for _, dir := range dirs {
		if out, err := exec.Command(cli.binary, "init", "--state", dir).CombinedOutput(); err != nil {
			t.Fatal(err, string(out))
		}
		ip := networkIP(t)
		peer, _ := net.Listen("tcp", net.JoinHostPort(ip, "0"))
		enrollment, _ := net.Listen("tcp", net.JoinHostPort(ip, "0"))
		s := config.DefaultRuntimeSettings()
		s.PeerListen, s.EnrollmentListen = peer.Addr().String(), enrollment.Addr().String()
		s.AdvertisedPeer, s.AdvertisedEnrollment = s.PeerListen, s.EnrollmentListen
		peer.Close()
		enrollment.Close()
		if err := config.SaveRuntimeSettings(dir, s); err != nil {
			t.Fatal(err)
		}
	}
	review := filepath.Join(base, "setup.json")
	cli.ok("setup", "--state", dirs[0], "--root", filepath.Join(base, "root-a"), "--label", "Laptop", "--name", "Notes", "--connection", "manual", "--preview", "--review-file", review, "--json")
	r := cli.ok("setup", "--state", dirs[0], "--request-file", review, "--timeout", "0", "--json")
	w06Wait(t, dirs[0], r.Operation.ID)

	status := cli.ok("network", "status", "--state", dirs[0], "--json")
	n := status.Network
	if n.Policy.Mode != "manual" || !n.AutomaticOffer || n.Builtin == nil || n.Builtin.Operator == "" || !strings.Contains(n.Action, "orbit network automatic") {
		t.Fatal("manual install not offered packaged Automatic", n)
	}
	digest := n.Builtin.Digest

	_, out, err := cli.call("", "network", "automatic", "--state", dirs[0])
	if err == nil || !strings.Contains(out, "--yes") {
		t.Fatal("non-TTY one-step applied without --yes", err, out)
	}
	out, err = ptyRun(t, "n\n", cli.binary, "network", "automatic", "--state", dirs[0])
	for _, want := range []string{"Connection: Manual -> Automatic", "Operator: \"" + n.Builtin.Operator + "\"", "Privacy: ", "LAN advertising: true", "Apply? [Y/n]", "not applied"} {
		if !strings.Contains(out, want) {
			t.Fatalf("one-step review missing %q:\n%s", want, out)
		}
	}
	if err == nil {
		t.Fatal("declined confirmation exited successfully")
	}
	if p, _ := config.LoadNetworkPolicy(dirs[0]); p.Mode != "manual" {
		t.Fatal("refused confirmation changed policy", p)
	}
	t.Logf("one-step review transcript:\n%s", out)

	// The keyboard TUI overview shows the same one-time offer (before decline).
	out, _ = ptyRun(t, "@4:q", cli.binary, "tui", "--state", dirs[0])
	if plain := ansiPattern.ReplaceAllString(out, ""); !strings.Contains(plain, "Automatic connection with") {
		t.Fatalf("TUI overview lacks the packaged offer:\n%s", plain)
	}
	cli.ok("network", "automatic", "--decline", "--state", dirs[0], "--json")
	n = cli.ok("network", "status", "--state", dirs[0], "--json").Network
	if n.AutomaticOffer || n.Policy.Mode != "manual" || n.Builtin == nil || n.Builtin.Digest != digest {
		t.Fatal("decline not retained", n)
	}
	if _, err = os.Stat(filepath.Join(dirs[0], "network-profile.json")); !os.IsNotExist(err) {
		t.Fatal("profile stored without review")
	}

	// One-line invitation code, pasted into the receiver's stdin.
	cmd := exec.Command(cli.binary, "devices", "invite", "--state", dirs[0], "--code")
	codeOut, err := cmd.Output()
	code := strings.TrimSpace(string(codeOut))
	if err != nil || !strings.HasPrefix(code, "orbit-invitation:v2:") || strings.Contains(code, "\n") {
		t.Fatal("invite --code", err, code)
	}
	joinReview := filepath.Join(base, "join.json")
	preview, out, err := cli.call(code+"\n", "join", "--state", dirs[1], "--root", filepath.Join(base, "root-b"), "--label", "Pi", "--name", "Notes", "--connection", "manual", "--invitation-stdin", "--preview", "--review-file", joinReview, "--json")
	if err != nil || preview.Preview == nil {
		t.Fatal("pasted code not accepted", err, out)
	}
	if strings.Contains(out, code) {
		t.Fatal("receiver echoed the invitation secret")
	}
}
