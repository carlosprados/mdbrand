package fig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

// Datasets supplies the rows for a spec's "data": {"name": …}: the document's
// data namespace, read the way its tables read it. nil where there is no
// document to read from.
type Datasets func(name string) ([]map[string]any, error)

// VegaSpec returns the figure's spec as JSON with every local data url made
// absolute and every named dataset filled in from ds, and the data files it
// names — absolute, whether they exist or not, since watch mode must watch the
// missing one: creating it is the fix.
func VegaSpec(f *doc.Fig, ds Datasets) (spec []byte, refs []string, err error) {
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
	r := &dataURLs{base: base, ds: ds, own: map[string]bool{}, named: map[string][]map[string]any{}}
	// A name the spec declares itself, under "datasets", is standard
	// Vega-Lite and stays exactly as written.
	if top, ok := v.(map[string]any); ok {
		if own, ok := top["datasets"].(map[string]any); ok {
			for k := range own {
				r.own[k] = true
			}
		}
	}
	r.walk(v)
	sort.Strings(r.refs)
	if len(r.errs) == 0 {
		used := map[string]bool{}
		usedFields(v, used)
		r.errs = mixedFields(r.named, used)
	}

	if len(r.errs) > 0 {
		return nil, r.refs, fmt.Errorf("%s: %s", figName(f), strings.Join(r.errs, "\n  "))
	}

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

// DataRefs lists the data files a Vega-Lite figure reads by url, for watch
// mode. Files read through a name are the data store's to report.
func DataRefs(f *doc.Fig) []string {
	_, refs, _ := VegaSpec(f, nil)
	return refs
}

type dataURLs struct {
	base    string
	ds      Datasets
	own     map[string]bool // names the spec's own datasets declare
	named   map[string][]map[string]any
	refs    []string
	missing []string // "as written  →  resolved"
	remote  []string
	errs    []string
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
		r.name(t)
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

// name fills a Vega-Lite "data": {"name": …} from the document's data. Only
// the object form is read: in a Vega spec's array of datasets, a name is the
// declaration of one, not a reference to be supplied.
func (r *dataURLs) name(m map[string]any) {
	n, ok := m["name"].(string)
	if !ok || r.own[n] {
		return
	}
	for _, k := range []string{"url", "values", "sequence", "graticule", "sphere"} {
		if _, ok := m[k]; ok {
			return
		}
	}
	if r.ds == nil {
		r.errs = append(r.errs, fmt.Sprintf("the chart names its data %q, which only a document's data files can supply", n))
		return
	}
	rows, err := r.ds(n)
	if err != nil {
		r.errs = append(r.errs, err.Error())
		return
	}
	delete(m, "name")
	m["values"] = rows
	r.named[n] = rows
}

// usedFields collects every "field" the spec encodes or transforms by,
// without descending into the data itself.
func usedFields(v any, used map[string]bool) {
	switch t := v.(type) {
	case map[string]any:
		for k, c := range t {
			switch {
			case k == "values" || k == "datasets":
			case k == "field":
				if f, ok := c.(string); ok {
					used[f] = true
				}
			default:
				usedFields(c, used)
			}
		}
	case []any:
		for _, c := range t {
			usedFields(c, used)
		}
	}
}

// mixedFields finds a field the chart uses that holds numbers and text at
// once. Vega-Lite drops a value it cannot read as a number, draws the rest and
// exits 0 — which is what 1250,5 from a Spanish spreadsheet does in an
// English document, where a decimal comma is text. Only the fields the spec
// names are checked: an id column mixing 2026 and m5.large harms no chart.
func mixedFields(named map[string][]map[string]any, used map[string]bool) []string {
	var errs []string
	for name, rows := range named {
		var mixed []string
		for field := range used {
			nums := 0
			var texts []string
			for _, row := range rows {
				switch t := row[field].(type) {
				case float64:
					nums++
				case string:
					texts = append(texts, strconv.Quote(t))
				}
			}
			if nums > 0 && len(texts) > 0 {
				mixed = append(mixed, fmt.Sprintf("%s: %d number(s) and %s", field, nums, strings.Join(texts, ", ")))
			}
		}
		if len(mixed) > 0 {
			sort.Strings(mixed)
			errs = append(errs, fmt.Sprintf(`data %q mixes numbers and text in a field the chart uses, and Vega-Lite silently drops what it cannot read as a number:
    %s
  Fix the data, or set the document's lang to one that writes these numbers
  (a decimal comma is a number only where lang writes one)`, name, strings.Join(mixed, "\n    ")))
		}
	}
	sort.Strings(errs)
	return errs
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

// vegaToSVG renders a Vega-Lite spec. In English it is vl2svg, as it always
// was; in a language with its own number format it is vl2vg and then vg2svg
// with that locale, the only route that applies one (see locale.go).
func vegaToSVG(dir, src, out, lang string) (string, error) {
	loc, err := localeArgs(dir, lang)
	if err != nil {
		return "", err
	}
	if loc == nil {
		return run.Cmd(dir, "vl2svg", src, out)
	}
	if err := probe(dir); err != nil {
		return "", err
	}
	vg := strings.TrimSuffix(src, ".vl.json") + ".vg.json"
	compiled, err := run.Cmd(dir, "vl2vg", src, vg)
	if err != nil {
		return compiled, err
	}
	log, err := run.Cmd(dir, "vg2svg", append(loc, vg, out)...)
	return compiled + log, err
}

// renderVega renders a copy of the spec whose data urls have been settled.
func renderVega(f *doc.Fig, out string, ds Datasets) error {
	spec, _, err := VegaSpec(f, ds)
	if err != nil {
		return err
	}
	dir := filepath.Dir(out)
	src := filepath.Join(dir, fmt.Sprintf(".mdbrand-fig%02d.vl.json", f.Index))
	if err := os.WriteFile(src, spec, 0o644); err != nil {
		return err
	}
	// No dark-mode rules are injected: this SVG is going onto white paper.
	log, err := vegaToSVG(dir, filepath.Base(src), out, f.Lang)
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
