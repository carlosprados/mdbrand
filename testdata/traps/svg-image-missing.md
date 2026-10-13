---
title: "Trap: a picture inside a figure that is not there"
mdbrand:
  brand: none
  style: note
---

The image mark names a picture nothing provides: rsvg-convert drew an empty
space and the build exited 0.

```vega-lite caption="A missing picture"
{"$schema": "https://vega.github.io/schema/vega-lite/v5.json",
 "width": 200, "height": 100,
 "data": {"values": [{"x": 1, "y": 1, "img": "no-such.png"}]},
 "mark": {"type": "image", "width": 40, "height": 40},
 "encoding": {"x": {"field": "x", "type": "quantitative"},
              "y": {"field": "y", "type": "quantitative"},
              "url": {"field": "img"}}}
```
