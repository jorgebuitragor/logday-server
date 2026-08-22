# Flujo de trabajo (Gitflow)

Este repo usa [Gitflow](https://nvie.com/posts/a-successful-git-branching-model/)
para versionar el proyecto, incluso siendo desarrollo de una sola persona:
mantiene separado el código en producción del trabajo en curso y deja un
historial de releases claro (tags en `main`).

## Ramas permanentes

- **`main`** — solo código que corresponde a una release. Cada merge a
  `main` se tagea con `vX.Y.Z` ([SemVer](https://semver.org/lang/es/)).
  Es la rama por defecto del repo en GitHub.
- **`develop`** — rama de integración. Todo el trabajo en curso vive acá
  antes de convertirse en una release.

## Ramas de trabajo

| Tipo | Sale de | Entra a | Naming |
|---|---|---|---|
| `feature/<slug>` | `develop` | `develop` | `feature/panel-reset-password` |
| `release/<version>` | `develop` | `main` + `develop` | `release/1.2.0` |
| `hotfix/<slug>` | `main` | `main` + `develop` | `hotfix/jwt-expiry-bug` |

- **Feature**: cualquier trabajo nuevo (feature, fix no urgente, chore).
  Se crea desde `develop`, se mergea de vuelta a `develop` cuando está
  lista. Se borra después del merge.
- **Release**: cuando `develop` tiene suficiente para una nueva versión,
  se crea `release/<version>` para estabilizar (fixes menores, changelog,
  bump de versión — sin features nuevas). Se mergea a `main` (con tag
  `vX.Y.Z`) y de vuelta a `develop`.
- **Hotfix**: bug urgente en producción. Sale directo de `main`, se
  mergea a `main` (con tag de patch) y a `develop`, para que el fix no se
  pierda en la siguiente release.

## Flujo típico

```bash
git checkout develop
git checkout -b feature/mi-feature
# ...trabajo, commits...
git checkout develop
git merge --no-ff feature/mi-feature
git branch -d feature/mi-feature
git push origin develop
```

Al cortar una release:

```bash
git checkout develop
git checkout -b release/1.2.0
# ...ajustes finales...
git checkout main
git merge --no-ff release/1.2.0
git tag -a v1.2.0 -m "v1.2.0"
git checkout develop
git merge --no-ff release/1.2.0
git branch -d release/1.2.0
git push origin main develop --tags
```

`--no-ff` es intencional: preserva la rama como un merge commit visible en
el historial, en vez de aplanarla con fast-forward.
