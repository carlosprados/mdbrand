---
title: "Trap: a chart plotting a field its data does not have"
mdbrand:
  brand: none
  style: note
---

`sedes` for `sede`: Vega-Lite drew one bar labelled undefined, exit 0.

```vega-lite caption="Coste por sede"
{"$schema": "https://vega.github.io/schema/vega-lite/v5.json",
 "width": 200, "height": 100,
 "data": {"values": [{"sede": "Zamora", "coste": 3}, {"sede": "Ávila", "coste": 5}]},
 "mark": "bar",
 "encoding": {"x": {"field": "sedes", "type": "nominal"},
              "y": {"field": "coste", "type": "quantitative"}}}
```
