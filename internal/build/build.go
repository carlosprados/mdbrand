// Package build is the whole pipeline: read the document, render its diagrams,
// generate the LaTeX fragments from the brand bundle, run pandoc and then
// xelatex, and refuse to hand over a PDF with holes in it.
//
// Two decisions are deliberate and worth knowing:
//
//   - xelatex is run by mdbrand, not by pandoc's --pdf-engine. pandoc throws the
//     engine's log away, and that log is the only place where "this font has no
//     glyph for ☐" appears. A checkbox that silently became a blank space is
//     exactly the class of defect this tool exists to stop.
//   - engine choice is not a flag. pdflatex cannot take the Unicode these
//     documents are full of, so there is nothing to choose.
package build

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/carlosprados/mdbrand/internal/data"
	"github.com/carlosprados/mdbrand/internal/doc"
	"github.com/carlosprados/mdbrand/internal/fig"
	"github.com/carlosprados/mdbrand/internal/run"
	"github.com/carlosprados/mdbrand/internal/words"
)

// Options drive one build.
type Options struct {
	Input     string
	Output    string
	BrandsDir string
	BrandName string
	Style     string
	// Defaults from configuration, used only when neither a flag nor the
	// document's own front matter says otherwise.
	DefaultBrand string
	DefaultStyle string
	// WordCount is the --wordcount profile. It replaces the document's whole
	// criterion, parts included; DefaultWordCount is configuration's.
	WordCount        string
	DefaultWordCount string
	WorkDir          string // when set, the work directory is kept for inspection
	AllowHoles       bool   // proceed even if the font lacks glyphs the text uses
	Log              func(string, ...any)
}

// Report is what the build produced.
type Report struct {
	Output   string
	Pages    int
	Brand    string
	Style    string
	Figures  []*fig.Result
	Warnings []string
	WorkDir  string
	Kept     bool
	Words    int    // by WordRule; counted on every build
	WordRule string // the criterion, as ib, ib+tables or all
	// Inputs is every file this build read that a person edits: the document,
	// its linked figures and pictures, bibliography and CSL, the bundle's
	// brand.yaml and logos. Watch mode rebuilds when one of them changes.
	Inputs []string
}

// Tools are the external programs a build needs, whatever the document holds.
var Tools = []string{"pandoc", "xelatex", "rsvg-convert"}

func (o *Options) logf(f string, a ...any) {
	if o.Log != nil {
		o.Log(f, a...)
	}
}

// Pick returns the first non-empty value, which encodes the precedence every
// setting follows: an explicit flag beats the document's front matter, and the
// front matter beats a machine-wide default.
//
// Getting this backwards is not a cosmetic bug. A configured default used to be
// applied before the front matter was read, so a document that asked for
// `brand: none` was built with whatever brand the reader had configured — and a
// colleague building the unbranded example got a font error about a typeface
// the document never mentions.
func Pick(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// Run executes the pipeline. The Report is never nil, even alongside an error:
// its Inputs hold what the build had read before it stopped, which is what watch
// mode must keep watching so the fix can trigger the next build.
func Run(o Options) (*Report, error) {
	var inputs []string
	rep, err := pipeline(o, &inputs)
	if rep == nil {
		rep = &Report{}
	}
	rep.Inputs = inputs
	return rep, err
}

// pipeline is one PDF build: the steps every format shares, then the PDF's own.
func pipeline(o Options, inputs *[]string) (*Report, error) {
	*inputs = append(*inputs, mustAbs(o.Input))
	if missing := run.Missing(Tools...); len(missing) > 0 {
		return nil, fmt.Errorf("missing tools: %s\n  run: mdbrand doctor", strings.Join(missing, ", "))
	}
	p, err := prepare(o, inputs)
	if p == nil {
		return nil, err
	}
	defer p.close()
	// Deferred, so that it also holds what a chart read through a name.
	defer func() { *inputs = append(*inputs, p.refs()...) }()
	if err != nil {
		return nil, err
	}
	return renderPDF(p, inputs)
}

func figTools(figs []*doc.Fig) []string {
	need := map[string]bool{}
	for _, f := range figs {
		switch f.Kind {
		case "d2":
			need["d2"] = true
		case "vega":
			need["vl2svg"] = true
		}
		// "svg" is already rendered: rsvg-convert, which every build needs.
	}
	out := make([]string, 0, len(need))
	for k := range need {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// copyFile lands dst whole or not at all: it writes a temporary beside it and
// renames it into place. A PDF viewer that reloads on change (zathura, evince)
// read a truncated file when os.WriteFile was caught halfway through, and in
// watch mode that happens on every build.
func copyFile(src, dst string) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(dst)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

func mustAbs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

// citeprocMissingRe matches pandoc's own wording for a citation key that has no
// entry: "[WARNING] Citeproc: citation fml not found".
var citeprocMissingRe = regexp.MustCompile(`Citeproc: citation ([^\s]+) not found`)

// missingCitations extracts the unresolved keys from pandoc's output, in order
// and without repeats. pandoc reports them as warnings and still exits 0, which
// is how a PDF reaches a reader with "(fml?)" printed in the middle of a
// sentence.
func missingCitations(out string) []string {
	var keys []string
	seen := map[string]bool{}
	for _, m := range citeprocMissingRe.FindAllStringSubmatch(out, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			keys = append(keys, m[1])
		}
	}
	return keys
}

// citeArgs turns the front matter's bibliography into pandoc flags. pandoc runs
// in the work directory, so a `bibliography:` left in the front matter would be
// resolved against the wrong place and silently yield "[@key?]" in the PDF.
// Passing the files absolutely on the command line overrides the metadata and
// removes the whole class of error.
func citeArgs(d *doc.File, input string, inputs *[]string) ([]string, error) {
	if len(d.Meta.Bibliography) == 0 {
		return nil, nil
	}
	docDir := filepath.Dir(mustAbs(input))
	args := []string{"--citeproc"}
	for _, ref := range d.Meta.Bibliography {
		p := ref
		if !filepath.IsAbs(p) {
			p = filepath.Join(docDir, p)
		}
		*inputs = append(*inputs, p)
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf(`bibliography %s: %w
  Paths are resolved relative to the document, not to the working directory.`, ref, err)
		}
		args = append(args, "--bibliography="+p)
	}
	if csl := d.Meta.CSL; csl != "" {
		if !filepath.IsAbs(csl) {
			csl = filepath.Join(docDir, csl)
		}
		*inputs = append(*inputs, csl)
		if _, err := os.Stat(csl); err != nil {
			return nil, fmt.Errorf("csl %s: %w", d.Meta.CSL, err)
		}
		args = append(args, "--csl="+csl)
	}
	return args, nil
}

// fillData prints the document's {{data…}} values into the body, the figure
// captions and the front matter. It runs before the word count, which counts
// what the reader will see, and after figure extraction, so that a caption is
// filled where it now lives.
func fillData(o Options, d *doc.File, store *data.Store, body string, figs []*doc.Fig) (string, error) {
	var err error
	// Tables first: a caption may hold a {{data…}} value, which the pass
	// below then fills like any other.
	if body, err = store.Tables(body, d.Meta.Lang); err != nil {
		return "", fmt.Errorf("%s: %w", o.Input, err)
	}
	if body, err = store.Fill(body); err != nil {
		return "", fmt.Errorf("%s: %w", o.Input, err)
	}
	for _, f := range figs {
		if f.Caption, err = store.Fill(f.Caption); err != nil {
			return "", fmt.Errorf("%s: caption: %w", o.Input, err)
		}
	}
	fm, err := store.FillFrontMatter(d.FrontMatter)
	if err != nil {
		return "", fmt.Errorf("%s: front matter: %w", o.Input, err)
	}
	return body, d.SetFrontMatter(fm)
}

// Datasets lets a chart's "data": {"name": …} read the document's data, with
// numbers parsed by the document's language, as its tables parse them.
func Datasets(store *data.Store, lang string) fig.Datasets {
	return func(name string) ([]map[string]any, error) {
		return store.Records(name, lang)
	}
}

// wordCount counts the body by the document's criterion and fills {{words}} in
// the body, the front matter and the figure captions. It runs after figure
// extraction, since pandoc would read a ```d2 fence as inline code and count
// the diagram's source as prose.
func wordCount(o Options, d *doc.File, body string, figs []*doc.Fig, work string, rep *Report) (string, error) {
	include := d.Meta.Options.WordCount.Include
	if o.WordCount != "" {
		include = nil
	}
	rules, err := words.NewRules(Pick(o.WordCount, d.Meta.Options.WordCount.Base, o.DefaultWordCount), include)
	if err != nil {
		return "", err
	}
	src := filepath.Join(work, ".mdbrand-words.md")
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		return "", err
	}
	args := []string{"-f", "markdown", "-t", "json", filepath.Base(src)}
	if rules.NeedsCiteproc() {
		var discard []string
		cites, err := citeArgs(d, o.Input, &discard)
		if err != nil {
			return "", err
		}
		args = append(args, cites...)
	}
	ast, err := run.Stdout(work, "pandoc", args...)
	if err != nil {
		return "", err
	}
	n, seen, err := words.Count(ast, rules)
	if err != nil {
		return "", err
	}
	if rules.Has("captions") {
		for _, f := range figs {
			n += words.CountText(f.Caption)
		}
	}
	rep.Words, rep.WordRule = n, rules.String()

	vals := map[string]string{"words": words.Format(n, d.Meta.Lang)}
	body, filled, err := words.Fill(body, vals, true)
	if err != nil {
		return "", fmt.Errorf("%s: %w", o.Input, err)
	}
	if filled != seen {
		// The two readings of the document disagree about where prose ends and
		// code or mathematics begins. Guessing would print {{words}} in the PDF
		// or a number inside a code sample, so the build stops instead.
		return "", fmt.Errorf(`%s: pandoc sees %d placeholder(s) in prose and mdbrand filled %d.
Some construct around a placeholder is read differently by the two; put the
placeholder in a plain sentence, and please report the case:
  https://github.com/carlosprados/mdbrand/issues`, o.Input, seen, filled)
	}
	fm, _, err := words.Fill(d.FrontMatter, vals, false)
	if err != nil {
		return "", fmt.Errorf("%s: front matter: %w", o.Input, err)
	}
	if err := d.SetFrontMatter(fm); err != nil {
		return "", err
	}
	for _, f := range figs {
		if f.Caption, _, err = words.Fill(f.Caption, vals, false); err != nil {
			return "", fmt.Errorf("%s: caption: %w", o.Input, err)
		}
	}
	return body, nil
}
