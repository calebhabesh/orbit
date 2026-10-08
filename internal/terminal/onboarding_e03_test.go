package terminal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// F06: a paste into the secret invitation field replaces its content and
// reports how much was received without revealing it.
func TestOnboardingE03F06SecretPasteReplacesAndReportsLength(t *testing.T) {
	_, code := e00Invitation()
	m, _ := workflowModel()
	m.flow = &workflow{screen: "invitation", fields: []field{newField("Private invitation", "", true)}}
	m.Update(tea.PasteMsg{Content: code[:800]}) // first, incomplete attempt
	m.Update(tea.PasteMsg{Content: code})       // retry with the whole code
	got := m.flow.fields[0].input.Value()
	if got != code {
		t.Errorf("second paste produced %d characters, want exactly the pasted %d (approved: paste replaces)", len(got), len(code))
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "characters") {
		t.Error("secret field shows no received-length status line")
	}
	if strings.Contains(view, code[20:60]) {
		t.Fatal("secret revealed")
	}
}

// F06: Ctrl-U clears the hidden field; Enter on an empty field submits
// nothing; after a failed attempt the next typed character starts over.
func TestOnboardingE03F06ClearEmptyEnterAndRetypeReplace(t *testing.T) {
	_, code := e00Invitation()
	m, w := workflowModel()
	m.flow = &workflow{screen: "invitation", fields: []field{newField("Private invitation", "", true)}}
	m.Update(tea.PasteMsg{Content: code})
	m.key(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	if v := m.flow.fields[0].input.Value(); v != "" {
		t.Fatalf("Ctrl-U left %d characters", len(v))
	}
	if cmd := m.key(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil || m.flow.err == "" || w.calls != 0 {
		t.Fatal("empty Enter submitted")
	}
	m.flow.fields[0].input.SetValue("stale")
	m.flow.fields[0].input.Focus()
	m.flow.replaceOnType = true
	press(m, "o")
	if v := m.flow.fields[0].input.Value(); v != "o" {
		t.Fatalf("typed retry appended: %q", v)
	}
}

// F12: startup is a selector, not a typed word, and its default is not manual.
func TestOnboardingE03F12StartupIsSelector(t *testing.T) {
	m, _ := loadedForm(t)
	f := m.flow
	label := f.fields[3].label
	t.Logf("startup field %q default %q", label, f.fields[3].input.Value())
	if strings.Contains(label, "(manual/login/unattended)") {
		t.Error("startup offered as typed enumeration")
	}
	m.focusField(3)
	before := f.fields[3].input.Value()
	press(m, "z")
	if got := f.fields[3].input.Value(); got != before {
		t.Errorf("typing changed startup to %q (approved: selector)", got)
	}
	f.fields[3].input.SetValue(before)
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if got := f.fields[3].input.Value(); got == before || (got != "manual" && got != "login" && got != "unattended") {
		t.Errorf("right arrow left startup at %q (approved: selector moves to the next choice)", got)
	}
}

// E03 convention: ↑/↓ move between form fields, Enter advances and
// confirms on the last, number keys switch views, and every view keeps
// selectors as ‹ value ›.
func TestOnboardingE03FormArrowsEnterAndViewKeys(t *testing.T) {
	m, _ := loadedForm(t)
	f := m.flow
	m.focusField(0)
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if f.focus != 2 {
		t.Fatalf("down arrows focus %d", f.focus)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if f.focus != 1 {
		t.Fatalf("up arrow focus %d", f.focus)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if f.focus != 2 || f.screen != "form" {
		t.Fatalf("Enter on a middle field: focus %d screen %s", f.focus, f.screen)
	}
	m.width, m.height = 120, 40
	if v := ansi.Strip(m.View().Content); !strings.Contains(v, "Startup: ‹ ") || !strings.Contains(v, "Connection: ‹ ") {
		t.Fatalf("selectors not shown inline:\n%s", v)
	}
	s := shellFixture()
	for i, k := range []string{"2", "3", "4", "1"} {
		press(s, k)
		if want := []int{1, 2, 3, 0}[i]; s.section != want {
			t.Fatalf("key %s opened section %d", k, s.section)
		}
	}
}
