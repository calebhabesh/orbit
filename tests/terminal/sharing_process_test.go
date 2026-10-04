package terminal_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/app"
	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/controlclient"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
)

func TestTerminalT05ThreeProcessSharingOfflineRolloutAndEndpointRefresh(t *testing.T) {
	ctx := context.Background()
	base := testkit.NewDisposable(t)
	os.Chmod(base, 0700)
	binary := filepath.Join(base, "filesync")
	build := exec.Command("go", "build", "-o", binary, "./cmd/filesync")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	var dirs [3]string
	var roots [3]string
	var clients [3]*controlclient.Client
	var cfgs [3]config.Config
	var settings [3]tc.Settings
	processes := map[int]*exec.Cmd{}
	wait := func(label string, seconds int, check func() bool) {
		t.Helper()
		until := time.Now().Add(time.Duration(seconds) * time.Second)
		for time.Now().Before(until) {
			if check() {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("timeout: %s", label)
	}
	stop := func(i int) {
		t.Helper()
		p := processes[i]
		if p == nil {
			return
		}
		if err := testkit.ValidateDestructiveTarget(base, dirs[i]); err != nil {
			t.Fatal(err)
		}
		p.Process.Signal(syscall.SIGTERM)
		if err := p.Wait(); err != nil {
			t.Fatal(err)
		}
		delete(processes, i)
	}
	t.Cleanup(func() {
		for i := range processes {
			stop(i)
		}
	})
	start := func(i int) {
		t.Helper()
		p := exec.Command(binary, "serve", "--state", dirs[i], "--control-listen", "127.0.0.1:0", "--sync-interval", "1s", "--no-watch")
		if err := p.Start(); err != nil {
			t.Fatal(err)
		}
		processes[i] = p
		wait("daemon", 10, func() bool {
			var r tc.Result
			return clients[i].Call(ctx, "POST", "/control/terminal/v1/query", tc.Query{Version: "1", Kind: "capabilities"}, &r) == nil
		})
	}
	reserve := func() string {
		t.Helper()
		l, err := net.Listen("tcp", net.JoinHostPort(networkIP(t), "0"))
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		return l.Addr().String()
	}
	for i := range dirs {
		dirs[i] = filepath.Join(base, []string{"a-state", "b-state", "c-state"}[i])
		roots[i] = filepath.Join(base, []string{"a-root", "b-root", "c-root"}[i])
		os.Mkdir(roots[i], 0700)
		var err error
		cfgs[i], err = app.Initialize(ctx, dirs[i], app.SystemDependencies())
		if err != nil {
			t.Fatal(err)
		}
		settings[i] = config.DefaultRuntimeSettings()
		settings[i].PeerListen = reserve()
		settings[i].EnrollmentListen = reserve()
		settings[i].AdvertisedPeer = settings[i].PeerListen
		settings[i].AdvertisedEnrollment = settings[i].EnrollmentListen
		if err = config.SaveRuntimeSettings(dirs[i], settings[i]); err != nil {
			t.Fatal(err)
		}
		identity, err := replication.LoadOrCreateIdentity(dirs[i], mustID(t, cfgs[i].DeviceID), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = config.WritePrivate(dirs[i], "public.crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: identity.Leaf.Raw})); err != nil {
			t.Fatal(err)
		}
		clients[i] = &controlclient.Client{StateDir: dirs[i]}
		start(i)
	}
	mutate := func(i int, m tc.Mutation) tc.Result {
		t.Helper()
		r, err := clients[i].Mutate(ctx, m)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	review := func(i int, root, kind string) tc.SetupIntent {
		t.Helper()
		p := tc.SetupIntent{DeviceName: []string{"Laptop", "Pi", "VPS"}[i], FolderName: "Notes", Root: root, Settings: settings[i]}
		r, err := clients[i].Query(ctx, tc.Query{Version: "1", Kind: "root_preview", Path: root, Name: kind, RootPlan: &p})
		if err != nil || !r.Preview.Complete {
			t.Fatalf("preview: %v", err)
		}
		p.Preview = *r.Review
		return p
	}
	create := func(root string) tc.Result {
		p := review(0, root, "setup")
		return mutate(0, tc.Mutation{Version: "1", Kind: "setup", OperationID: enrollmentRandom(t), Setup: &p})
	}
	os.WriteFile(filepath.Join(roots[0], "original"), []byte("initial owner bytes"), 0600)
	first := create(roots[0])
	folder := first.Join.Folder
	membership := func(i int) control.MembershipExportResult {
		t.Helper()
		var r control.MembershipExportResult
		if err := clients[i].Call(ctx, "POST", "/api/v1/membership/export?folder="+folder, nil, &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	invite := func(target string) tc.Invitation {
		t.Helper()
		m := membership(0)
		digest, _ := protocol.MembershipDigest(m.Membership)
		kind := "invite"
		if target != "" {
			kind = "share"
		}
		r := mutate(0, tc.Mutation{Version: "1", Kind: kind, OperationID: enrollmentRandom(t), Invite: &tc.InviteIntent{Folder: folder, Device: target, ExpectedMembership: hex.EncodeToString(digest[:]), ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)}})
		return *r.Invitation
	}
	join := func(i int, root string, inv tc.Invitation) tc.Result {
		t.Helper()
		p := review(i, root, "join")
		m := tc.Mutation{Version: "1", Kind: "join", OperationID: enrollmentRandom(t), Join: &tc.JoinIntent{Invitation: inv, Attempt: enrollmentRandom(t), DeviceName: p.DeviceName, FolderName: p.FolderName, Root: p.Root, Preview: p.Preview, Settings: p.Settings}}
		pending := mutate(i, m)
		if pending.Join.Request == "" || pending.Readiness.Ready() {
			t.Fatal("join bypassed approval")
		}
		rs, err := clients[0].Query(ctx, tc.Query{Version: "1", Kind: "requests", Folder: folder})
		if err != nil {
			t.Fatal(err)
		}
		var found bool
		for _, p := range rs.Requests {
			if p.ID == pending.Join.Request {
				found = true
				mutate(0, tc.Mutation{Version: "1", Kind: "approval", OperationID: enrollmentRandom(t), Approval: &tc.ApprovalIntent{Request: p.ID, Folder: p.Folder, Requester: p.Requester, KeyPin: p.KeyPin, TranscriptDigest: p.TranscriptDigest, ExpectedMembership: p.ExpectedMembership, Decision: "approve"}})
			}
		}
		if !found {
			t.Fatal("request missing")
		}
		var ready tc.Result
		wait("reviewed join", 55, func() bool {
			ready, err = clients[i].Query(ctx, tc.Query{Version: "1", Kind: "operation", ID: pending.Operation.ID})
			return err == nil && ready.State == "completed"
		})
		if !ready.Readiness.Ready() {
			t.Fatalf("false ready: %+v", ready.Readiness)
		}
		return ready
	}
	bytes := func(path, want string) {
		wait("exact file "+filepath.Base(path), 20, func() bool { b, e := os.ReadFile(path); return e == nil && string(b) == want })
	}
	joinedB := join(1, roots[1], invite(""))
	// Offline B retains its membership; C approval advances A only.
	stop(1)
	// Reopen the approving daemon too: persisted membership/request records must survive.
	stop(0)
	start(0)
	joinedC := join(2, roots[2], invite(""))
	if joinedB.Join.Request == joinedC.Join.Request {
		t.Fatal("request collision")
	}
	start(1)
	wait("offline member revision rollout", 20, func() bool { return membership(1).Membership.Revision == 3 })
	// Configure the direct B<->C pull directions explicitly using persistent certs.
	endpoint := func(i, j int, url string) {
		t.Helper()
		req := control.SetPeerEndpointRequest{Folder: folder, Device: cfgs[j].DeviceID, URL: url, Certificate: filepath.Join(dirs[j], "public.crt")}
		if err := clients[i].Call(ctx, "POST", "/api/v1/settings/peers", req, nil); err != nil {
			t.Fatal(err)
		}
	}
	endpoint(1, 2, "https://"+settings[2].AdvertisedPeer)
	endpoint(2, 1, "https://"+settings[1].AdvertisedPeer)
	// Reopen the inviter between enrollment journeys; the real per-IP listener
	// remains bounded to five requests/minute (each join uses two status proofs).
	stop(0)
	start(0)
	// Additional folder: same B key, independent membership and preexisting root.
	secondRoot := filepath.Join(base, "a-second")
	os.Mkdir(secondRoot, 0700)
	os.WriteFile(filepath.Join(secondRoot, "second-only"), []byte("separate folder bytes"), 0600)
	second := create(secondRoot)
	firstFolder := folder
	folder = second.Join.Folder
	// An existing first-folder member has no access to this unshared folder.
	id, err := replication.LoadOrCreateIdentity(dirs[1], mustID(t, cfgs[1].DeviceID), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	owner, err := replication.LoadOrCreateIdentity(dirs[0], mustID(t, cfgs[0].DeviceID), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	denied, err := replication.NewClient("https://"+settings[0].AdvertisedPeer, id, owner.Leaf, owner.KeyPin)
	if err != nil {
		t.Fatal(err)
	}
	dm := membership(0)
	dd, _ := protocol.MembershipDigest(dm.Membership)
	_, err = denied.Inventory(ctx, replication.InventoryRequest{ProtocolVersion: "1", DeviceID: cfgs[1].DeviceID, FolderID: folder, Revision: "1", MembershipDigest: hex.EncodeToString(dd[:]), Cursor: "0", PageSize: "128"})
	denied.CloseIdleConnections()
	if err == nil {
		t.Fatal("unshared folder disclosed data")
	}
	bSecond := filepath.Join(base, "b-second")
	os.Mkdir(bSecond, 0700)
	os.WriteFile(filepath.Join(bSecond, "local-second"), []byte("reviewed local bytes"), 0600)
	shareMembership := membership(0)
	shareDigest, _ := protocol.MembershipDigest(shareMembership.Membership)
	shareMutation := tc.Mutation{Version: "1", Kind: "share", OperationID: enrollmentRandom(t), Invite: &tc.InviteIntent{Folder: folder, Device: cfgs[1].DeviceID, ExpectedMembership: hex.EncodeToString(shareDigest[:]), ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)}}
	shareFile := filepath.Join(base, "share.json")
	shareBytes, _ := json.Marshal(shareMutation)
	if err = os.WriteFile(shareFile, shareBytes, 0600); err != nil {
		t.Fatal(err)
	}
	var shared tc.Invitation
	for replay := 0; replay < 2; replay++ {
		out, err := exec.Command(binary, "orbit", "folders", "share", "--state", dirs[0], "--request-file", shareFile, "--json").CombinedOutput()
		if err != nil {
			t.Fatalf("share CLI: %v %s", err, out)
		}
		var result tc.Result
		if err = json.Unmarshal(out, &result); err != nil {
			t.Fatal(err)
		}
		if replay == 1 && shared.Capability != result.Invitation.Capability {
			t.Fatal("CLI share replay renewed capability")
		}
		shared = *result.Invitation
	}
	// The earlier scoped attempts remain retained when additional sharing starts.
	rs, err := clients[0].Query(ctx, tc.Query{Version: "1", Kind: "requests"})
	if err != nil || len(rs.Requests) != 2 {
		t.Fatal("first attempts not retained")
	}
	secondJoin := join(1, bSecond, shared)
	if secondJoin.Join.Request == joinedB.Join.Request {
		t.Fatal("same device second-folder collision")
	}
	bytes(filepath.Join(bSecond, "second-only"), "separate folder bytes")
	bytes(filepath.Join(secondRoot, "local-second"), "reviewed local bytes")
	if _, err = os.Stat(filepath.Join(roots[2], "second-only")); !os.IsNotExist(err) {
		t.Fatal("future folder implicitly shared")
	}
	folder = firstFolder
	// Capture on A while C is offline; only B obtains it directly.
	stop(2)
	if err = os.WriteFile(filepath.Join(roots[0], "third-party"), []byte("original author forwarded through B"), 0600); err != nil {
		t.Fatal(err)
	}
	bytes(filepath.Join(roots[1], "third-party"), "original author forwarded through B")
	// Stop A, reopen C: the A-authored envelope/content must now flow via B.
	stop(0)
	start(2)
	bytes(filepath.Join(roots[2], "third-party"), "original author forwarded through B")
	// Both explicit B/C pull directions work while the inviter is offline.
	os.WriteFile(filepath.Join(roots[1], "via-b"), []byte("forwarded without inviter"), 0600)
	bytes(filepath.Join(roots[2], "via-b"), "forwarded without inviter")
	if err = os.WriteFile(filepath.Join(roots[2], "from-c"), []byte("reverse pull with inviter offline"), 0600); err != nil {
		t.Fatal(err)
	}
	bytes(filepath.Join(roots[1], "from-c"), "reverse pull with inviter offline")
	// Change B's path without rekeying/revising. C shows a real offline retry.
	stop(1)
	settings[1].PeerListen = reserve()
	settings[1].AdvertisedPeer = settings[1].PeerListen
	if err = config.SaveRuntimeSettings(dirs[1], settings[1]); err != nil {
		t.Fatal(err)
	}
	start(1)
	wait("offline observation", 10, func() bool {
		var work control.WorkListResult
		if clients[2].Call(ctx, "GET", "/api/v1/work/list?folder="+folder+"&limit=200", nil, &work) != nil {
			return false
		}
		for _, task := range work.Tasks {
			if task.Kind == "sync" && task.Peer != nil && *task.Peer == mustID(t, cfgs[1].DeviceID) && task.ErrorCode != "" {
				return true
			}
		}
		return false
	})
	// CLI refresh omits certificate and reuses C's validated saved B anchor.
	command := exec.Command(binary, "orbit", "devices", "endpoint", "--state", dirs[2], "--folder", folder, "--device", cfgs[1].DeviceID, "--url", "https://"+settings[1].AdvertisedPeer, "--json")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("refresh CLI: %v %s", err, out)
	}
	os.WriteFile(filepath.Join(roots[1], "after-refresh"), []byte("same key new address"), 0600)
	bytes(filepath.Join(roots[2], "after-refresh"), "same key new address")
	for i, p := range processes {
		_ = p
		stop(i)
	}
	var common string
	for i, dir := range dirs {
		cfg, err := config.Load(dir)
		if err != nil || cfg.DeviceID != cfgs[i].DeviceID {
			t.Fatal("identity rotated")
		}
		db, err := repository.Open(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		m, app, err := db.GetMembership(ctx, mustID(t, folder))
		if err != nil || m.Revision != 3 || len(m.Active) != 3 {
			t.Fatal("membership rollout mismatch")
		}
		if i > 0 {
			for _, path := range []string{"original", "third-party", "via-b", "from-c", "after-refresh"} {
				heads, err := db.Heads(ctx, mustID(t, folder), path)
				if err != nil || len(heads) != 1 || db.VerifyManifest(heads[0].Manifest) != nil {
					t.Fatal("verified head mismatch")
				}
				if path == "third-party" && heads[0].ID.Author != mustID(t, cfgs[0].DeviceID) {
					t.Fatal("forwarding changed original author")
				}
				raw, _ := json.Marshal(heads[0])
				if path == "after-refresh" {
					if common == "" {
						common = string(raw)
					} else if common != string(raw) {
						t.Fatal("unequal forwarded head sets")
					}
				}
			}
		}
		if i == 0 {
			_ = app
		}
		if i < 2 {
			heads, err := db.Heads(ctx, mustID(t, second.Join.Folder), "local-second")
			if err != nil || len(heads) != 1 || db.VerifyManifest(heads[0].Manifest) != nil {
				t.Fatal("additional folder metadata missing")
			}
		}
		db.Close()
	}
}
