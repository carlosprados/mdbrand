package build

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/carlosprados/mdbrand/internal/exit"
	"github.com/carlosprados/mdbrand/internal/run"
)

// AuditOptions name a PDF mdbrand did not build, and what it must meet.
type AuditOptions struct {
	PDF string
	// Log is the LaTeX log; "" reads the .log beside the PDF when there is one.
	Log      string
	MaxPages int
	// BodyUntil is the heading the counted pages stop at ("References"), for
	// a limit that leaves out references and appendices.
	BodyUntil string
	Paper     string // "a4" or "letter"; "" accepts any
}

// Audit is what reading a finished PDF and its log found. Defects are
// measured on the PDF or written in the log; warnings are worth a look.
type Audit struct {
	Pages     int
	Paper     string
	BodyPages int    // the pages counted against MaxPages; 0 without BodyUntil
	Log       string // the log read; "" when there was none
	Defects   []string
	Warnings  []string
}

// RunAudit makes the checks a build makes on its own PDF on someone else's: a PDF from
// Overleaf, a journal's class or a coursework template prints the same
// defects, and its log says the same things to nobody.
func RunAudit(o AuditOptions) (*Audit, error) {
	if missing := run.Missing("pdfinfo", "pdftotext", "pdffonts", "pdfimages"); len(missing) > 0 {
		return nil, exit.AsEnvironment(fmt.Errorf("missing tools: %s — they are poppler-utils (brew install poppler)\n  run: mdbrand doctor",
			strings.Join(missing, ", ")))
	}
	if _, err := os.Stat(o.PDF); err != nil {
		return nil, err
	}
	paper := strings.ToLower(o.Paper)
	if paper != "" && paper != "a4" && paper != "letter" {
		return nil, exit.AsUsage(fmt.Errorf("--paper %s: the papers are a4 and letter", o.Paper))
	}
	dir, pdf := filepath.Dir(mustAbs(o.PDF)), filepath.Base(o.PDF)
	a := &Audit{}

	info, err := run.Stdout(dir, "pdfinfo", pdf)
	if err != nil {
		return nil, err
	}
	a.Pages, a.Paper = pdfPages(string(info))
	if paper != "" && a.Paper != paper {
		a.Defects = append(a.Defects, fmt.Sprintf("the paper is %s, and %s was asked for", a.Paper, paper))
	}

	text, err := run.Stdout(dir, "pdftotext", "-enc", "UTF-8", pdf, "-")
	if err != nil {
		return nil, err
	}
	a.countPages(o, strings.Split(string(text), "\f"))
	if bad := privateUse(string(text), ""); len(bad) > 0 {
		a.Defects = append(a.Defects, "the text layer holds private-use characters, so copying, search and a plagiarism checker read garbage at:\n    "+
			strings.Join(bad, "\n    ")+"\n  the face maps alternate glyphs to private-use code points: set it with contextual alternates off, or use another face")
	}
	bbox, err := run.Stdout(dir, "pdftotext", "-enc", "UTF-8", "-bbox", pdf, "-")
	if err != nil {
		return nil, err
	}
	if cut := offPaper(string(bbox)); len(cut) > 0 {
		a.Defects = append(a.Defects, fmt.Sprintf("%d line(s) run off the paper, cut at the edge of the sheet:\n    %s",
			len(cut), strings.Join(cut, "\n    ")))
	}

	fonts, err := run.Stdout(dir, "pdffonts", pdf)
	if err != nil {
		return nil, err
	}
	a.judgeFonts(string(fonts))
	images, err := run.Stdout(dir, "pdfimages", "-list", pdf)
	if err != nil {
		return nil, err
	}
	a.judgeImages(string(images))

	if err := a.judgeLaTeXLog(o, a.Pages); err != nil {
		return nil, err
	}
	return a, nil
}

// pdfPages reads the page count and the first page's paper from pdfinfo.
func pdfPages(info string) (int, string) {
	var pages int
	paper := "unknown"
	for _, ln := range strings.Split(info, "\n") {
		k, v, ok := strings.Cut(ln, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "Pages":
			pages, _ = strconv.Atoi(strings.TrimSpace(v))
		case "Page size":
			var w, h float64
			if _, err := fmt.Sscanf(strings.TrimSpace(v), "%f x %f", &w, &h); err == nil {
				paper = paperName(w, h)
			}
		}
	}
	return pages, paper
}

// paperName names a page by its size in points, either way up. Within 2pt:
// A4 is 595.28 × 841.89, and a PDF rounds it.
func paperName(w, h float64) string {
	w, h = math.Min(w, h), math.Max(w, h)
	near := func(a, b float64) bool { return math.Abs(a-b) <= 2 }
	switch {
	case near(w, 595.28) && near(h, 841.89):
		return "a4"
	case near(w, 612) && near(h, 792):
		return "letter"
	}
	return fmt.Sprintf("%.0f × %.0f pt", w, h)
}

// countPages holds the document to its page limit: all its pages, or those
// up to the page where the BodyUntil heading starts, that page included,
// since the body may end on it.
func (a *Audit) countPages(o AuditOptions, pages []string) {
	counted, what := a.Pages, "pages"
	if o.BodyUntil != "" {
		if p := headingPage(pages, o.BodyUntil); p > 0 {
			a.BodyPages, counted = p, p
			what = fmt.Sprintf("pages up to %q", o.BodyUntil)
		} else {
			a.Warnings = append(a.Warnings, fmt.Sprintf("no line reads %q, so every page was counted against the limit", o.BodyUntil))
		}
	}
	if o.MaxPages > 0 && counted > o.MaxPages {
		a.Defects = append(a.Defects, fmt.Sprintf("%d %s, and the limit is %d", counted, what, o.MaxPages))
	}
}

// headingPage is the first page, from 1, holding a line that is the heading
// alone, numbered or not ("References", "8 References", "8. References");
// 0 when none does. A sentence mentioning the word is not the heading.
func headingPage(pages []string, heading string) int {
	want := strings.ToLower(strings.TrimSpace(heading))
	for i, page := range pages {
		for _, ln := range strings.Split(page, "\n") {
			ln = strings.ToLower(strings.TrimSpace(sectionNumberRe.ReplaceAllString(strings.TrimSpace(ln), "")))
			if ln == want {
				return i + 1
			}
		}
	}
	return 0
}

var sectionNumberRe = regexp.MustCompile(`^[0-9A-Z]+(\.[0-9]+)*\.?\s+`)

// judgeFonts reads pdffonts. A face not embedded prints in whatever the
// reader's machine substitutes, and journals' checkers refuse it; a Type 3
// face is drawn by procedures — matplotlib's default for PDF figures — and
// IEEE's PDF eXpress refuses it as well.
func (a *Audit) judgeFonts(list string) {
	var bare, type3 []string
	for _, ln := range strings.Split(list, "\n") {
		f := strings.Fields(ln)
		// The header and its rule of dashes have no yes/no in the emb column.
		if len(f) < 6 || strings.HasPrefix(ln, "---") {
			continue
		}
		name, emb := f[0], f[len(f)-5]
		if emb == "no" && !slices.Contains(bare, name) {
			bare = append(bare, name)
		}
		if strings.Contains(ln, "Type 3") && !slices.Contains(type3, name) {
			type3 = append(type3, name)
		}
	}
	if len(bare) > 0 {
		a.Defects = append(a.Defects, fmt.Sprintf("face(s) not embedded, so another machine prints them in a substitute: %s\n"+
			"  usually a figure's PDF: export it with its fonts embedded, or as PNG", strings.Join(bare, ", ")))
	}
	if len(type3) > 0 {
		a.Warnings = append(a.Warnings, fmt.Sprintf("Type 3 face(s), which print blurred on screen and which journals' PDF checkers (IEEE PDF eXpress) refuse: %s.\n"+
			"    Either a bitmap face LaTeX drew with METAFONT — T1 fontenc on a TeX without cm-super: install cm-super, or set\n"+
			"    \\renewcommand{\\ttdefault}{lmtt} for typewriter text (\\usepackage{lmodern} would also replace a template's body face) —\n"+
			"    or a matplotlib figure saved as PDF: set matplotlib.rcParams[\"pdf.fonttype\"] = 42", strings.Join(type3, ", ")))
	}
}

// judgeImages reads `pdfimages -list` and holds every raster picture to the
// resolution a page needs, as a build does with the document's own pictures.
// Pictures under 16 pixels a side are rules and spacers, not pictures.
func (a *Audit) judgeImages(list string) {
	var low, soft []string
	for _, ln := range strings.Split(list, "\n") {
		f := strings.Fields(ln)
		if len(f) < 14 || f[2] != "image" {
			continue
		}
		w, _ := strconv.Atoi(f[3])
		h, _ := strconv.Atoi(f[4])
		x, errX := strconv.Atoi(f[12])
		y, errY := strconv.Atoi(f[13])
		if errX != nil || errY != nil || w < 16 || h < 16 {
			continue
		}
		ppi := min(x, y)
		if ppi >= warnPPI {
			continue
		}
		good := math.Floor(float64(w) / warnPPI * 25.4)
		s := fmt.Sprintf("page %s: a %d×%d px picture prints at %d ppi (at %.0fmm wide it would print at %d)", f[0], w, h, ppi, good, warnPPI)
		if ppi < minPPI {
			low = append(low, s)
		} else {
			soft = append(soft, s)
		}
	}
	if len(low) > 0 {
		a.Defects = append(a.Defects, fmt.Sprintf("picture(s) under %d ppi, which print blocky:\n    %s", minPPI, strings.Join(low, "\n    ")))
	}
	if len(soft) > 0 {
		a.Warnings = append(a.Warnings, fmt.Sprintf("picture(s) under %d ppi, which print soft:\n    %s", warnPPI, strings.Join(soft, "\n    ")))
	}
}

// unwrapLog joins the lines TeX broke at its 79-column limit, which it does
// mid-word and mid-font-name with nothing at the break. A line exactly that
// long is taken to continue on the next.
func unwrapLog(log string) string {
	var b strings.Builder
	for _, ln := range strings.Split(log, "\n") {
		b.WriteString(ln)
		if utf8.RuneCountInString(ln) != 79 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// pdflatex writes a missing glyph without the code point XeLaTeX adds.
var anyMissingRe = regexp.MustCompile(`Missing character: There is no (.+?) in font ([^!]+)!`)

// rerunRe is LaTeX asking for one more pass: the PDF's references, contents
// or citations are from the pass before.
var rerunRe = regexp.MustCompile(`Rerun to get|Label\(s\) may have changed|Please \(re\)run`)

// judgeLaTeXLog reads what the LaTeX run said and nobody read. The log must
// be this PDF's: one from another run would vouch for a different document.
func (a *Audit) judgeLaTeXLog(o AuditOptions, pages int) error {
	path := o.Log
	if path == "" {
		guess := strings.TrimSuffix(o.PDF, filepath.Ext(o.PDF)) + ".log"
		if _, err := os.Stat(guess); err != nil {
			a.Warnings = append(a.Warnings, "no LaTeX log beside the PDF, so missing glyphs, undefined references and overfull lines were not read: pass --log (Overleaf: Logs and output files → Other logs and files)")
			return nil
		}
		path = guess
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return exit.AsUsage(fmt.Errorf("--log %s: no such file", path))
		}
		return err
	}
	a.Log = path
	log := unwrapLog(string(raw))
	sc := scanLog(log)
	if sc.Pages > 0 && sc.Pages != pages {
		a.Warnings = append(a.Warnings, fmt.Sprintf("the log is of a %d-page run and the PDF has %d pages: it may be another build's", sc.Pages, pages))
	}

	holes := sc.Holes
	for _, m := range anyMissingRe.FindAllStringSubmatch(log, -1) {
		if strings.Contains(m[1], "(U+") {
			continue // XeLaTeX's form, which scanLog has read
		}
		if h := fmt.Sprintf("%s missing from %s", m[1], strings.TrimSpace(m[2])); !slices.Contains(holes, h) {
			holes = append(holes, h)
		}
	}
	if len(holes) > 0 {
		a.Defects = append(a.Defects, fmt.Sprintf("character(s) the face has no glyph for, printed as nothing:\n    %s", strings.Join(holes, "\n    ")))
	}
	if len(sc.Shapes) > 0 {
		a.Defects = append(a.Defects, fmt.Sprintf("a style the face lacks, set in its regular instead (emphasis printed plain):\n    %s", strings.Join(sc.Shapes, "\n    ")))
	}
	if len(sc.Undefined) > 0 {
		a.Defects = append(a.Defects, fmt.Sprintf("reference(s) or citation(s) to nothing, printed as ?? or [?]:\n    %s", strings.Join(sc.Undefined, "\n    ")))
	}
	if len(sc.PagesOver) > 0 {
		a.Defects = append(a.Defects, pagesOverError(sc.PagesOver).Error())
	}
	// fancyhdr 4 enlarges \headheight itself after the first page it is short
	// on, so the layout shifts once rather than overprinting throughout.
	if sc.HeadShortPt > 0 {
		a.Warnings = append(a.Warnings, fmt.Sprintf("the running header is %.1fpt taller than \\headheight: the first pages are laid out differently from the rest — raise \\headheight", sc.HeadShortPt))
	}
	if rerunRe.MatchString(log) {
		a.Warnings = append(a.Warnings, "LaTeX asked for another run: references, contents or citations may be the previous pass's — compile again")
	}
	if len(sc.Over) > 0 {
		var w strings.Builder
		fmt.Fprintf(&w, "%d line(s) overflow the measure by more than 5pt:", len(sc.Over))
		for _, ov := range sc.Over {
			if ov.Text == "" {
				fmt.Fprintf(&w, "\n    %5.1fpt  (no quotable text — typically verbatim or a table)", ov.Pt)
				continue
			}
			fmt.Fprintf(&w, "\n    %5.1fpt  %q", ov.Pt, ov.Text)
		}
		a.Warnings = append(a.Warnings, w.String())
	}
	return nil
}
