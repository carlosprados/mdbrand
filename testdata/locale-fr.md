---
title: "Graphiques dans leur langue"
lang: fr-FR
mdbrand:
  brand: none
  style: note
---

Un graphique dans un document en français écrit ses nombres et ses mois en
français. Vega écrivait 30,000 et Jan, et en allemand les mois restaient
anglais.

```vegalite caption="Travaux par mois"
{
  "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
  "width": 380, "height": 150,
  "data": {"values": [
    {"mois": "2026-01-15", "travaux": 4200}, {"mois": "2026-02-15", "travaux": 8100},
    {"mois": "2026-03-15", "travaux": 15600}, {"mois": "2026-04-15", "travaux": 29800}]},
  "mark": "bar",
  "encoding": {
    "x": {"field": "mois", "type": "ordinal", "timeUnit": "month", "title": null},
    "y": {"field": "travaux", "type": "quantitative", "title": "travaux"}},
  "config": {"axis": {"labelFontSize": 13, "titleFontSize": 13, "labelAngle": 0}}
}
```
