package build

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/doc"
)

// Regression: a machine-wide default must never outrank the document. The bug
// this pins made `mdbrand build examples/demo.md` — a document declaring
// `brand: none` — build with the reader's configured brand instead, which then
// failed on a font the document has nothing to do with.
func TestPickPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name           string
		flag, doc, def string
		want           string
	}{
		{"flag wins over everything", "flag", "doc", "cfg", "flag"},
		{"document wins over the configured default", "", "doc", "cfg", "doc"},
		{"default only when nothing else says", "", "", "cfg", "cfg"},
		{"nothing at all", "", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Pick(tc.flag, tc.doc, tc.def); got != tc.want {
				t.Errorf("Pick(%q,%q,%q) = %q, want %q", tc.flag, tc.doc, tc.def, got, tc.want)
			}
		})
	}
	if got := Pick("", "", "", "report"); got != "report" {
		t.Errorf("built-in fallback = %q", got)
	}
}

// Regression: prepareLogoFile used to return a hardcoded "logo.pdf" whatever
// stem it was given, so declaring a second logo produced a cover with the first
// mark printed twice. Nothing failed — the build succeeded and the PDF was
// simply wrong, which is the only kind of bug worth a test here.
func TestPrepareLogoFileKeepsItsStem(t *testing.T) {
	work := t.TempDir()
	for _, tc := range []struct{ ext, stem, want string }{
		{".pdf", "logo", "logo.pdf"},
		{".pdf", "logo-secondary", "logo-secondary.pdf"},
		{".png", "logo-secondary", "logo-secondary.png"},
	} {
		src := filepath.Join(work, "src"+tc.stem+tc.ext)
		if err := os.WriteFile(src, []byte("not really artwork, but long enough to copy"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := prepareLogoFile(&brand.Brand{Name: "t"}, work, src, tc.stem)
		if err != nil {
			t.Fatalf("%s/%s: %v", tc.stem, tc.ext, err)
		}
		if got != tc.want {
			t.Errorf("prepareLogoFile(stem=%q, %s) = %q, want %q", tc.stem, tc.ext, got, tc.want)
		}
		if _, err := os.Stat(filepath.Join(work, tc.want)); err != nil {
			t.Errorf("%s was not written: %v", tc.want, err)
		}
	}
}

// Regression: pandoc reports an unresolved citation key as a warning and exits
// 0, so the PDF ships with "(fml?)" printed mid-sentence and nothing fails.
// This is exactly the class of silent defect the tool exists to stop.
func TestMissingCitations(t *testing.T) {
	out := `[WARNING] Citeproc: citation fml not found
[WARNING] Citeproc: citation miller2020 not found
[WARNING] Citeproc: citation fml not found
[WARNING] Missing character: There is no X in font Y!`
	got := missingCitations(out)
	want := []string{"fml", "miller2020"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if n := missingCitations("[WARNING] Deprecated: --smart"); len(n) != 0 {
		t.Errorf("unrelated warnings must not match: %v", n)
	}
}

// With nothing to cite from, pandoc prints [@key] as written and exits 0. The
// filter's report is what tells; a document with references in its front
// matter has a source, and one that cites nothing has nothing to stop.
func TestCitationProblemsWithoutASource(t *testing.T) {
	out := "MDBRAND-CITE knuth1984\nMDBRAND-CITE fig:arq\nMDBRAND-CITE knuth1984\n"
	err := citationProblems(doc.Meta{}, out, "PDF")
	if err == nil {
		t.Fatal("citations with no bibliography passed")
	}
	for _, want := range []string{"2 citation(s)", "@knuth1984, @fig:arq", "bibliography:", "@fig:arq read as pandoc-crossref"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message lacks %q:\n%s", want, err)
		}
	}
	if err := citationProblems(doc.Meta{References: []any{map[string]any{"id": "knuth1984"}}}, out, "PDF"); err != nil {
		t.Errorf("references in the front matter are a source: %v", err)
	}
	if err := citationProblems(doc.Meta{}, "[WARNING] something else\n", "PDF"); err != nil {
		t.Errorf("a document that cites nothing must pass: %v", err)
	}
}

// A header that overflows its box prints the logo across the first line of every
// page, and fancyhdr reports it only in the log. The parse has to be exact: the
// warning is repeated per page and the largest shortfall is the real one.
func TestScanLogFindsHeaderOverflow(t *testing.T) {
	log := `[1] Package fancyhdr Warning: \headheight is too small (14.5pt too short).
[2] Package fancyhdr Warning: \headheight is too small (23.17pt too short).
Output written on doc.pdf (2 pages, 12345 bytes).`
	sc := scanLog(log)
	if sc.Pages != 2 {
		t.Errorf("pages = %d, want 2", sc.Pages)
	}
	if sc.HeadShortPt != 23.17 {
		t.Errorf("headShortPt = %v, want 23.17 (the largest)", sc.HeadShortPt)
	}

	// fancyhdr 4.x, as a real log prints it: the shortfall is the difference.
	modern := "Package fancyhdr Warning: \\headheight is too small (22.0pt): \n" +
		"(fancyhdr)                Make it at least 186.09987pt, for example:\n" +
		"(fancyhdr)                \\setlength{\\headheight}{186.09987pt}.\n"
	if short := scanLog(modern).HeadShortPt; math.Abs(short-164.09987) > 1e-6 {
		t.Errorf("fancyhdr 4.x: headShortPt = %v, want 164.09987", short)
	}

	if short := scanLog("Output written on doc.pdf (1 page, 10 bytes).").HeadShortPt; short != 0 {
		t.Errorf("a clean log reported %v pt short", short)
	}
}

// The footer's confidentiality label and the page number are set by fancyhdr
// without regard for each other; the preamble's measurement is the only notice.
func TestScanLogFindsConfidentialTooWide(t *testing.T) {
	if over := scanLog("MDBRAND-CONFIDENTIAL-TOOWIDE over=37.81277pt\n").ConfOverPt; over != 37.81277 {
		t.Errorf("confOverPt = %v, want 37.81277", over)
	}
	if over := scanLog("Output written on doc.pdf (1 page, 10 bytes).").ConfOverPt; over != 0 {
		t.Errorf("a clean log reported the label %v pt too wide", over)
	}
	// The running title's marker is its own: one must not read as the other.
	sc := scanLog("MDBRAND-TITLE-TOOWIDE over=12.5pt\n")
	if sc.TitleOverPt != 12.5 || sc.ConfOverPt != 0 {
		t.Errorf("title over = %v, label over = %v; want 12.5 and 0", sc.TitleOverPt, sc.ConfOverPt)
	}
}

// The count alone sent the reader to the log with a grep and a map from .tex
// line numbers back to the Markdown. The quote is the diagnosis. What has to be
// right is the reassembly: the log hard-wraps at 79 columns with nothing
// inserted at the break, mid-word and mid-font-name, so stripping before
// joining would leave half a font name in the quote.
func TestScanLogQuotesOverfullLines(t *testing.T) {
	log := "Overfull \\hbox (7.1pt too wide) in paragraph at lines 9--12\n" +
		"[]\\TU/Inter(2)/m/n/10 Descargar el resto de repos con \\TU/lmtt/m/n/10 cli\\TU/In\n" +
		"ter(2)/m/n/10 .\n" +
		" []\n" +
		"\n" +
		"Overfull \\hbox (3.2pt too wide) in paragraph at lines 20--21\n" +
		"[]\\TU/Inter(2)/m/n/10 apenas se pasa\n" +
		" []\n" +
		"Output written on doc.pdf (2 pages, 12345 bytes)."

	over := scanLog(log).Over
	if len(over) != 1 {
		t.Fatalf("got %d overfull(s), want 1 — 3.2pt is below the 5pt floor: %+v", len(over), over)
	}
	if over[0].Pt != 7.1 {
		t.Errorf("Pt = %v, want 7.1", over[0].Pt)
	}
	// Joined across the wrap, font runs gone with the space that terminates
	// them, box markers gone, the text intact.
	want := "Descargar el resto de repos con cli."
	if over[0].Text != want {
		t.Errorf("Text = %q, want %q", over[0].Text, want)
	}
}

// A quote long enough to need cutting must still be readable, and must not cut
// a multi-byte character in half.
func TestOffendingTextEllipsizesOnAWordBoundary(t *testing.T) {
	log := "Overfull \\hbox (50.3pt too wide) in paragraph at lines 152--154\n" +
		"[]\\TU/Inter(2)/m/n/10 La identidad del modelo vive en el DNS y la regla de desp\n" +
		"liegue la resuelve contra \\TU/lmtt/m/n/10 trainingplan-catalog-api\\TU/Inter(2\n" +
		")/m/n/10 .\n" +
		" []\n"

	over := scanLog(log).Over
	if len(over) != 1 {
		t.Fatalf("got %d overfull(s), want 1", len(over))
	}
	got := over[0].Text
	if !strings.HasPrefix(got, "La identidad del modelo vive en el DNS") {
		t.Errorf("Text = %q, want it to start with the line as written", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("Text = %q, want a cut marked with an ellipsis", got)
	}
	if strings.ContainsAny(got, "\\[]") {
		t.Errorf("Text = %q still carries TeX plumbing", got)
	}
	if n := len([]rune(got)); n > 61 {
		t.Errorf("Text is %d runes, want it cut to 60 plus the ellipsis", n)
	}
	if !utf8.ValidString(got) {
		t.Errorf("Text = %q is not valid UTF-8 — a rune was cut in half", got)
	}
}

// A code block the preamble could not bring inside the measure reports itself
// through the log, because LaTeX is the only place that knows the mono face and
// therefore the column count. Both numbers have to survive the parse: "shorten
// it" is advice, "it is nine columns too wide" is an instruction.
func TestScanLogFindsCodeBlocksThatStayTooWide(t *testing.T) {
	log := "MDBRAND-CODE-TOOWIDE cols=101 fits=92\n" +
		"Overfull \\hbox (36.9pt too wide) in paragraph at lines 282--284\n" +
		"MDBRAND-CODE-TOOWIDE cols=200 fits=92\n" +
		"Output written on doc.pdf (20 pages, 12345 bytes)."

	wide := scanLog(log).Wide
	if len(wide) != 2 {
		t.Fatalf("got %d wide block(s), want 2: %+v", len(wide), wide)
	}
	if wide[0].Cols != 101 || wide[0].Fits != 92 {
		t.Errorf("first block = %+v, want {Cols:101 Fits:92}", wide[0])
	}
	if wide[1].Cols != 200 {
		t.Errorf("second block = %+v, want 200 columns", wide[1])
	}
}

// U+FE0F is the invisible "draw as emoji" after ⚠ in ⚠️. It has no glyph by
// design, so it must not count as a hole — while a real one still does.
func TestScanLogIgnoresVariationSelectors(t *testing.T) {
	log := `Missing character: There is no ️ (U+FE0F) in font Inter Regular/OT:script=latn;!
Missing character: There is no ☐ (U+2610) in font Inter Regular/OT:script=latn;!`
	holes := scanLog(log).Holes
	if len(holes) != 1 || !strings.Contains(holes[0], "U+2610") {
		t.Errorf("holes = %q, want only the ballot box", holes)
	}
}

// Dosis has no italic and no small caps: NFSS set both in the regular, and the
// warning is the only witness. The lines are a real xelatex log's. A swap that
// keeps the style (bx for b, sl for it) and a size substitution are not defects.
func TestScanLogFindsStylesSetInTheRegular(t *testing.T) {
	log := "LaTeX Font Warning: Font shape `TU/Dosis(0)/m/it' undefined\n" +
		"(Font)              using `TU/Dosis(0)/m/n' instead on input line 317.\n\n" +
		"LaTeX Font Warning: Font shape `TU/Dosis(0)/m/sc' undefined\n" +
		"(Font)              using `TU/Dosis(0)/m/n' instead on input line 318.\n\n" +
		"LaTeX Font Warning: Font shape `TU/Dosis(0)/m/it' undefined\n" +
		"(Font)              using `TU/Dosis(0)/m/n' instead on input line 402.\n\n" +
		"LaTeX Font Warning: Font shape `TU/PTSerifCaption(0)/b/n' undefined\n" +
		"(Font)              using `TU/PTSerifCaption(0)/m/n' instead on input line 9.\n\n" +
		"LaTeX Font Warning: Font shape `TU/lmr/bx/it' undefined\n" +
		"(Font)              using `TU/lmr/b/sl' instead on input line 12.\n\n" +
		"LaTeX Font Info:    Font shape `TU/lmtt/bx/n' in size <10> not available\n"
	got := scanLog(log).Shapes
	want := []string{"Dosis: italic", "Dosis: small caps", "PTSerifCaption: bold"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("shapes = %q, want %q", got, want)
	}
}

// Formats follow the precedence every setting does, default to pdf, and an
// unknown name stops the build instead of writing nothing for it.
func TestPickFormats(t *testing.T) {
	for _, c := range []struct {
		flag, fm, cfg []string
		want          string
	}{
		{nil, nil, nil, "pdf"},
		{nil, nil, []string{"docx"}, "docx"},
		{nil, []string{"pdf", "docx"}, []string{"docx"}, "pdf,docx"},
		{[]string{"docx"}, []string{"pdf"}, nil, "docx"},
		{[]string{"DOCX", "docx"}, nil, nil, "docx"},
	} {
		got, err := pickFormats(c.flag, c.fm, c.cfg)
		if err != nil || strings.Join(got, ",") != c.want {
			t.Errorf("pickFormats(%v, %v, %v) = %v, %v; want %s", c.flag, c.fm, c.cfg, got, err, c.want)
		}
	}
	if _, err := pickFormats([]string{"odt"}, nil, nil); err == nil {
		t.Error("odt accepted")
	}
}

// The PDF's path must not move: `./x.md` built to `x.pdf` before formats
// existed, and the result line prints it.
func TestOutputPath(t *testing.T) {
	for _, c := range []struct {
		in, out, format string
		n               int
		want            string
	}{
		{"./x.md", "", "pdf", 1, "x.pdf"},
		{"docs/x.md", "", "docx", 2, "docs/x.docx"},
		{"x.md", "out/informe.pdf", "docx", 2, "out/informe.docx"},
		{"x.md", "informe.txt", "pdf", 1, "informe.txt"},
		{"talks/x.md", "", "notes", 2, "talks/x-notes.pdf"},
		{"x.md", "out/talk.pdf", "notes", 2, "out/talk-notes.pdf"},
		{"x.md", "out/notas.pdf", "notes", 1, "out/notas.pdf"},
	} {
		got, err := outputPath(Options{Input: c.in, Output: c.out}, c.format, c.n)
		if err != nil || got != c.want {
			t.Errorf("outputPath(%s, %s, %s) = %q, %v; want %q", c.in, c.out, c.format, got, err, c.want)
		}
	}
	// A .docx named .pdf would hand Word a file it cannot read, and the other
	// way round.
	if _, err := outputPath(Options{Input: "x.md", Output: "x.pdf"}, "docx", 1); err == nil {
		t.Error("--to docx -o x.pdf accepted")
	}
}

// Inter's case forms copied as U+EE4E/U+EE4F. A private-use character the
// source holds itself — an icon font, on purpose — is not the defect.
func TestPrivateUseNotInTheSourceIsFound(t *testing.T) {
	text := "A switch in entity \uee4eSD1\uee4f and an icon \uf0e0 here."
	got := privateUse(text, "an icon \uf0e0 here")
	if len(got) != 1 || !strings.Contains(got[0], "<U+EE4E>SD1<U+EE4F>") {
		t.Errorf("privateUse = %q", got)
	}
	if got := privateUse("plain (SD1)", ""); got != nil {
		t.Errorf("clean text reported: %q", got)
	}
}

// The Lua filter reports on stderr, mixed with pandoc's own warnings.
func TestSpanProblemsReadTheFilterReport(t *testing.T) {
	out := "[WARNING] Could not fetch resource\nMDBRAND-SPAN color=\"#c2410c\"\torange words\n" +
		"MDBRAND-SPAN a ::: {.accent} block\tA block.\n"
	err := spanProblems(out)
	if err == nil {
		t.Fatal("no error for two refused colours")
	}
	for _, want := range []string{`color="#c2410c" on "orange words"`, `a ::: {.accent} block on "A block."`, "[words]{.accent}"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message lacks %q:\n%s", want, err)
		}
	}
	if err := spanProblems("[WARNING] nothing of ours\n"); err != nil {
		t.Errorf("unrelated output refused: %v", err)
	}
}

// A link to an anchor nothing carries, as spans.lua reports it, stops the
// build naming the nearest identifier that exists.
func TestAnchorProblemsNameTheNearest(t *testing.T) {
	out := "MDBRAND-ANCHOR setup\nMDBRAND-IDS install set-up\n"
	err := anchorProblems(out)
	if err == nil || !strings.Contains(err.Error(), "#setup — did you mean #set-up?") {
		t.Errorf("err = %v", err)
	}
	if err := anchorProblems("MDBRAND-ACCENT\n"); err != nil {
		t.Errorf("no anchor report must pass: %v", err)
	}
}

// Raw HTML, as spans.lua reports it, stops the build once per tag with its
// Markdown form; a table's rows and cells are the table's, not lines of their own.
func TestHTMLProblemsNameTheMarkdownForm(t *testing.T) {
	out := "MDBRAND-HTML br\t<br>\nMDBRAND-HTML br\t<br/>\nMDBRAND-HTML table\t<table>\n" +
		"MDBRAND-HTML tr\t<tr>\nMDBRAND-HTML td\t<td>\nMDBRAND-HTML marquee\t<marquee>\n"
	err := htmlProblems(out)
	if err == nil {
		t.Fatal("no error for raw HTML")
	}
	msg := err.Error()
	for _, want := range []string{"<br> — write a backslash", "<table> — write a pipe table", "<marquee> — write it in Markdown"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
	for _, not := range []string{"<br/>", "<tr>", "<td>"} {
		if strings.Contains(msg, not) {
			t.Errorf("message repeats %q:\n%s", not, msg)
		}
	}
	if err := htmlProblems("MDBRAND-ACCENT\n"); err != nil {
		t.Errorf("no HTML report must pass: %v", err)
	}
}

// A raw \ref or \cite to nothing prints "??" or "[?]", and the final pass's log
// is the only witness. The wording is LaTeX's own, wrapped as the log wraps it.
func TestScanLogFindsUndefinedReferences(t *testing.T) {
	log := "LaTeX Warning: Reference `tab:costes' on page 1 undefined on input line 52.\n" +
		"LaTeX Warning: Citation `knuth' on page 2 undefined on input line 60.\n" +
		"LaTeX Warning: Reference `tab:costes' on page 3 \nundefined on input line 80.\n" +
		"LaTeX Warning: There were undefined references.\n"
	got := strings.Join(scanLog(log).Undefined, "|")
	if got != `\ref{tab:costes}|\cite{knuth}` {
		t.Errorf("undefined = %q", got)
	}
}

// A row cut at the foot is reported once per page by the preamble; a second
// pass repeats it, and the page must not be listed twice.
func TestScanLogFindsRowsCutAtTheFoot(t *testing.T) {
	log := "MDBRAND-ROW-TOOTALL 2\n[2]\nMDBRAND-ROW-TOOTALL 2\nMDBRAND-ROW-TOOTALL iv\n"
	if got := strings.Join(scanLog(log).TallRows, "|"); got != "2|iv" {
		t.Errorf("tall rows = %q", got)
	}
}

// pdftotext clips a word the sheet cuts and gives it the edge as xMax; a word
// that merely ends near the margin is not cut, on any page.
func TestOffPaperFindsWordsCutAtTheSheet(t *testing.T) {
	bbox := `<page width="595.280000" height="841.890000">
<word xMin="72.0" yMin="100.0" xMax="532.9" yMax="110.0">see</word>
</page>
<page width="595.280000" height="841.890000">
<word xMin="299.8" yMin="100.0" xMax="597.263957" yMax="110.0">https://a.com/?x=1&amp;y=2</word>
<word xMin="0.0" yMin="200.0" xMax="40.0" yMax="210.0">left</word>
</page>`
	got := strings.Join(offPaper(bbox), "|")
	if got != `page 2: "https://a.com/?x=1&y=2"|page 2: "left"` {
		t.Errorf("off paper = %s", got)
	}
}

// Short of headsep the header only sits closer to the text: a warning. Past
// it, the logo prints over the text: the build stops.
func TestJudgeHeaderFailsOnlyPastTheGap(t *testing.T) {
	var rep Report
	if err := judgeHeader(scan{HeadShortPt: 1.9, HeadSepPt: 13}, &rep); err != nil || len(rep.Warnings) != 1 {
		t.Errorf("1.9pt into a 13pt gap: err %v, warnings %q", err, rep.Warnings)
	}
	if err := judgeHeader(scan{HeadShortPt: 164.1, HeadSepPt: 13}, &rep); err == nil {
		t.Error("164pt past a 13pt gap built")
	}
}

// A picture is found in pdfimages' list by its size in pixels, once per page;
// its soft mask and the rasters of a figure's shadows are not the author's.
func TestPlacedPPIFindsTheAuthorsPictures(t *testing.T) {
	list := `page   num  type   width height color comp bpc  enc interp  object ID x-ppi y-ppi size ratio
--------------------------------------------------------------------------------------------
   1     0 image     120    40  rgb     3   8  image  no         9  0    18    18  100B 1.0%
   1     1 smask     120    40  gray    1   8  image  no         9  0    18    18  100B 1.0%
   2     2 image     120    40  rgb     3   8  image  no         9  0    18    18  100B 1.0%
   2     3 image     120    40  rgb     3   8  image  no         9  0    18    18  100B 1.0%
   3     4 image     600   180  rgb     3   8  image  no        12  0    92    92 6060B 1.9%
   3     5 image     367   312  gray    1   1  image  yes       58  0    71    71   37B 0.3%
   4     6 image     600   180  rgb     3   8  image  no        12  0   300   300 6060B 1.9%
`
	low, soft := placedPPI(list, map[[2]int]string{{120, 40}: "tiny.png", {600, 180}: "mid.png"})
	if len(low) != 2 || !strings.HasPrefix(low[0], "tiny.png prints at 18 ppi on page 1") || !strings.Contains(low[1], "page 2") {
		t.Errorf("low = %q", low)
	}
	if len(soft) != 1 || !strings.Contains(soft[0], "mid.png prints at 92 ppi on page 3 (600 px wide; at 101mm it would print at 150)") {
		t.Errorf("soft = %q", soft)
	}
}

// A formula the .docx prints as TeX stops it; raw LaTeX it drops warns by
// command; pandoc's other warnings reach the reader, citations aside.
func TestPandocWarnings(t *testing.T) {
	out := "[WARNING] Could not convert TeX math \\foo{x}, rendering as TeX:\n  \\foo{x}\n"
	if err := pandocWarnings(out, &Report{}); err == nil || !strings.Contains(err.Error(), `$\foo{x}$`) {
		t.Errorf("err = %v", err)
	}
	rep := &Report{}
	out = "MDBRAND-RAWTEX \\newpage\nMDBRAND-RAWTEX \\newpage\n[WARNING] Citeproc: citation x not found\n[WARNING] Duplicate note reference '1'\n"
	if err := pandocWarnings(out, rep); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(rep.Warnings, "|")
	if strings.Count(got, `\newpage`) != 2 || !strings.Contains(got, "pandoc: Duplicate note reference '1'") || strings.Contains(got, "Citeproc") {
		t.Errorf("warnings = %q", rep.Warnings)
	}
}

// A document's work files keep its name when TeX can take it and the build
// has no file of that name; logo.md wrote over the logo and a % ended the job
// name, so those get a fixed one.
func TestWorkStem(t *testing.T) {
	for in, want := range map[string]string{
		"docs/informe-2026_v2.md": "informe-2026_v2",
		"logo.md":                 "mdbrand-document",
		"Preamble.md":             "mdbrand-document",
		"fig03.md":                "mdbrand-document",
		"informe 50% ~final.md":   "mdbrand-document",
		"figura.md":               "figura",
	} {
		if got := workStem(in); got != want {
			t.Errorf("workStem(%q) = %q, want %q", in, got, want)
		}
	}
}

// Only a string with Markdown in it goes through pandoc: a plain title keeps
// its .tex byte for byte, apostrophes and underscores inside words included.
func TestMarkdownSyntax(t *testing.T) {
	for s, want := range map[string]bool{
		"*Hamlet* and power":                   true,
		"The `go` tool":                        true,
		"__strong__":                           true,
		"OpenAI's 2025 recapitalisation":       false,
		"file_name_v2 and Spain's water":       false,
		"AI data centers, 50% of the load":     false,
		"H~2~O":                                true,
		"[Linked](https://example.org) report": true,
	} {
		if got := markdownSyntax.MatchString(s); got != want {
			t.Errorf("%q: %v, want %v", s, got, want)
		}
	}
}
