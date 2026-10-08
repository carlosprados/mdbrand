package build

import (
	"fmt"
	"regexp"
	"strings"
)

// frameSlackPt is what a slide may run past its frame before it is a defect:
// under a point is rounding, not a line of text over the footer.
const frameSlackPt = 1.0

// judgeFrames stops a deck with a slide that does not fit. beamer sets the
// excess over the footer or off the page and exits 0; the build names each
// slide by its title, and how much is too much. A slide with pauses is one
// page per overlay, each reported at the same \end{frame}: it is one slide,
// as tall as its tallest overlay.
func judgeFrames(frames []frameOver, texSrc string) error {
	var order []int
	tallest := map[int]float64{}
	for _, f := range frames {
		if f.Pt <= frameSlackPt {
			continue
		}
		if _, seen := tallest[f.Line]; !seen {
			order = append(order, f.Line)
		}
		tallest[f.Line] = max(tallest[f.Line], f.Pt)
	}
	n := len(order)
	if n == 0 {
		return nil
	}
	lines := strings.Split(texSrc, "\n")
	var w strings.Builder
	for _, line := range order {
		fmt.Fprintf(&w, "\n    %-40s %5.1f mm too tall", frameTitle(lines, line), tallest[line]*mmPerPt)
	}
	return fmt.Errorf(`%d slide(s) run past the bottom of the frame, over the footer or off the page:%s
  Split the slide (another ## heading), cut what it says, or give a figure on
  it less height: a diagram that is wider than tall suits a 16:9 frame`, n, w.String())
}

// mmPerPt converts TeX points to millimetres.
const mmPerPt = 25.4 / 72.27

var beginFrameRe = regexp.MustCompile(`^\\begin\{frame\}(\[[^\]]*\])?`)

// frameTitle names the slide whose box the log reported at 1-based line
// `end`. A frame is reported at its \end{frame}, so its title is on the
// nearest \begin{frame} above. The cover slides are not frame environments:
// the title page is \frame{\titlepage}, and a section's slide comes from
// \AtBeginSection and is reported at the \section command, on its last line
// when a long title wraps. Searching up for a frame from there would name the
// slide before it.
func frameTitle(lines []string, end int) string {
	if end < 1 || end > len(lines) {
		return fmt.Sprintf("(line %d of the .tex)", end)
	}
	if strings.HasPrefix(strings.TrimSpace(lines[end-1]), `\end{frame}`) {
		for i := end - 2; i >= 0; i-- {
			if m := beginFrameRe.FindStringIndex(lines[i]); m != nil {
				if title := braced(lines[i][m[1]:]); title != "" {
					return fmt.Sprintf("%q", ellipsize(plainTeX(title), 40))
				}
				return fmt.Sprintf("(untitled, line %d of the .tex)", i+1)
			}
		}
	}
	for i := end - 1; i >= 0; i-- {
		at := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(at, `\frame{\titlepage}`):
			return "the title slide"
		case strings.HasPrefix(at, `\section`):
			cmd := strings.Join(lines[i:end], "\n")
			cmd = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(cmd), `\section`), "*")
			return fmt.Sprintf("the section slide %q", ellipsize(plainTeX(braced(cmd)), 40))
		case strings.HasPrefix(at, `\begin{frame}`), strings.HasPrefix(at, `\end{frame}`):
			return fmt.Sprintf("(line %d of the .tex)", end)
		}
	}
	return fmt.Sprintf("(line %d of the .tex)", end)
}

// braced returns the contents of the brace group s starts with, or "".
func braced(s string) string {
	if !strings.HasPrefix(s, "{") {
		return ""
	}
	depth, escaped := 0, false
	for i, r := range s {
		if escaped {
			escaped = false
			continue
		}
		switch r {
		case '\\':
			escaped = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[1:i]
			}
		}
	}
	return ""
}

var texCmdRe = regexp.MustCompile(`\\[A-Za-z]+\s*`)

// plainTeX turns pandoc's LaTeX for a title back into something to recognise
// it by: escapes undone, commands and braces dropped, the dashes pandoc wrote
// as ligatures restored.
func plainTeX(s string) string {
	// Escaped braces are parked on control characters, so that dropping the
	// command braces does not take them too.
	s = strings.NewReplacer(`\{`, "\x01", `\}`, "\x02", `\&`, "&", `\%`, "%", `\$`, "$", `\#`, "#", `\_`, "_").Replace(s)
	s = texCmdRe.ReplaceAllString(s, "")
	s = strings.NewReplacer("{", "", "}", "", "\x01", "{", "\x02", "}", "---", "—", "--", "–", "~", " ").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}
