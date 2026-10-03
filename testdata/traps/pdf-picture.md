---
title: "Trap: a PDF picture in a .docx"
mdbrand: {brand: none, style: note}
---

A PDF picture is fine in the PDF, where xelatex embeds it as it is. A .docx
cannot hold one: pandoc writes a reference Word draws as an empty frame. Built
with `--to docx`, this must stop and say to export the picture as SVG or PNG.

![A box made elsewhere](pictures/box.pdf)
