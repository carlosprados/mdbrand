package build

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/docx"
	"github.com/carlosprados/mdbrand/internal/imgsize"
	"github.com/carlosprados/mdbrand/internal/mdtext"
	"github.com/carlosprados/mdbrand/internal/run"
	"github.com/carlosprados/mdbrand/internal/tex"
)

// dpi is the resolution a figure is rasterised at for a .docx: print quality
// at the size it is placed, which the sizing already settled.
const dpi = 300

// renderDOCX writes a prepared document as a .docx: the brand as a reference
// document, the cover or letterhead as OOXML, figures as PNG at their placed
// size, and Repair's fixes over what pandoc writes.
func renderDOCX(p *prepared, out string, inputs *[]string) error {
	o, d, b, work, rep := p.o, p.d, p.b, p.work, p.rep
	look, err := docxLook(p)
	if err != nil {
		return err
	}

	if n, room := utf8.RuneCountInString(look.Confidential), look.ConfidentialRoom(); n > room && !p.pdfToo {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf(
			"mdbrand.confidential has %d characters; the .docx footer fits about %d beside the page number. "+
				"That is an estimate, since Word sets it in the reader's fonts: shorten the label, or build the PDF too, which measures it", n, room))
	}

	body := p.body
	if pic := pdfPictureRe.FindStringSubmatch(body); pic != nil {
		return fmt.Errorf(`%s: the picture %s is a PDF, and a .docx cannot hold one.
Export it as SVG, which mdbrand sizes like a figure, or as PNG`, o.Input, pic[1])
	}
	for i, f := range p.figs {
		res := p.results[i]
		png := fmt.Sprintf("fig%02d.png", f.Index)
		px := strconv.Itoa(int(res.WidthMM/25.4*dpi + 0.5))
		if err := run.Quiet(work, "rsvg-convert", "-w", px, "-o", png, res.SVG); err != nil {
			return err
		}
		body = strings.Replace(body, f.Placeholder,
			fmt.Sprintf("![%s](%s){width=%.1fmm}", res.Fig.Caption, png, res.WidthMM), 1)
	}

	meta := docx.Meta{
		Title: d.Meta.Title, Subtitle: d.Meta.Subtitle, Author: d.Meta.AuthorString(), Date: d.Meta.Date,
		Reference: d.Meta.Options.Reference, Confidential: d.Meta.Options.Confidential, BrandFooter: b.Footer,
		To: d.Meta.Options.To, Place: d.Meta.Options.Place,
		Greeting: d.Meta.Options.Greeting, Signature: d.Meta.Options.Signature,
		TOC: d.Meta.TOC != nil && *d.Meta.TOC, TOCDepth: d.Meta.TOCDepth, Lang: d.Meta.Lang,
	}
	if meta.Logo, err = docxLogoFile(b.LogoPath(), work, "docx-logo"); err != nil {
		return err
	}
	if meta.LogoSecondary, err = docxLogoFile(b.LogoSecondaryPath(), work, "docx-logo-secondary"); err != nil {
		return err
	}
	meta.LogoWMM, _ = tex.ParseLenMM(b.Page.LogoWidthCover)
	meta.LogoSecondaryWMM, _ = tex.ParseLenMM(b.Page.LogoWidthCoverSecondary)
	if meta.Logo != "" {
		if look.Logo, err = os.ReadFile(filepath.Join(work, meta.Logo)); err != nil {
			return err
		}
		look.LogoAspect, _ = imgsize.Aspect(filepath.Join(work, meta.Logo))
	}
	display, plain := d.Meta.Printed()
	unchecked, err := docxGlyphs(look, body, append(display, b.Footer), plain, o.AllowHoles)
	if err != nil {
		return err
	}
	for _, face := range unchecked {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf(
			"%s is not installed here, so the .docx's characters were not checked against it", face))
	}

	before, after := docx.Front(p.style, meta, look)
	stem := strings.TrimSuffix(filepath.Base(o.Input), filepath.Ext(o.Input))
	mdName := stem + ".docx.md"
	var md strings.Builder
	if d.FrontMatter != "" {
		md.WriteString("---\n" + d.FrontMatter + "\n---\n\n")
	}
	md.WriteString(before + body + "\n\n" + after)
	if err := os.WriteFile(filepath.Join(work, mdName), []byte(md.String()), 0o644); err != nil {
		return err
	}

	// pandoc's own reference document, rewritten in the brand.
	if err := run.Quiet(work, "pandoc", "-o", "pandoc-reference.docx", "--print-default-data-file", "reference.docx"); err != nil {
		return err
	}
	base, err := os.ReadFile(filepath.Join(work, "pandoc-reference.docx"))
	if err != nil {
		return err
	}
	ref, err := docx.Reference(base, look)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(work, "reference.docx"), ref, 0o644); err != nil {
		return err
	}

	filter, err := writeSpanFilter(work, b)
	if err != nil {
		return err
	}
	args := []string{
		mdName, "-o", stem + ".pandoc.docx", filter,
		"--reference-doc=reference.docx",
		"--resource-path=" + work + ":" + filepath.Dir(mustAbs(o.Input)),
		// The table of contents is the cover's to place, after the cover.
		"-M", "toc=false",
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
	if keys := missingCitations(pandocOut); len(keys) > 0 {
		return fmt.Errorf(`the bibliography has no entry for %d citation key(s):
  %s
citeproc prints those as "(key?)" in the finished document and exits 0, so
nothing else would have told you. Fix the key or add the entry.`,
			len(keys), strings.Join(keys, ", "))
	}
	if err := spanProblems(pandocOut); err != nil {
		return err
	}
	if !p.pdfToo {
		// Built beside the PDF, the PDF has already said it.
		if err := accentContrast(b, pandocOut, false, rep); err != nil {
			return err
		}
	}

	raw, err := os.ReadFile(filepath.Join(work, stem+".pandoc.docx"))
	if err != nil {
		return err
	}
	fixed, warns, err := docx.Repair(raw, docx.Fix{
		Look: look, Labels: docx.LabelsFor(d.Meta.Lang), TOC: meta.TOC, KeepRows: tex.TableKeep,
	})
	if err != nil {
		return err
	}
	rep.Warnings = append(rep.Warnings, warns...)
	built := filepath.Join(work, stem+".docx")
	if err := os.WriteFile(built, fixed, 0o644); err != nil {
		return err
	}
	rep.Outputs = append(rep.Outputs, Output{Format: "docx", Path: out, built: built})
	return nil
}

var pdfPictureRe = regexp.MustCompile(`(?i)!\[[^\]]*\]\(<?([^)\s>]+\.pdf)>?`)

// docxLook is the brand in the terms a .docx needs.
func docxLook(p *prepared) (docx.Look, error) {
	b := p.b
	office, err := b.OfficeFonts()
	if err != nil {
		return docx.Look{}, err
	}
	margin, err := tex.ParseLenMM(b.Page.Margin)
	if err != nil {
		return docx.Look{}, err
	}
	header, err := tex.ParseLenMM(b.Page.LogoWidthHeader)
	if err != nil {
		return docx.Look{}, err
	}
	return docx.Look{
		Body: office.Body, Display: office.Display, Mono: office.Mono,
		Primary: b.Colors.Primary, Text: b.Colors.Text, Rule: b.Colors.Rule, Link: b.Colors.Link, Accent: b.Colors.Accent,
		PaperWMM: tex.PaperWidthMM(b.Page.PaperSize), PaperHMM: tex.PaperHeightMM(b.Page.PaperSize),
		MarginMM: margin, LineStretch: b.Page.LineStretch,
		LogoHeaderWMM: header, RunningTitle: p.d.Meta.RunningTitle(), Confidential: p.d.Meta.Options.Confidential,
		Layout: p.style, NumberFromCover: p.style == "report",
	}, nil
}

// docxLogoFile lands a logo in the work directory as PNG, the one format every
// .docx reader shows. A PDF logo cannot go in: Word does not draw PDF.
func docxLogoFile(src, work, stem string) (string, error) {
	if src == "" {
		return "", nil
	}
	name := stem + ".png"
	switch strings.ToLower(filepath.Ext(src)) {
	case ".png":
		return name, copyFile(src, filepath.Join(work, name))
	case ".svg":
		// Wide enough for the cover at print resolution.
		return name, run.Quiet(work, "rsvg-convert", "-w", "1200", "--keep-aspect-ratio", "-o", name, src)
	default:
		return "", fmt.Errorf(`logo %s: a .docx cannot hold a PDF. Give the bundle an SVG or PNG logo`, filepath.Base(src))
	}
}

// docxGlyphs is the .docx's missing-glyph check. The PDF's is read from
// XeLaTeX's log; a .docx is set on the reader's machine, so the best this one
// can do is ask fontconfig whether the faces, installed here, cover the text
// each sets: the prose and a letter's greeting in the body face, the front
// matter the cover, letterhead, header and footer print in the display face.
// A face that is not installed cannot be checked; it is returned, so the
// reader hears that rather than nothing.
func docxGlyphs(l docx.Look, body string, display, plain []string, allow bool) ([]string, error) {
	var prose strings.Builder
	last := 0
	for _, sp := range mdtext.Protected(body) {
		prose.WriteString(body[last:sp[0]])
		last = sp[1]
	}
	prose.WriteString(body[last:])
	var unchecked, found []string
	for _, set := range []struct{ face, role, text string }{
		{l.Body, "body", strings.Join(append([]string{prose.String()}, plain...), "\n")},
		{l.Display, "display", strings.Join(display, "\n")},
	} {
		cov, ok := brand.FontCharset(set.face)
		if !ok {
			if !slices.Contains(unchecked, set.face) {
				unchecked = append(unchecked, set.face)
			}
			continue
		}
		if holes := missingGlyphs(cov, set.text); len(holes) > 0 {
			found = append(found, fmt.Sprintf("%s, the .docx's %s face (fonts.office.%s), has no glyph for %d character(s) it sets:\n  %s",
				set.face, set.role, set.role, len(holes), strings.Join(holes, "\n  ")))
		}
	}
	if len(found) == 0 || allow {
		return unchecked, nil
	}
	return unchecked, fmt.Errorf(`%s
Word would print them in whatever face it finds, or as boxes. Fix the text, or
name a face that covers them; override with --allow-missing-glyphs`, strings.Join(found, "\n"))
}

// missingGlyphs lists the characters of text the face lacks, each once,
// leaving out ASCII, variation selectors and the no-break space.
func missingGlyphs(cov brand.Charset, text string) []string {
	seen := map[rune]bool{}
	var holes []string
	for _, r := range text {
		if r < 0x80 || seen[r] || (r >= 0xFE00 && r <= 0xFE0F) || r == '\u00a0' {
			continue
		}
		seen[r] = true
		if !cov.Has(r) {
			holes = append(holes, fmt.Sprintf("%c (U+%04X)", r, r))
		}
	}
	return holes
}
