---
title: "Oferta para {{data.cliente.nombre}}"   # a placeholder in the front matter is quoted
author: "Equipo de Preventa"
date: "9 de octubre de 2026"
lang: es-ES                       # numbers print as 1.250,5 and charts in Spanish
mdbrand:
  brand: none
  style: note
---

Los valores de este documento no están escritos a mano: salen de los ficheros
de `data/`. `mdbrand data datos.md` lista todo lo que se puede citar, y una
clave que no existe para el build nombrando las que sí.

# Valores en el texto

La instancia m5.xlarge ofrece {{data.maquinas[m5.xlarge].cpu}} vCPU y
{{data.maquinas[m5.xlarge].ram}} GiB de RAM a
{{data.maquinas[m5.xlarge].precio}} €/hora. La m5.large hereda
{{data.maquinas[m5.large].disco}} GB de disco de la base de su familia.

{{data.cliente.condiciones}}

# Tablas desde los datos

Filtrada, ordenada por memoria, con unidades, decimales redondeados en notación
española y una fila de total exacta:

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
total: [ram, precio]
caption: Instancias m5 para {{data.cliente.nombre}}
```

La comparativa con que acaba una oferta, con las máquinas como columnas:

```table
source: maquinas
rows: [m5.large, m5.xlarge, c6i.2xlarge]
columns: {id: Característica, cpu: vCPU, ram: {label: RAM, unit: GiB}, disco: {label: Disco, unit: GB}}
transpose: true
caption: Comparativa
```

Un CSV exportado de una hoja de cálculo, tal cual —punto y coma, comas
decimales, tildes—, enlazado como una imagen:

![Sedes](data/sedes.csv)

# Gráficos desde los mismos datos

Un gráfico lee los datos por su nombre, con el mismo lector que las tablas, así
que las comas decimales son números aquí igual que allí:

```vegalite caption="Coste por sede"
{
  "width": 360, "height": 140,
  "data": {"name": "sedes"},
  "mark": "bar",
  "encoding": {
    "y": {"field": "sede", "type": "nominal", "title": null},
    "x": {"field": "coste", "type": "quantitative", "title": "Coste (€)"}
  },
  "config": {"axis": {"labelFontSize": 14, "titleFontSize": 14}}
}
```
