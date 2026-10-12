---
title: "References in the front matter"
references:
  - id: knuth1984
    type: book
    author:
      - family: Knuth
        given: Donald E.
    title: The TeXbook
    publisher: Addison-Wesley
    issued:
      date-parts: [[1984]]
mdbrand:
  brand: none
  style: note
---

<!-- A fixture. With no bibliography: file, citeproc never ran and the
     citation below printed as "[@knuth1984, p. 3]"; entries written in the
     front matter are a source as well. -->

The argument rests on Knuth [@knuth1984, p. 3].
