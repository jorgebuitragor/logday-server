# Plantillas por empresa — Diseño

Estado: en diseño. Requisitos en `requirements.md` (P1–P29).

## 1. Modelo y contrato

### Tabla `templates`

Una fila por usuario y tipo: la plantilla **activa**. Si no hay fila, o
está borrada, se usa la de siempre (P3). Misma forma que
`overtime_month_meta`, con clave natural y sin id del cliente.

```sql
CREATE TABLE templates (
    user_id TEXT NOT NULL,
    kind TEXT NOT NULL,          -- 'daily_message' | 'overtime_xlsx'
    source TEXT NOT NULL,        -- 'builtin' | 'custom'
    builtin_key TEXT,            -- 'logday' | 'legacy' si source = 'builtin'
    name TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '', -- texto (daily) o base64 del .xlsx; vacío si builtin
    seq INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    field_updated_at TEXT NOT NULL DEFAULT '{}',
    deleted_at TEXT,
    PRIMARY KEY (user_id, kind)
);
CREATE INDEX idx_templates_user_id_seq ON templates (user_id, seq);
```

- Una plantilla incluida (`builtin`) se guarda por su clave, sin contenido.
  Así la fila es pequeña y una corrección de una incluida llega con la
  versión de la app.
- Un campo para cada cosa, no un JSON: el panel admin puede mostrar qué
  plantilla usa cada uno sin abrir el contenido.
- **LWW por campo con escrituras completas.** El cliente siempre manda
  todos los campos en el mismo `PUT`, con el mismo `updated_at`. Con el
  LWW por campo de `lww-por-campo/`, eso hace que una escritura gane o
  pierda entera y nunca mezcle el `source` de una con el `content` de otra.
  El servidor lo exige: un `PUT` sin todos los campos es 400.

### API

- `GET /templates`: las filas del usuario (como `GET /overtime-month-meta`).
- `PUT /templates/{kind}`: crea o reemplaza, con `updated_at` obligatorio.
  Validaciones (P26): `kind` y `source` conocidos; `builtin_key` presente
  si y solo si `source = builtin`; `content` hasta 1,5 MB (413 si pasa).
  El servidor no decodifica ni abre el base64.
- `DELETE /templates/{kind}`: borrado lógico (vuelve a la predeterminada).
- `/sync/changes` y `/ws` agregan `entity_type: template`, con `kind` como
  id sintético (igual que `year_month`).
- `GET /account/export` incluye `templates` y borrar la cuenta las borra
  (P29). Va en el mismo paquete de dominio: `internal/template/`.

### Sin migración de datos (P6)

La migración del servidor solo crea la tabla. Como lo predeterminado sigue
siendo lo de siempre, ningún cliente ni el servidor tiene que escribir nada
al actualizar.

### Compatibilidad entre versiones

- **Cliente nuevo con servidor viejo:** `PUT /templates/{kind}` responde
  404. El cliente guarda en local, marca "pendiente de subir" y vuelve a
  probar al iniciar sesión o al reconectar (P24). No es un error para el
  usuario.
- **Cliente viejo con servidor nuevo:** recibe `entity_type: template` en
  `/sync/changes`. Hay que verificar que los tres clientes publicados
  ignoren tipos desconocidos (P25). Si alguno falla, el servidor solo
  manda `template` a los clientes que lo pidan
  (`/sync/changes?types=…`); eso se decide al verificar (tarea T1).

## 2. Motor del mensaje del Daily

Un subconjunto de Mustache, implementado a mano, sin dependencias
(`renderDailyTemplate(template, context)`), igual en los tres clientes y,
si llega el modo cuenta de `daily-al-chat`, en Go:

- `{{nombre}}`: valor. Los nombres con punto (`hoy.fecha`) son rutas en el
  contexto.
- `{{#nombre}}…{{/nombre}}`: se muestra si el valor es verdadero.
  `{{^nombre}}…{{/nombre}}` se muestra si es falso. Sin listas ni parciales.
- Un nombre desconocido da texto vacío y se reporta en la vista previa
  (P14). Un error de sintaxis (una sección sin cerrar) se muestra en la
  vista previa, y el mensaje sale con el texto tal cual, sin interpretar.

Contexto (P12):

| Marcador | Valor |
|---|---|
| `anterior.fecha` | "Viernes 31 de Octubre" (día laborable anterior, como hoy) |
| `anterior.actividades` | la lista de ese día, o el texto de "sin actividades" |
| `hoy.fecha` | la fecha del Daily |
| `hoy.actividades` | su lista, o el texto de "sin actividades" |
| `hoy.es_hoy` | verdadero si la fecha del Daily es hoy |
| `usuario.nombre` | el nombre del usuario (o vacío) |

La plantilla de Logday es el mensaje de siempre, y un test exige que dé
exactamente lo de `buildDailyCopyText` (P4). Mientras esté elegida, el
mensaje sigue saliendo de `buildDailyCopyText`; la plantilla escrita sirve
de punto de partida para una propia:

```
Buenos días.

El día {{anterior.fecha}}:
{{anterior.actividades}}

{{#hoy.es_hoy}}El día de hoy, {{hoy.fecha}}:{{/hoy.es_hoy}}{{^hoy.es_hoy}}El día {{hoy.fecha}}:{{/hoy.es_hoy}}
{{hoy.actividades}}
```

El contexto también tiene las fechas en inglés ("Friday, October 31") para
las plantillas propias, según el idioma de la app. Una primera versión de
este spec proponía otra genérica ("Avance del día anterior…"); Jorge la
descartó el 1 nov 2026: la de Logday es la de siempre.

## 3. Motor del Excel

### Por qué no SheetJS

`xlsx-js-style` (lo que se usa hoy) no conserva bien un libro leído:
pierde estilos que no entiende, imágenes y formatos condicionales, y
reescribe el archivo entero. ExcelJS conserva más, pero no ajusta las
fórmulas al insertar filas y pesa mucho en Mobile. Así que el motor
trabaja **sobre el XML del `.xlsx`**: lo abre como zip (JSZip, que Desktop
ya usa, o fflate en Web y Mobile; se decide en T4), cambia solo el texto de
los marcadores y clona la fila de datos. Todo lo demás pasa sin tocar, byte
a byte (P17).

### Marcadores

En cualquier celda de texto (cadena compartida o inline) de cualquier hoja:

| Marcador | Valor | Tipo de celda |
|---|---|---|
| `{{mes}}` | "Noviembre 2026" (el `mesLabel` de hoy) | texto |
| `{{colaborador}}`, `{{cedula}}` | `OvertimeMonthMeta` | texto |
| `{{total.horas}}`, `{{total.diurnas}}`, `{{total.nocturnas}}`, `{{total.diurnas_festivas}}`, `{{total.nocturnas_festivas}}` | sumas del mes | número |

Fila de datos: la fila que tiene marcadores `{{fila.…}}`. No necesita otra
marca. Si hay `{{fila.…}}` en más de una fila, la plantilla se rechaza
(P18). Sus celdas usan `{{fila.fecha}}` (número de serie de Excel,
como hoy en `parseFecha`), `{{fila.solicitada_por}}`, `{{fila.actividad}}`,
`{{fila.observaciones}}`, `{{fila.hora_inicio}}` y `{{fila.hora_final}}`
(fracción de día, como `parseHora`), `{{fila.total}}`, `{{fila.diurnas}}`,
`{{fila.nocturnas}}`, `{{fila.diurnas_festivas}}` y
`{{fila.nocturnas_festivas}}`.

- Una celda que tiene **solo** un marcador numérico se escribe como número,
  con el estilo (y el formato de número) de la celda de la plantilla. Si el
  marcador va dentro de más texto, la celda queda como texto.
- Un valor del usuario nunca se escribe como fórmula (P28), aunque empiece
  con `=`.

### Algoritmo (hoja de la fila de datos)

1. Copiar la fila `R` una vez por registro (N filas, de `R` a `R+N-1`). Si
   el mes no tiene registros, la fila se quita (N = 0).
2. Correr N−1 filas hacia abajo todo lo que estaba debajo de `R`: los
   atributos `r` de filas y celdas, `mergeCells`, `conditionalFormatting`,
   `dataValidations`, `hyperlinks`, `dimension`, el área de impresión
   (`definedNames`) y los anclajes de imágenes (`drawing`).
3. Reescribir las referencias de las fórmulas de la hoja: una referencia
   debajo de `R` baja N−1 filas; un rango que empieza en `R` o antes y
   termina en `R` o después crece hasta `R+N-1`. En las filas copiadas, las
   referencias relativas a la propia fila se cambian a la fila nueva (la
   fórmula `SUM(H6:K6)` de cada fila). Se usa un lector de referencias A1
   propio, con `$` y nombres de hoja, que no toca lo que va entre comillas.
4. Llenar los marcadores. Las celdas de texto pasan de cadena compartida a
   `inlineStr` cuando cambian, para no tener que reescribir
   `sharedStrings.xml`.
5. Borrar los valores en caché de las fórmulas (`<v>` dentro de `<f>`) y
   poner `fullCalcOnLoad` en `workbook.xml`, para que Excel recalcule al
   abrir.

La validación al cargar (P18) usa el mismo lector: busca la fila de datos,
cuenta los marcadores, detecta `xl/tables/`, `xl/pivotTables/` y
`vbaProject.bin`, y limita el tamaño descomprimido antes de inflar cada
entrada (contra zips bomba).

### Plantillas incluidas

- **Legacy:** el Excel de siempre. En la v1 sigue saliendo de
  `generateOvertimeXlsx`. Si en la versión completa pasa a ser un `.xlsx`
  con marcadores, un test de P4 tiene que generar los dos para varios meses
  (sin registros, 1, 31, con festivos) y compararlos celda por celda.
- **Logday:** `assets/templates/logday-horas-extra.xlsx`, generado por
  `assets/templates/build_logday_xlsx.py` (openpyxl). El script recibe una
  carpeta y deja ahí la plantilla y un ejemplo lleno. Diseño aprobado por
  Jorge el 1 nov 2026:
  - **Sin marca de empresa ni logo.** Título "Horas extra — {{mes}}" en
    Calibri 16 negrita; debajo, "Nombre" y "Documento" en etiquetas grises
    con el valor en celdas combinadas B:D.
  - **Las mismas 11 columnas que Legacy, con nombres cortos.** Fecha,
    Solicitada por, Actividad, Observaciones, Inicio, Fin, Total, Diurnas,
    Nocturnas, Diurnas festivas y Nocturnas festivas. Encima, dos grupos
    combinados: "Horario" (E:G) y "Horas extra" (H:K).
  - **Color.** Índigo de Logday solo en los encabezados (fondo `#EEF2FF`,
    texto `#312E81`) y en la línea de totales (`#4F46E5`). Bordes finos
    grises (`#D4D4D8`), sin cuadrícula.
  - **Fila de datos** en la fila 8. "Total" por fila es la fórmula
    `SUM(H8:K8)`, para que siga bien si alguien corrige una hora a mano.
    Formatos: `dd/mm/yyyy`, `h:mm AM/PM` y `0.00;-0.00;"–"`, que muestra el
    cero como una raya para que se lean solo las horas que hay.
  - **Totales** con `SUM` sobre la fila de datos (el motor estira el
    rango). Debajo, una nota pequeña y gris con las franjas diurna,
    nocturna y festiva y qué va en Observaciones. Es texto normal:
    cada empresa la cambia en Excel.
  - **Impresión.** Paneles fijos debajo de los encabezados; horizontal,
    ajustado al ancho de la página, y los encabezados (filas 6 y 7) se
    repiten en cada página.
- Los dos archivos canónicos viven en `logday-server/assets/templates/`
  (con `go:embed` para el render del servidor, si llega) y se copian a cada
  cliente. Un test en cada cliente compara su copia con el hash anotado
  en este spec, que se actualiza cuando cambian.

## 4. Clientes

### Dónde se guarda la plantilla activa

- **Desktop:** el registro en el store, persistido como el resto (ver
  `localStorage como mecanismo único de persistencia (Desktop)`). El
  `.xlsx` de una plantilla propia va como archivo en la carpeta de datos,
  `<basePath>/.logday/plantillas/overtime.xlsx`, para no cargar 1 MB en
  `localStorage`. El registro guarda la ruta y el hash.
- **Web:** solo en el servidor, más una caché en memoria.
- **Mobile:** tabla `templates` en SQLite (misma forma), con el contenido en
  base64.

### Interfaz

Una sección nueva en Opciones, "Plantillas", en el grupo de datos, con dos
tarjetas:

- **Mensaje del Daily:** un selector "Logday · Propia" (P5). La propia
  tiene un editor de texto con los marcadores como chips para insertar, y
  arranca con el mensaje de siempre ("Empezar con la de Logday" lo
  restablece). Debajo, una vista previa con datos de ejemplo y los avisos
  (P14).
- **Excel de horas extra:** un selector "Legacy · Logday · Propia", con el
  nombre y la fecha de carga de la propia, y las acciones "Cargar plantilla
  (.xlsx)", "Descargar la cargada" y "Descargar la de Logday" como punto de
  partida (P21, P22). Al cargar: la validación
  (P18), los marcadores encontrados, los avisos (P19) y "Descargar un
  ejemplo" antes de "Activar" (P20).

El diseño fino (textos, ayuda, estados vacíos) lo hace `logday-design`
antes de implementar.

## 5. Alternativas descartadas

- **Elegir solo entre diseños fijos en el código:** no sirve
  para otras empresas, cuyo formato no conocemos.
- **Configurar el Excel con un formulario** (título, nombres de columna,
  colores): cubre poco (logos, celdas combinadas y fórmulas propias de
  cada empresa) y es más trabajo que leer un `.xlsx` hecho en Excel.
- **Generar el Excel en el servidor:** Desktop y Mobile funcionan sin
  servidor (local-first).
- **Plantillas como preferencia local:** se perderían al cambiar de equipo,
  y Web, Desktop y Mobile darían resultados distintos.
- **Cambiar lo predeterminado, con una migración que deje lo de siempre a
  las instalaciones de antes:** era la primera versión de este spec. Jorge
  la cambió el 1 nov 2026 por algo menos invasivo: lo de siempre sigue
  siendo lo predeterminado y no se migra nada. Si más adelante conviene cambiar la predeterminada, está en
  "Pendiente de decisión".
