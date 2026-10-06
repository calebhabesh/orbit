package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/app"
	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/controlclient"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

// setupNode creates an initialized Orbit test node in a disposable directory.
func setupNode(t *testing.T, label string, withEnrollment ...bool) (*control.Controller, *control.Server, *httptest.Server, *repository.DB, string, history.ID, func()) {
	t.Helper()
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state-"+label)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}

	rawDev := make([]byte, 32)
	rand.Read(rawDev)
	var localDevice history.ID
	copy(localDevice[:], rawDev)

	coreCfg := config.Config{
		FormatVersion: 1,
		DeviceID:      hex.EncodeToString(rawDev),
		CreatedAt:     time.Now().UTC(),
	}
	if err := config.Save(stateDir, coreCfg); err != nil {
		t.Fatal(err)
	}

	if _, err := replication.LoadOrCreateIdentity(stateDir, localDevice, time.Now()); err != nil {
		t.Fatal(err)
	}

	st := config.ProductSettings{
		FormatVersion: 1,
		DeviceLabel:   label,
	}
	if err := config.SaveSettings(stateDir, st); err != nil {
		t.Fatal(err)
	}

	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}

	ws := workspace.New(db, workspace.Options{})
	ctrl := control.New(db, ws, control.Options{
		LocalDevice: localDevice,
	})

	srv, err := control.NewServer(ctrl, stateDir)
	if err != nil {
		t.Fatal(err)
	}

	httpSrv := httptest.NewTLSServer(srv.Handler())
	var networkCancel context.CancelFunc
	var networkDone chan error
	if len(withEnrollment) > 0 && withEnrollment[0] {
		addresses, err := net.InterfaceAddrs()
		if err != nil {
			t.Fatal(err)
		}
		ip := ""
		for _, a := range addresses {
			candidate, _, err := net.ParseCIDR(a.String())
			if err == nil && candidate.To4() != nil && !candidate.IsLoopback() {
				ip = candidate.String()
				break
			}
		}
		if ip == "" {
			t.Fatal("enrollment fixture requires nonloopback IPv4")
		}
		enrollment, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
		if err != nil {
			t.Fatal(err)
		}
		peer, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
		if err != nil {
			enrollment.Close()
			t.Fatal(err)
		}
		identity, err := replication.LoadOrCreateIdentity(stateDir, localDevice, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		settings := config.DefaultRuntimeSettings()
		settings.EnrollmentListen = enrollment.Addr().String()
		settings.AdvertisedEnrollment = settings.EnrollmentListen
		settings.PeerListen = peer.Addr().String()
		settings.AdvertisedPeer = settings.PeerListen
		if err := config.SaveRuntimeSettings(stateDir, settings); err != nil {
			t.Fatal(err)
		}
		networkCtx, cancelNetwork := context.WithCancel(ctx)
		networkCancel = cancelNetwork
		networkDone = make(chan error, 2)
		go func() { networkDone <- replication.NewEnrollmentServer(db, identity).Serve(networkCtx, enrollment) }()
		go func() { networkDone <- replication.NewServer(db, identity).Serve(networkCtx, peer) }()

	}

	cleanup := func() {
		if networkCancel != nil {
			networkCancel()
			for i := 0; i < 2; i++ {
				if err := <-networkDone; err != nil {
					t.Error(err)
				}
			}
		}
		httpSrv.Close()
		db.Close()
	}

	return ctrl, srv, httpSrv, db, stateDir, localDevice, cleanup
}

func TestOrbitPairing_FullJoinFlowLifecycle(t *testing.T) {
	ctx := context.Background()

	// 1. Setup Node A (Owner)
	ctrlA, _, httpSrvA, dbA, stateDirA, devA, cleanupA := setupNode(t, "Owner-PC", true)
	defer cleanupA()

	var folderID history.ID
	rand.Read(folderID[:])
	if err := dbA.EnsureFolder(ctx, folderID, devA, 1); err != nil {
		t.Fatal(err)
	}

	identityA, e := replication.LoadOrCreateIdentity(dbA.StateDir(), devA, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	pinA := identityA.KeyPin
	if _, err := dbA.ApproveMembership(ctx, protocol.Membership{
		Folder:      folderID,
		Revision:    1,
		PriorDigest: history.Digest{},
		Active:      []protocol.ActiveMember{{Device: devA, KeyPin: pinA}},
	}); err != nil {
		t.Fatal(err)
	}

	// 2. Setup Node B (Joining Device)
	ctrlB, _, _, dbB, stateDirB, devB, cleanupB := setupNode(t, "Joining-Laptop", true)
	defer cleanupB()
	_ = dbB

	// Disposable sync root for Node B with preexisting files
	rootB := filepath.Join(testkit.NewDisposable(t), "sync-b")
	if err := os.MkdirAll(rootB, 0o700); err != nil {
		t.Fatal(err)
	}
	preexistingFile := filepath.Join(rootB, "preexisting.txt")
	if err := os.WriteFile(preexistingFile, []byte("preserve this file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// 3. Node A creates invitation
	invRes, err := ctrlA.CreateInvitation(ctx, control.CreateInvitationRequest{
		Folder:   folderID,
		TTLSecs:  3600,
		MaxUses:  1,
		Endpoint: httpSrvA.URL,
	})
	if err != nil {
		t.Fatalf("CreateInvitation failed: %v", err)
	}

	if invRes.Token == "" {
		t.Fatal("expected non-empty invitation token")
	}
	if !strings.HasPrefix(invRes.InvitationCode, "orbit-invitation:v2:") {
		t.Fatalf("expected formatted invitation code, got: %s", invRes.InvitationCode)
	}

	// Invariant I23: Token must not be stored in SQLite in plaintext
	backup := filepath.Join(t.TempDir(), "invitation-inspection.sqlite")
	if err := dbA.Backup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	rawDB, err := sql.Open("sqlite", backup)
	if err != nil {
		t.Fatal(err)
	}
	defer rawDB.Close()
	var rawMatchCount int
	if err := rawDB.QueryRowContext(ctx, "SELECT count(*) FROM invitations WHERE digest=?", []byte(invRes.Token)).Scan(&rawMatchCount); err != nil {
		t.Fatal(err)
	}
	if rawMatchCount > 0 {
		t.Fatal("Invariant I23 violation: plain text token found in invitations table")
	}

	// 4. Node B previews root path
	previewRes, err := ctrlB.PreviewCreateRoot(ctx, control.PreviewCreateRootRequest{
		Path: rootB,
	})
	if err != nil {
		t.Fatalf("PreviewCreateRoot failed: %v", err)
	}
	if previewRes.Disallowed {
		t.Fatalf("rootB was unexpectedly disallowed: %s", previewRes.Reason)
	}
	if previewRes.PreexistingRows < 1 {
		t.Fatalf("expected preexisting rows to be detected, got %d", previewRes.PreexistingRows)
	}

	invitation := decodePairingInvitation(t, invRes.InvitationCode)
	// 5. Node B submits join request to Node A
	submitRes, err := ctrlB.SubmitJoinFlow(ctx, control.JoinFlowSubmitRequest{
		Invitation:      &invitation,
		InvitationToken: invRes.Token,
		TargetFolder:    hex.EncodeToString(folderID[:]),
		RemoteEndpoint:  invitation.EnrollmentEndpoint,
		DeviceLabel:     "Laptop-B",
		RootPath:        rootB,
	})
	if err != nil {
		t.Fatalf("SubmitJoinFlow failed: %v", err)
	}

	if submitRes.RequestID == "" {
		t.Fatal("expected non-empty RequestID")
	}
	if submitRes.Status != "pending" {
		t.Fatalf("expected status 'pending', got %q", submitRes.Status)
	}

	// 6. Node A lists enrollment requests
	reqs, err := ctrlA.ListEnrollmentRequests(ctx, folderID, "pending")
	if err != nil {
		t.Fatalf("ListEnrollmentRequests failed: %v", err)
	}
	if len(reqs.Requests) != 1 {
		t.Fatalf("expected 1 pending request on Node A, got %d", len(reqs.Requests))
	}
	pendingReq := reqs.Requests[0]
	if pendingReq.RequestID != submitRes.RequestID {
		t.Fatalf("mismatched request ID: %s vs %s", pendingReq.RequestID, submitRes.RequestID)
	}
	if pendingReq.DeviceID != devB {
		t.Fatalf("mismatched device ID: %x vs %x", pendingReq.DeviceID, devB)
	}

	// 7. Node B polls status before approval -> pending
	statusPre, err := ctrlA.GetEnrollmentStatus(ctx, submitRes.RequestID)
	if err != nil {
		t.Fatalf("GetEnrollmentStatus failed: %v", err)
	}
	if statusPre.Status != "pending" {
		t.Fatalf("expected pending status, got %s", statusPre.Status)
	}

	// 8. Node A approves enrollment request with custom alias and endpoint
	reviewed := pairingReview(t, ctrlA, submitRes.RequestID, "approve")
	joiningSettings, e := config.LoadRuntimeSettings(stateDirB)
	if e != nil {
		t.Fatal(e)
	}
	approveRes, err := ctrlA.ApproveEnrollmentRequest(ctx, control.ApproveEnrollmentRequest{
		Reviewed: &reviewed, OperationID: pairingOperation(t),
		RequestID:      submitRes.RequestID,
		Folder:         folderID,
		SuggestedLabel: "Custom Laptop B",
		Endpoint:       "https://" + joiningSettings.AdvertisedPeer,
	})
	if err != nil {
		t.Fatalf("ApproveEnrollmentRequest failed: %v", err)
	}
	if approveRes.Revision != 2 {
		t.Fatalf("expected Revision 2 after approval, got %d", approveRes.Revision)
	}

	// 9. Node B polls status after approval -> approved
	statusPost, err := ctrlA.GetEnrollmentStatus(ctx, submitRes.RequestID)
	if err != nil {
		t.Fatalf("GetEnrollmentStatus post-approval failed: %v", err)
	}
	if statusPost.Status != "approved" {
		t.Fatalf("expected approved status, got %s", statusPost.Status)
	}

	// 10. Node B completes join flow
	time.Sleep(25 * time.Second)
	completeRes, err := ctrlB.CompleteJoinFlow(ctx, control.JoinFlowCompleteRequest{
		RequestID:      submitRes.RequestID,
		RemoteEndpoint: invitation.EnrollmentEndpoint,
		TargetFolder:   hex.EncodeToString(folderID[:]),
		RootPath:       rootB,
		DeviceLabel:    "Custom Laptop B",
	})
	if err != nil {
		t.Fatalf("CompleteJoinFlow failed: %v", err)
	}
	if !completeRes.Completed {
		t.Fatal("expected CompleteJoinFlow.Completed to be true")
	}

	// 11. Invariant I22: Preexisting file on Node B root was preserved without deletion
	content, err := os.ReadFile(preexistingFile)
	if err != nil {
		t.Fatalf("preexisting file was deleted or lost: %v", err)
	}
	if string(content) != "preserve this file\n" {
		t.Fatalf("preexisting file content corrupted: %s", string(content))
	}

	// 12. Verify Node B now has the folder in its database with Revision 2
	peersB, err := ctrlB.PeerList(ctx, folderID)
	if err != nil {
		t.Fatalf("PeerList on Node B failed: %v", err)
	}
	if peersB.Revision != 2 {
		t.Fatalf("Node B has revision %d, want 2", peersB.Revision)
	}
	if len(peersB.Active) != 2 {
		t.Fatalf("Node B sees %d active members, want 2", len(peersB.Active))
	}

	// 13. Verify Node B saved peer endpoint for Node A
	endpointsB, err := config.LoadPeerEndpoints(stateDirB)
	if err != nil {
		t.Fatal(err)
	}
	var foundOwnerEndpoint bool
	for _, ep := range endpointsB {
		if ep.Device == hex.EncodeToString(devA[:]) && ep.URL == invitation.PeerEndpoint {
			foundOwnerEndpoint = true
			break
		}
	}
	if !foundOwnerEndpoint {
		t.Fatalf("Node B did not configure endpoint for Node A: %+v", endpointsB)
	}

	// 14. Verify Node A configured peer endpoint for Node B
	endpointsA, err := config.LoadPeerEndpoints(stateDirA)
	if err != nil {
		t.Fatal(err)
	}
	var foundJoiningEndpoint bool
	for _, ep := range endpointsA {
		if ep.Device == hex.EncodeToString(devB[:]) && ep.URL == "https://"+joiningSettings.AdvertisedPeer {
			foundJoiningEndpoint = true
			break
		}
	}
	if !foundJoiningEndpoint {
		t.Fatalf("Node A did not configure endpoint for Node B: %+v", endpointsA)
	}

	// 15. Verify Node A saved display name alias for Node B
	aliasB, err := dbA.GetDeviceDisplayName(ctx, devB)
	if err != nil {
		t.Fatal(err)
	}
	if aliasB != "Custom Laptop B" {
		t.Fatalf("Node A alias for devB = %q, want 'Custom Laptop B'", aliasB)
	}
}

func TestOrbitPairing_DeclineJoinFlow(t *testing.T) {
	ctx := context.Background()

	// 1. Setup Node A and Node B
	ctrlA, _, httpSrvA, dbA, _, devA, cleanupA := setupNode(t, "Owner-PC", true)
	defer cleanupA()

	var folderID history.ID
	rand.Read(folderID[:])
	_ = dbA.EnsureFolder(ctx, folderID, devA, 1)
	identityA, e := replication.LoadOrCreateIdentity(dbA.StateDir(), devA, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	pinA := identityA.KeyPin
	_, _ = dbA.ApproveMembership(ctx, protocol.Membership{
		Folder:      folderID,
		Revision:    1,
		PriorDigest: history.Digest{},
		Active:      []protocol.ActiveMember{{Device: devA, KeyPin: pinA}},
	})

	ctrlB, _, _, _, _, _, cleanupB := setupNode(t, "Joining-Laptop", true)
	defer cleanupB()

	rootB := filepath.Join(testkit.NewDisposable(t), "sync-b")
	_ = os.MkdirAll(rootB, 0o700)

	// 2. Node A creates invitation
	invRes, err := ctrlA.CreateInvitation(ctx, control.CreateInvitationRequest{
		Folder:   folderID,
		TTLSecs:  3600,
		MaxUses:  1,
		Endpoint: httpSrvA.URL,
	})
	if err != nil {
		t.Fatal(err)
	}

	invitation := decodePairingInvitation(t, invRes.InvitationCode)
	// 3. Node B submits join request
	submitRes, err := ctrlB.SubmitJoinFlow(ctx, control.JoinFlowSubmitRequest{
		Invitation:      &invitation,
		InvitationToken: invRes.Token,
		TargetFolder:    hex.EncodeToString(folderID[:]),
		RemoteEndpoint:  invitation.EnrollmentEndpoint,
		DeviceLabel:     "Laptop-B",
		RootPath:        rootB,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 4. Node A declines join request
	reviewed := pairingReview(t, ctrlA, submitRes.RequestID, "decline")
	err = ctrlA.DeclineEnrollmentRequest(ctx, control.DeclineEnrollmentRequest{
		Reviewed: &reviewed, OperationID: pairingOperation(t),
		RequestID: submitRes.RequestID,
	})
	if err != nil {
		t.Fatalf("DeclineEnrollmentRequest failed: %v", err)
	}

	// 5. Node B checks status -> declined
	status, err := ctrlA.GetEnrollmentStatus(ctx, submitRes.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "declined" {
		t.Fatalf("expected status declined on polling, got %s", status.Status)
	}
}

func TestOrbitPairing_DeviceDetailAndRename(t *testing.T) {
	ctx := context.Background()

	ctrl, _, httpSrv, db, _, dev, cleanup := setupNode(t, "Initial-PC")
	defer cleanup()

	var folderID history.ID
	rand.Read(folderID[:])
	_ = db.EnsureFolder(ctx, folderID, dev, 1)
	pin := sha256.Sum256(dev[:])
	if _, err := db.ApproveMembership(ctx, protocol.Membership{
		Folder:      folderID,
		Revision:    1,
		PriorDigest: history.Digest{},
		Active:      []protocol.ActiveMember{{Device: dev, KeyPin: pin}},
	}); err != nil {
		t.Fatal(err)
	}

	// 1. Rename device
	renameRes, err := ctrl.RenameDevice(ctx, control.RenameDeviceRequest{
		DeviceID: dev,
		Alias:    "Master Server",
	})
	if err != nil {
		t.Fatalf("RenameDevice failed: %v", err)
	}
	if renameRes.Alias != "Master Server" {
		t.Fatalf("expected 'Master Server', got %q", renameRes.Alias)
	}

	// 2. PeerList returns Aliases map containing the updated alias
	peers, err := ctrl.PeerList(ctx, folderID)
	if err != nil {
		t.Fatal(err)
	}
	if peers.Aliases[hex.EncodeToString(dev[:])] != "Master Server" {
		t.Fatalf("expected peer alias 'Master Server', got %q", peers.Aliases[hex.EncodeToString(dev[:])])
	}

	// 3. Test peer reachability endpoint
	testRes, err := ctrl.TestPeerReachability(ctx, httpSrv.URL)
	if err != nil {
		t.Fatalf("TestPeerReachability failed: %v", err)
	}
	if !testRes.Reachable {
		t.Fatalf("expected endpoint %s to be reachable", httpSrv.URL)
	}
	if testRes.LatencyMS < 0 {
		t.Fatalf("expected non-negative latency, got %d", testRes.LatencyMS)
	}

	// 4. Test unreachable endpoint
	badRes, err := ctrl.TestPeerReachability(ctx, "https://127.0.0.1:19999")
	if err != nil {
		t.Fatalf("expected soft error result, got hard error: %v", err)
	}
	if badRes.Reachable {
		t.Fatal("expected port 19999 to be unreachable")
	}

	// 5. Test retirement preview
	preview, err := ctrl.PreviewDeviceRetirement(ctx, control.RetireDevicePreviewRequest{
		Folder:   folderID,
		DeviceID: dev,
	})
	if err != nil {
		t.Fatalf("PreviewDeviceRetirement failed: %v", err)
	}
	if preview.DeviceName != "Master Server" {
		t.Fatalf("expected device name 'Master Server', got %q", preview.DeviceName)
	}
	if !strings.Contains(preview.Disclaimer, "remotely erase") && !strings.Contains(preview.Disclaimer, "physical disk") {
		t.Fatalf("expected disclaimer mentioning remote disk erasure limit, got: %s", preview.Disclaimer)
	}
}

func reviewedCLISetup(t *testing.T, command func(...string) *exec.Cmd, disposable string, args ...string) *exec.Cmd {
	t.Helper()
	request := filepath.Join(disposable, "setup-request-"+strings.ReplaceAll(filepath.Base(args[1]), "/", "-")+".json")
	previewArgs := append([]string{"setup", "--preview", "--review-file", request}, args...)
	if out, err := command(previewArgs...).CombinedOutput(); err != nil {
		t.Fatalf("review setup: %v %s", err, out)
	}
	stateDir := ""
	for i, a := range args {
		if a == "--state" && i+1 < len(args) {
			stateDir = args[i+1]
		}
	}
	t.Cleanup(func() {
		if err := testkit.ValidateDestructiveTarget(disposable, stateDir); err != nil {
			t.Error(err)
			return
		}
		if _, err := os.Stat(filepath.Join(stateDir, ".agent.pid")); os.IsNotExist(err) {
			return
		}
		if err := app.StopAgent(stateDir, 5*time.Second); err != nil {
			t.Error(err)
		}
	})
	return command("setup", "--state", stateDir, "--request-file", request)
}

func TestOrbitPairing_CLI_Parity(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state-cli")
	_ = os.MkdirAll(stateDir, 0o700)
	rootPath := filepath.Join(disposable, "orbit-root")
	_ = os.MkdirAll(rootPath, 0o700)

	binPath := filepath.Join("..", "..", "bin", "orbit")
	if _, err := os.Stat(binPath); err != nil {
		binPath = filepath.Join("..", "..", "bin", "filesync")
		if _, err := os.Stat(binPath); err != nil {
			t.Skip("orbit binary not found in bin/; run 'make build' first")
		}
	}

	orbitCmd := func(subArgs ...string) *exec.Cmd {
		if filepath.Base(binPath) == "orbit" {
			return exec.Command(binPath, subArgs...)
		}
		return exec.Command(binPath, append([]string{"orbit"}, subArgs...)...)
	}

	// 1. orbit setup
	if err := os.Chmod(disposable, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := reviewedCLISetup(t, orbitCmd, disposable, "--state", stateDir, "--root", rootPath, "--label", "CLI-Node", "--name", "TestCLI", "--connection", "manual")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orbit setup failed: %v\nOutput: %s", err, string(out))
	}

	// 2. orbit invite create
	cmd = orbitCmd("invite", "create", "--state", stateDir, "--ttl", "1800", "--uses", "2")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orbit invite create failed: %v\nOutput: %s", err, string(out))
	}
	outStr := string(out)
	if !strings.Contains(outStr, "orbit-invitation:v1?") {
		t.Fatalf("expected orbit-invitation:v1 link in output, got: %s", outStr)
	}

	// Extract digest hex
	var digestHex string
	for _, line := range strings.Split(outStr, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Invitation Digest:") {
			digestHex = strings.TrimSpace(strings.TrimPrefix(trimmed, "Invitation Digest:"))
		}
	}
	if digestHex == "" {
		t.Fatalf("could not parse invitation digest from output: %s", outStr)
	}

	// 3. filesync orbit invite list
	// 3. orbit invite list
	cmd = orbitCmd("invite", "list", "--state", stateDir)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orbit invite list failed: %v\nOutput: %s", err, string(out))
	}
	if !strings.Contains(string(out), digestHex[:16]) {
		t.Fatalf("expected invite list to contain digest, got: %s", string(out))
	}

	// 4. orbit devices list
	cmd = orbitCmd("devices", "list", "--state", stateDir)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orbit devices list failed: %v\nOutput: %s", err, string(out))
	}
	if !strings.Contains(string(out), "CLI-Node") {
		t.Fatalf("expected devices list to show 'CLI-Node', got: %s", string(out))
	}

	// 5. orbit invite revoke
	cmd = orbitCmd("invite", "revoke", "--state", stateDir, "--digest", digestHex)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("orbit invite revoke failed: %v\nOutput: %s", err, string(out))
	}
	if !strings.Contains(string(out), "Invitation revoked") {
		t.Fatalf("expected revocation confirmation, got: %s", string(out))
	}
}

func TestOrbitPairing_CLI_RunningDaemon_Parity(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	if err := os.Chmod(disposable, 0700); err != nil {
		t.Fatal(err)
	}
	stateA := filepath.Join(disposable, "state-a")
	rootA := filepath.Join(disposable, "root-a")
	stateB := filepath.Join(disposable, "state-b")
	rootB := filepath.Join(disposable, "root-b")

	_ = os.MkdirAll(stateA, 0o700)
	_ = os.MkdirAll(rootA, 0o700)
	_ = os.MkdirAll(stateB, 0o700)
	_ = os.MkdirAll(rootB, 0o700)

	binPath := filepath.Join("..", "..", "bin", "orbit")
	if _, err := os.Stat(binPath); err != nil {
		binPath = filepath.Join("..", "..", "bin", "filesync")
		if _, err := os.Stat(binPath); err != nil {
			t.Skip("orbit binary not found in bin/; run 'make build' first")
		}
	}

	orbitCmd := func(subArgs ...string) *exec.Cmd {
		if filepath.Base(binPath) == "orbit" {
			return exec.Command(binPath, subArgs...)
		}
		return exec.Command(binPath, append([]string{"orbit"}, subArgs...)...)
	}

	// 1. orbit setup on node A
	cmd := reviewedCLISetup(t, orbitCmd, disposable, "--state", stateA, "--root", rootA, "--label", "Desktop-Workstation", "--name", "Lab-Workspace", "--connection", "manual")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("setup A failed: %v: %s", err, string(out))
	}

	if err := app.StopAgent(stateA, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	// Configure reviewed nonloopback peer/enrollment addresses before launch.
	interfaces, e := net.InterfaceAddrs()
	if e != nil {
		t.Fatal(e)
	}
	ip := ""
	for _, a := range interfaces {
		candidate, _, e := net.ParseCIDR(a.String())
		if e == nil && candidate.To4() != nil && !candidate.IsLoopback() {
			ip = candidate.String()
			break
		}
	}
	if ip == "" {
		t.Fatal("test requires nonloopback IPv4")
	}
	peer, e := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if e != nil {
		t.Fatal(e)
	}
	enrollment, e := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if e != nil {
		peer.Close()
		t.Fatal(e)
	}
	settings := config.DefaultRuntimeSettings()
	settings.PeerListen = peer.Addr().String()
	settings.EnrollmentListen = enrollment.Addr().String()
	settings.AdvertisedPeer = settings.PeerListen
	settings.AdvertisedEnrollment = settings.EnrollmentListen
	if e := config.SaveRuntimeSettings(stateA, settings); e != nil {
		t.Fatal(e)
	}
	peer.Close()
	enrollment.Close()
	// 2. orbit launch daemon on node A (loopback dynamic port)
	cmd = orbitCmd("launch", "--state", stateA, "--control-listen", "127.0.0.1:0", "--no-browser", "--json")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("launch A failed: %v: %s", err, string(out))
	}
	t.Cleanup(func() {
		if err := testkit.ValidateDestructiveTarget(disposable, stateA); err != nil {
			t.Error(err)
			return
		}
		if err := app.StopAgent(stateA, 5*time.Second); err != nil {
			t.Errorf("stop disposable launch daemon: %v", err)
		}
	})

	var launchRes struct {
		ControlAddress string `json:"control_address"`
	}
	jsonStart := strings.Index(string(out), "{")
	if jsonStart == -1 {
		t.Fatalf("no JSON found in launch output: %s", string(out))
	}
	if err := json.Unmarshal([]byte(string(out)[jsonStart:]), &launchRes); err != nil || launchRes.ControlAddress == "" {
		t.Fatalf("failed to parse launch output: %v, output: %s", err, string(out))
	}

	ctrlURL := launchRes.ControlAddress
	if !strings.HasPrefix(ctrlURL, "http://") {
		ctrlURL = "http://" + ctrlURL
	}

	// 3. orbit invite create while daemon is running (daemon fallback)
	cmd = orbitCmd("invite", "create", "--state", stateA, "--endpoint", ctrlURL, "--ttl", "3600", "--uses", "1")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("invite create while daemon running failed: %v: %s", err, string(out))
	}
	outStr := string(out)
	if !strings.Contains(outStr, "orbit-invitation:v2:") {
		t.Fatalf("expected invitation link, got: %s", outStr)
	}
	var inviteLink string
	for _, line := range strings.Split(outStr, "\n") {
		if idx := strings.Index(line, "orbit-invitation:v2:"); idx != -1 {
			inviteLink = strings.TrimSpace(line[idx:])
			break
		}
	}

	// 4. orbit invite list while daemon is running
	cmd = orbitCmd("invite", "list", "--state", stateA)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("invite list while daemon running failed: %v: %s", err, string(out))
	}
	if !strings.Contains(string(out), "active") {
		t.Fatalf("expected active invitation in list, got: %s", string(out))
	}

	// 5. orbit devices list while daemon is running
	cmd = orbitCmd("devices", "list", "--state", stateA)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("devices list while daemon running failed: %v: %s", err, string(out))
	}
	if !strings.Contains(string(out), "Desktop-Workstation") {
		t.Fatalf("expected Desktop-Workstation in devices list, got: %s", string(out))
	}

	// 6. orbit join from node B
	invitationPath := filepath.Join(disposable, "invitation.txt")
	if e := os.WriteFile(invitationPath, []byte(inviteLink), 0600); e != nil {
		t.Fatal(e)
	}
	joinReview := filepath.Join(disposable, "join-reviewed.json")
	if out, e := orbitCmd("join", "--state", stateB, "--root", rootB, "--label", "Headless-Server", "--invitation-file", invitationPath, "--preview", "--review-file", joinReview).CombinedOutput(); e != nil {
		t.Fatalf("join review: %v %s", e, out)
	}
	t.Cleanup(func() {
		if err := testkit.ValidateDestructiveTarget(disposable, stateB); err != nil {
			t.Error(err)
			return
		}
		if err := app.StopAgent(stateB, 5*time.Second); err != nil {
			t.Error(err)
		}
	})
	joinCmd := orbitCmd("join", "--state", stateB, "--request-file", joinReview, "--timeout", "45")
	var joinOut bytes.Buffer
	joinCmd.Stdout = &joinOut
	joinCmd.Stderr = &joinOut
	if err := joinCmd.Start(); err != nil {
		t.Fatalf("join start failed: %v", err)
	}
	joinWaited := false
	t.Cleanup(func() {
		if joinWaited {
			return
		}
		if err := testkit.ValidateDestructiveTarget(disposable, stateB); err != nil {
			t.Error(err)
			return
		}
		_ = joinCmd.Process.Signal(os.Interrupt)
		_ = joinCmd.Wait()
	})

	queryClient := &controlclient.Client{StateDir: stateA}
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		r, e := queryClient.Query(context.Background(), tc.Query{Version: "1", Kind: "requests"})
		if e == nil && len(r.Requests) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 7. orbit requests list on node A while daemon is running
	cmd = orbitCmd("requests", "list", "--state", stateA, "--status", "pending")
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("requests list while daemon running failed: %v: %s", err, string(out))
	}
	if !strings.Contains(string(out), "Headless-Server") {
		t.Fatalf("expected pending request for Headless-Server, got: %s", string(out))
	}

	var reqID string
	for _, line := range strings.Split(string(out), "\n") {
		if idx := strings.Index(line, "request_id="); idx != -1 {
			parts := strings.Fields(line[idx:])
			reqID = strings.TrimPrefix(parts[0], "request_id=")
			break
		}
	}
	if reqID == "" {
		t.Fatalf("could not extract request_id from: %s", string(out))
	}

	// 8. orbit requests approve on node A while daemon is running
	client := &controlclient.Client{StateDir: stateA}
	requests, e := client.Query(context.Background(), tc.Query{Version: "1", Kind: "requests"})
	if e != nil {
		t.Fatal(e)
	}
	var reviewed tc.ApprovalIntent
	for _, r := range requests.Requests {
		if r.ID == reqID {
			reviewed = tc.ApprovalIntent{Request: r.ID, Folder: r.Folder, Requester: r.Requester, KeyPin: r.KeyPin, TranscriptDigest: r.TranscriptDigest, ExpectedMembership: r.ExpectedMembership, Decision: "approve"}
		}
	}
	reviewPath := filepath.Join(disposable, "approval.json")
	reviewBytes, e := json.Marshal(reviewed)
	if e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(reviewPath, reviewBytes, 0600); e != nil {
		t.Fatal(e)
	}
	cmd = orbitCmd("requests", "approve", "--state", stateA, "--request", reqID, "--alias", "Compute Server", "--review-file", reviewPath, "--operation", pairingOperation(t))
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("requests approve while daemon running failed: %v: %s", err, string(out))
	}
	if !strings.Contains(string(out), "Revision:   2") {
		t.Fatalf("expected Revision 2, got: %s", string(out))
	}

	// 9. wait for node B join to complete
	joinErr := joinCmd.Wait()
	joinWaited = true
	if err := joinErr; err != nil {
		t.Fatalf("join failed: %v: %s", err, joinOut.String())
	}
	if !strings.Contains(joinOut.String(), "Workspace joined successfully") {
		t.Fatalf("expected join success, got: %s", joinOut.String())
	}

	// 10. verify devices list on node A shows Compute Server
	cmd = orbitCmd("devices", "list", "--state", stateA)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("final devices list failed: %v: %s", err, string(out))
	}
	if !strings.Contains(string(out), "Compute Server") {
		t.Fatalf("expected Compute Server in devices list, got: %s", string(out))
	}
}

func decodePairingInvitation(t *testing.T, code string) tc.Invitation {
	t.Helper()
	var inv tc.Invitation
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(code, "orbit-invitation:v2:"))
	if err != nil {
		t.Fatal(err)
	}
	if err := tc.Decode(b, &inv); err != nil {
		t.Fatal(err)
	}
	return inv
}
func pairingOperation(t *testing.T) string {
	t.Helper()
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b[:])
}
func pairingReview(t *testing.T, c *control.Controller, request, decision string) tc.ApprovalIntent {
	t.Helper()
	r, err := c.TerminalQuery(context.Background(), tc.Query{Version: "1", Kind: "requests"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range r.Requests {
		if p.ID == request {
			return tc.ApprovalIntent{Request: p.ID, Folder: p.Folder, Requester: p.Requester, KeyPin: p.KeyPin, TranscriptDigest: p.TranscriptDigest, ExpectedMembership: p.ExpectedMembership, Decision: decision}
		}
	}
	t.Fatal("missing scoped requester review")
	return tc.ApprovalIntent{}
}
