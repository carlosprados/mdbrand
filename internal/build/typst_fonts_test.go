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
