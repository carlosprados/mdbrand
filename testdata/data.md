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

The m5 family, filtered, sorted by memory, with units and prices rounded in
Spanish notation; a long text column that pandoc must wrap, not overflow.

```table
source: maquinas
where: {familia: m5}
columns:
  id: Tipo
  cpu: vCPU
  ram: {label: RAM, unit: GiB}
  precio: {label: "€/hora", decimals: 3}
  uso: Uso recomendado
sort: -ram
caption: Instancias m5 para {{data.cliente.nombre}}
```

The comparison a proposal ends with, machines as columns.

```table
source: maquinas
rows: [m5.large, m5.xlarge, c6i.2xlarge]
columns: {id: Característica, cpu: vCPU, ram: {label: RAM, unit: GiB}, disco: {label: Disco, unit: GB}}
transpose: true
caption: Comparativa
```

A spreadsheet export linked as it is: BOM, semicolons, accents, decimal commas.

![Sedes](data/sedes.csv)
