package terminal

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/charmbracelet/x/ansi"
)

type queryFunc func(context.Context, tc.Query) (tc.Result, error)

func (f queryFunc) Query(c context.Context, q tc.Query) (tc.Result, error) { return f(c, q) }
func press(m *model, text string) tea.Cmd {
	msg := tea.KeyPressMsg{Text: text, Code: []rune(text)[0]}
	return m.key(msg)
}
func shellFixture() *model {
	m := newModel(context.Background(), queryFunc(func(context.Context, tc.Query) (tc.Result, error) { return tc.Result{}, nil }), Options{Colorless: true})
	m.result = tc.Result{Items: []tc.NamedItem{{ID: strings.Repeat("a", 64), Name: "Notes界", Root: "/private/notes"}, {ID: strings.Repeat("b", 64), Name: "Photos", Root: "/private/photos"}}, Service: &tc.Service{Running: true, Mode: "manual"}}
	m.restoreSelection()
	return m
}

func TestTerminalT09FocusedInputAndKeyboard(t *testing.T) {
	m := shellFixture()
	press(m, "j")
	if m.selected != 1 {
		t.Fatal("j navigation")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.selected != 0 {
		t.Fatal("arrow navigation")
	}
	press(m, "/")
	for _, s := range []string{"j", "k", "q", "?"} {
		press(m, s)
	}
	if m.search.Value() != "jkq?" || m.quitting || m.help {
		t.Fatal("text stole global shortcuts")
	}
	m.Update(tea.PasteMsg{Content: strings.Repeat("界", 10000)})
	if len([]rune(m.search.Value())) > 256 {
		t.Fatal("unbounded draft")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.search.Focused() {
		t.Fatal("escape search")
	}
	press(m, "?")
	if !strings.Contains(m.View().Content, "Keyboard help") {
		t.Fatal("help")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	// E03: Tab moves between views (not into search); number keys pick one.
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.search.Focused() || m.section != 1 {
		t.Fatalf("tab: search focused %v, section %d", m.search.Focused(), m.section)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	press(m, "3")
	if m.section != 2 {
		t.Fatalf("3 opened section %d", m.section)
	}
}

func TestTopLevelArrowsCycleThroughAllFiveViews(t *testing.T) {
	for _, direction := range []struct {
		name string
		code rune
		step int
	}{{"right", tea.KeyRight, 1}, {"left", tea.KeyLeft, -1}} {
		t.Run(direction.name, func(t *testing.T) {
			m := shellFixture()
			m.firstLoad = true
			for i := 0; i < 2*len(sections); i++ {
				want := (m.section + direction.step + len(sections)) % len(sections)
				m.Update(tea.KeyPressMsg{Code: direction.code})
				if m.section != want {
					t.Fatalf("%s at step %d: selected tab %d, want %d", direction.name, i+1, m.section+1, want+1)
				}
			}
		})
	}
}

func TestTerminalT09CorrelatedCancellationAndBoundedPolling(t *testing.T) {
	started := make(chan tc.Query, 1)
	m := shellFixture()
	m.client = queryFunc(func(ctx context.Context, q tc.Query) (tc.Result, error) {
		started <- q
		<-ctx.Done()
		return tc.Result{}, ctx.Err()
	})
	m.section = 1
	cmd := m.startQuery()
	reply := make(chan tea.Msg, 1)
	go func() { reply <- cmd() }()
	select {
	case q := <-started:
		if q.Limit != 20 {
			t.Fatal("unbounded request")
		}
	case <-time.After(time.Second):
		t.Fatal("not started")
	}
	request := m.pending
	for i := 0; i < 100; i++ {
		m.Update(refreshMsg{})
		if m.pending != request {
			t.Fatal("poll spawned extra command")
		}
	}
	// Abandon folder inspection while its request is still outstanding.
	m.detailRow = row{folder: strings.Repeat("b", 64)}
	if m.changeSection(2) != nil {
		t.Fatal("parallel lane on navigation")
	}
	select {
	case msg := <-reply:
		_, next := m.Update(msg)
		if next == nil || m.pending == request {
			t.Fatal("latest context not scheduled")
		}
	case <-time.After(time.Second):
		t.Fatal("query not canceled")
	}
	current := m.pending
	m.Update(queryReply{request: request, generation: m.generation - 1, result: tc.Result{Items: []tc.NamedItem{{Name: "old folder"}}}})
	if m.pending != current || len(m.result.Items) != 0 {
		t.Fatal("stale reply changed context")
	}
	m.quit()
	if !m.quitting {
		t.Fatal("quit")
	}
	// No mutation/service operation exists on the query seam.
}

func TestTerminalT09RefreshPreservesSelectionAndDraft(t *testing.T) {
	m := shellFixture()
	m.selectRow(1)
	m.search.SetValue("Photo")
	m.search.Focus()
	m.pending = 9
	m.Update(queryReply{request: 9, generation: m.generation, result: tc.Result{Items: []tc.NamedItem{{ID: strings.Repeat("b", 64), Name: "Photos renamed"}, {ID: strings.Repeat("a", 64), Name: "Notes"}}}})
	if m.selectedKey != strings.Repeat("b", 64) || m.search.Value() != "Photo" || !m.search.Focused() {
		t.Fatal("refresh lost state")
	}
	// A reply for an abandoned form/detail is rejected even with the current lane ID.
	m.pending = 10
	m.Update(queryReply{request: 10, generation: m.generation - 1, result: tc.Result{Items: []tc.NamedItem{{Name: "alien"}}}})
	if m.result.Items[0].Name == "alien" {
		t.Fatal("late detail response accepted")
	}
}

func TestTerminalT09UnicodeEscapingAndLayout(t *testing.T) {
	m := shellFixture()
	for i := 0; i < 20; i++ {
		m.result.Items = append(m.result.Items, tc.NamedItem{ID: string(rune('c' + i)), Name: strings.Repeat("界e\u0301", 70) + "\x1b[2J\n\u202e", Root: "/long"})
	}
	m.selectRow(21)
	for _, size := range [][2]int{{80, 24}, {40, 16}, {20, 10}, {1, 1}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := m.View()
		if strings.ContainsAny(view.Content, "\x1b\u202e") {
			t.Fatal("display injection")
		}
		lines := strings.Split(view.Content, "\n")
		if len(lines) > size[1] {
			t.Fatal("height overflow")
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("width overflow %d: %q", size[0], line)
			}
		}
		if size[0] > 1 && !strings.Contains(view.Content, ">") {
			t.Fatal("selected row hidden")
		}
	}
	if !strings.Contains(safe("a\x1b[2J\n"), `\u001b`) {
		t.Fatal("escape control")
	}
}

func TestTerminalT09ToolAdapterAndNonTTY(t *testing.T) {
	m := shellFixture()
	m.opts.Tool = &Tool{Command: `editor --flag 'literal $(touch NEVER)'`, Paths: []string{"/scratch/a b"}, MaxBytes: 1 << 20}
	if m.launchTool() == nil || !m.toolRunning {
		t.Fatal("tool adapter not available")
	}
	gen := m.generation
	m.notice = "pending"
	m.Update(toolReply{generation: gen - 1})
	if m.notice != "pending" {
		t.Fatal("stale tool reply changed view")
	}
	m.Update(toolReply{generation: gen})
	if !strings.Contains(m.notice, "resumed") {
		t.Fatal("tool resume")
	}
	if err := Run(context.Background(), m.client, Options{}); err == nil {
		t.Fatal("nonTTY reached renderer")
	}
}
