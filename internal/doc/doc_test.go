package doc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const sample = `---
title: "Informe"
subtitle: "Sub"
author: "Departamento"
date: "7 de septiembre de 2026"
toc: true
mdbrand:
  brand: amplia
  style: report
  to: ["Alguien", "Empresa"]
---

# Uno

` + "```d2 caption=\"Arquitectura\" scale=0.4" + `
a -> b
` + "```" + `

Y una gráfica de fichero aparte:

![Latencia](diagrams/lat.vl.json)

` + "```go" + `
// A fence in another language must survive untouched.
fmt.Println("d2")
` + "```" + `
`

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadFrontMatter(t *testing.T) {
	dir := t.TempDir()
	f, err := Read(write(t, dir, "in.md", sample))
	if err != nil {
		t.Fatal(err)
	}
	if f.Meta.Title != "Informe" || f.Meta.Subtitle != "Sub" {
		t.Errorf("title/subtitle = %q/%q", f.Meta.Title, f.Meta.Subtitle)
	}
	if f.Meta.Options.Brand != "amplia" || f.Meta.Options.Style != "report" {
		t.Errorf("options = %+v", f.Meta.Options)
	}
	if f.Meta.TOC == nil || !*f.Meta.TOC {
		t.Error("toc not parsed")
	}
	if len(f.Meta.Options.To) != 2 {
		t.Errorf("to = %v", f.Meta.Options.To)
	}
	if strings.Contains(f.Body, "title:") {
		t.Error("front matter leaked into the body")
	}
}

func TestAuthorStringAcceptsBothShapes(t *testing.T) {
	if got := (Meta{Author: "A"}).AuthorString(); got != "A" {
		t.Errorf("string author = %q", got)
	}
	if got := (Meta{Author: []any{"A", "B"}}).AuthorString(); got != "A · B" {
		t.Errorf("list author = %q", got)
	}
	if got := (Meta{}).AuthorString(); got != "" {
		t.Errorf("absent author = %q", got)
	}
}

func TestExtractFigs(t *testing.T) {
	dir := t.TempDir()
	f, err := Read(write(t, dir, "in.md", sample))
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	body, figs, err := f.ExtractFigs(work)
	if err != nil {
		t.Fatal(err)
	}
	if len(figs) != 2 {
		t.Fatalf("got %d figures, want 2", len(figs))
	}

	fenced := figs[0]
	if fenced.Kind != "d2" || fenced.Caption != "Arquitectura" || fenced.Scale() != 0.4 {
		t.Errorf("fenced fig = %+v scale %g", fenced, fenced.Scale())
	}
	src, err := os.ReadFile(fenced.SrcPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(src)) != "a -> b" {
		t.Errorf("fenced source = %q", src)
	}

	side := figs[1]
	if side.Kind != "vega" || side.Caption != "Latencia" {
		t.Errorf("side fig = %+v", side)
	}
	if !filepath.IsAbs(side.SrcPath) || !strings.HasSuffix(side.SrcPath, filepath.Join("diagrams", "lat.vl.json")) {
		t.Errorf("side path = %q, want it resolved next to the document", side.SrcPath)
	}

	for _, fg := range figs {
		if !strings.Contains(body, fg.Placeholder) {
			t.Errorf("placeholder %s missing from the body", fg.Placeholder)
		}
	}
	// A fence in another language is not a diagram and must be left alone.
	if !strings.Contains(body, `fmt.Println("d2")`) {
		t.Error("a go fence was consumed")
	}
	if strings.Contains(body, "a -> b") {
		t.Error("the d2 fence was left in the body")
	}
}

func TestWidthAttr(t *testing.T) {
	fg := &Fig{Attrs: map[string]string{"width": "120mm"}}
	if got := fg.WidthMM(); got != 120 {
		t.Errorf("width = %g", got)
	}
	if got := (&Fig{Attrs: map[string]string{}}).WidthMM(); got != 0 {
		t.Errorf("absent width = %g, want 0", got)
	}
}

// `bibliography:` is pandoc's own key and people write it both ways. Accepting
// only a sequence would silently ignore the single-file form, which is the
// common one, and the PDF would come out full of "[@key?]".
func TestBibliographyAcceptsScalarOrList(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		want []string
	}{
		{"one file", "bibliography: refs.bib", []string{"refs.bib"}},
		{"a list", "bibliography:\n  - a.bib\n  - b.bib", []string{"a.bib", "b.bib"}},
		{"absent", "title: x", nil},
		{"empty", `bibliography: ""`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "d.md")
			if err := os.WriteFile(p, []byte("---\n"+tc.yaml+"\n---\n\nbody\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			f, err := Read(p)
			if err != nil {
				t.Fatal(err)
			}
			if len(f.Meta.Bibliography) != len(tc.want) {
				t.Fatalf("got %v, want %v", f.Meta.Bibliography, tc.want)
			}
			for i, w := range tc.want {
				if f.Meta.Bibliography[i] != w {
					t.Errorf("[%d] = %q, want %q", i, f.Meta.Bibliography[i], w)
				}
			}
		})
	}
}

// The keys pandoc lets a command-line variable evict. Detecting them is the
// whole of the fix: undetected, the document builds and simply is not what it
// asked to be, which is the failure mode this tool exists to remove.
func TestReservedKeysAreDetected(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		want []string
	}{
		{"none", "title: x\n", nil},
		{"header-includes as a list", "header-includes:\n  - \\usepackage{polyglossia}\n", []string{"header-includes"}},
		{"header-includes as a block", "header-includes: |\n  \\usepackage{polyglossia}\n", []string{"header-includes"}},
		{"all three", "header-includes: a\ninclude-before: b\ninclude-after: c\n", []string{"header-includes", "include-before", "include-after"}},
		// An empty key asks for nothing, so refusing the build would be noise.
		{"present but empty", "header-includes:\n", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var m Meta
			if err := yaml.Unmarshal([]byte(tc.yaml), &m); err != nil {
				t.Fatalf("front matter did not parse: %v", err)
			}
			got := m.ReservedKeys()
			if len(got) != len(tc.want) {
				t.Fatalf("ReservedKeys() = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("ReservedKeys()[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
