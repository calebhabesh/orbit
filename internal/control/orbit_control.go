package control

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/replication"
	"github.com/calebhabesh/file-sync/internal/repository"
)

// --- Product Settings Operations ---

// GetSettings retrieves product preferences from settings.json.
func (c *Controller) GetSettings(ctx context.Context) (*GetSettingsResult, error) {
	s, err := config.LoadSettings(c.db.StateDir())
	if err != nil {
		return nil, err
	}
	return &GetSettingsResult{Settings: s}, nil
}

// UpdateSettings updates product preferences in settings.json and database display names.
func (c *Controller) UpdateSettings(ctx context.Context, req UpdateSettingsRequest) (*UpdateSettingsResult, error) {
	current, err := config.LoadSettingsStrict(c.db.StateDir())
	if err != nil {
		current = config.DefaultSettings()
	}

	if req.DeviceLabel != nil {
		current.DeviceLabel = *req.DeviceLabel
		_ = c.db.SetDeviceDisplayName(ctx, c.options.LocalDevice, *req.DeviceLabel)
	}
	if req.DefaultWorkspace != nil {
		current.DefaultWorkspace = *req.DefaultWorkspace
	}
	if req.WorkspaceNames != nil {
		if current.WorkspaceNames == nil {
			current.WorkspaceNames = make(map[string]string)
		}
		for wsID, name := range req.WorkspaceNames {
			current.WorkspaceNames[wsID] = name
			var fid history.ID
			if err := fid.UnmarshalText([]byte(wsID)); err == nil {
				_ = c.db.SetFolderDisplayName(ctx, fid, name)
			}
		}
	}
	if req.Theme != nil {
		current.Theme = *req.Theme
		if *req.Theme == "dark" {
			current.DarkTheme = true
		} else if *req.Theme == "light" {
			current.DarkTheme = false
		}
	}
	if req.DarkTheme != nil {
		current.DarkTheme = *req.DarkTheme
		if *req.DarkTheme {
			current.Theme = "dark"
		} else {
			current.Theme = "light"
		}
	}

	if err := config.ValidateSettings(current); err != nil {
		return nil, &ControlError{
			Code:      "INVALID_SETTINGS",
			Message:   err.Error(),
			Retryable: false,
			Action:    "verify product settings parameters and retry",
			Err:       err,
		}
	}

	if err := config.SaveSettings(c.db.StateDir(), current); err != nil {
		return nil, fmt.Errorf("save settings: %w", err)
	}

	return &UpdateSettingsResult{
		Settings: current,
		Message:  "settings updated successfully",
	}, nil
}

// --- Peer Endpoint Operations ---

// ListPeerEndpoints retrieves configured peer endpoints.
func (c *Controller) ListPeerEndpoints(ctx context.Context) (*PeerEndpointsListResult, error) {
	peers, err := config.LoadPeerEndpoints(c.db.StateDir())
	if err != nil {
		return nil, fmt.Errorf("load peer endpoints: %w", err)
	}
	return &PeerEndpointsListResult{Peers: peers}, nil
}

// SetPeerEndpoint adds or updates a peer endpoint in peers.json.
func (c *Controller) SetPeerEndpoint(ctx context.Context, req SetPeerEndpointRequest) error {
	endpoint := config.PeerEndpoint{
		Folder:      req.Folder,
		Device:      req.Device,
		URL:         req.URL,
		Certificate: req.Certificate,
	}
	if err := config.SetPeerEndpoint(c.db.StateDir(), endpoint); err != nil {
		return &ControlError{
			Code:      "INVALID_PEER_ENDPOINT",
			Message:   err.Error(),
			Retryable: false,
			Action:    "verify folder hex, device hex, https origin, and certificate path",
			Err:       err,
		}
	}
	return nil
}

// RemovePeerEndpoint removes an endpoint from peers.json.
func (c *Controller) RemovePeerEndpoint(ctx context.Context, req RemovePeerEndpointRequest) error {
	if err := config.RemovePeerEndpoint(c.db.StateDir(), req.Folder, req.Device); err != nil {
		return fmt.Errorf("remove peer endpoint: %w", err)
	}
	return nil
}

// --- Setup Operations (O03 Foundation) ---

// InspectSetup checks state directory status, configuration, and default suggestions.
func (c *Controller) InspectSetup(ctx context.Context) (*InspectSetupResult, error) {
	stateDir := c.db.StateDir()
	cfg, err := config.Load(stateDir)
	initialized := err == nil && cfg.DeviceID != ""

	suggestedRoot := "~/Orbit"
	if home, err := os.UserHomeDir(); err == nil {
		suggestedRoot = filepath.Join(home, "Orbit")
	}

	settings, _ := config.LoadSettings(stateDir)
	registered, _ := c.db.RegisteredFolders(ctx)

	setupState, _ := c.db.GetSetupState(ctx)
	currentPhase := "initial"
	setupCompleted := false
	if setupState != nil {
		currentPhase = setupState.Phase
		setupCompleted = setupState.Completed
	}
	if len(registered) > 0 {
		setupCompleted = true
	}

	deviceID := ""
	if initialized {
		deviceID = cfg.DeviceID
	}

	return &InspectSetupResult{
		Initialized:     initialized,
		DeviceID:        deviceID,
		SuggestedRoot:   suggestedRoot,
		CurrentPhase:    currentPhase,
		SetupCompleted:  setupCompleted,
		Settings:        settings,
		RegisteredCount: len(registered),
	}, nil
}

// PreviewCreateRoot validates a proposed root directory.
func (c *Controller) PreviewCreateRoot(ctx context.Context, req PreviewCreateRootRequest) (*PreviewCreateRootResult, error) {
	if req.Path == "" {
		return nil, &ControlError{
			Code:      "INVALID_PATH",
			Message:   "root path cannot be empty",
			Retryable: false,
			Action:    "provide an absolute directory path",
		}
	}

	absPath, err := filepath.Abs(req.Path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	stateDirAbs, _ := filepath.Abs(c.db.StateDir())
	if absPath == stateDirAbs || strings.HasPrefix(absPath, stateDirAbs+string(filepath.Separator)) {
		return &PreviewCreateRootResult{
			Path:       absPath,
			Disallowed: true,
			Reason:     "root cannot be inside the private state directory",
		}, nil
	}

	// Check root directory itself
	if absPath == "/" || absPath == "/root" || absPath == "/etc" || absPath == "/var" || absPath == "/usr" || absPath == "/bin" || absPath == "/lib" || absPath == "/sys" || absPath == "/proc" || absPath == "/dev" || absPath == "/boot" || absPath == "/run" {
		return &PreviewCreateRootResult{
			Path:       absPath,
			Disallowed: true,
			Reason:     "system root directories cannot be used as sync folders",
		}, nil
	}

	// Check overlapping roots with registered folders
	registered, err := c.db.RegisteredFolders(ctx)
	if err == nil {
		for _, reg := range registered {
			regAbs, _ := filepath.Abs(reg.Path)
			if absPath == regAbs {
				return &PreviewCreateRootResult{
					Path:       absPath,
					Disallowed: true,
					Reason:     "directory is already registered as a sync folder",
				}, nil
			}
			if strings.HasPrefix(absPath, regAbs+string(filepath.Separator)) {
				return &PreviewCreateRootResult{
					Path:       absPath,
					Disallowed: true,
					Reason:     "directory is inside an already registered sync folder",
				}, nil
			}
			if strings.HasPrefix(regAbs, absPath+string(filepath.Separator)) {
				return &PreviewCreateRootResult{
					Path:       absPath,
					Disallowed: true,
					Reason:     "directory contains an already registered sync folder",
				}, nil
			}
		}
	}

	info, err := os.Stat(absPath)
	res := &PreviewCreateRootResult{
		Path: absPath,
	}

	if errors.Is(err, os.ErrNotExist) {
		res.Exists = false
		res.IsEmpty = true
		// Test parent directory writability
		parent := filepath.Dir(absPath)
		if pInfo, pErr := os.Stat(parent); pErr == nil && pInfo.IsDir() {
			testFile := filepath.Join(parent, fmt.Sprintf(".test-write-%d.tmp", time.Now().UnixNano()))
			if err := os.WriteFile(testFile, []byte("ok"), 0o600); err == nil {
				res.Writable = true
				_ = os.Remove(testFile)
			}
		}
		return res, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat path: %w", err)
	}

	if !info.IsDir() {
		res.Disallowed = true
		res.Reason = "path exists but is not a directory"
		return res, nil
	}

	res.Exists = true

	// Count preexisting entries
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	res.PreexistingRows = len(entries)
	res.IsEmpty = len(entries) == 0
	for i, e := range entries {
		if i >= 10 {
			break
		}
		res.ExistingSamples = append(res.ExistingSamples, e.Name())
	}

	// Test directory writability
	testFile := filepath.Join(absPath, fmt.Sprintf(".test-write-%d.tmp", time.Now().UnixNano()))
	if err := os.WriteFile(testFile, []byte("ok"), 0o600); err == nil {
		res.Writable = true
		_ = os.Remove(testFile)
	}

	return res, nil
}

// PreviewJoinRoot validates a proposed root directory for joining an existing folder.
func (c *Controller) PreviewJoinRoot(ctx context.Context, req PreviewJoinRootRequest) (*PreviewJoinRootResult, error) {
	if req.FolderID == ([32]byte{}) {
		return nil, &ControlError{
			Code:      "INVALID_FOLDER",
			Message:   "folder ID cannot be empty",
			Retryable: false,
			Action:    "provide a valid 32-byte folder ID",
		}
	}
	res, err := c.PreviewCreateRoot(ctx, PreviewCreateRootRequest{Path: req.Path})
	if err != nil {
		return nil, err
	}
	return &PreviewJoinRootResult{
		Path:            res.Path,
		FolderID:        req.FolderID,
		Exists:          res.Exists,
		IsEmpty:         res.IsEmpty,
		PreexistingRows: res.PreexistingRows,
		Writable:        res.Writable,
		Disallowed:      res.Disallowed,
		Reason:          res.Reason,
		ExistingSamples: res.ExistingSamples,
	}, nil
}

// StartSetup initializes workspace creation or registration during onboarding.
func (c *Controller) StartSetup(ctx context.Context, req StartSetupRequest) (*StartSetupResult, error) {
	preview, err := c.PreviewCreateRoot(ctx, PreviewCreateRootRequest{Path: req.RootPath})
	if err != nil {
		return nil, err
	}
	if preview.Disallowed {
		return nil, &ControlError{
			Code:      "DISALLOWED_ROOT",
			Message:   preview.Reason,
			Retryable: false,
			Action:    "select an accessible personal directory",
		}
	}

	if err := os.MkdirAll(preview.Path, 0o755); err != nil {
		return nil, fmt.Errorf("create root directory: %w", err)
	}

	// Generate deterministic or random folder ID
	var folderID history.ID
	if _, err := rand.Read(folderID[:]); err != nil {
		return nil, err
	}

	cfg, _ := config.Load(c.db.StateDir())
	deviceID := cfg.DeviceID

	opID := "setup-" + hex.EncodeToString(folderID[:4])
	_ = c.db.RecordOperationProgress(ctx, repository.OperationProgressRecord{
		OperationID:         opID,
		Kind:                "setup",
		Phase:               "registering_folder",
		ProgressNumerator:   25,
		ProgressDenominator: 100,
		Details:             "Registering root folder",
	})

	// 1. Register folder in repository & workspace
	if _, err := c.RegisterFolder(ctx, folderID, preview.Path); err != nil {
		_ = c.db.RecordOperationProgress(ctx, repository.OperationProgressRecord{
			OperationID: opID,
			Kind:        "setup",
			Phase:       "failed",
			Details:     "Failed to register folder: " + err.Error(),
			Retryable:   true,
		})
		return nil, fmt.Errorf("register folder: %w", err)
	}

	// Initialize Revision 1 membership for local author
	if c.options.LocalDevice != (history.ID{}) {
		ident, idErr := replication.LoadOrCreateIdentity(c.db.StateDir(), c.options.LocalDevice, c.options.Now())
		if idErr == nil {
			_, _ = c.db.ApproveMembership(ctx, protocol.Membership{
				Folder:      folderID,
				Revision:    1,
				PriorDigest: history.Digest{},
				Active: []protocol.ActiveMember{
					{Device: c.options.LocalDevice, KeyPin: ident.KeyPin},
				},
			})
		}
	}

	// 2. Set display name if provided
	if req.WorkspaceName != "" {
		_ = c.db.SetFolderDisplayName(ctx, folderID, req.WorkspaceName)
	}

	// 3. Record settings
	folderHex := hex.EncodeToString(folderID[:])
	_, _ = c.UpdateSettings(ctx, UpdateSettingsRequest{
		DeviceLabel:      &req.DeviceLabel,
		DefaultWorkspace: ptr(folderHex),
		WorkspaceNames:   map[string]string{folderHex: req.WorkspaceName},
	})

	_ = c.db.RecordOperationProgress(ctx, repository.OperationProgressRecord{
		OperationID:         opID,
		Kind:                "setup",
		Phase:               "scanning_initial_content",
		ProgressNumerator:   60,
		ProgressDenominator: 100,
		Details:             "Capturing initial directory contents",
	})

	// 4. Initial capture scan (Invariant I22: preexisting contents survive and are captured)
	if _, err := c.ws.Scan(ctx, folderID); err != nil {
		_ = c.db.RecordOperationProgress(ctx, repository.OperationProgressRecord{
			OperationID: opID,
			Kind:        "setup",
			Phase:       "failed",
			Details:     "Initial scan failed: " + err.Error(),
			Retryable:   true,
		})
		return nil, fmt.Errorf("initial scan failed: %w", err)
	}

	// 5. Update setup state in DB
	_ = c.db.SaveSetupState(ctx, repository.SetupStateRecord{
		Phase:           "completed",
		RootPath:        preview.Path,
		DefaultFolderID: &folderID,
		Completed:       true,
		UpdatedNS:       time.Now().UnixNano(),
	})

	_ = c.db.RecordOperationProgress(ctx, repository.OperationProgressRecord{
		OperationID:         opID,
		Kind:                "setup",
		Phase:               "completed",
		ProgressNumerator:   100,
		ProgressDenominator: 100,
		Details:             "Setup completed successfully",
		Retryable:           false,
		Canceled:            false,
	})

	return &StartSetupResult{
		OperationID: opID,
		DeviceID:    deviceID,
		FolderID:    folderID,
		RootPath:    preview.Path,
		Phase:       "completed",
		Message:     "workspace setup completed successfully",
	}, nil
}

// ResumeSetup resumes an in-progress or interrupted workspace setup.
func (c *Controller) ResumeSetup(ctx context.Context, req ResumeSetupRequest) (*ResumeSetupResult, error) {
	st, err := c.db.GetSetupState(ctx)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, &ControlError{
			Code:      "NO_SETUP_IN_PROGRESS",
			Message:   "no setup state record found to resume",
			Retryable: false,
			Action:    "run start-setup to begin onboarding",
		}
	}
	cfg, _ := config.Load(c.db.StateDir())
	deviceID := cfg.DeviceID

	if st.Completed {
		var fID history.ID
		if st.DefaultFolderID != nil {
			fID = *st.DefaultFolderID
		}
		return &ResumeSetupResult{
			OperationID: "setup-resume",
			DeviceID:    deviceID,
			FolderID:    fID,
			RootPath:    st.RootPath,
			Phase:       "completed",
			Completed:   true,
			Message:     "setup already completed",
		}, nil
	}

	// If rootPath is set, ensure folder registration and initial scan are complete
	if st.RootPath != "" && st.DefaultFolderID != nil {
		folderID := *st.DefaultFolderID
		registered, _ := c.db.RegisteredFolders(ctx)
		isReg := false
		for _, reg := range registered {
			if reg.Folder == folderID {
				isReg = true
				break
			}
		}
		if !isReg {
			if _, err := c.RegisterFolder(ctx, folderID, st.RootPath); err != nil {
				return nil, fmt.Errorf("resume: register folder: %w", err)
			}
		}
		if _, err := c.ws.Scan(ctx, folderID); err != nil {
			return nil, fmt.Errorf("resume: scan folder: %w", err)
		}
		st.Phase = "completed"
		st.Completed = true
		st.UpdatedNS = time.Now().UnixNano()
		_ = c.db.SaveSetupState(ctx, *st)

		return &ResumeSetupResult{
			OperationID: "setup-resume",
			DeviceID:    deviceID,
			FolderID:    folderID,
			RootPath:    st.RootPath,
			Phase:       "completed",
			Completed:   true,
			Message:     "setup resumed and completed successfully",
		}, nil
	}

	return nil, &ControlError{
		Code:      "SETUP_INCOMPLETE",
		Message:   "setup state lacks root path or folder ID",
		Retryable: false,
		Action:    "run start-setup to configure workspace",
	}
}

// GetSetupStatus returns current setup phase and progress.
func (c *Controller) GetSetupStatus(ctx context.Context) (*SetupStatusResult, error) {
	st, err := c.db.GetSetupState(ctx)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return &SetupStatusResult{
			Phase:     "uninitialized",
			Completed: false,
		}, nil
	}
	folderIDStr := ""
	if st.DefaultFolderID != nil {
		folderIDStr = hex.EncodeToString(st.DefaultFolderID[:])
	}
	return &SetupStatusResult{
		Phase:     st.Phase,
		Completed: st.Completed,
		RootPath:  st.RootPath,
		FolderID:  folderIDStr,
	}, nil
}

// BrowseDirectories provides an authenticated, paginated directory picker.
func (c *Controller) BrowseDirectories(ctx context.Context, req DirectoryPickerRequest) (*DirectoryPickerResult, error) {
	targetPath := req.Path
	if targetPath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			targetPath = home
		} else {
			targetPath = "/"
		}
	} else if strings.HasPrefix(targetPath, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			targetPath = filepath.Join(home, strings.TrimPrefix(targetPath, "~"))
		}
	}

	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		return nil, fmt.Errorf("resolve directory path: %w", err)
	}
	absPath = filepath.Clean(absPath)

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, &ControlError{
			Code:      "PATH_NOT_FOUND",
			Message:   fmt.Sprintf("directory %q does not exist: %v", absPath, err),
			Retryable: false,
			Action:    "select an existing directory",
		}
	}
	if !info.IsDir() {
		return nil, &ControlError{
			Code:      "NOT_A_DIRECTORY",
			Message:   fmt.Sprintf("path %q is a file, not a directory", absPath),
			Retryable: false,
			Action:    "select a directory",
		}
	}

	writable := false
	testFile := filepath.Join(absPath, fmt.Sprintf(".picker-test-%d.tmp", time.Now().UnixNano()))
	if err := os.WriteFile(testFile, []byte("ok"), 0o600); err == nil {
		writable = true
		_ = os.Remove(testFile)
	}

	parentPath := filepath.Dir(absPath)
	if parentPath == absPath {
		parentPath = ""
	}

	dirEntries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, &ControlError{
			Code:      "PERMISSION_DENIED",
			Message:   fmt.Sprintf("cannot read directory %q: %v", absPath, err),
			Retryable: false,
			Action:    "choose an accessible directory",
		}
	}

	stateDirAbs, _ := filepath.Abs(c.db.StateDir())

	var subdirs []DirectoryEntry
	for _, entry := range dirEntries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !entry.IsDir() {
			continue
		}
		childPath := filepath.Join(absPath, name)
		_, err := entry.Info()
		accessible := err == nil
		deniedReason := ""

		if childPath == stateDirAbs || strings.HasPrefix(childPath, stateDirAbs+string(filepath.Separator)) {
			accessible = false
			deniedReason = "private state directory"
		} else if childPath == "/proc" || childPath == "/sys" || childPath == "/dev" {
			accessible = false
			deniedReason = "system virtual filesystem"
		} else if !accessible {
			deniedReason = "permission denied"
		}

		subdirs = append(subdirs, DirectoryEntry{
			Name:         name,
			Path:         childPath,
			IsDir:        true,
			Accessible:   accessible,
			DeniedReason: deniedReason,
		})
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	} else if limit > 200 {
		limit = 200
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	total := len(subdirs)
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	paged := subdirs[start:end]
	hasMore := end < total

	return &DirectoryPickerResult{
		CurrentPath:  absPath,
		ParentPath:   parentPath,
		Writable:     writable,
		Entries:      paged,
		TotalEntries: total,
		Offset:       offset,
		Limit:        limit,
		HasMore:      hasMore,
	}, nil
}

// OpenLocalFolder safely dispatches a desktop file manager helper for a registered workspace folder.
func (c *Controller) OpenLocalFolder(ctx context.Context, req OpenFolderRequest) (*OpenFolderResult, error) {
	registered, err := c.db.RegisteredFolders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list registered folders: %w", err)
	}

	var targetPath string
	if req.Folder != "" {
		for _, reg := range registered {
			if hex.EncodeToString(reg.Folder[:]) == req.Folder {
				targetPath = reg.Path
				break
			}
		}
		if targetPath == "" {
			return nil, &ControlError{
				Code:      "FOLDER_NOT_FOUND",
				Message:   fmt.Sprintf("folder %s is not registered", req.Folder),
				Retryable: false,
				Action:    "select an active registered folder",
			}
		}
	} else if req.Path != "" {
		cleanReq, err := filepath.Abs(req.Path)
		if err != nil {
			return nil, fmt.Errorf("resolve path: %w", err)
		}
		allowed := false
		for _, reg := range registered {
			regAbs, _ := filepath.Abs(reg.Path)
			if cleanReq == regAbs || strings.HasPrefix(cleanReq, regAbs+string(filepath.Separator)) {
				allowed = true
				targetPath = cleanReq
				break
			}
		}
		if !allowed {
			return nil, &ControlError{
				Code:      "UNAUTHORIZED_PATH",
				Message:   "path is not within any registered workspace root",
				Retryable: false,
				Action:    "open paths only within registered sync folders",
			}
		}
	} else {
		if len(registered) > 0 {
			targetPath = registered[0].Path
		} else {
			return nil, &ControlError{
				Code:      "NO_FOLDERS_REGISTERED",
				Message:   "no folders are registered to open",
				Retryable: false,
				Action:    "complete setup and register a sync folder",
			}
		}
	}

	info, err := os.Stat(targetPath)
	if err != nil || !info.IsDir() {
		return nil, &ControlError{
			Code:      "DIRECTORY_NOT_FOUND",
			Message:   fmt.Sprintf("directory %q does not exist: %v", targetPath, err),
			Retryable: false,
			Action:    "verify folder location exists on disk",
		}
	}

	hasDisplay := os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
	opener, err := exec.LookPath("xdg-open")
	if !hasDisplay || err != nil {
		return nil, &ControlError{
			Code:      "DESKTOP_HELPER_UNAVAILABLE",
			Message:   "desktop file manager helper is unavailable in headless or non-GUI session",
			Retryable: false,
			Action:    fmt.Sprintf("open folder manually on host: %s", targetPath),
		}
	}

	cmd := exec.CommandContext(ctx, opener, targetPath)
	if err := cmd.Start(); err != nil {
		return nil, &ControlError{
			Code:      "DESKTOP_HELPER_FAILED",
			Message:   fmt.Sprintf("failed to launch desktop file manager: %v", err),
			Retryable: false,
			Action:    fmt.Sprintf("open folder manually: %s", targetPath),
		}
	}

	return &OpenFolderResult{
		Status:  "opened",
		Path:    targetPath,
		Message: "opened folder in desktop file manager",
	}, nil
}

// --- Invitation Operations (G02/O05) ---

// CreateInvitation generates an expiring single-use invitation capability for a workspace.
func (c *Controller) CreateInvitation(ctx context.Context, req CreateInvitationRequest) (*CreateInvitationResult, error) {
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return nil, fmt.Errorf("generate invitation secret: %w", err)
	}
	tokenStr := hex.EncodeToString(rawToken)
	digest := sha256.Sum256([]byte(tokenStr))

	ttl := 24 * time.Hour
	if req.TTLSecs > 0 {
		ttl = time.Duration(req.TTLSecs) * time.Second
	}
	maxUses := 1
	if req.MaxUses > 0 {
		maxUses = req.MaxUses
	}

	now := time.Now()
	expiresAt := now.Add(ttl)

	inv := repository.InvitationRecord{
		Digest:    digest,
		Folder:    req.Folder,
		CreatedNS: now.UnixNano(),
		ExpiresNS: expiresAt.UnixNano(),
		MaxUses:   maxUses,
		UsesCount: 0,
		Revoked:   false,
	}
	if err := c.db.CreateInvitation(ctx, inv); err != nil {
		return nil, fmt.Errorf("create invitation: %w", err)
	}

	endpoint := req.Endpoint
	if endpoint == "" {
		if addrBytes, err := os.ReadFile(filepath.Join(c.db.StateDir(), "control.addr")); err == nil {
			endpoint = strings.TrimSpace(string(addrBytes))
		}
	}

	var invCode string
	if endpoint != "" {
		invCode = fmt.Sprintf("orbit-invitation:v1?token=%s&folder=%x&endpoint=%s", tokenStr, req.Folder[:], url.QueryEscape(endpoint))
	} else {
		invCode = fmt.Sprintf("orbit-invitation:v1?token=%s&folder=%x", tokenStr, req.Folder[:])
	}

	return &CreateInvitationResult{
		Token:          tokenStr,
		Digest:         digest,
		Folder:         req.Folder,
		ExpiresAt:      expiresAt.UTC().Format(time.RFC3339),
		MaxUses:        maxUses,
		InvitationCode: invCode,
	}, nil
}

// RevokeInvitation invalidates an invitation token digest.
func (c *Controller) RevokeInvitation(ctx context.Context, req RevokeInvitationRequest) (*RevokeInvitationResult, error) {
	if err := c.db.RevokeInvitation(ctx, req.Digest); err != nil {
		return nil, err
	}
	return &RevokeInvitationResult{
		Digest:  req.Digest,
		Revoked: true,
	}, nil
}

// ListInvitations lists invitations for a workspace.
func (c *Controller) ListInvitations(ctx context.Context, folder history.ID) (*ListInvitationsResult, error) {
	invs, err := c.db.ListInvitations(ctx, folder)
	if err != nil {
		return nil, err
	}
	return &ListInvitationsResult{Invitations: invs}, nil
}

// --- Enrollment Operations (G02/O05) ---

// SubmitEnrollmentRequest handles an incoming join request proving key possession.
func (c *Controller) SubmitEnrollmentRequest(ctx context.Context, req SubmitJoinRequestPayload) (*SubmitJoinRequestResult, error) {
	if req.Token == "" {
		return nil, &ControlError{Code: "UNAUTHORIZED", Message: "invitation token is required", Action: "provide an active invitation token"}
	}
	if req.TargetFolder == (history.ID{}) {
		return nil, &ControlError{Code: "INVALID_REQUEST", Message: "target folder is required", Action: "specify target workspace folder ID"}
	}

	pubBytes, err := hex.DecodeString(req.PublicKey)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return nil, &ControlError{Code: "INVALID_REQUEST", Message: "public key must be 32-byte hex", Action: "provide a 32-byte Ed25519 public key"}
	}
	var pubKey [32]byte
	copy(pubKey[:], pubBytes)
	keyPin := sha256.Sum256(pubBytes)

	joiningDevice := req.JoiningDevice
	if joiningDevice == (history.ID{}) {
		joiningDevice = sha256.Sum256(pubBytes)
	}

	// Invariant I24: Retired devices cannot rejoin under the same identity
	retired, err := c.db.IsDeviceRetired(ctx, req.TargetFolder, joiningDevice)
	if err == nil && retired {
		return nil, RetiredMemberRevivalError("device was previously retired and cannot rejoin workspace (Invariant I24)")
	}

	// Verify cryptographic proof of possession of the joining private key
	sigBytes, err := hex.DecodeString(req.Signature)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return nil, InvalidSignatureError("signature must be 64-byte hex encoded Ed25519 signature")
	}

	var chalBytes []byte
	if b, err := hex.DecodeString(req.Challenge); err == nil && len(b) > 0 {
		chalBytes = b
	} else {
		chalBytes = []byte(req.Challenge)
	}
	if len(chalBytes) == 0 {
		return nil, InvalidSignatureError("challenge nonce or payload is required")
	}

	if !ed25519.Verify(pubBytes, chalBytes, sigBytes) {
		return nil, InvalidSignatureError("joining signature verification failed against presented public key")
	}

	tokenDigest := sha256.Sum256([]byte(req.Token))
	if err := c.db.ConsumeInvitation(ctx, tokenDigest, time.Now()); err != nil {
		if errors.Is(err, repository.ErrInvitationNotFound) {
			return nil, &ControlError{Code: "UNAUTHORIZED", Message: "invitation token not found or invalid", Action: "check invitation token"}
		}
		if errors.Is(err, repository.ErrInvitationRevoked) {
			return nil, &ControlError{Code: "UNAUTHORIZED", Message: "invitation token has been revoked", Action: "request a fresh invitation"}
		}
		if errors.Is(err, repository.ErrInvitationExpired) {
			return nil, &ControlError{Code: "UNAUTHORIZED", Message: "invitation token has expired or uses are exhausted", Action: "request a fresh invitation"}
		}
		return nil, err
	}

	now := time.Now().UnixNano()
	requestID := fmt.Sprintf("req-%x", pubKey[:8])
	enrReq := repository.EnrollmentRequestRecord{
		RequestID:      requestID,
		Folder:         req.TargetFolder,
		DeviceID:       joiningDevice,
		PublicKey:      pubKey,
		KeyPin:         keyPin,
		SuggestedLabel: req.SuggestedLabel,
		Status:         "pending",
		CreatedNS:      now,
		UpdatedNS:      now,
	}

	if err := c.db.RecordEnrollmentRequest(ctx, enrReq); err != nil {
		return nil, fmt.Errorf("record enrollment request: %w", err)
	}

	return &SubmitJoinRequestResult{
		RequestID: requestID,
		Status:    "pending",
		Message:   "join request submitted; pending explicit owner approval",
	}, nil
}

// ListEnrollmentRequests queries requests for a workspace.
func (c *Controller) ListEnrollmentRequests(ctx context.Context, folder history.ID, status string) (*ListEnrollmentRequestsResult, error) {
	reqs, err := c.db.ListEnrollmentRequests(ctx, folder, status)
	if err != nil {
		return nil, err
	}
	return &ListEnrollmentRequestsResult{Requests: reqs}, nil
}

// ApproveEnrollmentRequest marks an enrollment request as approved by the owner, minting Revision N+1.
func (c *Controller) ApproveEnrollmentRequest(ctx context.Context, req ApproveEnrollmentRequest) (*ApproveEnrollmentResult, error) {
	enrReq, err := c.db.GetEnrollmentRequest(ctx, req.RequestID)
	if err != nil {
		return nil, &ControlError{Code: "REQUEST_NOT_FOUND", Message: "enrollment request not found: " + err.Error(), Action: "check request ID"}
	}
	if req.Folder != (history.ID{}) && enrReq.Folder != req.Folder {
		return nil, &ControlError{Code: "FOLDER_MISMATCH", Message: "enrollment request does not match target folder", Action: "provide matching workspace folder ID"}
	}

	// Invariant I24: Retired devices cannot rejoin under the same identity
	retired, err := c.db.IsDeviceRetired(ctx, enrReq.Folder, enrReq.DeviceID)
	if err == nil && retired {
		return nil, RetiredMemberRevivalError("device was previously retired and cannot rejoin workspace (Invariant I24)")
	}

	cur, curApp, err := c.db.GetMembership(ctx, enrReq.Folder)
	var nextMembership protocol.Membership
	if err != nil {
		folders, fErr := c.db.Folders(ctx)
		if fErr != nil {
			return nil, err
		}
		var foundFolder *repository.FolderRecord
		for _, f := range folders {
			if f.Folder == enrReq.Folder {
				foundFolder = &f
				break
			}
		}
		if foundFolder == nil {
			return nil, &ControlError{Code: "FOLDER_NOT_FOUND", Message: "workspace folder not found", Action: "verify folder exists"}
		}
		author := foundFolder.LocalAuthor
		if author == (history.ID{}) {
			author = c.options.LocalDevice
		}
		authorPin, _ := c.db.DeviceKeyPin(ctx, author)
		if authorPin == (history.Digest{}) {
			if ident, idErr := replication.LoadOrCreateIdentity(c.db.StateDir(), author, c.options.Now()); idErr == nil {
				authorPin = ident.KeyPin
			} else {
				authorPin = sha256.Sum256(author[:])
			}
		}
		active := []protocol.ActiveMember{
			{Device: author, KeyPin: authorPin},
		}
		if enrReq.DeviceID != author {
			active = append(active, protocol.ActiveMember{
				Device: enrReq.DeviceID,
				KeyPin: enrReq.KeyPin,
			})
		}
		nextMembership = protocol.Membership{
			Folder:      enrReq.Folder,
			Revision:    1,
			PriorDigest: history.Digest{},
			Active:      active,
		}
	} else {
		// Idempotency: check if member is already active in current membership
		for _, m := range cur.Active {
			if m.Device == enrReq.DeviceID {
				_ = c.db.UpdateEnrollmentRequestStatus(ctx, req.RequestID, "approved")
				return &ApproveEnrollmentResult{
					RequestID: req.RequestID,
					Status:    "approved",
					Message:   "device enrollment already approved",
					Revision:  cur.Revision,
					Digest:    curApp.Digest,
					Replay:    true,
				}, nil
			}
		}

		nextRev := cur.Revision + 1
		nextActive := append(append([]protocol.ActiveMember(nil), cur.Active...), protocol.ActiveMember{
			Device: enrReq.DeviceID,
			KeyPin: enrReq.KeyPin,
		})

		nextMembership = protocol.Membership{
			Folder:      enrReq.Folder,
			Revision:    nextRev,
			PriorDigest: curApp.Digest,
			Active:      nextActive,
			Retired:     cur.Retired,
		}
	}

	approved, err := c.db.ApproveMembership(ctx, nextMembership)
	if err != nil {
		return nil, err
	}

	label := enrReq.SuggestedLabel
	if req.SuggestedLabel != "" {
		label = req.SuggestedLabel
	}
	if label != "" {
		_ = c.db.SetDeviceDisplayName(ctx, enrReq.DeviceID, label)
	}

	endpointURL := req.Endpoint
	if endpointURL != "" {
		certPath := req.Certificate
		if certPath == "" {
			certPath = filepath.Join(c.db.StateDir(), "identity.crt")
		}
		_ = config.SetPeerEndpoint(c.db.StateDir(), config.PeerEndpoint{
			Folder:      hex.EncodeToString(enrReq.Folder[:]),
			Device:      hex.EncodeToString(enrReq.DeviceID[:]),
			URL:         endpointURL,
			Certificate: certPath,
		})
	}

	if err := c.db.UpdateEnrollmentRequestStatus(ctx, req.RequestID, "approved"); err != nil {
		return nil, err
	}

	return &ApproveEnrollmentResult{
		RequestID: req.RequestID,
		Status:    "approved",
		Message:   "device enrollment request approved; membership updated",
		Revision:  approved.Revision,
		Digest:    approved.Digest,
		Replay:    false,
	}, nil
}

// DeclineEnrollmentRequest marks an enrollment request as declined.
func (c *Controller) DeclineEnrollmentRequest(ctx context.Context, req DeclineEnrollmentRequest) error {
	return c.db.UpdateEnrollmentRequestStatus(ctx, req.RequestID, "declined")
}

// DetectMembershipFork checks if candidate revision conflicts with local membership revisions.
func (c *Controller) DetectMembershipFork(ctx context.Context, req DetectForkRequest) (*DetectForkResult, error) {
	err := c.db.DetectMembershipFork(ctx, req.Folder, req.Candidate)
	if err != nil {
		if errors.Is(err, repository.ErrMembershipFork) {
			return &DetectForkResult{
				Folder:  req.Folder,
				Forked:  true,
				Message: err.Error(),
			}, nil
		}
		return nil, err
	}
	return &DetectForkResult{
		Folder: req.Folder,
		Forked: false,
	}, nil
}

// ReconcileMembership creates an explicit owner reconciling revision (Revision N+1)
// unifying active members across previously forked or partitioned branches.
func (c *Controller) ReconcileMembership(ctx context.Context, req ReconcileMembershipRequest) (*ReconcileMembershipResult, error) {
	cur, curApp, err := c.db.GetMembership(ctx, req.Folder)
	if err != nil {
		return nil, err
	}

	activeMap := make(map[history.ID]history.Digest)
	for _, m := range cur.Active {
		activeMap[m.Device] = m.KeyPin
	}

	for _, dev := range req.Active {
		if _, exists := activeMap[dev]; !exists {
			pin, err := c.db.DeviceKeyPin(ctx, dev)
			if err == nil && pin != (history.Digest{}) {
				activeMap[dev] = pin
			}
		}
	}

	var newActive []protocol.ActiveMember
	for dev, pin := range activeMap {
		retired, err := c.db.IsDeviceRetired(ctx, req.Folder, dev)
		if err == nil && retired {
			continue
		}
		newActive = append(newActive, protocol.ActiveMember{
			Device: dev,
			KeyPin: pin,
		})
	}
	sort.Slice(newActive, func(i, j int) bool {
		return bytes.Compare(newActive[i].Device[:], newActive[j].Device[:]) < 0
	})

	nextMembership := protocol.Membership{
		Folder:      req.Folder,
		Revision:    cur.Revision + 1,
		PriorDigest: curApp.Digest,
		Active:      newActive,
		Retired:     cur.Retired,
	}

	approved, err := c.db.ApproveMembership(ctx, nextMembership)
	if err != nil {
		return nil, err
	}

	return &ReconcileMembershipResult{
		Folder:           req.Folder,
		ApprovedRevision: approved.Revision,
		ApprovedDigest:   approved.Digest,
		Membership:       nextMembership,
	}, nil
}

// CatchUpMembership validates and durably installs a newer sequential membership revision received from a peer.
func (c *Controller) CatchUpMembership(ctx context.Context, folder history.ID, newer protocol.Membership, snapshots ...protocol.RetirementSnapshot) (*repository.ApprovedMembership, error) {
	if newer.Folder != folder {
		return nil, errors.New("folder mismatch in catch-up membership")
	}

	if err := c.db.DetectMembershipFork(ctx, folder, newer); err != nil {
		return nil, err
	}

	approved, err := c.db.ApproveMembership(ctx, newer, snapshots...)
	if err != nil {
		return nil, err
	}
	return &approved, nil
}

// --- Read Lease Operations (G03/O07 - Invariant I25) ---

// AcquireReadLease acquires a short-lived lease protecting CAS chunks against GC unlinking.
func (c *Controller) AcquireReadLease(ctx context.Context, req AcquireReadLeaseRequest) (*AcquireReadLeaseResult, error) {
	if err := c.readableWorkspace(ctx, req.Folder); err != nil {
		return nil, err
	}
	manifest, _, err := c.db.GetVersionManifest(ctx, history.VersionID{Folder: req.Folder, Author: req.VersionAuthor, Counter: req.VersionCounter})
	if err != nil {
		return nil, readError(err)
	}
	ready, err := c.db.ContentReady(ctx, history.VersionID{Folder: req.Folder, Author: req.VersionAuthor, Counter: req.VersionCounter})
	if err != nil {
		return nil, readError(err)
	}
	if !ready {
		return nil, readError(repository.ErrNotReady)
	}

	digests := make([]history.Digest, 0, len(manifest.Chunks))
	for _, chunk := range manifest.Chunks {
		digests = append(digests, chunk.Digest)
	}
	if len(req.ChunkDigests) > 0 {
		if len(req.ChunkDigests) != len(digests) {
			return nil, readError(fmt.Errorf("lease chunks do not match requested version"))
		}
		for i := range digests {
			if req.ChunkDigests[i] != digests[i] {
				return nil, readError(fmt.Errorf("lease chunks do not match requested version"))
			}
		}
	}
	req.ChunkDigests = digests
	ttl := 5 * time.Minute
	if req.TTLSeconds < 0 || req.TTLSeconds > 300 {
		return nil, readError(fmt.Errorf("lease TTL must be at most 300 seconds"))
	}
	if req.TTLSeconds > 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}

	now := time.Now()
	expiresAt := now.Add(ttl)

	rawID := make([]byte, 16)
	_, _ = rand.Read(rawID)
	leaseID := "lease-" + hex.EncodeToString(rawID)

	lease := repository.ReadLeaseRecord{
		LeaseID:        leaseID,
		Folder:         req.Folder,
		VersionAuthor:  req.VersionAuthor,
		VersionCounter: req.VersionCounter,
		ExpiresNS:      expiresAt.UnixNano(),
		CreatedNS:      now.UnixNano(),
		ChunkDigests:   req.ChunkDigests,
	}

	if err := c.db.AcquireReadLease(ctx, lease); err != nil {
		return nil, fmt.Errorf("acquire read lease: %w", err)
	}

	totalBytes := manifest.Size
	return &AcquireReadLeaseResult{
		LeaseID:       leaseID,
		ExpiresAt:     expiresAt.UTC().Format(time.RFC3339),
		ChunkCount:    len(req.ChunkDigests),
		TotalBytes:    totalBytes,
		TotalBytesStr: DecimalString(totalBytes),
	}, nil
}

// ReleaseReadLease explicitly frees a read lease and unpins chunks.
func (c *Controller) ReleaseReadLease(ctx context.Context, req ReleaseReadLeaseRequest) error {
	return c.db.ReleaseReadLease(ctx, req.LeaseID)
}

// --- Operation Progress & Cancellation Operations (O02/O09/O10) ---

// GetOperationProgress retrieves progress for a durable operation.
func (c *Controller) GetOperationProgress(ctx context.Context, opID string) (*OperationProgressResult, error) {
	prog, err := c.db.GetOperationProgress(ctx, opID)
	if err != nil {
		return nil, err
	}
	return &OperationProgressResult{Progress: *prog}, nil
}

// CancelOperation requests cooperative cancellation of an in-flight operation.
func (c *Controller) CancelOperation(ctx context.Context, req CancelOperationRequest) (*CancelOperationResult, error) {
	if err := c.db.CancelOperationProgress(ctx, req.OperationID); err != nil {
		return nil, err
	}
	return &CancelOperationResult{
		OperationID: req.OperationID,
		Canceled:    true,
		Message:     "operation cancellation recorded",
	}, nil
}

// --- Pairing & Device Management Operations (O06) ---

// GetEnrollmentStatus queries the status of a specific enrollment request.
func (c *Controller) GetEnrollmentStatus(ctx context.Context, requestID string) (*EnrollmentStatusResult, error) {
	if requestID == "" {
		return nil, &ControlError{Code: "INVALID_REQUEST", Message: "request_id is required"}
	}
	enrReq, err := c.db.GetEnrollmentRequest(ctx, requestID)
	if err != nil {
		return nil, &ControlError{Code: "NOT_FOUND", Message: "enrollment request not found"}
	}

	res := &EnrollmentStatusResult{
		RequestID:      enrReq.RequestID,
		Folder:         enrReq.Folder,
		DeviceID:       enrReq.DeviceID,
		SuggestedLabel: enrReq.SuggestedLabel,
		Status:         enrReq.Status,
		Message:        fmt.Sprintf("request is %s", enrReq.Status),
	}

	if enrReq.Status == "approved" {
		cur, _, err := c.db.GetMembership(ctx, enrReq.Folder)
		if err == nil {
			res.Revision = cur.Revision
			res.Membership = &cur
		}
	}

	return res, nil
}

// SubmitJoinFlow initiates enrollment from the joining device by proving key possession to the remote endpoint.
func (c *Controller) SubmitJoinFlow(ctx context.Context, req JoinFlowSubmitRequest) (*JoinFlowSubmitResult, error) {
	if req.InvitationToken == "" {
		return nil, &ControlError{Code: "INVALID_REQUEST", Message: "invitation token is required", Action: "provide invitation token"}
	}
	if req.RemoteEndpoint == "" {
		return nil, &ControlError{Code: "INVALID_REQUEST", Message: "remote endpoint URL is required", Action: "provide reachable remote URL"}
	}
	if req.TargetFolder == "" {
		return nil, &ControlError{Code: "INVALID_REQUEST", Message: "target folder ID is required", Action: "provide 64-hex folder ID"}
	}
	var folderID history.ID
	if err := folderID.UnmarshalText([]byte(req.TargetFolder)); err != nil {
		return nil, &ControlError{Code: "INVALID_REQUEST", Message: "invalid target folder ID hex format"}
	}

	rootPath := req.RootPath
	if rootPath == "" {
		home, _ := os.UserHomeDir()
		rootPath = filepath.Join(home, "Orbit")
	}

	// Validate root path using existing preview validation
	prev, err := c.PreviewJoinRoot(ctx, PreviewJoinRootRequest{Path: rootPath, FolderID: folderID})
	if err != nil {
		return nil, err
	}
	if prev.Disallowed {
		return nil, &ControlError{Code: "INVALID_ROOT", Message: prev.Reason, Action: "choose a standard directory"}
	}

	// Load local identity key
	ident, err := replication.LoadOrCreateIdentity(c.db.StateDir(), c.options.LocalDevice, c.options.Now())
	if err != nil {
		return nil, fmt.Errorf("load local identity: %w", err)
	}

	// Generate challenge nonce and sign with local private key
	challengeBytes := make([]byte, 32)
	if _, err := rand.Read(challengeBytes); err != nil {
		return nil, fmt.Errorf("generate challenge nonce: %w", err)
	}

	privKey, ok := ident.Certificate.PrivateKey.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("local identity private key is not an ed25519 key")
	}
	pubKey := privKey.Public().(ed25519.PublicKey)
	sig := ed25519.Sign(privKey, challengeBytes)

	label := req.DeviceLabel
	if label == "" {
		label = "Orbit Device"
	}

	// Assemble payload
	joinPayload := SubmitJoinRequestPayload{
		Token:          req.InvitationToken,
		JoiningDevice:  c.options.LocalDevice,
		PublicKey:      hex.EncodeToString(pubKey),
		Signature:      hex.EncodeToString(sig),
		Challenge:      hex.EncodeToString(challengeBytes),
		SuggestedLabel: label,
		TargetFolder:   folderID,
	}

	// Submit HTTP POST to remoteEndpoint + "/api/v1/enrollment/request"
	remoteURL := strings.TrimRight(req.RemoteEndpoint, "/") + "/api/v1/enrollment/request"
	bodyBytes, err := json.Marshal(joinPayload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, remoteURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request to remote: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, &ControlError{
			Code:      "REMOTE_UNREACHABLE",
			Message:   fmt.Sprintf("cannot reach remote endpoint %s: %v", req.RemoteEndpoint, err),
			Action:    "ensure the inviting device is running and reachable on this network",
			Retryable: true,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp ControlError
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Message != "" {
			return nil, &ControlError{
				Code:      errResp.Code,
				Message:   errResp.Message,
				Action:    errResp.Action,
				Retryable: errResp.Retryable,
			}
		}
		return nil, &ControlError{
			Code:      "REMOTE_ERROR",
			Message:   fmt.Sprintf("remote endpoint returned HTTP %d", resp.StatusCode),
			Retryable: resp.StatusCode >= 500,
		}
	}

	var submitRes SubmitJoinRequestResult
	if err := json.NewDecoder(resp.Body).Decode(&submitRes); err != nil {
		return nil, fmt.Errorf("decode remote response: %w", err)
	}

	// Store pending setup state
	_ = c.db.SaveSetupState(ctx, repository.SetupStateRecord{
		Phase:           "joining",
		RootPath:        rootPath,
		DefaultFolderID: &folderID,
		Completed:       false,
	})

	return &JoinFlowSubmitResult{
		RequestID:      submitRes.RequestID,
		Status:         submitRes.Status,
		TargetFolder:   req.TargetFolder,
		RemoteEndpoint: req.RemoteEndpoint,
		RootPath:       rootPath,
		DeviceID:       hex.EncodeToString(c.options.LocalDevice[:]),
		KeyPin:         hex.EncodeToString(ident.KeyPin[:]),
		Message:        submitRes.Message,
	}, nil
}

// CompleteJoinFlow finalizes enrollment on the joining device once approved.
func (c *Controller) CompleteJoinFlow(ctx context.Context, req JoinFlowCompleteRequest) (*JoinFlowCompleteResult, error) {
	if req.RequestID == "" {
		return nil, &ControlError{Code: "INVALID_REQUEST", Message: "request_id is required"}
	}
	if req.RemoteEndpoint == "" {
		return nil, &ControlError{Code: "INVALID_REQUEST", Message: "remote_endpoint is required"}
	}
	var folderID history.ID
	if err := folderID.UnmarshalText([]byte(req.TargetFolder)); err != nil {
		return nil, &ControlError{Code: "INVALID_REQUEST", Message: "invalid target folder ID"}
	}

	// Query remote status
	statusURL := fmt.Sprintf("%s/api/v1/enrollment/status?request_id=%s", strings.TrimRight(req.RemoteEndpoint, "/"), url.QueryEscape(req.RequestID))
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Get(statusURL)
	if err != nil {
		return nil, &ControlError{
			Code:      "REMOTE_UNREACHABLE",
			Message:   fmt.Sprintf("cannot reach remote endpoint to verify approval: %v", err),
			Retryable: true,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &ControlError{
			Code:    "REMOTE_ERROR",
			Message: fmt.Sprintf("status check returned HTTP %d", resp.StatusCode),
		}
	}

	var statusRes EnrollmentStatusResult
	if err := json.NewDecoder(resp.Body).Decode(&statusRes); err != nil {
		return nil, fmt.Errorf("decode remote status: %w", err)
	}

	if statusRes.Status == "declined" {
		return nil, &ControlError{
			Code:    "ENROLLMENT_DECLINED",
			Message: "workspace owner declined the join request",
			Action:  "request a new invitation or check workspace settings",
		}
	}
	if statusRes.Status != "approved" {
		return nil, &ControlError{
			Code:      "ENROLLMENT_PENDING",
			Message:   "join request is still pending owner approval",
			Retryable: true,
		}
	}

	// Setup local workspace and directory
	rootPath := req.RootPath
	if rootPath == "" {
		home, _ := os.UserHomeDir()
		rootPath = filepath.Join(home, "Orbit")
	}

	if err := os.MkdirAll(rootPath, 0o755); err != nil {
		return nil, fmt.Errorf("create root directory: %w", err)
	}

	// Register folder locally
	if _, err := c.RegisterFolder(ctx, folderID, rootPath); err != nil {
		// If already registered, ignore
		if !strings.Contains(err.Error(), "already registered") {
			return nil, fmt.Errorf("register folder: %w", err)
		}
	}
	// Apply membership
	if statusRes.Membership != nil {
		if _, err := c.db.ApproveMembership(ctx, *statusRes.Membership); err != nil {
			return nil, fmt.Errorf("apply approved membership: %w", err)
		}
	}

	// Set local device display name if provided
	if req.DeviceLabel != "" {
		_ = c.db.SetDeviceDisplayName(ctx, c.options.LocalDevice, req.DeviceLabel)
		// Update settings.json
		if st, err := config.LoadSettings(c.db.StateDir()); err == nil {
			st.DeviceLabel = req.DeviceLabel
			_ = config.SaveSettings(c.db.StateDir(), st)
		}
	}

	// Install remote peer endpoint in peers.json
	remotePeerDevHex := ""
	if statusRes.Membership != nil {
		for _, m := range statusRes.Membership.Active {
			if m.Device != c.options.LocalDevice {
				remotePeerDevHex = hex.EncodeToString(m.Device[:])
				break
			}
		}
	}
	if remotePeerDevHex != "" {
		_ = config.SetPeerEndpoint(c.db.StateDir(), config.PeerEndpoint{
			Folder:      hex.EncodeToString(folderID[:]),
			Device:      remotePeerDevHex,
			URL:         req.RemoteEndpoint,
			Certificate: filepath.Join(c.db.StateDir(), "identity.crt"),
		})
	}

	// Adopt preexisting files via scan (Invariant I22)
	_, _ = c.ws.Scan(ctx, folderID)

	// Mark setup complete
	_ = c.db.SaveSetupState(ctx, repository.SetupStateRecord{
		Phase:           "completed",
		RootPath:        rootPath,
		DefaultFolderID: &folderID,
		Completed:       true,
	})

	return &JoinFlowCompleteResult{
		Completed: true,
		FolderID:  hex.EncodeToString(folderID[:]),
		RootPath:  rootPath,
		Status:    "ready",
		Message:   "workspace joined and initialized successfully",
	}, nil
}

// CheckJoinStatus queries the enrollment status from the remote inviting device.
func (c *Controller) CheckJoinStatus(ctx context.Context, remoteEndpoint, requestID string) (*EnrollmentStatusResult, error) {
	if remoteEndpoint == "" || requestID == "" {
		return nil, &ControlError{Code: "INVALID_REQUEST", Message: "remote_endpoint and request_id are required"}
	}
	u := fmt.Sprintf("%s/api/v1/enrollment/status?request_id=%s", strings.TrimRight(remoteEndpoint, "/"), url.QueryEscape(requestID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, &ControlError{
			Code:      "REMOTE_UNREACHABLE",
			Message:   fmt.Sprintf("cannot reach remote endpoint %s: %v", remoteEndpoint, err),
			Retryable: true,
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &ControlError{
			Code:      "REMOTE_ERROR",
			Message:   fmt.Sprintf("remote endpoint returned HTTP %d", resp.StatusCode),
			Retryable: true,
		}
	}
	var res EnrollmentStatusResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode remote status response: %w", err)
	}
	return &res, nil
}

// TestPeerReachability tests network connectivity to a peer endpoint.
func (c *Controller) TestPeerReachability(ctx context.Context, targetURL string) (*PeerTestResult, error) {
	if targetURL == "" {
		return nil, &ControlError{Code: "INVALID_REQUEST", Message: "URL is required"}
	}
	testURL := strings.TrimRight(targetURL, "/") + "/api/v1/health"

	start := time.Now()
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, testURL, nil)
	if err != nil {
		return &PeerTestResult{
			Reachable: false,
			Status:    "error",
			Error:     err.Error(),
		}, nil
	}

	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return &PeerTestResult{
			Reachable: false,
			Status:    "unreachable",
			LatencyMS: latency,
			Error:     err.Error(),
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return &PeerTestResult{
			Reachable: true,
			Status:    "ok",
			LatencyMS: latency,
		}, nil
	}

	return &PeerTestResult{
		Reachable: false,
		Status:    "http_error",
		LatencyMS: latency,
		Error:     fmt.Sprintf("remote returned HTTP %d", resp.StatusCode),
	}, nil
}

// RenameDevice updates the display alias for a device.
func (c *Controller) RenameDevice(ctx context.Context, req RenameDeviceRequest) (*RenameDeviceResult, error) {
	if req.DeviceID == (history.ID{}) {
		return nil, &ControlError{Code: "INVALID_REQUEST", Message: "device_id is required"}
	}
	if req.Alias == "" {
		return nil, &ControlError{Code: "INVALID_REQUEST", Message: "alias cannot be empty"}
	}

	if err := c.db.SetDeviceDisplayName(ctx, req.DeviceID, req.Alias); err != nil {
		return nil, err
	}

	// If local device, also update settings.json
	if req.DeviceID == c.options.LocalDevice {
		if st, err := config.LoadSettings(c.db.StateDir()); err == nil {
			st.DeviceLabel = req.Alias
			_ = config.SaveSettings(c.db.StateDir(), st)
		}
	}

	return &RenameDeviceResult{
		DeviceID: req.DeviceID,
		Alias:    req.Alias,
		Status:   "updated",
	}, nil
}

// PreviewDeviceRetirement provides preview details and disclaimers before retiring a device.
func (c *Controller) PreviewDeviceRetirement(ctx context.Context, req RetireDevicePreviewRequest) (*RetireDevicePreviewResult, error) {
	if req.Folder == (history.ID{}) {
		return nil, &ControlError{Code: "INVALID_REQUEST", Message: "folder is required"}
	}
	if req.DeviceID == (history.ID{}) {
		return nil, &ControlError{Code: "INVALID_REQUEST", Message: "device_id is required"}
	}

	cur, _, err := c.db.GetMembership(ctx, req.Folder)
	if err != nil {
		return nil, fmt.Errorf("read membership: %w", err)
	}

	// Check if device is active
	found := false
	var remaining []string
	for _, m := range cur.Active {
		if m.Device == req.DeviceID {
			found = true
		} else {
			name, _ := c.db.GetDeviceDisplayName(ctx, m.Device)
			if name == "" {
				name = hex.EncodeToString(m.Device[:8])
			}
			remaining = append(remaining, name)
		}
	}
	if !found {
		return nil, &ControlError{Code: "NOT_FOUND", Message: "device is not an active member of this workspace"}
	}

	devName, _ := c.db.GetDeviceDisplayName(ctx, req.DeviceID)
	if devName == "" {
		devName = fmt.Sprintf("Device %x", req.DeviceID[:8])
	}

	return &RetireDevicePreviewResult{
		Folder:          req.Folder,
		DeviceID:        req.DeviceID,
		DeviceName:      devName,
		CurrentRevision: cur.Revision,
		NextRevision:    cur.Revision + 1,
		RemainingCount:  len(remaining),
		SurvivingPeers:  remaining,
		Warning:         "Permanent action: Device cannot rejoin this workspace under this cryptographic identity (Invariant I24).",
		Disclaimer:      "Retirement preserves this device's recorded history on remaining replicas. It cannot remotely erase files or data on the retired device's physical disk.",
	}, nil
}

func ptr[T any](v T) *T {
	return &v
}
