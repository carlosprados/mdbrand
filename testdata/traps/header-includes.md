---
title: "Trap: front matter that would evict the design"
header-includes: |
  \usepackage{lmodern}
mdbrand: {brand: none, style: note}
---

mdbrand owns the preamble. pandoc's --include-in-header replaces this field
rather than adding to it, so the document would build without the brand and
nothing would say why. It must be refused by name.
