---
title: "Trap: a chart whose data is not there"
mdbrand: {brand: none, style: note}
---

vl2svg treats a data file it cannot open as a warning: it draws the axes with
nothing between them and exits 0. The build must stop naming the url instead
of printing an empty chart.

```vegalite caption="Sales"
{
  "data": {"url": "data/sales.cvs"},
  "mark": "bar",
  "encoding": {
    "x": {"field": "month", "type": "nominal"},
    "y": {"field": "sales", "type": "quantitative"}
  }
}
```
