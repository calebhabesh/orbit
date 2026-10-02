package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
)

func TestOrbitBrowse_ContentHTTPAuthorizationAndPreviews(t *testing.T) {
	ctrl, srv, httpSrv, db, _, dev, cleanup := setupNode(t, "O07")
	defer cleanup()
	ctx := context.Background()
	var folder history.ID
	rand.Read(folder[:])
	if err := db.EnsureFolder(ctx, folder, dev, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.RegisterFolder(ctx, folder, testkit.NewDisposable(t)); err != nil {
		t.Fatal(err)
	}
	save := func(path string, content []byte) history.Envelope {
		manifest, err := db.StoreFile(ctx, bytes.NewReader(content), false)
		if err != nil {
			t.Fatal(err)
		}
		env, err := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{Folder: folder, Path: path, Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1})
		if err != nil {
			t.Fatal(err)
		}
		return env
	}
	active := save("docs/<script>\";\n.txt", []byte("<script>window.hacked=true</script>"))
	other := save("docs/other.txt", []byte("other"))
	contentURL := func(env history.Envelope) string {
		return fmt.Sprintf("/api/v1/content?folder=%s&author=%s&counter=%d", hex.EncodeToString(env.ID.Folder[:]), hex.EncodeToString(env.ID.Author[:]), env.ID.Counter)
	}
	fetch := func(path, token, rangeHeader string) *http.Response {
		req, err := http.NewRequest(http.MethodGet, httpSrv.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if rangeHeader != "" {
			req.Header.Set("Range", rangeHeader)
		}
		resp, err := httpSrv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	resp := fetch(contentURL(active), "", "")
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("unauthenticated content %d", resp.StatusCode)
	}
	resp = fetch(contentURL(active), srv.CLIToken(), "")
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || string(data) != "<script>window.hacked=true</script>" || resp.StatusCode != 200 {
		t.Fatalf("exact bytes %d %q %v", resp.StatusCode, data, err)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment;") || strings.ContainsAny(resp.Header.Get("Content-Disposition"), "\r\n") {
		t.Fatal("unsafe disposition", resp.Header)
	}
	for key, want := range map[string]string{"Content-Type": "application/octet-stream", "Cache-Control": "no-store", "X-Content-Type-Options": "nosniff", "Content-Security-Policy": "sandbox; default-src 'none'"} {
		if resp.Header.Get(key) != want {
			t.Fatalf("%s: %s", key, resp.Header.Get(key))
		}
	}
	resp = fetch(contentURL(active)+"&preview=text", srv.CLIToken(), "")
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatal("active text unsafe", resp.Header)
	}
	resp = fetch(contentURL(other), srv.CLIToken(), "bytes=1-3")
	data, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || resp.StatusCode != 206 || string(data) != "the" {
		t.Fatalf("range %d %q %v", resp.StatusCode, data, err)
	}
	resp = fetch(contentURL(other), srv.CLIToken(), "bytes=999-1000")
	resp.Body.Close()
	if resp.StatusCode != 416 {
		t.Fatalf("invalid range %d", resp.StatusCode)
	}
	for _, path := range []string{"/etc/passwd", "docs/../docs", "docs//x", ".filesync-internal"} {
		resp = fetch("/api/v1/browse?folder="+hex.EncodeToString(folder[:])+"&path="+url.QueryEscape(path), srv.CLIToken(), "")
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("invalid path %q %d", path, resp.StatusCode)
		}
	}
	var unknown history.ID
	rand.Read(unknown[:])
	bad := active
	bad.ID.Folder = unknown
	resp = fetch(contentURL(bad), srv.CLIToken(), "")
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("unknown folder %d", resp.StatusCode)
	}
	if _, err := ctrl.OpenContent(ctx, unknown, active.ID); err == nil {
		t.Fatal("cross-workspace version accepted")
	}
	oversized := save("oversized.txt", bytes.Repeat([]byte("a"), (1<<20)+1))
	resp = fetch(contentURL(oversized)+"&preview=text", srv.CLIToken(), "")
	resp.Body.Close()
	if resp.StatusCode != 413 {
		t.Fatalf("unbounded text preview %d", resp.StatusCode)
	}
	var picture bytes.Buffer
	png.Encode(&picture, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	raster := save("image.png", picture.Bytes())
	resp = fetch(contentURL(raster)+"&preview=raster", srv.CLIToken(), "")
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" {
		t.Fatal("raster preview", resp.StatusCode, resp.Header)
	}
	huge := append([]byte{}, picture.Bytes()...)
	binary.BigEndian.PutUint32(huge[16:20], 4096)
	binary.BigEndian.PutUint32(huge[20:24], 4096)
	binary.BigEndian.PutUint32(huge[29:33], crc32.ChecksumIEEE(huge[12:29]))
	hugeEnv := save("huge.png", huge)
	resp = fetch(contentURL(hugeEnv)+"&preview=raster", srv.CLIToken(), "")
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("pixel budget not enforced: %d", resp.StatusCode)
	}
	animation := make([]byte, 20)
	binary.BigEndian.PutUint32(animation[:4], 8)
	copy(animation[4:8], "acTL")
	binary.BigEndian.PutUint32(animation[8:12], 2)
	binary.BigEndian.PutUint32(animation[16:20], crc32.ChecksumIEEE(animation[4:16]))
	apng := append(append(append([]byte{}, picture.Bytes()[:33]...), animation...), picture.Bytes()[33:]...)
	animated := save("animated.png", apng)
	resp = fetch(contentURL(animated)+"&preview=raster", srv.CLIToken(), "")
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("animation decode budget bypass: %d", resp.StatusCode)
	}
	encoded := save("encoded-too-large.png", bytes.Repeat([]byte("p"), (10<<20)+1))
	resp = fetch(contentURL(encoded)+"&preview=raster", srv.CLIToken(), "")
	resp.Body.Close()
	if resp.StatusCode != 413 {
		t.Fatalf("encoded raster budget not enforced: %d", resp.StatusCode)
	}

	svg := save("active.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	resp = fetch(contentURL(svg)+"&preview=raster", srv.CLIToken(), "")
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("SVG preview %d", resp.StatusCode)
	}
	// Browse/search endpoints expose pending locally known states and cursors.
	resp = fetch("/api/v1/browse?folder="+hex.EncodeToString(folder[:])+"&limit=1", srv.CLIToken(), "")
	var listing repository.BrowseResult
	err = json.NewDecoder(resp.Body).Decode(&listing)
	resp.Body.Close()
	if err != nil || len(listing.Items) != 1 || listing.NextCursor == "" {
		t.Fatalf("listing %+v %v", listing, err)
	}
	save("new.txt", []byte("new"))
	resp = fetch("/api/v1/browse?folder="+hex.EncodeToString(folder[:])+"&cursor="+url.QueryEscape(listing.NextCursor), srv.CLIToken(), "")
	resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Fatalf("stale cursor HTTP %d", resp.StatusCode)
	}
	resp = fetch("/api/v1/search?folder="+hex.EncodeToString(folder[:])+"&q=docs&limit=1", srv.CLIToken(), "")
	var results repository.SearchResult
	err = json.NewDecoder(resp.Body).Decode(&results)
	resp.Body.Close()
	if err != nil || len(results.Items) != 1 || !results.HasMore {
		t.Fatalf("search %+v %v", results, err)
	}
	// A real browser cookie authenticates native GET download URLs without a
	// bearer secret in the URL; the middleware still enforces session expiry.
	token := srv.GenerateBootstrapToken()
	body := fmt.Sprintf(`{"token":%q}`, token)
	resp, err = httpSrv.Client().Post(httpSrv.URL+"/api/v1/auth/bootstrap", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	cookies := resp.Cookies()
	resp.Body.Close()
	if len(cookies) == 0 {
		t.Fatal("no browser session")
	}
	req, _ := http.NewRequest(http.MethodGet, httpSrv.URL+contentURL(other), nil)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	resp, err = httpSrv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal("browser download rejected", resp.StatusCode)
	}
	// Once a membership excludes the local identity, its stored replica is not
	// silently exposed by these control reads. Retirement cannot erase disk bytes.
	remote := history.ID{88}
	_, err = db.ApproveMembership(ctx, protocol.Membership{Folder: folder, Revision: 1, Active: []protocol.ActiveMember{{Device: remote, KeyPin: history.Digest{88}}}})
	if err != nil {
		t.Fatal(err)
	}
	resp = fetch(contentURL(other), srv.CLIToken(), "")
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("excluded local member read %d", resp.StatusCode)
	}

}

// This writer blocks after the first response bytes so GC is deterministically
// run while the real HTTP handler owns the stream. No wall-clock race schedule.
type pausedResponse struct {
	*httptest.ResponseRecorder
	once             sync.Once
	started, release chan struct{}
}

func (w *pausedResponse) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.started); <-w.release })
	return len(p), nil
}

func TestOrbitContent_HTTPStreamingCancellationConcurrentGC(t *testing.T) {
	ctrl, srv, httpSrv, db, _, dev, cleanup := setupNode(t, "O07-stream")
	defer cleanup()
	ctx := context.Background()
	var folder history.ID
	rand.Read(folder[:])
	if err := db.EnsureFolder(ctx, folder, dev, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.RegisterFolder(ctx, folder, testkit.NewDisposable(t)); err != nil {
		t.Fatal(err)
	}
	chunk := bytes.Repeat([]byte("q"), 1<<20)
	manifest, err := db.StoreFile(ctx, io.LimitReader(&constantReader{value: 'q'}, 64<<20), false)
	if err != nil {
		t.Fatal(err)
	}
	env, err := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{Folder: folder, Path: "archive.bin", Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	successor, err := db.StoreFile(ctx, strings.NewReader("new current head"), false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.CreateLocalVersion(ctx, repository.LocalVersionRequest{Folder: folder, Path: env.Path, Kind: history.KindFile, Manifest: successor, AuthoredRevision: 1, Basis: []history.VersionID{env.ID}})
	if err != nil {
		t.Fatal(err)
	}
	target := fmt.Sprintf("/api/v1/content?folder=%x&author=%x&counter=%d", folder[:], env.ID.Author[:], env.ID.Counter)
	cancelled, cancel := context.WithCancel(ctx)
	req := httptest.NewRequest(http.MethodGet, httpSrv.URL+target, nil).WithContext(cancelled)
	req.Header.Set("Authorization", "Bearer "+srv.CLIToken())
	writer := &pausedResponse{ResponseRecorder: httptest.NewRecorder(), started: make(chan struct{}), release: make(chan struct{})}
	done := make(chan struct{})
	go func() { defer close(done); srv.Handler().ServeHTTP(writer, req) }()
	select {
	case <-writer.started:
	case <-time.After(10 * time.Second):
		t.Fatal("stream did not start")
	}
	policy := repository.RetentionPolicy{RetentionDays: 0, MinSuperseded: 0}
	report, err := db.RunGC(ctx, folder, &policy, time.Now().Add(time.Hour))
	if err != nil || report.UnlinkedObjects != 0 {
		t.Fatalf("stream lost GC pins: %+v %v", report, err)
	}
	cancel()
	close(writer.release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("canceled response did not release")
	}
	report, err = db.RunGC(ctx, folder, &policy, time.Now().Add(time.Hour))
	if err != nil || report.UnlinkedObjects != 1 {
		t.Fatalf("disconnect retained stream pins: %+v %v", report, err)
	}
	// Reinstall identical historical bytes, then measure actual HTTP streaming.
	if err := db.InstallChunk(ctx, manifest.Chunks[0].Digest, uint64(len(chunk)), bytes.NewReader(chunk)); err != nil {
		t.Fatal(err)
	}
	measured, err := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{Folder: folder, Path: "measure.bin", Kind: history.KindFile, Manifest: manifest, AuthoredRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	target = fmt.Sprintf("/api/v1/content?folder=%x&author=%x&counter=%d", folder[:], measured.ID.Author[:], measured.ID.Counter)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	// Exercise an actual net/http server with a tiny absolute timeout: hashing
	// and the progressing download must use the content-specific idle deadline.
	deadlineServer := httptest.NewUnstartedServer(srv.Handler())
	deadlineServer.Config.WriteTimeout = time.Millisecond
	deadlineServer.Start()
	defer deadlineServer.Close()
	request, _ := http.NewRequest(http.MethodGet, httpSrv.URL+target, nil)
	request.Header.Set("Authorization", "Bearer "+srv.CLIToken())
	resp, err := httpSrv.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	n, err := io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	runtime.ReadMemStats(&after)
	if err != nil || n != 64<<20 || resp.StatusCode != 200 {
		t.Fatalf("large HTTP download %d %d %v", resp.StatusCode, n, err)
	}
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("64 MiB real TLS HTTP download total allocations: %d bytes", allocated)
	if allocated > 12<<20 {
		t.Fatalf("unbounded download allocation: %d", allocated)
	}
	deadlineRequest, _ := http.NewRequest(http.MethodGet, deadlineServer.URL+target, nil)
	deadlineRequest.Header.Set("Authorization", "Bearer "+srv.CLIToken())
	deadlineResponse, err := deadlineServer.Client().Do(deadlineRequest)
	if err != nil {
		t.Fatal(err)
	}
	count, err := io.Copy(io.Discard, deadlineResponse.Body)
	deadlineResponse.Body.Close()
	if err != nil || count != 64<<20 {
		t.Fatalf("absolute timeout interrupted large response: %d %v", count, err)
	}

}

type constantReader struct{ value byte }

func (r *constantReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.value
	}
	return len(p), nil
}
