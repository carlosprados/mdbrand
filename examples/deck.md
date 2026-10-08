---
title: "mdbrand"
subtitle: "The same Markdown, as a deck"
author: "mdbrand"
date: "October 2026"
lang: en-GB
mdbrand:
  brand: none
  style: slides
---

# How it works

## One command

```d2 caption="Where the pieces sit"
direction: right
md: Markdown
mb: mdbrand {
  figs: figures
  tex: beamer
}
pdf: 16:9 PDF
md -> mb.figs
mb.figs -> mb.tex
mb.tex -> pdf
```

## Figures sized for the frame

```vegalite caption="Smallest label, in points"
{
  "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
  "width": 460, "height": 150,
  "data": {"values": [
    {"where": "page floor", "pt": 8}, {"where": "slide floor", "pt": 8.5},
    {"where": "slide ceiling", "pt": 14}]},
  "mark": "bar",
  "encoding": {
    "x": {"field": "where", "type": "nominal", "title": null, "sort": null},
    "y": {"field": "pt", "type": "quantitative", "title": "pt"}},
  "config": {"axis": {"labelFontSize": 15, "titleFontSize": 15, "labelAngle": 0}}
}
```

# Writing one

## Two levels and two columns

:::: columns
::: column
```markdown
# A section
## A slide
```
:::
::: column
- `#` opens a section, with a slide of its own
- `##` is one slide
- `::: notes` never prints
:::
::::

::: notes
Speaker notes go here.
:::

## When a slide does not fit

- beamer sets the rest over the footer and exits 0
- mdbrand reads its log and stops the build
- naming the slide by its title, and by how much
