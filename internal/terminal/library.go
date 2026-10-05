// Package terminal owns the sole interactive input/render loop. Engine and
// shared-client interfaces contain no Charm types.
package terminal

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"
)

// Queries is deliberately sufficient for shell navigation. Workflow screens
// may use the larger terminalcontract.Client without redefining its operations.
type Queries interface {
	Query(context.Context, tc.Query) (tc.Result, error)
}

// Tool is an explicitly configured owner program. T11 supplies admitted session
// paths; the development shell can exercise terminal yielding on scratch files.
type Tool struct {
	Command  string
	Paths    []string
	MaxBytes uint64
}

type Options struct {
	Editor    string
	Diff      string
	Input     *os.File
	Output    io.Writer
	Tool      *Tool
	Colorless bool
	Refresh   time.Duration
}

func Interactive(input *os.File, output io.Writer) bool {
	f, ok := output.(*os.File)
	return input != nil && ok && term.IsTerminal(input.Fd()) && term.IsTerminal(f.Fd())
}

// Run never launches or stops a daemon. Non-TTY rendering is owned by the CLI.
func Run(ctx context.Context, client Queries, opts Options) error {
	if !Interactive(opts.Input, opts.Output) {
		return errors.New("interactive terminal input and output required")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	m := newModel(ctx, client, opts)
	// The CLI owns process signals and cancels this context, including active
	// tools. Avoid two signal handlers racing an unbuffered QuitMsg against
	// context shutdown (Bubble Tea v2.0.10 handleSignals).
	programOptions := []tea.ProgramOption{tea.WithContext(ctx), tea.WithoutSignalHandler(), tea.WithInput(opts.Input), tea.WithOutput(opts.Output), tea.WithFPS(30)}
	if opts.Colorless {
		programOptions = append(programOptions, tea.WithColorProfile(colorprofile.NoTTY))
	}
	_, err := tea.NewProgram(m, programOptions...).Run()
	return err
}
