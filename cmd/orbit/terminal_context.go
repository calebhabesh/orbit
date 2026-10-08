package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/launcher"
)

type ContextOptions struct {
	StateDir       string
	Folder         string
	Path           string
	AllowEmptyPath bool
}

type ResolvedContext struct {
	Folder     string
	FolderName string
	Root       string
	Path       string
	Generation string
	Result     tc.Result
}

func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// ResolveContext resolves human folder names, folder IDs, current working directory,
// and shell-relative file paths into validated synced-root targets and authoritative context.
func ResolveContext(ctx context.Context, client *controlclient.Client, opts ContextOptions) (*ResolvedContext, error) {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}

	q := tc.Query{
		Version: tc.Version,
		Kind:    "context",
		Cwd:     cwd,
	}

	if isHex64(opts.Folder) {
		q.Folder = opts.Folder
	} else if opts.Folder != "" {
		q.Name = opts.Folder
	}

	r, err := client.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	if r.Error != nil {
		return &ResolvedContext{Result: r}, nil
	}
	if r.Context == nil {
		return nil, errors.New("context query returned empty result without error")
	}

	// Case 1: Empty path
	if opts.Path == "" {
		if !opts.AllowEmptyPath {
			res := tc.Result{
				Version: tc.Version,
				State:   "failed",
				Error: &tc.Error{
					Code:    "INVALID_PATH",
					Message: "path is required",
					Action:  "specify a path argument",
				},
			}
			return &ResolvedContext{Result: res}, nil
		}
		return &ResolvedContext{
			Folder:     r.Context.Folder,
			FolderName: r.Context.FolderName,
			Root:       r.Context.Root,
			Path:       "",
			Generation: r.Context.Generation,
			Result:     r,
		}, nil
	}

	// Case 2: User supplied a target path
	cleanPath := filepath.Clean(opts.Path)
	var absTarget string
	if filepath.IsAbs(cleanPath) {
		absTarget = cleanPath
	} else {
		baseDir := cwd
		if baseDir == "" {
			baseDir = r.Context.Root
		}
		absTarget = filepath.Clean(filepath.Join(baseDir, cleanPath))
	}

	cleanRoot := filepath.Clean(r.Context.Root)
	rel, err := filepath.Rel(cleanRoot, absTarget)
	if err != nil || strings.HasPrefix(rel, "..") || (rel == "." && !opts.AllowEmptyPath) {
		res := tc.Result{
			Version: tc.Version,
			State:   "failed",
			Error: &tc.Error{
				Code:    "INVALID_PATH",
				Message: fmt.Sprintf("path %q escapes synced folder root %q", opts.Path, cleanRoot),
				Action:  "specify a path within the synced folder",
			},
		}
		return &ResolvedContext{Result: res}, nil
	}

	relSlash := filepath.ToSlash(rel)
	if err := history.ValidatePath(relSlash); err != nil {
		res := tc.Result{
			Version: tc.Version,
			State:   "failed",
			Error: &tc.Error{
				Code:    "INVALID_PATH",
				Message: fmt.Sprintf("invalid path: %v", err),
				Action:  "use a valid relative path within the synced folder",
			},
		}
		return &ResolvedContext{Result: res}, nil
	}

	// Query context with the validated root-relative path
	q2 := tc.Query{
		Version: tc.Version,
		Kind:    "context",
		Folder:  r.Context.Folder,
		Path:    relSlash,
	}
	r2, err := client.Query(ctx, q2)
	if err != nil {
		return nil, err
	}
	if r2.Error != nil {
		return &ResolvedContext{Result: r2}, nil
	}
	if r2.Context == nil {
		return nil, errors.New("context query returned empty result without error")
	}

	return &ResolvedContext{
		Folder:     r2.Context.Folder,
		FolderName: r2.Context.FolderName,
		Root:       r2.Context.Root,
		Path:       r2.Context.Path,
		Generation: r2.Context.Generation,
		Result:     r2,
	}, nil
}

func handleOrbitContext(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("orbit context", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDirFlag := flags.String("state", "", "explicit agent state directory")
	folderFlag := flags.String("folder", "", "target synced folder by name or 64-hex ID")
	jsonOutput := flags.Bool("json", false, "output structured JSON")

	// Extract path argument while respecting -- for literal paths
	var pathArg string
	var flagArgs []string
	literalMode := false
	for i := 0; i < len(args); i++ {
		if literalMode {
			if pathArg == "" {
				pathArg = args[i]
			}
			continue
		}
		if args[i] == "--" {
			literalMode = true
			continue
		}
		if strings.HasPrefix(args[i], "-") {
			flagArgs = append(flagArgs, args[i])
			if (args[i] == "--state" || args[i] == "-state" || args[i] == "--folder" || args[i] == "-folder") && i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
			continue
		}
		if pathArg == "" {
			pathArg = args[i]
		} else {
			flagArgs = append(flagArgs, args[i])
		}
	}

	if err := flags.Parse(flagArgs); err != nil {
		return err
	}

	stateDir, err := launcher.DiscoverState(*stateDirFlag)
	if err != nil {
		return err
	}

	client := &controlclient.Client{StateDir: stateDir}
	ctx := context.Background()

	resolved, err := ResolveContext(ctx, client, ContextOptions{
		StateDir:       stateDir,
		Folder:         *folderFlag,
		Path:           pathArg,
		AllowEmptyPath: true,
	})
	if err != nil {
		code := RenderError(stderr, err, *jsonOutput)
		return &CLIExitError{Code: code}
	}

	code := RenderResult(stdout, stderr, resolved.Result, *jsonOutput)
	if code != 0 {
		return &CLIExitError{Code: code}
	}
	return nil
}
