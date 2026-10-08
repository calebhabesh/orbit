package replication

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
)

var (
	ErrPeerUnavailable = errors.New("temporary peer unavailability: all contacted peers were unreachable")
	ErrRepairFailed    = errors.New("repair failed: no authorized peer has valid content")
)

type RepairPeer struct {
	DeviceID history.ID
	Client   PeerClient
}

type RepairerOptions struct {
	Retries int
	Hook    func(string) error
}

type Repairer struct {
	repo    *repository.DB
	localID history.ID
	peers   []RepairPeer
	options RepairerOptions
}

func NewRepairer(repo *repository.DB, localID history.ID, peers []RepairPeer, opts ...RepairerOptions) *Repairer {
	opt := RepairerOptions{Retries: 3}
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.Retries <= 0 {
		opt.Retries = 3
	}
	return &Repairer{
		repo:    repo,
		localID: localID,
		peers:   peers,
		options: opt,
	}
}

type RepairVersionReport struct {
	VersionID      history.VersionID `json:"version_id"`
	Path           string            `json:"path"`
	Status         string            `json:"status"` // "ready", "repaired", "already_ready", "unavailable", "peer_unavailable"
	RepairedChunks int               `json:"repaired_chunks"`
	TotalChunks    int               `json:"total_chunks"`
	Message        string            `json:"message,omitempty"`
}

// RepairVersion attempts to repair all missing or corrupt chunks of targetID
// by fetching authorized chunk payloads from available peer replicas.
func (r *Repairer) RepairVersion(ctx context.Context, folder history.ID, targetID history.VersionID, preferredPeer *history.ID) (*RepairVersionReport, error) {
	if targetID.Folder != folder {
		return nil, fmt.Errorf("%w: version %v does not belong to folder %x", repository.ErrUnauthorized, targetID, folder)
	}

	// 1. Authorize folder locally
	membership, approved, err := r.repo.GetMembership(ctx, folder)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", repository.ErrUnauthorized, err)
	}
	if approved.Revision == 0 {
		return nil, repository.ErrUnauthorized
	}

	// 2. Diagnose target version
	diag, err := r.repo.DiagnoseVersionAvailability(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if diag.Status == "ready" {
		return &RepairVersionReport{
			VersionID: targetID,
			Path:      diag.Path,
			Status:    "already_ready",
			Message:   "version content is already ready and verified",
		}, nil
	}

	envelope, err := r.repo.Envelope(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if envelope.Manifest == nil || len(envelope.Manifest.Chunks) == 0 {
		return &RepairVersionReport{
			VersionID: targetID,
			Path:      envelope.Path,
			Status:    "ready",
			Message:   "no chunks required for this version",
		}, nil
	}

	// Filter candidate peers: must be active in approved membership
	activeMap := make(map[history.ID]bool, len(membership.Active))
	for _, m := range membership.Active {
		activeMap[m.Device] = true
	}

	var candidates []RepairPeer
	// If preferred peer is specified and active, prioritize it
	if preferredPeer != nil && activeMap[*preferredPeer] {
		for _, p := range r.peers {
			if p.DeviceID == *preferredPeer {
				candidates = append(candidates, p)
				break
			}
		}
	}
	for _, p := range r.peers {
		if (preferredPeer == nil || p.DeviceID != *preferredPeer) && activeMap[p.DeviceID] {
			candidates = append(candidates, p)
		}
	}

	if len(candidates) == 0 {
		return &RepairVersionReport{
			VersionID:   targetID,
			Path:        envelope.Path,
			Status:      "unavailable",
			TotalChunks: len(envelope.Manifest.Chunks),
			Message:     "no authorized active peer available for repair",
		}, ErrRepairFailed
	}

	report := &RepairVersionReport{
		VersionID:   targetID,
		Path:        envelope.Path,
		TotalChunks: len(envelope.Manifest.Chunks),
	}

	totalRepaired := 0
	anyUnreachable := false
	allUnavailable := true

	for pos, chunk := range envelope.Manifest.Chunks {
		// Check if chunk is already valid on disk
		if !r.chunkNeedsRepair(ctx, chunk) {
			continue
		}

		pinID := fmt.Sprintf("repair-%d-%x-%d", time.Now().UnixNano(), chunk.Digest[:4], pos)
		_ = r.repo.Pin(ctx, chunk.Digest, "repair", pinID)
		defer r.repo.Unpin(ctx, chunk.Digest, "repair", pinID)

		chunkRepaired := false
		request := ChunkRequest{
			ProtocolVersion:  ProtocolVersion,
			DeviceID:         hex.EncodeToString(r.localID[:]),
			FolderID:         hex.EncodeToString(folder[:]),
			Revision:         strconv.FormatUint(approved.Revision, 10),
			MembershipDigest: hex.EncodeToString(approved.Digest[:]),
			AuthorID:         hex.EncodeToString(targetID.Author[:]),
			Counter:          strconv.FormatUint(targetID.Counter, 10),
			ChunkIndex:       strconv.Itoa(pos),
		}

		for _, candidate := range candidates {
			var lastErr error
			for attempt := 0; attempt < r.options.Retries; attempt++ {
				data, err := candidate.Client.Chunk(ctx, request, chunk)
				if err == nil {
					// Verify integrity of received bytes
					if uint64(len(data)) != chunk.Length || sha256.Sum256(data) != chunk.Digest {
						_ = r.repo.RecordPeerIntegrityIncident(ctx, candidate.DeviceID, folder, chunk.Digest, "peer sent corrupt chunk data")
						lastErr = errors.New("peer sent corrupt chunk data")
						break // Do not retry infinitely on corrupt responses
					}

					if installErr := r.repo.InstallChunk(ctx, chunk.Digest, chunk.Length, bytes.NewReader(data)); installErr != nil {
						lastErr = installErr
						break
					}
					if unqErr := r.repo.UnquarantineChunk(ctx, chunk.Digest); unqErr != nil {
						lastErr = unqErr
						break
					}

					chunkRepaired = true
					allUnavailable = false
					totalRepaired++
					break
				}

				lastErr = err
				var wireErr *WireError
				if errors.As(err, &wireErr) {
					if wireErr.Status == 401 || wireErr.Body.Code == "UNAUTHORIZED" {
						return nil, repository.ErrUnauthorized
					}
					if wireErr.Body.Code == "MEMBERSHIP_MISMATCH" {
						return nil, repository.ErrMembershipMismatch
					}
					if !wireErr.Body.Retryable {
						break
					}
				}

				var netErr net.Error
				if errors.As(err, &netErr) {
					anyUnreachable = true
				}
			}

			if chunkRepaired {
				break
			}
			_ = lastErr
		}

		if !chunkRepaired {
			if anyUnreachable && allUnavailable {
				report.Status = "peer_unavailable"
				report.Message = "temporary peer unavailability: peers could not be reached"
				return report, ErrPeerUnavailable
			}
			report.Status = "unavailable"
			report.Message = "no remaining copy yields explicit unavailable state"
			return report, ErrRepairFailed
		}
	}

	report.RepairedChunks = totalRepaired
	report.Status = "repaired"
	report.Message = fmt.Sprintf("successfully repaired %d chunk(s)", totalRepaired)
	return report, nil
}

func (r *Repairer) chunkNeedsRepair(ctx context.Context, chunk history.Chunk) bool {
	if isQ, _, _ := r.repo.IsChunkQuarantined(ctx, chunk.Digest); isQ {
		return true
	}
	// Verify through repo.VerifyManifest or checking file
	manifest := &history.Manifest{
		Size:   chunk.Length,
		Chunks: []history.Chunk{chunk},
		Digest: chunk.Digest,
	}
	return r.repo.VerifyManifest(manifest) != nil
}
