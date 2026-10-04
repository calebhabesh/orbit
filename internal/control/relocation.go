package control

import (
	"context"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

type RelocateFolderRequest struct {
	Folder       history.ID `json:"folder"`
	ExpectedPath string     `json:"expected_path"`
	Path         string     `json:"path"`
}

func (c *Controller) RelocateFolder(ctx context.Context, req RelocateFolderRequest) (*workspace.RelocationResult, error) {
	if req.Folder == (history.ID{}) || req.ExpectedPath == "" || req.Path == "" {
		return nil, &ControlError{Code: "INVALID_REQUEST", Message: "workspace, current location and new location are required"}
	}
	result, err := c.ws.Relocate(ctx, req.Folder, req.ExpectedPath, req.Path)
	if err != nil {
		return nil, &ControlError{Code: "RELOCATION_FAILED", Message: err.Error(), Action: "check the folder locations and retry; keep original and staging folders until recovery finishes"}
	}
	return result, nil
}
