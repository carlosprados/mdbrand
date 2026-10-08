package build

import (
	"strings"
	"testing"
)

// Each frame gets a \note naming its \end{frame} line in the deck's .tex, so
// a note too long for its page is reported by the title frameTitle finds
// there. Numbering after the class option and the preamble went in would
// name the wrong slide.
func TestNotesTeXNamesEachFrameByItsDeckLine(t *testing.T) {
	deck := "\\documentclass[\n  aspectratio=169]{beamer}\n\\begin{document}\n" +
		"\\begin{frame}{One}\nx\n\\end{frame}\n\\begin{frame}[fragile]{Two}\ny\n\\end{frame}\n\\end{document}"
	got, err := notesTeX(deck, "PREAMBLE\nLINES\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"  handout,\n", "PREAMBLE\nLINES\n\\begin{document}",
		"\\note{\\mdbrandNoteFrame{6}}\n\\end{frame}", "\\note{\\mdbrandNoteFrame{9}}\n\\end{frame}"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	lines := strings.Split(deck, "\n")
	if a, b := frameTitle(lines, 6), frameTitle(lines, 9); a != `"One"` || b != `"Two"` {
		t.Errorf("lines 6 and 9 name %s and %s", a, b)
	}
	if _, err := notesTeX("no class here", ""); err == nil {
		t.Error("a .tex in another shape must be refused, not half-converted")
	}
}

func TestJudgeNotesCountsInLines(t *testing.T) {
	deck := "\\begin{frame}{Long}\n\\end{frame}"
	log := "MDBRAND-NOTE-TOOLONG line=2 over=12.0pt\nMDBRAND-NOTE-TOOLONG line=2 over=12.0pt\n"
	err := judgeNotes(log, deck)
	if err == nil || !strings.HasPrefix(err.Error(), "1 speaker note(s)") ||
		!strings.Contains(err.Error(), `"Long"`) || !strings.Contains(err.Error(), "2 line(s) too long") {
		t.Errorf("want one note, by title, 12pt as 2 lines: %v", err)
	}
	if err := judgeNotes("nothing here", deck); err != nil {
		t.Errorf("no marker, no error: %v", err)
	}
}
