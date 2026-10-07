package control

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/state"
	"github.com/calebhabesh/file-sync/web"
)

var (
	Version = "1.0.1"
	Commit  = "release"
	Date    = "2026-10-01"
)

type sessionInfo struct {
	ID        string
	CSRFToken string
	ExpiresAt time.Time
}

type Server struct {
	deviceID        string
	ctrl            *Controller
	stateDir        string
	cliToken        string
	bootstrapTokens map[string]time.Time
	sessions        map[string]sessionInfo
	rateCounts      map[string]int
	rateWindow      time.Time
	mu              sync.Mutex
	httpServer      *http.Server
	contentSlots    chan struct{}
}

func NewServer(ctrl *Controller, stateDir string) (*Server, error) {
	var cliToken string
	data, err := state.ReadPrivate(stateDir, "control.token", 4096)
	if err == nil {
		if len(strings.TrimSpace(string(data))) < 32 {
			return nil, errors.New("invalid control credential; review credential recovery")
		}

		cliToken = strings.TrimSpace(string(data))
	} else if errors.Is(err, os.ErrNotExist) {
		tokenBytes := make([]byte, 32)
		if _, err := rand.Read(tokenBytes); err != nil {
			return nil, fmt.Errorf("generate control token: %w", err)
		}
		cliToken = hex.EncodeToString(tokenBytes)
		if err := config.WritePrivate(stateDir, "control.token", []byte(cliToken+"\n")); err != nil {
			return nil, fmt.Errorf("write control token: %w", err)
		}
	} else {
		return nil, err
	}
	deviceID := hex.EncodeToString(ctrl.options.LocalDevice[:])
	if cfg, err := config.Load(stateDir); err == nil {
		deviceID = cfg.DeviceID
	}
	s := &Server{
		deviceID:        deviceID,
		ctrl:            ctrl,
		contentSlots:    make(chan struct{}, 8),
		stateDir:        stateDir,
		cliToken:        cliToken,
		bootstrapTokens: make(map[string]time.Time),
		sessions:        make(map[string]sessionInfo),
	}
	// Publish immutable server ownership before Serve and Shutdown can run.
	s.httpServer = &http.Server{Handler: s.Handler(), ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second}
	return s, nil
}

func (s *Server) CLIToken() string {
	return s.cliToken
}

// GenerateBootstrapToken creates a one-use token valid for 5 minutes.
func (s *Server) GenerateBootstrapToken() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	s.bootstrapTokens[token] = time.Now().Add(5 * time.Minute)
	return token
}

func (s *Server) checkEnrollmentRateLimit(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.rateCounts == nil || now.Sub(s.rateWindow) > time.Minute {
		s.rateCounts = make(map[string]int)
		s.rateWindow = now
	}
	if s.rateCounts[ip] >= 5 {
		return false
	}
	s.rateCounts[ip]++
	return true
}

func isLoopbackHost(hostPort string) bool {
	h := hostPort
	if strings.Contains(h, ":") {
		host, _, err := net.SplitHostPort(hostPort)
		if err == nil {
			h = host
		}
	}
	h = strings.Trim(h, "[]")
	if h == "localhost" || h == "127.0.0.1" || h == "::1" || strings.HasPrefix(h, "127.") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.registerBrowse(mux)
	s.registerTerminal(mux)

	// Web UI Assets (embedded SPA)
	mux.Handle("/", web.Handler())

	// Public endpoints
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, r *http.Request) {
		assetInfo := web.GetAssetInfo()
		writeJSON(w, http.StatusOK, map[string]any{
			"product":               "Orbit",
			"version":               Version,
			"commit":                Commit,
			"built":                 Date,
			"protocol_version":      1,
			"schema_version":        repository.CurrentSchema,
			"config_format_version": config.FormatVersion,
			"embedded_assets": map[string]any{
				"total_files":           assetInfo.TotalFiles,
				"digest_sha256":         assetInfo.DigestSHA256,
				"pure_go_sqlite":        true,
				"node_runtime_required": false,
			},
		})
	})
	mux.HandleFunc("POST /api/v1/auth/bootstrap", s.handleBootstrap)
	mux.HandleFunc("POST /api/v1/auth/bootstrap-token", s.handleGenerateBootstrapToken)

	// Session management
	mux.HandleFunc("GET /api/v1/auth/session", s.handleSessionInfo)
	mux.HandleFunc("POST /api/v1/auth/logout", s.handleLogout)

	// Doctor
	mux.HandleFunc("GET /api/v1/doctor", func(w http.ResponseWriter, r *http.Request) {
		report, err := s.ctrl.Doctor(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, report)
	})

	// Folders
	mux.HandleFunc("GET /api/v1/folders", func(w http.ResponseWriter, r *http.Request) {
		folders, err := s.ctrl.Folders(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, folders)
	})
	mux.HandleFunc("POST /api/v1/folders", func(w http.ResponseWriter, r *http.Request) {
		var req FolderAddRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		reg, err := s.ctrl.RegisterFolder(r.Context(), req.Folder, req.Root)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, reg)
	})
	mux.HandleFunc("POST /api/v1/folders/pause", func(w http.ResponseWriter, r *http.Request) {
		var req FolderPauseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		if err := s.ctrl.PauseFolder(r.Context(), req.Folder, req.Reason); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "paused"})
	})
	mux.HandleFunc("POST /api/v1/folders/resume", func(w http.ResponseWriter, r *http.Request) {
		var req FolderResumeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		if err := s.ctrl.ResumeFolder(r.Context(), req.Folder); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "resumed"})
	})
	mux.HandleFunc("POST /api/v1/folders/remove", func(w http.ResponseWriter, r *http.Request) {
		var req FolderRemoveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		if err := s.ctrl.UnregisterFolder(r.Context(), req.Folder); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "removed", "message": "registration removed preserving working files"})
	})
	mux.HandleFunc("POST /api/v1/folders/relocate", func(w http.ResponseWriter, r *http.Request) {
		var req RelocateFolderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, err)
			return
		}
		result, err := s.ctrl.RelocateFolder(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("POST /api/v1/folders/revalidate", func(w http.ResponseWriter, r *http.Request) {
		var req FolderRevalidateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		if err := s.ctrl.RevalidateRoot(r.Context(), req.Folder); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "valid"})
	})

	// Peers & Membership
	mux.HandleFunc("GET /api/v1/peers", func(w http.ResponseWriter, r *http.Request) {
		folder, err := parseQueryFolder(r)
		if err != nil {
			writeError(w, err)
			return
		}
		list, err := s.ctrl.PeerList(r.Context(), folder)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	})
	mux.HandleFunc("POST /api/v1/peers/retire", func(w http.ResponseWriter, r *http.Request) {
		var req RetireMemberRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.RetireMemberExecute(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// Conflicts
	mux.HandleFunc("GET /api/v1/conflicts", func(w http.ResponseWriter, r *http.Request) {
		folder, err := parseQueryFolder(r)
		if err != nil {
			writeError(w, err)
			return
		}
		contentConflicts, structuralConflicts, err := s.ctrl.Conflicts(r.Context(), folder)
		if err != nil {
			writeError(w, err)
			return
		}
		if contentConflicts == nil {
			contentConflicts = []repository.ConflictSet{}
		}
		if structuralConflicts == nil {
			structuralConflicts = []history.StructuralConflict{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"content_conflicts":    contentConflicts,
			"structural_conflicts": structuralConflicts,
		})
	})
	mux.HandleFunc("POST /api/v1/conflicts/select", func(w http.ResponseWriter, r *http.Request) {
		var req ResolveSelectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.ResolveSelect(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/conflicts/keep-copies", func(w http.ResponseWriter, r *http.Request) {
		var req KeepCopiesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.ResolveKeepCopies(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/conflicts/merge", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Folder            history.ID          `json:"folder"`
			Path              string              `json:"path"`
			Reviewed          []history.VersionID `json:"reviewed"`
			ExpectedHeadToken history.Digest      `json:"expected_head_token"`
			Executable        bool                `json:"executable"`
			Content           string              `json:"content"`
			IdempotencyKey    string              `json:"idempotency_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.ResolveManualMerge(r.Context(), ResolveMergeRequest{
			Folder:            req.Folder,
			Path:              req.Path,
			Reviewed:          req.Reviewed,
			ExpectedHeadToken: req.ExpectedHeadToken,
			Executable:        req.Executable,
			Content:           []byte(req.Content),
			IdempotencyKey:    req.IdempotencyKey,
		})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// Files & History
	mux.HandleFunc("GET /api/v1/files", func(w http.ResponseWriter, r *http.Request) {
		folder, err := parseQueryFolder(r)
		if err != nil {
			writeError(w, err)
			return
		}
		files, err := s.ctrl.Files(r.Context(), folder)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, files)
	})
	mux.HandleFunc("GET /api/v1/files/deleted", func(w http.ResponseWriter, r *http.Request) {
		folder, err := parseQueryFolder(r)
		if err != nil {
			writeError(w, err)
			return
		}
		files, err := s.ctrl.DeletedFiles(r.Context(), folder)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, files)
	})
	mux.HandleFunc("GET /api/v1/files/history", func(w http.ResponseWriter, r *http.Request) {
		folder, err := parseQueryFolder(r)
		if err != nil {
			writeError(w, err)
			return
		}
		path := r.URL.Query().Get("path")
		if path == "" {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: "path query parameter is required"})
			return
		}
		hist, err := s.ctrl.History(r.Context(), folder, path)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, hist)
	})

	// Restore
	mux.HandleFunc("POST /api/v1/restore/preview", func(w http.ResponseWriter, r *http.Request) {
		var req RestorePreviewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.PreviewRestore(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/restore", func(w http.ResponseWriter, r *http.Request) {
		var req RestoreRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.Restore(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// Storage & GC
	mux.HandleFunc("GET /api/v1/storage/usage", func(w http.ResponseWriter, r *http.Request) {
		usage, err := s.ctrl.StorageUsage(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, usage)
	})
	mux.HandleFunc("POST /api/v1/storage/gc/run", func(w http.ResponseWriter, r *http.Request) {
		var req GCRunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.GCRun(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/storage/retention/preview", func(w http.ResponseWriter, r *http.Request) {
		var req RetentionPreviewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.RetentionPreview(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/storage/retention/change", func(w http.ResponseWriter, r *http.Request) {
		var req RetentionChangeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.RetentionChange(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/storage/gc/preview", func(w http.ResponseWriter, r *http.Request) {
		var req GCPreviewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.GCPreview(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/storage/recovery/reclaim", func(w http.ResponseWriter, r *http.Request) {
		var req ReclaimRecoveryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.ReclaimRecoveryCopies(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/storage/check", func(w http.ResponseWriter, r *http.Request) {
		var req StorageCheckRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.StorageIntegrityCheck(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/storage/repair", func(w http.ResponseWriter, r *http.Request) {
		var req StorageRepairRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.StorageRepair(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/storage/prune", func(w http.ResponseWriter, r *http.Request) {
		var req PruneRecordsRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		res, err := s.ctrl.PruneLifecycleRecords(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/maintenance/prune", func(w http.ResponseWriter, r *http.Request) {
		var req PruneRecordsRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		res, err := s.ctrl.PruneLifecycleRecords(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// Work
	mux.HandleFunc("GET /api/v1/work/status", func(w http.ResponseWriter, r *http.Request) {
		folder, _ := parseQueryFolder(r)
		var fPtr *history.ID
		if folder != ([32]byte{}) {
			fPtr = &folder
		}
		res, err := s.ctrl.WorkStatus(r.Context(), WorkStatusRequest{Folder: fPtr})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("GET /api/v1/work/list", func(w http.ResponseWriter, r *http.Request) {
		folder, _ := parseQueryFolder(r)
		state := r.URL.Query().Get("state")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		res, err := s.ctrl.WorkList(r.Context(), WorkListRequest{Folder: folder, State: state, Limit: limit})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/work/scan", func(w http.ResponseWriter, r *http.Request) {
		var req WorkScanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.WorkScan(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/work/retry", func(w http.ResponseWriter, r *http.Request) {
		var req WorkRetryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.WorkRetry(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/work/cancel", func(w http.ResponseWriter, r *http.Request) {
		var req WorkCancelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.WorkCancel(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// Support
	mux.HandleFunc("GET /api/v1/support/preview", func(w http.ResponseWriter, r *http.Request) {
		redact := r.URL.Query().Get("unredacted") != "true"
		prev, err := s.ctrl.PreviewSupportExport(r.Context(), redact)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, prev)
	})
	mux.HandleFunc("POST /api/v1/support/export", func(w http.ResponseWriter, r *http.Request) {
		var req SupportExportRequest
		if r.Body != nil && r.ContentLength > 0 {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}
		if r.URL.Query().Get("unredacted") == "true" {
			req.RedactPaths = false
		} else {
			req.RedactPaths = true
		}
		res, err := s.ctrl.ExportSupportBundle(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// Logs & Metrics
	mux.HandleFunc("GET /api/v1/logs", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		events, err := s.ctrl.ListEvents(r.Context(), limit)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, events)
	})
	mux.HandleFunc("GET /api/v1/metrics", func(w http.ResponseWriter, r *http.Request) {
		m, err := s.ctrl.Metrics(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, m)
	})

	// Maintenance
	mux.HandleFunc("POST /api/v1/maintenance/backup", func(w http.ResponseWriter, r *http.Request) {
		var req BackupRequest
		if r.Body != nil && r.ContentLength > 0 {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}
		res, err := s.ctrl.Backup(r.Context(), req.Target)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("GET /api/v1/maintenance/check", func(w http.ResponseWriter, r *http.Request) {
		res, err := s.ctrl.CheckMigration(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("GET /api/v1/maintenance/recovery", func(w http.ResponseWriter, r *http.Request) {
		res, err := s.ctrl.RecoveryInspection(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("GET /api/v1/recovery/status", func(w http.ResponseWriter, r *http.Request) {
		res, err := s.ctrl.RecoveryInspection(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/maintenance/reset-identity", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, &ControlError{
			Code:      "OPERATION_BLOCKED",
			Message:   "identity reset requires exclusive stopped state; cannot reset identity on running daemon",
			Retryable: false,
			Action:    "stop background service ('systemctl --user stop filesync' or 'filesync stop') and run 'filesync maintenance reset-identity'",
		})
	})
	mux.HandleFunc("GET /api/v1/maintenance/preflight", func(w http.ResponseWriter, r *http.Request) {
		res, err := s.ctrl.Preflight(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/maintenance/restore-backup", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, &ControlError{
			Code:      "OPERATION_BLOCKED",
			Message:   "backup restore requires exclusive stopped state; cannot restore backup on running daemon",
			Retryable: false,
			Action:    "stop background service ('systemctl --user stop filesync' or 'filesync stop') and run 'filesync maintenance restore-backup'",
		})
	})

	// Orbit Product Settings
	mux.HandleFunc("GET /api/v1/settings", func(w http.ResponseWriter, r *http.Request) {
		res, err := s.ctrl.GetSettings(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/settings", func(w http.ResponseWriter, r *http.Request) {
		var req UpdateSettingsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.UpdateSettings(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("GET /api/v1/settings/peers", func(w http.ResponseWriter, r *http.Request) {
		res, err := s.ctrl.ListPeerEndpoints(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/settings/peers", func(w http.ResponseWriter, r *http.Request) {
		var req SetPeerEndpointRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		if err := s.ctrl.SetPeerEndpoint(r.Context(), req); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("DELETE /api/v1/settings/peers", func(w http.ResponseWriter, r *http.Request) {
		var req RemovePeerEndpointRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		if err := s.ctrl.RemovePeerEndpoint(r.Context(), req); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Orbit Setup
	mux.HandleFunc("GET /api/v1/setup/inspect", func(w http.ResponseWriter, r *http.Request) {
		res, err := s.ctrl.InspectSetup(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/setup/preview-root", func(w http.ResponseWriter, r *http.Request) {
		var req PreviewCreateRootRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.PreviewCreateRoot(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/setup/preview-join-root", func(w http.ResponseWriter, r *http.Request) {
		var req PreviewJoinRootRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.PreviewJoinRoot(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/setup/start", func(w http.ResponseWriter, r *http.Request) {
		var req StartSetupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.StartSetup(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/setup/resume", func(w http.ResponseWriter, r *http.Request) {
		var req ResumeSetupRequest
		if r.Body != nil && r.ContentLength > 0 {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}
		res, err := s.ctrl.ResumeSetup(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("GET /api/v1/setup/status", func(w http.ResponseWriter, r *http.Request) {
		res, err := s.ctrl.GetSetupStatus(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// Orbit Directory Picker & Desktop Helper (O03)
	mux.HandleFunc("GET /api/v1/system/directories", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		res, err := s.ctrl.BrowseDirectories(r.Context(), DirectoryPickerRequest{
			Path:   path,
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/system/open-folder", func(w http.ResponseWriter, r *http.Request) {
		var req OpenFolderRequest
		if r.Body != nil && r.ContentLength > 0 {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}
		res, err := s.ctrl.OpenLocalFolder(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// Orbit User Service Lifecycle (O03)
	mux.HandleFunc("GET /api/v1/system/service/status", func(w http.ResponseWriter, r *http.Request) {
		res, err := s.ctrl.ServiceStatus(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/system/service/action", func(w http.ResponseWriter, r *http.Request) {
		var req ServiceActionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.ServiceAction(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// Orbit Invitations (G02)
	mux.HandleFunc("POST /api/v1/invitations", func(w http.ResponseWriter, r *http.Request) {
		var req CreateInvitationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.CreateInvitation(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("GET /api/v1/invitations", func(w http.ResponseWriter, r *http.Request) {
		folderStr := r.URL.Query().Get("folder")
		var folder history.ID
		if folderStr != "" {
			if err := folder.UnmarshalText([]byte(folderStr)); err != nil {
				writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: "invalid folder ID hex"})
				return
			}
		}
		res, err := s.ctrl.ListInvitations(r.Context(), folder)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/invitations/revoke", func(w http.ResponseWriter, r *http.Request) {
		var req RevokeInvitationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.RevokeInvitation(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// Orbit Enrollment (G02)
	mux.HandleFunc("POST /api/v1/enrollment/request", func(w http.ResponseWriter, r *http.Request) {
		clientIP := r.RemoteAddr
		if host, _, err := net.SplitHostPort(clientIP); err == nil {
			clientIP = host
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			clientIP = strings.TrimSpace(parts[0])
		}

		if !s.checkEnrollmentRateLimit(clientIP) {
			writeError(w, RateLimitError("too many enrollment requests; please wait before retrying"))
			return
		}

		if r.ContentLength > 16*1024 {
			writeError(w, PayloadTooLargeError("request payload exceeds bounded 16 KiB limit"))
			return
		}

		limitedBody := http.MaxBytesReader(w, r.Body, 16*1024)
		var req SubmitJoinRequestPayload
		if err := json.NewDecoder(limitedBody).Decode(&req); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				writeError(w, PayloadTooLargeError("request payload exceeds bounded 16 KiB limit"))
				return
			}
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.SubmitEnrollmentRequest(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("GET /api/v1/enrollment/requests", func(w http.ResponseWriter, r *http.Request) {
		folderStr := r.URL.Query().Get("folder")
		var folder history.ID
		if folderStr != "" {
			if err := folder.UnmarshalText([]byte(folderStr)); err != nil {
				writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: "invalid folder ID hex"})
				return
			}
		}
		status := r.URL.Query().Get("status")
		res, err := s.ctrl.ListEnrollmentRequests(r.Context(), folder, status)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/enrollment/approve", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Reviewed       *terminalcontract.ApprovalIntent `json:"reviewed,omitempty"`
			OperationID    string                           `json:"operation_id,omitempty"`
			RequestID      string                           `json:"request_id"`
			Folder         string                           `json:"folder"`
			Endpoint       string                           `json:"endpoint,omitempty"`
			Certificate    string                           `json:"certificate,omitempty"`
			SuggestedLabel string                           `json:"suggested_label,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		var folderID history.ID
		if body.Folder != "" {
			if raw, err := hex.DecodeString(body.Folder); err == nil && len(raw) == 32 {
				copy(folderID[:], raw)
			}
		}
		req := ApproveEnrollmentRequest{Reviewed: body.Reviewed, OperationID: body.OperationID,
			RequestID:      body.RequestID,
			Folder:         folderID,
			Endpoint:       body.Endpoint,
			Certificate:    body.Certificate,
			SuggestedLabel: body.SuggestedLabel,
		}
		res, err := s.ctrl.ApproveEnrollmentRequest(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/enrollment/decline", func(w http.ResponseWriter, r *http.Request) {
		var req DeclineEnrollmentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		if err := s.ctrl.DeclineEnrollmentRequest(r.Context(), req); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1/enrollment/status", func(w http.ResponseWriter, r *http.Request) {
		reqID := r.URL.Query().Get("request_id")
		if reqID == "" {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: "request_id is required"})
			return
		}
		res, err := s.ctrl.GetEnrollmentStatus(r.Context(), reqID)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/orbit/setup/join/submit", func(w http.ResponseWriter, r *http.Request) {
		var req JoinFlowSubmitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.SubmitJoinFlow(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/orbit/setup/join/complete", func(w http.ResponseWriter, r *http.Request) {
		var req JoinFlowCompleteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.CompleteJoinFlow(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("GET /api/v1/orbit/setup/join/status", func(w http.ResponseWriter, r *http.Request) {
		remote := r.URL.Query().Get("remote_endpoint")
		reqID := r.URL.Query().Get("request_id")
		res, err := s.ctrl.CheckJoinStatus(r.Context(), remote, reqID)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/devices/alias", func(w http.ResponseWriter, r *http.Request) {
		var req RenameDeviceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.RenameDevice(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/peers/test", func(w http.ResponseWriter, r *http.Request) {
		var req PeerTestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.TestPeerReachability(r.Context(), req.URL)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/peers/retire/preview", func(w http.ResponseWriter, r *http.Request) {
		var req RetireDevicePreviewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.PreviewDeviceRetirement(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// Orbit Membership Controls (G02/O05)
	mux.HandleFunc("GET /api/v1/membership", func(w http.ResponseWriter, r *http.Request) {
		folder, err := parseQueryFolder(r)
		if err != nil {
			writeError(w, err)
			return
		}
		res, err := s.ctrl.MembershipExport(r.Context(), folder)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/membership/export", func(w http.ResponseWriter, r *http.Request) {
		folder, err := parseQueryFolder(r)
		if err != nil {
			writeError(w, err)
			return
		}
		res, err := s.ctrl.MembershipExport(r.Context(), folder)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/membership/preview", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Folder     history.ID          `json:"folder"`
			Membership protocol.Membership `json:"membership"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.MembershipPreview(r.Context(), req.Folder, req.Membership)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/membership/import", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Folder     history.ID                    `json:"folder"`
			Membership protocol.Membership           `json:"membership"`
			Snapshots  []protocol.RetirementSnapshot `json:"snapshots,omitempty"`
			Approve    bool                          `json:"approve"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.MembershipImport(r.Context(), req.Folder, req.Membership, req.Snapshots, req.Approve)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/membership/fork/detect", func(w http.ResponseWriter, r *http.Request) {
		var req DetectForkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.DetectMembershipFork(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/membership/reconcile", func(w http.ResponseWriter, r *http.Request) {
		var req ReconcileMembershipRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.ReconcileMembership(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// Orbit Read Leases (G03 - Invariant I25)
	mux.HandleFunc("POST /api/v1/content/lease", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		var req AcquireReadLeaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.AcquireReadLease(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/content/release-lease", func(w http.ResponseWriter, r *http.Request) {
		var req ReleaseReadLeaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		if err := s.ctrl.ReleaseReadLease(r.Context(), req); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Orbit Operation Progress & Cancellation
	mux.HandleFunc("GET /api/v1/operations/progress", func(w http.ResponseWriter, r *http.Request) {
		opID := r.URL.Query().Get("id")
		if opID == "" {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: "operation id query parameter required"})
			return
		}
		res, err := s.ctrl.GetOperationProgress(r.Context(), opID)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /api/v1/operations/cancel", func(w http.ResponseWriter, r *http.Request) {
		var req CancelOperationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.CancelOperation(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// Orbit File Actions (O09)
	s.registerFileActions(mux)

	// Wrap in security middleware
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Strict Host header check (DNS rebinding prevention)
		if !isLoopbackHost(r.Host) {
			writeJSON(w, http.StatusBadRequest, &ControlError{
				Code:      "INVALID_HOST",
				Message:   fmt.Sprintf("Host %q is not a loopback address", r.Host),
				Retryable: false,
				Action:    "connect exclusively through loopback interface",
			})
			return
		}

		// 2. Strict Origin header check (Cross-origin browser request prevention)
		origin := r.Header.Get("Origin")
		if origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !isLoopbackHost(u.Host) {
				writeJSON(w, http.StatusForbidden, &ControlError{
					Code:      "UNAUTHORIZED",
					Message:   fmt.Sprintf("cross-origin requests from %q are rejected", origin),
					Retryable: false,
					Action:    "access control interface directly from local loopback",
				})
				return
			}
		}

		// 3. Authentication check for non-public paths
		// Web assets, health, version, bootstrap, and session-check are unauthenticated.
		// All other /api/ endpoints require valid CLI Bearer or browser session cookie.
		isPublicAPI := r.URL.Path == "/api/v1/health" || r.URL.Path == "/api/v1/version" || r.URL.Path == "/api/v1/auth/bootstrap" || r.URL.Path == "/api/v1/auth/session"
		isAPI := strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/control/terminal/")
		if isAPI {
			w.Header().Set("Cache-Control", "no-store")
		}

		if isAPI && !isPublicAPI {
			authHeader := r.Header.Get("Authorization")
			isCLI := false
			if strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				if token == s.cliToken {
					isCLI = true
				}
			}

			if !isCLI {
				// Check browser session cookie
				cookie, err := r.Cookie("filesync_session")
				if err != nil {
					writeJSON(w, http.StatusUnauthorized, UnauthorizedError(""))
					return
				}
				s.mu.Lock()
				sess, ok := s.sessions[cookie.Value]
				s.mu.Unlock()
				if !ok || time.Now().After(sess.ExpiresAt) {
					writeJSON(w, http.StatusUnauthorized, UnauthorizedError("session expired"))
					return
				}

				// Check CSRF token for browser mutations
				if r.Method == "POST" || r.Method == "PUT" || r.Method == "DELETE" || r.Method == "PATCH" {
					csrf := r.Header.Get("X-CSRF-Token")
					if csrf == "" || csrf != sess.CSRFToken {
						writeJSON(w, http.StatusForbidden, &ControlError{
							Code:      "UNAUTHORIZED",
							Message:   "missing or invalid CSRF token",
							Retryable: false,
							Action:    "include valid X-CSRF-Token header in mutating requests",
						})
						return
					}
				}
			}
		}

		w.Header().Set("X-Orbit-Device", s.deviceID)
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	var req BootstrapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		writeJSON(w, http.StatusBadRequest, &ControlError{Code: "INVALID_REQUEST", Message: "token is required"})
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	exp, ok := s.bootstrapTokens[req.Token]
	if !ok || time.Now().After(exp) {
		delete(s.bootstrapTokens, req.Token)
		writeJSON(w, http.StatusUnauthorized, UnauthorizedError("bootstrap token is invalid or expired"))
		return
	}

	// Burn token immediately (one-use)
	delete(s.bootstrapTokens, req.Token)

	// Issue browser session
	sBytes := make([]byte, 32)
	cBytes := make([]byte, 32)
	_, _ = rand.Read(sBytes)
	_, _ = rand.Read(cBytes)

	sessID := hex.EncodeToString(sBytes)
	csrfToken := hex.EncodeToString(cBytes)
	expiresAt := time.Now().Add(24 * time.Hour)

	s.sessions[sessID] = sessionInfo{
		ID:        sessID,
		CSRFToken: csrfToken,
		ExpiresAt: expiresAt,
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "filesync_session",
		Value:    sessID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Expires:  expiresAt,
	})

	writeJSON(w, http.StatusOK, BootstrapResult{
		SessionID: sessID,
		CSRFToken: csrfToken,
		ExpiresAt: expiresAt.UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleGenerateBootstrapToken(w http.ResponseWriter, r *http.Request) {
	tok := s.GenerateBootstrapToken()
	writeJSON(w, http.StatusOK, map[string]any{
		"bootstrap_token": tok,
		"expires_in_secs": 300,
	})
}

func (s *Server) handleSessionInfo(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("filesync_session")
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
		return
	}
	s.mu.Lock()
	sess, ok := s.sessions[cookie.Value]
	s.mu.Unlock()
	if !ok || time.Now().After(sess.ExpiresAt) {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"csrf_token":    sess.CSRFToken,
		"expires_at":    sess.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("filesync_session")
	if err == nil {
		s.mu.Lock()
		delete(s.sessions, cookie.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "filesync_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]any{"status": "logged_out"})
}

func (s *Server) Serve(l net.Listener) error {
	return s.httpServer.Serve(l)
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

func parseQueryFolder(r *http.Request) (history.ID, error) {
	var id history.ID
	text := r.URL.Query().Get("folder")
	if text == "" {
		return id, nil
	}
	raw, err := hex.DecodeString(text)
	if err != nil || len(raw) != 32 {
		return id, &ControlError{Code: "INVALID_REQUEST", Message: "folder must be 64 hexadecimal characters"}
	}
	copy(id[:], raw)
	return id, nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, err error) {
	var ce *ControlError
	if errors.As(err, &ce) {
		status := http.StatusBadRequest
		switch ce.Code {
		case "UNAUTHORIZED":
			status = http.StatusUnauthorized
		case "RATE_LIMITED":
			status = http.StatusTooManyRequests
		case "PAYLOAD_TOO_LARGE":
			status = http.StatusRequestEntityTooLarge
		case "NOT_FOUND":
			status = http.StatusNotFound
		case "MEMBERSHIP_FORK", "STALE_VIEW", "STRUCTURAL_CONFLICT", "DESTINATION_EXISTS", "SUBTREE_INVALIDATED", "IDEMPOTENCY_CONFLICT":
			status = http.StatusConflict
		case "ROOT_UNAVAILABLE", "CONTENT_PENDING", "CONTENT_UNAVAILABLE":
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, ce)
		return
	}
	writeJSON(w, http.StatusInternalServerError, &ControlError{
		Code:      "INTERNAL_ERROR",
		Message:   err.Error(),
		Retryable: false,
		Action:    "inspect error logs and retry operation",
	})
}
