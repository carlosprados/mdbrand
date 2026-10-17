package build

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/carlosprados/mdbrand/internal/imgsize"
	"github.com/carlosprados/mdbrand/internal/pptx"
	"github.com/carlosprados/mdbrand/internal/run"
	"github.com/carlosprados/mdbrand/internal/tex"
)

// renderPPTX writes a deck as a .pptx for a reader who must edit it: the same
// slides, brand and figures as the PDF deck, and nothing it did not say.
// Figures are the PDF's, scaled to pandoc's slide and rasterised at the size
// they are placed; the brand goes in through a generated reference.pptx.
func renderPPTX(p *prepared, out string, inputs *[]string) error {
	o, d, b, work, rep := p.o, p.d, p.b, p.work, p.rep
	look, err := pptxLook(p)
	if err != nil {
		return err
	}

	body := p.body
	if pic := pdfPictureRe.FindStringSubmatch(body); pic != nil {
		return fmt.Errorf(`%s: the picture %s is a PDF, and a .pptx cannot hold one.
Export it as SVG, which mdbrand sizes like a figure, or as PNG`, o.Input, pic[1])
	}
	for i, f := range p.figs {
		res := p.results[i]
		png := fmt.Sprintf("fig%02d-slide.png", f.Index)
		px := strconv.Itoa(int(res.WidthMM*pptx.Scale/25.4*pptx.PPI + 0.5))
		if err := run.Quiet(work, "rsvg-convert", "-w", px, "-o", png, res.SVG); err != nil {
			return err
		}
		body = strings.Replace(body, f.Placeholder, fmt.Sprintf("![%s](%s)", res.Fig.Caption, png), 1)
	}

	display, plain := d.Meta.Printed()
	unchecked, err := officeGlyphs(officeFaces{look.Body, look.Display, look.Mono}, ".pptx", body,
		append(display, look.Confidential, look.RunningTitle), plain, o.AllowHoles)
	if err != nil {
		return err
	}
	for _, face := range unchecked {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf(
			"%s is not installed here, so the .pptx's characters were not checked against it", face))
	}

	stem := workStem(o.Input)
	mdName := stem + ".pptx.md"
	var md strings.Builder
	if d.FrontMatter != "" {
		md.WriteString("---\n" + d.FrontMatter + "\n---\n\n")
	}
	md.WriteString(body)
	if err := os.WriteFile(filepath.Join(work, mdName), []byte(md.String()), 0o644); err != nil {
		return err
	}

	if err := run.Quiet(work, "pandoc", "-o", "pandoc-reference.pptx", "--print-default-data-file", "reference.pptx"); err != nil {
		return err
	}
	base, err := os.ReadFile(filepath.Join(work, "pandoc-reference.pptx"))
	if err != nil {
		return err
	}
	ref, err := pptx.Reference(base, look)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(work, "reference.pptx"), ref, 0o644); err != nil {
		return err
	}

	filter, err := writeSpanFilter(work, b)
	if err != nil {
		return err
	}
	args := []string{
		mdName, "-o", stem + ".pandoc.pptx", filter,
		"--reference-doc=reference.pptx", "--slide-level=2",
		resourcePath(work, o.Input),
		"-V", "monofont=" + look.Mono,
		"-M", "notes-title=" + pptx.NotesTitle(d.Meta.Lang),
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
	if err := citationProblems(d.Meta, pandocOut, "deck"); err != nil {
		return err
	}
	if err := filterProblems(pandocOut, b); err != nil {
		return err
	}
	if err := pptxColumns(pandocOut); err != nil {
		return err
	}
	if err := pandocWarnings(pandocOut, ".pptx", rep); err != nil {
		return err
	}
	if !p.pdfToo {
		if err := accentContrast(b, pandocOut, true, rep); err != nil {
			return err
		}
	}

	raw, err := os.ReadFile(filepath.Join(work, stem+".pandoc.pptx"))
	if err != nil {
		return err
	}
	fixed, warns, err := pptx.Repair(raw, look)
	if err != nil {
		return err
	}
	rep.Warnings = append(rep.Warnings, warns...)
	built := filepath.Join(work, stem+".pptx")
	if err := os.WriteFile(built, fixed, 0o644); err != nil {
		return err
	}
	if !p.pdfToo {
		// Built beside the PDF, beamer has measured every slide already.
		if err := pptxOverflow(work, built, fixed, look, rep); err != nil {
			return err
		}
	}
	rep.Outputs = append(rep.Outputs, Output{Format: "pptx", Path: out, built: built})
	return nil
}

// pptxOverflow warns of slides whose text LibreOffice sets past the footer or
// off the slide. It is an estimate — PowerPoint sets the text in the reader's
// fonts — so it warns, and without LibreOffice it says it did not look.
func pptxOverflow(work, built string, deck []byte, look pptx.Look, rep *Report) error {
	if missing := run.Missing("soffice", "pdftotext"); len(missing) > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf(
			"the .pptx's slides were not checked for text that does not fit: that estimate needs %s. "+
				"Install LibreOffice, or build the PDF too (--to pdf,pptx), which measures every slide", strings.Join(missing, " and ")))
		return nil
	}
	profile := filepath.ToSlash(filepath.Join(work, "lo-profile"))
	if !strings.HasPrefix(profile, "/") {
		profile = "/" + profile
	}
	dir := filepath.Join(work, "lo")
	if err := run.Quiet(work, "soffice", "--headless", "-env:UserInstallation=file://"+profile,
		"--convert-to", "pdf", "--outdir", dir, built); err != nil {
		return err
	}
	pdf := filepath.Join(dir, strings.TrimSuffix(filepath.Base(built), ".pptx")+".pdf")
	bbox, err := run.Stdout(work, "pdftotext", "-bbox", pdf, "-")
	if err != nil {
		return err
	}
	over, err := pptx.Overflow(deck, string(bbox), look)
	if err != nil {
		return err
	}
	for _, o := range over {
		rep.Warnings = append(rep.Warnings, o+" — estimated by LibreOffice; split the slide or cut it, or build the PDF too, which measures it")
	}
	return nil
}

var (
	pptxColumnsRe = regexp.MustCompile(`MDBRAND-PPTX-COLUMNS (\d+)\t([^\n]*)`)
	pptxFiguresRe = regexp.MustCompile(`MDBRAND-PPTX-FIGURES (\d+)\t([^\n]*)`)
)

// pptxColumns stops a slide of more than two columns, which pandoc's pptx
// writer drops but the first two of, and a slide of more than one figure,
// whose second it moves to an untitled slide of its own: in silence, both.
func pptxColumns(pandocOut string) error {
	if m := pptxFiguresRe.FindStringSubmatch(pandocOut); m != nil {
		return fmt.Errorf(`slide %q holds %s figures, and a .pptx slide holds one: pandoc moves
the rest to slides of their own, with no title. Give each figure its own
slide (another ## heading), put them side by side in :::: columns, or join
them in one diagram`, m[2], m[1])
	}
	m := pptxColumnsRe.FindStringSubmatch(pandocOut)
	if m == nil {
		return nil
	}
	return fmt.Errorf(`a slide with %s columns (%q): a .pptx holds two, and pandoc drops
the rest without a word. Keep two columns on the slide, or split it`, m[1], ellipsize(m[2], 40))
}

// pptxLook is the brand in the terms a deck needs, every length scaled from
// the PDF's frame to pandoc's slide.
func pptxLook(p *prepared) (pptx.Look, error) {
	b, work := p.b, p.work
	office, err := b.OfficeFonts()
	if err != nil {
		return pptx.Look{}, err
	}
	l := pptx.Look{
		Body: office.Body, Display: office.Display, Mono: office.Mono,
		Primary: b.Colors.Primary, Text: b.Colors.Text, Rule: b.Colors.Rule, Link: b.Colors.Link, Accent: b.Colors.Accent,
		Named: b.Colors.Named, NamedNames: b.Colors.NamedNames(),
		Ground: b.Slides.Background, OnGround: b.Slides.Foreground,
		MarginMM:     tex.SlideMarginMM * pptx.Scale,
		Confidential: p.d.Meta.Options.Confidential, RunningTitle: p.d.Meta.RunningTitle(),
	}
	asText, err := inlineMarkdown(work, "plain", l.Confidential, l.RunningTitle)
	if err != nil {
		return pptx.Look{}, err
	}
	for _, f := range []*string{&l.Confidential, &l.RunningTitle} {
		if t, ok := asText[*f]; ok {
			*f = t
		}
	}
	if l.LogoWMM, err = docxLength("slides.logo_width", b.Slides.LogoWidth); err != nil {
		return pptx.Look{}, err
	}
	if l.CoverLogoWMM, err = docxLength("slides.logo_width_cover", b.Slides.LogoWidthCover); err != nil {
		return pptx.Look{}, err
	}
	l.LogoWMM *= pptx.Scale
	l.CoverLogoWMM *= pptx.Scale
	for _, art := range []struct {
		src, stem string
		raw       *[]byte
		aspect    *float64
	}{
		{b.LogoPath(), "pptx-logo", &l.Logo, &l.LogoAspect},
		{b.SlideLogoPath(), "pptx-cover-logo", &l.CoverLogo, &l.CoverAspect},
		{b.SlideArtPath(), "pptx-art", &l.Art, &l.ArtAspect},
	} {
		name, err := docxLogoFile(art.src, work, art.stem)
		if err != nil || name == "" {
			if err != nil {
				return pptx.Look{}, err
			}
			continue
		}
		if *art.raw, err = os.ReadFile(filepath.Join(work, name)); err != nil {
			return pptx.Look{}, err
		}
		*art.aspect, _ = imgsize.Aspect(filepath.Join(work, name))
	}
	return l, nil
}
