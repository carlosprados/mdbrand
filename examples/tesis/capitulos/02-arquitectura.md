# Arquitectura

Como se dijo en el capítulo [-@sec:intro], cada agente decide con lo que ve.
La [@fig:arquitectura] dibuja un agente y su colectivo: un diagrama vertical,
que se lee a su tamaño en una página A4.

```d2 #fig:arquitectura caption="Un agente y su colectivo"
direction: down
sensores: Sensores
agente: Agente {
  percepcion: Percepción
  decision: Decisión
  percepcion -> decision
}
colectivo: Colectivo
sensores -> agente.percepcion
agente.decision -> colectivo: acción
```

El anexo [-@sec:entrevistas] recoge las entrevistas con operadores.
