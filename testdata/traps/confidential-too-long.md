---
title: "Trap: a confidentiality label longer than its footer"
mdbrand:
  brand: none
  style: note
  confidential: "Confidencial — uso exclusivo del destinatario, prohibida su difusión total o parcial"
---

The label repeats in every footer, left of the centred page number, and
fancyhdr sets the two without measuring either: at this length they overprint
on every page and the build would exit 0. It must stop and say how far over.
