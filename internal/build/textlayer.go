package build

import (
	"fmt"
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
		return nil
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
