# LWW por campo — Diseño

Estado: implementado (`feature/lww-por-campo`, no mergeado a `develop`/`main` todavía).

## Almacenamiento del timestamp por campo

Se evaluaron dos formas:

1. **Columna por campo** (`title_updated_at`, `status_updated_at`,
   ...): descartada — duplica el número de columnas en tablas que ya
   tienen hasta 10 campos LWW (ver `overtime_entries`), y cada campo
   nuevo requeriría dos migraciones (dato + timestamp) en vez de una.
2. **Columna JSON `field_updated_at`** (mapa `{"title": "<rfc3339>",
   "status": "<rfc3339>", ...}`): **elegida**. Consistente con el
   patrón ya usado para `tags` (JSON serializado en `TEXT`, ver
   `esquema-datos/design.md`), portable entre SQLite/Postgres sin
   tipos nativos de JSON, y no requiere migración por cada campo nuevo
   que se agregue a una entidad.

`field_updated_at` conserva únicamente los campos que ya recibieron al
menos una escritura explícita — un campo ausente en el mapa se trata
como "timestamp `-infinito`" (cualquier escritura entrante lo gana),
que es el estado natural de una fila recién creada por `POST` con
`field_updated_at` vacío o `{}`.

`updated_at` (columna existente, no-JSON) se mantiene como bookkeeping
de fila para `seq`/sync — ver "Interacción con `seq`" más abajo. No se
elimina, cambia de significado: en vez de ser el timestamp que se
compara para LWW, pasa a ser "cuándo se tocó la fila por última vez
para cualquier campo", puramente informativo/de cursor.

## Contrato de `PATCH /<entidad>/:id`

Request:

```json
{
  "updated_at": "2026-08-22T14:03:00Z",
  "title": "Nuevo título",
  "status": "in-progress"
}
```

Cualquier campo del recurso puede aparecer o no. `updated_at` es
obligatorio y aplica a todos los campos presentes en ese mismo
payload — una sola acción de usuario, un solo instante, aunque toque
varios campos a la vez (ej. un formulario que guarda título + tags
juntos).

Procesamiento server-side, por campo `f` presente en el payload:

```
si payload.updated_at > field_updated_at[f] (o field_updated_at[f] no existe):
    fila[f] = payload[f]
    field_updated_at[f] = payload.updated_at
    huboCambio = true
// si no, se descarta silenciosamente ese campo puntual
```

Al final:
- si `huboCambio`: `seq = next_seq()`, `updated_at = now()`, se
  notifica por WS/`/sync/changes` como cualquier otra escritura.
- si no `huboCambio`: no se toca `seq` ni `updated_at` de fila, no hay
  notificación — pero la respuesta HTTP igual es 200 con el estado
  actual (ver requirements.md, "Respuesta").

Response (200, siempre, mismo shape que `GET`/`POST` de la entidad):

```json
{
  "id": "...",
  "title": "Nuevo título",
  "status": "in-progress",
  "...resto de campos...": "...",
  "seq": 481,
  "updated_at": "2026-08-22T14:03:00Z"
}
```

No se expone `field_updated_at` en la response pública — es un detalle
interno del servidor para resolver conflictos, no algo que el cliente
necesite interpretar (refuerza el requisito de que el cliente solo
sobreescribe con la fila completa, sin lógica propia de merge).

## Interacción con `seq` / `/sync/changes`

`/sync/changes` sigue devolviendo la fila completa por cada entidad
modificada (sin cambios respecto al protocolo actual de
`sync-incremental`) — un `PATCH` parcial en el servidor no implica un
payload parcial en el feed de cambios. Un dispositivo que estaba
offline y se perdió varios `PATCH` intermedios de otro dispositivo
solo ve el resultado final ya mergeado, nunca la secuencia de parches
— coherente con que el servidor, no el cliente, es quien resuelve.

## Migración del esquema existente

Cada tabla con campos LWW (`tasks`, `notes` [metadata, no
`content_crdt`], `overtime_entries`, `overtime_month_meta`,
`calendar_events`, `absence_days`) agrega una columna
`field_updated_at TEXT NOT NULL DEFAULT '{}'`. Filas existentes quedan
con el mapa vacío — la primera escritura por campo que llegue después
del deploy gana sin comparación (comportamiento correcto: no hay
timestamp previo real que se pueda inferir por campo individual a
partir de un único `updated_at` de fila).

`daily_entries` no se toca — no tiene campos LWW, es 100% CRDT (ver
`esquema-datos/design.md`).

## Fuera de este diseño

- Compatibilidad hacia atrás con `PUT` de fila completa (se elimina,
  no se mantiene en paralelo — ver `requirements.md`, no hay razón
  para dos mecanismos de escritura conviviendo cuando el único cliente
  de referencia se migra en el mismo ciclo).
- Índice o consulta especial sobre `field_updated_at` — se lee/escribe
  siempre junto con la fila completa, nunca se filtra por él.
