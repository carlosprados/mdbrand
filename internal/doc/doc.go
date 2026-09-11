// Package doc reads a Markdown document's YAML front matter and rewrites its
// diagram sources into figure includes. Diagrams may be written two ways and
// both are supported: a fenced block (```d2 / ```vegalite) or a link to a side
// file (![Caption](diagrams/arch.d2)).
package doc

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Meta is the front matter mdbrand cares about. Everything else is left alone
// and handed to pandoc untouched.
type Meta struct {
	Title    string `yaml:"title"`
	Subtitle string `yaml:"subtitle"`
	Author   any    `yaml:"author"` // string or list, as pandoc allows
	Date     string `yaml:"date"`
	TOC      *bool  `yaml:"toc"`

	// Citations. These are pandoc's own metadata names and stay at the top
	// level rather than moving under `mdbrand:`, so a document already written
	// for pandoc builds unchanged. Declaring a bibliography is what turns
	// --citeproc on; there is no separate switch to forget.
	Bibliography StringList `yaml:"bibliography"`
	CSL          string     `yaml:"csl"`

	// Keys mdbrand has to refuse rather than drop. The preamble, the cover and
	// the closing matter reach pandoc as --include-in-header,
	// --include-before-body and --include-after-body; in pandoc each of those
	// flags *sets the variable* of the same name as one of these fields, and a
	// variable given on the command line replaces the metadata field it
	// matches. So the flags that inject the design are precisely what evict
	// whatever the document wrote here, and pandoc says nothing. Held as raw
	// nodes so that any shape parses and the build can stop with a message
	// instead of a YAML error.
	HeaderIncludes yaml.Node `yaml:"header-includes"`
	IncludeBefore  yaml.Node `yaml:"include-before"`
	IncludeAfter   yaml.Node `yaml:"include-after"`

	Options Options `yaml:"mdbrand"`
}

// ReservedKeys names the front matter keys this document sets that mdbrand
// would silently discard. A key present but empty asks for nothing, so it does
// not count.
func (m Meta) ReservedKeys() []string {
	var out []string
	for _, k := range []struct {
		name string
		node yaml.Node
	}{
		{"header-includes", m.HeaderIncludes},
		{"include-before", m.IncludeBefore},
		{"include-after", m.IncludeAfter},
	} {
		if k.node.Kind != 0 && k.node.Tag != "!!null" {
			out = append(out, k.name)
		}
	}
	return out
}

// StringList accepts either one value or a list, the way pandoc reads
// `bibliography:`. A document with a single .bib should not have to write a
// one-item sequence.
type StringList []string

func (s *StringList) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		var one string
		if err := n.Decode(&one); err != nil {
			return err
		}
		if strings.TrimSpace(one) != "" {
			*s = StringList{one}
		}
		return nil
	case yaml.SequenceNode:
		var many []string
		if err := n.Decode(&many); err != nil {
			return err
		}
		*s = StringList(many)
		return nil
	}
	return fmt.Errorf("bibliography: expected a path or a list of paths")
}

// Options are the mdbrand-specific keys, nested under `mdbrand:` so they never
// collide with a pandoc variable.
type Options struct {
	Brand        string `yaml:"brand"`
	Style        string `yaml:"style"`
	Confidential string `yaml:"confidential"` // stamped under the cover rule
	Reference    string `yaml:"reference"`    // file/offer number on the cover

	// letter style
	To        []string `yaml:"to"`
	Place     string   `yaml:"place"`
	Greeting  string   `yaml:"greeting"`
	Signature string   `yaml:"signature"`
}

// AuthorString renders Author for the cover, joining a list with " · ".
func (m Meta) AuthorString() string {
	switch v := m.Author.(type) {
	case nil:
		return ""
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, e := range v {
			parts = append(parts, fmt.Sprint(e))
		}
		return strings.Join(parts, " · ")
	default:
		return fmt.Sprint(v)
	}
}

// File is a parsed Markdown document.
type File struct {
	Path        string
	FrontMatter string // raw YAML between the --- fences, without them
	Body        string
	Meta        Meta
}

var fmRe = regexp.MustCompile(`(?s)\A---\r?\n(.*?)\r?\n(?:---|\.\.\.)[ \t]*\r?\n`)

// Read parses path, splitting front matter from body.
func Read(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f := &File{Path: path, Body: string(raw)}
	if m := fmRe.FindStringSubmatch(f.Body); m != nil {
		f.FrontMatter = m[1]
		f.Body = f.Body[len(m[0]):]
		if err := yaml.Unmarshal([]byte(f.FrontMatter), &f.Meta); err != nil {
			return nil, fmt.Errorf("%s: front matter: %w", path, err)
		}
	}
	return f, nil
}

// Fig is one diagram to render, extracted from the body.
type Fig struct {
	Kind        string            // "d2" or "vega"
	SrcPath     string            // absolute path of the source
	Caption     string            //
	Attrs       map[string]string // width=120mm, scale=0.6, …
	Placeholder string            // token left in the body
	Index       int
}

var (
	fenceRe = regexp.MustCompile(`^` + "```" + `+\s*(d2|vega|vegalite|vl)\b([^\n]*)$`)
	imgRe   = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+\.(?:d2|vl\.json|vl\.yaml|vega\.json))\)(\{[^}]*\})?`)
	attrRe  = regexp.MustCompile(`(\w+)\s*=\s*"([^"]*)"|(\w+)\s*=\s*(\S+)`)
)

func normKind(k string) string {
	if k == "d2" {
		return "d2"
	}
	return "vega"
}

func parseAttrs(s string) map[string]string {
	out := map[string]string{}
	for _, m := range attrRe.FindAllStringSubmatch(s, -1) {
		if m[1] != "" {
			out[strings.ToLower(m[1])] = m[2]
		} else {
			out[strings.ToLower(m[3])] = m[4]
		}
	}
	return out
}

// ExtractFigs replaces every diagram source in the body with a placeholder and
// writes fenced sources into srcDir. Side-file links are resolved relative to
// the document. The returned body is what pandoc will eventually see, once the
// placeholders are swapped for rendered figures.
func (f *File) ExtractFigs(srcDir string) (body string, figs []*Fig, err error) {
	docDir := filepath.Dir(f.Path)

	// Pass 1: fenced blocks, line by line, so an indented or longer fence still
	// terminates the block it opened.
	var out []string
	lines := strings.Split(f.Body, "\n")
	for i := 0; i < len(lines); i++ {
		m := fenceRe.FindStringSubmatch(lines[i])
		if m == nil {
			out = append(out, lines[i])
			continue
		}
		fence := lines[i][:strings.IndexFunc(lines[i], func(r rune) bool { return r != '`' })]
		var content []string
		i++
		for ; i < len(lines); i++ {
			if strings.HasPrefix(strings.TrimSpace(lines[i]), fence) && strings.TrimSpace(strings.Trim(lines[i], "`")) == "" {
				break
			}
			content = append(content, lines[i])
		}
		kind := normKind(m[1])
		attrs := parseAttrs(m[2])
		ext := ".d2"
		if kind == "vega" {
			ext = ".vl.json"
		}
		idx := len(figs)
		src := filepath.Join(srcDir, fmt.Sprintf("fig%02d%s", idx, ext))
		if err := os.WriteFile(src, []byte(strings.Join(content, "\n")+"\n"), 0o644); err != nil {
			return "", nil, err
		}
		ph := fmt.Sprintf("@@MDBRAND_FIG_%d@@", idx)
		figs = append(figs, &Fig{Kind: kind, SrcPath: src, Caption: attrs["caption"], Attrs: attrs, Placeholder: ph, Index: idx})
		out = append(out, ph)
	}
	body = strings.Join(out, "\n")

	// Pass 2: links to side files.
	body = imgRe.ReplaceAllStringFunc(body, func(s string) string {
		m := imgRe.FindStringSubmatch(s)
		p := m[2]
		if !filepath.IsAbs(p) {
			p = filepath.Join(docDir, p)
		}
		kind := "vega"
		if strings.HasSuffix(p, ".d2") {
			kind = "d2"
		}
		attrs := map[string]string{}
		if m[3] != "" {
			attrs = parseAttrs(m[3])
		}
		idx := len(figs)
		ph := fmt.Sprintf("@@MDBRAND_FIG_%d@@", idx)
		figs = append(figs, &Fig{Kind: kind, SrcPath: p, Caption: m[1], Attrs: attrs, Placeholder: ph, Index: idx})
		return ph
	})

	return body, figs, nil
}

// WidthMM returns an explicit width attribute in millimetres, or 0.
func (fg *Fig) WidthMM() float64 {
	v := fg.Attrs["width"]
	if v == "" {
		return 0
	}
	v = strings.TrimSuffix(strings.TrimSpace(v), "mm")
	x, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return x
}

// Scale returns an explicit d2 scale override, or 0.
func (fg *Fig) Scale() float64 {
	x, err := strconv.ParseFloat(fg.Attrs["scale"], 64)
	if err != nil {
		return 0
	}
	return x
}
