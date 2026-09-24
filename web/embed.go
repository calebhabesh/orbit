package web

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

//go:embed dist/*
var distFS embed.FS

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
