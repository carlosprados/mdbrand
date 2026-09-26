package brand

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/carlosprados/mdbrand/internal/run"
)

// Charset is the set of code points a font covers, as closed ranges.
type Charset [][2]rune

// ParseCharset reads fontconfig's %{charset}: space-separated hex code points
// and ranges, "20-7e a0 a2-ff".
func ParseCharset(s string) (Charset, error) {
	var c Charset
	for _, f := range strings.Fields(s) {
		lo, hi, isRange := strings.Cut(f, "-")
		a, err := strconv.ParseUint(lo, 16, 32)
		if err != nil {
			return nil, fmt.Errorf("charset: %q: %w", f, err)
		}
		b := a
		if isRange {
			if b, err = strconv.ParseUint(hi, 16, 32); err != nil {
				return nil, fmt.Errorf("charset: %q: %w", f, err)
			}
		}
		c = append(c, [2]rune{rune(a), rune(b)})
	}
	return c, nil
}

// Has reports whether r is covered.
func (c Charset) Has(r rune) bool {
	for _, rg := range c {
		if r >= rg[0] && r <= rg[1] {
			return true
		}
	}
	return false
}

// FontCharset returns the coverage of an installed family. It answers false
// when fontconfig does not have THAT family: fc-match always answers with
// something, and the coverage of a substitute is not the coverage of the font
// the bundle named.
func FontCharset(family string) (Charset, bool) {
	if family == "" || !run.Have("fc-match") {
		return nil, false
	}
	out, err := run.Cmd("", "fc-match", "--format", "%{family}\n%{charset}", family)
	if err != nil {
		return nil, false
	}
	names, charset, _ := strings.Cut(out, "\n")
	if !familyMatches(names, family) {
		return nil, false
	}
	c, err := ParseCharset(charset)
	if err != nil {
		return nil, false
	}
	return c, true
}

// familyMatches compares a requested family with fc-match's answer, which
// lists every name the match carries, comma separated.
func familyMatches(names, family string) bool {
	for _, n := range strings.Split(names, ",") {
		if strings.EqualFold(strings.TrimSpace(n), family) {
			return true
		}
	}
	return false
}

// FallbackRunes returns, in first-seen order, the characters of text that the
// body face lacks and the fallback face has: exactly the ones worth redirecting.
//
// A character neither face covers is left alone on purpose, so the missing-glyph
// check still stops the build over it. Variation selectors (U+FE00-FE0F, the
// invisible "draw as emoji" after ⚠) are skipped: they have no glyph to borrow,
// and redirecting one would only move an invisible character between fonts.
func FallbackRunes(text string, body, fallback Charset) []rune {
	seen := map[rune]bool{}
	var out []rune
	for _, r := range text {
		if r < 0x80 || seen[r] || (r >= 0xFE00 && r <= 0xFE0F) {
			continue
		}
		seen[r] = true
		if !body.Has(r) && fallback.Has(r) {
			out = append(out, r)
		}
	}
	return out
}
