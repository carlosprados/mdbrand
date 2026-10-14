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

// An e-reader's column: the same sizing in a smaller box, the SVG untouched,
// and a figure no width keeps legible a note rather than an error.
func TestFit(t *testing.T) {
	dir := t.TempDir()
	// 160 × 40 mm with 10pt labels: 604.72 × 151.18 px, 13.33 px text.
	svg := filepath.Join(dir, "fig00.svg")
	src := `<svg xmlns="http://www.w3.org/2000/svg" width="604.72" height="151.18"><text font-size="13.333">a</text></svg>`
	if err := os.WriteFile(svg, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	b := brand.Default()
	b.Diagrams = brand.Diagrams{MinTextPt: 8, MaxTextPt: 12, MaxHeightMM: 200}
	box := tex.Box{WidthMM: 80, HeightMM: 100}
	for _, tc := range []struct {
		name  string
		width string
		wantW float64
		note  bool
	}{
		{"wide labels fall under the floor", "", 80, true},
		{"an A4 width is held to the column", "150mm", 80, true},
		{"a narrow width is kept", "20mm", 20, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &doc.Fig{Kind: "d2", SrcPath: "x.d2", Attrs: map[string]string{"width": tc.width}}
			res, err := Fit(&Result{Fig: f, SVG: svg, WidthMM: 160, Note: "the page's"}, b, box)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(res.WidthMM-tc.wantW) > 0.01 || (res.Note != "") != tc.note {
				t.Errorf("%.1f mm, note %q; want %.0f mm, note %v", res.WidthMM, res.Note, tc.wantW, tc.note)
			}
		})
	}
	raw, _ := os.ReadFile(svg)
	if string(raw) != src {
		t.Error("Fit changed the SVG the page uses")
	}
}

// Only the data's colours count: Vega's text, axes and grid are drawn in the
// brand's text and rule colours, which are not series.
func TestMarkColors(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg">
<g class="mark-group role-axis"><g class="mark-rule role-axis-domain"><line stroke="#888888"/></g>
<g class="mark-text role-axis-label"><text fill="#5d6266">a</text></g></g>
<g class="mark-rect role-mark marks"><path fill="#F68E1B"/><path fill="#5D6266"/><path fill="#f68e1b"/></g>
<g class="mark-text role-mark marks"><text fill="#000000">5</text></g>
<g class="mark-symbol role-legend-symbol"><path fill="#1565C0" stroke="#1565c0"/></g>
</svg>`
	got, err := MarkColors([]byte(svg))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, " ") != "F68E1B 5D6266 1565C0" {
		t.Errorf("mark colours = %v", got)
	}
}

// Only a spec with one width to change is drawn again for a narrow column.
func TestNarrowable(t *testing.T) {
	for spec, want := range map[string]bool{
		`{"width": 260, "mark": "bar"}`:         true,
		`{"mark": "bar"}`:                       true,
		`{"width": "container", "mark": "bar"}`: false,
		`{"hconcat": [{"mark": "bar"}]}`:        false,
		`{"facet": {"field": "a"}, "spec": {}}`: false,
		`{"repeat": ["a"], "spec": {}}`:         false,
	} {
		if _, _, ok := narrowable([]byte(spec)); ok != want {
			t.Errorf("%s: narrowable %v, want %v", spec, ok, want)
		}
	}
	out, err := withWidth([]byte(`{"width": 260, "height": 300.5}`), 120)
	if err != nil || string(out) != `{"height":300.5,"width":120}` {
		t.Errorf("withWidth = %s, %v", out, err)
	}
}
