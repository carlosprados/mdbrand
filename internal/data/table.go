package data

import (
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/carlosprados/mdbrand/internal/words"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

// A table is declared, not programmed: which records, which columns, one sort
// key. Equality is the only filter and there is no expression language — the
// moment a table needs one, the data should be prepared before the document.

// tableSpec is the YAML body of a ```table block.
type tableSpec struct {
	Source    string            `yaml:"source"`
	Rows      []string          `yaml:"rows"`
	Where     map[string]string `yaml:"where"`
	Columns   columns           `yaml:"columns"`
	Sort      string            `yaml:"sort"`
	Transpose bool              `yaml:"transpose"`
	Caption   string            `yaml:"caption"`
}

var specKeys = []string{"source", "rows", "where", "columns", "sort", "transpose", "caption"}

type column struct {
	Field    string
	Label    string
	Unit     string
	Align    string // left, right, center; "" decides from the values
	Decimals *int
}

type columns []column

// UnmarshalYAML takes a list of field names, or a mapping of field to a label
// or to its settings. A mapping keeps its order, which is the column order.
func (cs *columns) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.SequenceNode:
		for _, c := range n.Content {
			if c.Kind != yaml.ScalarNode {
				return fmt.Errorf("columns: a list of columns holds field names")
			}
			*cs = append(*cs, column{Field: c.Value})
		}
		return nil
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			c := column{Field: n.Content[i].Value}
			v := n.Content[i+1]
			switch v.Kind {
			case yaml.ScalarNode:
				c.Label = v.Value
			case yaml.MappingNode:
				for j := 0; j+1 < len(v.Content); j += 2 {
					k, val := v.Content[j].Value, v.Content[j+1]
					switch k {
					case "label":
						c.Label = val.Value
					case "unit":
						c.Unit = val.Value
					case "align":
						switch val.Value {
						case "left", "right", "center":
							c.Align = val.Value
						default:
							return fmt.Errorf("columns.%s.align: %q — it is left, right or center", c.Field, val.Value)
						}
					case "decimals":
						d, err := strconv.Atoi(val.Value)
						if err != nil || d < 0 || d > 10 {
							return fmt.Errorf("columns.%s.decimals: %q is not a whole number from 0 to 10", c.Field, val.Value)
						}
						c.Decimals = &d
					default:
						return fmt.Errorf("columns.%s: unknown key %q: the keys are label, unit, align and decimals", c.Field, k)
					}
				}
			default:
				return fmt.Errorf("columns.%s: expected a label or a mapping of label, unit, align, decimals", c.Field)
			}
			*cs = append(*cs, c)
		}
		return nil
	}
	return fmt.Errorf("columns: expected a list of fields or a mapping of field to label")
}

func parseSpec(body string) (*tableSpec, error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(body), &root); err != nil {
		return nil, err
	}
	n := deref(&root)
	if n == nil || n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("the block holds YAML with at least a source: key")
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i].Value
		known := false
		for _, s := range specKeys {
			known = known || k == s
		}
		if !known {
			return nil, fmt.Errorf("unknown key %q: the keys are %s", k, strings.Join(specKeys, ", "))
		}
	}
	var s tableSpec
	if err := n.Decode(&s); err != nil {
		return nil, err
	}
	if s.Source == "" {
		return nil, fmt.Errorf("no source: name the data, as in source: maquinas")
	}
	if len(s.Rows) > 0 && len(s.Where) > 0 {
		return nil, fmt.Errorf("rows and where both choose the records; keep one")
	}
	return &s, nil
}

// record is one row of the data, before any formatting.
type record struct {
	id    string
	cells map[string]*yaml.Node
}

// records reads a node as a table: a mapping of records (the key becomes the
// id column), a list of records, or a mapping of values, which is one record.
func records(n *yaml.Node, shown string) ([]record, []string, error) {
	var recs []record
	var fields []string
	seen := map[string]bool{}
	addField := func(f string) {
		if !seen[f] {
			seen[f] = true
			fields = append(fields, f)
		}
	}
	fromMap := func(id string, m *yaml.Node) record {
		r := record{id: id, cells: map[string]*yaml.Node{}}
		for _, p := range listed(mapPairs(m)) {
			r.cells[p[0].Value] = p[1]
			addField(p[0].Value)
		}
		return r
	}
	switch n.Kind {
	case yaml.MappingNode:
		pairs := listed(mapPairs(n))
		scalars := 0
		for _, p := range pairs {
			if deref(p[1]).Kind == yaml.ScalarNode {
				scalars++
			}
		}
		switch scalars {
		case len(pairs):
			return []record{fromMap("", n)}, fields, nil
		case 0:
			addField("id")
			for _, p := range pairs {
				r := fromMap(p[0].Value, deref(p[1]))
				if _, own := r.cells["id"]; !own {
					r.cells["id"] = str(p[0].Value)
				}
				recs = append(recs, r)
			}
			return recs, fields, nil
		}
		return nil, nil, fmt.Errorf("%s mixes values and records, so it is neither one row nor a table", shown)
	case yaml.SequenceNode:
		for i, e := range n.Content {
			e = deref(e)
			if e.Kind != yaml.MappingNode {
				return nil, nil, fmt.Errorf("%s: entry %d is not a record (a mapping of fields)", shown, i)
			}
			r := fromMap(strconv.Itoa(i), e)
			if id, ok := r.cells["id"]; ok {
				r.id = deref(id).Value
			}
			recs = append(recs, r)
		}
		if len(recs) == 0 {
			return nil, nil, fmt.Errorf("%s is an empty list", shown)
		}
		return recs, fields, nil
	}
	return nil, nil, fmt.Errorf("%s is a single value, not a table", shown)
}

// Table renders a ```table block's body as a Markdown pipe table.
func (s *Store) Table(body, lang string) (string, error) {
	spec, err := parseSpec(body)
	if err != nil {
		return "", err
	}
	src := spec.Source
	if !strings.HasPrefix(src, "data.") && !strings.HasPrefix(src, "data[") {
		src = "data." + src
	}
	n, err := s.Node(src)
	if err != nil {
		return "", err
	}
	return render(n, src, spec, lang)
}

func render(n *yaml.Node, shown string, spec *tableSpec, lang string) (string, error) {
	recs, fields, err := records(n, shown)
	if err != nil {
		return "", err
	}
	fieldSet := map[string]bool{}
	for _, f := range fields {
		fieldSet[f] = true
	}
	needField := func(what, f string) error {
		if fieldSet[f] {
			return nil
		}
		return fmt.Errorf("%s: %s has no field %q; the fields are %s", what, shown, f, list(fields))
	}

	switch {
	case len(spec.Rows) > 0:
		byID := map[string]record{}
		var ids []string
		for _, r := range recs {
			byID[r.id] = r
			ids = append(ids, r.id)
		}
		var picked []record
		for _, id := range spec.Rows {
			r, ok := byID[id]
			if !ok {
				return "", fmt.Errorf("rows: %s has no %q; the ids are %s", shown, id, list(ids))
			}
			picked = append(picked, r)
		}
		recs = picked
	case len(spec.Where) > 0:
		keys := make([]string, 0, len(spec.Where))
		for k := range spec.Where {
			if err := needField("where", k); err != nil {
				return "", err
			}
			keys = append(keys, k)
		}
		var kept []record
		for _, r := range recs {
			match := true
			for _, k := range keys {
				c, ok := r.cells[k]
				match = match && ok && deref(c).Value == spec.Where[k]
			}
			if match {
				kept = append(kept, r)
			}
		}
		recs = kept
	}
	if len(recs) == 0 {
		return "", fmt.Errorf("where: no record of %s matches, so the table would be empty", shown)
	}

	if spec.Sort != "" {
		field, desc := strings.CutPrefix(spec.Sort, "-")
		if err := needField("sort", field); err != nil {
			return "", err
		}
		if err := sortRecords(recs, field, desc, lang); err != nil {
			return "", err
		}
	}

	cols := spec.Columns
	if len(cols) == 0 {
		for _, f := range fields {
			cols = append(cols, column{Field: f})
		}
	}
	header := make([]string, len(cols))
	for i, c := range cols {
		if err := needField("columns", c.Field); err != nil {
			return "", err
		}
		header[i] = c.Label
		if header[i] == "" {
			header[i] = c.Field
		}
		header[i] = strings.ReplaceAll(header[i], "|", `\|`)
	}

	_, dec := words.Separators(lang)
	cells := make([][]string, len(recs))
	numeric := make([][]bool, len(recs))
	for r, rec := range recs {
		cells[r] = make([]string, len(cols))
		numeric[r] = make([]bool, len(cols))
		for c, col := range cols {
			text, num, err := cell(rec, col, dec, lang)
			if err != nil {
				return "", fmt.Errorf("%s%s: %w", shown, formatPath([]string{rec.id}), err)
			}
			cells[r][c], numeric[r][c] = text, num
		}
	}

	align := make([]string, len(cols))
	for c, col := range cols {
		align[c] = col.Align
		if align[c] == "" {
			align[c] = "left"
			if allNumeric(len(recs), func(r int) bool { return numeric[r][c] }) {
				align[c] = "right"
			}
		}
	}

	if spec.Transpose {
		th := append([]string{header[0]}, column0(cells)...)
		var body [][]string
		for c := 1; c < len(cols); c++ {
			row := []string{header[c]}
			for r := range recs {
				row = append(row, cells[r][c])
			}
			body = append(body, row)
		}
		talign := []string{"left"}
		for r := range recs {
			a := "left"
			if allNumeric(len(cols)-1, func(c int) bool { return numeric[r][c+1] }) {
				a = "right"
			}
			talign = append(talign, a)
		}
		return pipeTable(th, body, talign, spec.Caption), nil
	}
	return pipeTable(header, cells, align, spec.Caption), nil
}

func column0(cells [][]string) []string {
	out := make([]string, len(cells))
	for i, row := range cells {
		out[i] = row[0]
	}
	return out
}

func allNumeric(n int, at func(int) bool) bool {
	if n == 0 {
		return false
	}
	for i := 0; i < n; i++ {
		if !at(i) {
			return false
		}
	}
	return true
}

// cell formats one value: as written, or rounded to the column's decimals in
// the document's notation, with the unit after a no-break space so that a
// number and its unit never land on different lines.
func cell(rec record, col column, dec, lang string) (string, bool, error) {
	n, ok := rec.cells[col.Field]
	if !ok {
		return "", false, fmt.Errorf("no field %q (write %s: ~ for an empty cell)", col.Field, col.Field)
	}
	n = deref(n)
	if n.Kind != yaml.ScalarNode {
		return "", false, fmt.Errorf("%s is not a single value", col.Field)
	}
	if n.ShortTag() == "!!null" || n.Value == "" {
		return "", false, nil
	}
	num, isNum := parseNumber(n.Value, dec)
	var text string
	switch {
	case col.Decimals != nil && !isNum:
		return "", false, fmt.Errorf("%s is %q, not a number, and its column rounds to %d decimals", col.Field, n.Value, *col.Decimals)
	case col.Decimals != nil:
		text = formatDecimal(num, *col.Decimals, lang)
	default:
		text = escapeMarkdown(Value{Text: n.Value, Markdown: n.Tag == "!md"})
		text = strings.Join(strings.Fields(text), " ")
		if n.Tag == "!md" {
			text = strings.ReplaceAll(text, "|", `\|`)
		}
	}
	if col.Unit != "" {
		text += " " + escapeMarkdown(Value{Text: col.Unit})
	}
	return text, isNum, nil
}

var plainNumberRe = regexp.MustCompile(`^[-+]?(\d+\.?\d*|\.\d+)([eE][-+]?\d+)?$`)

// parseNumber reads a number the way data files write one: a decimal point
// and no thousands marks, or — only where the document's language writes a
// decimal comma — a comma in its place, as a Spanish spreadsheet exports it.
// Accepting the comma everywhere would read an English 1,234 as one point two.
func parseNumber(s, dec string) (*big.Rat, bool) {
	s = strings.TrimSpace(s)
	if dec == "," && strings.Count(s, ",") == 1 && !strings.Contains(s, ".") {
		s = strings.Replace(s, ",", ".", 1)
	}
	if !plainNumberRe.MatchString(s) {
		return nil, false
	}
	r, ok := new(big.Rat).SetString(s)
	return r, ok
}

// formatDecimal rounds exactly — a big.Rat, never a float, halves away from
// zero — and writes the result with the language's marks.
func formatDecimal(r *big.Rat, decimals int, lang string) string {
	s := r.FloatString(decimals)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, frac, _ := strings.Cut(s, ".")
	th, dec := words.Separators(lang)
	out := words.Group(whole, th)
	if frac != "" {
		out += dec + frac
	}
	if neg && strings.Trim(out, "0.,") != "" {
		out = "-" + out
	}
	return out
}

// sortRecords orders by one field: numerically when every value is a number,
// otherwise by the document language's collation, so Á sorts beside A and not
// after Z. Ties keep the order of the data.
func sortRecords(recs []record, field string, desc bool, lang string) error {
	_, dec := words.Separators(lang)
	vals := make([]string, len(recs))
	nums := make([]*big.Rat, len(recs))
	allNum := true
	for i, r := range recs {
		if c, ok := r.cells[field]; ok {
			vals[i] = deref(c).Value
		}
		n, ok := parseNumber(vals[i], dec)
		nums[i] = n
		allNum = allNum && ok
	}
	idx := make([]int, len(recs))
	for i := range idx {
		idx[i] = i
	}
	col := collate.New(language.Make(lang))
	sort.SliceStable(idx, func(a, b int) bool {
		i, j := idx[a], idx[b]
		var cmp int
		if allNum {
			cmp = nums[i].Cmp(nums[j])
		} else {
			cmp = col.CompareString(vals[i], vals[j])
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
	sorted := make([]record, len(recs))
	for k, i := range idx {
		sorted[k] = recs[i]
	}
	copy(recs, sorted)
	return nil
}

// pipeTable writes a pandoc pipe table. The separator dashes are as long as
// each column's widest cell: once a row is wider than pandoc's 72 columns it
// sizes the columns in proportion to the dashes and wraps their text, so a
// long description gets the room and a number column does not. A column never
// gets less than its longest word and a little air, because a word cannot be
// wrapped: sized to the letter, "vCPU" printed past its column's edge.
func pipeTable(header []string, rows [][]string, align []string, caption string) string {
	width := make([]int, len(header))
	fit := func(c int, v string) {
		width[c] = max(width[c], 3, utf8.RuneCountInString(v))
		for _, w := range strings.Fields(v) {
			width[c] = max(width[c], utf8.RuneCountInString(w)+2)
		}
	}
	for c, h := range header {
		fit(c, h)
	}
	for _, row := range rows {
		for c, v := range row {
			fit(c, v)
		}
	}
	var b strings.Builder
	line := func(cells []string) {
		b.WriteString("|")
		for _, v := range cells {
			b.WriteString(" " + v + " |")
		}
		b.WriteString("\n")
	}
	line(header)
	b.WriteString("|")
	for c := range header {
		d := strings.Repeat("-", width[c])
		switch align[c] {
		case "right":
			d = d[1:] + ":"
		case "center":
			d = ":" + d[2:] + ":"
		default:
			d = ":" + d[1:]
		}
		b.WriteString(d + "|")
	}
	b.WriteString("\n")
	for _, row := range rows {
		line(row)
	}
	if c := strings.Join(strings.Fields(caption), " "); c != "" {
		b.WriteString("\n: " + c + "\n")
	}
	return b.String()
}
