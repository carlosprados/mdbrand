package epub

import (
	"archive/zip"
	"bytes"
	"math"
	"strings"
	"testing"
	"time"
)

// The pairs the design measured: Amplía's grey text and blue link are one
// bar on e-ink, the default bundle's series are not.
func TestGreyClash(t *testing.T) {
	if l := Lightness("FFFFFF"); math.Abs(l-100) > 0.01 {
		t.Errorf("white L* = %.2f", l)
	}
	if l := Lightness("000000"); l != 0 {
		t.Errorf("black L* = %.2f", l)
	}
	c, ok := GreyClash([]string{"F68E1B", "5D6266", "1565C0"})
	if !ok || c.A != "5D6266" || c.B != "1565C0" || math.Abs(math.Abs(c.LA-c.LB)-1.9) > 0.2 {
		t.Errorf("amplia: %+v %v; want text and link 1.9 L* apart", c, ok)
	}
	if c, ok := GreyClash([]string{"1F6FEB", "3A3A3A", "C8CCCE"}); ok {
		t.Errorf("default bundle clashes: %+v", c)
	}
}

func TestMetadata(t *testing.T) {
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	for date, want := range map[string]string{
		// pandoc wrote an empty <dc:date> for this, and epubcheck refused it.
		"14 de octubre de 2026": "2026-10-14",
		"":                      "2026-10-14",
		"2026-09":               "2026-09",
		"2025-03-01":            "2025-03-01",
	} {
		m := Metadata(Meta{Input: "/a/doc.md", Date: date, Now: now})
		if !strings.Contains(m, "<dc:date>"+want+"</dc:date>") {
			t.Errorf("%q: %s", date, m)
		}
	}
	// A rebuild is the same book to the reader, so its place is kept.
	a, b := Metadata(Meta{Input: "/a/doc.md", Now: now}), Metadata(Meta{Input: "/a/doc.md", Now: now.AddDate(0, 0, 1)})
	if strings.Split(a, "\n")[0] != strings.Split(b, "\n")[0] {
		t.Errorf("the identifier moved between builds:\n%s\n%s", a, b)
	}
	if strings.Split(a, "\n")[0] == strings.Split(Metadata(Meta{Input: "/b/doc.md", Now: now}), "\n")[0] {
		t.Error("two documents share an identifier")
	}
}

func TestProblems(t *testing.T) {
	out := `ERROR(RSC-005): bad.epub/EPUB/content.opf(6,39): Error while parsing file: character content of element "dc:date" invalid
WARNING(OPF-053): bad.epub/EPUB/content.opf(6,39): Date value "" does not follow recommended syntax

Check finished with errors
`
	ps := Problems(out)
	if len(ps) != 2 || ps[0].Severity != "ERROR" || ps[0].Code != "RSC-005" ||
		!strings.Contains(ps[0].Text, "dc:date") || ps[1].Severity != "WARNING" {
		t.Errorf("%+v", ps)
	}
}

func TestHasMath(t *testing.T) {
	pkg := func(page string) []byte {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, _ := zw.Create("EPUB/text/ch001.xhtml")
		w.Write([]byte(page))
		zw.Close()
		return buf.Bytes()
	}
	if m, err := HasMath(pkg(`<p><math display="inline">x</math></p>`)); err != nil || !m {
		t.Errorf("math missed: %v %v", m, err)
	}
	if m, _ := HasMath(pkg(`<p>$x$ in prose</p>`)); m {
		t.Error("no math reported as math")
	}
}

// pandoc declares MathML on chapters, not on the nav a heading's formula
// lands in, and epubcheck refused the package (OPF-014).
func TestRepairDeclaresMathML(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range []struct{ name, body string }{
		{"mimetype", "application/epub+zip"},
		{"EPUB/content.opf", `<manifest>
<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav" />
<item id="ch001_xhtml" href="text/ch001.xhtml" media-type="application/xhtml+xml" properties="mathml" />
<item id="ch002_xhtml" href="text/ch002.xhtml" media-type="application/xhtml+xml" />
</manifest>`},
		{"EPUB/nav.xhtml", `<li>Heading <math>x</math></li>`},
		{"EPUB/text/ch001.xhtml", `<math>y</math>`},
		{"EPUB/text/ch002.xhtml", `<p>prose</p>`},
	} {
		w, _ := zw.Create(f.name)
		w.Write([]byte(f.body))
	}
	zw.Close()
	out, err := Repair(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		t.Fatal(err)
	}
	if first := zr.File[0]; first.Name != "mimetype" || first.Method != zip.Store || len(first.Extra) > 0 {
		t.Errorf("mimetype is %s, method %d, %d bytes of extra: a reader would not take it for an EPUB",
			first.Name, first.Method, len(first.Extra))
	}
	opf, _ := readZip(zr.File[1])
	for _, want := range []string{`href="nav.xhtml" media-type="application/xhtml+xml" properties="nav mathml"`,
		`href="text/ch001.xhtml" media-type="application/xhtml+xml" properties="mathml" />`,
		`href="text/ch002.xhtml" media-type="application/xhtml+xml" />`} {
		if !strings.Contains(string(opf), want) {
			t.Errorf("no %s in\n%s", want, opf)
		}
	}
}
