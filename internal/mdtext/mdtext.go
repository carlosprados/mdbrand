// Package mdtext answers the one question every substitution into Markdown
// has to ask first: is this offset prose, or code and mathematics that must be
// left as written? Both placeholder kinds — {{words}} and {{data…}} — ask it
// here, so they cannot disagree about where a code span ends.
package mdtext

import "strings"

// Inside reports whether offset i falls in one of spans.
func Inside(spans [][2]int, i int) bool {
	for _, s := range spans {
		if i >= s[0] && i < s[1] {
			return true
		}
	}
	return false
}

// Protected finds fenced code blocks, inline code and $…$ / $$…$$ math,
// following pandoc's rules closely enough for the purpose: the word count checks
// the number of substitutions against the placeholders pandoc itself saw in
// prose, so a disagreement here fails loudly instead of printing {{words}}.
func Protected(s string) [][2]int {
	var spans [][2]int
	fence := ""
	fenceStart := 0
	i := 0
	for i < len(s) {
		atLineStart := i == 0 || s[i-1] == '\n'
		if atLineStart {
			line := s[i:]
			if j := strings.IndexByte(line, '\n'); j >= 0 {
				line = line[:j]
			}
			trimmed := strings.TrimLeft(line, " ")
			if fence == "" && len(line)-len(trimmed) <= 3 {
				if f := fenceOf(trimmed); f != "" {
					fence, fenceStart = f, i
					i += len(line)
					continue
				}
			} else if fence != "" {
				t := strings.TrimSpace(trimmed)
				if strings.HasPrefix(t, fence) && strings.Trim(t, fence[:1]) == "" {
					spans = append(spans, [2]int{fenceStart, i + len(line)})
					fence = ""
				}
				i += len(line)
				if i < len(s) {
					i++
				}
				continue
			}
		}
		if fence != "" {
			i++
			continue
		}
		switch s[i] {
		case '\\':
			i += 2
			continue
		case '`':
			run := countRun(s[i:], '`')
			tick := strings.Repeat("`", run)
			if end := closingRun(s[i+run:], tick); end >= 0 {
				stop := i + run + end + run
				spans = append(spans, [2]int{i, stop})
				i = stop
				continue
			}
			i += run
			continue
		case '$':
			if strings.HasPrefix(s[i:], "$$") {
				if end := strings.Index(s[i+2:], "$$"); end >= 0 {
					stop := i + 2 + end + 2
					spans = append(spans, [2]int{i, stop})
					i = stop
					continue
				}
			} else if stop := inlineMathEnd(s, i); stop > 0 {
				spans = append(spans, [2]int{i, stop})
				i = stop
				continue
			}
		}
		i++
	}
	if fence != "" {
		spans = append(spans, [2]int{fenceStart, len(s)})
	}
	return spans
}

func fenceOf(line string) string {
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
