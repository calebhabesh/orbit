package terminal

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// F05: the panel never renders (and so never truncates) the code; it shows
// the character count and the copy, show and save actions at any width.
func TestOnboardingE05F05PanelShowsCountNeverCode(t *testing.T) {
	inv, code := e00Invitation()
	for _, size := range [][2]int{{80, 24}, {200, 50}} {
		m, _ := workflowModel()
		m.width, m.height = size[0], size[1]
		m.flow = &workflow{screen: "invitation_out", invitation: inv}
		view := ansi.Strip(m.View().Content)
		if strings.Contains(view, code[20:60]) || strings.Contains(view, "…") {
			t.Fatalf("%dx%d: panel shows (part of) the code or an ellipsis", size[0], size[1])
		}
		if !strings.Contains(view, humanCount(len(code))+" characters") {
			t.Fatalf("%dx%d: no character count", size[0], size[1])
		}
	}
}

// F05: v writes the code to the ordinary screen as one unbroken line between
// a header and the return prompt, then clears screen and scrollback.
func TestOnboardingE05F05RevealWritesOneLine(t *testing.T) {
	_, code := e00Invitation()
	var out bytes.Buffer
	r := &plainReveal{code: code}
	r.SetStdin(strings.NewReader("\n"))
	r.SetStdout(&out)
	if err := r.Run(); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	i := strings.Index(text, code)
	if i < 0 {
		t.Fatal("code not written contiguously")
	}
	if before := text[:i]; !strings.HasSuffix(before, "\r\n\r\n") {
		t.Fatalf("code does not start its own line: %q", before[len(before)-10:])
	}
	if after := text[i+len(code):]; !strings.HasPrefix(after, "\r\n") || !strings.HasSuffix(after, "\x1b[2J\x1b[3J\x1b[H") {
		t.Fatal("code not followed by a line end, or screen not cleared afterwards")
	}
}

// F05: c sends OSC 52 (bounded) or says copying is unsupported; s prefills a
// private default path in the state directory.
func TestOnboardingE05F05CopyAndSaveDefault(t *testing.T) {
	inv, code := e00Invitation()
	m, _ := workflowModel()
	m.opts.StateDir = "/private/state"
	m.flow = &workflow{screen: "invitation_out", invitation: inv}
	t.Setenv("TERM", "xterm-256color")
	cmd := m.key(tea.KeyPressMsg{Text: "c", Code: 'c'})
	if cmd == nil || !strings.Contains(m.flow.notice, humanCount(len(code))+" characters to the clipboard") {
		t.Fatalf("copy: cmd %v notice %q", cmd != nil, m.flow.notice)
	}
	t.Setenv("TERM", "dumb")
	if cmd = m.key(tea.KeyPressMsg{Text: "c", Code: 'c'}); cmd != nil || !strings.Contains(m.flow.notice, "Copy not supported here") {
		t.Fatalf("unsupported copy: %q", m.flow.notice)
	}
	if len(code) > maxClipboardCode {
		t.Fatal("fixture exceeds the clipboard bound")
	}
	m.key(tea.KeyPressMsg{Text: "s", Code: 's'})
	path := m.flow.fields[0].input.Value()
	if filepath.Dir(path) != "/private/state" || !strings.HasPrefix(filepath.Base(path), "invitation-ffffffff-") || !strings.HasSuffix(path, ".json") {
		t.Fatalf("default save path %q", path)
	}
}
