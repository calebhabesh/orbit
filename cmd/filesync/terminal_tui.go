package main

import (
	"context"
	"errors"
	"flag"
	"github.com/calebhabesh/file-sync/internal/app"
	"github.com/calebhabesh/file-sync/internal/launcher"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/calebhabesh/file-sync/internal/config"
	"github.com/calebhabesh/file-sync/internal/controlclient"
	"github.com/calebhabesh/file-sync/internal/terminal"
)

// The explicit development entry remains separate from bare Orbit until T12.
func handleOrbitTUI(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("orbit tui", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state", config.DefaultStateDir(), "agent state directory")
	plain := flags.Bool("no-color", false, "use text focus and status without color")
	jsonOutput := flags.Bool("json", false, "structured status output without an interactive screen")
	tool := flags.String("tool", "", "explicit trusted external tool command for development")
	toolFile := flags.String("tool-file", "", "scratch file passed as a separate argument to the tool")
	toolLimit := flags.Uint64("tool-limit", 1<<20, "maximum tool result size in bytes")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("tui accepts no positional arguments")
	}
	if *toolFile != "" && *tool == "" {
		return errors.New("--tool-file requires --tool")
	}
	if !terminal.Interactive(os.Stdin, stdout) || *jsonOutput {
		statusArgs := []string{"--state", *stateDir}
		if *jsonOutput {
			statusArgs = append(statusArgs, "--json")
		}
		return handleOrbitStatus(statusArgs, stdout, stderr)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	opts := terminal.Options{Input: os.Stdin, Output: stdout, Colorless: *plain || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"}
	if *tool != "" {
		opts.Tool = &terminal.Tool{Command: *tool, MaxBytes: *toolLimit}
		if *toolFile != "" {
			opts.Tool.Paths = []string{*toolFile}
		}
	}
	dir, err := launcher.DiscoverState(*stateDir)
	if err != nil {
		return err
	}
	if _, err = launcher.EnsureDaemon(ctx, launcher.LaunchOptions{StateDir: dir, NoBrowser: true}); err != nil {
		return err
	}
	client := &controlclient.Client{StateDir: dir, RestartDaemon: func(ctx context.Context) error {
		if err := app.StopAgent(dir, 5*time.Second); err != nil {
			return err
		}
		_, err := launcher.EnsureDaemon(ctx, launcher.LaunchOptions{StateDir: dir, NoBrowser: true})
		return err
	}}
	return terminal.Run(ctx, client, opts)
}
