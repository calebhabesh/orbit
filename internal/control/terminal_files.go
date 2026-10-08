package control

import (
	"context"
	pathpkg "path"
	"time"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
)

// The Files view and `orbit files` share these read-only queries. Listing,
// search and details reuse repository browsing; nothing here captures,
// publishes or mutates a path.
func (c *Controller) terminalFiles(ctx context.Context, q tc.Query) (tc.Result, error) {
	r := terminalResult()
	folder := terminalID(q.Folder)
	var items []repository.BrowseItem
	if q.Name != "" {
		page, err := c.Search(ctx, folder, repository.SearchOptions{Query: q.Name, Limit: int(q.Limit), Cursor: q.Cursor})
		if err != nil {
			return r, err
		}
		items, r.Cursor = page.Items, page.NextCursor
	} else {
		page, err := c.Browse(ctx, folder, repository.BrowseOptions{DirPath: q.Path, Limit: int(q.Limit), Cursor: q.Cursor})
		if err != nil {
			return r, err
		}
		items, r.Cursor = page.Items, page.NextCursor
	}
	r.Files = make([]tc.FileEntry, 0, len(items))
	for _, it := range items {
		r.Files = append(r.Files, fileEntry(it.Path, it.IsDir, it.Size, it.MtimeNS, it.SyncState, it.BlockReason))
	}
	return r, nil
}

func (c *Controller) terminalFileDetails(ctx context.Context, q tc.Query) (tc.Result, error) {
	r := terminalResult()
	folder := terminalID(q.Folder)
	d, err := c.FileDetails(ctx, folder, q.Path)
	if err != nil {
		return r, err
	}
	detail := &tc.FileDetail{Entry: fileEntry(d.Path, d.IsDir, d.Size, d.MtimeNS, d.SyncState, d.BlockReason), Heads: []tc.VersionSummary{}, Observations: []tc.Observation{}}
	if d.LastScannedNS > 0 {
		detail.LastChecked = time.Unix(0, d.LastScannedNS).UTC().Format(time.RFC3339)
	}
	for _, h := range d.Heads {
		var author history.ID
		_ = author.UnmarshalText([]byte(h.AuthorID))
		detail.Heads = append(detail.Heads, tc.VersionSummary{
			Version:    terminalVersion(history.VersionID{Folder: folder, Author: author, Counter: h.Counter}),
			Path:       d.Path,
			DeviceName: h.AuthorName, DisplayTime: h.DisplayTime, Bytes: tc.Uint(h.FileSize), Digest: h.FileDigest, Availability: h.ContentState,
		})
	}
	now := time.Now()
	for _, p := range d.Peers {
		// Peer reports are about the newest head only and are as old as the
		// last contact; an empty time means the device never reported.
		obs := tc.Observation{DeviceName: p.PeerName, Device: p.PeerID, Folder: q.Folder,
			Version: tc.VersionID{Folder: q.Folder, Author: p.VersionAuthor, Counter: tc.Uint(p.VersionCounter)},
			Stored:  p.Receipt || p.RemoteStatus == "STORED" || p.RemoteStatus == "APPLIED",
			Applied: p.RemoteStatus == "APPLIED", Direct: p.Direct, Availability: "unknown"}
		obs.Saved = obs.Stored
		if p.LastContactNS > 0 {
			at := time.Unix(0, p.LastContactNS)
			obs.ObservedAt = at.UTC().Format(time.RFC3339)
			obs.LastContact = obs.ObservedAt
			obs.Online = now.Sub(at) < 5*time.Minute
		}
		detail.Observations = append(detail.Observations, obs)
	}
	r.File = detail
	return r, nil
}

func fileEntry(path string, dir bool, size uint64, mtimeNS int64, state, reason string) tc.FileEntry {
	e := tc.FileEntry{Path: path, Name: pathpkg.Base(path), Directory: dir, State: state, Reason: reason}
	if !dir {
		e.Bytes = tc.Uint(size)
	}
	if mtimeNS > 0 {
		e.Modified = time.Unix(0, mtimeNS).UTC().Format(time.RFC3339)
	}
	return e
}
