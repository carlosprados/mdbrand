---
title: "Trap: a data key that is not there"
mdbrand: {brand: none, style: note, data: [../data]}
---

A template engine prints nothing for a key it cannot find, and the sentence
reaches the client with a hole in it. The build must stop and list the ids
that do exist.

La m5.larg tiene {{data.maquinas[m5.larg].cpu}} vCPU.
