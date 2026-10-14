---
title: "Trap: two series one grey on e-ink"
mdbrand:
  brand: none
  style: note
---

A grey and a blue far apart in hue and 1.9 L* apart in lightness: an e-ink
screen draws them as the same bar. The build warns, naming both.

```vegalite caption="Two series"
{
  "width": 150, "height": 100,
  "data": {"values": [
    {"m": "a", "s": "uno", "v": 3}, {"m": "a", "s": "dos", "v": 4},
    {"m": "b", "s": "uno", "v": 5}, {"m": "b", "s": "dos", "v": 1}]},
  "mark": "bar",
  "encoding": {
    "x": {"field": "m", "type": "nominal"}, "xOffset": {"field": "s"},
    "y": {"field": "v", "type": "quantitative"},
    "color": {"field": "s", "type": "nominal", "scale": {"range": ["#5D6266", "#1565C0"]}}
  }
}
```
