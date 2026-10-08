package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
	"github.com/calebhabesh/orbit/internal/history"
	"github.com/calebhabesh/orbit/internal/launcher"
)

// handleOrbitFiles is the CLI form of the read-only Files view (E08): the same
// files and file_details queries, so both show the same entries and states.
func handleOrbitFiles(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("orbit files", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateFlag := flags.String("state", "", "agent state directory")
	folderFlag := flags.String("folder", "", "folder name or ID")
	searchFlag := flags.String("search", "", "list known paths containing this text")
	limitFlag := flags.Uint("limit", 50, "page size (1-200)")
	cursorFlag := flags.String("cursor", "", "cursor printed by the previous page")
	jsonFlag := flags.Bool("json", false, "structured JSON")
	path, flagArgs := parsePositionalFolder(args, "state", "folder", "search", "limit", "cursor")
	if err := flags.Parse(flagArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *limitFlag < 1 || *limitFlag > tc.MaxPage {
		return fmt.Errorf("--limit must be 1-%d", tc.MaxPage)
	}
	dir, err := launcher.DiscoverState(*stateFlag)
	if err != nil {
		return err
	}
	client := &controlclient.Client{StateDir: dir}
	ctx := context.Background()
	fail := func(err error) error { return &CLIExitError{Code: RenderError(stderr, err, *jsonFlag)} }
	resolved, err := ResolveContext(ctx, client, ContextOptions{StateDir: dir, Folder: *folderFlag, AllowEmptyPath: true})
	if err != nil {
		return fail(err)
	}
	if e := resolved.Result.Error; e != nil && e.Code == "AMBIGUOUS_CONTEXT" && *folderFlag == "" && len(resolved.Result.Items) == 1 {
		// Outside every synced folder with only one folder there is nothing
		// to choose between.
		only := resolved.Result.Items[0]
		resolved = &ResolvedContext{Folder: only.ID, FolderName: only.Name, Root: only.Root}
	}
	if resolved.Result.Error != nil {
		return &CLIExitError{Code: RenderResult(stdout, stderr, resolved.Result, *jsonFlag)}
	}
	rel, err := filesPath(resolved.Root, path)
	if err != nil {
		return fail(err)
	}
	q := tc.Query{Version: tc.Version, Kind: "files", Folder: resolved.Folder, Path: rel, Name: *searchFlag, Limit: tc.Uint(*limitFlag), Cursor: *cursorFlag}
	if *searchFlag != "" {
		q.Path = ""
	}
	r, err := client.Query(ctx, q)
	var ce *control.ControlError
	notFound := (err == nil && r.Error != nil && r.Error.Code == "NOT_FOUND") || (errors.As(err, &ce) && ce.Code == "NOT_FOUND")
	if notFound && rel != "" && *searchFlag == "" {
		// Not a directory: show the file's details instead.
		r, err = client.Query(ctx, tc.Query{Version: tc.Version, Kind: "file_details", Folder: resolved.Folder, Path: rel})
	}
	if err != nil {
		return fail(err)
	}
	if r.Error != nil {
		return &CLIExitError{Code: RenderResult(stdout, stderr, r, *jsonFlag)}
	}
	if *jsonFlag {
		return json.NewEncoder(stdout).Encode(r)
	}
	if r.File != nil {
		renderFileDetail(stdout, r.File, time.Now())
		return nil
	}
	w := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSIZE\tMODIFIED\tSTATE HERE")
	for _, f := range r.Files {
		name := f.Name
		if *searchFlag != "" {
			name = f.Path
		}
		size := ""
		if f.Directory {
			name += "/"
		} else {
			size = fmt.Sprintf("%d", f.Bytes)
		}
		modified := ""
		if t, err := time.Parse(time.RFC3339, f.Modified); err == nil {
			modified = t.Local().Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", EscapeTerminal(name), size, modified, tc.FileStateLabel(f.State))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if len(r.Files) == 0 {
		fmt.Fprintln(stdout, "(nothing here)")
	}
	if r.Cursor != "" {
		fmt.Fprintf(stdout, "More: orbit files %s--cursor %s\n", cursorArgs(path, *folderFlag, *searchFlag), r.Cursor)
	}
	return nil
}

// filesPath maps the argument to a folder-relative path: an absolute path or
// one relative to a working directory inside the root resolves on disk;
// otherwise it is taken relative to the folder root.
func filesPath(root, arg string) (string, error) {
	if arg == "" || arg == "." && !insideRoot(root) {
		return "", nil
	}
	target := arg
	if !filepath.IsAbs(arg) {
		if !insideRoot(root) {
			target = filepath.Join(root, arg)
		} else {
			cwd, _ := os.Getwd()
			target = filepath.Join(cwd, arg)
		}
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", errors.New("INVALID_PATH: the path is outside the synced folder")
	}
	if rel == "." {
		return "", nil
	}
	rel = filepath.ToSlash(rel)
	if err := history.ValidatePath(rel); err != nil {
		return "", errors.New("INVALID_PATH: " + err.Error())
	}
	return rel, nil
}

func insideRoot(root string) bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(root), cwd)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

func cursorArgs(path, folder, search string) string {
	var b strings.Builder
	if path != "" {
		b.WriteString(shellQuote(path) + " ")
	}
	if folder != "" {
		b.WriteString("--folder " + shellQuote(folder) + " ")
	}
	if search != "" {
		b.WriteString("--search " + shellQuote(search) + " ")
	}
	return b.String()
}

func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r == '/' || r == '.' || r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z')
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(EscapeTerminal(s), "'", `'\''`) + "'"
}

func renderFileDetail(w io.Writer, d *tc.FileDetail, now time.Time) {
	fmt.Fprintf(w, "Path: %s\n", EscapeTerminal(d.Entry.Path))
	if l := tc.FileStateLabel(d.Entry.State); l != "" {
		fmt.Fprintf(w, "State here: %s. %s\n", l, tc.FileStateHelp(d.Entry.State))
	}
	if d.Entry.Reason != "" {
		fmt.Fprintf(w, "Reason: %s\n", EscapeTerminal(d.Entry.Reason))
	}
	if !d.Entry.Directory {
		fmt.Fprintf(w, "Size: %d bytes\n", d.Entry.Bytes)
	}
	if d.LastChecked != "" {
		fmt.Fprintf(w, "Last checked here: %s\n", d.LastChecked)
	}
	for _, h := range d.Heads {
		fmt.Fprintf(w, "Version %s:%d by %s at %s\n", h.Version.Author[:min(16, len(h.Version.Author))], h.Version.Counter, EscapeTerminal(h.DeviceName), EscapeTerminal(h.DisplayTime))
	}
	fmt.Fprintln(w, "Other devices (their last report):")
	if len(d.Observations) == 0 {
		fmt.Fprintln(w, "  no report from another device about this version yet")
	}
	for _, o := range d.Observations {
		when := "time unknown"
		if t, err := time.Parse(time.RFC3339, o.ObservedAt); err == nil {
			when = fmt.Sprintf("reported %s (%s ago)", o.ObservedAt, now.Sub(t).Round(time.Second))
		}
		fmt.Fprintf(w, "  %s: stored=%t applied=%t, %s\n", EscapeTerminal(o.DeviceName), o.Stored, o.Applied, when)
	}
}
