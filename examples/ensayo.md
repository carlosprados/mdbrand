---
title: "¿Puede una máquina leer un plano?"
subtitle: "Monografía · {{words}} palabras"
author: "mdbrand"
date: "Septiembre de 2026"
lang: es-ES
mdbrand:
  brand: none
  style: report
---

# Introducción

Este documento es el segundo ejemplo de mdbrand: un ensayo con un límite de
palabras, como la Monografía del Bachillerato Internacional. La cifra de la
portada no se ha escrito a mano: la pone `{{words}}` en el subtítulo, y cada
compilación la vuelve a contar.[^criterio]

Constrúyelo con:

```sh
mdbrand build examples/ensayo.md --watch
```

Con `--watch`, cada vez que guardes el fichero se vuelve a generar el PDF, y la
cifra de la portada cambia con el texto.

# Desarrollo

Un plano técnico no es una fotografía. Es un lenguaje con su gramática: cotas,
vistas, secciones y símbolos normalizados que un ingeniero lee de un vistazo y
que una máquina tiene que aprender a interpretar.

> Un dibujo es un argumento: dice qué se va a construir y por qué así.

Lo que cuenta el criterio del IB, que es el que aplica mdbrand por defecto, es
la prosa: párrafos, listas, títulos, citas textuales y notas con contenido.

- Cuentan los párrafos y los títulos.
- Cuentan las citas textuales como la de arriba.
- No cuentan las tablas, las figuras ni el código.

| Elemento | ¿Cuenta? |
|---|---|
| Esta tabla | No |
| Su pie | Tampoco |

# Conclusión

Si el texto crece, la portada lo dice sin que nadie tenga que recontar. Si el
límite es de 4.000 palabras, conviene dejar margen: dos procesadores de texto
nunca cuentan exactamente igual.

# Apéndice {.nocount}

Todo lo que cuelga de un título marcado `{.nocount}` queda fuera del recuento:
apéndices, agradecimientos o un resumen con su propio límite. Estas líneas no
suman nada a la cifra de la portada.

[^criterio]: El criterio se cambia en el front matter, con `wordcount: all` o
    con `wordcount: {base: ib, include: [tables]}`.
