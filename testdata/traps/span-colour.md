---
title: "Trap: a colour pandoc would drop"
mdbrand: {brand: none, style: note}
---

pandoc ignores a span's colour attributes in LaTeX and in a .docx alike, so
these words would print black and the build would exit 0. It must stop and
name the accent, which both outputs print.

Some [orange words]{color=#c2410c} in the middle of a sentence.
