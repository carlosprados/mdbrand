package build

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/doc"
	"github.com/carlosprados/mdbrand/internal/imgsize"
	"github.com/carlosprados/mdbrand/internal/run"
	"github.com/carlosprados/mdbrand/internal/tex"
)

// renderPDF typesets a prepared document: LaTeX fragments from the brand,
// pandoc to LaTeX, xelatex, and a reading of its log that refuses a PDF with
// holes in it.
func renderPDF(p *prepared, out string, inputs *[]string) error {
	o, d, b, work, rep := p.o, p.d, p.b, p.work, p.rep

	// The logo goes into the work directory in a form xelatex can embed.
	logoSecondFile, err := prepareLogoSecondary(b, work)
	if err != nil {
		return err
	}
	logoFile, err := prepareLogo(b, work)
	if err != nil {
		return err
	}

	body := p.body
	for i, f := range p.figs {
		body = strings.Replace(body, f.Placeholder, p.results[i].Markdown(), 1)
	}

	fallbackFont, fallbackChars, err := fallback(b, body, d.Meta)
	if err != nil {
		return err
	}

	// LaTeX fragments.
	data := &tex.Data{
		FallbackFont:  fallbackFont,
		FallbackChars: fallbackChars,
		Brand:         b, Style: p.style,
		LogoFile:                logoFile,
		LogoSecondaryFile:       logoSecondFile,
		CoverLogoWidth:          b.Page.LogoWidthCover,
		CoverLogoSecondaryWidth: b.Page.LogoWidthCoverSecondary,
		HeaderLogoWidth:         b.Page.LogoWidthHeader,
		HeaderTitle:             d.Meta.Title,
		Title:                   d.Meta.Title,
		Subtitle:                d.Meta.Subtitle,
		Author:                  d.Meta.AuthorString(),
		Date:                    d.Meta.Date,
		Reference:               d.Meta.Options.Reference,
		Confidential:            d.Meta.Options.Confidential,
		Recipient:               d.Meta.Options.To,
		Place:                   d.Meta.Options.Place,
		Greeting:                d.Meta.Options.Greeting,
		Signature:               d.Meta.Options.Signature,
	}
	if s := d.Meta.Options.Signature; s != "" {
		data.SignatureLines = strings.Split(s, "\n")
	}
	if res, ok := b.ResolveDisplay(); ok {
		data.DisplayFont = true
		data.DisplayRegular, data.DisplayRegularDir = res.RegularFile, res.RegularDir
		data.DisplayBold, data.DisplayBoldDir = res.BoldFile, res.BoldDir
		if res.BoldIsRegular {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf(
				"display font: %s not found, so titles use %s",
				strings.Join(res.Missing, ", "), res.RegularFile))
		}
	} else if b.Fonts.Display.Family != "" {
		rep.Warnings = append(rep.Warnings, b.DisplayFontHint())
	}

	for _, frag := range []string{"preamble", "before", "after"} {
		s, err := tex.Render(frag, data)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(work, frag+".tex"), []byte(s), 0o644); err != nil {
			return err
		}
	}

	// The document pandoc will read: original front matter, rewritten body.
	stem := strings.TrimSuffix(filepath.Base(o.Input), filepath.Ext(o.Input))
	mdName := stem + ".md"
	var md strings.Builder
	if d.FrontMatter != "" {
		md.WriteString("---\n" + d.FrontMatter + "\n---\n\n")
	}
	md.WriteString(body)
	if err := os.WriteFile(filepath.Join(work, mdName), []byte(md.String()), 0o644); err != nil {
		return err
	}

	args, err := pandocArgs(p, stem, inputs)
	if err != nil {
		return err
	}
	o.logf("  pandoc %s", stem+".md")
	pandocOut, err := run.Cmd(work, "pandoc", args...)
	if err != nil {
		return err
	}
	if keys := missingCitations(pandocOut); len(keys) > 0 {
		return fmt.Errorf(`the bibliography has no entry for %d citation key(s):
  %s
citeproc prints those as "(key?)" in the finished PDF and exits 0, so nothing
else would have told you. Fix the key or add the entry.`,
			len(keys), strings.Join(keys, ", "))
	}
	if err := spanProblems(pandocOut); err != nil {
		return err
	}

	texPath := filepath.Join(work, stem+".tex")
	texSrc, err := os.ReadFile(texPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(texPath, []byte(tex.KeepTableRows(string(texSrc))), 0o644); err != nil {
		return err
	}

	// xelatex, run here so the log is ours to read.
	passes := 2
	if d.Meta.TOC != nil && *d.Meta.TOC {
		passes = 3
	}
	for i := 0; i < passes; i++ {
		o.logf("  xelatex pass %d/%d", i+1, passes)
		if _, err := run.Cmd(work, "xelatex", "-interaction=nonstopmode", "-halt-on-error", stem+".tex"); err != nil {
			return fmt.Errorf("xelatex failed: %w", err)
		}
	}

	logRaw, _ := os.ReadFile(filepath.Join(work, stem+".log"))
	sc := scanLog(string(logRaw))
	if err := judgeLog(sc, o, rep); err != nil {
		return err
	}
	if err := checkTextLayer(work, stem+".pdf", md.String(), rep); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(work, stem+".pdf"), out); err != nil {
		return err
	}
	rep.Outputs = append(rep.Outputs, Output{Format: "pdf", Path: out, Pages: sc.Pages})
	return nil
}

// pandocArgs is the command line that turns the rewritten Markdown into LaTeX:
// the brand's fragments, its page geometry and its body face.
func pandocArgs(p *prepared, stem string, inputs *[]string) ([]string, error) {
	o, d, b, rep := p.o, p.d, p.b, p.rep

	// The header's height depends on the shape of the logo that goes in it, so
	// it is measured rather than assumed. See tex.HeaderHeightMM.
	var logoAspect float64
	if logo := b.LogoPath(); logo != "" {
		a, err := imgsize.Aspect(logo)
		switch {
		case err != nil:
			// An unreadable size is not a defect in the artwork, so it must not
			// fail the build; but the header geometry is then a guess, and a
			// guess the reader should hear about.
			o.logf("  warning: cannot measure %s (%v) — header height left at %s, "+
				"which is wrong if the mark is not wide", filepath.Base(logo), err, b.Page.HeadHeight)
		case a > 0:
			logoAspect = a
		}
	}
	headHeight, err := tex.HeaderHeightSpec(b, logoAspect)
	if err != nil {
		return nil, err
	}
	geometry := fmt.Sprintf("margin=%s,headheight=%s,headsep=%s", b.Page.Margin, headHeight, b.Page.HeadSep)
	filter, err := writeSpanFilter(p.work)
	if err != nil {
		return nil, err
	}
	args := []string{
		stem + ".md", "-s", "-o", stem + ".tex", filter,
		"--include-in-header=preamble.tex",
		"--include-before-body=before.tex",
		"--include-after-body=after.tex",
		"--resource-path=" + p.work + ":" + filepath.Dir(mustAbs(o.Input)),
		"-V", "papersize=" + b.Page.PaperSize,
		"-V", "geometry=" + geometry,
		"-V", "linestretch=" + strconv.FormatFloat(b.Page.LineStretch, 'f', -1, 64),
	}

	// The body font, if the machine can actually supply it. An absent family
	// dies inside fontspec with an error naming XeLaTeX's font machinery and
	// not the bundle, so it is settled here instead.
	switch {
	case b.BodyFontInstalled():
		// mainfontoptions, not a \setmainfont of our own: pandoc's template
		// loads the face twice, the second time through \babelfont, and that
		// one would load it again without the option.
		args = append(args, "-V", "mainfont="+b.Fonts.Body, "-V", "mainfontoptions="+noContextualAlternates)
	case b.IsDefault():
		// No mainfont at all: pandoc's template loads lmodern, so the document
		// sets in Latin Modern, which every TeX Live has. The built-in bundle
		// is the one that has to work on a machine with nothing installed —
		// that is the whole reason it exists — and it carries no identity that
		// a substituted face would betray.
		rep.Warnings = append(rep.Warnings, fmt.Sprintf(
			"%s is not installed; the default bundle set this document in Latin Modern instead", b.Fonts.Body))
	default:
		return nil, fmt.Errorf(`the %s bundle sets its body type in %q, and fontconfig cannot find it.
The document would not be the one the bundle describes, so this stops here
rather than letting XeLaTeX substitute a face nobody chose. Install the family,
or change fonts.body in the bundle to one this machine has.`, b.Name, b.Fonts.Body)
	}

	cites, err := citeArgs(d, o.Input, inputs)
	if err != nil {
		return nil, err
	}
	return append(args, cites...), nil
}

// noContextualAlternates turns off a face's calt. Inter's swaps ( ) [ ] { }
// beside capitals and digits for case forms that its cmap gives private-use
// code points; xdvipdfmx builds the PDF's ToUnicode from that cmap, so
// "(SD1)" printed right and copied — and reached Turnitin — as U+EE4E SD1
// U+EE4F. RawFeature, because fontspec warns about a named feature a face
// lacks and stays quiet about a raw one.
const noContextualAlternates = "RawFeature=-calt"

// judgeLog turns what the xelatex log says into the build's verdict: holes and
// a header taller than its box stop it, code and lines past the measure warn.
func judgeLog(sc scan, o Options, rep *Report) error {
	if len(sc.Holes) > 0 && !o.AllowHoles {
		return fmt.Errorf(`the font has no glyph for %d character(s) the document uses, so they
would print as nothing at all and only this log would know:
  %s
Fix the text (a ballpoint tick beats a missing ☐ anyway) or pick a font that
covers it; override with --allow-missing-glyphs if you truly want the holes`,
			len(sc.Holes), strings.Join(sc.Holes, "\n  "))
	}
	if sc.HeadShortPt > 0 {
		return fmt.Errorf(`the running header is %.1fpt taller than its box, so its logo prints
across the first line of text on every page — and only the XeLaTeX log knew.
Raise page.headheight by at least %.0fpt in the brand bundle, or lower
page.logo_width_header so the mark is shorter.`, sc.HeadShortPt, math.Ceil(sc.HeadShortPt))
	}
	if len(sc.Wide) > 0 {
		var w strings.Builder
		fmt.Fprintf(&w, "%d code block(s) stay past the measure at the smallest legible size:", len(sc.Wide))
		for _, c := range sc.Wide {
			fmt.Fprintf(&w, "\n    %d columns, where %d fit", c.Cols, c.Fits)
		}
		w.WriteString("\n  The size was already stepped down as far as it goes. Shorten the lines\n  or split the block; an ASCII diagram is not wrapped, by design.")
		rep.Warnings = append(rep.Warnings, w.String())
	}
	if len(sc.Over) > 0 {
		var w strings.Builder
		fmt.Fprintf(&w, "%d line(s) overflow the measure by more than 5pt:", len(sc.Over))
		for _, ov := range sc.Over {
			// A verbatim line leaves nothing quotable in the log: TeX traces
			// the box and not its characters. Saying so beats printing `""`
			// and letting the reader hunt for a line that was never there.
			if ov.Text == "" {
				fmt.Fprintf(&w, "\n    %5.1fpt  (no quotable text — typically a code block, see above)", ov.Pt)
				continue
			}
			fmt.Fprintf(&w, "\n    %5.1fpt  %q", ov.Pt, ov.Text)
		}
		rep.Warnings = append(rep.Warnings, w.String())
	}
	return nil
}

// fallback decides which characters go to fonts.fallback: those the document
// uses, the body face lacks and the fallback face has. It compares fontconfig's
// coverage of the two faces with the text before xelatex runs, so nothing is
// redirected on a guess — and a character neither face has is left for the
// missing-glyph check to stop, which is the point of that check.
func fallback(b *brand.Brand, body string, m doc.Meta) (string, []tex.FallbackChar, error) {
	if b.Fonts.Fallback == "" {
		return "", nil, nil
	}
	fb, ok := brand.FontCharset(b.Fonts.Fallback)
	if !ok {
		return "", nil, fmt.Errorf(`the %s bundle names %q as fonts.fallback, and fontconfig cannot find it.
Install it, or remove fonts.fallback and let the missing-glyph check name the
characters the body face lacks`, b.Name, b.Fonts.Fallback)
	}
	bodyCov, ok := brand.FontCharset(b.Fonts.Body)
	if !ok {
		// The body face is absent: the build either stops over that or sets in
		// Latin Modern, whose coverage is not the one we would be comparing.
		return "", nil, nil
	}
	text := strings.Join([]string{body, m.Title, m.Subtitle, m.AuthorString(), m.Options.Reference}, "\n")
	rs := brand.FallbackRunes(text, bodyCov, fb)
	if len(rs) == 0 {
		return "", nil, nil
	}
	return b.Fonts.Fallback, tex.FallbackChars(rs), nil
}

func prepareLogo(b *brand.Brand, work string) (string, error) {
	return prepareLogoFile(b, work, b.LogoPath(), "logo")
}

func prepareLogoSecondary(b *brand.Brand, work string) (string, error) {
	return prepareLogoFile(b, work, b.LogoSecondaryPath(), "logo-secondary")
}

// prepareLogoFile lands one logo in the work directory under `stem`, converting
// SVG to PDF on the way. Each logo needs its own stem: writing both to logo.pdf
// left the cover showing the same mark twice.
func prepareLogoFile(b *brand.Brand, work, src, stem string) (string, error) {
	if src == "" {
		return "", nil
	}
	switch strings.ToLower(filepath.Ext(src)) {
	case ".pdf":
		return stem + ".pdf", copyFile(src, filepath.Join(work, stem+".pdf"))
	case ".png":
		return stem + ".png", copyFile(src, filepath.Join(work, stem+".png"))
	case ".svg":
		// rsvg-convert runs with the work directory as its cwd, so it is given
		// the bare name: handing it the joined path made `--work ./out` write to
		// out/out/ and fail, because the path was resolved twice.
		out := filepath.Join(work, stem+".pdf")
		if err := run.Quiet(work, "rsvg-convert", "-f", "pdf", "-o", stem+".pdf", src); err != nil {
			return "", err
		}
		st, err := os.Stat(out)
		if err != nil {
			return "", err
		}
		// A wrapper SVG whose content sits outside the viewBox, or one using the
		// invalid data:img/ MIME type, converts to an empty page without a word
		// of complaint. Size is the cheap tell.
		if st.Size() < 400 {
			return "", fmt.Errorf(`%s converted to an empty PDF (%d bytes).
  The usual causes: the artwork sits outside the SVG viewBox, or the embedded
  raster uses the invalid MIME type data:img/... instead of data:image/...
  Diagnose with: mdbrand brand validate %s`, filepath.Base(src), st.Size(), b.Name)
		}
		return stem + ".pdf", nil
	default:
		return "", fmt.Errorf("logo %s: unsupported format", filepath.Base(src))
	}
}

var (
	missingRe = regexp.MustCompile(`Missing character: There is no (.+?) \(U\+([0-9A-Fa-f]+)\) in font ([^!]+)!`)
	overRe    = regexp.MustCompile(`Overfull \\hbox \(([0-9.]+)pt too wide\)`)
	// The font XeLaTeX was in when it printed the offending line, e.g.
	// `\TU/Inter(2)/b/n/10 `. The trailing space belongs to the run — it is what
	// terminates the font name — so it goes with it; the space *before* the run
	// is real text and stays. Segments may not contain a space, which keeps the
	// pattern off a path like cli_config/rule_generator.py sitting in the text.
	fontRunRe = regexp.MustCompile(`\\[A-Za-z0-9]+(?:/[^/\s]+){3}/[0-9.]+ ?`)
	// The belt to tex.HeaderHeightMM's braces. If anything still puts more in
	// the running header than its box can hold — a logo whose size could not be
	// measured, a headheight declared taller than this code can foresee — the
	// mark prints across the first line of every page, and fancyhdr says so
	// only here.
	headRe  = regexp.MustCompile(`Package fancyhdr Warning: \\headheight is too small \(([0-9.]+)pt too short\)`)
	pagesRe = regexp.MustCompile(`Output written on .*? \((\d+) pages?`)
	// What the preamble's code-block fitter reports when even \footnotesize
	// leaves the block past the measure. LaTeX is the only place that can count
	// the columns, because only it knows the mono face the brand ended up with.
	codeRe = regexp.MustCompile(`MDBRAND-CODE-TOOWIDE cols=(\d+) fits=(\d+)`)
)

// overfull is one line the measure could not hold: how far past it went, and
// enough of its text to find it in the Markdown. The count on its own is a
// smell detector and sends the reader to grep the log; the quote is a
// diagnosis, in the same spirit as naming the character and font of a hole.
type overfull struct {
	Pt   float64
	Text string
}

// wideCode is a code block the fitter could not bring inside the measure: what
// it has, and what would have fitted. Both counts are needed — "shorten it" is
// advice, "shorten it by nine columns" is an instruction.
type wideCode struct {
	Cols int
	Fits int
}

// scan is what a xelatex log is worth reading for.
type scan struct {
	Holes       []string
	Over        []overfull
	Wide        []wideCode
	Pages       int
	HeadShortPt float64
}

// scanLog pulls the things that matter out of a xelatex log.
func scanLog(log string) scan {
	var (
		holes       []string
		over        []overfull
		wide        []wideCode
		pages       int
		headShortPt float64
	)
	seen := map[string]bool{}
	for _, m := range missingRe.FindAllStringSubmatch(log, -1) {
		// A variation selector (U+FE00-FE0F) is not a hole: it has no glyph by
		// design. U+FE0F is the invisible "draw as emoji" that follows ⚠ in ⚠️,
		// and printing nothing for it is exactly right — stopping the build over
		// it would make every emoji-styled warning sign a false alarm.
		if cp, err := strconv.ParseUint(m[2], 16, 32); err == nil && cp >= 0xFE00 && cp <= 0xFE0F {
			continue
		}
		// The log names the font with its whole OpenType feature string
		// appended; the family is the only part a reader needs.
		font := strings.TrimSpace(m[3])
		if i := strings.IndexByte(font, '/'); i > 0 {
			font = font[:i]
		}
		key := fmt.Sprintf("%s (U+%s) missing from %s", m[1], strings.ToUpper(m[2]), font)
		if !seen[key] {
			seen[key] = true
			holes = append(holes, key)
		}
	}
	sort.Strings(holes)
	lines := strings.Split(log, "\n")
	for i, ln := range lines {
		m := overRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		// Under 5pt the measure is a matter of taste, not a defect.
		if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > 5 {
			over = append(over, overfull{Pt: v, Text: offendingText(lines[i+1:])})
		}
	}
	for _, m := range codeRe.FindAllStringSubmatch(log, -1) {
		cols, _ := strconv.Atoi(m[1])
		fits, _ := strconv.Atoi(m[2])
		wide = append(wide, wideCode{Cols: cols, Fits: fits})
	}
	if m := pagesRe.FindStringSubmatch(log); m != nil {
		pages, _ = strconv.Atoi(m[1])
	}
	// The largest shortfall: the log repeats the warning once per page.
	for _, m := range headRe.FindAllStringSubmatch(log, -1) {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > headShortPt {
			headShortPt = v
		}
	}
	return scan{Holes: holes, Over: over, Wide: wide, Pages: pages, HeadShortPt: headShortPt}
}

// offendingText reassembles the line XeLaTeX prints just below an Overfull
// warning and reduces it to something quotable.
//
// The log is hard-wrapped at 79 columns with nothing inserted at the break, and
// it wraps mid-word and even mid-font-name, so the continuation lines are
// joined with no separator before anything else is done to them. Splitting the
// other way round would leave half a font name in the quote.
func offendingText(rest []string) string {
	var raw strings.Builder
	for _, ln := range rest {
		// The content block ends at the paragraph's closing box marker or at
		// the blank line after it.
		if t := strings.TrimSpace(ln); t == "" || t == "[]" {
			break
		}
		raw.WriteString(ln)
		if raw.Len() > 400 { // far more than will ever be shown
			break
		}
	}
	s := fontRunRe.ReplaceAllString(raw.String(), "")
	// Box markers, not text. A literal "[]" in the document loses its brackets
	// in the quote, which is a fair price for not quoting TeX's plumbing.
	s = strings.ReplaceAll(s, "[]", "")
	s = strings.Join(strings.Fields(s), " ")
	return ellipsize(s, 60)
}

// ellipsize cuts to at most n runes, preferring a word boundary in the second
// half so the quote ends on something readable.
func ellipsize(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	cut := string(r[:n])
	if i := strings.LastIndexByte(cut, ' '); i > len(cut)/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ") + "…"
}
