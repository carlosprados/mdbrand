---
title: "Gráficas en su idioma"
lang: es-ES
mdbrand:
  brand: none
  style: note
---

Una gráfica en un documento en español escribe sus números y sus meses en
español. Vega lo hacía en inglés, 30,000 y Jan, y ninguna de las vías obvias
para darle un *locale* llegaba a la vista.

```vegalite caption="Trabajos por mes"
{
  "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
  "width": 380, "height": 150,
  "data": {"values": [
    {"mes": "2026-01-15", "trabajos": 4200}, {"mes": "2026-02-15", "trabajos": 8100},
    {"mes": "2026-03-15", "trabajos": 15600}, {"mes": "2026-04-15", "trabajos": 29800}]},
  "mark": "bar",
  "encoding": {
    "x": {"field": "mes", "type": "ordinal", "timeUnit": "month", "title": null},
    "y": {"field": "trabajos", "type": "quantitative", "title": "trabajos"}},
  "config": {"axis": {"labelFontSize": 13, "titleFontSize": 13, "labelAngle": 0}}
}
```
