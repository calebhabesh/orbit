package terminal

import (
	"image/color"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/charmbracelet/x/ansi"
)

// theme renders Orbit's visual language: a header bar, rounded titled panels,
// semantic line colors and key-hint footers. Colors are ANSI palette indexes
// so the user's terminal theme decides the exact shades.
// A plain theme renders identical text and layout without escape sequences.
type theme struct {
	plain bool
}

var (
	cAccent = lipgloss.Color("6") // cyan: brand, active borders, keys
	cBlue   = lipgloss.Color("4")
	cGreen  = lipgloss.Color("2")
	cYellow = lipgloss.Color("3")
	cRed    = lipgloss.Color("1")
	cMuted  = lipgloss.Color("8")
	cBright = lipgloss.Color("15")
	cBlack  = lipgloss.Color("0")
)

func (t theme) style(s lipgloss.Style, text string) string {
	if t.plain || text == "" {
		return text
	}
	return s.Render(text)
}

func (t theme) fg(c color.Color, text string) string {
	return t.style(lipgloss.NewStyle().Foreground(c), text)
}
func (t theme) bold(text string) string { return t.style(lipgloss.NewStyle().Bold(true), text) }
func (t theme) muted(text string) string {
	return t.style(lipgloss.NewStyle().Foreground(cMuted), text)
}
func (t theme) accent(text string) string {
	return t.style(lipgloss.NewStyle().Foreground(cAccent).Bold(true), text)
}

// label colors a field name; values stay in the bright foreground.
func (t theme) label(text string) string { return t.fg(cAccent, text) }
func (t theme) good(text string) string  { return t.fg(cGreen, text) }
func (t theme) warn(text string) string  { return t.fg(cYellow, text) }
func (t theme) bad(text string) string   { return t.fg(cRed, text) }

// brand is the header badge.
func (t theme) brand() string {
	if t.plain {
		return "Orbit"
	}
	return lipgloss.NewStyle().Background(cAccent).Foreground(cBlack).Bold(true).Render(" ◉ ORBIT ")
}

// tab renders one section tab. Plain mode keeps the bracket selection marker.
func (t theme) tab(name, key string, active bool) string {
	if t.plain {
		if active {
			return "[" + name + "]"
		}
		return name
	}
	if active {
		return lipgloss.NewStyle().Background(cBlue).Foreground(cBright).Bold(true).Render(" " + key + " " + name + " ")
	}
	return t.muted(" "+key+" ") + name + " "
}

// pill summarises daemon state for the header's right edge.
func (t theme) pill(state, startup string) string {
	dot, color := "◌", cYellow
	switch {
	case strings.HasPrefix(state, "running"):
		dot, color = "●", cGreen
	case state == "stopped":
		dot, color = "○", cRed
	}
	return t.fg(color, dot+" "+state) + t.muted(" · startup ") + startup
}

// header is one full-width line: brand, a middle segment and a right segment.
func (t theme) header(width int, middle, right string) string {
	sep := "  "
	if t.plain {
		sep = " | "
	}
	left := t.brand()
	if middle != "" {
		left += sep + middle
	}
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if right == "" || gap < 2 {
		return ansi.Truncate(left, width, "…")
	}
	return left + strings.Repeat(" ", gap) + right
}

// keyPattern recognises key names inside free-form footer hint strings.
var keyPattern = regexp.MustCompile(`^(?:[a-zA-Z?/\[\]@]|Enter|Esc|Tab|Shift-Tab|Ctrl-[A-Z]|arrows|[0-9]-[0-9]|↑/↓|←/→|[a-zA-Z](?:/[a-zA-Z])+)$`)

// key renders one key name as a pill; Enter, the primary action, is green.
func (t theme) key(k string) string {
	if t.plain {
		return k
	}
	bg, fg := lipgloss.Color("8"), cBright
	if k == "Enter" {
		bg, fg = cGreen, cBlack
	}
	return lipgloss.NewStyle().Background(bg).Foreground(fg).Bold(true).Render(" " + k + " ")
}

// hints styles one footer group ("Enter Create Orbit"): a leading key name
// becomes a pill and the description follows; other text stays plain.
func (t theme) hints(group string) string {
	if t.plain {
		return group
	}
	group = strings.TrimSpace(group)
	if group == "|" {
		return t.muted("│")
	}
	k, desc, _ := strings.Cut(group, " ")
	if !keyPattern.MatchString(k) {
		return group
	}
	if desc == "" {
		return t.key(k)
	}
	if k == "Enter" {
		desc = t.style(lipgloss.NewStyle().Foreground(cGreen).Bold(true), desc)
	}
	return t.key(k) + " " + desc
}

// footer wraps key hints to width, keeping each "key description" group whole.
func (t theme) footer(text string, width int) []string {
	width = max(1, width)
	var groups []string
	for _, part := range strings.Split(text, "  ") {
		if part = strings.TrimSpace(part); part != "" {
			groups = append(groups, t.hints(part))
		}
	}
	var lines []string
	current := ""
	for _, g := range groups {
		switch {
		case current == "":
			current = g
		case ansi.StringWidth(current)+2+ansi.StringWidth(g) <= width:
			current += "  " + g
		default:
			lines = append(lines, current)
			current = g
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	var out []string
	for _, l := range lines {
		out = append(out, strings.Split(ansi.Wrap(l, width, ""), "\n")...)
	}
	return out
}

// keyRows renders aligned key/description pairs for help panels.
func (t theme) keyRows(pairs [][2]string) []string {
	w := 0
	for _, p := range pairs {
		w = max(w, ansi.StringWidth(p[0]))
	}
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		if p[0] == "" {
			out = append(out, p[1])
			continue
		}
		key := p[0] + strings.Repeat(" ", w-ansi.StringWidth(p[0]))
		if t.plain {
			out = append(out, p[0]+": "+p[1])
		} else {
			out = append(out, t.accent(key)+"  "+p[1])
		}
	}
	return out
}

var (
	kvLine    = regexp.MustCompile(`^([A-Z][A-Za-z0-9 /().'-]{0,34}?): (.*)$`)
	codeWord  = regexp.MustCompile(`\b[A-Z][A-Z0-9]*_[A-Z0-9_]+\b|\b(?:CONFLICT|VERIFIED|DIFF_READY|EDIT_READY|PENDING|FAILED)\b`)
	pairWord  = regexp.MustCompile(`([A-Za-z][A-Za-z ]{0,24}?)=([^\s;,]+)`)
	optionKey = regexp.MustCompile(`\[[^\]]{1,16}\]$`)
)

// value colors a status-like value; anything else gets inline highlighting.
func (t theme) value(v string) string {
	if t.plain {
		return v
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "running", "completed", "ready", "verified", "available", "direct connection", "recent", "enabled":
		return t.good(v)
	case "false":
		return t.muted(v)
	case "stopped", "failed", "blocked", "unavailable", "corrupt", "expired", "connection blocked":
		return t.bad(v)
	case "unknown", "pending", "stale", "partial", "not_tested", "awaiting_approval":
		return t.warn(v)
	}
	return t.inline(v)
}

// inline highlights machine codes and key=value pairs inside running text.
func (t theme) inline(s string) string {
	if t.plain {
		return s
	}
	s = pairWord.ReplaceAllStringFunc(s, func(m string) string {
		parts := pairWord.FindStringSubmatch(m)
		return t.muted(parts[1]+"=") + t.value(parts[2])
	})
	return codeWord.ReplaceAllStringFunc(s, func(m string) string {
		return t.style(lipgloss.NewStyle().Foreground(cYellow).Bold(true), m)
	})
}

// line styles one logical body line already wrapped into segments. The first
// segment decides the line's role so wrapped continuations keep its color.
func (t theme) line(segments []string, selected bool) []string {
	if t.plain || len(segments) == 0 {
		return segments
	}
	raw := segments[0]
	out := make([]string, len(segments))
	switch {
	case selected:
		sel := lipgloss.NewStyle().Foreground(cAccent).Bold(true)
		for i, s := range segments {
			if i == 0 && strings.HasPrefix(s, "> ") {
				s = "▌ " + strings.TrimPrefix(s, "> ")
			}
			out[i] = sel.Render(s)
		}
	case strings.HasPrefix(raw, "Error:") || strings.HasPrefix(raw, "Control unavailable"):
		for i, s := range segments {
			out[i] = t.style(lipgloss.NewStyle().Foreground(cRed).Bold(i == 0), s)
		}
	case strings.HasPrefix(raw, "  "):
		for i, s := range segments {
			out[i] = t.inline(s)
		}
	case optionKey.MatchString(raw) && len(segments) == 1:
		loc := optionKey.FindStringIndex(raw)
		out[0] = t.bold(raw[:loc[0]]) + t.accent(raw[loc[0]:])
	case kvLine.MatchString(raw):
		parts := kvLine.FindStringSubmatch(raw)
		out[0] = t.label(parts[1]+":") + " " + t.value(parts[2])
		for i := 1; i < len(segments); i++ {
			out[i] = t.inline(segments[i])
		}
	case isProse(segments):
		for i, s := range segments {
			out[i] = t.style(lipgloss.NewStyle().Foreground(cMuted).Italic(true), s)
		}
	default:
		for i, s := range segments {
			out[i] = t.inline(s)
		}
	}
	return out
}

// isProse reports explanatory sentences, which recede behind data and actions.
func isProse(segments []string) bool {
	last := strings.TrimSpace(segments[len(segments)-1])
	return strings.HasSuffix(last, ".") && !strings.Contains(segments[0], "=")
}

// body wraps and styles logical lines to width. It returns the wrapped lines
// and the index of the first wrapped line belonging to focus (or -1).
func (t theme) body(lines []string, width, focus int) ([]string, int) {
	width = max(1, width)
	var out []string
	focusWrapped := -1
	for i, line := range lines {
		if i == focus {
			focusWrapped = len(out)
		}
		if strings.Contains(line, "\x1b") {
			// Pre-styled content keeps its own colors.
			out = append(out, strings.Split(ansi.Wrap(line, width, ""), "\n")...)
			continue
		}
		segments := strings.Split(ansi.Wrap(line, width, ""), "\n")
		out = append(out, t.line(segments, i == focus || strings.HasPrefix(line, "> "))...)
	}
	return out, focusWrapped
}

// panel draws a rounded box of exactly width × height cells around rows.
// title sits in the top border; status (e.g. "3 of 12") in the bottom border.
func (t theme) panel(title, status string, rows []string, width, height int, active bool) []string {
	if width < 6 || height < 3 {
		out := rows
		if len(out) > height {
			out = out[:max(0, height)]
		}
		return out
	}
	border := lipgloss.NewStyle().Foreground(cMuted)
	titleStyle := lipgloss.NewStyle().Bold(true)
	if active {
		border = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
		titleStyle = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	}
	b := func(s string) string { return t.style(border, s) }
	inner := width - 2
	if title != "" {
		title = " " + ansi.Truncate(title, max(0, inner-5), "…") + " "
	}
	top := b("╭" + strings.Repeat("─", inner) + "╮")
	if title != "" {
		top = b("╭─") + t.style(titleStyle, title) + b(strings.Repeat("─", max(0, inner-1-ansi.StringWidth(title)))+"╮")
	}
	if status != "" {
		status = " " + ansi.Truncate(status, max(0, inner-5), "…") + " "
	}
	bottom := b("╰" + strings.Repeat("─", inner) + "╯")
	if status != "" {
		bottom = b("╰"+strings.Repeat("─", max(0, inner-1-ansi.StringWidth(status)))) + t.muted(status) + b("─╯")
	}
	out := []string{top}
	content := width - 4
	for i := 0; i < height-2; i++ {
		row := ""
		if i < len(rows) {
			row = ansi.Truncate(rows[i], content, "…")
		}
		pad := max(0, content-ansi.StringWidth(row))
		out = append(out, b("│")+" "+row+strings.Repeat(" ", pad)+" "+b("│"))
	}
	return append(out, bottom)
}

// besides joins two equal-height columns of rendered lines.
func besides(left, right []string) []string {
	n := max(len(left), len(right))
	out := make([]string, n)
	lw := 0
	for _, l := range left {
		lw = max(lw, ansi.StringWidth(l))
	}
	for i := 0; i < n; i++ {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out[i] = l + strings.Repeat(" ", max(0, lw-ansi.StringWidth(l))) + r
	}
	return out
}

func (m *model) theme() theme { return theme{plain: m.opts.Colorless} }

// orbitLogo is the Orbit mark: a stack of files with planets on nested
// orbits. The rings and planets are ASCII; the file stack at coreRow, coreCol
// is drawn with the same box-drawing characters as the panels.
type orbitLogo struct {
	art              []string
	coreRow, coreCol int
}

var logoLarge = orbitLogo{coreRow: 5, coreCol: 21, art: []string{
	`   *             _.----------o--._             +`,
	`         .------'                 '------.`,
	`     .--'         _.-----------._         '--.`,
	`   .'       .----'               '--O-.       '.`,
	` .'       .'       _.o--------._       '.       '.`,
	`/       .'      .-'   ╭────╮    '-.      '.       \`,
	`|       |      |     ╭│ ━━ ╰╮      |      |       |`,
	`@       |      |     ││ ━━━ │      |      |       |`,
	`|       |      |     │╰─────╯      |      |       |`,
	`\       '.      '-.  ╰─────╯    .-'      .'       /`,
	` '.       '.       '-----------'       .'       .'`,
	`   '.       '----.               .----'       .'`,
	`     '--.         '--@----------'         .--'`,
	`         '------.                 .--O---'`,
	`   +             '---------------'             *`,
}}

var logoSmall = orbitLogo{coreRow: 3, coreCol: 14, art: []string{
	`   *       _.---------o-._      +`,
	`     .----'               '----.`,
	`   .'       _.o--------._       '.`,
	` .'      .-'   ╭────╮    '-.      '.`,
	` |      |     ╭│ ━━ ╰╮      |      |`,
	` @      |     ││ ━━━ │      |      |`,
	` |      |     │╰─────╯      |      |`,
	` '.      '-.  ╰─────╯    .-'      .'`,
	`   '.       '-----------'       .'`,
	`     '--O-.               .----'`,
	`   +       '-------------'      *`,
}}

// coreShades colours the file stack cell by cell: f front page, b pages
// behind it, y text lines.
var coreShades = []string{
	` ffffff`,
	`bf yy ff`,
	`bf yyy f`,
	`bfffffff`,
	`bbbbbbb`,
}

var coreColors = map[byte]color.Color{'f': lipgloss.Color("208"), 'b': cRed, 'y': lipgloss.Color("11")}

// coreShade returns the stack's colour role at row i, cell j of l, or 0.
func (l *orbitLogo) coreShade(i, j int) byte {
	i, j = i-l.coreRow, j-l.coreCol
	if i < 0 || i >= len(coreShades) || j < 0 || j >= len(coreShades[i]) || coreShades[i][j] == ' ' {
		return 0
	}
	return coreShades[i][j]
}

// planetColors colour the planets in reading order.
var planetColors = []color.Color{cGreen, cBlue, lipgloss.Color("5"), cRed, cYellow, lipgloss.Color("14")}

// logo returns the mark that fits the space, or nil when none does.
func logo(width, height int) *orbitLogo {
	switch {
	case width >= 56 && height >= 32:
		return &logoLarge
	case width >= 42 && height >= 24:
		return &logoSmall
	}
	return nil
}

// banner is the welcome screen's mark with the product name beneath it.
func (t theme) banner(width, height int) []string {
	l := logo(width, height)
	if l == nil {
		return nil
	}
	art := l.art
	wide := 0
	for _, line := range art {
		wide = max(wide, ansi.StringWidth(line))
	}
	centre := func(s string) string { return strings.Repeat(" ", max(0, (wide-len(s))/2)) + s }
	name, tagline := centre("O R B I T"), centre("Your folders, on every device")
	if t.plain {
		return append(append(slices.Clone(art), "", name, tagline), "")
	}
	out := make([]string, 0, len(art)+4)
	planet := 0
	for i, line := range art {
		var b strings.Builder
		for j, r := range []rune(line) {
			c := cAccent
			switch shade := l.coreShade(i, j); {
			case r == ' ':
				b.WriteRune(r)
				continue
			case shade != 0:
				b.WriteString(lipgloss.NewStyle().Foreground(coreColors[shade]).Render(string(r)))
				continue
			case r == 'o' || r == 'O' || r == '@':
				c = planetColors[planet%len(planetColors)]
				planet++
			case r == '+' || r == '*':
				c = cMuted
			}
			b.WriteString(lipgloss.NewStyle().Foreground(c).Bold(c != cAccent && c != cMuted).Render(string(r)))
		}
		out = append(out, b.String())
	}
	return append(out, "", t.accent(name), t.muted(tagline), "")
}

// row renders one aligned label/value line of a summary card.
func (t theme) row(label, value string) string {
	pad := strings.Repeat(" ", max(1, 12-ansi.StringWidth(label)))
	if t.plain {
		if label == "" {
			return pad + value
		}
		return label + ":" + strings.Repeat(" ", max(1, 11-ansi.StringWidth(label))) + value
	}
	if !strings.Contains(value, "\x1b") {
		value = t.fg(cBright, value)
	}
	return t.label(label) + pad + value
}

// field renders one form input. The focused field carries the "> " marker in
// plain mode and an accent bar in color; byte-sized values gain a readable hint.
func (t theme) field(label, value, raw string, focused bool) string {
	hint := ""
	if strings.Contains(label, "bytes") {
		if n, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64); err == nil && n >= 1024 {
			hint = "  ≈ " + humanBytes(n)
		}
	}
	if t.plain {
		prefix := "  "
		if focused {
			prefix = "> "
		}
		return prefix + label + ": " + value + hint
	}
	if focused {
		return t.accent("▌ "+label+":") + " " + value + t.muted(hint)
	}
	return "  " + t.label(label+":") + " " + t.fg(cBright, value) + t.muted(hint)
}

// humanBytes formats a byte count with binary units (1.5 GiB).
func humanBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatUint(n, 10) + " B"
	}
	div, exp := uint64(unit), 0
	for v := n / unit; v >= unit && exp < 4; v /= unit {
		div *= unit
		exp++
	}
	value := strconv.FormatFloat(float64(n)/float64(div), 'f', 1, 64)
	value = strings.TrimSuffix(value, ".0")
	return value + " " + string("KMGTP"[exp]) + "iB"
}

func hb(n tc.Uint) string { return humanBytes(uint64(n)) }

func bandwidth(n tc.Uint) string {
	if n == 0 {
		return "unlimited"
	}
	return hb(n) + "/s"
}
