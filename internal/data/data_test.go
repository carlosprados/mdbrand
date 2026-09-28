package data

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// tree writes files under a temporary document directory and opens its store.
func tree(t *testing.T, files map[string]string, declared ...string) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s, err := Open(filepath.Join(dir, "doc.md"), declared)
	if err != nil {
		t.Fatal(err)
	}
	return s, dir
}

const catalogue = `
base: &base {familia: m5, disco: 50}
m5.large:  {<<: *base, cpu: 2, ram: 8, precio: 0.10}
m5.xlarge: {<<: *base, cpu: 4, ram: 16, disco: 100}
nota: !md "ver *anexo*"
vacio: ~
`

func TestLookup(t *testing.T) {
	s, _ := tree(t, map[string]string{
		"data/maquinas.yaml": catalogue,
		"data/aws/ec2.csv":   "\xef\xbb\xbfid;vcpu;precio\nt3.micro;2;0,0104\nc6i.large;2;0,085\n",
	})
	for expr, want := range map[string]string{
		// The text as written: a float64 would print 0.1.
		"data.maquinas[m5.large].precio": "0.10",
		// Merged from the anchor, and overridden by the entry's own key.
		"data.maquinas[m5.large].familia": "m5",
		"data.maquinas[m5.xlarge].disco":  "100",
		// A BOM-prefixed, semicolon-separated export, found by its id column.
		"data.aws.ec2[c6i.large].precio": "0,085",
		"data.aws.ec2.0.id":              "t3.micro",
	} {
		v, err := s.Lookup(expr)
		if err != nil {
			t.Errorf("%s: %v", expr, err)
			continue
		}
		if v.Text != want {
			t.Errorf("%s = %q, want %q", expr, v.Text, want)
		}
	}
	if v, _ := s.Lookup("data.maquinas.nota"); !v.Markdown {
		t.Error("a !md value lost its tag")
	}
}

// Every one of these would print nothing, or the wrong thing, in a template
// engine; here each must stop with the words that fix it.
func TestLookupRefuses(t *testing.T) {
	s, _ := tree(t, map[string]string{
		"data/maquinas.yaml": catalogue,
		"data/dup.yaml":      "a: 1",
		"data/dup.json":      `{"a": 1}`,
	})
	for expr, want := range map[string]string{
		"data.maquinas[m5.largo].cpu": `the ones that exist are base, m5.large, m5.xlarge`,
		"data.maquinas[m5.large]":     "is a mapping, not a value",
		"data.maquinas.vacio":         "has no value",
		"data.maquina.x":              "there is dup, maquinas",
		"data.dup.a":                  "could be any of",
		"data.maquinas.m5.large":      `has no key "m5"`,
	} {
		_, err := s.Lookup(expr)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want it to say %q", expr, err, want)
		}
	}
}

func TestCSVRefuses(t *testing.T) {
	for name, tc := range map[string]struct{ body, want string }{
		"ragged": {"a,b\n1,2\n3\n", "line 3 has a different number of fields"},
		"latin1": {"a,b\nca\xf1a,2\n", "line 2 is not UTF-8"},
		"tie":    {"a;b,c\n1;2,3\n", "as many of one separator as another"},
		"dupcol": {"a,a\n1,2\n", `names "a" twice`},
	} {
		s, _ := tree(t, map[string]string{"data/t.csv": tc.body})
		_, err := s.Lookup("data.t.0.a")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
}

// The store must report what it looked for as well as what it read: the
// file that is missing is the one whose creation should rebuild.
func TestRefsIncludeMissing(t *testing.T) {
	s, dir := tree(t, map[string]string{"data/a.yaml": "x: 1"})
	s.Lookup("data.a.x")
	s.Lookup("data.b.x")
	refs := strings.Join(s.Refs(), "\n")
	for _, want := range []string{"data/a.yaml", "data/b.yaml", "data/b.csv"} {
		if !strings.Contains(refs, filepath.Join(dir, want)) {
			t.Errorf("refs lack %s:\n%s", want, refs)
		}
	}
}

func TestDeclaredRootsReplaceDefault(t *testing.T) {
	s, _ := tree(t, map[string]string{
		"data/a.yaml":        "x: default",
		"../shared/a.yaml":   "x: shared",
		"extra/tarifas.json": `{"hora": "65"}`,
	}, "../shared", "extra/tarifas.json")
	if v, err := s.Lookup("data.a.x"); err != nil || v.Text != "shared" {
		t.Errorf("data.a.x = %q, %v; want the declared root, not data/", v.Text, err)
	}
	if v, err := s.Lookup("data.tarifas.hora"); err != nil || v.Text != "65" {
		t.Errorf("a declared file is its own namespace: %q, %v", v.Text, err)
	}
}

func TestFillEscapes(t *testing.T) {
	s, _ := tree(t, map[string]string{"data/v.yaml": `
precio: "1.500 $ *neto* para @acme"
lista: "3. no es una lista"
md: !md "*sí*"
`})
	got, err := s.Fill("Son {{data.v.precio}}.\n{{data.v.lista}}\n{{data.v.md}} y `{{data.v.nada}}`")
	if err != nil {
		t.Fatal(err)
	}
	want := "Son 1.500 \\$ \\*neto\\* para \\@acme.\n3\\. no es una lista\n*sí* y `{{data.v.nada}}`"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if _, err := s.Fill("{{datos.v.precio}}"); err == nil || !strings.Contains(err.Error(), "the only namespace is data") {
		t.Errorf("a misspelt namespace must not be left in the PDF: %v", err)
	}
}

// A value with ": " substituted into raw YAML text would break the front
// matter, or quietly turn the title into a mapping.
func TestFillFrontMatter(t *testing.T) {
	s, _ := tree(t, map[string]string{"data/c.yaml": `nombre: "ACME: División \"Norte\""`})
	out, err := s.FillFrontMatter("title: \"Oferta para {{data.c.nombre}}\"\nlang: es")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := yaml.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("front matter no longer parses: %v\n%s", err, out)
	}
	if m["title"] != `Oferta para ACME: División "Norte"` || m["lang"] != "es" {
		t.Errorf("front matter = %v", m)
	}
}

func TestParsePath(t *testing.T) {
	segs, err := ParsePath("data.aws[m5.large].ram")
	if err != nil || strings.Join(segs, "|") != "aws|m5.large|ram" {
		t.Errorf("segs = %q, %v", segs, err)
	}
	if _, err := ParsePath("data.a b"); err == nil {
		t.Error("a key with a space outside brackets must be refused")
	}
}
