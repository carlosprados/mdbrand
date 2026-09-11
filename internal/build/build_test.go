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
	_, _, pages, short := scanLog(log)
	if pages != 2 {
		t.Errorf("pages = %d, want 2", pages)
	}
	if short != 23.17 {
		t.Errorf("headShortPt = %v, want 23.17 (the largest)", short)
	}

	if _, _, _, short := scanLog("Output written on doc.pdf (1 page, 10 bytes)."); short != 0 {
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

	_, over, _, _ := scanLog(log)
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

	_, over, _, _ := scanLog(log)
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
