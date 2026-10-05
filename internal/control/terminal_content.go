package control

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"sort"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
)

func terminalVersion(v history.VersionID) tc.VersionID {
	return tc.VersionID{Folder: hex.EncodeToString(v.Folder[:]), Author: hex.EncodeToString(v.Author[:]), Counter: tc.Uint(v.Counter)}
}
func contentVersion(v tc.VersionID) (history.VersionID, error) {
	var id history.VersionID
	if err := id.Folder.UnmarshalText([]byte(v.Folder)); err != nil {
		return id, err
	}
	if err := id.Author.UnmarshalText([]byte(v.Author)); err != nil {
		return id, err
	}
	id.Counter = uint64(v.Counter)
	if id.Counter == 0 {
		return id, terminalError("INVALID_REQUEST")
	}
	return id, nil
}
func randomContentID() (string, error) {
	var b [32]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}
func contentKind(kind history.Kind) string {
	switch kind {
	case history.KindFile:
		return "file"
	case history.KindDirectory:
		return "directory"
	case history.KindTombstone:
		return "deleted"
	}
	return "unknown"
}
func summary(env history.Envelope, state string) tc.VersionSummary {
	v := tc.VersionSummary{Version: terminalVersion(env.ID), Path: env.Path, Kind: contentKind(env.Kind), DisplayTime: env.DisplayTime, Availability: state}
	if env.Manifest != nil {
		v.Bytes = tc.Uint(env.Manifest.Size)
		v.Digest = hex.EncodeToString(env.Manifest.Digest[:])
	}
	return v
}

type contentReviewRecord struct {
	Value              tc.ContentReview `json:"value"`
	Working            string           `json:"working"`
	DestinationWorking string           `json:"destination_working"`
	DestinationHeads   []tc.VersionID   `json:"destination_heads"`
}

func (c *Controller) contentSnapshot(ctx context.Context, folder history.ID, path string) (tc.Context, []tc.VersionID, string, bool, error) {
	var target tc.Context
	reg, err := c.db.Root(ctx, folder)
	if err != nil {
		return target, nil, "", false, err
	}
	if err = c.readableWorkspace(ctx, folder); err != nil {
		return target, nil, "", false, err
	}
	membership, e := c.db.Membership(ctx, folder)
	if e != nil && !errors.Is(e, repository.ErrMembershipMismatch) {
		return target, nil, "", false, e
	}
	fork, e := c.db.HasMembershipFork(ctx, folder)
	if e != nil {
		return target, nil, "", false, e
	}
	if fork {
		return target, nil, "", false, terminalError("MEMBERSHIP_FORK")
	}
	rootGeneration := generation(struct {
		Path          string
		Device, Inode uint64
		Registration  [32]byte
		Membership    repository.ApprovedMembership
	}{reg.Path, reg.Device, reg.Inode, reg.RegistrationID, membership})
	target = tc.Context{Folder: hex.EncodeToString(folder[:]), Path: path, Root: reg.Path, Generation: rootGeneration}
	heads, err := c.currentPathHeads(ctx, folder, path)
	if err != nil {
		return target, nil, "", false, err
	}
	ids := make([]tc.VersionID, 0, len(heads))
	for _, v := range heads {
		ids = append(ids, terminalVersion(v))
	}
	sort.Slice(ids, func(i, j int) bool {
		if ids[i].Author == ids[j].Author {
			return ids[i].Counter < ids[j].Counter
		}
		return ids[i].Author < ids[j].Author
	})
	if len(ids) > 64 {
		return target, nil, "", false, terminalError("PAYLOAD_TOO_LARGE")
	}
	if c.ws == nil {
		return target, ids, "", false, terminalError("ROOT_UNAVAILABLE")
	}
	working, captured, err := c.ws.ReviewPath(ctx, folder, path)
	return target, ids, working, captured, err
}

func (c *Controller) terminalContentQuery(ctx context.Context, q tc.Query) (tc.Result, error) {
	r := terminalResult()
	var folder history.ID
	if err := folder.UnmarshalText([]byte(q.Folder)); err != nil {
		return r, err
	}
	if err := c.readableWorkspace(ctx, folder); err != nil {
		return r, err
	}
	limit := int(q.Limit)
	if limit == 0 {
		limit = 50
	}
	switch q.Kind {
	case "conflicts":
		items, cursor, err := c.db.BrowseConflictPaths(ctx, folder, q.Cursor, limit)
		if err != nil {
			return r, readError(err)
		}
		r.Attention = items
		r.Cursor = cursor
	case "history":
		page, err := c.BrowseHistory(ctx, folder, q.Path, q.Cursor, limit)
		if err != nil {
			return r, err
		}
		for _, v := range page.Items {
			env, err := c.db.Envelope(ctx, history.VersionID{Folder: folder, Author: mustContentID(v.AuthorID), Counter: v.Counter})
			if err != nil {
				return r, err
			}
			r.Versions = append(r.Versions, summary(env, v.ContentState))
		}
		r.Cursor = page.NextCursor
	case "deleted":
		page, err := c.BrowseDeleted(ctx, folder, q.Cursor, limit)
		if err != nil {
			return r, err
		}
		for _, v := range page.Items {
			r.Items = append(r.Items, tc.NamedItem{ID: v.Path, Name: v.Path})
			if v.Source != nil {
				env, e := c.db.Envelope(ctx, *v.Source)
				if e != nil {
					return r, e
				}
				r.Versions = append(r.Versions, summary(env, v.ContentState))
			}
		}
		r.Cursor = page.NextCursor
	case "content_review":
		target, heads, working, captured, err := c.contentSnapshot(ctx, folder, q.Path)
		if err != nil {
			return r, err
		}
		token, err := randomContentID()
		if err != nil {
			return r, err
		}
		value := tc.ContentReview{Context: target, Heads: heads, WorkingCaptured: captured, ReplacementPaths: []string{q.Path}, Destination: q.Destination}
		rec := contentReviewRecord{Value: value, Working: working, DestinationHeads: []tc.VersionID{}}
		if q.Source != nil {
			id, err := contentVersion(*q.Source)
			if err != nil {
				return r, err
			}
			env, err := c.db.Envelope(ctx, id)
			if err != nil {
				return r, err
			}
			if env.Path != q.Path {
				return r, terminalError("INVALID_REQUEST")
			}
			state, err := c.db.ContentAvailability(ctx, id)
			if err != nil {
				return r, err
			}
			v := summary(env, string(state))
			value.Source = &v
		}
		if q.Destination != "" {
			if err := history.ValidatePath(q.Destination); err != nil || q.Destination == q.Path {
				return r, terminalError("INVALID_PATH")
			}
			_, dstHeads, dstWorking, _, err := c.contentSnapshot(ctx, folder, q.Destination)
			if err != nil {
				return r, err
			}
			if dstWorking != "absent" {
				return r, DestinationCollisionError(q.Destination)
			}
			rec.DestinationWorking, rec.DestinationHeads = dstWorking, dstHeads
			dst := tc.Review{Token: token, Generation: generation(struct {
				Heads   []tc.VersionID
				Working string
			}{dstHeads, dstWorking}), ExpiresAt: c.options.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339Nano)}
			value.DestinationReview = &dst
		}
		value.Review = tc.Review{Token: token, Generation: generation(struct {
			Context tc.Context
			Heads   []tc.VersionID
			Working string
		}{target, heads, working}), ExpiresAt: c.options.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339Nano)}
		rec.Value = value
		if err = c.db.SaveTerminalRecord(ctx, "contentreview/"+token, rec, true); err != nil {
			return r, err
		}
		r.Context = &target
		r.ContentReview = &value
		r.Review = &value.Review
		for _, v := range heads {
			id, _ := contentVersion(v)
			env, err := c.db.Envelope(ctx, id)
			if err != nil {
				return r, err
			}
			state, err := c.db.ContentAvailability(ctx, id)
			if err != nil {
				return r, err
			}
			r.Versions = append(r.Versions, summary(env, string(state)))
		}
	}
	for i := range r.Versions {
		name, e := c.db.GetDeviceDisplayName(ctx, mustContentID(r.Versions[i].Version.Author))
		if e != nil {
			return r, e
		}
		if name == "" {
			name = "Device " + r.Versions[i].Version.Author[:8]
		}
		r.Versions[i].DeviceName = name
	}
	if len(r.Items) == 0 && len(r.Versions) == 0 && len(r.Attention) == 0 && r.ContentReview == nil {
		r.State = "empty"
	}
	return r, nil
}
func mustContentID(s string) history.ID { var v history.ID; _ = v.UnmarshalText([]byte(s)); return v }

func (c *Controller) verifyContentReview(ctx context.Context, p tc.ContentIntent) (contentReviewRecord, history.ID, []history.VersionID, error) {
	var rec contentReviewRecord
	folder := mustContentID(p.Context.Folder)
	err := c.db.TerminalRecord(ctx, "contentreview/"+p.Review.Token, &rec)
	if err != nil {
		return rec, folder, nil, terminalError("STALE_VIEW")
	}
	expiry, err := time.Parse(time.RFC3339Nano, rec.Value.Review.ExpiresAt)
	activeSession := false
	if p.Session != "" {
		session, e := c.loadContentSession(ctx, p.Session, true)
		activeSession = e == nil && session.Session.Context == p.Context && session.Session.Review == p.Review && generation(session.Session.Heads) == generation(p.Heads)
	}

	if err != nil || (!c.options.Now().Before(expiry) && !activeSession) || rec.Value.Review != p.Review || rec.Value.Context != p.Context || generation(rec.Value.Heads) != generation(p.Heads) {
		return rec, folder, nil, terminalError("STALE_VIEW")
	}
	target, heads, working, captured, err := c.contentSnapshot(ctx, folder, p.Context.Path)
	if err != nil {
		return rec, folder, nil, err
	}
	if target != p.Context || generation(heads) != generation(p.Heads) || working != rec.Working || !captured {
		return rec, folder, nil, terminalError("STALE_VIEW")
	}
	ids := make([]history.VersionID, 0, len(heads))
	for _, v := range heads {
		id, err := contentVersion(v)
		if err != nil {
			return rec, folder, nil, err
		}
		ids = append(ids, id)
	}
	return rec, folder, ids, nil
}

// ExactRead reauthorizes every open; the returned stream owns repository pins.
func (c *Controller) ExactRead(ctx context.Context, intent tc.ReadIntent) (io.ReadCloser, error) {
	id, err := contentVersion(intent.Version)
	if err != nil {
		return nil, err
	}
	if err = c.verifyContentAvailability(ctx, id); err != nil {
		return nil, err
	}
	read, err := c.OpenContent(ctx, id.Folder, id)
	if err != nil {
		return nil, err
	}
	size := read.Envelope.Manifest.Size
	if uint64(intent.Offset) > size || (intent.Length != 0 && uint64(intent.Length) > size-uint64(intent.Offset)) {
		read.Close()
		return nil, terminalError("INVALID_REQUEST")
	}
	if _, err = read.Seek(int64(intent.Offset), io.SeekStart); err != nil {
		read.Close()
		return nil, err
	}
	length := size - uint64(intent.Offset)
	if intent.Length != 0 {
		length = uint64(intent.Length)
	}
	return &exactRange{Reader: io.LimitReader(read, int64(length)), Closer: read}, nil
}

type exactRange struct {
	io.Reader
	io.Closer
}

// observeContentPublication refreshes durable effects from the actual working
// projection. Polls never publish bytes or author another version.
func (c *Controller) observeContentPublication(ctx context.Context, record *repository.TerminalRecord) (tc.Result, error) {
	r := record.Result
	all := len(r.Effects) > 0
	for i, effect := range r.Effects {
		if effect.Version == nil || effect.State == "applied" {
			continue
		}
		id, err := contentVersion(*effect.Version)
		if err != nil {
			return r, err
		}
		applied, err := c.db.WorkingApplied(ctx, id)
		if err != nil {
			return r, err
		}
		if applied {
			r.Effects[i].State = "applied"
		} else {
			all = false
		}
	}
	r.Operation.CommittedEffects = r.Effects
	if all {
		r.State, r.Operation.State, r.Operation.Phase = "completed", "completed", "applied"
		record.CompletedAt = c.options.Now().UTC().Format(time.RFC3339Nano)
		record.Result = r
		if err := c.db.SaveTerminalRecord(ctx, "operation/"+record.Mutation.OperationID, *record, false); err != nil {
			return r, err
		}
	}
	return r, nil
}

// resumeContentPublication never authors another version on replay.
func (c *Controller) resumeContentPublication(ctx context.Context, record *repository.TerminalRecord) (tc.Result, error) {
	r := record.Result
	all := true
	for i, e := range r.Effects {
		if e.Version == nil || e.State == "applied" {
			continue
		}
		id, err := contentVersion(*e.Version)
		if err != nil {
			return r, err
		}
		applied, err := c.db.WorkingApplied(ctx, id)
		if err != nil {
			return r, err
		}
		if !applied && c.ws != nil {
			heads, err := c.currentPathHeads(ctx, id.Folder, e.Path)
			if err != nil {
				return r, err
			}
			if len(heads) != 1 || heads[0] != id {
				r.State = "partial"
				r.Error = &tc.Error{Code: "STALE_VIEW", Message: "committed publication has newer or competing heads", Action: "inspect current versions and obtain a fresh replacement review"}
				r.Operation.State = "blocked"
				r.Operation.Error = r.Error
				record.Result = r
				if err = c.db.SaveTerminalRecord(context.Background(), "operation/"+record.Mutation.OperationID, *record, false); err != nil {
					return r, err
				}
				return r, nil
			}
			err = c.ws.Apply(ctx, id)
			applied = err == nil
		}
		if applied {
			r.Effects[i].State = "applied"
		} else {
			all = false
		}
	}
	r.Operation.CommittedEffects = r.Effects
	if all {
		r.State = "completed"
		r.Operation.State = "completed"
		r.Operation.Phase = "applied"
		record.CompletedAt = c.options.Now().UTC().Format(time.RFC3339Nano)
	} else {
		r.State = "pending"
		r.Operation.State = "pending"
		r.Operation.Phase = "publication"
	}
	record.Result = r
	err := c.db.SaveTerminalRecord(context.Background(), "operation/"+record.Mutation.OperationID, *record, false)
	return r, err
}

func (c *Controller) terminalContentMutation(ctx context.Context, m tc.Mutation) (tc.Result, error) {
	c.contentMu.Lock()
	defer c.contentMu.Unlock()
	r := terminalResult()
	fingerprint, err := m.Fingerprint()
	if err != nil {
		return r, err
	}
	var record repository.TerminalRecord
	err = c.db.TerminalRecord(ctx, "operation/"+m.OperationID, &record)
	existing := err == nil
	if existing {
		if _, err = c.replayTerminal(record, fingerprint); err != nil {
			return r, err
		}
		if record.Result.Operation.State == "completed" {
			return record.Result, nil
		}
		if record.Result.Operation.Phase != "copies" {
			return c.resumeContentPublication(ctx, &record)
		}
	} else if !errors.Is(err, repository.ErrOperationNotFound) {
		return r, err
	}
	p := *m.Content
	rec, folder, ids, err := c.verifyContentReview(ctx, p)
	if err != nil {
		return r, err
	}
	if !existing {
		r.State = "partial"
		r.Operation = &tc.Operation{ID: m.OperationID, Kind: m.Kind, Fingerprint: fingerprint, State: "partial", Phase: "copies", CommittedEffects: []tc.Effect{}}
		cfg, e := config.Load(c.db.StateDir())
		if e != nil {
			return r, e
		}
		record = repository.TerminalRecord{Owner: cfg.DeviceID, Mutation: m, Result: r, CreatedAt: c.options.Now().UTC().Format(time.RFC3339Nano)}
	}
	// Copies commit individually, retaining durable effects before the original
	// resolution. This deliberately promises no cross-path atomic visibility.
	if p.Action == "keep_copies" || p.Action == "separate_copy" {
		copies := p.Copies
		if p.Action == "separate_copy" {
			copies = []tc.CopyPlan{{Source: p.Source, Destination: p.Destination, Review: p.DestinationReview}}
		}
		for _, copyPlan := range copies {
			done := false
			for _, e := range record.Result.Effects {
				if e.Path == copyPlan.Destination {
					done = true
				}
			}
			if done {
				continue
			}
			var dstRecord contentReviewRecord
			if err = c.db.TerminalRecord(ctx, "contentreview/"+copyPlan.Review.Token, &dstRecord); err != nil {
				return record.Result, terminalError("STALE_VIEW")
			}
			v := dstRecord.Value
			if v.Destination != copyPlan.Destination || v.DestinationReview == nil || *v.DestinationReview != copyPlan.Review || v.Context != p.Context || generation(v.Heads) != generation(p.Heads) || dstRecord.Working != rec.Working || v.Source == nil || v.Source.Version != copyPlan.Source {
				return record.Result, terminalError("STALE_VIEW")
			}
			expiry, err := time.Parse(time.RFC3339Nano, copyPlan.Review.ExpiresAt)
			if err != nil || !c.options.Now().Before(expiry) {
				return record.Result, terminalError("STALE_VIEW")
			}
			_, dstHeads, working, _, err := c.contentSnapshot(ctx, folder, copyPlan.Destination)
			if err != nil {
				return record.Result, err
			}
			if working != "absent" || generation(dstHeads) != generation(dstRecord.DestinationHeads) {
				return record.Result, DestinationCollisionError(copyPlan.Destination)
			}
			source, err := contentVersion(copyPlan.Source)
			if err != nil {
				return record.Result, err
			}
			env, err := c.db.Envelope(ctx, source)
			if err != nil {
				return record.Result, err
			}
			if env.Path != p.Context.Path {
				return record.Result, terminalError("INVALID_REQUEST")
			}
			if env.Kind != history.KindFile {
				return record.Result, terminalError("INVALID_REQUEST")
			}
			read, err := c.OpenContent(ctx, folder, source)
			if err != nil {
				return record.Result, err
			}
			if len(dstHeads) == 0 {
				_, err = c.db.CreateCopyVersion(ctx, repository.CopyVersionRequest{Folder: folder, Path: copyPlan.Destination, Kind: env.Kind, Manifest: env.Manifest, DisplayTime: c.options.Now().UTC().Format(time.RFC3339Nano), TerminalOperation: &record, TerminalSource: &copyPlan.Source})
			} else {
				dstIDs := []history.VersionID{}
				for _, v := range dstHeads {
					id, _ := contentVersion(v)
					dstIDs = append(dstIDs, id)
				}
				_, err = c.db.CreateResolutionVersion(ctx, repository.ResolutionVersionRequest{Folder: folder, Path: copyPlan.Destination, Reviewed: dstIDs, ExpectedHeadToken: history.HeadToken(dstIDs), Kind: env.Kind, Manifest: env.Manifest, DisplayTime: c.options.Now().UTC().Format(time.RFC3339Nano), TerminalOperation: &record, TerminalSource: &copyPlan.Source})
			}
			read.Close()
			if err != nil {
				return record.Result, err
			}
			if err = c.callHook("terminal.content.copy.committed"); err != nil {
				return record.Result, err
			}
		}
		if p.Action == "separate_copy" {
			record.Result.Operation.Phase = "publication"
			return c.resumeContentPublication(ctx, &record)
		}
	}
	var env history.Envelope
	switch p.Action {
	case "merge":
		f, manifest, err := c.openUpload(ctx, p)
		if err != nil {
			return record.Result, err
		}
		defer f.Close()
		env = history.Envelope{Kind: history.KindFile, Manifest: manifest}
	case "select", "restore", "keep_copies":
		source, err := contentVersion(p.Source)
		if err != nil {
			return record.Result, err
		}
		if p.Action == "keep_copies" {
			found := false
			for _, v := range p.Heads {
				if v == p.Source {
					found = true
				}
			}
			if !found {
				return record.Result, terminalError("INVALID_REQUEST")
			}
		}
		env, err = c.db.Envelope(ctx, source)
		if err != nil {
			return record.Result, err
		}
		if env.Path != p.Context.Path || source.Folder != folder {
			return record.Result, terminalError("INVALID_REQUEST")
		}
		if p.Action == "restore" && (rec.Value.Source == nil || rec.Value.Source.Version != p.Source) {
			return record.Result, terminalError("STALE_VIEW")
		}
		if err = c.verifyContentAvailability(ctx, source); err != nil {
			return record.Result, err
		}
		if env.Kind == history.KindFile {
			read, err := c.OpenContent(ctx, folder, source)
			if err != nil {
				return record.Result, err
			}
			defer read.Close()
		}
	default:
		return r, terminalError("UNSUPPORTED_CAPABILITY")
	}
	if _, _, _, err = c.verifyContentReview(ctx, p); err != nil {
		return record.Result, err
	}
	record.Result.State = "pending"
	record.Result.Operation.State = "pending"
	record.Result.Operation.Phase = "publication"
	var source *tc.VersionID
	if p.Action != "merge" {
		source = &p.Source
	}
	_, err = c.db.CreateResolutionVersion(ctx, repository.ResolutionVersionRequest{Folder: folder, Path: p.Context.Path, Reviewed: ids, ExpectedHeadToken: history.HeadToken(ids), Kind: env.Kind, Manifest: env.Manifest, DisplayTime: c.options.Now().UTC().Format(time.RFC3339Nano), TerminalOperation: &record, TerminalSource: source})
	if err != nil {
		return record.Result, err
	}
	if err = c.callHook("terminal.content.committed"); err != nil {
		return record.Result, err
	}
	return c.resumeContentPublication(ctx, &record)
}
