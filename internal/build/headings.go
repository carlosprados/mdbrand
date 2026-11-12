package build

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// printedHeading is a heading spans.lua reported: its level, whether it is
// kept out of the contents, and its text.
type printedHeading struct {
	level    int
	unlisted bool
	text     string
}

var headingRe = regexp.MustCompile(`MDBRAND-HEADING (\d+) (listed|unlisted)\t([^\n]*)`)

func reportedHeadings(pandocOut string) []printedHeading {
	var hs []printedHeading
	for _, m := range headingRe.FindAllStringSubmatch(pandocOut, -1) {
		l, _ := strconv.Atoi(m[1])
		hs = append(hs, printedHeading{level: l, unlisted: m[2] == "unlisted", text: m[3]})
	}
	return hs
}

// lostHeadings names the headings the PDF's text does not hold as often as
// the document prints them. A table in a kept block once held the page as a
// float: a section, its heading and the table never printed, exit 0, and the
// only witness was the text that was not there. A heading in the contents
// prints twice, there and in the body, so a body that lost it does not hide
// behind its entry; tocDepth is 0 when the document has no contents.
//
// Both sides keep letters and digits alone, case and compatibility forms
// folded: numbering, dot leaders, hyphens at line ends, ligatures and spacing
// differ between the source and pdftotext, and none of them are text.
func lostHeadings(pdfText string, hs []printedHeading, tocDepth int) []string {
	body := foldText(pdfText)
	want := map[string]int{}
	var order []string
	for _, h := range hs {
		k := foldText(h.text)
		if k == "" {
			continue
		}
		if _, ok := want[k]; !ok {
			order = append(order, h.text)
		}
		want[k]++
		if h.level <= tocDepth && !h.unlisted {
			want[k]++
		}
	}
	var lost []string
	for _, t := range order {
		k := foldText(t)
		if strings.Count(body, k) < want[k] {
			lost = append(lost, t)
		}
	}
	return lost
}

func foldText(s string) string {
	s = strings.ToLower(norm.NFKC.String(s))
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}

func lostHeadingsError(lost []string) error {
	return fmt.Errorf(`the PDF lost text: these headings are in the document and not in the PDF,
so the sections under them may be missing too:
  %s
This is mdbrand's defect, not the document's. Build with --work ./out and
report it with the .tex and the log at https://github.com/carlosprados/mdbrand/issues`,
		strings.Join(lost, "\n  "))
}
