package terminal

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/calebhabesh/orbit/internal/config"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/charmbracelet/x/ansi"
)

// F04: the header names who runs the daemon separately from the startup mode
// and shows a failing unit as failing.
func TestOnboardingE01F04HeaderNamesOwnerAndFailingUnit(t *testing.T) {
	m := shellFixture()
	m.result.Service = &tc.Service{Running: true, Enabled: true, Mode: "login", Owner: "terminal", UnitState: "failed"}
	m.width, m.height = 120, 30
	summary := m.summary()
	if !strings.Contains(summary, "running (terminal)") || !strings.Contains(summary, "Startup: login; service failed") {
		t.Fatalf("summary %q", summary)
	}
	if lines := strings.Join(m.statusLines(false), "\n"); !strings.Contains(lines, "Daemon: running (terminal)") {
		t.Fatalf("status lines %q", lines)
	}
	m.result.Service = &tc.Service{Running: true, Enabled: true, Mode: "login", Owner: "service", UnitState: "active"}
	if summary = m.summary(); !strings.Contains(summary, "running (service)") || strings.Contains(summary, "failed") {
		t.Fatalf("healthy service summary %q", summary)
	}
}

// F12/EG3: the form explains the host's startup default with the exact
// linger command, and Ctrl-R re-checks after the owner ran it.
func TestOnboardingE01F12HostNoteAndLingerRecheck(t *testing.T) {
	m, w := workflowModel()
	linger := false
	w.query = func(_ context.Context, q tc.Query) (tc.Result, error) {
		s := config.DefaultRuntimeSettings()
		h := tc.HostStartup{Class: "headless", Suggested: "login", Systemd: true, LingerCommand: "sudo loginctl enable-linger owner",
			Note: "Unattended startup keeps Orbit running after you log out. It needs lingering: run sudo loginctl enable-linger owner, then re-check. Until then startup is login."}
		if linger {
			h.Suggested, h.Lingering, h.Note = "unattended", true, ""
		}
		s.Startup = h.Suggested
		return tc.Result{Settings: &s, Host: &h}, nil
	}
	runReply(m, m.setupForm("adopt"))
	f := m.flow
	if f.screen != "form" || f.fields[3].input.Value() != "login" {
		t.Fatalf("screen %q startup %q", f.screen, f.fields[3].input.Value())
	}
	m.width, m.height = 160, 50
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "sudo loginctl enable-linger owner") || !strings.Contains(view, "Ctrl-R") {
		t.Fatalf("form lacks linger command or re-check:\n%s", view)
	}
	linger = true
	runReply(m, m.key(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}))
	if got := f.fields[3].input.Value(); got != "unattended" {
		t.Fatalf("after re-check startup %q", got)
	}
}
