package build

import (
	"fmt"
	"html"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/carlosprados/mdbrand/internal/run"
	"github.com/carlosprados/mdbrand/internal/tex"
)

var captionRe = regexp.MustCompile(`MDBRAND-CAPTION\t([^\n]*)`)

// reportedCaptions are the tables' captions spans.lua reported.
func reportedCaptions(pandocOut string) []string {
	var cs []string
	for _, m := range captionRe.FindAllStringSubmatch(pandocOut, -1) {
		cs = append(cs, m[1])
	}
	return cs
}

// checkPageFoot stops a PDF where a page ends on a heading or on a table's
// caption, its text or its rows overleaf. Each engine keeps them together —
// XeLaTeX reserves room before a heading and longtable's first rows, Typst
// makes headings sticky and reserves a breaking table's caption, header and
// first rows — and each once failed at it: a run of headings in XeLaTeX, a
// caption in Typst. An editor marks either on the proof; this reads the
// proof, the same way for both engines.
func checkPageFoot(p *prepared, work, pdf, pandocOut string) error {
	if p.style == tex.Slides || !run.Have("pdftotext") {
		return nil
	}
	out, err := run.Stdout(work, "pdftotext", "-enc", "UTF-8", "-bbox-layout", pdf, "-")
	if err != nil {
		return err
	}
	var heads []string
	for _, h := range reportedHeadings(pandocOut) {
		heads = append(heads, h.text)
	}
	bad := strandedAtFoot(string(out), heads, reportedCaptions(pandocOut))
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf(`%d page(s) end on a heading or a table's caption, with what it introduces
on the next page:
  %s
mdbrand keeps each with what follows it, and this one escaped: that is
mdbrand's defect, not the document's. Build with --work ./out and report it
with the document at https://github.com/carlosprados/mdbrand/issues. Until
then, ::: {.keep} around the heading and its first paragraph keeps them
together`, len(bad), strings.Join(bad, "\n  "))
}

var (
	pageLayoutRe = regexp.MustCompile(`<page width="[0-9.]+" height="([0-9.]+)">`)
	lineRe       = regexp.MustCompile(`(?s)<line xMin="([0-9.]+)" yMin="([0-9.]+)" xMax="[0-9.]+" yMax="[0-9.]+">(.*?)</line>`)
	wordTextRe   = regexp.MustCompile(`>([^<]*)</word>`)
)

// proofRow is one line of a page as the eye reads it: every piece of text
// pdftotext set at the same height, left to right.
type proofRow struct {
	x, y float64
	text string
}

// strandedAtFoot names each page whose last row of text above the running
// footer is a heading's, or a table caption's, last line. The footer is
// found as the height most pages end at — the cover and a page without one
// are not judged — and the last page is not, since nothing follows it.
// Text is folded as lostHeadings folds it. A heading starts at the text's
// left edge and may carry its number ("3.", "2.1", "A.1"): a bullet that
// repeats a heading's name is indented, and "Albacete 2023, 4 escaños."
// is not section 20234. A caption carries its label ("Tabla 4:") always.
func strandedAtFoot(bbox string, heads, captions []string) []string {
	pages := proofPages(bbox)
	foot := footerHeight(pages)
	if math.IsNaN(foot) {
		return nil
	}
	left := leftEdge(pages)
	var out []string
	for i, rows := range pages[:max(0, len(pages)-1)] {
		if len(rows) < 2 || math.Abs(rows[len(rows)-1].y-foot) > 1 {
			continue
		}
		body := rows[:len(rows)-1]
		for len(body) > 0 && math.Abs(body[len(body)-1].y-foot) <= 1 {
			body = body[:len(body)-1]
		}
		if what, ok := endsOn(body, heads, "the heading", left, isNumber); ok {
			out = append(out, fmt.Sprintf("page %d ends on %s; its text is on page %d", i+1, what, i+2))
		} else if what, ok := endsOn(body, captions, "the caption", math.NaN(), isLabel); ok {
			out = append(out, fmt.Sprintf("page %d ends on %s; its table is on page %d", i+1, what, i+2))
		}
	}
	return out
}

// endsOn says whether the last one to three rows are one of texts, whole,
// after what label allows, starting at left unless left is NaN.
func endsOn(body []proofRow, texts []string, kind string, left float64, label func(string) bool) (string, bool) {
	for k := 1; k <= 3 && k <= len(body); k++ {
		if !math.IsNaN(left) && math.Abs(body[len(body)-k].x-left) > 2 {
			continue
		}
		var parts []string
		for _, r := range body[len(body)-k:] {
			parts = append(parts, r.text)
		}
		s := foldText(strings.Join(parts, " "))
		for _, t := range texts {
			f := foldText(t)
			if utf8.RuneCountInString(f) < 2 || !strings.HasSuffix(s, f) {
				continue
			}
			if label(strings.TrimSuffix(s, f)) {
				return fmt.Sprintf("%s %q", kind, ellipsize(t, 60)), true
			}
		}
	}
	return "", false
}

// isNumber is what may print before a heading's text, folded: nothing, a
// number ("21" for 2.1.) or an appendix's letter and number ("a1").
func isNumber(s string) bool {
	rs := []rune(s)
	if len(rs) > 6 {
		return false
	}
	for i, r := range rs {
		if !unicode.IsDigit(r) && !(i == 0 && unicode.IsLetter(r)) {
			return false
		}
	}
	return len(rs) != 1 || unicode.IsDigit(rs[0])
}

// isLabel is what prints before a caption's text, folded: a word and a
// number, "tabla4" or "table21".
func isLabel(s string) bool {
	rs := []rune(s)
	i := 0
	for i < len(rs) && unicode.IsLetter(rs[i]) {
		i++
	}
	if i < 3 || i == len(rs) || len(rs) > 14 {
		return false
	}
	for _, r := range rs[i:] {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// leftEdge is where most rows start: the text block's left margin.
func leftEdge(pages [][]proofRow) float64 {
	count := map[float64]int{}
	for _, rows := range pages {
		for _, r := range rows {
			count[math.Round(r.x)]++
		}
	}
	best, n := math.NaN(), 0
	for x, c := range count {
		if c > n || (c == n && x < best) {
			best, n = x, c
		}
	}
	return best
}

// proofPages reads pdftotext's -bbox-layout output into each page's rows,
// top to bottom: lines within 2pt of the same height are one row.
func proofPages(bbox string) [][]proofRow {
	var pages [][]proofRow
	for _, page := range strings.Split(bbox, "<page ")[1:] {
		if pageLayoutRe.FindStringSubmatch("<page "+page) == nil {
			continue
		}
		var ps []proofRow
		for _, l := range lineRe.FindAllStringSubmatch(page, -1) {
			x, _ := strconv.ParseFloat(l[1], 64)
			y, _ := strconv.ParseFloat(l[2], 64)
			var ws []string
			for _, w := range wordTextRe.FindAllStringSubmatch(l[3], -1) {
				ws = append(ws, html.UnescapeString(w[1]))
			}
			ps = append(ps, proofRow{x, y, strings.Join(ws, " ")})
		}
		sort.SliceStable(ps, func(a, b int) bool {
			if math.Abs(ps[a].y-ps[b].y) > 2 {
				return ps[a].y < ps[b].y
			}
			return ps[a].x < ps[b].x
		})
		var rows []proofRow
		for _, p := range ps {
			if n := len(rows); n > 0 && math.Abs(rows[n-1].y-p.y) <= 2 {
				rows[n-1].text += " " + p.text
				continue
			}
			rows = append(rows, p)
		}
		pages = append(pages, rows)
	}
	return pages
}

// footerHeight is the height, to the point, at which most pages' last row
// sits: the running footer. NaN when no height is shared by two pages.
func footerHeight(pages [][]proofRow) float64 {
	count := map[float64]int{}
	for _, rows := range pages {
		if len(rows) > 0 {
			count[math.Round(rows[len(rows)-1].y)]++
		}
	}
	best, n := math.NaN(), 1
	for y, c := range count {
		if c > n || (c == n && !math.IsNaN(best) && y < best) {
			best, n = y, c
		}
	}
	return best
}
