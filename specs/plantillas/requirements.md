# Plantillas por empresa — Requirements

Estado: en progreso (v1).

## Alcance de la v1 (decidido por Jorge el 1 nov 2026)

Primero va una versión acotada en los tres clientes, sin servidor:

- **Qué entra.** En Opciones → Plantillas se elige:
  - para el Daily, entre la de Logday (predeterminada) y una propia, que se
    edita como texto;
  - para el Excel, entre Legacy (predeterminada), la de Logday y una propia,
    que se carga como `.xlsx`.
- **Sin validaciones** al cargar un `.xlsx` (P18 queda para después). Si
  el archivo no sirve, el error aparece al exportar.
- **Sin sync.** La elección y la plantilla propia se guardan solo en el
  equipo (P23–P26 quedan para después), así que no cambia el servidor ni el
  contrato.
- **Lo de siempre no pasa por el motor.** El mensaje de Logday sigue saliendo
  de `buildDailyCopyText` y el Excel Legacy de `generateOvertimeXlsx`, tal
  como hoy, así que P4 se cumple sin archivo ni test de equivalencia del
  Excel. El punto de partida para un `.xlsx` propio es la de Logday.
- **No se nombra a ninguna empresa** en la app, el código, los textos ni este
  spec (pedido de Jorge).

Lo demás de este documento describe la versión completa.

## Contexto

Dos salidas de Logday tienen hoy un formato fijo en el código de los tres
clientes (Desktop, Web y Mobile), el de la empresa donde nació Logday:

- **El mensaje del Daily** que se copia o se envía al chat ("Buenos días.
  El día…", `buildDailyCopyText` en Desktop y Web, `dailyCopyText.ts` en
  Mobile).
- **El Excel de horas extra** ("REPORTE HORAS EXTRAS", "NOMBRE
  COLABORADOR", "CEDULA", la leyenda diurna y nocturna), armado celda por
  celda en `overtimeExcel.ts`, portado igual en los tres clientes.

Logday sirve para cualquier empresa, así que estas dos salidas pasan a ser
**plantillas**:

- **Lo de siempre sigue siendo lo predeterminado.** El mensaje de siempre
  pasa a ser la plantilla del Daily **de Logday**, y el Excel de siempre se
  llama **Legacy**. Al actualizar no cambia nada para nadie: no hay
  migración de datos, ni en los clientes ni en el servidor.
- Para el Excel se agrega una **plantilla genérica de Logday**.
- Cada persona puede **cargar la plantilla de su empresa**: un texto para el
  Daily y un `.xlsx` con marcadores para las horas extra.

Una plantilla es un dato de la cuenta, no un gusto del equipo: en la
versión completa se sincroniza entre los equipos del usuario, a diferencia
de las preferencias de UI (`Preferencias de UI siempre locales` en el vault).

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
- P2. Cada cliente DEBERÁ incluir:
  - para el Daily, la plantilla **Logday** (el mensaje de siempre);
  - para el Excel, **Legacy** (el de siempre) y **Logday** (genérica).

  Se identifican por una clave (`logday`, `legacy`), no por su contenido.
- P3. Cuando un usuario no tiene una plantilla activa de un tipo, el
  cliente DEBERÁ usar la de siempre: Logday en el Daily y Legacy en el
  Excel.
- P4. Lo de siempre DEBERÁ salir idéntico a hoy:
  - el Excel Legacy, el mismo libro que genera `generateOvertimeXlsx`
    (valores, fórmulas, estilos, celdas combinadas, anchos y altos);
  - el Daily de Logday, el mismo texto que `buildDailyCopyText`, carácter
    por carácter.
- P5. El cliente DEBERÁ dejar elegir:
  - en el Daily, entre la de Logday y una propia;
  - en el Excel, entre Legacy, la de Logday y una cargada por el usuario.

### Actualizar sin cambios

- P6. Al actualizar a una versión con plantillas, lo que genera Logday NO
  DEBERÁ cambiar para nadie: sin plantilla activa se usa la de siempre (P3),
  así que no hace falta migrar datos ni en los clientes ni en el servidor.
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
- P21. El cliente DEBERÁ ofrecer descargar la de Logday como punto de
  partida, y la cargada, para pasársela a un compañero.
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
  si una empresa usa su propio servidor de Logday. Propuesta:
  dejarlo para una v2. En v1 se pasa el archivo con "Descargar
  la cargada".
- **Cambiar el Excel predeterminado a Logday más adelante.** Decidido por
  Jorge el 1 nov 2026: en esta versión lo predeterminado sigue siendo lo de
  siempre, Legacy en el Excel y el mensaje de siempre en el Daily. Si Logday
  llega a usuarios de otras empresas, se puede hacer que solo las
  instalaciones nuevas arranquen con la de Logday, con una migración en
  cada cliente para que las de antes sigan con Legacy.
