package words

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/carlosprados/mdbrand/internal/mdtext"
)

// placeholderRe is {{name}}, written without spaces so that pandoc keeps it in
// a single Str and the count can check the substitution against the AST. A dot,
// as in Go's {{ .Title }}, never matches: that is template code in a code
// block, not a placeholder.
var placeholderRe = regexp.MustCompile(`\{\{([a-z][a-z0-9_]*)\}\}`)

// Fill replaces every {{name}} in s with vals[name]. In Markdown (markdown true)
// it leaves code and mathematics alone — a document about mdbrand must be able
// to show {{words}} in a code block — and an unknown name in prose stops the
// build naming the ones that exist. It returns how many it replaced.
func Fill(s string, vals map[string]string, markdown bool) (string, int, error) {
	var protected [][2]int
	if markdown {
		protected = mdtext.Protected(s)
	}
	var unknown []string
	n := 0
	var b strings.Builder
	last := 0
	for _, m := range placeholderRe.FindAllStringSubmatchIndex(s, -1) {
		if mdtext.Inside(protected, m[0]) {
			continue
		}
		name := s[m[2]:m[3]]
		v, ok := vals[name]
		if !ok {
			unknown = append(unknown, "{{"+name+"}}")
			continue
		}
		b.WriteString(s[last:m[0]])
		b.WriteString(v)
		last = m[1]
		n++
	}
	if len(unknown) > 0 {
		names := make([]string, 0, len(vals))
		for k := range vals {
			names = append(names, "{{"+k+"}}")
		}
		sort.Strings(names)
		return "", 0, fmt.Errorf("unknown placeholder %s: the ones that exist are %s\n"+
			"  In a code span or block, placeholders are left as written.",
			strings.Join(unknown, ", "), strings.Join(names, ", "))
	}
	b.WriteString(s[last:])
	return b.String(), n, nil
}
