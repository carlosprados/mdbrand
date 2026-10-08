package fig

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/carlosprados/mdbrand/internal/brand"
)

// A figure used to arrive in Vega's and d2's own blue on a page in the
// brand's colours. The brand now reaches it: whatever a spec or a .d2 does
// not set itself is set from the bundle, and what the author set is left
// alone. The sizes never change, so neither does the legibility arithmetic.

// minGraphicContrast is WCAG's ratio for graphical objects. A filled bar is a
// large area and reads at less; a line or a point does not.
const minGraphicContrast = 3.0

// figureColors is the brand as a figure uses it.
type figureColors struct {
	fill   string   // bars, areas: the primary as it is
	line   string   // lines, points, rules: the primary, darkened to 3:1 if it falls short
	text   string   // labels and titles
	rule   string   // axis domains and ticks
	grid   string   // grid lines, lighter than the rule
	series []string // one colour per series
}

func colorsOf(b *brand.Brand) figureColors {
	line := b.Colors.Primary
	if brand.Contrast(line, "FFFFFF") < minGraphicContrast {
		line = brand.Darken(line, minGraphicContrast)
	}
	series := b.Figures.Palette
	if len(series) == 0 {
		series = dedupe(b.Colors.Primary, b.Colors.Text, b.Colors.Link, b.Colors.Accent, b.Colors.Rule)
	}
	return figureColors{
		fill: b.Colors.Primary, line: line, text: b.Colors.Text, rule: b.Colors.Rule,
		grid: tint(b.Colors.Rule, 0.5), series: series,
	}
}

func dedupe(hexes ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, h := range hexes {
		if h = strings.ToUpper(h); h != "" && !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}

// tint mixes hex with white, keeping f of the colour: 1 is the colour, 0 white.
func tint(hex string, f float64) string {
	var out [3]int
	for i := range out {
		v, _ := strconv.ParseUint(hex[2*i:2*i+2], 16, 8)
		out[i] = int(float64(v)*f + 255*(1-f) + 0.5)
	}
	return fmt.Sprintf("%02X%02X%02X", out[0], out[1], out[2])
}

// vegaTheme is the config a chart gets from the brand. font is the body face,
// or "" where it is not installed and Vega's own should stay.
func vegaTheme(b *brand.Brand, font string) map[string]any {
	c := colorsOf(b)
	h := func(s string) string { return "#" + s }
	text := map[string]any{"labelColor": h(c.text), "titleColor": h(c.text)}
	t := map[string]any{
		"mark":  map[string]any{"color": h(c.fill)},
		"line":  map[string]any{"color": h(c.line)},
		"point": map[string]any{"color": h(c.line)},
		"rule":  map[string]any{"color": h(c.line)},
		"trail": map[string]any{"color": h(c.line)},
		"tick":  map[string]any{"color": h(c.line)},
		"text":  map[string]any{"color": h(c.text)},
		"range": map[string]any{"category": prefixed(c.series)},
		"axis": map[string]any{"labelColor": h(c.text), "titleColor": h(c.text),
			"domainColor": h(c.rule), "tickColor": h(c.rule), "gridColor": h(c.grid)},
		"legend": text,
		"header": text,
		"title":  map[string]any{"color": h(c.text), "subtitleColor": h(c.text)},
	}
	if font != "" {
		t["font"] = font
	}
	return t
}

func prefixed(hexes []string) []any {
	out := make([]any, len(hexes))
	for i, x := range hexes {
		out[i] = "#" + x
	}
	return out
}

// applyTheme fills a spec's config from theme wherever the spec has not set
// the key itself: the author's choice always wins, at any depth.
func applyTheme(spec any, theme map[string]any) {
	top, ok := spec.(map[string]any)
	if !ok {
		return
	}
	cfg, ok := top["config"].(map[string]any)
	if !ok {
		cfg = map[string]any{}
		top["config"] = cfg
	}
	fillMissing(cfg, theme)
}

func fillMissing(dst, src map[string]any) {
	for k, v := range src {
		have, set := dst[k]
		if !set {
			dst[k] = v
			continue
		}
		hm, ok1 := have.(map[string]any)
		vm, ok2 := v.(map[string]any)
		if ok1 && ok2 {
			fillMissing(hm, vm)
		}
	}
}

// d2Theme is a vars block that recolours d2's theme from the brand: the
// lightest tints of the primary fill the shapes, deepening for containers,
// and the text colour draws strokes, edges and labels. The tints stop where
// the text on the deepest of them would fall under 4.5:1. d2 merges this with
// the author's own vars, so a layout-engine set there survives.
func d2Theme(b *brand.Brand) string {
	text := b.Colors.Text
	deep := 0.35
	for deep > 0.05 && brand.Contrast(text, tint(b.Colors.Primary, deep)) < 4.5 {
		deep -= 0.05
	}
	shades := map[string]string{
		"N1": text, "N2": text, "B1": text, "B2": b.Colors.Primary, "B3": b.Colors.Primary,
		"B4": tint(b.Colors.Primary, deep), "B5": tint(b.Colors.Primary, deep/2), "B6": tint(b.Colors.Primary, deep/4),
	}
	// The dark theme gets the same: d2 writes it into a prefers-color-scheme
	// block, and both themes are pinned to one for the reason they are pinned
	// at all — paper has no dark mode.
	var s strings.Builder
	s.WriteString("\n# --- added by mdbrand: the brand's colours ---\nvars: {\n  d2-config: {\n")
	for _, block := range []string{"theme-overrides", "dark-theme-overrides"} {
		fmt.Fprintf(&s, "    %s: {\n", block)
		for _, k := range []string{"N1", "N2", "B1", "B2", "B3", "B4", "B5", "B6"} {
			fmt.Fprintf(&s, "      %s: \"#%s\"\n", k, shades[k])
		}
		s.WriteString("    }\n")
	}
	s.WriteString("  }\n}\n")
	return s.String()
}
