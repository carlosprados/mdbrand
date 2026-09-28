package fig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/carlosprados/mdbrand/internal/doc"
	"github.com/carlosprados/mdbrand/internal/run"
	"gopkg.in/yaml.v3"
)

// Vega-Lite reads its data at render time, and vl2svg forgives everything about
// it: a file it cannot open is a WARN on stderr, an empty chart and exit 0. It
// also resolves every url against its own base directory — which for a fenced
// block is the work directory, where the author's data/ is not — and prefixes
// that base even to an absolute path. So the urls are settled here, where a
// missing file can still stop the build, and handed to vl2svg as file:// URLs,
// the one form it reads as written.

var schemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

// VegaSpec returns the figure's spec as JSON with every local data url made
// absolute, and the data files it names — absolute, whether they exist or not,
// since watch mode must watch the missing one: creating it is the fix.
func VegaSpec(f *doc.Fig) (spec []byte, refs []string, err error) {
	raw, err := os.ReadFile(f.SrcPath)
	if err != nil {
		return nil, nil, err
	}
	var v any
	if ext := filepath.Ext(f.SrcPath); ext == ".yaml" || ext == ".yml" {
		// vl2svg reads JSON only; a .vl.yaml used to die inside Node.
		if err := yaml.Unmarshal(raw, &v); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", figName(f), err)
		}
	} else {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber() // a float64 round trip would reformat the spec's numbers
		if err := dec.Decode(&v); err != nil {
			return nil, nil, fmt.Errorf("%s: not valid JSON: %w", figName(f), err)
		}
	}

	base := f.BaseDir
	if base == "" {
		base = filepath.Dir(f.SrcPath)
	}
	r := &dataURLs{base: base}
	r.walk(v)
	sort.Strings(r.refs)

	if len(r.remote) > 0 {
		sort.Strings(r.remote)
		return nil, r.refs, fmt.Errorf(`%s: Vega-Lite would fetch its data at build time:
  %s
A build that depends on the network does not reproduce, and a failed fetch
draws the same empty chart and exits 0. Save the data beside the document and
point url at the file`, figName(f), strings.Join(r.remote, "\n  "))
	}
	if len(r.missing) > 0 {
		sort.Strings(r.missing)
		return nil, r.refs, fmt.Errorf(`%s: Vega-Lite would load data that is not there:
  %s
vl2svg only warns about a missing file, draws an empty chart and exits 0.
A relative url resolves against the document for a fenced block, and against
the spec's own file for a linked one`, figName(f), strings.Join(r.missing, "\n  "))
	}

	spec, err = json.Marshal(v)
	return spec, r.refs, err
}

// DataRefs lists the data files a Vega-Lite figure reads, for watch mode.
func DataRefs(f *doc.Fig) []string {
	_, refs, _ := VegaSpec(f)
	return refs
}

type dataURLs struct {
	base    string
	refs    []string
	missing []string // "as written  →  resolved"
	remote  []string
}

// walk visits every "data" in the spec: the top level's, each layer's and
// concatenated view's, a lookup's `from`, and Vega's array of datasets.
func (r *dataURLs) walk(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, c := range t {
			if k == "data" {
				r.data(c)
			}
			r.walk(c)
		}
	case []any:
		for _, c := range t {
			r.walk(c)
		}
	}
}

func (r *dataURLs) data(v any) {
	switch t := v.(type) {
	case map[string]any:
		r.url(t)
	case []any:
		for _, c := range t {
			if m, ok := c.(map[string]any); ok {
				r.url(m)
			}
		}
	}
}

func (r *dataURLs) url(m map[string]any) {
	u, ok := m["url"].(string)
	if !ok || u == "" {
		return
	}
	p := u
	switch {
	case filepath.IsAbs(u):
		// Checked before the scheme, so that C:\data.csv is a path.
	case strings.HasPrefix(u, "file://"):
		p = strings.TrimPrefix(u, "file://")
	case strings.HasPrefix(u, "//"), schemeRe.MatchString(u):
		r.remote = append(r.remote, u)
		return
	default:
		p = filepath.Join(r.base, u)
	}
	r.refs = append(r.refs, p)
	if _, err := os.Stat(p); err != nil {
		r.missing = append(r.missing, u+"  →  "+p)
		return
	}
	m["url"] = "file://" + filepath.ToSlash(p)
}

// figName names a figure for a message. A fenced block's source is a copy in
// the work directory called fig00.vl.json, which the author has never seen, so
// its caption is the better handle when it has one.
func figName(f *doc.Fig) string {
	if f.Caption != "" {
		return fmt.Sprintf("%s (%q)", filepath.Base(f.SrcPath), f.Caption)
	}
	return filepath.Base(f.SrcPath)
}

// renderVega renders a copy of the spec whose data urls have been settled.
func renderVega(f *doc.Fig, out string) error {
	spec, _, err := VegaSpec(f)
	if err != nil {
		return err
	}
	dir := filepath.Dir(out)
	src := filepath.Join(dir, fmt.Sprintf(".mdbrand-fig%02d.vl.json", f.Index))
	if err := os.WriteFile(src, spec, 0o644); err != nil {
		return err
	}
	// No dark-mode rules are injected: this SVG is going onto white paper.
	log, err := run.Cmd(dir, "vl2svg", filepath.Base(src), out)
	if err != nil {
		return err
	}
	// The belt to VegaSpec's braces: any load that still fails — a url in a
	// place walk does not visit — is the same empty chart behind exit 0.
	if strings.Contains(log, "Loading failed") {
		return fmt.Errorf("%s: vl2svg could not load the chart's data, and would have drawn it empty:\n%s",
			figName(f), strings.TrimSpace(log))
	}
	return nil
}
