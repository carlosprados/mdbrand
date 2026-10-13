---
title: "Trap: a chart reading a Spanish CSV by url"
lang: en
mdbrand: {brand: none, style: note}
---

The same spreadsheet, linked by url instead of named. Vega's CSV reader splits
on commas alone, so the `;` export came out as one column and the chart drew
one bar, exit 0. Read through the document's parser, the decimal commas are
text in an English document, and the build says so.

```vegalite caption="Cost by site"
{
  "data": {"url": "../data/sedes.csv"},
  "mark": "bar",
  "encoding": {
    "y": {"field": "sede", "type": "nominal"},
    "x": {"field": "coste", "type": "quantitative"}
  }
}
```
