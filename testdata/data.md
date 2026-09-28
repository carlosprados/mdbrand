---
title: "Oferta para {{data.cliente.nombre}}"
lang: es
mdbrand: {brand: none, style: note}
---

La instancia m5.xlarge ofrece {{data.maquinas[m5.xlarge].cpu}} vCPU,
{{data.maquinas[m5.xlarge].ram}} GiB de RAM y {{data.maquinas[m5.xlarge].disco}} GB
de disco, a {{data.maquinas[m5.xlarge].precio}} €/hora. La m5.large hereda
{{data.maquinas[m5.large].disco}} GB de disco de la familia
{{data.maquinas[m5.large].familia}}.

Condiciones: {{data.cliente.condiciones}}

In a code span a placeholder is left as written: `{{data.maquinas[m5.large].cpu}}`.

```vegalite caption="Latencia para {{data.cliente.nombre}}"
{
  "width": 360, "height": 160,
  "data": {"url": "data/latency.csv"},
  "mark": "bar",
  "encoding": {
    "x": {"field": "p", "type": "nominal", "title": "Percentil"},
    "y": {"field": "ms", "type": "quantitative", "title": "ms"}
  },
  "config": {"axis": {"labelFontSize": 14, "titleFontSize": 14}}
}
```
