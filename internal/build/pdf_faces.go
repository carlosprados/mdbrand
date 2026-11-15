package build

import (
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/carlosprados/mdbrand/internal/run"
	"github.com/carlosprados/mdbrand/internal/tex"
)

// boldFaceOptions names the body family's bold and bold italic for
// fontspec, as fontconfig gives them at weight 700. Left to itself fontspec
// took Inter ExtraBold for \bfseries: every bold word at 10pt a dark blot
// that competed with the headings, where Typst set Inter Bold. A face whose
// family has no true bold (fontconfig answers with another family or weight)
// keeps fontspec's own choice.
func boldFaceOptions(family string) string {
	var opts []string
	for _, q := range []struct{ pattern, key string }{{":bold", "BoldFont"}, {":bold:italic", "BoldItalicFont"}} {
		out, err := run.Stdout("", "fc-match", "-f", "%{family[0]}|%{weight}|%{fullname[0]}", family+q.pattern)
		if err != nil {
			continue
		}
		f := strings.Split(string(out), "|")
		if len(f) != 3 || !strings.EqualFold(f[0], family) || f[1] != "200" || f[2] == "" {
			continue
		}
		opts = append(opts, q.key+"={"+f[2]+"}")
	}
	return strings.Join(opts, ",")
}

// monoFontArgs sets code in DejaVu Sans Mono at 0.9 of the text, the face
// and size Typst sets it in. Latin Modern Mono Light, pandoc's default, is a
// thin typewriter beside a sans body; DejaVu's x-height is Inter's, it reads
// at 8pt and covers more of what code holds. A document that names its own
// monofont keeps it, and a machine without DejaVu keeps pandoc's default.
func monoFontArgs(front string, args []string) []string {
	var m map[string]any
	if yaml.Unmarshal([]byte(front), &m) == nil && m["monofont"] != nil {
		return args
	}
	out, err := run.Stdout("", "fc-match", "-f", "%{family[0]}", typstMono)
	if err != nil || string(out) != typstMono {
		return args
	}
	return append(args, "-V", "monofont="+typstMono, "-V", "monofontoptions=Scale=0.9")
}

// pageBodyPt is the body size of a report, note or letter that does not set
// fontsize. At article's 10pt with A4 and a 22mm margin a line of Inter held
// 98 characters; 75 to 85 is where an eye finds the next line without
// searching. 11pt with the default bundle's 25mm margin gives 86, at the
// price of a document about a sixth longer.
const pageBodyPt = 11.0

// pageFontSizeArgs gives XeLaTeX pageBodyPt where the front matter sets no
// fontsize. A thesis has the UPM's size and a deck beamer's.
func pageFontSizeArgs(front, style string, args []string) []string {
	if style == tex.Thesis || style == tex.Slides {
		return args
	}
	var m map[string]any
	if yaml.Unmarshal([]byte(front), &m) == nil && m["fontsize"] != nil {
		return args
	}
	return append(args, "-V", "fontsize="+strconv.FormatFloat(pageBodyPt, 'f', -1, 64)+"pt")
}

// headingSizes are \Large and \large as article's size files have them: a
// table, not a proportion, so 11pt keeps 10pt's headings.
func headingSizes(body float64) (h1, h2 float64) {
	switch body {
	case 10, 11:
		return 14.4, 12
	case 12:
		return 17.28, 14.4
	}
	return 1.44 * body, 1.2 * body
}

// baselineSkip is article's \baselineskip for a body size: 12pt at 10, 13.6
// at 11 and 14.5 at 12, a table and not 1.2 times the size.
func baselineSkip(body float64) float64 {
	switch body {
	case 10:
		return 12
	case 11:
		return 13.6
	case 12:
		return 14.5
	}
	return 1.2 * body
}
