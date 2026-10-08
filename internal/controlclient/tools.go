package controlclient

import (
	"context"
	"errors"
	"fmt"
	"github.com/calebhabesh/orbit/internal/control"
	"io"
	"os/exec"
	"strings"
	"unicode"
)

// ToolArgv supports quoting/backslash escapes without interpreting expansions,
// redirections, pipelines or commands. Paths are appended as separate arguments.
func ToolArgv(command string) ([]string, error) {
	var args []string
	var b strings.Builder
	var quote rune
	escaped, started := false, false
	for _, r := range command {
		if escaped {
			b.WriteRune(r)
			escaped = false
			started = true
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			started = true
			continue
		}
		if unicode.IsSpace(r) {
			if started {
				args = append(args, b.String())
				b.Reset()
				started = false
			}
			continue
		}
		if r == 0 || r == '\n' || r == '\r' {
			return nil, errors.New("invalid tool command")
		}
		b.WriteRune(r)
		started = true
	}
	if escaped || quote != 0 {
		return nil, errors.New("unterminated tool quoting")
	}
	if started {
		args = append(args, b.String())
	}
	if len(args) == 0 || args[0] == "" {
		return nil, errors.New("tool executable required")
	}
	return args, nil
}
func ToolCommand(ctx context.Context, command string, paths ...string) (*exec.Cmd, error) {
	argv, err := ToolArgv(command)
	if err != nil {
		return nil, err
	}
	argv = append(argv, paths...)
	return exec.CommandContext(ctx, argv[0], argv[1:]...), nil
}
func RunTool(ctx context.Context, command string, stdin io.Reader, stdout, stderr io.Writer, paths ...string) error {
	cmd, err := ToolCommand(ctx, command, paths...)
	if err != nil {
		return err
	}
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// Linux prlimit sets the admitted maximum result file size before exec. It is
// a direct argv adapter; missing tooling leaves editor launch unavailable.
func RunLimitedTool(ctx context.Context, command string, limit uint64, stdin io.Reader, stdout, stderr io.Writer, paths ...string) error {
	cmd, err := LimitedToolCommand(ctx, command, limit, paths...)
	if err != nil {
		return err
	}
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// LimitedToolCommand lets a terminal owner yield to the same bounded argv
// adapter used by the CLI. Session paths and admission remain controller-owned.
func LimitedToolCommand(ctx context.Context, command string, limit uint64, paths ...string) (*exec.Cmd, error) {
	if limit == 0 || limit > 64<<30 {
		return nil, errors.New("invalid tool result size limit")
	}
	argv, err := ToolArgv(command)
	if err != nil {
		return nil, err
	}
	limiter, err := exec.LookPath("prlimit")
	if err != nil {
		return nil, &control.ControlError{Code: "UNSUPPORTED_CAPABILITY", Message: "prlimit is required for bounded editor results", Action: "install util-linux prlimit or supply --file"}
	}
	args := []string{fmt.Sprintf("--fsize=%d:%d", limit, limit), "--"}
	args = append(args, argv...)
	args = append(args, paths...)
	return exec.CommandContext(ctx, limiter, args...), nil
}
