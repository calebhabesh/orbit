package terminal

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func safe(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			fmt.Fprintf(&b, "\\u%04x", r)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (m *model) rows() []row {
	rows := make([]row, 0, 2*pageSize)
	if m.section == 0 || m.section == 2 {
		for _, a := range m.result.Attention {
			rows = append(rows, row{key: "a:" + a.ID, name: safe(a.Path), subtitle: safe(a.Code), folder: a.Folder, path: a.Path, action: safe(a.Action)})
		}
	}
	if m.section != 2 {
		for _, it := range m.result.Items {
			folder := it.ID
			if m.section == 3 {
				folder = ""
			}
			rows = append(rows, row{key: it.ID, name: safe(it.Name), subtitle: safe(it.Root), folder: folder, action: "orbit status / orbit doctor"})
		}
	}
	filter := strings.ToLower(m.search.Value())
	filtered := rows[:0]
	for _, r := range rows {
		if filter == "" || strings.Contains(strings.ToLower(r.name+" "+r.subtitle), filter) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func (m *model) View() tea.View {
	if m.flow != nil {
		return m.workflowView()
	}
	lines := []string{"Orbit", m.summary()}
	nav := make([]string, len(sections))
	for i, name := range sections {
		nav[i] = name
		if i == m.section {
			nav[i] = "[" + name + "]"
		}
	}
	lines = append(lines, strings.Join(nav, "  "))
	if m.search.Focused() {
		lines = append(lines, "> "+m.search.View())
	} else {
		lines = append(lines, "Search page: "+safe(m.search.Value()))
	}
	if m.width < 60 {
		lines[2] = sections[m.section] + " | f n d o"
		for i, line := range lines {
			lines[i] = ansi.Truncate(line, m.width, "…")
		}
	}
	if m.section == 0 && !m.help && !m.detail && len(m.devices) > 0 {
		names := make([]string, 0, 3)
		for _, it := range m.devices[:min(3, len(m.devices))] {
			names = append(names, safe(it.Name))
		}
		lines = append(lines, ansi.Truncate("Devices: "+strings.Join(names, ", ")+" | d: inspect", m.width, "…"))
	}
	if m.errText != "" {
		lines = append(lines, m.errText, "Last successful page retained; current observations unavailable.")
	}
	if m.notice != "" {
		lines = append(lines, m.notice)
	}
	if m.help {
		lines = append(lines, "Keyboard help", "q / Ctrl-C: close interface", "Sync and committed work continue.", "j/k or arrows: select", "left/right or o/f/n/d: section", "Tab or /: search this page", "Enter: inspect   Esc: back", "r: refresh   ]: next page   [: first page", "e: configured external tool", "Daemon stop: orbit service stop", "c: create/adopt  J: join  a: add device", "s: share folder  w: requests  u: unfinished setup")
	} else if m.detail {
		lines = append(lines, "Inspect", "Next action: "+m.detailRow.action, m.detailRow.name, m.detailRow.subtitle)
		for _, a := range m.result.Attention {
			lines = append(lines, safe(a.Code+": "+a.Path), safe(a.Action))
		}
	} else {
		if m.section == 0 {
			lines = append(lines, "Needs attention, then synced folders (first pages)")
		}
		rows := m.rows()
		if len(rows) == 0 {
			lines = append(lines, "No items on this page.", "Create or join a folder: orbit setup / orbit join")
		}
		available := max(1, m.height-len(lines)-3)
		start := max(0, m.selected-available+1)
		for i := start; i < min(len(rows), start+available); i++ {
			prefix := "  "
			if i == m.selected {
				prefix = "> "
			}
			lines = append(lines, ansi.Truncate(prefix+rows[i].name+"  "+rows[i].subtitle, m.width, "…"))
		}
		if m.result.Cursor != "" {
			lines = append(lines, "More available: ] next page")
		}
	}
	footer := "j/k Enter / ? q | c create J join a add s share w requests u setup"
	if m.width < 60 {
		footer = "j/k Enter / ? q"
	}
	if m.search.Focused() {
		footer = "Type to filter this page; Enter/Esc returns to navigation"
	}
	if m.pending != 0 && m.width >= 60 {
		footer += " | Refreshing"
	}
	// Keep focus and recovery text visible in narrow terminals through wrapping.
	// Never split an escape sequence or a wide/combining grapheme at the edge.
	var output []string
	for _, line := range lines {
		output = append(output, strings.Split(ansi.Wrap(line, m.width, ""), "\n")...)
	}
	footerLines := strings.Split(ansi.Wrap(footer, m.width, ""), "\n")
	if len(output)+len(footerLines) > m.height {
		output = output[:max(0, m.height-len(footerLines))]
	}
	output = append(output, footerLines...)
	if len(output) > m.height {
		output = output[:m.height]
	}
	if !m.opts.Colorless {
		if len(output) > 0 {
			output[0] = lipgloss.NewStyle().Bold(true).Render(output[0])
		}
	}
	content := strings.Join(output, "\n")
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}
