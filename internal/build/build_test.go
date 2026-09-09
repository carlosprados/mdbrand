package build

import (
	"os"
	"path/filepath"
	"testing"

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
