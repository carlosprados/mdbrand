package doc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
