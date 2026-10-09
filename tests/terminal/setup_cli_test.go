package terminal_test

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/app"
	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
)

func TestTerminalT04TwoDeviceCLIInterruptedJoinAndEdits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	base := testkit.NewDisposable(t)
	os.Chmod(base, 0700)
	binary := filepath.Join(base, "orbit")
	build := exec.Command("go", "build", "-o", binary, "./cmd/orbit")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	command := func(args ...string) []byte {
		t.Helper()
		out, err := exec.Command(binary, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("CLI %s failed: %v", args[0], err)
		}
		return out
	}
	dirs := [2]string{filepath.Join(base, "state-a"), filepath.Join(base, "state-b")}
	roots := [2]string{filepath.Join(base, "root-a"), filepath.Join(base, "root-b")}
	cfgs := [2]config.Config{}
	clients := [2]*controlclient.Client{}
	processes := map[int]*exec.Cmd{}
	waitLive := func(i int) {
		t.Helper()
		until := time.Now().Add(10 * time.Second)
		for time.Now().Before(until) {
			var r tc.Result
			if clients[i].Call(ctx, "POST", "/control/terminal/v1/query", tc.Query{Version: "1", Kind: "capabilities"}, &r) == nil {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("daemon failed readiness")
	}
	start := func(i int) {
		t.Helper()
		p := exec.Command(binary, "engine", "serve", "--state", dirs[i], "--control-listen", "127.0.0.1:0", "--sync-interval", "1s", "--no-watch")
		if err := p.Start(); err != nil {
			t.Fatal(err)
		}
		processes[i] = p
		waitLive(i)
	}
	for i := range dirs {
		var err error
		cfgs[i], err = app.Initialize(ctx, dirs[i], app.SystemDependencies())
		if err != nil {
			t.Fatal(err)
		}
		os.Mkdir(roots[i], 0700)
		// Reserve both addresses together so they cannot alias.
		ip := networkIP(t)
		peer, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
		if err != nil {
			t.Fatal(err)
		}
		enrollment, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
		if err != nil {
			t.Fatal(err)
		}
		s := config.DefaultRuntimeSettings()
		s.PeerListen = peer.Addr().String()
		s.EnrollmentListen = enrollment.Addr().String()
		s.AdvertisedPeer = s.PeerListen
		s.AdvertisedEnrollment = s.EnrollmentListen
		if err = config.SaveRuntimeSettings(dirs[i], s); err != nil {
			t.Fatal(err)
		}
		peer.Close()
		enrollment.Close()
		clients[i] = &controlclient.Client{StateDir: dirs[i]}
		t.Cleanup(func() {
			if p := processes[i]; p != nil {
				if err := testkit.ValidateDestructiveTarget(base, dirs[i]); err != nil {
					t.Error(err)
					return
				}
				p.Process.Signal(syscall.SIGTERM)
				if err := p.Wait(); err != nil {
					t.Error("disposable daemon exit failed")
				}
			}
		})
		start(i)
	}
	os.WriteFile(filepath.Join(roots[0], "from-owner"), []byte("initial remote content"), 0600)
	os.WriteFile(filepath.Join(roots[1], "local-keep"), []byte("preexisting join bytes"), 0600)
	requestA := filepath.Join(base, "create.json")
	command("setup", "--state", dirs[0], "--root", roots[0], "--label", "Laptop", "--name", "Notes", "--preview", "--review-file", requestA, "--json")
	var created tc.Result
	if err := json.Unmarshal(command("setup", "--state", dirs[0], "--request-file", requestA, "--json"), &created); err != nil || created.State != "completed" {
		t.Fatal("CLI creation not complete")
	}
	var invite control.CreateInvitationResult
	if err := json.Unmarshal(command("invite", "create", "--state", dirs[0], "--json"), &invite); err != nil {
		t.Fatal(err)
	}
	invitation := filepath.Join(base, "invitation.txt")
	os.WriteFile(invitation, []byte(invite.InvitationCode), 0600)
	requestB := filepath.Join(base, "join.json")
	command("join", "--state", dirs[1], "--root", roots[1], "--label", "Pi", "--name", "Notes", "--invitation-file", invitation, "--preview", "--review-file", requestB, "--json")
	var pending tc.Result
	if err := json.Unmarshal(command("join", "--state", dirs[1], "--request-file", requestB, "--timeout", "0", "--json"), &pending); err != nil || pending.Join.Request == "" || pending.Readiness.Ready() {
		t.Fatal("join was not pending")
	}
	// Only this owned child under a marked root is killed. Its journal was
	// acknowledged through live HTTP before SIGKILL.
	if err := testkit.ValidateDestructiveTarget(base, dirs[1]); err != nil {
		t.Fatal(err)
	}
	if err := processes[1].Process.Kill(); err != nil {
		t.Fatal(err)
	}
	processes[1].Wait()
	delete(processes, 1)
	start(1)
	requests, err := clients[0].Query(ctx, tc.Query{Version: "1", Kind: "requests"})
	if err != nil || len(requests.Requests) != 1 {
		t.Fatal("durable enrollment missing")
	}
	p := requests.Requests[0]
	review := tc.ApprovalIntent{Request: p.ID, Folder: p.Folder, Requester: p.Requester, KeyPin: p.KeyPin, TranscriptDigest: p.TranscriptDigest, ExpectedMembership: p.ExpectedMembership, Decision: "approve"}
	b, _ := json.Marshal(review)
	approval := filepath.Join(base, "approval.json")
	os.WriteFile(approval, b, 0600)
	command("requests", "approve", "--state", dirs[0], "--request", p.ID, "--review-file", approval, "--operation", enrollmentRandom(t), "--json")
	until := time.Now().Add(50 * time.Second)
	var ready tc.Result
	for time.Now().Before(until) {
		ready, err = clients[1].Query(ctx, tc.Query{Version: "1", Kind: "operation", ID: pending.Operation.ID})
		if err != nil {
			t.Fatal(err)
		}
		if ready.State == "completed" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if ready.State != "completed" || !ready.Readiness.Ready() || ready.Join.Request != pending.Join.Request || ready.Join.Attempt != pending.Join.Attempt {
		t.Fatalf("daemon failed resume: state=%s error=%+v readiness=%+v", ready.State, ready.Error, ready.Readiness)
	}
	for i, dir := range dirs {
		cfg, err := config.Load(dir)
		if err != nil || cfg.DeviceID != cfgs[i].DeviceID {
			t.Fatal("join restart changed identity")
		}
	}
	waitBytes := func(path, want string) {
		t.Helper()
		until := time.Now().Add(15 * time.Second)
		for time.Now().Before(until) {
			if b, err := os.ReadFile(path); err == nil && string(b) == want {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("expected transferred bytes at %s", filepath.Base(path))
	}
	waitBytes(filepath.Join(roots[1], "from-owner"), "initial remote content")
	waitBytes(filepath.Join(roots[0], "local-keep"), "preexisting join bytes")
	os.WriteFile(filepath.Join(roots[0], "owner-edit"), []byte("ordinary owner edit"), 0600)
	waitBytes(filepath.Join(roots[1], "owner-edit"), "ordinary owner edit")
	os.WriteFile(filepath.Join(roots[1], "receiver-edit"), []byte("ordinary receiver edit"), 0600)
	waitBytes(filepath.Join(roots[0], "receiver-edit"), "ordinary receiver edit")
	// Stop both owners before exact durable head/hash assertions.
	for i, p := range processes {
		p.Process.Signal(syscall.SIGTERM)
		if err := p.Wait(); err != nil {
			t.Fatal(err)
		}
		delete(processes, i)
	}
	for _, dir := range dirs {
		db, err := repository.Open(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		folder := mustID(t, created.Join.Folder)
		for _, path := range []string{"from-owner", "local-keep", "owner-edit", "receiver-edit"} {
			heads, err := db.Heads(ctx, folder, path)
			if err != nil || len(heads) != 1 || heads[0].Manifest == nil {
				db.Close()
				t.Fatal("exact head set mismatch")
			}
			if err = db.VerifyManifest(heads[0].Manifest); err != nil {
				db.Close()
				t.Fatal("stored digest mismatch")
			}
		}
		db.Close()
	}
	safe, _ := json.Marshal(ready)
	if strings.Contains(string(safe), invite.Token) {
		t.Fatal("capability leaked")
	}
}

func TestTerminalT04InteractiveReviewRetainsInputs(t *testing.T) {
	base := testkit.NewDisposable(t)
	os.Chmod(base, 0700)
	binary := filepath.Join(base, "orbit")
	build := exec.Command("go", "build", "-o", binary, "./cmd/orbit")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	dir := filepath.Join(base, "state")
	root := filepath.Join(base, "adopt")
	os.Mkdir(root, 0700)
	os.WriteFile(filepath.Join(root, "keep"), []byte("interactive preserved"), 0600)
	t.Cleanup(func() {
		if _, err := os.Stat(filepath.Join(dir, ".agent.pid")); os.IsNotExist(err) {
			return
		}
		if err := testkit.ValidateDestructiveTarget(base, dir); err != nil {
			t.Error(err)
			return
		}
		if err := app.StopAgent(dir, 5*time.Second); err != nil {
			t.Error(err)
		}
	})
	script := `
import os,pty,select,subprocess,sys,time
binary,state,root=sys.argv[1:]
master,slave=pty.openpty()
process=subprocess.Popen([binary,'setup','--state',state,'--root',root],stdin=slave,stdout=slave,stderr=slave)
os.close(slave)
transcript=b'';pending=b'';reviews=0;invalid=False;deadline=time.monotonic()+30
while time.monotonic()<deadline:
 ready,_,_=select.select([master],[],[],0.1)
 if ready:
  try: chunk=os.read(master,65536)
  except OSError: break
  if not chunk: break
  transcript+=chunk;pending+=chunk
  if pending.endswith(b']: '):
   prompt=pending.split(b'\n')[-1];answer=b''
   if b'Device name' in prompt:
    if reviews: assert b'Laptop' in prompt
    else: answer=b'Laptop'
   elif b'Orbit name' in prompt:
    if reviews: assert b'Notes' in prompt
    else: answer=b'Notes'
   elif b'Data budget bytes' in prompt and not invalid: answer=b'0';invalid=True
   elif b'Create/adopt' in prompt: answer=b'edit' if reviews==0 else b'yes';reviews+=1
   os.write(master,answer+b'\n');pending=b''
 if process.poll() is not None: break
assert process.wait(timeout=5)==0
assert reviews==2 and invalid
assert b'Enter a positive byte/count value.' in transcript
assert b'phase=ready' in transcript
assert b'\x1b[' not in transcript
os.close(master)
print('PTY create/adopt: invalid finite input corrected, review edited, names retained, Ready observed, plain output')
`
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", "-c", script, binary, dir, root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PTY onboarding: %v %s", err, out)
	}
	client := &controlclient.Client{StateDir: dir}
	r, err := client.Query(context.Background(), tc.Query{Version: "1", Kind: "capabilities"})
	if err != nil || r.Version != "1" {
		t.Fatal("client exit stopped daemon")
	}
	settings, err := config.LoadSettings(dir)
	if err != nil || settings.DeviceLabel != "Laptop" {
		t.Fatal("edited review lost label")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "keep")); string(b) != "interactive preserved" {
		t.Fatal("interactive adoption lost bytes")
	}
}
