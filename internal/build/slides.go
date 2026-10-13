package build

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/carlosprados/mdbrand/internal/fig"
	"github.com/carlosprados/mdbrand/internal/mdtext"
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

// slideOver is a line too wide for its slide, named by the slide's number and
// title.
type slideOver struct {
	Pt    float64
	Num   string
	Title string
}

var (
	slideMarkRe = regexp.MustCompile(`^MDBRAND-SLIDE (\d+)$`)
	shipRe      = regexp.MustCompile(`^\[\d`)
)

// slideOverflows assigns each Overfull \hbox in a deck's log to its slide.
// The footline writes the slide's number and title several times per page;
// the mark right before the log's [N is the pass that ships the page, after
// that page's body and its warnings, so the warnings since the last page are
// that slide's. One from the output routine is the footline's own.
func slideOverflows(lines []string) []slideOver {
	var out []slideOver
	var pending []float64
	seen := map[string]bool{}
	for i, ln := range lines {
		if m := overRe.FindStringSubmatch(ln); m != nil && !strings.Contains(ln, `\output is active`) {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > 5 {
				pending = append(pending, v)
			}
			continue
		}
		m := slideMarkRe.FindStringSubmatch(ln)
		if m == nil || !ships(lines[i+1:]) {
			continue
		}
		for _, pt := range pending {
			// A slide with pauses ships a page per overlay, each with the
			// same line: one slide, one report, at its widest.
			if seen[m[1]] {
				for j := range out {
					if out[j].Num == m[1] {
						out[j].Pt = max(out[j].Pt, pt)
					}
				}
				continue
			}
			seen[m[1]] = true
			out = append(out, slideOver{Pt: pt, Num: m[1], Title: slideTitle(markedTitle(lines, i))})
		}
		pending = nil
	}
	return out
}

// ships reports whether the page is shipped before the next mark or warning:
// what the footline itself loads (its logo) may come between.
func ships(after []string) bool {
	for _, ln := range after {
		switch {
		case shipRe.MatchString(ln):
			return true
		case strings.HasPrefix(ln, "MDBRAND-"), overRe.MatchString(ln):
			return false
		}
	}
	return false
}

// markedTitle is the title written just above the mark at line i, joined
// back from the log's 79-column wrap.
func markedTitle(lines []string, i int) string {
	for j := i - 1; j >= 0 && j >= i-8; j-- {
		if t, ok := strings.CutPrefix(lines[j], "MDBRAND-TITLE "); ok {
			return t + strings.Join(lines[j+1:i], "")
		}
	}
	return ""
}

// slideTitle is the frame title as \detokenize wrote it: in braces, followed
// by beamer's own continuation-numbering code.
func slideTitle(raw string) string {
	if i := strings.Index(raw, `\ifnum \beamer@autobreakcount`); i >= 0 {
		raw = raw[:i]
	}
	return plainTeX(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(raw), "{"), "}"))
}

// judgeSlideWidths stops a deck with a line too wide for its slide. It runs
// into the margin, over the column beside it, or off the page, and beamer
// exits 0; on a page the same overflow is only a warning, since a page has
// margins to spare and a slide's columns do not.
func judgeSlideWidths(over []slideOver) error {
	if len(over) == 0 {
		return nil
	}
	var w strings.Builder
	for _, o := range over {
		name := "slide " + o.Num
		if o.Title != "" {
			name += fmt.Sprintf(" %q", ellipsize(o.Title, 40))
		}
		fmt.Fprintf(&w, "\n    %-46s %5.1f mm too wide", name, o.Pt*mmPerPt)
	}
	return fmt.Errorf(`%d slide(s) hold a line wider than the slide or its column has room for: it
runs into the margin, across the column beside it or off the page:%s
  A word TeX cannot break did it — a URL, a path, a long name. Write it as a
  `+"`code span`"+` or <https://…>, which break, shorten it, or give its column more
  width`, len(over), w.String())
}

var (
	divRe      = regexp.MustCompile(`^(:{3,})\s*(.*)$`)
	colWidthRe = regexp.MustCompile(`width\s*=\s*"?([0-9.]+)%`)
	// doc.ExtractFigs' placeholder, "@@MDBRAND_FIG_3@@".
	placeholderRe = regexp.MustCompile(`@@MDBRAND_FIG_\d+@@`)
)

// pandocColumnShare is what pandoc's beamer writer gives a column with no
// width: 0.48 of the line, however many columns there are.
const pandocColumnShare = 0.48

// columnShares is, for each figure placeholder inside a ::: column, the share
// of the slide's width pandoc gives that column. A figure there was sized for
// the whole slide and drawn across the column beside it, or off the page.
func columnShares(body string) map[string]float64 {
	shares := map[string]float64{}
	var stack []float64 // one per open div: its share, or 0 when not a column
	fence := ""
	for _, ln := range strings.Split(body, "\n") {
		t := strings.TrimSpace(ln)
		if fence != "" {
			if strings.HasPrefix(t, fence) && strings.Trim(t, fence[:1]) == "" {
				fence = ""
			}
			continue
		}
		if f := mdtext.Fence(t); f != "" {
			fence = f
			continue
		}
		if m := divRe.FindStringSubmatch(t); m != nil {
			if m[2] == "" {
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
				continue
			}
			stack = append(stack, columnShare(m[2]))
			continue
		}
		for i := len(stack) - 1; i >= 0; i-- {
			if stack[i] > 0 {
				for _, ph := range placeholderRe.FindAllString(t, -1) {
					shares[ph] = stack[i]
				}
				break
			}
		}
	}
	return shares
}

// columnShare reads a div's opening attributes: a column's share of the line,
// or 0 for any other div.
func columnShare(attrs string) float64 {
	words := strings.FieldsFunc(attrs, func(r rune) bool { return r == ' ' || r == '{' || r == '}' })
	isColumn := false
	for _, w := range words {
		if w == "column" || w == ".column" {
			isColumn = true
		}
	}
	if !isColumn {
		return 0
	}
	if m := colWidthRe.FindStringSubmatch(attrs); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > 0 {
			return v / 100
		}
	}
	return pandocColumnShare
}
