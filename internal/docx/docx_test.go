package docx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func cell(text string, mono bool) string {
	style := ""
	if mono {
		style = `<w:rPr><w:rStyle w:val="VerbatimChar" /></w:rPr>`
	}
	return `<w:tc><w:tcPr /><w:p><w:pPr><w:pStyle w:val="Compact" /></w:pPr><w:r>` + style +
		`<w:t xml:space="preserve">` + text + `</w:t></w:r></w:p></w:tc>`
}

func row(cells ...string) string { return "<w:tr>" + strings.Join(cells, "") + "</w:tr>" }

// The defect that started this: pandoc shares a table's width by its text, and
// Word then breaks "m5.xlarge" into "m5.xla" and "rge".
func TestColumnsNeverNarrowerThanTheirLongestWord(t *testing.T) {
	long := "Servicios de aplicación con tráfico moderado y bases de datos pequeñas y medianas"
	rows := []string{
		row(cell("Tipo", false), cell("vCPU", false), cell("Uso recomendado", false)),
		row(cell("m5.xlarge", false), cell("4", false), cell(long, false)),
	}
	measure := twips(100)
	widths, need := columnWidths(rows, measure, 0.6)
	total := 0
	for _, w := range widths {
		total += w
	}
	if total > measure {
		t.Errorf("total %d twips is past the %d measure", total, measure)
	}
	words := map[int]string{0: "m5.xlarge", 1: "vCPU", 2: "aplicación"}
	for i, word := range words {
		min := setWidth(lineOf(word, false), 0.6) + cellPadTwips
		if widths[i] < min {
			t.Errorf("column %d is %d twips, narrower than %q at %d", i, widths[i], word, min)
		}
	}
	if need > measure {
		t.Errorf("need = %d, but the longest words fit the %d measure", need, measure)
	}
}

// Natural widths are kept when the table is narrower than the measure: the PDF
// sets such a table at its own width, centred, not stretched.
func TestColumnsKeepTheirNaturalWidth(t *testing.T) {
	rows := []string{row(cell("sede", false), cell("coste", false))}
	widths, _ := columnWidths(rows, twips(160), 0.6)
	for i, w := range widths {
		if w > twips(30) {
			t.Errorf("column %d stretched to %d twips", i, w)
		}
	}
}

// Code in a cell sets in the mono face, which is wider than prose: measured as
// prose, `report` broke in Google Docs.
func TestCodeInACellIsMeasuredInTheCodeFace(t *testing.T) {
	prose, _ := columnWidths([]string{row(cell("report", false))}, twips(160), 0.6)
	code, _ := columnWidths([]string{row(cell("report", true))}, twips(160), 0.6)
	if code[0] <= prose[0] {
		t.Errorf("code %d twips, prose %d: code must measure wider", code[0], prose[0])
	}
}

func lineOf(s string, mono bool) []glyph {
	var g []glyph
	for _, r := range s {
		g = append(g, glyph{r, mono})
	}
	return g
}

// The PDF's rule, tex.TableKeep: no break may leave fewer than keep body rows
// on either side, and the header always stays with the first row.
func TestKeepNextFollowsTheTableKeepRule(t *testing.T) {
	header := `<w:tr><w:trPr><w:tblHeader w:val="on" /></w:trPr>` + cell("h", false) + `</w:tr>`
	tbl := "<w:tbl>" + header
	for i := 0; i < 8; i++ {
		tbl += row(cell(fmt.Sprint(i), false))
	}
	tbl += "</w:tbl>"
	rows := rowRe.FindAllString(ruleAndKeep(tbl, 3), -1)
	// header, body 0 and 1 glue forward; body 5 and 6 glue to the last; body 7
	// is last and glues to nothing.
	want := []bool{true, true, true, false, false, false, true, true, false}
	for i, r := range rows {
		if got := strings.Contains(r, "<w:keepNext/>"); got != want[i] {
			t.Errorf("row %d keepNext = %v, want %v", i, got, want[i])
		}
		if !strings.Contains(r, "<w:cantSplit/>") {
			t.Errorf("row %d may split across pages", i)
		}
		if bad := misordered(r); bad != "" {
			t.Errorf("row %d out of schema order: %s", i, bad)
		}
	}
}

// Word refuses property elements out of the schema's order; nothing else says
// so. setChildren must write them in order, and misordered must catch them.
func TestPropertiesInSchemaOrder(t *testing.T) {
	inner := `<w:jc w:val="left"/><w:pStyle w:val="Heading1" /><w:outlineLvl w:val="0"/><w:keepNext/>`
	got := setChildren(inner, []elem{{"spacing", `<w:spacing w:before="0"/>`}, {"keepNext", "<w:keepNext/>"}}, pPrOrder)
	want := `<w:pStyle w:val="Heading1" /><w:keepNext/><w:spacing w:before="0"/><w:jc w:val="left"/><w:outlineLvl w:val="0"/>`
	if got != want {
		t.Errorf("setChildren:\n got %s\nwant %s", got, want)
	}
	for _, c := range []struct {
		xml string
		bad bool
	}{
		{`<w:pPr><w:jc w:val="center"/><w:spacing w:before="0"/></w:pPr>`, true},
		{`<w:rPr><w:b/><w:b/></w:rPr>`, true},
		{`<w:rPr><w:sz w:val="18"/><w:b/></w:rPr>`, true},
		{`<w:pPr><w:pStyle w:val="x" /><w:keepNext/><w:spacing/><w:jc w:val="left"/><w:rPr><w:b/><w:sz w:val="2"/></w:rPr></w:pPr>`, false},
	} {
		if got := misordered(c.xml) != ""; got != c.bad {
			t.Errorf("misordered(%s) = %v, want %v", c.xml, got, c.bad)
		}
	}
}

func TestCaptionsAreNumberedByKind(t *testing.T) {
	doc := `<w:p><w:pPr><w:pStyle w:val="ImageCaption" /></w:pPr><w:r><w:t>a</w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:pStyle w:val="TableCaption" /></w:pPr><w:r><w:t>b</w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:pStyle w:val="ImageCaption" /></w:pPr><w:r><w:t>c</w:t></w:r></w:p>`
	got := numberCaptions(doc, LabelsFor("es-ES"))
	for _, want := range []string{"Figura </w:t>", "Tabla </w:t>", "<w:t>2</w:t>"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Count(got, "SEQ Table") != 1 || strings.Count(got, "SEQ Figure") != 2 {
		t.Errorf("sequences mixed up: %s", got)
	}
}

// pandoc's title block goes; the first paragraph of the document stays.
func TestTitleBlockIsDropped(t *testing.T) {
	doc := `<w:document><w:body><w:p><w:pPr><w:pStyle w:val="Title" /></w:pPr><w:r><w:t>T</w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:pStyle w:val="Author" /></w:pPr><w:r><w:t>A</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>cover</w:t></w:r></w:p><w:p><w:pPr><w:pStyle w:val="Title" /></w:pPr></w:p></w:body></w:document>`
	got := dropTitleBlock(doc)
	if strings.Contains(got, ">T<") || strings.Contains(got, ">A<") || !strings.Contains(got, ">cover<") {
		t.Errorf("dropTitleBlock: %s", got)
	}
	if strings.Count(got, `w:val="Title"`) != 1 {
		t.Errorf("a Title after the body's start was removed too: %s", got)
	}
}

// A face nobody declared — pandoc's Consolas, the theme's Aptos — is what
// Word and Google Docs substitute in silence.
func TestStrayFontsAreFound(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"word/styles.xml":       `<w:rFonts w:ascii="Inter" w:hAnsi="Consolas"/>`,
		"word/theme/theme1.xml": `<a:majorFont><a:latin typeface="Aptos Display"/></a:majorFont>`,
		"word/document.xml":     `<w:rFonts w:ascii="Cambria Math"/>`,
	} {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	p, err := readPkg(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(strayFonts(p, Look{Body: "Inter", Display: "Montserrat", Mono: "Roboto Mono"}), ",")
	if got != "Aptos Display,Consolas" {
		t.Errorf("stray = %q", got)
	}
}

// The prefilled contents link each entry to its heading's bookmark, and keep
// a title with a % in it as written.
func TestTOCIsFilledFromTheHeadings(t *testing.T) {
	field := tocField(Meta{TOC: true, TOCDepth: 2}, Look{})
	doc := strings.TrimSuffix(strings.TrimPrefix(raw(field), "```{=openxml}\n"), "\n```\n\n") +
		`<w:p><w:pPr><w:pStyle w:val="Heading1" /></w:pPr><w:bookmarkStart w:id="1" w:name="growth" /><w:r><w:t xml:space="preserve">Growth 10%</w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:pStyle w:val="Heading3" /></w:pPr><w:r><w:t>too deep</w:t></w:r></w:p>`
	got := fillTOC(doc)
	if !regexp.MustCompile(`TOC1"/></w:pPr>.*w:anchor="growth".*Growth 10%`).MatchString(got) {
		t.Errorf("entry missing: %s", got)
	}
	if strings.Count(got, "TOC1") != 1 || strings.Contains(got, `TOC3"`) {
		t.Errorf("depth not honoured: %s", got)
	}
}

// pandoc 3.1 writes <w:bCs/> before <w:b/>; normalize puts it right, keeps the
// last of a repeat, and leaves a block it cannot read in full alone.
func TestNormalizeFixesPandocsOrder(t *testing.T) {
	got := normalize(`<w:r><w:rPr><w:bCs /><w:b /><w:sz w:val="18"/><w:sz w:val="20"/></w:rPr></w:r>`)
	if want := `<w:r><w:rPr><w:b /><w:bCs /><w:sz w:val="20"/></w:rPr></w:r>`; got != want {
		t.Errorf("normalize:\n got %s\nwant %s", got, want)
	}
	foreign := `<w:rPr><w:sz w:val="18"/><w14:ligatures w14:val="all"/><w:b/></w:rPr>`
	if got := normalize(foreign); got != foreign {
		t.Errorf("a block with a w14 element was rewritten: %s", got)
	}
}
