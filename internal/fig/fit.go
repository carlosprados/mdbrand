package fig

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/tex"
)

// Fit places a figure Render has drawn in another box — an e-reader's
// column — by the same sizing the page used. The SVG does not depend on the
// box, so nothing is drawn again and the page's files are left as they are.
// A figure no width keeps legible there is a Note, not an error: the column
// is an assumption, and the page it was drawn for has built.
func Fit(res *Result, b *brand.Brand, box tex.Box) (*Result, error) {
	raw, err := os.ReadFile(res.SVG)
	if err != nil {
		return nil, err
	}
	src := string(raw)
	natW, natH, unit, err := svgSize(src)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(res.SVG), err)
	}
	natWmm, natHmm := natW*pxToMM, natH*pxToMM
	d := b.Diagrams
	if box.HeightMM > 0 && (d.MaxHeightMM <= 0 || box.HeightMM < d.MaxHeightMM) {
		d.MaxHeightMM = box.HeightMM
	}
	// Render has accepted the width already; only the room has changed.
	explicit, _ := explicitWidth(res.Fig, box.WidthMM)
	explicit = math.Min(explicit, box.WidthMM)
	if explicit > 0 && d.MaxHeightMM > 0 && natHmm*explicit/natWmm > d.MaxHeightMM {
		explicit = natWmm * d.MaxHeightMM / natHmm
	}
	out := *res
	out.Note = ""
	minFont, found := minFontSize(src)
	out.WidthMM, out.HeightMM, out.TextPt = place(natWmm, natHmm, minFont*unit*pxToPt, explicit, box.WidthMM, d)
	if found && out.TextPt < d.MinTextPt {
		tall := d.MaxHeightMM > 0 && out.HeightMM >= d.MaxHeightMM-0.5
		var fix string
		switch {
		case tall:
			fix = "it is held by the screen's height: split it in two"
		case res.Fig.Kind == "d2":
			fix = "lay it out taller than wide (direction: down), or split it in two"
		case res.Fig.Kind == "vega":
			fix = "a narrower \"width\" in its spec keeps the labels' size on a narrower chart"
		default:
			fix = "it needs drawing narrower, or in two"
		}
		out.Note = fmt.Sprintf("on an e-reader's %.0f mm column its smallest label prints at %.1fpt, under the %.1fpt floor; the reader can zoom the picture, but %s",
			box.WidthMM, out.TextPt, d.MinTextPt, fix)
	}
	return &out, nil
}

// narrowPt is the label size a chart drawn again for a narrow column is
// aimed at: the middle of the legibility band.
func narrowPt(d brand.Diagrams) float64 { return (d.MinTextPt + d.MaxTextPt) / 2 }

// Narrow fits a figure to a narrower box, and a Vega-Lite chart that would
// print its labels under the floor there is drawn again with a narrower
// "width", so that it takes the column with its labels at their size —
// scaling the A4 drawing down took the text with it. The copy goes to
// <figNN>-<suffix>.svg; the page's files are not touched. A width= on the
// block is the author's, and a chart of several views has no one width to
// change: both are only fitted.
func Narrow(res *Result, b *brand.Brand, box tex.Box, ds *Datasets, suffix string) (*Result, error) {
	fitted, err := Fit(res, b, box)
	if err != nil || fitted.Note == "" || res.Fig.Kind != "vega" || res.Fig.Attrs["width"] != "" {
		return fitted, err
	}
	spec, _, err := VegaSpec(res.Fig, ds)
	if err != nil {
		return nil, err
	}
	setting, known, ok := narrowable(spec)
	if !ok {
		return fitted, nil
	}
	natPx, pt, err := svgWidthAndPt(res.SVG)
	if err != nil || pt <= 0 {
		return fitted, err
	}
	// The width the drawing must have, in its own pixels, for the column to
	// set its labels at narrowPt. A spec's width is the plot's; axes and
	// legend come on top, so the first try takes off what they took before
	// and the next corrects by what it missed.
	targetPx := box.WidthMM / pxToMM * pt / narrowPt(b.Diagrams)
	if !known {
		setting = natPx * 0.75
	}
	out := filepath.Join(filepath.Dir(res.SVG), strings.TrimSuffix(filepath.Base(res.SVG), ".svg")+"-"+suffix+".svg")
	// A chart whose labels and legend alone are wider than the target is
	// drawn with the narrowest plot that still reads, and kept only if that
	// raises its labels: better, if not yet legible.
	const minPlotPx = 120
	w := setting - (natPx - targetPx)
	for range 3 {
		w = math.Max(w, minPlotPx)
		if err := renderVega(res.Fig, b, out, ds, int(math.Round(w))); err != nil {
			return nil, err
		}
		got, _, err := svgWidthAndPt(out)
		if err != nil {
			return nil, err
		}
		if math.Abs(got-targetPx) < 0.02*targetPx || (w == minPlotPx && got > targetPx) {
			break
		}
		w += targetPx - got
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		return nil, err
	}
	src, err := inlineImages(string(raw), res.Fig.BaseDir, filepath.Base(res.Fig.SrcPath))
	if err != nil {
		return nil, err
	}
	if src != string(raw) {
		if err := os.WriteFile(out, []byte(src), 0o644); err != nil {
			return nil, err
		}
	}
	drawn := *res
	drawn.SVG = out
	narrow, err := Fit(&drawn, b, box)
	if err != nil || narrow.TextPt <= fitted.TextPt {
		return fitted, err
	}
	return narrow, nil
}

// svgWidthAndPt is an SVG's width in CSS pixels and its smallest label in
// points at that size.
func svgWidthAndPt(path string) (float64, float64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	w, _, unit, err := svgSize(string(raw))
	if err != nil {
		return 0, 0, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	px, _ := minFontSize(string(raw))
	return w, px * unit * pxToPt, nil
}

// MarkColors are the distinct colours a Vega chart draws its data in: the
// fills and strokes of its marks and legend symbols, not of its text, axes or
// grid. On a grey screen these are what must stay apart.
func MarkColors(svg []byte) ([]string, error) {
	dec := xml.NewDecoder(strings.NewReader(string(svg)))
	var stack []bool // whether each open element is inside data marks
	seen := map[string]bool{}
	var out []string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			inside := len(stack) > 0 && stack[len(stack)-1]
			class := attr(t, "class")
			if t.Name.Local == "g" && strings.Contains(class, "mark-") {
				inside = !strings.Contains(class, "mark-text") &&
					(strings.Contains(class, "role-mark") || strings.Contains(class, "role-legend-symbol"))
			}
			stack = append(stack, inside)
			if !inside || t.Name.Local == "g" {
				continue
			}
			for _, k := range []string{"fill", "stroke"} {
				c := strings.ToUpper(strings.TrimPrefix(attr(t, k), "#"))
				if hexRe6.MatchString(c) && !seen[c] {
					seen[c] = true
					out = append(out, c)
				}
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return out, nil
}

var hexRe6 = regexp.MustCompile(`^[0-9A-F]{6}$`)

func attr(t xml.StartElement, name string) string {
	for _, a := range t.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}
