package data

import (
	"strings"
	"testing"
)

const machines = `
_base: &base {familia: m5, disco: 50}
m5.xlarge:   {<<: *base, cpu: 4, ram: 16, precio: 0.192}
m5.large:    {<<: *base, cpu: 2, ram: 8,  precio: 0.096}
c6i.2xlarge: {familia: c6i, cpu: 8, ram: 16, disco: 100, precio: 0.34}
`

func TestTableRenders(t *testing.T) {
	s, _ := tree(t, map[string]string{"data/maquinas.yaml": machines})
	got, err := s.Table(`
source: maquinas
where: {familia: m5}
columns:
  id: Tipo
  ram: {label: RAM, unit: GiB}
  precio: {label: "€/h", decimals: 3}
sort: ram
caption: Instancias m5
`, "es")
	if err != nil {
		t.Fatal(err)
	}
	// Sorted by RAM, numbers right-aligned by themselves, the unit bound with a
	// no-break space, the price in Spanish notation; _base is not a machine.
	want := "| Tipo | RAM | €/h |\n" +
		"|:----------|-----:|------:|\n" +
		"| m5.large | 8\u00a0GiB | 0,096 |\n" +
		"| m5.xlarge | 16\u00a0GiB | 0,192 |\n" +
		"\n: Instancias m5\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestTableTranspose(t *testing.T) {
	s, _ := tree(t, map[string]string{"data/maquinas.yaml": machines})
	got, err := s.Table("source: maquinas\nrows: [m5.large, c6i.2xlarge]\ncolumns: {id: Tipo, cpu: vCPU}\ntranspose: true", "es")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"| Tipo | m5.large | c6i.2xlarge |", "| vCPU | 2 | 8 |"} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %q:\n%s", want, got)
		}
	}
}

func TestTableRefuses(t *testing.T) {
	s, _ := tree(t, map[string]string{
		"data/maquinas.yaml": machines,
		"data/raro.yaml":     "a: {x: 1}\nb: {y: 2}\n",
		"data/texto.yaml":    "a: {precio: consultar}\n",
	})
	for spec, want := range map[string]string{
		"source: maquinas\nrows: [m5.largo]":                  "the ids are m5.xlarge, m5.large, c6i.2xlarge",
		"source: maquinas\nwhere: {famila: m5}":               `no field "famila"`,
		"source: maquinas\nwhere: {familia: t3}":              "the table would be empty",
		"source: maquinas\nrows: [m5.large]\nwhere: {cpu: 2}": "keep one",
		"source: maquinas\nsortt: ram":                        `unknown key "sortt"`,
		"source: maquinas\ncolumns: {ram: {unidad: GiB}}":     `unknown key "unidad"`,
		"source: raro\ncolumns: [x]":                          `no field "x" (write x: ~`,
		"source: texto\ncolumns: {precio: {decimals: 2}}":     `not a number`,
		"source: maquinas[m5.large].cpu":                      "a single value, not a table",
	} {
		_, err := s.Table(spec, "es")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want %q", spec, err, want)
		}
	}
}

// A decimal comma is a number only where the document writes one: accepting it
// everywhere would read an English 1,234 as one point two three four.
func TestParseNumber(t *testing.T) {
	for _, tc := range []struct {
		in, dec string
		ok      bool
	}{
		{"0,085", ",", true},
		{"0,085", ".", false},
		{"1,234", ".", false},
		{"1.234,5", ",", false}, // thousands marks are never data
		{"0.192", ",", true},
		{"1e3", ".", true},
		{"1/3", ".", false},
	} {
		if _, ok := parseNumber(tc.in, tc.dec); ok != tc.ok {
			t.Errorf("parseNumber(%q, %q) = %v, want %v", tc.in, tc.dec, ok, tc.ok)
		}
	}
}

func TestFormatDecimal(t *testing.T) {
	for _, tc := range []struct {
		in, lang, want string
		d              int
	}{
		{"1234.5", "es", "1.234,50", 2},
		{"0.125", "es", "0,13", 2}, // exact half, away from zero: a float says 0.12
		{"0.125", "en", "0.13", 2},
		{"-0.001", "es", "0,00", 2},
		{"1234567", "en", "1,234,567", 0},
	} {
		r, _ := parseNumber(tc.in, ".")
		if got := formatDecimal(r, tc.d, tc.lang); got != tc.want {
			t.Errorf("formatDecimal(%s, %d, %s) = %q, want %q", tc.in, tc.d, tc.lang, got, tc.want)
		}
	}
}

func TestTableSortCollates(t *testing.T) {
	s, _ := tree(t, map[string]string{"data/sedes.csv": "ciudad,n\nZamora,1\nÁvila,2\nburgos,3\n"})
	got, err := s.Table("source: sedes\nsort: ciudad\ncolumns: [ciudad]", "es")
	if err != nil {
		t.Fatal(err)
	}
	a, b, z := strings.Index(got, "Ávila"), strings.Index(got, "burgos"), strings.Index(got, "Zamora")
	if !(a < b && b < z) {
		t.Errorf("want Ávila, burgos, Zamora — byte order puts Á after Z:\n%s", got)
	}
}

func TestTablesScanner(t *testing.T) {
	s, _ := tree(t, map[string]string{"data/maquinas.yaml": machines, "data/sedes.csv": "ciudad\nSoria\n"})
	doc := "Intro.\n\n```table\nsource: maquinas\ncolumns: [id]\n```\n\n" +
		"````markdown\n```table\nsource: nada\n```\n````\n\n" +
		"![Sedes](data/sedes.csv)\n"
	got, err := s.Tables(doc, "es")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "| m5.large |") || !strings.Contains(got, "| Soria |") || !strings.Contains(got, ": Sedes") {
		t.Errorf("tables not rendered:\n%s", got)
	}
	if !strings.Contains(got, "```table\nsource: nada\n```") {
		t.Errorf("a table fence shown inside a code block was expanded:\n%s", got)
	}
	if _, err := s.Tables("Ver ![Sedes](data/sedes.csv) aquí.", "es"); err == nil || !strings.Contains(err.Error(), "its own line") {
		t.Errorf("a table link inside a sentence: err = %v", err)
	}
}
