package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/calebhabesh/file-sync/internal/app"
	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/history"
	"github.com/calebhabesh/file-sync/internal/launcher"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

// The stopped CLI and live daemon route both use the same read operations.
func handleOrbitBrowse(action string, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("orbit "+action, flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateFlag := flags.String("state", "", "agent state directory")
	folderFlag := flags.String("folder", "", "workspace identity")
	pathFlag := flags.String("path", "", "relative directory or detail path")
	queryFlag := flags.String("query", "", "literal path substring (ASCII case insensitive)")
	limitFlag := flags.Int("limit", 50, "page items (1-200)")
	cursorFlag := flags.String("cursor", "", "cursor returned by preceding page")
	sortFlag := flags.String("sort", "name", "name, size, mtime, kind")
	directionFlag := flags.String("direction", "asc", "asc or desc")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *limitFlag < 1 || *limitFlag > 200 {
		return errors.New("expected flags only; limit must be 1-200")
	}
	var folder history.ID
	if err := folder.UnmarshalText([]byte(*folderFlag)); err != nil {
		return err
	}
	stateDir, err := launcher.DiscoverState(*stateFlag)
	if err != nil {
		return err
	}
	if isOrbitDaemonRunning(stateDir) {
		endpoint := "/api/v1/browse"
		params := url.Values{"folder": {*folderFlag}, "path": {*pathFlag}, "limit": {strconv.Itoa(*limitFlag)}, "cursor": {*cursorFlag}, "sort": {*sortFlag}, "direction": {*directionFlag}}
		if action == "search" {
			endpoint = "/api/v1/search"
			params.Set("q", *queryFlag)
		}
		if action == "details" {
			endpoint = "/api/v1/browse/details"
		}
		var result json.RawMessage
		if err := callOrbitDaemonAPI(stateDir, http.MethodGet, endpoint+"?"+params.Encode(), nil, &result); err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(result)
	}
	return app.WithWorkspace(context.Background(), stateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		local, _ := parseID(cfg.DeviceID)
		ctrl := control.New(db, ws, control.Options{LocalDevice: local})
		ctx := context.Background()
		var result any
		var err error
		switch action {
		case "browse":
			result, err = ctrl.Browse(ctx, folder, repository.BrowseOptions{DirPath: *pathFlag, SortBy: *sortFlag, SortDir: *directionFlag, Limit: *limitFlag, Cursor: *cursorFlag})
		case "search":
			result, err = ctrl.Search(ctx, folder, repository.SearchOptions{Query: *queryFlag, Limit: *limitFlag, Cursor: *cursorFlag})
		case "details":
			result, err = ctrl.FileDetails(ctx, folder, *pathFlag)
		}
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(result)
	})
}
