package build

import (
	"slices"
	"testing"

	"github.com/carlosprados/mdbrand/internal/brand"
)

// The listing's shape, as typst 0.15 prints it; the paths are invented and do
// not exist, so they are kept as written.
const typstListing = `MdbrandTestFace
  ├ /fonts/MdbrandTestFace-Italic.otf
  │   Style: Italic, Weight: 400, Stretch: 100%
  ├ /fonts/MdbrandTestFace-Regular.otf
  │   Style: Normal, Weight: 400, Stretch: 100%
  └ /fonts/MdbrandTestFace-Bold.otf
      Style: Normal, Weight: 700, Stretch: 100%

MdbrandTestSymbols
  └ /fonts/MdbrandTestSymbols-Regular.otf
      Style: Normal, Weight: 400, Stretch: 100%
`

func TestParseTypstFontsStyles(t *testing.T) {
	seen := parseTypstFonts(typstListing)
	if v := seen["/fonts/MdbrandTestFace-Italic.otf"]; v.family != "MdbrandTestFace" || !v.italic || v.weight != 400 {
		t.Errorf("italic face read as %+v", v)
	}
	if got := familyStyles(seen, "MdbrandTestFace"); !slices.Contains(got, "bold") || !slices.Contains(got, "italic") {
		t.Errorf("MdbrandTestFace styles = %v, want bold and italic", got)
	}
	if got := familyStyles(seen, "MdbrandTestSymbols"); len(got) != 0 {
		t.Errorf("a one-face family has styles %v", got)
	}
}

// A character the body lacks goes to the fallback; in bold, italic or a bold
// heading, a fallback with one face printed it in the regular at exit 0.
func TestFallbackStyleLosses(t *testing.T) {
	body, _ := brand.ParseCharset("20-7e")
	fb, _ := brand.ParseCharset("20-7e 26d4 2234")
	out := "MDBRAND-STYLED heading ⛔ Revision\n" +
		"MDBRAND-STYLED bold plain words\n" +
		"MDBRAND-STYLED italic therefore ∴\n"

	got := fallbackStyleLosses(out, body, fb, nil, 700)
	want := []string{"bold, for ⛔ (U+26D4)", "italic, for ∴ (U+2234)"}
	if !slices.Equal(got, want) {
		t.Errorf("one-face fallback: %q, want %q", got, want)
	}
	// A display bold that asks for less than 600 is the fallback's regular.
	if got := fallbackStyleLosses(out, body, fb, []string{"italic"}, 400); len(got) != 0 {
		t.Errorf("light heading, italic fallback: %q, want nothing", got)
	}
	if got := fallbackStyleLosses(out, body, fb, []string{"bold", "italic"}, 700); len(got) != 0 {
		t.Errorf("fallback with both styles: %q, want nothing", got)
	}
}

// The oldest pandoc the Typst engine accepts: 3.6 wrote citations Typst 0.15
// refuses, and an unread minor ("3.10") must not read as smaller than 3.7.
func TestVersionAtLeast(t *testing.T) {
	for v, want := range map[string]bool{
		"3.7": true, "3.7.0.2": true, "3.8.3": true, "3.10": true, "4.0": true,
		"3.6.4": false, "3.1.3": false, "3": false, "2.19.2": false,
	} {
		if got := versionAtLeast(v, typstPandoc); got != want {
			t.Errorf("versionAtLeast(%q) = %v, want %v", v, got, want)
		}
	}
}

// fontsize is honoured; a variable only XeLaTeX's template reads stops a
// Typst build rather than be dropped. Under thesis mdbrand sets the size and
// the class itself on both paths, so the document's are not the issue there.
func TestTypstFontSize(t *testing.T) {
	cases := []struct {
		front, style string
		size         float64
		fails        bool
	}{
		{"title: x", "report", 10, false},
		{"fontsize: 12pt", "note", 12, false},
		{"fontsize: 11pt\nmonofont: DejaVu Sans Mono", "report", 11, false},
		{"fontsize: 12", "report", 0, true},
		{"fontsize: 30pt", "report", 0, true},
		{"fontsize: 12pt", "slides", 0, true},
		{"fontsize: 12pt\ndocumentclass: book", "thesis", 10, false},
		{"monofont: Fira Code", "report", 0, true},
		{"indent: true", "letter", 0, true},
	}
	for _, c := range cases {
		size, err := typstFontSize(c.front, c.style)
		if (err != nil) != c.fails || (!c.fails && size != c.size) {
			t.Errorf("%s %q: size %v, err %v; want %v, fails %v", c.style, c.front, size, err, c.size, c.fails)
		}
	}
}
