---
title: "Trap: a bundle whose body face is not on this machine"
mdbrand: {style: note}
---

Built with `--brand ghost --brands-dir testdata/brands`. fontconfig cannot find
the family the bundle names, so the build must stop and name the bundle, the
family and the two ways out — not let XeLaTeX substitute silently.
