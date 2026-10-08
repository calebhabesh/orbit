package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
)

// filesSection is the read-only Files view (E08). It lists what this replica
// records; renaming, moving, deleting and editing happen in the owner's tools.
const filesSection = 4

type filesState struct {
	folder, root, dir, query string
	page                     string   // cursor of the shown page; "" is the first
	back                     []string // cursors of earlier pages, for [
	detailPath               string
}

type filesDone struct {
	generation uint64
	err        error
	action     string
}

func fileState(s string) string { return tc.FileStateLabel(s) }

// filesQuery fills one Files page: the folder list (to pick a folder and its
// root), the directory or search page, the inspected file and the status
// counts shown above the list.
func filesQuery(ctx context.Context, client Queries, fs filesState, detail bool) (tc.Result, []tc.NamedItem, filesState, error) {
	r, err := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "folders", Limit: pageSize})
	if err != nil || r.Error != nil {
		return r, nil, fs, err
	}
	if fs.folder == "" && len(r.Items) == 1 {
		fs.folder = r.Items[0].ID
	}
	fs.root = ""
	for _, it := range r.Items {
		if it.ID == fs.folder {
			fs.root = it.Root
		}
	}
	if fs.root == "" {
		fs = filesState{}
	}
	if fs.folder != "" {
		q := tc.Query{Version: tc.Version, Kind: "files", Folder: fs.folder, Path: fs.dir, Name: fs.query, Limit: pageSize, Cursor: fs.page}
		if fs.query != "" {
			q.Path = ""
		}
		p, err := client.Query(ctx, q)
		var ce *control.ControlError
		stale := (p.Error != nil && p.Error.Code == "STALE_VIEW") || (errors.As(err, &ce) && ce.Code == "STALE_VIEW")
		if stale && q.Cursor != "" {
			fs.page, fs.back, q.Cursor = "", nil, ""
			p, err = client.Query(ctx, q)
		}
		if err != nil || p.Error != nil {
			return p, nil, fs, err
		}
		r.Files, r.Cursor = p.Files, p.Cursor
		if detail && fs.detailPath != "" {
			d, err := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "file_details", Folder: fs.folder, Path: fs.detailPath})
			if err != nil {
				return d, nil, fs, err
			}
			if d.Error == nil {
				r.File = d.File
			}
		}
	}
	var devices []tc.NamedItem
	if a, e := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "attention", Limit: pageSize}); e == nil && a.Error == nil {
		r.Attention = a.Attention
	}
	if d, e := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "devices", Limit: pageSize}); e == nil && d.Error == nil {
		devices = d.Items[:min(len(d.Items), pageSize)]
	}
	if n, e := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "network_status"}); e == nil && n.Error == nil {
		r.Network = n.Network
	}
	if s, e := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "service", Limit: pageSize}); e == nil && s.Error == nil {
		r.Service = s.Service
	}
	r.Files = r.Files[:min(len(r.Files), pageSize)]
	return r, devices, fs, nil
}

// fileRows lists folders until one is chosen, then the entries of a page.
func (m *model) fileRows() []row {
	if m.files.folder == "" {
		rows := make([]row, 0, len(m.result.Items))
		for _, it := range m.result.Items {
			rows = append(rows, row{key: "folder:" + it.ID, name: safe(it.Name) + "/", subtitle: homePath(safe(it.Root)), folder: it.ID, action: "Enter opens this folder."})
		}
		return rows
	}
	rows := make([]row, 0, len(m.result.Files))
	for _, f := range m.result.Files {
		name := safe(f.Name)
		if m.files.query != "" {
			name = safe(f.Path)
		}
		size, modified := "", ""
		if f.Directory {
			name += "/"
		} else {
			size = humanBytes(uint64(f.Bytes))
		}
		if t, err := time.Parse(time.RFC3339, f.Modified); err == nil {
			modified = t.Local().Format("2006-01-02 15:04")
		}
		rows = append(rows, row{key: "file:" + f.Path, name: name, subtitle: fileState(f.State), folder: m.files.folder, path: f.Path,
			cols: []string{size, modified}, dir: f.Directory})
	}
	return rows
}

func (m *model) selectedFile() (row, bool) {
	rows := m.rows()
	if len(rows) == 0 || m.files.folder == "" {
		return row{}, false
	}
	r := rows[min(m.selected, len(rows)-1)]
	return r, strings.HasPrefix(r.key, "file:")
}

func (m *model) filesReset(fs filesState) tea.Cmd {
	m.files = fs
	m.selected, m.selectedKey, m.detail = 0, "", false
	m.result.Files, m.result.File, m.result.Cursor = nil, nil, ""
	return m.invalidate()
}

// filesKey handles the Files view's own keys; it returns handled=false for
// keys the main screen owns (views, help, quit, setup actions).
func (m *model) filesKey(k string) (tea.Cmd, bool) {
	fs := m.files
	switch k {
	case "enter", "right", "l":
		rows := m.rows()
		if len(rows) == 0 {
			return nil, true
		}
		r := rows[min(m.selected, len(rows)-1)]
		switch {
		case strings.HasPrefix(r.key, "folder:"):
			return m.filesReset(filesState{folder: r.folder}), true
		case r.dir:
			return m.filesReset(filesState{folder: fs.folder, dir: r.path}), true
		case k == "enter":
			m.files.detailPath, m.detail = r.path, true
			m.result.File = nil
			return m.invalidate(), true
		}
		return nil, true
	case "left", "backspace", "esc":
		switch {
		case m.detail:
			m.detail, m.files.detailPath = false, ""
			return nil, true
		case fs.query != "":
			m.search.SetValue("")
			return m.filesReset(filesState{folder: fs.folder, dir: fs.dir}), true
		case fs.dir != "":
			parent := filepath.Dir(fs.dir)
			if parent == "." {
				parent = ""
			}
			cmd := m.filesReset(filesState{folder: fs.folder, dir: parent})
			m.selectedKey = "file:" + fs.dir
			return cmd, true
		case fs.folder != "" && len(m.result.Items) > 1:
			return m.filesReset(filesState{}), true
		}
		return nil, true
	case "/":
		if fs.folder == "" {
			return nil, true
		}
		m.focus = 1
		m.search.Placeholder = "part of a path in this folder"
		return m.search.Focus(), true
	case "]":
		if m.result.Cursor != "" {
			m.files.back = append(m.files.back, fs.page)
			m.files.page = m.result.Cursor
			m.selected, m.selectedKey = 0, ""
			return m.invalidate(), true
		}
		return nil, true
	case "[":
		if n := len(fs.back); n > 0 {
			m.files.page, m.files.back = fs.back[n-1], fs.back[:n-1]
			m.selected, m.selectedKey = 0, ""
			return m.invalidate(), true
		}
		return nil, true
	case "o", "e", "y":
		r, ok := m.selectedFile()
		if !ok {
			return nil, true
		}
		full := filepath.Join(m.files.root, r.path)
		switch k {
		case "o":
			return m.openDesktop(full), true
		case "e":
			if r.dir {
				m.notice = "e edits files; press Enter to open a directory."
				return nil, true
			}
			return m.openEditor(full), true
		}
		if !clipboardSupported() {
			m.notice = "Copy not supported here. Path: " + safe(full)
			return nil, true
		}
		m.notice = "Sent the path to the clipboard (OSC 52)."
		return tea.SetClipboard(full), true
	case "h":
		if r, ok := m.selectedFile(); ok && !r.dir {
			return m.everyday(fs.folder, "history", r.path), true
		}
		return nil, true
	case "c":
		if fs.folder != "" {
			return m.everyday(fs.folder, "conflicts", ""), true
		}
		return nil, true
	case "D":
		if fs.folder != "" {
			return m.everyday(fs.folder, "deleted", ""), true
		}
		return nil, true
	}
	return nil, false
}

// filesSearch submits the search field as a folder-wide path search.
func (m *model) filesSearch() tea.Cmd {
	return m.filesReset(filesState{folder: m.files.folder, dir: m.files.dir, query: strings.TrimSpace(m.search.Value())})
}

// openDesktop hands the path to the desktop's default application. Over SSH
// or on a headless host there is no desktop session, so it says so.
func (m *model) openDesktop(path string) tea.Cmd {
	open := m.opts.Open
	if open == nil {
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			m.notice = "No desktop session here; press e to open it in $EDITOR."
			return nil
		}
		open = xdgOpen
	}
	if err := open(path); err != nil {
		m.notice = "Could not open it: xdg-open is unavailable here."
		return nil
	}
	m.notice = "Opened with the desktop's default application."
	return nil
}

func xdgOpen(path string) error {
	cmd := exec.Command("xdg-open", path)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// openEditor suspends the interface for the owner's editor on the working
// copy itself; Orbit captures the saved file on its next scan.
func (m *model) openEditor(path string) tea.Cmd {
	if m.opts.Editor == "" {
		m.notice = "No editor configured; set EDITOR or pass --editor."
		return nil
	}
	argv, err := controlclient.ToolArgv(m.opts.Editor)
	if err != nil || len(argv) == 0 {
		m.notice = "The configured editor command could not be read."
		return nil
	}
	cmd := exec.CommandContext(m.ctx, argv[0], append(argv[1:], path)...)
	m.toolRunning = true
	m.generation++
	if m.cancel != nil {
		m.cancel()
		m.dirty = true
	}
	generation := m.generation
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return filesDone{generation: generation, err: err, action: "editor"} })
}

func (m *model) fileDetailLines() (string, []string) {
	r, ok := m.selectedFile()
	if !ok {
		if m.files.folder == "" {
			return "Files", []string{"Choose a folder and press Enter.", "", "Files shows what this device has; other devices' reports are in each file's details."}
		}
		return "Files", []string{"This folder is empty here.", "Files appear after they are captured or arrive from another device."}
	}
	lines := []string{"Path: " + safe(r.path)}
	state := ""
	for _, f := range m.result.Files {
		if f.Path == r.path {
			state = f.State
			if f.Reason != "" {
				lines = append(lines, "Reason: "+safe(f.Reason))
			}
		}
	}
	if l := fileState(state); l != "" {
		lines = append(lines, "State here: "+l, tc.FileStateHelp(state))
	}
	if r.cols != nil && r.cols[0] != "" {
		lines = append(lines, "Size: "+r.cols[0])
	}
	if r.cols != nil && r.cols[1] != "" {
		lines = append(lines, "Modified: "+r.cols[1])
	}
	d := m.result.File
	if !m.detail || d == nil || d.Entry.Path != r.path {
		hint := "Enter: versions and other devices · o open · e edit · h history · y copy path"
		if r.dir {
			hint = "Enter or →: open directory · o open · y copy path"
		}
		return "Details", append(lines, "", hint)
	}
	now := time.Now()
	if d.LastChecked != "" {
		lines = append(lines, "Last checked here: "+ago(d.LastChecked, now))
	}
	if len(d.Heads) > 0 {
		lines = append(lines, "", "Current version:")
		if len(d.Heads) > 1 {
			lines[len(lines)-1] = fmt.Sprintf("Current versions (%d, in conflict):", len(d.Heads))
		}
		for _, h := range d.Heads {
			who := safe(h.DeviceName)
			if who == "" {
				who = "device " + safe(h.Version.Author[:min(8, len(h.Version.Author))])
			}
			lines = append(lines, "  by "+who+" · "+safe(h.DisplayTime))
		}
	}
	lines = append(lines, "", "Other devices (their last report):")
	if len(d.Observations) == 0 {
		lines = append(lines, "  No report from another device about this version yet.")
	}
	for _, o := range d.Observations {
		who := safe(o.DeviceName)
		if who == "" {
			who = "device " + safe(o.Device[:min(8, len(o.Device))])
		}
		var has []string
		if o.Stored {
			has = append(has, "stored")
		}
		if o.Applied {
			has = append(has, "in its folder")
		}
		what := "not stored yet"
		if len(has) > 0 {
			what = strings.Join(has, ", ")
		}
		when := "time unknown"
		if o.ObservedAt != "" {
			when = "reported " + ago(o.ObservedAt, now)
		}
		lines = append(lines, "  "+who+": "+what+" · "+when)
	}
	return "Details", append(lines, "", "h history · c conflicts · o open · e edit · Esc back")
}

// ago states the age of a report so that an old one never reads as current.
func ago(stamp string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return "time unknown"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d/time.Minute))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h ago", int(d/time.Hour))
	}
	return fmt.Sprintf("%d days ago", int(d/(24*time.Hour)))
}

func (m *model) filesTitle() string {
	if m.files.folder == "" {
		return "Files"
	}
	name := ""
	for _, it := range m.result.Items {
		if it.ID == m.files.folder {
			name = safe(it.Name)
		}
	}
	switch {
	case m.files.query != "":
		return "Files: " + name + " · search “" + safe(m.files.query) + "”"
	case m.files.dir != "":
		return "Files: " + name + "/" + safe(m.files.dir)
	}
	return "Files: " + name
}
