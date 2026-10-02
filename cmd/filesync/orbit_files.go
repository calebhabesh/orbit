package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/calebhabesh/file-sync/internal/app"
	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/launcher"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

// handleOrbitFileAction handles mkdir, import, move, rename, and delete CLI operations.
func handleOrbitFileAction(action string, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("orbit "+action, flag.ContinueOnError)
	flags.SetOutput(stderr)

	stateFlag := flags.String("state", "", "agent state directory")
	folderFlag := flags.String("folder", "", "workspace identity (64-hex)")
	pathFlag := flags.String("path", "", "relative target path (mkdir, import, delete)")
	fileFlag := flags.String("file", "", "local source file to import from (import)")
	sourceFlag := flags.String("source", "", "source path (move/rename)")
	destFlag := flags.String("dest", "", "destination path (move/rename)")
	overwriteFlag := flags.Bool("overwrite", false, "allow displacing existing destination")
	recursiveFlag := flags.Bool("recursive", false, "delete non-empty directory recursively")
	tokenFlag := flags.String("token", "", "reviewed generation token or hash")
	idempotencyFlag := flags.String("idempotency-key", "", "idempotent operation key")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}

	stateDir, err := launcher.DiscoverState(*stateFlag)
	if err != nil {
		return err
	}

	var folder history.ID
	if *folderFlag != "" {
		if err := folder.UnmarshalText([]byte(*folderFlag)); err != nil {
			return fmt.Errorf("invalid folder hex ID: %w", err)
		}
	} else {
		// Discover first folder from live daemon or repository
		if isOrbitDaemonRunning(stateDir) {
			var folders []repository.FolderRecord
			if err := callOrbitDaemonAPI(stateDir, http.MethodGet, "/api/v1/folders", nil, &folders); err == nil && len(folders) > 0 {
				folder = folders[0].Folder
			}
		}
		if folder == (history.ID{}) {
			_ = app.WithWorkspace(context.Background(), stateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
				fList, err := db.Folders(context.Background())
				if err == nil && len(fList) > 0 {
					folder = fList[0].Folder
				}
				return nil
			})
		}
		if folder == (history.ID{}) {
			return errors.New("folder flag is required and could not be discovered")
		}
	}

	folderHex := hex.EncodeToString(folder[:])

	// Dispatch to live daemon if running
	if isOrbitDaemonRunning(stateDir) {
		return executeDaemonFileAction(stateDir, action, folderHex, *pathFlag, *fileFlag, *sourceFlag, *destFlag, *overwriteFlag, *recursiveFlag, *tokenFlag, *idempotencyFlag, stdout)
	}

	// Stopped daemon: execute in-process via app.WithWorkspace
	return app.WithWorkspace(context.Background(), stateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		local, _ := parseID(cfg.DeviceID)
		ctrl := control.New(db, ws, control.Options{LocalDevice: local})
		ctx := context.Background()

		var result any
		var actionErr error

		switch action {
		case "mkdir":
			if *pathFlag == "" {
				return errors.New("--path is required for mkdir")
			}
			result, actionErr = ctrl.CreateDir(ctx, control.CreateDirRequest{
				Folder:         folder,
				Path:           *pathFlag,
				IdempotencyKey: *idempotencyFlag,
			})

		case "import":
			if *pathFlag == "" {
				return errors.New("--path is required for import")
			}
			if *fileFlag == "" {
				return errors.New("--file is required for import")
			}
			f, err := os.Open(*fileFlag)
			if err != nil {
				return fmt.Errorf("open input file: %w", err)
			}
			defer f.Close()
			fi, err := f.Stat()
			if err != nil {
				return err
			}
			result, actionErr = ctrl.ImportFile(ctx, control.ImportFileRequest{
				Folder:         folder,
				Path:           *pathFlag,
				Executable:     fi.Mode()&0o111 != 0,
				Overwrite:      *overwriteFlag,
				ReviewedToken:  *tokenFlag,
				IdempotencyKey: *idempotencyFlag,
			}, f, uint64(fi.Size()))

		case "move", "rename":
			if *sourceFlag == "" || *destFlag == "" {
				return errors.New("--source and --dest are required for move/rename")
			}
			result, actionErr = ctrl.MoveFile(ctx, control.MoveFileRequest{
				Folder:         folder,
				SourcePath:     *sourceFlag,
				DestPath:       *destFlag,
				Overwrite:      *overwriteFlag,
				ReviewedToken:  *tokenFlag,
				IdempotencyKey: *idempotencyFlag,
			})

		case "delete":
			if *pathFlag == "" {
				return errors.New("--path is required for delete")
			}
			result, actionErr = ctrl.DeleteFile(ctx, control.DeleteFileRequest{
				Folder:         folder,
				Path:           *pathFlag,
				Recursive:      *recursiveFlag,
				ReviewedToken:  *tokenFlag,
				IdempotencyKey: *idempotencyFlag,
			})

		default:
			return fmt.Errorf("unsupported action %q", action)
		}

		if actionErr != nil {
			return actionErr
		}
		return json.NewEncoder(stdout).Encode(result)
	})
}

func executeDaemonFileAction(stateDir, action, folder, path, file, source, dest string, overwrite, recursive bool, token, idempotencyKey string, stdout io.Writer) error {
	var result json.RawMessage

	switch action {
	case "mkdir":
		if path == "" {
			return errors.New("--path is required for mkdir")
		}
		body := map[string]any{
			"folder":          folder,
			"path":            path,
			"idempotency_key": idempotencyKey,
		}
		if err := callOrbitDaemonAPI(stateDir, http.MethodPost, "/api/v1/files/mkdir", body, &result); err != nil {
			return err
		}

	case "import":
		if path == "" || file == "" {
			return errors.New("--path and --file are required for import")
		}
		return callDaemonImportMultipart(stateDir, folder, path, file, overwrite, token, idempotencyKey, stdout)

	case "move", "rename":
		if source == "" || dest == "" {
			return errors.New("--source and --dest are required for move/rename")
		}
		body := map[string]any{
			"folder":          folder,
			"source_path":     source,
			"dest_path":       dest,
			"overwrite":       overwrite,
			"reviewed_token":  token,
			"idempotency_key": idempotencyKey,
		}
		if err := callOrbitDaemonAPI(stateDir, http.MethodPost, "/api/v1/files/move", body, &result); err != nil {
			return err
		}

	case "delete":
		if path == "" {
			return errors.New("--path is required for delete")
		}
		body := map[string]any{
			"folder":          folder,
			"path":            path,
			"recursive":       recursive,
			"reviewed_token":  token,
			"idempotency_key": idempotencyKey,
		}
		if err := callOrbitDaemonAPI(stateDir, http.MethodPost, "/api/v1/files/delete", body, &result); err != nil {
			return err
		}

	default:
		return fmt.Errorf("unsupported action %q", action)
	}

	return json.NewEncoder(stdout).Encode(result)
}

func callDaemonImportMultipart(stateDir, folder, path, localFile string, overwrite bool, token, idempotencyKey string, stdout io.Writer) error {
	fileData, err := os.Open(localFile)
	if err != nil {
		return fmt.Errorf("open file for import: %w", err)
	}
	defer fileData.Close()

	bodyBuf := &bytes.Buffer{}
	mw := multipart.NewWriter(bodyBuf)
	_ = mw.WriteField("folder", folder)
	_ = mw.WriteField("path", path)
	if overwrite {
		_ = mw.WriteField("overwrite", "true")
	}
	if token != "" {
		_ = mw.WriteField("reviewed_token", token)
	}
	if idempotencyKey != "" {
		_ = mw.WriteField("idempotency_key", idempotencyKey)
	}

	fw, err := mw.CreateFormFile("file", filepath.Base(localFile))
	if err != nil {
		return err
	}
	if _, err := io.Copy(fw, fileData); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}

	// Read daemon control address and token
	addrBytes, err := os.ReadFile(filepath.Join(stateDir, "control.addr"))
	if err != nil {
		return fmt.Errorf("control.addr unreadable: %w", err)
	}
	tokenBytes, err := os.ReadFile(filepath.Join(stateDir, "control.token"))
	if err != nil {
		return fmt.Errorf("control.token unreadable: %w", err)
	}
	addr := strings.TrimSpace(string(addrBytes))
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}
	apiToken := strings.TrimSpace(string(tokenBytes))

	endpointURL, parseErr := url.Parse(addr)
	if parseErr != nil {
		return parseErr
	}
	endpointURL.Path = "/api/v1/files/import"

	req, err := http.NewRequest(http.MethodPost, endpointURL.String(), bodyBuf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+apiToken)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("daemon HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp control.ControlError
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Message != "" {
			return &errResp
		}
		return fmt.Errorf("daemon returned HTTP %d", resp.StatusCode)
	}

	var result json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(result)
}
