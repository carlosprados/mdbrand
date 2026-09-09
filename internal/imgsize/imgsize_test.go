package imgsize

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// The formats are measured in different units on purpose, so the tests assert
// the ratio: that is what page geometry consumes.

func TestSVGWidthHeight(t *testing.T) {
	w, h, unit, err := SVG(`<svg width="200" height="50" viewBox="0 0 400 100"></svg>`)
	if err != nil {
		t.Fatal(err)
	}
	if w != 200 || h != 50 {
		t.Fatalf("got %vx%v, want 200x50", w, h)
	}
	if unit != 0.5 {
		t.Fatalf("unit %v, want 0.5: inner user units are half a pixel here", unit)
	}
}

// d2 without --scale emits no width/height at all.
func TestSVGViewBoxOnly(t *testing.T) {
	w, h, unit, err := SVG(`<svg viewBox="0 0 300 150" xmlns="http://www.w3.org/2000/svg"></svg>`)
	if err != nil {
		t.Fatal(err)
	}
	if w != 300 || h != 150 || unit != 1 {
		t.Fatalf("got %vx%v unit %v, want 300x150 unit 1", w, h, unit)
	}
}

func TestSVGRejectsNonsense(t *testing.T) {
	for _, src := range []string{
		``,
		`<html></html>`,
		`<svg xmlns="http://www.w3.org/2000/svg"></svg>`,
		`<svg width="0" height="0" viewBox="0 0 0 0"></svg>`,
	} {
		if _, _, _, err := SVG(src); err == nil {
			t.Errorf("accepted %q", src)
		}
	}
}

func TestAspect(t *testing.T) {
	dir := t.TempDir()

	square := filepath.Join(dir, "crest.png")
	writePNG(t, square, 421, 421)
	wide := filepath.Join(dir, "wordmark.png")
	writePNG(t, wide, 400, 100)

	svg := filepath.Join(dir, "mark.svg")
	if err := os.WriteFile(svg, []byte(`<svg viewBox="0 0 520.2 675.8"></svg>`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Enough of a PDF for the MediaBox scan; a real one carries the same line.
	pdf := filepath.Join(dir, "mark.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.4\n1 0 obj\n<< /Type /Page /MediaBox [ 0 0 72 216 ] >>\nendobj\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		path string
		want float64
	}{
		{square, 1},
		{wide, 0.25},
		{svg, 675.8 / 520.2},
		{pdf, 3},
	} {
		got, err := Aspect(c.path)
		if err != nil {
			t.Fatalf("%s: %v", filepath.Base(c.path), err)
		}
		if d := got - c.want; d > 1e-9 || d < -1e-9 {
			t.Errorf("%s: aspect %v, want %v", filepath.Base(c.path), got, c.want)
		}
	}
}

func TestAspectRefusesUnknownFormat(t *testing.T) {
	p := filepath.Join(t.TempDir(), "logo.eps")
	if err := os.WriteFile(p, []byte("%!PS"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Aspect(p); err == nil {
		t.Fatal("an EPS was measured; it cannot be")
	}
}

// A PDF whose page tree is inside an object stream is unreadable by a plain
// scan. That is not a defect in the artwork, so it must surface as an error the
// caller can choose to tolerate, never as a silent zero.
func TestPDFWithoutReadableMediaBox(t *testing.T) {
	p := filepath.Join(t.TempDir(), "compressed.pdf")
	if err := os.WriteFile(p, []byte("%PDF-1.7\n5 0 obj\n<< /Type /ObjStm >>\nstream\n...\nendstream\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Aspect(p); err == nil {
		t.Fatal("reported a size it cannot know")
	}
}

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
