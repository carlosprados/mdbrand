package cmd

import (
	"strings"
	"testing"

	"github.com/carlosprados/mdbrand/internal/brand"
)

// A school's bundle carries a square escudo, and its thesis has no running
// header: validate warned about a header height no thesis uses. A bundle
// without a thesis section keeps the note.
func TestHeaderNoteSkipsThesisBundles(t *testing.T) {
	for name, want := range map[string]string{"thesis-school": "", "square-logo": "derived from a logo"} {
		b, err := brand.Load("../testdata/brands", name)
		if err != nil {
			t.Fatal(err)
		}
		note, err := headerGeometry(b)
		if err != nil {
			t.Fatal(err)
		}
		if (want == "") != (note == "") || !strings.Contains(note, want) {
			t.Errorf("%s: note %q, want %q", name, note, want)
		}
	}
}
