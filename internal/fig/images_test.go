package fig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A picture an SVG links is read from the source's directory and put into the
// SVG, since rsvg-convert reads a copy elsewhere; one it cannot have stops.
func TestInlineImagesResolveAgainstTheSource(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a b.png"), []byte("PNG"), 0o644); err != nil {
		t.Fatal(err)
	}
	svg := `<svg><image xlink:href="a%20b.png"/><image href='a b.png' x="1"/>` +
		`<image href="data:image/png;base64,AA=="/><a href="x.png">link</a></svg>`
	got, err := inlineImages(svg, dir, "f.svg")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(got, `"data:image/png;base64,UE5H"`); n != 2 {
		t.Errorf("%d pictures inlined, want 2:\n%s", n, got)
	}
	for _, keep := range []string{`base64,AA==`, `<a href="x.png">`} {
		if !strings.Contains(got, keep) {
			t.Errorf("%s changed:\n%s", keep, got)
		}
	}

	_, err = inlineImages(`<image href="gone.png"/><image href="https://x.org/i.png"/>`, dir, "f.svg")
	if err == nil {
		t.Fatal("no error for a missing and a remote picture")
	}
	for _, want := range []string{"gone.png: not found at " + filepath.Join(dir, "gone.png"), "https://x.org/i.png: a remote picture"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message lacks %q:\n%s", want, err)
		}
	}
}
