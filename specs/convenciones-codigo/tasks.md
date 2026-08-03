# Convenciones de código — Tareas

Estado: en diseño

- [x] Decidir organización de paquetes (vertical por dominio bajo
      `internal/`).
- [x] Configurar `golangci-lint` con preset reforzado
      (`standard` + `gosec` + `revive`) en `.golangci.yml`.
- [x] Agregar `Makefile` con targets `build`, `run`, `test`, `lint`,
      `fmt`.
- [x] Agregar CI en GitHub Actions (`.github/workflows/ci.yml`) con
      jobs de lint y build+test.
- [x] Corregir los issues que el linter encontró en el scaffold
      existente (errcheck, gosec: timeouts HTTP y permisos de
      directorio).
- [x] Documentar convención de nombres de archivo dentro de un paquete
      de dominio (`models.go`, `store.go`, `handlers.go`, etc.).
- [x] Extraer primitivas genéricas de `internal/auth` a
      `internal/security` (hash de password, JWT, tokens opacos) —
      aplicado retroactivamente sobre auth, primer caso real del
      criterio "sin conocimiento del dominio → security".
- [ ] Decidir convenciones de logging y manejo de errores (diferido a
      la primera feature real).
