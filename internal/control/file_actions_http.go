package control

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/calebhabesh/file-sync/internal/history"
)

func (s *Server) registerFileActions(mux *http.ServeMux) {
	// POST /api/v1/files/import (streaming upload or multipart)
	mux.HandleFunc("POST /api/v1/files/import", func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")

		var folder history.ID
		var path string
		var overwrite, executable bool
		var reviewedToken, idempotencyKey, opID string
		var reader io.Reader
		var size uint64

		if strings.HasPrefix(contentType, "multipart/form-data") {
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: "failed to parse multipart form: " + err.Error()})
				return
			}
			folderHex := r.FormValue("folder")
			if err := folder.UnmarshalText([]byte(folderHex)); err != nil {
				writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: "invalid folder ID hex format"})
				return
			}
			path = r.FormValue("path")
			overwrite = r.FormValue("overwrite") == "true"
			executable = r.FormValue("executable") == "true"
			reviewedToken = r.FormValue("reviewed_token")
			idempotencyKey = r.FormValue("idempotency_key")
			opID = r.FormValue("operation_id")

			file, header, err := r.FormFile("file")
			if err != nil {
				writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: "file field required in multipart form"})
				return
			}
			defer file.Close()
			reader = file
			if header.Size > 0 {
				size = uint64(header.Size)
			}
		} else {
			// Streaming raw body
			q := r.URL.Query()
			folderHex := q.Get("folder")
			if err := folder.UnmarshalText([]byte(folderHex)); err != nil {
				writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: "invalid folder ID hex format"})
				return
			}
			path = q.Get("path")
			overwrite = q.Get("overwrite") == "true"
			executable = q.Get("executable") == "true"
			reviewedToken = q.Get("reviewed_token")
			idempotencyKey = q.Get("idempotency_key")
			opID = q.Get("operation_id")

			reader = r.Body
			defer r.Body.Close()
			if r.ContentLength > 0 {
				size = uint64(r.ContentLength)
			}
		}

		if path == "" {
			writeError(w, &ControlError{Code: "INVALID_PATH", Message: "path parameter is required"})
			return
		}

		req := ImportFileRequest{
			Folder:         folder,
			Path:           path,
			Executable:     executable,
			Overwrite:      overwrite,
			ReviewedToken:  reviewedToken,
			OperationID:    opID,
			IdempotencyKey: idempotencyKey,
		}

		res, err := s.ctrl.ImportFile(r.Context(), req, reader, size)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// POST /api/v1/files/mkdir
	mux.HandleFunc("POST /api/v1/files/mkdir", func(w http.ResponseWriter, r *http.Request) {
		var req CreateDirRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: "invalid JSON body: " + err.Error()})
			return
		}
		res, err := s.ctrl.CreateDir(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// POST /api/v1/files/move
	mux.HandleFunc("POST /api/v1/files/move", func(w http.ResponseWriter, r *http.Request) {
		var req MoveFileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: "invalid JSON body: " + err.Error()})
			return
		}
		res, err := s.ctrl.MoveFile(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// POST /api/v1/files/delete
	mux.HandleFunc("POST /api/v1/files/delete", func(w http.ResponseWriter, r *http.Request) {
		var req DeleteFileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Also support query parameters if body empty
			q := r.URL.Query()
			folderHex := q.Get("folder")
			if folderHex != "" {
				_ = req.Folder.UnmarshalText([]byte(folderHex))
				req.Path = q.Get("path")
				req.Recursive, _ = strconv.ParseBool(q.Get("recursive"))
				req.ReviewedToken = q.Get("reviewed_token")
				req.IdempotencyKey = q.Get("idempotency_key")
				req.OperationID = q.Get("operation_id")
			} else {
				writeError(w, &ControlError{Code: "INVALID_REQUEST", Message: "invalid JSON body: " + err.Error()})
				return
			}
		}
		res, err := s.ctrl.DeleteFile(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}
