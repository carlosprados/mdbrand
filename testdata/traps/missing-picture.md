---
title: "Trap: a picture that is not where the document says"
mdbrand: {brand: none, style: note}
---

A relative picture path resolves against the document. This one points at
nothing, and the build must stop naming it — not hand LaTeX a path that fails
later with a message about xelatex, or finds some other file of the same name.

![Nowhere](nowhere/missing.png)
