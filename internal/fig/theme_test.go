package fig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/doc"
)

func amplia() *brand.Brand {
	b := brand.Default()
	b.Colors.Primary, b.Colors.Text, b.Colors.Rule, b.Colors.Link, b.Colors.Accent = "F68E1B", "5D6266", "C8CCCE", "1565C0", "B35A00"
	return b
}

// A filled bar takes the brand's own primary; a line or a point, thin, is
// darkened until it reaches 3:1 — and only when it needs to.
func TestFigureColors(t *testing.T) {
	c := colorsOf(amplia())
	if c.fill != "F68E1B" {
		t.Errorf("fill = %s, want the primary as it is", c.fill)
	}
	if r := brand.Contrast(c.line, "FFFFFF"); r < 3 || c.line == "F68E1B" {
		t.Errorf("line = %s at %.2f:1, want the primary darkened to 3:1", c.line, r)
	}
	if got := strings.Join(c.series, ","); got != "F68E1B,5D6266,1565C0,B35A00,C8CCCE" {
		t.Errorf("series = %s", got)
	}

	b := brand.Default() // 1F6FEB passes 3:1 already
	if c := colorsOf(b); c.line != b.Colors.Primary {
		t.Errorf("a primary that passes must not be darkened: %s", c.line)
	}
	if got := strings.Join(colorsOf(b).series, ","); strings.Count(got, b.Colors.Primary) != 1 {
		t.Errorf("link and accent equal to the primary must not repeat it: %s", got)
	}
	b.Figures.Palette = []string{"112233", "445566"}
	if got := strings.Join(colorsOf(b).series, ","); got != "112233,445566" {
		t.Errorf("a declared palette must be used as is: %s", got)
	}
}

// The author's config wins at every depth; only what is missing is filled.
func TestApplyThemeKeepsTheAuthorsChoices(t *testing.T) {
	spec := map[string]any{"config": map[string]any{
		"axis": map[string]any{"labelColor": "#000000"},
		"mark": "not a map",
	}}
	applyTheme(spec, vegaTheme(amplia(), "Inter"))
	cfg := spec["config"].(map[string]any)
	axis := cfg["axis"].(map[string]any)
	if axis["labelColor"] != "#000000" {
		t.Errorf("the author's labelColor was replaced: %v", axis["labelColor"])
	}
	if axis["titleColor"] != "#5D6266" {
		t.Errorf("a missing key was not filled: %v", axis["titleColor"])
	}
	if cfg["mark"] != "not a map" || cfg["font"] != "Inter" {
		t.Errorf("mark %v, font %v", cfg["mark"], cfg["font"])
	}

	bare := map[string]any{}
	applyTheme(bare, vegaTheme(amplia(), ""))
	if _, ok := bare["config"].(map[string]any)["font"]; ok {
		t.Error("a body face that is not installed must not be named")
	}
}

// The text colour draws on the deepest tint, so the tints stop at 4.5:1.
func TestD2ThemeKeepsTextLegible(t *testing.T) {
	b := amplia()
	b.Colors.Text = "6B7075" // passes on white, not on a strong tint
	theme := d2Theme(b)
	i := strings.Index(theme, `B4: "#`)
	b4 := theme[i+6 : i+12]
	if r := brand.Contrast(b.Colors.Text, b4); r < 4.5 {
		t.Errorf("text on the container tint %s is %.2f:1", b4, r)
	}
}

// A .d2 with theme-overrides of its own keeps them; one with only a font
// size still gets the brand's colours.
func TestD2SourceRespectsTheAuthorsTheme(t *testing.T) {
	dir := t.TempDir()
	for name, want := range map[string]bool{
		"vars: {d2-config: {theme-overrides: {B1: \"#000000\"}}}\na -> b\n": false,
		"a.style.font-size: 20\na -> b\n":                                   true,
		"a -> b\n":                                                          true,
	} {
		src := filepath.Join(dir, "x.d2")
		if err := os.WriteFile(src, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		path, _, err := d2Source(&doc.Fig{SrcPath: src}, amplia(), 0.5, dir)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := os.ReadFile(path)
		if got := strings.Contains(string(raw), "added by mdbrand: the brand's colours"); got != want {
			t.Errorf("%q: brand colours added = %v, want %v", name, got, want)
		}
	}
}

func TestTint(t *testing.T) {
	if tint("000000", 0) != "FFFFFF" || tint("F68E1B", 1) != "F68E1B" || tint("000000", 0.5) != "808080" {
		t.Errorf("tint: %s %s %s", tint("000000", 0), tint("F68E1B", 1), tint("000000", 0.5))
	}
}
