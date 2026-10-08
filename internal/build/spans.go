package build

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/carlosprados/mdbrand/internal/brand"
)

// spanFilter colours [text]{.accent} in both outputs and reports any other
// colour asked of a span or div, which pandoc would drop in silence.
//
//go:embed spans.lua
var spanFilter []byte

const spanFilterName = "spans.lua"

// writeSpanFilter puts the filter in the work directory and returns the pandoc
// flag that runs it. The filter names brandPrimary; a bundle whose accent is
// a colour of its own gets brandAccent instead, and one without keeps the
// filter, and so the .tex, exactly as it was.
func writeSpanFilter(work string, b *brand.Brand) (string, error) {
	filter := spanFilter
	if b.Colors.Accent != b.Colors.Primary {
		filter = bytes.ReplaceAll(filter, []byte(`\\textcolor{brandPrimary}`), []byte(`\\textcolor{brandAccent}`))
	}
	if err := os.WriteFile(filepath.Join(work, spanFilterName), filter, 0o644); err != nil {
		return "", err
	}
	return "--lua-filter=" + spanFilterName, nil
}

var spanRe = regexp.MustCompile(`MDBRAND-SPAN ([^\t\n]+)\t([^\n]*)`)

// spanProblems turns the filter's report into the build's refusal.
func spanProblems(pandocOut string) error {
	ms := spanRe.FindAllStringSubmatch(pandocOut, -1)
	if len(ms) == 0 {
		return nil
	}
	var lines []string
	for _, m := range ms {
		lines = append(lines, fmt.Sprintf("%s on %q", m[1], ellipsize(strings.TrimSpace(m[2]), 50)))
	}
	return fmt.Errorf(`%d colour(s) asked for in a way that neither the PDF nor the .docx can print:
  %s
pandoc drops colour attributes without a word, so the text would come out
black, and the accent colours words, not blocks. Colour words with the brand's
accent, which both outputs print:
  [words]{.accent}`, len(ms), strings.Join(lines, "\n  "))
}

// minAccentContrast is WCAG's AA ratio for text, which accented words are.
const minAccentContrast = 4.5

// accentContrast measures the accent against the white page when the
// document prints words in it. A deck stops, since a projector washes a pale
// colour out to nothing; a page warns, because a pale accent has always
// printed and a document that built yesterday must still build.
func accentContrast(b *brand.Brand, pandocOut string, deck bool, rep *Report) error {
	if !strings.Contains(pandocOut, "MDBRAND-ACCENT") {
		return nil
	}
	c := brand.Contrast(b.Colors.Accent, "FFFFFF")
	if c >= minAccentContrast {
		return nil
	}
	msg := fmt.Sprintf(`[words]{.accent} print in %s, %.1f:1 against the white page, under the
%.1f:1 that text needs to be read. Set colors.accent in the bundle to a darker
shade for words, and keep the primary for rules: %s, the same hue as dark as
it needs to be, reaches %.1f:1`,
		b.Colors.Accent, c, minAccentContrast,
		brand.Darken(b.Colors.Accent, minAccentContrast),
		brand.Contrast(brand.Darken(b.Colors.Accent, minAccentContrast), "FFFFFF"))
	if deck {
		return fmt.Errorf("%s", msg)
	}
	rep.Warnings = append(rep.Warnings, msg)
	return nil
}
