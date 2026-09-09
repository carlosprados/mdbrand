// Package imgsize measures the intrinsic size of the picture formats a brand
// bundle may hand to LaTeX: SVG, PNG and PDF.
//
// It exists because a logo's proportions decide page geometry. `logo_width` is
// a width, but what a running header has to reserve is a HEIGHT, and the two
// are only the same number for a square mark. Guessing cost a defect: a square
// crest at 13mm is 37pt tall, it overflowed a 22pt headheight, ate the headsep
// whole and printed on top of the first line of every page — silently, because
// LaTeX writes "\headheight is too small" into a log nobody reads.
package imgsize

import (
	"fmt"
	"image"
	_ "image/png" // registers the PNG decoder for image.DecodeConfig
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	rootRe    = regexp.MustCompile(`(?s)<svg\b[^>]*>`)
	viewBoxRe = regexp.MustCompile(`viewBox\s*=\s*"\s*([-0-9.eE]+)[ ,]+([-0-9.eE]+)[ ,]+([-0-9.eE]+)[ ,]+([-0-9.eE]+)`)
	attrWRe   = regexp.MustCompile(`\bwidth\s*=\s*"\s*([0-9.]+)`)
	attrHRe   = regexp.MustCompile(`\bheight\s*=\s*"\s*([0-9.]+)`)
	// A PDF may carry several MediaBox entries; the first belongs to the page
	// that a one-page logo is.
	mediaBoxRe = regexp.MustCompile(`/MediaBox\s*\[\s*([-0-9.]+)\s+([-0-9.]+)\s+([-0-9.]+)\s+([-0-9.]+)\s*\]`)
)

// SVG returns the root element's size in pixels and the factor that turns inner
// user units into those pixels. d2 without --scale emits a root <svg> carrying
// a viewBox and no width/height at all, so the viewBox is the fallback, not an
// afterthought.
func SVG(src string) (w, h, unit float64, err error) {
	root := rootRe.FindString(src)
	if root == "" {
		return 0, 0, 0, fmt.Errorf("no <svg> element")
	}
	vb := viewBoxRe.FindStringSubmatch(root)
	if m := attrWRe.FindStringSubmatch(root); m != nil {
		w, _ = strconv.ParseFloat(m[1], 64)
	}
	if m := attrHRe.FindStringSubmatch(root); m != nil {
		h, _ = strconv.ParseFloat(m[1], 64)
	}
	if w == 0 || h == 0 {
		if vb == nil {
			return 0, 0, 0, fmt.Errorf("root <svg> has neither width/height nor viewBox")
		}
		w, _ = strconv.ParseFloat(vb[3], 64)
		h, _ = strconv.ParseFloat(vb[4], 64)
	}
	unit = 1
	if vb != nil {
		if vbw, _ := strconv.ParseFloat(vb[3], 64); vbw > 0 {
			unit = w / vbw
		}
	}
	if w <= 0 || h <= 0 {
		return 0, 0, 0, fmt.Errorf("root <svg> has a non-positive size")
	}
	return w, h, unit, nil
}

// Aspect returns height divided by width for the image at path: 1 for a square
// mark, 0.25 for the wide 4:1 logo most companies have.
//
// The unit does not matter and is deliberately not returned. A ratio is all
// page geometry needs, and it is the one thing that survives every conversion
// mdbrand does to a logo on the way to the PDF.
func Aspect(path string) (float64, error) {
	w, h, err := Of(path)
	if err != nil {
		return 0, err
	}
	return h / w, nil
}

// Of returns the intrinsic width and height of an SVG, PNG or PDF, each in its
// own natural unit: pixels for a bitmap, user units for an SVG, points for a
// PDF page.
func Of(path string) (w, h float64, err error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".svg":
		raw, err := os.ReadFile(path)
		if err != nil {
			return 0, 0, err
		}
		w, h, _, err := SVG(string(raw))
		return w, h, err
	case ".png":
		f, err := os.Open(path)
		if err != nil {
			return 0, 0, err
		}
		defer f.Close()
		cfg, _, err := image.DecodeConfig(f)
		if err != nil {
			return 0, 0, fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		if cfg.Width <= 0 || cfg.Height <= 0 {
			return 0, 0, fmt.Errorf("%s: non-positive size", filepath.Base(path))
		}
		return float64(cfg.Width), float64(cfg.Height), nil
	case ".pdf":
		raw, err := os.ReadFile(path)
		if err != nil {
			return 0, 0, err
		}
		m := mediaBoxRe.FindStringSubmatch(string(raw))
		if m == nil {
			// Object streams can compress the page tree out of reach of a plain
			// scan. Not a defect in the PDF, just a size mdbrand cannot read.
			return 0, 0, fmt.Errorf("%s: no readable /MediaBox", filepath.Base(path))
		}
		x0, _ := strconv.ParseFloat(m[1], 64)
		y0, _ := strconv.ParseFloat(m[2], 64)
		x1, _ := strconv.ParseFloat(m[3], 64)
		y1, _ := strconv.ParseFloat(m[4], 64)
		w, h = x1-x0, y1-y0
		if w <= 0 || h <= 0 {
			return 0, 0, fmt.Errorf("%s: /MediaBox has a non-positive size", filepath.Base(path))
		}
		return w, h, nil
	}
	return 0, 0, fmt.Errorf("%s: cannot measure this format", filepath.Base(path))
}
