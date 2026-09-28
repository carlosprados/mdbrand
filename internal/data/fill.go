package data

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/carlosprados/mdbrand/internal/mdtext"
	"gopkg.in/yaml.v3"
)

// candidateRe is anything placeholder-shaped with a dot or bracket in it: a
// data path, or a typo of one. {{words}} has neither and is left to the word
// count; Go's {{ .Title }} has a space and a leading dot and matches nothing.
var candidateRe = regexp.MustCompile(`\{\{([A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+|\[[^\]\n]*\])+)\}\}`)

// Fill replaces every {{data…}} in Markdown prose, leaving code and
// mathematics as written, and escapes each value so that it prints as the
// characters in the data: a price of "1.500 $" must not open a formula that
// runs to the next dollar sign. A value tagged !md goes in as Markdown.
func (s *Store) Fill(md string) (string, error) {
	return s.fill(md, mdtext.Protected(md), escapeMarkdown)
}

// FillRaw replaces placeholders in text that is not Markdown source, such as
// a scalar of the front matter: the value goes in as written.
func (s *Store) FillRaw(text string) (string, error) {
	return s.fill(text, nil, func(v Value) string { return v.Text })
}

func (s *Store) fill(text string, protected [][2]int, render func(Value) string) (string, error) {
	var b strings.Builder
	var errs []string
	last := 0
	for _, m := range candidateRe.FindAllStringSubmatchIndex(text, -1) {
		if mdtext.Inside(protected, m[0]) {
			continue
		}
		expr := text[m[2]:m[3]]
		if !strings.HasPrefix(expr, "data.") && !strings.HasPrefix(expr, "data[") {
			errs = append(errs, fmt.Sprintf("{{%s}}: the only namespace is data, as in {{data.file.key}}", expr))
			continue
		}
		v, err := s.Lookup(expr)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		b.WriteString(text[last:m[0]])
		b.WriteString(render(v))
		last = m[1]
	}
	if len(errs) > 0 {
		return "", fmt.Errorf("%s\n  In a code span or block, placeholders are left as written.", strings.Join(errs, "\n"))
	}
	b.WriteString(text[last:])
	return b.String(), nil
}

// FillFrontMatter replaces placeholders inside the front matter's scalars and
// writes the YAML back out, quoting what it changed. Substituting into the raw
// text, as {{words}} can because it only ever inserts a number, would let a
// value holding ": " or a quote break the YAML it lands in.
func (s *Store) FillFrontMatter(raw string) (string, error) {
	if !strings.Contains(raw, "{{") {
		return raw, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		return "", err
	}
	changed := false
	var walk func(*yaml.Node) error
	walk = func(n *yaml.Node) error {
		if n.Kind == yaml.ScalarNode {
			if !candidateRe.MatchString(n.Value) {
				return nil
			}
			v, err := s.FillRaw(n.Value)
			if err != nil {
				return err
			}
			n.Value, n.Style, n.Tag, changed = v, yaml.DoubleQuotedStyle, "!!str", true
			return nil
		}
		for _, c := range n.Content {
			if err := walk(c); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(&doc); err != nil {
		return "", err
	}
	if !changed {
		return raw, nil
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// escapeMarkdown makes a value print as its characters. It escapes what opens
// an inline construct anywhere (emphasis, code, links, math, citations, raw
// HTML and entities, sub/superscripts, table pipes, attributes) and, at the
// start, what opens a block — the value may be the first thing on its line.
// Quotes are left alone so that smart punctuation still curls them. Line
// breaks become spaces: a blank line inside a value would end the paragraph.
func escapeMarkdown(v Value) string {
	if v.Markdown {
		return v.Text
	}
	t := strings.Join(strings.Fields(v.Text), " ")
	var b strings.Builder
	for i, r := range t {
		switch r {
		case '\\', '`', '*', '_', '[', ']', '<', '>', '$', '~', '^', '@', '#', '|', '{', '}', '&':
			b.WriteByte('\\')
		case '-', '+', ':':
			if i == 0 {
				b.WriteByte('\\')
			}
		case '.', ')':
			if orderedMarker(t[:i]) && (i+1 == len(t) || t[i+1] == ' ') {
				b.WriteByte('\\')
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

// orderedMarker reports whether prefix is all digits, so that the next . or )
// would make "3. algo" at the start of a line an ordered list.
func orderedMarker(prefix string) bool {
	if prefix == "" || len(prefix) > 9 {
		return false
	}
	for _, c := range prefix {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
