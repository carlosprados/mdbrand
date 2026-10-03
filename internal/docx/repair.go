package docx

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
)

// Fix is what Repair needs to know about the document.
type Fix struct {
	Look   Look
	Labels Labels
	TOC    bool
	// KeepRows is the fewest body rows a page break may leave on either side
	// of one, the rule the PDF follows.
	KeepRows int
}

// Repair puts right what pandoc's .docx gets wrong for these documents, and
// refuses one that would ask the reader's machine for a face nobody chose.
// The warnings are the .docx's own versions of the PDF's: a code block or a
// table that does not fit the measure.
func Repair(b []byte, f Fix) ([]byte, []string, error) {
	p, err := readPkg(b)
	if err != nil {
		return nil, nil, err
	}
	doc := p.get("word/document.xml")
	var warn []string

	doc = dropTitleBlock(doc)
	doc = numberCaptions(doc, f.Labels)
	doc, w := tables(doc, f)
	warn = append(warn, w...)
	doc, w = codeBlocks(doc, f.Look)
	warn = append(warn, w...)
	if f.TOC {
		doc = fillTOC(doc)
	}
	p.set("word/document.xml", doc)

	if f.TOC {
		p.set("word/settings.xml", updateFields(p.get("word/settings.xml")))
	}
	if stray := strayFonts(p, f.Look); len(stray) > 0 {
		return nil, nil, fmt.Errorf(`the .docx asks for %s, which the bundle's fonts.office does not name.
Word and Google Docs would substitute it without a word. pandoc wrote it, and
mdbrand did not expect it: please report this, with the document, at
  https://github.com/carlosprados/mdbrand/issues`, strings.Join(stray, ", "))
	}
	for _, name := range p.names {
		if strings.HasPrefix(name, "word/") && strings.HasSuffix(name, ".xml") {
			p.set(name, normalize(p.get(name)))
			if bad := misordered(p.get(name)); bad != "" {
				return nil, nil, fmt.Errorf(`%s holds properties out of the order Word requires:
  %s
LibreOffice and Google Docs would open it; Word would call it unreadable. This
is a bug in mdbrand: please report it at
  https://github.com/carlosprados/mdbrand/issues`, name, ellipsis(bad, 300))
			}
		}
	}
	out, err := p.bytes()
	return out, warn, err
}

func ellipsis(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

var (
	paraRe   = regexp.MustCompile(`(?s)<w:p>.*?</w:p>|<w:p\b[^>]*/>`)
	pStyleRe = regexp.MustCompile(`<w:pStyle w:val="([^"]+)"\s*/>`)
)

// dropTitleBlock removes pandoc's title, subtitle, author and date. The cover
// or letterhead prints them in the brand's layout; pandoc's version would
// stand on top of it. They stay in the file's properties, where Word and Drive
// show them.
func dropTitleBlock(doc string) string {
	i := strings.Index(doc, "<w:body>")
	if i < 0 {
		return doc
	}
	i += len("<w:body>")
	titleStyles := map[string]bool{"Title": true, "Subtitle": true, "Author": true, "Date": true, "Abstract": true, "AbstractTitle": true}
	rest := doc[i:]
	for {
		trimmed := strings.TrimLeft(rest, " \n\t")
		loc := paraRe.FindStringIndex(trimmed)
		if loc == nil || loc[0] != 0 {
			break
		}
		m := pStyleRe.FindStringSubmatch(trimmed[:loc[1]])
		if m == nil || !titleStyles[m[1]] {
			break
		}
		rest = trimmed[loc[1]:]
	}
	return doc[:i] + rest
}

// numberCaptions prefixes each caption with its label and number, as the PDF
// prints them. The number is a SEQ field with its value already in place, so
// it reads right before anyone updates a field and survives Google's import
// as plain text.
func numberCaptions(doc string, lab Labels) string {
	n := map[string]int{}
	return paraRe.ReplaceAllStringFunc(doc, func(par string) string {
		m := pStyleRe.FindStringSubmatch(par)
		if m == nil {
			return par
		}
		var label, seq string
		switch m[1] {
		case "ImageCaption":
			label, seq = lab.Figure, "Figure"
		case "TableCaption":
			label, seq = lab.Table, "Table"
		default:
			return par
		}
		n[seq]++
		prefix := fmt.Sprintf(`<w:r><w:rPr><w:b/><w:bCs/></w:rPr><w:t xml:space="preserve">%s </w:t></w:r>`+
			`<w:fldSimple w:instr=" SEQ %s \* ARABIC "><w:r><w:rPr><w:b/><w:bCs/></w:rPr><w:t>%d</w:t></w:r></w:fldSimple>`+
			`<w:r><w:rPr><w:b/><w:bCs/></w:rPr><w:t xml:space="preserve">: </w:t></w:r>`, esc(label), seq, n[seq])
		end := strings.Index(par, "</w:pPr>")
		if end < 0 {
			return par
		}
		end += len("</w:pPr>")
		return par[:end] + prefix + par[end:]
	})
}

var (
	tableRe = regexp.MustCompile(`(?s)<w:tbl>.*?</w:tbl>`)
	rowRe   = regexp.MustCompile(`(?s)<w:tr>.*?</w:tr>`)
	cellRe  = regexp.MustCompile(`(?s)<w:tc>.*?</w:tc>`)
	textRe  = regexp.MustCompile(`(?s)<w:t(?:\s[^>]*)?>(.*?)</w:t>`)
	tcPrRe  = regexp.MustCompile(`(?s)<w:tcPr\s*/>|<w:tcPr>.*?</w:tcPr>`)
	tblWRe  = regexp.MustCompile(`<w:tblW\b[^>]*/>`)
	gridRe  = regexp.MustCompile(`(?s)<w:tblGrid>.*?</w:tblGrid>`)
	trPrRe  = regexp.MustCompile(`(?s)<w:trPr\s*/>|<w:trPr>.*?</w:trPr>`)
	tcWRe   = regexp.MustCompile(`<w:tcW\b[^>]*/>`)
	tcBdrRe = regexp.MustCompile(`(?s)<w:tcBorders>.*?</w:tcBorders>`)
)

// tables sizes, rules and keeps every table.
func tables(doc string, f Fix) (string, []string) {
	var warn []string
	n := 0
	measure := twips(f.Look.TextWidthMM())
	out := tableRe.ReplaceAllStringFunc(doc, func(tbl string) string {
		n++
		if strings.Count(tbl, "<w:tbl>") > 1 || strings.Contains(tbl, "<w:gridSpan") {
			// A nested table or a spanning cell: the widths stay pandoc's. The
			// rules and keeps still apply.
			return ruleAndKeep(tbl, f.KeepRows)
		}
		rows := rowRe.FindAllString(tbl, -1)
		widths, need := columnWidths(rows, measure, MonoAdvance(f.Look.Mono))
		if need > measure {
			warn = append(warn, fmt.Sprintf(
				"table %d: its longest words need %.0f mm side by side and the measure is %.0f mm; "+
					"shorten them or drop a column, or Word will run the table past the margin",
				n, float64(need)/1440*25.4, f.Look.TextWidthMM()))
		}
		if widths != nil {
			tbl = setWidths(tbl, widths)
		}
		return ruleAndKeep(tbl, f.KeepRows)
	})
	// Air under each table, as the PDF leaves. A paragraph cannot say "space
	// before me if a table precedes", so a short empty one stands in.
	out = strings.ReplaceAll(out, "</w:tbl>",
		`</w:tbl><w:p><w:pPr><w:spacing w:before="0" w:after="0" w:line="160" w:lineRule="exact"/></w:pPr></w:p>`)
	return out, warn
}

var (
	tocFieldRe = regexp.MustCompile(`(?s)<w:p><w:r><w:fldChar w:fldCharType="begin" w:dirty="true"/></w:r><w:r><w:instrText xml:space="preserve"> TOC \\o "1-(\d)".*?</w:p>`)
	headingRe  = regexp.MustCompile(`<w:pStyle w:val="Heading(\d)"\s*/>`)
	bookmarkRe = regexp.MustCompile(`<w:bookmarkStart\b[^>]*w:name="([^"]+)"`)
)

// fillTOC writes the table of contents' entries into its field, each linked
// to its heading. Word replaces them, page numbers and all, when it updates
// the field on opening; LibreOffice and Google Docs show them as they are, so
// the contents page is never the empty one an unfilled field leaves.
func fillTOC(doc string) string {
	loc := tocFieldRe.FindStringSubmatchIndex(doc)
	if loc == nil {
		return doc
	}
	field := doc[loc[0]:loc[1]]
	depth := int(doc[loc[2]] - '0')
	instr := field[:strings.Index(field, `<w:r><w:fldChar w:fldCharType="separate"/>`)]
	instr = strings.TrimPrefix(instr, "<w:p>")
	tail := field[strings.Index(field, `<w:r><w:fldChar w:fldCharType="end"/></w:r>`):]

	type entry struct{ ppr, link string }
	var entries []entry
	for _, par := range paraRe.FindAllString(doc[loc[1]:], -1) {
		h := headingRe.FindStringSubmatch(par)
		if h == nil || int(h[1][0]-'0') > depth {
			continue
		}
		bm := bookmarkRe.FindStringSubmatch(par)
		var text strings.Builder
		for _, m := range textRe.FindAllStringSubmatch(par, -1) {
			text.WriteString(m[1]) // already escaped
		}
		link := text.String()
		if bm != nil {
			link = fmt.Sprintf(`<w:hyperlink w:anchor="%s" w:history="1"><w:r><w:t xml:space="preserve">%s</w:t></w:r></w:hyperlink>`, bm[1], link)
		} else {
			link = `<w:r><w:t xml:space="preserve">` + link + `</w:t></w:r>`
		}
		entries = append(entries, entry{fmt.Sprintf(`<w:pPr><w:pStyle w:val="TOC%s"/></w:pPr>`, h[1]), link})
	}
	if len(entries) == 0 {
		return doc
	}
	var b strings.Builder
	for i, e := range entries {
		lead := ""
		if i == 0 {
			lead = instr + `<w:r><w:fldChar w:fldCharType="separate"/></w:r>`
		}
		b.WriteString("<w:p>" + e.ppr + lead + e.link + "</w:p>")
	}
	b.WriteString("<w:p>" + tail)
	return doc[:loc[0]] + b.String() + doc[loc[1]:]
}

// cellPadTwips is the left and right cell margin pandoc's table style sets.
const cellPadTwips = 2 * 108

// columnWidths fits the columns to their content. A column is never narrower
// than its longest word, the rule the PDF's tables follow: Word otherwise
// breaks "m5.xlarge" into "m5.xla" and "rge". Natural widths are kept when
// they fit; otherwise the slack is shared by what each column could still use.
// need is the width of the longest words side by side.
func columnWidths(rows []string, measure int, monoEm float64) ([]int, int) {
	var minW, maxW []int
	for _, row := range rows {
		for i, cell := range cellRe.FindAllString(row, -1) {
			for len(minW) <= i {
				minW, maxW = append(minW, 0), append(maxW, 0)
			}
			for _, line := range cellLines(cell) {
				if w := setWidth(line, monoEm) + cellPadTwips; w > maxW[i] {
					maxW[i] = w
				}
				for _, word := range words(line) {
					if w := setWidth(word, monoEm) + cellPadTwips; w > minW[i] {
						minW[i] = w
					}
				}
			}
		}
	}
	if len(minW) == 0 {
		return nil, 0
	}
	sumMin, sumMax := 0, 0
	for i := range minW {
		sumMin += minW[i]
		sumMax += maxW[i]
	}
	widths := make([]int, len(minW))
	switch {
	case sumMax <= measure:
		copy(widths, maxW)
	case sumMin >= measure:
		copy(widths, minW)
	default:
		slack, spread := measure-sumMin, sumMax-sumMin
		for i := range widths {
			widths[i] = minW[i] + slack*(maxW[i]-minW[i])/spread
		}
	}
	return widths, sumMin
}

// glyph is one character of a cell and whether it sets in the code face,
// which is wider than the body's: `report` in a cell broke in Google Docs
// when it was measured as prose.
type glyph struct {
	r    rune
	mono bool
}

var runRe = regexp.MustCompile(`(?s)<w:r>.*?</w:r>`)

// cellLines reads a cell's paragraphs as lines of glyphs.
func cellLines(cell string) [][]glyph {
	var lines [][]glyph
	for _, par := range paraRe.FindAllString(cell, -1) {
		var line []glyph
		for _, r := range runRe.FindAllString(par, -1) {
			st := rStyleRe.FindStringSubmatch(r)
			mono := st != nil && (st[1] == "VerbatimChar" || strings.HasSuffix(st[1], "Tok"))
			for _, m := range textRe.FindAllStringSubmatch(r, -1) {
				for _, c := range html.UnescapeString(m[1]) {
					line = append(line, glyph{c, mono})
				}
			}
		}
		lines = append(lines, line)
	}
	return lines
}

// words splits a line at its spaces, where a cell may wrap.
func words(line []glyph) [][]glyph {
	var out [][]glyph
	start := -1
	for i, g := range line {
		if g.r == ' ' {
			if start >= 0 {
				out = append(out, line[start:i])
			}
			start = -1
		} else if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, line[start:])
	}
	return out
}

// setWidth is a run of glyphs' width at the table's size, in twips.
func setWidth(gs []glyph, monoEm float64) int {
	em := 0.0
	for _, g := range gs {
		if g.mono {
			em += monoEm
		} else {
			em += advance(g.r)
		}
	}
	return int(em*SmallPt*20*1.06 + 0.5)
}

func setWidths(tbl string, widths []int) string {
	total := 0
	var grid strings.Builder
	grid.WriteString("<w:tblGrid>")
	for _, w := range widths {
		total += w
		fmt.Fprintf(&grid, `<w:gridCol w:w="%d"/>`, w)
	}
	grid.WriteString("</w:tblGrid>")
	tbl = gridRe.ReplaceAllLiteralString(tbl, grid.String())
	// Centred at its own width, as the PDF sets a table narrower than the
	// measure.
	tbl = tblWRe.ReplaceAllLiteralString(tbl, fmt.Sprintf(`<w:tblW w:w="%d" w:type="dxa"/><w:jc w:val="center"/>`, total))
	tbl = strings.Replace(tbl, "<w:tblLayout w:type=\"autofit\" />", `<w:tblLayout w:type="fixed"/>`, 1)

	return rowRe.ReplaceAllStringFunc(tbl, func(row string) string {
		i := 0
		return cellRe.ReplaceAllStringFunc(row, func(cell string) string {
			w := 0
			if i < len(widths) {
				w = widths[i]
			}
			i++
			return setCellProp(cell, fmt.Sprintf(`<w:tcW w:w="%d" w:type="dxa"/>`, w), "")
		})
	})
}

// setCellProp puts width first and borders after it, the order the schema
// requires, replacing any of either already there.
func setCellProp(cell, width, borders string) string {
	return tcPrRe.ReplaceAllStringFunc(cell, func(pr string) string {
		inner := strings.TrimSuffix(strings.TrimPrefix(pr, "<w:tcPr>"), "</w:tcPr>")
		if strings.HasPrefix(pr, "<w:tcPr ") || pr == "<w:tcPr/>" {
			inner = ""
		}
		w, bd := width, borders
		if w == "" {
			w = tcWRe.FindString(inner)
		}
		if bd == "" {
			bd = tcBdrRe.FindString(inner)
		}
		inner = tcBdrRe.ReplaceAllString(tcWRe.ReplaceAllString(inner, ""), "")
		return "<w:tcPr>" + w + bd + inner + "</w:tcPr>"
	})
}

// ruleAndKeep draws booktabs rules on the cells, keeps every row whole, glues
// the rows a break must not separate, and sets the table's text a step down.
// Borders go on cells because Google Docs drops a table style's on import.
func ruleAndKeep(tbl string, keep int) string {
	rows := rowRe.FindAllString(tbl, -1)
	if len(rows) == 0 {
		return tbl
	}
	header := 0
	for header < len(rows) && strings.Contains(rows[header], "<w:tblHeader") {
		header++
	}
	body := len(rows) - header
	const heavy, light = 8, 4 // eighths of a point: booktabs' top/bottom and mid rules
	idx := 0
	return rowRe.ReplaceAllStringFunc(tbl, func(row string) string {
		i := idx
		idx++
		var b strings.Builder
		if i == 0 {
			fmt.Fprintf(&b, `<w:top w:val="single" w:sz="%d" w:space="0" w:color="000000"/>`, heavy)
		}
		if header > 0 && i == header-1 {
			fmt.Fprintf(&b, `<w:bottom w:val="single" w:sz="%d" w:space="0" w:color="000000"/>`, light)
		}
		if i == len(rows)-1 {
			fmt.Fprintf(&b, `<w:bottom w:val="single" w:sz="%d" w:space="0" w:color="000000"/>`, heavy)
		}
		if b.Len() > 0 {
			borders := "<w:tcBorders>" + b.String() + "</w:tcBorders>"
			row = cellRe.ReplaceAllStringFunc(row, func(cell string) string { return setCellProp(cell, "", borders) })
		}

		// No break after this row when it is a header row, or when a break
		// here would leave fewer than keep body rows on either side.
		j := i - header // body row index
		glue := i < len(rows)-1 && (i < header || j+1 < keep || body-(j+1) < keep)
		row = rowProps(row)
		if glue {
			row = keepNext(row)
		}
		return smallRuns(row)
	})
}

// rowProps forbids a row from breaking across pages.
func rowProps(row string) string {
	if loc := trPrRe.FindStringIndex(row); loc != nil {
		pr := row[loc[0]:loc[1]]
		if strings.Contains(pr, "cantSplit") {
			return row
		}
		inner := ""
		if strings.HasPrefix(pr, "<w:trPr>") {
			inner = strings.TrimSuffix(strings.TrimPrefix(pr, "<w:trPr>"), "</w:trPr>")
		}
		return row[:loc[0]] + "<w:trPr><w:cantSplit/>" + inner + "</w:trPr>" + row[loc[1]:]
	}
	return strings.Replace(row, "<w:tr>", "<w:tr><w:trPr><w:cantSplit/></w:trPr>", 1)
}

var (
	rPrRe   = regexp.MustCompile(`(?s)<w:rPr>.*?</w:rPr>`)
	sizeRe  = regexp.MustCompile(`<w:sz(Cs)?\b[^>]*/>`)
	afterSz = regexp.MustCompile(`<w:(highlight|u|effect|bdr|shd|fitText|vertAlign|rtl|cs|em|lang|eastAsianLayout|specVanish|oMath)\b`)
)

func smallRuns(s string) string { return sizeRuns(s, SmallPt) }

// sizeRuns sets every run's size, keeping <w:sz> where the schema puts it.
func sizeRuns(s string, pt float64) string {
	sz := fmt.Sprintf(`<w:sz w:val="%d"/><w:szCs w:val="%d"/>`, halfPt(pt), halfPt(pt))
	s = rPrRe.ReplaceAllStringFunc(s, func(pr string) string {
		pr = sizeRe.ReplaceAllString(pr, "")
		if loc := afterSz.FindStringIndex(pr); loc != nil {
			return pr[:loc[0]] + sz + pr[loc[0]:]
		}
		return strings.Replace(pr, "</w:rPr>", sz+"</w:rPr>", 1)
	})
	// Runs with no properties at all get some.
	var b strings.Builder
	for {
		i := strings.Index(s, "<w:r>")
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		i += len("<w:r>")
		b.WriteString(s[:i])
		s = s[i:]
		if !strings.HasPrefix(s, "<w:rPr>") {
			b.WriteString("<w:rPr>" + sz + "</w:rPr>")
		}
	}
}

func keepNext(s string) string {
	return pStyleRe.ReplaceAllStringFunc(s, func(m string) string {
		return m + "<w:keepNext/>"
	})
}

var (
	codeParaRe = regexp.MustCompile(`(?s)<w:p><w:pPr><w:pStyle w:val="SourceCode"\s*/>.*?</w:p>`)
	rStyleRe   = regexp.MustCompile(`<w:rStyle w:val="([^"]+)"\s*/>`)
	brRe       = regexp.MustCompile(`<w:br\s*/>`)
)

// codeBlocks fits each bare code block to the measure the way the PDF does:
// the largest size down to TinyPt at which its longest line fits. A bare fence
// is never wrapped — it may be an ASCII diagram — and Word would wrap it. A
// fence that declares a language is highlighted, and wraps.
func codeBlocks(doc string, l Look) (string, []string) {
	var wide []string
	measure := l.TextWidthMM() / 25.4 * 72 // points
	em := MonoAdvance(l.Mono)
	out := codeParaRe.ReplaceAllStringFunc(doc, func(par string) string {
		for _, m := range rStyleRe.FindAllStringSubmatch(par, -1) {
			if m[1] != "VerbatimChar" {
				return par
			}
		}
		cols := 0
		for _, line := range brRe.Split(par, -1) {
			n := 0
			for _, m := range textRe.FindAllStringSubmatch(line, -1) {
				n += len([]rune(html.UnescapeString(m[1])))
			}
			cols = max(cols, n)
		}
		size := float64(TinyPt)
		for _, pt := range []float64{SmallPt, 8.5, TinyPt} {
			if float64(cols)*em*pt <= measure {
				size = pt
				break
			}
		}
		if fits := int(measure / (em * TinyPt)); cols > fits {
			wide = append(wide, fmt.Sprintf("    %d columns, where %d fit", cols, fits))
		}
		if size == SmallPt {
			return par
		}
		return sizeRuns(par, size)
	})
	if len(wide) == 0 {
		return out, nil
	}
	return out, []string{fmt.Sprintf("%d code block(s) stay past the measure at the smallest legible size:\n%s\n"+
		"  The size was already stepped down as far as it goes. Shorten the lines\n"+
		"  or split the block; an ASCII diagram is not wrapped, by design.", len(wide), strings.Join(wide, "\n"))}
}

var settingsAfter = regexp.MustCompile(`<w:(hdrShapeDefaults|footnotePr|endnotePr|compat|docVars|rsids|attachedSchema|themeFontLang|clrSchemeMapping|doNotIncludeSubdocsInStats|doNotAutoCompressPictures|forceUpgrade|captions|readModeInkLockDown|smartTagType|schemaLibrary|shapeDefaults|doNotEmbedSmartTags|decimalSymbol|listSeparator)\b|<m:mathPr\b`)

// updateFields makes Word offer to fill the table of contents when the file
// opens. Without it the TOC field stays empty until someone thinks of F9.
func updateFields(settings string) string {
	if settings == "" || strings.Contains(settings, "<w:updateFields") {
		return settings
	}
	tag := `<w:updateFields w:val="true"/>`
	if loc := settingsAfter.FindStringIndex(settings); loc != nil {
		return settings[:loc[0]] + tag + settings[loc[0]:]
	}
	return strings.Replace(settings, "</w:settings>", tag+"</w:settings>", 1)
}

var (
	fontAttrRe   = regexp.MustCompile(`w:(?:ascii|hAnsi|eastAsia|cs)="([^"]*)"`)
	themeLatinRe = regexp.MustCompile(`<a:latin typeface="([^"]*)"`)
)

// strayFonts lists the faces the package names besides the declared ones.
// Cambria Math is Word's equation face and comes with every copy of it.
func strayFonts(p *pkg, l Look) []string {
	ok := map[string]bool{l.Body: true, l.Display: true, l.Mono: true, "Cambria Math": true, "": true}
	seen := map[string]bool{}
	for _, name := range p.names {
		if !strings.HasPrefix(name, "word/") || !strings.HasSuffix(name, ".xml") {
			continue
		}
		s := p.get(name)
		re := fontAttrRe
		if strings.HasPrefix(name, "word/theme/") {
			re = themeLatinRe
		}
		for _, m := range re.FindAllStringSubmatch(s, -1) {
			if !ok[m[1]] {
				seen[m[1]] = true
			}
		}
	}
	var out []string
	for f := range seen {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}
