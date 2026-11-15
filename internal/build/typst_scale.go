package build

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/carlosprados/mdbrand/internal/run"
)

// typstDisplayScale is what fontspec's Scale=MatchLowercase does to every
// face but the body's under pandoc's LaTeX template: each display face is
// scaled until its x-height is the body's. XeLaTeX printed Gotham Light 6.6%
// larger than asked, and Typst, which takes a size as given, set every
// running head, cover line and title smaller than the PDFs Charlie had
// approved. Matching x-heights is also what an editor would ask of two
// families on one page. Typst measures them, one probe for both faces.
func typstDisplayScale(f typstFaces) (regular, bold float64, err error) {
	if f.Display == f.Body && f.DisplayBold == f.Body {
		return 1, 1, nil
	}
	dir, err := os.MkdirTemp("", "mdbrand-xh-")
	if err != nil {
		return 0, 0, err
	}
	defer os.RemoveAll(dir)
	src := fmt.Sprintf(`#let xh(font, w) = measure(text(font: font, weight: w, size: 100pt, top-edge: "x-height", bottom-edge: "baseline")[x]).height / 1pt
#context [#metadata((xh(%q, 400), xh(%q, %d), xh(%q, %d)))<xh>]
`, f.Body, f.Display, f.Weight, f.DisplayBold, f.BoldWeight)
	if err := os.WriteFile(filepath.Join(dir, "xh.typ"), []byte(src), 0o644); err != nil {
		return 0, 0, err
	}
	out, err := run.Stdout(dir, "typst", "query", "--ignore-system-fonts", "--font-path", f.Dir, "xh.typ", "<xh>", "--field", "value", "--one")
	if err != nil {
		return 0, 0, err
	}
	var xh [3]float64
	if err := json.Unmarshal(out, &xh); err != nil || xh[1] <= 0 || xh[2] <= 0 {
		return 0, 0, fmt.Errorf("measuring the display faces' x-height: %q", out)
	}
	return xh[0] / xh[1], xh[0] / xh[2], nil
}

// scaleString writes a scale for the template, to four places.
func scaleString(s float64) string { return strconv.FormatFloat(s, 'f', 4, 64) }
