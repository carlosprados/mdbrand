package words

import (
	"strings"
	"testing"
)

func TestFormat(t *testing.T) {
	for _, c := range []struct {
		n    int
		lang string
		want string
	}{
		{35, "es-ES", "35"},
		{4512, "es-ES", "4.512"},
		{1234567, "es", "1.234.567"},
		{4512, "en-GB", "4,512"},
		{4512, "", "4512"},   // no language: no separator rather than a wrong one
		{4512, "ja", "4512"}, // nor for one it does not know
	} {
		if got := Format(c.n, c.lang); got != c.want {
			t.Errorf("Format(%d, %q) = %q, want %q", c.n, c.lang, got, c.want)
		}
	}
}

// TestNewRules: a misspelt profile or part must stop the build, since reading
// it leniently prints a number by a criterion nobody asked for.
func TestNewRules(t *testing.T) {
	if r, err := NewRules("", nil); err != nil || r.String() != "ib" {
		t.Errorf("empty profile = %v, %v; want ib", r, err)
	}
	if r, _ := NewRules("ib", []string{"tables", "captions"}); r.String() != "ib+captions+tables" {
		t.Errorf("parts are named in a fixed order, got %s", r)
	}
	if r, _ := NewRules("all", nil); !r.Has("references") || !r.NeedsCiteproc() {
		t.Error("all must count the bibliography, which needs citeproc")
	}
	if _, err := NewRules("apa", nil); err == nil || !strings.Contains(err.Error(), "ib, all") {
		t.Errorf("unknown profile must name the valid ones, got %v", err)
	}
	if _, err := NewRules("ib", []string{"tabels"}); err == nil || !strings.Contains(err.Error(), "tabels") {
		t.Errorf("unknown part must be named, got %v", err)
	}
}

// TestFill: code and mathematics are left as written, prose is filled, and an
// unknown name in prose stops the build.
func TestFill(t *testing.T) {
	vals := map[string]string{"words": "35"}
	in := strings.Join([]string{
		"Tiene {{words}} palabras.",
		"`{{words}}` y ``a ` {{words}}`` en código.",
		"```go",
		`fmt.Println("{{words}}", "{{nope}}")`,
		"```",
		"~~~~",
		"{{words}}",
		"~~~~",
		"$x = {{a}}$ y $$",
		"{{b}}",
		"$$",
		"De $5 a {{words}} y $6.",
		`Escapado \$ {{words}}.`,
		"Go dice {{ .Title }}.",
	}, "\n")
	out, n, err := Fill(in, vals, true)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"Tiene 35 palabras.",
		"`{{words}}` y ``a ` {{words}}`` en código.",
		"```go",
		`fmt.Println("{{words}}", "{{nope}}")`,
		"```",
		"~~~~",
		"{{words}}",
		"~~~~",
		"$x = {{a}}$ y $$",
		"{{b}}",
		"$$",
		"De $5 a 35 y $6.", // currency, not math: $6 follows a space
		`Escapado \$ 35.`,
		"Go dice {{ .Title }}.",
	}, "\n")
	if out != want {
		t.Errorf("Fill:\n%s\nwant:\n%s", out, want)
	}
	if n != 3 {
		t.Errorf("filled %d, want 3", n)
	}

	if _, _, err := Fill("Hay {{palabras}}.", vals, true); err == nil || !strings.Contains(err.Error(), "{{words}}") {
		t.Errorf("an unknown placeholder must fail naming the valid ones, got %v", err)
	}
	// Front matter is YAML, not Markdown: nothing in it is code.
	if out, _, _ := Fill("subtitle: \"`{{words}}` palabras\"", vals, false); !strings.Contains(out, "`35`") {
		t.Errorf("front matter is filled everywhere, got %s", out)
	}
}

// ast is a pandoc 3 document written by hand, so the test does not depend on
// the pandoc installed. It holds one of each shape the criterion decides on.
const ast = `{"blocks":[
 {"t":"Header","c":[1,["",[],[]],[{"t":"Str","c":"Uno"},{"t":"Space"},{"t":"Str","c":"dos"}]]},
 {"t":"Para","c":[
   {"t":"Str","c":"tres"},{"t":"Space"},{"t":"Str","c":"—"},{"t":"Space"},{"t":"Str","c":"{{words}}."},
   {"t":"Note","c":[{"t":"Para","c":[{"t":"Str","c":"nota"},{"t":"Space"},{"t":"Str","c":"real"}]}]},
   {"t":"Note","c":[{"t":"Para","c":[{"t":"Cite","c":[[],[{"t":"Str","c":"(Knuth"},{"t":"Space"},{"t":"Str","c":"1984)"}]]}]}]},
   {"t":"Cite","c":[[],[{"t":"Str","c":"(Knuth"},{"t":"Space"},{"t":"Str","c":"1984)"}]]},
   {"t":"Code","c":[["",[],[]],"go build"]},
   {"t":"Math","c":[{"t":"InlineMath"},"a+b"]},
   {"t":"Str","c":"@@MDBRAND_FIG_0@@"}
 ]},
 {"t":"CodeBlock","c":[["",["go"],[]],"fmt.Println(x)"]},
 {"t":"CodeBlock","c":[["",["d2"],[]],"a -> b"]},
 {"t":"Figure","c":[["",[],[]],[null,[{"t":"Plain","c":[{"t":"Str","c":"pie"},{"t":"Space"},{"t":"Str","c":"foto"}]}]],
   [{"t":"Plain","c":[{"t":"Image","c":[["",[],[]],[{"t":"Str","c":"pie"},{"t":"Space"},{"t":"Str","c":"foto"}],["x.png",""]]}]}]]},
 {"t":"Table","c":[["",[],[]],[null,[]],[],[["",[],[]],[]],[[["",[],[]],0,[],[[["",[],[]],[[["",[],[]],{"t":"AlignDefault"},1,1,[{"t":"Plain","c":[{"t":"Str","c":"celda"},{"t":"Space"},{"t":"Str","c":"{{words}}"}]}]]]]]]],[["",[],[]],[]]]},
 {"t":"Header","c":[1,["",["nocount"],[]],[{"t":"Str","c":"Apéndice"}]]},
 {"t":"Para","c":[{"t":"Str","c":"fuera"}]},
 {"t":"Header","c":[2,["",[],[]],[{"t":"Str","c":"sub"}]]},
 {"t":"Para","c":[{"t":"Str","c":"fuera"}]},
 {"t":"Header","c":[1,["",[],[]],[{"t":"Str","c":"Referencias"}]]},
 {"t":"Div","c":[["refs",[],[]],[{"t":"Para","c":[{"t":"Str","c":"Knuth,"},{"t":"Space"},{"t":"Str","c":"TeXbook."}]}]]},
 {"t":"Div","c":[["",["nocount"],[]],[{"t":"Para","c":[{"t":"Str","c":"fuera"}]}]]}
]}`

// TestCount pins what each criterion counts. ib: Uno dos tres (3), the note
// with content (2), inline code (2), Referencias (1) = 8. The dash, the
// placeholders, the figure placeholder, the citation, the note that only cites,
// the formula, both code blocks, the figure, the table, the {.nocount} section
// with its subsection and the refs div are left out.
func TestCount(t *testing.T) {
	for _, c := range []struct {
		profile string
		include []string
		want    int
	}{
		{"ib", nil, 8},
		{"ib", []string{"captions"}, 10},  // the caption once, not again as alt text
		{"ib", []string{"footnotes"}, 10}, // + the note that only cites, as printed
		{"ib", []string{"citations"}, 10},
		{"ib", []string{"references"}, 10},
		{"ib", []string{"tables"}, 9},
		{"ib", []string{"code"}, 9}, // the go block; the d2 fence is a figure
		{"ib", []string{"math"}, 9},
		// all: every part at once. The note that only cites counts its
		// citation, the citation counts, so do captions, tables, code and math.
		{"all", nil, 8 + 2 + 2 + 2 + 2 + 1 + 1 + 1},
	} {
		r, err := NewRules(c.profile, c.include)
		if err != nil {
			t.Fatal(err)
		}
		n, ph, err := Count([]byte(ast), r)
		if err != nil {
			t.Fatal(err)
		}
		if n != c.want {
			t.Errorf("%s: %d words, want %d", r, n, c.want)
		}
		// Placeholders are seen wherever they are, counted or not: the one in
		// the uncounted table is filled too.
		if ph != 2 {
			t.Errorf("%s: saw %d placeholders, want 2", r, ph)
		}
	}
}
