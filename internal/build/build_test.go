package build

import "testing"

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
