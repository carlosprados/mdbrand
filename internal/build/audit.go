package build

import (
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
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
	// Anonymous is a submission that must not name its author: an author in
	// the PDF's properties is then a defect, not a missing field.
	Anonymous bool
}

// Audit is what reading a finished PDF and its log found. Defects are
// measured on the PDF or written in the log; warnings are worth a look.
type Audit struct {
	Pages     int
	Paper     string
	BodyPages int // the pages counted against MaxPages; 0 without BodyUntil
	// BodyLength is the body's length in pages, the last one in part: 9.6
	// says how much there is to cut, or how much room is left.
	BodyLength float64
	Log        string // the log read; "" when there was none
	// Underfull counts the lines TeX set loose, and WorstBadness the worst:
	// spacing, never text out of place, so neither is a warning.
	Underfull, WorstBadness int
	Defects                 []string
	Warnings                []string
	// Notes say what was not measured, and why.
	Notes []string
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
	a.judgeMeta(string(info), pdf, o.Anonymous)
	if paper != "" && a.Paper != paper {
		a.Defects = append(a.Defects, fmt.Sprintf("the paper is %s, and %s was asked for", a.Paper, paper))
	}

	text, err := run.Stdout(dir, "pdftotext", "-enc", "UTF-8", pdf, "-")
	if err != nil {
		return nil, err
	}
	var layout string
	if o.BodyUntil != "" {
		raw, err := run.Stdout(dir, "pdftotext", "-enc", "UTF-8", "-bbox-layout", pdf, "-")
		if err != nil {
			return nil, err
		}
		layout = string(raw)
	}
	a.countPages(o, strings.Split(string(text), "\f"), layout)
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
	if run.Have("mutool") {
		stext, err := run.Stdout(dir, "mutool", "draw", "-q", "-F", "stext", "-o", "-", pdf)
		if err != nil {
			return nil, err
		}
		a.judgeTextSizes(stext)
	} else {
		a.Notes = append(a.Notes, "text sizes not measured: mutool is not installed (apt install mupdf-tools, brew install mupdf-tools)")
	}

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

// countPages holds the document to its page limit: all its pages, or the
// body up to the BodyUntil heading. The body's length is measured to the
// heading's line, so a heading at the top of page 11 leaves a 10-page body;
// without a layout to measure, the heading's page counts whole.
func (a *Audit) countPages(o AuditOptions, pages []string, layout string) {
	counted, what := a.Pages, fmt.Sprintf("%d pages", a.Pages)
	if o.BodyUntil != "" {
		p := headingPage(pages, o.BodyUntil)
		switch {
		case p == 0:
			a.Warnings = append(a.Warnings, fmt.Sprintf("no line reads %q, so every page was counted against the limit", o.BodyUntil))
		default:
			a.BodyPages, counted = p, p
			what = fmt.Sprintf("%d pages up to %q", p, o.BodyUntil)
			if length, ok := bodyLength(layoutPages(layout), o.BodyUntil); ok {
				a.BodyLength = length
				// A heading under the running header at the top of a page
				// starts that page: the body ended on the one before.
				a.BodyPages = max(int(math.Ceil(length-0.05)), 1)
				counted = a.BodyPages
				what = fmt.Sprintf("the body runs %.1f pages up to %q", length, o.BodyUntil)
			}
		}
	}
	if o.MaxPages > 0 && counted > o.MaxPages {
		a.Defects = append(a.Defects, fmt.Sprintf("%s, and the limit is %d", what, o.MaxPages))
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

// underfullRe is a line or a page TeX set looser than it liked.
var underfullRe = regexp.MustCompile(`Underfull \\[hv]box \(badness (\d+)\)`)

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
	for _, m := range underfullRe.FindAllStringSubmatch(log, -1) {
		a.Underfull++
		if b, _ := strconv.Atoi(m[1]); b > a.WorstBadness {
			a.WorstBadness = b
		}
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

// pdfInfoField is one field of pdfinfo's output, trimmed.
func pdfInfoField(info, key string) string {
	for _, ln := range strings.Split(info, "\n") {
		if k, v, ok := strings.Cut(ln, ":"); ok && strings.TrimSpace(k) == key {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// judgeMeta reads the title and author the PDF carries in its properties:
// what a reader's tab, a library and a marking system show. An anonymous
// submission that names its author has given itself away.
func (a *Audit) judgeMeta(info, file string, anonymous bool) {
	title, author := pdfInfoField(info, "Title"), pdfInfoField(info, "Author")
	const fix = `\hypersetup{pdftitle={…}, pdfauthor={…}} after \usepackage{hyperref}`
	switch {
	case title == "":
		a.Warnings = append(a.Warnings, "the PDF has no title, so a reader's tab and a library show the file name: "+fix)
	case titleIsFileName(title, file):
		a.Warnings = append(a.Warnings, fmt.Sprintf("the PDF's title is %q, a file's name rather than the document's: %s", title, fix))
	}
	switch {
	case anonymous && author != "":
		a.Defects = append(a.Defects, fmt.Sprintf(`the PDF names its author, %q, and the submission is anonymous: set \hypersetup{pdfauthor={}}, and check the text for the name too`, author))
	case !anonymous && author == "":
		a.Warnings = append(a.Warnings, "the PDF names no author: "+fix+" — or pass --anonymous for a submission that must not")
	}
}

// titleIsFileName tells a title some tool filled in from a file: the PDF's
// own name, a source's, "untitled", or Word's "Microsoft Word - x.docx".
func titleIsFileName(title, file string) bool {
	t := strings.ToLower(strings.TrimSpace(title))
	f := strings.ToLower(file)
	if t == f || t == strings.TrimSuffix(f, filepath.Ext(f)) || t == "untitled" || strings.HasPrefix(t, "microsoft word - ") {
		return true
	}
	for _, ext := range []string{".tex", ".pdf", ".dvi", ".docx", ".doc", ".md"} {
		if strings.HasSuffix(t, ext) {
			return true
		}
	}
	return false
}

// smallTextPt is the size under which the audit names text as too small to
// read on paper. Below the 7pt of a template's running header and a
// subscript's, where a figure scaled to the column lands its labels.
const smallTextPt = 6.0

// judgeTextSizes reads mutool's structured text, where each run of text
// carries the size it prints at — a figure's font scaled by its placement,
// which is how a chart drawn at 15 cm and set at \textwidth gets 5pt
// labels. A run counts when it has two letters: a lone sub-subscript or a
// footnote mark is not prose.
func (a *Audit) judgeTextSizes(stext []byte) {
	dec := xml.NewDecoder(strings.NewReader(xmlSafe(string(stext))))
	page, size := 0, 0.0
	var run strings.Builder
	var found []string
	more := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "page":
				page++
			case "font":
				size, run = 0, strings.Builder{}
				for _, at := range t.Attr {
					if at.Name.Local == "size" {
						size, _ = strconv.ParseFloat(at.Value, 64)
					}
				}
			case "char":
				for _, at := range t.Attr {
					if at.Name.Local == "c" {
						run.WriteString(at.Value)
					}
				}
			}
		case xml.EndElement:
			if t.Name.Local != "font" || size <= 0 || size >= smallTextPt || letters(run.String()) < 2 {
				continue
			}
			if len(found) < 8 {
				found = append(found, fmt.Sprintf("page %d: %.1fpt %q", page, size, strings.TrimSpace(run.String())))
			} else {
				more++
			}
		}
	}
	if len(found) == 0 {
		return
	}
	if more > 0 {
		found = append(found, fmt.Sprintf("and %d more", more))
	}
	a.Warnings = append(a.Warnings, fmt.Sprintf("text under %.0fpt, too small to read on paper — usually a figure drawn wide and scaled to the column;\n"+
		"    draw it at the width it prints (matplotlib: figsize in inches of that width) with labels of 8pt or more:\n    %s",
		smallTextPt, strings.Join(found, "\n    ")))
}

// xmlSafe drops the characters XML does not allow, written or as a
// reference. mutool writes &#xffff; for a glyph it cannot map — exactly the
// PDFs worth auditing — and Go's decoder stopped there, so every page after
// it went unread with no error.
func xmlSafe(s string) string {
	s = strings.Map(func(r rune) rune {
		if xmlChar(r) {
			return r
		}
		return -1
	}, s)
	return charRefRe.ReplaceAllStringFunc(s, func(ref string) string {
		body := strings.TrimSuffix(strings.TrimPrefix(ref, "&#"), ";")
		var n int64
		var err error
		if strings.HasPrefix(body, "x") || strings.HasPrefix(body, "X") {
			n, err = strconv.ParseInt(body[1:], 16, 32)
		} else {
			n, err = strconv.ParseInt(body, 10, 32)
		}
		if err != nil || !xmlChar(rune(n)) {
			return ""
		}
		return ref
	})
}

var charRefRe = regexp.MustCompile(`&#[xX]?[0-9a-fA-F]+;`)

func xmlChar(r rune) bool {
	return r == '\t' || r == '\n' || r == '\r' || (r >= 0x20 && r <= 0xD7FF) || (r >= 0xE000 && r <= 0xFFFD) || (r >= 0x10000 && r <= 0x10FFFF)
}

func letters(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			n++
		}
	}
	return n
}

// layoutLine is one line of pdftotext's -bbox-layout, in points from the
// top of its page.
type layoutLine struct {
	yMin, yMax float64
	text       string
}

// layoutPages reads pdftotext -bbox-layout into each page's lines.
func layoutPages(layout string) [][]layoutLine {
	var pages [][]layoutLine
	dec := xml.NewDecoder(strings.NewReader(xmlSafe(layout)))
	var line *layoutLine
	var words []string
	inWord := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "page":
				pages = append(pages, nil)
			case "line":
				line, words = &layoutLine{}, nil
				for _, at := range t.Attr {
					v, _ := strconv.ParseFloat(at.Value, 64)
					switch at.Name.Local {
					case "yMin":
						line.yMin = v
					case "yMax":
						line.yMax = v
					}
				}
			case "word":
				inWord = true
			}
		case xml.CharData:
			if inWord {
				words = append(words, string(t))
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "word":
				inWord = false
			case "line":
				if line != nil && len(pages) > 0 {
					line.text = strings.Join(words, " ")
					pages[len(pages)-1] = append(pages[len(pages)-1], *line)
				}
				line = nil
			}
		}
	}
	return pages
}

// bodyLength is how many pages the body fills before the heading: the
// pages before the heading's, and the part of that one above it, measured
// against the text's usual top and bottom.
func bodyLength(pages [][]layoutLine, heading string) (float64, bool) {
	want := strings.ToLower(strings.TrimSpace(heading))
	var tops, bottoms []float64
	for _, pg := range pages {
		if len(pg) == 0 {
			continue
		}
		top, bottom := pg[0].yMin, pg[0].yMax
		for _, l := range pg {
			top, bottom = math.Min(top, l.yMin), math.Max(bottom, l.yMax)
		}
		tops, bottoms = append(tops, top), append(bottoms, bottom)
	}
	if len(tops) == 0 {
		return 0, false
	}
	top, bottom := median(tops), median(bottoms)
	if bottom <= top {
		return 0, false
	}
	for i, pg := range pages {
		for _, l := range pg {
			t := strings.ToLower(strings.TrimSpace(sectionNumberRe.ReplaceAllString(strings.TrimSpace(l.text), "")))
			if t == want {
				fill := math.Max(0, math.Min(1, (l.yMin-top)/(bottom-top)))
				return float64(i) + fill, true
			}
		}
	}
	return 0, false
}

func median(v []float64) float64 {
	s := slices.Clone(v)
	slices.Sort(s)
	return s[len(s)/2]
}
