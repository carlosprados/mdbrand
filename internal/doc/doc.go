// Package doc reads a Markdown document's YAML front matter and rewrites its
// diagram sources into figure includes. Diagrams may be written two ways and
// both are supported: a fenced block (```d2 / ```vegalite) or a link to a side
// file (![Caption](diagrams/arch.d2)).
package doc

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/carlosprados/mdbrand/internal/mdtext"
	"github.com/carlosprados/mdbrand/internal/suggest"
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
	TOCDepth int    `yaml:"toc-depth"`
	Lang     string `yaml:"lang"`
	// pandoc's LaTeX template reads numbersections as a variable; the .docx
	// writer reads only the flag, so a numbered PDF came with an unnumbered
	// .docx.
	NumberSections bool `yaml:"numbersections"`

	// Citations. These are pandoc's own metadata names and stay at the top
	// level rather than moving under `mdbrand:`, so a document already written
	// for pandoc builds unchanged. Declaring a bibliography is what turns
	// --citeproc on; there is no separate switch to forget.
	Bibliography StringList `yaml:"bibliography"`
	CSL          string     `yaml:"csl"`
	// References are entries written in the front matter itself, which pandoc
	// cites from as it does from a file. Only their presence matters here.
	References any `yaml:"references"`

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
	return fmt.Errorf("expected a path or a list of paths")
}

// Options are the mdbrand-specific keys, nested under `mdbrand:` so they never
// collide with a pandoc variable.
type Options struct {
	Brand        string    `yaml:"brand"`
	Style        string    `yaml:"style"`
	Confidential string    `yaml:"confidential"` // stamped under the cover rule
	Reference    string    `yaml:"reference"`    // file/offer number on the cover
	ShortTitle   string    `yaml:"short_title"`  // the running header's title, when the full one is too long
	WordCount    WordCount `yaml:"wordcount"`
	// Data replaces the default data/ beside the document with these
	// directories or files, resolved against the document.
	Data StringList `yaml:"data"`
	// Formats are the outputs to write: pdf, docx, or both. `to:` would have
	// been pandoc's word for it, and it is the letter's recipient here.
	Formats StringList `yaml:"formats"`

	// letter style
	To        []string `yaml:"to"`
	Place     string   `yaml:"place"`
	Greeting  string   `yaml:"greeting"`
	Signature string   `yaml:"signature"`
}

// WordCount is the criterion {{words}} counts by: a profile name, or a mapping
// with a base profile and the parts to add to it. The names themselves are
// validated by the words package; unknown keys are refused here, since
// `incluide: [tables]` would otherwise count by the default without a word.
type WordCount struct {
	Base    string
	Include []string
}

func (w *WordCount) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		return n.Decode(&w.Base)
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i].Value, n.Content[i+1]
			switch k {
			case "base":
				if err := v.Decode(&w.Base); err != nil {
					return err
				}
			case "include":
				var one string
				if v.Kind == yaml.ScalarNode && v.Decode(&one) == nil {
					w.Include = []string{one}
					continue
				}
				if err := v.Decode(&w.Include); err != nil {
					return err
				}
			default:
				return fmt.Errorf("mdbrand.wordcount: unknown key %q: the keys are base and include", k)
			}
		}
		return nil
	}
	return fmt.Errorf("mdbrand.wordcount: expected a profile name or a mapping with base and include")
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
	// Refs is every file the body links to — side-file figures and pictures —
	// absolute, whether or not it exists. ExtractFigs fills it, even when it
	// fails: watch mode must watch the picture that is missing, since creating
	// it is exactly the change that should rebuild.
	Refs []string
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
		f.Body = f.Body[len(m[0]):]
		if err := f.SetFrontMatter(m[1]); err != nil {
			return nil, err
		}
	}
	return f, nil
}

// SetFrontMatter replaces the raw front matter and parses it again, so that
// Meta and what pandoc reads never disagree — after {{words}} is filled in, the
// cover takes its subtitle from Meta and pandoc its metadata from the raw text.
func (f *File) SetFrontMatter(raw string) error {
	var m Meta
	if err := yaml.Unmarshal([]byte(raw), &m); err != nil {
		return fmt.Errorf("%s: front matter: %w", f.Path, err)
	}
	if err := checkOptionKeys(raw); err != nil {
		return fmt.Errorf("%s: %w", f.Path, err)
	}
	f.FrontMatter, f.Meta = raw, m
	return nil
}

// checkOptionKeys refuses a key under mdbrand: that no option reads. YAML
// drops an unknown field in silence, so `confidencial:` built a document with
// no label and exited 0. Only mdbrand's own block is held to this: the rest of
// the front matter is pandoc's, and anything goes there.
func checkOptionKeys(raw string) error {
	var top struct {
		Options yaml.Node `yaml:"mdbrand"`
	}
	if err := yaml.Unmarshal([]byte(raw), &top); err != nil || top.Options.Kind != yaml.MappingNode {
		return nil
	}
	known := optionKeys()
	for i := 0; i+1 < len(top.Options.Content); i += 2 {
		k := top.Options.Content[i].Value
		if slices.Contains(known, k) {
			continue
		}
		hint := ""
		if near := suggest.Closest(k, known); near != "" {
			hint = fmt.Sprintf(" — did you mean %s?", near)
		}
		return fmt.Errorf("mdbrand.%s is not an option mdbrand reads%s\nThe options are %s",
			k, hint, strings.Join(known, ", "))
	}
	return nil
}

// optionKeys are the yaml names of Options' fields, so a new option is known
// here the moment it is declared.
func optionKeys() []string {
	t := reflect.TypeOf(Options{})
	keys := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		if name, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ","); name != "" && name != "-" {
			keys = append(keys, name)
		}
	}
	return keys
}

// Fig is one diagram to render, extracted from the body.
type Fig struct {
	Kind    string // "d2", "vega", or "svg" (already rendered)
	SrcPath string // absolute path of the source
	// BaseDir is what a relative path inside the source means relative to:
	// the document's directory for a fenced block, whose source is only a copy
	// written into the work directory, and the file's own for a side file.
	BaseDir     string
	Caption     string            //
	Attrs       map[string]string // width=120mm, scale=0.6, …
	Placeholder string            // token left in the body
	Index       int
	// Lang is the document's lang, which a chart formats its numbers and
	// dates in; "" for a source rendered on its own.
	Lang string
}

var (
	fenceRe = regexp.MustCompile(`^` + "```" + `+\s*(d2|vega|vegalite|vl)\b([^\n]*)$`)
	// A path as written, bare or in <…> (which may hold spaces), and an
	// optional title: a figure written either way was left to pandoc.
	imgRe = regexp.MustCompile(`!\[([^\]]*)\]\((<[^>\n]+\.(?i:d2|vl\.json|vl\.yaml|vega\.json|svg)>|[^)\s<]+\.(?i:d2|vl\.json|vl\.yaml|vega\.json|svg))(?:\s+(?:"[^"]*"|'[^']*'))?\)(\{[^}]*\})?`)
	// Pictures that go to LaTeX as they are. Only their path needs settling.
	picRe = regexp.MustCompile(`(!\[[^\]]*\]\()(<[^>\n]+\.(?i:png|jpe?g|pdf)>|[^)\s<]+\.(?i:png|jpe?g|pdf))((?:\s+(?:"[^"]*"|'[^']*'))?\))`)
	// A reference image, ![alt][label] or ![label], and the definition its
	// path is in: [label]: path "title". .PNG, <…> and references reached
	// xelatex unresolved, as its own "file not found".
	imgRefRe = regexp.MustCompile(`!\[([^\]]*)\](?:\[([^\]]*)\])?`)
	refDefRe = regexp.MustCompile(`(?m)^( {0,3}\[([^\]]+)\]:[ \t]*)(<[^>\n]+>|\S+)(.*)$`)
	attrRe   = regexp.MustCompile(`(\w+)\s*=\s*"([^"]*)"|(\w+)\s*=\s*(\S+)`)
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
	absDocDir := docDir
	if abs, err := filepath.Abs(docDir); err == nil {
		absDocDir = abs
	}
	f.Refs = nil

	body, figs, err = f.extractFenced(srcDir, absDocDir)
	if err != nil {
		return "", nil, err
	}

	// Pass 2: links to side files. A link inside code is an example of one —
	// a sample of Markdown in a fence, `![](x.d2)` in a sentence — and stays
	// as written.
	body = replaceOutsideCode(imgRe, body, func(m []string) string {
		p := strings.Trim(m[2], "<>")
		if !filepath.IsAbs(p) {
			p = filepath.Join(docDir, p)
		}
		// Absolute, always. Every renderer runs with its working directory set
		// to the source's own directory, so that d2 can resolve an @import —
		// which means a relative path here is resolved a second time against
		// itself and `mdbrand diagrams informe.md`, run from the document's own
		// directory, cannot find diagrams/x.d2.
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		// An .svg is a figure too, already rendered. It goes through the same
		// door as a diagram source — the <foreignObject> check, the legibility
		// sizing, rsvg-convert — because left to pandoc it became \includesvg,
		// which needs Inkscape, and its relative path was looked up from the
		// work directory, where it is not.
		kind := "vega"
		switch {
		case strings.HasSuffix(p, ".d2"):
			kind = "d2"
		case strings.HasSuffix(p, ".svg"):
			kind = "svg"
		}
		attrs := map[string]string{}
		if m[3] != "" {
			attrs = parseAttrs(m[3])
		}
		idx := len(figs)
		ph := fmt.Sprintf("@@MDBRAND_FIG_%d@@", idx)
		figs = append(figs, &Fig{Kind: kind, SrcPath: p, BaseDir: filepath.Dir(p), Caption: m[1], Attrs: attrs, Placeholder: ph, Index: idx, Lang: f.Meta.Lang})
		f.Refs = append(f.Refs, p)
		return ph
	})

	// Pass 3: plain pictures. pandoc hands their path to LaTeX untouched and
	// xelatex runs in the work directory, so a relative path that worked in
	// every Markdown viewer came back as "file not found" — and one that
	// happened to exist there would have been the wrong picture.
	var missing []string
	// In code it is an example, and printed this machine's absolute path.
	settle := func(written string) (string, bool) {
		p := strings.Trim(written, "<>")
		if strings.Contains(p, "://") {
			return written, false
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(docDir, p)
		}
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		f.Refs = append(f.Refs, p)
		if _, err := os.Stat(p); err != nil {
			missing = append(missing, written)
		}
		// In <…> with forward slashes: a directory with a space in it broke
		// the link, and a Windows path's backslashes read as escapes.
		return "<" + filepath.ToSlash(p) + ">", true
	}
	body = replaceOutsideCode(picRe, body, func(m []string) string {
		p, _ := settle(m[2])
		return m[1] + p + m[3]
	})
	body, err = settleRefPictures(body, settle)
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w", f.Path, err)
	}
	if len(missing) > 0 {
		return "", nil, fmt.Errorf("%s: pictures not found (paths resolve against the document): %s",
			f.Path, strings.Join(missing, ", "))
	}

	return body, figs, nil
}

// settleRefPictures resolves the definitions reference images use, as pass 3
// resolves inline ones. Only a label an image uses is touched: [spec]: x.pdf
// behind an ordinary link is a link, and stays as written. A figure source
// behind a reference would reach pandoc unrendered, so it stops the build.
func settleRefPictures(body string, settle func(string) (string, bool)) (string, error) {
	code := mdtext.Protected(body)
	labels := map[string]bool{}
	for _, ix := range imgRefRe.FindAllStringSubmatchIndex(body, -1) {
		end := ix[1]
		if mdtext.Inside(code, ix[0]) || end < len(body) && body[end] == '(' {
			continue // inline, pass 3's
		}
		label := body[ix[2]:ix[3]]
		if ix[4] >= 0 && ix[5] > ix[4] {
			label = body[ix[4]:ix[5]]
		}
		labels[strings.ToLower(strings.TrimSpace(label))] = true
	}
	if len(labels) == 0 {
		return body, nil
	}
	var figure []string
	out := replaceOutsideCode(refDefRe, body, func(m []string) string {
		if !labels[strings.ToLower(strings.TrimSpace(m[2]))] {
			return m[0]
		}
		target := strings.Trim(m[3], "<>")
		switch strings.ToLower(filepath.Ext(target)) {
		case ".png", ".jpg", ".jpeg", ".pdf":
			p, _ := settle(m[3])
			return m[1] + p + m[4]
		case ".svg", ".d2", ".json", ".yaml":
			figure = append(figure, m[3])
		}
		return m[0]
	})
	if len(figure) > 0 {
		return "", fmt.Errorf("a figure linked by reference is not rendered: %s — link it inline, ![caption](%s)",
			strings.Join(figure, ", "), figure[0])
	}
	return out, nil
}

// extractFenced is the first pass: each d2 or Vega-Lite fence becomes a
// placeholder and its source a file in srcDir. Line by line, so an indented or
// longer fence still terminates the block it opened; and a fence of any other
// language is copied whole, since a d2 block inside a ````markdown sample is
// an example of one, not a figure.
func (f *File) extractFenced(srcDir, baseDir string) (string, []*Fig, error) {
	var out []string
	var figs []*Fig
	lines := strings.Split(f.Body, "\n")
	for i := 0; i < len(lines); i++ {
		m := fenceRe.FindStringSubmatch(lines[i])
		if m == nil {
			out = append(out, lines[i])
			if code := otherFence(lines[i]); code != "" {
				for i++; i < len(lines); i++ {
					out = append(out, lines[i])
					if closesFence(lines[i], code) {
						break
					}
				}
			}
			continue
		}
		fence := lines[i][:strings.IndexFunc(lines[i], func(r rune) bool { return r != '`' })]
		var content []string
		for i++; i < len(lines); i++ {
			if closesFence(lines[i], fence) {
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
		figs = append(figs, &Fig{Kind: kind, SrcPath: src, BaseDir: baseDir, Caption: attrs["caption"], Attrs: attrs, Placeholder: ph, Index: idx, Lang: f.Meta.Lang})
		out = append(out, ph)
	}
	return strings.Join(out, "\n"), figs, nil
}

// otherFence is the fence a line opens that is not a figure's, or "".
func otherFence(line string) string {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return ""
	}
	return mdtext.Fence(trimmed)
}

// closesFence reports whether line closes a block opened by fence: the same
// character, at least as many, and nothing else on the line.
func closesFence(line, fence string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, fence) && strings.Trim(t, fence[:1]) == ""
}

// replaceOutsideCode is ReplaceAllStringFunc for matches that start in prose;
// one inside a code fence, a code span or mathematics is left as written.
func replaceOutsideCode(re *regexp.Regexp, s string, fn func(m []string) string) string {
	code := mdtext.Protected(s)
	var b strings.Builder
	last := 0
	for _, ix := range re.FindAllStringSubmatchIndex(s, -1) {
		if mdtext.Inside(code, ix[0]) {
			continue
		}
		m := make([]string, len(ix)/2)
		for g := range m {
			if ix[2*g] >= 0 {
				m[g] = s[ix[2*g]:ix[2*g+1]]
			}
		}
		b.WriteString(s[last:ix[0]])
		b.WriteString(fn(m))
		last = ix[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

// Scale returns an explicit d2 scale override, or 0.
func (fg *Fig) Scale() float64 {
	x, err := strconv.ParseFloat(fg.Attrs["scale"], 64)
	if err != nil {
		return 0
	}
	return x
}

// RunningTitle is the title the running header prints: short_title when the
// document gives one, the full title otherwise.
func (m Meta) RunningTitle() string {
	if m.Options.ShortTitle != "" {
		return m.Options.ShortTitle
	}
	return m.Title
}

// CitesFrom reports whether the document gives citeproc anything to cite
// from: a bibliography file or references in the front matter.
func (m Meta) CitesFrom() bool { return len(m.Bibliography) > 0 || m.References != nil }

// HasTOC reports whether the document asks for a table of contents.
func (m Meta) HasTOC() bool { return m.TOC != nil && *m.TOC }

// Printed is every front matter string a page prints, split by the face both
// writers set it in: a letter's greeting in the body face; the cover, the
// letterhead, the running header, the footer and the signature in the display
// face. The glyph checks read this, so a field printed somewhere and missing
// here goes unchecked — TestPrintedCoversEveryField is there for that.
func (m Meta) Printed() (display, body []string) {
	o := m.Options
	display = append([]string{m.Title, o.ShortTitle, m.Subtitle, m.AuthorString(), m.Date,
		o.Reference, o.Confidential, o.Place, o.Signature}, o.To...)
	return display, []string{o.Greeting}
}

// LiftAbstract takes abstract: out of the front matter and returns it as
// Markdown. pandoc's LaTeX template prints the abstract before the
// include-before body, so it came above the cover, and the .docx's repair
// dropped it with the title block. Each writer places it after its cover
// instead. A front matter without one comes back as it was, byte for byte.
func LiftAbstract(front string) (rest, abstract string, err error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(front), &root); err != nil || len(root.Content) == 0 {
		return front, "", err
	}
	m := root.Content[0]
	if m.Kind != yaml.MappingNode {
		return front, "", nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != "abstract" {
			continue
		}
		v := m.Content[i+1]
		if v.Kind != yaml.ScalarNode {
			return "", "", fmt.Errorf("abstract: write it as text — a block scalar (abstract: |) for several paragraphs")
		}
		m.Content = append(m.Content[:i], m.Content[i+2:]...)
		out, err := yaml.Marshal(&root)
		if err != nil {
			return "", "", err
		}
		return strings.TrimRight(string(out), "\n"), strings.TrimSpace(v.Value), nil
	}
	return front, "", nil
}
