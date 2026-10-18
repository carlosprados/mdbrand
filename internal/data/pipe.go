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
		lines[i] = resize(sep, colWidths(cells, len(runs))) + "\n"
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

// measureChars is the measure assumed in characters of a table's type, so a
// column's narrowest share can be told from the dashes: A4 at footnotesize
// holds about a hundred, and assuming fewer only widens the short columns.
const measureChars = 80

// unbroken is the longest cell kept on one line whole: a date or a short
// label wrapped at its hyphen ("12-" over "oct") reads as two things.
const unbroken = 12

// colWidths gives each column its dashes. Each first gets the least it can
// be set in — its longest word and a little air, or a short cell whole —
// and what the measure has left is shared by how much more text each holds.
// Shares by text alone gave a column of dates 3% of the measure beside two
// of long sentences: "Pendiente" ran 19pt into the next column.
func colWidths(cells [][]string, n int) []int {
	least, most := make([]int, n), make([]int, n)
	for _, row := range cells {
		for c, v := range row {
			if c >= n {
				continue
			}
			l, m := cellRoom(v)
			least[c], most[c] = max(least[c], l), max(most[c], m)
		}
	}
	sumLeast, sumMost, extra := 0, 0, 0
	for c := range n {
		sumLeast += least[c]
		sumMost += most[c]
		extra += max(0, most[c]-least[c])
	}
	if sumMost <= measureChars || sumLeast >= measureChars || extra == 0 {
		return most
	}
	width := make([]int, n)
	room := measureChars - sumLeast
	for c := range n {
		width[c] = least[c] + room*max(0, most[c]-least[c])/extra
	}
	return width
}

// cellRoom is the least a cell can be set in and the most it would take. A
// citation is measured as the number it prints: measured by its key, a
// column of [@enc-20261011-sociometrica-el-espanol] took the width of eight
// others and their headers overflowed the measure.
func cellRoom(v string) (least, most int) {
	v = citeGroup.ReplaceAllString(v, "[00]")
	n := utf8.RuneCountInString(v)
	least = 3
	if n <= unbroken {
		least = max(least, n+2)
	}
	for f := range strings.FieldsSeq(v) {
		least = max(least, utf8.RuneCountInString(f)+2)
	}
	// The most stays what it was before least existed, so a table that fits
	// the measure keeps its dashes byte for byte.
	most = max(3, n)
	for f := range strings.FieldsSeq(v) {
		most = max(most, utf8.RuneCountInString(f)+2)
	}
	return least, most
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
