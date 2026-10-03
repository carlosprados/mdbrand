package build

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// spanFilter colours [text]{.accent} in both outputs and reports any other
// colour asked of a span or div, which pandoc would drop in silence.
//
//go:embed spans.lua
var spanFilter []byte

const spanFilterName = "spans.lua"

// writeSpanFilter puts the filter in the work directory and returns the pandoc
// flag that runs it.
func writeSpanFilter(work string) (string, error) {
	if err := os.WriteFile(filepath.Join(work, spanFilterName), spanFilter, 0o644); err != nil {
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
