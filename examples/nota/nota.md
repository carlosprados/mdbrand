---
title: "Migración del planificador a la versión 2"
author: "Equipo de Plataforma"
date: "9 de octubre de 2026"
lang: es-ES
mdbrand:
  brand: none
  style: note                     # sin portada: título arriba y cabecera desde la página 1
  confidential: "Uso interno"
  formats: [pdf, docx]            # el PDF y un .docx para quien quiera comentarlo en Word o Google Docs
---

Una nota interna: sin portada, el título arriba y la cabecera desde la primera
página. Se construye con `mdbrand build nota.md`, que escribe `nota.pdf` y,
por `formats: [pdf, docx]`, también `nota.docx` con la misma marca.

# Qué cambia

- El planificador lee las ventanas de ejecución de la base de datos, no del
  fichero de configuración.
- Los trabajos que fallan tres veces pasan a una cola de revisión.
- La API conserva sus rutas; solo cambia la respuesta de `GET /jobs/{id}`.

```d2 caption="Ruta de un trabajo en la versión 2"
direction: right
api: API
cola: Cola
plan: Planificador
rev: Revisión
api -> cola -> plan
plan -> rev: 3 fallos
```

# Calendario

| Fase | Fecha | Responsable |
|---|---|---|
| Pruebas en preproducción | 14 de octubre | Plataforma |
| Despliegue gradual | 21 de octubre | Operaciones |
| Retirada de la versión 1 | 4 de noviembre | Plataforma |

Para subirlo a Google Docs como documento editable:
`gog drive upload nota.docx --convert-to doc`.
