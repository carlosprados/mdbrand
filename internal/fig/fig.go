// Package fig renders diagram sources to vector PDF and sizes them for paper.
//
// The route is always source -> SVG -> rsvg-convert -> PDF, for both d2 and
// Vega-Lite, and that is a decision with reasons:
//
//   - d2's own PDF export downloads a Playwright driver at run time and the
//     URLs it uses are dead (404), so it fails on a machine that worked
//     yesterday.
//   - vl2pdf writes a page whose points equal the spec's pixels, while every
//     SVG route converts at 96 dpi (1px = 0.75pt). Mixing the two makes two
//     charts with the same spec width come out 33% apart on the page.
//
// Sizing is the part that is easy to get wrong and invisible in the files. A
// figure is placed at the widest size that keeps BOTH bounds: its label text
// must not fall below min_text_pt (illegible) nor rise above max_text_pt
// (a diagram shouting over the body text), and it must not be taller than
// max_height_mm (which is what "it ate the whole page" means).
package fig

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/doc"
	"github.com/carlosprados/mdbrand/internal/run"
)

const (
	pxToPt = 0.75        // 96 dpi CSS pixel
	pxToMM = 25.4 / 96.0 //

)

// Result is what a rendered figure ended up as.
type Result struct {
	Fig      *doc.Fig
	PDF      string  // absolute path
	WidthMM  float64 // as it will be placed
	HeightMM float64
	TextPt   float64 // smallest label on the page, in points
	Note     string  // non-fatal remark worth printing
}

// Render renders one figure and computes its placement. textWidthMM is the
// document's text measure.
func Render(f *doc.Fig, b *brand.Brand, workDir string, textWidthMM float64) (*Result, error) {
	if _, err := os.Stat(f.SrcPath); err != nil {
		return nil, fmt.Errorf("figure %d: %w", f.Index, err)
	}
	svg := filepath.Join(workDir, fmt.Sprintf("fig%02d.svg", f.Index))
	pdf := filepath.Join(workDir, fmt.Sprintf("fig%02d.pdf", f.Index))

	switch f.Kind {
	case "d2":
		if err := renderD2(f, b, svg); err != nil {
			return nil, err
		}
	default:
		if err := renderVega(f, svg); err != nil {
			return nil, err
		}
	}

	raw, err := os.ReadFile(svg)
	if err != nil {
		return nil, err
	}
	src := string(raw)

	// rsvg-convert drops <foreignObject> without a word, so a d2 |md| block
	// disappears from the PDF and nothing says why.
	if strings.Contains(src, "<foreignObject") {
		return nil, fmt.Errorf(`%s renders a <foreignObject>, which rsvg-convert silently drops:
  the node's text would be missing from the PDF and nothing would warn you.
  In d2 this comes from a |md| … | block. Keep diagram labels short and put the
  prose in the document body`, filepath.Base(f.SrcPath))
	}

	natW, natH, unit, err := svgSize(src)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(svg), err)
	}
	natWmm, natHmm := natW*pxToMM, natH*pxToMM

	// Label size at natural size. unit converts inner user units to outer
	// pixels: with `d2 --scale 0.5` the outer <svg> is half the viewBox, so an
	// inline font-size:32 is really 16px on the page.
	res := &Result{Fig: f, PDF: pdf}
	minFont, found := minFontSize(src)
	naturalPt := minFont * unit * pxToPt

	width := textWidthMM
	switch {
	case f.WidthMM() > 0:
		width = f.WidthMM()
	default:
		if found && naturalPt > 0 && b.Diagrams.MaxTextPt > 0 {
			width = math.Min(width, natWmm*(b.Diagrams.MaxTextPt/naturalPt))
		}
		if b.Diagrams.MaxHeightMM > 0 && natHmm > 0 {
			width = math.Min(width, natWmm*(b.Diagrams.MaxHeightMM/natHmm))
		}
	}
	if width <= 0 {
		width = textWidthMM
	}

	res.WidthMM = width
	res.HeightMM = natHmm * (width / natWmm)
	if found {
		res.TextPt = naturalPt * (width / natWmm)
	}

	if err := run.Quiet(workDir, "rsvg-convert", "-f", "pdf", "-o", pdf, svg); err != nil {
		return nil, err
	}

	switch {
	case !found:
		res.Note = "no font-size found in the SVG; legibility not checked"
	case res.TextPt < b.Diagrams.MinTextPt:
		return res, fmt.Errorf(`%s would print its smallest label at %.1fpt, below the %.1fpt floor.
  It is %.0f×%.0f mm on the page. Three fixes, in order of preference:
   1. raise the font size in the source and lower the scale by the same factor,
      so the layout whitespace shrinks and the text returns to reading size:
        d2:   **.style.font-size: 48   with   scale=0.33
        vega: "config": {"axis": {"labelFontSize": 14}}
   2. split the figure along a distinction that matters anyway;
   3. shorten the labels so the layout is less wide`,
			filepath.Base(f.SrcPath), res.TextPt, b.Diagrams.MinTextPt, res.WidthMM, res.HeightMM)
	case res.HeightMM > b.Diagrams.MaxHeightMM && f.WidthMM() > 0:
		res.Note = fmt.Sprintf("%.0f mm tall with an explicit width= — over the %.0f mm guide",
			res.HeightMM, b.Diagrams.MaxHeightMM)
	}
	return res, nil
}

func renderD2(f *doc.Fig, b *brand.Brand, out string) error {
	scale := b.Diagrams.D2Scale
	if s := f.Scale(); s > 0 {
		scale = s
	}
	theme := strconv.Itoa(b.Diagrams.D2Theme)
	// Both themes pinned to the light one: paper has no prefers-color-scheme,
	// so a dark-mode media query inside the SVG would decide the ink colour by
	// the reader's OS at render time and can hand you a dark diagram on white.
	return run.Quiet(filepath.Dir(f.SrcPath), "d2",
		"--theme", theme, "--dark-theme", theme,
		"--pad", strconv.Itoa(b.Diagrams.D2Pad),
		"--scale", strconv.FormatFloat(scale, 'f', -1, 64),
		f.SrcPath, out)
}

func renderVega(f *doc.Fig, out string) error {
	// No dark-mode rules are injected: this SVG is going onto white paper.
	return run.Quiet(filepath.Dir(f.SrcPath), "vl2svg", f.SrcPath, out)
}

var (
	rootRe    = regexp.MustCompile(`<svg\b[^>]*>`)
	attrWRe   = regexp.MustCompile(`\swidth="([0-9.]+)(?:px)?"`)
	attrHRe   = regexp.MustCompile(`\sheight="([0-9.]+)(?:px)?"`)
	viewBoxRe = regexp.MustCompile(`viewBox="\s*(-?[0-9.]+)[\s,]+(-?[0-9.]+)[\s,]+([0-9.]+)[\s,]+([0-9.]+)\s*"`)
	fontRe    = regexp.MustCompile(`font-size\s*[:=]\s*"?\s*([0-9.]+)`)
)

// svgSize returns the root element's size in pixels and the factor that turns
// inner user units into those pixels. d2 without --scale emits a root <svg>
// carrying a viewBox and no width/height at all, so the viewBox is the
// fallback, not an afterthought.
func svgSize(src string) (w, h, unit float64, err error) {
	root := rootRe.FindString(src)
	if root == "" {
		return 0, 0, 0, fmt.Errorf("no <svg> element")
	}
	vb := viewBoxRe.FindStringSubmatch(root)
	if m := attrWRe.FindStringSubmatch(root); m != nil {
		w, _ = strconv.ParseFloat(m[1], 64)
	}
	if m := attrHRe.FindStringSubmatch(root); m != nil {
		h, _ = strconv.ParseFloat(m[1], 64)
	}
	if w == 0 || h == 0 {
		if vb == nil {
			return 0, 0, 0, fmt.Errorf("root <svg> has neither width/height nor viewBox")
		}
		w, _ = strconv.ParseFloat(vb[3], 64)
		h, _ = strconv.ParseFloat(vb[4], 64)
	}
	unit = 1
	if vb != nil {
		if vbw, _ := strconv.ParseFloat(vb[3], 64); vbw > 0 {
			unit = w / vbw
		}
	}
	if w <= 0 || h <= 0 {
		return 0, 0, 0, fmt.Errorf("root <svg> has a non-positive size")
	}
	return w, h, unit, nil
}

// minFontSize returns the smallest font-size declared anywhere in the SVG, in
// user units. The smallest is what decides legibility.
func minFontSize(src string) (float64, bool) {
	min, found := math.MaxFloat64, false
	for _, m := range fontRe.FindAllStringSubmatch(src, -1) {
		v, err := strconv.ParseFloat(m[1], 64)
		if err != nil || v <= 0 {
			continue
		}
		found = true
		if v < min {
			min = v
		}
	}
	if !found {
		return 0, false
	}
	return min, true
}

// Markdown is the include that replaces the figure's placeholder.
func (r *Result) Markdown() string {
	caption := r.Fig.Caption
	return fmt.Sprintf("![%s](%s){width=%.1fmm}", caption, filepath.Base(r.PDF), r.WidthMM)
}
