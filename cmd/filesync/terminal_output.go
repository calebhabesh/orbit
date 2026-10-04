package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
)

// EscapeTerminal sanitizes strings before printing to a terminal emulator, preventing
// ANSI escape injection, cursor repositioning, or terminal corruption from user-controlled names.
func EscapeTerminal(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\x1b':
			b.WriteString(`\x1b`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) || unicode.Is(unicode.C, r):
			fmt.Fprintf(&b, "\\u%04x", r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// RenderResult renders a tc.Result in either machine JSON format or human terminal format,
// ensuring machine output is strictly isolated to stdout and exit codes match T01 contracts.
func RenderResult(stdout, stderr io.Writer, r tc.Result, asJSON bool) int {
	exitCode := tc.ExitCode(r)
	if asJSON {
		_ = json.NewEncoder(stdout).Encode(r)
		return exitCode
	}

	if r.Error != nil {
		fmt.Fprintf(stderr, "Error [%s]: %s\n", r.Error.Code, EscapeTerminal(r.Error.Message))
		if r.Error.Action != "" {
			fmt.Fprintf(stderr, "Action: %s\n", EscapeTerminal(r.Error.Action))
		}
		if len(r.Items) > 0 {
			fmt.Fprintln(stderr, "\nAvailable candidates:")
			for _, item := range r.Items {
				shortID := item.ID
				if len(shortID) > 12 {
					shortID = shortID[:12]
				}
				if item.Root != "" {
					fmt.Fprintf(stderr, "  %s (%s) -> %s\n", EscapeTerminal(item.Name), shortID, EscapeTerminal(item.Root))
				} else {
					fmt.Fprintf(stderr, "  %s (%s)\n", EscapeTerminal(item.Name), shortID)
				}
			}
		}
		return exitCode
	}

	if r.Context != nil {
		fmt.Fprintf(stdout, "Folder:        %s (%s)\n", EscapeTerminal(r.Context.FolderName), r.Context.Folder)
		fmt.Fprintf(stdout, "Root:          %s\n", EscapeTerminal(r.Context.Root))
		if r.Context.Path != "" {
			fmt.Fprintf(stdout, "Relative Path: %s\n", EscapeTerminal(r.Context.Path))
		}
		fmt.Fprintf(stdout, "Generation:    %s\n", r.Context.Generation)
		return exitCode
	}

	if len(r.Items) > 0 {
		for _, item := range r.Items {
			if item.Root != "" {
				fmt.Fprintf(stdout, "  %-20s %s  (%s)\n", EscapeTerminal(item.Name), EscapeTerminal(item.Root), item.ID)
			} else {
				pinInfo := ""
				if item.KeyPin != "" {
					pin := item.KeyPin
					if len(pin) > 12 {
						pin = pin[:12]
					}
					pinInfo = " pin=" + pin
				}
				fmt.Fprintf(stdout, "  %-20s %s%s\n", EscapeTerminal(item.Name), item.ID, pinInfo)
			}
		}
		return exitCode
	}

	if r.Service != nil {
		fmt.Fprintf(stdout, "Service: running=%v enabled=%v mode=%s unattended=%v root_healthy=%v capture_healthy=%v\n",
			r.Service.Running, r.Service.Enabled, r.Service.Mode, r.Service.UnattendedVerified, r.Service.RootHealthy, r.Service.CaptureHealthy)
		return exitCode
	}

	if r.Operation != nil {
		fmt.Fprintf(stdout, "Operation: %s (kind=%s, state=%s, phase=%s)\n",
			r.Operation.ID, r.Operation.Kind, r.Operation.State, r.Operation.Phase)
		return exitCode
	}

	if len(r.Attention) > 0 {
		fmt.Fprintf(stdout, "Needs attention (%d):\n", len(r.Attention))
		for _, att := range r.Attention {
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
		return exitCode
	}

	if len(r.Observations) > 0 {
		fmt.Fprintf(stdout, "Device-copy observations (%d):\n", len(r.Observations))
		for _, obs := range r.Observations {
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
			authorShort := obs.Version.Author
			if len(authorShort) > 12 {
				authorShort = authorShort[:12]
			}
			fmt.Fprintf(stdout, "  Device %s: v=%s:%d saved=%v stored=%v applied=%v (%s, %s, %s)\n",
				devShort, authorShort, obs.Version.Counter,
				obs.Saved, obs.Stored, obs.Applied, directStr, onlineStr, obs.Availability)
		}
		return exitCode
	}

	if r.State != "" {
		fmt.Fprintf(stdout, "State: %s\n", r.State)
	}

	return exitCode
}

// RenderError converts an error into a structured Result or human output with matching exit code.
func RenderError(stderr io.Writer, err error, asJSON bool) int {
	var ce *control.ControlError
	if errors.As(err, &ce) {
		r := tc.Result{
			Version: tc.Version,
			State:   "failed",
			Error: &tc.Error{
				Code:      ce.Code,
				Message:   ce.Message,
				Retryable: ce.Retryable,
				Action:    ce.Action,
			},
		}
		return RenderResult(os.Stdout, stderr, r, asJSON)
	}
	r := tc.Result{
		Version: tc.Version,
		State:   "failed",
		Error: &tc.Error{
			Code:    "INTERNAL_ERROR",
			Message: err.Error(),
			Action:  "inspect error logs and retry operation",
		},
	}
	return RenderResult(os.Stdout, stderr, r, asJSON)
}

// CLIExitError communicates a specific exit code to the CLI entry point.
type CLIExitError struct {
	Code int
	Err  error
}

func (e *CLIExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit code %d", e.Code)
}

func (e *CLIExitError) ExitCode() int {
	return e.Code
}
