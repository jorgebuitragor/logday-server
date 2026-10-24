# Web app embebida — Requirements

Estado: implementado — servir el bundle bajo `/app` funciona
end-to-end (validado con Playwright headless contra un contenedor
Docker real, login incluido). El repo `logday-web` en sí (Tasks +
Notes y el resto de entidades) es trabajo aparte, en progreso.

## Contexto

[`logday-web`](https://github.com/jorgebuitragor/logday-web) es un
cliente de sync para navegador — a diferencia de Logday Desktop
(Tauri, evita CORS haciendo las requests desde Rust), un cliente web
de verdad corre en el navegador y sí choca con que este servidor no
implementa CORS a propósito (ver `arquitectura-inicial/requirements.md`).

Se evaluaron dos formas de resolverlo: CORS configurable en el
servidor (rompe "cero configuración obligatoria"), o servir
`logday-web` desde el mismo origen que la API, igual que ya se hace
con el panel de administración (`/admin/panel`). Se eligió la
segunda — mismo criterio de simplicidad, sin superficie nueva de
configuración.

## Requisitos (EARS)

- El sistema DEBERÁ servir el build estático de `logday-web` bajo
  `/app`, embebido en el mismo binario (mismo patrón que el panel de
  administración: un solo contenedor, sin dependencias externas).
- Cualquier ruta bajo `/app` que no corresponda a un archivo real del
  build DEBERÁ devolver `index.html` (fallback de SPA), para que el
  ruteo del lado cliente funcione en un deep link o refresh duro.
- El mount de `/app` NO DEBERÁ interferir con ninguna ruta de la API
  de sync (`/tasks`, `/notes`, `/sync/changes`, etc.) ni con
  `/admin/panel` — son namespaces distintos.
- El sistema NO DEBERÁ requerir que `go build`/`go test` tengan el
  bundle real de `logday-web` disponible — debe compilar y correr
  localmente con un placeholder committeado, ya que `//go:embed`
  exige que el directorio exista y no esté vacío.
- `logday-web` es un repo privado aparte — el sistema NO DEBERÁ
  requerir clonarlo anónimamente durante el build. El build de Docker
  lo toma de un checkout local hermano (`../logday-web`) vía build
  context adicional, no de un `git clone` remoto.

## Fuera de este spec

- Contenido de `logday-web` en sí (qué entidades soporta, su UI) —
  vive en ese repo.
- CORS real para un deploy de `logday-web` separado del servidor —
  descartado como enfoque principal, no se implementa acá.
- Un pipeline de CI que resuelva el checkout hermano de forma
  automatizada (hoy es manual, ver README) — pendiente si en algún
  momento se automatizan releases.
