package control

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
)

const textPreviewLimit = 1 << 20
const rasterPreviewLimit = 10 << 20
const rasterPixelLimit = 16_000_000

func readInt(r *http.Request, key string, maximum int) (int, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 || n > maximum {
		return 0, readError(fmt.Errorf("invalid %s", key))
	}
	return n, nil
}
func (s *Server) registerBrowse(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/browse", func(w http.ResponseWriter, r *http.Request) {
		folder, err := parseQueryFolder(r)
		if err != nil {
			writeError(w, err)
			return
		}
		limit, err := readInt(r, "limit", 200)
		if err != nil {
			writeError(w, err)
			return
		}
		res, err := s.ctrl.Browse(r.Context(), folder, repository.BrowseOptions{DirPath: r.URL.Query().Get("path"), SortBy: r.URL.Query().Get("sort"), SortDir: r.URL.Query().Get("direction"), Limit: limit, Cursor: r.URL.Query().Get("cursor")})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("GET /api/v1/search", func(w http.ResponseWriter, r *http.Request) {
		folder, err := parseQueryFolder(r)
		if err != nil {
			writeError(w, err)
			return
		}
		limit, err := readInt(r, "limit", 200)
		if err != nil {
			writeError(w, err)
			return
		}
		res, err := s.ctrl.Search(r.Context(), folder, repository.SearchOptions{Query: r.URL.Query().Get("q"), Limit: limit, Cursor: r.URL.Query().Get("cursor")})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("GET /api/v1/browse/details", func(w http.ResponseWriter, r *http.Request) {
		folder, err := parseQueryFolder(r)
		if err != nil {
			writeError(w, err)
			return
		}
		res, err := s.ctrl.FileDetails(r.Context(), folder, r.URL.Query().Get("path"))
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("GET /api/v1/browse/history", func(w http.ResponseWriter, r *http.Request) {
		folder, err := parseQueryFolder(r)
		if err != nil {
			writeError(w, err)
			return
		}
		limit, err := readInt(r, "limit", 200)
		if err != nil {
			writeError(w, err)
			return
		}
		res, err := s.ctrl.BrowseHistory(r.Context(), folder, r.URL.Query().Get("path"), r.URL.Query().Get("cursor"), limit)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("GET /api/v1/browse/deleted", func(w http.ResponseWriter, r *http.Request) {
		folder, err := parseQueryFolder(r)
		if err != nil {
			writeError(w, err)
			return
		}
		limit, err := readInt(r, "limit", 200)
		if err != nil {
			writeError(w, err)
			return
		}
		res, err := s.ctrl.BrowseDeleted(r.Context(), folder, r.URL.Query().Get("cursor"), limit)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("GET /api/v1/content", s.serveContent)
}

func (s *Server) serveContent(w http.ResponseWriter, r *http.Request) {
	select {
	case s.contentSlots <- struct{}{}:
		defer func() { <-s.contentSlots }()
	default:
		writeError(w, &ControlError{Code: "RATE_LIMITED", Message: "too many concurrent content reads", Retryable: true})
		return
	}

	// The control server's ordinary response timeout is absolute. Downloads use
	// a per-write idle timeout so a progressing large file can exceed 30 seconds.
	responseControl := http.NewResponseController(w)
	if err := responseControl.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		writeError(w, err)
		return
	}
	w = &idleContentResponse{ResponseWriter: w, control: responseControl}

	folder, err := parseQueryFolder(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var author history.ID
	if err := author.UnmarshalText([]byte(r.URL.Query().Get("author"))); err != nil {
		writeError(w, readError(fmt.Errorf("invalid version author")))
		return
	}
	counter, err := strconv.ParseUint(r.URL.Query().Get("counter"), 10, 64)
	if err != nil || counter == 0 {
		writeError(w, readError(fmt.Errorf("invalid version counter")))
		return
	}
	preview := r.URL.Query().Get("preview")
	if preview != "" && preview != "text" && preview != "raster" {
		writeError(w, readError(fmt.Errorf("unsupported preview mode")))
		return
	}
	id := history.VersionID{Folder: folder, Author: author, Counter: counter}
	if err := s.ctrl.readableWorkspace(r.Context(), folder); err != nil {
		writeError(w, err)
		return
	}
	// Reject oversized previews before pinning/verifying a potentially huge file.
	manifest, _, err := s.ctrl.db.GetVersionManifest(r.Context(), id)
	if err != nil {
		writeError(w, readError(err))
		return
	}
	if (preview == "text" && manifest.Size > textPreviewLimit) || (preview == "raster" && manifest.Size > rasterPreviewLimit) {
		writeError(w, &ControlError{Code: "PAYLOAD_TOO_LARGE", Message: "preview exceeds byte budget; download this version"})
		return
	}
	read, err := s.ctrl.OpenContent(r.Context(), folder, id)
	if err != nil {
		writeError(w, err)
		return
	}
	defer read.Close()
	filename := filepath.Base(read.Envelope.Path)
	// FormatMediaType encodes unusual names and strips all control characters.
	filename = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, filename)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("ETag", `"`+hex.EncodeToString(read.Envelope.Manifest.Digest[:])+`"`)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	if preview != "" {
		// Allocation is explicitly bounded by the preview admission limits.
		data, err := io.ReadAll(read)
		if err != nil {
			writeError(w, readError(err))
			return
		}
		contentType := "text/plain; charset=utf-8"
		if preview == "text" {
			if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
				writeError(w, readError(fmt.Errorf("content is not plain UTF-8 text; download this version")))
				return
			}
		} else {
			cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
			if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || uint64(cfg.Width)*uint64(cfg.Height) > rasterPixelLimit {
				writeError(w, readError(fmt.Errorf("unsupported raster or decoded pixel budget exceeded")))
				return
			}
			switch format {
			case "png":
				contentType = "image/png"
			case "jpeg":
				contentType = "image/jpeg"
			case "gif":
				contentType = "image/gif"
			default:
				writeError(w, readError(fmt.Errorf("unsupported raster format")))
				return
			}
			// Browser preview accepts single-frame raster only: animated formats can
			// multiply the decode budget beyond the dimensions reported by DecodeConfig.
			if format == "gif" || (format == "png" && animatedPNG(data)) {
				writeError(w, readError(fmt.Errorf("GIF preview requires download; use PNG or JPEG")))
				return
			}
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", "inline")
		http.ServeContent(w, r, filename, time.Time{}, bytes.NewReader(data))
		return
	}
	http.ServeContent(w, r, filename, time.Time{}, read)
}

// DecodeConfig does not account for APNG frames. Walk validated chunk lengths
// and reject its animation control chunk before handing bytes to a browser.
func animatedPNG(data []byte) bool {
	if len(data) < 8 {
		return true
	}
	for offset := 8; offset < len(data); {
		if len(data)-offset < 12 {
			return true
		}
		n := uint64(binary.BigEndian.Uint32(data[offset : offset+4]))
		if n > uint64(len(data)-offset-12) {
			return true
		}
		if string(data[offset+4:offset+8]) == "acTL" {
			return true
		}
		offset += int(n) + 12
	}
	return false
}

// Refresh only while writing, retaining bounded admission for slow readers.
type idleContentResponse struct {
	http.ResponseWriter
	control *http.ResponseController
}

func (w *idleContentResponse) Write(data []byte) (int, error) {
	if err := w.control.SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return 0, err
	}
	return w.ResponseWriter.Write(data)
}
