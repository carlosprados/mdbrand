---
title: "Raíces de datos con nombre"
lang: es
mdbrand:
  brand: none
  style: note
  data: [data, {tao: data-named/tao}]
---

Del propio data/: {{data.cliente.nombre}}. De la raíz con nombre:
{{data.tao.principios[p06].name}}, con peso {{data.tao.principios[p06].peso}}.

```table
source: tao.principios
columns: {id: Clave, name: Principio, peso: Peso}
caption: Principios del otro proyecto
```

```vegalite caption="Pesos de la raíz con nombre"
{
  "width": 300, "height": 120,
  "data": {"name": "tao.principios"},
  "mark": "bar",
  "encoding": {
    "x": {"field": "name", "type": "nominal", "title": "Principio"},
    "y": {"field": "peso", "type": "quantitative", "title": "Peso"}
  }
}
```
