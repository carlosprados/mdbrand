// Package tex turns a brand bundle plus a document's front matter into the
// three LaTeX fragments pandoc needs: a preamble, a front page and a closing
// block. The strings are escaped here, in Go, so the templates never have to
// guess whether a title contains an ampersand.
package tex

import (
	"embed"
	"fmt"
	"math"
	"strconv"
	"strings"
	"text/template"

	"github.com/carlosprados/mdbrand/internal/brand"
)

//go:embed templates/*.tmpl
var files embed.FS

// Styles are the document shapes mdbrand knows.
var Styles = []string{"report", "note", "letter"}

// ValidStyle reports whether s is a known style.
func ValidStyle(s string) bool {
	for _, v := range Styles {
		if v == s {
			return true
		}
	}
	return false
}

// Data is what the templates see.
type Data struct {
	Brand *brand.Brand
	Style string

	LogoFile          string // basename inside the work dir; "" when there is none
	LogoSecondaryFile string // cover only; "" when the bundle declares none

	CoverLogoWidth          string
	CoverLogoSecondaryWidth string
	HeaderLogoWidth         string

	DisplayFont       bool
	DisplayRegularDir string
	DisplayRegular    string
	DisplayBoldDir    string
	DisplayBold       string

	HeaderTitle  string
	Title        string
	Subtitle     string
	Author       string
	Date         string
	Reference    string
	Confidential string

	Recipient      []string
	Place          string
	Greeting       string
	Signature      string
	SignatureLines []string
}

var funcs = template.FuncMap{"texEscape": Escape}

// Escape makes a plain string safe inside LaTeX.
func Escape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\textbackslash{}`)
		case '{', '}', '$', '&', '#', '%', '_':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '~':
			b.WriteString(`\textasciitilde{}`)
		case '^':
			b.WriteString(`\textasciicircum{}`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Render renders one embedded template by name ("preamble", "before", "after").
func Render(name string, d *Data) (string, error) {
	t, err := template.New(name+".tex.tmpl").Funcs(funcs).ParseFS(files, "templates/"+name+".tex.tmpl")
	if err != nil {
		return "", err
	}
	var out strings.Builder
	if err := t.Execute(&out, d); err != nil {
		return "", fmt.Errorf("template %s: %w", name, err)
	}
	return out.String(), nil
}

// ParseLenMM converts a LaTeX-ish length ("22mm", "2.2cm", "0.85in", "62pt")
// to millimetres.
func ParseLenMM(s string) (float64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	for unit, factor := range map[string]float64{
		"mm": 1, "cm": 10, "in": 25.4, "pt": 25.4 / 72.27, "bp": 25.4 / 72,
	} {
		if strings.HasSuffix(s, unit) {
			v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, unit)), 64)
			if err != nil {
				return 0, fmt.Errorf("length %q: %w", s, err)
			}
			return v * factor, nil
		}
	}
	return 0, fmt.Errorf("length %q: no unit (use mm, cm, in or pt)", s)
}

// PaperWidthMM returns the paper width for the sizes pandoc accepts.
func PaperWidthMM(paper string) float64 {
	switch strings.ToLower(paper) {
	case "a4", "a4paper":
		return 210
	case "a5", "a5paper":
		return 148
	case "letter", "letterpaper":
		return 215.9
	case "legal", "legalpaper":
		return 215.9
	default:
		return 210
	}
}

// TextWidthMM is the measure a figure has to fit into.
func TextWidthMM(b *brand.Brand) (float64, error) {
	m, err := ParseLenMM(b.Page.Margin)
	if err != nil {
		return 0, err
	}
	w := PaperWidthMM(b.Page.PaperSize) - 2*m
	if w <= 0 {
		return 0, fmt.Errorf("margin %s leaves no text width on %s", b.Page.Margin, b.Page.PaperSize)
	}
	return w, nil
}

// mmPerPt converts TeX points, the unit headheight and headsep are written in.
const mmPerPt = 25.4 / 72.272

// headSlackMM is the air left above and below the mark inside the header box.
// Without it fancyhdr reports "\headheight is too small (0.0pt too short)" on
// rounding alone, and that warning only exists in a log nobody reads.
const headSlackMM = 2 * mmPerPt

// HeaderHeightMM settles the running header's box height, in millimetres.
//
// logoAspect is the header logo's height divided by its width, or 0 when the
// document carries no logo. This is the one piece of geometry that cannot be
// written down in advance: `logo_width_header` is a WIDTH, and what the header
// must reserve is a height. For the wide 4:1 marks most bundles carry the
// difference is invisible; for a square crest the mark is four times taller
// than the default box, overflows it, eats headsep whole and prints across the
// first line of body text on every page.
//
// So the rule is: an undeclared headheight is derived from the logo, and a
// declared one that cannot hold it fails the build naming both fixes.
func HeaderHeightMM(b *brand.Brand, logoAspect float64) (float64, error) {
	declared, err := ParseLenMM(b.Page.HeadHeight)
	if err != nil {
		return 0, fmt.Errorf("page.headheight: %w", err)
	}
	sep, err := ParseLenMM(b.Page.HeadSep)
	if err != nil {
		return 0, fmt.Errorf("page.headsep: %w", err)
	}
	margin, err := ParseLenMM(b.Page.Margin)
	if err != nil {
		return 0, fmt.Errorf("page.margin: %w", err)
	}

	var logoH float64
	if logoAspect > 0 {
		logoW, err := ParseLenMM(b.Page.LogoWidthHeader)
		if err != nil {
			return 0, fmt.Errorf("page.logo_width_header: %w", err)
		}
		logoH = logoW * logoAspect
	}

	height := declared
	switch {
	case logoH+headSlackMM <= declared:
		// Fits as declared, whoever chose the number.
	case b.Page.HeadHeightDeclared():
		return 0, fmt.Errorf(
			"page.headheight %s cannot hold the header logo, which is %.1fmm tall at "+
				"logo_width_header %s: raise headheight to %.0fpt, or lower logo_width_header to %.0fmm",
			b.Page.HeadHeight, logoH, b.Page.LogoWidthHeader,
			math.Ceil((logoH+headSlackMM)/mmPerPt), math.Floor((declared-headSlackMM)/logoAspect))
	default:
		height = logoH + headSlackMM
	}

	// The header lives in the top margin, above the text block. If it is taller
	// than the margin it runs off the top of the paper instead of onto the text,
	// which is the same defect wearing a different hat.
	if height+sep >= margin {
		return 0, fmt.Errorf(
			"the running header needs %.1fmm (headheight) + %s (headsep) and the top margin is only %s: "+
				"lower logo_width_header, or raise page.margin",
			height, b.Page.HeadSep, b.Page.Margin)
	}
	return height, nil
}
