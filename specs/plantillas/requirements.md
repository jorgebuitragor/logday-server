# Plantillas por empresa — Requirements

Estado: en progreso (v1).

## Alcance de la v1 (decidido por Jorge el 1 nov 2026)

Primero va una versión acotada en los tres clientes, sin servidor:

- **Qué entra.** En Opciones → Plantillas se elige, para el Daily y para el
  Excel, entre FCV (la predeterminada), Logday y una propia: el texto del
  Daily se edita y el `.xlsx` se carga.
- **Sin validaciones** al cargar un `.xlsx` (P18 queda para después). Si
  el archivo no sirve, el error aparece al exportar.
- **Sin sync.** La elección y la plantilla propia se guardan solo en el
  equipo (P23–P26 quedan para después), así que no cambia el servidor ni el
  contrato.
- **La FCV no pasa por el motor.** Sigue saliendo de
  `generateOvertimeXlsx` y `buildDailyCopyText`, tal como hoy, así que P4 se
  cumple sin archivo FCV ni test de equivalencia. No hay "Descargar la FCV":
  el punto de partida para una propia es la de Logday.

Lo demás de este documento describe la versión completa.

## Contexto

Dos salidas de Logday tienen hoy el formato de la Fundación Cardiovascular
de Colombia (FCV) fijo en el código de los tres clientes (Desktop, Web y
Mobile):

- **El mensaje del Daily** que se copia o se envía al chat ("Buenos días.
  El día…", `buildDailyCopyText` en Desktop y Web, `dailyCopyText.ts` en
  Mobile).
- **El Excel de horas extra** ("REPORTE HORAS EXTRAS", "NOMBRE
  COLABORADOR", "CEDULA", la leyenda diurna y nocturna), armado celda por
  celda en `overtimeExcel.ts`, portado igual en los tres clientes.

Logday sirve para cualquier empresa, así que estas dos salidas pasan a ser
**plantillas**:

- **La plantilla de hoy (FCV) sigue siendo la predeterminada.** Al
  actualizar no cambia nada para nadie: no hay migración de datos, ni en los
  clientes ni en el servidor.
- Se agrega una **plantilla genérica de Logday**, para quien la prefiera.
- Cada persona puede **cargar la plantilla de su empresa**: un texto para el
  Daily y un `.xlsx` con marcadores para las horas extra.

Una plantilla es un dato de la cuenta, no un gusto del equipo: se
sincroniza entre los equipos del usuario, a diferencia de las preferencias
de UI (`Preferencias de UI siempre locales` en el vault).

Fuera de alcance (v1):

- Varias plantillas guardadas por tipo. Hay una sola activa por tipo; se
  cambia cargando otra.
- Plantillas compartidas por empresa o por instancia. Ver "Pendiente de
  decisión".
- Plantillas para otras salidas (exportar Dailys del mes en PDF, notas).
- Editar el `.xlsx` dentro de Logday: se edita en Excel o LibreOffice y se
  vuelve a cargar.

## Requisitos (EARS)

### Tipos y plantillas incluidas

- P1. El sistema DEBERÁ manejar dos tipos de plantilla: `daily_message`
  (texto) y `overtime_xlsx` (archivo `.xlsx`).
- P2. Cada cliente DEBERÁ incluir dos plantillas de cada tipo: **Logday**
  (genérica) y **FCV** (la de hoy). Se identifican por una clave
  (`logday`, `fcv`), no por su contenido.
- P3. Cuando un usuario no tiene una plantilla activa de un tipo, el
  cliente DEBERÁ usar la FCV, que es la de hoy.
- P4. La plantilla FCV del Excel, aplicada a los mismos datos, DEBERÁ dar
  el mismo libro que genera hoy `generateOvertimeXlsx`: valores, fórmulas,
  estilos, celdas combinadas, anchos y altos. La FCV del Daily DEBERÁ dar el
  mismo texto que `buildDailyCopyText`, carácter por carácter.
- P5. Para cada tipo, el cliente DEBERÁ dejar elegir entre las dos
  incluidas (FCV y Logday) y una cargada por el usuario.

### Actualizar sin cambios

- P6. Al actualizar a una versión con plantillas, lo que genera Logday NO
  DEBERÁ cambiar para nadie: sin plantilla activa se usa la FCV (P3), así
  que no hace falta migrar datos ni en los clientes ni en el servidor.
- P7–P10. Quitados en la revisión del 1 nov 2026, junto con la migración.

### Plantilla del Daily (`daily_message`)

- P11. La plantilla del Daily DEBERÁ ser texto con marcadores
  `{{nombre}}` y secciones condicionales `{{#nombre}}…{{/nombre}}` y
  `{{^nombre}}…{{/nombre}}` (subconjunto de Mustache, sin HTML ni código).
- P12. El sistema DEBERÁ ofrecer al menos estos marcadores: la fecha del
  día anterior laborable y sus actividades, la fecha del día y sus
  actividades, si ese día es hoy, y el nombre del usuario.
- P13. Copiar el Daily, "Enviar al chat" y el webhook saliente DEBERÁN usar
  la plantilla activa. Si en el futuro el servidor arma el mensaje (modo
  cuenta de `daily-al-chat`), DEBERÁ usar la misma plantilla y el mismo
  motor.
- P14. Cuando el usuario edita la plantilla, el cliente DEBERÁ mostrar una
  vista previa con el Daily de hoy y señalar los marcadores que no
  existen. Un marcador desconocido NO DEBERÁ impedir guardar: se deja
  vacío y se avisa.
- P15. El texto de la plantilla NO DEBERÁ pasar de 10 000 caracteres.

### Plantilla del Excel (`overtime_xlsx`)

- P16. La plantilla del Excel DEBERÁ ser un `.xlsx` normal con marcadores
  en el texto de las celdas: valores sueltos (`{{colaborador}}`,
  `{{cedula}}`, `{{mes}}`, los totales…) y una **fila de datos** que se
  repite una vez por registro del mes.
- P17. Al generar el Excel, el sistema DEBERÁ conservar todo lo demás del
  libro tal cual: estilos, logos, celdas combinadas, anchos, otras hojas y
  fórmulas. Las fórmulas y las celdas combinadas que quedan debajo de la
  fila de datos, o que la abarcan, DEBERÁN ajustarse a las filas
  insertadas.
- P18. Cuando el usuario carga un `.xlsx`, el cliente DEBERÁ validarlo
  antes de guardarlo y rechazarlo, diciendo por qué, si:
  - no es un `.xlsx` válido, o es `.xlsm` o `.xls`;
  - pesa más de 1 MB, o más de 20 MB descomprimido;
  - no tiene ninguna fila de datos, o tiene más de una;
  - tiene tablas de Excel, tablas dinámicas o macros en la hoja de la fila
    de datos.
- P19. Un marcador desconocido NO DEBERÁ impedir guardar: se deja la celda
  vacía y se avisa al cargarla.
- P20. Al cargar una plantilla, el cliente DEBERÁ mostrar qué marcadores
  encontró y ofrecer "Descargar un ejemplo" generado con los datos del mes
  actual antes de activarla.
- P21. El cliente DEBERÁ ofrecer descargar cualquiera de las incluidas
  (FCV o Logday) como punto de partida, y "Descargar la plantilla actual"
  (la activa, sea cual sea), para pasársela a un compañero.
- P22. Elegir una plantilla incluida cuando hay una cargada DEBERÁ
  reemplazar la cargada. Antes, el cliente DEBERÁ ofrecer descargarla,
  porque no se guarda otra copia.

### Sync

- P23. La plantilla activa de cada tipo DEBERÁ sincronizarse como una
  entidad nueva, `template`, con clave natural `(user_id, kind)` (sin id
  del cliente, como `overtime_month_meta`). Los conflictos se resuelven con
  LWW por campo, como en el resto de las entidades (`lww-por-campo/`).
- P24. Si el servidor todavía no conoce `template` (responde 404 a su
  ruta), el cliente DEBERÁ guardar la plantilla solo en el equipo y subirla
  cuando el servidor la admita, sin mostrar error.
- P25. Un cliente que recibe en `/sync/changes` un tipo de entidad que no
  conoce DEBERÁ ignorarlo sin fallar (vale también para versiones viejas de
  los clientes).
- P26. El servidor DEBERÁ rechazar con 413 una plantilla cuyo contenido
  pase de 1,5 MB (el `.xlsx` de 1 MB codificado en base64) y con 400 un
  `kind` o un `source` desconocidos. El servidor NO DEBERÁ abrir ni
  interpretar el `.xlsx`.

### Privacidad y seguridad

- P27. Generar el Excel y el mensaje DEBERÁ hacerse en el equipo, sin red.
- P28. El motor NO DEBERÁ evaluar fórmulas, ejecutar macros ni seguir
  vínculos externos de la plantilla: copia lo que hay y llena los
  marcadores. Los valores que vienen del usuario se escriben como texto o
  número, nunca como fórmula.
- P29. `GET /account/export` DEBERÁ incluir las plantillas del usuario, y
  borrar la cuenta DEBERÁ borrarlas.

## Pendiente de decisión

- **Plantilla por instancia.** Que el admin de un servidor publique una
  plantilla que reciben los usuarios nuevos de esa instancia. Solo sirve
  si la FCV llega a usar un servidor propio, que hoy no usa. Propuesta:
  dejarlo para una v2. En v1 se pasa el archivo con "Descargar
  la plantilla actual".
- **Cambiar la predeterminada a Logday más adelante.** Si Logday llega a
  usuarios de otras empresas, se puede hacer que solo las instalaciones
  nuevas arranquen con la de Logday. Necesitaría una migración en cada
  cliente para que las instalaciones de antes sigan con la FCV. Queda fuera
  de v1.
