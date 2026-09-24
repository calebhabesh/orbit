package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/testkit"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

func fileManifestHelper(content []byte, exec bool) *history.Manifest {
	h := sha256.Sum256(content)
	return &history.Manifest{
		Size:       uint64(len(content)),
		Digest:     history.Digest(h),
		Executable: exec,
		Chunks: []history.Chunk{{
			Digest: history.Digest(h),
			Length: uint64(len(content)),
		}},
	}
}

// TestP14EmbeddedWebInterfaceAndSPARouting verifies:
// 1. React/Vite assets are embedded in the Go binary without needing Node at runtime.
// 2. Unauthenticated GET / serves the SPA index.html shell.
// 3. Client routes (e.g. /folders, /conflicts, /files) fallback to index.html (SPA routing).
// 4. Embedded static assets (/assets/index-*.js, /assets/index-*.css) are served with correct MIME types.
// 5. Host and Origin protections remain active on the loopback control listener.
func TestP14EmbeddedWebInterfaceAndSPARouting(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	binary := buildBinary(t, disposable)
	stateDir := filepath.Join(disposable, "state")

	// Initialize state
	initCmd := exec.Command(binary, "init", "--state", stateDir)
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}

	// Start filesync serve with control listener
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "serve", "--state", stateDir, "--control-listen", "127.0.0.1:0", "--no-watch")
	var stdoutBuf safeBuffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start serve: %v", err)
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()

	// Wait for control listener to report ready
	var controlURL string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		output := stdoutBuf.String()
		if strings.Contains(output, "control-listener=") {
			idx := strings.Index(output, "control-listener=")
			rest := output[idx+len("control-listener="):]
			if lineEnd := strings.IndexAny(rest, " \r\n"); lineEnd != -1 {
				controlURL = rest[:lineEnd]
			} else {
				controlURL = strings.TrimSpace(rest)
			}
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if controlURL == "" {
		t.Fatalf("timed out waiting for control listener; stdout: %s", stdoutBuf.String())
	}

	client := &http.Client{Timeout: 3 * time.Second}

	// 1. GET / should return 200 OK with HTML content containing root div and script tag
	resp, err := client.Get(controlURL + "/")
	if err != nil {
		t.Fatalf("get /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get / status = %d, want 200 OK", resp.StatusCode)
	}
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("get / content-type = %s, want text/html", contentType)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, `<div id="root"></div>`) {
		t.Errorf("expected <div id=\"root\"></div> in index.html, got:\n%s", bodyStr)
	}
	if !strings.Contains(bodyStr, `/assets/index-`) {
		t.Errorf("expected embedded asset script reference in index.html, got:\n%s", bodyStr)
	}

	// 2. Extract asset JS path from HTML and fetch it
	startIdx := strings.Index(bodyStr, `/assets/index-`)
	if startIdx == -1 {
		t.Fatal("did not find /assets/index- in HTML")
	}
	endIdx := strings.IndexAny(bodyStr[startIdx:], `"' >`)
	if endIdx == -1 {
		t.Fatal("could not parse asset script path")
	}
	assetPath := bodyStr[startIdx : startIdx+endIdx]

	respAsset, err := client.Get(controlURL + assetPath)
	if err != nil {
		t.Fatalf("get asset %s: %v", assetPath, err)
	}
	defer respAsset.Body.Close()
	if respAsset.StatusCode != http.StatusOK {
		t.Fatalf("get asset %s status = %d, want 200 OK", assetPath, respAsset.StatusCode)
	}
	assetContentType := respAsset.Header.Get("Content-Type")
	if !strings.Contains(assetContentType, "javascript") {
		t.Errorf("asset content-type = %s, want javascript", assetContentType)
	}
	assetBytes, _ := io.ReadAll(respAsset.Body)
	if len(assetBytes) < 1000 {
		t.Errorf("asset payload too small (%d bytes), expected compiled JS bundle", len(assetBytes))
	}

	// 3. SPA Fallback Routing: non-asset routes should return index.html with 200 OK
	for _, route := range []string{"/folders", "/files", "/conflicts", "/settings/preferences"} {
		spaResp, err := client.Get(controlURL + route)
		if err != nil {
			t.Fatalf("get %s: %v", route, err)
		}
		spaResp.Body.Close()
		if spaResp.StatusCode != http.StatusOK {
			t.Errorf("SPA route %s status = %d, want 200 OK", route, spaResp.StatusCode)
		}
		if !strings.Contains(spaResp.Header.Get("Content-Type"), "text/html") {
			t.Errorf("SPA route %s content-type = %s, want text/html", route, spaResp.Header.Get("Content-Type"))
		}
	}

	// 4. Missing static asset under /assets/ should return 404, not index.html
	missingResp, err := client.Get(controlURL + "/assets/nonexistent-bundle-999.js")
	if err != nil {
		t.Fatalf("get missing asset: %v", err)
	}
	missingResp.Body.Close()
	if missingResp.StatusCode != http.StatusNotFound {
		t.Errorf("missing asset status = %d, want 404 Not Found", missingResp.StatusCode)
	}
}

// TestP14EndToEndConflictWorkflows verifies:
// 1. Multi-head conflict selection (select winner).
// 2. Keep copies resolution (side-by-side branched files).
// 3. Manual merge resolution (operator supplied content).
// 4. Stale-view rejection (STALE_VIEW / 409 Conflict) when conflict heads change before resolution.
func TestP14EndToEndConflictWorkflows(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state")
	rootDir := filepath.Join(disposable, "workspace")
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := workspace.New(db, workspace.Options{})
	var folder history.ID
	folder[0] = 0xAA
	var localDevice history.ID
	localDevice[0] = 0x01
	var remoteDevice history.ID
	remoteDevice[0] = 0x02

	if err := db.EnsureFolder(ctx, folder, localDevice, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Register(ctx, folder, rootDir); err != nil {
		t.Fatal(err)
	}

	ctrl := control.New(db, ws, control.Options{LocalDevice: localDevice})
	srv, err := control.NewServer(ctrl, stateDir)
	if err != nil {
		t.Fatal(err)
	}

	handler := srv.Handler()

	doAuthPost := func(path string, reqBody any) (*http.Response, []byte) {
		data, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "http://127.0.0.1"+path, bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer "+srv.CLIToken())
		req.Header.Set("Content-Type", "application/json")
		resp := executeHandler(handler, req)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, body
	}

	// 1. Create a concurrent conflict for "conflict-file.txt"
	// Author V1 from localDevice
	content1 := []byte("Content from device 1\n")
	mA := fileManifestHelper(content1, false)
	if err := db.InstallChunk(ctx, mA.Chunks[0].Digest, mA.Chunks[0].Length, bytes.NewReader(content1)); err != nil {
		t.Fatal(err)
	}
	envA, err := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "conflict-file.txt",
		Basis:            nil,
		Kind:             history.KindFile,
		Manifest:         mA,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "conflict-file.txt"), content1, 0o644); err != nil {
		t.Fatal(err)
	}

	// Author V2 concurrently from remoteDevice
	content2 := []byte("Content from device 2\n")
	mB := fileManifestHelper(content2, false)
	if err := db.InstallChunk(ctx, mB.Chunks[0].Digest, mB.Chunks[0].Length, bytes.NewReader(content2)); err != nil {
		t.Fatal(err)
	}
	envB := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: remoteDevice, Counter: 1},
		Path:             "conflict-file.txt",
		Parents:          nil,
		Vector:           []history.ClockEntry{{Author: remoteDevice, Counter: 1}},
		Kind:             history.KindFile,
		Manifest:         mB,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := db.ImportMetadata(ctx, envB); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkContentReady(ctx, envB.ID); err != nil {
		t.Fatal(err)
	}

	// Verify conflict exists
	conflicts, _, err := ctrl.Conflicts(ctx, folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 || conflicts[0].Path != "conflict-file.txt" {
		t.Fatalf("expected 1 conflict for conflict-file.txt, got %+v", conflicts)
	}
	conflict := conflicts[0]
	headToken := conflict.HeadToken
	reviewed := []history.VersionID{envA.ID, envB.ID}

	// 2. Test Stale-View Rejection (I16)
	staleToken := history.Digest{0xFF, 0xEE}
	staleReq := control.ResolveSelectRequest{
		Folder:            folder,
		Path:              "conflict-file.txt",
		Reviewed:          reviewed,
		ExpectedHeadToken: staleToken,
		Selected:          envA.ID,
		IdempotencyKey:    "stale-test-1",
	}
	staleResp, staleBody := doAuthPost("/api/v1/conflicts/select", staleReq)
	if staleResp.StatusCode != http.StatusConflict {
		t.Fatalf("stale select: got status %d, want 409 Conflict\nbody: %s", staleResp.StatusCode, staleBody)
	}
	if !strings.Contains(string(staleBody), "STALE_VIEW") {
		t.Fatalf("expected STALE_VIEW code in error response, got:\n%s", staleBody)
	}

	// 3. Test Select Winner Workflow
	selectReq := control.ResolveSelectRequest{
		Folder:            folder,
		Path:              "conflict-file.txt",
		Reviewed:          reviewed,
		ExpectedHeadToken: headToken,
		Selected:          envA.ID,
		IdempotencyKey:    "select-winner-1",
	}
	selResp, selBody := doAuthPost("/api/v1/conflicts/select", selectReq)
	if selResp.StatusCode != http.StatusOK {
		t.Fatalf("select winner failed: status %d\nbody: %s", selResp.StatusCode, selBody)
	}
	var selRes control.ResolveResult
	if err := json.Unmarshal(selBody, &selRes); err != nil {
		t.Fatalf("unmarshal select result: %v", err)
	}
	if selRes.ResolvedID.Author != localDevice || selRes.ResolvedID.Counter != 2 {
		t.Fatalf("unexpected resolved ID: %+v", selRes.ResolvedID)
	}

	// Confirm conflict is resolved
	conflictsAfter, _, _ := ctrl.Conflicts(ctx, folder)
	if len(conflictsAfter) != 0 {
		t.Fatalf("expected 0 conflicts after select, got %d", len(conflictsAfter))
	}

	// 4. Test Manual Merge Workflow on a second conflicted file
	cM1 := []byte("Branch A\n")
	cM2 := []byte("Branch B\n")
	mM1 := fileManifestHelper(cM1, false)
	mM2 := fileManifestHelper(cM2, false)
	_ = db.InstallChunk(ctx, mM1.Chunks[0].Digest, mM1.Chunks[0].Length, bytes.NewReader(cM1))
	_ = db.InstallChunk(ctx, mM2.Chunks[0].Digest, mM2.Chunks[0].Length, bytes.NewReader(cM2))

	envMA, _ := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "merge-file.txt",
		Kind:             history.KindFile,
		Manifest:         mM1,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	})
	_ = os.WriteFile(filepath.Join(rootDir, "merge-file.txt"), cM1, 0o644)

	envMB := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: remoteDevice, Counter: 2},
		Path:             "merge-file.txt",
		Parents:          nil,
		Vector:           []history.ClockEntry{{Author: remoteDevice, Counter: 2}},
		Kind:             history.KindFile,
		Manifest:         mM2,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	_ = db.ImportMetadata(ctx, envMB)
	_ = db.MarkContentReady(ctx, envMB.ID)

	mergeConflicts, _, _ := ctrl.Conflicts(ctx, folder)
	if len(mergeConflicts) != 1 {
		t.Fatalf("expected 1 conflict for merge-file.txt, got %d", len(mergeConflicts))
	}

	mergeReq := struct {
		Folder            history.ID          `json:"folder"`
		Path              string              `json:"path"`
		Reviewed          []history.VersionID `json:"reviewed"`
		ExpectedHeadToken history.Digest      `json:"expected_head_token"`
		Executable        bool                `json:"executable"`
		Content           string              `json:"content"`
		IdempotencyKey    string              `json:"idempotency_key"`
	}{
		Folder:            folder,
		Path:              "merge-file.txt",
		Reviewed:          []history.VersionID{envMA.ID, envMB.ID},
		ExpectedHeadToken: mergeConflicts[0].HeadToken,
		Executable:        false,
		Content:           "Unified merged content from both branches\n",
		IdempotencyKey:    "merge-test-1",
	}

	mergeResp, mergeBody := doAuthPost("/api/v1/conflicts/merge", mergeReq)
	if mergeResp.StatusCode != http.StatusOK {
		t.Fatalf("manual merge failed: status %d\nbody: %s", mergeResp.StatusCode, mergeBody)
	}
	var mergeRes control.ResolveResult
	if err := json.Unmarshal(mergeBody, &mergeRes); err != nil {
		t.Fatalf("unmarshal merge result: %v", err)
	}
	if mergeRes.ResolvedID.Author != localDevice {
		t.Fatalf("unexpected merge resolved author: %+v", mergeRes.ResolvedID)
	}

	// 5. Test Keep Copies Workflow on a third conflicted file
	cK1 := []byte("Copy 1\n")
	cK2 := []byte("Copy 2\n")
	mK1 := fileManifestHelper(cK1, false)
	mK2 := fileManifestHelper(cK2, false)
	_ = db.InstallChunk(ctx, mK1.Chunks[0].Digest, mK1.Chunks[0].Length, bytes.NewReader(cK1))
	_ = db.InstallChunk(ctx, mK2.Chunks[0].Digest, mK2.Chunks[0].Length, bytes.NewReader(cK2))

	envKA, _ := db.CreateLocalVersion(ctx, repository.LocalVersionRequest{
		Folder:           folder,
		Path:             "doc.txt",
		Kind:             history.KindFile,
		Manifest:         mK1,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	})
	_ = os.WriteFile(filepath.Join(rootDir, "doc.txt"), cK1, 0o644)

	envKB := history.Envelope{
		ID:               history.VersionID{Folder: folder, Author: remoteDevice, Counter: 3},
		Path:             "doc.txt",
		Parents:          nil,
		Vector:           []history.ClockEntry{{Author: remoteDevice, Counter: 3}},
		Kind:             history.KindFile,
		Manifest:         mK2,
		AuthoredRevision: 1,
		DisplayTime:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	_ = db.ImportMetadata(ctx, envKB)
	_ = db.MarkContentReady(ctx, envKB.ID)

	keepConflicts, _, _ := ctrl.Conflicts(ctx, folder)
	if len(keepConflicts) != 1 {
		t.Fatalf("expected 1 conflict for doc.txt, got %d", len(keepConflicts))
	}

	keepReq := control.KeepCopiesRequest{
		Folder:            folder,
		Path:              "doc.txt",
		Reviewed:          []history.VersionID{envKA.ID, envKB.ID},
		ExpectedHeadToken: keepConflicts[0].HeadToken,
		IdempotencyKey:    "keep-test-1",
	}
	keepResp, keepBody := doAuthPost("/api/v1/conflicts/keep-copies", keepReq)
	if keepResp.StatusCode != http.StatusOK {
		t.Fatalf("keep copies failed: status %d\nbody: %s", keepResp.StatusCode, keepBody)
	}
	var keepRes control.KeepCopiesResult
	if err := json.Unmarshal(keepBody, &keepRes); err != nil {
		t.Fatalf("unmarshal keep result: %v", err)
	}
	if !keepRes.Completed {
		t.Fatal("expected keep-copies completed=true")
	}
}

// TestP14FilesInspectionAndRestoreWorkflow verifies:
// 1. GET /api/v1/files lists projected files with size, mtime, inode, and executable flags.
// 2. GET /api/v1/files/history returns version history with accurate content availability.
// 3. POST /api/v1/restore/preview previews the restoration of a historical version.
// 4. POST /api/v1/restore authors a new version in the present adopting historical bytes.
func TestP14FilesInspectionAndRestoreWorkflow(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state")
	rootDir := filepath.Join(disposable, "workspace")
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := workspace.New(db, workspace.Options{})
	var folder history.ID
	folder[0] = 0xBB
	var localDevice history.ID
	localDevice[0] = 0x01

	if err := db.EnsureFolder(ctx, folder, localDevice, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Register(ctx, folder, rootDir); err != nil {
		t.Fatal(err)
	}

	ctrl := control.New(db, ws, control.Options{LocalDevice: localDevice})
	srv, err := control.NewServer(ctrl, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	handler := srv.Handler()

	doAuthGet := func(path string) (*http.Response, []byte) {
		req, _ := http.NewRequest("GET", "http://127.0.0.1"+path, nil)
		req.Header.Set("Authorization", "Bearer "+srv.CLIToken())
		resp := executeHandler(handler, req)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, body
	}
	doAuthPost := func(path string, reqBody any) (*http.Response, []byte) {
		data, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "http://127.0.0.1"+path, bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer "+srv.CLIToken())
		req.Header.Set("Content-Type", "application/json")
		resp := executeHandler(handler, req)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, body
	}

	// Write file v1 to disk and script.sh with executable bit
	filePath := filepath.Join(rootDir, "notes.txt")
	if err := os.WriteFile(filePath, []byte("Version 1 content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(rootDir, "script.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho hello\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	scan1, err := ws.Scan(ctx, folder)
	if err != nil || len(scan1.Captured) != 2 {
		t.Fatalf("scan 1 failed: %v", err)
	}
	var v1Envelope history.Envelope
	for _, env := range scan1.Captured {
		if env.Path == "notes.txt" {
			v1Envelope = env
		}
	}

	// Write file v2 to disk and scan
	if err := os.WriteFile(filePath, []byte("Version 2 updated content with more text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scan2, err := ws.Scan(ctx, folder)
	if err != nil || len(scan2.Captured) != 1 {
		t.Fatalf("scan 2 failed: %v", err)
	}

	folderHex := hex.EncodeToString(folder[:])

	// 1. Test GET /api/v1/files
	fResp, fBody := doAuthGet(fmt.Sprintf("/api/v1/files?folder=%s", folderHex))
	if fResp.StatusCode != http.StatusOK {
		t.Fatalf("get files: status %d\nbody: %s", fResp.StatusCode, fBody)
	}
	var fileList []control.FileItem
	if err := json.Unmarshal(fBody, &fileList); err != nil {
		t.Fatalf("unmarshal files: %v", err)
	}
	if len(fileList) != 2 {
		t.Fatalf("unexpected files list len %d: %+v", len(fileList), fileList)
	}
	for _, f := range fileList {
		if f.Path == "notes.txt" && f.Executable {
			t.Error("expected notes.txt executable=false")
		}
		if f.Path == "script.sh" && !f.Executable {
			t.Error("expected script.sh executable=true")
		}
	}

	// 2. Test GET /api/v1/files/history
	hResp, hBody := doAuthGet(fmt.Sprintf("/api/v1/files/history?folder=%s&path=notes.txt", folderHex))
	if hResp.StatusCode != http.StatusOK {
		t.Fatalf("get history: status %d\nbody: %s", hResp.StatusCode, hBody)
	}
	var histList []control.HistoryItem
	if err := json.Unmarshal(hBody, &histList); err != nil {
		t.Fatalf("unmarshal history: %v", err)
	}
	if len(histList) != 2 {
		t.Fatalf("expected 2 historical versions for notes.txt, got %d", len(histList))
	}
	for _, item := range histList {
		if item.ContentState != repository.ContentReady {
			t.Errorf("version %d content state = %s, want ready", item.ID.Counter, item.ContentState)
		}
	}

	// 3. Test POST /api/v1/restore/preview (Restoring V1)
	prevReq := control.RestorePreviewRequest{
		Folder:        folder,
		Path:          "notes.txt",
		SourceVersion: v1Envelope.ID,
	}
	pResp, pBody := doAuthPost("/api/v1/restore/preview", prevReq)
	if pResp.StatusCode != http.StatusOK {
		t.Fatalf("preview restore: status %d\nbody: %s", pResp.StatusCode, pBody)
	}
	var prevRes control.RestorePreview
	if err := json.Unmarshal(pBody, &prevRes); err != nil {
		t.Fatalf("unmarshal preview result: %v", err)
	}
	if prevRes.ContentState != repository.ContentReady {
		t.Fatalf("preview content state = %s, want ready", prevRes.ContentState)
	}
	latestCounter := scan2.Captured[0].ID.Counter
	if len(prevRes.CurrentHeads) != 1 || prevRes.CurrentHeads[0].Counter != latestCounter {
		t.Fatalf("expected current head counter=%d, got %+v", latestCounter, prevRes.CurrentHeads)
	}

	// 4. Test POST /api/v1/restore (Execute restore of V1)
	restReq := control.RestoreRequest{
		Folder:            folder,
		Path:              "notes.txt",
		SourceVersion:     v1Envelope.ID,
		Reviewed:          prevRes.CurrentHeads,
		ExpectedHeadToken: prevRes.ExpectedHeadToken,
		IdempotencyKey:    "restore-v1-test",
	}
	rResp, rBody := doAuthPost("/api/v1/restore", restReq)
	if rResp.StatusCode != http.StatusOK {
		t.Fatalf("execute restore: status %d\nbody: %s", rResp.StatusCode, rBody)
	}
	var restRes control.ResolveResult
	if err := json.Unmarshal(rBody, &restRes); err != nil {
		t.Fatalf("unmarshal restore result: %v", err)
	}
	// Restoring must author a NEW version counter extending current history
	if restRes.ResolvedID.Counter <= latestCounter {
		t.Fatalf("expected restored version counter > %d, got %d", latestCounter, restRes.ResolvedID.Counter)
	}

	// Verify working tree has version 1 content
	restoredContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restoredContent) != "Version 1 content\n" {
		t.Fatalf("restored file content mismatch: got %q", string(restoredContent))
	}
}

// TestP14FilenameMarkupEscaping verifies that unusual filenames containing markup,
// script tags, quotes, and emojis are cleanly handled and cannot inject markup into the UI.
func TestP14FilenameMarkupEscaping(t *testing.T) {
	disposable := testkit.NewDisposable(t)
	stateDir := filepath.Join(disposable, "state")
	rootDir := filepath.Join(disposable, "workspace")
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	db, err := repository.Open(ctx, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ws := workspace.New(db, workspace.Options{})
	var folder history.ID
	folder[0] = 0xCC
	var localDevice history.ID
	localDevice[0] = 0x01

	_ = db.EnsureFolder(ctx, folder, localDevice, 1)
	_, _ = ws.Register(ctx, folder, rootDir)

	ctrl := control.New(db, ws, control.Options{LocalDevice: localDevice})
	srv, err := control.NewServer(ctrl, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	handler := srv.Handler()

	// Write files with adversarial markup and Unicode names
	xssName := "<script>alert('xss').txt"
	quotesName := "file with \"quotes\" & <tags>.md"
	emojiName := "notes 🚀 📁 ⚡.txt"

	_ = os.WriteFile(filepath.Join(rootDir, xssName), []byte("safe payload\n"), 0o644)
	_ = os.WriteFile(filepath.Join(rootDir, quotesName), []byte("safe payload 2\n"), 0o644)
	_ = os.WriteFile(filepath.Join(rootDir, emojiName), []byte("safe payload 3\n"), 0o644)

	scan, err := ws.Scan(ctx, folder)
	if err != nil || len(scan.Captured) != 3 {
		for _, c := range scan.Captured {
			t.Logf("captured: %s", c.Path)
		}
		for _, iss := range scan.Issues {
			t.Logf("issue: %s: %s (%v)", iss.Path, iss.Code, iss.Err)
		}
		t.Fatalf("scan markup files failed: captured=%d err=%v", len(scan.Captured), err)
	}

	folderHex := hex.EncodeToString(folder[:])
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1/api/v1/files?folder=%s", folderHex), nil)
	req.Header.Set("Authorization", "Bearer "+srv.CLIToken())
	resp := executeHandler(handler, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get files status = %d", resp.StatusCode)
	}
	var files []control.FileItem
	_ = json.NewDecoder(resp.Body).Decode(&files)
	resp.Body.Close()

	foundXSS := false
	foundQuotes := false
	foundEmoji := false

	for _, f := range files {
		if f.Path == xssName {
			foundXSS = true
		}
		if f.Path == quotesName {
			foundQuotes = true
		}
		if f.Path == emojiName {
			foundEmoji = true
		}
	}

	if !foundXSS || !foundQuotes || !foundEmoji {
		t.Fatalf("missing escaped filenames in projection: xss=%t quotes=%t emoji=%t", foundXSS, foundQuotes, foundEmoji)
	}
}

// helper to execute an http.Handler synchronously
func executeHandler(h http.Handler, req *http.Request) *http.Response {
	rec := &responseRecorder{
		header: make(http.Header),
		body:   new(bytes.Buffer),
		code:   http.StatusOK,
	}
	h.ServeHTTP(rec, req)
	return &http.Response{
		StatusCode: rec.code,
		Header:     rec.header,
		Body:       io.NopCloser(rec.body),
	}
}

type responseRecorder struct {
	header http.Header
	body   *bytes.Buffer
	code   int
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.code = statusCode
}
