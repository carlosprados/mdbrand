package build

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/carlosprados/mdbrand/internal/run"
	"github.com/carlosprados/mdbrand/internal/tex"
)

var (
	// beamer's own total, from the deck's .nav. The notes run cannot count
	// it: under pgfpages and show only notes it came out as 1.
	navTotalRe = regexp.MustCompile(`\\inserttotalframenumber \{(\d+)\}`)
	// A note taller than its page, from notes.tex.tmpl: the \end{frame} line
	// of its slide in the deck's .tex, and by how much.
	noteRe = regexp.MustCompile(`MDBRAND-NOTE-TOOLONG line=(\d+) over=([0-9.]+)pt`)
)

// renderNotes writes the speaker notes from the deck's .tex, which renderPDF
// has already typeset in the work directory: the same document, so the
// slides beside the notes are the slides the audience sees.
func renderNotes(p *prepared, out string) error {
	o, work, rep := p.o, p.work, p.rep
	stem := workStem(o.Input)
	deck, err := os.ReadFile(filepath.Join(work, stem+".tex"))
	if err != nil {
		return err
	}
	nav, err := os.ReadFile(filepath.Join(work, stem+".nav"))
	if err != nil {
		return err
	}
	m := navTotalRe.FindAllSubmatch(nav, -1)
	if len(m) == 0 {
		return fmt.Errorf("the deck's %s.nav holds no slide count", stem)
	}
	total, _ := strconv.Atoi(string(m[len(m)-1][1]))
	preamble, err := tex.Render("notes", &tex.Data{
		NotesMarginMM: tex.NotesMarginMM,
		NotesHeightMM: tex.NotesHeightMM,
		NotesTotal:    total,
	})
	if err != nil {
		return err
	}
	src, err := notesTeX(string(deck), preamble)
	if err != nil {
		return err
	}
	name := stem + "-notes"
	if err := os.WriteFile(filepath.Join(work, name+".tex"), []byte(src), 0o644); err != nil {
		return err
	}
	for i := 0; i < 2; i++ {
		o.logf("  xelatex notes pass %d/2", i+1)
		if _, err := run.Cmd(work, "xelatex", "-interaction=nonstopmode", "-halt-on-error", name+".tex"); err != nil {
			return fmt.Errorf("xelatex failed on the speaker notes: %w", err)
		}
	}

	// Every check below reads this log; without it they would all pass.
	logRaw, err := os.ReadFile(filepath.Join(work, name+".log"))
	if err != nil {
		return fmt.Errorf("xelatex left no log to check: %w", err)
	}
	log := string(logRaw)
	// The deck's own log has judged the slides; the notes run sets them
	// again for the thumbnails, so only what the notes add is read here.
	sc := scanLog(log)
	if err := judgeLog(scan{Holes: sc.Holes, Shapes: sc.Shapes}, o, rep); err != nil {
		return err
	}
	if err := judgeNotes(log, string(deck)); err != nil {
		return err
	}
	rep.Outputs = append(rep.Outputs, Output{Format: "notes", Path: out, Pages: sc.Pages, built: filepath.Join(work, name+".pdf")})
	return nil
}

// notesTeX turns the deck's .tex into its notes: handout mode, the notes
// preamble, and a \note in every frame naming the line its \end{frame} is on.
func notesTeX(deck, preamble string) (string, error) {
	const class = "\\documentclass[\n"
	if !strings.Contains(deck, class) || !strings.Contains(deck, "\\begin{document}") {
		return "", fmt.Errorf("the deck's .tex is not in the shape the notes expect: no %q or \\begin{document}", strings.TrimSpace(class))
	}
	// The notes first, so the line numbers are the deck's own.
	lines := strings.Split(deck, "\n")
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), `\end{frame}`) {
			lines[i] = fmt.Sprintf(`\note{\mdbrandNoteFrame{%d}}`, i+1) + "\n" + l
		}
	}
	deck = strings.Join(lines, "\n")
	deck = strings.Replace(deck, class, class+"  handout,\n", 1)
	return strings.Replace(deck, "\\begin{document}", preamble+"\\begin{document}", 1), nil
}

// noteLinePt is the baseline skip of a note, from notes.tex.tmpl: a length
// a writer can act on is a number of lines, not millimetres of a frame that
// pgfpages then scales onto the sheet.
const noteLinePt = 11.5

// judgeNotes stops on a note too long for its page, which runs off the
// sheet and leaves no trace but the measurement.
func judgeNotes(log, deck string) error {
	lines := strings.Split(deck, "\n")
	var w strings.Builder
	n := 0
	seen := map[string]bool{}
	for _, m := range noteRe.FindAllStringSubmatch(log, -1) {
		if seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		line, _ := strconv.Atoi(m[1])
		pt, _ := strconv.ParseFloat(m[2], 64)
		n++
		fmt.Fprintf(&w, "\n    %-40s %3.0f line(s) too long", frameTitle(lines, line), math.Ceil(pt/noteLinePt))
	}
	if n == 0 {
		return wideNotes(log, lines)
	}
	return fmt.Errorf(`%d speaker note(s) run past the bottom of their page:%s
  A note has room for %.0f lines beside its slide. Cut it to what you need to
  say, or split the slide and its note with another ## heading`, n, w.String(), math.Floor(tex.NotesHeightMM/mmPerPt/noteLinePt))
}

var noteEndRe = regexp.MustCompile(`^MDBRAND-NOTE-END line=(\d+)`)

// wideNotes stops on a note holding a word wider than its column, which runs
// off the sheet. The deck has already stopped on any slide this wide, so a
// line this wide in the notes' log is a note's, set when its \note is read and
// so before the mark notes.tex.tmpl writes at its page; one from the output
// routine is the footer's.
func wideNotes(log string, deck []string) error {
	var w strings.Builder
	n := 0
	widest := 0.0
	for _, ln := range strings.Split(log, "\n") {
		if m := overRe.FindStringSubmatch(ln); m != nil && !strings.Contains(ln, `\output is active`) {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > 5 {
				widest = max(widest, v)
			}
			continue
		}
		m := noteEndRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		if widest > 0 {
			line, _ := strconv.Atoi(m[1])
			n++
			fmt.Fprintf(&w, "\n    %-40s %5.1f mm too wide", frameTitle(deck, line), widest*mmPerPt)
		}
		widest = 0
	}
	if n == 0 {
		return nil
	}
	return fmt.Errorf(`%d speaker note(s) hold a word wider than the note's column, which runs off the
sheet:%s
  Shorten it, or split it: code and long names do not break, a link written
  as <https://…> does`, n, w.String())
}
