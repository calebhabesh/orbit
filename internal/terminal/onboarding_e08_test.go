package terminal

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
)

const e08Folder = "ab00000000000000000000000000000000000000000000000000000000000000"

// e08Client serves one folder with a 10,000-entry directory, one file in each
// EG2 state and an unsafe name, paging with offset cursors like the daemon.
type e08Client struct {
	root    string
	queries []tc.Query
	attn    []tc.Attention
}

func (c *e08Client) Query(_ context.Context, q tc.Query) (tc.Result, error) {
	c.queries = append(c.queries, q)
	r := tc.Result{Version: tc.Version}
	switch q.Kind {
	case "folders":
		r.Items = []tc.NamedItem{{ID: e08Folder, Name: "Notes", Root: c.root}}
	case "attention":
		r.Attention = c.attn
	case "files":
		var all []tc.FileEntry
		switch {
		case q.Name != "":
			for i := 0; i < 10000; i++ {
				if p := fmt.Sprintf("big/f%05d.txt", i); strings.Contains(p, q.Name) {
					all = append(all, tc.FileEntry{Path: p, Name: p[4:], State: "captured"})
				}
			}
		case q.Path == "big":
			for i := 0; i < 10000; i++ {
				all = append(all, tc.FileEntry{Path: fmt.Sprintf("big/f%05d.txt", i), Name: fmt.Sprintf("f%05d.txt", i), Bytes: 10, State: "captured"})
			}
		case q.Path == "":
			all = []tc.FileEntry{{Path: "big", Name: "big", Directory: true, State: "captured"}}
			for _, s := range []string{"blocked", "captured", "content_missing", "conflict", "downloading", "waiting_publish"} {
				all = append(all, tc.FileEntry{Path: s + ".txt", Name: s + ".txt", Bytes: 2048, Modified: "2026-10-08T12:00:00Z", State: s})
			}
			all = append(all, tc.FileEntry{Path: "evil\x1b[31m\n.txt", Name: "evil\x1b[31m\n.txt", State: "captured"})
		}
		off, _ := strconv.Atoi(q.Cursor)
		end := min(len(all), off+int(q.Limit))
		r.Files = all[off:end]
		if end < len(all) {
			r.Cursor = strconv.Itoa(end)
		}
	case "file_details":
		old := time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339)
		r.File = &tc.FileDetail{Entry: tc.FileEntry{Path: q.Path, Name: q.Path, State: "captured"}, LastChecked: time.Now().UTC().Format(time.RFC3339),
			Heads:        []tc.VersionSummary{{DeviceName: "laptop", DisplayTime: "2026-10-08 12:00"}},
			Observations: []tc.Observation{{DeviceName: "pi", Device: "cd", Stored: true, Applied: true, ObservedAt: old}}}
	}
	return r, nil
}

// e08Run drives the model's query lane synchronously.
func e08Run(m *model, cmd tea.Cmd) {
	for i := 0; cmd != nil && i < 20; i++ {
		msg := cmd()
		switch msg := msg.(type) {
		case tea.BatchMsg:
			cmd = nil
			for _, c := range msg {
				if c != nil {
					if _, ok := c().(queryReply); ok {
						cmd = c
					}
				}
			}
			continue
		case queryReply:
			_, cmd = m.Update(msg)
		default:
			return
		}
	}
}

func e08Model(t *testing.T) (*model, *e08Client) {
	c := &e08Client{root: t.TempDir()}
	m := newModel(context.Background(), c, Options{Colorless: true})
	m.width, m.height = 140, 40
	e08Run(m, m.startQuery())
	return m, c
}

func e08Key(m *model, k string) {
	var msg tea.KeyPressMsg
	switch k {
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	case "left":
		msg = tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		msg = tea.KeyPressMsg{Code: tea.KeyRight}
	case "esc":
		msg = tea.KeyPressMsg{Code: tea.KeyEscape}
	case "backspace":
		msg = tea.KeyPressMsg{Code: tea.KeyBackspace}
	default:
		msg = tea.KeyPressMsg{Text: k, Code: []rune(k)[0]}
	}
	e08Run(m, m.key(msg))
}

func TestOnboardingE08LandsOnFilesWhenNothingNeedsAttention(t *testing.T) {
	m, _ := e08Model(t)
	if m.section != filesSection || !strings.Contains(m.View().Content, "Files: Notes") {
		t.Fatalf("landing section %d\n%s", m.section, m.View().Content)
	}
	view := m.View().Content
	for _, want := range []string{"0 attention items", "Saved here", "Conflict", "Downloading", "Arriving", "Content missing", "Blocked"} {
		if !strings.Contains(view, want) {
			t.Errorf("Files view lacks %q\n%s", want, view)
		}
	}
	if strings.Contains(view, "synced everywhere") || strings.Contains(view, "\x1b[31m") {
		t.Fatalf("global claim or raw escape in view\n%q", view)
	}
	if !strings.Contains(view, `evil\u001b[31m\u000a.txt`) {
		t.Fatalf("unsafe name not escaped\n%s", view)
	}

	c := &e08Client{root: t.TempDir(), attn: []tc.Attention{{ID: "x", Folder: e08Folder, Code: "SCAN_FAILED"}}}
	m2 := newModel(context.Background(), c, Options{Colorless: true})
	e08Run(m2, m2.startQuery())
	if m2.section != 0 {
		t.Fatalf("attention should keep Overview, got %d", m2.section)
	}
	e08Key(m2, "5")
	if m2.section != filesSection {
		t.Fatal("5 selects Files")
	}
}

func TestOnboardingE08NavigationPagingAndSearch(t *testing.T) {
	m, c := e08Model(t)
	e08Key(m, "enter") // big/
	if m.files.dir != "big" || len(m.result.Files) != pageSize || m.result.Files[0].Name != "f00000.txt" {
		t.Fatalf("enter dir: %+v %d", m.files, len(m.result.Files))
	}
	for i := 0; i < 499; i++ {
		e08Key(m, "]")
	}
	if got := m.result.Files[pageSize-1].Name; got != "f09999.txt" || m.result.Cursor != "" {
		t.Fatalf("last page ends with %q cursor %q", got, m.result.Cursor)
	}
	e08Key(m, "]") // no further page
	e08Key(m, "[")
	if m.result.Files[0].Name != "f09960.txt" {
		t.Fatalf("previous page starts %q", m.result.Files[0].Name)
	}
	for _, q := range c.queries {
		if q.Kind == "files" && q.Limit > pageSize {
			t.Fatalf("unbounded page %d", q.Limit)
		}
	}
	e08Key(m, "left")
	if m.files.dir != "" || m.selectedKey != "file:big" {
		t.Fatalf("up: %+v selected %q", m.files, m.selectedKey)
	}
	e08Key(m, "right")
	e08Key(m, "backspace")
	if m.files.dir != "" {
		t.Fatal("backspace goes up")
	}

	e08Key(m, "/")
	for _, r := range "f0999" {
		e08Key(m, string(r))
	}
	e08Key(m, "enter")
	if m.files.query != "f0999" || len(m.result.Files) != 10 || !strings.Contains(m.View().Content, "big/f09990.txt") {
		t.Fatalf("search: %+v %d", m.files, len(m.result.Files))
	}
	e08Key(m, "esc")
	if m.files.query != "" || m.files.dir != "" {
		t.Fatalf("esc clears search: %+v", m.files)
	}
}

func TestOnboardingE08DetailsShowReportAge(t *testing.T) {
	m, _ := e08Model(t)
	e08Key(m, "j")
	e08Key(m, "enter") // blocked.txt details
	view := m.View().Content
	for _, want := range []string{"State here: Blocked", "Other devices (their last report):", "pi: stored, in its folder · reported 3 h ago", "by laptop"} {
		if !strings.Contains(view, want) {
			t.Errorf("details lack %q\n%s", want, view)
		}
	}
	e08Key(m, "esc")
	if m.detail {
		t.Fatal("esc closes details")
	}
}

func TestOnboardingE08OpenCopyAndReadOnlyKeys(t *testing.T) {
	m, c := e08Model(t)
	var opened string
	m.opts.Open = func(p string) error { opened = p; return nil }
	e08Key(m, "j")
	e08Key(m, "o")
	if opened != c.root+"/blocked.txt" {
		t.Fatalf("opened %q", opened)
	}
	t.Setenv("TERM", "xterm-256color")
	if cmd := m.key(tea.KeyPressMsg{Text: "y", Code: 'y'}); cmd == nil || !strings.Contains(m.notice, "clipboard") {
		t.Fatalf("y copy: %q", m.notice)
	}
	m.opts.Editor = ""
	e08Key(m, "e")
	if !strings.Contains(m.notice, "EDITOR") {
		t.Fatalf("editor notice %q", m.notice)
	}
	// No key in the Files view sends a mutation: the query seam has none.
	for _, k := range []string{"d", "x", "r", "m"} {
		e08Key(m, k)
	}
	for _, q := range c.queries {
		switch q.Kind {
		case "folders", "files", "file_details", "attention", "devices", "network_status", "service":
		default:
			t.Fatalf("unexpected query %q", q.Kind)
		}
	}
}

func TestOnboardingE08ExplicitViewWinsInitialLanding(t *testing.T) {
	c := &e08Client{root: t.TempDir()}
	m := newModel(context.Background(), c, Options{Colorless: true})
	initial := m.startQuery()
	m.key(tea.KeyPressMsg{Text: "1", Code: '1'})
	e08Run(m, initial)
	if m.section != 0 {
		t.Fatal("initial response overrode explicit Overview selection")
	}
}

// Trial request: the left list is "Orbits"; the right pane previews the
// highlighted Orbit's files without Enter, and 5 opens that Orbit in Files.
func TestOnboardingTrialOrbitsListPreviewsFiles(t *testing.T) {
	m, c := e08Model(t)
	e08Key(m, "2")
	view := m.View().Content
	if !strings.Contains(view, "Orbits") || strings.Contains(view, "Folders") {
		t.Fatalf("list not titled Orbits:\n%s", view)
	}
	previewed := false
	for _, q := range c.queries {
		previewed = previewed || (q.Kind == "files" && q.Folder == e08Folder && q.Path == "")
	}
	if !previewed {
		t.Fatal("no files query for the highlighted Orbit")
	}
	for _, want := range []string{"Files", "big/", "conflict.txt", "Conflict", "Saved here", `evil\u001b[31m\u000a.txt`, "5 open in Files"} {
		if !strings.Contains(view, want) {
			t.Errorf("preview lacks %q\n%s", want, view)
		}
	}
	if strings.Contains(view, "\x1b[31m") {
		t.Fatal("raw escape in preview")
	}
	e08Key(m, "5")
	if m.section != filesSection || m.files.folder != e08Folder {
		t.Fatalf("5 did not open the highlighted Orbit: section %d folder %q", m.section, m.files.folder)
	}
	// Narrow terminals have no preview pane and issue no preview query.
	n, nc := e08Model(t)
	n.width = 80
	e08Key(n, "2")
	before := len(nc.queries)
	e08Run(n, n.startQuery())
	for _, q := range nc.queries[before:] {
		if q.Kind == "files" {
			t.Fatal("narrow layout fetched a preview")
		}
	}
}
