package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/controlclient"
	"github.com/calebhabesh/file-sync/internal/launcher"
)

// handleOrbitFolders routes and dispatches folders management commands.
func handleOrbitFolders(args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "list":
			return handleOrbitFoldersList(args[1:], stdout, stderr)
		case "add":
			return handleFoldersAdd(args[1:], stdout, stderr)
		case "share":
			return shareFolderCLI(args[1:], stdout, stderr)
		case "pause":
			return handleOrbitFoldersPause(args[1:], stdout, stderr)
		case "resume":
			return handleOrbitFoldersResume(args[1:], stdout, stderr)
		case "relocate":
			return handleFoldersRelocate(args[1:], stdout, stderr)
		case "revalidate":
			return handleOrbitFoldersRevalidate(args[1:], stdout, stderr)
		case "remove":
			return handleFoldersRemove(args[1:], stdout, stderr)
		default:
			return fmt.Errorf("unknown folders subcommand %q; run 'orbit help folders' for available subcommands", args[0])
		}
	}
	return handleOrbitFoldersList(args, stdout, stderr)
}

func handleOrbitFoldersList(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("orbit folders list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDirFlag := flags.String("state", "", "explicit agent state directory")
	jsonOutput := flags.Bool("json", false, "output structured JSON")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_ = handleOrbitHelp([]string{"folders"}, stdout, stderr)
			return nil
		}
		return err
	}

	stateDir, err := launcher.DiscoverState(*stateDirFlag)
	if err != nil {
		return err
	}

	client := &controlclient.Client{StateDir: stateDir}
	ctx := context.Background()

	res, err := client.Query(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "folders",
	})
	if err != nil {
		code := RenderError(stderr, err, *jsonOutput)
		return &CLIExitError{Code: code}
	}

	code := RenderResult(stdout, stderr, res, *jsonOutput)
	if code != 0 {
		return &CLIExitError{Code: code}
	}
	return nil
}

func parsePositionalFolder(args []string, valueFlags ...string) (string, []string) {
	vFlags := make(map[string]bool, len(valueFlags)*2)
	for _, f := range valueFlags {
		vFlags["-"+f] = true
		vFlags["--"+f] = true
	}
	var positional string
	var flagArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			if vFlags[arg] && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flagArgs = append(flagArgs, args[i])
			}
		} else if positional == "" {
			positional = arg
		} else {
			flagArgs = append(flagArgs, arg)
		}
	}
	return positional, flagArgs
}

func handleOrbitFoldersPause(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("orbit folders pause", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDirFlag := flags.String("state", "", "explicit agent state directory")
	folderFlag := flags.String("folder", "", "target synced folder by name or 64-hex ID")
	reasonFlag := flags.String("reason", "operator paused", "reason for pausing folder")
	jsonOutput := flags.Bool("json", false, "output structured JSON")

	positionalFolder, flagArgs := parsePositionalFolder(args, "state", "folder", "reason")
	if err := flags.Parse(flagArgs); err != nil {
		return err
	}
	targetFolder := *folderFlag
	if targetFolder == "" {
		targetFolder = positionalFolder
	}

	stateDir, err := launcher.DiscoverState(*stateDirFlag)
	if err != nil {
		return err
	}
	client := &controlclient.Client{StateDir: stateDir}
	ctx := context.Background()

	resolved, err := ResolveContext(ctx, client, ContextOptions{
		StateDir:       stateDir,
		Folder:         targetFolder,
		AllowEmptyPath: true,
	})
	if err != nil {
		code := RenderError(stderr, err, *jsonOutput)
		return &CLIExitError{Code: code}
	}
	if resolved.Result.Error != nil {
		code := RenderResult(stdout, stderr, resolved.Result, *jsonOutput)
		return &CLIExitError{Code: code}
	}

	folderID, err := parseID(resolved.Folder)
	if err != nil {
		code := RenderError(stderr, err, *jsonOutput)
		return &CLIExitError{Code: code}
	}

	var pauseRes map[string]any
	err = client.WithController(ctx, func() error {
		return client.Call(ctx, http.MethodPost, "/api/v1/folders/pause", control.FolderPauseRequest{
			Folder: folderID,
			Reason: *reasonFlag,
		}, &pauseRes)
	}, func(ctrl *control.Controller) error {
		return ctrl.PauseFolder(ctx, folderID, *reasonFlag)
	})
	if err != nil {
		code := RenderError(stderr, err, *jsonOutput)
		return &CLIExitError{Code: code}
	}

	res := tc.Result{
		Version: tc.Version,
		State:   "paused",
		Context: resolved.Result.Context,
	}
	code := RenderResult(stdout, stderr, res, *jsonOutput)
	if code != 0 {
		return &CLIExitError{Code: code}
	}
	if !*jsonOutput {
		fmt.Fprintf(stdout, "Paused folder %s (%s)\n", EscapeTerminal(resolved.FolderName), resolved.Folder)
	}
	return nil
}

func handleOrbitFoldersResume(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("orbit folders resume", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDirFlag := flags.String("state", "", "explicit agent state directory")
	folderFlag := flags.String("folder", "", "target synced folder by name or 64-hex ID")
	jsonOutput := flags.Bool("json", false, "output structured JSON")

	positionalFolder, flagArgs := parsePositionalFolder(args, "state", "folder")
	if err := flags.Parse(flagArgs); err != nil {
		return err
	}
	targetFolder := *folderFlag
	if targetFolder == "" {
		targetFolder = positionalFolder
	}

	stateDir, err := launcher.DiscoverState(*stateDirFlag)
	if err != nil {
		return err
	}
	client := &controlclient.Client{StateDir: stateDir}
	ctx := context.Background()

	resolved, err := ResolveContext(ctx, client, ContextOptions{
		StateDir:       stateDir,
		Folder:         targetFolder,
		AllowEmptyPath: true,
	})
	if err != nil {
		code := RenderError(stderr, err, *jsonOutput)
		return &CLIExitError{Code: code}
	}
	if resolved.Result.Error != nil {
		code := RenderResult(stdout, stderr, resolved.Result, *jsonOutput)
		return &CLIExitError{Code: code}
	}

	folderID, err := parseID(resolved.Folder)
	if err != nil {
		code := RenderError(stderr, err, *jsonOutput)
		return &CLIExitError{Code: code}
	}

	var resumeRes map[string]any
	err = client.WithController(ctx, func() error {
		return client.Call(ctx, http.MethodPost, "/api/v1/folders/resume", control.FolderResumeRequest{
			Folder: folderID,
		}, &resumeRes)
	}, func(ctrl *control.Controller) error {
		return ctrl.ResumeFolder(ctx, folderID)
	})
	if err != nil {
		code := RenderError(stderr, err, *jsonOutput)
		return &CLIExitError{Code: code}
	}

	res := tc.Result{
		Version: tc.Version,
		State:   "resumed",
		Context: resolved.Result.Context,
	}
	code := RenderResult(stdout, stderr, res, *jsonOutput)
	if code != 0 {
		return &CLIExitError{Code: code}
	}
	if !*jsonOutput {
		fmt.Fprintf(stdout, "Resumed folder %s (%s)\n", EscapeTerminal(resolved.FolderName), resolved.Folder)
	}
	return nil
}

func handleOrbitFoldersRevalidate(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("orbit folders revalidate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDirFlag := flags.String("state", "", "explicit agent state directory")
	folderFlag := flags.String("folder", "", "target synced folder by name or 64-hex ID")
	jsonOutput := flags.Bool("json", false, "output structured JSON")

	positionalFolder, flagArgs := parsePositionalFolder(args, "state", "folder")
	if err := flags.Parse(flagArgs); err != nil {
		return err
	}
	targetFolder := *folderFlag
	if targetFolder == "" {
		targetFolder = positionalFolder
	}

	stateDir, err := launcher.DiscoverState(*stateDirFlag)
	if err != nil {
		return err
	}
	client := &controlclient.Client{StateDir: stateDir}
	ctx := context.Background()

	resolved, err := ResolveContext(ctx, client, ContextOptions{
		StateDir:       stateDir,
		Folder:         targetFolder,
		AllowEmptyPath: true,
	})
	if err != nil {
		code := RenderError(stderr, err, *jsonOutput)
		return &CLIExitError{Code: code}
	}
	if resolved.Result.Error != nil {
		code := RenderResult(stdout, stderr, resolved.Result, *jsonOutput)
		return &CLIExitError{Code: code}
	}

	folderID, err := parseID(resolved.Folder)
	if err != nil {
		code := RenderError(stderr, err, *jsonOutput)
		return &CLIExitError{Code: code}
	}

	var revRes map[string]any
	err = client.WithController(ctx, func() error {
		return client.Call(ctx, http.MethodPost, "/api/v1/folders/revalidate", control.FolderRevalidateRequest{
			Folder: folderID,
		}, &revRes)
	}, func(ctrl *control.Controller) error {
		return ctrl.RevalidateRoot(ctx, folderID)
	})
	if err != nil {
		code := RenderError(stderr, err, *jsonOutput)
		return &CLIExitError{Code: code}
	}

	res := tc.Result{
		Version: tc.Version,
		State:   "valid",
		Context: resolved.Result.Context,
	}
	code := RenderResult(stdout, stderr, res, *jsonOutput)
	if code != 0 {
		return &CLIExitError{Code: code}
	}
	if !*jsonOutput {
		fmt.Fprintf(stdout, "Revalidated folder %s (%s)\n", EscapeTerminal(resolved.FolderName), resolved.Folder)
	}
	return nil
}

// handleOrbitConflicts inspects and manages sync conflicts.
func handleOrbitConflicts(args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "show", "edit", "diff", "upload", "session", "discard", "cancel", "renew":
			return handleTerminalContent(args[0], args[1:], stdout, stderr)
		case "select":
			return handleTerminalContent("select", args[1:], stdout, stderr)
		case "merge":
			return handleTerminalContent("merge", args[1:], stdout, stderr)
		case "keep-copies":
			return handleTerminalContent("keep_copies", args[1:], stdout, stderr)
		}
	}

	return handleTerminalContent("conflicts", args, stdout, stderr)
}

// handleOrbitHistory lists known versions and content availability for a file.
func handleOrbitHistory(args []string, stdout, stderr io.Writer) error {
	return handleTerminalContent("history", args, stdout, stderr)
}

// handleOrbitDeleted finds deleted files and available restore candidates.
func handleOrbitDeleted(args []string, stdout, stderr io.Writer) error {
	return handleTerminalContent("deleted", args, stdout, stderr)
}

// handleOrbitRestore restores a previous version to original path or copy.
func handleOrbitRestore(args []string, stdout, stderr io.Writer) error {
	return handleTerminalContent("restore", args, stdout, stderr)
}

// handleOrbitDoctor runs actionable diagnostics and returns safe recovery advice.
func handleOrbitDoctor(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("orbit doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDirFlag := flags.String("state", "", "explicit agent state directory")
	jsonOutput := flags.Bool("json", false, "output structured JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}

	stateDir, err := launcher.DiscoverState(*stateDirFlag)
	if err != nil {
		return err
	}
	client := &controlclient.Client{StateDir: stateDir}
	ctx := context.Background()

	var report *control.DoctorReport
	err = client.WithController(ctx, func() error {
		return client.Call(ctx, http.MethodGet, "/api/v1/doctor", nil, &report)
	}, func(ctrl *control.Controller) error {
		var dErr error
		report, dErr = ctrl.Doctor(ctx)
		return dErr
	})
	if err != nil {
		code := RenderError(stderr, err, *jsonOutput)
		return &CLIExitError{Code: code}
	}

	if *jsonOutput {
		return json.NewEncoder(stdout).Encode(report)
	}

	fmt.Fprintf(stdout, "Orbit Doctor Report: overall=%s\n", report.OverallStatus)
	for _, c := range report.Checks {
		fmt.Fprintf(stdout, "  [%s] %s: %s\n", c.Status, c.Name, EscapeTerminal(c.Message))
		if c.Remediation != "" {
			fmt.Fprintf(stdout, "    remediation: %s\n", EscapeTerminal(c.Remediation))
		}
	}
	if report.OverallStatus == control.StatusFail {
		return &CLIExitError{Code: 1, Err: errors.New("doctor checks reported failure")}
	}
	return nil
}

// handleOrbitStatus reports named folders, local capture/pending work, attention,
// observed device-copy state/freshness, and daemon/startup facts.
func handleOrbitStatus(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("orbit status", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDirFlag := flags.String("state", "", "explicit agent state directory")
	folderFlag := flags.String("folder", "", "target synced folder by name or 64-hex ID")
	limitFlag := flags.Uint("limit", 50, "maximum items to query")
	cursorFlag := flags.String("cursor", "", "pagination cursor")
	jsonOutput := flags.Bool("json", false, "output structured JSON")

	positionalFolder, flagArgs := parsePositionalFolder(args, "state", "folder", "limit", "cursor")
	if err := flags.Parse(flagArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_ = handleOrbitHelp([]string{"status"}, stdout, stderr)
			return nil
		}
		return err
	}

	stateDir, err := launcher.DiscoverState(*stateDirFlag)
	if err != nil {
		return err
	}

	client := &controlclient.Client{StateDir: stateDir}
	ctx := context.Background()

	targetFolder := *folderFlag
	if targetFolder == "" {
		targetFolder = positionalFolder
	}

	var resolvedFolder string
	if targetFolder != "" {
		resolved, err := ResolveContext(ctx, client, ContextOptions{
			StateDir:       stateDir,
			Folder:         targetFolder,
			AllowEmptyPath: true,
		})
		if err != nil {
			code := RenderError(stderr, err, *jsonOutput)
			return &CLIExitError{Code: code}
		}
		if resolved.Result.Error != nil {
			code := RenderResult(stdout, stderr, resolved.Result, *jsonOutput)
			return &CLIExitError{Code: code}
		}
		resolvedFolder = resolved.Folder
	}

	res, err := client.Query(ctx, tc.Query{
		Version: tc.Version,
		Kind:    "status",
		Folder:  resolvedFolder,
		Limit:   tc.Uint(*limitFlag),
		Cursor:  *cursorFlag,
	})
	if err != nil {
		code := RenderError(stderr, err, *jsonOutput)
		return &CLIExitError{Code: code}
	}

	if *jsonOutput {
		daemonRunning := false
		if res.Service != nil && res.Service.Running {
			daemonRunning = true
		}
		var controlAddr string
		if daemonRunning {
			if addrBytes, err := os.ReadFile(filepath.Join(stateDir, "control.addr")); err == nil {
				controlAddr = strings.TrimSpace(string(addrBytes))
			}
		}
		svcStatus, _ := control.CheckServiceStatus(ctx, stateDir, nil)
		var setupResult *control.InspectSetupResult
		_ = client.WithController(ctx, func() error {
			return client.Call(ctx, http.MethodGet, "/api/v1/setup/inspect", nil, &setupResult)
		}, func(ctrl *control.Controller) error {
			insp, err := ctrl.InspectSetup(ctx)
			if err == nil {
				setupResult = insp
			}
			return nil
		})

		type OrbitOverallStatusJSON struct {
			tc.Result
			DaemonRunning  bool                         `json:"daemon_running"`
			StateDirectory string                       `json:"state_directory"`
			ControlAddress string                       `json:"control_address,omitempty"`
			ServiceStatus  *control.ServiceStatusResult `json:"service_status,omitempty"`
			Setup          *control.InspectSetupResult  `json:"setup,omitempty"`
			Folders        []tc.NamedItem               `json:"folders"`
		}

		out := OrbitOverallStatusJSON{
			Result:         res,
			DaemonRunning:  daemonRunning,
			StateDirectory: stateDir,
			ControlAddress: controlAddr,
			ServiceStatus:  svcStatus,
			Setup:          setupResult,
			Folders:        res.Items,
		}
		return json.NewEncoder(stdout).Encode(out)
	}

	RenderOrbitStatusHuman(stdout, res, stateDir)
	return nil
}

func RenderOrbitStatusHuman(stdout io.Writer, res tc.Result, stateDir string) {
	daemonStatus := "stopped"
	startupMode := "manual"
	if res.Service != nil {
		if res.Service.Running {
			daemonStatus = "running"
		}
		if res.Service.Mode != "" {
			startupMode = res.Service.Mode
		}
	}
	fmt.Fprintf(stdout, "Orbit                                Daemon: %-9s Startup: %s\n", daemonStatus, startupMode)

	// Needs attention section
	if len(res.Attention) > 0 {
		fmt.Fprintf(stdout, "\nNeeds attention (%d):\n", len(res.Attention))
		for _, att := range res.Attention {
			target := att.Path
			if target == "" {
				target = att.Folder
				if len(target) > 12 {
					target = target[:12]
				}
				if target == "" {
					target = "System"
				}
			}
			fmt.Fprintf(stdout, "  %-32s %s\n", EscapeTerminal(target), EscapeTerminal(att.Code))
			if att.Action != "" {
				fmt.Fprintf(stdout, "    Action: %s\n", EscapeTerminal(att.Action))
			}
		}
	}

	// Synced folders section
	if len(res.Items) > 0 {
		fmt.Fprintf(stdout, "\nSynced folders (%d):\n", len(res.Items))
		for _, folder := range res.Items {
			root := folder.Root
			if root == "" {
				root = "(no local root)"
			}
			statusSummary := "Ready"
			if res.Readiness != nil && !res.Readiness.Ready() {
				var parts []string
				if res.Readiness.Uncaptured > 0 {
					parts = append(parts, fmt.Sprintf("%d uncaptured", res.Readiness.Uncaptured))
				}
				if res.Readiness.MissingContent > 0 {
					parts = append(parts, fmt.Sprintf("%d missing content", res.Readiness.MissingContent))
				}
				if res.Readiness.PendingPublication > 0 {
					parts = append(parts, fmt.Sprintf("%d pending pub", res.Readiness.PendingPublication))
				}
				if res.Readiness.Conflicts > 0 {
					parts = append(parts, fmt.Sprintf("%d conflicts", res.Readiness.Conflicts))
				}
				if res.Readiness.StorageBlocked {
					parts = append(parts, "storage blocked")
				}
				if len(parts) > 0 {
					statusSummary = "Pending: " + strings.Join(parts, ", ")
				} else if !res.Readiness.RootAvailable {
					statusSummary = "Root unavailable"
				}
			}
			fmt.Fprintf(stdout, "  %-16s %-24s %s\n", EscapeTerminal(folder.Name), EscapeTerminal(root), statusSummary)
		}
	} else {
		fmt.Fprintln(stdout, "\nSynced folders: (none registered)")
	}

	// Observations section
	if len(res.Observations) > 0 {
		fmt.Fprintf(stdout, "\nDevice-copy observations (%d):\n", len(res.Observations))
		for _, obs := range res.Observations {
			devShort := obs.Device
			if len(devShort) > 12 {
				devShort = devShort[:12]
			}
			onlineStr := "offline"
			if obs.Online {
				onlineStr = "online"
			}
			directStr := "indirect"
			if obs.Direct {
				directStr = "direct"
			}
			lastContact := obs.LastContact
			if lastContact == "" {
				lastContact = "never"
			}
			authorShort := obs.Version.Author
			if len(authorShort) > 12 {
				authorShort = authorShort[:12]
			}
			fmt.Fprintf(stdout, "  Device %s: v=%s:%d saved=%v stored=%v applied=%v (%s, %s, %s, last_contact=%s)\n",
				devShort, authorShort, obs.Version.Counter,
				obs.Saved, obs.Stored, obs.Applied, directStr, onlineStr, obs.Availability, lastContact)
		}
	}
}
