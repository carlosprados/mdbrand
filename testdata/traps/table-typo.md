---
title: "Trap: a table filtered on a field that does not exist"
mdbrand: {brand: none, style: note, data: [../data]}
---

A misspelt field in `where` matches no record. Treated as a filter, it would
print an empty table, or — with a default — every machine in the catalogue.
The build must stop and list the fields there are.

```table
source: maquinas
where: {famila: m5}
```
