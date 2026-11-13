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
	"github.com/carlosprados/mdbrand/internal/docx"
	"github.com/carlosprados/mdbrand/internal/exit"
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
	if err := settleSchool(t, b); err != nil {
		return err
	}
	if _, abstract, _ := doc.LiftAbstract(d.FrontMatter); abstract != "" {
		return fmt.Errorf("abstract: in the front matter — a thesis's Abstract is a file of its own, read through mdbrand.thesis.abstract, beside its Resumen")
	}
	var missing []string
	for _, f := range []struct{ key, v string }{
		{"title", d.Meta.Title}, {"author", d.Meta.AuthorString()}, {"date (the year on the cover)", thesisYear(d.Meta.Date)},
		{"mdbrand.thesis.programme", t.Programme}, {"mdbrand.thesis.degree", t.Degree},
		{"mdbrand.thesis.abstract", t.Abstract}, {"mdbrand.thesis.resumen", t.Resumen}, {"mdbrand.thesis.acronyms", t.Acronyms},
		{"thesis.university (in the bundle, or mdbrand.thesis.university without one)", b.Thesis.University},
		{"thesis.centre (in the bundle, or mdbrand.thesis.centre without one)", b.Thesis.Centre},
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

// settleSchool puts the university and the centre a thesis names in its
// front matter where the bundle's would be: a thesis built without a bundle
// names them itself. A bundle that names them too must agree, since a
// school's official name has one spelling, and either is held to the
// bundle's rule: written out, never abbreviated.
func settleSchool(t *doc.Thesis, b *brand.Brand) error {
	for _, f := range []struct {
		key, own string
		bundle   *string
	}{{"university", t.University, &b.Thesis.University}, {"centre", t.Centre, &b.Thesis.Centre}} {
		if f.own == "" {
			continue
		}
		if *f.bundle != "" && *f.bundle != f.own {
			return fmt.Errorf("mdbrand.thesis.%s is %q, and brand %q names it %q: the bundle's is the official name. Drop the document's",
				f.key, f.own, b.Name, *f.bundle)
		}
		if abbr := brand.Abbreviations(f.own); len(abbr) > 0 {
			return fmt.Errorf("mdbrand.thesis.%s: %q holds %s — a thesis prints the official name, in Spanish and unabbreviated",
				f.key, f.own, strings.Join(abbr, ", "))
		}
		*f.bundle = f.own
	}
	return nil
}

var yearRe = regexp.MustCompile(`\b(19|20)\d\d\b`)

// thesisYear is the year the cover prints, read from date:.
func thesisYear(date string) string { return yearRe.FindString(date) }

// readThesis reads a thesis's parts, before any figure is rendered: the
// chapters and appendices into the body, so every check after sees one
// document, and the front parts aside. pandoc-crossref is checked only when
// an output takes its numbers from it.
func (p *prepared) readThesis(inputs *[]string, crossref bool) error {
	if p.style != tex.Thesis {
		return nil
	}
	if crossref {
		if err := crossrefReady(); err != nil {
			return err
		}
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

// thesisMarks are what a format writes at the template's three turns: the
// contents and lists, the first chapter, and the first appendix. Lettered
// asks for each appendix's letter as a heading label, for pandoc-crossref to
// number by where LaTeX's \appendix does not reach.
type thesisMarks struct {
	contents, mainmatter, appendix string
	lettered                       bool
}

func rawLaTeX(latex string) string { return "```{=latex}\n" + latex + "\n```\n\n" }

// latexMarks are the PDF's: LaTeX numbers the pages, the chapters and the
// appendices itself.
var latexMarks = thesisMarks{
	contents:   rawLaTeX(`\tableofcontents` + "\n" + `\listoffigures` + "\n" + `\listoftables`),
	mainmatter: rawLaTeX(`\mainmatter`),
	appendix:   rawLaTeX(`\appendix`),
}

// thesisBody assembles the thesis in the order the UPM's template fixes:
// the front parts under the template's headings, the contents and the lists,
// the acronyms, then the chapters with arabic numbers, the references before
// the appendices, and the appendices lettered.
func thesisBody(parts *thesisParts, body string, en, cites bool, marks thesisMarks) string {
	names := map[bool][5]string{
		false: {"Dedicatoria", "Agradecimientos", "Abstract", "Resumen", "Abreviaturas y Acrónimos"},
		true:  {"Dedication", "Acknowledgements", "Abstract", "Resumen", "Abbreviations and Acronyms"},
	}[en]
	var b strings.Builder
	if parts.dedication != "" {
		b.WriteString("# " + names[0] + " {.unnumbered .unlisted}\n\n" + parts.dedication + "\n\n")
	}
	if parts.acknowledgements != "" {
		b.WriteString("# " + names[1] + " {.unnumbered}\n\n" + frontSections(parts.acknowledgements) + "\n\n")
	}
	// The summary in the other language is set in that language's
	// hyphenation. Only that one: a div in the thesis's own language made an
	// older pandoc load the main language a second time, and xelatex stopped
	// on \spanishdate defined twice.
	b.WriteString("# " + names[2] + " {.unnumbered}\n\n" + inLang(!en, "en-GB", frontSections(parts.abstract)))
	b.WriteString("# " + names[3] + " {.unnumbered}\n\n" + inLang(en, "es-ES", frontSections(parts.resumen)))
	b.WriteString(marks.contents)
	b.WriteString("# " + names[4] + " {.unnumbered}\n\n" + parts.acronyms + "\n\n")
	b.WriteString(marks.mainmatter)

	main, appendices := splitAppendices(body)
	b.WriteString(main)
	if cites && !strings.Contains(body, "{#refs}") {
		title := map[bool]string{false: "Referencias", true: "References"}[en]
		b.WriteString("\n\n# " + title + " {.unnumbered}\n\n::: {#refs}\n:::\n\n")
	}
	if appendices != "" {
		if marks.lettered {
			appendices = letterAppendices(appendices)
		}
		b.WriteString("\n\n" + marks.appendix + appendices)
	}
	return b.String()
}

// letterAppendices labels each appendix's chapter heading with its letter,
// A, B, C…, which pandoc-crossref prints and numbers its figures by.
func letterAppendices(text string) string {
	spans := mdtext.Protected(text)
	var b strings.Builder
	last, n := 0, 0
	for _, m := range chapterHeadRe.FindAllStringIndex(text, -1) {
		if mdtext.Inside(spans, m[0]) {
			continue
		}
		label := fmt.Sprintf("label=%c", 'A'+n)
		n++
		h := strings.TrimRight(text[m[0]:m[1]], " \t")
		if strings.HasSuffix(h, "}") && strings.Contains(h, "{") {
			h = h[:len(h)-1] + " " + label + "}"
		} else {
			h += " {" + label + "}"
		}
		b.WriteString(text[last:m[0]] + h)
		last = m[1]
	}
	return b.String() + text[last:]
}

// inLang wraps text in a div of another language when other is set.
func inLang(other bool, lang, text string) string {
	if !other {
		return text + "\n\n"
	}
	return "::: {lang=" + lang + "}\n" + text + "\n:::\n\n"
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
// at the top, sections numbered, lower-case roman page numbers, which
// babel's Spanish otherwise prints in capitals, and pandoc-crossref with the
// thesis's language, before citeproc so [@fig:x] is not read as a citation.
func thesisArgs(work, lang string) ([]string, error) {
	meta := filepath.Join(work, "crossref.yaml")
	if err := os.WriteFile(meta, []byte(crossrefMeta(lang)), 0o644); err != nil {
		return nil, err
	}
	args := []string{
		"--top-level-division=chapter", "--number-sections",
		"-V", "documentclass=book", "-V", "classoption=oneside", "-V", "classoption=openany",
		"-V", "fontsize=11pt",
		"--filter", "pandoc-crossref", "--metadata-file=crossref.yaml",
	}
	// es-lcroman is an option of babel-spanish's spanish.ldf; where Spanish
	// comes only from babel's own ini file there is no such option to pass.
	if strings.HasPrefix(strings.ToLower(lang), "es") && haveSpanishLdf() {
		args = append(args, "-V", "babeloptions=es-lcroman")
	}
	return args, nil
}

// crossrefMeta is pandoc-crossref's settings in the thesis's language:
// figures, tables and equations numbered by chapter, and the words it
// prints before a number. In the PDF LaTeX numbers them; the words are
// crossref's.
func crossrefMeta(lang string) string {
	words := `figureTitle: Figura
tableTitle: Tabla
figPrefix: [figura, figuras]
tblPrefix: [tabla, tablas]
eqnPrefix: [ecuación, ecuaciones]
secPrefix: [sección, secciones]
`
	if strings.HasPrefix(strings.ToLower(lang), "en") {
		words = `figureTitle: Figure
tableTitle: Table
figPrefix: [figure, figures]
tblPrefix: [table, tables]
eqnPrefix: [equation, equations]
secPrefix: [section, sections]
`
	}
	return words + "chapters: true\nlinkReferences: true\n"
}

var (
	crossrefBuiltRe   = regexp.MustCompile(`built with Pandoc v?([0-9][0-9.]*[0-9])`)
	pandocVersionRe   = regexp.MustCompile(`^pandoc(?:\.exe)? ([0-9][0-9.]*[0-9])`)
	crossrefMissingRe = regexp.MustCompile(`Undefined cross-reference: (\S+)`)
)

// crossrefReady stops a thesis build without pandoc-crossref, or with one
// built against another pandoc: a filter speaks pandoc's AST, and a
// mismatched one fails, or mangles the document, in ways its own message
// does not explain.
func crossrefReady() error {
	const releases = "https://github.com/lierdakil/pandoc-crossref/releases"
	if !run.Have("pandoc-crossref") {
		return exit.AsEnvironment(fmt.Errorf(`style thesis numbers figures, tables and equations with pandoc-crossref, which is not installed.
Install the release built for your pandoc (pandoc --version names it):
  %s
  run: mdbrand doctor`, releases))
	}
	xr, err := run.Stdout("", "pandoc-crossref", "--version")
	if err != nil {
		return err
	}
	pd, err := run.Stdout("", "pandoc", "--version")
	if err != nil {
		return err
	}
	built, running := crossrefBuiltRe.FindStringSubmatch(string(xr)), pandocVersionRe.FindStringSubmatch(string(pd))
	if built == nil || running == nil {
		return nil // an unreadable version is not a reason to refuse
	}
	if built[1] != running[1] {
		return exit.AsEnvironment(fmt.Errorf(`pandoc-crossref was built for pandoc %s and this is pandoc %s: a filter reads
pandoc's own data, and a mismatched one fails or garbles the document. Install
the pandoc-crossref release built for pandoc %s:
  %s`, built[1], running[1], running[1], releases))
	}
	return nil
}

// crossrefMisses stops a build that refers to a figure, table, equation or
// section nothing labels. pandoc-crossref prints "¿fig:x?" in the text and
// exits 0; only its warning says so.
func crossrefMisses(pandocOut string) error {
	if refs := uniqueMatches(crossrefMissingRe, pandocOut); len(refs) > 0 {
		return crossrefMissError(refs)
	}
	return nil
}

var crossrefRefRe = regexp.MustCompile(`^\\(?:c|C)?ref\{((?:fig|tbl|eq|sec|lst):[^}]+)\}$`)

// crossrefRefs are the labels among a log's undefined references that are
// pandoc-crossref's.
func crossrefRefs(undefined []string) []string {
	var out []string
	for _, u := range undefined {
		if m := crossrefRefRe.FindStringSubmatch(u); m != nil {
			out = append(out, m[1])
		}
	}
	return out
}

func crossrefMissError(refs []string) error {
	return fmt.Errorf(`%d cross-reference(s) to nothing: @%s
Nothing carries the label: give the figure, table, equation or heading its
{#fig:…}, {#tbl:…}, {#eq:…} or {#sec:…}, or fix the reference`, len(refs), strings.Join(refs, ", @"))
}

// haveSpanishLdf reports whether babel-spanish is installed.
func haveSpanishLdf() bool {
	out, err := run.Stdout("", "kpsewhich", "spanish.ldf")
	return err == nil && strings.TrimSpace(string(out)) != ""
}

// assembleThesis is the body of a thesis in the template's order, and the
// strings of its three pages; any other style's body comes back as it was.
func (p *prepared) assembleThesis(body string) (*tex.ThesisPages, string) {
	if p.style != tex.Thesis {
		return nil, body
	}
	pages := thesisPages(p.d, p)
	return pages, thesisBody(p.thesis, body, pages.EN, p.d.Meta.CitesFrom(), latexMarks)
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

// autoLabels gives every captioned figure and table without an identifier
// one, before pandoc-crossref runs: crossref numbers only what is labelled,
// and LaTeX numbers every captioned float, so a .docx of the same thesis
// printed "Figura 2.1" in the PDF as a caption with no number at all.
const autoLabels = `-- Written by mdbrand for a thesis's .docx and .epub.
local n = 0
local function label(el, prefix)
  if el.identifier == "" and #el.caption.long > 0 then
    n = n + 1
    el.identifier = prefix .. "mdbrand-" .. n
    return el
  end
end
function Figure(el) return label(el, "fig:") end
function Table(el) return label(el, "tbl:") end
`

// dropLabels takes the appendix letters off the headings once
// pandoc-crossref has read them: pandoc 3.1 wrote label="A" into the
// .epub's HTML as it was, and epubcheck refused the package (RSC-005).
const dropLabels = `-- Written by mdbrand: the letters are pandoc-crossref's to read, not HTML.
function Header(h)
  h.attributes.label = nil
  return h
end
`

// reflowArgs are pandoc's settings for a thesis's .docx and .epub, where no
// LaTeX numbers anything: pandoc-crossref numbers the chapters, sections to
// the PDF's depth, figures, tables and equations, and the appendices by the
// letters thesisBody labels them with. In an .epub the numbers are not
// links: pandoc's epub writer leaves a link to a figure in another chapter
// pointing into the chapter it is in, and epubcheck refuses it (RSC-012).
func reflowArgs(work, lang string, epub bool) ([]string, error) {
	if err := os.WriteFile(filepath.Join(work, "crossref.yaml"), []byte(crossrefMeta(lang)), 0o644); err != nil {
		return nil, err
	}
	for name, src := range map[string]string{"autolabels.lua": autoLabels, "droplabels.lua": dropLabels} {
		if err := os.WriteFile(filepath.Join(work, name), []byte(src), 0o644); err != nil {
			return nil, err
		}
	}
	args := []string{
		"--lua-filter", "autolabels.lua",
		"--filter", "pandoc-crossref", "--metadata-file=crossref.yaml",
		"--lua-filter", "droplabels.lua",
		// The book class numbers chapters, sections and subsections.
		"-M", "numberSections=true", "-M", "sectionsDepth=3",
	}
	if epub {
		args = append(args, "-M", "linkReferences=false")
	}
	return args, nil
}

// reflowThesis is a thesis's body for a .docx or an .epub, contents at the
// template's place: Word's own field in a .docx, the reader's menu in an
// .epub (contents "").
func (p *prepared) reflowThesis(body, contents string) string {
	en := strings.HasPrefix(strings.ToLower(p.d.Meta.Lang), "en")
	return thesisBody(p.thesis, body, en, p.d.Meta.CitesFrom(), thesisMarks{contents: contents, lettered: true})
}

// thesisTitlePage is the portada's data for a .docx's title block, in the
// thesis's language and the PDF's words.
func thesisTitlePage(d *doc.File, b *brand.Brand) *docx.ThesisPage {
	t := d.Meta.Options.Thesis
	en := strings.HasPrefix(strings.ToLower(d.Meta.Lang), "en")
	pick := func(es, eng string) string {
		if en {
			return eng
		}
		return es
	}
	page := &docx.ThesisPage{
		University: b.Thesis.University, Centre: b.Thesis.Centre,
		Programme:    pick("Doctorado en ", "Doctoral Programme in ") + t.Programme,
		Kind:         pick("TESIS DOCTORAL", "DOCTORAL THESIS"),
		SubmittedBy:  pick("Presentada para optar al título de Doctor por:", "Submitted for the degree of Doctor by:"),
		Degree:       t.Degree,
		SupervisedBy: pick("Bajo la dirección de:", "Supervised by:"),
		PlaceYear:    "Madrid, " + thesisYear(d.Meta.Date),
	}
	for _, dir := range t.Directors {
		page.Directors = append(page.Directors, dir.Name)
	}
	return page
}

// numberingArgs adds how a .docx or an .epub numbers its headings: pandoc's
// --number-sections when the document asks, pandoc-crossref's for a thesis.
func numberingArgs(p *prepared, args []string, epub bool) ([]string, error) {
	if p.style == tex.Thesis {
		more, err := reflowArgs(p.work, p.d.Meta.Lang, epub)
		return append(args, more...), err
	}
	if p.d.Meta.NumberSections {
		args = append(args, "--number-sections")
	}
	return args, nil
}

// reflowProblems stops a .docx or an .epub on a citation or, in a thesis, a
// cross-reference that leads nowhere.
func reflowProblems(m doc.Meta, pandocOut string) error {
	if err := citationProblems(m, pandocOut, "document"); err != nil {
		return err
	}
	if m.Options.Thesis != nil {
		return crossrefMisses(pandocOut)
	}
	return nil
}
