package pptx

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Overflow estimates which slides' text does not fit, from a rendering of
// the deck: bbox is `pdftotext -bbox` of LibreOffice's PDF, one page per
// slide. LibreOffice draws text past its box rather than shrinking it, and
// the page clips what falls off, so a slide overflows when a word of its
// body sits below the footer's rule or a word of it never reaches the page.
// It is an estimate: the reader's PowerPoint sets the text in its own fonts.
func Overflow(deck []byte, bbox string, l Look) ([]string, error) {
	p, err := readPkg(deck)
	if err != nil {
		return nil, err
	}
	pages := bboxPageRe.FindAllStringSubmatch(bbox, -1)
	slides := slideNames(p)
	footer := words(l.Confidential + " " + l.RunningTitle)
	var over []string
	for i, name := range slides {
		if i >= len(pages) {
			break
		}
		x := p.get(name)
		if coverSlide(p, name) {
			continue
		}
		h, _ := strconv.ParseFloat(pages[i][1], 64)
		ruleY := h * (footerRuleMM() / SlideHMM)
		seen := map[string]bool{}
		low := false
		for _, w := range bboxWordRe.FindAllStringSubmatch(pages[i][2], -1) {
			y, _ := strconv.ParseFloat(w[1], 64)
			text := unescape(w[2])
			for t := range words(text) {
				seen[t] = true
			}
			if y > ruleY+1 && !footerWord(text, footer) {
				low = true
			}
		}
		lost := 0
		for _, m := range textRe.FindAllStringSubmatch(x, -1) {
			for t := range words(unescape(m[1])) {
				if !seen[t] {
					lost++
				}
			}
		}
		switch {
		case lost > 0:
			over = append(over, fmt.Sprintf("slide %d %q: %d word(s) do not reach the slide", i+1, slideTitle(x), lost))
		case low:
			over = append(over, fmt.Sprintf("slide %d %q: text runs over the footer", i+1, slideTitle(x)))
		}
	}
	return over, nil
}

var (
	bboxPageRe  = regexp.MustCompile(`(?s)<page width="[\d.]+" height="([\d.]+)">(.*?)</page>`)
	bboxWordRe  = regexp.MustCompile(`<word xMin="[-\d.]+" yMin="[-\d.]+" xMax="[-\d.]+" yMax="([-\d.]+)">([^<]*)</word>`)
	layoutRelRe = regexp.MustCompile(`Target="\.\./slideLayouts/(slideLayout\d+\.xml)"`)
)

// footerRuleMM is where the master's rule is drawn, from the slide's top.
func footerRuleMM() float64 {
	const bottom, textH = 3 * Scale, 6.0
	return SlideHMM - bottom - textH - 1.2*Scale - 1
}

// coverSlide reports a slide laid out as a title or section slide, which carries
// no footer to run over.
func coverSlide(p *pkg, slide string) bool {
	rels := p.get(strings.Replace(slide, "slides/", "slides/_rels/", 1) + ".rels")
	m := layoutRelRe.FindStringSubmatch(rels)
	if m == nil {
		return false
	}
	name := layoutNameRe.FindStringSubmatch(p.get("ppt/slideLayouts/" + m[1]))
	return name != nil && (name[1] == "Title Slide" || name[1] == "Section Header")
}

// words is the set of a text's words, lower case, letters and digits only:
// pdftotext and the slide's XML split punctuation differently.
func words(s string) map[string]bool {
	out := map[string]bool{}
	for _, f := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		out[f] = true
	}
	return out
}

// footerWord reports a word the footer prints: its label, its title, or the
// slide number.
func footerWord(text string, footer map[string]bool) bool {
	for t := range words(text) {
		if _, err := strconv.Atoi(t); err != nil && !footer[t] {
			return false
		}
	}
	return true
}
