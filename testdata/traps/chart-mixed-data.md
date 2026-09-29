---
title: "Trap: a chart whose numbers are partly text"
lang: en
mdbrand: {brand: none, style: note, data: [../data]}
---

The costs come from a Spanish spreadsheet, with decimal commas. In an English
document those are text, and Vega-Lite drops a value it cannot read as a
number: it would draw Ávila alone, exit 0, and nobody would count the bars.

```vegalite caption="Cost by site"
{
  "data": {"name": "sedes"},
  "mark": "bar",
  "encoding": {
    "y": {"field": "sede", "type": "nominal"},
    "x": {"field": "coste", "type": "quantitative"}
  }
}
```
