# Auth multi-usuario — Tareas

Estado: implementado (v1) — ver `design.md` y `requirements.md`.

- [x] Decidir algoritmo de hash de password (argon2id).
- [x] Decidir modelo de tokens (JWT access de 15 min + refresh
      rotativo de 30 días, por dispositivo).
- [x] Decidir si hay revocación manual de dispositivos (sí, desde v1).
- [x] Decidir modelo de registro (invite-only, admin crea usuarios).
- [x] Definir esquema exacto de columnas/tipos de `users` y `devices`.
      Ver migraciones en `internal/db/migrations/`. Se agregó además
      `used_refresh_tokens` (no estaba en el diseño original) para
      poder distinguir "token inválido" de "token ya rotado" y así
      implementar la detección de reuso/robo.
- [x] Decidir mecanismo de bootstrap del primer admin: variables de
      entorno `ADMIN_EMAIL`/`ADMIN_PASSWORD`, leídas una sola vez si la
      tabla `users` está vacía. Implementado en `internal/auth/bootstrap.go`.
- [x] Decidir si hay recuperación de password en v1: no, diferido a v2
      (requiere SMTP, no decidido).
- [x] Decidir si hay rate limiting de login: sí, básico en v1 — límite
      en memoria de 5 intentos/minuto por IP+email, en
      `internal/auth/ratelimit.go`. Válido para el target de despliegue
      (instancia única, no multi-réplica).
- [x] Implementar el flujo completo: login, refresh con rotación y
      detección de reuso, `/devices` (listar/revocar), `/admin/users`.
      Validado end-to-end contra un contenedor real (curl): login,
      refresh rota el token, reintentar el token viejo revoca toda la
      sesión del dispositivo (incluido el token nuevo ya emitido), y el
      6to intento de login fallido en un minuto responde 429.
