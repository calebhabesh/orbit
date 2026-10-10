package terminal

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestLogoSVGMatchesTerminal keeps the README logo in step with logoLarge and
// coreShades. ORBIT_UPDATE_LOGO=1 rewrites the SVG after an intended change.
func TestLogoSVGMatchesTerminal(t *testing.T) {
	const path = "../../docs/assets/orbit-logo.svg"
	got := logoSVG()
	if os.Getenv("ORBIT_UPDATE_LOGO") == "1" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Fatalf("%s is out of date; rerun with ORBIT_UPDATE_LOGO=1", path)
	}
}

// logoSVG draws logoLarge as the terminal does, with hex shades for the ANSI
// colours in a dark theme.
func logoSVG() string {
	const (
		left, cellW, top, cellH = 43.58, 10.84, 48.0, 24.0
		ring, muted             = "#22d3ee", "#94a3b8"
	)
	planets := []string{"#4ade80", "#60a5fa", "#c084fc", "#f87171", "#fbbf24", "#67e8f9"}
	shades := map[byte]string{'f': "#fb923c", 'b': "#f87171", 'y': "#fde047"}
	escape := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	l := &logoLarge
	x := func(col int) string { return fmt.Sprintf("%.2f", left+float64(col)*cellW) }
	wide := 0
	for _, line := range l.art {
		wide = max(wide, len([]rune(line)))
	}
	width, height := 2*left+cellW*float64(wide), top+cellH*float64(len(l.art))+72

	// Runs of one colour share a tspan; planets are bold and stand alone.
	var art, core strings.Builder
	planet := 0
	for i, line := range l.art {
		y := top + cellH*float64(i)
		fmt.Fprintf(&art, `    <text y="%.0f">`, y)
		// Box-drawing glyphs are shorter than a terminal cell, so the stack is
		// stretched about each cell's centre to join its rows as a terminal does.
		fmt.Fprintf(&core, `    <text y="%.0f" transform="translate(0 %.0f) scale(1 1.2) translate(0 -%.0f)">`, y, y-6, y-6)
		var run strings.Builder
		start, colour, bold, inCore := 0, "", false, false
		flush := func() {
			switch {
			case run.Len() == 0:
			case inCore:
				fmt.Fprintf(&core, `<tspan x="%s" fill="%s">%s</tspan>`, x(start), colour, run.String())
			case bold:
				fmt.Fprintf(&art, `<tspan x="%s" fill="%s" font-weight="bold">%s</tspan>`, x(start), colour, run.String())
			default:
				fmt.Fprintf(&art, `<tspan x="%s" fill="%s">%s</tspan>`, x(start), colour, run.String())
			}
			run.Reset()
		}
		for j, r := range []rune(line) {
			c, b, shade := ring, false, l.coreShade(i, j)
			switch {
			case r == ' ':
				flush()
				continue
			case shade != 0:
				c = shades[shade]
			case r == 'o' || r == 'O' || r == '@':
				c, b = planets[planet%len(planets)], true
				planet++
			case r == '+' || r == '*':
				c = muted
			}
			if run.Len() == 0 || c != colour || b || bold || (shade != 0) != inCore {
				flush()
				start, colour, bold, inCore = j, c, b, shade != 0
			}
			run.WriteString(escape.Replace(string(r)))
		}
		flush()
		art.WriteString("</text>\n")
		core.WriteString("</text>\n")
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" role="img" aria-labelledby="title description">`+"\n", width, height, width, height)
	b.WriteString(`  <title id="title">Orbit</title>` + "\n")
	b.WriteString(`  <desc id="description">The Orbit terminal logo: cyan ASCII orbital rings and colorful planets circle a stack of files drawn in orange and yellow. Your folders, on every device.</desc>` + "\n")
	b.WriteString(`  <!-- Generated from internal/terminal/theme.go by TestLogoSVGMatchesTerminal. -->` + "\n")
	b.WriteString(`  <g font-family="DejaVu Sans Mono, monospace" font-size="18">` + "\n")
	b.WriteString(art.String())
	b.WriteString(core.String())
	name := top + cellH*float64(len(l.art)) + 8
	fmt.Fprintf(&b, `    <text x="%.0f" y="%.0f" text-anchor="middle" fill="%s" font-weight="bold">O R B I T</text>`+"\n", width/2, name, ring)
	fmt.Fprintf(&b, `    <text x="%.0f" y="%.0f" text-anchor="middle" fill="%s" font-size="16">Your folders, on every device</text>`+"\n", width/2, name+32, muted)
	b.WriteString("  </g>\n</svg>\n")
	return b.String()
}
