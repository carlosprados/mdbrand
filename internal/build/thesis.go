package build

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/doc"
	"github.com/carlosprados/mdbrand/internal/mdtext"
	"github.com/carlosprados/mdbrand/internal/run"
	"github.com/carlosprados/mdbrand/internal/tex"
)

// AbstractMaxChars is the UPM's limit on the Resumen and on the Abstract,
// characters including spaces as Word counts them. It is the template's
// number, not a setting.
const AbstractMaxChars = 4000

// thesisParts are a thesis's front parts as read from their files: Markdown
// with no heading, which mdbrand writes in the template's words.
type thesisParts struct {
	dedication, acknowledgements, abstract, resumen, acronyms string
}

// checkThesisShape refuses, before anything is rendered, a thesis the UPM's
// template would not accept, and the thesis keys on any other style.
func checkThesisShape(d *doc.File, b *brand.Brand, style string) error {
	t := d.Meta.Options.Thesis
	if style != tex.Thesis {
		if t != nil {
			return fmt.Errorf("mdbrand.thesis describes a doctoral thesis, and this is style %s: build it with style: thesis, or drop thesis:", style)
		}
		return nil
	}
	if t == nil {
		return fmt.Errorf(`style thesis prints the UPM's three fixed pages, and mdbrand.thesis is missing:
it names the programme, the previous degree, the directors and the files of the
front parts. See: mdbrand example tesis`)
	}
	lang := strings.ToLower(d.Meta.Lang)
	if !strings.HasPrefix(lang, "es") && !strings.HasPrefix(lang, "en") {
		return fmt.Errorf("lang: %q — a UPM thesis is written in Spanish or English: set lang: es-ES or en-GB", d.Meta.Lang)
	}
	if !strings.EqualFold(b.Page.PaperSize, "a4") && !strings.EqualFold(b.Page.PaperSize, "a4paper") {
		return fmt.Errorf("brand %q sets page.papersize %s, and a UPM thesis is A4", b.Name, b.Page.PaperSize)
	}
	if _, abstract, _ := doc.LiftAbstract(d.FrontMatter); abstract != "" {
		return fmt.Errorf("abstract: in the front matter — a thesis's Abstract is a file of its own, read through mdbrand.thesis.abstract, beside its Resumen")
	}
	var missing []string
	for _, f := range []struct{ key, v string }{
		{"title", d.Meta.Title}, {"author", d.Meta.AuthorString()}, {"date (the year on the cover)", thesisYear(d.Meta.Date)},
		{"mdbrand.thesis.programme", t.Programme}, {"mdbrand.thesis.degree", t.Degree},
		{"mdbrand.thesis.abstract", t.Abstract}, {"mdbrand.thesis.resumen", t.Resumen}, {"mdbrand.thesis.acronyms", t.Acronyms},
		{"thesis.university in the bundle", b.Thesis.University}, {"thesis.centre in the bundle", b.Thesis.Centre},
	} {
		if strings.TrimSpace(f.v) == "" {
			missing = append(missing, f.key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf(`a UPM thesis needs %s.
The three fixed pages print the title, the candidate, the year, the programme,
the previous degree, the university and the centre; and the UPM requires a
Resumen in Spanish and an Abstract in English whatever the thesis's language,
and the list of acronyms`, strings.Join(missing, ", "))
	}
	if len(t.Directors) == 0 {
		return fmt.Errorf(`mdbrand.thesis.directors is missing: the portada prints "Bajo la dirección de:"
and the credits page each director's position and institution`)
	}
	for i, dir := range t.Directors {
		for _, f := range []struct{ key, v string }{{"name", dir.Name}, {"position", dir.Position}, {"institution", dir.Institution}} {
			if f.v == "" {
				return fmt.Errorf("mdbrand.thesis.directors[%d] has no %s: the credits page prints each director's name, position and institution", i, f.key)
			}
		}
	}
	if list := b.Thesis.Programmes; len(list) > 0 && !slices.Contains(list, t.Programme) {
		return fmt.Errorf("mdbrand.thesis.programme %q is not one of the programmes brand %q lists, which are its official names:\n  %s",
			t.Programme, b.Name, strings.Join(list, "\n  "))
	}
	return nil
}

var yearRe = regexp.MustCompile(`\b(19|20)\d\d\b`)

// thesisYear is the year the cover prints, read from date:.
func thesisYear(date string) string { return yearRe.FindString(date) }

// thesisFormats refuses any output but the PDF for a thesis, before another
// format's own checks speak of a bundle the thesis does not need yet. Its
// .docx and .epub come later; built as a report's, they would be a different
// document under the same name.
func thesisFormats(style string, formats []string) error {
	if style != tex.Thesis {
		return nil
	}
	if i := slices.IndexFunc(formats, func(f string) bool { return f != "pdf" }); i >= 0 {
		return fmt.Errorf("style thesis builds as a PDF; its --to %s is not written yet. Build it with --to pdf", formats[i])
	}
	return nil
}

// readThesis reads a thesis's parts, before any figure is rendered: the
// chapters and appendices into the body, so every check after sees one
// document, and the front parts aside.
func (p *prepared) readThesis(inputs *[]string) error {
	if p.style != tex.Thesis {
		return nil
	}
	if err := p.assembleChapters(inputs); err != nil {
		return err
	}
	var err error
	p.thesis, err = readThesisParts(p.o, p.d.Meta.Options.Thesis, p.work, p.rep, inputs)
	return err
}

// assembleChapters puts the chapter and appendix files named by
// mdbrand.thesis in the body, in order, each rebased to the master's
// directory. The first appendix is marked {.appendix} for thesisBody.
func (p *prepared) assembleChapters(inputs *[]string) error {
	t, d := p.d.Meta.Options.Thesis, p.d
	if len(t.Chapters) == 0 {
		if len(t.Appendices) > 0 {
			return fmt.Errorf("mdbrand.thesis.appendices names files, and chapters: does not: name the chapters' files too, or write the appendices in %s after a # … {.appendix} heading", filepath.Base(p.o.Input))
		}
		return nil
	}
	if strings.TrimSpace(d.Body) != "" {
		return fmt.Errorf(`%s has a body, and mdbrand.thesis.chapters names the chapters' files: the
body would have no place in the thesis. Move it into a chapter's file, or drop
chapters: and write the chapters in the body`, filepath.Base(p.o.Input))
	}
	dir := filepath.Dir(mustAbs(p.o.Input))
	var b strings.Builder
	for i, list := range [][]string{t.Chapters, t.Appendices} {
		files, err := doc.ExpandParts(dir, list)
		if err != nil {
			return fmt.Errorf("mdbrand.thesis.%s: %w", map[int]string{0: "chapters", 1: "appendices"}[i], err)
		}
		for j, f := range files {
			*inputs = append(*inputs, f)
			text, err := doc.ReadPart(f, dir)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(dir, f)
			if err := doc.CheckChapter(rel, text); err != nil {
				return err
			}
			if i == 1 && j == 0 {
				text = markAppendix(text)
			}
			b.WriteString(strings.TrimRight(text, "\n") + "\n\n")
		}
	}
	d.Body = b.String()
	return nil
}

var chapterHeadRe = regexp.MustCompile(`(?m)^# [^\n]*$`)

// markAppendix adds the appendix class to a part's chapter heading, inside
// its attributes when it has some.
func markAppendix(text string) string {
	spans := mdtext.Protected(text)
	for _, m := range chapterHeadRe.FindAllStringIndex(text, -1) {
		if mdtext.Inside(spans, m[0]) {
			continue
		}
		h := strings.TrimRight(text[m[0]:m[1]], " \t")
		if strings.HasSuffix(h, "}") && strings.Contains(h, "{") {
			h = h[:len(h)-1] + " .appendix}"
		} else {
			h += " {.appendix}"
		}
		return text[:m[0]] + h + text[m[1]:]
	}
	return text
}

// readThesisParts reads the front parts and holds the Resumen and the
// Abstract to the UPM's rules: at most 4000 characters, plain text.
func readThesisParts(o Options, t *doc.Thesis, work string, rep *Report, inputs *[]string) (*thesisParts, error) {
	dir := filepath.Dir(mustAbs(o.Input))
	read := func(key, rel string) (string, error) {
		if rel == "" {
			return "", nil
		}
		path := rel
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, rel)
		}
		*inputs = append(*inputs, path)
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("mdbrand.thesis.%s: %s: no such file beside the document", key, rel)
		}
		text, err := doc.ReadPart(path, dir)
		if err != nil {
			return "", err
		}
		if h := firstHeading(text); h != "" {
			return "", fmt.Errorf("%s: front parts are headless — %q: mdbrand writes the heading the UPM's template names. Delete it", rel, h)
		}
		return text, nil
	}
	var p thesisParts
	for _, part := range []struct {
		key, rel string
		dst      *string
	}{
		{"dedication", t.Dedication, &p.dedication}, {"acknowledgements", t.Acknowledgements, &p.acknowledgements},
		{"abstract", t.Abstract, &p.abstract}, {"resumen", t.Resumen, &p.resumen}, {"acronyms", t.Acronyms, &p.acronyms},
	} {
		var err error
		if *part.dst, err = read(part.key, part.rel); err != nil {
			return nil, err
		}
	}
	for _, a := range []struct{ name, rel, text, lang string }{
		{"Abstract", t.Abstract, p.abstract, "en"}, {"Resumen", t.Resumen, p.resumen, "es"},
	} {
		if err := checkAbstract(work, a.name, a.rel, a.text, a.lang, rep); err != nil {
			return nil, err
		}
	}
	return &p, nil
}

// firstHeading is the first level-1 ATX heading of a Markdown text outside
// code, or "". Level 2 is the structure a summary may have.
func firstHeading(text string) string {
	spans := mdtext.Protected(text)
	off := 0
	for _, ln := range strings.SplitAfter(text, "\n") {
		if (strings.HasPrefix(ln, "# ") || strings.TrimSpace(ln) == "#") && !mdtext.Inside(spans, off) {
			return strings.TrimSpace(ln)
		}
		off += len(ln)
	}
	return ""
}

// checkAbstract counts and screens one abstract on pandoc's own reading of
// it. The UPM asks for "texto plano (sin símbolos)": a citation prints as
// [12], a formula as a formula, a footnote as a mark, so each stops; a
// symbol character is a reviewer's call, so it warns, as does a text that
// reads as the other language — the likeliest slip is the two files swapped.
func checkAbstract(work, name, rel, text, lang string, rep *Report) error {
	src := filepath.Join(work, ".mdbrand-abstract.md")
	if err := os.WriteFile(src, []byte(text), 0o644); err != nil {
		return err
	}
	out, err := run.Stdout(work, "pandoc", "-f", "markdown", "-t", "json", filepath.Base(src))
	if err != nil {
		return err
	}
	var ast struct {
		Blocks []json.RawMessage `json:"blocks"`
	}
	if err := json.Unmarshal(out, &ast); err != nil {
		return fmt.Errorf("%s: reading pandoc's AST: %w", rel, err)
	}
	var s abstractScan
	for _, b := range ast.Blocks {
		s.block(b)
	}
	if s.offender != "" {
		return fmt.Errorf(`%s: the %s holds %s, and the UPM asks for plain text without symbols:
it prints as something other than words. Write it out in words, or move it to
the body`, rel, name, s.offender)
	}
	if n := utf8.RuneCountInString(s.text.String()); n > AbstractMaxChars {
		return fmt.Errorf("%s: the %s has %d characters including spaces, and the UPM allows %d (%d over). Cut it: the limit is the template's",
			rel, name, n, AbstractMaxChars, n-AbstractMaxChars)
	}
	if syms := symbols(s.text.String()); len(syms) > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: the %s holds %s, which a reviewer may read as symbols the UPM's plain text excludes",
			rel, name, strings.Join(syms, " ")))
	}
	es, en := stopwordShare(s.text.String())
	switch {
	case lang == "en" && en < 0.10 && es > 0.10:
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s reads as Spanish (%.0f%% Spanish stopwords, %.0f%% English): the Abstract is the English summary", rel, es*100, en*100))
	case lang == "es" && es < 0.10 && en > 0.10:
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s reads as English (%.0f%% English stopwords, %.0f%% Spanish): the Resumen is the Spanish summary", rel, en*100, es*100))
	}
	return nil
}

// abstractScan walks an abstract's AST: the text as Word would count it, and
// the first thing in it that is not plain text.
type abstractScan struct {
	text     strings.Builder
	offender string
}

type node struct {
	T string          `json:"t"`
	C json.RawMessage `json:"c"`
}

var refusedInline = map[string]string{
	"Cite": "a citation", "Math": "a formula", "Code": "code", "Note": "a footnote",
	"Link": "a link", "Image": "a picture", "RawInline": "raw markup",
}

func (s *abstractScan) block(raw json.RawMessage) {
	var b node
	if json.Unmarshal(raw, &b) != nil || s.offender != "" {
		return
	}
	switch b.T {
	case "Para", "Plain":
		s.inlines(b.C)
	case "Header":
		// Level 2 only: the structured sections the template asks for.
		var h []json.RawMessage
		var level int
		if json.Unmarshal(b.C, &h) == nil && len(h) == 3 && json.Unmarshal(h[0], &level) == nil && level == 2 {
			s.inlines(h[2])
			return
		}
		s.offender = "a heading other than ## (a section of the summary)"
	default:
		s.offender = map[string]string{
			"BulletList": "a list", "OrderedList": "a list", "Table": "a table", "Figure": "a figure",
			"CodeBlock": "a code block", "RawBlock": "raw markup", "BlockQuote": "a quotation block",
			"Div": "a block with attributes", "HorizontalRule": "a rule", "DefinitionList": "a list",
		}[b.T]
		if s.offender == "" {
			s.offender = "a " + b.T
		}
	}
}

func (s *abstractScan) inlines(raw json.RawMessage) {
	var ins []json.RawMessage
	if json.Unmarshal(raw, &ins) != nil {
		return
	}
	for _, r := range ins {
		if s.offender != "" {
			return
		}
		var n node
		if json.Unmarshal(r, &n) != nil {
			continue
		}
		if what, bad := refusedInline[n.T]; bad {
			s.offender = what
			if n.T == "Cite" {
				s.offender += " (" + citeKeys(n.C) + ")"
			}
			return
		}
		switch n.T {
		case "Str":
			var v string
			_ = json.Unmarshal(n.C, &v) // a Str's content is a string
			s.text.WriteString(v)
		case "Space", "SoftBreak", "LineBreak":
			s.text.WriteByte(' ')
		case "Quoted":
			var q []json.RawMessage
			if json.Unmarshal(n.C, &q) == nil && len(q) == 2 {
				s.text.WriteString("«")
				s.inlines(q[1])
				s.text.WriteString("»")
			}
		case "Span":
			var sp []json.RawMessage
			if json.Unmarshal(n.C, &sp) == nil && len(sp) == 2 {
				s.inlines(sp[1])
			}
		default: // Emph, Strong, Underline, SmallCaps, Strikeout, Superscript, Subscript
			s.inlines(n.C)
		}
	}
}

// citeKeys names the keys of a Cite node, as they were written.
func citeKeys(c json.RawMessage) string {
	var parts []json.RawMessage
	if json.Unmarshal(c, &parts) != nil || len(parts) == 0 {
		return ""
	}
	var cites []struct {
		ID string `json:"citationId"`
	}
	_ = json.Unmarshal(parts[0], &cites) // a Cite's first element is its citations
	var keys []string
	for _, ct := range cites {
		keys = append(keys, "@"+ct.ID)
	}
	return strings.Join(keys, "; ")
}

// symbols lists each Unicode symbol character in s once (+, →, €, ©, °).
func symbols(s string) []string {
	var out []string
	for _, r := range s {
		if unicode.IsSymbol(r) && !slices.Contains(out, string(r)) {
			out = append(out, string(r))
		}
	}
	return out
}

var (
	esStop = []string{"de", "la", "el", "que", "los", "las", "una", "para", "con", "por", "del", "se"}
	enStop = []string{"the", "and", "of", "to", "that", "is", "in", "a", "for", "with", "this", "are"}
)

// stopwordShare is the share of s's words that are common Spanish and common
// English function words: on real abstracts about 28% for their own language
// and under 2% for the other.
func stopwordShare(s string) (es, en float64) {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) })
	if len(words) == 0 {
		return 0, 0
	}
	var nes, nen int
	for _, w := range words {
		if slices.Contains(esStop, w) {
			nes++
		}
		if slices.Contains(enStop, w) {
			nen++
		}
	}
	return float64(nes) / float64(len(words)), float64(nen) / float64(len(words))
}

// thesisPages are the strings of the three fixed pages.
func thesisPages(d *doc.File, p *prepared) *tex.ThesisPages {
	t := d.Meta.Options.Thesis
	pages := &tex.ThesisPages{
		University: p.b.Thesis.University, Centre: p.b.Thesis.Centre,
		Programme: t.Programme, Degree: t.Degree, Funding: t.Funding,
		Year: thesisYear(d.Meta.Date), EN: strings.HasPrefix(strings.ToLower(d.Meta.Lang), "en"),
		TOCDepth: d.Meta.TOCDepth,
	}
	if pages.TOCDepth <= 0 {
		pages.TOCDepth = 2
	}
	for _, dir := range t.Directors {
		pages.Directors = append(pages.Directors, tex.ThesisDirector{Name: dir.Name, Position: dir.Position, Institution: dir.Institution, Co: dir.Co})
	}
	return pages
}

// thesisBody assembles the thesis in the order the UPM's template fixes:
// the front parts under the template's headings, the contents and the lists,
// the acronyms, then the chapters with arabic numbers, the references before
// the appendices, and the appendices lettered.
func thesisBody(parts *thesisParts, body string, en, cites bool) string {
	names := map[bool][5]string{
		false: {"Dedicatoria", "Agradecimientos", "Abstract", "Resumen", "Abreviaturas y Acrónimos"},
		true:  {"Dedication", "Acknowledgements", "Abstract", "Resumen", "Abbreviations and Acronyms"},
	}[en]
	raw := func(latex string) string { return "```{=latex}\n" + latex + "\n```\n\n" }
	var b strings.Builder
	if parts.dedication != "" {
		b.WriteString("# " + names[0] + " {.unnumbered .unlisted}\n\n" + parts.dedication + "\n\n")
	}
	if parts.acknowledgements != "" {
		b.WriteString("# " + names[1] + " {.unnumbered}\n\n" + frontSections(parts.acknowledgements) + "\n\n")
	}
	// Each summary is set in its own language's hyphenation, whatever the
	// thesis's.
	b.WriteString("# " + names[2] + " {.unnumbered}\n\n::: {lang=en-GB}\n" + frontSections(parts.abstract) + "\n:::\n\n")
	b.WriteString("# " + names[3] + " {.unnumbered}\n\n::: {lang=es-ES}\n" + frontSections(parts.resumen) + "\n:::\n\n")
	b.WriteString(raw(`\tableofcontents` + "\n" + `\listoffigures` + "\n" + `\listoftables`))
	b.WriteString("# " + names[4] + " {.unnumbered}\n\n" + parts.acronyms + "\n\n")
	b.WriteString(raw(`\mainmatter`))

	main, appendices := splitAppendices(body)
	b.WriteString(main)
	if cites && !strings.Contains(body, "{#refs}") {
		title := map[bool]string{false: "Referencias", true: "References"}[en]
		b.WriteString("\n\n# " + title + " {.unnumbered}\n\n::: {#refs}\n:::\n\n")
	}
	if appendices != "" {
		b.WriteString("\n\n" + raw(`\appendix`) + appendices)
	}
	return b.String()
}

var plainSubheadRe = regexp.MustCompile(`(?m)^(#{2,6} [^\n{]*?)[ \t]*$`)

// frontSections takes a front part's section headings out of the numbering
// and the contents: under an unnumbered chapter they printed as "0.1".
func frontSections(text string) string {
	spans := mdtext.Protected(text)
	return replaceOutside(text, spans, plainSubheadRe, "$1 {.unnumbered .unlisted}")
}

// replaceOutside is ReplaceAllString for the matches that start outside spans.
func replaceOutside(text string, spans [][2]int, re *regexp.Regexp, repl string) string {
	var b strings.Builder
	last := 0
	for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
		if mdtext.Inside(spans, m[0]) {
			continue
		}
		b.WriteString(text[last:m[0]])
		b.Write(re.ExpandString(nil, repl, text, m))
		last = m[1]
	}
	return b.String() + text[last:]
}

var appendixHeadRe = regexp.MustCompile(`(?m)^# .*\{[^}\n]*\.appendix\b[^}\n]*\}[ \t]*$`)

// splitAppendices cuts the body at its first level-1 heading marked
// {.appendix}, outside code: from there on, chapters are appendices.
func splitAppendices(body string) (main, appendices string) {
	spans := mdtext.Protected(body)
	for _, m := range appendixHeadRe.FindAllStringIndex(body, -1) {
		if !mdtext.Inside(spans, m[0]) {
			return body[:m[0]], body[m[0]:]
		}
	}
	return body, ""
}

// thesisArgs are pandoc's settings for a thesis: the book class, chapters
// at the top, sections numbered, and lower-case roman page numbers, which
// babel's Spanish otherwise prints in capitals.
func thesisArgs(lang string) []string {
	args := []string{
		"--top-level-division=chapter", "--number-sections",
		"-V", "documentclass=book", "-V", "classoption=oneside", "-V", "classoption=openany",
		"-V", "fontsize=11pt",
	}
	if strings.HasPrefix(strings.ToLower(lang), "es") {
		args = append(args, "-V", "babeloptions=es-lcroman")
	}
	return args
}

// assembleThesis is the body of a thesis in the template's order, and the
// strings of its three pages; any other style's body comes back as it was.
func (p *prepared) assembleThesis(body string) (*tex.ThesisPages, string) {
	if p.style != tex.Thesis {
		return nil, body
	}
	pages := thesisPages(p.d, p)
	return pages, thesisBody(p.thesis, body, pages.EN, p.d.Meta.CitesFrom())
}

// pdfFrontMatter is the front matter pandoc reads for the PDF.
func (p *prepared) pdfFrontMatter() (string, error) {
	if p.style != tex.Thesis {
		return p.d.FrontMatter, nil
	}
	front, err := thesisFrontMatter(p.d.FrontMatter)
	if err != nil {
		return "", fmt.Errorf("%s: %w", p.o.Input, err)
	}
	return front, nil
}

// thesisFrontMatter is the front matter pandoc reads for a thesis: without
// the contents and lists, which mdbrand writes at the template's place —
// pandoc's own land before the front parts, and -V toc=false is the string
// "false", which is true.
func thesisFrontMatter(front string) (string, error) {
	return doc.StripKeys(front, "toc", "toc-depth", "lof", "lot")
}
