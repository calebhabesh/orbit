package replication

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/repository"
)

type peerFixture struct {
	t          *testing.T
	ctx        context.Context
	cancel     context.CancelFunc
	serverRepo *repository.DB
	serverID   Identity
	clientID   Identity
	attackerID Identity
	folder     history.ID
	membership protocol.Membership
	digest     history.Digest
	server     *Server
	client     *Client
	baseURL    string
}

func fixedID(value byte) history.ID {
	var id history.ID
	for i := range id {
		id[i] = value
	}
	return id
}

func newPeerFixture(t *testing.T) *peerFixture {
	t.Helper()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	serverDevice, clientDevice, attackerDevice := fixedID('A'), fixedID('B'), fixedID('X')
	serverID, err := LoadOrCreateIdentity(filepath.Join(t.TempDir(), "server"), serverDevice, now)
	if err != nil {
		t.Fatal(err)
	}
	clientID, err := LoadOrCreateIdentity(filepath.Join(t.TempDir(), "client"), clientDevice, now)
	if err != nil {
		t.Fatal(err)
	}
	attackerID, err := LoadOrCreateIdentity(filepath.Join(t.TempDir(), "attacker"), attackerDevice, now)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := repository.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	folder := fixedID('F')
	if err := repo.EnsureFolder(context.Background(), folder, serverDevice, 1); err != nil {
		t.Fatal(err)
	}
	membership := protocol.Membership{Folder: folder, Revision: 1, Active: []protocol.ActiveMember{{Device: serverDevice, KeyPin: serverID.KeyPin}, {Device: clientDevice, KeyPin: clientID.KeyPin}}}
	approved, err := repo.ApproveMembership(context.Background(), membership)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	server := NewServer(repo, serverID)
	server.now = func() time.Time { return now }
	go func() { _ = server.Serve(ctx, listener) }()
	client, err := NewClient("https://"+listener.Addr().String(), clientID, serverID.Leaf, serverID.KeyPin)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &peerFixture{t: t, ctx: ctx, cancel: cancel, serverRepo: repo, serverID: serverID, clientID: clientID, attackerID: attackerID, folder: folder, membership: membership, digest: approved.Digest, server: server, client: client, baseURL: "https://" + listener.Addr().String()}
	t.Cleanup(func() {
		client.CloseIdleConnections()
		cancel()
		_ = repo.Close()
	})
	return fixture
}

func (fixture *peerFixture) handshake() FolderHandshake {
	return FolderHandshake{FolderID: hex.EncodeToString(fixture.folder[:]), Revision: strconv.FormatUint(fixture.membership.Revision, 10), MembershipDigest: hex.EncodeToString(fixture.digest[:])}
}

func (fixture *peerFixture) hello(device history.ID, folder FolderHandshake) HelloRequest {
	return HelloRequest{ProtocolVersion: ProtocolVersion, DeviceID: hex.EncodeToString(device[:]), Folders: []FolderHandshake{folder}, Limits: Limits{MetadataBytes: strconv.FormatInt(MaxMetadataBytes, 10), InventoryPage: strconv.Itoa(MaxInventoryPage)}}
}

func (fixture *peerFixture) inventory(token string, cursor string, pageSize int) InventoryRequest {
	handshake := fixture.handshake()
	return InventoryRequest{ProtocolVersion: ProtocolVersion, DeviceID: hex.EncodeToString(fixture.clientID.DeviceID[:]), FolderID: handshake.FolderID, Revision: handshake.Revision, MembershipDigest: handshake.MembershipDigest, SnapshotToken: token, Cursor: cursor, PageSize: strconv.Itoa(pageSize)}
}

func TestIdentityPersistsAndPrivateMaterialIsProtected(t *testing.T) {
	dir := t.TempDir()
	device := fixedID('D')
	first, err := LoadOrCreateIdentity(dir, device, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreateIdentity(dir, device, time.Now().AddDate(1, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if first.KeyPin != second.KeyPin {
		t.Fatal("reconnect generated a new key identity")
	}
	info, err := os.Stat(filepath.Join(dir, "identity", "peer-identity.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("identity mode = %o, want 0600", info.Mode().Perm())
	}
	if err := os.Chmod(filepath.Join(dir, "identity", "peer-identity.pem"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateIdentity(dir, device, time.Now()); err == nil {
		t.Fatal("insecure private-key permissions were accepted")
	}
}

func TestRequestTokenBucketIsBoundedAndRefills(t *testing.T) {
	server := &Server{rateLast: time.Unix(0, 0), rateTokens: 2}
	if !server.allowRequest(time.Unix(0, 0)) || !server.allowRequest(time.Unix(0, 0)) || server.allowRequest(time.Unix(0, 0)) {
		t.Fatal("token bucket did not enforce its burst")
	}
	if !server.allowRequest(time.Unix(0, 0).Add(time.Second / 64)) {
		t.Fatal("token bucket did not refill at the configured rate")
	}
}

func TestGoldenEndpointFixturesDecodeStrictly(t *testing.T) {
	cases := []struct {
		name string
		file string
		out  any
	}{
		{"hello", "peer-hello-request-v1.json", &HelloRequest{}},
		{"inventory request", "peer-inventory-request-v1.json", &InventoryRequest{}},
		{"inventory response", "peer-inventory-response-v1.json", &InventoryResponse{}},
		{"versions request", "peer-versions-request-v1.json", &VersionsRequest{}},
		{"chunk request", "peer-chunk-request-v1.json", &ChunkRequest{}},
		{"error", "peer-error-v1.json", &ErrorResponse{}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "schemas", "fixtures", test.file))
			if err != nil {
				t.Fatal(err)
			}
			if err := protocol.DecodeStrict(data, test.out); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRealTLSHelloInventorySnapshotAndEnvelopeFetch(t *testing.T) {
	fixture := newPeerFixture(t)
	for i := 0; i < 129; i++ {
		_, err := fixture.serverRepo.CreateLocalVersion(context.Background(), repository.LocalVersionRequest{Folder: fixture.folder, Path: fmt.Sprintf("file-%03d", i), Kind: history.KindDirectory, AuthoredRevision: 1})
		if err != nil {
			t.Fatal(err)
		}
	}
	hello, err := fixture.client.Hello(context.Background(), fixture.hello(fixture.clientID.DeviceID, fixture.handshake()))
	if err != nil {
		t.Fatal(err)
	}
	if hello.DeviceID != hex.EncodeToString(fixture.serverID.DeviceID[:]) || len(hello.Folders) != 1 {
		t.Fatalf("unexpected hello response: %+v", hello)
	}
	first, err := fixture.client.Inventory(context.Background(), fixture.inventory("", "0", MaxInventoryPage))
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) != MaxInventoryPage || first.Done || first.NextCursor != "128" {
		t.Fatalf("unexpected first page: entries=%d done=%v cursor=%s", len(first.Entries), first.Done, first.NextCursor)
	}
	late, err := fixture.serverRepo.CreateLocalVersion(context.Background(), repository.LocalVersionRequest{Folder: fixture.folder, Path: "late", Kind: history.KindDirectory, AuthoredRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := fixture.client.Inventory(context.Background(), fixture.inventory(first.SnapshotToken, first.NextCursor, MaxInventoryPage))
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Entries) != 1 || !second.Done {
		t.Fatalf("snapshot changed under pagination: entries=%d done=%v", len(second.Entries), second.Done)
	}
	restarted, err := fixture.client.Inventory(context.Background(), fixture.inventory("", "0", MaxInventoryPage))
	if err != nil {
		t.Fatal(err)
	}
	if len(restarted.Entries) != MaxInventoryPage {
		t.Fatalf("restart first page entries=%d", len(restarted.Entries))
	}
	versions, err := fixture.client.Versions(context.Background(), VersionsRequest{ProtocolVersion: ProtocolVersion, DeviceID: hex.EncodeToString(fixture.clientID.DeviceID[:]), FolderID: fixture.handshake().FolderID, Revision: "1", MembershipDigest: fixture.handshake().MembershipDigest, Versions: []VersionIDWire{{AuthorID: hex.EncodeToString(late.ID.Author[:]), Counter: strconv.FormatUint(late.ID.Counter, 10)}}})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := protocol.DecodeEnvelope(versions.Envelopes[0])
	if err != nil || decoded.ID != late.ID {
		t.Fatalf("fetched envelope = %+v, err=%v", decoded.ID, err)
	}
}

func TestAuthenticationAuthorizationAndCompatibilityMatrix(t *testing.T) {
	fixture := newPeerFixture(t)
	if _, err := NewClient(fixture.baseURL, fixture.clientID, fixture.serverID.Leaf, fixture.attackerID.KeyPin); err == nil {
		t.Fatal("client accepted a server certificate with the wrong approved pin")
	}
	valid := fixture.hello(fixture.clientID.DeviceID, fixture.handshake())
	wrongFolder := fixedID('Z')
	cases := []struct {
		name   string
		client *Client
		body   HelloRequest
		code   string
	}{
		{name: "wrong key for claimed paired device", body: valid, code: "UNAUTHORIZED"},
		{name: "unpaired device", body: fixture.hello(fixture.attackerID.DeviceID, fixture.handshake()), code: "UNAUTHORIZED"},
		{name: "wrong folder", body: fixture.hello(fixture.clientID.DeviceID, FolderHandshake{FolderID: hex.EncodeToString(wrongFolder[:]), Revision: "1", MembershipDigest: fixture.handshake().MembershipDigest}), code: "UNAUTHORIZED"},
		{name: "membership mismatch", body: fixture.hello(fixture.clientID.DeviceID, FolderHandshake{FolderID: fixture.handshake().FolderID, Revision: "2", MembershipDigest: fixture.handshake().MembershipDigest}), code: "MEMBERSHIP_MISMATCH"},
		{name: "protocol mismatch", body: valid, code: "INCOMPATIBLE_VERSION"},
	}
	attacker, err := NewClient(fixture.baseURL, fixture.attackerID, fixture.serverID.Leaf, fixture.serverID.KeyPin)
	if err != nil {
		t.Fatal(err)
	}
	defer attacker.CloseIdleConnections()
	for i := range cases {
		cases[i].client = attacker
	}
	cases[2].client = fixture.client
	cases[3].client = fixture.client
	cases[4].client = fixture.client
	cases[4].body.ProtocolVersion = "2"
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.client.Hello(context.Background(), test.body)
			var wire *WireError
			if !errors.As(err, &wire) || wire.Body.Code != test.code {
				t.Fatalf("error = %#v, want wire code %s", err, test.code)
			}
		})
	}
}

func TestRevokedPeerFailsCurrentMembership(t *testing.T) {
	fixture := newPeerFixture(t)
	snapshot := sha256.Sum256([]byte("retirement snapshot"))
	revision2 := protocol.Membership{Folder: fixture.folder, Revision: 2, PriorDigest: fixture.digest, Active: []protocol.ActiveMember{{Device: fixture.serverID.DeviceID, KeyPin: fixture.serverID.KeyPin}}, Retired: []protocol.RetiredMember{{Device: fixture.clientID.DeviceID, RetiredAt: 2, SnapshotDigest: snapshot}}}
	approved, err := fixture.serverRepo.ApproveMembership(context.Background(), revision2)
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.hello(fixture.clientID.DeviceID, FolderHandshake{FolderID: fixture.handshake().FolderID, Revision: "2", MembershipDigest: hex.EncodeToString(approved.Digest[:])})
	_, err = fixture.client.Hello(context.Background(), request)
	var wire *WireError
	if !errors.As(err, &wire) || wire.Body.Code != "UNAUTHORIZED" {
		t.Fatalf("revoked request error = %#v", err)
	}
}

func TestSnapshotExpiryRequiresSafeRestart(t *testing.T) {
	fixture := newPeerFixture(t)
	clock := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	fixture.server.now = func() time.Time { return clock }
	_, err := fixture.serverRepo.CreateLocalVersion(context.Background(), repository.LocalVersionRequest{Folder: fixture.folder, Path: "one", Kind: history.KindDirectory, AuthoredRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	first, err := fixture.client.Inventory(context.Background(), fixture.inventory("", "0", 1))
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(31 * time.Second)
	_, err = fixture.client.Inventory(context.Background(), fixture.inventory(first.SnapshotToken, first.NextCursor, 1))
	var wire *WireError
	if !errors.As(err, &wire) || wire.Body.Code != "SNAPSHOT_EXPIRED" {
		t.Fatalf("expired snapshot error = %#v", err)
	}
	restarted, err := fixture.client.Inventory(context.Background(), fixture.inventory("", "0", 1))
	if err != nil || len(restarted.Entries) != 1 {
		t.Fatalf("safe restart response=%+v err=%v", restarted, err)
	}
}

func TestChunkServingRequiresAuthorizedManifestVersion(t *testing.T) {
	fixture := newPeerFixture(t)
	content := []byte("manifest-scoped secret")
	manifest, err := fixture.serverRepo.StoreFile(context.Background(), bytes.NewReader(content), false)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := fixture.serverRepo.CreateLocalVersion(context.Background(), repository.LocalVersionRequest{Folder: fixture.folder, Path: "secret", Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	handshake := fixture.handshake()
	request := ChunkRequest{ProtocolVersion: ProtocolVersion, DeviceID: hex.EncodeToString(fixture.clientID.DeviceID[:]), FolderID: handshake.FolderID, Revision: handshake.Revision, MembershipDigest: handshake.MembershipDigest, AuthorID: hex.EncodeToString(envelope.ID.Author[:]), Counter: strconv.FormatUint(envelope.ID.Counter, 10), ChunkIndex: "0"}
	got, err := fixture.client.Chunk(context.Background(), request, manifest.Chunks[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("chunk = %q, want %q", got, content)
	}
	request.Counter = strconv.FormatUint(envelope.ID.Counter+1, 10)
	_, err = fixture.client.Chunk(context.Background(), request, manifest.Chunks[0])
	var wire *WireError
	if !errors.As(err, &wire) || wire.Body.Code != "CONTENT_UNAVAILABLE" {
		t.Fatalf("unbound digest request error = %#v", err)
	}
}

func TestMalformedAndOverLimitInputsAreBounded(t *testing.T) {
	fixture := newPeerFixture(t)
	valid, err := json.Marshal(fixture.hello(fixture.clientID.DeviceID, fixture.handshake()))
	if err != nil {
		t.Fatal(err)
	}
	duplicate := bytes.Replace(valid, []byte(`"device_id":`), []byte(`"device_id":"`+hex.EncodeToString(fixture.clientID.DeviceID[:])+`","device_id":`), 1)
	for name, body := range map[string]io.Reader{
		"duplicate key":  bytes.NewReader(duplicate),
		"oversized body": io.MultiReader(bytes.NewReader(valid[:len(valid)-1]), strings.NewReader(`,"padding":"`), io.LimitReader(zeroReader{}, MaxMetadataBytes), strings.NewReader(`"}`)),
	} {
		t.Run(name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodPost, fixture.baseURL+"/peer/v1/hello", body)
			if err != nil {
				t.Fatal(err)
			}
			response, err := fixture.client.http.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusBadRequest && response.StatusCode != http.StatusRequestEntityTooLarge {
				t.Fatalf("status = %d", response.StatusCode)
			}
		})
	}
	_, err = fixture.client.Inventory(context.Background(), fixture.inventory("", "0", MaxInventoryPage+1))
	var wire *WireError
	if !errors.As(err, &wire) || wire.Body.Code != "INVALID_REQUEST" {
		t.Fatalf("over-limit page error = %#v", err)
	}
	versions := make([]VersionIDWire, MaxVersionBatch+1)
	for i := range versions {
		versions[i] = VersionIDWire{AuthorID: hex.EncodeToString(fixture.serverID.DeviceID[:]), Counter: "1"}
	}
	_, err = fixture.client.Versions(context.Background(), VersionsRequest{ProtocolVersion: ProtocolVersion, DeviceID: hex.EncodeToString(fixture.clientID.DeviceID[:]), FolderID: fixture.handshake().FolderID, Revision: "1", MembershipDigest: fixture.handshake().MembershipDigest, Versions: versions})
	if !errors.As(err, &wire) || wire.Body.Code != "INVALID_REQUEST" {
		t.Fatalf("over-limit version batch error = %#v", err)
	}
}

type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) {
	for i := range buffer {
		buffer[i] = 'x'
	}
	return len(buffer), nil
}

func TestReceiptsEndpoint(t *testing.T) {
	fixture := newPeerFixture(t)
	content := []byte("receipt payload")
	manifest, err := fixture.serverRepo.StoreFile(context.Background(), bytes.NewReader(content), false)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := fixture.serverRepo.CreateLocalVersion(context.Background(), repository.LocalVersionRequest{Folder: fixture.folder, Path: "receipt-file", Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	handshake := fixture.handshake()
	wireVer := VersionIDWire{AuthorID: hex.EncodeToString(envelope.ID.Author[:]), Counter: strconv.FormatUint(envelope.ID.Counter, 10)}

	// Successful receipt
	req := ReceiptsRequest{
		ProtocolVersion:  ProtocolVersion,
		DeviceID:         hex.EncodeToString(fixture.clientID.DeviceID[:]),
		FolderID:         handshake.FolderID,
		Revision:         handshake.Revision,
		MembershipDigest: handshake.MembershipDigest,
		Versions:         []VersionIDWire{wireVer},
	}
	resp, err := fixture.client.Receipts(context.Background(), req)
	if err != nil {
		t.Fatalf("receipts: %v", err)
	}
	if len(resp.Accepted) != 1 || resp.Accepted[0] != wireVer {
		t.Fatalf("accepted = %+v, want %+v", resp.Accepted, wireVer)
	}
	peers, err := fixture.serverRepo.PeerProgress(context.Background(), fixture.folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 || peers[0].Peer != fixture.clientID.DeviceID || !peers[0].Receipt {
		t.Fatalf("peer progress = %+v", peers)
	}

	// Unknown version returns CONTENT_UNAVAILABLE
	unknownReq := req
	unknownReq.Versions = []VersionIDWire{{AuthorID: hex.EncodeToString(fixture.serverID.DeviceID[:]), Counter: "999"}}
	_, err = fixture.client.Receipts(context.Background(), unknownReq)
	var wire *WireError
	if !errors.As(err, &wire) || wire.Body.Code != "CONTENT_UNAVAILABLE" {
		t.Fatalf("unknown version error = %#v", err)
	}

	// Duplicate version in batch returns INVALID_REQUEST
	dupReq := req
	dupReq.Versions = []VersionIDWire{wireVer, wireVer}
	_, err = fixture.client.Receipts(context.Background(), dupReq)
	if !errors.As(err, &wire) || wire.Body.Code != "INVALID_REQUEST" {
		t.Fatalf("duplicate version error = %#v", err)
	}

	// Empty batch returns INVALID_REQUEST
	emptyReq := req
	emptyReq.Versions = nil
	_, err = fixture.client.Receipts(context.Background(), emptyReq)
	if !errors.As(err, &wire) || wire.Body.Code != "INVALID_REQUEST" {
		t.Fatalf("empty version batch error = %#v", err)
	}

	// Over-limit batch returns INVALID_REQUEST
	overReq := req
	overReq.Versions = make([]VersionIDWire, MaxVersionBatch+1)
	for i := range overReq.Versions {
		overReq.Versions[i] = VersionIDWire{AuthorID: hex.EncodeToString(fixture.serverID.DeviceID[:]), Counter: strconv.Itoa(i + 1)}
	}
	_, err = fixture.client.Receipts(context.Background(), overReq)
	if !errors.As(err, &wire) || wire.Body.Code != "INVALID_REQUEST" {
		t.Fatalf("over-limit batch error = %#v", err)
	}

	// Incompatible protocol version returns INCOMPATIBLE_VERSION
	badProtoReq := req
	badProtoReq.ProtocolVersion = "99"
	_, err = fixture.client.Receipts(context.Background(), badProtoReq)
	if !errors.As(err, &wire) || wire.Body.Code != "INCOMPATIBLE_VERSION" {
		t.Fatalf("incompatible protocol version error = %#v", err)
	}
}

func TestStatusEndpoint(t *testing.T) {
	fixture := newPeerFixture(t)
	content := []byte("status test file")
	manifest, err := fixture.serverRepo.StoreFile(context.Background(), bytes.NewReader(content), false)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := fixture.serverRepo.CreateLocalVersion(context.Background(), repository.LocalVersionRequest{Folder: fixture.folder, Path: "status-file", Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	handshake := fixture.handshake()
	wireVer := VersionIDWire{AuthorID: hex.EncodeToString(envelope.ID.Author[:]), Counter: strconv.FormatUint(envelope.ID.Counter, 10)}
	unknownVer := VersionIDWire{AuthorID: hex.EncodeToString(fixture.serverID.DeviceID[:]), Counter: "999"}

	req := StatusRequest{
		ProtocolVersion:  ProtocolVersion,
		DeviceID:         hex.EncodeToString(fixture.clientID.DeviceID[:]),
		FolderID:         handshake.FolderID,
		Revision:         handshake.Revision,
		MembershipDigest: handshake.MembershipDigest,
		Versions:         []VersionIDWire{wireVer, unknownVer},
	}
	resp, err := fixture.client.Status(context.Background(), req)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(resp.Entries) != 2 {
		t.Fatalf("entries count = %d, want 2", len(resp.Entries))
	}
	if !resp.Entries[0].MetadataKnown || !resp.Entries[0].Stored || resp.Entries[0].ContentState != "ready" {
		t.Fatalf("known version status = %+v", resp.Entries[0])
	}
	if resp.Entries[1].MetadataKnown {
		t.Fatalf("unknown version should not be metadata_known: %+v", resp.Entries[1])
	}

	// Duplicate version in batch returns INVALID_REQUEST
	dupReq := req
	dupReq.Versions = []VersionIDWire{wireVer, wireVer}
	_, err = fixture.client.Status(context.Background(), dupReq)
	var wire *WireError
	if !errors.As(err, &wire) || wire.Body.Code != "INVALID_REQUEST" {
		t.Fatalf("duplicate version error = %#v", err)
	}

	// Empty batch returns INVALID_REQUEST
	emptyReq := req
	emptyReq.Versions = nil
	_, err = fixture.client.Status(context.Background(), emptyReq)
	if !errors.As(err, &wire) || wire.Body.Code != "INVALID_REQUEST" {
		t.Fatalf("empty version batch error = %#v", err)
	}

	// Over-limit batch returns INVALID_REQUEST
	overReq := req
	overReq.Versions = make([]VersionIDWire, MaxVersionBatch+1)
	for i := range overReq.Versions {
		overReq.Versions[i] = VersionIDWire{AuthorID: hex.EncodeToString(fixture.serverID.DeviceID[:]), Counter: strconv.Itoa(i + 1)}
	}
	_, err = fixture.client.Status(context.Background(), overReq)
	if !errors.As(err, &wire) || wire.Body.Code != "INVALID_REQUEST" {
		t.Fatalf("over-limit batch error = %#v", err)
	}
}

func TestOrbitMembership_ReplicationGet(t *testing.T) {
	fixture := newPeerFixture(t)
	ctx := context.Background()

	// 1. Query membership as active client member
	resp, err := fixture.client.MembershipGet(ctx, MembershipGetRequest{
		ProtocolVersion: ProtocolVersion,
		FolderID:        hex.EncodeToString(fixture.folder[:]),
		DeviceID:        hex.EncodeToString(fixture.clientID.DeviceID[:]),
	})
	if err != nil {
		t.Fatalf("MembershipGet failed: %v", err)
	}
	if resp.Membership.Revision != 1 {
		t.Fatalf("expected revision 1, got %d", resp.Membership.Revision)
	}

	// 2. Query as unknown/unauthorized device returns UNAUTHORIZED
	attackerClient, err := NewClient(fixture.baseURL, fixture.attackerID, fixture.serverID.Leaf, fixture.serverID.KeyPin)
	if err != nil {
		t.Fatal(err)
	}
	_, err = attackerClient.MembershipGet(ctx, MembershipGetRequest{
		ProtocolVersion: ProtocolVersion,
		FolderID:        hex.EncodeToString(fixture.folder[:]),
		DeviceID:        hex.EncodeToString(fixture.attackerID.DeviceID[:]),
	})
	var wire *WireError
	if !errors.As(err, &wire) || wire.Body.Code != "UNAUTHORIZED" {
		t.Fatalf("expected UNAUTHORIZED, got: %v", err)
	}
}
