package words

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// figPlaceholder is how doc.ExtractFigs leaves a diagram in the body. The count
// runs after extraction, because pandoc does not read ```d2 caption="…" as a
// code block at all — it becomes inline code, and its source would be counted
// as prose.
var figPlaceholder = regexp.MustCompile(`@@MDBRAND_FIG_\d+@@`)

// figureFences are code block classes that are diagrams, not code, should a
// fence reach pandoc unextracted.
var figureFences = map[string]bool{"d2": true, "vega": true, "vegalite": true, "vl": true}

type node = map[string]any

// Count walks a pandoc JSON document and counts its words by r. placeholders is
// how many {{name}} tokens pandoc saw anywhere in prose — counted or not — for
// the caller to check against the ones it substitutes.
func Count(ast []byte, r Rules) (words, placeholders int, err error) {
	var doc struct {
		Blocks []any `json:"blocks"`
	}
	if err := json.Unmarshal(ast, &doc); err != nil {
		return 0, 0, fmt.Errorf("word count: reading pandoc's AST: %w", err)
	}
	c := &counter{r: r}
	c.list(doc.Blocks)
	return c.n, countPlaceholders(doc.Blocks), nil
}

// countPlaceholders visits every Str, whatever the criterion skips: a {{words}}
// inside an uncounted table is still substituted.
func countPlaceholders(x any) int {
	n := 0
	switch v := x.(type) {
	case []any:
		for _, e := range v {
			n += countPlaceholders(e)
		}
	case node:
		if s, ok := v["c"].(string); ok && v["t"] == "Str" {
			return len(placeholderRe.FindAllString(s, -1))
		}
		n += countPlaceholders(v["c"])
	}
	return n
}

type counter struct {
	r        Rules
	n        int
	inFigure bool
}

// list walks a sequence of elements. It is where a {.nocount} heading takes
// effect: everything after it is skipped until a heading of the same level or
// higher closes its section.
func (c *counter) list(xs []any) {
	skipBelow := 0
	for _, x := range xs {
		el, ok := x.(node)
		if ok && el["t"] == "Header" {
			level, attr := headerOf(el)
			if skipBelow > 0 && level <= skipBelow {
				skipBelow = 0
			}
			if skipBelow == 0 && hasClass(attr, "nocount") {
				skipBelow = level
			}
		}
		if skipBelow > 0 {
			continue
		}
		c.any(x)
	}
}

func (c *counter) any(x any) {
	switch v := x.(type) {
	case []any:
		c.list(v)
	case node:
		c.element(v)
	}
}

func (c *counter) element(el node) {
	t, _ := el["t"].(string)
	args, _ := el["c"].([]any)
	switch t {
	case "Str":
		s, _ := el["c"].(string)
		c.str(s)
	case "Code":
		// Inline code sits inside a sentence and is read as part of it.
		if len(args) == 2 {
			c.text(fmt.Sprint(args[1]))
		}
	case "CodeBlock":
		if len(args) == 2 && c.r.Has("code") && !isFigureFence(args[0]) {
			c.text(fmt.Sprint(args[1]))
		}
	case "Math":
		if c.r.Has("math") {
			c.n++
		}
	case "RawBlock", "RawInline":
	case "Table":
		if c.r.Has("tables") {
			c.any(args)
		}
	case "Figure":
		// [attr, caption, content]. Inside a figure an image's alt text is the
		// caption again, so it is left to the caption.
		if len(args) == 3 {
			if c.r.Has("captions") {
				c.any(args[1])
			}
			was := c.inFigure
			c.inFigure = true
			c.any(args[2])
			c.inFigure = was
		}
	case "Image":
		if len(args) == 3 && c.r.Has("captions") && !c.inFigure {
			c.any(args[1])
		}
	case "Cite":
		if len(args) == 2 && c.r.Has("citations") {
			c.any(args[1])
		}
	case "Note":
		if c.r.Has("footnotes") {
			// Every note, as printed: its citations are part of its text, or
			// the note that only cites would add nothing.
			sub := &counter{r: c.r, inFigure: c.inFigure}
			sub.r.include = with(c.r.include, "citations")
			sub.any(args)
			c.n += sub.n
			return
		}
		// A note that only cites is a reference, not prose: IB leaves it out.
		sub := &counter{r: c.r}
		sub.r.include = without(c.r.include, "citations")
		sub.any(args)
		if sub.n > 0 {
			c.any(args)
		}
	case "Div", "Span":
		if len(args) == 2 {
			if hasClass(args[0], "nocount") {
				return
			}
			if id := attrID(args[0]); id == "refs" && !c.r.Has("references") {
				return
			}
			c.any(args[1])
		}
	default:
		// Every other element — paragraphs, lists, emphasis, links, headings,
		// quotes — holds its words somewhere under "c".
		c.any(el["c"])
	}
}

func (c *counter) str(s string) {
	s = placeholderRe.ReplaceAllString(s, "")
	s = figPlaceholder.ReplaceAllString(s, "")
	if isWord(s) {
		c.n++
	}
}

func (c *counter) text(s string) {
	for f := range strings.FieldsSeq(s) {
		if isWord(f) {
			c.n++
		}
	}
}

// CountText counts the words of a plain string, for the captions of extracted
// figures, which never reach pandoc as text.
func CountText(s string) int {
	c := &counter{}
	c.text(s)
	return c.n
}

// isWord: a token with a letter or a digit. A lone dash or a stray bracket is
// punctuation, not a word, which is also how word processors see it.
func isWord(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func headerOf(el node) (int, any) {
	args, _ := el["c"].([]any)
	if len(args) < 2 {
		return 0, nil
	}
	level, _ := args[0].(float64)
	return int(level), args[1]
}

// attr is pandoc's [id, [classes], [[key, value]]].
func hasClass(attr any, class string) bool {
	a, _ := attr.([]any)
	if len(a) < 2 {
		return false
	}
	classes, _ := a[1].([]any)
	for _, c := range classes {
		if c == class {
			return true
		}
	}
	return false
}

func attrID(attr any) string {
	a, _ := attr.([]any)
	if len(a) < 1 {
		return ""
	}
	id, _ := a[0].(string)
	return id
}

func isFigureFence(attr any) bool {
	for k := range figureFences {
		if hasClass(attr, k) {
			return true
		}
	}
	return false
}

func with(m map[string]bool, key string) map[string]bool {
	out := make(map[string]bool, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	out[key] = true
	return out
}

func without(m map[string]bool, key string) map[string]bool {
	out := make(map[string]bool, len(m))
	for k, v := range m {
		if k != key {
			out[k] = v
		}
	}
	return out
}
