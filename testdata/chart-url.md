---
title: "Un gráfico que lee un CSV español por url"
lang: es
mdbrand: {brand: none, style: note}
---

`data/sedes.csv` es una exportación de hoja de cálculo española: `;` entre
campos, coma decimal y BOM. Enlazada por `url`, Vega la partía por comas y
dibujaba una sola barra; leída por el parser del documento, salen las tres sedes.

```vegalite caption="Coste por sede"
{
  "data": {"url": "data/sedes.csv"},
  "mark": "bar",
  "encoding": {
    "y": {"field": "sede", "type": "nominal"},
    "x": {"field": "coste", "type": "quantitative"}
  }
}
```
