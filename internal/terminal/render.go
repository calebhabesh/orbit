package terminal

import (
	"fmt"
	"os"
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
			action := "Enter opens the folder: copy status, history, sharing and storage."
			if folder == "" {
				action = "Enter opens this device's folders and contact state."
			}
			rows = append(rows, row{key: it.ID, name: safe(it.Name), subtitle: homePath(safe(it.Root)), folder: folder, action: action})
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
	var output []string
	if m.width < 60 || m.height < 14 {
		output = m.compactLines()
	} else {
		output = m.panelLines()
	}
	if len(output) > m.height {
		output = output[:m.height]
	}
	v := tea.NewView(strings.Join(output, "\n"))
	v.AltScreen = true
	return v
}

var helpKeys = [][2]string{
	{"q / Ctrl-C", "close interface (sync and committed work continue)"},
	{"j/k arrows", "select"},
	{"o f n d", "Overview, Folders, Attention, Devices"},
	{"Tab or /", "search this page"},
	{"Enter", "inspect"},
	{"Esc", "back"},
	{"r", "refresh"},
	{"] [", "next page / first page"},
	{"e", "configured external tool"},
	{"b", "storage"},
	{"v h D C", "folder status, history, deleted, conflicts"},
	{"c", "create/adopt"},
	{"J", "join"},
	{"a", "add device"},
	{"s", "share folder"},
	{"w", "requests"},
	{"u", "unfinished setup"},
	{"N", "connection details"},
	{"", "Daemon stop: orbit service stop"},
}

func (m *model) footer() string {
	footer := "j/k select  Enter inspect  / search  ? help  q quit  |  c create  J join  a add  s share  w requests  u setup  N connection"
	if m.width < 60 {
		footer = "j/k Enter / ? q"
	}
	if m.search.Focused() {
		footer = "Type to filter this page; Enter/Esc returns to navigation"
	}
	if m.pending != 0 && m.width >= 60 {
		footer += " | Refreshing"
	}
	return footer
}

func (m *model) footerLines(t theme) []string {
	return t.footer(m.footer(), m.width)
}

func (m *model) tabs(t theme) string {
	keys := []string{"o", "f", "n", "d"}
	nav := make([]string, len(sections))
	for i, name := range sections {
		nav[i] = t.tab(name, keys[i], i == m.section)
	}
	if t.plain {
		return strings.Join(nav, "  ")
	}
	return strings.Join(nav, "")
}

// statusLines describe the daemon, connection and transient notices.
func (m *model) statusLines(counts bool) []string {
	s := m.result.Service
	startup := "unknown"
	if s != nil {
		startup = safe(s.Mode)
		if !s.Enabled {
			startup += " (not enabled)"
		}
	}
	lines := []string{"Daemon: " + running(s), "Startup: " + startup}
	if counts && m.section == 0 {
		lines = append(lines, count(len(m.result.Items), "folder")+" · "+count(len(m.result.Attention), "attention item")+" · "+count(len(m.devices), "device"))
	}
	if m.errText != "" {
		lines = append(lines, "Error: "+m.errText, "Last successful page retained; current observations unavailable.")
	}
	if n := m.result.Network; n != nil {
		lines = append(lines, "Connection: "+connectionMode(n.Policy.Mode), "Services: "+safe(n.Code))
		if n.AutomaticOffer || n.ProfileUpdate == "available" {
			lines = append(lines, safe(n.Action))
		}
		for _, o := range n.Observations {
			label := o.Route
			if o.Route == "relay" {
				label = "Connected via relay"
			}
			lines = append(lines, safe(o.Device[:min(12, len(o.Device))])+": "+safe(label)+"; "+safe(o.Freshness))
		}
	}
	if m.notice != "" {
		lines = append(lines, m.notice)
	}
	return lines
}

func (m *model) searchLine() string {
	if m.search.Focused() {
		return "> " + m.search.View()
	}
	return "Search page: " + safe(m.search.Value())
}

// rowLines renders list rows for a content width, keeping the selection
// visible. The overview groups attention items above folders under headings.
func (m *model) rowLines(t theme, width, height int) ([]string, string) {
	rows := m.rows()
	if len(rows) == 0 {
		return []string{"No items on this page.", "Create or join a folder: c create, J join."}, ""
	}
	type entry struct {
		heading string
		row     int
	}
	var entries []entry
	selected := 0
	group := ""
	grouped := m.section == 0 && len(m.result.Attention) > 0 && len(rows) > len(m.result.Attention)
	for i, r := range rows {
		if grouped {
			g := "Folders"
			if strings.HasPrefix(r.key, "a:") {
				g = "Needs attention"
			}
			if g != group {
				if group != "" {
					entries = append(entries, entry{heading: " ", row: -1})
				}
				entries = append(entries, entry{heading: g, row: -1})
				group = g
			}
		}
		if i == m.selected {
			selected = len(entries)
		}
		entries = append(entries, entry{row: i})
	}
	height = max(1, height)
	start := max(0, selected-height+1)
	var out []string
	for _, e := range entries[start:min(len(entries), start+height)] {
		if e.row < 0 {
			if e.heading == " " {
				out = append(out, "")
			} else if t.plain {
				out = append(out, e.heading)
			} else {
				out = append(out, t.style(lipgloss.NewStyle().Foreground(cMuted).Bold(true), strings.ToUpper(e.heading)))
			}
			continue
		}
		r := rows[e.row]
		if t.plain {
			prefix := "  "
			if e.row == m.selected {
				prefix = "> "
			}
			out = append(out, ansi.Truncate(prefix+r.name+"  "+r.subtitle, width, "…"))
			continue
		}
		mark, sub := "  ", t.muted(r.subtitle)
		if strings.HasPrefix(r.key, "a:") {
			mark = t.warn("▲ ")
			sub = t.style(lipgloss.NewStyle().Foreground(cYellow).Bold(true), r.subtitle)
		}
		line := ansi.Truncate(mark+r.name+"  "+sub, width, "…")
		if e.row == m.selected {
			text := ansi.Truncate("▌ "+r.name+"  "+r.subtitle, width, "…")
			text += strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
			line = lipgloss.NewStyle().Background(cBlue).Foreground(cBright).Bold(true).Render(text)
		}
		out = append(out, line)
	}
	status := fmt.Sprintf("%d of %d", m.selected+1, len(rows))
	if m.result.Cursor != "" {
		status += " · ] more"
	}
	return out, status
}

// detailLines describe the selected row, help or an inspected item.
func (m *model) detailLines() (string, []string) {
	if m.help {
		return "Keyboard help", m.theme().keyRows(helpKeys)
	}
	if m.detail {
		lines := []string{"Next action: " + m.detailRow.action, m.detailRow.name, m.detailRow.subtitle}
		for _, a := range m.result.Attention {
			lines = append(lines, safe(a.Code+": "+a.Path), safe(a.Action))
		}
		return "Inspect", lines
	}
	rows := m.rows()
	if len(rows) == 0 {
		return "Details", []string{"Nothing selected.", "Create or join a folder: c create, J join."}
	}
	r := rows[min(m.selected, len(rows)-1)]
	lines := []string{"Name: " + r.name}
	if strings.HasPrefix(r.key, "a:") {
		lines = append(lines, "Issue: "+r.subtitle, "Next action: "+r.action, "", "Enter opens the review for this item.")
	} else {
		if r.subtitle != "" {
			label := "Root: "
			if r.folder == "" {
				label = "ID: "
			}
			lines = append(lines, label+r.subtitle)
		}
		for _, a := range m.result.Attention {
			if r.folder != "" && a.Folder == r.folder {
				lines = append(lines, safe(a.Code+": "+a.Path), safe(a.Action))
			}
		}
		lines = append(lines, "", r.action)
	}
	return "Details", lines
}

func (m *model) listTitle() string {
	switch m.section {
	case 0, 1:
		return "Folders"
	case 2:
		return "Attention"
	}
	return "Devices"
}

// panelLines lays out the overview as titled panels. Wide terminals show
// status, list and devices on the left and details of the selection on the right.
func (m *model) panelLines() []string {
	t := m.theme()
	right := ""
	if !t.plain {
		right = t.pill(running(m.result.Service), m.startup())
	}
	header := t.header(m.width, m.tabs(t), right)
	footer := m.footerLines(t)
	height := max(3, m.height-1-len(footer))
	wide := m.width >= 100
	leftWidth := m.width
	if wide {
		leftWidth = max(40, m.width*2/5)
	}
	statusText := m.statusLines(true)
	if t.plain {
		// The plain header carries no pill, so daemon state opens the status panel.
		statusText = append([]string{m.summary()}, statusText[2:]...)
	} else {
		statusText = statusText[2:]
	}
	status, _ := t.body(statusText, leftWidth-4, -1)
	statusHeight := min(len(status)+2, max(3, height/3))
	devHeight := 0
	var devices []string
	if m.section == 0 && len(m.devices) > 0 && !m.help && !m.detail {
		for _, it := range m.devices[:min(4, len(m.devices))] {
			devices = append(devices, safe(it.Name))
		}
		devHeight = min(len(devices)+2, 6)
		if !wide {
			devices = []string{strings.Join(devices, ", ")}
			devHeight = 3
		}
	}
	listHeight := max(3, height-statusHeight-devHeight)
	left := t.panel("Status", "", status, leftWidth, statusHeight, false)
	listInner := leftWidth - 4
	// An idle, empty search stays out of the way; "/" opens it.
	var search []string
	if m.search.Focused() || m.search.Value() != "" || t.plain {
		search = []string{m.searchLine()}
		if !m.search.Focused() {
			search[0] = t.muted(search[0])
		}
	}
	if !wide && (m.help || m.detail) {
		title, lines := m.detailLines()
		body, _ := t.body(lines, listInner, -1)
		left = append(left, t.panel(title, "", body, leftWidth, listHeight, true)...)
	} else {
		rows, listStatus := m.rowLines(t, listInner, listHeight-2-len(search))
		listBody := append(search, rows...)
		left = append(left, t.panel(m.listTitle(), listStatus, listBody, leftWidth, listHeight, !m.help && !m.detail)...)
	}
	if devHeight > 0 {
		devBody, _ := t.body(devices, listInner, -1)
		left = append(left, t.panel("Devices", "d inspect", devBody, leftWidth, devHeight, false)...)
	}
	body := left
	if wide {
		title, lines := m.detailLines()
		detail, _ := t.body(lines, m.width-leftWidth-4, -1)
		body = besides(left, t.panel(title, "", detail, m.width-leftWidth, height, m.help || m.detail))
	}
	return append(append([]string{header}, body...), footer...)
}

func (m *model) startup() string {
	if s := m.result.Service; s != nil {
		if !s.Enabled {
			return safe(s.Mode) + " (not enabled)"
		}
		return safe(s.Mode)
	}
	return "unknown"
}

// compactLines keep small terminals usable: one column, no borders.
func (m *model) compactLines() []string {
	t := m.theme()
	lines := []string{"Orbit", m.summary(), sections[m.section] + " | f n d o"}
	if m.search.Focused() {
		lines = append(lines, "> "+m.search.View())
	} else {
		lines = append(lines, "Search page: "+safe(m.search.Value()))
	}
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, m.width, "…")
	}
	status := m.statusLines(false)[2:]
	lines = append(lines, status...)
	if m.help {
		lines = append(lines, "Keyboard help")
		lines = append(lines, m.theme().keyRows(helpKeys)...)
	} else if m.detail {
		_, d := m.detailLines()
		lines = append(lines, "Inspect")
		lines = append(lines, d...)
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
	// Keep focus and recovery text visible in narrow terminals through wrapping.
	// Never split an escape sequence or a wide/combining grapheme at the edge.
	output, _ := t.body(lines, m.width, -1)
	if !t.plain && len(output) > 2 {
		output[0] = t.header(m.width, "", t.pill(running(m.result.Service), m.startup()))
		output[1] = t.tab(sections[m.section], "", true) + " " + t.hints("f n d o")
	}
	footerLines := m.footerLines(t)
	if len(output)+len(footerLines) > m.height {
		output = output[:max(0, m.height-len(footerLines))]
	}
	return append(output, footerLines...)
}

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// homePath shortens a path under the user's home directory to ~/… for lists.
func homePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || home == "/" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+"/") {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}
