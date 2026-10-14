package pptx

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"strings"
	"testing"
)

// zipOf builds a package from parts, in the order given.
func zipOf(t *testing.T, parts ...string) []byte {
	t.Helper()
	p := &pkg{parts: map[string][]byte{}}
	for i := 0; i < len(parts); i += 2 {
		p.set(parts[i], parts[i+1])
	}
	b, err := p.bytes()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func read(t *testing.T, b []byte) *pkg {
	t.Helper()
	p, err := readPkg(b)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// png is a PNG header saying w×h, all a size reader needs.
func png(w, h int) []byte {
	b := make([]byte, 24)
	copy(b, "\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR")
	binary.BigEndian.PutUint32(b[16:], uint32(w))
	binary.BigEndian.PutUint32(b[20:], uint32(h))
	return b
}

var look = Look{
	Body: "MdbrandTestBody", Display: "MdbrandTestDisplay", Mono: "MdbrandTestMono",
	Primary: "E8710A", Text: "3A3F44", Rule: "C0C0C0", Link: "1565C0", Accent: "A8520A",
	Ground: "1F2328", OnGround: "F2F2F2", MarginMM: 10 * Scale,
	Logo: png(400, 100), LogoAspect: 0.25, LogoWMM: 12 * Scale,
	Confidential: "Internal", RunningTitle: "Decks",
}

const theme1 = `<a:theme><a:clrScheme><a:dk1><a:sysClr val="windowText"/></a:dk1><a:dk2><a:srgbClr val="44546A"/></a:dk2>` +
	`<a:accent1><a:srgbClr val="4472C4"/></a:accent1><a:hlink><a:srgbClr val="0563C1"/></a:hlink></a:clrScheme>` +
	`<a:fontScheme><a:majorFont><a:latin typeface="Calibri Light"/><a:font script="Jpan" typeface="Yu Gothic"/></a:majorFont>` +
	`<a:minorFont><a:latin typeface="Calibri"/></a:minorFont></a:fontScheme></a:theme>`

const master = `<p:sldMaster><p:cSld><p:spTree><p:grpSpPr/></p:spTree></p:cSld><p:txStyles>` +
	`<p:titleStyle><a:lvl1pPr algn="ctr"><a:defRPr sz="3300"/></a:lvl1pPr></p:titleStyle>` +
	`<p:bodyStyle><a:lvl1pPr><a:buFont typeface="Arial"/><a:defRPr sz="2400"/></a:lvl1pPr><a:lvl2pPr><a:defRPr sz="2100"/></a:lvl2pPr></p:bodyStyle>` +
	`</p:txStyles></p:sldMaster>`

func layoutXML(name, shapes string) string {
	return `<p:sldLayout><p:cSld name="` + name + `"><p:spTree><p:grpSpPr/>` + shapes + `</p:spTree></p:cSld></p:sldLayout>`
}

const rels = `<Relationships></Relationships>`

func base(t *testing.T) []byte {
	ph := func(attrs string) string {
		return `<p:sp><p:nvSpPr><p:nvPr><p:ph ` + attrs + `/></p:nvPr></p:nvSpPr><p:spPr><a:xfrm><a:off x="1" y="1"/><a:ext cx="1" cy="1"/></a:xfrm></p:spPr>` +
			`<p:txBody><a:lstStyle/><a:p/></p:txBody></p:sp>`
	}
	return zipOf(t,
		"[Content_Types].xml", `<Types><Default Extension="xml" ContentType="application/xml"/></Types>`,
		"ppt/theme/theme1.xml", theme1,
		"ppt/theme/theme2.xml", theme1,
		"ppt/slideMasters/slideMaster1.xml", master,
		"ppt/slideMasters/_rels/slideMaster1.xml.rels", rels,
		"ppt/slideLayouts/slideLayout1.xml", layoutXML("Title Slide", ph(`type="ctrTitle"`)+ph(`type="subTitle" idx="1"`)),
		"ppt/slideLayouts/_rels/slideLayout1.xml.rels", rels,
		"ppt/slideLayouts/slideLayout3.xml", layoutXML("Section Header", ph(`type="title"`)),
		"ppt/slideLayouts/_rels/slideLayout3.xml.rels", rels,
		"ppt/slideLayouts/slideLayout4.xml", layoutXML("Two Content", `<p:sp><a:defRPr sz="2100"/></p:sp>`),
	)
}

// The brand reaches every part a deck draws with: both themes' colours and
// faces, the master's sizes and bullets, the footer, and the covers.
func TestReference(t *testing.T) {
	out, err := Reference(base(t), look)
	if err != nil {
		t.Fatal(err)
	}
	p := read(t, out)
	for _, part := range []string{"ppt/theme/theme1.xml", "ppt/theme/theme2.xml"} {
		th := p.get(part)
		for _, want := range []string{`<a:dk1><a:srgbClr val="3A3F44"/>`, `<a:accent1><a:srgbClr val="E8710A"/>`,
			`<a:majorFont><a:latin typeface="MdbrandTestDisplay"/>`, `<a:minorFont><a:latin typeface="MdbrandTestBody"/>`} {
			if !strings.Contains(th, want) {
				t.Errorf("%s lacks %s", part, want)
			}
		}
	}
	m := p.get("ppt/slideMasters/slideMaster1.xml")
	for _, want := range []string{`algn="l"`, `sz="2300" b="1"`, `sz="1800"`, `sz="1600"`, `<a:buFont typeface="MdbrandTestBody"`,
		`name="mdbrand logo"`, `type="slidenum"`, "Internal", "Decks"} {
		if !strings.Contains(m, want) {
			t.Errorf("master lacks %s", want)
		}
	}
	if strings.Contains(m, "Arial") {
		t.Error("master still names Arial")
	}
	title := p.get("ppt/slideLayouts/slideLayout1.xml")
	for _, want := range []string{`showMasterSp="0"`, `<a:srgbClr val="1F2328"/>`, `<a:srgbClr val="F2F2F2"/>`, `sz="3810" b="1"`} {
		if !strings.Contains(title, want) {
			t.Errorf("title layout lacks %s", want)
		}
	}
	if strings.Contains(p.get("ppt/slideLayouts/slideLayout4.xml"), `sz="2100"`) {
		t.Error("Two Content keeps pandoc's larger body")
	}
	if !bytes.Equal(p.parts[mediaLogo], look.Logo) || !strings.Contains(p.get("[Content_Types].xml"), `Extension="png"`) {
		t.Error("the logo is not in the reference as a PNG part")
	}
	if !strings.Contains(p.get("ppt/slideMasters/_rels/slideMaster1.xml.rels"), "../media/mdbrand-logo.png") {
		t.Error("the master does not relate the logo")
	}
}

func slideXML(title, body string) string {
	return `<p:sld><p:cSld><p:spTree><p:sp><p:nvSpPr><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr>` +
		`<p:txBody><a:p><a:r><a:t>` + title + `</a:t></a:r></a:p></p:txBody></p:sp>` + body + `</p:spTree></p:cSld></p:sld>`
}

const slideRels = `<Relationships><Relationship Id="rId2" Type="image" Target="../media/image1.png"/>` +
	`<Relationship Id="rId9" Type="layout" Target="../slideLayouts/slideLayout2.xml"/></Relationships>`

func pic(x, y, cx, cy int) string {
	return `<p:pic><p:blipFill><a:blip r:embed="rId2"/></p:blipFill><p:spPr><a:xfrm><a:off x="` +
		itoa(x) + `" y="` + itoa(y) + `"/><a:ext cx="` + itoa(cx) + `" cy="` + itoa(cy) + `"/></a:xfrm></p:spPr></p:pic>`
}

func itoa(n int) string { return strconv.Itoa(n) }

// pandoc fits a picture to its placeholder whatever width= it had: Repair
// writes it at the width its pixels were rasterised for, centred where pandoc
// put it. A 1181-pixel PNG at 300 ppi is 100 mm.
func TestRepairSizesPictures(t *testing.T) {
	deck := zipOf(t,
		"ppt/slides/slide1.xml", slideXML("A diagram", pic(emu(10), emu(30), emu(200), emu(80))),
		"ppt/slides/_rels/slide1.xml.rels", slideRels,
		"ppt/media/image1.png", string(png(1181, 472)),
	)
	out, _, err := Repair(deck, Look{Body: "B", Display: "D", Mono: "M"})
	if err != nil {
		t.Fatal(err)
	}
	x := read(t, out).get("ppt/slides/slide1.xml")
	wantW, wantX := emu(1181.0/PPI*25.4), emu(10)+emu(200)/2-emu(1181.0/PPI*25.4)/2
	if !strings.Contains(x, `<a:off x="`+itoa(wantX)+`" y="`+itoa(emu(30))+`"/><a:ext cx="`+itoa(wantW)+`"`) {
		t.Errorf("picture not resized to its pixels:\n%s", x)
	}
}

func TestRepairRefusesTwoFigures(t *testing.T) {
	deck := zipOf(t,
		"ppt/slides/slide1.xml", slideXML("Two at once", pic(1, 1, 1, 1)+pic(1, 1, 1, 1)),
		"ppt/slides/_rels/slide1.xml.rels", slideRels,
	)
	_, _, err := Repair(deck, Look{Body: "B", Display: "D", Mono: "M"})
	if err == nil || !strings.Contains(err.Error(), `"Two at once": 2 figures`) {
		t.Errorf("err = %v", err)
	}
}

// A pause is printed as text by pandoc's writer: it goes, with a warning.
func TestRepairDropsPauses(t *testing.T) {
	body := `<p:sp><p:txBody><a:p><a:r><a:t>Said first.</a:t></a:r></a:p><a:p><a:r><a:t>. . .</a:t></a:r></a:p></p:txBody></p:sp>`
	deck := zipOf(t, "ppt/slides/slide1.xml", slideXML("Pauses", body))
	out, warns, err := Repair(deck, Look{Body: "B", Display: "D", Mono: "M"})
	if err != nil {
		t.Fatal(err)
	}
	if x := read(t, out).get("ppt/slides/slide1.xml"); strings.Contains(x, ". . .") || !strings.Contains(x, "Said first.") {
		t.Errorf("pause not dropped:\n%s", x)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], `"Pauses"`) {
		t.Errorf("warns = %v", warns)
	}
}

// The accent marks from spans.lua become runs in the accent colour, across a
// run boundary too; a mark never reaches the deck.
func TestRepairAccents(t *testing.T) {
	body := `<p:sp><p:txBody><a:p><a:r><a:rPr lang="en"/><a:t>a ` + AccentOpen + `red</a:t></a:r>` +
		`<a:r><a:rPr b="1"><a:latin typeface="B"/></a:rPr><a:t> bold` + AccentClose + ` z</a:t></a:r></a:p></p:txBody></p:sp>`
	deck := zipOf(t, "ppt/slides/slide1.xml", slideXML("Colour", body))
	out, _, err := Repair(deck, Look{Body: "B", Display: "D", Mono: "M", Accent: "A8520A"})
	if err != nil {
		t.Fatal(err)
	}
	x := read(t, out).get("ppt/slides/slide1.xml")
	fill := `<a:solidFill><a:srgbClr val="A8520A"/></a:solidFill>`
	for _, want := range []string{
		`<a:r><a:rPr lang="en"/><a:t>a </a:t></a:r>`,
		`<a:r><a:rPr lang="en">` + fill + `</a:rPr><a:t>red</a:t></a:r>`,
		`<a:r><a:rPr b="1">` + fill + `<a:latin typeface="B"/></a:rPr><a:t> bold</a:t></a:r>`,
		`<a:r><a:rPr b="1"><a:latin typeface="B"/></a:rPr><a:t> z</a:t></a:r>`,
	} {
		if !strings.Contains(x, want) {
			t.Errorf("missing %s\nin %s", want, x)
		}
	}
	if strings.ContainsAny(x, AccentOpen+AccentClose) {
		t.Error("an accent mark reached the deck")
	}
}

// A face the bundle did not name stops the build: pandoc's own bullets were
// Arial, and a code run left without monofont is Courier.
func TestRepairRefusesStrayFonts(t *testing.T) {
	body := `<p:sp><p:txBody><a:p><a:pPr><a:buFont typeface="Arial"/></a:pPr><a:r><a:rPr><a:latin typeface="Courier"/></a:rPr><a:t>x</a:t></a:r></a:p></p:txBody></p:sp>`
	deck := zipOf(t, "ppt/slides/slide1.xml", slideXML("Code", body))
	_, _, err := Repair(deck, Look{Body: "B", Display: "D", Mono: "M"})
	if err == nil || !strings.Contains(err.Error(), "Courier") || strings.Contains(err.Error(), "Arial") {
		t.Errorf("err = %v; want Courier refused and Arial rewritten to the body face", err)
	}
}

func TestRuleTables(t *testing.T) {
	cell := `<a:tc><a:txBody><a:p/></a:txBody></a:tc>`
	tbl := `<a:tbl><a:tblPr firstRow="1" bandRow="1"><a:tableStyleId>{5C22544A}</a:tableStyleId></a:tblPr>` +
		`<a:tr h="0"><a:tc><a:txBody><a:p/></a:txBody><a:tcPr /></a:tc></a:tr><a:tr h="0">` + cell + `</a:tr><a:tr h="0">` + cell + `</a:tr></a:tbl>`
	x := ruleTables(tbl, "333333")
	if strings.Contains(x, "bandRow") || !strings.Contains(x, "{2D5ABB26-0587-4C30-8999-92F81FD0307C}") {
		t.Errorf("banded style kept:\n%s", x)
	}
	if strings.Count(x, "<a:lnT") != 1 || strings.Count(x, "<a:lnB") != 2 {
		t.Errorf("want a rule over and under the header and one at the foot:\n%s", x)
	}
}

// LibreOffice draws text past its box and the page clips it: a body slide is
// reported when a word sits below the footer's rule or never reaches the
// page; the footer's own words and a cover are not.
func TestOverflow(t *testing.T) {
	layout := func(name string) string { return layoutXML(name, "") }
	rel := func(n string) string {
		return `<Relationships><Relationship Id="rId1" Type="layout" Target="../slideLayouts/slideLayout` + n + `.xml"/></Relationships>`
	}
	text := func(s string) string {
		return `<p:sp><p:txBody><a:p><a:r><a:t>` + s + `</a:t></a:r></a:p></p:txBody></p:sp>`
	}
	deck := zipOf(t,
		"ppt/slideLayouts/slideLayout1.xml", layout("Title Slide"),
		"ppt/slideLayouts/slideLayout2.xml", layout("Title and Content"),
		"ppt/slides/slide1.xml", slideXML("Cover", text("byline")),
		"ppt/slides/_rels/slide1.xml.rels", rel("1"),
		"ppt/slides/slide2.xml", slideXML("Fits", text("short")),
		"ppt/slides/_rels/slide2.xml.rels", rel("2"),
		"ppt/slides/slide3.xml", slideXML("Lost", text("seen gone")),
		"ppt/slides/_rels/slide3.xml.rels", rel("2"),
		"ppt/slides/slide4.xml", slideXML("Low", text("deep")),
		"ppt/slides/_rels/slide4.xml.rels", rel("2"),
	)
	word := func(y float64, s string) string {
		return `<word xMin="1" yMin="1" xMax="2" yMax="` + strconv.FormatFloat(y, 'f', 1, 64) + `">` + s + `</word>`
	}
	page := func(words ...string) string {
		return `<page width="720" height="405">` + strings.Join(words, "") + `</page>`
	}
	bbox := page(word(50, "Cover"), word(400, "byline")) +
		page(word(30, "Fits"), word(100, "short"), word(380, "Internal"), word(380, "Decks"), word(380, "2")) +
		page(word(30, "Lost"), word(100, "seen")) +
		page(word(30, "Low"), word(390, "deep"))
	over, err := Overflow(deck, bbox, look)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(over, "\n")
	if len(over) != 2 || !strings.Contains(got, `slide 3 "Lost": 1 word(s)`) || !strings.Contains(got, `slide 4 "Low": text runs over the footer`) {
		t.Errorf("over = %q", over)
	}
}
