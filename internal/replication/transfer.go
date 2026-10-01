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
	"strconv"
	"sync"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

const (
	MaxTransferWorkers = 4
	MaxQueuedVersions  = 1024
	MaxRetryAttempts   = 5
)

const (
	HookChunkVerified = "transfer.chunk.verified"
	HookBeforeReceipt = "transfer.receipt.before_send"
	HookAfterReceipt  = "transfer.receipt.after_send"
	HookBeforeReady   = "transfer.readiness.before"
	HookAfterReady    = "transfer.readiness.after"
)

type PeerClient interface {
	Hello(context.Context, HelloRequest) (HelloResponse, error)
	Inventory(context.Context, InventoryRequest) (InventoryResponse, error)
	Versions(context.Context, VersionsRequest) (VersionsResponse, error)
	Chunk(context.Context, ChunkRequest, history.Chunk) ([]byte, error)
	Receipts(context.Context, ReceiptsRequest) (ReceiptsResponse, error)
	Status(context.Context, StatusRequest) (StatusResponse, error)
}

type Publisher interface {
	Apply(context.Context, history.VersionID) error
}

type BandwidthLimiter interface {
	Acquire(ctx context.Context, peer *history.ID, bytes int) error
}

type TransferOptions struct {
	Workers   int
	Retries   int
	Now       func() time.Time
	Hook      func(string) error
	Fallbacks []PeerClient
	Limiter   BandwidthLimiter
}

type Syncer struct {
	repo       *repository.DB
	publisher  Publisher
	client     PeerClient
	local      history.ID
	peer       history.ID
	folder     history.ID
	membership repository.ApprovedMembership
	workers    int
	retries    int
	now        func() time.Time
	hook       func(string) error
	fallbacks  []PeerClient
	limiter    BandwidthLimiter
}

type SyncResult struct {
	Inventoried     int `json:"inventoried"`
	MetadataAdded   int `json:"metadata_added"`
	ChunksFetched   int `json:"chunks_fetched"`
	ChunksReused    int `json:"chunks_reused"`
	VersionsStored  int `json:"versions_stored"`
	ReceiptsSent    int `json:"receipts_sent"`
	VersionsApplied int `json:"versions_applied"`
}

func NewSyncer(repo *repository.DB, publisher Publisher, client PeerClient, local, peer, folder history.ID, membership repository.ApprovedMembership, options TransferOptions) *Syncer {
	if options.Workers <= 0 || options.Workers > MaxTransferWorkers {
		options.Workers = MaxTransferWorkers
	}
	if options.Retries <= 0 || options.Retries > MaxRetryAttempts {
		options.Retries = MaxRetryAttempts
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return &Syncer{repo: repo, publisher: publisher, client: client, local: local, peer: peer, folder: folder, membership: membership, workers: options.Workers, retries: options.Retries, now: options.Now, hook: options.Hook, fallbacks: options.Fallbacks, limiter: options.Limiter}
}

func (syncer *Syncer) callHook(name string) error {
	if syncer.hook == nil {
		return nil
	}
	return syncer.hook(name)
}

func (syncer *Syncer) handshake() FolderHandshake {
	return FolderHandshake{FolderID: hex.EncodeToString(syncer.folder[:]), Revision: strconv.FormatUint(syncer.membership.Revision, 10), MembershipDigest: hex.EncodeToString(syncer.membership.Digest[:])}
}

func (syncer *Syncer) common() (string, string, string, string) {
	h := syncer.handshake()
	return hex.EncodeToString(syncer.local[:]), h.FolderID, h.Revision, h.MembershipDigest
}

func (syncer *Syncer) Sync(ctx context.Context) (SyncResult, error) {
	var result SyncResult
	device, folder, revision, digest := syncer.common()
	hello, err := syncer.client.Hello(ctx, HelloRequest{ProtocolVersion: ProtocolVersion, DeviceID: device, Folders: []FolderHandshake{syncer.handshake()}, Limits: Limits{MetadataBytes: strconv.FormatInt(MaxMetadataBytes, 10), InventoryPage: strconv.Itoa(MaxInventoryPage)}})
	if err != nil {
		return result, err
	}
	if hello.DeviceID != hex.EncodeToString(syncer.peer[:]) {
		return result, errors.New("authenticated peer returned a different device identity")
	}
	entries, err := syncer.inventory(ctx, device, folder, revision, digest)
	if err != nil {
		return result, err
	}
	defer entries.Close()
	// The snapshot is completely received before slow storage work. Replay the
	// bounded disk spool in two passes, preserving metadata-before-content rules.
	visit := func(run func(InventoryEntry) error) error {
		if _, err := entries.Seek(0, io.SeekStart); err != nil {
			return err
		}
		decoder := json.NewDecoder(entries)
		for {
			var entry InventoryEntry
			if err := decoder.Decode(&entry); errors.Is(err, io.EOF) {
				return nil
			} else if err != nil {
				return err
			}
			if err := run(entry); err != nil {
				return err
			}
		}
	}
	if err := visit(func(entry InventoryEntry) error {
		result.Inventoried++
		id, err := parseInventoryID(syncer.folder, entry)
		if err != nil {
			return err
		}
		known, err := syncer.repo.MetadataKnown(ctx, id)
		if err != nil {
			return err
		}
		if !known {
			// Cache only this ancestry fetch, never the entire folder's envelopes.
			cache := map[history.VersionID]history.Envelope{}
			visiting := map[history.VersionID]bool{}
			added, err := syncer.importVersion(ctx, id, entry.EnvelopeDigest, cache, visiting, 0)
			if err != nil {
				return err
			}
			result.MetadataAdded += added
		}
		return nil
	}); err != nil {
		return result, err
	}
	if err := visit(func(entry InventoryEntry) error {
		if entry.Availability != "ready" {
			return nil
		}
		id, err := parseInventoryID(syncer.folder, entry)
		if err != nil {
			return err
		}
		ready, err := syncer.repo.ContentReady(ctx, id)
		if err != nil {
			return err
		}
		if !ready {
			fetched, reused, err := syncer.fetchVersion(ctx, id)
			if err != nil {
				return err
			}
			result.ChunksFetched += fetched
			result.ChunksReused += reused
			result.VersionsStored++
		}
		if err := syncer.sendReceipt(ctx, id); err != nil {
			return err
		}
		result.ReceiptsSent++
		return nil
	}); err != nil {
		return result, err
	}

	if syncer.publisher != nil {
		heads, err := syncer.repo.UnappliedSingleHeads(ctx, syncer.folder)
		if err != nil {
			return result, err
		}
		for _, id := range heads {
			if err := syncer.publisher.Apply(ctx, id); err != nil {
				if errors.Is(err, workspace.ErrStructuralConflict) {
					continue
				}
				return result, err
			}
			result.VersionsApplied++
		}
	}
	if err := syncer.refreshPeerStatus(ctx); err != nil {
		return result, err
	}
	return result, nil
}

func (syncer *Syncer) inventory(ctx context.Context, device, folder, revision, digest string) (*repository.InventorySpool, error) {
	spool, err := syncer.repo.NewInventorySpool(ctx)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			_ = spool.Close()
		}
	}()
	for restarts := 0; restarts < 3; restarts++ {
		if err := spool.Truncate(0); err != nil {
			return nil, err
		}
		if _, err := spool.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		encoder := json.NewEncoder(spool)
		token, cursor := "", "0"
		for {
			response, err := syncer.client.Inventory(ctx, InventoryRequest{ProtocolVersion: ProtocolVersion, DeviceID: device, FolderID: folder, Revision: revision, MembershipDigest: digest, SnapshotToken: token, Cursor: cursor, PageSize: strconv.Itoa(MaxInventoryPage)})
			if err != nil {
				var wire *WireError
				if errors.As(err, &wire) && wire.Body.Code == "SNAPSHOT_EXPIRED" {
					break
				}
				return nil, err
			}
			if len(response.Entries) > MaxInventoryPage {
				return nil, errors.New("peer inventory page exceeds requested bound")
			}
			for _, entry := range response.Entries {
				if err := encoder.Encode(entry); err != nil {
					return nil, err
				}
			}
			if response.Done {
				success = true
				return spool, nil
			}
			next, err := strconv.ParseUint(response.NextCursor, 10, 64)
			prior, priorErr := strconv.ParseUint(cursor, 10, 64)
			if err != nil || priorErr != nil || next <= prior || response.SnapshotToken == "" {
				return nil, errors.New("peer inventory cursor did not advance")
			}
			token, cursor = response.SnapshotToken, response.NextCursor
		}
	}
	return nil, errors.New("peer inventory snapshot repeatedly expired")
}

func parseInventoryID(folder history.ID, entry InventoryEntry) (history.VersionID, error) {
	return parseWireVersion(folder, entry.VersionIDWire)
}

func (syncer *Syncer) importVersion(ctx context.Context, id history.VersionID, advertisedDigest string, cache map[history.VersionID]history.Envelope, visiting map[history.VersionID]bool, depth int) (int, error) {
	if depth > 64 || len(cache) >= MaxQueuedVersions {
		return 0, errors.New("peer ancestry exceeds transfer bounds")
	}
	known, err := syncer.repo.MetadataKnown(ctx, id)
	if err != nil || known {
		return 0, err
	}
	if visiting[id] {
		return 0, errors.New("peer ancestry contains a cycle")
	}
	visiting[id] = true
	defer delete(visiting, id)
	envelope, ok := cache[id]
	if !ok {
		device, folder, revision, digest := syncer.common()
		response, err := syncer.client.Versions(ctx, VersionsRequest{ProtocolVersion: ProtocolVersion, DeviceID: device, FolderID: folder, Revision: revision, MembershipDigest: digest, Versions: []VersionIDWire{wireVersion(id)}})
		if err != nil {
			return 0, err
		}
		if len(response.Envelopes) != 1 {
			return 0, errors.New("peer returned an incomplete version batch")
		}
		envelope, err = protocol.DecodeEnvelope(response.Envelopes[0])
		if err != nil || envelope.ID != id || envelope.ID.Folder != syncer.folder {
			return 0, errors.New("peer returned a mismatched version envelope")
		}
		fingerprint := repository.EnvelopeDigest(envelope)
		if advertisedDigest != "" && hex.EncodeToString(fingerprint[:]) != advertisedDigest {
			return 0, errors.New("peer envelope does not match its inventory digest")
		}
		cache[id] = envelope
	}
	added := 0
	for _, parent := range envelope.Parents {
		count, err := syncer.importVersion(ctx, parent, "", cache, visiting, depth+1)
		if err != nil {
			return added, err
		}
		added += count
	}
	if err := syncer.repo.ImportMetadata(ctx, envelope); err != nil {
		return added, err
	}
	return added + 1, nil
}

func (syncer *Syncer) fetchVersion(ctx context.Context, id history.VersionID) (int, int, error) {
	envelope, err := syncer.repo.Envelope(ctx, id)
	if err != nil {
		return 0, 0, err
	}
	if envelope.Manifest == nil {
		return 0, 0, errors.New("ready file transfer has no manifest")
	}
	transferID := stableTransferID(syncer.peer, id)
	var missingBytes uint64
	for _, chunk := range uniqueChunks(envelope.Manifest.Chunks) {
		available, err := syncer.repo.VerifiedChunk(ctx, chunk)
		if err != nil {
			return 0, 0, err
		}
		if !available {
			missingBytes += chunk.Length
		}
	}
	if err := syncer.repo.BeginTransfer(ctx, repository.Transfer{ID: transferID, Folder: syncer.folder, Peer: syncer.peer, Version: id, ReservedBytes: missingBytes}, envelope.Manifest); err != nil {
		return 0, 0, err
	}
	verified, err := syncer.repo.TransferVerifiedPositions(ctx, transferID)
	if err != nil {
		return 0, 0, err
	}
	type chunkJob struct {
		chunk     history.Chunk
		positions []int
		fetchAt   int
	}
	jobsByDigest := map[history.Digest]*chunkJob{}
	var jobList []*chunkJob
	reused := 0
	for position, chunk := range envelope.Manifest.Chunks {
		available, err := syncer.repo.VerifiedChunk(ctx, chunk)
		if err != nil {
			return 0, reused, err
		}
		if available {
			if !verified[position] {
				if err := syncer.repo.MarkTransferChunkVerified(ctx, transferID, position, chunk); err != nil {
					return 0, reused, err
				}
			}
			reused++
			continue
		}
		job := jobsByDigest[chunk.Digest]
		if job == nil {
			job = &chunkJob{chunk: chunk, fetchAt: position}
			jobsByDigest[chunk.Digest] = job
			jobList = append(jobList, job)
		} else {
			reused++
		}
		if job.chunk.Length != chunk.Length {
			return 0, reused, errors.New("same chunk digest has inconsistent lengths")
		}
		job.positions = append(job.positions, position)
	}
	jobs := make(chan *chunkJob)
	errCh := make(chan error, 1)
	var fetched int
	var fetchedMu sync.Mutex
	var workers sync.WaitGroup
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	for i := 0; i < syncer.workers; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobs {
				if err := syncer.fetchChunk(workerCtx, id, job.fetchAt, job.chunk); err != nil {
					select {
					case errCh <- err:
						cancel()
					default:
					}
					return
				}
				for _, position := range job.positions {
					if err := syncer.repo.MarkTransferChunkVerified(workerCtx, transferID, position, job.chunk); err != nil {
						select {
						case errCh <- err:
							cancel()
						default:
						}
						return
					}
					if err := syncer.callHook(HookChunkVerified); err != nil {
						select {
						case errCh <- err:
							cancel()
						default:
						}
						return
					}
				}
				fetchedMu.Lock()
				fetched++
				fetchedMu.Unlock()
			}
		}()
	}
sendJobs:
	for _, job := range jobList {
		select {
		case jobs <- job:
		case <-workerCtx.Done():
			break sendJobs
		}
	}
	close(jobs)
	workers.Wait()
	select {
	case err := <-errCh:
		_ = syncer.repo.FailTransfer(ctx, transferID, err)
		return fetched, reused, err
	default:
	}
	if err := syncer.callHook(HookBeforeReady); err != nil {
		_ = syncer.repo.FailTransfer(ctx, transferID, err)
		return fetched, reused, err
	}
	if err := syncer.repo.MarkContentReady(ctx, id); err != nil {
		_ = syncer.repo.FailTransfer(ctx, transferID, err)
		return fetched, reused, err
	}
	if err := syncer.callHook(HookAfterReady); err != nil {
		return fetched, reused, err
	}
	if err := syncer.repo.CompleteTransfer(ctx, transferID); err != nil {
		return fetched, reused, err
	}
	return fetched, reused, nil
}

func (syncer *Syncer) fetchChunk(ctx context.Context, id history.VersionID, position int, chunk history.Chunk) error {
	device, folder, revision, digest := syncer.common()
	request := ChunkRequest{ProtocolVersion: ProtocolVersion, DeviceID: device, FolderID: folder, Revision: revision, MembershipDigest: digest, AuthorID: hex.EncodeToString(id.Author[:]), Counter: strconv.FormatUint(id.Counter, 10), ChunkIndex: strconv.Itoa(position)}
	var last error
	for attempt := 0; attempt < syncer.retries; attempt++ {
		data, err := syncer.client.Chunk(ctx, request, chunk)
		if err == nil {
			if syncer.limiter != nil {
				if lErr := syncer.limiter.Acquire(ctx, &syncer.peer, len(data)); lErr != nil {
					return lErr
				}
			}
			return syncer.repo.InstallChunk(ctx, chunk.Digest, chunk.Length, bytes.NewReader(data))
		}
		last = err
		var wire *WireError
		if errors.As(err, &wire) && !wire.Body.Retryable {
			break
		}
		if attempt+1 < syncer.retries {
			delay := time.Duration(1<<attempt) * 10 * time.Millisecond
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	for _, fallback := range syncer.fallbacks {
		data, err := fallback.Chunk(ctx, request, chunk)
		if err == nil {
			return syncer.repo.InstallChunk(ctx, chunk.Digest, chunk.Length, bytes.NewReader(data))
		}
		last = err
	}

	return fmt.Errorf("RETRY_EXHAUSTED after %d attempts: %w", syncer.retries, last)
}

func (syncer *Syncer) sendReceipt(ctx context.Context, id history.VersionID) error {
	ready, err := syncer.repo.CanIssueDurableReceipt(ctx, id)
	if err != nil || !ready {
		return fmt.Errorf("cannot issue receipt before durable readiness: %w", err)
	}
	if err := syncer.callHook(HookBeforeReceipt); err != nil {
		return err
	}
	device, folder, revision, digest := syncer.common()
	response, err := syncer.client.Receipts(ctx, ReceiptsRequest{ProtocolVersion: ProtocolVersion, DeviceID: device, FolderID: folder, Revision: revision, MembershipDigest: digest, Versions: []VersionIDWire{wireVersion(id)}})
	if err != nil {
		return err
	}
	if len(response.Accepted) != 1 || response.Accepted[0] != wireVersion(id) {
		return errors.New("peer did not confirm the durable receipt")
	}
	return syncer.callHook(HookAfterReceipt)
}

func (syncer *Syncer) refreshPeerStatus(ctx context.Context) error {
	ids, err := syncer.repo.VersionIDs(ctx, syncer.folder)
	if err != nil {
		return err
	}
	device, folder, revision, digest := syncer.common()
	for start := 0; start < len(ids); start += MaxVersionBatch {
		end := start + MaxVersionBatch
		if end > len(ids) {
			end = len(ids)
		}
		versions := make([]VersionIDWire, end-start)
		for i, id := range ids[start:end] {
			versions[i] = wireVersion(id)
		}
		response, err := syncer.client.Status(ctx, StatusRequest{ProtocolVersion: ProtocolVersion, DeviceID: device, FolderID: folder, Revision: revision, MembershipDigest: digest, Versions: versions})
		if err != nil {
			return err
		}
		for _, entry := range response.Entries {
			id, err := parseWireVersion(syncer.folder, entry.VersionIDWire)
			if err != nil {
				return err
			}
			state := entry.ContentState
			if entry.Applied {
				state += ",applied"
			}
			if entry.Conflict {
				state += ",conflict"
			}
			if entry.Blocked {
				state += ",blocked"
			}
			if !entry.MetadataKnown {
				state = "unknown"
			}
			if err := syncer.repo.RecordPeerStatus(ctx, syncer.folder, syncer.peer, id, state, syncer.now()); err != nil {
				return err
			}
		}
	}
	return nil
}

func wireVersion(id history.VersionID) VersionIDWire {
	return VersionIDWire{AuthorID: hex.EncodeToString(id.Author[:]), Counter: strconv.FormatUint(id.Counter, 10)}
}

func stableTransferID(peer history.ID, id history.VersionID) string {
	h := sha256.New()
	h.Write([]byte("filesync-transfer-v1\x00"))
	h.Write(peer[:])
	h.Write(id.Folder[:])
	h.Write(id.Author[:])
	h.Write([]byte(strconv.FormatUint(id.Counter, 10)))
	return "transfer-" + hex.EncodeToString(h.Sum(nil))
}

func uniqueChunks(chunks []history.Chunk) []history.Chunk {
	seen := map[history.Digest]history.Chunk{}
	var result []history.Chunk
	for _, chunk := range chunks {
		if prior, ok := seen[chunk.Digest]; ok {
			if prior.Length != chunk.Length {
				continue
			}
			continue
		}
		seen[chunk.Digest] = chunk
		result = append(result, chunk)
	}
	return result
}
