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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/web"
)

type sessionInfo struct {
	ID        string
	CSRFToken string
	ExpiresAt time.Time
}

type Server struct {
	ctrl            *Controller
	stateDir        string
	cliToken        string
	bootstrapTokens map[string]time.Time
	sessions        map[string]sessionInfo
	mu              sync.Mutex
	httpServer      *http.Server
}

func NewServer(ctrl *Controller, stateDir string) (*Server, error) {
	tokenFile := filepath.Join(stateDir, "control.token")
	var cliToken string
	if data, err := os.ReadFile(tokenFile); err == nil && len(strings.TrimSpace(string(data))) >= 32 {
		cliToken = strings.TrimSpace(string(data))
	} else {
		tokenBytes := make([]byte, 32)
		if _, err := rand.Read(tokenBytes); err != nil {
			return nil, fmt.Errorf("generate control token: %w", err)
		}
		cliToken = hex.EncodeToString(tokenBytes)
		if err := os.WriteFile(tokenFile, []byte(cliToken+"\n"), 0o600); err != nil {
			return nil, fmt.Errorf("write control token: %w", err)
		}
	}

	s := &Server{
		ctrl:            ctrl,
		stateDir:        stateDir,
		cliToken:        cliToken,
		bootstrapTokens: make(map[string]time.Time),
		sessions:        make(map[string]sessionInfo),
	}
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

	// Web UI Assets (embedded SPA)
	mux.Handle("/", web.Handler())

	// Public endpoints
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"version":          "dev",
			"protocol_version": 1,
			"schema_version":   repository.CurrentSchema,
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
	mux.HandleFunc("POST /api/v1/maintenance/reset-identity", func(w http.ResponseWriter, r *http.Request) {
		res, err := s.ctrl.ResetIdentity(r.Context(), "")
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
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
		var req RestoreBackupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: err.Error()})
			return
		}
		res, err := s.ctrl.RestoreBackup(r.Context(), req.BackupPath)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

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
		isAPI := strings.HasPrefix(r.URL.Path, "/api/")

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
	s.httpServer = &http.Server{
		Handler:      s.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
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
		case "STALE_VIEW", "STRUCTURAL_CONFLICT":
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
