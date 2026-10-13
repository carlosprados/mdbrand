package data

import (
	"path/filepath"
	"strings"

	"github.com/carlosprados/mdbrand/internal/words"
	"gopkg.in/yaml.v3"
)

// Records reads a namespace as a chart's rows: the same records a table would
// show, with every value typed. It is how a Vega-Lite spec that says
// "data": {"name": "maquinas"} reads the data the tables read, through the
// same parser: a decimal comma from a Spanish spreadsheet is a number here
// exactly when it is one in a table, instead of a string that vl2svg plots as
// nothing.
func (s *Store) Records(name, lang string) ([]map[string]any, error) {
	src := name
	if !strings.HasPrefix(src, "data.") && !strings.HasPrefix(src, "data[") {
		src = "data." + src
	}
	n, err := s.Node(src)
	if err != nil {
		return nil, err
	}
	return typedRecords(n, src, lang)
}

// FileRecords reads one data file as a chart's rows, through the parser
// Records uses: a chart whose "data": {"url": "x.csv"} points at a Spanish
// spreadsheet gets its separator sniffed and its decimal commas read as
// numbers where lang writes them, rather than Vega's comma-only CSV reader
// splitting nothing and drawing one bar.
func FileRecords(path, lang string) ([]map[string]any, error) {
	n, err := load(path)
	if err != nil {
		return nil, err
	}
	return typedRecords(n, filepath.Base(path), lang)
}

func typedRecords(n *yaml.Node, shown, lang string) ([]map[string]any, error) {
	recs, _, err := records(n, shown)
	if err != nil {
		return nil, err
	}
	_, dec := words.Separators(lang)
	out := make([]map[string]any, len(recs))
	for i, r := range recs {
		row := map[string]any{}
		for field, c := range r.cells {
			row[field] = typed(deref(c), dec)
		}
		out[i] = row
	}
	return out, nil
}

// typed turns a scalar into the JSON value a chart needs. A number becomes a
// number whatever file it came from — YAML tags it, CSV does not — and
// anything that is not a scalar is left out of a chart's reach as null.
func typed(n *yaml.Node, dec string) any {
	if n.Kind != yaml.ScalarNode {
		return nil
	}
	switch n.ShortTag() {
	case "!!null":
		return nil
	case "!!bool":
		return strings.EqualFold(n.Value, "true")
	}
	if r, ok := parseNumber(n.Value, dec); ok {
		f, _ := r.Float64()
		return f
	}
	return n.Value
}
