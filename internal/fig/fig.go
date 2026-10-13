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
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/doc"
	"github.com/carlosprados/mdbrand/internal/imgsize"
	"github.com/carlosprados/mdbrand/internal/run"
	"github.com/carlosprados/mdbrand/internal/tex"
)

const (
	pxToPt = 0.75        // 96 dpi CSS pixel
	pxToMM = 25.4 / 96.0 //

)

// Result is what a rendered figure ended up as.
type Result struct {
	Fig      *doc.Fig
	PDF      string  // absolute path
	SVG      string  // absolute path; the source of every other format
	WidthMM  float64 // as it will be placed
	HeightMM float64
	TextPt   float64 // smallest label on the page, in points
	Note     string  // non-fatal remark worth printing
}

// Render renders one figure and computes its placement. box is the room the
// document gives it; ds supplies a chart's named data, and may be nil.
func Render(f *doc.Fig, b *brand.Brand, workDir string, box tex.Box, ds *Datasets) (*Result, error) {
	if _, err := os.Stat(f.SrcPath); err != nil {
		return nil, fmt.Errorf("figure %d: %w", f.Index, err)
	}
	textWidthMM := box.WidthMM
	explicit, err := explicitWidth(f, textWidthMM)
	if err != nil {
		return nil, err
	}
	svg := filepath.Join(workDir, fmt.Sprintf("fig%02d.svg", f.Index))
	pdf := filepath.Join(workDir, fmt.Sprintf("fig%02d.pdf", f.Index))

	switch f.Kind {
	case "d2":
		if err := renderD2(f, b, svg); err != nil {
			return nil, err
		}
	case "svg":
		// Already rendered: nothing to compile, everything else still applies.
		raw, err := os.ReadFile(f.SrcPath)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(svg, raw, 0o644); err != nil {
			return nil, err
		}
	default:
		if err := renderVega(f, b, svg, ds); err != nil {
			return nil, err
		}
	}

	raw, err := os.ReadFile(svg)
	if err != nil {
		return nil, err
	}
	src, err := inlineImages(string(raw), f.BaseDir, filepath.Base(f.SrcPath))
	if err != nil {
		return nil, err
	}
	if src != string(raw) {
		if err := os.WriteFile(svg, []byte(src), 0o644); err != nil {
			return nil, err
		}
	}

	// rsvg-convert drops <foreignObject> without a word, so a d2 |md| block
	// disappears from the PDF and nothing says why.
	if strings.Contains(src, "<foreignObject") {
		return nil, fmt.Errorf(`%s renders a <foreignObject>, which rsvg-convert silently drops:
  the node's text would be missing from the PDF and nothing would warn you.
  In d2 this comes from a |md| … | block. Keep diagram labels short and put the
  prose in the document body`, filepath.Base(f.SrcPath))
	}
	if err := RefuseMasks(src, filepath.Base(f.SrcPath)); err != nil {
		return nil, err
	}

	natW, natH, unit, err := svgSize(src)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(svg), err)
	}
	natWmm, natHmm := natW*pxToMM, natH*pxToMM

	// Label size at natural size. unit converts inner user units to outer
	// pixels: with `d2 --scale 0.5` the outer <svg> is half the viewBox, so an
	// inline font-size:32 is really 16px on the page.
	res := &Result{Fig: f, PDF: pdf, SVG: svg}
	minFont, found := minFontSize(src)
	naturalPt := minFont * unit * pxToPt

	width := textWidthMM
	switch {
	case explicit > 0:
		width = explicit
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
	if explicit > 0 {
		if err := fitsBox(res, box, natWmm, natHmm); err != nil {
			return nil, err
		}
	}

	if err := run.Quiet(workDir, "rsvg-convert", "-f", "pdf", "-o", pdf, svg); err != nil {
		return nil, err
	}

	switch {
	case !found:
		res.Note = "no font-size found in the SVG; legibility not checked"
	case res.TextPt < b.Diagrams.MinTextPt:
		tall := b.Diagrams.MaxHeightMM > 0 && res.HeightMM >= b.Diagrams.MaxHeightMM-0.5
		held := ""
		if tall {
			held = fmt.Sprintf(", held there by the %.0f mm height limit", b.Diagrams.MaxHeightMM)
		}
		return res, fmt.Errorf("%s would print its smallest label at %.1fpt, below the %.1fpt floor.\n"+
			"  It is %.0f×%.0f mm on the page%s. In order of preference:\n%s",
			filepath.Base(f.SrcPath), res.TextPt, b.Diagrams.MinTextPt, res.WidthMM, res.HeightMM,
			held, illegibleFixes(f.Kind, tall))
	case res.HeightMM > b.Diagrams.MaxHeightMM && explicit > 0:
		res.Note = fmt.Sprintf("%.0f mm tall with an explicit width= — over the %.0f mm guide",
			res.HeightMM, b.Diagrams.MaxHeightMM)
	}
	return res, nil
}

// explicitWidth reads a width= the author wrote, in mm, or 0 when there is
// none. A percentage is of the measure, as pandoc reads it. Anything else is
// refused: a figure placed at a width nobody asked for is the defect.
func explicitWidth(f *doc.Fig, textWidthMM float64) (float64, error) {
	v := strings.TrimSpace(f.Attrs["width"])
	if v == "" {
		return 0, nil
	}
	var w float64
	var err error
	if pc, ok := strings.CutSuffix(v, "%"); ok {
		w, err = strconv.ParseFloat(strings.TrimSpace(pc), 64)
		w *= textWidthMM / 100
	} else {
		w, err = tex.ParseLenMM(v)
	}
	if err != nil || w <= 0 {
		return 0, fmt.Errorf("%s: width=%s is not a width mdbrand can place: write it in mm,\n"+
			"  cm, in or pt, or as a share of the text measure (width=80%%) — or drop it\n"+
			"  and let mdbrand size the figure for legible labels", filepath.Base(f.SrcPath), v)
	}
	return w, nil
}

// fitsBox stops a figure that an explicit width= puts outside the room it has.
// Wider than the measure, it prints into the margin. Taller than the text
// height, pandoc's LaTeX scales it down to fit — height=\textheight with
// keepaspectratio — and its labels shrink with it while the report quotes
// the size before.
func fitsBox(res *Result, box tex.Box, natWmm, natHmm float64) error {
	name := filepath.Base(res.Fig.SrcPath)
	if res.WidthMM > box.WidthMM+0.5 {
		return fmt.Errorf("%s: width=%s is %.0f mm, wider than the %.0f mm text measure: it would\n"+
			"  print into the margin. Lower it to %.0fmm, or drop width= and let mdbrand size it",
			name, res.Fig.Attrs["width"], res.WidthMM, box.WidthMM, math.Floor(box.WidthMM))
	}
	if box.HeightMM > 0 && res.HeightMM > box.HeightMM+0.5 {
		fit := math.Floor(natWmm * box.HeightMM / natHmm)
		msg := fmt.Sprintf("%s: at width=%s it is %.0f mm tall, taller than the %.0f mm of text a\n"+
			"  page holds: LaTeX would shrink it to fit, labels and all, and say nothing.\n"+
			"  Lower width= to %.0fmm", name, res.Fig.Attrs["width"], res.HeightMM, box.HeightMM, fit)
		if res.TextPt > 0 {
			msg += fmt.Sprintf(" (labels at %.1fpt)", res.TextPt*fit/res.WidthMM)
		}
		return fmt.Errorf("%s, or lay it out wider than tall", msg)
	}
	return nil
}

// illegibleFixes is the advice for a figure whose labels fall under the
// floor, by what the figure is and by what held it back. A figure held by
// the height limit — the usual one on a 16:9 slide — needs to be wider than
// tall, and shrinking its whitespace does nothing for that; one held by the
// width needs packing. Vega-Lite has neither a scale nor a layout engine.
func illegibleFixes(kind string, tall bool) string {
	switch {
	case kind == "d2" && tall:
		return `   1. lay it out wider than tall: direction: right at the root of the .d2
      (inside a container d2 ignores it);
   2. split the figure along a distinction that matters anyway;
   3. shorten the labels`
	case kind == "d2":
		return `   1. lower the scale: scale=0.33 on the fence. mdbrand raises the source font
      size by the same factor, so what shrinks is the layout whitespace and the
      text comes back to reading size;
   2. drop the labels on the edges you can: a single one has widened a
      dagre layout enough to take its labels from 12pt to under 8pt;
   3. try the other layout engine, in the .d2 itself:
        vars: { d2-config: { layout-engine: elk } }
      ELK often packs a graph tighter than dagre, at the cost of routing the
      edges differently — look at the result before keeping it;
   4. split the figure along a distinction that matters anyway, or shorten
      the labels so the layout is less wide`
	case tall:
		return `   1. make the chart wider than tall: lower "height" and raise "width" in the
      spec, so the frame's height stops being the limit;
   2. raise the label sizes in the spec:
        "config": {"axis": {"labelFontSize": 14, "titleFontSize": 14}}`
	default:
		return `   1. raise the label sizes in the spec:
        "config": {"axis": {"labelFontSize": 14, "titleFontSize": 14},
                   "legend": {"labelFontSize": 14}}
   2. plot fewer categories, or shorten their names`
	}
}

func renderD2(f *doc.Fig, b *brand.Brand, out string) error {
	scale := b.Diagrams.D2Scale
	if s := f.Scale(); s > 0 {
		scale = s
	}
	src, generated, err := d2Source(f, b, scale, filepath.Dir(out))
	if err != nil {
		return err
	}
	if generated {
		defer os.Remove(src)
	}
	theme := strconv.Itoa(b.Diagrams.D2Theme)
	// Both themes pinned to the light one: paper has no prefers-color-scheme,
	// so a dark-mode media query inside the SVG would decide the ink colour by
	// the reader's OS at render time and can hand you a dark diagram on white.
	return run.Quiet(filepath.Dir(f.SrcPath), "d2",
		"--theme", theme, "--dark-theme", theme,
		"--pad", strconv.Itoa(b.Diagrams.D2Pad),
		"--scale", strconv.FormatFloat(scale, 'f', -1, 64),
		src, out)
}

// How deep the shape globs reach. One selector per level, because the
// recursive form cannot be used — see d2Globs.
const d2MaxDepth = 6

// d2Globs is the font-size declaration mdbrand appends to a d2 source.
//
// d2 defaults labels to 16px, which the render scale then halves: 8px on the
// page, and a diagram nobody reads. The size has to be set in the source, since
// d2 lays the boxes out around the text it was given and no CLI flag reaches
// it. It used to be the author's job — two glob lines, pasted into every .d2 by
// hand — and it is the tool's now, because the tool is what knows the point
// size the label ends up at, having measured it.
//
// One glob per level of nesting rather than the recursive `**.style.font-size`:
// the recursive form also matches the keys inside a `vars` block, and d2 then
// refuses the whole file with `"style" needs a value`, an error that names the
// glob and not the cause. `*.*.…` reaches the same shapes and never descends
// into a scalar. Edges keep their own recursive selector, which is safe: an
// edge cannot be declared inside vars.
//
// Appended, not prepended, so the line numbers d2 reports in an error still
// point at the author's own source.
func d2Globs(px int) string {
	var b strings.Builder
	b.WriteString("\n# --- added by mdbrand: label size for print legibility ---\n")
	sel := "*"
	for i := 0; i < d2MaxDepth; i++ {
		fmt.Fprintf(&b, "%s.style.font-size: %d\n", sel, px)
		sel += ".*"
	}
	fmt.Fprintf(&b, "(** -> **)[*].style.font-size: %d\n", px)
	return b.String()
}

// d2FontPx is the source size that puts the labels at the top of the brand's
// legible band once --scale has been applied and the SVG's pixels have become
// points: the widest the tool may make them, since Render only ever shrinks a
// figure from there.
func d2FontPx(b *brand.Brand, scale float64) int {
	if scale <= 0 {
		scale = 0.5
	}
	px := int(math.Round(b.Diagrams.MaxTextPt / (scale * pxToPt)))
	if px < 8 {
		px = 8
	}
	return px
}

// d2Source returns the path to hand d2, writing a copy carrying the injected
// globs unless the source sets a font size of its own — declaring one is the
// author taking the decision back.
//
// The copy goes beside the original when the source imports anything, because
// d2 resolves `@import` against the importing file's own directory and a copy
// in the work directory would break it. Everything else is generated in the
// work directory, where a read-only document tree cannot stop the build.
func d2Source(f *doc.Fig, b *brand.Brand, scale float64, workDir string) (path string, generated bool, err error) {
	raw, err := os.ReadFile(f.SrcPath)
	if err != nil {
		return "", false, err
	}
	sized := d2KeyRe("font-size").Match(raw)
	themed := d2KeyRe("theme-overrides").Match(raw)
	// A relative import or icon resolves against the file d2 compiles, so
	// that file must sit where its author wrote it: beside a side file, and
	// beside the document for a fenced block, whose source is in the work
	// directory — icon: ./x.png in a fenced block was looked for there.
	relative := d2RelativeRe.Match(raw)
	base := f.BaseDir
	if base == "" {
		base = filepath.Dir(f.SrcPath)
	}
	if sized && themed && (!relative || base == filepath.Dir(f.SrcPath)) {
		return f.SrcPath, false, nil
	}
	dir := workDir
	if relative {
		dir = base
	}
	gen := filepath.Join(dir, fmt.Sprintf(".mdbrand-fig%02d.d2", f.Index))
	body := bytes.TrimRight(raw, "\n")
	if !sized {
		body = append(body, []byte(d2Globs(d2FontPx(b, scale)))...)
	}
	// Colours likewise: a .d2 with theme-overrides of its own has taken the
	// decision back, and an appended block would override it.
	if !themed {
		if body, err = expandRecursiveGlobs(body, f.SrcPath); err != nil {
			return "", false, err
		}
		body = append(body, []byte(d2Theme(b))...)
	}
	if err := os.WriteFile(gen, body, 0o644); err != nil {
		return "", false, fmt.Errorf("writing the sized copy of %s: %w", filepath.Base(f.SrcPath), err)
	}
	return gen, true, nil
}

// d2KeyRe finds key used as a key on a line that is not a comment: the
// substring alone, in a comment or a label, turned the sizing off.
func d2KeyRe(key string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^[^#\n]*\b` + regexp.QuoteMeta(key) + `\s*:`)
}

// d2RelativeRe finds what d2 resolves against the compiled file's directory:
// an import (x: @lib, ...@lib) or an icon that is not a URL. Any @ used to
// count — an e-mail address in a label wrote a file beside the source.
var d2RelativeRe = regexp.MustCompile(`(?m)^[^#\n]*(?::\s*|\.\.\.)@[\w./"'-]|^[^#\n]*\bicon\s*:\s*["']?(?:\.{1,2}/|[\w-]+[/.])`)

// recursiveGlobRe is a shape glob that descends without limit: `**.style…`.
// The edge form `(** -> **)` starts with a parenthesis and is not matched.
var recursiveGlobRe = regexp.MustCompile(`^([ \t]*)\*\*\.(.*)$`)

// expandRecursiveGlobs rewrites each `**.x: v` line as `*.x: v; *.*.x: v; …`,
// one statement per level, on the same line. `**` also matches the keys of
// the vars block the brand's theme appends, and d2 then refuses the file with
// `"style" needs a value`: the luchando.org essays, whose .d2 files carry the
// glob lines the README once asked for, stopped building when figures took
// the brand's colours. The levels reach what `**` reached for every diagram
// d2MaxDepth covers, and the same line keeps d2's line numbers the author's.
func expandRecursiveGlobs(src []byte, name string) ([]byte, error) {
	lines := strings.Split(string(src), "\n")
	for i, ln := range lines {
		m := recursiveGlobRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		stmt, comment := splitD2Comment(m[2])
		if strings.HasSuffix(strings.TrimSpace(stmt), "{") {
			return nil, fmt.Errorf(`%s:%d: a **. glob that opens a block cannot be kept: d2 applies **
to the vars block mdbrand appends for the brand's colours and refuses the
file. Write the properties on one line each (**.style.font-size: 32), or
delete the block if it only sets font-size: mdbrand sizes labels itself`,
				filepath.Base(name), i+1)
		}
		var b strings.Builder
		b.WriteString(m[1])
		sel := "*"
		for level := 0; level < d2MaxDepth; level++ {
			if level > 0 {
				b.WriteString("; ")
			}
			b.WriteString(sel + "." + strings.TrimRight(stmt, " \t"))
			sel += ".*"
		}
		b.WriteString(comment)
		lines[i] = b.String()
	}
	return []byte(strings.Join(lines, "\n")), nil
}

// splitD2Comment separates a trailing `# comment`, which would otherwise
// swallow every statement written after it on the line. A # inside quotes is
// a colour, not a comment.
func splitD2Comment(s string) (stmt, comment string) {
	quoted := false
	for i, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
		case r == '#' && !quoted && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t'):
			return s[:i], " " + s[i:]
		}
	}
	return s, ""
}

var fontRe = regexp.MustCompile(`font-size\s*[:=]\s*"?\s*([0-9.]+)`)

// svgSize returns the root element's size in pixels and the factor that turns
// inner user units into those pixels. The measurement itself lives in imgsize,
// because page geometry needs the same numbers for a logo and two copies of
// this regex would drift apart.
func svgSize(src string) (w, h, unit float64, err error) {
	return imgsize.SVG(src)
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

// MarkdownBox is Markdown with the height written out as well, for a writer
// that would otherwise fit the figure to a box of its own.
func (r *Result) MarkdownBox() string {
	return fmt.Sprintf("![%s](%s){width=%.1fmm height=%.1fmm}", r.Fig.Caption, filepath.Base(r.PDF), r.WidthMM, r.HeightMM)
}
