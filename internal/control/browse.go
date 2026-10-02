package control

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
)

func readError(err error) error {
	if err == nil {
		return nil
	}
	code := "INVALID_REQUEST"
	action := "correct the workspace, relative path or pagination parameters"
	switch {
	case errors.Is(err, repository.ErrStaleCursor):
		code = "STALE_VIEW"
		action = "refresh the listing and restart pagination"
	case errors.Is(err, repository.ErrFolderUnknown), errors.Is(err, repository.ErrUnauthorized):
		code = "UNAUTHORIZED"
		action = "select a locally authorized workspace"
	case errors.Is(err, repository.ErrPathNotFound), errors.Is(err, repository.ErrDirectoryNotFound), errors.Is(err, sql.ErrNoRows):
		code = "NOT_FOUND"
		action = "refresh locally known workspace state"
	case errors.Is(err, repository.ErrNotReady):
		code = "CONTENT_UNAVAILABLE"
		action = "inspect version availability; wait for transfer or repair the requested version"
	case errors.Is(err, repository.ErrContentMissing), errors.Is(err, repository.ErrContentMismatch), errors.Is(err, repository.ErrContentCorrupt):
		code = "CONTENT_UNAVAILABLE"
		action = "inspect integrity status and repair the exact requested content"
	case errors.Is(err, repository.ErrGCIntentActive):
		code = "CONTENT_UNAVAILABLE"
		action = "requested historical content is being collected"
	}
	return &ControlError{Code: code, Message: err.Error(), Action: action, Err: err}
}

func (c *Controller) readableWorkspace(ctx context.Context, folder history.ID) error {
	if folder == (history.ID{}) {
		return readError(fmt.Errorf("workspace is required"))
	}
	return readError(c.db.CheckLocalWorkspaceAccess(ctx, folder))
}

func (c *Controller) Browse(ctx context.Context, folder history.ID, options repository.BrowseOptions) (*repository.BrowseResult, error) {
	if err := c.readableWorkspace(ctx, folder); err != nil {
		return nil, err
	}
	res, err := c.db.BrowseWorkspaceDirectory(ctx, folder, options)
	return res, readError(err)
}
func (c *Controller) Search(ctx context.Context, folder history.ID, options repository.SearchOptions) (*repository.SearchResult, error) {
	if err := c.readableWorkspace(ctx, folder); err != nil {
		return nil, err
	}
	res, err := c.db.SearchWorkspace(ctx, folder, options)
	return res, readError(err)
}
func (c *Controller) FileDetails(ctx context.Context, folder history.ID, path string) (*repository.FileDetails, error) {
	if err := c.readableWorkspace(ctx, folder); err != nil {
		return nil, err
	}
	res, err := c.db.FileDetails(ctx, folder, path)
	return res, readError(err)
}
func (c *Controller) BrowseDeleted(ctx context.Context, folder history.ID, cursor string, limit int) (*repository.DeletedFilesResult, error) {
	if err := c.readableWorkspace(ctx, folder); err != nil {
		return nil, err
	}
	res, err := c.db.BrowseDeletedFiles(ctx, folder, cursor, limit)
	return res, readError(err)
}
func (c *Controller) BrowseHistory(ctx context.Context, folder history.ID, path, cursor string, limit int) (*repository.FileHistoryResult, error) {
	if err := c.readableWorkspace(ctx, folder); err != nil {
		return nil, err
	}
	res, err := c.db.BrowsePathHistory(ctx, folder, path, cursor, limit)
	return res, readError(err)
}

func (c *Controller) OpenContent(ctx context.Context, folder history.ID, id history.VersionID) (*repository.VersionRead, error) {
	if id.Folder != folder || id.Author == (history.ID{}) || id.Counter == 0 {
		return nil, readError(fmt.Errorf("exact version must belong to requested workspace"))
	}
	if err := c.readableWorkspace(ctx, folder); err != nil {
		return nil, err
	}
	res, err := c.db.OpenVersionRead(ctx, id)
	return res, readError(err)
}
