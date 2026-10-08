// Package docx writes the Word side of a document: a reference.docx that
// carries the brand, the cover or letterhead as OOXML, and the repairs pandoc's
// output needs before Word, LibreOffice or Google Docs lay it out the way the
// PDF is laid out.
//
// Everything here is a pure function of bytes and numbers. Running pandoc,
// rendering figures and deciding what the brand means belong to the caller, so
// this package knows nothing of LaTeX and LaTeX nothing of it.
//
// One rule shapes all of it: only what Google Docs keeps on import is used.
// Text boxes, anchored frames and table styles do not survive that importer,
// so the cover is made of plain paragraphs, borders are set on cells, and
// every size is direct formatting.
package docx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"
)

// Look is the brand as a .docx needs it: plain values, already resolved.
type Look struct {
	Body, Display, Mono string // font families the reader's machine is asked for
	Primary, Text, Rule string // hex colours, no '#'
	Link                string
	Accent              string // [words]{.accent}; the primary when the bundle sets none

	PaperWMM, PaperHMM float64
	MarginMM           float64
	LineStretch        float64

	// Logo is a PNG; LogoAspect its height over its width.
	Logo            []byte
	LogoAspect      float64
	LogoHeaderWMM   float64
	RunningTitle    string
	Confidential    string // repeated left of the page number in every footer
	Layout          string // report | note | letter
	NumberFromCover bool   // report: the cover is page 0, so the first text page is 1
}

// accent is the colour of accented words: Accent, or the primary for a Look
// built without one.
func (l Look) accent() string {
	if l.Accent != "" {
		return l.Accent
	}
	return l.Primary
}

// TextWidthMM is the measure between the margins.
func (l Look) TextWidthMM() float64 { return l.PaperWMM - 2*l.MarginMM }

// ConfidentialRoom is roughly how many characters of the confidentiality label
// fit left of the centred page number. A .docx is set in the reader's fonts,
// so nothing here can measure it: 0.6em a character is a generous average for
// a sans, and the label clears half a three-digit number and 4mm, as the
// PDF's does.
func (l Look) ConfidentialRoom() int {
	const mmPerPt = 25.4 / 72
	digit := 0.55 * TinyPt * mmPerPt
	room := l.TextWidthMM()/2 - 1.5*digit - 4
	return int(room / (0.6 * TinyPt * mmPerPt))
}

// Sizes, in points. The body matches the PDF's article class at 10pt; the
// rest follow its \small and \footnotesize steps.
const (
	BodyPt    = 10
	SmallPt   = 9
	TinyPt    = 8
	CaptionPt = 9
)

// pkg is a .docx held as its parts, in the order they were read: Word does
// not care, but a stable order keeps two builds of one document identical.
type pkg struct {
	names []string
	parts map[string][]byte
}

func readPkg(b []byte) (*pkg, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, fmt.Errorf("docx: %w", err)
	}
	p := &pkg{parts: map[string][]byte{}}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(rc)
		if cerr := rc.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return nil, err
		}
		p.names = append(p.names, f.Name)
		p.parts[f.Name] = raw
	}
	return p, nil
}

func (p *pkg) get(name string) string { return string(p.parts[name]) }

func (p *pkg) set(name, content string) {
	if _, ok := p.parts[name]; !ok {
		p.names = append(p.names, name)
	}
	p.parts[name] = []byte(content)
}

func (p *pkg) bytes() ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range p.names {
		// [Content_Types].xml first is what the format asks for; the rest keep
		// their order. A fixed modification time keeps output reproducible.
		w, err := zw.CreateHeader(&zip.FileHeader{Name: n, Method: zip.Deflate})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(p.parts[n]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// esc makes text safe inside an XML element or attribute.
func esc(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Unit conversions. OOXML measures pages in twentieths of a point (twips),
// fonts in half-points and pictures in English Metric Units.
func twips(mm float64) int  { return int(mm/25.4*1440 + 0.5) }
func emu(mm float64) int    { return int(mm*36000 + 0.5) }
func halfPt(pt float64) int { return int(pt*2 + 0.5) }
