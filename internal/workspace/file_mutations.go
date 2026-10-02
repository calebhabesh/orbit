package workspace

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
)

type ImportRequest struct {
	Folder         history.ID
	Path           string
	Source         io.Reader
	Size           uint64
	Executable     bool
	Overwrite      bool
	ReviewedToken  string
	OperationID    string
	IdempotencyKey string
}

type ImportResult struct {
	OperationID string            `json:"operation_id"`
	VersionID   history.VersionID `json:"version_id"`
	Path        string            `json:"path"`
	Size        uint64            `json:"size"`
	Digest      history.Digest    `json:"digest"`
	Completed   bool              `json:"completed"`
}

type CreateDirRequest struct {
	Folder         history.ID
	Path           string
	OperationID    string
	IdempotencyKey string
}

type CreateDirResult struct {
	OperationID   string            `json:"operation_id"`
	VersionID     history.VersionID `json:"version_id"`
	Path          string            `json:"path"`
	AlreadyExists bool              `json:"already_exists"`
	Completed     bool              `json:"completed"`
}

type MoveRequest struct {
	Folder         history.ID
	SourcePath     string
	DestPath       string
	Overwrite      bool
	ReviewedToken  string
	OperationID    string
	IdempotencyKey string
}

type MoveResult struct {
	OperationID    string `json:"operation_id"`
	SourcePath     string `json:"source_path"`
	DestPath       string `json:"dest_path"`
	SourceRetained bool   `json:"source_retained"`
	Completed      bool   `json:"completed"`
}

type DeleteRequest struct {
	Folder         history.ID
	Path           string
	Recursive      bool
	ReviewedToken  string
	OperationID    string
	IdempotencyKey string
}

type DeleteResult struct {
	OperationID    string   `json:"operation_id"`
	Path           string   `json:"path"`
	DeletedCount   int      `json:"deleted_count"`
	AlreadyDeleted bool     `json:"already_deleted"`
	Completed      bool     `json:"completed"`
	DeletedPaths   []string `json:"deleted_paths"`
}

// computePathHash calculates SHA-256 hex digest of a regular file beneath the opened root.
func computePathHash(rootFD int, path string) (string, error) {
	fd, err := safeOpen(rootFD, path, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return "", err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// computeSubtreeToken calculates a deterministic snapshot digest of immediate children.
func computeSubtreeToken(rootFD int, dirPath string) (string, error) {
	fd, err := safeOpen(rootFD, dirPath, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		return "", err
	}
	f := os.NewFile(uintptr(fd), dirPath)
	defer f.Close()

	entries, err := f.ReadDir(-1)
	if err != nil {
		return "", err
	}

	names := make([]string, 0, len(entries))
	typeMap := make(map[string]string)
	for _, e := range entries {
		names = append(names, e.Name())
		t := "file"
		if e.IsDir() {
			t = "dir"
		}
		typeMap[e.Name()] = t
	}
	sort.Strings(names)

	h := sha256.New()
	for _, name := range names {
		h.Write([]byte(name))
		h.Write([]byte(":"))
		h.Write([]byte(typeMap[name]))
		h.Write([]byte(";"))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func genOpID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "op-" + hex.EncodeToString(b)
}

// ImportFile streams and installs a new or overwritten file into the workspace.
func (workspace *Workspace) ImportFile(ctx context.Context, req ImportRequest) (*ImportResult, error) {
	if err := history.ValidatePath(req.Path); err != nil {
		return nil, err
	}
	if req.OperationID == "" {
		req.OperationID = genOpID()
	}

	root, err := workspace.openRoot(ctx, req.Folder)
	if err != nil {
		return nil, err
	}
	defer root.close()

	// Destination collision check
	var prevStat unix.Stat_t
	destExists := false
	if err := statPath(root.fd, req.Path, &prevStat); err == nil {
		destExists = true
		if prevStat.Mode&unix.S_IFMT == unix.S_IFDIR {
			return nil, ErrStructuralConflict
		}
		if !req.Overwrite {
			return nil, ErrDestinationExists
		}
		if req.ReviewedToken != "" {
			currentHash, err := computePathHash(root.fd, req.Path)
			if err == nil && currentHash != req.ReviewedToken {
				return nil, ErrStaleReview
			}
		}
	}

	// Disk budget reservation
	reserveBytes := req.Size
	if destExists && prevStat.Size > 0 {
		reserveBytes += uint64(prevStat.Size)
	}
	if reserveBytes > 0 {
		if err := workspace.repo.Reserve(ctx, "mutation-"+req.OperationID, reserveBytes, "file import staging"); err != nil {
			return nil, err
		}
		defer func() {
			_ = workspace.repo.ReleaseReservation(ctx, "mutation-"+req.OperationID)
		}()
	}

	// Journal record: PLANNED
	journal := repository.FileMutationRecord{
		OperationID:   req.OperationID,
		Folder:        req.Folder,
		Action:        "import",
		DestPath:      req.Path,
		ReviewedToken: req.ReviewedToken,
		Overwrite:     req.Overwrite,
		Phase:         "PLANNED",
	}
	if err := workspace.repo.RecordFileMutation(ctx, journal); err != nil {
		return nil, err
	}
	_ = workspace.repo.RecordOperationProgress(ctx, repository.OperationProgressRecord{
		OperationID:         req.OperationID,
		Kind:                "import",
		Phase:               "planning",
		ProgressDenominator: int64(req.Size),
	})

	if err := workspace.callHook(HookFileMutationPlanned); err != nil {
		return nil, err
	}

	// Advance to STAGED
	if err := workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "STAGED"); err != nil {
		return nil, err
	}
	_ = workspace.repo.RecordOperationProgress(ctx, repository.OperationProgressRecord{
		OperationID: req.OperationID,
		Kind:        "import",
		Phase:       "staging",
	})
	if err := workspace.callHook(HookFileMutationStaged); err != nil {
		return nil, err
	}

	// Store file into CAS objects (1 MiB bounded chunk streaming)
	manifest, err := workspace.repo.StoreFile(ctx, req.Source, req.Executable)
	if err != nil {
		_ = workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "ABORTED")
		return nil, err
	}

	// Author local version envelope
	nowStr := workspace.now().UTC().Format(time.RFC3339Nano)
	envelope, err := workspace.repo.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           req.Folder,
		Path:             req.Path,
		Kind:             history.KindFile,
		Manifest:         manifest,
		AuthoredRevision: 1,
		DisplayTime:      nowStr,
		SkipProjection:   true,
	})
	if err != nil {
		_ = workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "ABORTED")
		return nil, err
	}

	// Apply version to disk using publication machinery (with RENAME_EXCHANGE / recovery displaced)
	if err := workspace.ApplyWithOperationID(ctx, envelope.ID, req.OperationID); err != nil {
		return nil, err
	}

	// Advance to INSTALLED
	if err := workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "INSTALLED"); err != nil {
		return nil, err
	}
	_ = workspace.repo.RecordOperationProgress(ctx, repository.OperationProgressRecord{
		OperationID: req.OperationID,
		Kind:        "import",
		Phase:       "installing",
	})
	if err := workspace.callHook(HookFileMutationInstalled); err != nil {
		return nil, err
	}

	// Advance to COMPLETED
	if err := workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "COMPLETED"); err != nil {
		return nil, err
	}
	_ = workspace.repo.RecordOperationProgress(ctx, repository.OperationProgressRecord{
		OperationID:       req.OperationID,
		Kind:              "import",
		Phase:             "completed",
		ProgressNumerator: int64(manifest.Size),
	})
	if err := workspace.callHook(HookFileMutationCompleted); err != nil {
		return nil, err
	}

	return &ImportResult{
		OperationID: req.OperationID,
		VersionID:   envelope.ID,
		Path:        req.Path,
		Size:        manifest.Size,
		Digest:      manifest.Digest,
		Completed:   true,
	}, nil
}

// CreateDirectory creates a directory with durable versioning and scaffold tracking.
func (workspace *Workspace) CreateDirectory(ctx context.Context, req CreateDirRequest) (*CreateDirResult, error) {
	if err := history.ValidatePath(req.Path); err != nil {
		return nil, err
	}
	if req.OperationID == "" {
		req.OperationID = genOpID()
	}

	root, err := workspace.openRoot(ctx, req.Folder)
	if err != nil {
		return nil, err
	}
	defer root.close()

	// Check collision
	var stat unix.Stat_t
	if err := statPath(root.fd, req.Path, &stat); err == nil {
		if stat.Mode&unix.S_IFMT == unix.S_IFDIR {
			return &CreateDirResult{
				OperationID:   req.OperationID,
				Path:          req.Path,
				AlreadyExists: true,
				Completed:     true,
			}, nil
		}
		return nil, ErrStructuralConflict
	}

	journal := repository.FileMutationRecord{
		OperationID: req.OperationID,
		Folder:      req.Folder,
		Action:      "mkdir",
		DestPath:    req.Path,
		Phase:       "PLANNED",
	}
	if err := workspace.repo.RecordFileMutation(ctx, journal); err != nil {
		return nil, err
	}
	_ = workspace.repo.RecordOperationProgress(ctx, repository.OperationProgressRecord{
		OperationID: req.OperationID,
		Kind:        "mkdir",
		Phase:       "planning",
	})
	if err := workspace.callHook(HookFileMutationPlanned); err != nil {
		return nil, err
	}

	nowStr := workspace.now().UTC().Format(time.RFC3339Nano)
	envelope, err := workspace.repo.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           req.Folder,
		Path:             req.Path,
		Kind:             history.KindDirectory,
		AuthoredRevision: 1,
		DisplayTime:      nowStr,
		SkipProjection:   true,
	})
	if err != nil {
		_ = workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "ABORTED")
		return nil, err
	}

	if err := workspace.ApplyWithOperationID(ctx, envelope.ID, req.OperationID); err != nil {
		return nil, err
	}

	if err := workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "INSTALLED"); err != nil {
		return nil, err
	}
	if err := workspace.callHook(HookFileMutationInstalled); err != nil {
		return nil, err
	}

	if err := workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "COMPLETED"); err != nil {
		return nil, err
	}
	_ = workspace.repo.RecordOperationProgress(ctx, repository.OperationProgressRecord{
		OperationID: req.OperationID,
		Kind:        "mkdir",
		Phase:       "completed",
	})
	if err := workspace.callHook(HookFileMutationCompleted); err != nil {
		return nil, err
	}

	return &CreateDirResult{
		OperationID: req.OperationID,
		VersionID:   envelope.ID,
		Path:        req.Path,
		Completed:   true,
	}, nil
}

// Move renames or relocates a file or directory within the workspace root.
func (workspace *Workspace) Move(ctx context.Context, req MoveRequest) (*MoveResult, error) {
	if err := history.ValidatePath(req.SourcePath); err != nil {
		return nil, err
	}
	if err := history.ValidatePath(req.DestPath); err != nil {
		return nil, err
	}
	if req.SourcePath == req.DestPath {
		return nil, errors.New("source and destination paths are identical")
	}
	if strings.HasPrefix(req.DestPath, req.SourcePath+"/") {
		return nil, errors.New("cannot move directory into a subdirectory of itself")
	}
	if req.OperationID == "" {
		req.OperationID = genOpID()
	}

	root, err := workspace.openRoot(ctx, req.Folder)
	if err != nil {
		return nil, err
	}
	defer root.close()

	// Stat source
	var srcStat unix.Stat_t
	if err := statPath(root.fd, req.SourcePath, &srcStat); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	isDir := srcStat.Mode&unix.S_IFMT == unix.S_IFDIR

	// Destination collision check
	var destStat unix.Stat_t
	if err := statPath(root.fd, req.DestPath, &destStat); err == nil {
		destIsDir := destStat.Mode&unix.S_IFMT == unix.S_IFDIR
		if isDir != destIsDir {
			return nil, ErrStructuralConflict
		}
		if !req.Overwrite {
			return nil, ErrDestinationExists
		}
	}

	// Compute review tokens
	reviewedToken := req.ReviewedToken
	if !isDir {
		if reviewedToken == "" {
			srcHash, err := computePathHash(root.fd, req.SourcePath)
			if err != nil {
				return nil, err
			}
			reviewedToken = srcHash
		}
	} else {
		subToken, err := computeSubtreeToken(root.fd, req.SourcePath)
		if err != nil {
			return nil, err
		}
		if req.ReviewedToken != "" && req.ReviewedToken != subToken {
			return nil, ErrSubtreeInvalidated
		}
		reviewedToken = subToken
	}

	// Journal: PLANNED
	journal := repository.FileMutationRecord{
		OperationID:   req.OperationID,
		Folder:        req.Folder,
		Action:        "move",
		SourcePath:    req.SourcePath,
		DestPath:      req.DestPath,
		ReviewedToken: reviewedToken,
		Overwrite:     req.Overwrite,
		Phase:         "PLANNED",
	}
	if err := workspace.repo.RecordFileMutation(ctx, journal); err != nil {
		return nil, err
	}
	_ = workspace.repo.RecordOperationProgress(ctx, repository.OperationProgressRecord{
		OperationID: req.OperationID,
		Kind:        "move",
		Phase:       "planning",
	})
	if err := workspace.callHook(HookFileMutationPlanned); err != nil {
		return nil, err
	}

	// Advance to STAGED
	if err := workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "STAGED"); err != nil {
		return nil, err
	}
	_ = workspace.repo.RecordOperationProgress(ctx, repository.OperationProgressRecord{
		OperationID: req.OperationID,
		Kind:        "move",
		Phase:       "staging",
	})
	if err := workspace.callHook(HookFileMutationStaged); err != nil {
		return nil, err
	}

	if !isDir {
		return workspace.moveSingleFile(ctx, root, req, reviewedToken)
	}
	return workspace.moveDirectory(ctx, root, req, reviewedToken)
}

func (workspace *Workspace) moveSingleFile(ctx context.Context, root *openedRoot, req MoveRequest, reviewedHash string) (*MoveResult, error) {
	var manifest *history.Manifest
	if reviewedHash != "" {
		if proj, err := workspace.repo.Projection(ctx, req.Folder, req.SourcePath); err == nil && hex.EncodeToString(proj.Digest[:]) == reviewedHash && len(proj.Basis) > 0 {
			if m, _, err := workspace.repo.GetVersionManifest(ctx, proj.Basis[len(proj.Basis)-1]); err == nil && m != nil {
				manifest = m
			}
		}
		if manifest == nil {
			if historyItems, err := workspace.repo.FilePathHistory(ctx, req.Folder, req.SourcePath); err == nil {
				for _, item := range historyItems {
					if item.FileDigest == reviewedHash {
						var author history.ID
						authorBytes, _ := hex.DecodeString(item.AuthorID)
						copy(author[:], authorBytes)
						vid := history.VersionID{Folder: req.Folder, Author: author, Counter: item.Counter}
						if m, _, err := workspace.repo.GetVersionManifest(ctx, vid); err == nil && m != nil {
							manifest = m
							break
						}
					}
				}
			}
		}
	}

	if manifest == nil {
		// Read source bytes and store into CAS (or reuse existing projection manifest)
		fd, err := safeOpen(root.fd, req.SourcePath, unix.O_RDONLY|unix.O_NONBLOCK, 0)
		if err != nil {
			return nil, err
		}
		f := os.NewFile(uintptr(fd), req.SourcePath)
		var st unix.Stat_t
		_ = unix.Fstat(fd, &st)
		executable := st.Mode&0o111 != 0
		var storeErr error
		manifest, storeErr = workspace.repo.StoreFile(ctx, f, executable)
		f.Close()
		if storeErr != nil {
			_ = workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "ABORTED")
			return nil, storeErr
		}
	}

	// Author destination version
	nowStr := workspace.now().UTC().Format(time.RFC3339Nano)
	destEnv, err := workspace.repo.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           req.Folder,
		Path:             req.DestPath,
		Kind:             history.KindFile,
		Manifest:         manifest,
		AuthoredRevision: 1,
		DisplayTime:      nowStr,
		SkipProjection:   true,
	})
	if err != nil {
		_ = workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "ABORTED")
		return nil, err
	}

	// Apply destination to disk (displaces any existing destination into recovery)
	if err := workspace.ApplyWithOperationID(ctx, destEnv.ID, req.OperationID); err != nil {
		return nil, err
	}

	// Advance to INSTALLED
	if err := workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "INSTALLED"); err != nil {
		return nil, err
	}
	_ = workspace.repo.RecordOperationProgress(ctx, repository.OperationProgressRecord{
		OperationID: req.OperationID,
		Kind:        "move",
		Phase:       "installing",
	})
	if err := workspace.callHook(HookFileMutationInstalled); err != nil {
		return nil, err
	}

	// Revalidate source for concurrent editor modification (Invariant I26)
	currHash, err := computePathHash(root.fd, req.SourcePath)
	sourceModified := (err != nil || currHash != reviewedHash)

	if sourceModified {
		// Concurrent editor change detected! Source must NOT be deleted.
		_ = workspace.repo.SetFileMutationSourceRetained(ctx, req.OperationID, true)
		if err := workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "SOURCE_VERIFIED"); err != nil {
			return nil, err
		}
		if err := workspace.callHook(HookFileMutationSourceVerified); err != nil {
			return nil, err
		}

		if err := workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "COMPLETED"); err != nil {
			return nil, err
		}
		_ = workspace.repo.RecordOperationProgress(ctx, repository.OperationProgressRecord{
			OperationID: req.OperationID,
			Kind:        "move",
			Phase:       "completed",
			Details:     "source file concurrently modified; both copies preserved (Invariant I26)",
		})
		if err := workspace.callHook(HookFileMutationCompleted); err != nil {
			return nil, err
		}
		return &MoveResult{
			OperationID:    req.OperationID,
			SourcePath:     req.SourcePath,
			DestPath:       req.DestPath,
			SourceRetained: true,
			Completed:      true,
		}, nil
	}

	// Source unchanged: advance to SOURCE_VERIFIED
	if err := workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "SOURCE_VERIFIED"); err != nil {
		return nil, err
	}
	if err := workspace.callHook(HookFileMutationSourceVerified); err != nil {
		return nil, err
	}

	// Author tombstone for source
	srcTomb, err := workspace.repo.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           req.Folder,
		Path:             req.SourcePath,
		Kind:             history.KindTombstone,
		AuthoredRevision: 1,
		DisplayTime:      nowStr,
		SkipProjection:   true,
	})
	if err != nil {
		return nil, err
	}

	// Apply tombstone (unlinks source file safely)
	if err := workspace.Apply(ctx, srcTomb.ID); err != nil {
		return nil, err
	}

	if err := workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "COMPLETED"); err != nil {
		return nil, err
	}
	_ = workspace.repo.RecordOperationProgress(ctx, repository.OperationProgressRecord{
		OperationID: req.OperationID,
		Kind:        "move",
		Phase:       "completed",
	})
	if err := workspace.callHook(HookFileMutationCompleted); err != nil {
		return nil, err
	}

	return &MoveResult{
		OperationID:    req.OperationID,
		SourcePath:     req.SourcePath,
		DestPath:       req.DestPath,
		SourceRetained: false,
		Completed:      true,
	}, nil
}

type subtreeEntry struct {
	relPath string
	isDir   bool
}

func (workspace *Workspace) walkSubtree(rootFD int, dirPath string) ([]subtreeEntry, error) {
	var entries []subtreeEntry
	var recurse func(current string) error

	recurse = func(current string) error {
		fd, err := safeOpen(rootFD, current, unix.O_RDONLY|unix.O_DIRECTORY, 0)
		if err != nil {
			return err
		}
		f := os.NewFile(uintptr(fd), current)
		defer f.Close()

		des, err := f.ReadDir(-1)
		if err != nil {
			return err
		}
		for _, de := range des {
			childRel := current + "/" + de.Name()
			entries = append(entries, subtreeEntry{relPath: childRel, isDir: de.IsDir()})
			if de.IsDir() {
				if err := recurse(childRel); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := recurse(dirPath); err != nil {
		return nil, err
	}
	return entries, nil
}

func (workspace *Workspace) moveDirectory(ctx context.Context, root *openedRoot, req MoveRequest, reviewedToken string) (*MoveResult, error) {
	// Re-verify subtree token before beginning install
	tokenNow, err := computeSubtreeToken(root.fd, req.SourcePath)
	if err != nil {
		return nil, err
	}
	if tokenNow != reviewedToken {
		_ = workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "ABORTED")
		return nil, ErrSubtreeInvalidated
	}

	// Enumerate all descendants
	descendants, err := workspace.walkSubtree(root.fd, req.SourcePath)
	if err != nil {
		return nil, err
	}

	// Record entries
	mutationEntries := make([]repository.FileMutationEntry, 0, len(descendants)+1)
	// Add root directory itself as first entry
	mutationEntries = append(mutationEntries, repository.FileMutationEntry{
		OperationID: req.OperationID,
		Position:    0,
		SourcePath:  req.SourcePath,
		DestPath:    req.DestPath,
		Kind:        history.KindDirectory,
		Phase:       "PENDING",
	})
	for i, d := range descendants {
		suffix := strings.TrimPrefix(d.relPath, req.SourcePath)
		dst := req.DestPath + suffix
		k := history.KindFile
		if d.isDir {
			k = history.KindDirectory
		}
		mutationEntries = append(mutationEntries, repository.FileMutationEntry{
			OperationID: req.OperationID,
			Position:    i + 1,
			SourcePath:  d.relPath,
			DestPath:    dst,
			Kind:        k,
			Phase:       "PENDING",
		})
	}
	_ = workspace.repo.RecordFileMutationEntries(ctx, req.OperationID, mutationEntries)
	_ = workspace.repo.RecordOperationProgress(ctx, repository.OperationProgressRecord{
		OperationID:         req.OperationID,
		Kind:                "move",
		Phase:               "staging",
		ProgressDenominator: int64(len(mutationEntries)),
	})

	// Install destination entries: create destination directories first, then files
	nowStr := workspace.now().UTC().Format(time.RFC3339Nano)

	// Create destination root directory
	destEnv, err := workspace.repo.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           req.Folder,
		Path:             req.DestPath,
		Kind:             history.KindDirectory,
		AuthoredRevision: 1,
		DisplayTime:      nowStr,
		SkipProjection:   true,
	})
	if err == nil {
		_ = workspace.Apply(ctx, destEnv.ID)
	}

	progressCount := 1
	var anyRetained bool

	for _, e := range mutationEntries[1:] {
		// Check cooperative cancellation
		if workspace.isOperationCanceled(ctx, req.OperationID) {
			_ = workspace.repo.RecordOperationProgress(ctx, repository.OperationProgressRecord{
				OperationID: req.OperationID,
				Kind:        "move",
				Phase:       "canceled",
			})
			return nil, ErrOperationCanceled
		}

		if e.Kind == history.KindDirectory {
			dEnv, err := workspace.repo.CreateLocalVersion(ctx, repository.LocalVersionRequest{
				Folder:           req.Folder,
				Path:             e.DestPath,
				Kind:             history.KindDirectory,
				AuthoredRevision: 1,
				DisplayTime:      nowStr,
				SkipProjection:   true,
			})
			if err == nil {
				_ = workspace.Apply(ctx, dEnv.ID)
			}
		} else {
			// Copy file to dest
			fd, err := safeOpen(root.fd, e.SourcePath, unix.O_RDONLY|unix.O_NONBLOCK, 0)
			if err == nil {
				f := os.NewFile(uintptr(fd), e.SourcePath)
				var st unix.Stat_t
				_ = unix.Fstat(fd, &st)
				manifest, storeErr := workspace.repo.StoreFile(ctx, f, st.Mode&0o111 != 0)
				f.Close()
				if storeErr == nil {
					fEnv, fErr := workspace.repo.CreateLocalVersion(ctx, repository.LocalVersionRequest{
						Folder:           req.Folder,
						Path:             e.DestPath,
						Kind:             history.KindFile,
						Manifest:         manifest,
						AuthoredRevision: 1,
						DisplayTime:      nowStr,
						SkipProjection:   true,
					})
					if fErr == nil {
						_ = workspace.Apply(ctx, fEnv.ID)
					}
				}
			}
		}
		_ = workspace.repo.SetFileMutationEntryPhase(ctx, req.OperationID, e.Position, "COMPLETED", "")
		progressCount++
		_ = workspace.repo.RecordOperationProgress(ctx, repository.OperationProgressRecord{
			OperationID:       req.OperationID,
			Kind:              "move",
			Phase:             "installing",
			ProgressNumerator: int64(progressCount),
		})
	}

	if err := workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "INSTALLED"); err != nil {
		return nil, err
	}
	if err := workspace.callHook(HookFileMutationInstalled); err != nil {
		return nil, err
	}

	// Delete source entries in reverse order (files first, then directories)
	for i := len(mutationEntries) - 1; i >= 0; i-- {
		e := mutationEntries[i]
		if e.Kind == history.KindFile {
			// Verify file stability
			tomb, err := workspace.repo.CreateLocalVersion(ctx, repository.LocalVersionRequest{
				Folder:           req.Folder,
				Path:             e.SourcePath,
				Kind:             history.KindTombstone,
				AuthoredRevision: 1,
				DisplayTime:      nowStr,
				SkipProjection:   true,
			})
			if err == nil {
				_ = workspace.Apply(ctx, tomb.ID)
			}
		} else {
			// Directory: author tombstone and unlink
			tomb, err := workspace.repo.CreateLocalVersion(ctx, repository.LocalVersionRequest{
				Folder:           req.Folder,
				Path:             e.SourcePath,
				Kind:             history.KindTombstone,
				AuthoredRevision: 1,
				DisplayTime:      nowStr,
				SkipProjection:   true,
			})
			if err == nil {
				_ = workspace.Apply(ctx, tomb.ID)
			}
		}
	}

	if err := workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "SOURCE_VERIFIED"); err != nil {
		return nil, err
	}
	if err := workspace.callHook(HookFileMutationSourceVerified); err != nil {
		return nil, err
	}

	if err := workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "COMPLETED"); err != nil {
		return nil, err
	}
	_ = workspace.repo.RecordOperationProgress(ctx, repository.OperationProgressRecord{
		OperationID: req.OperationID,
		Kind:        "move",
		Phase:       "completed",
	})
	if err := workspace.callHook(HookFileMutationCompleted); err != nil {
		return nil, err
	}

	return &MoveResult{
		OperationID:    req.OperationID,
		SourcePath:     req.SourcePath,
		DestPath:       req.DestPath,
		SourceRetained: anyRetained,
		Completed:      true,
	}, nil
}

// Delete removes a file or directory recursively with safe tombstone publication.
func (workspace *Workspace) Delete(ctx context.Context, req DeleteRequest) (*DeleteResult, error) {
	if err := history.ValidatePath(req.Path); err != nil {
		return nil, err
	}
	if req.OperationID == "" {
		req.OperationID = genOpID()
	}

	root, err := workspace.openRoot(ctx, req.Folder)
	if err != nil {
		return nil, err
	}
	defer root.close()

	var stat unix.Stat_t
	if err := statPath(root.fd, req.Path, &stat); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return &DeleteResult{
				OperationID:    req.OperationID,
				Path:           req.Path,
				AlreadyDeleted: true,
				Completed:      true,
			}, nil
		}
		return nil, err
	}

	isDir := stat.Mode&unix.S_IFMT == unix.S_IFDIR

	// If directory and not recursive, check if empty
	var targets []subtreeEntry
	if isDir {
		descendants, err := workspace.walkSubtree(root.fd, req.Path)
		if err != nil {
			return nil, err
		}
		if len(descendants) > 0 && !req.Recursive {
			return nil, ErrDirectoryNotEmpty
		}
		targets = descendants
	}

	journal := repository.FileMutationRecord{
		OperationID: req.OperationID,
		Folder:      req.Folder,
		Action:      "delete",
		SourcePath:  req.Path,
		Phase:       "PLANNED",
	}
	if err := workspace.repo.RecordFileMutation(ctx, journal); err != nil {
		return nil, err
	}
	_ = workspace.repo.RecordOperationProgress(ctx, repository.OperationProgressRecord{
		OperationID: req.OperationID,
		Kind:        "delete",
		Phase:       "planning",
	})
	if err := workspace.callHook(HookFileMutationPlanned); err != nil {
		return nil, err
	}

	// Advance to STAGED
	if err := workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "STAGED"); err != nil {
		return nil, err
	}
	if err := workspace.callHook(HookFileMutationStaged); err != nil {
		return nil, err
	}

	// Advance to INSTALLED
	if err := workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "INSTALLED"); err != nil {
		return nil, err
	}
	if err := workspace.callHook(HookFileMutationInstalled); err != nil {
		return nil, err
	}

	nowStr := workspace.now().UTC().Format(time.RFC3339Nano)
	var deletedPaths []string

	if !isDir {
		tomb, err := workspace.repo.CreateLocalVersion(ctx, repository.LocalVersionRequest{
			Folder:           req.Folder,
			Path:             req.Path,
			Kind:             history.KindTombstone,
			AuthoredRevision: 1,
			DisplayTime:      nowStr,
			SkipProjection:   true,
		})
		if err != nil {
			return nil, err
		}
		if err := workspace.Apply(ctx, tomb.ID); err != nil {
			return nil, err
		}
		deletedPaths = append(deletedPaths, req.Path)
	} else {
		// Delete descendants in reverse order (deepest files first)
		for i := len(targets) - 1; i >= 0; i-- {
			if workspace.isOperationCanceled(ctx, req.OperationID) {
				_ = workspace.repo.RecordOperationProgress(ctx, repository.OperationProgressRecord{
					OperationID: req.OperationID,
					Kind:        "delete",
					Phase:       "canceled",
				})
				return nil, ErrOperationCanceled
			}

			target := targets[i]
			tomb, err := workspace.repo.CreateLocalVersion(ctx, repository.LocalVersionRequest{
				Folder:           req.Folder,
				Path:             target.relPath,
				Kind:             history.KindTombstone,
				AuthoredRevision: 1,
				DisplayTime:      nowStr,
				SkipProjection:   true,
			})
			if err == nil {
				_ = workspace.Apply(ctx, tomb.ID)
			}
			deletedPaths = append(deletedPaths, target.relPath)
		}

		// Finally delete the root directory itself
		rootTomb, err := workspace.repo.CreateLocalVersion(ctx, repository.LocalVersionRequest{
			Folder:           req.Folder,
			Path:             req.Path,
			Kind:             history.KindTombstone,
			AuthoredRevision: 1,
			DisplayTime:      nowStr,
			SkipProjection:   true,
		})
		if err == nil {
			_ = workspace.Apply(ctx, rootTomb.ID)
		}
		deletedPaths = append(deletedPaths, req.Path)
	}

	if err := workspace.repo.SetFileMutationPhase(ctx, req.OperationID, "COMPLETED"); err != nil {
		return nil, err
	}
	_ = workspace.repo.RecordOperationProgress(ctx, repository.OperationProgressRecord{
		OperationID: req.OperationID,
		Kind:        "delete",
		Phase:       "completed",
	})
	if err := workspace.callHook(HookFileMutationCompleted); err != nil {
		return nil, err
	}

	return &DeleteResult{
		OperationID:  req.OperationID,
		Path:         req.Path,
		DeletedCount: len(deletedPaths),
		Completed:    true,
		DeletedPaths: deletedPaths,
	}, nil
}

// RecoverFileMutations cleans up or completes interrupted mutations.
func (workspace *Workspace) RecoverFileMutations(ctx context.Context, folder history.ID) error {
	mutations, err := workspace.repo.ListIncompleteFileMutations(ctx, folder)
	if err != nil {
		return err
	}
	if len(mutations) == 0 {
		return nil
	}

	root, err := workspace.openRoot(ctx, folder)
	if err != nil {
		return err
	}
	defer root.close()

	for _, m := range mutations {
		switch m.Phase {
		case "PLANNED", "STAGED":
			// Nothing committed yet; abort safely
			if m.StagePath != "" {
				_ = unix.Unlinkat(root.scratch, m.StagePath, 0)
			}
			_ = workspace.repo.SetFileMutationPhase(ctx, m.OperationID, "ABORTED")

		case "INSTALLED":
			// Destination is installed; resume verifying and authoring source deletion if move
			if m.Action == "move" {
				currHash, err := computePathHash(root.fd, m.SourcePath)
				if err == nil && currHash == m.ReviewedToken {
					nowStr := workspace.now().UTC().Format(time.RFC3339Nano)
					tomb, err := workspace.repo.CreateLocalVersion(ctx, repository.LocalVersionRequest{
						Folder:           m.Folder,
						Path:             m.SourcePath,
						Kind:             history.KindTombstone,
						AuthoredRevision: 1,
						DisplayTime:      nowStr,
						SkipProjection:   true,
					})
					if err == nil {
						_ = workspace.Apply(ctx, tomb.ID)
					}
				} else {
					_ = workspace.repo.SetFileMutationSourceRetained(ctx, m.OperationID, true)
				}
			}
			_ = workspace.repo.SetFileMutationPhase(ctx, m.OperationID, "COMPLETED")

		case "SOURCE_VERIFIED":
			if m.Action == "move" && !m.SourceRetained {
				nowStr := workspace.now().UTC().Format(time.RFC3339Nano)
				tomb, err := workspace.repo.CreateLocalVersion(ctx, repository.LocalVersionRequest{
					Folder:           m.Folder,
					Path:             m.SourcePath,
					Kind:             history.KindTombstone,
					AuthoredRevision: 1,
					DisplayTime:      nowStr,
					SkipProjection:   true,
				})
				if err == nil {
					_ = workspace.Apply(ctx, tomb.ID)
				}
			}
			_ = workspace.repo.SetFileMutationPhase(ctx, m.OperationID, "COMPLETED")
		}
	}
	return nil
}

func (workspace *Workspace) isOperationCanceled(ctx context.Context, opID string) bool {
	prog, err := workspace.repo.GetOperationProgress(ctx, opID)
	if err == nil && prog != nil && prog.Canceled {
		return true
	}
	return false
}
