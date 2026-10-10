package replication

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/repository"
)

type Server struct {
	repo       *repository.DB
	identity   Identity
	now        func() time.Time
	admit      chan struct{}
	rateMu     sync.Mutex
	rateLast   time.Time
	rateTokens float64
	lan        *network.LANExchange
}

func NewServer(repo *repository.DB, identity Identity) *Server {
	return &Server{repo: repo, identity: identity, now: time.Now, admit: make(chan struct{}, 32), rateLast: time.Now(), rateTokens: 128}
}

// WithLAN serves the peer LAN exchange; without it the endpoint stays unknown,
// exactly as on peers that predate it.
func (server *Server) WithLAN(lan *network.LANExchange) *Server {
	server.lan = lan
	return server
}

func (server *Server) HTTPServer() *http.Server {
	return &http.Server{
		Handler:           server,
		TLSConfig:         server.identity.ServerTLSConfig(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
}

// Serve runs the peer API on an explicitly supplied listener and stops on
// context cancellation. Callers choose the bind interface; no firewall state
// is modified by Orbit.
func (server *Server) Serve(ctx context.Context, listener net.Listener) error {
	httpServer := server.HTTPServer()
	tlsListener := tls.NewListener(listener, httpServer.TLSConfig)
	done := make(chan error, 1)
	go func() { done <- httpServer.Serve(tlsListener) }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		err := <-done
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

type exchangeReleasesKey struct{}

func (server *Server) holdExchange(request *http.Request, folder history.ID) error {
	ctx, release, err := server.repo.BeginFolderExchange(request.Context(), folder)
	if err != nil {
		return err
	}
	releases, ok := request.Context().Value(exchangeReleasesKey{}).(*[]func())
	if !ok {
		release()
		return repository.ErrUnauthorized
	}
	*releases = append(*releases, release)
	*request = *request.WithContext(ctx)
	return nil
}

func (server *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	var releases []func()
	request = request.WithContext(context.WithValue(request.Context(), exchangeReleasesKey{}, &releases))
	defer func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	}()
	if !server.allowRequest(time.Now()) {
		writeWireError(writer, http.StatusTooManyRequests, "RETRY_EXHAUSTED", "peer request rate limit reached", true, "retry with backoff")
		return
	}
	select {
	case server.admit <- struct{}{}:
		defer func() { <-server.admit }()
	default:
		writeWireError(writer, http.StatusTooManyRequests, "RETRY_EXHAUSTED", "peer request concurrency limit reached", true, "retry with backoff")
		return
	}
	if request.Method != http.MethodPost {
		writeWireError(writer, http.StatusMethodNotAllowed, "INVALID_REQUEST", "peer endpoints require POST", false, "use the frozen endpoint method")
		return
	}
	if request.Header.Get("Content-Encoding") != "" {
		writeWireError(writer, http.StatusUnsupportedMediaType, "INVALID_REQUEST", "compressed metadata is not supported", false, "send uncompressed JSON")
		return
	}
	if request.TLS == nil || len(request.TLS.PeerCertificates) == 0 {
		writeWireError(writer, http.StatusUnauthorized, "UNAUTHORIZED", "mutual TLS client certificate required", false, "pair this device out of band")
		return
	}
	switch request.URL.Path {
	case "/peer/v1/hello":
		server.handleHello(writer, request)
	case "/peer/v1/inventory":
		server.handleInventory(writer, request)
	case "/peer/v1/versions/get":
		server.handleVersions(writer, request)
	case "/peer/v1/chunks/get":
		server.handleChunk(writer, request)
	case "/peer/v1/receipts":
		server.handleReceipts(writer, request)
	case "/peer/v1/status":
		server.handleStatus(writer, request)
	case retirementPath:
		server.handleRetirement(writer, request)
	case "/peer/v1/membership/get":
		server.handleMembershipGet(writer, request)
	case namesPath:
		server.handleNames(writer, request)
	case network.LANExchangePath:
		if server.lan == nil {
			writeWireError(writer, http.StatusNotFound, "INVALID_REQUEST", "unknown peer endpoint", false, "use a versioned peer endpoint")
			return
		}
		server.handleLAN(writer, request)
	default:
		writeWireError(writer, http.StatusNotFound, "INVALID_REQUEST", "unknown peer endpoint", false, "use a versioned peer endpoint")
	}
}

// allowRequest is a process-wide pre-parse token bucket. Per-peer scheduling
// comes later; this bound prevents malformed certificate holders from driving
// unbounded metadata parses or allocations.
func (server *Server) allowRequest(now time.Time) bool {
	server.rateMu.Lock()
	defer server.rateMu.Unlock()
	elapsed := now.Sub(server.rateLast).Seconds()
	if elapsed > 0 {
		server.rateTokens += elapsed * 64
		if server.rateTokens > 128 {
			server.rateTokens = 128
		}
		server.rateLast = now
	}
	if server.rateTokens < 1 {
		return false
	}
	server.rateTokens--
	return true
}

func readRequest(writer http.ResponseWriter, request *http.Request, value any) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, MaxMetadataBytes)
	data, err := io.ReadAll(request.Body)
	if err != nil {
		code := http.StatusBadRequest
		if strings.Contains(err.Error(), "too large") {
			code = http.StatusRequestEntityTooLarge
		}
		writeWireError(writer, code, "INVALID_REQUEST", "metadata body is malformed or exceeds 8 MiB", false, "send a bounded canonical request")
		return false
	}
	if err := protocol.DecodeStrict(data, value); err != nil {
		writeWireError(writer, http.StatusBadRequest, "INVALID_REQUEST", "metadata body is not canonical v1 JSON", false, "remove duplicate, unknown, or trailing fields")
		return false
	}
	return true
}

func (server *Server) requestIdentity(request *http.Request, deviceText string) (history.ID, history.Digest, error) {
	device, err := parseID(deviceText)
	if err != nil {
		return device, history.Digest{}, err
	}
	if request.TLS == nil || len(request.TLS.PeerCertificates) == 0 {
		return device, history.Digest{}, errors.New("missing client certificate")
	}
	return device, PublicKeyPin(request.TLS.PeerCertificates[0]), nil
}

func (server *Server) authorize(request *http.Request, deviceText, folderText, revisionText, digestText string) (history.ID, history.ID, error) {
	device, pin, err := server.requestIdentity(request, deviceText)
	if err != nil {
		return device, history.ID{}, repository.ErrUnauthorized
	}
	folder, err := parseID(folderText)
	if err != nil {
		return device, folder, repository.ErrUnauthorized
	}
	revision, err := parseDecimal(revisionText, false)
	if err != nil {
		return device, folder, repository.ErrMembershipMismatch
	}
	digest, err := parseDigest(digestText)
	if err != nil {
		return device, folder, repository.ErrMembershipMismatch
	}
	if err := server.repo.AuthorizePeer(request.Context(), folder, device, pin, revision, digest); err != nil {
		if errors.Is(err, repository.ErrPeerRetired) {
			err = server.removalError(request.Context(), folder, device)
		}
		return device, folder, err
	}
	return device, folder, server.holdExchange(request, folder)
}

func (server *Server) handleLAN(writer http.ResponseWriter, request *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, network.MaxPeerLANRequestBytes))
	if err != nil {
		writeWireError(writer, http.StatusRequestEntityTooLarge, "INVALID_REQUEST", "LAN exchange body is malformed or too large", false, "send at most one signed record per interface")
		return
	}
	reply, err := server.lan.ExchangePeerLAN(PublicKeyPin(request.TLS.PeerCertificates[0]), data)
	switch {
	case errors.Is(err, network.ErrLANExchangeUnavailable):
		writeWireError(writer, http.StatusNotFound, "INVALID_REQUEST", "unknown peer endpoint", false, "use a versioned peer endpoint")
	case errors.Is(err, network.ErrLANExchangePeer):
		writeWireError(writer, http.StatusForbidden, "UNAUTHORIZED", "LAN records are shared only with approved peers", false, "pair this device first")
	case err != nil:
		writeWireError(writer, http.StatusBadRequest, "INVALID_REQUEST", "LAN exchange records are invalid", false, "send current signed records")
	default:
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(reply)
	}
}

func (server *Server) handleHello(writer http.ResponseWriter, request *http.Request) {
	var body HelloRequest
	if !readRequest(writer, request, &body) {
		return
	}
	if body.ProtocolVersion != ProtocolVersion {
		writeWireError(writer, http.StatusUpgradeRequired, "INCOMPATIBLE_VERSION", "protocol version is not supported", false, "use protocol version 1")
		return
	}
	if len(body.Folders) == 0 || len(body.Folders) > protocol.MaxActiveMembers {
		writeWireError(writer, http.StatusBadRequest, "INVALID_REQUEST", "hello folder count is outside v1 limits", false, "send 1 through 16 folders")
		return
	}
	metadataLimit, metadataErr := parseDecimal(body.Limits.MetadataBytes, false)
	pageLimit, pageErr := parseDecimal(body.Limits.InventoryPage, false)
	if metadataErr != nil || pageErr != nil || metadataLimit > uint64(MaxMetadataBytes) || pageLimit > MaxInventoryPage {
		writeWireError(writer, http.StatusBadRequest, "INVALID_REQUEST", "advertised limits are missing or exceed v1 bounds", false, "advertise bounded v1 metadata and inventory limits")
		return
	}
	seenFolders := make(map[string]bool, len(body.Folders))
	for _, folder := range body.Folders {
		if seenFolders[folder.FolderID] {
			writeWireError(writer, http.StatusBadRequest, "INVALID_REQUEST", "hello contains a duplicate folder", false, "send each folder once")
			return
		}
		seenFolders[folder.FolderID] = true
		if _, _, err := server.authorize(request, body.DeviceID, folder.FolderID, folder.Revision, folder.MembershipDigest); err != nil {
			server.writeAuthorizationError(writer, err)
			return
		}
	}
	writeJSON(writer, http.StatusOK, HelloResponse{ProtocolVersion: ProtocolVersion, DeviceID: hex.EncodeToString(server.identity.DeviceID[:]), Folders: body.Folders, Limits: Limits{MetadataBytes: strconv.FormatInt(MaxMetadataBytes, 10), InventoryPage: strconv.Itoa(MaxInventoryPage)}})
}

func (server *Server) handleInventory(writer http.ResponseWriter, request *http.Request) {
	var body InventoryRequest
	if !readRequest(writer, request, &body) {
		return
	}
	if body.ProtocolVersion != ProtocolVersion {
		writeWireError(writer, http.StatusUpgradeRequired, "INCOMPATIBLE_VERSION", "protocol version is not supported", false, "use protocol version 1")
		return
	}
	device, folder, err := server.authorize(request, body.DeviceID, body.FolderID, body.Revision, body.MembershipDigest)
	if err != nil {
		server.writeAuthorizationError(writer, err)
		return
	}
	cursor, err := parseDecimal(body.Cursor, true)
	if err != nil || cursor > math.MaxInt64 {
		writeWireError(writer, http.StatusBadRequest, "INVALID_REQUEST", "cursor is not canonical", false, "restart at cursor 0")
		return
	}
	pageSize, err := parseDecimal(body.PageSize, false)
	if err != nil || pageSize > MaxInventoryPage {
		writeWireError(writer, http.StatusBadRequest, "INVALID_REQUEST", "page_size is outside 1..128", false, "request at most 128 summaries")
		return
	}
	var token [32]byte
	if body.SnapshotToken == "" {
		if cursor != 0 {
			writeWireError(writer, http.StatusBadRequest, "INVALID_REQUEST", "a new snapshot must start at cursor 0", false, "restart inventory")
			return
		}
		token, err = server.repo.NewInventorySnapshot(request.Context(), folder, device, server.now(), 30*time.Second, 4)
	} else {
		raw, decodeErr := hex.DecodeString(body.SnapshotToken)
		if decodeErr != nil || len(raw) != len(token) || hex.EncodeToString(raw) != body.SnapshotToken {
			writeWireError(writer, http.StatusBadRequest, "INVALID_REQUEST", "snapshot token is malformed", false, "restart inventory")
			return
		}
		copy(token[:], raw)
	}
	if err != nil {
		if errors.Is(err, repository.ErrSnapshotLimit) {
			writeWireError(writer, http.StatusTooManyRequests, "RETRY_EXHAUSTED", "open snapshot limit reached", true, "finish a snapshot or retry after expiry")
			return
		}
		writeWireError(writer, http.StatusInternalServerError, "IO_ERROR", "cannot create inventory snapshot", true, "retry later")
		return
	}
	page, err := server.repo.InventoryPage(request.Context(), token, folder, device, cursor, int(pageSize), server.now())
	if errors.Is(err, repository.ErrSnapshotExpired) {
		writeWireError(writer, http.StatusConflict, "SNAPSHOT_EXPIRED", "inventory snapshot expired", true, "restart inventory at cursor 0 without a token")
		return
	}
	if errors.Is(err, repository.ErrUnauthorized) {
		server.writeAuthorizationError(writer, err)
		return
	}
	if err != nil {
		writeWireError(writer, http.StatusInternalServerError, "IO_ERROR", "cannot read inventory snapshot", true, "retry later")
		return
	}
	response := InventoryResponse{ProtocolVersion: ProtocolVersion, SnapshotToken: hex.EncodeToString(token[:]), NextCursor: strconv.FormatUint(page.NextCursor, 10), Done: page.Done, Entries: make([]InventoryEntry, len(page.Entries))}
	for i, entry := range page.Entries {
		response.Entries[i] = InventoryEntry{VersionIDWire: VersionIDWire{AuthorID: hex.EncodeToString(entry.ID.Author[:]), Counter: strconv.FormatUint(entry.ID.Counter, 10)}, Path: entry.Path, Kind: kindName(entry.Kind), Availability: entry.ContentState, EnvelopeDigest: hex.EncodeToString(entry.EnvelopeDigest[:])}
	}
	writeJSON(writer, http.StatusOK, response)
}

func (server *Server) handleVersions(writer http.ResponseWriter, request *http.Request) {
	var body VersionsRequest
	if !readRequest(writer, request, &body) {
		return
	}
	if body.ProtocolVersion != ProtocolVersion {
		writeWireError(writer, http.StatusUpgradeRequired, "INCOMPATIBLE_VERSION", "protocol version is not supported", false, "use protocol version 1")
		return
	}
	_, folder, err := server.authorize(request, body.DeviceID, body.FolderID, body.Revision, body.MembershipDigest)
	if err != nil {
		server.writeAuthorizationError(writer, err)
		return
	}
	if len(body.Versions) == 0 || len(body.Versions) > MaxVersionBatch {
		writeWireError(writer, http.StatusBadRequest, "INVALID_REQUEST", "version batch is outside 1..128", false, "split the request")
		return
	}
	response := VersionsResponse{ProtocolVersion: ProtocolVersion, Envelopes: make([]json.RawMessage, 0, len(body.Versions))}
	responseBytes := int64(64)
	for _, requested := range body.Versions {
		author, parseErr := parseID(requested.AuthorID)
		counter, counterErr := parseDecimal(requested.Counter, false)
		if parseErr != nil || counterErr != nil {
			writeWireError(writer, http.StatusBadRequest, "INVALID_REQUEST", "version identity is malformed", false, "send canonical IDs and counters")
			return
		}
		envelope, loadErr := server.repo.Envelope(request.Context(), history.VersionID{Folder: folder, Author: author, Counter: counter})
		if loadErr != nil {
			writeWireError(writer, http.StatusNotFound, "CONTENT_UNAVAILABLE", "version metadata is unavailable", false, "refresh inventory")
			return
		}
		encoded, encodeErr := protocol.EncodeEnvelope(envelope)
		if encodeErr != nil {
			writeWireError(writer, http.StatusInternalServerError, "IO_ERROR", "stored envelope cannot be encoded", false, "run diagnostics")
			return
		}
		responseBytes += int64(len(encoded) + 1)
		if responseBytes > MaxMetadataBytes {
			writeWireError(writer, http.StatusRequestEntityTooLarge, "INVALID_REQUEST", "requested envelopes exceed the metadata response limit", false, "request fewer envelopes")
			return
		}
		response.Envelopes = append(response.Envelopes, encoded)
	}
	writeJSON(writer, http.StatusOK, response)
}

func (server *Server) handleChunk(writer http.ResponseWriter, request *http.Request) {
	var body ChunkRequest
	if !readRequest(writer, request, &body) {
		return
	}
	if body.ProtocolVersion != ProtocolVersion {
		writeWireError(writer, http.StatusUpgradeRequired, "INCOMPATIBLE_VERSION", "protocol version is not supported", false, "use protocol version 1")
		return
	}
	_, folder, err := server.authorize(request, body.DeviceID, body.FolderID, body.Revision, body.MembershipDigest)
	if err != nil {
		server.writeAuthorizationError(writer, err)
		return
	}
	author, authorErr := parseID(body.AuthorID)
	counter, counterErr := parseDecimal(body.Counter, false)
	index, indexErr := parseDecimal(body.ChunkIndex, true)
	if authorErr != nil || counterErr != nil || indexErr != nil {
		writeWireError(writer, http.StatusBadRequest, "INVALID_REQUEST", "chunk version or index is malformed", false, "use a manifest-advertised version and index")
		return
	}
	data, chunk, err := server.repo.ReadAuthorizedChunk(request.Context(), history.VersionID{Folder: folder, Author: author, Counter: counter}, index)
	if err != nil {
		writeWireError(writer, http.StatusNotFound, "CONTENT_UNAVAILABLE", "authorized manifest chunk is unavailable", false, "refresh version availability")
		return
	}
	writer.Header().Set("Content-Type", "application/octet-stream")
	writer.Header().Set("Content-Length", strconv.FormatUint(chunk.Length, 10))
	writer.Header().Set("X-Orbit-Chunk-SHA256", hex.EncodeToString(chunk.Digest[:]))
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(data)
}

func (server *Server) handleReceipts(writer http.ResponseWriter, request *http.Request) {
	var body ReceiptsRequest
	if !readRequest(writer, request, &body) {
		return
	}
	if body.ProtocolVersion != ProtocolVersion {
		writeWireError(writer, http.StatusUpgradeRequired, "INCOMPATIBLE_VERSION", "protocol version is not supported", false, "use protocol version 1")
		return
	}
	peer, folder, err := server.authorize(request, body.DeviceID, body.FolderID, body.Revision, body.MembershipDigest)
	if err != nil {
		server.writeAuthorizationError(writer, err)
		return
	}
	if len(body.Versions) == 0 || len(body.Versions) > MaxVersionBatch {
		writeWireError(writer, http.StatusBadRequest, "INVALID_REQUEST", "receipt batch is outside 1..128", false, "split the request")
		return
	}
	response := ReceiptsResponse{ProtocolVersion: ProtocolVersion, Accepted: make([]VersionIDWire, 0, len(body.Versions))}
	seen := map[string]bool{}
	for _, wireID := range body.Versions {
		id, parseErr := parseWireVersion(folder, wireID)
		key := wireID.AuthorID + ":" + wireID.Counter
		if parseErr != nil || seen[key] {
			writeWireError(writer, http.StatusBadRequest, "INVALID_REQUEST", "receipt contains a malformed or duplicate version", false, "send unique canonical version identities")
			return
		}
		seen[key] = true
		if err := server.repo.RecordPeerReceipt(request.Context(), folder, peer, id, server.now()); err != nil {
			writeWireError(writer, http.StatusNotFound, "CONTENT_UNAVAILABLE", "receipt names unknown metadata", false, "refresh inventory before acknowledging")
			return
		}
		response.Accepted = append(response.Accepted, wireID)
	}
	writeJSON(writer, http.StatusOK, response)
}

func (server *Server) handleStatus(writer http.ResponseWriter, request *http.Request) {
	var body StatusRequest
	if !readRequest(writer, request, &body) {
		return
	}
	if body.ProtocolVersion != ProtocolVersion {
		writeWireError(writer, http.StatusUpgradeRequired, "INCOMPATIBLE_VERSION", "protocol version is not supported", false, "use protocol version 1")
		return
	}
	_, folder, err := server.authorize(request, body.DeviceID, body.FolderID, body.Revision, body.MembershipDigest)
	if err != nil {
		server.writeAuthorizationError(writer, err)
		return
	}
	if len(body.Versions) == 0 || len(body.Versions) > MaxVersionBatch {
		writeWireError(writer, http.StatusBadRequest, "INVALID_REQUEST", "status batch is outside 1..128", false, "split the request")
		return
	}
	response := StatusResponse{ProtocolVersion: ProtocolVersion, Entries: make([]StatusEntry, 0, len(body.Versions))}
	seen := map[string]bool{}
	for _, wireID := range body.Versions {
		id, parseErr := parseWireVersion(folder, wireID)
		key := wireID.AuthorID + ":" + wireID.Counter
		if parseErr != nil || seen[key] {
			writeWireError(writer, http.StatusBadRequest, "INVALID_REQUEST", "status contains a malformed or duplicate version", false, "send unique canonical version identities")
			return
		}
		seen[key] = true
		status, statusErr := server.repo.VersionStatus(request.Context(), id)
		if statusErr != nil {
			writeWireError(writer, http.StatusInternalServerError, "IO_ERROR", "cannot read local version status", true, "retry later")
			return
		}
		response.Entries = append(response.Entries, StatusEntry{VersionIDWire: wireID, MetadataKnown: status.MetadataKnown, ContentState: status.ContentState, Stored: status.Stored, Applied: status.Applied, Conflict: status.Conflict, Blocked: status.Blocked})
	}
	writeJSON(writer, http.StatusOK, response)
}

func (server *Server) handleMembershipGet(writer http.ResponseWriter, request *http.Request) {
	var body MembershipGetRequest
	if !readRequest(writer, request, &body) {
		return
	}
	if body.ProtocolVersion != ProtocolVersion {
		writeWireError(writer, http.StatusUpgradeRequired, "INCOMPATIBLE_VERSION", "protocol version is not supported", false, "use protocol version 1")
		return
	}
	device, pin, err := server.requestIdentity(request, body.DeviceID)
	if err != nil {
		writeWireError(writer, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized client identity", false, "pair client certificate")
		return
	}
	folder, err := parseID(body.FolderID)
	if err != nil {
		writeWireError(writer, http.StatusBadRequest, "INVALID_REQUEST", "invalid folder ID", false, "send valid 64-hex folder ID")
		return
	}

	// Gated authorization check: peer must be an active or historical member
	isMember, err := server.repo.IsActiveOrHistoricalMember(request.Context(), folder, device)
	if err != nil || !isMember {
		writeWireError(writer, http.StatusForbidden, "UNAUTHORIZED", "peer is not a member of this folder", false, "obtain workspace membership approval")
		return
	}

	knownPin, pinErr := server.repo.DeviceKeyPin(request.Context(), device)
	if pinErr != nil || knownPin != pin {
		writeWireError(writer, http.StatusForbidden, "UNAUTHORIZED", "member key mismatch", false, "use approved device identity")
		return
	}

	if left, err := server.repo.FolderLeft(request.Context(), folder); err != nil || left {
		server.writeAuthorizationError(writer, repository.ErrFolderLeft)
		return
	}
	if err := server.holdExchange(request, folder); err != nil {
		server.writeAuthorizationError(writer, err)
		return
	}
	// Retired device check: retired devices cannot fetch membership updates (Invariant I24)
	retired, err := server.repo.IsDeviceRetired(request.Context(), folder, device)
	if err != nil {
		writeWireError(writer, http.StatusInternalServerError, "IO_ERROR", "cannot verify retirement", true, "retry later")
		return
	}
	if retired {
		server.writeAuthorizationError(writer, server.removalError(request.Context(), folder, device))
		return
	}

	membership, _, err := server.repo.GetMembership(request.Context(), folder)
	if err != nil {
		writeWireError(writer, http.StatusInternalServerError, "IO_ERROR", "cannot read local membership", true, "retry later")
		return
	}

	// Return one successor at a time. The requester must name the exact
	// predecessor it has durably approved; data routes keep their exact gate.
	if body.FromRevision != "" {
		from, parseErr := parseDecimal(body.FromRevision, false)
		if parseErr != nil || from > membership.Revision {
			server.writeAuthorizationError(writer, repository.ErrMembershipMismatch)
			return
		}
		_, prior, priorErr := server.repo.GetMembership(request.Context(), folder, from)
		if priorErr != nil || body.ExpectedDigest != hex.EncodeToString(prior.Digest[:]) {
			writeWireError(writer, http.StatusConflict, "MEMBERSHIP_FORK", "reviewed predecessor differs", false, "pause exchange and review membership recovery")
			return
		}
		if from < membership.Revision {
			membership, _, err = server.repo.GetMembership(request.Context(), folder, from+1)
			if err != nil {
				server.writeAuthorizationError(writer, err)
				return
			}
		}
	}

	snaps, _ := server.repo.ListRetirementSnapshots(request.Context(), folder, membership.Revision)

	writeJSON(writer, http.StatusOK, MembershipGetResponse{
		ProtocolVersion: ProtocolVersion,
		FolderID:        body.FolderID,
		Membership:      membership,
		Snapshots:       snaps,
	})
}

func parseWireVersion(folder history.ID, wire VersionIDWire) (history.VersionID, error) {
	author, err := parseID(wire.AuthorID)
	if err != nil {
		return history.VersionID{}, err
	}
	counter, err := parseDecimal(wire.Counter, false)
	if err != nil {
		return history.VersionID{}, err
	}
	return history.VersionID{Folder: folder, Author: author, Counter: counter}, nil
}

func (server *Server) writeAuthorizationError(writer http.ResponseWriter, err error) {
	if errors.Is(err, repository.ErrFolderLeft) {
		writeWireError(writer, http.StatusForbidden, PeerLeftCode, "this device left the folder", false, "remove this device from the Orbit")
		return
	}
	if errors.Is(err, repository.ErrPeerRetired) {
		by := ""
		var removed *peerRemovalError
		if errors.As(err, &removed) && removed.by != ([32]byte{}) {
			by = hex.EncodeToString(removed.by[:])
		}
		writeJSON(writer, http.StatusForbidden, ErrorResponse{ProtocolVersion: ProtocolVersion, Code: DeviceRemovedCode, Message: "this device was removed from the Orbit", Action: "leave the Orbit on this device; files stay", RemovedBy: by})
		return
	}
	if errors.Is(err, repository.ErrMembershipMismatch) {
		writeWireError(writer, http.StatusConflict, "MEMBERSHIP_MISMATCH", "folder membership does not match", false, "import and approve the same membership revision")
		return
	}
	writeWireError(writer, http.StatusForbidden, "UNAUTHORIZED", "peer is not authorized for this folder", false, "approve this device and key out of band")
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeWireError(writer http.ResponseWriter, status int, code, message string, retryable bool, action string) {
	writeJSON(writer, status, ErrorResponse{ProtocolVersion: ProtocolVersion, Code: code, Message: message, Retryable: retryable, Action: action})
}

// Wire codes for membership ending (2.3.0): the serving device left the
// folder, or removed the requesting device from it. Neither is retryable.
const (
	PeerLeftCode      = "PEER_LEFT"
	DeviceRemovedCode = "DEVICE_REMOVED"
)

// Removal attribution comes from the exact prepared retirement, not the relay.
type peerRemovalError struct{ by history.ID }

func (e *peerRemovalError) Error() string { return repository.ErrPeerRetired.Error() }
func (e *peerRemovalError) Unwrap() error { return repository.ErrPeerRetired }
func (server *Server) removalError(ctx context.Context, folder, device history.ID) error {
	by, _ := server.repo.RetirementActor(ctx, folder, device)
	return &peerRemovalError{by: by}
}
