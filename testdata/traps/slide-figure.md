---
title: "Figure"
mdbrand:
  style: slides
---

<!-- A tall chain fits a page and not a 16:9 frame: at the frame's 48mm its
     labels fall under the slide's 8.5pt floor. -->

## A pipeline

```d2 caption="Stages"
direction: down
a: Ingest
b: Validate
c: Transform
d: Store
e: Publish
a -> b -> c -> d -> e
```
