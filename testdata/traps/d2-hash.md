---
title: "Trap: a # in a d2 label"
mdbrand:
  brand: none
  style: note
---

d2 reads an unquoted # as the start of a comment. "PKCS#11" printed "PKCS",
exit 0; with a brace after it the brace went too, and d2 blamed the closing
one, lines below.

```d2 caption="Custodia de claves"
b: B. HSM con PKCS#11 {
  hsm: CloudHSM
  op: AES-ECB
  hsm -> op
}
```
