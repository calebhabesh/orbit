package control

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/replication"
	"github.com/calebhabesh/orbit/internal/workspace"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
)

func TestOnboardingE13LeavePreservesFilesHistoryAndStopsWorkAcrossRestart(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(env.rootDir, "keep.txt"), []byte("kept bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := env.ctrl.WorkScan(ctx, WorkScanRequest{Folder: &env.folder}); err != nil {
		t.Fatal(err)
	}
	before, err := env.db.VersionsByAuthor(ctx, env.folder, env.authorA)
	if err != nil {
		t.Fatal(err)
	}
	task, err := env.db.EnqueueDurableTask(ctx, repository.DurableTask{Folder: env.folder, Kind: "scan"})
	if err != nil {
		t.Fatal(err)
	}
	req := LeaveOrbitRequest{Folder: env.folder, ExpectedRoot: env.rootDir + "-wrong"}
	if err := env.ctrl.LeaveOrbit(ctx, req); err == nil {
		t.Fatal("stale root accepted")
	}
	req.ExpectedRoot = env.rootDir
	if err := env.ctrl.LeaveOrbit(ctx, req); err != nil {
		t.Fatal(err)
	}
	if err := env.ctrl.LeaveOrbit(ctx, req); err != nil {
		t.Fatal("replay", err)
	}
	bytes, err := os.ReadFile(filepath.Join(env.rootDir, "keep.txt"))
	if err != nil || string(bytes) != "kept bytes" {
		t.Fatalf("working bytes lost: %q %v", bytes, err)
	}
	after, err := env.db.VersionsByAuthor(ctx, env.folder, env.authorA)
	if err != nil || len(after) != len(before) {
		t.Fatal("history changed", after, err)
	}
	if _, err := env.db.EnqueueDurableTask(ctx, repository.DurableTask{Folder: env.folder, Kind: "sync", Peer: &env.authorB}); !errors.Is(err, repository.ErrFolderLeft) {
		t.Fatal("new sync accepted", err)
	}
	if err := env.db.UpdateDurableTaskState(ctx, task, "queued", 1, "", "", 0); err != nil {
		t.Fatal(err)
	}
	state, err := env.db.GetDurableTask(ctx, task)
	if err != nil || state.State != "canceled" {
		t.Fatal("in-flight completion resurrected work", state, err)
	}
	if err := env.db.RetryDurableTask(ctx, task); !errors.Is(err, repository.ErrFolderLeft) {
		t.Fatal("retry resurrected work", err)
	}
	if _, err := env.ctrl.RegisterFolder(ctx, env.folder, env.rootDir); !errors.Is(err, repository.ErrFolderLeft) {
		t.Fatal("root registration revived left folder", err)
	}
	if err := env.db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := repository.Open(ctx, env.stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if left, err := reopened.FolderLeft(ctx, env.folder); err != nil || !left {
		t.Fatal("Leave lost on restart", left, err)
	}
}

func TestOnboardingE13LeaveCancelsAndDrainsOnlyItsOrbit(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	var other history.ID
	other[0] = 99
	if err := env.db.EnsureFolder(ctx, other, env.authorA, 1); err != nil {
		t.Fatal(err)
	}
	transfer, release, err := env.db.BeginFolderExchange(ctx, env.folder)
	if err != nil {
		t.Fatal(err)
	}
	unrelated, releaseOther, err := env.db.BeginFolderExchange(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseOther()
	done := make(chan error, 1)
	go func() {
		done <- env.ctrl.LeaveOrbit(ctx, LeaveOrbitRequest{Folder: env.folder, ExpectedRoot: env.rootDir})
	}()
	select {
	case <-transfer.Done():
	case <-time.After(time.Second):
		t.Fatal("active transfer not canceled")
	}
	select {
	case err := <-done:
		t.Fatal("Leave returned before transfer drained", err)
	default:
	}
	if unrelated.Err() != nil {
		t.Fatal("other Orbit canceled")
	}
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Leave did not finish")
	}
}

func e13Membership(t *testing.T, env *testEnv) {
	t.Helper()
	m := protocol.Membership{Folder: env.folder, Revision: 1, Active: []protocol.ActiveMember{{Device: env.authorA, KeyPin: testDigest('a')}, {Device: env.authorB, KeyPin: testDigest('b')}}}
	if _, err := env.db.ApproveMembership(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if err := env.db.RenameDevice(context.Background(), env.authorB, "Laptop", env.authorA); err != nil {
		t.Fatal(err)
	}
	env.ctrl.options.LocalDevice = env.authorA
}

func TestOnboardingE13RemovalRequiresNameAndExactReceivedReview(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	e13Membership(t, env)
	e08Remote(t, env, "remote", 1, history.KindDirectory, nil, true)
	preview, err := env.ctrl.PreviewDeviceRetirement(ctx, RetireDevicePreviewRequest{Folder: env.folder, DeviceID: env.authorB})
	if err != nil {
		t.Fatal(err)
	}
	if preview.ReceivedChanges != 1 || !strings.Contains(preview.Disclaimer, "not received") {
		t.Fatal("misleading review", preview)
	}
	req := RemoveDeviceRequest{Folder: env.folder, DeviceID: env.authorB, OperationID: strings.Repeat("1", 64), ConfirmName: "wrong", MembershipDigest: preview.MembershipDigest, SnapshotDigest: preview.SnapshotDigest}
	if _, err := env.ctrl.RemoveDevice(ctx, req); err == nil {
		t.Fatal("wrong name accepted")
	}
	req.ConfirmName = "Laptop"
	e08Remote(t, env, "later", 2, history.KindDirectory, nil, true)
	if _, err := env.ctrl.RemoveDevice(ctx, req); err == nil {
		t.Fatal("stale snapshot accepted")
	}
	preview, err = env.ctrl.PreviewDeviceRetirement(ctx, RetireDevicePreviewRequest{Folder: env.folder, DeviceID: env.authorB})
	if err != nil {
		t.Fatal(err)
	}
	req.SnapshotDigest = preview.SnapshotDigest
	task, err := env.db.EnqueueDurableTask(ctx, repository.DurableTask{Folder: env.folder, Kind: "sync", Peer: &env.authorB})
	if err != nil {
		t.Fatal(err)
	}
	out, err := env.ctrl.RemoveDevice(ctx, req)
	if err != nil || out.State != "completed" || out.ReceivedChanges != 2 {
		t.Fatal(out, err)
	}
	if replay, err := env.ctrl.RemoveDevice(ctx, req); err != nil || replay.State != "completed" {
		t.Fatal("retry", replay, err)
	}
	if retired, err := env.db.IsDeviceRetired(ctx, env.folder, env.authorB); err != nil || !retired {
		t.Fatal(retired, err)
	}
	if _, err := env.db.EnqueueDurableTask(ctx, repository.DurableTask{Folder: env.folder, Kind: "sync", Peer: &env.authorB}); !errors.Is(err, repository.ErrPeerRetired) {
		t.Fatal("removed peer still scheduled", err)
	}
	if err := env.db.UpdateDurableTaskState(ctx, task, "retry", 1, "late response", "IO_ERROR", 0); err != nil {
		t.Fatal(err)
	}
	state, err := env.db.GetDurableTask(ctx, task)
	if err != nil || state.State != "canceled" {
		t.Fatal("late response revived removed peer", state, err)
	}
	if err := env.db.RetryDurableTask(ctx, task); !errors.Is(err, repository.ErrPeerRetired) {
		t.Fatal("retry revived removed peer", err)
	}
}

func TestOnboardingE13PendingRemovalRequiresFreshReviewBeforeContactingSurvivors(t *testing.T) {
	env := setupTestEnv(t)
	if err := os.Chmod(env.stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	env.ctrl.options.LocalDevice = env.authorA
	membership := protocol.Membership{Folder: env.folder, Revision: 1, Active: []protocol.ActiveMember{{Device: env.authorA, KeyPin: testDigest('a')}, {Device: env.authorB, KeyPin: testDigest('b')}, {Device: testID('C'), KeyPin: testDigest('c')}}}
	if _, err := env.db.ApproveMembership(ctx, membership); err != nil {
		t.Fatal(err)
	}
	if err := env.db.RenameDevice(ctx, env.authorB, "Laptop", env.authorA); err != nil {
		t.Fatal(err)
	}
	e08Remote(t, env, "received", 1, history.KindDirectory, nil, true)
	preview, err := env.ctrl.PreviewDeviceRetirement(ctx, RetireDevicePreviewRequest{Folder: env.folder, DeviceID: env.authorB})
	if err != nil {
		t.Fatal(err)
	}
	req := RemoveDeviceRequest{Folder: env.folder, DeviceID: env.authorB, OperationID: strings.Repeat("3", 64), ConfirmName: "Laptop", MembershipDigest: preview.MembershipDigest, SnapshotDigest: preview.SnapshotDigest}
	out, err := env.ctrl.RemoveDevice(ctx, req)
	if err != nil || out.State != "pending" {
		t.Fatal("offline survivor", out, err)
	}
	// New history makes the saved review stale even while a survivor is offline.
	e08Remote(t, env, "arrived-later", 2, history.KindDirectory, nil, true)
	out, err = env.ctrl.RemoveDevice(ctx, req)
	if err != nil || out.State != "needs_review" {
		t.Fatal("stale pending removal must require a fresh review", out, err)
	}
	app, err := env.db.Membership(ctx, env.folder)
	if err != nil || app.Digest != preview.MembershipDigest {
		t.Fatal("stale review advanced membership", app, err)
	}
	if suspended, _, err := env.db.IsCleanupSuspended(ctx, env.folder); err != nil || suspended {
		t.Fatal("aborted review must release cleanup suspension", suspended, err)
	}
	preview, err = env.ctrl.PreviewDeviceRetirement(ctx, RetireDevicePreviewRequest{Folder: env.folder, DeviceID: env.authorB})
	if err != nil || preview.ReceivedChanges != 2 {
		t.Fatal("new review lost received history", preview, err)
	}
	req.OperationID, req.SnapshotDigest = strings.Repeat("4", 64), preview.SnapshotDigest
	out, err = env.ctrl.RemoveDevice(ctx, req)
	if err != nil || out.State != "pending" {
		t.Fatal("aborted operation blocked a fresh review", out, err)
	}
}

func TestOnboardingE13PendingRemovalSurvivesLeaveAndEnrollment(t *testing.T) {
	for _, change := range []string{"leave", "enrollment"} {
		t.Run(change, func(t *testing.T) {
			env := setupTestEnv(t)
			ctx := context.Background()
			e13Membership(t, env)
			current, app, err := env.db.GetMembership(ctx, env.folder)
			if err != nil {
				t.Fatal(err)
			}
			current.Revision++
			current.PriorDigest = app.Digest
			current.Active = append(current.Active, protocol.ActiveMember{Device: testID('C'), KeyPin: testDigest('c')})
			if _, err := env.db.ApproveMembership(ctx, current); err != nil {
				t.Fatal(err)
			}
			preview, err := env.ctrl.PreviewDeviceRetirement(ctx, RetireDevicePreviewRequest{Folder: env.folder, DeviceID: env.authorB})
			if err != nil {
				t.Fatal(err)
			}
			req := RemoveDeviceRequest{Folder: env.folder, DeviceID: env.authorB, OperationID: strings.Repeat("5", 64), ConfirmName: "Laptop", MembershipDigest: preview.MembershipDigest, SnapshotDigest: preview.SnapshotDigest}
			out, err := env.ctrl.RemoveDevice(ctx, req)
			if err != nil || out.State != "pending" {
				t.Fatal(out, err)
			}
			if change == "leave" {
				err = env.ctrl.LeaveOrbit(ctx, LeaveOrbitRequest{Folder: env.folder, ExpectedRoot: env.rootDir})
				var failure *ControlError
				if !errors.As(err, &failure) || failure.Code != "REMOVAL_PENDING" {
					t.Fatal("Leave stranded pending removal", err)
				}
				if left, err := env.db.FolderLeft(ctx, env.folder); err != nil || left {
					t.Fatal("pending removal lost local participation", left, err)
				}
				out, err = env.ctrl.ResumeRemoval(ctx, ResumeRemovalRequest{Folder: env.folder, OperationID: req.OperationID})
				if err != nil || out.State != "pending" {
					t.Fatal("pending operation cannot resume", out, err)
				}
				return
			}
			current, app, err = env.db.GetMembership(ctx, env.folder)
			if err != nil {
				t.Fatal(err)
			}
			current.Revision++
			current.PriorDigest = app.Digest
			current.Active = append(current.Active, protocol.ActiveMember{Device: testID('D'), KeyPin: testDigest('d')})
			if _, err = env.db.ApproveMembership(ctx, current); err != nil {
				t.Fatal(err)
			}
			out, err = env.ctrl.ResumeRemoval(ctx, ResumeRemovalRequest{Folder: env.folder, OperationID: req.OperationID})
			if err != nil || out.State != "needs_review" {
				t.Fatal("enrollment stranded uncommitted removal", out, err)
			}
			if suspended, _, err := env.db.IsCleanupSuspended(ctx, env.folder); err != nil || suspended {
				t.Fatal("aborted removal still suspends cleanup", suspended, err)
			}
			preview, err = env.ctrl.PreviewDeviceRetirement(ctx, RetireDevicePreviewRequest{Folder: env.folder, DeviceID: env.authorB})
			if err != nil {
				t.Fatal(err)
			}
			req.OperationID, req.MembershipDigest, req.SnapshotDigest = strings.Repeat("6", 64), preview.MembershipDigest, preview.SnapshotDigest
			out, err = env.ctrl.RemoveDevice(ctx, req)
			if err != nil || out.State != "pending" {
				t.Fatal("fresh review blocked after enrollment", out, err)
			}
		})
	}
}

func TestOnboardingE13RemovedScreenAndDeduplicatedPeerLeftAttention(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	e13Membership(t, env)
	if err := os.Chmod(env.stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := env.db.EnqueueDurableTask(ctx, repository.DurableTask{Folder: env.folder, Kind: "sync", Peer: &env.authorB, State: "exhausted", ErrorCode: "PEER_LEFT"}); err != nil {
			t.Fatal(err)
		}
	}
	r, err := env.ctrl.terminalAttention(ctx, tc.Query{})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, a := range r.Attention {
		if a.Code == "PEER_LEFT" {
			count++
		}
	}
	if count != 1 {
		t.Fatal("peer-left attention duplicated", count, r.Attention)
	}
	if err := env.db.MarkDeviceRemoved(ctx, env.folder, env.authorB); err != nil {
		t.Fatal(err)
	}
	r, err = env.ctrl.terminalFolderManagement(ctx, tc.Query{Folder: hex.EncodeToString(env.folder[:])})
	if err != nil {
		t.Fatal(err)
	}
	if r.FolderManagement.RemovedBy != "Laptop" {
		t.Fatal("removal attribution missing", r.FolderManagement)
	}
	if err := env.ctrl.ResumeFolder(ctx, env.folder); !errors.Is(err, repository.ErrFolderLeft) {
		t.Fatal("removed Orbit resumed", err)
	}
}

func TestOnboardingE13LegacyFinishedMaintenanceAllowsRemovalAndLeave(t *testing.T) {
	for _, phase := range []string{"completed", "aborted"} {
		t.Run(phase, func(t *testing.T) {
			env := setupTestEnv(t)
			ctx := context.Background()
			e13Membership(t, env)
			save := func() {
				t.Helper()
				if err := env.db.SaveResumableMaintenance(ctx, "legacy", env.folder, env.authorB, phase, []byte(`{}`), time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			save()
			preview, err := env.ctrl.PreviewDeviceRetirement(ctx, RetireDevicePreviewRequest{Folder: env.folder, DeviceID: env.authorB})
			if err != nil {
				t.Fatal(err)
			}
			req := RemoveDeviceRequest{Folder: env.folder, DeviceID: env.authorB, OperationID: strings.Repeat("7", 64), ConfirmName: "Laptop", MembershipDigest: preview.MembershipDigest, SnapshotDigest: preview.SnapshotDigest}
			out, err := env.ctrl.RemoveDevice(ctx, req)
			if err != nil || out.State != "completed" {
				t.Fatal("finished legacy maintenance blocked removal", out, err)
			}
			save()
			if err := env.ctrl.LeaveOrbit(ctx, LeaveOrbitRequest{Folder: env.folder, ExpectedRoot: env.rootDir}); err != nil {
				t.Fatal("finished legacy maintenance blocked Leave", err)
			}
		})
	}
}

func TestOnboardingE13ThreeDevicesOfflineDivergentAndInterruptedRollout(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	if err := os.Chmod(env.stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	a, err := replication.LoadOrCreateIdentity(env.stateDir, env.authorA, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	bdir := t.TempDir()
	b, err := replication.LoadOrCreateIdentity(bdir, env.authorB, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	bdb, err := repository.Open(ctx, bdir)
	if err != nil {
		t.Fatal(err)
	}
	defer bdb.Close()
	var target history.ID
	target[0] = 67
	m := protocol.Membership{Folder: env.folder, Revision: 1, Active: []protocol.ActiveMember{{Device: a.DeviceID, KeyPin: a.KeyPin}, {Device: b.DeviceID, KeyPin: b.KeyPin}, {Device: target, KeyPin: testDigest('c')}}}
	if err = bdb.EnsureFolder(ctx, env.folder, b.DeviceID, 1); err != nil {
		t.Fatal(err)
	}
	for _, db := range []*repository.DB{env.db, bdb} {
		if _, err = db.ApproveMembership(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	env.ctrl.options.LocalDevice = a.DeviceID
	if err = env.db.RenameDevice(ctx, target, "Pi", a.DeviceID); err != nil {
		t.Fatal(err)
	}
	if err = env.db.RenameDevice(ctx, b.DeviceID, "Laptop", a.DeviceID); err != nil {
		t.Fatal(err)
	}
	e := history.Envelope{ID: history.VersionID{Folder: env.folder, Author: target, Counter: 1}, Path: "pi-only", Kind: history.KindDirectory, Vector: []history.ClockEntry{{Author: target, Counter: 1}}, AuthoredRevision: 1, DisplayTime: time.Now().UTC().Format(time.RFC3339Nano)}
	if err = env.db.ImportMetadata(ctx, e); err != nil {
		t.Fatal(err)
	}
	preview, err := env.ctrl.PreviewDeviceRetirement(ctx, RetireDevicePreviewRequest{Folder: env.folder, DeviceID: target})
	if err != nil {
		t.Fatal(err)
	}
	req := RemoveDeviceRequest{Folder: env.folder, DeviceID: target, OperationID: strings.Repeat("a", 64), ConfirmName: "Pi", MembershipDigest: preview.MembershipDigest, SnapshotDigest: preview.SnapshotDigest}
	// Missing/offline survivor cannot cause a unilateral membership change.
	out, err := env.ctrl.RemoveDevice(ctx, req)
	if err != nil || out.State != "pending" {
		t.Fatal("offline", out, err)
	}
	app, _ := env.db.Membership(ctx, env.folder)
	if app.Digest != preview.MembershipDigest {
		t.Fatal("offline survivor was skipped")
	}
	if suspended, _, err := env.db.IsCleanupSuspended(ctx, env.folder); err != nil || !suspended {
		t.Fatal("pending removal must suspend cleanup", suspended, err)
	}
	var dropCommit atomic.Bool
	dropCommit.Store(true)
	bserver := replication.NewServer(bdb, b)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/peer/v1/membership/retirement" {
			var wire replication.RetirementRequest
			if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(wire)
			r.Body = io.NopCloser(bytes.NewReader(raw))
			if dropCommit.Load() && wire.Action == "commit" {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		}
		bserver.ServeHTTP(w, r)
	}))
	server.TLS = b.ServerTLSConfig()
	server.StartTLS()
	defer server.Close()
	certPath := filepath.Join(env.stateDir, "b.crt")
	if err = os.WriteFile(certPath, b.CertificatePEM(), 0600); err != nil {
		t.Fatal(err)
	}
	if err = config.SetPeerEndpoint(env.stateDir, config.PeerEndpoint{Folder: fmt.Sprintf("%x", env.folder[:]), Device: fmt.Sprintf("%x", b.DeviceID[:]), URL: server.URL, Certificate: certPath}); err != nil {
		t.Fatal(err)
	}
	out, err = env.ctrl.RemoveDevice(ctx, req)
	if err != nil || out.State != "pending" {
		t.Fatal("divergent", out, err)
	}
	app, _ = env.db.Membership(ctx, env.folder)
	if app.Digest != preview.MembershipDigest {
		t.Fatal("different survivor snapshot ignored")
	}
	if err = bdb.ImportMetadata(ctx, e); err != nil {
		t.Fatal(err)
	}
	// Local commit succeeds only after every survivor prepares; remote response
	// loss leaves an exact operation that survives a controller restart.
	out, err = env.ctrl.RemoveDevice(ctx, req)
	if err != nil || out.State != "pending" {
		t.Fatal("interrupted rollout", out, err)
	}
	app, _ = env.db.Membership(ctx, env.folder)
	if app.Revision != 2 {
		t.Fatal("local retirement absent", app, out)
	}
	remote, _ := bdb.Membership(ctx, env.folder)
	if remote.Revision != 1 {
		t.Fatal("fault fixture did not interrupt remote commit")
	}
	aserver := httptest.NewUnstartedServer(replication.NewServer(env.db, a))
	aserver.TLS = a.ServerTLSConfig()
	aserver.StartTLS()
	defer aserver.Close()
	bc, err := replication.NewClient(aserver.URL, b, a.Leaf, a.KeyPin)
	if err != nil {
		t.Fatal(err)
	}
	defer bc.CloseIdleConnections()
	if _, err = replication.NewSyncer(bdb, nil, bc, b.DeviceID, a.DeviceID, env.folder, remote, replication.TransferOptions{}).Sync(ctx); err != nil {
		t.Fatal("prepared survivor could not catch up", err)
	}
	remote, _ = bdb.Membership(ctx, env.folder)
	if remote.Digest != app.Digest {
		t.Fatal("prepared catchup disagrees")
	}
	dropCommit.Store(false)
	resumed := New(env.db, workspace.New(env.db, workspace.Options{}), Options{LocalDevice: a.DeviceID})
	out, err = resumed.ResumeRemoval(ctx, ResumeRemovalRequest{Folder: req.Folder, OperationID: req.OperationID})
	if err != nil || out.State != "completed" {
		t.Fatal("resume", out, err)
	}
	remote, _ = bdb.Membership(ctx, env.folder)
	if remote.Digest != app.Digest {
		t.Fatal("survivors disagree after resume")
	}
	if suspended, _, err := env.db.IsCleanupSuspended(ctx, env.folder); err != nil || suspended {
		t.Fatal("completed removal must release cleanup suspension", suspended, err)
	}
	// Every accepted retired-author record is still present.
	for _, db := range []*repository.DB{env.db, bdb} {
		versions, e := db.VersionsByAuthor(ctx, env.folder, target)
		if e != nil || len(versions) != 1 {
			t.Fatal("retired history lost", versions, e)
		}
	}
}
