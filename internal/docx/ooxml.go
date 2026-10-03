package docx

import (
	"regexp"
	"slices"
	"strings"
)

// Word refuses a file whose property elements are out of the schema's order,
// with a message about unreadable content and an offer to repair it; LibreOffice
// and Google Docs read it without a word, so nothing but Word would ever tell.
// Every <w:pPr> and <w:rPr> mdbrand writes into goes through setChildren, which
// rebuilds it in this order.
var (
	pPrOrder = []string{
		"pStyle", "keepNext", "keepLines", "pageBreakBefore", "framePr", "widowControl", "numPr",
		"suppressLineNumbers", "pBdr", "shd", "tabs", "suppressAutoHyphens", "kinsoku", "wordWrap",
		"overflowPunct", "topLinePunct", "autoSpaceDE", "autoSpaceDN", "bidi", "adjustRightInd",
		"snapToGrid", "spacing", "ind", "contextualSpacing", "mirrorIndents", "suppressOverlap", "jc",
		"textDirection", "textAlignment", "textboxTightWrap", "outlineLvl", "divId", "cnfStyle", "rPr",
		"sectPr", "pPrChange",
	}
	rPrOrder = []string{
		"rStyle", "rFonts", "b", "bCs", "i", "iCs", "caps", "smallCaps", "strike", "dstrike", "outline",
		"shadow", "emboss", "imprint", "noProof", "snapToGrid", "vanish", "webHidden", "color", "spacing",
		"w", "kern", "position", "sz", "szCs", "highlight", "u", "effect", "bdr", "shd", "fitText",
		"vertAlign", "rtl", "cs", "em", "lang", "eastAsianLayout", "specVanish", "oMath",
	}
)

type elem struct{ tag, xml string }

var tagRe = regexp.MustCompile(`^<w:([A-Za-z]+)`)

// children splits the inside of a container into its top-level elements.
func children(inner string) []elem {
	var out []elem
	for i := 0; i < len(inner); {
		start := strings.Index(inner[i:], "<w:")
		if start < 0 {
			break
		}
		start += i
		m := tagRe.FindStringSubmatch(inner[start:])
		if m == nil {
			break
		}
		tag := m[1]
		gt := strings.IndexByte(inner[start:], '>')
		if gt < 0 {
			break
		}
		end := start + gt + 1
		if inner[end-2] != '/' {
			// Not self-closing. No property element nests one of its own
			// name (pBdr holds top and bottom, tabs holds tab), so the first
			// close is the matching one.
			c := strings.Index(inner[end:], "</w:"+tag+">")
			if c < 0 {
				return out
			}
			end += c + len("</w:"+tag+">")
		}
		out = append(out, elem{tag, inner[start:end]})
		i = end
	}
	return out
}

// setChildren replaces or adds elements in a container and writes it back in
// schema order, one of each: where an element repeats, the last wins, as it
// does for Word. Elements the order does not list keep their place at the end.
func setChildren(inner string, set []elem, order []string) string {
	var have []elem
	for _, e := range children(inner) {
		have = slices.DeleteFunc(have, func(h elem) bool { return h.tag == e.tag })
		have = append(have, e)
	}
	for _, s := range set {
		replaced := false
		for i := range have {
			if have[i].tag == s.tag {
				have[i] = s
				replaced = true
			}
		}
		if !replaced {
			have = append(have, s)
		}
	}
	// Drop the elements a caller cleared by setting them empty.
	have = slices.DeleteFunc(have, func(e elem) bool { return e.xml == "" })
	rank := func(tag string) int {
		if i := slices.Index(order, tag); i >= 0 {
			return i
		}
		return len(order)
	}
	slices.SortStableFunc(have, func(a, b elem) int { return rank(a.tag) - rank(b.tag) })
	var b strings.Builder
	for _, e := range have {
		b.WriteString(e.xml)
	}
	return b.String()
}

var (
	pPrBlockRe = regexp.MustCompile(`(?s)<w:pPr>(.*?)</w:pPr>`)
	rPrBlockRe = regexp.MustCompile(`(?s)<w:rPr>(.*?)</w:rPr>`)
)

// misordered names the first property block in a part that Word would refuse:
// an element out of the schema's order, or one given twice.
func misordered(xml string) string {
	check := func(re *regexp.Regexp, order []string) string {
		for _, m := range re.FindAllStringSubmatch(xml, -1) {
			inner := m[1]
			if re == pPrBlockRe {
				// The paragraph mark's own run properties are checked as rPr.
				inner = rPrBlockRe.ReplaceAllString(inner, "<w:rPr/>")
			}
			last, seen := -1, map[string]bool{}
			for _, e := range children(inner) {
				r := slices.Index(order, e.tag)
				if r < 0 {
					continue
				}
				if r < last || seen[e.tag] {
					return m[0]
				}
				last, seen[e.tag] = r, true
			}
		}
		return ""
	}
	if bad := check(pPrBlockRe, pPrOrder); bad != "" {
		return bad
	}
	return check(rPrBlockRe, rPrOrder)
}

// foreignRe finds an element outside the w: namespace — w14:, mc: and the
// like — which children does not read and a rebuild would drop.
var foreignRe = regexp.MustCompile(`</?(?:[A-Za-vx-z][A-Za-z0-9]*|w[0-9][A-Za-z0-9]*):`)

// normalize writes every <w:pPr> and <w:rPr> in a part in schema order, the
// last of a repeated element winning. pandoc does not always: 3.1 writes
// <w:bCs/> before <w:b/>. A block holding a foreign element is left as it is.
func normalize(xml string) string {
	fix := func(re *regexp.Regexp, tag string, order []string) {
		xml = re.ReplaceAllStringFunc(xml, func(block string) string {
			inner := block[len("<w:"+tag+">") : len(block)-len("</w:"+tag+">")]
			if foreignRe.MatchString(inner) {
				return block
			}
			return "<w:" + tag + ">" + setChildren(inner, nil, order) + "</w:" + tag + ">"
		})
	}
	fix(rPrBlockRe, "rPr", rPrOrder)
	fix(pPrBlockRe, "pPr", pPrOrder)
	return xml
}
