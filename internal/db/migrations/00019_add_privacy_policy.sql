-- +goose Up
-- Política de tratamiento de datos personales, por instancia — mismo
-- criterio que el resto de instance_settings (specs/panel-admin/):
-- editable por el admin desde el panel, versión sube a mano cuando el
-- texto cambia (ver specs/cumplimiento-datos-personales/). El texto
-- por defecto es una PLANTILLA, no un documento legal final — cada
-- operador debe adaptarlo antes de usarlo con usuarios reales.
ALTER TABLE instance_settings ADD COLUMN privacy_policy_text TEXT NOT NULL DEFAULT
'POLÍTICA DE TRATAMIENTO DE DATOS PERSONALES (PLANTILLA)

Esta es una plantilla de referencia basada en la Ley 1581 de 2012 y el
Decreto 1377 de 2013 de Colombia — NO es un documento legal definitivo.
Antes de usarla con usuarios reales, reemplaza los campos entre
corchetes y haz que la revise alguien con conocimiento legal.

1. Responsable del tratamiento
[NOMBRE DEL RESPONSABLE], identificado con [NIT/CC], con domicilio en
[CIUDAD, PAÍS], correo de contacto [CORREO DE CONTACTO].

2. Finalidad del tratamiento
Los datos que ingresas en esta instancia de Logday (tareas, notas,
registro de horas extra, calendario, ausencias y actividad diaria) se
almacenan para permitirte gestionar tu trabajo personal y, si
corresponde, compartir esa información con tu organización dentro de
los fines para los que fuiste invitado a esta instancia.

3. Datos que se tratan
Datos de cuenta (correo electrónico, contraseña cifrada) y el
contenido que tú mismo generas dentro de la aplicación.

4. Datos sensibles
El campo "incapacidad" del registro de ausencias es un dato sensible
de salud. No estás obligado a proporcionarlo, y su tratamiento
requiere tu consentimiento explícito y diferenciado, que se te pide
aparte la primera vez que lo usas.

5. Derechos del titular
Tienes derecho a conocer, actualizar, rectificar y solicitar la
supresión de tus datos personales. Puedes ejercerlos desde Ajustes →
Privacidad y datos dentro de la propia aplicación (exportar o eliminar
tu cuenta), o escribiendo a [CORREO DE CONTACTO].

6. Vigencia
Esta política rige desde [FECHA] y puede ser actualizada por el
responsable; se te notificará pidiéndote aceptarla de nuevo cuando eso
ocurra.';
ALTER TABLE instance_settings ADD COLUMN privacy_policy_version INTEGER NOT NULL DEFAULT 1;

-- +goose Down
ALTER TABLE instance_settings DROP COLUMN privacy_policy_text;
ALTER TABLE instance_settings DROP COLUMN privacy_policy_version;
