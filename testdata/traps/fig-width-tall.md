---
title: "Trap: an explicit width that makes a figure taller than the page"
mdbrand:
  brand: none
  style: note
---

At width=160mm this chain is far taller than the page. pandoc's LaTeX scales
it down to the text height, labels and all, while the build reported the
labels at the size before.

```d2 width=160mm caption="A chain drawn top to bottom"
ingesta -> validación -> enriquecimiento -> almacén -> informe
```
