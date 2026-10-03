package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/carlosprados/mdbrand/internal/brand"
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

	if short := scanLog("Output written on doc.pdf (1 page, 10 bytes).").HeadShortPt; short != 0 {
		t.Errorf("a clean log reported %v pt short", short)
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
