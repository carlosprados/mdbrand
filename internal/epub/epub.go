// Package epub is what an .epub needs beyond pandoc's epub3 writer: the
// metadata pandoc gets wrong, a stylesheet in the brand's colours, and the
// checks for a 6" grey screen. It takes plain values from build and imports
// no other package of mdbrand.
//
// An e-reader reflows the text in the reader's own face and size, so nothing
// the A4 page measured has a counterpart. What the document itself sets —
// figures, code, tables, colour — is checked again for the screen assumed
// here: a 6" e-ink reader (Tagus, Nubico, Kobo Clara), 300 ppi, sixteen greys,
// a text column of about 80 mm.
package epub

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"fmt"
	"html"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// BoxWidthMM and BoxHeightMM are the room a figure has on the 6" screen:
	// the column, and the page's 112 mm of text less a caption.
	BoxWidthMM  = 80.0
	BoxHeightMM = 100.0
	// PPI is the screen's resolution, which figures are rasterised at for
	// the size they are placed.
	PPI = 300
	// MinDeltaL is how far apart two series colours must be in CIELAB
	// lightness to read as two greys: sixteen levels are 6.25 L* apart, and
	// two bars want two clear steps between them.
	MinDeltaL = 15.0
	// CodeColumns is the widest code line the column holds at a small
	// reader size; past it the line wraps.
	CodeColumns = 45
)

// Meta is what the package's metadata is written from.
type Meta struct {
	Input string // absolute path of the source, for a stable identifier
	Date  string // the front matter's date:, as written
	Now   time.Time
}

var isoDateRe = regexp.MustCompile(`^\d{4}(-\d{2}(-\d{2})?)?$`)

// Metadata is pandoc's --epub-metadata file. pandoc writes an empty
// <dc:date> for any date it cannot read as English or ISO — "Septiembre de
// 2026" — and epubcheck refuses the package; and it writes a random
// identifier, so every rebuild is a new book to the reader and its bookmarks
// and position are lost. The date is the front matter's when it is ISO, the
// day of the build otherwise; the identifier is a UUID from the source's path.
func Metadata(m Meta) string {
	date := strings.TrimSpace(m.Date)
	if !isoDateRe.MatchString(date) {
		date = m.Now.Format("2006-01-02")
	}
	return fmt.Sprintf("<dc:identifier id=\"BookId\">urn:uuid:%s</dc:identifier>\n<dc:date>%s</dc:date>\n",
		uuidOf(m.Input), html.EscapeString(date))
}

// uuidOf is a name-based UUID (version 5 layout) of s.
func uuidOf(s string) string {
	h := sha1.Sum([]byte("mdbrand:" + s))
	h[6] = h[6]&0x0f | 0x50
	h[8] = h[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

// Look is the brand in the terms the stylesheet needs, colours as hex
// without '#'.
type Look struct {
	Accent, Link, Rule string
}

// CSS is the stylesheet laid over pandoc's own, which keeps the headings',
// lists' and quotes' spacing. It names no face — pandoc's Georgia becomes
// the reader's serif — and no size: the reader's are the right ones on a
// screen tuned for them, and a licensed face inside a file copied to a
// device or a cloud library is a distribution. pre-wrap is one opinion: a
// code line wider than the screen wraps rather than running off its edge.
// The cells' is the other: Play Books clips a table wider than the screen,
// and a four-column table on a phone lost its last column whole, so a cell
// may break a word, hyphenated where the reader can.
func CSS(l Look) string {
	return fmt.Sprintf(`html { font-family: serif; }
.accent { color: #%s; }
a { color: #%s; }
pre { white-space: pre-wrap; font-size: 0.85em; }
table { border-collapse: collapse; margin: 1em auto; }
table { max-width: 100%%; }
th, td { border-bottom: 1px solid #%s; padding: 0.2em 0.4em; vertical-align: top;
  overflow-wrap: anywhere; -webkit-hyphens: auto; hyphens: auto; }
td.whole, th.whole { overflow-wrap: normal; white-space: nowrap; }
th { text-align: left; }
figure { margin: 1em 0; text-align: center; page-break-inside: avoid; }
figcaption { font-size: 0.9em; }
`, l.Accent, l.Link, l.Rule)
}

// Lightness is a colour's CIELAB L*, 0 for black to 100 for white: what is
// left of it on a grey screen.
func Lightness(hex string) float64 {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	var lin [3]float64
	for i := range lin {
		v, _ := strconv.ParseUint(hex[2*i:2*i+2], 16, 8)
		c := float64(v) / 255
		if c <= 0.04045 {
			lin[i] = c / 12.92
		} else {
			lin[i] = math.Pow((c+0.055)/1.055, 2.4)
		}
	}
	y := 0.2126*lin[0] + 0.7152*lin[1] + 0.0722*lin[2]
	if y > 216.0/24389 {
		return 116*math.Cbrt(y) - 16
	}
	return y * 24389 / 27
}

// Clash is two colours a grey screen draws as one.
type Clash struct {
	A, B   string
	LA, LB float64
}

// GreyClash finds the closest pair of colours under MinDeltaL, or reports
// none. colors are the distinct fills and strokes of a chart's data marks.
func GreyClash(colors []string) (Clash, bool) {
	best, found := Clash{}, false
	for i := range colors {
		for j := i + 1; j < len(colors); j++ {
			la, lb := Lightness(colors[i]), Lightness(colors[j])
			if math.Abs(la-lb) >= MinDeltaL {
				continue
			}
			if !found || math.Abs(la-lb) < math.Abs(best.LA-best.LB) {
				best, found = Clash{colors[i], colors[j], la, lb}, true
			}
		}
	}
	return best, found
}

// Problem is one finding of epubcheck.
type Problem struct {
	Severity string // FATAL, ERROR, WARNING
	Code     string // RSC-005
	Text     string
}

var checkLineRe = regexp.MustCompile(`(?m)^(FATAL|ERROR|WARNING)\(([A-Z]+-\d+[a-z]?)\): (.*)$`)

// Problems reads epubcheck's report. Its line is the severity, the code, the
// file and position inside the package, and the message.
func Problems(out string) []Problem {
	var ps []Problem
	for _, m := range checkLineRe.FindAllStringSubmatch(out, -1) {
		ps = append(ps, Problem{Severity: m[1], Code: m[2], Text: strings.TrimSpace(m[3])})
	}
	return ps
}

// TableWidthMM estimates how wide a table must be so that no word in it
// breaks: each column as wide as its longest word, in an assumed 11 pt face
// whose average letter is half an em, plus the cells' padding.
func TableWidthMM(longest []int) float64 {
	const charMM, padMM = 11 * 0.5 * 25.4 / 72, 0.8 * 11 * 25.4 / 72
	w := 0.0
	for _, n := range longest {
		w += float64(n)*charMM + padMM
	}
	return w
}

// HasMath reports whether any page of the package holds a MathML formula.
func HasMath(pkg []byte) (bool, error) {
	zr, err := zip.NewReader(bytes.NewReader(pkg), int64(len(pkg)))
	if err != nil {
		return false, err
	}
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".xhtml") {
			continue
		}
		page, err := readZip(f)
		if err != nil {
			return false, err
		}
		if bytes.Contains(page, []byte("<math")) {
			return true, nil
		}
	}
	return false, nil
}

var (
	manifestItemRe = regexp.MustCompile(`<item [^>]*href="([^"]+\.xhtml)"[^>]*/>`)
	propertiesRe   = regexp.MustCompile(`properties="([^"]*)"`)
)

// Repair declares the mathml property of every page that holds a formula.
// pandoc declares it for chapters but not for nav.xhtml, where a heading's
// formula lands too — "$\varepsilon$-greedy" in a title — and epubcheck
// refuses the package (OPF-014).
func Repair(pkg []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(pkg), int64(len(pkg)))
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	opfName := ""
	for _, f := range zr.File {
		b, err := readZip(f)
		if err != nil {
			return nil, err
		}
		files[f.Name] = b
		if strings.HasSuffix(f.Name, ".opf") {
			opfName = f.Name
		}
	}
	if opfName == "" {
		return nil, fmt.Errorf("the .epub has no package document")
	}
	dir := opfName[:strings.LastIndex(opfName, "/")+1]
	opf := string(files[opfName])
	fixed := manifestItemRe.ReplaceAllStringFunc(opf, func(item string) string {
		href := manifestItemRe.FindStringSubmatch(item)[1]
		if !bytes.Contains(files[dir+href], []byte("<math")) {
			return item
		}
		m := propertiesRe.FindStringSubmatch(item)
		switch {
		case m == nil:
			return strings.Replace(item, " />", ` properties="mathml" />`, 1)
		case strings.Contains(" "+m[1]+" ", " mathml "):
			return item
		}
		return strings.Replace(item, m[0], `properties="`+m[1]+` mathml"`, 1)
	})
	if fixed == opf {
		return pkg, nil
	}
	files[opfName] = []byte(fixed)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		// The mimetype comes first, stored and with no extra field — a
		// modification time is one — or the file is not an EPUB; zr.File
		// keeps pandoc's order.
		h := f.FileHeader
		if f.Name == "mimetype" {
			h = zip.FileHeader{Name: "mimetype", Method: zip.Store}
		}
		w, err := zw.CreateHeader(&h)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(files[f.Name]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func readZip(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(rc)
	if cerr := rc.Close(); err == nil {
		err = cerr
	}
	return b, err
}
