// Package workspace owns safe Linux root observation, capture, publication,
// and publication-journal recovery. It never interprets causal history beyond
// using the repository's persisted working basis.
package workspace

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/repository"
	"golang.org/x/sys/unix"
)

const scratchName = ".orbit-internal"
const markerName = "registration"

const (
	HookCaptureRead           = "workspace.capture.read"
	HookStageFlushed          = "publication.stage.flushed"
	HookBeforeExchange        = "publication.exchange.before"
	HookParentCreated         = "publication.parent.created"
	HookFilesystemTransition  = "publication.filesystem.transition"
	HookRecoveryNamed         = "publication.recovery.named"
	HookPublicationDirFlushed = "publication.directory.flushed"

	// O09 file mutation journal hooks
	HookFileMutationPlanned        = "file_mutation.planned"
	HookFileMutationStaged         = "file_mutation.staged"
	HookFileMutationInstalled      = "file_mutation.installed"
	HookFileMutationSourceVerified = "file_mutation.source_verified"
	HookFileMutationCompleted      = "file_mutation.completed"
)

var (
	ErrRootUnavailable    = errors.New("workspace root is unavailable or replaced")
	ErrUnsupportedEntry   = errors.New("workspace entry type is unsupported")
	ErrUnstableFile       = errors.New("UNSTABLE_FILE")
	ErrStructuralConflict = errors.New("structural conflict blocks publication")

	// O09 file mutation errors
	ErrDestinationExists   = errors.New("destination already exists without overwrite approval")
	ErrSubtreeInvalidated  = errors.New("directory subtree modified; reviewed token invalidated")
	ErrConcurrentSourceMod = errors.New("source modified concurrently during move; retained")
	ErrOperationCanceled   = errors.New("operation canceled")
	ErrDirectoryNotEmpty   = errors.New("directory not empty")
	ErrStaleReview         = errors.New("stale reviewed state; target was modified")
)

type FaultHook func(string) error

type MassDeletionPolicy struct {
	Absolute        int
	Ratio           float64
	MinimumForRatio int
}

func DefaultMassDeletionPolicy() MassDeletionPolicy {
	return MassDeletionPolicy{Absolute: 100, Ratio: 0.25, MinimumForRatio: 10}
}

func (policy MassDeletionPolicy) RequiresApproval(deleted, tracked int) bool {
	if deleted <= 0 || tracked <= 0 {
		return false
	}
	absolute := policy.Absolute > 0 && deleted >= policy.Absolute
	ratio := policy.Ratio > 0 && deleted >= policy.MinimumForRatio && float64(deleted)/float64(tracked) >= policy.Ratio
	return absolute || ratio
}

type Options struct {
	Random         io.Reader
	Now            func() time.Time
	CaptureRetries int
	DeletionPolicy MassDeletionPolicy
	FaultHook      FaultHook
}

type Workspace struct {
	ioGate         sync.RWMutex
	relocationMu   sync.Mutex
	folderMu       sync.Mutex
	folderGates    map[history.ID]chan struct{}
	repo           *repository.DB
	random         io.Reader
	now            func() time.Time
	captureRetries int
	deletionPolicy MassDeletionPolicy
	hook           FaultHook
}

func New(repo *repository.DB, options Options) *Workspace {
	if options.Random == nil {
		options.Random = rand.Reader
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.CaptureRetries <= 0 {
		options.CaptureRetries = 3
	}
	if options.DeletionPolicy == (MassDeletionPolicy{}) {
		options.DeletionPolicy = DefaultMassDeletionPolicy()
	}
	return &Workspace{repo: repo, random: options.Random, now: options.Now, captureRetries: options.CaptureRetries, deletionPolicy: options.DeletionPolicy, hook: options.FaultHook}
}

func (workspace *Workspace) callHook(name string) error {
	if workspace.hook == nil {
		return nil
	}
	if err := workspace.hook(name); err != nil {
		return fmt.Errorf("fault hook %s: %w", name, err)
	}
	return nil
}

func (workspace *Workspace) Register(ctx context.Context, folder history.ID, path string) (repository.RootRegistration, error) {
	return workspace.registerReviewed(ctx, folder, path, 0, 0)
}

func (workspace *Workspace) RegisterReviewed(ctx context.Context, folder history.ID, path string, device, inode uint64) (repository.RootRegistration, error) {
	return workspace.registerReviewed(ctx, folder, path, device, inode)
}

func (workspace *Workspace) registerReviewed(ctx context.Context, folder history.ID, path string, device, inode uint64) (repository.RootRegistration, error) {
	ctx, release := workspace.enterIO(ctx)
	defer release()
	if _, err := workspace.repo.Root(ctx, folder); err == nil {
		return repository.RootRegistration{}, errors.New("folder already has a registered root")
	} else if !errors.Is(err, repository.ErrRootNotRegistered) {
		return repository.RootRegistration{}, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return repository.RootRegistration{}, err
	}
	absolute = filepath.Clean(absolute)
	state, err := filepath.EvalSymlinks(workspace.repo.StateDir())
	if err != nil {
		return repository.RootRegistration{}, err
	}
	state, err = filepath.Abs(state)
	if err != nil {
		return repository.RootRegistration{}, err
	}
	if overlaps(absolute, filepath.Clean(state)) {
		return repository.RootRegistration{}, errors.New("workspace root and state directory overlap")
	}
	if err := workspace.repo.CheckRootCandidate(ctx, folder, absolute); err != nil {
		return repository.RootRegistration{}, err
	}
	rootFD, stat, err := openAbsoluteDirectory(absolute)
	if err != nil {
		return repository.RootRegistration{}, fmt.Errorf("open root without following symlinks: %w", err)
	}
	defer unix.Close(rootFD)
	if inode != 0 && (uint64(stat.Dev) != device || stat.Ino != inode) {
		return repository.RootRegistration{}, ErrRootUnavailable
	}
	var registrationID [32]byte
	if _, err := io.ReadFull(workspace.random, registrationID[:]); err != nil {
		return repository.RootRegistration{}, fmt.Errorf("create root registration identity: %w", err)
	}
	if err := ensureScratch(rootFD, uint64(stat.Dev), registrationID); err != nil {
		return repository.RootRegistration{}, err
	}
	registration := repository.RootRegistration{Folder: folder, Path: absolute, Device: uint64(stat.Dev), Inode: stat.Ino, RegistrationID: registrationID}
	if err := workspace.repo.RegisterRoot(ctx, registration); err != nil {
		return repository.RootRegistration{}, err
	}
	return registration, nil
}

func overlaps(a, b string) bool {
	within := func(parent, child string) bool {
		rel, err := filepath.Rel(parent, child)
		return err == nil && !filepath.IsAbs(rel) && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
	}
	return within(a, b) || within(b, a)
}

func openAbsoluteDirectory(path string) (int, unix.Stat_t, error) {
	var stat unix.Stat_t
	if !filepath.IsAbs(path) {
		return -1, stat, errors.New("root must be absolute")
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, stat, err
	}
	for _, component := range strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/") {
		if component == "" {
			continue
		}
		next, openErr := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		unix.Close(fd)
		if openErr != nil {
			return -1, stat, openErr
		}
		fd = next
	}
	if err := unix.Fstat(fd, &stat); err != nil {
		unix.Close(fd)
		return -1, stat, err
	}
	return fd, stat, nil
}

func ensureScratch(rootFD int, rootDevice uint64, registrationID [32]byte) error {
	if err := unix.Mkdirat(rootFD, scratchName, 0o700); err != nil && !errors.Is(err, syscall.EEXIST) {
		return fmt.Errorf("create root scratch: %w", err)
	}
	scratchFD, err := unix.Openat(rootFD, scratchName, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open root scratch: %w", err)
	}
	defer unix.Close(scratchFD)
	var stat unix.Stat_t
	if err := unix.Fstat(scratchFD, &stat); err != nil || uint64(stat.Dev) != rootDevice || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o077 != 0 {
		return errors.New("root scratch is not a private same-filesystem directory owned by this user")
	}
	markerFD, err := unix.Openat(scratchFD, markerName, unix.O_WRONLY|unix.O_CREAT|unix.O_TRUNC|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("create private root marker: %w", err)
	}
	var mStat unix.Stat_t
	if err := unix.Fstat(markerFD, &mStat); err != nil || mStat.Nlink != 1 || mStat.Mode&unix.S_IFREG == 0 {
		unix.Close(markerFD)
		return errors.New("root marker is not a single-link regular file")
	}
	marker := []byte(hex.EncodeToString(registrationID[:]) + "\n")
	if _, err := unix.Write(markerFD, marker); err != nil {
		unix.Close(markerFD)
		return fmt.Errorf("write root marker: %w", err)
	}
	if err := unix.Fsync(markerFD); err != nil {
		unix.Close(markerFD)
		return fmt.Errorf("flush root marker: %w", err)
	}
	if err := unix.Close(markerFD); err != nil {
		return err
	}
	if err := unix.Fsync(scratchFD); err != nil {
		return fmt.Errorf("flush scratch directory: %w", err)
	}
	return unix.Fsync(rootFD)
}

type openedRoot struct {
	fd           int
	scratch      int
	registration repository.RootRegistration
}

func (root *openedRoot) close() {
	unix.Close(root.scratch)
	unix.Close(root.fd)
}

func (workspace *Workspace) openRoot(ctx context.Context, folder history.ID) (*openedRoot, error) {
	if err := workspace.recoverRelocation(ctx, folder); err != nil {
		return nil, err
	}
	return workspace.openRootRegistered(ctx, folder)
}

func (workspace *Workspace) openRootRegistered(ctx context.Context, folder history.ID) (*openedRoot, error) {
	registration, err := workspace.repo.Root(ctx, folder)
	if err != nil {
		return nil, err
	}
	return workspace.openRegistration(registration)
}

func (workspace *Workspace) openRegistration(registration repository.RootRegistration) (*openedRoot, error) {
	fd, stat, err := openAbsoluteDirectory(registration.Path)
	if err != nil || uint64(stat.Dev) != registration.Device || stat.Ino != registration.Inode {
		if fd >= 0 {
			unix.Close(fd)
		}
		return nil, fmt.Errorf("%w: root device/inode check failed: %v", ErrRootUnavailable, err)
	}
	how := &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV}
	scratch, err := unix.Openat2(fd, scratchName, how)
	if err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("%w: invalid scratch directory: %v", ErrRootUnavailable, err)
	}
	var scratchStat unix.Stat_t
	if err := unix.Fstat(scratch, &scratchStat); err != nil || uint64(scratchStat.Dev) != registration.Device || scratchStat.Uid != uint32(os.Geteuid()) || scratchStat.Mode&0o077 != 0 {
		unix.Close(scratch)
		unix.Close(fd)
		return nil, fmt.Errorf("%w: scratch device/ownership/mode check failed: %v", ErrRootUnavailable, err)
	}
	marker, err := unix.Openat(scratch, markerName, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		unix.Close(scratch)
		unix.Close(fd)
		return nil, fmt.Errorf("%w: open registration marker: %v", ErrRootUnavailable, err)
	}
	var markerStat unix.Stat_t
	if err := unix.Fstat(marker, &markerStat); err != nil || markerStat.Mode&unix.S_IFMT != unix.S_IFREG || markerStat.Nlink != 1 || markerStat.Uid != uint32(os.Geteuid()) || markerStat.Mode&0o077 != 0 || markerStat.Size != 65 {
		unix.Close(marker)
		unix.Close(scratch)
		unix.Close(fd)
		return nil, fmt.Errorf("%w: registration marker type/ownership/size check failed: %v", ErrRootUnavailable, err)
	}
	data := make([]byte, 65)
	n, readErr := unix.Read(marker, data)
	unix.Close(marker)
	want := hex.EncodeToString(registration.RegistrationID[:]) + "\n"
	if readErr != nil || string(data[:n]) != want {
		unix.Close(scratch)
		unix.Close(fd)
		return nil, fmt.Errorf("%w: registration marker contents check failed: %v", ErrRootUnavailable, readErr)
	}
	return &openedRoot{fd: fd, scratch: scratch, registration: registration}, nil
}

func safeOpen(rootFD int, path string, flags uint64, mode uint64) (int, error) {
	if err := history.ValidatePath(path); err != nil {
		return -1, err
	}
	return unix.Openat2(rootFD, path, &unix.OpenHow{Flags: flags | unix.O_CLOEXEC | unix.O_NOFOLLOW, Mode: mode, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV})
}

type ScanIssue struct {
	Path string
	Code string
	Err  error
}

type DeletionPreview struct {
	Token      string
	Generation uint64
	Paths      []string
}

type ScanOptions struct {
	FullContent bool
	Paths       []string
}

type ScanResult struct {
	Captured []history.Envelope
	Issues   []ScanIssue
	Deletion *DeletionPreview
}

func (workspace *Workspace) Scan(ctx context.Context, folder history.ID) (ScanResult, error) {
	ctx, release, holdErr := workspace.enterFolder(ctx, folder)
	if holdErr != nil {
		return ScanResult{}, holdErr
	}
	defer release()
	return workspace.ScanWithOptions(ctx, folder, ScanOptions{FullContent: true})
}

func (workspace *Workspace) ScanWithOptions(ctx context.Context, folder history.ID, opts ScanOptions) (ScanResult, error) {
	ctx, release, holdErr := workspace.enterFolder(ctx, folder)
	if holdErr != nil {
		return ScanResult{}, holdErr
	}
	defer release()
	if err := workspace.Recover(ctx, folder); err != nil {
		return ScanResult{}, err
	}
	root, err := workspace.openRoot(ctx, folder)
	if err != nil {
		return ScanResult{}, err
	}
	defer root.close()
	generation, err := workspace.repo.BeginScan(ctx, folder)
	if err != nil {
		return ScanResult{}, err
	}
	scaffolds, err := workspace.repo.Scaffolds(ctx, folder)
	if err != nil {
		return ScanResult{}, err
	}
	result := ScanResult{}
	seen := map[string]bool{}
	failed := map[string]bool{}
	if err := workspace.walk(ctx, root, "", scaffolds, seen, failed, opts, &result); err != nil {
		return result, err
	}
	for path := range scaffolds {
		if !seen[path] && !underFailed(path, failed) {
			if err := workspace.repo.RemoveAnyScaffold(ctx, folder, path); err != nil {
				return result, err
			}
		}
	}
	projections, err := workspace.repo.Projections(ctx, folder)
	if err != nil {
		return result, err
	}
	var deleted []string
	tracked := 0
	for _, projection := range projections {
		if projection.Kind != history.KindFile && projection.Kind != history.KindDirectory {
			continue
		}
		tracked++
		if !seen[projection.Path] && !underFailed(projection.Path, failed) {
			deleted = append(deleted, projection.Path)
		}
	}
	sort.Strings(deleted)
	if root.registration.BootstrapComplete && workspace.deletionPolicy.RequiresApproval(len(deleted), tracked) {
		token, err := workspace.randomToken()
		if err != nil {
			return result, err
		}
		if err := workspace.repo.SaveDeletionProposal(ctx, folder, generation, token, deleted); err != nil {
			return result, err
		}
		result.Deletion = &DeletionPreview{Token: token, Generation: generation, Paths: deleted}
	} else if root.registration.BootstrapComplete {
		for _, path := range deleted {
			projection, err := workspace.repo.Projection(ctx, folder, path)
			if err != nil {
				return result, err
			}
			envelope, err := workspace.repo.CreateLocalVersion(ctx, repository.LocalVersionRequest{Folder: folder, Path: path, Basis: projection.Basis, Kind: history.KindTombstone, AuthoredRevision: 0, DisplayTime: workspace.now().UTC().Format(time.RFC3339Nano)})
			if err != nil {
				return result, err
			}
			result.Captured = append(result.Captured, envelope)
		}
	} else if len(failed) == 0 {
		if err := workspace.repo.MarkBootstrapComplete(ctx, folder); err != nil {
			return result, err
		}
	}
	return result, nil
}

func underFailed(path string, failed map[string]bool) bool {
	for prefix := range failed {
		if prefix == "" || path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func (workspace *Workspace) walk(ctx context.Context, root *openedRoot, directory string, scaffolds map[string]bool, seen, failed map[string]bool, opts ScanOptions, result *ScanResult) error {
	fd := root.fd
	owned := false
	if directory != "" {
		var err error
		fd, err = safeOpen(root.fd, directory, unix.O_RDONLY|unix.O_DIRECTORY, 0)
		if err != nil {
			failed[directory] = true
			result.Issues = append(result.Issues, ScanIssue{directory, "INCOMPLETE_SUBTREE", err})
			return nil
		}
		owned = true
	}
	if owned {
		defer unix.Close(fd)
	}
	duplicate, err := unix.Dup(fd)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(duplicate), directory)
	entries, err := file.ReadDir(-1)
	file.Close()
	if err != nil {
		failed[directory] = true
		result.Issues = append(result.Issues, ScanIssue{directory, "INCOMPLETE_SUBTREE", err})
		return nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if directory == "" && entry.Name() == scratchName {
			continue
		}
		path := entry.Name()
		if directory != "" {
			path = directory + "/" + entry.Name()
		}
		if err := history.ValidatePath(path); err != nil {
			failed[path] = true
			result.Issues = append(result.Issues, ScanIssue{path, "INVALID_PATH", err})
			continue
		}
		var stat unix.Stat_t
		if err := unix.Fstatat(fd, entry.Name(), &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			failed[path] = true
			result.Issues = append(result.Issues, ScanIssue{path, "INCOMPLETE_SUBTREE", err})
			continue
		}
		if uint64(stat.Dev) != root.registration.Device {
			failed[path] = true
			result.Issues = append(result.Issues, ScanIssue{path, "NESTED_MOUNT", ErrUnsupportedEntry})
			continue
		}
		switch stat.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
			seen[path] = true
			if !scaffolds[path] {
				envelope, changed, err := workspace.captureDirectory(ctx, root.registration.Folder, path)
				if err != nil {
					failed[path] = true
					result.Issues = append(result.Issues, ScanIssue{path, "CAPTURE_FAILED", err})
				} else if changed {
					result.Captured = append(result.Captured, envelope)
				}
			}
			if err := workspace.walk(ctx, root, path, scaffolds, seen, failed, opts, result); err != nil {
				return err
			}
		case unix.S_IFREG:
			seen[path] = true
			if scaffolds[path] {
				if err := workspace.repo.RemoveAnyScaffold(ctx, root.registration.Folder, path); err != nil {
					return err
				}
				delete(scaffolds, path)
			}
			if stat.Nlink != 1 {
				failed[path] = true
				result.Issues = append(result.Issues, ScanIssue{path, "HARD_LINK_UNSUPPORTED", ErrUnsupportedEntry})
				continue
			}
			if !opts.FullContent {
				projection, err := workspace.repo.Projection(ctx, root.registration.Folder, path)
				if err == nil && projection.Kind == history.KindFile && projection.Digest != (history.Digest{}) {
					if projection.ObservedSize == uint64(stat.Size) &&
						projection.ObservedMtimeNS == stat.Mtim.Nano() &&
						projection.ObservedInode == uint64(stat.Ino) &&
						((stat.Mode&0o111 != 0) == projection.Executable) {
						continue
					}
				}
			}
			envelope, changed, err := workspace.captureFile(ctx, root, path)
			if err != nil {
				failed[path] = true
				code := "CAPTURE_FAILED"
				if errors.Is(err, ErrUnstableFile) {
					code = "UNSTABLE_FILE"
				}
				result.Issues = append(result.Issues, ScanIssue{path, code, err})
				workspace.repo.SetPathBlock(ctx, root.registration.Folder, path, code)
			} else if changed {
				result.Captured = append(result.Captured, envelope)
			}
		default:
			failed[path] = true
			result.Issues = append(result.Issues, ScanIssue{path, "UNSUPPORTED_OBJECT", ErrUnsupportedEntry})
		}
	}
	return nil
}

func (workspace *Workspace) captureDirectory(ctx context.Context, folder history.ID, path string) (history.Envelope, bool, error) {
	projection, err := workspace.repo.Projection(ctx, folder, path)
	if err == nil && projection.Kind == history.KindDirectory && projection.BlockReason == "" {
		return history.Envelope{}, false, nil
	}
	var basis []history.VersionID
	if err == nil {
		basis = projection.Basis
	} else if !errors.Is(err, sql.ErrNoRows) {
		return history.Envelope{}, false, err
	}
	envelope, err := workspace.repo.CreateLocalVersion(ctx, repository.LocalVersionRequest{Folder: folder, Path: path, Basis: basis, Kind: history.KindDirectory, AuthoredRevision: 0, DisplayTime: workspace.now().UTC().Format(time.RFC3339Nano)})
	return envelope, err == nil, err
}

func (workspace *Workspace) captureFile(ctx context.Context, root *openedRoot, path string) (history.Envelope, bool, error) {
	var last error
	for attempt := 0; attempt < workspace.captureRetries; attempt++ {
		envelope, changed, err := workspace.captureFileOnce(ctx, root, path)
		if !errors.Is(err, ErrUnstableFile) {
			return envelope, changed, err
		}
		last = err
	}
	return history.Envelope{}, false, fmt.Errorf("%w after %d attempts: %v", ErrUnstableFile, workspace.captureRetries, last)
}

func (workspace *Workspace) captureFileOnce(ctx context.Context, root *openedRoot, path string) (history.Envelope, bool, error) {
	fd, err := safeOpen(root.fd, path, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return history.Envelope{}, false, err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	var before unix.Stat_t
	if err := unix.Fstat(fd, &before); err != nil {
		return history.Envelope{}, false, err
	}
	if before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 {
		return history.Envelope{}, false, ErrUnsupportedEntry
	}
	executable := before.Mode&0o111 != 0
	manifest, err := workspace.repo.StoreFile(ctx, file, executable)
	if err != nil {
		return history.Envelope{}, false, err
	}
	if err := workspace.callHook(HookCaptureRead); err != nil {
		return history.Envelope{}, false, err
	}
	var after, named unix.Stat_t
	if err := unix.Fstat(fd, &after); err != nil {
		return history.Envelope{}, false, err
	}
	if err := statPath(root.fd, path, &named); err != nil {
		return history.Envelope{}, false, ErrUnstableFile
	}
	if !sameObservation(before, after) || before.Dev != named.Dev || before.Ino != named.Ino || named.Mode&unix.S_IFMT != unix.S_IFREG || uint64(after.Size) != manifest.Size {
		return history.Envelope{}, false, ErrUnstableFile
	}
	projection, projectionErr := workspace.repo.Projection(ctx, root.registration.Folder, path)
	if projectionErr == nil && projection.Kind == history.KindFile && projection.Digest == manifest.Digest && projection.Executable == manifest.Executable {
		_ = workspace.repo.UpdateObservedStat(ctx, root.registration.Folder, path, uint64(named.Size), named.Mtim.Nano(), named.Ctim.Nano(), uint64(named.Ino))
		return history.Envelope{}, false, nil
	}
	var basis []history.VersionID
	if projectionErr == nil {
		basis = projection.Basis
	} else if !errors.Is(projectionErr, sql.ErrNoRows) {
		return history.Envelope{}, false, projectionErr
	}
	envelope, err := workspace.repo.CreateLocalVersion(ctx, repository.LocalVersionRequest{Folder: root.registration.Folder, Path: path, Basis: basis, Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 0, DisplayTime: workspace.now().UTC().Format(time.RFC3339Nano)})
	if err == nil {
		_ = workspace.repo.UpdateObservedStat(ctx, root.registration.Folder, path, uint64(named.Size), named.Mtim.Nano(), named.Ctim.Nano(), uint64(named.Ino))
	}
	return envelope, err == nil, err
}

func sameObservation(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Size == b.Size && a.Mtim == b.Mtim && a.Ctim == b.Ctim && a.Mode == b.Mode
}

func (workspace *Workspace) randomToken() (string, error) {
	var raw [16]byte
	if _, err := io.ReadFull(workspace.random, raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func (workspace *Workspace) ApproveDeletions(ctx context.Context, folder history.ID, token string) ([]history.Envelope, error) {
	ctx, release, holdErr := workspace.enterFolder(ctx, folder)
	if holdErr != nil {
		return nil, holdErr
	}
	defer release()
	_, paths, err := workspace.repo.DeletionProposal(ctx, folder, token)
	if err != nil {
		return nil, err
	}
	root, err := workspace.openRoot(ctx, folder)
	if err != nil {
		return nil, err
	}
	defer root.close()
	for _, path := range paths {
		var stat unix.Stat_t
		if err := statPath(root.fd, path, &stat); err == nil {
			return nil, repository.ErrStaleGeneration
		} else if !errors.Is(err, syscall.ENOENT) {
			return nil, err
		}
	}
	var result []history.Envelope
	for _, path := range paths {
		projection, err := workspace.repo.Projection(ctx, folder, path)
		if err != nil {
			return result, err
		}
		if projection.Kind == history.KindTombstone {
			continue
		}
		envelope, err := workspace.repo.CreateLocalVersion(ctx, repository.LocalVersionRequest{Folder: folder, Path: path, Basis: projection.Basis, Kind: history.KindTombstone, AuthoredRevision: 0, DisplayTime: workspace.now().UTC().Format(time.RFC3339Nano)})
		if err != nil {
			return result, err
		}
		result = append(result, envelope)
	}
	if err := workspace.repo.FinishDeletionProposal(ctx, folder, token, false); err != nil {
		return result, err
	}
	return result, nil
}

func (workspace *Workspace) Apply(ctx context.Context, id history.VersionID) error {
	ctx, release, holdErr := workspace.enterFolder(ctx, id.Folder)
	if holdErr != nil {
		return holdErr
	}
	defer release()
	return workspace.ApplyWithOperationID(ctx, id, "")
}

func (workspace *Workspace) ApplyWithOperationID(ctx context.Context, id history.VersionID, opID string) error {
	ctx, release, holdErr := workspace.enterFolder(ctx, id.Folder)
	if holdErr != nil {
		return holdErr
	}
	defer release()
	if err := workspace.Recover(ctx, id.Folder); err != nil {
		return err
	}
	if err := workspace.repo.VerifyVersionContent(ctx, id); err != nil {
		return err
	}
	envelope, err := workspace.repo.Envelope(ctx, id)
	if err != nil {
		return err
	}
	root, err := workspace.openRoot(ctx, id.Folder)
	if err != nil {
		return err
	}
	defer root.close()
	if err := workspace.guardApplyTarget(ctx, root, envelope); err != nil {
		return err
	}
	operation := opID
	if operation == "" {
		var err error
		operation, err = workspace.randomToken()
		if err != nil {
			return err
		}
	}
	publication := repository.Publication{OperationID: operation, Folder: id.Folder, Path: envelope.Path, Intended: id, Kind: envelope.Kind, StagePath: "stage-" + operation, RecoveryPath: "recovery-" + operation}
	if err := workspace.repo.PreparePublication(ctx, publication); err != nil {
		return err
	}
	reserveBytes := uint64(0)
	if envelope.Manifest != nil {
		reserveBytes = envelope.Manifest.Size
	}
	var previous unix.Stat_t
	if err := statPath(root.fd, envelope.Path, &previous); err == nil && previous.Mode&unix.S_IFMT == unix.S_IFREG && previous.Size > 0 {
		if uint64(previous.Size) > ^uint64(0)-reserveBytes {
			_ = workspace.repo.AbortPublication(ctx, operation)
			return repository.ErrBudgetExceeded
		}
		reserveBytes += uint64(previous.Size)
	}
	if reserveBytes > 0 {
		if err := workspace.repo.Reserve(ctx, "publication-"+operation, reserveBytes, "workspace staging and recovery"); err != nil {
			_ = workspace.repo.AbortPublication(ctx, operation)
			return err
		}
	}
	switch envelope.Kind {
	case history.KindFile:
		return workspace.applyFile(ctx, root, publication, envelope)
	case history.KindDirectory:
		return workspace.applyDirectory(ctx, root, publication)
	case history.KindTombstone:
		return workspace.applyTombstone(ctx, root, publication)
	default:
		return history.ErrInvalidEnvelope
	}
}

// PathExists reports whether path currently exists on disk beneath the workspace root.
// If no workspace root is registered for this folder, it returns false, nil.
func (workspace *Workspace) PathExists(ctx context.Context, folder history.ID, path string) (bool, error) {
	ctx, release := workspace.enterIO(ctx)
	defer release()
	root, err := workspace.openRoot(ctx, folder)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	defer root.close()
	var st unix.Stat_t
	if err := statPath(root.fd, path, &st); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (workspace *Workspace) guardApplyTarget(ctx context.Context, root *openedRoot, envelope history.Envelope) error {
	projection, err := workspace.repo.Projection(ctx, envelope.ID.Folder, envelope.Path)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	fd, openErr := safeOpen(root.fd, envelope.Path, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if errors.Is(openErr, syscall.ENOENT) {
		if err == nil && projection.Kind != history.KindTombstone && projection.Kind != 0 {
			workspace.repo.SetPathBlock(ctx, envelope.ID.Folder, envelope.Path, "UNOBSERVED_LOCAL_DELETION")
			return ErrStructuralConflict
		}
		return nil
	}
	if openErr != nil {
		if errors.Is(openErr, syscall.ENOTDIR) || errors.Is(openErr, syscall.ELOOP) {
			workspace.repo.SetPathBlock(ctx, envelope.ID.Folder, envelope.Path, "STRUCTURAL_CONFLICT")
			return ErrStructuralConflict
		}
		return openErr
	}
	file := os.NewFile(uintptr(fd), envelope.Path)
	defer file.Close()
	stat, statErr := file.Stat()
	if statErr != nil {
		return statErr
	}
	if stat.Mode().IsRegular() {
		if err != nil || projection.Kind != history.KindFile || projection.BlockReason != "" {
			workspace.repo.SetPathBlock(ctx, envelope.ID.Folder, envelope.Path, "UNOBSERVED_LOCAL_CONTENT")
			return ErrStructuralConflict
		}
		hasher := sha256.New()
		if _, err := io.Copy(hasher, file); err != nil {
			return err
		}
		if !bytes.Equal(hasher.Sum(nil), projection.Digest[:]) || (stat.Mode().Perm()&0o111 != 0) != projection.Executable {
			workspace.repo.SetPathBlock(ctx, envelope.ID.Folder, envelope.Path, "UNOBSERVED_LOCAL_CONTENT")
			return ErrStructuralConflict
		}
		return nil
	}
	if stat.IsDir() && err == nil && projection.Kind == history.KindDirectory && (envelope.Kind == history.KindDirectory || envelope.Kind == history.KindTombstone) {
		return nil
	}
	workspace.repo.SetPathBlock(ctx, envelope.ID.Folder, envelope.Path, "STRUCTURAL_CONFLICT")
	return ErrStructuralConflict
}

func (workspace *Workspace) ensureParents(ctx context.Context, root *openedRoot, folder history.ID, path string) (int, string, error) {
	parts := strings.Split(path, "/")
	parent := root.fd
	owned := false
	current := ""
	for _, component := range parts[:len(parts)-1] {
		if current == "" {
			current = component
		} else {
			current += "/" + component
		}
		next, err := safeOpen(root.fd, current, unix.O_RDONLY|unix.O_DIRECTORY, 0)
		if errors.Is(err, syscall.ENOENT) {
			if err := workspace.repo.MarkScaffold(ctx, folder, current); err != nil {
				if owned {
					unix.Close(parent)
				}
				return -1, "", err
			}
			if err := unix.Mkdirat(parent, component, 0o700); err != nil {
				_ = workspace.repo.RemoveScaffold(ctx, folder, current)
				if owned {
					unix.Close(parent)
				}
				return -1, "", err
			}
			if err := workspace.callHook(HookParentCreated); err != nil {
				if owned {
					unix.Close(parent)
				}
				return -1, "", err
			}
			if err := unix.Fsync(parent); err != nil {
				if owned {
					unix.Close(parent)
				}
				return -1, "", err
			}
			if err := workspace.repo.CompleteScaffold(ctx, folder, current); err != nil {
				if owned {
					unix.Close(parent)
				}
				return -1, "", err
			}
			next, err = safeOpen(root.fd, current, unix.O_RDONLY|unix.O_DIRECTORY, 0)
		}
		if err != nil {
			if owned {
				unix.Close(parent)
			}
			workspace.repo.SetPathBlock(ctx, folder, path, "STRUCTURAL_CONFLICT")
			return -1, "", ErrStructuralConflict
		}
		if owned {
			unix.Close(parent)
		}
		parent, owned = next, true
	}
	if !owned {
		duplicate, err := unix.Dup(root.fd)
		if err != nil {
			return -1, "", err
		}
		parent = duplicate
	}
	return parent, parts[len(parts)-1], nil
}

func (workspace *Workspace) applyFile(ctx context.Context, root *openedRoot, publication repository.Publication, envelope history.Envelope) error {
	stageFD, err := unix.Openat(root.scratch, publication.StagePath, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	stage := os.NewFile(uintptr(stageFD), publication.StagePath)
	_, writeErr := workspace.repo.WriteVersion(ctx, envelope.ID, stage)
	if writeErr == nil {
		mode := os.FileMode(0o600)
		if envelope.Manifest.Executable {
			mode = 0o700
		}
		writeErr = stage.Chmod(mode)
	}
	if writeErr == nil {
		writeErr = stage.Sync()
	}
	closeErr := stage.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := workspace.callHook(HookStageFlushed); err != nil {
		return err
	}
	if err := workspace.repo.SetPublicationPhase(ctx, publication.OperationID, "STAGED"); err != nil {
		return err
	}
	parent, name, err := workspace.ensureParents(ctx, root, envelope.ID.Folder, envelope.Path)
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	var target unix.Stat_t
	targetErr := unix.Fstatat(parent, name, &target, unix.AT_SYMLINK_NOFOLLOW)
	if targetErr == nil && (target.Mode&unix.S_IFMT != unix.S_IFREG || target.Nlink != 1) {
		workspace.repo.SetPathBlock(ctx, envelope.ID.Folder, envelope.Path, "STRUCTURAL_CONFLICT")
		return ErrStructuralConflict
	}
	if targetErr != nil && !errors.Is(targetErr, syscall.ENOENT) {
		return targetErr
	}
	if err := workspace.repo.SetPublicationPhase(ctx, publication.OperationID, "REPLACEMENT_INTENT"); err != nil {
		return err
	}
	if err := workspace.callHook(HookBeforeExchange); err != nil {
		return err
	}
	if targetErr == nil {
		if err := unix.Renameat2(root.scratch, publication.StagePath, parent, name, unix.RENAME_EXCHANGE); err != nil {
			return fmt.Errorf("exchange publication: %w", err)
		}
		if err := workspace.callHook(HookFilesystemTransition); err != nil {
			return err
		}
		if err := unix.Renameat2(root.scratch, publication.StagePath, root.scratch, publication.RecoveryPath, unix.RENAME_NOREPLACE); err != nil {
			return fmt.Errorf("name displaced recovery file: %w", err)
		}
		if err := workspace.callHook(HookRecoveryNamed); err != nil {
			return err
		}
	} else {
		if err := unix.Renameat2(root.scratch, publication.StagePath, parent, name, unix.RENAME_NOREPLACE); err != nil {
			return fmt.Errorf("publish new file: %w", err)
		}
		if err := workspace.callHook(HookFilesystemTransition); err != nil {
			return err
		}
	}
	if err := unix.Fsync(parent); err != nil {
		return err
	}
	if err := unix.Fsync(root.scratch); err != nil {
		return err
	}
	if err := workspace.callHook(HookPublicationDirFlushed); err != nil {
		return err
	}
	if matches, _, err := workspace.targetMatches(root, envelope); err != nil || !matches {
		_ = workspace.repo.SetPathBlock(ctx, publication.Folder, publication.Path, "AMBIGUOUS_PUBLICATION")
		if err != nil {
			return err
		}
		return ErrStructuralConflict
	}
	if err := workspace.repo.SetPublicationPhase(ctx, publication.OperationID, "FILESYSTEM_PUBLISHED"); err != nil {
		return err
	}
	lateCandidate, err := workspace.recoveryDiffersFromProjection(ctx, root, publication)
	if err != nil {
		return err
	}
	if lateCandidate {
		if err := workspace.repo.SetPathBlock(ctx, publication.Folder, publication.Path, "LATE_EDITOR_CANDIDATE"); err != nil {
			return err
		}
	}
	if err := workspace.repo.CommitPublication(ctx, publication.OperationID); err != nil {
		return err
	}
	return workspace.releaseIfNoRecovery(ctx, root, publication)
}

func (workspace *Workspace) releaseIfNoRecovery(ctx context.Context, root *openedRoot, publication repository.Publication) error {
	if existsAt(root.scratch, publication.RecoveryPath) {
		return nil
	}
	return workspace.repo.ReleaseReservation(ctx, "publication-"+publication.OperationID)
}

func (workspace *Workspace) abortPublication(ctx context.Context, publication repository.Publication) error {
	if err := workspace.repo.AbortPublication(ctx, publication.OperationID); err != nil {
		return err
	}
	return workspace.repo.ReleaseReservation(ctx, "publication-"+publication.OperationID)
}

func (workspace *Workspace) recoveryDiffersFromProjection(ctx context.Context, root *openedRoot, publication repository.Publication) (bool, error) {
	fd, err := unix.Openat(root.scratch, publication.RecoveryPath, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if errors.Is(err, syscall.ENOENT) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	file := os.NewFile(uintptr(fd), publication.RecoveryPath)
	defer file.Close()
	projection, err := workspace.repo.Projection(ctx, publication.Folder, publication.Path)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if projection.Kind != history.KindFile {
		return true, nil
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return false, err
	}
	return !bytes.Equal(hasher.Sum(nil), projection.Digest[:]), nil
}

func (workspace *Workspace) applyDirectory(ctx context.Context, root *openedRoot, publication repository.Publication) error {
	parent, name, err := workspace.ensureParents(ctx, root, publication.Folder, publication.Path)
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	var stat unix.Stat_t
	err = unix.Fstatat(parent, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, syscall.ENOENT) {
		if err := workspace.repo.SetPublicationPhase(ctx, publication.OperationID, "REPLACEMENT_INTENT"); err != nil {
			return err
		}
		if err := unix.Mkdirat(parent, name, 0o700); err != nil {
			return err
		}
		if err := workspace.callHook(HookFilesystemTransition); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		workspace.repo.SetPathBlock(ctx, publication.Folder, publication.Path, "STRUCTURAL_CONFLICT")
		return ErrStructuralConflict
	}
	if err := unix.Fsync(parent); err != nil {
		return err
	}
	if err := workspace.repo.SetPublicationPhase(ctx, publication.OperationID, "FILESYSTEM_PUBLISHED"); err != nil {
		return err
	}
	if err := workspace.repo.CommitPublication(ctx, publication.OperationID); err != nil {
		return err
	}
	return workspace.releaseIfNoRecovery(ctx, root, publication)
}

func (workspace *Workspace) applyTombstone(ctx context.Context, root *openedRoot, publication repository.Publication) error {
	parent, name, err := workspace.ensureParents(ctx, root, publication.Folder, publication.Path)
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	var stat unix.Stat_t
	err = unix.Fstatat(parent, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, syscall.ENOENT) {
		if err := workspace.repo.SetPublicationPhase(ctx, publication.OperationID, "FILESYSTEM_PUBLISHED"); err != nil {
			return err
		}
		if err := workspace.repo.CommitPublication(ctx, publication.OperationID); err != nil {
			return err
		}
		if err := workspace.releaseIfNoRecovery(ctx, root, publication); err != nil {
			return err
		}
		_ = workspace.pruneEmptyScaffolds(ctx, root, publication.Folder, publication.Path)
		return nil
	}
	if err != nil {
		return err
	}
	if err := workspace.repo.SetPublicationPhase(ctx, publication.OperationID, "REPLACEMENT_INTENT"); err != nil {
		return err
	}
	switch stat.Mode & unix.S_IFMT {
	case unix.S_IFREG:
		if stat.Nlink != 1 {
			return ErrUnsupportedEntry
		}
		if err := unix.Renameat2(parent, name, root.scratch, publication.RecoveryPath, unix.RENAME_NOREPLACE); err != nil {
			return err
		}
	case unix.S_IFDIR:
		if err := unix.Unlinkat(parent, name, unix.AT_REMOVEDIR); err != nil {
			workspace.repo.SetPathBlock(ctx, publication.Folder, publication.Path, "STRUCTURAL_CONFLICT")
			return ErrStructuralConflict
		}
	default:
		return ErrUnsupportedEntry
	}
	if err := workspace.callHook(HookFilesystemTransition); err != nil {
		return err
	}
	if err := unix.Fsync(parent); err != nil {
		return err
	}
	if err := unix.Fsync(root.scratch); err != nil {
		return err
	}
	if err := workspace.repo.SetPublicationPhase(ctx, publication.OperationID, "FILESYSTEM_PUBLISHED"); err != nil {
		return err
	}
	if err := workspace.repo.CommitPublication(ctx, publication.OperationID); err != nil {
		return err
	}
	if err := workspace.releaseIfNoRecovery(ctx, root, publication); err != nil {
		return err
	}
	_ = workspace.pruneEmptyScaffolds(ctx, root, publication.Folder, publication.Path)
	return nil
}

func (workspace *Workspace) pruneEmptyScaffolds(ctx context.Context, root *openedRoot, folder history.ID, childPath string) error {
	for parent := parentDir(childPath); parent != ""; parent = parentDir(parent) {
		isScaffold, err := workspace.repo.IsScaffold(ctx, folder, parent)
		if err != nil || !isScaffold {
			break
		}
		hasActive, err := workspace.repo.HasActiveDescendantProjections(ctx, folder, parent)
		if err != nil || hasActive {
			break
		}
		grandParent := root.fd
		name := parent
		owned := false
		if idx := strings.LastIndex(parent, "/"); idx >= 0 {
			gpDir := parent[:idx]
			name = parent[idx+1:]
			fd, err := safeOpen(root.fd, gpDir, unix.O_RDONLY|unix.O_DIRECTORY, 0)
			if err != nil {
				break
			}
			grandParent = fd
			owned = true
		}
		unlinkErr := unix.Unlinkat(grandParent, name, unix.AT_REMOVEDIR)
		if unlinkErr == nil {
			_ = unix.Fsync(grandParent)
			_ = workspace.repo.RemoveAnyScaffold(ctx, folder, parent)
		}
		if owned {
			unix.Close(grandParent)
		}
		if unlinkErr != nil {
			break
		}
	}
	return nil
}

func parentDir(p string) string {
	idx := strings.LastIndex(p, "/")
	if idx <= 0 {
		return ""
	}
	return p[:idx]
}

func (workspace *Workspace) Recover(ctx context.Context, folder history.ID) error {
	ctx, release, holdErr := workspace.enterFolder(ctx, folder)
	if holdErr != nil {
		return holdErr
	}
	defer release()
	if err := workspace.RecoverFileMutations(ctx, folder); err != nil {
		return err
	}
	publications, err := workspace.repo.Publications(ctx, folder)
	if err != nil {
		return err
	}
	pendingScaffolds, err := workspace.repo.PendingScaffolds(ctx, folder)
	if err != nil {
		return err
	}
	if len(publications) == 0 && len(pendingScaffolds) == 0 {
		return nil
	}
	root, err := workspace.openRoot(ctx, folder)
	if err != nil {
		return err
	}
	defer root.close()
	for _, path := range pendingScaffolds {
		var stat unix.Stat_t
		err := statPath(root.fd, path, &stat)
		if errors.Is(err, syscall.ENOENT) {
			if err := workspace.repo.RemoveScaffold(ctx, folder, path); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if stat.Mode&unix.S_IFMT != unix.S_IFDIR || uint64(stat.Dev) != root.registration.Device {
			if err := workspace.repo.RemoveScaffold(ctx, folder, path); err != nil {
				return err
			}
			if err := workspace.repo.SetPathBlock(ctx, folder, path, "AMBIGUOUS_SCAFFOLD"); err != nil {
				return err
			}
			continue
		}
		if err := syncParent(root.fd, path); err != nil {
			return err
		}
		if err := workspace.repo.CompleteScaffold(ctx, folder, path); err != nil {
			return err
		}
		if err := workspace.repo.SetPathBlock(ctx, folder, path, "AMBIGUOUS_SCAFFOLD"); err != nil {
			return err
		}
	}
	for _, publication := range publications {
		if publication.Phase == "COMMITTED" {
			_ = unix.Unlinkat(root.scratch, publication.StagePath, 0)
			if err := workspace.releaseIfNoRecovery(ctx, root, publication); err != nil {
				return err
			}
			if err := workspace.repo.RemovePublication(ctx, publication.OperationID); err != nil {
				return err
			}
			continue
		}
		envelope, err := workspace.repo.Envelope(ctx, publication.Intended)
		if err != nil {
			return err
		}
		stageExists := existsAt(root.scratch, publication.StagePath)
		if publication.Phase == "PREPARED" || publication.Phase == "STAGED" {
			if stageExists {
				if err := unix.Unlinkat(root.scratch, publication.StagePath, 0); err != nil {
					return err
				}
				if err := unix.Fsync(root.scratch); err != nil {
					return err
				}
			}
			if err := workspace.abortPublication(ctx, publication); err != nil {
				return err
			}
			continue
		}
		if publication.Phase == "REPLACEMENT_INTENT" && publication.Kind == history.KindDirectory {
			var stat unix.Stat_t
			if err := statPath(root.fd, publication.Path, &stat); errors.Is(err, syscall.ENOENT) {
				if err := workspace.abortPublication(ctx, publication); err != nil {
					return err
				}
				continue
			}
		}
		if publication.Phase == "REPLACEMENT_INTENT" && publication.Kind == history.KindTombstone && !existsAt(root.scratch, publication.RecoveryPath) {
			var stat unix.Stat_t
			if err := statPath(root.fd, publication.Path, &stat); err == nil {
				if err := workspace.abortPublication(ctx, publication); err != nil {
					return err
				}
				continue
			}
		}
		stageMatches, err := workspace.scratchMatches(root, publication.StagePath, envelope)
		if err != nil {
			return err
		}
		matches, targetExists, err := workspace.targetMatches(root, envelope)
		if err != nil {
			return err
		}
		if stageMatches && publication.Phase == "REPLACEMENT_INTENT" && matches {
			workspace.repo.SetPathBlock(ctx, folder, publication.Path, "AMBIGUOUS_PUBLICATION")
			return fmt.Errorf("%w: same bytes at stage and target for %s", ErrStructuralConflict, publication.Path)
		}
		if matches {
			if envelope.Kind == history.KindFile {
				if existsAt(root.scratch, publication.StagePath) && !existsAt(root.scratch, publication.RecoveryPath) {
					if err := unix.Renameat2(root.scratch, publication.StagePath, root.scratch, publication.RecoveryPath, unix.RENAME_NOREPLACE); err != nil {
						return err
					}
				}
			}
			lateCandidate, err := workspace.recoveryDiffersFromProjection(ctx, root, publication)
			if err != nil {
				return err
			}
			if lateCandidate {
				if err := workspace.repo.SetPathBlock(ctx, folder, publication.Path, "LATE_EDITOR_CANDIDATE"); err != nil {
					return err
				}
			}
			if err := unix.Fsync(root.scratch); err != nil {
				return err
			}
			if err := syncParent(root.fd, publication.Path); err != nil {
				return err
			}
			if publication.Phase != "FILESYSTEM_PUBLISHED" {
				if err := workspace.repo.SetPublicationPhase(ctx, publication.OperationID, "FILESYSTEM_PUBLISHED"); err != nil {
					return err
				}
			}
			if err := workspace.repo.CommitPublication(ctx, publication.OperationID); err != nil {
				return err
			}
			if err := workspace.releaseIfNoRecovery(ctx, root, publication); err != nil {
				return err
			}
			continue
		}
		if stageMatches && publication.Phase == "REPLACEMENT_INTENT" {
			if err := unix.Unlinkat(root.scratch, publication.StagePath, 0); err != nil && !errors.Is(err, syscall.ENOENT) {
				return err
			}
			if err := workspace.abortPublication(ctx, publication); err != nil {
				return err
			}
			continue
		}
		if !targetExists && envelope.Kind == history.KindTombstone {
			if err := syncParent(root.fd, publication.Path); err != nil {
				return err
			}
			if publication.Phase != "FILESYSTEM_PUBLISHED" {
				if err := workspace.repo.SetPublicationPhase(ctx, publication.OperationID, "FILESYSTEM_PUBLISHED"); err != nil {
					return err
				}
			}
			if err := workspace.repo.CommitPublication(ctx, publication.OperationID); err != nil {
				return err
			}
			if err := workspace.releaseIfNoRecovery(ctx, root, publication); err != nil {
				return err
			}
			continue
		}
		workspace.repo.SetPathBlock(ctx, folder, publication.Path, "AMBIGUOUS_PUBLICATION")
		return fmt.Errorf("%w: %s", ErrStructuralConflict, publication.Path)
	}
	return nil
}

func syncParent(rootFD int, path string) error {
	index := strings.LastIndexByte(path, '/')
	if index < 0 {
		return unix.Fsync(rootFD)
	}
	parentFD, err := safeOpen(rootFD, path[:index], unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	defer unix.Close(parentFD)
	return unix.Fsync(parentFD)
}

func existsAt(dirFD int, name string) bool {
	var stat unix.Stat_t
	return unix.Fstatat(dirFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW) == nil
}

func statPath(rootFD int, path string, stat *unix.Stat_t) error {
	if err := history.ValidatePath(path); err != nil {
		return err
	}
	index := strings.LastIndexByte(path, '/')
	if index < 0 {
		return unix.Fstatat(rootFD, path, stat, unix.AT_SYMLINK_NOFOLLOW)
	}
	parentFD, err := safeOpen(rootFD, path[:index], unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	defer unix.Close(parentFD)
	return unix.Fstatat(parentFD, path[index+1:], stat, unix.AT_SYMLINK_NOFOLLOW)
}

func (workspace *Workspace) targetMatches(root *openedRoot, envelope history.Envelope) (bool, bool, error) {
	var stat unix.Stat_t
	err := statPath(root.fd, envelope.Path, &stat)
	if errors.Is(err, syscall.ENOENT) {
		return envelope.Kind == history.KindTombstone, false, nil
	}
	if err != nil {
		return false, false, err
	}
	switch envelope.Kind {
	case history.KindDirectory:
		return stat.Mode&unix.S_IFMT == unix.S_IFDIR, true, nil
	case history.KindTombstone:
		return false, true, nil
	case history.KindFile:
		if stat.Mode&unix.S_IFMT != unix.S_IFREG {
			return false, true, nil
		}
		fd, err := safeOpen(root.fd, envelope.Path, unix.O_RDONLY|unix.O_NONBLOCK, 0)
		if err != nil {
			return false, true, err
		}
		file := os.NewFile(uintptr(fd), envelope.Path)
		matches, err := fileMatches(file, envelope.Manifest)
		file.Close()
		return matches, true, err
	default:
		return false, true, history.ErrInvalidEnvelope
	}
}

func (workspace *Workspace) scratchMatches(root *openedRoot, name string, envelope history.Envelope) (bool, error) {
	if envelope.Kind != history.KindFile {
		return false, nil
	}
	fd, err := unix.Openat(root.scratch, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if errors.Is(err, syscall.ENOENT) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	file := os.NewFile(uintptr(fd), name)
	matches, err := fileMatches(file, envelope.Manifest)
	file.Close()
	return matches, err
}

func fileMatches(file *os.File, manifest *history.Manifest) (bool, error) {
	if manifest == nil {
		return false, nil
	}
	hasher := sha256.New()
	n, err := io.Copy(hasher, file)
	if err != nil {
		return false, err
	}
	return uint64(n) == manifest.Size && bytes.Equal(hasher.Sum(nil), manifest.Digest[:]), nil
}

// InspectRecoveryCopies safely scans the workspace scratch directory for unreferenced
// or committed recovery files without unlinking them or modifying reservations/publications.
func (workspace *Workspace) InspectRecoveryCopies(ctx context.Context, folder history.ID) (int, uint64, error) {
	ctx, release := workspace.enterIO(ctx)
	defer release()
	root, err := workspace.openRoot(ctx, folder)
	if err != nil {
		return 0, 0, err
	}
	defer root.close()

	duplicate, err := unix.Dup(root.scratch)
	if err != nil {
		return 0, 0, err
	}
	dir := os.NewFile(uintptr(duplicate), scratchName)
	entries, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		return 0, 0, err
	}

	pubs, err := workspace.repo.Publications(ctx, folder)
	if err != nil {
		return 0, 0, err
	}
	// Active (non-committed) publications must not have their recovery files counted as reclaimable
	activeRecovery := make(map[string]bool)
	for _, p := range pubs {
		if p.Phase != "COMMITTED" && p.RecoveryPath != "" {
			activeRecovery[p.RecoveryPath] = true
		}
	}

	var count int
	var totalBytes uint64
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "recovery-") {
			continue
		}
		if activeRecovery[name] {
			continue
		}

		var st unix.Stat_t
		if err := unix.Fstatat(root.scratch, name, &st, 0); err == nil && st.Mode&unix.S_IFMT == unix.S_IFREG {
			totalBytes += uint64(st.Size)
		}
		count++
	}

	return count, totalBytes, nil
}

// ReclaimRecoveryCopies safely scans the workspace scratch directory for unreferenced
// or committed recovery files, releases their storage reservations, and unlinks them.
func (workspace *Workspace) ReclaimRecoveryCopies(ctx context.Context, folder history.ID) (int, uint64, error) {
	ctx, release, holdErr := workspace.enterFolder(ctx, folder)
	if holdErr != nil {
		return 0, 0, holdErr
	}
	defer release()
	root, err := workspace.openRoot(ctx, folder)
	if err != nil {
		return 0, 0, err
	}
	defer root.close()

	duplicate, err := unix.Dup(root.scratch)
	if err != nil {
		return 0, 0, err
	}
	dir := os.NewFile(uintptr(duplicate), scratchName)
	entries, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		return 0, 0, err
	}

	pubs, err := workspace.repo.Publications(ctx, folder)
	if err != nil {
		return 0, 0, err
	}
	// Active (non-committed) publications must not have their recovery files reclaimed
	activeRecovery := make(map[string]bool)
	for _, p := range pubs {
		if p.Phase != "COMMITTED" && p.RecoveryPath != "" {
			activeRecovery[p.RecoveryPath] = true
		}
	}

	var reclaimedCount int
	var reclaimedBytes uint64
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "recovery-") {
			continue
		}
		if activeRecovery[name] {
			continue
		}

		var st unix.Stat_t
		if err := unix.Fstatat(root.scratch, name, &st, 0); err == nil && st.Mode&unix.S_IFMT == unix.S_IFREG {
			reclaimedBytes += uint64(st.Size)
		}
		if err := unix.Unlinkat(root.scratch, name, 0); err != nil && !errors.Is(err, syscall.ENOENT) {
			continue
		}
		reclaimedCount++

		opID := strings.TrimPrefix(name, "recovery-")
		_ = workspace.repo.ReleaseReservation(ctx, "publication-"+opID)
		_ = workspace.repo.RemovePublication(ctx, opID)
	}

	if reclaimedCount > 0 {
		_ = unix.Fsync(root.scratch)
	}
	return reclaimedCount, reclaimedBytes, nil
}

func (workspace *Workspace) Unregister(ctx context.Context, folder history.ID) error {
	ctx, release := workspace.enterIO(ctx)
	defer release()
	return workspace.repo.UnregisterFolder(ctx, folder)
}

func (workspace *Workspace) Pause(ctx context.Context, folder history.ID, reason string) error {
	ctx, release := workspace.enterIO(ctx)
	defer release()
	return workspace.repo.PauseFolder(ctx, folder, reason)
}

func (workspace *Workspace) Resume(ctx context.Context, folder history.ID) error {
	ctx, release := workspace.enterIO(ctx)
	defer release()
	return workspace.repo.ResumeFolder(ctx, folder)
}

func (workspace *Workspace) Revalidate(ctx context.Context, folder history.ID) error {
	ctx, release := workspace.enterIO(ctx)
	defer release()
	opened, err := workspace.openRoot(ctx, folder)
	if err != nil {
		_ = workspace.repo.PauseFolder(ctx, folder, "ROOT_UNAVAILABLE")
		return err
	}
	opened.close()
	reg, err := workspace.repo.Root(ctx, folder)
	if err == nil && reg.Paused && reg.PauseReason == "ROOT_UNAVAILABLE" {
		_ = workspace.repo.ResumeFolder(ctx, folder)
	}
	return nil
}
