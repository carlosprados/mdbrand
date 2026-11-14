package build

import (
	"fmt"
	"strings"
)

// typstCellCheck measures every table cell's unbreakable runs against the
// cell. A word wider than its column printed into the next one, or off the
// paper, exit 0: Typst breaks a line only where one may break, and the
// read-back measures the margin, not the neighbour — "T4-26T1-27T2-27"
// came out as one word. A run ends at a space, or after a hyphen, dash or
// slash not followed by a digit, as Unicode's line breaking has it. Each run
// is measured in the cell's own style (the header's bold included). Its
// excess spills one way, or half each way when centred; once that passes
// the cell's own inset the run is in the next column's, and it is left as
// metadata for typstCellReport, so every one is named at once. Less is
// only the gap between columns narrowing: centred cells in real documents
// ran 7pt over and read fine. The rule returns the cell untouched: no
// position moves for it.
const typstCellCheck = `// A word wider than its column: see typstCellCheck in typst_cells.go.
#let mdbrand-cell-x = 6pt
#let mdbrand-runs(c, wrap) = {
  if c == none or type(c) != content { return () }
  let f = c.func()
  if c.has("text") and type(c.text) == str {
    c.text.matches(regex("(?:[^\\s\\-/–—]|[\\-/–—]\\d)+[\\-/–—]?")).map(m =>
      (t: m.text, c: wrap(if f == raw { raw(m.text) } else { m.text })))
  } else if c.has("children") {
    c.children.map(x => mdbrand-runs(x, wrap)).flatten()
  } else if f == strong {
    mdbrand-runs(c.body, x => wrap(strong(x)))
  } else if f == emph {
    mdbrand-runs(c.body, x => wrap(emph(x)))
  } else if c.has("body") {
    mdbrand-runs(c.body, wrap)
  } else if c.has("child") {
    mdbrand-runs(c.child, wrap)
  } else { () }
}
#show table.cell: it => layout(size => {
  let room = size.width - 2 * mdbrand-cell-x
  // auto is the alignment around the table: pandoc centres every one.
  let a = if it.align == auto { align.alignment } else { it.align }
  let h = if a.axis() == none { a.x } else if a.axis() == "horizontal" { a } else { left }
  // A centred run spills half its excess each side.
  let side = if h == center { 0.5 } else { 1 }
  it
  for r in mdbrand-runs(it.body, x => x) {
    let over = measure(r.c).width - room
    if over * side > mdbrand-cell-x { [#metadata((t: r.t, o: over))<mdbrand-cell-over>] }
  }
})
`

// typstCellReport stops the build at the document's end with every cell
// typstCellCheck found, in one message.
const typstCellReport = `#context {
  let over = query(<mdbrand-cell-over>)
  if over.len() > 0 {
    panic("MDBRAND-CELL-OVER " + over.map(m => str(m.location().page()) + "|" +
      m.value.t.replace("\"", "'").replace("~", "-") + "|" + repr(calc.round(m.value.o.pt(), digits: 1))).join("~"))
  }
}
`

// cellOverError words MDBRAND-CELL-OVER: page|run|pt entries, ~ apart.
func cellOverError(arg string) error {
	var lines []string
	for e := range strings.SplitSeq(arg, "~") {
		f := strings.Split(e, "|")
		if len(f) != 3 {
			continue
		}
		lines = append(lines, fmt.Sprintf("page %s: %q, %spt wider than its column", f[0], f[1], f[2]))
	}
	return fmt.Errorf(`%d word(s) in a table set into the next column or the margin:
  %s
Widen the column (longer dashes under its header), shorten the word, or give
it a space to break at`, len(lines), strings.Join(lines, "\n  "))
}
