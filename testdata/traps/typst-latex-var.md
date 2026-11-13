---
title: "Trap: a pandoc variable only XeLaTeX reads"
indent: true
mdbrand:
  brand: none
  style: note
---

XeLaTeX indents every paragraph's first line when indent is set; Typst's
template never reads it, and the paragraphs printed flush, exit 0.
