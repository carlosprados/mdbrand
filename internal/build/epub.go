package build

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/carlosprados/mdbrand/internal/epub"
	"github.com/carlosprados/mdbrand/internal/fig"
	"github.com/carlosprados/mdbrand/internal/run"
	"github.com/carlosprados/mdbrand/internal/tex"
)

// renderEPUB writes the document for an e-reader: pandoc's epub3, with the
// metadata it gets wrong written by mdbrand, figures placed again for a 6"
// column and rasterised for it, and the package validated by epubcheck.
func renderEPUB(p *prepared, out string, inputs *[]string) error {
	o, d, b, work, rep := p.o, p.d, p.b, p.work, p.rep

	body := p.body
	if pic := pdfPictureRe.FindStringSubmatch(body); pic != nil {
		return fmt.Errorf(`%s: the picture %s is a PDF, and an .epub cannot hold one.
Export it as SVG, which mdbrand sizes like a figure, or as PNG`, o.Input, pic[1])
	}
	body, err := epubFigures(p, body)
	if err != nil {
		return err
	}

	// The front matter goes as written: pandoc's title page prints the
	// abstract under its own localised heading, where the body's copy the
	// PDF uses would be a chapter titled with the document's title.
	stem := workStem(o.Input)
	mdName := stem + ".epub.md"
	var md strings.Builder
	if d.FrontMatter != "" {
		md.WriteString("---\n" + d.FrontMatter + "\n---\n\n")
	}
	md.WriteString(body)
	if err := os.WriteFile(filepath.Join(work, mdName), []byte(md.String()), 0o644); err != nil {
		return err
	}
	meta := epub.Metadata(epub.Meta{Input: mustAbs(o.Input), Date: d.Meta.Date, Now: time.Now()})
	css := epub.CSS(epub.Look{Accent: b.Colors.Accent, Link: b.Colors.Link, Rule: b.Colors.Rule})
	for name, content := range map[string]string{"epub-metadata.xml": meta, "epub.css": css} {
		if err := os.WriteFile(filepath.Join(work, name), []byte(content), 0o644); err != nil {
			return err
		}
	}

	if err := run.Quiet(work, "pandoc", "-o", "pandoc-epub.css", "--print-default-data-file", "epub.css"); err != nil {
		return err
	}
	filter, err := writeSpanFilter(work, b)
	if err != nil {
		return err
	}
	built := filepath.Join(work, stem+".epub")
	args := []string{
		mdName, "-t", "epub3", "-o", built, filter,
		"--epub-metadata=epub-metadata.xml", "--css=pandoc-epub.css", "--css=epub.css", "--mathml",
		resourcePath(work, o.Input),
		// The reader's own contents menu is the nav; a printed page of it
		// is one more page to turn past.
		"-M", "toc=false",
	}
	if d.Meta.NumberSections {
		args = append(args, "--number-sections")
	}
	cites, err := citeArgs(d, o.Input, inputs)
	if err != nil {
		return err
	}
	args = append(args, cites...)
	o.logf("  pandoc %s", mdName)
	pandocOut, err := run.Cmd(work, "pandoc", args...)
	if err != nil {
		return err
	}
	if err := citationProblems(d.Meta, pandocOut, "document"); err != nil {
		return err
	}
	if err := filterProblems(pandocOut); err != nil {
		return err
	}
	if err := pandocWarnings(pandocOut, ".epub", rep); err != nil {
		return err
	}
	epubWidths(pandocOut, rep)
	if !p.pdfToo {
		if err := accentContrast(b, pandocOut, false, rep); err != nil {
			return err
		}
	}
	raw, err := os.ReadFile(built)
	if err != nil {
		return err
	}
	pkg, err := epub.Repair(raw)
	if err != nil {
		return err
	}
	if err := os.WriteFile(built, pkg, 0o644); err != nil {
		return err
	}
	if math, err := epub.HasMath(pkg); err != nil {
		return err
	} else if math {
		rep.Warnings = append(rep.Warnings,
			"the formulas are written as MathML, which not every reader draws: Adobe's RMSDK engine, common on e-ink readers, "+
				"and older Kobos show them as plain text. Open the .epub on the reader before relying on them")
	}

	o.logf("  epubcheck %s", filepath.Base(built))
	if err := epubCheck(work, built, rep); err != nil {
		return err
	}
	rep.Outputs = append(rep.Outputs, Output{Format: "epub", Path: out, built: built})
	return nil
}

// epubFigures places every figure again for the e-reader's column and puts
// its picture, rasterised for that size, in the body.
func epubFigures(p *prepared, body string) (string, error) {
	b, work, rep := p.b, p.work, p.rep
	box := tex.Box{WidthMM: epub.BoxWidthMM, HeightMM: epub.BoxHeightMM}
	var small []*fig.Result
	var grey []string
	for i, f := range p.figs {
		res, err := fig.Narrow(p.results[i], b, box, Datasets(p.store, p.d.Meta.Lang), "epub")
		if err != nil {
			return "", err
		}
		if !p.pdfToo {
			// The sizes reported are the ones this file has.
			rep.Figures[i] = res
		}
		name := filepath.Base(f.SrcPath)
		if res.Note != "" {
			small = append(small, res)
		}
		if f.Kind == "vega" {
			clash, err := greySeries(res.SVG, name)
			if err != nil {
				return "", err
			}
			if clash != "" {
				grey = append(grey, clash)
			}
		}
		png := fmt.Sprintf("fig%02d-epub.png", f.Index)
		px := strconv.Itoa(int(res.WidthMM/25.4*epub.PPI + 0.5))
		if err := run.Quiet(work, "rsvg-convert", "-w", px, "-o", png, res.SVG); err != nil {
			return "", err
		}
		body = strings.Replace(body, f.Placeholder,
			fmt.Sprintf("![%s](%s){width=%.1fmm}", res.Fig.Caption, png, res.WidthMM), 1)
	}

	smallFigures(small, b.Diagrams.MinTextPt, rep)
	if len(grey) > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf(
			"%d chart(s) draw two series in colours an e-ink screen shows as one grey, under %.0f L* apart: %s. "+
				"Pick colours apart in lightness — order figures.palette in the bundle by it, where the chart takes the brand's — "+
				"or tell the series apart by shape or strokeDash too",
			len(grey), epub.MinDeltaL, strings.Join(grey, "; ")))
	}

	return body, nil
}

// epubCheck validates the package. pandoc writes an invalid one at exit 0 —
// an empty <dc:date> for a Spanish date was how this was found — so an error
// stops the build, by its code; a warning is reported.
func epubCheck(work, built string, rep *Report) error {
	out, runErr := run.Cmd(work, "epubcheck", "--quiet", built)
	var errs []string
	for _, p := range epub.Problems(out) {
		line := fmt.Sprintf("%s %s", p.Code, p.Text)
		if p.Severity == "WARNING" {
			rep.Warnings = append(rep.Warnings, "epubcheck: "+line)
			continue
		}
		errs = append(errs, line)
	}
	if len(errs) > 0 {
		return fmt.Errorf("epubcheck found the .epub invalid, and a reader may refuse it or show it wrong:\n  %s\nThis is mdbrand's defect, not the document's, unless the message names something it wrote: report it at https://github.com/carlosprados/mdbrand/issues",
			strings.Join(errs, "\n  "))
	}
	// A failure with nothing parsed is the tool's own: no Java, a broken jar.
	return runErr
}

var (
	epubCodeRe  = regexp.MustCompile(`MDBRAND-EPUB-CODE (\d+)\t([^\n]*)`)
	epubTableRe = regexp.MustCompile(`MDBRAND-EPUB-TABLE ([\d,]*)\t([^\n]*)`)
)

// epubWidths warns of code and tables wider than a 6" column. Both are
// estimates — the reader picks the size — so neither stops the build.
func epubWidths(pandocOut string, rep *Report) {
	var wide []string
	for _, m := range epubCodeRe.FindAllStringSubmatch(pandocOut, -1) {
		if n, _ := strconv.Atoi(m[1]); n > epub.CodeColumns {
			wide = append(wide, fmt.Sprintf("%q (%d columns)", ellipsize(m[2], 40), n))
		}
	}
	if len(wide) > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf(
			"%d code block(s) wider than the %d columns a 6\" e-reader holds will wrap there: %s. Shorten the lines if the layout matters",
			len(wide), epub.CodeColumns, strings.Join(wide, ", ")))
	}
	var tables []string
	for _, m := range epubTableRe.FindAllStringSubmatch(pandocOut, -1) {
		var longest []int
		for _, c := range strings.Split(m[1], ",") {
			n, _ := strconv.Atoi(c)
			longest = append(longest, n)
		}
		if w := epub.TableWidthMM(longest); w > epub.BoxWidthMM {
			name := "one without a caption or header"
			if m[2] != "" {
				name = strconv.Quote(ellipsize(m[2], 40))
			}
			tables = append(tables, fmt.Sprintf("%s (%.0f mm)", name, math.Ceil(w)))
		}
	}
	if len(tables) > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf(
			"%d table(s) need more than a 6\" e-reader's %.0f mm for their longest words, which will be broken to fit: %s. Fewer columns, or the table transposed",
			len(tables), epub.BoxWidthMM, strings.Join(tables, ", ")))
	}
}

// smallFigures warns of figures whose labels fall under the floor on the
// e-reader's column: one by its own advice, several in one line.
func smallFigures(small []*fig.Result, floor float64, rep *Report) {
	switch len(small) {
	case 0:
		return
	case 1:
		rep.Warnings = append(rep.Warnings, filepath.Base(small[0].Fig.SrcPath)+": "+small[0].Note)
		return
	}
	names := make([]string, len(small))
	for i, r := range small {
		names[i] = fmt.Sprintf("%s %.1fpt", filepath.Base(r.Fig.SrcPath), r.TextPt)
	}
	rep.Warnings = append(rep.Warnings, fmt.Sprintf(
		"%d figures print their smallest label under the %.1fpt floor on an e-reader's %.0f mm column: %s. "+
			"The reader can zoom a picture; to fix one, lay a d2 diagram out taller than wide (direction: down), "+
			"give a chart a narrower \"width\" in its spec, or draw an SVG narrower",
		len(small), floor, epub.BoxWidthMM, strings.Join(names, ", ")))
}

// greySeries names the two colours of a chart a grey screen draws as one,
// or returns "". The colours are read off the drawing, so they are the ones
// it uses; that the reader's screen is grey is the assumption, so it warns.
func greySeries(svg, name string) (string, error) {
	raw, err := os.ReadFile(svg)
	if err != nil {
		return "", err
	}
	colors, err := fig.MarkColors(raw)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	// A colour scale of more is a gradient, read by its legend, not by
	// telling two series apart.
	if len(colors) > 10 {
		return "", nil
	}
	c, ok := epub.GreyClash(colors)
	if !ok {
		return "", nil
	}
	return fmt.Sprintf("%s #%s and #%s (L* %.0f and %.0f)", name, c.A, c.B, c.LA, c.LB), nil
}
