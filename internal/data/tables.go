package data

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/carlosprados/mdbrand/internal/mdtext"
)

// Tables replaces every ```table block, and every link to a data file standing
// on its own line, with the Markdown table it describes. A table fence inside
// another code block is an example of one and is left alone — which is why the
// fences are followed line by line rather than matched by a pattern.
func (s *Store) Tables(md, lang string) (string, error) {
	lines := strings.Split(md, "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimLeft(lines[i], " ")
		fence := ""
		if len(lines[i])-len(trimmed) <= 3 {
			fence = mdtext.Fence(trimmed)
		}
		if fence == "" {
			out = append(out, lines[i])
			continue
		}
		j := i + 1
		for ; j < len(lines); j++ {
			t := strings.TrimSpace(lines[j])
			if strings.HasPrefix(t, fence) && strings.Trim(t, fence[:1]) == "" {
				break
			}
		}
		info := strings.Fields(trimmed[len(fence):])
		if len(info) == 0 || info[0] != "table" {
			end := min(j, len(lines)-1)
			out = append(out, lines[i:end+1]...)
			i = end
			continue
		}
		if j == len(lines) {
			return "", fmt.Errorf("a ```table block is never closed")
		}
		t, err := s.Table(strings.Join(lines[i+1:j], "\n"), lang)
		if err != nil {
			return "", fmt.Errorf("table: %w", err)
		}
		// Blank lines around it: a pipe table glued to a paragraph is read as
		// part of the paragraph.
		out = append(out, "", strings.TrimRight(t, "\n"), "")
		i = j
	}
	return s.linkedTables(strings.Join(out, "\n"), lang)
}

// tableLinkRe is a link to a data file, the short form of a table with every
// column in the data's own order. Vega-Lite specs (.vl.json, .vega.json) were
// taken by figure extraction before this runs.
var tableLinkRe = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+\.(?:csv|tsv|ya?ml|json))\)`)

func (s *Store) linkedTables(md, lang string) (string, error) {
	protected := mdtext.Protected(md)
	var errs []string
	var b strings.Builder
	last := 0
	for _, m := range tableLinkRe.FindAllStringSubmatchIndex(md, -1) {
		if mdtext.Inside(protected, m[0]) {
			continue
		}
		caption, ref := md[m[2]:m[3]], md[m[4]:m[5]]
		lineStart := strings.LastIndexByte(md[:m[0]], '\n') + 1
		lineEnd := strings.IndexByte(md[m[1]:], '\n')
		if lineEnd < 0 {
			lineEnd = len(md) - m[1]
		}
		if strings.TrimSpace(md[lineStart:m[0]]) != "" || strings.TrimSpace(md[m[1]:m[1]+lineEnd]) != "" {
			errs = append(errs, fmt.Sprintf("![%s](%s): a table link stands on its own line, since a table cannot sit inside a sentence", caption, ref))
			continue
		}
		t, err := s.fileTable(ref, caption, lang)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		b.WriteString(md[last:m[0]])
		b.WriteString("\n" + strings.TrimRight(t, "\n") + "\n")
		last = m[1]
	}
	if len(errs) > 0 {
		return "", fmt.Errorf("%s", strings.Join(errs, "\n"))
	}
	b.WriteString(md[last:])
	return b.String(), nil
}

func (s *Store) fileTable(ref, caption, lang string) (string, error) {
	p := expandPath(ref)
	if !filepath.IsAbs(p) {
		p = filepath.Join(s.docDir, p)
	}
	s.refs[p] = true
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("table %s: not found (paths resolve against the document)", ref)
	}
	n, err := s.read(p)
	if err != nil {
		return "", err
	}
	return render(deref(n), ref, &tableSpec{Caption: caption}, lang)
}
