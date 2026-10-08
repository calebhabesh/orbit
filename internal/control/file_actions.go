package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
	"github.com/calebhabesh/orbit/internal/workspace"
)

func mutationError(err error) error {
	if err == nil {
		return nil
	}
	var ctrlErr *ControlError
	if errors.As(err, &ctrlErr) {
		return ctrlErr
	}
	code := "IO_ERROR"
	action := "check path and retry"
	retryable := false

	switch {
	case errors.Is(err, workspace.ErrDestinationExists):
		code = "DESTINATION_EXISTS"
		action = "review existing destination and provide overwrite approval if intended"
	case errors.Is(err, workspace.ErrSubtreeInvalidated):
		code = "SUBTREE_INVALIDATED"
		action = "directory children changed during operation; re-review and retry"
	case errors.Is(err, workspace.ErrStaleReview):
		code = "STALE_VIEW"
		action = "target file was modified concurrently; review latest version"
	case errors.Is(err, workspace.ErrDirectoryNotEmpty):
		code = "DIRECTORY_NOT_EMPTY"
		action = "use recursive option to delete non-empty directory"
	case errors.Is(err, workspace.ErrStructuralConflict):
		code = "STRUCTURAL_CONFLICT"
		action = "resolve conflicting file or directory type at path"
	case errors.Is(err, workspace.ErrRootUnavailable):
		code = "ROOT_UNAVAILABLE"
		action = "ensure workspace root filesystem is mounted and accessible"
		retryable = true
	case errors.Is(err, workspace.ErrOperationCanceled):
		code = "OPERATION_CANCELED"
		action = "operation was canceled by user request"
	case errors.Is(err, os.ErrNotExist):
		code = "NOT_FOUND"
		action = "ensure path exists"
	case errors.Is(err, repository.ErrBudgetExceeded), errors.Is(err, repository.ErrStorageExhausted):
		code = "DISK_BUDGET"
		action = "free space or reclaim recovery copies"
	case errors.Is(err, ErrIdempotencyConflict):
		code = "IDEMPOTENCY_CONFLICT"
		action = "use a fresh idempotency key or matching parameters"
	case errors.Is(err, ErrExpiredReplay):
		code = "IDEMPOTENCY_EXPIRED"
		action = "idempotency key expired; generate a new key and review latest state"
	}
	return &ControlError{
		Code:      code,
		Message:   err.Error(),
		Action:    action,
		Retryable: retryable,
		Err:       err,
	}
}

func computeMutationDigest(action string, req any) history.Digest {
	b, _ := json.Marshal(req)
	h := sha256.New()
	h.Write([]byte(action))
	h.Write([]byte(":"))
	h.Write(b)
	var d history.Digest
	copy(d[:], h.Sum(nil))
	return d
}

// ImportFile streams and installs a file into the workspace with full durability.
func (c *Controller) ImportFile(ctx context.Context, req ImportFileRequest, r io.Reader, size uint64) (*ImportFileResult, error) {
	if err := c.readableWorkspace(ctx, req.Folder); err != nil {
		return nil, err
	}

	// Idempotency check
	if req.IdempotencyKey != "" {
		reqDigest := computeMutationDigest("import", req)
		rec, storedDigest, _, err := c.db.GetControlOperation(ctx, req.IdempotencyKey)
		if errors.Is(err, repository.ErrExpiredReplay) {
			return nil, mutationError(ErrExpiredReplay)
		}
		if err == nil && rec != nil {
			if storedDigest != reqDigest {
				return nil, mutationError(ErrIdempotencyConflict)
			}
			var cached ImportFileResult
			if err := json.Unmarshal(rec.Payload, &cached); err == nil {
				return &cached, nil
			}
		}
	}

	res, err := c.ws.ImportFile(ctx, workspace.ImportRequest{
		Folder:         req.Folder,
		Path:           req.Path,
		Source:         r,
		Size:           size,
		Executable:     req.Executable,
		Overwrite:      req.Overwrite,
		ReviewedToken:  req.ReviewedToken,
		OperationID:    req.OperationID,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, mutationError(err)
	}

	ctrlRes := &ImportFileResult{
		OperationID: res.OperationID,
		VersionID:   res.VersionID,
		Path:        res.Path,
		Size:        res.Size,
		Digest:      hex.EncodeToString(res.Digest[:]),
		Completed:   res.Completed,
	}

	if req.IdempotencyKey != "" {
		reqDigest := computeMutationDigest("import", req)
		payload, _ := json.Marshal(ctrlRes)
		now := time.Now()
		_ = c.db.PutControlOperation(ctx, req.IdempotencyKey, reqDigest, 0, &repository.ControlOpRecord{
			Status:      "completed",
			Action:      "import",
			CompletedAt: &now,
			Payload:     payload,
		}, now.Add(24*time.Hour))
	}

	return ctrlRes, nil
}

// CreateDir creates a directory in the workspace with durable versioning.
func (c *Controller) CreateDir(ctx context.Context, req CreateDirRequest) (*CreateDirResult, error) {
	if err := c.readableWorkspace(ctx, req.Folder); err != nil {
		return nil, err
	}

	// Idempotency check
	if req.IdempotencyKey != "" {
		reqDigest := computeMutationDigest("mkdir", req)
		rec, storedDigest, _, err := c.db.GetControlOperation(ctx, req.IdempotencyKey)
		if errors.Is(err, repository.ErrExpiredReplay) {
			return nil, mutationError(ErrExpiredReplay)
		}
		if err == nil && rec != nil {
			if storedDigest != reqDigest {
				return nil, mutationError(ErrIdempotencyConflict)
			}
			var cached CreateDirResult
			if err := json.Unmarshal(rec.Payload, &cached); err == nil {
				return &cached, nil
			}
		}
	}

	res, err := c.ws.CreateDirectory(ctx, workspace.CreateDirRequest{
		Folder:         req.Folder,
		Path:           req.Path,
		OperationID:    req.OperationID,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, mutationError(err)
	}

	ctrlRes := &CreateDirResult{
		OperationID:   res.OperationID,
		VersionID:     res.VersionID,
		Path:          res.Path,
		AlreadyExists: res.AlreadyExists,
		Completed:     res.Completed,
	}

	if req.IdempotencyKey != "" {
		reqDigest := computeMutationDigest("mkdir", req)
		payload, _ := json.Marshal(ctrlRes)
		now := time.Now()
		_ = c.db.PutControlOperation(ctx, req.IdempotencyKey, reqDigest, 0, &repository.ControlOpRecord{
			Status:      "completed",
			Action:      "mkdir",
			CompletedAt: &now,
			Payload:     payload,
		}, now.Add(24*time.Hour))
	}

	return ctrlRes, nil
}

// MoveFile relocates or renames a file or directory within the workspace.
func (c *Controller) MoveFile(ctx context.Context, req MoveFileRequest) (*MoveFileResult, error) {
	if err := c.readableWorkspace(ctx, req.Folder); err != nil {
		return nil, err
	}

	// Idempotency check
	if req.IdempotencyKey != "" {
		reqDigest := computeMutationDigest("move", req)
		rec, storedDigest, _, err := c.db.GetControlOperation(ctx, req.IdempotencyKey)
		if errors.Is(err, repository.ErrExpiredReplay) {
			return nil, mutationError(ErrExpiredReplay)
		}
		if err == nil && rec != nil {
			if storedDigest != reqDigest {
				return nil, mutationError(ErrIdempotencyConflict)
			}
			var cached MoveFileResult
			if err := json.Unmarshal(rec.Payload, &cached); err == nil {
				return &cached, nil
			}
		}
	}

	res, err := c.ws.Move(ctx, workspace.MoveRequest{
		Folder:         req.Folder,
		SourcePath:     req.SourcePath,
		DestPath:       req.DestPath,
		Overwrite:      req.Overwrite,
		ReviewedToken:  req.ReviewedToken,
		OperationID:    req.OperationID,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, mutationError(err)
	}

	ctrlRes := &MoveFileResult{
		OperationID:    res.OperationID,
		SourcePath:     res.SourcePath,
		DestPath:       res.DestPath,
		SourceRetained: res.SourceRetained,
		Completed:      res.Completed,
	}

	if req.IdempotencyKey != "" {
		reqDigest := computeMutationDigest("move", req)
		payload, _ := json.Marshal(ctrlRes)
		now := time.Now()
		_ = c.db.PutControlOperation(ctx, req.IdempotencyKey, reqDigest, 0, &repository.ControlOpRecord{
			Status:      "completed",
			Action:      "move",
			CompletedAt: &now,
			Payload:     payload,
		}, now.Add(24*time.Hour))
	}

	return ctrlRes, nil
}

// DeleteFile removes a file or directory recursively from the workspace.
func (c *Controller) DeleteFile(ctx context.Context, req DeleteFileRequest) (*DeleteFileResult, error) {
	if err := c.readableWorkspace(ctx, req.Folder); err != nil {
		return nil, err
	}

	// Idempotency check
	if req.IdempotencyKey != "" {
		reqDigest := computeMutationDigest("delete", req)
		rec, storedDigest, _, err := c.db.GetControlOperation(ctx, req.IdempotencyKey)
		if errors.Is(err, repository.ErrExpiredReplay) {
			return nil, mutationError(ErrExpiredReplay)
		}
		if err == nil && rec != nil {
			if storedDigest != reqDigest {
				return nil, mutationError(ErrIdempotencyConflict)
			}
			var cached DeleteFileResult
			if err := json.Unmarshal(rec.Payload, &cached); err == nil {
				return &cached, nil
			}
		}
	}

	res, err := c.ws.Delete(ctx, workspace.DeleteRequest{
		Folder:         req.Folder,
		Path:           req.Path,
		Recursive:      req.Recursive,
		ReviewedToken:  req.ReviewedToken,
		OperationID:    req.OperationID,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, mutationError(err)
	}

	ctrlRes := &DeleteFileResult{
		OperationID:    res.OperationID,
		Path:           res.Path,
		DeletedCount:   res.DeletedCount,
		AlreadyDeleted: res.AlreadyDeleted,
		Completed:      res.Completed,
		DeletedPaths:   res.DeletedPaths,
	}

	if req.IdempotencyKey != "" {
		reqDigest := computeMutationDigest("delete", req)
		payload, _ := json.Marshal(ctrlRes)
		now := time.Now()
		_ = c.db.PutControlOperation(ctx, req.IdempotencyKey, reqDigest, 0, &repository.ControlOpRecord{
			Status:      "completed",
			Action:      "delete",
			CompletedAt: &now,
			Payload:     payload,
		}, now.Add(24*time.Hour))
	}

	return ctrlRes, nil
}
