package fig

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/carlosprados/mdbrand/internal/suggest"
)

// dataColumns is every column of every dataset the spec carries as values,
// and whether that is all of its data: a url to a JSON file, a generator or
// inline CSV text is data whose columns are not known here.
func dataColumns(v any) (cols map[string]bool, complete bool) {
	cols, complete = map[string]bool{}, true
	var walk func(any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			for k, c := range t {
				if k == "data" {
					complete = addColumns(c, cols) && complete
				}
				if k == "datasets" {
					if m, ok := c.(map[string]any); ok {
						for _, rows := range m {
							complete = addRows(rows, cols) && complete
						}
					}
					continue
				}
				walk(c)
			}
		case []any:
			for _, c := range t {
				walk(c)
			}
		}
	}
	walk(v)
	return cols, complete
}

func addColumns(data any, cols map[string]bool) bool {
	m, ok := data.(map[string]any)
	if !ok {
		return false // Vega's array of datasets
	}
	if vals, ok := m["values"]; ok {
		return addRows(vals, cols)
	}
	// {"name": …} naming one of the spec's own datasets is read there.
	_, named := m["name"]
	return named && len(m) == 1
}

func addRows(rows any, cols map[string]bool) bool {
	switch t := rows.(type) {
	case []map[string]any:
		for _, r := range t {
			for k := range r {
				cols[k] = true
			}
		}
	case []any:
		for _, r := range t {
			m, ok := r.(map[string]any)
			if !ok {
				return false // values that are not records: [1, 2, 3]
			}
			for k := range m {
				cols[k] = true
			}
		}
	default:
		return false
	}
	return true
}

// derives reports whether the spec makes fields of its own — a transform, or a
// Vega spec's marks and signals — so that a field outside the data is no typo.
func derives(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		for k, c := range t {
			if k == "values" || k == "datasets" {
				continue
			}
			if k == "transform" || k == "marks" || k == "signals" || derives(c) {
				return true
			}
		}
	case []any:
		for _, c := range t {
			if derives(c) {
				return true
			}
		}
	}
	return false
}

// unknownFields finds a field the chart plots that its data does not have.
// Vega-Lite draws a typo'd nominal field as one bar labelled "undefined", and
// a quantitative one as no bars at all, exit 0. Checked only where the data's
// columns are all known and the spec derives none of its own.
func unknownFields(v any) []string {
	cols, complete := dataColumns(v)
	if !complete || len(cols) == 0 || derives(v) {
		return nil
	}
	used := map[string]bool{}
	usedFields(v, used)
	var missing []string
	for f := range used {
		if !cols[f] && !cols[fieldRoot(f)] {
			missing = append(missing, f)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return []string{fieldsMessage(missing, cols)}
}

// fieldRoot is a nested field's first step: "a.b" reads column a. An escaped
// dot is part of the name and is unescaped.
func fieldRoot(f string) string {
	var b strings.Builder
	for i := 0; i < len(f); i++ {
		switch {
		case f[i] == '\\' && i+1 < len(f):
			i++
			b.WriteByte(f[i])
		case f[i] == '.' || f[i] == '[':
			return b.String()
		default:
			b.WriteByte(f[i])
		}
	}
	return b.String()
}

var infiniteRe = regexp.MustCompile(`Infinite extent for field "([^"]+)"`)

// emptyExtents reads Vega's own report of a quantitative field with no number
// in it — the case a transform hides from unknownFields. A stacked bar's
// field arrives as X_start and X_end.
func emptyExtents(log string, cols map[string]bool) []string {
	seen := map[string]bool{}
	var fields []string
	for _, m := range infiniteRe.FindAllStringSubmatch(log, -1) {
		f := m[1]
		for _, s := range []string{"_start", "_end"} {
			if t := strings.TrimSuffix(f, s); t != f && !cols[f] {
				f = t
			}
		}
		if !seen[f] {
			seen[f] = true
			fields = append(fields, f)
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return []string{fieldsMessage(fields, cols)}
}

func fieldsMessage(fields []string, cols map[string]bool) string {
	sort.Strings(fields)
	known := make([]string, 0, len(cols))
	for c := range cols {
		known = append(known, c)
	}
	sort.Strings(known)
	var lines []string
	for _, f := range fields {
		line := fmt.Sprintf("%q", f)
		if near := suggest.Closest(f, known); near != "" {
			line += fmt.Sprintf(" — did you mean %q?", near)
		}
		lines = append(lines, line)
	}
	msg := fmt.Sprintf(`the chart plots field(s) with no value in its data, and Vega-Lite would draw
  it empty or as one bar labelled undefined, exit 0:
    %s`, strings.Join(lines, "\n    "))
	if len(known) > 0 {
		msg += "\n  The data's fields: " + strings.Join(known, ", ")
	}
	return msg
}
