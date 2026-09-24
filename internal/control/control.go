package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

type FaultHook func(name string) error

type Options struct {
	FaultHook      FaultHook
	Now            func() time.Time
	IdempotencyTTL time.Duration
	LocalDevice    history.ID
	RepairPeers    []replication.RepairPeer
}

type Controller struct {
	db      *repository.DB
	ws      *workspace.Workspace
	options Options
}

func New(db *repository.DB, ws *workspace.Workspace, opts ...Options) *Controller {
	opt := Options{}
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.IdempotencyTTL == 0 {
		opt.IdempotencyTTL = 24 * time.Hour
	}
	return &Controller{
		db:      db,
		ws:      ws,
		options: opt,
	}
}

func (c *Controller) callHook(name string) error {
	if c.options.FaultHook != nil {
		return c.options.FaultHook(name)
	}
	return nil
}

func (c *Controller) checkIdempotency(ctx context.Context, key string, req any) (*repository.ControlOpRecord, history.Digest, error) {
	if key == "" {
		return nil, history.Digest{}, nil
	}
	reqDigest, err := requestDigest(req)
	if err != nil {
		return nil, history.Digest{}, err
	}
	record, storedDigest, _, err := c.db.GetControlOperation(ctx, key)
	if errors.Is(err, repository.ErrExpiredReplay) {
		return nil, reqDigest, ExpiredReplayError(key)
	}
	if err != nil {
		return nil, reqDigest, err
	}
	if record != nil {
		if storedDigest != reqDigest {
			return nil, reqDigest, IdempotencyConflictError(key)
		}
	}
	return record, reqDigest, nil
}

func (c *Controller) saveIdempotency(ctx context.Context, key string, digest history.Digest, record *repository.ControlOpRecord) error {
	if key == "" {
		return nil
	}
	expiresAt := c.options.Now().Add(c.options.IdempotencyTTL)
	return c.db.PutControlOperation(ctx, key, digest, 0, record, expiresAt)
}

func (c *Controller) verifyCurrentHeads(ctx context.Context, folder history.ID, path string, reviewed []history.VersionID, expectedToken history.Digest) error {
	currentHeads, err := c.currentPathHeads(ctx, folder, path)
	if err != nil {
		return err
	}
	token := history.HeadToken(currentHeads)
	if token != expectedToken || !sameIDs(currentHeads, reviewed) {
		return StaleViewError(currentHeads, token)
	}
	return nil
}

func (c *Controller) currentPathHeads(ctx context.Context, folder history.ID, path string) ([]history.VersionID, error) {
	heads, err := c.db.Heads(ctx, folder, path)
	if err != nil {
		return nil, err
	}
	ids := make([]history.VersionID, len(heads))
	for i, h := range heads {
		ids[i] = h.ID
	}
	return ids, nil
}

func (c *Controller) verifyContentAvailability(ctx context.Context, id history.VersionID) error {
	state, err := c.db.ContentAvailability(ctx, id)
	if err != nil {
		return err
	}
	switch state {
	case repository.ContentReady:
		return nil
	case repository.ContentPending:
		return ContentPendingError(fmt.Sprintf("version %v content is still pending transfer", id))
	case repository.ContentExpired:
		return ContentExpiredError(fmt.Sprintf("version %v content has expired under retention policy", id))
	case repository.ContentUnavailable:
		return ContentUnavailableError(fmt.Sprintf("version %v content is unavailable locally", id))
	default:
		return ContentUnavailableError(fmt.Sprintf("version %v content status: %s", id, state))
	}
}

func (c *Controller) checkCollision(ctx context.Context, folder history.ID, path string) error {
	active, err := c.db.PathActiveInRepository(ctx, folder, path)
	if err != nil {
		return err
	}
	if active {
		return DestinationCollisionError(path)
	}
	if c.ws != nil {
		exists, err := c.ws.PathExists(ctx, folder, path)
		if err != nil {
			return err
		}
		if exists {
			return DestinationCollisionError(path)
		}
	}
	return nil
}

// ResolveSelect resolves conflicting heads by selecting one of the reviewed heads.
func (c *Controller) ResolveSelect(ctx context.Context, req ResolveSelectRequest) (*ResolveResult, error) {
	if req.Folder == (history.ID{}) || req.Path == "" || len(req.Reviewed) == 0 {
		return nil, fmt.Errorf("%w: folder, path, and reviewed heads are required", ErrInvalidRequest)
	}
	if !containsID(req.Reviewed, req.Selected) {
		return nil, fmt.Errorf("%w: selected version %v is not among reviewed heads", ErrInvalidRequest, req.Selected)
	}

	record, reqDigest, err := c.checkIdempotency(ctx, req.IdempotencyKey, req)
	if err != nil {
		return nil, err
	}
	if record != nil && record.Status == "completed" && record.Envelope != nil {
		return &ResolveResult{
			Folder:     req.Folder,
			Path:       req.Path,
			Action:     "select",
			ResolvedID: *record.ResolvedID,
			Envelope:   *record.Envelope,
			Applied:    record.Applied,
			Replay:     true,
		}, nil
	}

	if err := c.verifyCurrentHeads(ctx, req.Folder, req.Path, req.Reviewed, req.ExpectedHeadToken); err != nil {
		return nil, err
	}

	if err := c.verifyContentAvailability(ctx, req.Selected); err != nil {
		return nil, err
	}

	selectedEnv, err := c.db.Envelope(ctx, req.Selected)
	if err != nil {
		return nil, err
	}

	resReq := repository.ResolutionVersionRequest{
		Folder:            req.Folder,
		Path:              req.Path,
		Reviewed:          req.Reviewed,
		ExpectedHeadToken: req.ExpectedHeadToken,
		Kind:              selectedEnv.Kind,
		Manifest:          selectedEnv.Manifest,
		DisplayTime:       c.options.Now().UTC().Format(time.RFC3339Nano),
	}

	envelope, err := c.db.CreateResolutionVersion(ctx, resReq)
	if errors.Is(err, history.ErrStaleView) {
		heads, _ := c.currentPathHeads(ctx, req.Folder, req.Path)
		return nil, StaleViewError(heads, history.HeadToken(heads))
	}
	if err != nil {
		return nil, err
	}

	applied := false
	if c.ws != nil {
		if err := c.ws.Apply(ctx, envelope.ID); err == nil {
			applied = true
		}
	}

	if err := c.callHook("control.select.committed"); err != nil {
		return nil, err
	}

	now := c.options.Now()
	opRecord := &repository.ControlOpRecord{
		Status:      "completed",
		Action:      "select",
		ResolvedID:  &envelope.ID,
		Envelope:    &envelope,
		Applied:     applied,
		CompletedAt: &now,
	}
	if err := c.saveIdempotency(ctx, req.IdempotencyKey, reqDigest, opRecord); err != nil {
		return nil, err
	}

	return &ResolveResult{
		Folder:     req.Folder,
		Path:       req.Path,
		Action:     "select",
		ResolvedID: envelope.ID,
		Envelope:   envelope,
		Applied:    applied,
	}, nil
}

// ResolveManualMerge resolves conflicting heads by supplying new merged content.
func (c *Controller) ResolveManualMerge(ctx context.Context, req ResolveMergeRequest) (*ResolveResult, error) {
	if req.Folder == (history.ID{}) || req.Path == "" || len(req.Reviewed) == 0 {
		return nil, fmt.Errorf("%w: folder, path, and reviewed heads are required", ErrInvalidRequest)
	}

	record, reqDigest, err := c.checkIdempotency(ctx, req.IdempotencyKey, req)
	if err != nil {
		return nil, err
	}
	if record != nil && record.Status == "completed" && record.Envelope != nil {
		return &ResolveResult{
			Folder:     req.Folder,
			Path:       req.Path,
			Action:     "merge",
			ResolvedID: *record.ResolvedID,
			Envelope:   *record.Envelope,
			Applied:    record.Applied,
			Replay:     true,
		}, nil
	}

	if err := c.verifyCurrentHeads(ctx, req.Folder, req.Path, req.Reviewed, req.ExpectedHeadToken); err != nil {
		return nil, err
	}

	var reader io.Reader
	if req.ContentReader != nil {
		reader = req.ContentReader
	} else {
		reader = bytes.NewReader(req.Content)
	}

	manifest, err := c.installContent(ctx, reader, req.Executable)
	if err != nil {
		return nil, err
	}

	resReq := repository.ResolutionVersionRequest{
		Folder:            req.Folder,
		Path:              req.Path,
		Reviewed:          req.Reviewed,
		ExpectedHeadToken: req.ExpectedHeadToken,
		Kind:              history.KindFile,
		Manifest:          manifest,
		DisplayTime:       c.options.Now().UTC().Format(time.RFC3339Nano),
	}

	envelope, err := c.db.CreateResolutionVersion(ctx, resReq)
	if errors.Is(err, history.ErrStaleView) {
		heads, _ := c.currentPathHeads(ctx, req.Folder, req.Path)
		return nil, StaleViewError(heads, history.HeadToken(heads))
	}
	if err != nil {
		return nil, err
	}

	applied := false
	if c.ws != nil {
		if err := c.ws.Apply(ctx, envelope.ID); err == nil {
			applied = true
		}
	}

	if err := c.callHook("control.merge.committed"); err != nil {
		return nil, err
	}

	now := c.options.Now()
	opRecord := &repository.ControlOpRecord{
		Status:      "completed",
		Action:      "merge",
		ResolvedID:  &envelope.ID,
		Envelope:    &envelope,
		Applied:     applied,
		CompletedAt: &now,
	}
	if err := c.saveIdempotency(ctx, req.IdempotencyKey, reqDigest, opRecord); err != nil {
		return nil, err
	}

	return &ResolveResult{
		Folder:     req.Folder,
		Path:       req.Path,
		Action:     "merge",
		ResolvedID: envelope.ID,
		Envelope:   envelope,
		Applied:    applied,
	}, nil
}

// Restore creates a new version from a historical version's content, setting its causal
// parents to the reviewed current heads of the path.
func (c *Controller) Restore(ctx context.Context, req RestoreRequest) (*ResolveResult, error) {
	if req.Folder == (history.ID{}) || req.Path == "" || req.SourceVersion == (history.VersionID{}) || len(req.Reviewed) == 0 {
		return nil, fmt.Errorf("%w: folder, path, source version, and reviewed heads are required", ErrInvalidRequest)
	}

	record, reqDigest, err := c.checkIdempotency(ctx, req.IdempotencyKey, req)
	if err != nil {
		return nil, err
	}
	if record != nil && record.Status == "completed" && record.Envelope != nil {
		return &ResolveResult{
			Folder:     req.Folder,
			Path:       req.Path,
			Action:     "restore",
			ResolvedID: *record.ResolvedID,
			Envelope:   *record.Envelope,
			Applied:    record.Applied,
			Replay:     true,
		}, nil
	}

	if err := c.verifyCurrentHeads(ctx, req.Folder, req.Path, req.Reviewed, req.ExpectedHeadToken); err != nil {
		return nil, err
	}

	if err := c.verifyContentAvailability(ctx, req.SourceVersion); err != nil {
		return nil, err
	}

	sourceEnv, err := c.db.Envelope(ctx, req.SourceVersion)
	if err != nil {
		return nil, err
	}

	resReq := repository.ResolutionVersionRequest{
		Folder:            req.Folder,
		Path:              req.Path,
		Reviewed:          req.Reviewed,
		ExpectedHeadToken: req.ExpectedHeadToken,
		Kind:              sourceEnv.Kind,
		Manifest:          sourceEnv.Manifest,
		DisplayTime:       c.options.Now().UTC().Format(time.RFC3339Nano),
	}

	envelope, err := c.db.CreateResolutionVersion(ctx, resReq)
	if errors.Is(err, history.ErrStaleView) {
		heads, _ := c.currentPathHeads(ctx, req.Folder, req.Path)
		return nil, StaleViewError(heads, history.HeadToken(heads))
	}
	if err != nil {
		return nil, err
	}

	applied := false
	if c.ws != nil {
		if err := c.ws.Apply(ctx, envelope.ID); err == nil {
			applied = true
		}
	}

	if err := c.callHook("control.restore.committed"); err != nil {
		return nil, err
	}

	now := c.options.Now()
	opRecord := &repository.ControlOpRecord{
		Status:      "completed",
		Action:      "restore",
		ResolvedID:  &envelope.ID,
		Envelope:    &envelope,
		Applied:     applied,
		CompletedAt: &now,
	}
	if err := c.saveIdempotency(ctx, req.IdempotencyKey, reqDigest, opRecord); err != nil {
		return nil, err
	}

	return &ResolveResult{
		Folder:     req.Folder,
		Path:       req.Path,
		Action:     "restore",
		ResolvedID: envelope.ID,
		Envelope:   envelope,
		Applied:    applied,
	}, nil
}

// PreviewRestore inspects what would be restored from sourceVersion without modifying history or workspace.
func (c *Controller) PreviewRestore(ctx context.Context, req RestorePreviewRequest) (*RestorePreview, error) {
	if req.Folder == (history.ID{}) || req.Path == "" || req.SourceVersion == (history.VersionID{}) {
		return nil, fmt.Errorf("%w: folder, path, and source version are required", ErrInvalidRequest)
	}

	sourceEnv, err := c.db.Envelope(ctx, req.SourceVersion)
	if err != nil {
		return nil, err
	}

	state, err := c.db.ContentAvailability(ctx, req.SourceVersion)
	if err != nil {
		return nil, err
	}

	currentHeads, err := c.currentPathHeads(ctx, req.Folder, req.Path)
	if err != nil {
		return nil, err
	}

	token := history.HeadToken(currentHeads)
	size := uint64(0)
	digest := history.Digest{}
	executable := false
	if sourceEnv.Manifest != nil {
		size = sourceEnv.Manifest.Size
		digest = sourceEnv.Manifest.Digest
		executable = sourceEnv.Manifest.Executable
	}

	return &RestorePreview{
		Path:              req.Path,
		SourceVersion:     req.SourceVersion,
		SourceKind:        sourceEnv.Kind,
		SourceSize:        size,
		SourceDigest:      digest,
		SourceExecutable:  executable,
		ContentState:      state,
		CurrentHeads:      currentHeads,
		ExpectedHeadToken: token,
	}, nil
}

// ResolveKeepCopies preserves conflicting heads at separate destination paths,
// then resolves the original path with reviewed heads.
func (c *Controller) ResolveKeepCopies(ctx context.Context, req KeepCopiesRequest) (*KeepCopiesResult, error) {
	if req.Folder == (history.ID{}) || req.Path == "" || len(req.Reviewed) == 0 {
		return nil, fmt.Errorf("%w: folder, path, and reviewed heads are required", ErrInvalidRequest)
	}

	copies := req.Copies
	if len(copies) == 0 {
		for _, h := range req.Reviewed {
			copies = append(copies, CopyTarget{
				HeadID:          h,
				DestinationPath: autoDestinationPath(req.Path, h),
			})
		}
	}

	for _, target := range copies {
		if err := history.ValidatePath(target.DestinationPath); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidRequest, err)
		}
		if target.DestinationPath == req.Path {
			return nil, fmt.Errorf("%w: destination path cannot equal original path", ErrInvalidRequest)
		}
	}

	record, reqDigest, err := c.checkIdempotency(ctx, req.IdempotencyKey, req)
	if err != nil {
		return nil, err
	}
	if record != nil && record.Status == "completed" && record.Envelope != nil {
		return &KeepCopiesResult{
			Folder:     req.Folder,
			Path:       req.Path,
			ResolvedID: *record.ResolvedID,
			Envelope:   *record.Envelope,
			Copies:     record.Copies,
			Completed:  true,
			Replay:     true,
		}, nil
	}

	completedSteps := make(map[string]repository.CopyStepResult)
	if record != nil {
		for _, s := range record.Copies {
			if s.Completed {
				completedSteps[s.DestinationPath] = s
			}
		}
	}

	for _, target := range copies {
		if _, done := completedSteps[target.DestinationPath]; done {
			continue
		}
		if err := c.checkCollision(ctx, req.Folder, target.DestinationPath); err != nil {
			return nil, err
		}
	}

	var stepResults []repository.CopyStepResult
	for _, target := range copies {
		if existing, done := completedSteps[target.DestinationPath]; done {
			stepResults = append(stepResults, existing)
			continue
		}

		if err := c.verifyContentAvailability(ctx, target.HeadID); err != nil {
			return nil, err
		}
		headEnv, err := c.db.Envelope(ctx, target.HeadID)
		if err != nil {
			return nil, err
		}

		copyEnv, err := c.db.CreateCopyVersion(ctx, repository.CopyVersionRequest{
			Folder:           req.Folder,
			Path:             target.DestinationPath,
			Kind:             headEnv.Kind,
			Manifest:         headEnv.Manifest,
			AuthoredRevision: 0,
			DisplayTime:      c.options.Now().UTC().Format(time.RFC3339Nano),
		})
		if err != nil {
			return nil, fmt.Errorf("create copy for %s: %w", target.DestinationPath, err)
		}

		if c.ws != nil {
			if err := c.ws.Apply(ctx, copyEnv.ID); err != nil {
				return nil, fmt.Errorf("apply copy for %s: %w", target.DestinationPath, err)
			}
		}

		stepResult := repository.CopyStepResult{
			HeadID:          target.HeadID,
			DestinationPath: target.DestinationPath,
			VersionID:       copyEnv.ID,
			Completed:       true,
		}
		stepResults = append(stepResults, stepResult)

		if err := c.callHook("control.keep_copies.step"); err != nil {
			inProgressRecord := &repository.ControlOpRecord{
				Status: "in_progress",
				Action: "keep_copies",
				Copies: stepResults,
			}
			_ = c.saveIdempotency(ctx, req.IdempotencyKey, reqDigest, inProgressRecord)
			return nil, err
		}

		inProgressRecord := &repository.ControlOpRecord{
			Status: "in_progress",
			Action: "keep_copies",
			Copies: stepResults,
		}
		if err := c.saveIdempotency(ctx, req.IdempotencyKey, reqDigest, inProgressRecord); err != nil {
			return nil, err
		}
	}

	if err := c.verifyCurrentHeads(ctx, req.Folder, req.Path, req.Reviewed, req.ExpectedHeadToken); err != nil {
		return nil, err
	}

	selectedHead := req.Reviewed[0]
	if req.OriginalSelected != nil {
		selectedHead = *req.OriginalSelected
	}
	selectedEnv, err := c.db.Envelope(ctx, selectedHead)
	if err != nil {
		return nil, err
	}

	resReq := repository.ResolutionVersionRequest{
		Folder:            req.Folder,
		Path:              req.Path,
		Reviewed:          req.Reviewed,
		ExpectedHeadToken: req.ExpectedHeadToken,
		Kind:              selectedEnv.Kind,
		Manifest:          selectedEnv.Manifest,
		DisplayTime:       c.options.Now().UTC().Format(time.RFC3339Nano),
	}

	resolvedEnv, err := c.db.CreateResolutionVersion(ctx, resReq)
	if errors.Is(err, history.ErrStaleView) {
		heads, _ := c.currentPathHeads(ctx, req.Folder, req.Path)
		return nil, StaleViewError(heads, history.HeadToken(heads))
	}
	if err != nil {
		return nil, err
	}

	applied := false
	if c.ws != nil {
		if err := c.ws.Apply(ctx, resolvedEnv.ID); err == nil {
			applied = true
		}
	}

	if err := c.callHook("control.keep_copies.committed"); err != nil {
		return nil, err
	}

	now := c.options.Now()
	finalRecord := &repository.ControlOpRecord{
		Status:      "completed",
		Action:      "keep_copies",
		ResolvedID:  &resolvedEnv.ID,
		Envelope:    &resolvedEnv,
		Applied:     applied,
		Copies:      stepResults,
		CompletedAt: &now,
	}
	if err := c.saveIdempotency(ctx, req.IdempotencyKey, reqDigest, finalRecord); err != nil {
		return nil, err
	}

	return &KeepCopiesResult{
		Folder:     req.Folder,
		Path:       req.Path,
		ResolvedID: resolvedEnv.ID,
		Envelope:   resolvedEnv,
		Copies:     stepResults,
		Completed:  true,
	}, nil
}

// Export streams a verified version payload to destination with whole-file verification.
func (c *Controller) Export(ctx context.Context, folder history.ID, id history.VersionID, destination io.Writer) error {
	if folder == (history.ID{}) || id == (history.VersionID{}) {
		return fmt.Errorf("%w: folder and version ID are required", ErrInvalidRequest)
	}
	if err := c.verifyContentAvailability(ctx, id); err != nil {
		return err
	}
	_, err := c.db.WriteVersion(ctx, id, destination)
	return err
}

// History reports historical versions for path with accurate content availability.
func (c *Controller) History(ctx context.Context, folder history.ID, path string) ([]HistoryItem, error) {
	if folder == (history.ID{}) || path == "" {
		return nil, fmt.Errorf("%w: folder and path are required", ErrInvalidRequest)
	}
	envelopes, err := c.db.PathHistory(ctx, folder, path)
	if err != nil {
		return nil, err
	}
	heads, err := c.currentPathHeads(ctx, folder, path)
	if err != nil {
		return nil, err
	}
	headMap := make(map[history.VersionID]bool, len(heads))
	for _, h := range heads {
		headMap[h] = true
	}

	var items []HistoryItem
	for _, env := range envelopes {
		state, err := c.db.ContentAvailability(ctx, env.ID)
		if err != nil {
			return nil, err
		}
		applied, err := c.db.WorkingApplied(ctx, env.ID)
		if err != nil {
			return nil, err
		}
		items = append(items, HistoryItem{
			ID:           env.ID,
			Path:         env.Path,
			Kind:         env.Kind,
			Parents:      env.Parents,
			Vector:       env.Vector,
			Manifest:     env.Manifest,
			DisplayTime:  env.DisplayTime,
			ContentState: state,
			IsHead:       headMap[env.ID],
			Applied:      applied,
		})
	}
	return items, nil
}

// Files returns current active projected files in folder.
func (c *Controller) Files(ctx context.Context, folder history.ID) ([]FileItem, error) {
	if folder == (history.ID{}) {
		return nil, fmt.Errorf("%w: folder is required", ErrInvalidRequest)
	}
	projections, err := c.db.Projections(ctx, folder)
	if err != nil {
		return nil, err
	}
	var items []FileItem
	for _, p := range projections {
		if p.Kind == history.KindTombstone {
			continue
		}
		items = append(items, FileItem{
			Path:        p.Path,
			Kind:        p.Kind,
			Size:        p.ObservedSize,
			MtimeNS:     p.ObservedMtimeNS,
			Inode:       p.ObservedInode,
			Executable:  p.Executable,
			BlockReason: p.BlockReason,
		})
	}
	return items, nil
}

// Conflicts returns active multi-head conflict sets and structural conflicts.
func (c *Controller) Conflicts(ctx context.Context, folder history.ID) ([]repository.ConflictSet, []history.StructuralConflict, error) {
	conflicts, err := c.db.Conflicts(ctx, folder)
	if err != nil {
		return nil, nil, err
	}
	structural, err := c.db.StructuralConflicts(ctx, folder)
	if err != nil {
		return nil, nil, err
	}
	return conflicts, structural, nil
}

func (c *Controller) installContent(ctx context.Context, reader io.Reader, executable bool) (*history.Manifest, error) {
	const chunkSize = 1024 * 1024
	buf := make([]byte, chunkSize)
	whole := sha256.New()
	var chunks []history.Chunk
	var totalSize uint64

	for {
		n, err := io.ReadFull(reader, buf)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return nil, err
		}
		if n == 0 {
			break
		}
		totalSize += uint64(n)
		part := buf[:n]
		whole.Write(part)
		chunkDigest := history.Digest(sha256.Sum256(part))
		chunks = append(chunks, history.Chunk{
			Digest: chunkDigest,
			Length: uint64(n),
		})
		if err := c.db.InstallChunk(ctx, chunkDigest, uint64(n), bytes.NewReader(part)); err != nil {
			return nil, err
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
	}

	manifestDigest := history.Digest(whole.Sum(nil))
	return &history.Manifest{
		Size:       totalSize,
		Digest:     manifestDigest,
		Chunks:     chunks,
		Executable: executable,
	}, nil
}

func requestDigest(req any) (history.Digest, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return history.Digest{}, err
	}
	h := sha256.Sum256(append([]byte("filesync-control-op-v1\x00"), data...))
	return history.Digest(h), nil
}

func containsID(ids []history.VersionID, target history.VersionID) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func sameIDs(a, b []history.VersionID) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]history.VersionID(nil), a...)
	bb := append([]history.VersionID(nil), b...)
	sort.Slice(aa, func(i, j int) bool { return history.CompareVersionID(aa[i], aa[j]) < 0 })
	sort.Slice(bb, func(i, j int) bool { return history.CompareVersionID(bb[i], bb[j]) < 0 })
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}

func autoDestinationPath(originalPath string, head history.VersionID) string {
	dir := ""
	base := originalPath
	if idx := strings.LastIndex(originalPath, "/"); idx >= 0 {
		dir = originalPath[:idx+1]
		base = originalPath[idx+1:]
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	authorShort := hex.EncodeToString(head.Author[:4])
	return fmt.Sprintf("%s%s (conflict %s-%d)%s", dir, stem, authorShort, head.Counter, ext)
}

func (c *Controller) MembershipExport(ctx context.Context, folder history.ID, revision ...uint64) (MembershipExportResult, error) {
	m, app, err := c.db.GetMembership(ctx, folder, revision...)
	if err != nil {
		return MembershipExportResult{}, err
	}
	snaps, err := c.db.ListRetirementSnapshots(ctx, folder, m.Revision)
	if err != nil {
		return MembershipExportResult{}, err
	}
	return MembershipExportResult{
		Membership: m,
		Digest:     app.Digest,
		Snapshots:  snaps,
	}, nil
}

func (c *Controller) MembershipImport(ctx context.Context, folder history.ID, membership protocol.Membership, snapshots []protocol.RetirementSnapshot, approve bool) (repository.ApprovedMembership, error) {
	if membership.Folder != folder {
		return repository.ApprovedMembership{}, errors.New("imported membership folder mismatch")
	}
	digest, err := protocol.MembershipDigest(membership)
	if err != nil {
		return repository.ApprovedMembership{}, err
	}
	if !approve {
		return repository.ApprovedMembership{Revision: membership.Revision, Digest: digest}, nil
	}
	return c.db.ApproveMembership(ctx, membership, snapshots...)
}

func (c *Controller) MembershipPreview(ctx context.Context, folder history.ID, next protocol.Membership) (MembershipPreviewResult, error) {
	if next.Folder != folder {
		return MembershipPreviewResult{}, errors.New("membership preview folder mismatch")
	}
	nextDigest, err := protocol.MembershipDigest(next)
	if err != nil {
		return MembershipPreviewResult{}, err
	}
	cur, curApp, err := c.db.GetMembership(ctx, folder)
	var curRevision uint64
	var curDigest history.Digest
	if err == nil {
		curRevision = cur.Revision
		curDigest = curApp.Digest
	}

	result := MembershipPreviewResult{
		Folder:          folder,
		CurrentRevision: curRevision,
		CurrentDigest:   curDigest,
		NextRevision:    next.Revision,
		NextDigest:      nextDigest,
	}

	if curRevision == 0 {
		if next.Revision == 1 && next.PriorDigest == (history.Digest{}) {
			result.PriorDigestMatches = true
			result.ValidTransition = true
			for _, m := range next.Active {
				result.AddedActive = append(result.AddedActive, m.Device)
			}
		} else {
			result.ValidTransition = false
			result.Reason = "initial revision must be revision 1 with zero prior digest"
		}
		return result, nil
	}

	result.PriorDigestMatches = (next.PriorDigest == curDigest)
	if next.Revision != curRevision+1 {
		result.ValidTransition = false
		result.Reason = fmt.Sprintf("next revision %d is not sequential after current revision %d", next.Revision, curRevision)
		return result, nil
	}
	if !result.PriorDigestMatches {
		result.ValidTransition = false
		result.Reason = "prior digest does not match locally approved digest"
		return result, nil
	}

	curActive := make(map[history.ID]bool)
	for _, m := range cur.Active {
		curActive[m.Device] = true
	}
	nextActive := make(map[history.ID]bool)
	for _, m := range next.Active {
		nextActive[m.Device] = true
		if !curActive[m.Device] {
			result.AddedActive = append(result.AddedActive, m.Device)
		}
	}
	for _, m := range cur.Active {
		if !nextActive[m.Device] {
			result.RemovedActive = append(result.RemovedActive, m.Device)
		}
	}
	curRetired := make(map[history.ID]bool)
	for _, m := range cur.Retired {
		curRetired[m.Device] = true
	}
	for _, m := range next.Retired {
		if !curRetired[m.Device] {
			result.NewlyRetired = append(result.NewlyRetired, m.Device)
		}
	}
	result.ValidTransition = true
	return result, nil
}

func (c *Controller) RetireMemberPreview(ctx context.Context, folder, targetDevice history.ID) (RetireMemberPreview, error) {
	cur, curApp, err := c.db.GetMembership(ctx, folder)
	if err != nil {
		return RetireMemberPreview{}, err
	}
	var targetActive bool
	var survivors []history.ID
	for _, m := range cur.Active {
		if m.Device == targetDevice {
			targetActive = true
		} else {
			survivors = append(survivors, m.Device)
		}
	}
	if !targetActive {
		return RetireMemberPreview{}, errors.New("target device is not an active member")
	}

	versions, err := c.db.VersionsByAuthor(ctx, folder, targetDevice)
	if err != nil {
		return RetireMemberPreview{}, err
	}

	snapshot := protocol.RetirementSnapshot{
		Folder:            folder,
		ConfigurationRev:  cur.Revision,
		RetiredDevice:     targetDevice,
		AcceptedByRetiree: versions,
	}
	snapDigest, err := protocol.RetirementSnapshotDigest(snapshot)
	if err != nil {
		return RetireMemberPreview{}, err
	}

	var activeMembers []protocol.ActiveMember
	for _, m := range cur.Active {
		if m.Device != targetDevice {
			activeMembers = append(activeMembers, m)
		}
	}
	retiredMembers := append([]protocol.RetiredMember(nil), cur.Retired...)
	retiredMembers = append(retiredMembers, protocol.RetiredMember{
		Device:         targetDevice,
		RetiredAt:      cur.Revision + 1,
		SnapshotDigest: snapDigest,
	})

	nextMembership := protocol.Membership{
		Folder:      folder,
		Revision:    cur.Revision + 1,
		PriorDigest: curApp.Digest,
		Active:      activeMembers,
		Retired:     retiredMembers,
	}
	nextDigest, err := protocol.MembershipDigest(nextMembership)
	if err != nil {
		return RetireMemberPreview{}, err
	}

	return RetireMemberPreview{
		Folder:                folder,
		TargetDevice:          targetDevice,
		CurrentRevision:       cur.Revision,
		NextRevision:          cur.Revision + 1,
		AcceptedVersionsCount: len(versions),
		SnapshotDigest:        snapDigest,
		NextMembershipDigest:  nextDigest,
		SurvivingMembers:      survivors,
	}, nil
}

func (c *Controller) RetireMemberExecute(ctx context.Context, req RetireMemberRequest) (RetireMemberResult, error) {
	if req.Folder == (history.ID{}) || req.TargetDevice == (history.ID{}) {
		return RetireMemberResult{}, errors.New("folder and target device must be non-zero")
	}
	record, reqDigest, err := c.checkIdempotency(ctx, req.IdempotencyKey, req)
	if err != nil {
		return RetireMemberResult{}, err
	}
	if record != nil && record.Status == "completed" {
		var cached RetireMemberResult
		if len(record.Payload) > 0 {
			_ = json.Unmarshal(record.Payload, &cached)
		}
		cached.Replay = true
		return cached, nil
	}

	preview, err := c.RetireMemberPreview(ctx, req.Folder, req.TargetDevice)
	if err != nil {
		return RetireMemberResult{}, err
	}

	versions, err := c.db.VersionsByAuthor(ctx, req.Folder, req.TargetDevice)
	if err != nil {
		return RetireMemberResult{}, err
	}

	snapshot := protocol.RetirementSnapshot{
		Folder:            req.Folder,
		ConfigurationRev:  preview.CurrentRevision,
		RetiredDevice:     req.TargetDevice,
		AcceptedByRetiree: versions,
	}

	cur, curApp, err := c.db.GetMembership(ctx, req.Folder)
	if err != nil {
		return RetireMemberResult{}, err
	}
	var activeMembers []protocol.ActiveMember
	for _, m := range cur.Active {
		if m.Device != req.TargetDevice {
			activeMembers = append(activeMembers, m)
		}
	}
	retiredMembers := append([]protocol.RetiredMember(nil), cur.Retired...)
	retiredMembers = append(retiredMembers, protocol.RetiredMember{
		Device:         req.TargetDevice,
		RetiredAt:      cur.Revision + 1,
		SnapshotDigest: preview.SnapshotDigest,
	})
	nextMembership := protocol.Membership{
		Folder:      req.Folder,
		Revision:    cur.Revision + 1,
		PriorDigest: curApp.Digest,
		Active:      activeMembers,
		Retired:     retiredMembers,
	}

	maintID := fmt.Sprintf("retire-%x-%d", req.TargetDevice[:8], cur.Revision+1)
	maintPayload, _ := json.Marshal(nextMembership)
	if err := c.db.SaveResumableMaintenance(ctx, maintID, req.Folder, req.TargetDevice, "PROPOSED", maintPayload, c.options.Now()); err != nil {
		return RetireMemberResult{}, err
	}

	approved, err := c.db.ApproveMembership(ctx, nextMembership, snapshot)
	if err != nil {
		return RetireMemberResult{}, err
	}

	_ = c.db.SaveResumableMaintenance(ctx, maintID, req.Folder, req.TargetDevice, "COMPLETED", maintPayload, c.options.Now())

	result := RetireMemberResult{
		Folder:           req.Folder,
		TargetDevice:     req.TargetDevice,
		ApprovedRevision: approved.Revision,
		ApprovedDigest:   approved.Digest,
		Snapshot:         snapshot,
		NextMembership:   nextMembership,
	}

	if req.IdempotencyKey != "" {
		payload, _ := json.Marshal(result)
		now := c.options.Now()
		opRecord := &repository.ControlOpRecord{
			Status:      "completed",
			Action:      "retire_member",
			Payload:     payload,
			CompletedAt: &now,
		}
		_ = c.saveIdempotency(ctx, req.IdempotencyKey, reqDigest, opRecord)
	}

	return result, nil
}

func (c *Controller) PeerList(ctx context.Context, folder history.ID) (PeerListResult, error) {
	active, retired, rev, digest, err := c.db.PeerMembers(ctx, folder)
	if err != nil {
		if errors.Is(err, repository.ErrFolderUnknown) || errors.Is(err, sql.ErrNoRows) {
			return PeerListResult{
				Folder:  folder,
				Active:  []protocol.ActiveMember{},
				Retired: []protocol.RetiredMember{},
			}, nil
		}
		return PeerListResult{}, err
	}
	if active == nil {
		active = []protocol.ActiveMember{}
	}
	if retired == nil {
		retired = []protocol.RetiredMember{}
	}
	return PeerListResult{
		Folder:   folder,
		Revision: rev,
		Digest:   digest,
		Active:   active,
		Retired:  retired,
	}, nil
}

func (c *Controller) EnrollPreview(ctx context.Context, folder history.ID, rootPath string) (EnrollPreviewResult, error) {
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return EnrollPreviewResult{}, err
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return EnrollPreviewResult{}, fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return EnrollPreviewResult{}, errors.New("root path is not a directory")
	}

	diskFiles := make(map[string]history.Digest)
	err = filepath.Walk(absRoot, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if fi.Name() == ".filesync-internal" {
				return filepath.SkipDir
			}
			return nil
		}
		if !fi.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(absRoot, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		diskFiles[rel] = sha256.Sum256(data)
		return nil
	})
	if err != nil {
		return EnrollPreviewResult{}, err
	}

	projections, err := c.db.Projections(ctx, folder)
	if err != nil && !errors.Is(err, repository.ErrFolderUnknown) {
		return EnrollPreviewResult{}, err
	}

	projMap := make(map[string]repository.Projection)
	for _, pr := range projections {
		projMap[pr.Path] = pr
	}

	res := EnrollPreviewResult{
		Folder:          folder,
		RootPath:        absRoot,
		LocalFilesCount: len(diskFiles),
	}

	for p, diskDigest := range diskFiles {
		res.ExistingPaths = append(res.ExistingPaths, p)
		if proj, ok := projMap[p]; ok {
			if proj.Kind == history.KindFile {
				if proj.Digest == diskDigest {
					res.IdenticalPaths = append(res.IdenticalPaths, p)
				} else {
					res.ConflictingPaths = append(res.ConflictingPaths, p)
				}
			} else {
				res.ConflictingPaths = append(res.ConflictingPaths, p)
			}
		}
	}

	for p, proj := range projMap {
		if _, ok := diskFiles[p]; !ok {
			if proj.Kind == history.KindFile || proj.Kind == history.KindDirectory {
				res.RemoteOnlyPaths = append(res.RemoteOnlyPaths, p)
			}
		}
	}

	sort.Strings(res.ExistingPaths)
	sort.Strings(res.IdenticalPaths)
	sort.Strings(res.ConflictingPaths)
	sort.Strings(res.RemoteOnlyPaths)

	return res, nil
}

func (c *Controller) EnrollBootstrap(ctx context.Context, folder history.ID, rootPath string) (EnrollBootstrapResult, error) {
	if c.ws == nil {
		return EnrollBootstrapResult{}, errors.New("workspace is required for enrollment bootstrap")
	}
	_, err := c.ws.Register(ctx, folder, rootPath)
	if err != nil && !strings.Contains(err.Error(), "already has a registered root") {
		return EnrollBootstrapResult{}, err
	}
	scanRes, err := c.ws.Scan(ctx, folder)
	if err != nil {
		return EnrollBootstrapResult{}, err
	}
	return EnrollBootstrapResult{
		Folder:        folder,
		RootPath:      rootPath,
		CapturedCount: len(scanRes.Captured),
		Envelopes:     scanRes.Captured,
	}, nil
}

func (c *Controller) StorageUsage(ctx context.Context) (StorageUsageResult, error) {
	usage, err := c.db.DetailedStorageUsage(ctx)
	if err != nil {
		return StorageUsageResult{}, err
	}
	return StorageUsageResult{Usage: usage}, nil
}

func (c *Controller) RetentionPreview(ctx context.Context, req RetentionPreviewRequest) (RetentionPreviewResult, error) {
	var policy *repository.RetentionPolicy
	if req.RetentionDays != nil || req.MinSuperseded != nil {
		p := repository.DefaultRetentionPolicy()
		if req.RetentionDays != nil {
			p.RetentionDays = *req.RetentionDays
		}
		if req.MinSuperseded != nil {
			p.MinSuperseded = *req.MinSuperseded
		}
		policy = &p
	}
	preview, err := c.db.RetentionPreview(ctx, req.Folder, policy, time.Now())
	if err != nil {
		return RetentionPreviewResult{}, err
	}
	return RetentionPreviewResult{Preview: *preview}, nil
}

func (c *Controller) RetentionChange(ctx context.Context, req RetentionChangeRequest) (RetentionChangeResult, error) {
	if req.RetentionDays < 0 || req.MinSuperseded < 0 {
		return RetentionChangeResult{}, errors.New("retention days and min superseded must be non-negative")
	}
	policy := repository.RetentionPolicy{
		RetentionDays: req.RetentionDays,
		MinSuperseded: req.MinSuperseded,
	}
	if err := c.db.SetRetentionPolicy(ctx, req.Folder, policy); err != nil {
		return RetentionChangeResult{}, err
	}
	return RetentionChangeResult{
		Folder: req.Folder,
		Policy: policy,
	}, nil
}

func (c *Controller) GCPreview(ctx context.Context, req GCPreviewRequest) (GCPreviewResult, error) {
	var policy *repository.RetentionPolicy
	if req.RetentionDays != nil || req.MinSuperseded != nil {
		p := repository.DefaultRetentionPolicy()
		if req.RetentionDays != nil {
			p.RetentionDays = *req.RetentionDays
		}
		if req.MinSuperseded != nil {
			p.MinSuperseded = *req.MinSuperseded
		}
		policy = &p
	}
	report, err := c.db.GCPreview(ctx, req.Folder, policy, time.Now())
	if err != nil {
		return GCPreviewResult{}, err
	}
	return GCPreviewResult{Report: *report}, nil
}

func (c *Controller) GCRun(ctx context.Context, req GCRunRequest) (GCRunResult, error) {
	record, reqDigest, err := c.checkIdempotency(ctx, req.IdempotencyKey, req)
	if err != nil {
		return GCRunResult{}, err
	}
	if record != nil && record.Status == "completed" {
		var cached GCRunResult
		if len(record.Payload) > 0 {
			_ = json.Unmarshal(record.Payload, &cached)
		}
		cached.Replay = true
		return cached, nil
	}

	var policy *repository.RetentionPolicy
	if req.RetentionDays != nil || req.MinSuperseded != nil {
		p := repository.DefaultRetentionPolicy()
		if req.RetentionDays != nil {
			p.RetentionDays = *req.RetentionDays
		}
		if req.MinSuperseded != nil {
			p.MinSuperseded = *req.MinSuperseded
		}
		policy = &p
	}

	report, err := c.db.RunGC(ctx, req.Folder, policy, time.Now())
	if err != nil {
		return GCRunResult{}, err
	}

	result := GCRunResult{Report: *report}
	if req.IdempotencyKey != "" {
		payload, _ := json.Marshal(result)
		now := c.options.Now()
		opRecord := &repository.ControlOpRecord{
			Status:      "completed",
			Action:      "gc_run",
			Payload:     payload,
			CompletedAt: &now,
		}
		_ = c.saveIdempotency(ctx, req.IdempotencyKey, reqDigest, opRecord)
	}

	return result, nil
}

func (c *Controller) ReclaimRecoveryCopies(ctx context.Context, req ReclaimRecoveryRequest) (ReclaimRecoveryResult, error) {
	if c.ws == nil {
		return ReclaimRecoveryResult{}, errors.New("workspace is required for recovery reclaim")
	}
	count, bytesReclaimed, err := c.ws.ReclaimRecoveryCopies(ctx, req.Folder)
	if err != nil {
		return ReclaimRecoveryResult{}, err
	}
	return ReclaimRecoveryResult{
		Folder:         req.Folder,
		ReclaimedCount: count,
		ReclaimedBytes: bytesReclaimed,
	}, nil
}

func (c *Controller) StorageIntegrityCheck(ctx context.Context, req StorageCheckRequest) (StorageCheckResult, error) {
	res, err := c.db.CheckIntegrity(ctx, repository.IntegrityCheckRequest{
		Folder:         req.Folder,
		Path:           req.Path,
		VersionID:      req.VersionID,
		AutoQuarantine: req.AutoQuarantine,
		Limit:          req.Limit,
	})
	if err != nil {
		return StorageCheckResult{}, err
	}
	return StorageCheckResult{
		Folder:             res.Folder,
		TotalChunksChecked: res.TotalChunksChecked,
		CleanChunks:        res.CleanChunks,
		CorruptChunks:      res.CorruptChunks,
		MissingProtected:   res.MissingProtected,
		ExpiredHistorical:  res.ExpiredHistorical,
		DurationNS:         res.DurationNS,
	}, nil
}

func (c *Controller) StorageRepair(ctx context.Context, req StorageRepairRequest) (StorageRepairResult, error) {
	record, reqDigest, err := c.checkIdempotency(ctx, req.IdempotencyKey, req)
	if err != nil {
		return StorageRepairResult{}, err
	}
	if record != nil && record.Status == "completed" {
		var cached StorageRepairResult
		if len(record.Payload) > 0 {
			_ = json.Unmarshal(record.Payload, &cached)
		}
		cached.Replay = true
		return cached, nil
	}

	peers := req.Peers
	if len(peers) == 0 {
		peers = c.options.RepairPeers
	}

	repairer := replication.NewRepairer(c.db, c.options.LocalDevice, peers)
	report, err := repairer.RepairVersion(ctx, req.Folder, req.VersionID, req.PreferredPeer)
	if err != nil {
		if report != nil {
			return StorageRepairResult{
				VersionID:      report.VersionID,
				Path:           report.Path,
				Status:         report.Status,
				RepairedChunks: report.RepairedChunks,
				TotalChunks:    report.TotalChunks,
				Message:        report.Message,
			}, err
		}
		return StorageRepairResult{}, err
	}

	result := StorageRepairResult{
		VersionID:      report.VersionID,
		Path:           report.Path,
		Status:         report.Status,
		RepairedChunks: report.RepairedChunks,
		TotalChunks:    report.TotalChunks,
		Message:        report.Message,
	}

	if req.IdempotencyKey != "" {
		payload, _ := json.Marshal(result)
		now := c.options.Now()
		opRecord := &repository.ControlOpRecord{
			Status:      "completed",
			Action:      "storage_repair",
			Payload:     payload,
			CompletedAt: &now,
		}
		_ = c.saveIdempotency(ctx, req.IdempotencyKey, reqDigest, opRecord)
	}

	return result, nil
}

func (c *Controller) WorkScan(ctx context.Context, req WorkScanRequest) (*WorkScanResult, error) {
	var reqDigest history.Digest
	if req.IdempotencyKey != "" {
		existing, digest, err := c.checkIdempotency(ctx, req.IdempotencyKey, req)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			var replayResult WorkScanResult
			if err := json.Unmarshal(existing.Payload, &replayResult); err != nil {
				return nil, fmt.Errorf("unmarshal cached work scan result: %w", err)
			}
			replayResult.Replay = true
			return &replayResult, nil
		}
		reqDigest = digest
	}

	start := time.Now()
	var folders []history.ID
	if req.Folder != nil {
		folders = append(folders, *req.Folder)
	} else {
		registered, err := c.db.RegisteredFolders(ctx)
		if err != nil {
			return nil, err
		}
		for _, reg := range registered {
			folders = append(folders, reg.Folder)
		}
	}

	combinedResult := &WorkScanResult{
		FullContent: req.FullContent,
	}

	for _, folder := range folders {
		combinedResult.Folder = folder
		scanRes, err := c.ws.ScanWithOptions(ctx, folder, workspace.ScanOptions{FullContent: req.FullContent})
		if err != nil {
			return nil, err
		}
		combinedResult.CapturedCount += len(scanRes.Captured)
		combinedResult.CapturedEnvelopes = append(combinedResult.CapturedEnvelopes, scanRes.Captured...)
		combinedResult.Issues = append(combinedResult.Issues, scanRes.Issues...)
		if scanRes.Deletion != nil {
			combinedResult.Deletion = scanRes.Deletion
		}
	}
	combinedResult.DurationNS = time.Since(start).Nanoseconds()

	if req.IdempotencyKey != "" {
		payload, _ := json.Marshal(combinedResult)
		now := c.options.Now()
		opRecord := &repository.ControlOpRecord{
			Status:      "completed",
			Action:      "work_scan",
			Payload:     payload,
			CompletedAt: &now,
		}
		_ = c.saveIdempotency(ctx, req.IdempotencyKey, reqDigest, opRecord)
	}

	return combinedResult, nil
}

func (c *Controller) WorkSync(ctx context.Context, req WorkSyncRequest, client replication.PeerClient, localID history.ID, membership repository.ApprovedMembership) (*WorkSyncResult, error) {
	var reqDigest history.Digest
	if req.IdempotencyKey != "" {
		existing, digest, err := c.checkIdempotency(ctx, req.IdempotencyKey, req)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			var replayResult WorkSyncResult
			if err := json.Unmarshal(existing.Payload, &replayResult); err != nil {
				return nil, fmt.Errorf("unmarshal cached work sync result: %w", err)
			}
			replayResult.Replay = true
			return &replayResult, nil
		}
		reqDigest = digest
	}

	peer := history.ID{}
	if req.PeerDevice != nil {
		peer = *req.PeerDevice
	}

	syncer := replication.NewSyncer(c.db, c.ws, client, localID, peer, req.Folder, membership, replication.TransferOptions{})
	syncRes, err := syncer.Sync(ctx)
	if err != nil {
		return nil, err
	}

	result := &WorkSyncResult{
		Folder:          req.Folder,
		PeerDevice:      peer,
		Inventoried:     syncRes.Inventoried,
		MetadataAdded:   syncRes.MetadataAdded,
		ChunksFetched:   syncRes.ChunksFetched,
		ChunksReused:    syncRes.ChunksReused,
		VersionsStored:  syncRes.VersionsStored,
		ReceiptsSent:    syncRes.ReceiptsSent,
		VersionsApplied: syncRes.VersionsApplied,
	}

	if req.IdempotencyKey != "" {
		payload, _ := json.Marshal(result)
		now := c.options.Now()
		opRecord := &repository.ControlOpRecord{
			Status:      "completed",
			Action:      "work_sync",
			Payload:     payload,
			CompletedAt: &now,
		}
		_ = c.saveIdempotency(ctx, req.IdempotencyKey, reqDigest, opRecord)
	}

	return result, nil
}

func (c *Controller) WorkStatus(ctx context.Context, req WorkStatusRequest) (*WorkStatusResult, error) {
	var folders []history.ID
	if req.Folder != nil {
		folders = append(folders, *req.Folder)
	} else {
		registered, err := c.db.RegisteredFolders(ctx)
		if err != nil {
			return nil, err
		}
		for _, reg := range registered {
			folders = append(folders, reg.Folder)
		}
	}

	result := &WorkStatusResult{}
	for _, f := range folders {
		tasks, err := c.db.ListDurableTasks(ctx, repository.TaskFilter{Folder: f})
		if err != nil {
			return nil, err
		}
		status := FolderWorkStatus{Folder: f}
		for _, t := range tasks {
			switch t.State {
			case "queued":
				status.QueuedTasks++
			case "running":
				status.RunningTasks++
			case "retry":
				status.RetryTasks++
			case "exhausted":
				status.ExhaustedTasks++
				if t.ErrorCode == "ROOT_UNAVAILABLE" {
					status.Paused = true
					status.PauseReason = "ROOT_UNAVAILABLE"
				}
			}
		}
		result.Folders = append(result.Folders, status)
	}
	return result, nil
}

func (c *Controller) WorkRetry(ctx context.Context, req WorkRetryRequest) (*WorkRetryResult, error) {
	if req.TaskID != "" {
		if err := c.db.RetryDurableTask(ctx, req.TaskID); err != nil {
			return nil, err
		}
		return &WorkRetryResult{
			RetriedCount: 1,
			Message:      fmt.Sprintf("retried task %s", req.TaskID),
		}, nil
	}

	var folderPtr *history.ID
	if req.Folder != (history.ID{}) {
		folderPtr = &req.Folder
	}
	count, err := c.db.RetryAllExhaustedTasks(ctx, folderPtr)
	if err != nil {
		return nil, err
	}
	return &WorkRetryResult{
		RetriedCount: count,
		Message:      fmt.Sprintf("retried %d tasks", count),
	}, nil
}

func (c *Controller) WorkCancel(ctx context.Context, req WorkCancelRequest) (*WorkCancelResult, error) {
	if req.TaskID == "" {
		return nil, errors.New("task_id is required")
	}
	if err := c.db.CancelDurableTask(ctx, req.TaskID); err != nil {
		return nil, err
	}
	return &WorkCancelResult{
		TaskID:  req.TaskID,
		Status:  "canceled",
		Message: fmt.Sprintf("task %s canceled successfully", req.TaskID),
	}, nil
}

func (c *Controller) WorkList(ctx context.Context, req WorkListRequest) (*WorkListResult, error) {
	tasks, err := c.db.ListDurableTasks(ctx, repository.TaskFilter{
		Folder: req.Folder,
		State:  req.State,
		Limit:  req.Limit,
	})
	if err != nil {
		return nil, err
	}
	return &WorkListResult{Tasks: tasks}, nil
}
