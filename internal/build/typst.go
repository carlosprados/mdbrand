package build

// The PDF typeset by Typst instead of XeLaTeX, when the document's engine is
// typst. Everything before the typesetter — brand, data, words, sized
// figures — is prepare's, unchanged; this file is what a Typst engine has to
// do differently. Typst writes no log that names a defect, so what XeLaTeX's
// log witnessed is either asserted inside the document — the template
// measures and panics with an MDBRAND- message — or read off the PDF.

import (
	"bytes"
	"cmp"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/doc"
	"github.com/carlosprados/mdbrand/internal/exit"
	"github.com/carlosprados/mdbrand/internal/run"
	"github.com/carlosprados/mdbrand/internal/tex"
	"gopkg.in/yaml.v3"
)

// The engines that typeset a PDF. XeLaTeX is the default; Typst is chosen
// per document, by flag or by configuration.
const (
	EngineLaTeX = "latex"
	EngineTypst = "typst"
)

// typstReadBack reads the compiled PDF for what Typst set in silence: a face
// the bundle does not name, a text layer that copies wrong, a heading that
// never printed, a line past the measure, a picture under its resolution.
func typstReadBack(p *prepared, pdf, source, pandocOut string, faces typstFaces) error {
	work, rep := p.work, p.rep
	if err := typstFonts(work, pdf, faces, p.placedPDFs()); err != nil {
		return err
	}
	heads, tocDepth := soughtHeadings(p, pandocOut)
	if err := checkTextLayer(work, pdf, source, heads, tocDepth, rep); err != nil {
		return err
	}
	if p.style == tex.Slides {
		if err := typstDeckWidths(work, pdf); err != nil {
			return err
		}
	} else if err := pastMeasure(work, pdf, p.b, rep); err != nil {
		return err
	}
	return checkResolution(work, pdf, p.pictures(), rep)
}

// typstPandoc is the oldest pandoc whose Typst writer Typst 0.15 reads as
// written. 3.6 and before cite with a string where a label is now required,
// and stopped the build on any document with a bibliography — or, worse,
// printed a citation as "a cross-reference to nothing".
var typstPandoc = []int{3, 7}

// checkTypstPandoc stops a Typst build on a pandoc too old to write for it.
func checkTypstPandoc() error {
	out, err := run.Stdout("", "pandoc", "--version")
	if err != nil {
		return err
	}
	m := pandocVersionRe.FindStringSubmatch(string(out))
	if m == nil || versionAtLeast(m[1], typstPandoc) {
		return nil
	}
	return exit.AsEnvironment(fmt.Errorf(`engine typst needs pandoc 3.7 or later, and this is pandoc %s: an older
pandoc writes Typst that Typst refuses, or reads wrong. Install a newer pandoc
(https://github.com/jgm/pandoc/releases), or build with --engine latex`, m[1]))
}

// versionAtLeast reports whether a dotted version is at least min, part by part.
func versionAtLeast(v string, min []int) bool {
	parts := strings.Split(v, ".")
	for i, want := range min {
		got := 0
		if i < len(parts) {
			got, _ = strconv.Atoi(parts[i])
		}
		if got != want {
			return got > want
		}
	}
	return true
}

// latexOnlyVars are pandoc variables XeLaTeX's template reads from the front
// matter and Typst's never does. Under engine typst each one would be dropped
// in silence: a document that asked for a typeface or a class got another.
var latexOnlyVars = []string{"mainfont", "sansfont", "monofont", "mathfont", "fontfamily",
	"fontfamilyoptions", "CJKmainfont", "documentclass", "classoption", "indent",
	"secnumdepth", "pagestyle", "mainfontoptions", "sansfontoptions", "monofontoptions"}

// typstFontSize reads the front matter's fontsize — the one pandoc variable
// of the kind Typst can honour — and refuses the others it would drop. A
// thesis's size is the UPM's and a brand's; on XeLaTeX's path mdbrand sets
// those variables itself, so the document's own are dropped there too. 10 is
// article's default.
func typstFontSize(front, style string) (float64, error) {
	var m map[string]any
	if err := yaml.Unmarshal([]byte(front), &m); err != nil {
		return 0, err
	}
	var dropped []string
	for _, k := range latexOnlyVars {
		v, ok := m[k]
		if !ok || v == nil || (k == "monofont" && v == typstMono) {
			continue
		}
		if k == "mainfont" || ((k == "documentclass" || k == "classoption") && style == tex.Thesis) {
			continue // mdbrand sets these on XeLaTeX's path as well
		}
		dropped = append(dropped, k)
	}
	size := 10.0
	if v, ok := m["fontsize"]; ok && v != nil && style != tex.Thesis {
		s := strings.TrimSpace(fmt.Sprint(v))
		n, err := strconv.ParseFloat(strings.TrimSuffix(s, "pt"), 64)
		switch {
		case style == tex.Slides:
			dropped = append(dropped, "fontsize")
		case err != nil || !strings.HasSuffix(s, "pt") || n < 8 || n > 14:
			return 0, fmt.Errorf("fontsize: %q: write it in points, 8pt to 14pt, as 12pt", s)
		default:
			size = n
		}
	}
	if len(dropped) > 0 {
		return 0, fmt.Errorf(`the front matter sets %s, which XeLaTeX's template reads and Typst's
never does: under engine typst the document would print without it. Take it
out, or build with --engine latex. Code is set in %s`, strings.Join(dropped, ", "), typstMono)
	}
	return size, nil
}

// typstFigures places each sized figure in the body: the PDF rsvg-convert
// made for XeLaTeX, not the SVG, since Typst would draw an SVG's text from its
// own faces, which are the bundle's alone. Under a PDF standard that PDF is
// redrawn with its text as outlines, and an author's PDF picture is refused.
func typstFigures(p *prepared) (string, error) {
	if len(p.pdfStandard) > 0 {
		if err := refusePDFPictures(p.d.Refs); err != nil {
			return "", err
		}
	}
	body := p.body
	for i, f := range p.figs {
		r := p.results[i]
		file, err := stdPicture(p.work, r.PDF, p.pdfStandard)
		if err != nil {
			return "", err
		}
		body = strings.Replace(body, f.Placeholder, r.Picture(filepath.Base(file), r.WidthMM), 1)
	}
	return body, nil
}

// TypstInstall is how to get the typst program, for doctor and the build.
const TypstInstall = "cargo install --locked typst-cli (brew install typst)  ·  https://github.com/typst/typst/releases"

// Engines are the values engine may take.
var Engines = []string{EngineLaTeX, EngineTypst}

//go:embed typst.lua
var typstFilter []byte

// typstMono is the code face. XeLaTeX's is Latin Modern Mono, which Typst
// cannot find by name on every machine.
const typstMono = "DejaVu Sans Mono"

// typstSerif is the body face when the default bundle's Inter is absent:
// Typst's own, as Latin Modern is XeLaTeX's.
const typstSerif = "Libertinus Serif"

func renderTypst(p *prepared, out string, inputs *[]string) error {
	o, d, b, work, rep := p.o, p.d, p.b, p.work, p.rep
	switch p.style {
	case "report", "note", "letter", tex.Thesis, tex.Slides:
	default:
		return fmt.Errorf("the Typst engine sets no style %s", p.style)
	}
	deck := p.style == tex.Slides
	if _, err := typstFontSize(d.FrontMatter, p.style); err != nil {
		return err
	}
	if _, err := bodyFontArgs(b, rep, nil, typstSerif); err != nil {
		return err
	}

	body, err := typstFigures(p)
	if err != nil {
		return err
	}
	var pages *tex.ThesisPages
	if p.style == tex.Thesis {
		pages = thesisPages(d, p)
		body = thesisBody(p.thesis, body, pages.EN, d.Meta.CitesFrom(), typstMarks(pages))
	}
	if err := typstStyles(b, body); err != nil {
		return err
	}

	stem := workStem(o.Input)
	mdName := stem + ".typst.md"
	front, err := p.pdfFrontMatter()
	if err != nil {
		return err
	}
	source, err := writePDFSource(filepath.Join(work, mdName), o.Input, front, body)
	if err != nil {
		return err
	}

	faces, err := typstFontSet(work, b)
	if err != nil {
		return err
	}
	shown, plain := d.Meta.Printed()
	if err := typstGlyphs(faces, body, append(shown, b.Footer), plain, o.AllowHoles); err != nil {
		return err
	}
	tpl, err := typstTemplate(p, faces, pages)
	if deck {
		tpl, err = typstDeckTemplate(p, faces)
	}
	if err != nil {
		return err
	}
	luaFilter := typstLua(d, deck)
	if len(p.pdfStandard) > 0 {
		luaFilter = bytes.Replace(luaFilter, []byte("local alt = false"), []byte("local alt = true"), 1)
	}
	for name, content := range map[string][]byte{"mdbrand.typst": tpl, "typst.lua": luaFilter} {
		if err := os.WriteFile(filepath.Join(work, name), content, 0o644); err != nil {
			return err
		}
	}
	filter, err := writeSpanFilter(work, b)
	if err != nil {
		return err
	}
	if deck {
		// spans.lua's deck rules are beamer's: a .keep on a slide is refused.
		path := filepath.Join(work, spanFilterName)
		f, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f = bytes.Replace(f, []byte(`if FORMAT == "beamer" then`), []byte(`if FORMAT == "beamer" or FORMAT == "typst" then`), 1)
		if err := os.WriteFile(path, f, 0o644); err != nil {
			return err
		}
	}
	args := []string{
		mdName, "-s", "-t", "typst", "--template=mdbrand.typst", "-o", stem + ".typ",
		"--lua-filter=typst.lua", filter, resourcePath(work, o.Input),
	}
	if d.Meta.NumberSections || pages != nil {
		args = append(args, "--number-sections")
	}
	cites, err := citeArgs(d, o.Input, inputs)
	if err != nil {
		return err
	}
	args = append(args, cites...)
	o.logf("  pandoc %s (typst)", mdName)
	pandocOut, err := run.Cmd(work, "pandoc", args...)
	if err != nil {
		return err
	}
	if err := pdfPandocProblems(d.Meta, b, pandocOut, deck, rep); err != nil {
		return err
	}
	if err := typstFallbackStyles(faces, b, pandocOut); err != nil {
		return err
	}
	if needsAlt(p.pdfStandard) {
		if err := missingAlt(pandocOut); err != nil {
			return err
		}
	}
	if cmds := uniqueMatches(rawTeXRe, pandocOut); len(cmds) > 0 {
		return fmt.Errorf("raw LaTeX that Typst cannot set: %s. It would be left out of the PDF\n"+
			"  without a word; write it in Markdown (a cross-reference is @fig:x, @tbl:x, @sec:x)",
			strings.Join(cmds, ", "))
	}

	o.logf("  typst compile %s.typ", stem)
	cargs := typstCompileArgs(faces, stem)
	if len(p.pdfStandard) > 0 {
		cargs = append(cargs[:len(cargs)-2:len(cargs)-2], "--pdf-standard", strings.Join(p.pdfStandard, ","), stem+".typ", stem+".pdf")
	}
	tout, err := run.Cmd(work, "typst", cargs...)
	if err != nil {
		return typstPanic(err, p)
	}
	if m := typstUnknownFontRe.FindStringSubmatch(tout); m != nil {
		return fmt.Errorf("Typst does not know the font family %q and set those words in another face", m[1])
	}
	if w := strings.TrimSpace(tout); w != "" {
		rep.Warnings = append(rep.Warnings, "typst: "+w)
	}

	pdf := stem + ".pdf"
	if err := typstReadBack(p, pdf, source, pandocOut, faces); err != nil {
		return err
	}
	count := 0
	if info, err := run.Stdout(work, "pdfinfo", pdf); err == nil {
		if m := regexp.MustCompile(`(?m)^Pages:\s+(\d+)`).FindSubmatch(info); m != nil {
			count, _ = strconv.Atoi(string(m[1]))
		}
	}
	rep.Outputs = append(rep.Outputs, Output{Format: "pdf", Path: out, Pages: count, built: filepath.Join(work, pdf)})
	return nil
}

// typstLua is typst.lua as this document needs it: cutting a deck, printing
// citations as text without a bibliography, and its language's words.
func typstLua(d *doc.File, deck bool) []byte {
	luaFilter := typstFilter
	if deck {
		luaFilter = bytes.Replace(luaFilter, []byte("local deck = false"), []byte("local deck = true"), 1)
	}
	if !d.Meta.CitesFrom() {
		luaFilter = bytes.Replace(luaFilter, []byte("local citeproc = true"), []byte("local citeproc = false"), 1)
	}
	if strings.HasPrefix(strings.ToLower(d.Meta.Lang), "en") {
		luaFilter = bytes.Replace(luaFilter,
			[]byte(`{ fig = "figura", tbl = "tabla", eq = "ecuación", sec = "sección" }`),
			[]byte(`{ fig = "figure", tbl = "table", eq = "equation", sec = "section" }`), 1)
	}
	return luaFilter
}

var typstLabelRe = regexp.MustCompile("label `<([^>]+)>` does not exist")

var typstPanicRe = regexp.MustCompile(`panicked with: "?MDBRAND-([A-Z-]+) ?([^"\n]*)`)

// typstPanic turns the template's own assertions into the build's words.
func typstPanic(err error, p *prepared) error {
	// A reference to nothing is Typst's own error, once per label: the same
	// words a LaTeX build says, naming each.
	if labels := typstLabelRe.FindAllStringSubmatch(err.Error(), -1); len(labels) > 0 {
		var refs []string
		for _, l := range labels {
			if r := l[1]; !slices.Contains(refs, r) {
				refs = append(refs, r)
			}
		}
		return crossrefMissError(refs)
	}
	m := typstPanicRe.FindStringSubmatch(err.Error())
	if m == nil {
		return err
	}
	arg := strings.ReplaceAll(m[2], `\"`, `"`)
	if derr := typstDeckPanic(p, m[1], arg); derr != nil {
		return derr
	}
	switch m[1] {
	case "KEEP-TALL":
		return fmt.Errorf(`::: {.keep} "%s": the block is taller than a page, so it cannot be kept on
one; Typst would set it past the foot of the paper. Split it, or drop .keep`, arg)
	case "HEADER-TITLE":
		return fmt.Errorf(`the running header has one line beside the logo, and the title takes %s of it.
Set mdbrand.short_title to a form that fits`, arg)
	case "FOOTER-LABEL":
		return fmt.Errorf(`the confidentiality label is too wide for the footer (%s): shorten
mdbrand.confidential`, arg)
	case "HEADER-LOGO":
		h, room, _ := strings.Cut(arg, " ")
		return fmt.Errorf(`the header's logo is %s tall at logo_width_header, and the top margin
leaves %s for it: it would print across the page's edge. Raise page.margin,
or lower page.logo_width_header in the bundle`, h, room)
	case "COVER-ROOM":
		return fmt.Errorf(`the cover leaves %s of height for mdbrand.cover.image, under the 45mm a
picture needs to read: shorten the title or the subtitle, or drop the picture`, arg)
	}
	return err
}

// typstStyles stops a document whose body face lacks a style it asks for:
// Typst sets an emphasis in a face without italic upright, and says nothing.
func typstStyles(b *brand.Brand, body string) error {
	if !b.BodyFontInstalled() {
		return nil
	}
	styles, err := run.Cmd("", "fc-list", b.Fonts.Body, "style")
	if err != nil {
		return nil
	}
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(styles, w) {
				return true
			}
		}
		return false
	}
	var lost []string
	if emphRe.MatchString(body) && !has("Italic", "Oblique") && !slices.Contains(b.Fonts.Synthesize, "italic") {
		lost = append(lost, "italic")
	}
	if smallCapsRe.MatchString(body) && !fontHasFeature(b.Fonts.Body, "smcp") {
		// Typst draws small caps only from a face's smcp feature.
		lost = append(lost, "small caps (Typst sets them only from the face's own)")
	}
	if len(lost) == 0 {
		return nil
	}
	return fmt.Errorf("%s has no %s, and the document asks for it: it would print as the regular.\n"+
		"  Set fonts.synthesize in the bundle, or use a family that has it", b.Fonts.Body, strings.Join(lost, ", "))
}

var (
	emphRe      = regexp.MustCompile(`(^|[^*\w])\*[^*\s][^*]*\*|(^|\W)_[^_\s][^_]*_`)
	smallCapsRe = regexp.MustCompile(`\{\.smallcaps\}`)
)

type typstData struct {
	Primary, Text, Rule, Link, Accent string
	Body, Display, DisplayB, Mono     string
	DisplayW, DisplayBW               int
	PDFAuthor                         string // quoted; "" without an author
	Margin, Paper, Lang               string
	Leading                           string
	Logo, LogoWidthHeader, LogoCover  string
	Logo2, Logo2Cover                 string
	Footer, Confidential, Reference   string
	Title, Subtitle, Running          string
	Cover, Credit                     string
	Report, TOC                       bool
	TOCDepth                          int
	BodySize                          string
	Letter                            bool
	Recipient, Signature              []string
	PlaceDate, Greeting               string
	HasTitle                          bool
	TableKeep                         int
	Thesis                            *tex.ThesisPages
	Author, Programme, Degree, Year   string
	Directors                         []string
	Credits                           []string
	Funding                           string
	EN                                bool
}

// typstMarks are a thesis's three turns in Typst: the contents and lists,
// the first chapter — arabic numbers from 1, numbered chapters — and the
// first appendix, lettered.
func typstMarks(pg *tex.ThesisPages) thesisMarks {
	names := [3]string{"Tabla de Contenido", "Lista de Figuras", "Lista de Tablas"}
	if pg.EN {
		names = [3]string{"Contents", "List of Figures", "List of Tables"}
	}
	raw := func(s string) string { return "```{=typst}\n" + s + "\n```\n\n" }
	return thesisMarks{
		contents: raw(fmt.Sprintf(`#outline(title: [%s], depth: %d)
#outline(title: [%s], target: figure.where(kind: image))
#outline(title: [%s], target: figure.where(kind: table))`, names[0], pg.TOCDepth, names[1], names[2])),
		mainmatter: raw(`#set page(numbering: "1")
#counter(page).update(1)
#set heading(numbering: "1.1")`),
		appendix: raw(`#in-appendix.update(true)
#counter(heading).update(0)
#set heading(numbering: "A.1")`),
	}
}

// typstBaseData is what every Typst template reads from the brand and the
// front matter.
func typstBaseData(p *prepared, faces typstFaces) (typstData, error) {
	b, m := p.b, p.d.Meta
	lang := m.Lang
	if lang == "" {
		lang = "en"
	}
	lang, _, _ = strings.Cut(lang, "-")
	body := strconv.Quote(faces.Body)
	if faces.Fallback != "" {
		body += ", " + strconv.Quote(faces.Fallback)
	}
	inline, err := inlineMarkdown(p.work, "typst", m.Title, m.Subtitle, m.RunningTitle(), m.Options.Cover.Credit)
	if err != nil {
		return typstData{}, err
	}
	// content is a front matter string as Typst markup: pandoc's rendering
	// of its Markdown, or the string itself, escaped, inside [ ].
	content := func(s string) string {
		if r, ok := inline[s]; ok {
			return "[" + r + "]"
		}
		return "[" + typstEscape(s) + "]"
	}
	q := strconv.Quote
	td := typstData{
		Primary: b.Colors.Primary, Text: b.Colors.Text, Rule: b.Colors.Rule,
		Link: b.Colors.Link, Accent: b.Colors.Accent,
		Body: body, Display: q(faces.Display), DisplayB: q(faces.DisplayBold), Mono: q(typstMono),
		DisplayW: faces.Weight, DisplayBW: faces.BoldWeight,
		PDFAuthor: map[bool]string{true: q(m.AuthorString())}[m.AuthorString() != ""],
		Margin:    b.Page.Margin, Paper: map[string]string{"a4": "a4", "letter": "us-letter"}[strings.ToLower(b.Page.PaperSize)],
		Lang:    q(lang),
		Leading: fmt.Sprintf("%.2fem", 0.65*max(b.Page.LineStretch, 1)),
		Title:   content(m.Title), Running: content(m.RunningTitle()),
		Footer: content(b.Footer), Confidential: content(m.Options.Confidential), Reference: content(m.Options.Reference),
		LogoWidthHeader: b.Page.LogoWidthHeader, LogoCover: b.Page.LogoWidthCover, Logo2Cover: b.Page.LogoWidthCoverSecondary,
		Report: p.style == "report", TOC: m.HasTOC(), TOCDepth: cmp.Or(m.TOCDepth, 3), TableKeep: tex.TableKeep,
		Author: "[" + typstEscape(m.AuthorString()) + "]",
	}
	if m.Subtitle != "" {
		td.Subtitle = content(m.Subtitle)
	}
	if m.Options.Reference == "" {
		td.Reference = ""
	}
	if m.Options.Confidential == "" {
		td.Confidential = ""
	}
	if b.Footer == "" {
		td.Footer = ""
	}
	if p.cover != "" {
		td.Cover = q(p.cover)
		if m.Options.Cover.Credit != "" {
			td.Credit = content(m.Options.Cover.Credit)
		}
	}
	if td.Paper == "" {
		td.Paper = "a4"
	}
	// Under a PDF standard a PDF logo is placed as its SVG, and every picture
	// the template sets says what it is: the bundle's name, the cover's credit.
	logo := func(l string) (string, error) {
		f, err := stdPicture(p.work, l, p.pdfStandard)
		if err != nil || len(p.pdfStandard) == 0 {
			return q(f), err
		}
		return q(f) + ", alt: " + q(cmp.Or(b.DisplayName, b.Name)), nil
	}
	if l := b.LogoPath(); l != "" {
		if td.Logo, err = logo(l); err != nil {
			return td, err
		}
	}
	if l := b.LogoSecondaryPath(); l != "" {
		if td.Logo2, err = logo(l); err != nil {
			return td, err
		}
	}
	if td.Cover != "" && len(p.pdfStandard) > 0 {
		td.Cover += ", alt: " + q(cmp.Or(m.Options.Cover.Credit, m.Title))
	}
	if p.style == "letter" {
		o := m.Options
		td.Letter = true
		for _, r := range o.To {
			td.Recipient = append(td.Recipient, "["+typstEscape(r)+"]")
		}
		if o.Signature != "" {
			for _, l := range strings.Split(strings.TrimRight(o.Signature, "\n"), "\n") {
				td.Signature = append(td.Signature, "["+typstEscape(l)+"]")
			}
		}
		pd := o.Place
		if pd != "" && m.Date != "" {
			pd += ", "
		}
		td.PlaceDate = "[" + typstEscape(pd+m.Date) + "]"
		if o.Greeting != "" {
			td.Greeting = "[" + typstEscape(o.Greeting) + "]"
		}
		td.HasTitle = m.Title != ""
	}
	return td, nil
}

func typstTemplate(p *prepared, faces typstFaces, pages *tex.ThesisPages) ([]byte, error) {
	td, err := typstBaseData(p, faces)
	if err != nil {
		return nil, err
	}
	td.Thesis = pages
	size, err := typstFontSize(p.d.FrontMatter, p.style)
	if err != nil {
		return nil, err
	}
	td.BodySize = strconv.FormatFloat(size, 'f', -1, 64)
	if pages != nil {
		td.EN = pages.EN
		td.Programme, td.Degree, td.Year = "["+typstEscape(pages.Programme)+"]", "["+typstEscape(pages.Degree)+"]", pages.Year
		td.Footer = "[" + typstEscape(strings.ToUpper(pages.University)) + "]"
		td.Reference = "[" + typstEscape(pages.Centre) + "]"
		if pages.Funding != "" {
			td.Funding = "[" + typstEscape(pages.Funding) + "]"
		}
		for _, dir := range pages.Directors {
			role := map[[2]bool]string{{false, false}: "Director", {false, true}: "Codirector", {true, false}: "Supervisor", {true, true}: "Co-supervisor"}[[2]bool{pages.EN, dir.Co}]
			td.Directors = append(td.Directors, "["+typstEscape(dir.Name)+"]")
			td.Credits = append(td.Credits, "["+typstEscape(fmt.Sprintf("%s, %s, %s (%s)", dir.Name, dir.Position, dir.Institution, role))+"]")
		}
		t, err := template.New("typst").Delims("[[", "]]").Parse(typstThesisSrc)
		if err != nil {
			return nil, err
		}
		var out bytes.Buffer
		err = t.Execute(&out, td)
		return out.Bytes(), err
	}
	t, err := template.New("typst").Delims("[[", "]]").Parse(typstTemplateSrc)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	err = t.Execute(&out, td)
	return out.Bytes(), err
}

// typstEscape escapes a plain string for Typst markup.
func typstEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(`\#*_$@<>[]`+"`"+`~=-/+"`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// typstTemplateSrc is a pandoc template ($…$) built by a Go template ([[…]]).
// Every place XeLaTeX's log used to witness is an assertion here: the
// template measures and panics with MDBRAND-, which typstPanic words.
const typstTemplateSrc = `#let brand-primary = rgb("#[[.Primary]]")
#let brand-text = rgb("#[[.Text]]")
#let brand-rule = rgb("#[[.Rule]]")
#let brand-link = rgb("#[[.Link]]")
#let brand-accent = rgb("#[[.Accent]]")
// pandoc writes a --- between paragraphs as #horizontalrule, which its own
// template defines; XeLaTeX draws half the line's width, centred.
// Typst's own entry puts a word joiner (U+2060) before each page number,
// and the text layer carried it: a character the author never wrote.
#show outline.entry: it => link(it.element.location(), it.indented(it.prefix(),
  [#it.body()#box(width: 1fr, if it.fill != none { [ #it.fill ] } else { [ ] })#it.page()]))
#let horizontalrule = align(center, line(length: 50%, stroke: 0.5pt))
#let display = ([[.Display]], [[.Body]])
#let display-b = ([[.DisplayB]], [[.Body]])
#let display-w = [[.DisplayW]]
#let display-bw = [[.DisplayBW]]
#let running = [[.Running]]
#let margin = [[.Margin]]
#let logo-header = [[.LogoWidthHeader]]

#set document(title: [[.Title]][[if .PDFAuthor]], author: [[.PDFAuthor]][[end]])
// The document's fontsize, as pandoc's LaTeX template takes it; the sizes
// below are its multiples, as article's \Large and \large are.
#let body-size = [[.BodySize]]pt
#set text(font: ([[.Body]],), size: body-size, lang: [[.Lang]], fallback: true)
#set par(justify: true, leading: [[.Leading]], first-line-indent: 0pt, spacing: 1.1em)
// Typst sets raw at 0.8em before this rule: 1.125em makes it 0.9 of the text.
#show raw: set text(font: [[.Mono]], size: 1.125em)
// A code block too wide for the measure steps down as XeLaTeX's does, before
// it is let run on and warned about. DejaVu Sans Mono is wider than Latin
// Modern Mono, so the floor is 0.7 of the text, not XeLaTeX's 0.8: as wide as
// LM Mono at 8pt, with a taller x-height (3.8pt to 3.45pt). At 0.8 a block of
// 120 columns that XeLaTeX kept on the paper ran off it.
#show raw.where(block: true): it => layout(size => context {
  let widest(t) = calc.max(0pt, ..it.lines.map(l => measure(text(font: [[.Mono]], size: t, l.text)).width))
  let t = (0.9, 0.8, 0.7).map(f => f * body-size).find(t => widest(t) <= size.width)
  if t == none { t = 0.7 * body-size }
  // Set here, the size is the one printed: raw's relative sizes are spent.
  set text(size: t)
  it
})
#show math.equation: set text(font: "New Computer Modern Math")
#show heading: set text(font: display-b, fill: brand-text, weight: display-bw)
#show heading.where(level: 1): set text(size: 1.4 * body-size)
#show heading.where(level: 2): set text(size: 1.2 * body-size)
#show heading: set block(above: 1.6em, below: 0.9em, sticky: true)
#show link: set text(fill: brand-link)
#show figure.caption: set text(size: 0.9 * body-size)
// pandoc puts every table in a figure, and a figure never breaks: a row
// taller than the page ran through the footer. A short table that fits is
// kept whole instead, since Typst cannot keep rows together across a break.
#let mdbrand-table(rows, captioned, body) = context {
  set figure(numbering: none, outlined: false) if not captioned
  let h = measure(block(width: page.width - 2 * margin, body)).height
  if rows < 2 * [[.TableKeep]] and h <= page.height - 2 * margin {
    body
  } else {
    show figure: set block(breakable: true)
    body
  }
}
#set math.equation(numbering: "(1)")
$if(section-numbering)$
// "3." and "4.1." as XeLaTeX numbers them, while @sec prints the bare 3.
#set heading(numbering: "1.1.")
#show ref: it => {
  let el = it.element
  if el == none or el.func() != heading or el.numbering == none { return it }
  let num = numbering("1.1", ..counter(heading).at(el.location()))
  link(el.location(), if it.supplement == none { num } else { [#it.supplement #num] })
}
$endif$
// A table as XeLaTeX sets one: footnotesize with a little air, ragged cells
// and no hyphens, its caption above. Justified and hyphenated in a 10%
// column, a date printed as "14   jul" and a header as "Arran-que".
// Rows 13pt apart, as arraystretch 1.25 puts them: Typst's line box runs
// from cap height to baseline, and rows 8.8pt apart overlapped, so text
// extraction read the table column by column.
// Columns 12pt apart and the header set off by 6pt more, as \tabcolsep and
// booktabs' rule do.
#set table(stroke: none, inset: (_, y) => (x: 6pt, top: 2pt, bottom: if y == 0 { 8pt } else { 2pt }))
#show table: set text(size: 0.8em, hyphenate: false, top-edge: "ascender", bottom-edge: "descender")
#show table: set par(justify: false, leading: 0.5em)
#show figure.where(kind: table): set figure.caption(position: top)
#set figure.caption(separator: [: ])
#show table.cell.where(y: 0): set text(weight: "bold")

// A kept block is measured at the measure's width before it is set: one
// taller than the text height cannot be kept, and Typst would run it past
// the foot of the paper.
#let mdbrand-keep(name, body) = context {
  let h = measure(block(width: page.width - 2 * margin, body)).height
  if h > page.height - 2 * margin { panic("MDBRAND-KEEP-TALL " + name) }
  block(breakable: false, body)
}

#set page(paper: "[[.Paper]]", margin: margin,
  header: context { if [[if .Letter]]here().page() > 1[[else]]true[[end]] {
    set text(font: display, weight: display-w, size: 8.5pt, fill: brand-text)
    let room = page.width - 2 * margin[[if .Logo]] - logo-header - 2mm[[end]]
    [[if .Logo]]let lh = measure(image([[.Logo]], width: logo-header)).height
    if lh > margin - 10mm { panic("MDBRAND-HEADER-LOGO " + repr(calc.round(lh.mm())) + "mm " + repr(calc.round((margin - 10mm).mm())) + "mm") }
    // measure takes the styles in force at context, not the set rule above:
    // the title was measured at 10pt and a fitting one stopped at 117%.
    [[end]]let w = measure(text(font: display, weight: display-w, size: 8.5pt, running)).width
    if w > room { panic("MDBRAND-HEADER-TITLE " + repr(calc.round(w / room * 100)) + "%") }
    grid(columns: (1fr, auto), align: (left + bottom, right + bottom),
      running, [[if .Logo]]image([[.Logo]], width: logo-header)[[else]][][[end]])
    v(-0.6em)
    line(length: 100%, stroke: 0.4pt + brand-rule)
  } },
  footer: context { if [[if .Letter]]here().page() > 1[[else]]true[[end]] {
    set text(font: display, weight: display-w, size: 8.5pt, fill: brand-text)
    [[if .Confidential]]let label = text(font: display, weight: display-w, size: 8.5pt, fill: brand-primary, [[.Confidential]])
    if measure(label).width > (page.width - 2 * margin) / 2 - 8mm { panic("MDBRAND-FOOTER-LABEL " + repr(calc.round(measure(label).width.mm())) + "mm") }
    [[else]]let label = []
    [[end]]grid(columns: (1fr, auto, 1fr), align: (left, center, right), label, counter(page).display(), [])
  } })

[[if .Report]]
#page(header: none, footer: none)[
  #set par(justify: false)
  #grid(rows: (auto, 1fr, auto, auto), row-gutter: 0mm,
    [[if .Logo2]]grid(columns: (1fr, auto), image([[.Logo]], width: [[.LogoCover]]), image([[.Logo2]], width: [[.Logo2Cover]]))[[else if .Logo]]image([[.Logo]], width: [[.LogoCover]])[[else]][][[end]],
    [[if .Cover]]layout(size => {
      let room = calc.min(size.height - 20mm, 0.62 * (page.height - 2 * margin))
      if room < 45mm { panic("MDBRAND-COVER-ROOM " + repr(calc.round(room.mm())) + "mm") }
      v(1fr)
      image([[.Cover]], width: 100%, height: room, fit: "cover")
      [[if .Credit]]text(size: 7.5pt, fill: brand-text)[[.Credit]][[end]]
      v(1fr)
    })[[else]][][[end]],
    [
      #line(length: 100%, stroke: 1.6pt + brand-primary)
      #v(9mm)
      #text(font: display-b, weight: display-bw, size: 27pt, fill: brand-text)[[.Title]]
      [[if .Subtitle]]#v(6mm)
      #text(font: display, weight: display-w, size: 13pt, fill: brand-text)[[.Subtitle]][[end]]
      [[if .Reference]]#v(5mm)
      #text(font: display, weight: display-w, size: 10pt, fill: brand-text)[[.Reference]][[end]]
      #v(20mm)
    ],
    [
      #line(length: 100%, stroke: 0.4pt + brand-rule)
      #v(3.5mm)
      #set text(font: display, weight: display-w, size: 9.5pt, fill: brand-text)
      $for(author)$$author$$sep$, $endfor$ \
      $date$
      [[if .Confidential]]#v(2.5mm) #text(fill: brand-primary)[[.Confidential]][[end]]
      [[if .Footer]]#v(2.5mm) [[.Footer]][[end]]
    ])
]
#counter(page).update(1)
[[else if .Letter]]
// Letterhead: no header or footer on this page, so the label goes under
// the date. The signature follows the body.
#block(width: 100%)[
  #set par(justify: false, spacing: 0pt)
  #set text(font: display, weight: display-w, size: 10pt, fill: brand-text)
  [[if .Logo]]#image([[.Logo]], width: [[.LogoCover]])[[end]]
  #v(14mm)
  [[range .Recipient]]#[[.]] \
  [[end]][[if .Recipient]]#v(10mm)[[end]]
  #[[.PlaceDate]]
  [[if .Confidential]]#v(1.5mm)
  #text(fill: brand-primary)[[.Confidential]][[end]]
  #v(10mm)
  [[if .HasTitle]]#text(weight: "bold", size: 12pt)[[.Title]]
  #v(6mm)[[end]]
]
[[if .Greeting]]#[[.Greeting]]
#v(3mm)[[end]]
[[else]]
#block(width: 100%)[
  #set par(justify: false)
  #text(font: display-b, weight: display-bw, size: 19pt, fill: brand-text)[[.Title]]
  [[if .Subtitle]]#v(2.5mm, weak: true)
  #text(font: display, weight: display-w, size: 11pt, fill: brand-text)[[.Subtitle]][[end]]
  #v(3mm, weak: true)
  #line(length: 100%, stroke: 1pt + brand-primary)
  #v(2mm, weak: true)
  #text(font: display, weight: display-w, size: 9pt, fill: brand-text)[#(($if(author)$[$for(author)$$author$$sep$, $endfor$],$endif$ $if(date)$[$date$],$endif$ [[if .Confidential]]text(fill: brand-primary)[[.Confidential]],[[end]]).join[ · ])]
]
#v(7mm)
[[end]]
[[if .TOC]]
#outline(depth: [[.TOCDepth]])
[[end]]
$body$
[[if .Letter]]
#v(10mm)
#block[
  #set par(justify: false, spacing: 0pt)
  #set text(font: display, weight: display-w, size: 10pt, fill: brand-text)
  [[range .Signature]]#[[.]] \
  [[end]]
]
[[end]]
`

var pdffontsRe = regexp.MustCompile(`(?m)^(?:[A-Z]{6}\+)?(\S+)`)

// typstFonts stops a PDF in which a face nobody chose printed. Typst sees
// the bundle's faces and its own — the code face, the math face, Libertinus
// for a body face that is absent — and the figures' PDFs carry the faces
// rsvg-convert drew their labels in, as do the PDFs the author placed and
// a PDF logo. Anything else, or a display face that
// should have printed and did not, stops the build: Gotham once fell back to
// Inter on every page of a document, exit 0, and the body face was allowed.
func typstFonts(work, pdf string, f typstFaces, placed []string) error {
	fonts := func(file string) ([]string, error) {
		out, err := run.Stdout(work, "pdffonts", file)
		if err != nil {
			return nil, err
		}
		var names []string
		lines := strings.Split(string(out), "\n")
		for _, l := range lines[min(2, len(lines)):] {
			if m := pdffontsRe.FindStringSubmatch(l); m != nil {
				names = append(names, m[1])
			}
		}
		return names, nil
	}
	norm := func(s string) string {
		return strings.ToLower(strings.NewReplacer(" ", "", "-", "", "_", "").Replace(s))
	}
	allowed := []string{norm(f.Body), norm(f.Fallback), norm(f.Display), norm(f.DisplayBold), norm(typstMono), "newcmmath"}
	for _, file := range placed {
		names, err := fonts(file)
		if err != nil {
			return err
		}
		for _, n := range names {
			allowed = append(allowed, norm(n))
		}
	}
	names, err := fonts(pdf)
	if err != nil {
		return err
	}
	var borrowed []string
	displayPrinted := f.Display == f.Body
	for _, n := range names {
		k := norm(n)
		if strings.HasPrefix(k, norm(f.Display)) {
			displayPrinted = true
		}
		if !slices.ContainsFunc(allowed, func(a string) bool { return a != "" && strings.HasPrefix(k, a) }) && !slices.Contains(borrowed, n) {
			borrowed = append(borrowed, n)
		}
	}
	if len(borrowed) > 0 {
		return fmt.Errorf("Typst set text in %s, a face nobody chose: a glyph the bundle's faces lack.\n"+
			"  Replace the character, or name a face that has it as fonts.fallback in the bundle",
			strings.Join(borrowed, ", "))
	}
	if !displayPrinted {
		return fmt.Errorf("the display face %s is in no page of the PDF: Typst set the headings and the cover in another face", f.Display)
	}
	return nil
}

// placedPDFs are the PDFs a document places whole, which carry their faces:
// the figures, the author's own and the bundle's logos.
func (p *prepared) placedPDFs() []string {
	var out []string
	for _, r := range p.results {
		out = append(out, r.PDF)
	}
	for _, f := range append(slices.Clip(p.d.Refs), p.b.LogoPath(), p.b.LogoSecondaryPath()) {
		if strings.EqualFold(filepath.Ext(f), ".pdf") {
			if _, err := os.Stat(f); err == nil {
				out = append(out, f)
			}
		}
	}
	return out
}

// typstUnknownFontRe is Typst's warning for a family it does not have, which
// it follows by setting the words in another.
var typstUnknownFontRe = regexp.MustCompile(`unknown font family: ([^\n]+)`)

// typstCompileArgs compiles stem.typ with the bundle's faces and Typst's own,
// and none of the machine's.
func typstCompileArgs(f typstFaces, stem string) []string {
	return []string{"compile", "--root", "/", "--ignore-system-fonts", "--font-path", f.Dir, stem + ".typ", stem + ".pdf"}
}

// overhangPt is how far Typst hangs punctuation into the margin on purpose,
// at 10pt; the hang grows with the type, so a word is allowed overhangEm of
// its own height where that is more. At 12pt a period hung 3.3pt.
const (
	overhangPt = 3
	overhangEm = 0.35
)

var typstWordRe = regexp.MustCompile(`<word xMin="[0-9.]+" yMin="([0-9.]+)" xMax="([0-9.]+)" yMax="([0-9.]+)">([^<]*)</word>`)

// pastMeasure warns of words past the text's right edge: Typst lets a line of
// code or an unbreakable word run into the margin without a word, where
// XeLaTeX's log said Overfull \hbox.
func pastMeasure(work, pdf string, b *brand.Brand, rep *Report) error {
	marginMM, err := tex.ParseLenMM(b.Page.Margin)
	if err != nil {
		return nil
	}
	out, err := run.Stdout(work, "pdftotext", "-enc", "UTF-8", "-bbox", pdf, "-")
	if err != nil {
		return err
	}
	marginPt := marginMM * 72 / 25.4
	var past []string
	for i, page := range strings.Split(string(out), "<page ")[1:] {
		m := pageRe.FindStringSubmatch("<page " + page)
		if m == nil {
			continue
		}
		width, _ := strconv.ParseFloat(m[1], 64)
		for _, w := range typstWordRe.FindAllStringSubmatch(page, -1) {
			yMin, _ := strconv.ParseFloat(w[1], 64)
			xMax, _ := strconv.ParseFloat(w[2], 64)
			yMax, _ := strconv.ParseFloat(w[3], 64)
			if over := xMax - (width - marginPt); over > max(overhangPt, overhangEm*(yMax-yMin)) {
				past = append(past, fmt.Sprintf("page %d: %q, %.0fpt", i+1, ellipsize(w[4], 40), over))
			}
		}
	}
	if len(past) > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%d line(s) past the measure, into the margin: %s; shorten code lines to fit the columns, where a line can break",
			len(past), strings.Join(past, "; ")))
	}
	return nil
}

// fontHasFeature reports whether family's regular face lists an OpenType
// GSUB feature, by reading the font file's FeatureList: Typst draws small
// caps only from smcp, and fontconfig does not list features.
func fontHasFeature(family, tag string) bool {
	file, err := run.Cmd("", "fc-match", "--format", "%{file}", family)
	if err != nil {
		return false
	}
	raw, err := os.ReadFile(strings.TrimSpace(file))
	if err != nil || len(raw) < 12 {
		return false
	}
	u16 := func(o int) int {
		if o+2 > len(raw) {
			return 0
		}
		return int(raw[o])<<8 | int(raw[o+1])
	}
	u32 := func(o int) int { return u16(o)<<16 | u16(o+2) }
	base := 0
	if string(raw[:4]) == "ttcf" {
		base = u32(12) // the collection's first face
	}
	for i, n := 0, u16(base+4); i < n; i++ {
		rec := base + 12 + 16*i
		if rec+16 > len(raw) || string(raw[rec:rec+4]) != "GSUB" {
			continue
		}
		gsub := u32(rec + 8)
		list := gsub + u16(gsub+6)
		for j, m := 0, u16(list); j < m; j++ {
			at := list + 2 + 6*j
			if at+4 <= len(raw) && string(raw[at:at+4]) == tag {
				return true
			}
		}
	}
	return false
}

// typstThesisSrc is a thesis as the UPM's template lays it out: three fixed
// pages, no running header, the number alone at the foot — none on the first
// two, roman from the credits page, arabic from the first chapter — chapters
// on a page of their own, figures, tables and equations numbered by chapter,
// and appendices lettered. University is in .Footer and the centre in
// .Reference, the report's names for the same two lines.
const typstThesisSrc = `// pandoc writes a --- between paragraphs as #horizontalrule, which its own
// template defines; XeLaTeX draws half the line's width, centred.
// Typst's own entry puts a word joiner (U+2060) before each page number,
// and the text layer carried it: a character the author never wrote.
#show outline.entry: it => link(it.element.location(), it.indented(it.prefix(),
  [#it.body()#box(width: 1fr, if it.fill != none { [ #it.fill ] } else { [ ] })#it.page()]))
#let horizontalrule = align(center, line(length: 50%, stroke: 0.5pt))
#let display = ([[.Display]], [[.Body]])
#let display-b = ([[.DisplayB]], [[.Body]])
#let display-w = [[.DisplayW]]
#let display-bw = [[.DisplayBW]]
#let margin = [[.Margin]]
#let in-appendix = state("in-appendix", false)
#let tr(es, en) = [[if .EN]]en[[else]]es[[end]]

#set document(title: [[.Title]][[if .PDFAuthor]], author: [[.PDFAuthor]][[end]])
#set text(font: ([[.Body]],), size: 11pt, lang: [[.Lang]], fallback: true)
#set par(justify: true, leading: [[.Leading]], first-line-indent: 0pt, spacing: 1.1em)
// Typst sets raw at 0.8em before this rule: 1.125em makes it 0.9 of the text.
#show raw: set text(font: [[.Mono]], size: 1.125em)
// A code block too wide for the measure steps down as XeLaTeX's does, before
// it is let run on and warned about. DejaVu Sans Mono is wider than Latin
// Modern Mono, so the floor is 0.7 of the text, not XeLaTeX's 0.8: as wide as
// LM Mono at 8pt, with a taller x-height (3.8pt to 3.45pt). At 0.8 a block of
// 120 columns that XeLaTeX kept on the paper ran off it.
#show raw.where(block: true): it => layout(size => context {
  let widest(t) = calc.max(0pt, ..it.lines.map(l => measure(text(font: [[.Mono]], size: t, l.text)).width))
  let t = (0.9, 0.8, 0.7).map(f => f * 11pt).find(t => widest(t) <= size.width)
  if t == none { t = 0.7 * 11pt }
  // Set here, the size is the one printed: raw's relative sizes are spent.
  set text(size: t)
  it
})
#show math.equation: set text(font: "New Computer Modern Math")
#show link: set text(fill: rgb("#[[.Link]]"))
#show figure.caption: set text(size: 9.5pt)
// A table as XeLaTeX sets one: footnotesize with a little air, ragged cells
// and no hyphens, its caption above. Justified and hyphenated in a 10%
// column, a date printed as "14   jul" and a header as "Arran-que".
// Rows 13pt apart, as arraystretch 1.25 puts them: Typst's line box runs
// from cap height to baseline, and rows 8.8pt apart overlapped, so text
// extraction read the table column by column.
// Columns 12pt apart and the header set off by 6pt more, as \tabcolsep and
// booktabs' rule do.
#set table(stroke: none, inset: (_, y) => (x: 6pt, top: 2pt, bottom: if y == 0 { 8pt } else { 2pt }))
#show table: set text(size: 0.8em, hyphenate: false, top-edge: "ascender", bottom-edge: "descender")
#show table: set par(justify: false, leading: 0.5em)
#show table.cell.where(y: 0): set text(weight: "bold")
#set page(paper: "a4", margin: margin, header: none, numbering: none)
#set outline.entry(fill: repeat[.])
#show outline.entry.where(level: 1): set text(weight: "bold")

#let mdbrand-keep(name, body) = context {
  let h = measure(block(width: page.width - 2 * margin, body)).height
  if h > page.height - 2 * margin { panic("MDBRAND-KEEP-TALL " + name) }
  block(breakable: false, body)
}
#let mdbrand-table(rows, captioned, body) = context {
  set figure(numbering: none, outlined: false) if not captioned
  let h = measure(block(width: page.width - 2 * margin, body)).height
  if rows < 2 * [[.TableKeep]] and h <= page.height - 2 * margin { body } else {
    show figure: set block(breakable: true)
    body
  }
}

// Figures, tables and equations by chapter: 2.1, A.1. A numbering function
// runs where its element is, for the caption and for every reference to it;
// a context of its own would read the chapter where the reference stands.
#let chapter-at(loc) = numbering(if in-appendix.at(loc) { "A" } else { "1" }, counter(heading).at(loc).first())
#set figure(numbering: n => chapter-at(here()) + "." + str(n))
#set figure.caption(separator: [: ])
#set math.equation(numbering: n => "(" + chapter-at(here()) + "." + str(n) + ")")
// A reference to an equation reads "ecuación 1.1", as pandoc-crossref wrote
// it, not the "(1.1)" beside the formula.
#show ref: it => {
  let el = it.element
  if el == none or el.func() != math.equation { return it }
  let loc = el.location()
  let num = chapter-at(loc) + "." + str(counter(math.equation).at(loc).first())
  link(loc, if it.supplement == none { num } else { [#it.supplement #num] })
}
#show figure.where(kind: table): set figure(supplement: tr([Tabla], [Table]))
#show figure.where(kind: image): set figure(supplement: tr([Figura], [Figure]))
#show figure.where(kind: table): set figure.caption(position: top)

#show heading: set text(font: display-b, weight: display-bw)
#show heading: set block(above: 1.6em, below: 0.9em, sticky: true)
#show heading.where(level: 2): set text(size: 13pt)
#show heading.where(level: 3): set text(size: 11.5pt)
#show heading.where(level: 1): it => {
  pagebreak(weak: true)
  counter(figure.where(kind: image)).update(0)
  counter(figure.where(kind: table)).update(0)
  counter(math.equation).update(0)
  set par(justify: false)
  v(20mm)
  if it.numbering != none {
    block(text(size: 14pt, weight: display-bw, font: display-b)[#context { if in-appendix.get() { tr([Anexo], [Appendix]) } else { tr([Capítulo], [Chapter]) } } #counter(heading).display()])
    v(4mm)
  }
  block(text(size: 22pt, weight: display-bw, font: display-b, it.body))
  v(10mm)
}

// Cubierta.
#align(center)[
  #set text(hyphenate: false)
  #set par(justify: false)
  #text(size: 13pt, [[.Footer]]) \
  #v(4mm)
  #[[.Reference]]
  #v(14mm)
  [[if .Logo]]#image([[.Logo]], width: 50.8mm)[[end]]
  #v(1fr)
  #text(size: 16pt, weight: "bold")[[.Title]]
  #v(10mm)
  *#tr([TESIS DOCTORAL], [DOCTORAL THESIS])*
  #v(10mm)
  #tr([Presentada para optar al título de Doctor por:], [Submitted for the degree of Doctor by:])
  #v(4mm)
  #strong([[.Author]]) \
  #v(2mm)
  #[[.Degree]]
  #v(1fr)
  Madrid, [[.Year]]
]
#pagebreak()

// Portada.
#[
#set text(hyphenate: false)
#set par(justify: false)
#grid(columns: (22%, 22%, 1fr), align: horizon, column-gutter: 2mm,
  [[if .Logo2]]align(center, image([[.Logo2]], width: 32.6mm))[[else]][][[end]],
  [[if .Logo]]align(center, image([[.Logo]], width: 32.6mm))[[else]][][[end]],
  [#[[.Footer]] \ #v(2mm) #[[.Reference]]])
#align(center)[
  #v(18mm)
  #strong[#tr([Doctorado en], [Doctoral Programme in]) #[[.Programme]]]
  #v(1fr)
  #text(size: 16pt, weight: "bold")[[.Title]]
  #v(10mm)
  *#tr([TESIS DOCTORAL], [DOCTORAL THESIS])*
  #v(8mm)
  #tr([Presentada para optar al título de Doctor por:], [Submitted for the degree of Doctor by:])
  #v(3mm)
  #strong([[.Author]]) \
  #[[.Degree]]
  #v(8mm)
  #tr([Bajo la dirección de:], [Supervised by:])
  #v(3mm)
  [[range .Directors]]#[[.]] \
  [[end]]
  #v(1fr)
  Madrid, [[.Year]]
]
]

// Créditos: page i.
#set page(numbering: "i")
#counter(page).update(1)
#[
  #set par(justify: false, spacing: 3mm)
  #tr([Título], [Title]): #[[.Title]]

  #tr([Autor], [Author]): #[[.Author]]

  #tr([Programa de Doctorado], [Doctoral Programme]): #[[.Programme]]

  #tr([Dirección de tesis], [Supervision]):
  [[range .Credits]]\ #[[.]][[end]]

  #tr([Revisores externos], [External reviewers]):

  #tr([Tribunal de tesis], [Thesis committee]):

  #tr([Fecha de defensa], [Date of defence]):

  #v(1fr)
  [[if .Funding]]#tr([Esta tesis ha sido parcialmente financiada por], [This thesis has been partially funded by]) #[[.Funding]].[[end]]
]
#pagebreak()

$body$
`
