package docx

import (
	"fmt"
	"strings"
)

// Meta is what the cover, the letterhead and the signature print.
type Meta struct {
	Title, Subtitle, Author, Date string
	Reference, Confidential       string
	To                            []string
	Place, Greeting, Signature    string
	TOC                           bool
	TOCDepth                      int
	Lang                          string

	// Logo files as pandoc will find them, with their placed widths.
	Logo, LogoSecondary       string
	LogoWMM, LogoSecondaryWMM float64
}

// Labels are the words a document prints in its own language.
type Labels struct{ Figure, Table, Contents string }

// LabelsFor follows babel's names, except that Spanish tables are Tabla: the
// PDF overrides babel's Cuadro, and the two outputs must agree.
func LabelsFor(lang string) Labels {
	switch strings.ToLower(strings.SplitN(strings.SplitN(lang, "-", 2)[0], "_", 2)[0]) {
	case "es":
		return Labels{"Figura", "Tabla", "Índice"}
	case "ca":
		return Labels{"Figura", "Taula", "Índex"}
	case "pt":
		return Labels{"Figura", "Tabela", "Sumário"}
	case "fr":
		return Labels{"Figure", "Table", "Table des matières"}
	case "it":
		return Labels{"Figura", "Tabella", "Indice"}
	case "de":
		return Labels{"Abbildung", "Tabelle", "Inhaltsverzeichnis"}
	default:
		return Labels{"Figure", "Table", "Contents"}
	}
}

// Front is the Markdown that goes before and after the body: raw OOXML for the
// text, pandoc images for the logos so pandoc owns their relationships.
func Front(layout string, m Meta, l Look) (before, after string) {
	switch layout {
	case "report":
		return cover(m, l), ""
	case "letter":
		return letterhead(m, l), signature(m, l)
	default:
		return noteTitle(m, l), ""
	}
}

func cover(m Meta, l Look) string {
	var b strings.Builder
	b.WriteString(logos(m))
	var x strings.Builder
	// The title block sits a third of the way down under a hairline, as the
	// PDF's does. Spacing before a paragraph lies outside its border, so the
	// rule travels with the title: a spacing value, not an anchored frame,
	// which is what every importer keeps.
	x.WriteString(para(pPr(twips(55), 120, 0, "", l.Rule, "top", false), run(l.Display, l.Text, 26, true, m.Title)))
	if m.Subtitle != "" {
		x.WriteString(para(pPr(0, 120, 0, "", "", "", false), run(l.Display, l.Text, 13, false, m.Subtitle)))
	}
	for _, s := range []string{m.Reference, m.Confidential} {
		if s != "" {
			x.WriteString(para(pPr(120, 0, 0, "", "", "", false), run(l.Display, l.Primary, SmallPt, true, s)))
		}
	}
	// Author and date at the foot, the first of them under a hairline.
	var foot []string
	for _, s := range []string{m.Author, m.Date} {
		if s != "" {
			foot = append(foot, s)
		}
	}
	if len(foot) == 0 {
		foot = []string{""}
	}
	for i, s := range foot {
		ppr := pPr(0, 0, 0, "", "", "", false)
		if i == 0 {
			ppr = pPr(twips(80), 0, 0, "", l.Rule, "top", false)
		}
		r := run(l.Display, l.Text, SmallPt, false, s)
		if i == len(foot)-1 {
			r += pageBreak
		}
		x.WriteString(para(ppr, r))
	}
	if m.TOC {
		x.WriteString(tocField(m, l))
	}
	b.WriteString(raw(x.String()))
	return b.String()
}

func noteTitle(m Meta, l Look) string {
	if m.Title == "" && m.Subtitle == "" {
		return ""
	}
	var x strings.Builder
	if m.Title != "" {
		x.WriteString(para(pPr(0, 60, 0, "", "", "", false), run(l.Display, l.Text, 16, true, m.Title)))
	}
	if m.Subtitle != "" {
		x.WriteString(para(pPr(0, 60, 0, "", "", "", false), run(l.Display, l.Text, 11, false, m.Subtitle)))
	}
	x.WriteString(para(pPr(0, 240, 0, "", l.Rule, "bottom", false), ""))
	if r := noteMeta(m, l); r != "" {
		x.WriteString(para(pPr(0, 240, 0, "", "", "", false), r))
	}
	if m.TOC {
		x.WriteString(tocField(m, l))
	}
	return raw(x.String())
}

// noteMeta is the line under a note's rule, as the PDF sets it: author and
// date in the text colour, the confidentiality label in the primary, joined
// by middle dots that only ever stand between two of them.
func noteMeta(m Meta, l Look) string {
	var b strings.Builder
	sep := func() {
		if b.Len() > 0 {
			b.WriteString(run(l.Display, l.Text, SmallPt, false, " · "))
		}
	}
	for _, s := range []string{m.Author, m.Date} {
		if s != "" {
			sep()
			b.WriteString(run(l.Display, l.Text, SmallPt, false, s))
		}
	}
	if m.Confidential != "" {
		sep()
		b.WriteString(run(l.Display, l.Primary, SmallPt, false, m.Confidential))
	}
	return b.String()
}

func letterhead(m Meta, l Look) string {
	var b strings.Builder
	b.WriteString(logos(m))
	var x strings.Builder
	for i, s := range m.To {
		before := 0
		if i == 0 {
			before = 480
		}
		x.WriteString(para(pPr(before, 0, 0, "", "", "", false), run(l.Display, l.Text, SmallPt, false, s)))
	}
	dateLine := m.Date
	if m.Place != "" && m.Date != "" {
		dateLine = m.Place + ", " + m.Date
	} else if m.Place != "" {
		dateLine = m.Place
	}
	if dateLine != "" {
		x.WriteString(para(pPr(480, 0, 0, "", "", "", false), run(l.Display, l.Text, SmallPt, false, dateLine)))
	}
	if m.Title != "" {
		x.WriteString(para(pPr(480, 0, 0, "", "", "", false), run(l.Display, l.Text, 11, true, m.Title)))
	}
	if m.Greeting != "" {
		x.WriteString(para(pPr(360, 120, 0, "", "", "", false), run(l.Body, "", BodyPt, false, m.Greeting)))
	}
	b.WriteString(raw(x.String()))
	return b.String()
}

func signature(m Meta, l Look) string {
	if m.Signature == "" {
		return ""
	}
	var x strings.Builder
	for i, s := range strings.Split(strings.TrimRight(m.Signature, "\n"), "\n") {
		before := 0
		if i == 0 {
			before = 480
		}
		x.WriteString(para(pPr(before, 0, 0, "", "", "", true), run(l.Display, l.Text, SmallPt, false, s)))
	}
	return raw(x.String())
}

func logos(m Meta) string {
	var imgs []string
	if m.Logo != "" {
		imgs = append(imgs, fmt.Sprintf("![](%s){width=%.1fmm}", m.Logo, m.LogoWMM))
	}
	if m.LogoSecondary != "" {
		imgs = append(imgs, fmt.Sprintf("![](%s){width=%.1fmm}", m.LogoSecondary, m.LogoSecondaryWMM))
	}
	if len(imgs) == 0 {
		return ""
	}
	// Two images on one line with a space between them: one paragraph, so
	// pandoc does not turn a lone picture into a captioned figure.
	return strings.Join(imgs, "   ") + "\\\n\n"
}

// tocField is a table of contents Word fills when it opens the file; the
// setting that makes it ask is written by Repair.
func tocField(m Meta, l Look) string {
	depth := m.TOCDepth
	if depth <= 0 {
		depth = 3
	}
	lab := LabelsFor(m.Lang)
	return para(`<w:pPr><w:pStyle w:val="TOCHeading"/></w:pPr>`, run(l.Display, l.Text, 14, true, lab.Contents)) +
		`<w:p><w:r><w:fldChar w:fldCharType="begin" w:dirty="true"/></w:r>` +
		fmt.Sprintf(`<w:r><w:instrText xml:space="preserve"> TOC \o "1-%d" \h \z \u </w:instrText></w:r>`, depth) +
		`<w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t></w:t></w:r>` +
		`<w:r><w:fldChar w:fldCharType="end"/></w:r>` + pageBreak + `</w:p>`
}

const pageBreak = `<w:r><w:br w:type="page"/></w:r>`

func raw(x string) string { return "```{=openxml}\n" + x + "\n```\n\n" }

func para(ppr, runs string) string { return "<w:p>" + ppr + runs + "</w:p>" }

// pPr is a paragraph's spacing, alignment and an optional rule above or below.
func pPr(before, after, line int, jc, ruleColor, ruleSide string, keepTogether bool) string {
	var b strings.Builder
	b.WriteString("<w:pPr>")
	if keepTogether {
		b.WriteString("<w:keepNext/><w:keepLines/>")
	}
	if ruleColor != "" {
		fmt.Fprintf(&b, `<w:pBdr><w:%s w:val="single" w:sz="4" w:space="1" w:color="%s"/></w:pBdr>`, ruleSide, ruleColor)
	}
	fmt.Fprintf(&b, `<w:spacing w:before="%d" w:after="%d"`, before, after)
	if line > 0 {
		fmt.Fprintf(&b, ` w:line="%d" w:lineRule="auto"`, line)
	}
	b.WriteString("/>")
	if jc != "" {
		fmt.Fprintf(&b, `<w:jc w:val="%s"/>`, jc)
	}
	b.WriteString("</w:pPr>")
	return b.String()
}

func run(font, color string, pt float64, bold bool, text string) string {
	var b strings.Builder
	b.WriteString("<w:r><w:rPr>")
	if font != "" {
		b.WriteString(fontsXML(font))
	}
	if bold {
		b.WriteString("<w:b/><w:bCs/>")
	}
	if color != "" {
		fmt.Fprintf(&b, `<w:color w:val="%s"/>`, color)
	}
	fmt.Fprintf(&b, `<w:sz w:val="%d"/><w:szCs w:val="%d"/></w:rPr>`, halfPt(pt), halfPt(pt))
	fmt.Fprintf(&b, `<w:t xml:space="preserve">%s</w:t></w:r>`, esc(text))
	return b.String()
}
