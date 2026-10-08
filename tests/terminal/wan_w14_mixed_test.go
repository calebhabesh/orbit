package terminal_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/testkit"
)

// w14LegacyRef is the last pre-WAN source revision (T11 terminal release). Its
// binaries are the "old peers" an upgraded device must keep working with.
const w14LegacyRef = "ef462f2"

func buildLegacyOrbit(t *testing.T, base string) string {
	t.Helper()
	ref := os.Getenv("ORBIT_W14_LEGACY_REF")
	if ref == "" {
		ref = w14LegacyRef
	}
	src := filepath.Join(base, "legacy-src")
	if err := os.Mkdir(src, 0700); err != nil {
		t.Fatal(err)
	}
	archive := exec.Command("git", "archive", "--format=tar", ref)
	archive.Dir = "../.."
	tarball, err := archive.Output()
	if err != nil {
		t.Skipf("legacy revision %s unavailable: %v", ref, err)
	}
	extract := exec.Command("tar", "-x", "-C", src)
	extract.Stdin = bytes.NewReader(tarball)
	if out, err := extract.CombinedOutput(); err != nil {
		t.Fatalf("extract legacy source: %v %s", err, out)
	}
	binary := filepath.Join(base, "orbit-legacy")
	build := exec.Command("go", "build", "-o", binary, "./cmd/orbit")
	build.Dir = src
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build legacy %s: %v\n%s", ref, err, out)
	}
	return binary
}

// Mixed versions: a pre-WAN device invites an upgraded one over the explicit
// manual path, files move both ways, the old device's state is then upgraded
// in place (packaged profile present, manual policy kept, no service traffic)
// and finally rolled back to the old binary, which still opens and syncs.
func TestWANW14MixedVersionUpgradeAndRollback(t *testing.T) {
	ctx := context.Background()
	base := testkit.NewDisposable(t)
	os.Chmod(base, 0700)
	legacy := buildLegacyOrbit(t, base)
	current := buildOrbitBinary(t, base)
	dirs := [2]string{filepath.Join(base, "state-old"), filepath.Join(base, "state-new")}
	roots := [2]string{filepath.Join(base, "root-old"), filepath.Join(base, "root-new")}
	binaries := [2]string{legacy, current}
	clients := [2]*controlclient.Client{{StateDir: dirs[0]}, {StateDir: dirs[1]}}
	processes := map[int]*exec.Cmd{}
	packaged := map[int]bool{}
	run := func(i int, args ...string) []byte {
		t.Helper()
		if filepath.Base(binaries[i]) != "orbit" { // argv0 "orbit" implies the orbit namespace
			args = append([]string{"orbit"}, args...)
		}
		cmd := exec.Command(binaries[i], args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s CLI %v: %v\n%s%s", filepath.Base(binaries[i]), args, err, out, stderr.Bytes())
		}
		return out
	}
	start := func(i int) {
		t.Helper()
		p := exec.Command(binaries[i], "serve", "--state", dirs[i], "--control-listen", "127.0.0.1:0", "--sync-interval", "1s", "--no-watch")
		if packaged[i] {
			// Real packaged release profile on a manual install: it must stay
			// manual and never contact the hosted service.
			p.Env = append(os.Environ(), network.DisablePackagedProfileEnv+"=0")
		}
		if err := p.Start(); err != nil {
			t.Fatal(err)
		}
		processes[i] = p
		until := time.Now().Add(15 * time.Second)
		for time.Now().Before(until) {
			var r tc.Result
			if clients[i].Call(ctx, "POST", "/control/terminal/v1/query", tc.Query{Version: "1", Kind: "capabilities"}, &r) == nil {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("daemon failed readiness")
	}
	stop := func(i int) {
		t.Helper()
		if err := testkit.ValidateDestructiveTarget(base, dirs[i]); err != nil {
			t.Fatal(err)
		}
		p := processes[i]
		p.Process.Signal(syscall.SIGTERM)
		if err := p.Wait(); err != nil {
			t.Fatal("daemon exit", err)
		}
		delete(processes, i)
	}
	t.Cleanup(func() {
		for i, p := range processes {
			if testkit.ValidateDestructiveTarget(base, dirs[i]) == nil {
				p.Process.Signal(syscall.SIGTERM)
				p.Wait()
			}
		}
	})
	ids := [2]string{}
	for i := range dirs {
		if out, err := exec.Command(binaries[i], "init", "--state", dirs[i]).CombinedOutput(); err != nil {
			t.Fatalf("init %d: %v %s", i, err, out)
		}
		cfg, err := config.Load(dirs[i])
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = cfg.DeviceID
		os.Mkdir(roots[i], 0700)
		ip := networkIP(t)
		peer, _ := net.Listen("tcp", net.JoinHostPort(ip, "0"))
		enrollment, _ := net.Listen("tcp", net.JoinHostPort(ip, "0"))
		s := config.DefaultRuntimeSettings()
		s.PeerListen, s.EnrollmentListen = peer.Addr().String(), enrollment.Addr().String()
		s.AdvertisedPeer, s.AdvertisedEnrollment = s.PeerListen, s.EnrollmentListen
		if err = config.SaveRuntimeSettings(dirs[i], s); err != nil {
			t.Fatal(err)
		}
		peer.Close()
		enrollment.Close()
		start(i)
	}
	os.WriteFile(filepath.Join(roots[0], "from-old"), []byte("written by the pre-WAN device"), 0600)
	request := filepath.Join(base, "create.json")
	run(0, "setup", "--state", dirs[0], "--root", roots[0], "--label", "Old laptop", "--name", "Notes", "--preview", "--review-file", request, "--json")
	var created tc.Result
	if err := json.Unmarshal(run(0, "setup", "--state", dirs[0], "--request-file", request, "--json"), &created); err != nil || created.State != "completed" {
		t.Fatal("legacy creation incomplete", err)
	}
	folder := created.Join.Folder
	var invite control.CreateInvitationResult
	if err := json.Unmarshal(run(0, "invite", "create", "--state", dirs[0], "--json"), &invite); err != nil {
		t.Fatal(err)
	}
	invitation := filepath.Join(base, "invitation.txt")
	os.WriteFile(invitation, []byte(invite.InvitationCode), 0600)
	join := filepath.Join(base, "join.json")
	run(1, "join", "--state", dirs[1], "--root", roots[1], "--label", "New Pi", "--name", "Notes", "--connection", "manual", "--invitation-file", invitation, "--preview", "--review-file", join, "--json")
	var pending tc.Result
	if err := json.Unmarshal(run(1, "join", "--state", dirs[1], "--request-file", join, "--timeout", "0", "--json"), &pending); err != nil || pending.Join.Request == "" {
		t.Fatal("new device join not pending", err)
	}
	requests, err := clients[0].Query(ctx, tc.Query{Version: "1", Kind: "requests"})
	if err != nil || len(requests.Requests) != 1 {
		t.Fatal("legacy inviter did not record request", err)
	}
	p := requests.Requests[0]
	b, _ := json.Marshal(tc.ApprovalIntent{Request: p.ID, Folder: p.Folder, Requester: p.Requester, KeyPin: p.KeyPin, TranscriptDigest: p.TranscriptDigest, ExpectedMembership: p.ExpectedMembership, Decision: "approve"})
	approval := filepath.Join(base, "approval.json")
	os.WriteFile(approval, b, 0600)
	run(0, "requests", "approve", "--state", dirs[0], "--request", p.ID, "--review-file", approval, "--operation", enrollmentRandom(t), "--json")
	waitBytes := func(path, want string) {
		t.Helper()
		until := time.Now().Add(40 * time.Second)
		for time.Now().Before(until) {
			if b, err := os.ReadFile(path); err == nil && string(b) == want {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("expected transferred bytes at %s", filepath.Base(path))
	}
	waitBytes(filepath.Join(roots[1], "from-old"), "written by the pre-WAN device")
	os.WriteFile(filepath.Join(roots[1], "from-new"), []byte("written by the upgraded device"), 0600)
	waitBytes(filepath.Join(roots[0], "from-new"), "written by the upgraded device")
	t.Log("legacy inviter -> new joiner over v2 manual invitation; both directions transferred")

	// Snapshot the legacy device's durable identity and heads, then upgrade it
	// in place with the packaged release profile present.
	stop(0)
	heads := func(dir string) map[string]string {
		t.Helper()
		db, err := repository.Open(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		out := map[string]string{}
		for _, path := range []string{"from-old", "from-new"} {
			h, err := db.Heads(ctx, mustID(t, folder), path)
			if err != nil || len(h) != 1 || h[0].Manifest == nil {
				t.Fatal("head set", path, err)
			}
			if err = db.VerifyManifest(h[0].Manifest); err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(h[0])
			out[path] = string(raw)
		}
		return out
	}
	before := heads(dirs[0])
	binaries[0], packaged[0] = current, true
	start(0)
	status, err := clients[0].Query(ctx, tc.Query{Version: "1", Kind: "network_status"})
	if err != nil || status.Network.Policy.Mode != "manual" || !status.Network.AutomaticOffer || status.Network.Builtin == nil || status.Network.Code != "MANUAL" {
		t.Fatal("upgraded legacy install not offered review / not kept manual", status.Network, err)
	}
	if _, err = os.Stat(filepath.Join(dirs[0], "network-profile.json")); !os.IsNotExist(err) {
		t.Fatal("upgrade adopted a profile without review")
	}
	os.WriteFile(filepath.Join(roots[0], "after-upgrade"), []byte("upgraded old device edit"), 0600)
	waitBytes(filepath.Join(roots[1], "after-upgrade"), "upgraded old device edit")
	stop(0)
	after := heads(dirs[0])
	for path, h := range before {
		if after[path] != h {
			t.Fatal("upgrade rewrote durable head", path)
		}
	}
	if cfg, _ := config.Load(dirs[0]); cfg.DeviceID != ids[0] {
		t.Fatal("upgrade changed identity")
	}
	t.Log("in-place upgrade: identity and exact heads retained; manual policy kept; Automatic offered, not adopted")

	// Roll back to the pre-WAN binary: it still opens the upgraded state and syncs.
	binaries[0], packaged[0] = legacy, false
	start(0)
	os.WriteFile(filepath.Join(roots[1], "after-rollback"), []byte("edit seen by rolled-back device"), 0600)
	waitBytes(filepath.Join(roots[0], "after-rollback"), "edit seen by rolled-back device")
	stop(0)
	stop(1)
	for i, dir := range dirs {
		if cfg, _ := config.Load(dir); cfg.DeviceID != ids[i] {
			t.Fatal("identity changed", i)
		}
	}
	t.Log("rollback to pre-WAN binary: state opens and syncs")
}
