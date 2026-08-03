# Auth multi-usuario — Tareas

Estado: en diseño

- [x] Decidir algoritmo de hash de password (argon2id).
- [x] Decidir modelo de tokens (JWT access de 15 min + refresh
      rotativo de 30 días, por dispositivo).
- [x] Decidir si hay revocación manual de dispositivos (sí, desde v1).
- [x] Decidir modelo de registro (invite-only, admin crea usuarios).
- [ ] Definir esquema exacto de columnas/tipos de `users` y `devices`.
- [ ] Decidir mecanismo de bootstrap del primer admin (env var vs.
      wizard).
- [ ] Decidir si hay recuperación de password en v1 (depende de si se
      configura SMTP).
- [ ] Decidir si hay rate limiting de login y de qué tipo.
