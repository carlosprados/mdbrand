package build

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/carlosprados/mdbrand/internal/run"
)

// checkTextLayer reads the finished PDF back as text and refuses private-use
// characters the source does not contain. Such a PDF prints right and copies
// wrong: a face's alternate glyphs mapped to private-use code points reach the
// ToUnicode table, and copy-paste, search and plagiarism checkers read the
// code points. Nothing in the log says so; only the text layer does.
//
// Without pdftotext the check cannot run, and the reader hears that.
func checkTextLayer(work, pdf, source string, rep *Report) error {
	if !run.Have("pdftotext") {
		rep.Warnings = append(rep.Warnings,
			"pdftotext is not installed, so the PDF's text layer was not checked for unreadable characters (mdbrand doctor)")
		return nil
	}
	out, err := run.Stdout(work, "pdftotext", "-enc", "UTF-8", pdf, "-")
	if err != nil {
		return err
	}
	bad := privateUse(string(out), source)
	if len(bad) == 0 {
		return checkEdges(work, pdf)
	}
	return fmt.Errorf(`the PDF prints right, but its text layer holds private-use characters, so
copying, searching or a plagiarism checker reads garbage at:
  %s
The face substitutes alternate glyphs that it maps to private-use code points.
mdbrand already turns contextual alternates (calt) off; this face uses another
feature. Set the body or display face to a different family in the bundle, and
report it with the document at
  https://github.com/carlosprados/mdbrand/issues`, strings.Join(bad, "\n  "))
}

// privateUse quotes each place in text where a private-use character appears
// that source does not contain: an icon font used on purpose is not a defect.
func privateUse(text, source string) []string {
	wanted := map[rune]bool{}
	for _, r := range source {
		if isPrivateUse(r) {
			wanted[r] = true
		}
	}
	const around = 18
	var out []string
	seen := map[string]bool{}
	rs := []rune(text)
	covered := -1 // the last index the previous quote already shows
	for i, r := range rs {
		if !isPrivateUse(r) || wanted[r] || i <= covered {
			continue
		}
		covered = i + around
		q := quoteAround(rs, i, around)
		if !seen[q] {
			seen[q] = true
			out = append(out, q)
		}
		if len(out) == 8 { // enough to find them; the fix is the same for all
			break
		}
	}
	return out
}

func isPrivateUse(r rune) bool { return unicode.In(r, unicode.Co) }

// quoteAround is the text around rs[i] on one line, private-use characters
// written as U+XXXX because a terminal shows them as nothing.
func quoteAround(rs []rune, i, n int) string {
	lo, hi := max(0, i-n), min(len(rs), i+n+1)
	var b strings.Builder
	for _, r := range rs[lo:hi] {
		switch {
		case isPrivateUse(r):
			fmt.Fprintf(&b, "<U+%04X>", r)
		case unicode.IsSpace(r):
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	return fmt.Sprintf("%q", strings.TrimSpace(b.String()))
}

var (
	pageRe = regexp.MustCompile(`<page width="([0-9.]+)"`)
	wordRe = regexp.MustCompile(`<word xMin="([0-9.]+)" yMin="[0-9.]+" xMax="([0-9.]+)" yMax="[0-9.]+">([^<]*)</word>`)
)

// edgeSlackPt is how close to the sheet's edge a word may end. pdftotext
// clips a word the sheet cuts and gives it the edge as xMax, within a couple
// of points; nothing a document sets on purpose comes this close.
const edgeSlackPt = 0.5

// checkEdges refuses a PDF with words cut at the edge of the sheet. A line
// that overflows the measure by more than the margin runs off the paper: the
// log says "Overfull \hbox" — a warning, and also what a cell a few points
// too narrow says — and the PDF prints the line cut short. Only the page can
// tell the two apart, so the finished one is measured.
func checkEdges(work, pdf string) error {
	out, err := run.Stdout(work, "pdftotext", "-enc", "UTF-8", "-bbox", pdf, "-")
	if err != nil {
		return err
	}
	cut := offPaper(string(out))
	if len(cut) == 0 {
		return nil
	}
	return fmt.Errorf(`%d line(s) run off the paper, cut at the edge of the sheet — the rest of
each line is not in the PDF at all:
  %s
A word TeX cannot break did it. Write a URL or a path as <https://…> or as a
`+"`code span`"+`, which break; give a long word a soft hyphen (\-) or rephrase; shorten
the lines of a code block or split it; a table too wide for the page needs
fewer or narrower columns`, len(cut), strings.Join(cut, "\n  "))
}

// offPaper lists, by page, the words pdftotext found touching either side of
// the sheet in its -bbox output.
func offPaper(bbox string) []string {
	var cut []string
	for i, page := range strings.Split(bbox, "<page ")[1:] {
		m := pageRe.FindStringSubmatch("<page " + page)
		if m == nil {
			continue
		}
		width, _ := strconv.ParseFloat(m[1], 64)
		for _, w := range wordRe.FindAllStringSubmatch(page, -1) {
			xMin, _ := strconv.ParseFloat(w[1], 64)
			xMax, _ := strconv.ParseFloat(w[2], 64)
			if xMax >= width-edgeSlackPt || xMin <= edgeSlackPt {
				cut = append(cut, fmt.Sprintf("page %d: %q", i+1, ellipsize(html.UnescapeString(w[3]), 50)))
			}
		}
	}
	return cut
}
