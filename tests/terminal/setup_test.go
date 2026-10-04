package terminal_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/workspace"
	"golang.org/x/sys/unix"
)

func setupReview(t *testing.T, f *fixture, root, kind string) (tc.SetupIntent, tc.Result) {
	t.Helper()
	s, err := config.LoadRuntimeSettings(f.state)
	if err != nil {
		t.Fatal(err)
	}
	p := tc.SetupIntent{DeviceName: "Laptop", FolderName: "Notes", Root: root, Settings: s}
	q := tc.Query{Version: tc.Version, Kind: "root_preview", Path: root, Name: kind, RootPlan: &p}
	var r tc.Result
	for {
		r, err = f.ctrl.TerminalQuery(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		if r.Preview.Complete {
			break
		}
		q.Cursor = r.Cursor
	}
	p.Preview = *r.Review
	return p, r
}
func TestTerminalT04RecursiveReviewedAdoption(t *testing.T) {
	f := fresh(t)
	root := filepath.Join(f.root, "adopt")
	os.MkdirAll(filepath.Join(root, "sub"), 0700)
	os.WriteFile(filepath.Join(root, "a"), []byte("alpha"), 0600)
	os.WriteFile(filepath.Join(root, "sub", "b"), []byte("beta"), 0700)
	p, prev := setupReview(t, f, root, "adopt")
	if prev.Preview.Files != 2 || prev.Preview.Directories != 1 || prev.Preview.Bytes != 9 || !prev.Preview.CapacityKnown {
		t.Fatalf("incorrect measured preview: %+v", prev.Preview)
	}
	m := tc.Mutation{Version: "1", OperationID: enrollmentRandom(t), Kind: "adopt", Setup: &p}
	r, err := f.ctrl.TerminalMutate(context.Background(), m)
	if err != nil || r.State != "completed" || !r.Readiness.Ready() {
		t.Fatalf("adoption: %v %+v", err, r.Error)
	}
	id := mustID(t, r.Join.Folder)
	heads, err := f.db.Heads(context.Background(), id, "sub/b")
	if err != nil || len(heads) != 1 || heads[0].Manifest == nil || !heads[0].Manifest.Executable {
		t.Fatal("missing captured executable version")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "a")); string(b) != "alpha" {
		t.Fatal("adoption changed bytes")
	}
	replay, err := f.ctrl.TerminalMutate(context.Background(), m)
	if err != nil || replay.Join.Folder != r.Join.Folder {
		t.Fatal("lost response created a new folder")
	}
	changed := m
	copyPlan := *m.Setup
	copyPlan.FolderName = "changed"
	changed.Setup = &copyPlan
	if _, err = f.ctrl.TerminalMutate(context.Background(), changed); err == nil || !strings.Contains(err.Error(), "IDEMPOTENCY_CONFLICT") {
		t.Fatal("changed replay accepted")
	}
}
func TestTerminalT04StaleRootAndPlan(t *testing.T) {
	for _, change := range []string{"bytes", "swap", "symlink", "name", "settings"} {
		t.Run(change, func(t *testing.T) {
			f := fresh(t)
			root := filepath.Join(f.root, "review")
			os.Mkdir(root, 0700)
			os.WriteFile(filepath.Join(root, "a"), []byte("before"), 0600)
			p, _ := setupReview(t, f, root, "setup")
			switch change {
			case "bytes":
				os.WriteFile(filepath.Join(root, "a"), []byte("after!"), 0600)
			case "swap":
				os.Rename(root, root+"-old")
				os.Mkdir(root, 0700)
			case "symlink":
				os.Rename(root, root+"-old")
				os.Symlink(root+"-old", root)
			case "name":
				p.FolderName = "changed"
			case "settings":
				p.Settings.DataBudget++
			}
			_, err := f.ctrl.TerminalMutate(context.Background(), tc.Mutation{Version: "1", OperationID: enrollmentRandom(t), Kind: "setup", Setup: &p})
			if err == nil {
				t.Fatal("stale review accepted")
			}
			regs, _ := f.db.RegisteredFolders(context.Background())
			if len(regs) != 0 {
				t.Fatal("registered an unreviewed root")
			}
		})
	}
}
func TestTerminalT04UnsupportedAndOverlap(t *testing.T) {
	f := fresh(t)
	root := filepath.Join(f.root, "unsupported")
	os.Mkdir(root, 0700)
	os.Symlink("missing", filepath.Join(root, "link"))
	if err := unix.Mkfifo(filepath.Join(root, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	p, r := setupReview(t, f, root, "adopt")
	if r.Preview.Unsupported != 2 || len(r.Preview.Issues) != 2 {
		t.Fatal("unsupported objects omitted")
	}
	if _, err := f.ctrl.TerminalMutate(context.Background(), tc.Mutation{Version: "1", Kind: "adopt", OperationID: enrollmentRandom(t), Setup: &p}); err == nil {
		t.Fatal("incomplete adoption accepted")
	}
	for _, path := range []string{f.state, filepath.Join(f.state, "nested"), f.root} {
		if _, err := f.ctrl.TerminalQuery(context.Background(), tc.Query{Version: "1", Kind: "root_preview", Path: path}); err == nil {
			t.Fatal("state overlap accepted")
		}
	}
}
func TestTerminalT04BoundedPreviewContinuation(t *testing.T) {
	f := fresh(t)
	root := filepath.Join(f.root, "large")
	os.Mkdir(root, 0700)
	file, err := os.Create(filepath.Join(root, "large-file"))
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(34 << 20); err != nil {
		t.Fatal(err)
	}
	file.Close()
	s, _ := config.LoadRuntimeSettings(f.state)
	p := tc.SetupIntent{DeviceName: "Laptop", FolderName: "Large", Root: root, Settings: s}
	q := tc.Query{Version: "1", Kind: "root_preview", Path: root, Name: "adopt", RootPlan: &p}
	r, err := f.ctrl.TerminalQuery(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if r.Preview.Complete || r.Cursor == "" {
		t.Fatal("slice did not bound hashing")
	}
	cursor := r.Cursor
	f.close()
	f.open()
	q.Cursor = cursor
	r, err = f.ctrl.TerminalQuery(context.Background(), q)
	if err != nil || !r.Preview.Complete || r.Preview.Files != 1 || r.Preview.Bytes != 34<<20 {
		t.Fatalf("continuation lost observations: %v", err)
	}
	p.Preview = *r.Review
	_, err = f.ctrl.TerminalMutate(context.Background(), tc.Mutation{Version: "1", Kind: "adopt", OperationID: enrollmentRandom(t), Setup: &p})
	if err != nil {
		t.Fatal(err)
	}
}
func TestTerminalT04CapacityAndExpiredReview(t *testing.T) {
	f := fresh(t)
	root := filepath.Join(f.root, "capacity")
	os.Mkdir(root, 0700)
	os.WriteFile(filepath.Join(root, "keep"), []byte("keep bytes"), 0600)
	s, _ := config.LoadRuntimeSettings(f.state)
	s.DataBudget = 1
	p := tc.SetupIntent{DeviceName: "Laptop", FolderName: "Notes", Root: root, Settings: s}
	r, err := f.ctrl.TerminalQuery(context.Background(), tc.Query{Version: "1", Kind: "root_preview", Path: root, RootPlan: &p})
	if err != nil || r.Error == nil || r.Error.Code != "STORAGE_BLOCKED" {
		t.Fatalf("disk admission: %v", err)
	}
	p.Preview = *r.Review
	if _, err = f.ctrl.TerminalMutate(context.Background(), tc.Mutation{Version: "1", Kind: "setup", OperationID: enrollmentRandom(t), Setup: &p}); err == nil {
		t.Fatal("capacity denial bypassed")
	}
	p, _ = setupReview(t, f, root, "setup")
	ctrl := control.New(f.db, f.ws, control.Options{LocalDevice: f.device, Now: func() time.Time { return time.Now().Add(301 * time.Second) }})
	if _, err = ctrl.TerminalMutate(context.Background(), tc.Mutation{Version: "1", Kind: "setup", OperationID: enrollmentRandom(t), Setup: &p}); err == nil {
		t.Fatal("expired root review accepted")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "keep")); string(b) != "keep bytes" {
		t.Fatal("denial lost working bytes")
	}
}
func TestTerminalT04ResumeAtDurablePhases(t *testing.T) {
	for _, phase := range []string{"reviewed", "bootstrap_capture", "content_pending", "ready"} {
		t.Run(phase, func(t *testing.T) {
			f := fresh(t)
			root := filepath.Join(f.root, "resume")
			os.Mkdir(root, 0700)
			os.WriteFile(filepath.Join(root, "keep"), []byte("retained"), 0600)
			p, _ := setupReview(t, f, root, "setup")
			ctrl := control.New(f.db, f.ws, control.Options{LocalDevice: f.device, FaultHook: func(name string) error {
				if name == "terminal.setup."+phase {
					return errors.New("simulated interrupted boundary")
				}
				return nil
			}})
			m := tc.Mutation{Version: "1", Kind: "setup", OperationID: enrollmentRandom(t), Setup: &p}
			_, err := ctrl.TerminalMutate(context.Background(), m)
			if err == nil {
				t.Fatal("hook did not interrupt")
			}
			identity := f.device
			f.close()
			f.open()
			r, err := f.ctrl.TerminalMutate(context.Background(), m)
			if err != nil || r.State != "completed" || !r.Readiness.Ready() {
				t.Fatalf("resume: %v %+v", err, r.Error)
			}
			if f.device != identity {
				t.Fatal("identity replaced")
			}
			regs, _ := f.db.RegisteredFolders(context.Background())
			if len(regs) != 1 {
				t.Fatal("duplicate registration")
			}
			if b, _ := os.ReadFile(filepath.Join(root, "keep")); string(b) != "retained" {
				t.Fatal("resume changed bytes")
			}
		})
	}
}
func TestTerminalT04InitialCaptureFailureBlocksBootstrap(t *testing.T) {
	f := fresh(t)
	root := filepath.Join(f.root, "scan")
	os.Mkdir(root, 0700)
	os.WriteFile(filepath.Join(root, "keep"), []byte("preserved"), 0600)
	p, _ := setupReview(t, f, root, "setup")
	m := tc.Mutation{Version: "1", Kind: "setup", OperationID: enrollmentRandom(t), Setup: &p}
	interrupted := control.New(f.db, f.ws, control.Options{LocalDevice: f.device, FaultHook: func(name string) error {
		if name == "terminal.setup.bootstrap_capture" {
			os.Symlink("missing", filepath.Join(root, "unsupported"))
			return errors.New("interrupt")
		}
		return nil
	}})
	if _, err := interrupted.TerminalMutate(context.Background(), m); err == nil {
		t.Fatal("hook not reached")
	}
	r, err := f.ctrl.TerminalMutate(context.Background(), m)
	if err != nil || r.State != "blocked" || r.Readiness.Ready() || r.Readiness.Unsupported != 1 {
		t.Fatalf("false ready: %v %+v", err, r.Error)
	}
	reg, _ := f.db.Root(context.Background(), mustID(t, r.Join.Folder))
	if reg.BootstrapComplete {
		t.Fatal("partial bootstrap marked complete")
	}
	os.Remove(filepath.Join(root, "unsupported"))
	r, err = f.ctrl.TerminalMutate(context.Background(), m)
	if err != nil || !r.Readiness.Ready() {
		t.Fatalf("corrected scan did not resume: %v %+v", err, r.Error)
	}
}
func TestTerminalT04FatalScanFailurePreservesCapture(t *testing.T) {
	f := fresh(t)
	root := filepath.Join(f.root, "fatal")
	os.Mkdir(root, 0700)
	os.WriteFile(filepath.Join(root, "keep"), []byte("keep"), 0600)
	p, _ := setupReview(t, f, root, "setup")
	m := tc.Mutation{Version: "1", Kind: "setup", OperationID: enrollmentRandom(t), Setup: &p}
	ctrl := control.New(f.db, f.ws, control.Options{LocalDevice: f.device, FaultHook: func(name string) error {
		if name == "terminal.setup.bootstrap_capture" {
			os.Rename(root, root+"-old")
			return errors.New("interrupt")
		}
		return nil
	}})
	ctrl.TerminalMutate(context.Background(), m)
	r, err := f.ctrl.TerminalMutate(context.Background(), m)
	if err != nil || r.State != "blocked" || r.Readiness.RootAvailable {
		t.Fatalf("unavailable root ready: %v", err)
	}
	os.Rename(root+"-old", root)
	r, err = f.ctrl.TerminalMutate(context.Background(), m)
	if err != nil || !r.Readiness.Ready() {
		t.Fatal("restored root did not resume")
	}
}
func TestTerminalT04NoBootstrapTombstone(t *testing.T) {
	f := fresh(t)
	root := filepath.Join(f.root, "empty")
	p, _ := setupReview(t, f, root, "setup")
	r, err := f.ctrl.TerminalMutate(context.Background(), tc.Mutation{Version: "1", Kind: "setup", OperationID: enrollmentRandom(t), Setup: &p})
	if err != nil || r.State != "completed" {
		t.Fatalf("empty setup %v %+v", err, r.Error)
	}
	reg, err := f.db.Root(context.Background(), mustID(t, r.Join.Folder))
	if err != nil || !reg.BootstrapComplete {
		t.Fatal("empty root not captured")
	}
}

func approveSetupRequest(t *testing.T, f *enrollmentFixture, id string) {
	t.Helper()
	r, err := f.control.Query(context.Background(), tc.Query{Version: "1", Kind: "requests"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range r.Requests {
		if p.ID == id {
			_, err = f.control.Mutate(context.Background(), tc.Mutation{Version: "1", Kind: "approval", OperationID: enrollmentRandom(t), Approval: &tc.ApprovalIntent{Request: p.ID, Folder: p.Folder, Requester: p.Requester, KeyPin: p.KeyPin, TranscriptDigest: p.TranscriptDigest, ExpectedMembership: p.ExpectedMembership, Decision: "approve"}})
			if err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("pending request not found")
}
func TestTerminalT04DelayedJoinRestartAndTransfer(t *testing.T) {
	t.Parallel()
	f := newEnrollmentFixture(t)
	ctx := context.Background()
	ownerRoot := filepath.Join(f.owner.root, "source")
	os.Mkdir(ownerRoot, 0700)
	os.WriteFile(filepath.Join(ownerRoot, "remote-only"), []byte("owner saved bytes"), 0600)
	if _, err := f.owner.ws.Register(ctx, f.folder, ownerRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.ws.Scan(ctx, f.folder); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(f.joiner.root, "join")
	os.Mkdir(root, 0700)
	os.WriteFile(filepath.Join(root, "local-only"), []byte("local saved bytes"), 0600)
	p, _ := setupReview(t, f.joiner, root, "join")
	m := tc.Mutation{Version: "1", Kind: "join", OperationID: enrollmentRandom(t), Join: &tc.JoinIntent{Invitation: f.inv, Attempt: enrollmentRandom(t), DeviceName: p.DeviceName, FolderName: p.FolderName, Root: p.Root, Preview: p.Preview, Settings: p.Settings}}
	r, err := f.joiner.ctrl.TerminalMutate(ctx, m)
	if err != nil || r.Operation.Phase != "awaiting_approval" || r.Readiness.Ready() {
		t.Fatalf("pending join: %v %+v", err, r.Error)
	}
	request, attempt, device := r.Join.Request, r.Join.Attempt, f.joiner.device
	if _, err = f.joiner.db.Membership(ctx, f.folder); err == nil {
		t.Fatal("pending join authorized")
	}
	f.joiner.close()
	f.joiner.open()
	r, err = f.joiner.ctrl.TerminalMutate(ctx, m)
	if err != nil || r.Join.Request != request || r.Join.Attempt != attempt || f.joiner.device != device || r.Operation.Phase != "awaiting_approval" {
		t.Fatalf("pending resume: %v", err)
	}
	approveSetupRequest(t, f, request)
	time.Sleep(25 * time.Second)
	r, err = f.joiner.ctrl.TerminalMutate(ctx, m)
	if err != nil || r.State != "completed" || !r.Readiness.Ready() {
		t.Fatalf("approved join: %v %+v readiness=%+v", err, r.Error, r.Readiness)
	}
	for name, want := range map[string]string{"local-only": "local saved bytes", "remote-only": "owner saved bytes"} {
		b, e := os.ReadFile(filepath.Join(root, name))
		if e != nil || string(b) != want {
			t.Fatalf("lost %s: %v", name, e)
		}
		heads, e := f.joiner.db.Heads(ctx, f.folder, name)
		if e != nil || len(heads) != 1 || heads[0].Kind == history.KindTombstone || f.joiner.db.VerifyManifest(heads[0].Manifest) != nil {
			t.Fatal("head/hash/bootstrap mismatch")
		}
	}
	local, _ := f.joiner.db.Heads(ctx, f.folder, "local-only")
	if local[0].AuthoredRevision != 2 {
		t.Fatal("capture authored before admission")
	}
	safe, _ := json.Marshal(r)
	if strings.Contains(string(safe), f.inv.Capability) {
		t.Fatal("capability leaked in status")
	}
}
func TestTerminalT04DivergentBootstrapRemainsConflict(t *testing.T) {
	t.Parallel()
	f := newEnrollmentFixture(t)
	ctx := context.Background()
	ownerRoot := filepath.Join(f.owner.root, "source")
	os.Mkdir(ownerRoot, 0700)
	os.WriteFile(filepath.Join(ownerRoot, "same"), []byte("owner branch"), 0600)
	f.owner.ws.Register(ctx, f.folder, ownerRoot)
	f.owner.ws.Scan(ctx, f.folder)
	root := filepath.Join(f.joiner.root, "divergent")
	os.Mkdir(root, 0700)
	os.WriteFile(filepath.Join(root, "same"), []byte("local branch"), 0600)
	p, _ := setupReview(t, f.joiner, root, "join")
	m := tc.Mutation{Version: "1", Kind: "join", OperationID: enrollmentRandom(t), Join: &tc.JoinIntent{Invitation: f.inv, Attempt: enrollmentRandom(t), DeviceName: p.DeviceName, FolderName: p.FolderName, Root: p.Root, Preview: p.Preview, Settings: p.Settings}}
	r, err := f.joiner.ctrl.TerminalMutate(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	approveSetupRequest(t, f, r.Join.Request)
	time.Sleep(25 * time.Second)
	r, err = f.joiner.ctrl.TerminalMutate(ctx, m)
	if err != nil || r.State != "blocked" || r.Readiness.Conflicts != 1 || r.Readiness.Ready() {
		t.Fatalf("false conflict readiness: %v %+v", err, r.Error)
	}
	heads, err := f.joiner.db.Heads(ctx, f.folder, "same")
	if err != nil || len(heads) != 2 {
		t.Fatal("divergent histories merged")
	}
	for _, h := range heads {
		if err = f.joiner.db.VerifyManifest(h.Manifest); err != nil {
			t.Fatal(err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(root, "same")); string(b) != "local branch" {
		t.Fatal("overwrote preexisting bytes")
	}
}
func TestTerminalT04ExpiredAttemptDoesNotRenew(t *testing.T) {
	f := newEnrollmentFixture(t)
	root := filepath.Join(f.joiner.root, "expired")
	p, _ := setupReview(t, f.joiner, root, "join")
	inv := f.inv
	inv.ExpiresAt = time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano)
	m := tc.Mutation{Version: "1", Kind: "join", OperationID: enrollmentRandom(t), Join: &tc.JoinIntent{Invitation: inv, Attempt: enrollmentRandom(t), DeviceName: p.DeviceName, FolderName: p.FolderName, Root: p.Root, Preview: p.Preview, Settings: p.Settings}}
	r, err := f.joiner.ctrl.TerminalMutate(context.Background(), m)
	if err != nil || r.Error == nil || r.Error.Code != "EXPIRED_ATTEMPT" {
		t.Fatalf("expiry: %v %+v", err, r.Error)
	}
	replay, err := f.joiner.ctrl.TerminalMutate(context.Background(), m)
	if err != nil || replay.Join.Attempt != m.Join.Attempt || replay.Join.Request != "" {
		t.Fatal("expired retry renewed attempt")
	}
}

func TestTerminalT04UnreadablePreviewAndCaptureFault(t *testing.T) {
	f := fresh(t)
	root := filepath.Join(f.root, "unreadable")
	os.Mkdir(root, 0700)
	sub := filepath.Join(root, "sub")
	os.Mkdir(sub, 0700)
	os.WriteFile(filepath.Join(sub, "keep"), []byte("protected"), 0600)
	os.Chmod(sub, 0000)
	defer os.Chmod(sub, 0700)
	_, r := setupReview(t, f, root, "adopt")
	if r.Preview.Unreadable == 0 {
		t.Fatal("unreadable subtree omitted")
	}
	os.Chmod(sub, 0700)
	p, _ := setupReview(t, f, root, "adopt")
	ws := workspace.New(f.db, workspace.Options{FaultHook: func(name string) error {
		if name == workspace.HookCaptureRead {
			return errors.New("capture read fault")
		}
		return nil
	}})
	ctrl := control.New(f.db, ws, control.Options{LocalDevice: f.device})
	m := tc.Mutation{Version: "1", Kind: "adopt", OperationID: enrollmentRandom(t), Setup: &p}
	r, err := ctrl.TerminalMutate(context.Background(), m)
	if err != nil || r.Error == nil || r.Readiness.Uncaptured == 0 || r.Readiness.Ready() {
		t.Fatalf("failed capture ready: %v", err)
	}
	reg, _ := f.db.Root(context.Background(), mustID(t, r.Join.Folder))
	if reg.BootstrapComplete {
		t.Fatal("failed capture bootstrap completed")
	}
	if b, _ := os.ReadFile(filepath.Join(sub, "keep")); string(b) != "protected" {
		t.Fatal("failed capture lost bytes")
	}
	r, err = f.ctrl.TerminalMutate(context.Background(), m)
	if err != nil || !r.Readiness.Ready() {
		t.Fatal("capture fault not resumable")
	}
}

func TestTerminalT04JoinPhaseRecovery(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"request_prepared", "membership_received", "content_pending", "publishing"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			f := newEnrollmentFixture(t)
			source := filepath.Join(f.owner.root, "source")
			os.Mkdir(source, 0700)
			os.WriteFile(filepath.Join(source, "remote"), []byte("verified remote"), 0600)
			f.owner.ws.Register(ctx, f.folder, source)
			f.owner.ws.Scan(ctx, f.folder)
			root := filepath.Join(f.joiner.root, "root")
			os.Mkdir(root, 0700)
			os.WriteFile(filepath.Join(root, "local"), []byte("captured local"), 0600)
			p, _ := setupReview(t, f.joiner, root, "join")
			m := tc.Mutation{Version: "1", Kind: "join", OperationID: enrollmentRandom(t), Join: &tc.JoinIntent{Invitation: f.inv, Attempt: enrollmentRandom(t), DeviceName: p.DeviceName, FolderName: p.FolderName, Root: p.Root, Preview: p.Preview, Settings: p.Settings}}
			ctrl := control.New(f.joiner.db, f.joiner.ws, control.Options{LocalDevice: f.joiner.device, FaultHook: func(name string) error {
				if name == "terminal.setup."+phase {
					return errors.New("persisted phase interrupted")
				}
				return nil
			}})
			r, err := ctrl.TerminalMutate(ctx, m)
			if phase == "request_prepared" {
				if err == nil {
					t.Fatal("prepared phase not interrupted")
				}
				f.joiner.close()
				f.joiner.open()
				r, err = f.joiner.ctrl.TerminalMutate(ctx, m)
			}
			if err != nil || r.Join.Request == "" {
				t.Fatalf("request: %v", err)
			}
			request := r.Join.Request
			approveSetupRequest(t, f, request)
			time.Sleep(25 * time.Second)
			if phase != "request_prepared" {
				if _, err = ctrl.TerminalMutate(ctx, m); err == nil {
					t.Fatal("phase not interrupted")
				}
			}
			f.joiner.close()
			f.joiner.open()
			r, err = f.joiner.ctrl.TerminalMutate(ctx, m)
			if err != nil || r.State != "completed" || !r.Readiness.Ready() || r.Join.Request != request || r.Join.Attempt != m.Join.Attempt {
				t.Fatalf("phase resume: %v %+v", err, r.Error)
			}
			for name, want := range map[string]string{"remote": "verified remote", "local": "captured local"} {
				if b, _ := os.ReadFile(filepath.Join(root, name)); string(b) != want {
					t.Fatal("phase recovery lost bytes")
				}
			}
		})
	}
}
func TestTerminalT04StartupFailureAndConsumedReview(t *testing.T) {
	f := fresh(t)
	t.Setenv("PATH", filepath.Join(f.root, "no-systemctl"))
	root := filepath.Join(f.root, "startup")
	os.Mkdir(root, 0700)
	os.WriteFile(filepath.Join(root, "keep"), []byte("keep"), 0600)
	s, _ := config.LoadRuntimeSettings(f.state)
	s.Startup = "login"
	p := tc.SetupIntent{DeviceName: "Laptop", FolderName: "Notes", Root: root, Settings: s}
	preview, err := f.ctrl.TerminalQuery(context.Background(), tc.Query{Version: "1", Kind: "root_preview", Path: root, RootPlan: &p})
	if err != nil {
		t.Fatal(err)
	}
	p.Preview = *preview.Review
	m := tc.Mutation{Version: "1", Kind: "setup", OperationID: enrollmentRandom(t), Setup: &p}
	r, err := f.ctrl.TerminalMutate(context.Background(), m)
	if err != nil || r.State != "blocked" || r.Error == nil || r.Error.Code != "SYSTEMD_UNAVAILABLE" {
		t.Fatalf("startup failure: %v %+v", err, r.Error)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "keep")); string(b) != "keep" {
		t.Fatal("startup failure lost bytes")
	}
	m.OperationID = enrollmentRandom(t)
	if _, err = f.ctrl.TerminalMutate(context.Background(), m); err == nil {
		t.Fatal("one review authorized two operations")
	}
}

func TestTerminalT04IssuePagesAndPreviewFamily(t *testing.T) {
	f := fresh(t)
	root := filepath.Join(f.root, "issues")
	os.Mkdir(root, 0700)
	for i := 0; i < 201; i++ {
		if err := os.Symlink("missing", filepath.Join(root, fmt.Sprintf("link-%03d", i))); err != nil {
			t.Fatal(err)
		}
	}
	q := tc.Query{Version: "1", Kind: "root_preview", Path: root, Name: "adopt"}
	r, err := f.ctrl.TerminalQuery(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Preview.Issues) != 200 || r.Preview.Complete || r.Cursor == "" {
		t.Fatal("issue page was not bounded/resumable")
	}
	q.Cursor = r.Cursor
	r, err = f.ctrl.TerminalQuery(context.Background(), q)
	if err != nil || !r.Preview.Complete || len(r.Preview.Issues) != 1 || r.Preview.Unsupported != 201 {
		t.Fatal("issue continuation omitted items")
	}
	other := filepath.Join(f.root, "family")
	p, _ := setupReview(t, f, other, "adopt")
	if _, err = f.ctrl.TerminalMutate(context.Background(), tc.Mutation{Version: "1", Kind: "setup", OperationID: enrollmentRandom(t), Setup: &p}); err == nil {
		t.Fatal("review reused for another family")
	}
}

func TestTerminalT04ExpiredPreparedRequestRecovery(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(fmt.Sprint(accepted), func(t *testing.T) {
			f := newEnrollmentFixture(t)
			ctx := context.Background()
			root := filepath.Join(f.joiner.root, "prepared")
			p, _ := setupReview(t, f.joiner, root, "join")
			m := tc.Mutation{Version: "1", Kind: "join", OperationID: enrollmentRandom(t), Join: &tc.JoinIntent{Invitation: f.inv, Attempt: enrollmentRandom(t), DeviceName: p.DeviceName, FolderName: p.FolderName, Root: p.Root, Preview: p.Preview, Settings: p.Settings}}
			ctrl := control.New(f.joiner.db, f.joiner.ws, control.Options{LocalDevice: f.joiner.device, FaultHook: func(name string) error {
				if name == "terminal.setup.request_prepared" {
					return errors.New("lost acknowledgement")
				}
				return nil
			}})
			if _, err := ctrl.TerminalMutate(ctx, m); err == nil {
				t.Fatal("prepared interruption missing")
			}
			var job map[string]json.RawMessage
			if err := f.joiner.db.TerminalRecord(ctx, "setupjob/"+m.OperationID, &job); err != nil {
				t.Fatal(err)
			}
			var wire protocol.TerminalEnrollmentWire
			if err := tc.Decode(job["wire"], &wire); err != nil {
				t.Fatal(err)
			}
			if accepted {
				if _, err := f.client.Submit(ctx, wire); err != nil {
					t.Fatal(err)
				}
			}
			resume := control.New(f.joiner.db, f.joiner.ws, control.Options{LocalDevice: f.joiner.device, Now: func() time.Time { return time.Now().Add(2 * time.Minute) }})
			r, err := resume.TerminalMutate(ctx, m)
			if err != nil {
				t.Fatal(err)
			}
			if r.Join.Attempt != m.Join.Attempt {
				t.Fatal("expired transcript changed attempt")
			}
			if accepted {
				if r.Operation.Phase != "awaiting_approval" || r.Error != nil {
					t.Fatalf("lost acceptance not recovered: %+v", r.Error)
				}
			} else {
				if r.Error == nil || r.Error.Code != "EXPIRED_ATTEMPT" || r.Error.Retryable {
					t.Fatal("expired unsent transcript silently renewed")
				}
			}
		})
	}
}
