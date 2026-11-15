package build

// typstTable is the template's mdbrand-table, which every pandoc table goes
// through with its row count (typst.lua).
//
// pandoc puts every table in a figure, and a figure never breaks: a row
// taller than the page ran through the footer. So a long table breaks, and
// a short one that fits is kept whole, since Typst cannot keep rows
// together across a break.
//
// A table that breaks broke between its caption and its first row: the
// heading, sticky to the figure, and the caption sat at the foot of a page
// and every row went over — a client report and an offer, exit 0. So
// the caption, the header and the first TableKeep rows are measured as a
// table of their own, and that height is reserved before the figure: an
// unbreakable sticky block that goes to the next page when it does not fit,
// taking the heading with it, and is then taken back by as much, so a
// table that fits moves by nothing. It is XeLaTeX's needspace, in Typst.
//
// Its last rows broke the same way: an observatorio report's table left
// one row overleaf, after two charts. The last TableKeep rows are a footer
// that does not repeat, which Typst sets whole and at the end: they go over
// together, as XeLaTeX's \\* keeps them.
//
// A table rebuilt here is a new element, and the template's show-set rules
// apply to it again: a size in em shrank its text twice, to 7pt where
// XeLaTeX sets 9. The table's size is absolute for that.
const typstTable = `#let mdbrand-table-head(t, k) = {
  let cols = if type(t.columns) == array { t.columns.len() } else if type(t.columns) == int { t.columns } else { 1 }
  let f = t.fields()
  let children = f.remove("children")
  let kept = ()
  let n = 0
  for c in children {
    let g = c.func()
    if g == table.header or g == table.hline or g == table.vline {
      kept.push(c)
    } else if g != table.footer and n < k * cols {
      kept.push(c)
      n += if g == table.cell and c.has("colspan") { c.colspan } else { 1 }
    }
  }
  [#table(..f, ..kept) <mdbrand-head>]
}
#let mdbrand-table-tail(t, k) = {
  let cols = if type(t.columns) == array { t.columns.len() } else if type(t.columns) == int { t.columns } else { 1 }
  let f = t.fields()
  let children = f.remove("children")
  // From the end: the last k rows' cells, and any rule among or after them.
  let n = 0
  let i = children.len()
  while i > 0 and n < k * cols {
    let c = children.at(i - 1)
    let g = c.func()
    if g == table.header or g == table.footer { break }
    if g != table.hline and g != table.vline {
      n += if g == table.cell and c.has("colspan") { c.colspan } else { 1 }
    }
    i -= 1
  }
  if n < k * cols { return [#table(..f, ..children) <mdbrand-tail>] }
  [#table(..f, ..children.slice(0, i), table.footer(repeat: false, ..children.slice(i))) <mdbrand-tail>]
}
#let mdbrand-table(rows, captioned, body) = context {
  set figure(numbering: none, outlined: false) if not captioned
  let w = page.width - 2 * margin
  let h = measure(block(width: w, body)).height
  if rows < 2 * [[.TableKeep]] and h <= page.height - 2 * margin {
    body
  } else {
    let need = measure(block(width: w, {
      show table: t => if t.at("label", default: none) == <mdbrand-head> { t } else { mdbrand-table-head(t, [[.TableKeep]]) }
      body
    })).height
    block(sticky: true, breakable: false, above: 0pt, below: 0pt, height: need)
    v(-need)
    show figure: set block(breakable: true)
    show table: t => if t.at("label", default: none) == <mdbrand-tail> { t } else if rows >= 2 * [[.TableKeep]] { mdbrand-table-tail(t, [[.TableKeep]]) } else { t }
    body
  }
}
`
