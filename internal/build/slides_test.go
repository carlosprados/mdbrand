package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carlosprados/mdbrand/internal/brand"
	"github.com/carlosprados/mdbrand/internal/doc"
	"github.com/carlosprados/mdbrand/internal/fig"
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
	if err := judgeFrames([]frameOver{{Pt: 0.4, Line: 2}}, src, nil, 8.5, coverLogo{}); err != nil {
		t.Errorf("under a point is rounding, not a defect: %v", err)
	}
	err := judgeFrames([]frameOver{{Pt: 28.45, Line: 2}}, src, nil, 8.5, coverLogo{})
	if err == nil || !strings.Contains(err.Error(), `"A"`) || !strings.Contains(err.Error(), "10.0 mm") {
		t.Errorf("want the slide named and the excess in mm, got %v", err)
	}
}

// beamer reports a slide with pauses once per overlay, all at its \end{frame}.
func TestJudgeFramesCountsOverlaysOnce(t *testing.T) {
	src := "\\begin{frame}{A}\n\\end{frame}"
	err := judgeFrames([]frameOver{{Pt: 20, Line: 2}, {Pt: 28.45, Line: 2}, {Pt: 5, Line: 2}}, src, nil, 8.5, coverLogo{})
	if err == nil || !strings.HasPrefix(err.Error(), "1 slide(s)") ||
		strings.Count(err.Error(), `"A"`) != 1 || !strings.Contains(err.Error(), "10.0 mm") {
		t.Errorf("want one slide, as tall as its tallest overlay, got %v", err)
	}
}

// A slide with one figure is told the width that makes room: the figure's
// height and labels shrink with its width, so 100 mm at 48 mm tall and 10pt,
// over by 10 mm, fits at 79 mm wide and 7.9pt — under a 8.5pt floor, over 7.
func TestJudgeFramesSizesTheFigure(t *testing.T) {
	src := "\\begin{frame}{A}\n\\includegraphics[width=100mm,height=48mm]{fig00.pdf}\n\\end{frame}"
	figs := []*fig.Result{{
		Fig: &doc.Fig{SrcPath: "/w/fig00.d2", Caption: "Where a job goes"}, PDF: "/w/fig00.pdf",
		WidthMM: 100, HeightMM: 48, TextPt: 10,
	}}
	over := []frameOver{{Pt: 10 / mmPerPt, Line: 3}}
	err := judgeFrames(over, src, figs, 7, coverLogo{})
	if err == nil || !strings.Contains(err.Error(), "width=79mm") || !strings.Contains(err.Error(), "7.9pt") ||
		!strings.Contains(err.Error(), `"Where a job goes" (fig00.d2)`) {
		t.Errorf("want the width that makes room and the labels' size, got %v", err)
	}
	err = judgeFrames(over, src, figs, 8.5, coverLogo{})
	if err == nil || strings.Contains(err.Error(), "width=") || !strings.Contains(err.Error(), "a slide of its own") {
		t.Errorf("under the floor no width will do, got %v", err)
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

// A deck's footline writes each slide's title and number several times a
// page; the warnings since the last page belong to the mark that ships the
// next one — the one before its [N, logo lines between — and an overlay's
// repeat is the same slide. The footer's own, from the output routine, is not.
func TestSlideOverflowsByTheMarkThatShips(t *testing.T) {
	log := `MDBRAND-TITLE {Fine\ifnum \beamer@autobreakcount >0 \relax {}\space \beamer@ins
ertcontinuationcount \fi }
MDBRAND-SLIDE 2
[2
]
MDBRAND-TITLE {Fine\ifnum \beamer@autobreakcount >0 \relax {}\space \beamer@ins
MDBRAND-SLIDE 3
Overfull \hbox (38.9pt too wide) in paragraph at lines 345--345
Overfull \hbox (3.0pt too wide) in paragraph at lines 346--346
Overfull \hbox (60.0pt too wide) has occurred while \output is active
MDBRAND-TITLE {Two --- columns\ifnum \beamer@autobreakcount >0 \relax {}\space
MDBRAND-SLIDE 3
File: logo.pdf Graphic file (type pdf)
<use logo.pdf>
[3
]
Overfull \hbox (40.0pt too wide) in paragraph at lines 345--345
MDBRAND-TITLE {Two --- columns\ifnum \beamer@autobreakcount >0 \relax {}\space
MDBRAND-SLIDE 3
[4
]`
	got := slideOverflows(strings.Split(log, "\n"))
	if len(got) != 1 || got[0].Num != "3" || got[0].Pt != 40 || got[0].Title != "Two — columns" {
		t.Fatalf("got %+v", got)
	}
}

// pandoc gives a column its width= as a share of the line, and 0.48 when it
// has none; a figure in a nested div is still in its column, and one in a
// code sample is not a figure at all.
func TestColumnSharesFollowPandoc(t *testing.T) {
	body := ":::: columns\n::: column\n@@MDBRAND_FIG_0@@\n:::\n" +
		"::: {.column width=\"30%\"}\n::: note\nsee @@MDBRAND_FIG_1@@\n:::\n:::\n::::\n" +
		"```\n::: column\n@@MDBRAND_FIG_9@@\n```\n\n@@MDBRAND_FIG_2@@\n"
	got := columnShares(body)
	want := map[string]float64{"@@MDBRAND_FIG_0@@": 0.48, "@@MDBRAND_FIG_1@@": 0.3}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %g, want %g", k, got[k], v)
		}
	}
}

// A square mark at the default 34 mm filled the title slide, and the advice
// blamed the title. Measured on a real bundle: 18.4 mm over at 34 mm, and
// 15 mm the widest that fits.
func TestTitleSlideAdviceNamesTheLogo(t *testing.T) {
	square := coverLogo{Width: "34mm", WidthMM: 34, Aspect: 1}
	if got := titleSlideAdvice(18.4, square); !strings.Contains(got, "slides.logo_width_cover: 15mm") || !strings.Contains(got, "34 mm tall") {
		t.Errorf("square logo: %q", got)
	}
	for name, logo := range map[string]coverLogo{
		"no logo":                    {},
		"shrinking it is not enough": square, // 30 mm over would leave a 4 mm mark
	} {
		over := 18.4
		if logo.WidthMM > 0 {
			over = 30
		}
		if got := titleSlideAdvice(over, logo); strings.Contains(got, "logo_width_cover") || !strings.Contains(got, "shorten the title") {
			t.Errorf("%s: %q", name, got)
		}
	}
}
