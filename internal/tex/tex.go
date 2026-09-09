// Package tex turns a brand bundle plus a document's front matter into the
// three LaTeX fragments pandoc needs: a preamble, a front page and a closing
// block. The strings are escaped here, in Go, so the templates never have to
// guess whether a title contains an ampersand.
package tex

import (
	"embed"
	"fmt"
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

	LogoFile        string // basename inside the work dir; "" when there is none
	CoverLogoWidth  string
	HeaderLogoWidth string

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
