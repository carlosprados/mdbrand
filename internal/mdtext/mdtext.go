// Package mdtext answers the one question every substitution into Markdown
// has to ask first: is this offset prose, or code and mathematics that must be
// left as written? Both placeholder kinds — {{words}} and {{data…}} — ask it
// here, so they cannot disagree about where a code span ends.
package mdtext

import (
	"regexp"
	"strings"
)

// Inside reports whether offset i falls in one of spans.
func Inside(spans [][2]int, i int) bool {
	for _, s := range spans {
		if i >= s[0] && i < s[1] {
			return true
		}
	}
	return false
}

// Protected finds code blocks, fenced or indented, inline code and $…$ /
// $$…$$ math, following pandoc's rules closely enough for the purpose: the
// word count checks the number of substitutions against the placeholders
// pandoc itself saw in prose, so a disagreement here fails loudly instead of
// printing {{words}}. Inline spans are sought only between blocks, since a
// code block ends any paragraph that could hold one.
func Protected(s string) [][2]int {
	var spans [][2]int
	at := 0
	for _, b := range codeBlocks(s) {
		spans = append(spans, inlineSpans(s[:b[0]], at)...)
		spans = append(spans, b)
		at = b[1]
	}
	return append(spans, inlineSpans(s, at)...)
}

// listLead is a line whose indented continuation belongs to it rather than
// being code: a list item, a footnote, a definition.
var listLead = regexp.MustCompile(`^([-*+]|\d+[.)]|\[\^[^\]]+\]:|:)(\s|$)`)

// codeBlocks finds the fenced code blocks and the indented ones, in order. A
// block runs to the end of its last line, newline excluded. An indented block
// needs a blank line before it, as in pandoc, and is not one when the last
// line written at the margin opened a list item, a footnote or a definition:
// four spaces there continue it.
func codeBlocks(s string) [][2]int {
	var (
		blocks [][2]int
		fence  string
		open   = -1 // start of the block being read, fenced or indented
		last   int  // end of the indented block's last indented line
		blank  = true
		lead   bool
	)
	for start := 0; start < len(s); {
		end := strings.IndexByte(s[start:], '\n')
		if end < 0 {
			end = len(s)
		} else {
			end += start
		}
		line := s[start:end]
		trimmed := strings.TrimLeft(line, " ")
		isBlank := strings.TrimSpace(line) == ""
		switch {
		case fence != "":
			if t := strings.TrimSpace(trimmed); strings.HasPrefix(t, fence) && strings.Trim(t, fence[:1]) == "" {
				blocks = append(blocks, [2]int{open, end})
				fence, open = "", -1
			}
		case len(line)-len(trimmed) <= 3 && Fence(trimmed) != "":
			if open >= 0 {
				blocks = append(blocks, [2]int{open, last})
			}
			fence, open = Fence(trimmed), start
		case open >= 0 && indented(line):
			last = end
		case open >= 0 && !isBlank:
			blocks = append(blocks, [2]int{open, last})
			open = -1
		case open < 0 && blank && !isBlank && indented(line) && !lead:
			open, last = start, end
		}
		if !isBlank && !indented(line) && fence == "" {
			lead = listLead.MatchString(trimmed)
		}
		blank = isBlank
		start = end + 1
	}
	switch {
	case fence != "":
		blocks = append(blocks, [2]int{open, len(s)})
	case open >= 0:
		blocks = append(blocks, [2]int{open, last})
	}
	return blocks
}

// indented reports a line set four columns in, a tab counting to the next
// stop: the indent of a code block.
func indented(line string) bool {
	col := 0
	for _, c := range []byte(line) {
		switch c {
		case ' ':
			col++
		case '\t':
			col += 4 - col%4
		default:
			return false
		}
		if col >= 4 {
			return true
		}
	}
	return false
}

// inlineSpans finds inline code and math in s from offset i on. s ends where
// the prose does, so no span is closed past it.
func inlineSpans(s string, i int) [][2]int {
	var spans [][2]int
	for i < len(s) {
		stop := 0
		switch s[i] {
		case '\\':
			i += 2
			continue
		case '`':
			run := countRun(s[i:], '`')
			if end := closingRun(s[i+run:], strings.Repeat("`", run)); end >= 0 {
				stop = i + run + end + run
			} else {
				i += run
				continue
			}
		case '$':
			if strings.HasPrefix(s[i:], "$$") {
				if end := strings.Index(s[i+2:], "$$"); end >= 0 {
					stop = i + 2 + end + 2
				}
			} else {
				stop = inlineMathEnd(s, i)
			}
		}
		if stop > 0 {
			spans = append(spans, [2]int{i, stop})
			i = stop
			continue
		}
		i++
	}
	return spans
}

// Fence returns the run of three or more backticks or tildes that opens a code
// fence at the start of line, or "".
func Fence(line string) string {
	for _, c := range []byte{'`', '~'} {
		if n := countRun(line, c); n >= 3 {
			return strings.Repeat(string(c), n)
		}
	}
	return ""
}

func countRun(s string, c byte) int {
	n := 0
	for n < len(s) && s[n] == c {
		n++
	}
	return n
}

// closingRun finds a backtick run of exactly the opening length.
func closingRun(s, tick string) int {
	off := 0
	for {
		j := strings.Index(s[off:], tick)
		if j < 0 {
			return -1
		}
		j += off
		if countRun(s[j:], '`') == len(tick) {
			return j
		}
		off = j + countRun(s[j:], '`')
	}
}

// inlineMathEnd applies pandoc's tex_math_dollars rule: the opening $ is
// followed by a non-space, the closing one preceded by a non-space and not
// followed by a digit. It returns the index after the closing $, or 0.
func inlineMathEnd(s string, i int) int {
	if i+1 >= len(s) || s[i+1] == ' ' || s[i+1] == '\n' {
		return 0
	}
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\n':
			if j+1 < len(s) && s[j+1] == '\n' {
				return 0 // a paragraph break ends any candidate
			}
		case '\\':
			j++
		case '$':
			if s[j-1] != ' ' && (j+1 >= len(s) || s[j+1] < '0' || s[j+1] > '9') {
				return j + 1
			}
		}
	}
	return 0
}
