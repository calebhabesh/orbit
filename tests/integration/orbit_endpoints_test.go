package integration_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

// TestOrbitEndpoints_PersistenceAndValidation tests saving, loading, validating,
// and removing peer endpoints in peers.json and via control operations.
func TestOrbitEndpoints_PersistenceAndValidation(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state-endpoints")
	_ = os.MkdirAll(stateDir, 0o700)

	var devID history.ID
	rand.Read(devID[:])
	_ = config.Save(stateDir, config.Config{FormatVersion: 1, DeviceID: hex.EncodeToString(devID[:]), CreatedAt: time.Now().UTC()})

	db, _ := repository.Open(ctx, stateDir)
	defer db.Close()
	ws := workspace.New(db, workspace.Options{})
	ctrl := control.New(db, ws, control.Options{LocalDevice: devID})

	folderHex := strings.Repeat("a", 64)
	peerHex := strings.Repeat("b", 64)
	certPath := filepath.Join(stateDir, "peer.crt")
	_ = os.WriteFile(certPath, []byte("CERT_DATA"), 0o600)

	// 1. Valid endpoint installation via controller
	err := ctrl.SetPeerEndpoint(ctx, control.SetPeerEndpointRequest{
		Folder:      folderHex,
		Device:      peerHex,
		URL:         "https://peer.example.com:8443",
		Certificate: certPath,
	})
	if err != nil {
		t.Fatalf("SetPeerEndpoint failed: %v", err)
	}

	// List endpoints
	listRes, err := ctrl.ListPeerEndpoints(ctx)
	if err != nil || len(listRes.Peers) != 1 {
		t.Fatalf("ListPeerEndpoints failed: %+v (err: %v)", listRes, err)
	}
	if listRes.Peers[0].URL != "https://peer.example.com:8443" {
		t.Fatalf("endpoint URL = %s, want https://peer.example.com:8443", listRes.Peers[0].URL)
	}

	// 2. Reject invalid endpoints
	invalidURLs := []string{
		"http://insecure.example.com:8443",
		"ftp://peer.example.com",
		"https://user:pass@peer.example.com",
		"https://peer.example.com/some/path",
		"https://peer.example.com?query=val",
		"https://peer.example.com#fragment",
	}
	for _, badURL := range invalidURLs {
		err := ctrl.SetPeerEndpoint(ctx, control.SetPeerEndpointRequest{
			Folder:      folderHex,
			Device:      peerHex,
			URL:         badURL,
			Certificate: certPath,
		})
		if err == nil {
			t.Fatalf("expected error for invalid peer URL %q", badURL)
		}
	}

	// Reject missing certificate
	err = ctrl.SetPeerEndpoint(ctx, control.SetPeerEndpointRequest{
		Folder:      folderHex,
		Device:      peerHex,
		URL:         "https://peer.example.com:8443",
		Certificate: "",
	})
	if err == nil {
		t.Fatal("expected error for empty certificate path")
	}

	// 3. Remove endpoint
	err = ctrl.RemovePeerEndpoint(ctx, control.RemovePeerEndpointRequest{
		Folder: folderHex,
		Device: peerHex,
	})
	if err != nil {
		t.Fatalf("RemovePeerEndpoint failed: %v", err)
	}

	listRes2, err := ctrl.ListPeerEndpoints(ctx)
	if err != nil || len(listRes2.Peers) != 0 {
		t.Fatalf("expected 0 endpoints after remove, got %d", len(listRes2.Peers))
	}
}

// TestOrbitEndpoints_TwoPeerDirectPullSync tests full two-peer direct pull sync:
// Peer A authors content, Peer B pulls over mutual TLS HTTPS using configured endpoints.
func TestOrbitEndpoints_TwoPeerDirectPullSync(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)

	// Node A setup
	dirA := filepath.Join(disposable, "node-a")
	rootA := filepath.Join(disposable, "root-a")
	_ = os.MkdirAll(dirA, 0o700)
	_ = os.MkdirAll(rootA, 0o755)

	var devA history.ID
	rand.Read(devA[:])
	_ = config.Save(dirA, config.Config{FormatVersion: 1, DeviceID: hex.EncodeToString(devA[:]), CreatedAt: time.Now().UTC()})
	identA, _ := replication.LoadOrCreateIdentity(dirA, devA, time.Now())
	dbA, _ := repository.Open(ctx, dirA)
	defer dbA.Close()
	wsA := workspace.New(dbA, workspace.Options{})

	// Node B setup
	dirB := filepath.Join(disposable, "node-b")
	rootB := filepath.Join(disposable, "root-b")
	_ = os.MkdirAll(dirB, 0o700)
	_ = os.MkdirAll(rootB, 0o755)

	var devB history.ID
	rand.Read(devB[:])
	_ = config.Save(dirB, config.Config{FormatVersion: 1, DeviceID: hex.EncodeToString(devB[:]), CreatedAt: time.Now().UTC()})
	identB, _ := replication.LoadOrCreateIdentity(dirB, devB, time.Now())
	dbB, _ := repository.Open(ctx, dirB)
	defer dbB.Close()
	wsB := workspace.New(dbB, workspace.Options{})

	var folderID history.ID
	rand.Read(folderID[:])
	_ = dbA.EnsureFolder(ctx, folderID, devA, 1)
	_ = dbB.EnsureFolder(ctx, folderID, devB, 1)
	_, _ = wsA.Register(ctx, folderID, rootA)
	_, _ = wsB.Register(ctx, folderID, rootB)

	// Approved Revision 1
	mem := protocol.Membership{
		Folder:      folderID,
		Revision:    1,
		PriorDigest: history.Digest{},
		Active: []protocol.ActiveMember{
			{Device: devA, KeyPin: identA.KeyPin},
			{Device: devB, KeyPin: identB.KeyPin},
		},
	}
	appA, err := dbA.ApproveMembership(ctx, mem)
	if err != nil {
		t.Fatal(err)
	}
	appB, err := dbB.ApproveMembership(ctx, mem)
	if err != nil {
		t.Fatal(err)
	}

	// Start Node A's replication server
	listenerA, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listenerA.Close()
	serverA := replication.NewServer(dbA, identA)
	srvCtx, cancelA := context.WithCancel(ctx)
	defer cancelA()
	go func() { _ = serverA.Serve(srvCtx, listenerA) }()

	// Author a test file on Node A
	testFilePath := filepath.Join(rootA, "hello.txt")
	testContent := []byte("Hello Orbit Decentralized Sync!")
	if err := os.WriteFile(testFilePath, testContent, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wsA.Scan(ctx, folderID); err != nil {
		t.Fatalf("Node A scan failed: %v", err)
	}

	// Node B configures peer endpoint for Node A
	certAPath := filepath.Join(dirB, "nodeA.crt")
	_ = os.WriteFile(certAPath, []byte("DUMMY_CERT"), 0o600)
	_ = config.SetPeerEndpoint(dirB, config.PeerEndpoint{
		Folder:      hex.EncodeToString(folderID[:]),
		Device:      hex.EncodeToString(devA[:]),
		URL:         "https://" + listenerA.Addr().String(),
		Certificate: certAPath,
	})

	// Create replication client and run syncer on Node B
	clientB, err := replication.NewClient("https://"+listenerA.Addr().String(), identB, identA.Leaf, identA.KeyPin)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	syncerB := replication.NewSyncer(
		dbB,
		wsB,
		clientB,
		devB,
		devA,
		folderID,
		appB,
		replication.TransferOptions{Retries: 2},
	)

	syncRes, err := syncerB.Sync(ctx)
	if err != nil {
		t.Fatalf("Node B pull sync failed: %v", err)
	}
	if syncRes.ChunksFetched != 1 {
		t.Fatalf("chunks fetched = %d, want 1", syncRes.ChunksFetched)
	}
	if syncRes.VersionsApplied != 1 {
		t.Fatalf("versions applied = %d, want 1", syncRes.VersionsApplied)
	}

	// Verify file arrived bit-for-bit identical on Node B
	pulledFile := filepath.Join(rootB, "hello.txt")
	pulledData, err := os.ReadFile(pulledFile)
	if err != nil {
		t.Fatalf("read pulled file on Node B: %v", err)
	}
	if string(pulledData) != string(testContent) {
		t.Fatalf("content mismatch on Node B: got %q, want %q", pulledData, testContent)
	}
	_ = appA
}

// TestOrbitEndpoints_ForwardingSync tests A→Hub→B forwarding sync:
// Node A has no direct network connectivity to Node B; synchronization
// flows forward through Hub H cleanly.
func TestOrbitEndpoints_ForwardingSync(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)

	// Node A setup
	dirA := filepath.Join(disposable, "fwd-a")
	rootA := filepath.Join(disposable, "root-a")
	_ = os.MkdirAll(dirA, 0o700)
	_ = os.MkdirAll(rootA, 0o755)
	var devA history.ID
	rand.Read(devA[:])
	_ = config.Save(dirA, config.Config{FormatVersion: 1, DeviceID: hex.EncodeToString(devA[:]), CreatedAt: time.Now().UTC()})
	identA, _ := replication.LoadOrCreateIdentity(dirA, devA, time.Now())
	dbA, _ := repository.Open(ctx, dirA)
	defer dbA.Close()
	wsA := workspace.New(dbA, workspace.Options{})

	// Hub H setup
	dirH := filepath.Join(disposable, "fwd-h")
	rootH := filepath.Join(disposable, "root-h")
	_ = os.MkdirAll(dirH, 0o700)
	_ = os.MkdirAll(rootH, 0o755)
	var devH history.ID
	rand.Read(devH[:])
	_ = config.Save(dirH, config.Config{FormatVersion: 1, DeviceID: hex.EncodeToString(devH[:]), CreatedAt: time.Now().UTC()})
	identH, _ := replication.LoadOrCreateIdentity(dirH, devH, time.Now())
	dbH, _ := repository.Open(ctx, dirH)
	defer dbH.Close()
	wsH := workspace.New(dbH, workspace.Options{})

	// Node B setup
	dirB := filepath.Join(disposable, "fwd-b")
	rootB := filepath.Join(disposable, "root-b")
	_ = os.MkdirAll(dirB, 0o700)
	_ = os.MkdirAll(rootB, 0o755)
	var devB history.ID
	rand.Read(devB[:])
	_ = config.Save(dirB, config.Config{FormatVersion: 1, DeviceID: hex.EncodeToString(devB[:]), CreatedAt: time.Now().UTC()})
	identB, _ := replication.LoadOrCreateIdentity(dirB, devB, time.Now())
	dbB, _ := repository.Open(ctx, dirB)
	defer dbB.Close()
	wsB := workspace.New(dbB, workspace.Options{})

	var folderID history.ID
	rand.Read(folderID[:])
	_ = dbA.EnsureFolder(ctx, folderID, devA, 1)
	_ = dbH.EnsureFolder(ctx, folderID, devH, 1)
	_ = dbB.EnsureFolder(ctx, folderID, devB, 1)
	_, _ = wsA.Register(ctx, folderID, rootA)
	_, _ = wsH.Register(ctx, folderID, rootH)
	_, _ = wsB.Register(ctx, folderID, rootB)

	// Approved Revision 1 across A, H, and B
	mem := protocol.Membership{
		Folder:      folderID,
		Revision:    1,
		PriorDigest: history.Digest{},
		Active: []protocol.ActiveMember{
			{Device: devA, KeyPin: identA.KeyPin},
			{Device: devH, KeyPin: identH.KeyPin},
			{Device: devB, KeyPin: identB.KeyPin},
		},
	}
	_, _ = dbA.ApproveMembership(ctx, mem)
	appH, _ := dbH.ApproveMembership(ctx, mem)
	appB, _ := dbB.ApproveMembership(ctx, mem)

	// Hub H runs replication server
	listenerH, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listenerH.Close()
	serverH := replication.NewServer(dbH, identH)
	srvCtx, cancelH := context.WithCancel(ctx)
	defer cancelH()
	go func() { _ = serverH.Serve(srvCtx, listenerH) }()

	// Step 1: Node A creates file and syncs forward to Hub H
	fwdFile := filepath.Join(rootA, "forwarded_document.txt")
	fwdContent := []byte("Originates at Node A, traverses Hub H, lands on Node B.")
	if err := os.WriteFile(fwdFile, fwdContent, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wsA.Scan(ctx, folderID); err != nil {
		t.Fatal(err)
	}

	// Node A serves listener so Hub H can pull from A
	listenerA, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listenerA.Close()
	serverA := replication.NewServer(dbA, identA)
	srvCtxA, cancelA := context.WithCancel(ctx)
	defer cancelA()
	go func() { _ = serverA.Serve(srvCtxA, listenerA) }()

	clientH_from_A, err := replication.NewClient("https://"+listenerA.Addr().String(), identH, identA.Leaf, identA.KeyPin)
	if err != nil {
		t.Fatal(err)
	}
	syncerH := replication.NewSyncer(dbH, wsH, clientH_from_A, devH, devA, folderID, appH, replication.TransferOptions{Retries: 2})
	resH, err := syncerH.Sync(ctx)
	if err != nil {
		t.Fatalf("Hub pull from Node A failed: %v", err)
	}
	if resH.VersionsApplied != 1 {
		t.Fatalf("Hub versions applied = %d, want 1", resH.VersionsApplied)
	}

	// Step 2: Node B pulls from Hub H (Node B has ZERO connectivity to Node A)
	clientB_from_H, err := replication.NewClient("https://"+listenerH.Addr().String(), identB, identH.Leaf, identH.KeyPin)
	if err != nil {
		t.Fatal(err)
	}
	syncerB := replication.NewSyncer(dbB, wsB, clientB_from_H, devB, devH, folderID, appB, replication.TransferOptions{Retries: 2})
	resB, err := syncerB.Sync(ctx)
	if err != nil {
		t.Fatalf("Node B pull from Hub failed: %v", err)
	}
	if resB.VersionsApplied != 1 {
		t.Fatalf("Node B versions applied = %d, want 1", resB.VersionsApplied)
	}

	// Verify file arrived on Node B bit-for-bit
	receivedFile := filepath.Join(rootB, "forwarded_document.txt")
	receivedBytes, err := os.ReadFile(receivedFile)
	if err != nil {
		t.Fatalf("read file on Node B: %v", err)
	}
	if string(receivedBytes) != string(fwdContent) {
		t.Fatalf("forwarded content mismatch: got %q, want %q", receivedBytes, fwdContent)
	}
}

// TestOrbitEndpoints_ErrorCategorization tests that network errors, pin mismatches,
// version mismatches, and membership mismatches are reported as distinct categorized errors.
func TestOrbitEndpoints_ErrorCategorization(t *testing.T) {
	ctx := context.Background()
	disposable := testkit.NewDisposable(t)

	dir := filepath.Join(disposable, "state-errors")
	_ = os.MkdirAll(dir, 0o700)
	var dev history.ID
	rand.Read(dev[:])
	_ = config.Save(dir, config.Config{FormatVersion: 1, DeviceID: hex.EncodeToString(dev[:]), CreatedAt: time.Now().UTC()})
	ident, _ := replication.LoadOrCreateIdentity(dir, dev, time.Now())
	db, _ := repository.Open(ctx, dir)
	defer db.Close()

	// 1. Connection Refused (port with no listener)
	// Find unused port
	dummyListener, _ := net.Listen("tcp", "127.0.0.1:0")
	unusedAddr := dummyListener.Addr().String()
	dummyListener.Close()

	badClient, err := replication.NewClient("https://"+unusedAddr, ident, ident.Leaf, ident.KeyPin)
	if err != nil {
		t.Fatal(err)
	}
	var folderID history.ID
	rand.Read(folderID[:])
	_, connErr := badClient.Inventory(ctx, replication.InventoryRequest{
		ProtocolVersion:  replication.ProtocolVersion,
		DeviceID:         hex.EncodeToString(dev[:]),
		FolderID:         hex.EncodeToString(folderID[:]),
		Revision:         "1",
		MembershipDigest: strings.Repeat("0", 64),
		Cursor:           "0",
		PageSize:         "10",
	})
	if connErr == nil {
		t.Fatal("expected connection refused error")
	}
	// Verify it is a network connection error
	if !strings.Contains(connErr.Error(), "connection refused") && !strings.Contains(connErr.Error(), "connect") {
		t.Fatalf("expected network connection error message, got: %v", connErr)
	}

	// 2. Pin Mismatch (client presents wrong pinned certificate/pin)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	repServer := replication.NewServer(db, ident)
	srvCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = repServer.Serve(srvCtx, listener) }()

	// Client expects wrong key pin
	var bogusPin history.Digest
	rand.Read(bogusPin[:])
	_, pinErr := replication.NewClient("https://"+listener.Addr().String(), ident, ident.Leaf, bogusPin)
	if pinErr == nil {
		t.Fatal("expected error on pin mismatch during client creation")
	}
	if !strings.Contains(pinErr.Error(), "key pin") {
		t.Fatalf("expected key pin error, got: %v", pinErr)
	}

	// 3. Membership Mismatch / Unauthorized
	// Real client with correct pin connects, but queries folder it is not authorized for
	var otherDev history.ID
	rand.Read(otherDev[:])
	otherDir := filepath.Join(disposable, "other-state")
	_ = os.MkdirAll(otherDir, 0o700)
	otherIdent, _ := replication.LoadOrCreateIdentity(otherDir, otherDev, time.Now())

	clientValidPin, err := replication.NewClient("https://"+listener.Addr().String(), otherIdent, ident.Leaf, ident.KeyPin)
	if err != nil {
		t.Fatal(err)
	}

	var unknownFolder history.ID
	rand.Read(unknownFolder[:])
	_, memErr := clientValidPin.Inventory(ctx, replication.InventoryRequest{
		ProtocolVersion:  replication.ProtocolVersion,
		DeviceID:         hex.EncodeToString(otherDev[:]),
		FolderID:         hex.EncodeToString(unknownFolder[:]),
		Revision:         "1",
		MembershipDigest: strings.Repeat("0", 64),
		Cursor:           "0",
		PageSize:         "10",
	})
	if memErr == nil {
		t.Fatal("expected unauthorized or membership error on unknown folder")
	}
	var wireErr *replication.WireError
	if errors.As(memErr, &wireErr) {
		if wireErr.Body.Code != "UNAUTHORIZED" && wireErr.Body.Code != "MEMBERSHIP_MISMATCH" {
			t.Fatalf("expected UNAUTHORIZED or MEMBERSHIP_MISMATCH, got %s", wireErr.Body.Code)
		}
	}
}
