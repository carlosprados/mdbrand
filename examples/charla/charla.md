---
title: "mdbrand"
subtitle: "The same Markdown, as a deck"
author: "mdbrand"
date: "October 2026"
lang: en-GB
mdbrand:
  brand: none
  style: slides                   # a 16:9 deck; # opens a section, ## is a slide
  formats: [pdf, notes]           # the deck, and charla-notes.pdf to speak from
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

::: notes
One command, no flags: everything comes from the front matter. The diagram is
drawn wide and low because a frame is 16:9.
:::

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

::: notes
A slide is read across a room, so its labels have a higher floor than a page's.
:::

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
Speaker notes never print on a slide. They print beside it in
charla-notes.pdf, two slides to an A4 sheet.
:::

## When a slide does not fit

- beamer sets the rest over the footer and exits 0
- mdbrand reads its log and stops the build
- naming the slide by its title, and by how much

::: notes
Show the error: the slide's title, and by how much it runs over.
:::

## Pauses

A line on its own with three spaced dots pauses:

. . .

and the rest of the slide appears on the next click.

## Point by point

::: incremental
- a list inside `::: incremental` reveals one point per click
- every step keeps the slide's number in the footer
- the notes show the slide whole
:::

::: notes
This slide has no note of its own beyond this one; a slide without notes still
gets its place in the notes, with room to write by hand.
:::
