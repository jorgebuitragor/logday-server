# Logday Server

API de sync self-hosted para [Logday](https://github.com/jorgebuitragor/logday),
al estilo Vaultwarden/Bitwarden: se despliega con un único contenedor
Docker, y cada cliente (desktop, web, móvil, extensión) se configura para
apuntar a su propio host.

El sync es **opcional**: cualquier cliente de Logday sigue siendo
100% funcional sin conexión a un servidor. Este proyecto existe para
quienes eligen sincronizar sus datos entre dispositivos o con su equipo,
manteniendo el control total sobre dónde viven esos datos.

## Estado del proyecto

En diseño — todavía no hay código. Ver [`specs/`](./specs/README.md)
para el proceso de spec-driven development y el estado actual de cada
decisión de arquitectura.

## Stack (propuesto, ver spec para detalle y justificación)

- Go (`net/http` + `chi`)
- SQLite por defecto, Postgres intercambiable
- Docker multi-stage build
