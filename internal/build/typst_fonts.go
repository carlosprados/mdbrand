package build

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/exit"
	"github.com/carlosprados/mdbrand/internal/mdtext"
	"github.com/carlosprados/mdbrand/internal/run"
)

// typstFaces are the only faces Typst may set a document in, as Typst names
// them. fontspec loads a face by its file; Typst asks for a family and a
// weight, and Gotham's files say Light is 500 and Medium 350, as do XLight,
// Thin and Ultra: with the machine's fonts in view, "Gotham at 500" printed
// Thin, and a family name it did not know printed Inter, both at exit 0. So
// Typst sees a directory of the declared files alone, with the machine's
// fonts ignored, and is asked for each face by the weight its own file gives.
type typstFaces struct {
	Dir                  string // the work directory's font directory, for --font-path
	Body                 string // Typst's family for the body face
	Fallback             string // fonts.fallback's family, "" without one
	Display, DisplayBold string // the display faces' families; the body's without one
	Weight, BoldWeight   int    // the weights that select them
	// FallbackStyles are the styles the fallback family has beyond its
	// regular: "bold", "italic". Typst sets a character it takes from the
	// fallback in the nearest face it has, and says nothing.
	FallbackStyles []string
}

// typstFontsDir is the work directory's font directory.
const typstFontsDir = "fonts"

// typstVariant is one face as `typst fonts --variants` lists it.
type typstVariant struct {
	family string
	weight int
	italic bool
}

// typstFontSet links the bundle's faces into the work directory and asks
// Typst what it calls them.
func typstFontSet(work string, b *brand.Brand) (typstFaces, error) {
	dir := filepath.Join(work, typstFontsDir)
	// The notes are compiled after the deck, in the same work directory.
	if err := os.RemoveAll(dir); err != nil {
		return typstFaces{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return typstFaces{}, err
	}
	n := 0
	link := func(file string) error {
		n++
		abs, err := filepath.Abs(file)
		if err != nil {
			return err
		}
		return os.Symlink(abs, filepath.Join(dir, fmt.Sprintf("%02d-%s", n, filepath.Base(file))))
	}
	bodyFile, bodyOK := "", b.Fonts.Body != "" && b.BodyFontInstalled() && run.Have("fc-list")
	if bodyOK {
		if err := linkFamily(b.Fonts.Body, link); err != nil {
			return typstFaces{}, err
		}
		bodyFile = matchFile(b.Fonts.Body)
	}
	fallbackFile := ""
	if fb := b.Fonts.Fallback; fb != "" {
		if _, ok := brand.FontCharset(fb); !ok {
			return typstFaces{}, exit.AsEnvironment(fmt.Errorf(`the %s bundle names %q as fonts.fallback, and fontconfig cannot find it.
Install it, or remove fonts.fallback and let the missing-glyph check name the
characters the body face lacks`, b.Name, fb))
		}
		if err := linkFamily(fb, link); err != nil {
			return typstFaces{}, err
		}
		fallbackFile = matchFile(fb)
	}
	res, hasDisplay := b.ResolveDisplay()
	var regular, bold string
	if hasDisplay {
		regular, bold = filepath.Join(res.RegularDir, res.RegularFile), filepath.Join(res.BoldDir, res.BoldFile)
		if err := link(regular); err != nil {
			return typstFaces{}, err
		}
		if bold != regular {
			if err := link(bold); err != nil {
				return typstFaces{}, err
			}
		}
	}

	out, err := run.Cmd(work, "typst", "fonts", "--ignore-system-fonts", "--font-path", dir, "--variants")
	if err != nil {
		return typstFaces{}, err
	}
	seen := parseTypstFonts(out)
	at := func(file string) (typstVariant, bool) {
		real, err := filepath.EvalSymlinks(file)
		if err != nil {
			return typstVariant{}, false
		}
		v, ok := seen[real]
		return v, ok
	}

	f := typstFaces{Dir: dir, Body: typstSerif, Weight: 400, BoldWeight: 700}
	if v, ok := at(bodyFile); ok {
		f.Body = v.family
	}
	if v, ok := at(fallbackFile); ok {
		f.Fallback = v.family
		f.FallbackStyles = familyStyles(seen, v.family)
	}
	f.Display, f.DisplayBold = f.Body, f.Body
	if !hasDisplay {
		return f, nil
	}
	r, okR := at(regular)
	bo, okB := at(bold)
	if !okR || !okB {
		return typstFaces{}, fmt.Errorf("fonts.display: Typst cannot read %s or %s as a font", filepath.Base(regular), filepath.Base(bold))
	}
	if bold != regular && r.family == bo.family && r.weight == bo.weight {
		return typstFaces{}, fmt.Errorf(`fonts.display: %s and %s both call themselves %s at weight %d, and Typst picks
  a face by family and weight alone: it cannot set one as regular and the other as bold.
  Name faces of different weights, or build with the LaTeX engine`,
			res.RegularFile, res.BoldFile, r.family, r.weight)
	}
	f.Display, f.Weight = r.family, r.weight
	f.DisplayBold, f.BoldWeight = bo.family, bo.weight
	return f, nil
}

// familyStyles are the styles beyond the regular that a family's faces give.
func familyStyles(seen map[string]typstVariant, family string) []string {
	var styles []string
	for _, v := range seen {
		if v.family != family {
			continue
		}
		if v.weight >= 600 && !slices.Contains(styles, "bold") {
			styles = append(styles, "bold")
		}
		if v.italic && !slices.Contains(styles, "italic") {
			styles = append(styles, "italic")
		}
	}
	return styles
}

// linkFamily links every file of a family.
func linkFamily(family string, link func(string) error) error {
	files, err := familyFiles(family)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := link(f); err != nil {
			return err
		}
	}
	return nil
}

// familyFiles lists the files fontconfig holds for a family.
func familyFiles(family string) ([]string, error) {
	esc := strings.NewReplacer(`\`, `\\`, "-", `\-`, ":", `\:`, ",", `\,`).Replace(family)
	out, err := run.Cmd("", "fc-list", "--format", "%{file}\n", ":family="+esc)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" && !slices.Contains(files, l) {
			files = append(files, l)
		}
	}
	return files, nil
}

// matchFile is the file fontconfig gives for a family's regular face.
func matchFile(family string) string {
	out, err := run.Cmd("", "fc-match", "--format", "%{file}", family)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

var (
	typstFontPathRe  = regexp.MustCompile(`^\s*[├└]\s+(\S.*)$`)
	typstFontStyleRe = regexp.MustCompile(`Style: (\w+), Weight: (\d+)`)
)

// parseTypstFonts reads `typst fonts --variants`: a family at the margin, then
// each of its files with its style and weight on the line below.
func parseTypstFonts(out string) map[string]typstVariant {
	seen := map[string]typstVariant{}
	family, file := "", ""
	for _, l := range strings.Split(out, "\n") {
		switch {
		case l == "":
		case !strings.HasPrefix(l, " ") && !strings.ContainsAny(l[:1], "│├└"):
			family = strings.TrimSpace(l)
		case typstFontPathRe.MatchString(l):
			file = typstFontPathRe.FindStringSubmatch(l)[1]
		default:
			if m := typstFontStyleRe.FindStringSubmatch(l); m != nil && file != "" {
				w, _ := strconv.Atoi(m[2])
				if real, err := filepath.EvalSymlinks(file); err == nil {
					file = real
				}
				if _, dup := seen[file]; !dup {
					seen[file] = typstVariant{family: family, weight: w, italic: m[1] != "Normal"}
				}
				file = ""
			}
		}
	}
	return seen
}

// typstOwnCharset is the part of text's characters that a face Typst carries
// inside itself can set. fontconfig cannot say: without Inter, the built-in
// bundle's body is Libertinus Serif, and ☐ came out of the code face, exit 0.
// So Typst is asked, one character to a page, and pdffonts reads which face
// each page used.
func typstOwnCharset(f typstFaces, family, text string) (brand.Charset, error) {
	cs := brand.Charset{{0x20, 0x7e}}
	var runes []rune
	seen := map[rune]bool{}
	for _, r := range text {
		if r >= 0x80 && !seen[r] && !unicode.IsSpace(r) && !unicode.IsControl(r) {
			seen[r] = true
			runes = append(runes, r)
		}
	}
	if len(runes) == 0 {
		return cs, nil
	}
	dir, err := os.MkdirTemp("", "mdbrand-probe-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	var src strings.Builder
	fmt.Fprintf(&src, "#set page(width: 20mm, height: 20mm, margin: 1mm)\n#set text(font: %q)\n", family)
	for i, r := range runes {
		if i > 0 {
			src.WriteString("#pagebreak()\n")
		}
		fmt.Fprintf(&src, "#str.from-unicode(%d)\n", r)
	}
	if err := os.WriteFile(filepath.Join(dir, "probe.typ"), []byte(src.String()), 0o644); err != nil {
		return nil, err
	}
	if _, err := run.Cmd(dir, "typst", "compile", "--ignore-system-fonts", "--font-path", f.Dir, "probe.typ", "probe.pdf"); err != nil {
		return nil, err
	}
	face := strings.ReplaceAll(family, " ", "")
	for i, r := range runes {
		page := strconv.Itoa(i + 1)
		out, err := run.Stdout(dir, "pdffonts", "-f", page, "-l", page, "probe.pdf")
		if err != nil {
			return nil, err
		}
		if strings.Contains(string(out), face) {
			cs = append(cs, [2]rune{r, r})
		}
	}
	return cs, nil
}

// typstGlyphs is Typst's missing-glyph check, made before it runs. Typst says
// nothing of a glyph its faces lack: it draws it from any face it has —
// ⭐ came out of its math face and ☐ out of its code face, exit 0 — so the
// faces are compared with the text first, each with the text it sets.
func typstGlyphs(f typstFaces, body string, display, plain []string, allow bool) error {
	cover := func(text string, families ...string) (brand.Charset, bool) {
		var all brand.Charset
		for _, fam := range families {
			if fam == "" {
				continue
			}
			if fam == typstSerif {
				// Typst's own face: fontconfig has never seen it.
				c, err := typstOwnCharset(f, fam, text)
				if err != nil {
					return nil, false
				}
				all = append(all, c...)
				continue
			}
			c, ok := brand.FontCharset(fam)
			if !ok {
				return nil, false
			}
			all = append(all, c...)
		}
		return all, len(all) > 0
	}
	var prose, code strings.Builder
	last := 0
	for _, sp := range mdtext.Protected(body) {
		prose.WriteString(body[last:sp[0]])
		if body[sp[0]] != '$' {
			code.WriteString(body[sp[0]:sp[1]] + "\n")
		}
		last = sp[1]
	}
	prose.WriteString(body[last:])
	var found []string
	for _, set := range []struct {
		role     string
		text     string
		families []string
	}{
		{"body", strings.Join(append([]string{prose.String()}, plain...), "\n"), []string{f.Body, f.Fallback}},
		{"display", strings.Join(display, "\n"), []string{f.Display, f.DisplayBold, f.Body, f.Fallback}},
		{"code", code.String(), []string{typstMono}},
	} {
		cov, ok := cover(set.text, set.families...)
		if !ok || strings.TrimSpace(set.text) == "" {
			continue
		}
		if holes := missingGlyphs(cov, set.text); len(holes) > 0 {
			found = append(found, fmt.Sprintf("the %s face (%s) has no glyph for:\n    %s",
				set.role, strings.Join(slices.DeleteFunc(slices.Clone(set.families), func(s string) bool { return s == "" }), " + "),
				strings.Join(holes, "\n    ")))
		}
	}
	if len(found) == 0 || allow {
		return nil
	}
	return fmt.Errorf(`%s
  Typst would draw them from a face nobody chose. Fix the text, pick a body font
  that covers them, or name a fonts.fallback in the bundle that does; override
  with --allow-missing-glyphs if you truly want that`, strings.Join(found, "\n  "))
}

var typstStyledRe = regexp.MustCompile(`MDBRAND-STYLED (bold|italic|heading) ([^\n]*)`)

// typstFallbackStyles stops on a character that goes to fonts.fallback inside
// bold or italic the fallback family has no face for. Typst set it in the
// fallback's regular at exit 0; XeLaTeX's NFSS reports the same substitution
// and the build stops there, so the two engines refuse the same document.
// typst.lua reports the text of every strong, emph and heading on stderr; a
// heading is asked of the fallback at the display bold's weight, which is
// bold from 600.
func typstFallbackStyles(f typstFaces, b *brand.Brand, pandocOut string) error {
	if f.Fallback == "" || !b.BodyFontInstalled() {
		return nil
	}
	// fontconfig's names, not Typst's: Typst may call a face by its typographic
	// family, which fontconfig gives another face's coverage for.
	body, okB := brand.FontCharset(b.Fonts.Body)
	fb, okF := brand.FontCharset(b.Fonts.Fallback)
	if !okB || !okF {
		return nil
	}
	lost := fallbackStyleLosses(pandocOut, body, fb, f.FallbackStyles, f.BoldWeight)
	if len(lost) == 0 {
		return nil
	}
	return fmt.Errorf(`fonts.fallback has no face for a style the document sets its characters in,
so Typst would print them in the regular:
  %s: %s
Take the characters out of the bold, italic or heading, or name a fonts.fallback with that style`,
		b.Fonts.Fallback, strings.Join(lost, "\n  "+b.Fonts.Fallback+": "))
}

// fallbackStyleLosses reads typst.lua's reports and names, by style, the
// characters set in the fallback in a style it lacks: "bold, for ⛔ (U+26D4)".
func fallbackStyleLosses(out string, body, fb brand.Charset, has []string, boldWeight int) []string {
	lost := map[string][]string{}
	var order []string
	for _, m := range typstStyledRe.FindAllStringSubmatch(out, -1) {
		style := m[1]
		if style == "heading" {
			if boldWeight < 600 {
				continue
			}
			style = "bold"
		}
		if slices.Contains(has, style) {
			continue
		}
		for _, r := range brand.FallbackRunes(m[2], body, fb) {
			c := fmt.Sprintf("%c (U+%04X)", r, r)
			if _, ok := lost[style]; !ok {
				order = append(order, style)
			}
			if !slices.Contains(lost[style], c) {
				lost[style] = append(lost[style], c)
			}
		}
	}
	lines := make([]string, 0, len(order))
	for _, style := range order {
		lines = append(lines, style+", for "+strings.Join(lost[style], ", "))
	}
	return lines
}
