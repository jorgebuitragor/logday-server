# Specs (SDD)

Este directorio contiene specs de features siguiendo un flujo ligero de
Spec-Driven Development (SDD), sin herramientas externas. Mismas reglas
que en el repo de Logday Desktop (`task-manager`).

## Estructura

Cada feature vive en su propia carpeta: `specs/<feature-slug>/`, con hasta
tres archivos:

- **`requirements.md`** — qué debe hacer la feature, en formato EARS
  ("Cuando X, el sistema DEBERÁ Y"). Es el contrato: si un comportamiento no
  está aquí, no se considera parte de la feature.
- **`design.md`** — cómo se implementa: componentes, esquema de datos,
  contratos entre la API y los clientes, decisiones y sus alternativas
  descartadas.
- **`tasks.md`** — checklist de implementación, casilla por casilla,
  referenciando los requirements que cada tarea satisface.

## Convenciones

- Los specs de features **ya existentes** documentan el comportamiento actual
  como baseline (reverse-spec) — no son aspiracionales. Se marcan con
  `Estado: implementado (baseline)` al inicio de `requirements.md`.
- Los specs de features **nuevas** se escriben antes de tocar código y guían
  la implementación. Se marcan `Estado: en diseño` o `Estado: en progreso`.
- Al modificar una feature ya especificada: actualiza el spec en el mismo PR
  que el código. Un spec desactualizado es peor que no tener spec.
- No se especifica retroactivamente todo el código existente — solo las
  áreas que se van a tocar o que son complejas.
- Toda decisión que aún no esté tomada se marca explícitamente como
  **PENDIENTE DE DECISIÓN** en el spec correspondiente — no se asume ni se
  decide implícitamente por omisión.

## Índice de features

| Feature | Estado | Carpeta |
|---|---|---|
| Arquitectura inicial | en diseño | [`arquitectura-inicial/`](./arquitectura-inicial/requirements.md) |
| Sync incremental | en progreso | [`sync-incremental/`](./sync-incremental/requirements.md) |
| Auth multi-usuario | implementado (v1) | [`auth-multiusuario/`](./auth-multiusuario/requirements.md) |
| Esquema de datos | implementado | [`esquema-datos/`](./esquema-datos/requirements.md) |
| Convenciones de código | en diseño | [`convenciones-codigo/`](./convenciones-codigo/requirements.md) |
| Panel de administración web | implementado | [`panel-admin/`](./panel-admin/requirements.md) |
| LWW por campo | implementado (sin mergear) | [`lww-por-campo/`](./lww-por-campo/requirements.md) |
| Web app embebida | implementado (sin mergear) | [`webapp-embebida/`](./webapp-embebida/requirements.md) |
| Papelera compartida entre servicios | parcial | [`papelera-compartida/`](./papelera-compartida/requirements.md) |
