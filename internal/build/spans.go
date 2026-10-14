package build

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/suggest"
)

// spanFilter colours [text]{.accent} in both outputs and reports any other
// colour asked of a span or div, which pandoc would drop in silence.
//
//go:embed spans.lua
var spanFilter []byte

const spanFilterName = "spans.lua"

// writeSpanFilter puts the filter in the work directory and returns the pandoc
// flag that runs it. The filter names brandPrimary; a bundle whose accent is
// a colour of its own gets brandAccent instead, and one without keeps the
// filter, and so the .tex, exactly as it was.
func writeSpanFilter(work string, b *brand.Brand) (string, error) {
	filter := spanFilter
	if b.Colors.Accent != b.Colors.Primary {
		filter = bytes.ReplaceAll(filter, []byte(`\\textcolor{brandPrimary}`), []byte(`\\textcolor{brandAccent}`))
	}
	if err := os.WriteFile(filepath.Join(work, spanFilterName), filter, 0o644); err != nil {
		return "", err
	}
	return "--lua-filter=" + spanFilterName, nil
}

var spanRe = regexp.MustCompile(`MDBRAND-SPAN ([^\t\n]+)\t([^\n]*)`)

// filterProblems is everything spans.lua reports that stops a build.
func filterProblems(pandocOut string) error {
	if err := spanProblems(pandocOut); err != nil {
		return err
	}
	if err := htmlProblems(pandocOut); err != nil {
		return err
	}
	if err := keepProblems(pandocOut); err != nil {
		return err
	}
	return anchorProblems(pandocOut)
}

var (
	keepRe       = regexp.MustCompile(`MDBRAND-KEEP (\d+)\t([^\n]*)`)
	keepDeckRe   = regexp.MustCompile(`MDBRAND-KEEP-DECK\t([^\n]*)`)
	keepNestedRe = regexp.MustCompile(`MDBRAND-KEEP-NESTED\t([^\n]*)`)
)

// keepProblems refuses a ::: {.keep} where it cannot mean anything: on a
// slide, which never breaks, and inside another, which already holds it.
func keepProblems(pandocOut string) error {
	if m := keepDeckRe.FindStringSubmatch(pandocOut); m != nil {
		return fmt.Errorf(`::: {.keep} on a slide, which never breaks across pages:
  %q
There is nothing to keep together. Drop the div; a slide that does not fit
stops the build by its title`, ellipsize(m[1], 50))
	}
	if m := keepNestedRe.FindStringSubmatch(pandocOut); m != nil {
		return fmt.Errorf(`::: {.keep} inside another, which already keeps everything in it
together:
  %q
Drop the inner one`, ellipsize(m[1], 50))
	}
	return nil
}

// keepNames is each kept block's name by its number: the log reports a block
// taller than a page by number alone, since its heading is not TeX's to print.
func keepNames(pandocOut string) map[string]string {
	names := map[string]string{}
	for _, m := range keepRe.FindAllStringSubmatch(pandocOut, -1) {
		names[m[1]] = m[2]
	}
	return names
}

var htmlRe = regexp.MustCompile(`MDBRAND-HTML (\w+)\t([^\n]*)`)

// htmlForms is the Markdown that prints what each HTML tag asked for.
var htmlForms = map[string]string{
	"br":     `a backslash at the end of the line`,
	"b":      `**bold**`,
	"strong": `**bold**`,
	"i":      `*italic*`,
	"em":     `*italic*`,
	"u":      `[words]{.underline}`,
	"s":      `~~struck~~`,
	"del":    `~~struck~~`,
	"sup":    `^superscript^`,
	"sub":    `~subscript~`,
	"code":   "`code`",
	"kbd":    "`Ctrl-C`",
	"img":    `![caption](picture.png)`,
	"a":      `[words](https://…), or []{#anchor} to be linked to`,
	"table":  "a pipe table, or a ```table block over a data file",
	"hr":     `a line of three dashes, ---`,
	"p":      `a blank line`,
}

var tableParts = map[string]bool{"thead": true, "tbody": true, "tfoot": true, "tr": true, "th": true, "td": true, "caption": true, "colgroup": true, "col": true}

// htmlProblems stops raw HTML, which neither writer prints: an <img> or a
// <table> vanishes, a <br> joins its lines, a <b> prints plain.
func htmlProblems(pandocOut string) error {
	ms := htmlRe.FindAllStringSubmatch(pandocOut, -1)
	if len(ms) == 0 {
		return nil
	}
	first := map[string]string{}
	var tags []string
	for _, m := range ms {
		if tableParts[m[1]] && strings.Contains(pandocOut, "MDBRAND-HTML table\t") {
			continue // the table's own line covers its rows and cells
		}
		if _, ok := first[m[1]]; !ok {
			first[m[1]] = m[2]
			tags = append(tags, m[1])
		}
	}
	var lines []string
	for _, t := range tags {
		line := fmt.Sprintf("%s — write %s", ellipsize(first[t], 40), htmlForms[t])
		if htmlForms[t] == "" {
			line = fmt.Sprintf("%s — write it in Markdown", ellipsize(first[t], 40))
		}
		lines = append(lines, line)
	}
	return fmt.Errorf(`raw HTML that neither the PDF nor the .docx prints:
  %s
pandoc drops HTML when it writes LaTeX or Word, without a word: a picture or
a table is gone, a line break joins its lines, bold prints plain. An HTML
comment is fine`, strings.Join(lines, "\n  "))
}

var (
	anchorRe = regexp.MustCompile(`MDBRAND-ANCHOR (\S+)`)
	idsRe    = regexp.MustCompile(`MDBRAND-IDS ([^\n]*)`)
)

// anchorProblems stops a link to an anchor nothing carries: it prints as a
// link, and leads nowhere in the PDF and the .docx alike.
func anchorProblems(pandocOut string) error {
	missing := uniqueMatches(anchorRe, pandocOut)
	if len(missing) == 0 {
		return nil
	}
	var known []string
	if m := idsRe.FindStringSubmatch(pandocOut); m != nil {
		known = strings.Fields(m[1])
	}
	var lines []string
	for _, a := range missing {
		line := "#" + a
		if near := suggest.Closest(a, known); near != "" {
			line += " — did you mean #" + near + "?"
		}
		lines = append(lines, line)
	}
	return fmt.Errorf(`%d link(s) to an anchor nothing in the document carries:
  %s
The words print as a link that leads nowhere. A heading's anchor is its text
in lower case with spaces as hyphens and punctuation dropped ("Set-up" is
#set-up); or give the target one of its own: # Set-up {#setup}`,
		len(missing), strings.Join(lines, "\n  "))
}

// spanProblems turns the filter's report into the build's refusal.
func spanProblems(pandocOut string) error {
	ms := spanRe.FindAllStringSubmatch(pandocOut, -1)
	if len(ms) == 0 {
		return nil
	}
	var lines []string
	for _, m := range ms {
		lines = append(lines, fmt.Sprintf("%s on %q", m[1], ellipsize(strings.TrimSpace(m[2]), 50)))
	}
	return fmt.Errorf(`%d colour(s) asked for in a way that neither the PDF nor the .docx can print:
  %s
pandoc drops colour attributes without a word, so the text would come out
black, and the accent colours words, not blocks. Colour words with the brand's
accent, which both outputs print:
  [words]{.accent}`, len(ms), strings.Join(lines, "\n  "))
}

// minAccentContrast is WCAG's AA ratio for text, which accented words are.
const minAccentContrast = 4.5

// accentContrast measures the accent against the white page when the
// document prints words in it. A deck stops, since a projector washes a pale
// colour out to nothing; a page warns, because a pale accent has always
// printed and a document that built yesterday must still build.
func accentContrast(b *brand.Brand, pandocOut string, deck bool, rep *Report) error {
	if !strings.Contains(pandocOut, "MDBRAND-ACCENT") {
		return nil
	}
	c := brand.Contrast(b.Colors.Accent, "FFFFFF")
	if c >= minAccentContrast {
		return nil
	}
	msg := fmt.Sprintf(`[words]{.accent} print in %s, %.1f:1 against the white page, under the
%.1f:1 that text needs to be read. Set colors.accent in the bundle to a darker
shade for words, and keep the primary for rules: %s, the same hue as dark as
it needs to be, reaches %.1f:1`,
		b.Colors.Accent, c, minAccentContrast,
		brand.Darken(b.Colors.Accent, minAccentContrast),
		brand.Contrast(brand.Darken(b.Colors.Accent, minAccentContrast), "FFFFFF"))
	if deck {
		return fmt.Errorf("%s", msg)
	}
	rep.Warnings = append(rep.Warnings, msg)
	return nil
}

var (
	texMathRe = regexp.MustCompile(`\[WARNING\] Could not convert TeX math (.*?), rendering as TeX`)
	rawTeXRe  = regexp.MustCompile(`MDBRAND-RAWTEX (\S+)`)
	warningRe = regexp.MustCompile(`(?m)^\[WARNING\] (.+)$`)
)

// pandocWarnings reads what pandoc said on its way. A formula the .docx's
// equation writer cannot read was printed as its TeX source, so the reader
// saw \mathbb{R} \foo{x}: that stops the build. Raw LaTeX a .docx cannot
// hold is dropped, which a \newpage meant for the PDF only does on purpose:
// that warns, by command. Every other warning was swallowed, on both writers,
// and now reaches the reader; citations have their own check.
func pandocWarnings(pandocOut, format string, rep *Report) error {
	if ms := uniqueMatches(texMathRe, pandocOut); len(ms) > 0 {
		for i, m := range ms {
			ms[i] = "$" + ellipsize(m, 60) + "$"
		}
		return fmt.Errorf(`%d formula(s) the %s cannot write as an equation, so it would print their
TeX source as text:
  %s
Its equations take the LaTeX that pandoc reads: a macro of the document's
own, or an environment beyond amsmath's, is not in it. Write the formula with
standard commands`, len(ms), format, strings.Join(ms, "\n  "))
	}
	cmds := uniqueMatches(rawTeXRe, pandocOut)
	if format == ".epub" {
		// A reflowed page has no breaks to force: these lose nothing there.
		cmds = slices.DeleteFunc(cmds, func(c string) bool {
			return c == `\newpage` || c == `\clearpage` || c == `\pagebreak` || c == `\cleardoublepage`
		})
	}
	if len(cmds) > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf(
			"raw LaTeX the %s cannot hold was left out of it: %s. Fine for a \\newpage meant for the PDF; anything that carries text or colour is missing from the %s",
			format, strings.Join(cmds, ", "), format))
	}
	for _, m := range uniqueMatches(warningRe, pandocOut) {
		if strings.HasPrefix(m, "Citeproc:") || strings.HasPrefix(m, "Could not convert TeX math") {
			continue
		}
		rep.Warnings = append(rep.Warnings, "pandoc: "+m)
	}
	return nil
}
