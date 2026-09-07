package tex

import (
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
