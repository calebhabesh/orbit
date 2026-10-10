package terminal_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/app"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
	"github.com/calebhabesh/orbit/internal/workspace"
)

// e08Daemon starts a daemon on disposable state with one registered folder
// holding a few files, a directory and a name with a control character.
func e08Daemon(t *testing.T) (binary, state, root, base string, env []string) {
	t.Helper()
	base = testkit.NewDisposable(t)
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
	root = filepath.Join(base, "Notes")
	for path, body := range map[string]string{"plan.md": "plan\n", "docs/a.txt": "a\n", "docs/b.txt": "b\n", "tab\tname.txt": "t\n"} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 10000; i++ {
		if err := os.WriteFile(filepath.Join(root, "docs", fmt.Sprintf("f%05d.txt", i)), []byte("page\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	db, err := repository.Open(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	folder := history.ID{0xe8}
	err = db.EnsureFolder(ctx, folder, local, 1)
	if err == nil {
		_, err = db.ApproveMembership(ctx, protocol.Membership{Folder: folder, Revision: 1, Active: []protocol.ActiveMember{{Device: local, KeyPin: history.Digest{1}}}})
	}
	if err == nil {
		_, err = workspace.New(db, workspace.Options{}).Register(ctx, folder, root)
	}
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	editor := filepath.Join(base, "editor.sh")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\nprintf 'edited in editor\\n' >> \"$1\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	env = []string{"HOME=" + base, "PATH=" + os.Getenv("PATH"), "ORBIT_DISABLE_PACKAGED_PROFILE=1", "EDITOR=" + editor}
	e00StopOnCleanup(t, base, state)
	if out, err := e00Run(t, env, 30*time.Second, binary, "launch", "--state", state, "--no-browser", "--json"); err != nil {
		t.Fatalf("launch: %v\n%s", err, out)
	}
	return binary, state, root, base, env
}

func e08Files(t *testing.T, env []string, binary, state string, args ...string) tc.Result {
	t.Helper()
	out, err := e00Run(t, env, 30*time.Second, binary, append([]string{"files", "--state", state, "--json"}, args...)...)
	if err != nil {
		t.Fatalf("orbit files %v: %v\n%s", args, err, out)
	}
	var r tc.Result
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	return r
}

// E08 in a real terminal: the TUI lands on Files, shows the same entries and
// states as `orbit files`, walks the tree, round-trips $EDITOR and survives a
// resize.
func TestOnboardingE08RealPTYFilesViewMatchesCLI(t *testing.T) {
	binary, state, root, _, env := e08Daemon(t)
	var listing tc.Result
	deadline := time.Now().Add(120 * time.Second)
	for {
		listing = e08Files(t, env, binary, state)
		ready := len(listing.Files) == 3
		if ready {
			found := e08Files(t, env, binary, state, "--search", "f09999")
			ready = len(found.Files) == 1
		}
		for _, f := range listing.Files {
			ready = ready && f.State == "captured"
		}
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("scan did not capture the fixture: %+v", listing.Files)
		}
		time.Sleep(300 * time.Millisecond)
	}
	var expect []string
	for _, f := range listing.Files {
		name := f.Name
		if f.Directory {
			name += "/"
		}
		expect = append(expect, strings.ReplaceAll(name, "\t", `\u0009`)+"|"+tc.FileStateLabel(f.State))
	}
	t.Logf("CLI listing: %v", expect)
	out, err := e00Run(t, env, 30*time.Second, binary, "files", "--state", state)
	t.Logf("orbit files:\n%s", out)
	if err != nil || !strings.Contains(out, "Saved here") || strings.Contains(out, "\t name") {
		t.Fatalf("text listing: %v", err)
	}

	script := `
import os,pty,select,signal,subprocess,sys,time,struct,fcntl,termios
sys.path.insert(0,sys.argv[3])
from terminal_vt import Screen
binary,state=sys.argv[1:3]
expect=sys.argv[4].split(';')
master,slave=pty.openpty()
fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',40,140,0,0))
env=dict(os.environ,TERM='xterm-256color')
p=subprocess.Popen([binary,'--state',state],stdin=slave,stdout=slave,stderr=slave,env=env,start_new_session=True)
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
wait('Files: Notes')
print('--- landing ---'); print(screen.text())
for e in expect:
 name,label=e.split('|')
 line=[l for l in screen.text().split('\n') if name in l]
 if not line or label not in line[0]: raise SystemExit('row %r with %r missing'%(name,label))
os.write(master,b'\r'); wait('Files: Notes/docs')
wait('b.txt')
os.write(master,b']'); wait('f00018.txt')
os.write(master,b'/f09999\r'); wait('docs/f09999.txt')
print('--- paged and searched 10,000 entries ---'); print(screen.text())
os.write(master,b'\x1b'); wait('Files: Notes/docs')
os.write(master,b'\x7f'); wait('▌ docs/')
os.write(master,b'j'); time.sleep(0.3)
os.write(master,b'e'); wait('Editor closed')
print('--- after editor ---'); print(screen.text())
fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',20,58,0,0))
os.kill(p.pid,signal.SIGWINCH)
screen.resize(58,20)
wait('j/k'); wait('Enter')
print('--- narrow ---'); print(screen.text())
os.write(master,b'q')
p.wait(timeout=10)
`
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	scripts, _ := filepath.Abs("../../scripts")
	cmd := exec.CommandContext(ctx, "python3", "-c", script, binary, state, scripts, strings.Join(expect, ";"))
	cmd.Env = env
	pty, err := cmd.CombinedOutput()
	t.Logf("%s", pty)
	if err != nil {
		t.Fatalf("PTY: %v", err)
	}
	// The editor ran on the working copy of the selected row (plan.md, the
	// first file after docs/), and Orbit captures the saved file.
	body, err := os.ReadFile(filepath.Join(root, "plan.md"))
	if err != nil || !strings.Contains(string(body), "edited in editor") {
		t.Fatalf("editor did not edit plan.md: %q %v", body, err)
	}
	deadline = time.Now().Add(30 * time.Second)
	for {
		d := e08Files(t, env, binary, state, "plan.md").File
		if d != nil && len(d.Heads) == 1 && d.Heads[0].Bytes == tc.Uint(len(body)) && d.Entry.State == "captured" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("edit not captured: %+v", d)
		}
		time.Sleep(300 * time.Millisecond)
	}
}
