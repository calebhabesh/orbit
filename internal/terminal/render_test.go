package terminal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/charmbracelet/x/ansi"
)

func sampleModel(colorless bool, width, height int) *model {
	m := newModel(context.Background(), queryFunc(func(context.Context, tc.Query) (tc.Result, error) { return tc.Result{}, nil }), Options{Colorless: colorless})
	m.width, m.height = width, height
	m.result = tc.Result{
		Service: &tc.Service{Running: true, Enabled: true, Mode: "login"},
		Items: []tc.NamedItem{
			{ID: strings.Repeat("a", 64), Name: "Notes", Root: "/home/owner/Notes"},
			{ID: strings.Repeat("b", 64), Name: "Photos", Root: "/home/owner/Pictures/Camera"},
			{ID: strings.Repeat("c", 64), Name: "Projects", Root: "/home/owner/dev/projects"},
		},
		Attention: []tc.Attention{{ID: "x1", Folder: strings.Repeat("a", 64), Path: "todo.md", Code: "CONFLICT", Action: "Review both versions with C conflicts."}},
		Network:   &tc.NetworkStatus{Policy: tc.NetworkPolicy{Mode: "automatic"}, Code: "READY", Observations: []tc.NetworkObservation{{Device: "3f9a7c21b4d0aa", Route: "relay", Freshness: "recent"}}},
	}
	m.devices = []tc.NamedItem{{Name: "Laptop"}, {Name: "Raspberry Pi"}}
	return m
}

// TestRenderedFramesFitTerminal renders representative screens in color and
// plain modes and checks no frame exceeds the terminal. ORBIT_TUI_PREVIEW=dir
// also writes each frame for visual review.
func TestRenderedFramesFitTerminal(t *testing.T) {
	dir := os.Getenv("ORBIT_TUI_PREVIEW")
	screens := map[string]func(*model){
		"overview": func(*model) {},
		"help":     func(m *model) { m.help = true },
		"folders":  func(m *model) { m.section = 1; m.selected = 1 },
		"welcome":  func(m *model) { m.flow = &workflow{screen: "welcome"} },
		"folder": func(m *model) {
			m.flow = &workflow{screen: "folder", result: tc.Result{FolderManagement: &tc.FolderManagement{Root: "/home/owner/Notes", Revision: 4, Members: []tc.NamedItem{{Name: "Laptop"}, {Name: "Raspberry Pi"}}}, Observations: []tc.Observation{{Device: "d", LastContact: "2 minutes ago", Availability: "available", Stored: true, Applied: true, ObservedAt: "21:14"}}}}
		},
		"network": func(m *model) {
			m.flow = &workflow{screen: "network", result: tc.Result{Network: &tc.NetworkStatus{Policy: tc.NetworkPolicy{Mode: "automatic"}, Code: "READY", Ready: true, Operator: "Orbit", Privacy: "addresses and timing only", Action: "none", Observations: []tc.NetworkObservation{{Device: "3f9a7c21b4d0aa", Route: "relay", Freshness: "recent", Code: "RELAY_CONNECTED", Action: "none", UDPCode: "UDP_BLOCKED"}}}}}
		},
	}
	for name, setup := range screens {
		for _, size := range [][2]int{{140, 40}, {90, 28}, {50, 18}, {40, 16}} {
			for _, colorless := range []bool{false, true} {
				m := sampleModel(colorless, size[0], size[1])
				setup(m)
				view := m.View().Content
				lines := strings.Split(view, "\n")
				if len(lines) > size[1] {
					t.Errorf("%s %v colorless=%t: %d lines exceed height", name, size, colorless, len(lines))
				}
				for i, line := range lines {
					if w := ansi.StringWidth(line); w > size[0] {
						t.Errorf("%s %v colorless=%t line %d: width %d exceeds %d: %q", name, size, colorless, i, w, size[0], ansi.Strip(line))
					}
				}
				if dir != "" {
					mode := "color"
					if colorless {
						mode = "plain"
					}
					path := filepath.Join(dir, fmt.Sprintf("%s-%dx%d-%s.ans", name, size[0], size[1], mode))
					if err := os.WriteFile(path, []byte(view), 0o600); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
}
