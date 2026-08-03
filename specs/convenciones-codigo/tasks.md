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
- [ ] Decidir convenciones de logging y manejo de errores (diferido a
      la primera feature real).
