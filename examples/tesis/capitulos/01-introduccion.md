# Introducción {#sec:intro}

Este es el ejemplo de tesis de mdbrand. Las tres páginas fijas, la
numeración romana de las páginas iniciales y la arábiga desde aquí, el índice
y las listas los pone la plantilla; el texto solo escribe los capítulos.

Constrúyelo con `mdbrand build tesis.md`, y añade `--to pdf,docx` para la
copia en Word que comenta el director.

## Objetivo

Un colectivo de agentes maximiza la recompensa acumulada
[@wooldridge2009], que la [@eq:objetivo] escribe:

$$ J = \sum_{t} r_t $$ {#eq:objetivo}

La [@tbl:partes] resume qué numera cada parte de la tesis.

| Parte              | Numeración |
|--------------------|------------|
| Cubierta y portada | ninguna    |
| Páginas iniciales  | romana     |
| Capítulos          | arábiga    |
| Anexos             | letras     |

: Numeración de las partes de una tesis {#tbl:partes}
