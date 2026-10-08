package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carlosprados/mdbrand/internal/brand"
)

func TestScanLogFindsFrameOverflows(t *testing.T) {
	log := "Overfull \\vbox (67.15442pt too high) detected at line 364\n []\n\n[4\n\n]\n" +
		"Overfull \\hbox (3.0pt too wide) in paragraph at lines 3--4\n"
	f := scanLog(log).Frames
	if len(f) != 1 || f[0].Line != 364 || f[0].Pt < 67 || f[0].Pt > 67.2 {
		t.Errorf("frames = %+v", f)
	}
}

// The log names a line, and a reader knows a slide by its title. The cover
// slides are the trap: they are not frame environments, and searching up for
// one would name the slide before them.
func TestFrameTitleNamesTheSlide(t *testing.T) {
	tex := strings.Join([]string{
		`\frame{\titlepage}`,                                        // 1
		`\section{Figures \& more}\label{figures}`,                  // 2
		`\begin{frame}[fragile]{Un job en \texttt{Goja} --- \{x\}}`, // 3
		`body`,                                 // 4
		`\end{frame}`,                          // 5
		`\section{A section title long enough`, // 6
		`to wrap}\label{s}`,                    // 7
		`\begin{frame}`,                        // 8
		`\end{frame}`,                          // 9
	}, "\n")
	lines := strings.Split(tex, "\n")
	for _, c := range []struct {
		line int
		want string
	}{
		{1, "the title slide"},
		{2, `the section slide "Figures & more"`},
		{5, `"Un job en Goja — {x}"`},
		{7, `the section slide "A section title long enough to wrap"`},
		{9, "(untitled, line 8 of the .tex)"},
		{4, "(line 4 of the .tex)"},
		{99, "(line 99 of the .tex)"},
	} {
		if got := frameTitle(lines, c.line); got != c.want {
			t.Errorf("line %d: got %s, want %s", c.line, got, c.want)
		}
	}
}

func TestJudgeFramesIgnoresRounding(t *testing.T) {
	src := "\\begin{frame}{A}\n\\end{frame}"
	if err := judgeFrames([]frameOver{{Pt: 0.4, Line: 2}}, src); err != nil {
		t.Errorf("under a point is rounding, not a defect: %v", err)
	}
	err := judgeFrames([]frameOver{{Pt: 28.45, Line: 2}}, src)
	if err == nil || !strings.Contains(err.Error(), `"A"`) || !strings.Contains(err.Error(), "10.0 mm") {
		t.Errorf("want the slide named and the excess in mm, got %v", err)
	}
}

// beamer reports a slide with pauses once per overlay, all at its \end{frame}.
func TestJudgeFramesCountsOverlaysOnce(t *testing.T) {
	src := "\\begin{frame}{A}\n\\end{frame}"
	err := judgeFrames([]frameOver{{Pt: 20, Line: 2}, {Pt: 28.45, Line: 2}, {Pt: 5, Line: 2}}, src)
	if err == nil || !strings.HasPrefix(err.Error(), "1 slide(s)") ||
		strings.Count(err.Error(), `"A"`) != 1 || !strings.Contains(err.Error(), "10.0 mm") {
		t.Errorf("want one slide, as tall as its tallest overlay, got %v", err)
	}
}

// The accent is measured only when a document prints words in it, stops a
// deck and warns on a page — a pale accent has always printed on paper.
func TestAccentContrast(t *testing.T) {
	b := brand.Default()
	b.Colors.Primary, b.Colors.Accent = "F68E1B", "F68E1B"
	rep := &Report{}
	if err := accentContrast(b, "no accents here", true, rep); err != nil || len(rep.Warnings) > 0 {
		t.Errorf("a document with no accent must not be measured: %v %v", err, rep.Warnings)
	}
	err := accentContrast(b, "MDBRAND-ACCENT\n", true, rep)
	if err == nil || !strings.Contains(err.Error(), "colors.accent") || !strings.Contains(err.Error(), "AE6413") {
		t.Errorf("a deck must stop, naming colors.accent and a shade that passes: %v", err)
	}
	if err := accentContrast(b, "MDBRAND-ACCENT\n", false, rep); err != nil || len(rep.Warnings) != 1 {
		t.Errorf("a page must warn, not stop: %v %v", err, rep.Warnings)
	}
	b.Colors.Accent = "AE6413"
	if err := accentContrast(b, "MDBRAND-ACCENT\n", true, &Report{}); err != nil {
		t.Errorf("a dark enough accent must pass: %v", err)
	}
}

// A bundle without colors.accent must leave the filter, and so the .tex,
// byte for byte as it was.
func TestSpanFilterNamesTheAccentOnlyWhenItDiffers(t *testing.T) {
	b := brand.Default()
	for _, c := range []struct{ accent, want string }{{b.Colors.Primary, "brandPrimary"}, {"AE6413", "brandAccent"}} {
		b.Colors.Accent = c.accent
		work := t.TempDir()
		if _, err := writeSpanFilter(work, b); err != nil {
			t.Fatal(err)
		}
		raw, _ := os.ReadFile(filepath.Join(work, spanFilterName))
		if !strings.Contains(string(raw), `\\textcolor{`+c.want+`}`) {
			t.Errorf("accent %s: filter does not name %s", c.accent, c.want)
		}
	}
}
