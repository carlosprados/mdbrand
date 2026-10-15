---
title: "Layers that give one time scale a literal domain and a data one"
lang: es
mdbrand: {brand: none, style: note}
---

Each layer carries its own data, and one of them sets the time axis's domain
by hand. Vega-Lite unions the two and builds a dataset of its own for the
literal one, whose field is "data"; Vega reports that field's extent as
infinite. The chart draws whole, so the build must too.

```vegalite caption="Dos tramos y un hito"
{
  "width": 300, "height": 80,
  "layer": [
    {
      "data": {"values": [
        {"row": "Uno", "start": "2026-08-01", "end": "2026-11-15"},
        {"row": "Dos", "start": "2026-10-01", "end": "2027-02-01"}]},
      "mark": "bar",
      "encoding": {
        "y": {"field": "row", "type": "nominal", "title": null},
        "x": {"field": "start", "type": "temporal", "title": null,
              "scale": {"domain": ["2026-07-01", "2027-03-01"]}},
        "x2": {"field": "end"}
      }
    },
    {
      "data": {"values": [{"row": "Uno", "start": "2026-11-15"}]},
      "mark": {"type": "point", "shape": "diamond", "filled": true},
      "encoding": {
        "y": {"field": "row", "type": "nominal"},
        "x": {"field": "start", "type": "temporal"}
      }
    }
  ]
}
```
