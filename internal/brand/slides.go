package brand

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Slides is what `style: slides` takes from a bundle beyond its colours, type
// and logo. Everything is optional: a bundle without the section builds a
// white deck in its own colours.
type Slides struct {
	// Background, when set, fills the title and section slides; Foreground is
	// the type on it. A projector washes out pale type on a dark ground, so
	// the pair is held to a measured contrast rather than trusted.
	Background string `yaml:"background"`
	Foreground string `yaml:"foreground"`
	// Logo is a variant of the mark for that background. Absent, the bundle's
	// own logo is used, and checked against the background in its place.
	Logo string `yaml:"logo"`
	// Art is bled to the right edge of the title and section slides.
	Art string `yaml:"art"`

	LogoWidth      string `yaml:"logo_width"`       // footer of every slide
	LogoWidthCover string `yaml:"logo_width_cover"` // title slide

	Diagrams SlideDiagrams `yaml:"diagrams"`
}

// SlideDiagrams is the legibility band on a slide. The geometry is beamer's
// and not the bundle's, so only the band is the bundle's to choose.
type SlideDiagrams struct {
	MinTextPt float64 `yaml:"min_text_pt"`
	MaxTextPt float64 `yaml:"max_text_pt"`
}

// Dark reports whether the title and section slides have a ground of their own.
func (s Slides) Dark() bool { return s.Background != "" }

func (s *Slides) applyDefaults() {
	if s.LogoWidth == "" {
		s.LogoWidth = "12mm"
	}
	if s.LogoWidthCover == "" {
		s.LogoWidthCover = "34mm"
	}
	// A 16:9 frame is 160mm wide and is shown about 2.1 times the size of a
	// 13.33in PowerPoint slide, whose floor for type read across a room is
	// 18pt: 18 × 160/338.7 is 8.5pt on the frame.
	if s.Diagrams.MinTextPt == 0 {
		s.Diagrams.MinTextPt = 8.5
	}
	if s.Diagrams.MaxTextPt == 0 {
		s.Diagrams.MaxTextPt = 14
	}
}

// SlideLogoPath is the mark for the title and section slides: the variant
// when the bundle has one, its logo otherwise.
func (b *Brand) SlideLogoPath() string {
	if p := b.resolveLogo(b.Slides.Logo); p != "" {
		return p
	}
	return b.LogoPath()
}

// SlideArtPath is the artwork for the title and section slides, or "".
func (b *Brand) SlideArtPath() string { return b.resolveLogo(b.Slides.Art) }

// minContrast is WCAG's AA ratio for body text. Slide titles are large, but
// a projector in a lit room takes more contrast than a monitor gives back.
const minContrast = 4.5

// minMarkContrast is WCAG's ratio for graphics, which is what a logo is.
const minMarkContrast = 3.0

// CheckSlides validates the slides section. It is apart from Check because
// only a deck reads it: a fault here must not stop a report. A pair of colours
// is measured, so a pair that fails is a problem; a raster logo cannot be
// measured against the ground, so it earns a warning.
func (b *Brand) CheckSlides() (problems, warnings []string) {
	s := b.Slides
	add := func(dst *[]string, f string, a ...any) { *dst = append(*dst, fmt.Sprintf(f, a...)) }
	if s.Logo != "" {
		checkArtwork("slides.logo", b.SlideLogoPath(), &problems, &warnings)
	}
	if art := b.SlideArtPath(); art != "" {
		checkArtwork("slides.art", art, &problems, &warnings)
	}
	for label, v := range map[string]string{"slides.background": s.Background, "slides.foreground": s.Foreground} {
		if v != "" && !hexRe.MatchString(v) {
			add(&problems, "%s: %q is not a 6-digit hex without '#'", label, v)
		}
	}
	if s.Foreground != "" && s.Background == "" {
		add(&problems, "slides.foreground is set without slides.background, so it would colour nothing")
	}
	if !s.Dark() || !hexRe.MatchString(s.Background) {
		return problems, warnings
	}
	if s.Foreground == "" {
		add(&problems, "slides.background %s needs slides.foreground, the colour of the type on it", s.Background)
	} else if hexRe.MatchString(s.Foreground) {
		if c := Contrast(s.Foreground, s.Background); c < minContrast {
			add(&problems, "slides.foreground %s on slides.background %s has a contrast of %.1f:1, under the %.1f:1 a projected title needs",
				s.Foreground, s.Background, c, minContrast)
		}
	}

	logo := b.SlideLogoPath()
	if logo == "" {
		return problems, warnings
	}
	label := "slides.logo"
	if b.Slides.Logo == "" {
		label = "logo"
	}
	if !strings.EqualFold(filepath.Ext(logo), ".svg") {
		add(&warnings, "%s: %s is drawn on slides.background %s and, not being an SVG, cannot be measured against it — look at the title slide",
			label, filepath.Base(logo), s.Background)
		return problems, warnings
	}
	raw, err := os.ReadFile(logo)
	if err != nil {
		return problems, warnings // the logo checks report an unreadable file
	}
	var faint []string
	for _, c := range svgFills(string(raw)) {
		if r := Contrast(c, s.Background); r < minMarkContrast {
			faint = append(faint, fmt.Sprintf("%s (%.1f:1)", c, r))
		}
	}
	if len(faint) > 0 {
		add(&problems, "%s: %s is drawn on slides.background %s, and these fills fall under %.0f:1 against it: %s.\n"+
			"    Give slides.logo a variant of the mark made for a dark ground",
			label, filepath.Base(logo), s.Background, minMarkContrast, strings.Join(faint, ", "))
	}
	return problems, warnings
}

var fillRe = regexp.MustCompile(`fill\s*[:=]\s*"?\s*#([0-9A-Fa-f]{6}|[0-9A-Fa-f]{3})\b`)

// svgFills returns the distinct fill colours an SVG declares, as 6-digit hex.
// A fill of none, a gradient or currentColor is not a colour to measure.
func svgFills(src string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range fillRe.FindAllStringSubmatch(src, -1) {
		h := strings.ToUpper(m[1])
		if len(h) == 3 {
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		}
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}

// Contrast is the WCAG 2 contrast ratio between two 6-digit hex colours.
func Contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(hex string) float64 {
	var ch [3]float64
	for i := range ch {
		v, _ := strconv.ParseUint(hex[2*i:2*i+2], 16, 8)
		c := float64(v) / 255
		if c <= 0.03928 {
			ch[i] = c / 12.92
		} else {
			ch[i] = math.Pow((c+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*ch[0] + 0.7152*ch[1] + 0.0722*ch[2]
}
