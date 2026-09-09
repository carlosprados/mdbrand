// Package build is the whole pipeline: read the document, render its diagrams,
// generate the LaTeX fragments from the brand bundle, run pandoc and then
// xelatex, and refuse to hand over a PDF with holes in it.
//
// Two decisions are deliberate and worth knowing:
//
//   - xelatex is run by mdbrand, not by pandoc's --pdf-engine. pandoc throws the
//     engine's log away, and that log is the only place where "this font has no
//     glyph for ☐" appears. A checkbox that silently became a blank space is
//     exactly the class of defect this tool exists to stop.
//   - engine choice is not a flag. pdflatex cannot take the Unicode these
//     documents are full of, so there is nothing to choose.
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
	"github.com/carlosprados/mdbrand/internal/fig"
	"github.com/carlosprados/mdbrand/internal/imgsize"
	"github.com/carlosprados/mdbrand/internal/run"
	"github.com/carlosprados/mdbrand/internal/tex"
)

// Options drive one build.
type Options struct {
	Input     string
	Output    string
	BrandsDir string
	BrandName string
	Style     string
	// Defaults from configuration, used only when neither a flag nor the
	// document's own front matter says otherwise.
	DefaultBrand string
	DefaultStyle string
	WorkDir      string // when set, the work directory is kept for inspection
	AllowHoles   bool   // proceed even if the font lacks glyphs the text uses
	Log          func(string, ...any)
}

// Report is what the build produced.
type Report struct {
	Output   string
	Pages    int
	Brand    string
	Style    string
	Figures  []*fig.Result
	Warnings []string
	WorkDir  string
	Kept     bool
}

// Tools are the external programs a build needs, whatever the document holds.
var Tools = []string{"pandoc", "xelatex", "rsvg-convert"}

func (o *Options) logf(f string, a ...any) {
	if o.Log != nil {
		o.Log(f, a...)
	}
}

// Pick returns the first non-empty value, which encodes the precedence every
// setting follows: an explicit flag beats the document's front matter, and the
// front matter beats a machine-wide default.
//
// Getting this backwards is not a cosmetic bug. A configured default used to be
// applied before the front matter was read, so a document that asked for
// `brand: none` was built with whatever brand the reader had configured — and a
// colleague building the unbranded example got a font error about a typeface
// the document never mentions.
func Pick(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// Run executes the pipeline.
func Run(o Options) (*Report, error) {
	if missing := run.Missing(Tools...); len(missing) > 0 {
		return nil, fmt.Errorf("missing tools: %s\n  run: mdbrand doctor", strings.Join(missing, ", "))
	}
	d, err := doc.Read(o.Input)
	if err != nil {
		return nil, err
	}

	name := Pick(o.BrandName, d.Meta.Options.Brand, o.DefaultBrand)
	b, err := brand.Load(o.BrandsDir, name)
	if err != nil {
		return nil, err
	}
	if probs, _ := b.Check(); len(probs) > 0 {
		return nil, fmt.Errorf("brand %q has problems that would break the build:\n  - %s\n  see: mdbrand brand validate %s",
			b.Name, strings.Join(probs, "\n  - "), b.Name)
	}

	style := Pick(o.Style, d.Meta.Options.Style, o.DefaultStyle, "report")
	if !tex.ValidStyle(style) {
		return nil, fmt.Errorf("unknown style %q: pick one of %s", style, strings.Join(tex.Styles, ", "))
	}

	work := o.WorkDir
	keep := work != ""
	if work == "" {
		work, err = os.MkdirTemp("", "mdbrand-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(work)
	} else if err := os.MkdirAll(work, 0o755); err != nil {
		return nil, err
	}
	rep := &Report{Brand: b.Name, Style: style, WorkDir: work, Kept: keep}

	// The logo goes into the work directory in a form xelatex can embed.
	logoSecondFile, err := prepareLogoSecondary(b, work)
	if err != nil {
		return nil, err
	}
	logoFile, err := prepareLogo(b, work)
	if err != nil {
		return nil, err
	}

	textWidth, err := tex.TextWidthMM(b)
	if err != nil {
		return nil, err
	}

	body, figs, err := d.ExtractFigs(work)
	if err != nil {
		return nil, err
	}
	if len(figs) > 0 {
		if missing := run.Missing(figTools(figs)...); len(missing) > 0 {
			return nil, fmt.Errorf("this document has diagrams but these are missing: %s\n  run: mdbrand doctor",
				strings.Join(missing, ", "))
		}
	}
	for _, f := range figs {
		o.logf("  fig %s", filepath.Base(f.SrcPath))
		res, ferr := fig.Render(f, b, work, textWidth)
		if ferr != nil {
			return nil, ferr
		}
		rep.Figures = append(rep.Figures, res)
		if res.Note != "" {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: %s", filepath.Base(f.SrcPath), res.Note))
		}
		body = strings.Replace(body, f.Placeholder, res.Markdown(), 1)
	}

	// LaTeX fragments.
	data := &tex.Data{
		Brand: b, Style: style,
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
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(work, frag+".tex"), []byte(s), 0o644); err != nil {
			return nil, err
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
		return nil, err
	}

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
	args := []string{
		mdName, "-s", "-o", stem + ".tex",
		"--include-in-header=preamble.tex",
		"--include-before-body=before.tex",
		"--include-after-body=after.tex",
		"--resource-path=" + work + ":" + filepath.Dir(mustAbs(o.Input)),
		"-V", "mainfont=" + b.Fonts.Body,
		"-V", "papersize=" + b.Page.PaperSize,
		"-V", "geometry=" + geometry,
		"-V", "linestretch=" + strconv.FormatFloat(b.Page.LineStretch, 'f', -1, 64),
	}

	// Citations. pandoc runs in the work directory, so a `bibliography:` left in
	// the front matter would be resolved against the wrong place and silently
	// yield "[@key?]" in the PDF. Passing the files absolutely on the command
	// line overrides the metadata and removes the whole class of error.
	if len(d.Meta.Bibliography) > 0 {
		docDir := filepath.Dir(mustAbs(o.Input))
		args = append(args, "--citeproc")
		for _, ref := range d.Meta.Bibliography {
			p := ref
			if !filepath.IsAbs(p) {
				p = filepath.Join(docDir, p)
			}
			if _, err := os.Stat(p); err != nil {
				return nil, fmt.Errorf(`bibliography %s: %w
  Paths are resolved relative to the document, not to the working directory.`, ref, err)
			}
			args = append(args, "--bibliography="+p)
		}
		if csl := d.Meta.CSL; csl != "" {
			if !filepath.IsAbs(csl) {
				csl = filepath.Join(docDir, csl)
			}
			if _, err := os.Stat(csl); err != nil {
				return nil, fmt.Errorf("csl %s: %w", d.Meta.CSL, err)
			}
			args = append(args, "--csl="+csl)
		}
	}

	o.logf("  pandoc %s", stem+".md")
	pandocOut, err := run.Cmd(work, "pandoc", args...)
	if err != nil {
		return nil, err
	}
	if keys := missingCitations(pandocOut); len(keys) > 0 {
		return nil, fmt.Errorf(`the bibliography has no entry for %d citation key(s):
  %s
citeproc prints those as "(key?)" in the finished PDF and exits 0, so nothing
else would have told you. Fix the key or add the entry.`,
			len(keys), strings.Join(keys, ", "))
	}

	// xelatex, run here so the log is ours to read.
	passes := 2
	if d.Meta.TOC != nil && *d.Meta.TOC {
		passes = 3
	}
	for i := 0; i < passes; i++ {
		o.logf("  xelatex pass %d/%d", i+1, passes)
		if _, err := run.Cmd(work, "xelatex", "-interaction=nonstopmode", "-halt-on-error", stem+".tex"); err != nil {
			return nil, fmt.Errorf("xelatex failed: %w", err)
		}
	}

	logRaw, _ := os.ReadFile(filepath.Join(work, stem+".log"))
	holes, over, pages, headShort := scanLog(string(logRaw))
	rep.Pages = pages
	if len(holes) > 0 && !o.AllowHoles {
		return nil, fmt.Errorf(`the font has no glyph for %d character(s) the document uses, so they
would print as nothing at all and only this log would know:
  %s
Fix the text (a ballpoint tick beats a missing ☐ anyway) or pick a font that
covers it; override with --allow-missing-glyphs if you truly want the holes`,
			len(holes), strings.Join(holes, "\n  "))
	}
	if headShort > 0 {
		return nil, fmt.Errorf(`the running header is %.1fpt taller than its box, so its logo prints
across the first line of text on every page — and only the XeLaTeX log knew.
Raise page.headheight by at least %.0fpt in the brand bundle, or lower
page.logo_width_header so the mark is shorter.`, headShort, math.Ceil(headShort))
	}
	if over > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf(
			"%d line(s) overflow the measure by more than 5pt — usually a wide table or an unbreakable URL", over))
	}

	out := o.Output
	if out == "" {
		out = filepath.Join(filepath.Dir(o.Input), stem+".pdf")
	}
	if err := copyFile(filepath.Join(work, stem+".pdf"), out); err != nil {
		return nil, err
	}
	rep.Output = out
	return rep, nil
}

func figTools(figs []*doc.Fig) []string {
	need := map[string]bool{}
	for _, f := range figs {
		if f.Kind == "d2" {
			need["d2"] = true
		} else {
			need["vl2svg"] = true
		}
	}
	out := make([]string, 0, len(need))
	for k := range need {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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
	// The belt to tex.HeaderHeightMM's braces. If anything still puts more in
	// the running header than its box can hold — a logo whose size could not be
	// measured, a headheight declared taller than this code can foresee — the
	// mark prints across the first line of every page, and fancyhdr says so
	// only here.
	headRe  = regexp.MustCompile(`Package fancyhdr Warning: \\headheight is too small \(([0-9.]+)pt too short\)`)
	pagesRe = regexp.MustCompile(`Output written on .*? \((\d+) pages?`)
)

// scanLog pulls the three things that matter out of a xelatex log.
func scanLog(log string) (holes []string, overfull, pages int, headShortPt float64) {
	seen := map[string]bool{}
	for _, m := range missingRe.FindAllStringSubmatch(log, -1) {
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
	for _, m := range overRe.FindAllStringSubmatch(log, -1) {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > 5 {
			overfull++
		}
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
	return holes, overfull, pages, headShortPt
}

func copyFile(src, dst string) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, raw, 0o644)
}

func mustAbs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

// citeprocMissingRe matches pandoc's own wording for a citation key that has no
// entry: "[WARNING] Citeproc: citation fml not found".
var citeprocMissingRe = regexp.MustCompile(`Citeproc: citation ([^\s]+) not found`)

// missingCitations extracts the unresolved keys from pandoc's output, in order
// and without repeats. pandoc reports them as warnings and still exits 0, which
// is how a PDF reaches a reader with "(fml?)" printed in the middle of a
// sentence.
func missingCitations(out string) []string {
	var keys []string
	seen := map[string]bool{}
	for _, m := range citeprocMissingRe.FindAllStringSubmatch(out, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			keys = append(keys, m[1])
		}
	}
	return keys
}
