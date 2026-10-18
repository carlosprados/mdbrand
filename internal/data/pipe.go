package data

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/carlosprados/mdbrand/internal/mdtext"
)

// pandocColumns is pandoc's --columns default: a pipe table with a line wider
// than this sizes its columns by the separator's dashes.
const pandocColumns = 72

var (
	pipeSep   = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)+\|?\s*$`)
	dashRun   = regexp.MustCompile(`-+`)
	citeGroup = regexp.MustCompile(`\[[^\[\]]*@[^\[\]]*\]`)
)

// SizePipeTables gives a hand-written pipe table the column widths a data
// table gets. Once a row is wider than pandoc's 72 columns, pandoc sizes the
// columns in proportion to the separator's dashes, and the usual |---|---|
// made them all equal: a column of long names wrapped to four lines beside
// numbers with room to spare. A separator whose dashes are all the same
// length said nothing about widths, so it is rewritten from the cells; one
// with different lengths is the author's choice and kept.
func SizePipeTables(body string) string {
	prot := mdtext.Protected(body)
	lines := strings.SplitAfter(body, "\n")
	off := make([]int, len(lines))
	for i, at := 1, 0; i < len(lines); i++ {
		at += len(lines[i-1])
		off[i] = at
	}
	for i := 1; i < len(lines); i++ {
		sep := strings.TrimRight(lines[i], "\n")
		if !pipeSep.MatchString(sep) || mdtext.Inside(prot, off[i]) || !strings.Contains(lines[i-1], "|") {
			continue
		}
		runs := dashRun.FindAllString(sep, -1)
		if !sameLength(runs) {
			continue
		}
		end := i + 1
		for end < len(lines) && strings.Contains(lines[end], "|") && strings.TrimSpace(lines[end]) != "" {
			end++
		}
		table := append([]string{lines[i-1]}, lines[i+1:end]...)
		wide := false
		var cells [][]string
		for _, ln := range table {
			ln = strings.TrimRight(ln, "\n")
			wide = wide || utf8.RuneCountInString(ln) > pandocColumns
			cells = append(cells, splitRow(ln))
		}
		if !wide {
			continue
		}
		width := make([]int, len(runs))
		for _, row := range cells {
			for c, v := range row {
				if c < len(width) {
					width[c] = max(width[c], cellWidth(v))
				}
			}
		}
		lines[i] = resize(sep, width) + "\n"
		i = end - 1
	}
	return strings.Join(lines, "")
}

func sameLength(runs []string) bool {
	for _, r := range runs[1:] {
		if len(r) != len(runs[0]) {
			return false
		}
	}
	return true
}

// splitRow cuts a row at its unescaped bars, dropping the empty cells the
// leading and trailing bars leave.
func splitRow(ln string) []string {
	ln = strings.TrimSpace(ln)
	var cells []string
	from := 0
	for i := 0; i < len(ln); i++ {
		switch ln[i] {
		case '\\':
			i++
		case '|':
			cells = append(cells, strings.TrimSpace(ln[from:i]))
			from = i + 1
		}
	}
	cells = append(cells, strings.TrimSpace(ln[from:]))
	if strings.HasPrefix(ln, "|") {
		cells = cells[1:]
	}
	if len(cells) > 0 && from == len(ln) {
		cells = cells[:len(cells)-1]
	}
	return cells
}

// cellWidth is the room a cell asks for, by the rule pipeTable follows: its
// length, and never less than its longest word and a little air. A citation
// is measured as the number it prints: measured by its key, a column of
// [@enc-20261011-sociometrica-el-espanol] took the width of eight others and
// their headers overflowed the measure.
func cellWidth(v string) int {
	v = citeGroup.ReplaceAllString(v, "[00]")
	w := max(3, utf8.RuneCountInString(v))
	for f := range strings.FieldsSeq(v) {
		w = max(w, utf8.RuneCountInString(f)+2)
	}
	return w
}

// resize writes sep again with each column's dashes width[c] long, keeping
// its alignment colons.
func resize(sep string, width []int) string {
	c := 0
	return dashRun.ReplaceAllStringFunc(sep, func(string) string {
		w := 3
		if c < len(width) {
			w = width[c]
		}
		c++
		return strings.Repeat("-", w)
	})
}
