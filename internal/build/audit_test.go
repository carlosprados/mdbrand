package build

import (
	"strings"
	"testing"
)

// The page limit counts up to the heading, not up to the first sentence that
// mentions it: "see the references below" on page 2 would cut a 10-page body
// to 2 and pass a document far over its limit.
func TestHeadingPageFindsTheHeadingNotAMention(t *testing.T) {
	pages := []string{
		"1 Introduction\nWe list the references at the end.\n",
		"2 Method\nReferences are numbered.\n",
		"7 Conclusion\nDone.\n8 References\n[1] Someone.\n",
	}
	if p := headingPage(pages, "References"); p != 3 {
		t.Errorf("headingPage = %d, want 3", p)
	}
	if p := headingPage([]string{"A.1 Appendix\n", "REFERENCES\n"}, "references"); p != 2 {
		t.Errorf("an unnumbered heading in capitals: page %d, want 2", p)
	}
	if p := headingPage(pages, "Bibliography"); p != 0 {
		t.Errorf("a heading that is not there was found on page %d", p)
	}
}

// A PDF rounds A4's 595.28 × 841.89 points, and a landscape page is the same
// paper turned.
func TestPaperNameToleratesRoundingAndLandscape(t *testing.T) {
	for _, c := range []struct {
		w, h float64
		want string
	}{
		{595, 842, "a4"}, {596, 842, "a4"}, {842, 595.28, "a4"},
		{612, 792, "letter"}, {420, 595, "420 × 595 pt"},
	} {
		if got := paperName(c.w, c.h); got != c.want {
			t.Errorf("paperName(%v, %v) = %q, want %q", c.w, c.h, got, c.want)
		}
	}
	if pages, paper := pdfPages("Pages:           10\nPage size:      612 x 792 pts (letter)\n"); pages != 10 || paper != "letter" {
		t.Errorf("pdfPages = %d, %q", pages, paper)
	}
}

// pdffonts' type column holds one to three words ("Type 1", "CID Type 0C",
// "TrueType"), so emb is read from the right; read from the left, a CID face
// not embedded passed as embedded.
func TestJudgeFontsReadsEmbeddedFromTheRight(t *testing.T) {
	list := `name                                 type              encoding         emb sub uni object ID
------------------------------------ ----------------- ---------------- --- --- --- ---------
ABCDEF+NimbusRomNo9L-Regu            Type 1            Custom           yes yes yes     40  0
Helvetica                            Type 1            Custom           no  no  no      41  0
ArialMT                              CID TrueType      Identity-H       no  no  yes     42  0
F52                                  Type 3            Custom           yes no  no      43  0
`
	a := &Audit{}
	a.judgeFonts(list)
	if len(a.Defects) != 1 || !strings.Contains(a.Defects[0], "Helvetica, ArialMT") {
		t.Errorf("defects = %q, want Helvetica and ArialMT not embedded", a.Defects)
	}
	if len(a.Warnings) != 1 || !strings.Contains(a.Warnings[0], "F52") {
		t.Errorf("warnings = %q, want F52 as Type 3", a.Warnings)
	}
}

// mutool writes &#xffff; for a glyph it cannot map, and the decoder stopped
// there: the small text after it went unread, with no error.
func TestJudgeTextSizesReadsPastAnUnmappedGlyph(t *testing.T) {
	stext := `<document><page id="page1">
<font name="A" size="9.96"><char c="&#xffff;"/><char c="x"/></font>
<font name="A" size="3.98"><char c="A"/><char c="x"/><char c="i"/><char c="s"/></font>
<font name="A" size="4.5"><char c="2"/></font>
</page></document>`
	a := &Audit{}
	a.judgeTextSizes([]byte(stext))
	if len(a.Warnings) != 1 || !strings.Contains(a.Warnings[0], `page 1: 4.0pt "Axis"`) {
		t.Errorf("warnings = %q, want the 4pt run named and the lone digit left out", a.Warnings)
	}
}

func TestTitleIsFileName(t *testing.T) {
	for title, want := range map[string]bool{
		"report": true, "report.pdf": true, "main.tex": true, "Untitled": true,
		"Microsoft Word - informe.docx": true, "Predicting Hospital Readmission": false,
	} {
		if got := titleIsFileName(title, "report.pdf"); got != want {
			t.Errorf("titleIsFileName(%q) = %v, want %v", title, got, want)
		}
	}
}

// A heading at the top of a page ends the body on the page before: counted
// by its page, a 10-page body with References atop page 11 was 11 pages.
func TestBodyLengthMeasuresToTheHeadingsLine(t *testing.T) {
	page := func(lines ...layoutLine) []layoutLine { return lines }
	full := page(layoutLine{72, 82, "Text"}, layoutLine{760, 770, "more"})
	pages := [][]layoutLine{full, full,
		page(layoutLine{72, 86, "3 References"}, layoutLine{100, 110, "[1] A."})}
	if got, ok := bodyLength(pages, "References"); !ok || got != 2 {
		t.Errorf("bodyLength = %v, %v; want 2, true", got, ok)
	}
	pages[2] = page(layoutLine{72, 82, "Text"}, layoutLine{421, 435, "References"})
	if got, _ := bodyLength(pages, "References"); got < 2.49 || got > 2.51 {
		t.Errorf("a heading halfway down page 3: bodyLength = %v, want 2.5", got)
	}
}
