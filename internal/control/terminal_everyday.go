package control

import (
	"context"
	"fmt"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/repository"
)

// Everyday queries reuse repository search and maintenance ownership. No read
// captures, publishes, changes retention or runs cleanup.
func (c *Controller) terminalEveryday(ctx context.Context, q tc.Query) (tc.Result, error) {
	r := terminalResult()
	folder := terminalID(q.Folder)
	if q.Kind == "paths" {
		page, err := c.Search(ctx, folder, repository.SearchOptions{Query: q.Name, Limit: int(q.Limit), Cursor: q.Cursor})
		if err != nil {
			return r, err
		}
		for _, it := range page.Items {
			r.Items = append(r.Items, tc.NamedItem{ID: it.Path, Name: it.Path})
		}
		r.Cursor = page.NextCursor
		return r, nil
	}
	usage, err := c.StorageUsage(ctx)
	if err != nil {
		return r, err
	}
	u := usage.Usage
	r.Storage = &tc.Storage{Objects: tc.Uint(u.ObjectBytes), Metadata: tc.Uint(u.MetadataBytes), Staging: tc.Uint(u.StagingBytes), Recovery: tc.Uint(u.RecoveryBytes), Quarantine: tc.Uint(u.QuarantineBytes), DataBudget: tc.Uint(u.DataBudgetBytes), MetadataBudget: tc.Uint(u.MetadataBudgetBytes), Reserve: tc.Uint(u.FreeSpaceReserveBytes)}
	if q.Kind == "maintenance" {
		if err := c.readableWorkspace(ctx, folder); err != nil {
			return r, err
		}
		preview, err := c.RetentionPreview(ctx, RetentionPreviewRequest{Folder: folder})
		if err != nil {
			return r, err
		}
		p := preview.Preview
		r.Items = append(r.Items, tc.NamedItem{Name: fmt.Sprintf("Retention: %d days; minimum superseded: %d", p.Policy.RetentionDays, p.Policy.MinSuperseded)}, tc.NamedItem{Name: fmt.Sprintf("Known versions=%d retained=%d expired=%d", p.TotalVersions, p.RetainedVersions, p.ExpiredVersions)}, tc.NamedItem{Name: fmt.Sprintf("Cleanup candidates=%d reclaimable=%d bytes; suspended=%t", p.CandidateChunks, p.ReclaimableBytes, p.Suspended), Root: p.SuspendReason})
	}
	return r, nil
}
