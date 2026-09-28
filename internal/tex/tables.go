package tex

import "strings"

// TableKeep is the fewest body rows a table may leave on either side of a
// page break, so a table of fewer than twice this never breaks at all.
const TableKeep = 3

// KeepTableRows gives pandoc's longtables widow and orphan control. longtable
// may break a page after any row, and pandoc writes every table as one — so a
// three-row table landed with its caption, header and first row at the foot
// of a page and two rows alone on the next. Here the end of every body row
// that would leave fewer than TableKeep rows on one side becomes \\*, which
// longtable does not break at. A long table still breaks, just not badly.
//
// It works on pandoc's own output, whose shape is regular: the body runs from
// \endlastfoot to \end{longtable}, and a row ends with " \\" at the end of a
// line. A line that does not end that way is part of a row and left alone.
func KeepTableRows(src string) string {
	lines := strings.Split(src, "\n")
	inBody := false
	var rows []int // line index of each row end in the current body
	flush := func() {
		for j, at := range rows {
			if j+1 < TableKeep || len(rows)-(j+1) < TableKeep {
				lines[at] = strings.TrimSuffix(lines[at], `\\`) + `\\*`
			}
		}
		rows = rows[:0]
	}
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		switch {
		case t == `\endlastfoot`:
			inBody = true
		case strings.HasPrefix(t, `\end{longtable}`):
			if inBody {
				flush()
			}
			inBody = false
		case inBody && strings.HasSuffix(ln, ` \\`):
			rows = append(rows, i)
		}
	}
	return strings.Join(lines, "\n")
}
