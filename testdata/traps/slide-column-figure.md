---
title: "Figure in a column"
mdbrand:
  style: slides
---

<!-- A figure in a column was sized for the whole slide, 140mm, and drawn
     across the column beside it. At its column's width its labels are
     too small, so the build stops. -->

## A chart beside a list

:::: columns
::: column
```vega-lite caption="Load"
{"$schema": "https://vega.github.io/schema/vega-lite/v5.json",
 "width": 420, "height": 120,
 "data": {"values": [{"x": "a", "y": 3}, {"x": "b", "y": 5}, {"x": "c", "y": 4}]},
 "mark": "bar",
 "encoding": {"x": {"field": "x", "type": "nominal"},
              "y": {"field": "y", "type": "quantitative"}}}
```
:::
::: column
- One point
:::
::::
