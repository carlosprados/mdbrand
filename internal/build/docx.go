package build

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

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
		Reference: d.Meta.Options.Reference, Confidential: d.Meta.Options.Confidential,
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
	if checked, err := docxGlyphs(look, body, meta, o.AllowHoles); err != nil {
		return err
	} else if !checked {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf(
			"%s is not installed here, so the .docx's characters were not checked against it", look.Body))
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

	filter, err := writeSpanFilter(work)
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
	if err := copyFile(built, out); err != nil {
		return err
	}
	rep.Outputs = append(rep.Outputs, Output{Format: "docx", Path: out})
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
		Primary: b.Colors.Primary, Text: b.Colors.Text, Rule: b.Colors.Rule, Link: b.Colors.Link,
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
// can do is ask fontconfig whether the body face, installed here, covers the
// prose. A face that is not installed cannot be checked, and the reader hears
// that rather than nothing.
func docxGlyphs(l docx.Look, body string, m docx.Meta, allow bool) (bool, error) {
	cov, ok := brand.FontCharset(l.Body)
	if !ok {
		return false, nil
	}
	var prose strings.Builder
	last := 0
	for _, sp := range mdtext.Protected(body) {
		prose.WriteString(body[last:sp[0]])
		last = sp[1]
	}
	prose.WriteString(body[last:])
	text := strings.Join([]string{prose.String(), m.Title, m.Subtitle, m.Author, m.Greeting, m.Signature}, "\n")
	seen := map[rune]bool{}
	var holes []string
	for _, r := range text {
		if r < 0x80 || seen[r] || (r >= 0xFE00 && r <= 0xFE0F) || r == ' ' {
			continue
		}
		seen[r] = true
		if !cov.Has(r) {
			holes = append(holes, fmt.Sprintf("%c (U+%04X)", r, r))
		}
	}
	if len(holes) == 0 || allow {
		return true, nil
	}
	return true, fmt.Errorf(`%s, the .docx's body face, has no glyph for %d character(s) the document uses:
  %s
Word would print them in whatever face it finds, or as boxes. Fix the text, or
name a fonts.office.body that covers them; override with --allow-missing-glyphs`,
		l.Body, len(holes), strings.Join(holes, "\n  "))
}
