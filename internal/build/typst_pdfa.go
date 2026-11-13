package build

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/doc"
	"github.com/carlosprados/mdbrand/internal/run"
	"github.com/carlosprados/mdbrand/internal/tex"
)

// PDFStandards are the values pdf_standard may take: the ones Typst enforces
// conformance with as it writes the file.
var PDFStandards = []string{"1.4", "1.5", "1.6", "1.7", "2.0",
	"a-1b", "a-1a", "a-2b", "a-2u", "a-2a", "a-3b", "a-3u", "a-3a",
	"a-4", "a-4f", "a-4e", "ua-1"}

// pickPDFStandards settles the standards the PDF must meet: the document's
// list, else its bundle's. Only Typst can promise one, so a standard under
// XeLaTeX stops the build rather than print a PDF that only looks archival.
func pickPDFStandards(d *doc.File, b *brand.Brand, engine, style string) ([]string, error) {
	std := []string(d.Meta.Options.PDFStandard)
	if len(std) == 0 {
		std = b.PDFStandard
	}
	if len(std) == 0 {
		return nil, nil
	}
	for _, s := range std {
		if !slices.Contains(PDFStandards, s) {
			return nil, fmt.Errorf("unknown pdf_standard %q: pick from %s", s, strings.Join(PDFStandards, ", "))
		}
	}
	switch {
	case engine != EngineTypst:
		return nil, fmt.Errorf(`pdf_standard %s needs engine typst: XeLaTeX cannot promise PDF/A or PDF/UA,
and a PDF that only claimed it would be refused where it is filed. Set
mdbrand.engine: typst, or take pdf_standard out`, strings.Join(std, ", "))
	case style == tex.Slides:
		return nil, fmt.Errorf("pdf_standard is for pages: a deck (style slides) cannot be built to %s", strings.Join(std, ", "))
	}
	return std, nil
}

// needsAlt reports whether a standard requires every picture to carry alt
// text: PDF/UA, and PDF/A's accessible ("a") levels.
func needsAlt(std []string) bool {
	return slices.ContainsFunc(std, func(s string) bool { return s == "ua-1" || strings.HasSuffix(s, "a") })
}

// asSVG turns a PDF the document places — a rendered figure, a logo — into
// an SVG beside it: Typst 0.15 cannot embed a PDF under any PDF/A or PDF/UA
// standard. pdftocairo draws text as outlines, so no face is left to embed.
func asSVG(work, pdf string) (string, error) {
	out := filepath.Join(work, strings.TrimSuffix(filepath.Base(pdf), filepath.Ext(pdf))+".std.svg")
	if _, err := run.Cmd(work, "pdftocairo", "-svg", pdf, out); err != nil {
		return "", err
	}
	return out, nil
}

// refusePDFPictures stops a standard-bound build on a picture the author gave
// as a PDF: placed, it would break the standard, and redrawn it would no
// longer be the file the author chose.
func refusePDFPictures(refs []string) error {
	var pdfs []string
	for _, f := range refs {
		if strings.EqualFold(filepath.Ext(f), ".pdf") {
			pdfs = append(pdfs, filepath.Base(f))
		}
	}
	if len(pdfs) == 0 {
		return nil
	}
	return fmt.Errorf(`%s: a PDF picture cannot go into a PDF/A or PDF/UA document set by Typst.
Give it as SVG, PNG or JPEG`, strings.Join(pdfs, ", "))
}

var noAltRe = regexp.MustCompile(`MDBRAND-NOALT ([^\n]+)`)

// missingAlt stops on a picture without a description where the standard
// asks every picture for one: typst.lua reports each by file.
func missingAlt(pandocOut string) error {
	files := uniqueMatches(noAltRe, pandocOut)
	if len(files) == 0 {
		return nil
	}
	for i, f := range files {
		files[i] = filepath.Base(f)
	}
	return fmt.Errorf(`%d picture(s) without a description, which PDF/UA reads out in their place:
  %s
Write what each shows inside the brackets: ![A map of the region](map.png)`,
		len(files), strings.Join(files, "\n  "))
}

// stdPicture is where a placed PDF goes under a standard: its SVG.
func stdPicture(work, pdf string, std []string) (string, error) {
	if len(std) == 0 || !strings.EqualFold(filepath.Ext(pdf), ".pdf") {
		return pdf, nil
	}
	if _, err := os.Stat(pdf); err != nil {
		return pdf, nil
	}
	return asSVG(work, pdf)
}
