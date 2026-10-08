package build

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/carlosprados/mdbrand/internal/fig"
)

// frameSlackPt is what a slide may run past its frame before it is a defect:
// under a point is rounding, not a line of text over the footer.
const frameSlackPt = 1.0

// judgeFrames stops a deck with a slide that does not fit. beamer sets the
// excess over the footer or off the page and exits 0; the build names each
// slide by its title, and how much is too much. A slide with pauses is one
// page per overlay, each reported at the same \end{frame}: it is one slide,
// as tall as its tallest overlay. A slide holding one figure is told the
// width that makes room, since a figure with a paragraph above it is the
// usual slide that does not fit; minPt is the deck's legibility floor.
func judgeFrames(frames []frameOver, texSrc string, figs []*fig.Result, minPt float64) error {
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
	byFile := map[string]*fig.Result{}
	for _, r := range figs {
		byFile[filepath.Base(r.PDF)] = r
	}
	for _, line := range order {
		overMM := tallest[line] * mmPerPt
		fmt.Fprintf(&w, "\n    %-40s %5.1f mm too tall", frameTitle(lines, line), overMM)
		if on := frameFigures(lines, line); len(on) == 1 && byFile[on[0]] != nil {
			w.WriteString(figureRoom(byFile[on[0]], overMM, minPt))
		}
	}
	return fmt.Errorf(`%d slide(s) run past the bottom of the frame, over the footer or off the page:%s
  Split the slide (another ## heading), cut what it says, or give a figure on
  it less height: a diagram that is wider than tall suits a 16:9 frame`, n, w.String())
}

var includeRe = regexp.MustCompile(`\\includegraphics(?:\[[^\]]*\])?\{([^}]+)\}`)

// frameFigures lists the files a frame includes, for the frame the log
// reported at 1-based line `end`; nothing when `end` is not an \end{frame}.
func frameFigures(lines []string, end int) []string {
	if end < 1 || end > len(lines) || !strings.HasPrefix(strings.TrimSpace(lines[end-1]), `\end{frame}`) {
		return nil
	}
	var files []string
	for i := end - 2; i >= 0 && !beginFrameRe.MatchString(lines[i]); i-- {
		for _, m := range includeRe.FindAllStringSubmatch(lines[i], -1) {
			files = append(files, m[1])
		}
	}
	return files
}

// figureRoom is the advice for a slide whose one figure can give back what
// the slide runs over. A figure is drawn at a width and its height follows,
// so the fix is a width, rounded down to the millimetre; its labels shrink by
// the same factor, and when they would fall under the floor no width will do.
func figureRoom(r *fig.Result, overMM, minPt float64) string {
	name := filepath.Base(r.Fig.SrcPath)
	if r.Fig.Caption != "" {
		name = fmt.Sprintf("%q (%s)", ellipsize(r.Fig.Caption, 30), name)
	}
	room := r.HeightMM - overMM
	if room <= 0 || r.HeightMM <= 0 {
		return fmt.Sprintf("\n        even without its figure %s the slide would not fit", name)
	}
	width := math.Floor(r.WidthMM * room / r.HeightMM)
	pt := r.TextPt * width / r.WidthMM
	if r.TextPt > 0 && pt < minPt {
		return fmt.Sprintf("\n        its figure %s would have to be %.0f mm tall, and its labels would fall to\n"+
			"        %.1fpt, under the %.1fpt floor: give the text a slide of its own, or draw\n"+
			"        the figure wider than tall", name, room, pt, minPt)
	}
	return fmt.Sprintf("\n        its figure %s is %.0f mm tall and the slide has room for %.0f:\n"+
		"        put width=%.0fmm on its block — its labels stay at %.1fpt", name, r.HeightMM, room, width, pt)
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
