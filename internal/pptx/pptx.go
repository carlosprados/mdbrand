// Package pptx turns pandoc's PowerPoint output into the brand's deck: a
// reference.pptx generated from the bundle, and repairs over what pandoc
// writes. Like internal/docx it is a pure leaf: it knows OOXML, not how a
// build runs.
//
// A .pptx is laid out by the reader's PowerPoint in the reader's fonts, so
// text cannot be measured here. Figures can: their size is an extent this
// package writes. pandoc's writer fits every picture to its placeholder,
// prints a pause as ". . .", and drops the master's media, all at exit 0;
// Repair puts each right or refuses by slide.
package pptx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"
)

// Scale maps the PDF deck to the .pptx slide. beamer's frame is 160 × 90 mm
// and pandoc's slide 254 × 142.875 mm: 1.5875 on both axes, so a figure sized
// for the frame is the same figure on the same screen at this factor.
const Scale = 1.5875

// Slide size, in mm, as pandoc's reference declares it (16:9).
const (
	SlideWMM = 254.0
	SlideHMM = 142.875
)

// PPI is the resolution figures are rasterised at for their placed size.
const PPI = 300

// Look is the brand in the terms a deck needs. Lengths are on the slide,
// already scaled from the PDF's frame.
type Look struct {
	Body, Display, Mono                string
	Primary, Text, Rule, Link, Accent  string
	Named                              map[string]string // [words]{.ok}, by name
	NamedNames                         []string          // Named's names, in mark order
	Ground, OnGround                   string            // the covers' fill and type; Ground "" is white
	Logo, CoverLogo, Art               []byte            // PNG; nil when the bundle has none
	LogoAspect, CoverAspect, ArtAspect float64
	LogoWMM, CoverLogoWMM, MarginMM    float64
	Confidential, RunningTitle         string // the PDF footline's words
}

// NotesTitle names pandoc's closing slide of footnotes in the deck's language.
func NotesTitle(lang string) string {
	switch strings.ToLower(strings.SplitN(strings.SplitN(lang, "-", 2)[0], "_", 2)[0]) {
	case "es", "pt":
		return "Notas"
	case "it":
		return "Note"
	case "de":
		return "Anmerkungen"
	default: // en, ca, fr
		return "Notes"
	}
}

// pkg is a .pptx held as its parts, in the order they were read, so two
// builds of one deck are identical.
type pkg struct {
	names []string
	parts map[string][]byte
}

func readPkg(b []byte) (*pkg, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, fmt.Errorf("pptx: %w", err)
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

func (p *pkg) set(name, content string) { p.setBytes(name, []byte(content)) }

func (p *pkg) setBytes(name string, content []byte) {
	if _, ok := p.parts[name]; !ok {
		p.names = append(p.names, name)
	}
	p.parts[name] = content
}

func (p *pkg) bytes() ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range p.names {
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
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// emu is mm in English Metric Units, DrawingML's length.
func emu(mm float64) int { return int(mm*36000 + 0.5) }

// hundredths is a point size as DrawingML's sz writes it.
func hundredths(pt float64) int { return int(pt*100 + 0.5) }

// ptEMU is a line width in points, as English Metric Units.
func ptEMU(pt float64) int { return int(pt*12700 + 0.5) }
