---
title: "mdbrand"
subtitle: "What one command produces from this file"
author: "mdbrand"
date: "September 2026"
lang: en-GB
toc: true
toc-depth: 2
mdbrand:
  brand: none                     # a bundle name, e.g. amplia; none = built-in defaults
  style: report
  reference: "MDB-2026-001"       # on the cover
  confidential: "Internal"        # on the cover and every footer
---

# What this is

This file is the example of a `report`: a cover, a table of contents, a
running header, figures, a table and code. Build it with:

```sh
mdbrand build informe.md
```

`brand: none` uses the built-in defaults, so it works on a machine with no
bundle set up. Change it to a real bundle and the cover, the header logo, the
colours and the type change; the Markdown does not.

# Figures

A D2 diagram, written inline. No font-size in it: mdbrand sizes the labels
itself, from the bundle's band and the render scale, on a copy of the source.
`direction` is honoured here at the root — inside a container d2 v0.7.1 ignores
it, so containers group boxes without reshaping the layout.

```d2 caption="Where the pieces sit"
direction: right
md: Markdown
mb: mdbrand {
  figs: figures
  tex: LaTeX
}
pdf: A4 PDF
md -> mb.figs: d2 / vega
mb.figs -> mb.tex
mb.tex -> pdf: xelatex
```

A Vega-Lite chart, also inline. Its labels are set at 13px because the default
10px lands at 7.5pt on paper, below the legibility floor.

```vegalite caption="Pages per build"
{
  "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
  "width": 340, "height": 140,
  "data": {"values": [
    {"doc": "note", "pages": 3}, {"doc": "report", "pages": 7}, {"doc": "letter", "pages": 1}]},
  "mark": "bar",
  "encoding": {
    "x": {"field": "doc", "type": "nominal", "title": null},
    "y": {"field": "pages", "type": "quantitative", "title": "pages"}},
  "config": {"axis": {"labelFontSize": 13, "titleFontSize": 13, "labelAngle": 0}}
}
```

Each figure is placed at the widest size that fits the text measure while
keeping its labels inside the legibility band and its height under the page
guide. `mdbrand diagrams informe.md` reports those numbers without
building anything.

# Tables and text

| Style | Cover | Header | For |
|---|---|---|---|
| `report` | yes | from page 2 | Client-facing documents |
| `note` | no | from page 1 | Internal notes |
| `letter` | letterhead | none | Formal letters |

Ordinary Markdown works as expected: **emphasis**, `code`, footnotes, block
quotes and nested lists all pass straight through pandoc.

```go
// Code is fitted to the measure: the size steps down until its longest line
// fits, and a fence with a language wraps what still does not.
func build(doc string) error { return mdbrand.Build(doc) }
```
