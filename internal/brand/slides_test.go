package brand

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContrastMatchesWCAG(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want float64
	}{
		{"000000", "FFFFFF", 21},
		{"FFFFFF", "000000", 21}, // symmetric
		{"777777", "FFFFFF", 4.48},
		{"F2EDF5", "222629", 13.22},
	} {
		if got := Contrast(c.a, c.b); math.Abs(got-c.want) > 0.05 {
			t.Errorf("Contrast(%s, %s) = %.2f, want %.2f", c.a, c.b, got, c.want)
		}
	}
}

// A dark ground is the one colour pair mdbrand cannot leave to taste: pale
// type on a projector in a lit room is the slide nobody at the back can read.
func TestCheckSlidesMeasuresTheGround(t *testing.T) {
	problems := func(s Slides) string {
		b := Default()
		b.Slides = s
		b.Slides.applyDefaults()
		p, _ := b.CheckSlides()
		return strings.Join(p, "\n")
	}
	if p := problems(Slides{}); p != "" {
		t.Errorf("no slides section must be fine, got %q", p)
	}
	if p := problems(Slides{Background: "222629", Foreground: "F2EDF5"}); p != "" {
		t.Errorf("a legible pair was refused: %q", p)
	}
	if p := problems(Slides{Background: "222629", Foreground: "5D6266"}); !strings.Contains(p, "contrast") {
		t.Errorf("grey on charcoal must be refused for its contrast, got %q", p)
	}
	if p := problems(Slides{Background: "222629"}); !strings.Contains(p, "slides.foreground") {
		t.Errorf("a ground without type colour must name slides.foreground, got %q", p)
	}
	if p := problems(Slides{Foreground: "FFFFFF"}); !strings.Contains(p, "without slides.background") {
		t.Errorf("a foreground with no ground colours nothing, got %q", p)
	}
}

// The logo is drawn on the ground too. Measured fill by fill, because a mark
// in the text colour vanishes on a dark slide while its orange half survives.
func TestCheckSlidesMeasuresTheLogoOnTheGround(t *testing.T) {
	dir := t.TempDir()
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10">` +
		`<path style="fill:#f68e1b" d="M0 0h5v5z"/><path fill="#5D6266" d="M5 5h5v5z"/>` +
		strings.Repeat(" ", 300) + `</svg>`
	if err := os.WriteFile(filepath.Join(dir, "logo.svg"), []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
	b := Default()
	b.Dir, b.Logo = dir, "logo.svg"
	b.Slides = Slides{Background: "222629", Foreground: "F2EDF5"}
	p, _ := b.CheckSlides()
	got := strings.Join(p, "\n")
	if !strings.Contains(got, "5D6266") || strings.Contains(got, "F68E1B") {
		t.Errorf("want the grey fill named and the orange one passed, got %q", got)
	}
	if !strings.Contains(got, "slides.logo") {
		t.Errorf("the fix, a variant in slides.logo, must be named: %q", got)
	}
}

func TestSvgFillsExpandsShortHex(t *testing.T) {
	got := svgFills(`<g fill="#fff"><path style="fill: #A3A5A4"/><path fill="none"/><path fill="#ffffff"/></g>`)
	if strings.Join(got, ",") != "FFFFFF,A3A5A4" {
		t.Errorf("svgFills = %v", got)
	}
}
