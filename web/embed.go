package web

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed dist/*
var distFS embed.FS

// AssetInfo captures build and freshness metadata for embedded UI assets.
type AssetInfo struct {
	TotalFiles   int    `json:"total_files"`
	DigestSHA256 string `json:"digest_sha256"`
}

var (
	cachedAssetInfo AssetInfo
	assetInfoOnce   sync.Once
)

// GetAssetInfo computes deterministic metadata and SHA-256 digest of embedded assets.
func GetAssetInfo() AssetInfo {
	assetInfoOnce.Do(func() {
		h := sha256.New()
		count := 0
		sub := Dist()
		_ = fs.WalkDir(sub, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			data, err := fs.ReadFile(sub, path)
			if err != nil {
				return nil
			}
			count++
			h.Write([]byte(path))
			h.Write(data)
			return nil
		})
		cachedAssetInfo = AssetInfo{
			TotalFiles:   count,
			DigestSHA256: hex.EncodeToString(h.Sum(nil)),
		}
	})
	return cachedAssetInfo
}

// Dist returns an fs.FS rooted at the embedded "dist" directory.
func Dist() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}

// Handler returns an http.Handler that serves the embedded web assets with SPA fallback.
func Handler() http.Handler {
	sub := Dist()
	fileServer := http.FileServer(http.FS(sub))
	indexHTML, _ := fs.ReadFile(sub, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqPath := strings.TrimPrefix(r.URL.Path, "/")
		if reqPath == "" || reqPath == "index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(indexHTML))
			return
		}

		// If the static file exists directly in dist, serve it
		f, err := sub.Open(reqPath)
		if err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// If missing asset under assets/, return 404
		if strings.HasPrefix(reqPath, "assets/") {
			http.NotFound(w, r)
			return
		}

		// Client-side SPA routing: serve index.html without redirects
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(indexHTML))
	})
}
