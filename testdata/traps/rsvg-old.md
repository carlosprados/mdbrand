---
title: "Trap: a labelled d2 connection under rsvg-convert 2.40"
mdbrand: {brand: none, style: note}
---

rsvg-convert 2.40, the build most Windows installs carry, prints every SVG
`<mask>` as a black box, and d2 masks each labelled connection. torture.sh puts
an rsvg-convert that reports 2.40.20 first on the PATH: the build must stop
naming the fix instead of printing the boxes.

```d2
a -> b: a label
```
