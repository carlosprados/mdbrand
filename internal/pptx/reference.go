package pptx

import (
	"fmt"
	"regexp"
	"strings"
)

// Media the brand adds to the reference. pandoc copies the master and layouts
// that point at them and drops the files themselves, so Repair puts them back
// under the same names.
const (
	mediaLogo      = "ppt/media/mdbrand-logo.png"
	mediaCoverLogo = "ppt/media/mdbrand-cover-logo.png"
	mediaArt       = "ppt/media/mdbrand-art.png"
)

func (l Look) media() map[string][]byte {
	m := map[string][]byte{}
	if l.Logo != nil {
		m[mediaLogo] = l.Logo
	}
	if l.CoverLogo != nil {
		m[mediaCoverLogo] = l.CoverLogo
	}
	if l.Art != nil {
		m[mediaArt] = l.Art
	}
	return m
}

// Type sizes on the slide, in points: the PDF deck's, scaled. beamer sets
// the body at 11pt and frame titles at \Large; pandoc's master asks 24/21/18
// and 33, which holds a third less text than the slide the author checked.
const (
	titlePt  = 23
	footerPt = 11
)

var bodyPt = []float64{18, 16, 14}

// Reference turns pandoc's own reference.pptx into the brand's: theme colours
// and fonts, the footer on the master, dark covers, and room for a figure
// under the text on a slide that has both.
func Reference(base []byte, l Look) ([]byte, error) {
	p, err := readPkg(base)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"ppt/theme/theme1.xml", "ppt/theme/theme2.xml"} {
		if x := p.get(name); x != "" {
			p.set(name, theme(x, l))
		}
	}
	master := "ppt/slideMasters/slideMaster1.xml"
	if p.get(master) == "" {
		return nil, fmt.Errorf("pptx: pandoc's reference has no %s", master)
	}
	m := sizes(p.get(master), l)
	if l.Logo != nil || l.Confidential != "" || l.RunningTitle != "" {
		m = strings.Replace(m, "</p:spTree>", footer(l)+"</p:spTree>", 1)
	}
	p.set(master, m)
	if l.Logo != nil {
		addRel(p, "ppt/slideMasters/_rels/slideMaster1.xml.rels", "rIdMdLogo", "../media/mdbrand-logo.png")
	}
	for _, name := range []string{"Title Slide", "Section Header"} {
		part, ok := layout(p, name)
		if !ok {
			return nil, fmt.Errorf("pptx: pandoc's reference has no %q layout", name)
		}
		p.set(part, cover(p, part, name == "Title Slide", l))
	}
	// Columns are body text: pandoc's layouts set them larger than the
	// slide's own body, at 21pt, so a two-column slide held less than the
	// frame the author checked.
	for _, name := range []string{"Two Content", "Comparison"} {
		if part, ok := layout(p, name); ok {
			p.set(part, defRPrSzRe.ReplaceAllString(p.get(part), "$1"))
		}
	}
	if part, ok := layout(p, "Title Slide"); ok {
		p.set(part, titleSlide(p.get(part), l))
	}
	if part, ok := layout(p, "Content with Caption"); ok {
		x := p.get(part)
		x = placeholderEdit(x, `type="title"`, func(sp string) string { return setRPr(sp, titlePt, true) })
		x = placeholderEdit(x, `type="body" sz="half" idx="2"`, func(sp string) string { return setRPr(sp, bodyPt[0], false) })
		x = placeholderXfrm(x, `type="title"`, 12.7, 5.7, 228.6, 24.2)
		x = placeholderXfrm(x, `type="body" sz="half" idx="2"`, 12.7, 31, 228.6, 30)
		x = placeholderXfrm(x, `idx="1"`, 12.7, 63, 228.6, 64)
		p.set(part, x)
	}
	for name, raw := range l.media() {
		p.setBytes(name, raw)
	}
	addPNGType(p)
	return p.bytes()
}

var (
	majorLatinRe = regexp.MustCompile(`(?s)(<a:majorFont>\s*<a:latin typeface=")[^"]*(")`)
	minorLatinRe = regexp.MustCompile(`(?s)(<a:minorFont>\s*<a:latin typeface=")[^"]*(")`)
)

// theme sets the colours a slide draws with and the two faces, in the deck's
// theme and the notes' alike: the notes master asked for Calibri.
func theme(x string, l Look) string {
	set := func(slot, hex string) {
		if hex == "" {
			return
		}
		re := regexp.MustCompile(`(?s)<a:` + slot + `>.*?</a:` + slot + `>`)
		x = re.ReplaceAllLiteralString(x, `<a:`+slot+`><a:srgbClr val="`+hex+`"/></a:`+slot+`>`)
	}
	set("dk1", l.Text)
	set("dk2", l.Ground)
	set("accent1", l.Primary)
	set("hlink", l.Link)
	set("folHlink", l.Link)
	x = majorLatinRe.ReplaceAllString(x, "${1}"+esc(l.Display)+"${2}")
	return minorLatinRe.ReplaceAllString(x, "${1}"+esc(l.Body)+"${2}")
}

var (
	titleStyleRe = regexp.MustCompile(`(?s)<p:titleStyle>.*?</p:titleStyle>`)
	bodyStyleRe  = regexp.MustCompile(`(?s)<p:bodyStyle>.*?</p:bodyStyle>`)
	buFontRe     = regexp.MustCompile(`<a:buFont typeface="[^"]*"`)
)

// sizes sets the master's type in the PDF's proportions, titles at the left
// and bold as the frame's are, and its bullets in the body face: pandoc's are
// Arial, nine times over.
func sizes(m string, l Look) string {
	m = titleStyleRe.ReplaceAllStringFunc(m, func(s string) string {
		s = regexp.MustCompile(`sz="\d+"`).ReplaceAllString(s, fmt.Sprintf(`sz="%d" b="1"`, hundredths(titlePt)))
		return strings.Replace(s, `algn="ctr"`, `algn="l"`, 1)
	})
	m = bodyStyleRe.ReplaceAllStringFunc(m, func(s string) string {
		for lvl := 1; lvl <= 9; lvl++ {
			pt := bodyPt[min(lvl, len(bodyPt))-1]
			re := regexp.MustCompile(fmt.Sprintf(`(?s)(<a:lvl%dpPr[^>]*>.*?<a:defRPr[^>]*?)sz="\d+"`, lvl))
			s = re.ReplaceAllString(s, fmt.Sprintf(`${1}sz="%d"`, hundredths(pt)))
		}
		return s
	})
	return buFontRe.ReplaceAllString(m, `<a:buFont typeface="`+esc(l.Body)+`"`)
}

// footer is the PDF's footline on the master, so every body slide carries it:
// a rule, the logo at the left, the label and title with the slide number at
// the right. The number is a field in a text box rather than the master's
// placeholder, which PowerPoint shows only on slides that carry their own.
func footer(l Look) string {
	const bottom = 3 * Scale
	textH := 6.0
	textY := SlideHMM - bottom - textH
	var b strings.Builder
	ruleY := textY - 1.2*Scale - 1
	fmt.Fprintf(&b, `<p:cxnSp><p:nvCxnSpPr><p:cNvPr id="90" name="mdbrand rule"/><p:cNvCxnSpPr/><p:nvPr userDrawn="1"/></p:nvCxnSpPr>`+
		`<p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="0"/></a:xfrm><a:prstGeom prst="line"><a:avLst/></a:prstGeom>`+
		`<a:ln w="%d"><a:solidFill><a:srgbClr val="%s"/></a:solidFill></a:ln></p:spPr></p:cxnSp>`,
		emu(l.MarginMM), emu(ruleY), emu(SlideWMM-2*l.MarginMM), ptEMU(0.3*Scale), l.Rule)
	textX := l.MarginMM
	if l.Logo != nil {
		h := l.LogoWMM * l.LogoAspect
		b.WriteString(picture(91, "mdbrand logo", "rIdMdLogo", l.MarginMM, textY+textH/2-h/2, l.LogoWMM, h))
		textX += l.LogoWMM + 4
	}
	rpr := func(hex string) string {
		return fmt.Sprintf(`<a:rPr lang="en-GB" sz="%d" dirty="0"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/></a:rPr>`,
			hundredths(footerPt), hex, esc(l.Display))
	}
	const gap = "\u2003" // an em space, as the PDF's \quad
	runs := ""
	if l.Confidential != "" {
		runs += `<a:r>` + rpr(l.Primary) + `<a:t>` + esc(l.Confidential) + gap + `</a:t></a:r>`
	}
	if l.RunningTitle != "" {
		runs += `<a:r>` + rpr(l.Text) + `<a:t>` + esc(l.RunningTitle) + gap + `</a:t></a:r>`
	}
	runs += `<a:fld id="{B6F15528-21DE-4FAA-801E-634DDDAF4B2B}" type="slidenum">` + rpr(l.Text) + `<a:t>‹#›</a:t></a:fld>`
	b.WriteString(textBox(92, "mdbrand footer", textX, textY, SlideWMM-l.MarginMM-textX, textH, "r", runs))
	return b.String()
}

func picture(id int, name, rid string, x, y, w, h float64) string {
	return fmt.Sprintf(`<p:pic><p:nvPicPr><p:cNvPr id="%d" name="%s"/><p:cNvPicPr><a:picLocks noChangeAspect="1"/></p:cNvPicPr><p:nvPr userDrawn="1"/></p:nvPicPr>`+
		`<p:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></p:blipFill>`+
		`<p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic>`,
		id, name, rid, emu(x), emu(y), emu(w), emu(h))
}

func textBox(id int, name string, x, y, w, h float64, align, runs string) string {
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s"/><p:cNvSpPr txBox="1"/><p:nvPr userDrawn="1"/></p:nvSpPr>`+
		`<p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/></p:spPr>`+
		`<p:txBody><a:bodyPr wrap="square" lIns="0" tIns="0" rIns="0" bIns="0" anchor="ctr"><a:noAutofit/></a:bodyPr><a:lstStyle/><a:p><a:pPr algn="%s"/>%s</a:p></p:txBody></p:sp>`,
		id, name, emu(x), emu(y), emu(w), emu(h), align, runs)
}

var (
	layoutNameRe = regexp.MustCompile(`<p:cSld name="([^"]*)"`)
	cSldOpenRe   = regexp.MustCompile(`<p:cSld name="[^"]*">`)
	defRPrSelfRe = regexp.MustCompile(`<a:defRPr([^>]*?)/>`)
	defRPrOpenRe = regexp.MustCompile(`<a:defRPr([^>]*?[^/])>`)
	solidFillRe  = regexp.MustCompile(`(?s)<a:solidFill>.*?</a:solidFill>`)
	grpSpPrEndRe = regexp.MustCompile(`</p:grpSpPr>`)
)

// layout finds a slide layout by the name pandoc places slides by.
func layout(p *pkg, name string) (string, bool) {
	for _, n := range p.names {
		if !strings.HasPrefix(n, "ppt/slideLayouts/slideLayout") || !strings.HasSuffix(n, ".xml") {
			continue
		}
		if m := layoutNameRe.FindStringSubmatch(p.get(n)); m != nil && m[1] == name {
			return n, true
		}
	}
	return "", false
}

// cover makes a title or section slide the PDF's: no footer, the bundle's
// ground and its type colour on every placeholder, the art bled to the right
// edge and, on the title slide, the logo.
func cover(p *pkg, part string, title bool, l Look) string {
	x := sldLayoutRe.ReplaceAllString(p.get(part), `<p:sldLayout showMasterSp="0"`)
	ink := l.Text
	if l.Ground != "" {
		ink = l.OnGround
		bg := `<p:bg><p:bgPr><a:solidFill><a:srgbClr val="` + l.Ground + `"/></a:solidFill><a:effectLst/></p:bgPr></p:bg>`
		x = cSldOpenRe.ReplaceAllStringFunc(x, func(s string) string { return s + bg })
	}
	fill := `<a:solidFill><a:srgbClr val="` + ink + `"/></a:solidFill>`
	x = strings.ReplaceAll(x, "<a:lstStyle/>", `<a:lstStyle><a:lvl1pPr><a:defRPr>`+fill+`</a:defRPr></a:lvl1pPr></a:lstStyle>`)
	x = strings.ReplaceAll(x, ` cap="all"`, "")
	x = defRPrSelfRe.ReplaceAllString(x, `<a:defRPr$1>`+fill+`</a:defRPr>`)
	x = defRPrOpenRe.ReplaceAllStringFunc(x, func(s string) string {
		if strings.Contains(s, fill) {
			return s
		}
		return s + fill
	})
	// A defRPr that already had a fill now holds two; the last would win in
	// LibreOffice and the first in PowerPoint, so keep only the brand's.
	x = dedupeFills(x, fill)
	rels := strings.Replace(part, "slideLayouts/", "slideLayouts/_rels/", 1) + ".rels"
	var art string
	if l.Art != nil && l.ArtAspect > 0 {
		w := SlideHMM / l.ArtAspect
		art += picture(80, "mdbrand art", "rIdMdArt", SlideWMM-w, 0, w, SlideHMM)
		addRel(p, rels, "rIdMdArt", "../media/mdbrand-art.png")
	}
	if title && l.CoverLogo != nil {
		art += picture(81, "mdbrand cover logo", "rIdMdCoverLogo", l.MarginMM, l.MarginMM, l.CoverLogoWMM, l.CoverLogoWMM*l.CoverAspect)
		addRel(p, rels, "rIdMdCoverLogo", "../media/mdbrand-cover-logo.png")
	}
	// First in the tree, so the placeholders draw over them.
	return grpSpPrEndRe.ReplaceAllLiteralString(x, "</p:grpSpPr>"+art)
}

var sldLayoutRe = regexp.MustCompile(`<p:sldLayout\b`)

var defRPrBlockRe = regexp.MustCompile(`(?s)<a:defRPr[^>]*>.*?</a:defRPr>`)

func dedupeFills(x, fill string) string {
	return defRPrBlockRe.ReplaceAllStringFunc(x, func(s string) string {
		if strings.Count(s, "<a:solidFill>") < 2 {
			return s
		}
		s = solidFillRe.ReplaceAllString(s, "")
		i := strings.Index(s, ">")
		return s[:i+1] + fill + s[i+1:]
	})
}

// placeholderXfrm moves the placeholder carrying attrs. A layout holds no
// group shapes, so each <p:sp> is flat.
func placeholderXfrm(x, attrs string, xmm, ymm, wmm, hmm float64) string {
	off := regexp.MustCompile(`<a:off x="\d+" y="\d+"/><a:ext cx="\d+" cy="\d+"/>`)
	return shapeRe.ReplaceAllStringFunc(x, func(sp string) string {
		if !strings.Contains(sp, "<p:ph "+attrs) {
			return sp
		}
		return off.ReplaceAllString(sp, fmt.Sprintf(`<a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/>`, emu(xmm), emu(ymm), emu(wmm), emu(hmm)))
	})
}

var shapeRe = regexp.MustCompile(`(?s)<p:sp>.*?</p:sp>`)

// placeholderEdit rewrites the placeholder carrying attrs.
func placeholderEdit(x, attrs string, edit func(string) string) string {
	return shapeRe.ReplaceAllStringFunc(x, func(sp string) string {
		if !strings.Contains(sp, "<p:ph "+attrs) {
			return sp
		}
		return edit(sp)
	})
}

var defRPrSzRe = regexp.MustCompile(`(<a:defRPr[^>]*?) sz="\d+"`)

var lvl1DefRPrRe = regexp.MustCompile(`<a:defRPr\b`)

// setRPr gives a placeholder's first level a size, and bold when asked. Its
// list style may be empty, or hold a size of its own, which goes.
func setRPr(sp string, pt float64, bold bool) string {
	sp = strings.Replace(sp, "<a:lstStyle/>", "<a:lstStyle><a:lvl1pPr><a:defRPr/></a:lvl1pPr></a:lstStyle>", 1)
	attrs := fmt.Sprintf(` sz="%d"`, hundredths(pt))
	if bold {
		attrs += ` b="1"`
	}
	i := lvl1DefRPrRe.FindStringIndex(sp)
	if i == nil {
		return sp
	}
	head, tail := sp[:i[1]], sp[i[1]:]
	end := strings.IndexByte(tail, '>')
	tag := regexp.MustCompile(` (?:sz|b)="[^"]*"`).ReplaceAllString(tail[:end], "")
	return head + attrs + tag + tail[end:]
}

// titleSlide lays the title slide out as the PDF's: the logo at the top left,
// the title large and bold at the left below it, the subtitle and byline
// under the title.
func titleSlide(x string, l Look) string {
	x = strings.ReplaceAll(x, `algn="ctr"`, `algn="l"`)
	w := SlideWMM - 2*l.MarginMM - 40
	x = placeholderXfrm(x, `type="ctrTitle"`, l.MarginMM, 48, w, 26)
	x = placeholderXfrm(x, `type="subTitle" idx="1"`, l.MarginMM, 76, w, 34)
	x = placeholderEdit(x, `type="ctrTitle"`, func(sp string) string { return setRPr(sp, 24*Scale, true) })
	return placeholderEdit(x, `type="subTitle" idx="1"`, func(sp string) string { return setRPr(sp, 13*Scale, false) })
}

func addRel(p *pkg, part, id, target string) {
	x := p.get(part)
	if x == "" || strings.Contains(x, `Id="`+id+`"`) {
		return
	}
	p.set(part, strings.Replace(x, "</Relationships>",
		`<Relationship Id="`+id+`" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="`+target+`"/></Relationships>`, 1))
}

func addPNGType(p *pkg) {
	const name = "[Content_Types].xml"
	ct := p.get(name)
	if strings.Contains(ct, `Extension="png"`) {
		return
	}
	p.set(name, strings.Replace(ct, "<Default ", `<Default Extension="png" ContentType="image/png"/><Default `, 1))
}
