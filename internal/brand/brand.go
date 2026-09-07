// Package brand loads a brand bundle: a directory holding brand.yaml, a logo
// and nothing that a licence forbids redistributing. Bundles live OUTSIDE this
// repository on purpose — corporate logos and commercially licensed fonts
// (Gotham, Brandon, Circular…) must not be committed or shipped. Fonts are
// referenced by path; if the path is absent the document falls back to the body
// font instead of failing.
package brand

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Brand is one bundle, as parsed from brand.yaml plus the defaults.
type Brand struct {
	Name        string   `yaml:"name"`
	DisplayName string   `yaml:"display_name"`
	Logo        string   `yaml:"logo"`
	Colors      Colors   `yaml:"colors"`
	Fonts       Fonts    `yaml:"fonts"`
	Page        Page     `yaml:"page"`
	Diagrams    Diagrams `yaml:"diagrams"`
	Footer      string   `yaml:"footer"`

	Dir string `yaml:"-"` // resolved bundle directory
}

// Colors are hex without '#', the form LaTeX's xcolor HTML model wants.
type Colors struct {
	Primary string `yaml:"primary"` // rules and accents
	Text    string `yaml:"text"`    // cover and header type
	Rule    string `yaml:"rule"`    // hairlines
}

type Fonts struct {
	Body    string  `yaml:"body"`    // fontconfig family name, e.g. Inter
	Display Display `yaml:"display"` // brand font for cover and header
}

// Display is referenced by path and never copied into the bundle, so a
// commercial licence is not breached by sharing the bundle.
type Display struct {
	Family  string `yaml:"family"`
	Path    string `yaml:"path"`
	Regular string `yaml:"regular"`
	Bold    string `yaml:"bold"`
}

type Page struct {
	PaperSize       string  `yaml:"papersize"`
	Margin          string  `yaml:"margin"`
	LineStretch     float64 `yaml:"linestretch"`
	HeadHeight      string  `yaml:"headheight"`
	HeadSep         string  `yaml:"headsep"`
	LogoWidthCover  string  `yaml:"logo_width_cover"`
	LogoWidthHeader string  `yaml:"logo_width_header"`
}

// Diagrams carries the print numbers. They are per brand because they depend on
// the text measure, which depends on the margin.
type Diagrams struct {
	D2Theme     int     `yaml:"d2_theme"`
	D2Scale     float64 `yaml:"d2_scale"`
	D2Pad       int     `yaml:"d2_pad"`
	MinTextPt   float64 `yaml:"min_text_pt"`
	MaxTextPt   float64 `yaml:"max_text_pt"`
	MaxHeightMM float64 `yaml:"max_height_mm"`
}

// Default is the fallback bundle: no logo, no display font, sober defaults that
// still produce a correct A4 document. `mdbrand build --brand none` uses it.
func Default() *Brand {
	b := &Brand{Name: "none", DisplayName: ""}
	b.applyDefaults()
	return b
}

func (b *Brand) applyDefaults() {
	if b.Colors.Primary == "" {
		b.Colors.Primary = "1F6FEB"
	}
	if b.Colors.Text == "" {
		b.Colors.Text = "3A3A3A"
	}
	if b.Colors.Rule == "" {
		b.Colors.Rule = "C8CCCE"
	}
	if b.Fonts.Body == "" {
		b.Fonts.Body = "Inter"
	}
	if b.Page.PaperSize == "" {
		b.Page.PaperSize = "a4"
	}
	if b.Page.Margin == "" {
		b.Page.Margin = "22mm"
	}
	if b.Page.LineStretch == 0 {
		b.Page.LineStretch = 1.125
	}
	if b.Page.HeadHeight == "" {
		b.Page.HeadHeight = "22pt"
	}
	if b.Page.HeadSep == "" {
		b.Page.HeadSep = "13pt"
	}
	if b.Page.LogoWidthCover == "" {
		b.Page.LogoWidthCover = "46mm"
	}
	if b.Page.LogoWidthHeader == "" {
		b.Page.LogoWidthHeader = "16mm"
	}
	if b.Diagrams.D2Scale == 0 {
		b.Diagrams.D2Scale = 0.5
	}
	if b.Diagrams.D2Pad == 0 {
		b.Diagrams.D2Pad = 8
	}
	if b.Diagrams.MinTextPt == 0 {
		b.Diagrams.MinTextPt = 8
	}
	if b.Diagrams.MaxTextPt == 0 {
		b.Diagrams.MaxTextPt = 12
	}
	if b.Diagrams.MaxHeightMM == 0 {
		b.Diagrams.MaxHeightMM = 150
	}
}

// Load reads <dir>/<name>/brand.yaml. Name "none" yields the built-in default.
func Load(brandsDir, name string) (*Brand, error) {
	if name == "" || name == "none" {
		return Default(), nil
	}
	dir := filepath.Join(brandsDir, name)
	f := filepath.Join(dir, "brand.yaml")
	raw, err := os.ReadFile(f)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("brand %q not found: no %s\n  brands dir: %s\n  available: %s\n  create it with: mdbrand brand new %s",
				name, f, brandsDir, strings.Join(List(brandsDir), ", "), name)
		}
		return nil, err
	}
	var b Brand
	if err := yaml.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("%s: %w", f, err)
	}
	b.Dir = dir
	if b.Name == "" {
		b.Name = name
	}
	b.applyDefaults()
	return &b, nil
}

// List returns the bundle names found in brandsDir.
func List(brandsDir string) []string {
	ents, err := os.ReadDir(brandsDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(brandsDir, e.Name(), "brand.yaml")); err == nil {
			out = append(out, e.Name())
		}
	}
	return out
}

// LogoPath is the absolute path of the logo, or "" when the bundle has none.
func (b *Brand) LogoPath() string {
	if b.Logo == "" || b.Dir == "" {
		return ""
	}
	if filepath.IsAbs(b.Logo) {
		return b.Logo
	}
	return filepath.Join(b.Dir, b.Logo)
}

// DisplayFontDir returns the display font directory when it is usable, plus
// whether it is. A missing directory is not an error: the templates fall back
// to the body font, so a bundle stays usable on a machine without the licensed
// font installed.
func (b *Brand) DisplayFontDir() (string, bool) {
	d := b.Fonts.Display
	if d.Path == "" || d.Regular == "" {
		return "", false
	}
	p := d.Path
	if !strings.HasSuffix(p, string(os.PathSeparator)) {
		p += string(os.PathSeparator)
	}
	if _, err := os.Stat(filepath.Join(d.Path, d.Regular)); err != nil {
		return p, false
	}
	return p, true
}

var hexRe = regexp.MustCompile(`^[0-9A-Fa-f]{6}$`)

// Check validates a bundle and returns human-readable problems and warnings.
// It knows the two traps that cost real time: an SVG that is only a wrapper
// around a small bitmap, and text kept as <foreignObject>, which rsvg-convert
// silently drops.
func (b *Brand) Check() (problems, warnings []string) {
	add := func(dst *[]string, f string, a ...any) { *dst = append(*dst, fmt.Sprintf(f, a...)) }

	if b.Name == "" {
		add(&problems, "name: empty")
	}
	for label, v := range map[string]string{
		"colors.primary": b.Colors.Primary, "colors.text": b.Colors.Text, "colors.rule": b.Colors.Rule,
	} {
		if !hexRe.MatchString(v) {
			add(&problems, "%s: %q is not a 6-digit hex without '#'", label, v)
		}
	}

	logo := b.LogoPath()
	switch {
	case logo == "":
		add(&warnings, "logo: none declared — cover and header will carry type only")
	default:
		st, err := os.Stat(logo)
		if err != nil {
			add(&problems, "logo: %s: %v", logo, err)
			break
		}
		switch strings.ToLower(filepath.Ext(logo)) {
		case ".pdf", ".png":
			// Directly embeddable by xelatex.
		case ".svg":
			raw, err := os.ReadFile(logo)
			if err != nil {
				add(&problems, "logo: %v", err)
				break
			}
			s := string(raw)
			if strings.Contains(s, "<image") {
				add(&warnings, "logo: %s embeds a raster <image> — it is a bitmap in an SVG coat, "+
					"so it will look soft on a cover. Get the vector original.", filepath.Base(logo))
			}
			if strings.Contains(s, "data:img/") {
				add(&problems, "logo: %s uses the invalid MIME type data:img/... (it must be data:image/...); "+
					"rsvg-convert renders nothing and fails silently", filepath.Base(logo))
			}
			if strings.Contains(s, "<foreignObject") {
				add(&problems, "logo: %s contains <foreignObject>; rsvg-convert drops it without a word", filepath.Base(logo))
			}
		default:
			add(&problems, "logo: %s: unsupported extension (use .svg, .pdf or .png)", filepath.Base(logo))
		}
		if st.Size() < 300 {
			add(&warnings, "logo: %s is only %d bytes — suspiciously small for artwork", filepath.Base(logo), st.Size())
		}
	}

	if d := b.Fonts.Display; d.Family != "" || d.Path != "" {
		if _, ok := b.DisplayFontDir(); !ok {
			add(&warnings, "fonts.display: %s not found at %s — cover and header fall back to %s",
				d.Regular, d.Path, b.Fonts.Body)
		}
	}
	if b.Fonts.Body == "" {
		add(&problems, "fonts.body: empty")
	}
	if b.Diagrams.MinTextPt < 6 {
		add(&warnings, "diagrams.min_text_pt: %.1f is below the readable floor on paper (~7pt)", b.Diagrams.MinTextPt)
	}
	return problems, warnings
}

// Scaffold writes a starter bundle at <brandsDir>/<name>.
func Scaffold(brandsDir, name string) (string, error) {
	dir := filepath.Join(brandsDir, name)
	if _, err := os.Stat(filepath.Join(dir, "brand.yaml")); err == nil {
		return dir, fmt.Errorf("%s already has a brand.yaml", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return dir, err
	}
	tpl := fmt.Sprintf(`# %[1]s brand bundle — read by mdbrand.
# Assets stay here; commercially licensed fonts are referenced by path and
# never copied in, so this bundle can be shared without breaching a licence.
name: %[1]s
display_name: %[1]s

# Vector is what you want. An SVG that merely wraps a PNG will look soft on a
# cover; "mdbrand brand validate %[1]s" says so.
logo: logo.svg

colors:
  primary: "1F6FEB"   # rules and accents, 6-digit hex, no '#'
  text: "3A3A3A"      # cover and header type
  rule: "C8CCCE"      # hairlines

fonts:
  body: Inter         # fontconfig family; must cover the glyphs you type
  # display:          # optional brand font for cover and header
  #   family: Gotham
  #   path: /absolute/path/to/otf
  #   regular: Gotham-Light.otf
  #   bold: Gotham-Medium.otf

page:
  papersize: a4
  margin: 22mm
  linestretch: 1.125
  logo_width_cover: 46mm    # tune per logo: a tall mark needs less width
  logo_width_header: 16mm

diagrams:
  d2_theme: 0         # light: paper has no prefers-color-scheme
  d2_scale: 0.5       # pairs with **.style.font-size: 32 in the .d2
  min_text_pt: 8      # fail below this once scaled onto the page
  max_text_pt: 12     # and do not let a figure shout over the body text
  max_height_mm: 150  # a figure taller than this eats the page

footer: ""            # optional line under the cover rule
`, name)
	return dir, os.WriteFile(filepath.Join(dir, "brand.yaml"), []byte(tpl), 0o644)
}
