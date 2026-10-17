package pptx

import (
	"encoding/binary"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Accent marks: spans.lua wraps a [words]{.accent} in these private-use
// characters for a .pptx, since pandoc's writer has no character styles,
// and Repair turns what they hold into runs in the accent colour.
const (
	AccentOpen  = ""
	AccentClose = ""
)

// NamedMark is the private-use mark that opens the i-th named colour's words,
// [words]{.ok}, in the bundle's sorted order; AccentClose closes them all.
func NamedMark(i int) rune { return 0xE002 + rune(i) }

// Repair fixes what pandoc's pptx writer gets wrong, and returns the
// warnings worth printing. It refuses a deck it would otherwise publish
// wrong: two figures drawn over each other, a face the bundle did not name,
// an accent it could not set.
func Repair(b []byte, l Look) ([]byte, []string, error) {
	p, err := readPkg(b)
	if err != nil {
		return nil, nil, err
	}
	// pandoc copies the master and layouts that draw the logo and the art,
	// and not the pictures they draw.
	for name, raw := range l.media() {
		p.setBytes(name, raw)
	}
	if len(l.media()) > 0 {
		addPNGType(p)
	}
	var warns, overlaps []string
	for _, name := range slideNames(p) {
		x := p.get(name)
		title := slideTitle(x)
		if n := strings.Count(x, "<p:pic>"); n > 1 {
			overlaps = append(overlaps, fmt.Sprintf("%q: %d figures", title, n))
		}
		x = sizePictures(p, name, x)
		var paused bool
		if x, paused = dropPauses(x); paused {
			warns = append(warns, fmt.Sprintf("slide %q: a pause (. . .) does not carry into a .pptx; the slide shows all at once", title))
		}
		x = ruleTables(x, l.Text)
		if x, err = accents(x, l.marks()); err != nil {
			return nil, nil, fmt.Errorf("slide %q: %w", title, err)
		}
		p.set(name, x)
	}
	if len(overlaps) > 0 {
		return nil, nil, fmt.Errorf(`slide(s) with more than one figure, which PowerPoint draws over each other:
  %s
pandoc places every figure of a slide in the same box. Give each figure a slide
of its own (another ## heading), or join them in one diagram`, strings.Join(overlaps, "\n  "))
	}
	for _, name := range p.names {
		if strings.HasSuffix(name, ".xml") {
			p.set(name, buFontRe.ReplaceAllString(p.get(name), `<a:buFont typeface="`+esc(l.Body)+`"`))
		}
	}
	if stray := strayFonts(p, l); len(stray) > 0 {
		return nil, nil, fmt.Errorf(`the .pptx asks for %s, which the bundle's fonts.office does not name.
PowerPoint would substitute it without a word. pandoc wrote it, and mdbrand
did not expect it: please report this, with the document, at
  https://github.com/carlosprados/mdbrand/issues`, strings.Join(stray, ", "))
	}
	out, err := p.bytes()
	return out, warns, err
}

var slideNameRe = regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)

// slideNames lists the slides in their order.
func slideNames(p *pkg) []string {
	var names []string
	for _, n := range p.names {
		if slideNameRe.MatchString(n) {
			names = append(names, n)
		}
	}
	num := func(n string) int {
		v, _ := strconv.Atoi(slideNameRe.FindStringSubmatch(n)[1])
		return v
	}
	sort.Slice(names, func(i, j int) bool { return num(names[i]) < num(names[j]) })
	return names
}

var (
	textRe  = regexp.MustCompile(`<a:t>([^<]*)</a:t>`)
	titleRe = regexp.MustCompile(`<p:ph type="(?:title|ctrTitle)"`)
)

// slideTitle is the text of a slide's title placeholder, for messages.
func slideTitle(x string) string {
	for _, sp := range shapeRe.FindAllString(x, -1) {
		if titleRe.MatchString(sp) {
			var b strings.Builder
			for _, m := range textRe.FindAllStringSubmatch(sp, -1) {
				b.WriteString(m[1])
			}
			return unescape(b.String())
		}
	}
	return "(untitled)"
}

func unescape(s string) string {
	return strings.NewReplacer("&lt;", "<", "&gt;", ">", "&quot;", `"`, "&amp;", "&").Replace(s)
}

var (
	picRe   = regexp.MustCompile(`(?s)<p:pic>.*?</p:pic>`)
	embedRe = regexp.MustCompile(`<a:blip r:embed="([^"]+)"`)
	xfrmRe  = regexp.MustCompile(`<a:off x="(\d+)" y="(\d+)"\s*/>\s*<a:ext cx="(\d+)" cy="(\d+)"\s*/>`)
	relRe   = regexp.MustCompile(`<Relationship [^>]*>`)
	attrRe  = regexp.MustCompile(`(\w+)="([^"]*)"`)
)

// sizePictures writes each figure at the size mdbrand settled for it.
// pandoc's writer ignores width= and fits a picture to its placeholder: a
// figure placed at 100 mm came out 166 mm wide, its labels scaled with it. A
// figure's PNG is rasterised at PPI for its placed width, so its pixels say
// that width; it is centred where pandoc put it, top aligned.
func sizePictures(p *pkg, slide, x string) string {
	rels := relTargets(p.get(path.Join(path.Dir(slide), "_rels", path.Base(slide)+".rels")))
	return picRe.ReplaceAllStringFunc(x, func(pic string) string {
		m := embedRe.FindStringSubmatch(pic)
		if m == nil {
			return pic
		}
		target, ok := rels[m[1]]
		if !ok {
			return pic
		}
		w, h, ok := pngSize(p.parts[path.Clean(path.Join(path.Dir(slide), target))])
		if !ok {
			return pic
		}
		wmm, hmm := float64(w)/PPI*25.4, float64(h)/PPI*25.4
		return xfrmRe.ReplaceAllStringFunc(pic, func(xf string) string {
			v := xfrmRe.FindStringSubmatch(xf)
			x0, _ := strconv.Atoi(v[1])
			y0, _ := strconv.Atoi(v[2])
			cx, _ := strconv.Atoi(v[3])
			left := x0 + cx/2 - emu(wmm)/2
			return fmt.Sprintf(`<a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/>`, max(left, 0), y0, emu(wmm), emu(hmm))
		})
	})
}

func relTargets(rels string) map[string]string {
	out := map[string]string{}
	for _, r := range relRe.FindAllString(rels, -1) {
		var id, target string
		for _, a := range attrRe.FindAllStringSubmatch(r, -1) {
			switch a[1] {
			case "Id":
				id = a[2]
			case "Target":
				target = a[2]
			}
		}
		out[id] = target
	}
	return out
}

// pngSize reads a PNG's width and height from its header.
func pngSize(b []byte) (w, h int, ok bool) {
	if len(b) < 24 || string(b[1:4]) != "PNG" || string(b[12:16]) != "IHDR" {
		return 0, 0, false
	}
	return int(binary.BigEndian.Uint32(b[16:20])), int(binary.BigEndian.Uint32(b[20:24])), true
}

var paraRe = regexp.MustCompile(`(?s)<a:p>.*?</a:p>`)

// dropPauses removes the paragraphs pandoc wrote for `. . .`: its pptx
// writer prints the pause as text.
func dropPauses(x string) (string, bool) {
	found := false
	x = paraRe.ReplaceAllStringFunc(x, func(par string) string {
		var b strings.Builder
		for _, m := range textRe.FindAllStringSubmatch(par, -1) {
			b.WriteString(m[1])
		}
		if strings.TrimSpace(b.String()) == ". . ." {
			found = true
			return ""
		}
		return par
	})
	return x, found
}

var (
	runRe     = regexp.MustCompile(`(?s)<a:r>(<a:rPr[^>]*/>|<a:rPr[^>]*>.*?</a:rPr>)?<a:t>([^<]*)</a:t></a:r>`)
	rPrOpenRe = regexp.MustCompile(`^<a:rPr([^>]*?)(/?)>`)
)

// marks maps each mark spans.lua opens coloured words with to the colour.
func (l Look) marks() map[rune]string {
	m := map[rune]string{[]rune(AccentOpen)[0]: l.Accent}
	for i, n := range l.NamedNames {
		m[NamedMark(i)] = l.Named[n]
	}
	return m
}

// accents sets in their colour the words spans.lua marked, splitting runs at
// the marks; a mark left in the text would print as a box.
func accents(x string, marks map[rune]string) (string, error) {
	isMark := func(c rune) bool { _, ok := marks[c]; return ok || string(c) == AccentClose }
	if !strings.ContainsFunc(x, isMark) {
		return x, nil
	}
	x = paraRe.ReplaceAllStringFunc(x, func(par string) string {
		on := ""
		return runRe.ReplaceAllStringFunc(par, func(run string) string {
			m := runRe.FindStringSubmatch(run)
			rpr, text := m[1], m[2]
			if !strings.ContainsFunc(text, isMark) && on == "" {
				return run
			}
			var b strings.Builder
			seg := ""
			flush := func() {
				if seg != "" {
					r := rpr
					if on != "" {
						r = coloured(rpr, on)
					}
					b.WriteString("<a:r>" + r + "<a:t>" + seg + "</a:t></a:r>")
				}
				seg = ""
			}
			for _, c := range text {
				if hex, ok := marks[c]; ok {
					flush()
					on = hex
				} else if string(c) == AccentClose {
					flush()
					on = ""
				} else {
					seg += string(c)
				}
			}
			flush()
			return b.String()
		})
	})
	if strings.ContainsFunc(x, isMark) {
		return "", fmt.Errorf("a colour mark mdbrand could not resolve is left in the text; please report it at https://github.com/carlosprados/mdbrand/issues")
	}
	return x, nil
}

// coloured gives a run's properties the accent colour, in the place the
// schema wants a fill: after a line, before everything else.
func coloured(rpr, hex string) string {
	fill := `<a:solidFill><a:srgbClr val="` + hex + `"/></a:solidFill>`
	if rpr == "" {
		return "<a:rPr>" + fill + "</a:rPr>"
	}
	m := rPrOpenRe.FindStringSubmatch(rpr)
	if m == nil {
		return rpr
	}
	if m[2] == "/" {
		return "<a:rPr" + m[1] + ">" + fill + "</a:rPr>"
	}
	inner := solidFillRe.ReplaceAllString(rpr[len(m[0]):len(rpr)-len("</a:rPr>")], "")
	if i := strings.Index(inner, "</a:ln>"); i >= 0 {
		inner = inner[:i+len("</a:ln>")] + fill + inner[i+len("</a:ln>"):]
	} else {
		inner = fill + inner
	}
	return "<a:rPr" + m[1] + ">" + inner + "</a:rPr>"
}

var (
	tblRe   = regexp.MustCompile(`(?s)<a:tbl>.*?</a:tbl>`)
	tblPrRe = regexp.MustCompile(`<a:tblPr[^>]*>\s*<a:tableStyleId>[^<]*</a:tableStyleId>\s*</a:tblPr>`)
	trRe    = regexp.MustCompile(`(?s)<a:tr\b.*?</a:tr>`)
	tcPrRe  = regexp.MustCompile(`<a:tcPr\s*/>`)
)

// ruleTables draws a table as the PDF and the .docx do, with rules above and
// below the header and at the foot, rather than PowerPoint's banded style in
// the primary.
func ruleTables(x, ink string) string {
	line := func(side string, pt float64) string {
		return fmt.Sprintf(`<a:%s w="%d"><a:solidFill><a:srgbClr val="%s"/></a:solidFill></a:%s>`, side, ptEMU(pt), ink, side)
	}
	return tblRe.ReplaceAllStringFunc(x, func(tbl string) string {
		tbl = tblPrRe.ReplaceAllLiteralString(tbl, `<a:tblPr firstRow="1"><a:tableStyleId>{2D5ABB26-0587-4C30-8999-92F81FD0307C}</a:tableStyleId></a:tblPr>`)
		rows := trRe.FindAllStringIndex(tbl, -1)
		var b strings.Builder
		at := 0
		for i, r := range rows {
			b.WriteString(tbl[at:r[0]])
			var sides string
			switch {
			case i == 0 && len(rows) == 1:
				sides = line("lnT", 1.2) + line("lnB", 1.2)
			case i == 0:
				sides = line("lnT", 1.2) + line("lnB", 0.6)
			case i == len(rows)-1:
				sides = line("lnB", 1.2)
			}
			row := tcPrRe.ReplaceAllLiteralString(tbl[r[0]:r[1]], "<a:tcPr>"+sides+"</a:tcPr>")
			// Body cells come without properties at all.
			row = strings.ReplaceAll(row, "</a:txBody></a:tc>", "</a:txBody><a:tcPr>"+sides+"</a:tcPr></a:tc>")
			b.WriteString(row)
			at = r[1]
		}
		b.WriteString(tbl[at:])
		return b.String()
	})
}

var typefaceRe = regexp.MustCompile(`<a:(?:latin|buFont|sym) typeface="([^"]*)"`)

// strayFonts lists the faces the package asks for that fonts.office does not
// name. Theme references (+mj-lt) resolve to the faces the theme was given;
// the theme's per-script fallbacks are not latin and are left alone.
func strayFonts(p *pkg, l Look) []string {
	var stray []string
	for _, name := range p.names {
		if !strings.HasSuffix(name, ".xml") {
			continue
		}
		for _, m := range typefaceRe.FindAllStringSubmatch(p.get(name), -1) {
			face := unescape(m[1])
			if face == "" || strings.HasPrefix(face, "+") || face == l.Body || face == l.Display || face == l.Mono || slices.Contains(stray, face) {
				continue
			}
			stray = append(stray, face)
		}
	}
	return stray
}
