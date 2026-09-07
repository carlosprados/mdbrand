---
title: "mdbrand"
subtitle: "What one command produces from this file"
author: "mdbrand"
date: "September 2026"
lang: en-GB
toc: true
toc-depth: 2
mdbrand:
  brand: none
  style: report
---

# What this is

This file is the example. Build it with:

```sh
mdbrand build examples/demo.md --brand none
```

`--brand none` uses the built-in defaults, so it works on a machine with no
bundle set up. Point it at a real bundle and the cover, the header logo, the
colours and the type change; the Markdown does not.

# Figures

A D2 diagram, written inline. Both glob lines are mandatory: `**` reaches
shapes nested inside containers and `*` does not, so without them a nested box
keeps d2's 16px default and prints at 8px once the SVG is halved.

```d2 caption="Where the pieces sit"
**.style.font-size: 32
(** -> **)[*].style.font-size: 32
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
guide. `mdbrand diagrams examples/demo.md` reports those numbers without
building anything.

# Tables and text

| Style | Cover | Header | For |
|---|---|---|---|
| `report` | yes | from page 2 | Client-facing documents |
| `note` | no | from page 1 | Internal notes |
| `letter` | letterhead | none | Formal letters |

Ordinary Markdown works as expected: **emphasis**, `code`, footnotes, block
quotes and nested lists all pass straight through pandoc.
