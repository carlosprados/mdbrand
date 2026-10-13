package build

import (
	"fmt"
	"image"
	_ "image/jpeg" // DecodeConfig reads the formats registered here
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/carlosprados/mdbrand/internal/run"
)

// A picture printed under minPPI is visibly blocky; under warnPPI, soft. Print
// wants 300; a screenshot placed at the measure is usually 100 to 150.
const (
	minPPI  = 72
	warnPPI = 150
)

// checkResolution measures each raster picture the document links at the
// resolution it was printed at. LaTeX stretches a small picture to the width
// it is given without a word: a 400-pixel screenshot across the measure
// printed at 60 ppi. pdfimages reads the placed size off the finished PDF; a
// picture is recognised by its size in pixels, since the PDF also holds the
// rasters rsvg-convert makes of a figure's shadows, which are not the
// author's.
func checkResolution(work, pdf string, refs []string, rep *Report) error {
	pics := map[[2]int]string{}
	for _, r := range refs {
		switch strings.ToLower(filepath.Ext(r)) {
		case ".png", ".jpg", ".jpeg":
		default:
			continue
		}
		f, err := os.Open(r)
		if err != nil {
			continue // ExtractFigs has already stopped a missing one
		}
		cfg, _, err := image.DecodeConfig(f)
		_ = f.Close() // read-only: nothing to lose
		if err == nil {
			pics[[2]int{cfg.Width, cfg.Height}] = filepath.Base(r)
		}
	}
	if len(pics) == 0 || !run.Have("pdfimages") {
		return nil
	}
	out, err := run.Stdout(work, "pdfimages", "-list", pdf)
	if err != nil {
		return err
	}
	low, soft := placedPPI(string(out), pics)
	for _, s := range soft {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s, under the %d ppi a printed page wants: it will look soft. A larger original, or a smaller width=, fixes it", s, warnPPI))
	}
	if len(low) == 0 {
		return nil
	}
	return fmt.Errorf(`picture(s) stretched past what their pixels can print:
  %s
Under %d ppi a picture prints blocky. Use a larger original (a screenshot at
2x, the source file rather than a copy), or place it smaller with width=`,
		strings.Join(low, "\n  "), minPPI)
}

// placedPPI reads `pdfimages -list` and reports each of pics printed under
// minPPI (low) or warnPPI (soft), once per picture and page, with the width it
// would print well at.
func placedPPI(list string, pics map[[2]int]string) (low, soft []string) {
	seen := map[string]bool{}
	for _, line := range strings.Split(list, "\n") {
		f := strings.Fields(line)
		if len(f) < 14 || f[2] != "image" {
			continue
		}
		w, _ := strconv.Atoi(f[3])
		h, _ := strconv.Atoi(f[4])
		name, ok := pics[[2]int{w, h}]
		if !ok {
			continue
		}
		x, errX := strconv.Atoi(f[12])
		y, errY := strconv.Atoi(f[13])
		if errX != nil || errY != nil {
			continue
		}
		ppi := min(x, y)
		key := name + " " + f[0]
		if seen[key] || ppi >= warnPPI {
			continue
		}
		seen[key] = true
		good := math.Floor(float64(w) / warnPPI * 25.4)
		s := fmt.Sprintf("%s prints at %d ppi on page %s (%d px wide; at %.0fmm it would print at %d)", name, ppi, f[0], w, good, warnPPI)
		if ppi < minPPI {
			low = append(low, s)
		} else {
			soft = append(soft, s)
		}
	}
	return low, soft
}
