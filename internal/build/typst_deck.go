package build

// A deck (style slides) typeset by Typst instead of beamer. typst.lua
// cuts the document into slides; the template below draws each, measures it,
// and stops the build by its title when it does not fit.

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/carlosprados/mdbrand/internal/run"
	"github.com/carlosprados/mdbrand/internal/tex"
)

type typstDeckData struct {
	typstData
	Ground, OnGround      string
	SlideLogo, Art        string
	LogoWidth, LogoCover  string
	Date, Authors         string
	MarginMM              float64
	Name                  string
	CoverRule, CoverTitle string
}

func typstDeckTemplate(p *prepared, faces typstFaces) ([]byte, error) {
	b, m := p.b, p.d.Meta
	td, err := typstBaseData(p, faces)
	if err != nil {
		return nil, err
	}
	q := strconv.Quote
	dd := typstDeckData{typstData: td, MarginMM: tex.SlideMarginMM,
		LogoWidth: b.Slides.LogoWidth, LogoCover: b.Slides.LogoWidthCover,
		Ground: "ffffff", OnGround: b.Colors.Text}
	if b.Slides.Dark() {
		dd.Ground, dd.OnGround = b.Slides.Background, b.Slides.Foreground
	}
	if l := b.SlideLogoPath(); l != "" {
		dd.SlideLogo = q(l)
	}
	if a := b.SlideArtPath(); a != "" {
		dd.Art = q(a)
	}
	dd.Name = "[" + typstEscape(b.DisplayName) + "]"
	dd.Authors = "[" + typstEscape(m.AuthorString()) + "]"
	dd.Date = "[" + typstEscape(m.Date) + "]"
	t, err := template.New("deck").Delims("[[", "]]").Parse(typstDeckSrc)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	err = t.Execute(&out, dd)
	return out.Bytes(), err
}

// typstDeckSrc is the deck: beamer's 16:9 frame, its title slide and
// section slides, a footer of rule, logo, label, title and number, and each
// slide's body measured against the room between its title and its footer.
const typstDeckSrc = `#let brand-primary = rgb("#[[.Primary]]")
#let brand-text = rgb("#[[.Text]]")
#let brand-rule = rgb("#[[.Rule]]")
#let brand-accent = rgb("#[[.Accent]]")
#let ground = rgb("#[[.Ground]]")
#let on-ground = rgb("#[[.OnGround]]")
// pandoc writes a --- between paragraphs as #horizontalrule, which its own
// template defines; XeLaTeX draws half the line's width, centred.
#let horizontalrule = align(center, line(length: 50%, stroke: 0.5pt))
#let display = ([[.Display]], [[.Body]])
#let display-b = ([[.DisplayB]], [[.Body]])
#let display-w = [[.DisplayW]]
#let display-bw = [[.DisplayBW]]
#let running = [[.Running]]
#let side = [[.MarginMM]]mm

#set document(title: [[.Title]][[if .PDFAuthor]], author: [[.PDFAuthor]][[end]])
#set page(width: 160mm, height: 90mm, margin: 0mm)
#set text(font: ([[.Body]],), size: 11pt, lang: [[.Lang]], fill: brand-text, fallback: true)
#set par(justify: false, leading: 0.55em, spacing: 0.9em)
#show raw: set text(font: [[.Mono]], size: 8pt)
#show math.equation: set text(font: "New Computer Modern Math")
#show link: set text(fill: rgb("#[[.Link]]"))
#set figure(numbering: none)
#show figure.caption: set text(size: 8pt)
#set figure(gap: 4pt)
#set list(marker: (text(fill: brand-primary, size: 0.55em, baseline: -0.2em)[■], text(fill: brand-primary, size: 0.55em, baseline: -0.2em)[□], text(fill: brand-primary)[–]))
#set table(stroke: none)
#show table.cell.where(y: 0): set text(weight: "bold")
#let mdbrand-table(rows, captioned, body) = body
#let mdbrand-keep(name, body) = body

#let logo(w) = [[if .Logo]]image([[.Logo]], width: w)[[else]]text(font: display-b, weight: display-bw, [[.Name]])[[end]]
#let cover-logo(w) = [[if .SlideLogo]]image([[.SlideLogo]], width: w)[[else]]logo(w)[[end]]

// The footer's text is measured against the logo and "99 / 99" once: at
// this length the number would be pushed off the slide.
#let foot-text(n, total) = text(font: display, weight: display-w, size: 7pt, fill: brand-text, {
  [[if .Confidential]]text(fill: brand-primary)[[.Confidential]]; h(1em);[[end]] running; h(1em); [#n / #total]
})
#context {
  let over = measure(foot-text(99, 99)).width + measure(logo([[.LogoWidth]])).width + 4mm + 2 * side - 160mm
  if over > 0pt { panic("MDBRAND-SLIDE-FOOT " + repr(calc.round(over.pt(), digits: 1))) }
}

#let foot(n, total) = block(width: 100%, inset: (x: side))[
  #line(length: 100%, stroke: 0.3pt + brand-rule)
  #v(1.2mm, weak: false)
  #grid(columns: (auto, 1fr), align: (left + horizon, right + horizon), logo([[.LogoWidth]]), foot-text(n, total))
  #v(3mm, weak: false)
]

#let cover-page(body) = page(fill: ground, background: [[if .Art]]align(right, image([[.Art]], height: 100%))[[else]]none[[end]], body)

// Title slide.
#cover-page(block(width: 100%, height: 100%, inset: (x: side, top: 2mm, bottom: 4mm), layout(size => {
  set text(fill: on-ground)
  let head = cover-logo([[.LogoCover]])
  let title = [
    #line(length: 20mm, stroke: 1.2pt + brand-primary)
    #v(5mm)
    #text(font: display-b, weight: display-bw, size: 24pt)[[.Title]]
    [[if .Subtitle]]#v(3mm)
    #text(font: display, weight: display-w, size: 13pt)[[.Subtitle]][[end]]
  ]
  let meta = text(font: display, weight: display-w, size: 9pt)[#[[.Authors]][[if .Date]] #h(0.6em)·#h(0.6em) #[[.Date]][[end]][[if .Reference]] \ #[[.Reference]][[end]][[if .Confidential]] \ #text(fill: brand-primary)[[.Confidential]][[end]][[if .Footer]] \ #[[.Footer]][[end]]]
  // Each gap keeps 6 mm at least: at none the subtitle sat on the author.
  let used = measure(block(width: size.width, head)).height + measure(block(width: size.width, title)).height + measure(block(width: size.width, meta)).height + 12mm
  if used > size.height { panic("MDBRAND-COVER-OVER " + repr(calc.round((used - size.height).mm(), digits: 1))) }
  head
  v(1fr, weak: false)
  v(6mm)
  title
  v(1fr)
  v(6mm)
  meta
})))

#let mdbrand-section(n, total, title) = cover-page(block(width: 100%, height: 100%, inset: (x: side))[
  #set text(fill: on-ground)
  #v(1fr)
  #line(length: 20mm, stroke: 1.2pt + brand-primary)
  #v(5mm)
  #text(font: display-b, weight: display-bw, size: 22pt, title)
  #v(1fr)
])

// A slide: its title, its body centred in what is left above the footer,
// measured first. The panic names the slide and how far over it runs, and
// the figures on it, for the width that would make room.
#let mdbrand-slide(n, total, name, figs, title, body) = page(grid(rows: (auto, 1fr, auto),
  block(width: 100%, inset: (x: side, top: 4mm))[
    #text(font: display-b, weight: display-bw, size: 14.4pt, title)
    #v(1.2mm, weak: false)
    #line(length: 12mm, stroke: 0.8pt + brand-primary)
    #v(3mm, weak: false)
  ],
  block(width: 100%, height: 100%, inset: (x: side), layout(size => {
    let h = measure(block(width: size.width, body)).height
    if h > size.height + 0.5pt {
      panic("MDBRAND-SLIDE-OVER " + str(n) + "|" + repr(calc.round((h - size.height).mm(), digits: 1)) + "|" + figs.join(",") + "|" + name)
    }
    block(height: size.height, align(horizon, block(width: 100%, body)))
  })),
  foot(n, total)))

$body$
`

// typstDeckPanic words the deck's own assertions.
func typstDeckPanic(p *prepared, kind, arg string) error {
	switch kind {
	case "SLIDE-OVER":
		parts := strings.SplitN(arg, "|", 4)
		if len(parts) < 4 {
			return nil
		}
		over, _ := strconv.ParseFloat(parts[1], 64)
		msg := fmt.Sprintf("\n    %-40s %5.1f mm too tall", fmt.Sprintf("slide %s %q", parts[0], parts[3]), over)
		if figs := strings.Split(parts[2], ","); len(figs) == 1 && figs[0] != "" {
			for _, r := range p.results {
				if filepath.Base(r.PDF) == figs[0] {
					msg += figureRoom(r, over, p.b.Slides.Diagrams.MinTextPt)
				}
			}
		}
		return fmt.Errorf(`1 slide(s) run past the bottom of the frame, over the footer or off the page:%s
  Split the slide (another ## heading), cut what it says, or give a figure on
  it less height: a diagram that is wider than tall suits a 16:9 frame`, msg)
	case "COVER-OVER":
		over, _ := strconv.ParseFloat(arg, 64)
		return fmt.Errorf(`1 slide(s) run past the bottom of the frame, over the footer or off the page:
    %-40s %5.1f mm too tall%s`, "the title slide", over, titleSlideAdvice(over, measureCoverLogo(p.b)))
	case "SLIDE-FOOT":
		over, _ := strconv.ParseFloat(arg, 64)
		return fmt.Errorf(`the slides' footer is %.1fpt too wide: the running title and the
confidentiality label share one line with the logo and the slide number, and
at this length the number is pushed off the slide. Set mdbrand.short_title to
a shorter form, or shorten mdbrand.confidential — the title slide has room
for both in full`, over)
	case "NOTE-LONG":
		parts := strings.SplitN(arg, "|", 3)
		if len(parts) < 3 {
			return nil
		}
		return fmt.Errorf(`1 speaker note(s) run past the bottom of their page:
    %-40s %3s line(s) too long
  A note has room for %s lines beside its slide. Cut it to what you need to
  say, or split the slide and its note with another ## heading`, strconv.Quote(parts[2]), parts[0], parts[1])
	}
	return nil
}

var (
	slideNumRe = regexp.MustCompile(`(\d+) / \d+\s*$`)
	boxRe      = regexp.MustCompile(`<word xMin="([0-9.]+)" yMin="([0-9.]+)" xMax="([0-9.]+)"`)
)

// typstDeckWidths stops a deck with a word past the slide's side margin: a
// slide has no margin to spare, and Typst lets an unbreakable word run on.
func typstDeckWidths(work, pdf string) error {
	out, err := run.Stdout(work, "pdftotext", "-enc", "UTF-8", "-bbox", pdf, "-")
	if err != nil {
		return err
	}
	edge := (tex.SlideWidthMM - tex.SlideMarginMM) * 72 / 25.4
	var over []string
	for i, page := range strings.Split(string(out), "<page ")[1:] {
		worst := 0.0
		words := boxRe.FindAllStringSubmatch(page, -1)
		for j, w := range words {
			x0, _ := strconv.ParseFloat(w[1], 64)
			y0, _ := strconv.ParseFloat(w[2], 64)
			x1, _ := strconv.ParseFloat(w[3], 64)
			worst = max(worst, x1-edge)
			// A word across the column beside it is drawn over that
			// column's words, on the same line: the boxes overlap.
			for _, v := range words[j+1:] {
				u0, _ := strconv.ParseFloat(v[1], 64)
				v0, _ := strconv.ParseFloat(v[2], 64)
				u1, _ := strconv.ParseFloat(v[3], 64)
				if math.Abs(v0-y0) < 2 && min(x1, u1)-max(x0, u0) > 1 {
					worst = max(worst, min(x1, u1)-max(x0, u0))
				}
			}
		}
		if worst <= overhangPt {
			continue
		}
		num := strconv.Itoa(i + 1)
		if text, err := run.Stdout(work, "pdftotext", "-f", strconv.Itoa(i+1), "-l", strconv.Itoa(i+1), pdf, "-"); err == nil {
			if m := slideNumRe.FindSubmatch(bytes.TrimSpace(text)); m != nil {
				num = string(m[1])
			}
			first := strings.TrimSpace(strings.SplitN(strings.TrimSpace(string(text)), "\n", 2)[0])
			num += fmt.Sprintf(" %q", ellipsize(first, 40))
		}
		over = append(over, fmt.Sprintf("\n    slide %-40s %5.1f mm too wide", num, worst*25.4/72))
	}
	if len(over) == 0 {
		return nil
	}
	return fmt.Errorf(`%d slide(s) hold a line wider than the slide or its column has room for: it
runs into the margin, across the column beside it or off the page:%s
  A word that cannot break did it — a URL, a path, a long name. Write it as a
  `+"`code span`"+` or <https://…>, which break, shorten it, or give its column more
  width`, len(over), strings.Join(over, ""))
}

// renderTypstNotes sets the speaker notes on A4, two slides to a sheet: each
// slide's last page, drawn from the deck's PDF, beside its note. typst.lua
// wrote the notes as it cut the deck.
func renderTypstNotes(p *prepared, out string) error {
	work := p.work
	stem := workStem(p.o.Input)
	faces, err := typstFontSet(work, p.b)
	if err != nil {
		return err
	}
	src := fmt.Sprintf(`#set page(paper: "a4", margin: 10mm)
#set text(font: (%q,), size: 10pt)
#show raw: set text(font: %q, size: 8pt)
#let mdbrand-table(rows, captioned, body) = body
#let mdbrand-keep(name, body) = body
#let half = (297mm - 20mm) / 2
#set block(spacing: 0pt)
#let mdbrand-note(n, total, page-no, name, body) = block(height: half, width: 100%%, breakable: false, inset: (bottom: 4mm))[
  #grid(columns: (100mm, 1fr), column-gutter: 6mm,
    box(stroke: 0.3pt + luma(160), image(%q, page: page-no, width: 100mm)),
    layout(size => {
      let h = measure(block(width: size.width, body)).height + 12pt
      if h > half - 4mm {
        // In lines, as the XeLaTeX notes say it: a writer acts on lines.
        let pitch = measure(block(width: size.width)[x \ x]).height - measure(block(width: size.width)[x]).height
        let over = calc.ceil((h - (half - 4mm)) / pitch)
        let room = calc.floor((half - 4mm - 12pt) / pitch)
        panic("MDBRAND-NOTE-LONG " + str(over) + "|" + str(room) + "|" + name)
      }
      [#text(size: 8pt, fill: luma(90))[#n / #total] \ #body]
    }))
]
`, faces.Body, typstMono, stem+".pdf")
	// Inlined, not included: an included file does not see these functions.
	notes, err := os.ReadFile(filepath.Join(work, "notes-body.typ"))
	if err != nil {
		return err
	}
	src += string(notes)
	name := stem + "-notes"
	if err := os.WriteFile(filepath.Join(work, name+".typ"), []byte(src), 0o644); err != nil {
		return err
	}
	if _, err := run.Cmd(work, "typst", typstCompileArgs(faces, name)...); err != nil {
		return typstPanic(err, p)
	}
	if err := notesEdges(work, name+".pdf", string(notes)); err != nil {
		return err
	}
	count := 0
	if info, err := run.Stdout(work, "pdfinfo", name+".pdf"); err == nil {
		if m := regexp.MustCompile(`(?m)^Pages:\s+(\d+)`).FindSubmatch(info); m != nil {
			count, _ = strconv.Atoi(string(m[1]))
		}
	}
	p.rep.Outputs = append(p.rep.Outputs, Output{Format: "notes", Path: out, Pages: count, built: filepath.Join(work, name+".pdf")})
	return nil
}

var noteNameRe = regexp.MustCompile(`#mdbrand-note\(\d+, \d+, \d+, "((?:[^"\\]|\\.)*)"\)`)

// notesEdges stops a note with a word cut at the sheet's edge, by the slide
// it is beside: two to a sheet, in the order typst.lua wrote them.
func notesEdges(work, pdf, body string) error {
	var names []string
	for _, m := range noteNameRe.FindAllStringSubmatch(body, -1) {
		names = append(names, strings.ReplaceAll(m[1], `\"`, `"`))
	}
	out, err := run.Stdout(work, "pdftotext", "-enc", "UTF-8", "-bbox", pdf, "-")
	if err != nil {
		return err
	}
	var cut []string
	for i, page := range strings.Split(string(out), "<page ")[1:] {
		m := regexp.MustCompile(`^width="([0-9.]+)" height="([0-9.]+)"`).FindStringSubmatch(page)
		if m == nil {
			continue
		}
		width, _ := strconv.ParseFloat(m[1], 64)
		height, _ := strconv.ParseFloat(m[2], 64)
		for _, w := range boxRe.FindAllStringSubmatch(page, -1) {
			y, _ := strconv.ParseFloat(w[2], 64)
			x1, _ := strconv.ParseFloat(w[3], 64)
			if x1 < width-edgeSlackPt {
				continue
			}
			k := 2 * i
			if y > height/2 {
				k++
			}
			if k < len(names) && !slices.Contains(cut, names[k]) {
				cut = append(cut, names[k])
			}
		}
	}
	if len(cut) == 0 {
		return nil
	}
	return fmt.Errorf(`%d speaker note(s) hold a word cut at the edge of the sheet, the rest of it
not printed at all:
    %q
  Write a long name as a shorter one, or break it with spaces`, len(cut), strings.Join(cut, `", "`))
}
