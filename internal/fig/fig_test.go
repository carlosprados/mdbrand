package fig

import "testing"

// The shapes below are what d2 and vl2svg actually emit; the sizing maths is
// built on them, so a change in either tool should break this test rather than
// a document.
const (
	d2Scaled = `<?xml version="1.0" encoding="utf-8"?><svg xmlns="http://www.w3.org/2000/svg" ` +
		`viewBox="0 0 366 638" width="183" height="319"><svg class="d2-svg" width="366" height="638" ` +
		`viewBox="-9 -9 366 638"><text style="font-size:32">Mesa</text></svg></svg>`

	// Without --scale, d2 leaves the root element with a viewBox and no size
	// at all: the case that silently stretched diagrams on the web.
	d2Unsized = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 200">` +
		`<text style="font-size:16">a</text></svg>`

	vega = `<svg xmlns="http://www.w3.org/2000/svg" width="364" height="186" viewBox="0 0 364 186">` +
		`<text font-size="10">Q1</text><text font-size="13">axis</text></svg>`
)

func TestSVGSize(t *testing.T) {
	for _, tc := range []struct {
		name       string
		src        string
		w, h, unit float64
	}{
		{"d2 with scale halves the root", d2Scaled, 183, 319, 0.5},
		{"d2 without scale falls back to the viewBox", d2Unsized, 400, 200, 1},
		{"vega declares both", vega, 364, 186, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, h, unit, err := svgSize(tc.src)
			if err != nil {
				t.Fatalf("svgSize: %v", err)
			}
			if w != tc.w || h != tc.h || unit != tc.unit {
				t.Errorf("got %g×%g unit %g, want %g×%g unit %g", w, h, unit, tc.w, tc.h, tc.unit)
			}
		})
	}
}

func TestSVGSizeRejectsNonsense(t *testing.T) {
	for _, src := range []string{"", "<html></html>", `<svg xmlns="x"></svg>`} {
		if _, _, _, err := svgSize(src); err == nil {
			t.Errorf("svgSize(%q) = nil error, want one", src)
		}
	}
}

func TestMinFontSize(t *testing.T) {
	// The smallest label is what decides legibility, so a chart mixing 10 and
	// 13 must report 10.
	if v, ok := minFontSize(vega); !ok || v != 10 {
		t.Errorf("got %g (%v), want 10", v, ok)
	}
	if _, ok := minFontSize(`<svg width="10" height="10"></svg>`); ok {
		t.Error("reported a font size where there is none")
	}
}

// TestLegibilityMaths pins the conversion the placement depends on: an inner
// font-size of 32 in a root halved by --scale is 16px on the page, which is
// 12pt, and it stays 12pt only while the figure is placed at its natural width.
func TestLegibilityMaths(t *testing.T) {
	w, _, unit, err := svgSize(d2Scaled)
	if err != nil {
		t.Fatal(err)
	}
	font, ok := minFontSize(d2Scaled)
	if !ok {
		t.Fatal("no font size found")
	}
	if got := font * unit * pxToPt; got != 12 {
		t.Errorf("natural label size = %gpt, want 12pt", got)
	}
	naturalMM := w * pxToMM
	if got := naturalMM; got < 48.3 || got > 48.5 {
		t.Errorf("natural width = %.2f mm, want ~48.4 mm", got)
	}
	// Doubling the placed width doubles the printed label.
	if got := font * unit * pxToPt * (2 * naturalMM / naturalMM); got != 24 {
		t.Errorf("label at double width = %gpt, want 24pt", got)
	}
}
