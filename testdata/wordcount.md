---
title: "Word count"
subtitle: "{{words}} palabras"
lang: es-ES
bibliography: refs.bib
mdbrand:
  brand: none
  style: note
---

<!-- The count is 35 by the IB criterion, worked out by hand; torture.sh
asserts it in the PDF. Change this document and recount it. Counted: headings,
prose, the block quote, list items, the footnote with content, a code span
inside a sentence, the References heading. Not counted: the citation, the note
that only cites, the dash, the formula, the code block, the table, the diagram
and its caption, the {.nocount} section and its subsection. -->

# Uno dos

Tres cuatro cinco seis siete ocho nueve diez.[^a] Once [@mdbrand2026] doce.[^b]

> trece catorce

- quince
- dieciséis — diecisiete

Este texto tiene {{words}} palabras.

`{{words}}` en código y $x = {{a}}$ en fórmula no se tocan.

```go
fmt.Println("{{ .Title }} is neither counted nor filled")
```

| cabecera | tabla |
|---|---|
| no | cuenta |

```d2 caption="Pie del diagrama"
a -> b
```

# Apéndice {.nocount}

Esto no cuenta nada de nada.

## Sub del apéndice

Tampoco.

# Referencias

[^a]: Nota con contenido real.
[^b]: [@mdbrand2026]
