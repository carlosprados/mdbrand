package fig

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/doc"
	"github.com/carlosprados/mdbrand/internal/tex"
)

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

// The size mdbrand injects is the one that lands the labels at the top of the
// brand's band once --scale and the px->pt conversion have been applied. 32 at
// scale 0.5 is the pair the instructions used to ask authors for by hand; it
// has to come out of the arithmetic, not out of a constant.
func TestD2FontPxLandsOnTheTopOfTheBand(t *testing.T) {
	b := brand.Default() // max_text_pt 12, d2_scale 0.5
	if got := d2FontPx(b, b.Diagrams.D2Scale); got != 32 {
		t.Errorf("d2FontPx = %d, want 32 (12pt / (0.5 × 0.75))", got)
	}
	// Halving the scale doubles the source size: same point size on paper.
	if got := d2FontPx(b, 0.25); got != 64 {
		t.Errorf("d2FontPx at scale 0.25 = %d, want 64", got)
	}
	if got := d2FontPx(b, 0); got != 32 {
		t.Errorf("d2FontPx with no scale = %d, want the default scale's 32", got)
	}
}

// The recursive glob `**.style.font-size` also matches the keys inside a d2
// `vars` block, and d2 then refuses the file with `"style" needs a value` —
// naming the glob instead of the cause. One glob per level reaches the same
// shapes without ever descending into a scalar, so the recursive form must not
// come back in the shape selectors.
func TestD2GlobsAvoidTheRecursiveShapeSelector(t *testing.T) {
	g := d2Globs(32)
	for _, ln := range strings.Split(strings.TrimSpace(g), "\n") {
		if strings.HasPrefix(ln, "**") {
			t.Errorf("glob %q is recursive: it would match the keys inside a vars block", ln)
		}
	}
	if !strings.Contains(g, "*.*.*.style.font-size: 32") {
		t.Error("the globs stop short: a shape three containers deep keeps the 16px default")
	}
	// Edges are selected by their own pattern, which carries no risk: an edge
	// cannot be declared inside vars.
	if !strings.Contains(g, "(** -> **)[*].style.font-size: 32") {
		t.Error("edge labels are left at the default size")
	}
}

// Declaring a font size is the author taking the decision back, and mdbrand
// then has nothing to add: it must hand d2 the original file, untouched.
func TestD2SourceLeavesASourceThatSizesItself(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "own.d2")
	if err := os.WriteFile(src, []byte("**.style.font-size: 48\na -> b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &doc.Fig{Kind: "d2", SrcPath: src}
	got, _, err := d2Source(f, brand.Default(), 0.5, dir)
	if err != nil {
		t.Fatal(err)
	}
	// The copy still carries the brand's colours, but not one glob: the
	// author's font size is the author's.
	raw, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "added by mdbrand: label size") {
		t.Errorf("d2Source added font-size globs to a source that sizes itself:\n%s", raw)
	}
	// Kept, but level by level on its own line: `**` would reach the theme's
	// vars block and d2 would refuse the file.
	if !strings.Contains(string(raw), "*.style.font-size: 48; *.*.style.font-size: 48") ||
		strings.Contains(string(raw), "**.style") {
		t.Errorf("the author's own font size is not kept level by level:\n%s", raw)
	}
	if strings.SplitN(string(raw), "\n", 3)[1] != "a -> b" {
		t.Errorf("the expansion moved the author's second line:\n%s", raw)
	}
}

// A trailing comment stays at the end of the line, where it cannot swallow the
// statements written after it; a # inside quotes is a colour. A ** glob that
// opens a block cannot be rewritten on one line and stops by line number.
func TestExpandRecursiveGlobs(t *testing.T) {
	src := "x\n  **.style.stroke: \"#555555\" # grey\n(** -> **)[*].style.font-size: 32\n"
	got, err := expandRecursiveGlobs([]byte(src), "f.d2")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(got), "\n")
	if !strings.HasPrefix(lines[1], `  *.style.stroke: "#555555"; *.*.style.stroke: "#555555";`) ||
		!strings.HasSuffix(lines[1], `"#555555" # grey`) {
		t.Errorf("line 2 = %q", lines[1])
	}
	if lines[2] != "(** -> **)[*].style.font-size: 32" {
		t.Errorf("the edge glob was touched: %q", lines[2])
	}
	if _, err := expandRecursiveGlobs([]byte("a\n**.style: {\n  font-size: 32\n}\n"), "f.d2"); err == nil ||
		!strings.Contains(err.Error(), "f.d2:2:") {
		t.Errorf("a block glob passed, err = %v", err)
	}
}

// A source that imports must be compiled from a copy beside the original: d2
// resolves `...@lib` against the importing file's own directory, so a copy in
// the work directory would fail to find what the original could.
func TestD2SourceKeepsAnImportingSourceBesideItsImports(t *testing.T) {
	docDir, workDir := t.TempDir(), t.TempDir()
	src := filepath.Join(docDir, "main.d2")
	if err := os.WriteFile(src, []byte("...@lib\na -> b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &doc.Fig{Kind: "d2", SrcPath: src}
	got, generated, err := d2Source(f, brand.Default(), 0.5, workDir)
	if err != nil {
		t.Fatal(err)
	}
	if !generated {
		t.Fatal("no copy was generated, so the labels keep d2's 16px default")
	}
	if filepath.Dir(got) != docDir {
		t.Errorf("copy went to %q, want it beside the source in %q", filepath.Dir(got), docDir)
	}
	body, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), "...@lib\na -> b\n") {
		t.Errorf("the author's source was not preserved verbatim at the top:\n%s", body)
	}
	// Appended, not prepended: the line numbers d2 reports in an error still
	// point at the author's own source.
	if !strings.Contains(string(body), "\n*.style.font-size: 32\n") {
		t.Errorf("the globs are missing from the copy:\n%s", body)
	}
	os.Remove(got)
}

// pandoc reads width= in any LaTeX unit or as a share of the measure, and
// mdbrand read only mm: width=10cm or 50% placed the figure at a width
// nobody asked for, with its labels reported at that size.
func TestExplicitWidthReadsUnitsAndRefusesTheRest(t *testing.T) {
	for in, want := range map[string]float64{
		"": 0, "120mm": 120, "10cm": 100, "2in": 50.8, "50%": 83, " 72.27pt ": 25.4,
	} {
		got, err := explicitWidth(&doc.Fig{Attrs: map[string]string{"width": in}}, 166)
		if err != nil || math.Abs(got-want) > 1e-9 {
			t.Errorf("width=%q = %g, %v; want %g", in, got, err, want)
		}
	}
	for _, in := range []string{"400", "400px", "wide", "0mm", "-5%"} {
		if _, err := explicitWidth(&doc.Fig{SrcPath: "a.d2", Attrs: map[string]string{"width": in}}, 166); err == nil {
			t.Errorf("width=%q was accepted", in)
		}
	}
}

// A tall figure at an explicit width is shrunk by LaTeX to the text height,
// labels and all; the build must stop and name the width that fits.
func TestFitsBoxStopsWhatLaTeXWouldShrink(t *testing.T) {
	box := tex.Box{WidthMM: 166, HeightMM: 253}
	tall := &Result{Fig: &doc.Fig{SrcPath: "a.d2", Attrs: map[string]string{"width": "60mm"}},
		WidthMM: 60, HeightMM: 600, TextPt: 10}
	err := fitsBox(tall, box, 100, 1000)
	if err == nil || !strings.Contains(err.Error(), "Lower width= to 25mm (labels at 4.2pt)") {
		t.Errorf("tall: %v", err)
	}
	wide := &Result{Fig: tall.Fig, WidthMM: 180, HeightMM: 50}
	if err := fitsBox(wide, box, 100, 1000); err == nil || !strings.Contains(err.Error(), "wider than the 166 mm") {
		t.Errorf("wide: %v", err)
	}
	slide := tex.Box{WidthMM: 166}
	if err := fitsBox(tall, slide, 100, 1000); err != nil {
		t.Errorf("a slide's height is the frame check's: %v", err)
	}
}

// What d2Source reads off a .d2: sizing and theme as keys, never in a comment
// or a label, and only the references d2 resolves against the file.
func TestD2SourceHeuristics(t *testing.T) {
	sized := d2KeyRe("font-size")
	for src, want := range map[string]bool{
		"a.style.font-size: 20":           true,
		"*.style: {font-size: 20}":        true,
		"# font-size: 20, says a comment": false,
		`a: "font-size"`:                  false,
	} {
		if got := sized.MatchString(src); got != want {
			t.Errorf("sized(%q) = %v", src, got)
		}
	}
	for src, want := range map[string]bool{
		"x: @lib":                          true,
		"...@shared/styles":                true,
		"a: {icon: ./x.png}":               true,
		"a.icon: icons/db.svg":             true,
		"a.icon: https://icons.org/db.svg": false,
		`b: "ops@example.org"`:             false,
		"# mail me@example.org, icon: ./x": false,
	} {
		if got := d2RelativeRe.MatchString(src); got != want {
			t.Errorf("relative(%q) = %v", src, got)
		}
	}
}
