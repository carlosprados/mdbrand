package tex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carlosprados/mdbrand/internal/brand"
)

func TestEscape(t *testing.T) {
	// A title with an ampersand or an underscore must not become a LaTeX error
	// three hundred lines into a generated file.
	for in, want := range map[string]string{
		"Ventas & Marketing": `Ventas \& Marketing`,
		"coste_total":        `coste\_total`,
		"100 % del parque":   `100 \% del parque`,
		"a~b":                `a\textasciitilde{}b`,
		`C:\ruta`:            `C:\textbackslash{}ruta`,
		"Amplía Soluciones":  "Amplía Soluciones",
	} {
		if got := Escape(in); got != want {
			t.Errorf("Escape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseLenMM(t *testing.T) {
	for in, want := range map[string]float64{"22mm": 22, "2.2cm": 22, "1in": 25.4} {
		got, err := ParseLenMM(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got < want-0.01 || got > want+0.01 {
			t.Errorf("ParseLenMM(%q) = %g, want %g", in, got, want)
		}
	}
	if _, err := ParseLenMM("22"); err == nil {
		t.Error("a length without a unit must be rejected, not guessed")
	}
}

func TestTextWidthMM(t *testing.T) {
	b := brand.Default() // a4, 22mm margins
	w, err := TextWidthMM(b)
	if err != nil {
		t.Fatal(err)
	}
	if w != 166 {
		t.Errorf("measure = %g mm, want 166", w)
	}
	b.Page.Margin = "110mm"
	if _, err := TextWidthMM(b); err == nil {
		t.Error("a margin that leaves no measure must be an error")
	}
}

func TestRenderReportCoverUsesEscapedStrings(t *testing.T) {
	b := brand.Default()
	out, err := Render("before", &Data{
		Brand: b, Style: "report", Title: "Ventas & Marketing", Author: "A_B",
		CoverLogoWidth: "46mm",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `Ventas \& Marketing`) || !strings.Contains(out, `A\_B`) {
		t.Errorf("cover did not escape its strings:\n%s", out)
	}
	if strings.Contains(out, "@@") {
		t.Error("unrendered template action left behind")
	}
}

func TestEveryStyleRenders(t *testing.T) {
	b := brand.Default()
	for _, s := range Styles {
		for _, frag := range []string{"preamble", "before", "after"} {
			if _, err := Render(frag, &Data{Brand: b, Style: s, Title: "T"}); err != nil {
				t.Errorf("style %s fragment %s: %v", s, frag, err)
			}
		}
	}
}

// bundleFor writes a brand.yaml whose `page:` block holds the given keys, and
// loads it, so the test sees the same declared/defaulted distinction a real
// bundle produces. Building a Brand by hand cannot: whether headheight was
// declared is recorded during Load.
func bundleFor(t *testing.T, pageKeys ...string) *brand.Brand {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	y := "name: b\n"
	if len(pageKeys) > 0 {
		y += "page:\n"
		for _, k := range pageKeys {
			y += "  " + k + "\n"
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "b", "brand.yaml"), []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := brand.Load(dir, "b")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestHeaderHeightMM(t *testing.T) {
	const pt = 25.4 / 72.272

	// The 4:1 wordmark every default was written for: 16mm wide is 4mm tall, so
	// the 22pt floor still wins and existing documents do not move a hair.
	b := bundleFor(t)
	h, err := HeaderHeightMM(b, 0.25)
	if err != nil {
		t.Fatal(err)
	}
	if want := 22 * pt; h < want-0.01 || h > want+0.01 {
		t.Errorf("wide logo: headheight %.2fmm, want the %.2fmm default", h, want)
	}

	// No logo at all: nothing to reserve room for.
	if h, err := HeaderHeightMM(b, 0); err != nil || h < 22*pt-0.01 || h > 22*pt+0.01 {
		t.Errorf("no logo: %.2fmm, %v; want the default", h, err)
	}

	// A square crest at the same width is four times taller than that floor.
	// This is the defect: the box has to grow, and nobody wrote a number.
	h, err = HeaderHeightMM(b, 1)
	if err != nil {
		t.Fatal(err)
	}
	if h < 16 {
		t.Errorf("square logo: headheight %.2fmm cannot hold a 16mm mark", h)
	}

	// Declared and sufficient: mdbrand must not second-guess it.
	b = bundleFor(t, "logo_width_header: 10mm", "headheight: 32pt", "headsep: 12pt", "margin: 25mm")
	h, err = HeaderHeightMM(b, 1)
	if err != nil {
		t.Fatal(err)
	}
	if want := 32 * pt; h < want-0.01 || h > want+0.01 {
		t.Errorf("declared headheight: got %.2fmm, want %.2fmm", h, want)
	}

	// Declared and too small. Silently raising it would hide a bundle the
	// author still has to fix, so it fails — and the message has to carry both
	// ways out, because the reader is looking at a PDF, not at this code.
	b = bundleFor(t, "logo_width_header: 13mm", "headheight: 22pt")
	_, err = HeaderHeightMM(b, 1)
	if err == nil {
		t.Fatal("a 13mm square logo in a 22pt header must fail")
	}
	for _, want := range []string{"headheight", "logo_width_header"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %q: %v", want, err)
		}
	}

	// Too tall for the margin: the header runs off the top of the paper, which
	// is the same defect facing the other way.
	b = bundleFor(t, "logo_width_header: 20mm", "margin: 10mm")
	if _, err := HeaderHeightMM(b, 1); err == nil {
		t.Fatal("a header taller than the top margin must fail")
	}
}
