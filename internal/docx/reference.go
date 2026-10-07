package docx

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	nsW   = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"`
	nsR   = `xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	nsWP  = `xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"`
	nsA   = `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"`
	nsPic = `xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"`

	relHeader = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/header"
	relFooter = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer"
	relImage  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"
	ctHeader  = "application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"
	ctFooter  = "application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"
)

// Reference turns pandoc's own reference.docx into the brand's. pandoc copies
// its styles, theme, headers, footers and section properties into every
// document it writes, so this is where the identity goes in.
func Reference(base []byte, l Look) ([]byte, error) {
	p, err := readPkg(base)
	if err != nil {
		return nil, err
	}
	if p.get("word/styles.xml") == "" || p.get("word/document.xml") == "" {
		return nil, fmt.Errorf("pandoc's reference.docx has no styles or document part")
	}
	p.set("word/theme/theme1.xml", themeFonts(p.get("word/theme/theme1.xml"), l))
	p.set("word/styles.xml", styles(p.get("word/styles.xml"), l))

	var refs strings.Builder
	if l.Layout != "letter" {
		hdr, err := header(p, l)
		if err != nil {
			return nil, err
		}
		ftr := footer(p, l)
		fmt.Fprintf(&refs, `<w:headerReference w:type="default" r:id="%s"/><w:footerReference w:type="default" r:id="%s"/>`, hdr, ftr)
	}
	sect := fmt.Sprintf(`<w:sectPr>%s<w:pgSz w:w="%d" w:h="%d"/>`+
		`<w:pgMar w:top="%d" w:right="%d" w:bottom="%d" w:left="%d" w:header="%d" w:footer="%d" w:gutter="0"/>`,
		refs.String(), twips(l.PaperWMM), twips(l.PaperHMM),
		twips(l.MarginMM), twips(l.MarginMM), twips(l.MarginMM), twips(l.MarginMM),
		twips(l.MarginMM*0.45), twips(l.MarginMM*0.45))
	if l.NumberFromCover {
		sect += `<w:pgNumType w:start="0"/>`
	}
	if l.Layout == "report" {
		// The cover's page: no header, no footer, because none is defined for it.
		sect += `<w:titlePg/>`
	}
	sect += `</w:sectPr>`

	doc := p.get("word/document.xml")
	if sectRe.MatchString(doc) {
		doc = sectRe.ReplaceAllLiteralString(doc, sect)
	} else {
		doc = strings.Replace(doc, "</w:body>", sect+"</w:body>", 1)
	}
	if !strings.Contains(doc, "xmlns:r=") {
		doc = strings.Replace(doc, "<w:document ", "<w:document "+nsR+" ", 1)
	}
	p.set("word/document.xml", doc)
	return p.bytes()
}

var sectRe = regexp.MustCompile(`(?s)<w:sectPr.*?</w:sectPr>|<w:sectPr\s*/>`)

// themeFonts replaces the theme's faces. pandoc's reference names Aptos, which
// Google Docs and any Word older than 2023 substitute without a word, and every
// style that says "the theme's font" would follow it there.
func themeFonts(theme string, l Look) string {
	theme = replaceLatin(theme, "majorFont", l.Display)
	return replaceLatin(theme, "minorFont", l.Body)
}

func replaceLatin(theme, block, face string) string {
	re := regexp.MustCompile(`(?s)(<a:` + block + `>.*?<a:latin typeface=")[^"]*(")`)
	return re.ReplaceAllString(theme, "${1}"+esc(face)+"${2}")
}

// styleSpec is the direct formatting one paragraph or character style gets.
// Empty fields leave the style's own setting alone.
type styleSpec struct {
	font    string
	color   string
	sizePt  float64
	bold    *bool
	italic  *bool
	jc      string
	before  int // twips; -1 leaves it alone
	after   int
	keep    bool
	borderB string // colour of a bottom rule
}

var yes, no = true, false

func styles(s string, l Look) string {
	line := int(240*l.LineStretch + 0.5)
	s = docDefaults(s, l.Body, line)

	set := map[string]styleSpec{
		"Normal":         {font: l.Body, sizePt: BodyPt, before: -1, after: -1},
		"BodyText":       {before: 0, after: 120},
		"FirstParagraph": {before: 0, after: 120},
		"Compact":        {before: 20, after: 20},
		"Title":          {font: l.Display, color: l.Text, sizePt: 26, bold: &yes, jc: "left", before: 0, after: 120},
		"Subtitle":       {font: l.Display, color: l.Text, sizePt: 14, bold: &no, italic: &no, jc: "left", before: 0, after: 120},
		"Author":         {font: l.Display, color: l.Text, sizePt: SmallPt, jc: "left", before: 0, after: 0},
		"Date":           {font: l.Display, color: l.Text, sizePt: SmallPt, jc: "left", before: 0, after: 0},
		// The PDF never colours a heading, so neither does the .docx: primary
		// headings made every Word copy of a brand-none document look blue.
		"Heading1":     {font: l.Display, color: l.Text, sizePt: 14, bold: &yes, before: 360, after: 120, keep: true},
		"Heading2":     {font: l.Display, color: l.Text, sizePt: 12, bold: &yes, before: 240, after: 100, keep: true},
		"Heading3":     {font: l.Display, color: l.Text, sizePt: 10, bold: &yes, before: 200, after: 80, keep: true},
		"Heading4":     {font: l.Display, color: l.Text, sizePt: 10, bold: &yes, italic: &no, before: 160, after: 60, keep: true},
		"TOCHeading":   {font: l.Display, color: l.Text, sizePt: 14, bold: &yes, before: 0, after: 200},
		"Caption":      {sizePt: CaptionPt, color: l.Text, italic: &no, jc: "center", before: 60, after: 200},
		"ImageCaption": {sizePt: CaptionPt, color: l.Text, italic: &no, jc: "center", before: 60, after: 200},
		"TableCaption": {sizePt: CaptionPt, color: l.Text, italic: &no, jc: "center", before: 200, after: 80, keep: true},
		"FootnoteText": {sizePt: TinyPt, before: 0, after: 40},
		"BlockText":    {sizePt: SmallPt, color: l.Text, before: 100, after: 100},
		"Bibliography": {sizePt: SmallPt, before: 0, after: 80},
		"VerbatimChar": {font: l.Mono, sizePt: SmallPt, before: -1, after: -1},
		"Hyperlink":    {color: l.Primary, before: -1, after: -1},
	}
	// Every entry sets before and after: zero is a real spacing, so -1 is
	// what leaves one alone.
	for id, spec := range set {
		s = applyStyle(s, id, spec)
	}
	// pandoc writes code with a "Source Code" paragraph style it adds itself
	// when the reference lacks one; declaring it here keeps its face ours.
	for lvl := 1; lvl <= 3; lvl++ {
		if !strings.Contains(s, fmt.Sprintf(`w:styleId="TOC%d"`, lvl)) {
			s = strings.Replace(s, "</w:styles>", tocStyle(lvl, l.Text), 1)
		}
	}
	if !strings.Contains(s, `w:styleId="SourceCode"`) {
		s = strings.Replace(s, "</w:styles>", sourceCodeStyle(l.Mono)+"</w:styles>", 1)
	}
	if !strings.Contains(s, `w:styleId="`+AccentStyle+`"`) {
		s = strings.Replace(s, "</w:styles>", accentStyle(l.Primary)+"</w:styles>", 1)
	}
	return s
}

// AccentStyle is the character style an [accented]{.accent} span is written
// with. Repair also colours its runs directly, because Google Docs drops
// character styles on import; the style is what a Word user sees named.
const AccentStyle = "Accent"

func accentStyle(color string) string {
	return fmt.Sprintf(`<w:style w:type="character" w:customStyle="1" w:styleId="%s">`+
		`<w:name w:val="%s"/><w:basedOn w:val="DefaultParagraphFont"/>`+
		`<w:rPr><w:color w:val="%s"/></w:rPr></w:style>`, AccentStyle, AccentStyle, color)
}

// tocStyle is one level of the table of contents, indented like the PDF's,
// with the first level bold.
func tocStyle(lvl int, color string) string {
	bold := ""
	if lvl == 1 {
		bold = "<w:b/><w:bCs/>"
	}
	return fmt.Sprintf(`<w:style w:type="paragraph" w:styleId="TOC%d"><w:name w:val="toc %d"/>`+
		`<w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:uiPriority w:val="39"/><w:unhideWhenUsed/>`+
		`<w:pPr><w:tabs><w:tab w:val="right" w:leader="dot" w:pos="9000"/></w:tabs>`+
		`<w:spacing w:before="%d" w:after="40"/><w:ind w:left="%d"/></w:pPr>`+
		`<w:rPr>%s<w:color w:val="%s"/></w:rPr></w:style></w:styles>`,
		lvl, lvl, map[bool]int{true: 120, false: 0}[lvl == 1], (lvl-1)*220, bold, color)
}

func sourceCodeStyle(mono string) string {
	return fmt.Sprintf(`<w:style w:type="paragraph" w:customStyle="1" w:styleId="SourceCode">`+
		`<w:name w:val="Source Code"/><w:basedOn w:val="Normal"/><w:link w:val="VerbatimChar"/>`+
		`<w:pPr><w:spacing w:before="60" w:after="160" w:line="240" w:lineRule="auto"/></w:pPr>`+
		`<w:rPr>%s<w:sz w:val="%d"/><w:szCs w:val="%d"/></w:rPr></w:style>`,
		fontsXML(mono), halfPt(SmallPt), halfPt(SmallPt))
}

var defaultsRe = regexp.MustCompile(`(?s)<w:docDefaults>.*?</w:docDefaults>`)

func docDefaults(s, body string, line int) string {
	d := fmt.Sprintf(`<w:docDefaults><w:rPrDefault><w:rPr>%s<w:sz w:val="%d"/><w:szCs w:val="%d"/>`+
		`</w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="120" w:line="%d" w:lineRule="auto"/>`+
		`</w:pPr></w:pPrDefault></w:docDefaults>`, fontsXML(body), halfPt(BodyPt), halfPt(BodyPt), line)
	if defaultsRe.MatchString(s) {
		return defaultsRe.ReplaceAllLiteralString(s, d)
	}
	return s
}

// fontsXML names one face for every script slot. Theme attributes are left out
// on purpose: they win over the explicit names, and they point at the theme.
func fontsXML(face string) string {
	f := esc(face)
	return fmt.Sprintf(`<w:rFonts w:ascii="%s" w:hAnsi="%s" w:eastAsia="%s" w:cs="%s"/>`, f, f, f, f)
}

// applyStyle rewrites one style's formatting in place. A style the reference
// lacks is left alone: pandoc adds its own when it needs one.
func applyStyle(s, id string, sp styleSpec) string {
	re := regexp.MustCompile(`(?s)<w:style\b[^>]*w:styleId="` + id + `".*?</w:style>`)
	loc := re.FindStringIndex(s)
	if loc == nil {
		return s
	}
	st := s[loc[0]:loc[1]]

	var r []elem
	if sp.font != "" {
		r = append(r, elem{"rFonts", fontsXML(sp.font)})
	}
	if sp.bold != nil {
		r = append(r, toggle("b", *sp.bold), toggle("bCs", *sp.bold))
	}
	if sp.italic != nil {
		r = append(r, toggle("i", *sp.italic), toggle("iCs", *sp.italic))
	}
	if sp.color != "" {
		r = append(r, elem{"color", fmt.Sprintf(`<w:color w:val="%s"/>`, sp.color)})
	}
	if sp.sizePt > 0 {
		hp := halfPt(sp.sizePt)
		r = append(r, elem{"sz", fmt.Sprintf(`<w:sz w:val="%d"/>`, hp)}, elem{"szCs", fmt.Sprintf(`<w:szCs w:val="%d"/>`, hp)})
	}

	var pp []elem
	if sp.keep {
		pp = append(pp, elem{"keepNext", "<w:keepNext/>"}, elem{"keepLines", "<w:keepLines/>"})
	}
	if sp.borderB != "" {
		pp = append(pp, elem{"pBdr", fmt.Sprintf(`<w:pBdr><w:bottom w:val="single" w:sz="4" w:space="4" w:color="%s"/></w:pBdr>`, sp.borderB)})
	}
	if sp.before >= 0 || sp.after >= 0 {
		var sb strings.Builder
		sb.WriteString(`<w:spacing`)
		if sp.before >= 0 {
			fmt.Fprintf(&sb, ` w:before="%d"`, sp.before)
		}
		if sp.after >= 0 {
			fmt.Fprintf(&sb, ` w:after="%d"`, sp.after)
		}
		sb.WriteString(`/>`)
		pp = append(pp, elem{"spacing", sb.String()})
	}
	if sp.jc != "" {
		pp = append(pp, elem{"jc", fmt.Sprintf(`<w:jc w:val="%s"/>`, sp.jc)})
	}

	st = withProps(st, "pPr", pp, pPrOrder)
	st = withProps(st, "rPr", r, rPrOrder)
	return s[:loc[0]] + st + s[loc[1]:]
}

// toggle is an on/off property: present to set it, empty to remove it.
func toggle(tag string, on bool) elem {
	if on {
		return elem{tag, "<w:" + tag + "/>"}
	}
	return elem{tag, ""}
}

// withProps merges elements into a style's <w:pPr> or <w:rPr>, creating it if
// needed, in schema order. The style itself orders pPr before rPr.
func withProps(st, tag string, set []elem, order []string) string {
	if len(set) == 0 {
		return st
	}
	re := regexp.MustCompile(`(?s)<w:` + tag + `\s*/>|<w:` + tag + `>.*?</w:` + tag + `>`)
	if loc := re.FindStringIndex(st); loc != nil {
		cur := st[loc[0]:loc[1]]
		inner := ""
		if strings.HasPrefix(cur, "<w:"+tag+">") {
			inner = strings.TrimSuffix(strings.TrimPrefix(cur, "<w:"+tag+">"), "</w:"+tag+">")
		}
		return st[:loc[0]] + "<w:" + tag + ">" + setChildren(inner, set, order) + "</w:" + tag + ">" + st[loc[1]:]
	}
	block := "<w:" + tag + ">" + setChildren("", set, order) + "</w:" + tag + ">"
	for _, before := range []string{"<w:rPr", "<w:tblPr", "</w:style>"} {
		if tag == "rPr" && before == "<w:rPr" {
			continue
		}
		if i := strings.Index(st, before); i >= 0 {
			return st[:i] + block + st[i:]
		}
	}
	return st
}

// header writes the running header: the title on the left, the logo on the
// right, a hairline under both — the PDF's header, in paragraphs.
func header(p *pkg, l Look) (string, error) {
	var logo string
	if len(l.Logo) > 0 {
		p.set("word/media/mdbrand-logo.png", string(l.Logo))
		addDefault(p, "png", "image/png")
		w := l.LogoHeaderWMM
		logo = `<w:r><w:tab/></w:r><w:r>` + drawing("rIdMdbrandLogo", 9001, w, w*l.LogoAspect) + `</w:r>`
		p.set("word/_rels/header1.xml.rels", rels(map[string][2]string{
			"rIdMdbrandLogo": {relImage, "media/mdbrand-logo.png"},
		}))
	}
	tw := twips(l.TextWidthMM())
	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<w:hdr %s %s %s %s %s><w:p><w:pPr><w:pBdr><w:bottom w:val="single" w:sz="4" w:space="4" w:color="%s"/></w:pBdr>`+
		`<w:tabs><w:tab w:val="right" w:pos="%d"/></w:tabs><w:spacing w:before="0" w:after="0"/></w:pPr>`+
		`<w:r><w:rPr>%s<w:color w:val="%s"/><w:sz w:val="%d"/></w:rPr><w:t xml:space="preserve">%s</w:t></w:r>%s</w:p></w:hdr>`,
		nsW, nsR, nsWP, nsA, nsPic, l.Rule, tw, fontsXML(l.Display), l.Text, halfPt(TinyPt), esc(l.RunningTitle), logo)
	return addPart(p, "word/header1.xml", xml, relHeader, ctHeader, "rIdMdbrandHeader"), nil
}

// footer centres the page number, as the PDF does, with the confidentiality
// label, when there is one, on the left.
func footer(p *pkg, l Look) string {
	align, label := `<w:jc w:val="center"/>`, ""
	if l.Confidential != "" {
		align = fmt.Sprintf(`<w:tabs><w:tab w:val="center" w:pos="%d"/></w:tabs>`, twips(l.TextWidthMM()/2))
		label = fmt.Sprintf(`<w:r><w:rPr>%s<w:color w:val="%s"/><w:sz w:val="%d"/></w:rPr><w:t xml:space="preserve">%s</w:t></w:r><w:r><w:tab/></w:r>`,
			fontsXML(l.Display), l.Primary, halfPt(TinyPt), esc(l.Confidential))
	}
	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<w:ftr %s %s><w:p><w:pPr><w:spacing w:before="0" w:after="0"/>%s</w:pPr>%s`+
		`<w:r><w:rPr><w:sz w:val="%d"/></w:rPr><w:fldChar w:fldCharType="begin"/></w:r>`+
		`<w:r><w:rPr><w:sz w:val="%d"/></w:rPr><w:instrText xml:space="preserve"> PAGE </w:instrText></w:r>`+
		`<w:r><w:rPr><w:sz w:val="%d"/></w:rPr><w:fldChar w:fldCharType="separate"/></w:r>`+
		`<w:r><w:rPr><w:sz w:val="%d"/></w:rPr><w:t>1</w:t></w:r>`+
		`<w:r><w:rPr><w:sz w:val="%d"/></w:rPr><w:fldChar w:fldCharType="end"/></w:r></w:p></w:ftr>`,
		nsW, nsR, align, label, halfPt(TinyPt), halfPt(TinyPt), halfPt(TinyPt), halfPt(TinyPt), halfPt(TinyPt))
	return addPart(p, "word/footer1.xml", xml, relFooter, ctFooter, "rIdMdbrandFooter")
}

// addPart stores a part, registers its content type and relates it to the
// document body, returning the relationship id.
func addPart(p *pkg, name, xml, relType, contentType, id string) string {
	p.set(name, xml)
	ct := p.get("[Content_Types].xml")
	part := "/" + name
	if !strings.Contains(ct, `PartName="`+part+`"`) {
		ct = strings.Replace(ct, "</Types>", fmt.Sprintf(`<Override PartName="%s" ContentType="%s"/></Types>`, part, contentType), 1)
		p.set("[Content_Types].xml", ct)
	}
	r := p.get("word/_rels/document.xml.rels")
	target := strings.TrimPrefix(name, "word/")
	if !strings.Contains(r, `Id="`+id+`"`) {
		r = strings.Replace(r, "</Relationships>", fmt.Sprintf(`<Relationship Id="%s" Type="%s" Target="%s"/></Relationships>`, id, relType, target), 1)
		p.set("word/_rels/document.xml.rels", r)
	}
	return id
}

func addDefault(p *pkg, ext, contentType string) {
	ct := p.get("[Content_Types].xml")
	if strings.Contains(ct, `Extension="`+ext+`"`) {
		return
	}
	ct = strings.Replace(ct, "<Default ", fmt.Sprintf(`<Default Extension="%s" ContentType="%s"/><Default `, ext, contentType), 1)
	p.set("[Content_Types].xml", ct)
}

func rels(m map[string][2]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for id, v := range m {
		fmt.Fprintf(&b, `<Relationship Id="%s" Type="%s" Target="%s"/>`, id, v[0], v[1])
	}
	b.WriteString(`</Relationships>`)
	return b.String()
}

// drawing is an inline picture, the only kind every importer keeps in place.
func drawing(rid string, id int, wMM, hMM float64) string {
	cx, cy := emu(wMM), emu(hMM)
	return fmt.Sprintf(`<w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0">`+
		`<wp:extent cx="%d" cy="%d"/><wp:docPr id="%d" name="logo"/>`+
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">`+
		`<pic:pic><pic:nvPicPr><pic:cNvPr id="%d" name="logo.png"/><pic:cNvPicPr/></pic:nvPicPr>`+
		`<pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`+
		`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm>`+
		`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic>`+
		`</wp:inline></w:drawing>`, cx, cy, id, id, rid, cx, cy)
}
