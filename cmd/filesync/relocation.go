package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/calebhabesh/file-sync/internal/app"
	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/control"
	"github.com/calebhabesh/file-sync/internal/repository"
	"github.com/calebhabesh/file-sync/internal/state"
	"github.com/calebhabesh/file-sync/internal/workspace"
)

func handleFoldersRelocate(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("folders relocate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	alt := flags.String("state-dir", "", "agent state directory")
	folderText := flags.String("folder", "", "workspace ID")
	expected := flags.String("from", "", "current registered location")
	dest := flags.String("to", "", "unused destination directory (parent must exist)")
	jsonOutput := flags.Bool("json", false, "write structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *alt != "" {
		*stateDir = *alt
	}
	if *folderText == "" || *expected == "" || *dest == "" {
		return errors.New("folders relocate requires --folder, --from and --to")
	}
	folder, err := parseID(*folderText)
	if err != nil {
		return err
	}
	req := control.RelocateFolderRequest{Folder: folder, ExpectedPath: *expected, Path: *dest}
	var result workspace.RelocationResult
	err = app.WithWorkspace(context.Background(), *stateDir, func(cfg config.Config, db *repository.DB, ws *workspace.Workspace) error {
		res, err := control.New(db, ws).RelocateFolder(context.Background(), req)
		if err != nil {
			return err
		}
		result = *res
		return nil
	})
	if errors.Is(err, state.ErrLocked) {
		err = callOrbitDaemonAPI(*stateDir, "POST", "/api/v1/folders/relocate", req, &result)
	}
	if err != nil {
		return err
	}
	if *jsonOutput {
		return json.NewEncoder(stdout).Encode(result)
	}
	fmt.Fprintf(stdout, "Folder location changed to %s\n", result.Path)
	if result.SourceRetained {
		fmt.Fprintf(stdout, "Original safety copy retained at %s\n", result.SourcePath)
	}
	return nil
}
