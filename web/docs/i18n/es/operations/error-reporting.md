<!-- source: error-reporting 1ccf0c1c98a4 -->
# Informes de errores

Cómo te informa el servidor de sus propios errores y fallos, dónde leer los informes en cada plataforma y cómo enviarlos a un rastreador de errores.
Las métricas te dicen que algo va mal; un informe de error te dice qué ha fallado, con su traza de pila. La [referencia de configuración](../reference/configuration.md) enumera todos las configuraciones.

Cada informe recibe tres tratamientos, sea cual sea tu configuración:

1. Una línea `error report` va a la salida de error estándar, con la traza de pila ([La línea de registro](#the-log-line)).
2. El informe se cuenta en un grupo de errores de la base de datos. La pantalla de administración muestra los grupos, y las personas con el permiso `manage_errors` reciben notificaciones de los grupos nuevos y de los que reaparecen ([Grupos de errores y notificaciones](#error-groups-and-notifications)).
3. Si estableces `errors_exporter`, el informe también se envía a un servicio externo ([Envío de informes a un servicio](#sending-reports-to-a-service)).

Nada sale del servidor a menos que establezcas `errors_exporter`.

<!-- source: what-is-reported 8e5ac9d14c36 -->
## Qué se informa

| Se informa | No se informa (lo cubren las métricas y las demás líneas de registro) |
| --- | --- |
| Un pánico en un manejador HTTP, un trabajo o una tarea en segundo plano | Las respuestas 4xx (entrada incorrecta, autorización, límites de frecuencia) y el modo de mantenimiento |
| Un error detrás de una respuesta 5xx de la API de cliente, la API de administración, las páginas de inicio de sesión, ActivityPub, OAuth o los archivos multimedia | Una solicitud cuyo cliente se ha ido |
| Un trabajo que ha fallado definitivamente (`dead`) | El reintento de un trabajo. Se informa cuando agota los reintentos |
| Un fallo al arrancar: una configuración no válida, o una base de datos o un adaptador que no se puede preparar | Una comprobación de disponibilidad fallida |
| Un error que el cliente web no gestionó, enviado por el navegador al servidor | Los fallos causados por otros servidores: reintentos y abandonos de entregas, firmas rechazadas, consultas remotas fallidas |
| | El ruido propio del navegador: una red perdida, errores de extensiones del navegador y fragmentos (chunks) que un nuevo despliegue eliminó |

Un fallo de un servicio externo (correo, almacenamiento, pagos, voz) solo se informa cuando hace que un trabajo falle definitivamente o que una solicitud responda 5xx. Un fallo temporal aparece en cambio en las métricas `my.adapter.*` ([Ejecutar el servidor](running.md#metrics)).

Un pánico en una tarea en segundo plano detiene el proceso después de informarse, de modo que el servidor nunca sigue funcionando en un estado roto. Tu plataforma lo reinicia.

<!-- source: the-log-line ef6f03400831 -->
## La línea de registro

La línea tiene `msg` = `error report` y `level` `ERROR` (`severity` es `CRITICAL` cuando el fallo detiene el proceso).
Sus campos, y lo que hace la depuración de datos sensibles con ellos, están en [Ejecutar el servidor](running.md#error-reports). En resumen:

- `my.error.fingerprint` es el mismo para cada aparición del mismo error. Busca o agrupa por él.
- `my.error.where` indica dónde ocurrió, por ejemplo `client_api GET /api/v1/statuses/{id}`.
- `request_id`, `job_id`, `trace_id` y `span_id` encuentran las demás líneas de la misma solicitud o trabajo.
- La línea de un error de una solicitud tiene `context.httpRequest`: el método, la ruta como URL (`/api/v1/statuses/{id}`, nunca la ruta que pidió el cliente) y el estado devuelto. Error Reporting los muestra en el grupo y puede filtrar por ellos.
- `message` es lo último de la línea: el tipo y el mensaje del error, un salto de línea y la pila de Go.
- La pila empieza donde se originó el error, no donde se informó: para un error de base de datos, la función del servidor que ejecutó la consulta. Por tanto, los errores de lugares distintos caen en grupos distintos, tanto en Error Reporting como en la pantalla de administración.

Ninguna otra línea lleva una pila.

<!-- source: google-cloud-cloud-run a263d8621ca1 -->
### Google Cloud (Cloud Run)

Cloud Error Reporting lee la línea sin ninguna configuración y enumera los errores en la consola, en Error Reporting. Es gratuito; solo pagas Cloud Logging.

- Agrupa los errores por el tipo y los frames superiores de la pila. No puede tomar la huella del servidor, así que un grupo de Error Reporting puede diferir de un grupo de la pantalla de administración.
- Conserva los grupos durante 30 días, y te avisa de errores nuevos y de errores resueltos que vuelven a ocurrir, como máximo 20 notificaciones por hora para el proyecto. Configura su canal de notificación en la consola de Google Cloud.
- Para encontrar lo que ocurrió en torno a un informe, busca en Cloud Logging `jsonPayload."request_id"="<id>"`, o lo mismo con `trace_id`.

Con el módulo de OpenTofu [`mycroblog-gcp`](../../deploy/opentofu/README.md#sections), la sección `observability` envía aquí los informes de forma predeterminada (`errors = "error-reporting"`): activa la API de Error Reporting y deja `errors_exporter` en `none`.

<!-- source: aws bc7e48e1321b -->
### AWS

CloudWatch Logs lee la línea como JSON sin más.

- Activa la detección de anomalías de registros para el grupo de registros. Señala un patrón de línea que nunca había visto y puede alarmar a SNS. Lee solo los primeros 1500 caracteres de una línea, por lo que `message` es el último campo. No conserva un estado de resuelto.
- Para contar informes, crea un filtro de métricas como `{ $.msg = "error report" }`. No uses ninguna dimensión tomada de `my.error.fingerprint`: cada valor de una dimensión se factura como una métrica propia. `my.error.source` y `my.error.kind` son seguras.
- Encuentra el contexto con Logs Insights: ``filter `request_id` = "<id>"``.

Con el módulo de OpenTofu [`mycroblog-aws`](../../deploy/opentofu/README.md#sections-of-mycroblog-aws), la sección `observability` conserva los informes en CloudWatch Logs de forma predeterminada (`errors = "cloudwatch"`), con `errors_exporter` en `none`. La detección de anomalías de registros sigue siendo un paso tuyo.

<!-- source: self-hosted 90de7a419474 -->
### Autoalojado

Lee la línea en el registro del contenedor: `docker compose logs my | jq -c 'select(.msg == "error report")'`.
Con `metrics_exporter = prometheus`, la alerta `MyErrorReportsIncreasing` ([Alertas y guía de actuación](alerts.md#myerrorreportsincreasing)) avisa cuando los informes aumentan bruscamente; el contador en el que se basa es `my.errors.reported`.
Para notificaciones que no necesiten un servicio externo, usa la pantalla de administración y los webhooks que se describen más abajo. Para un rastreador, usa [`errors_exporter`](#sending-reports-to-a-service).

<!-- source: error-groups-and-notifications 8f740a328309 -->
## Grupos de errores y notificaciones

El servidor cuenta los informes por huella en la base de datos. No necesita ninguna configuración ni servicio adicional, y escribe poco: el primer informe de una huella se escribe de inmediato, y los siguientes se suman en memoria y se escriben una vez por minuto. Una terminación forzosa del proceso puede perder los recuentos del último minuto; las líneas de registro no se pierden así.
Los recuentos de más de 90 días se eliminan cada día. Si lo que ha fallado es la base de datos, el informe queda solo en la línea de registro.

<!-- source: the-administration-screen e7dacd037150 -->
### La pantalla de administración

Las personas cuyo rol tiene `manage_errors` (el rol integrado `admin` lo tiene) ven «Errores» en el menú de administración (`/admin/errors`).

- La lista muestra los grupos, por defecto los abiertos, el visto más recientemente primero. Filtra por estado (abierto, resuelto, silenciado, todos los estados) y por origen (el servidor, el cliente web). Cada fila muestra la primera línea del mensaje, dónde ocurrió, los recuentos de las últimas 24 horas y 7 días, cuándo se vio por última vez y su estado.
- La página de un grupo empieza con lo que puedes hacer en su estado («Resolver» y «Silenciar» para un grupo abierto, «Silenciar» para uno resuelto, «Dejar de silenciar» para uno silenciado). Muestra los datos del grupo (dónde, el mensaje, la primera y la última vez y versión, la huella), sus recuentos de las últimas 24 horas y 7 días, la última aparición (hora, versión, rol del proceso, los tipos, el ID de solicitud y el ID de traza, y la pila), las apariciones por hora de los últimos 90 días, la más reciente primero, y el historial de sus cambios de estado. La huella, el ID de solicitud, el ID de traza y la pila tienen un botón de copiar: pega los ID en tu búsqueda de registros.
- «Resolver» es para un error que corregiste. Si vuelve a ocurrir en otra versión del servidor, el grupo se abre de nuevo y vuelve a notificar. El mismo error en la versión que se resolvió solo se cuenta, para que las últimas solicitudes de la versión antigua no lo reabran.
- «Silenciar» sigue contando pero no envía ninguna notificación ni evento de webhook. «Dejar de silenciar» abre el grupo de nuevo.

Las mismas acciones están disponibles como `GET /api/v1/admin/error_groups` y `POST /api/v1/admin/error_groups/{id}/resolve` (`/mute`, `/unmute`).

<!-- source: notifications-and-webhooks 99836c7f02ec -->
### Notificaciones y webhooks

- Las personas con `manage_errors` reciben la notificación `admin_error_group` por un grupo de errores nuevo y por una reaparición. Llegan como máximo 10 por hora para todo el servidor; el resto de la hora se agrupa en una sola notificación que indica que se han informado más errores. También pueden recibirla por correo electrónico ([Moderación](moderation.md#notifications-for-moderators-and-the-pending-counts)).
- Los eventos de webhook `error_group.created` y `error_group.regressed` llevan los mismos grupos, de modo que puedes enviarlos a Slack, Discord o una herramienta de automatización ([Webhooks del servidor](webhooks.md), [Referencia de webhooks](../reference/webhooks.md)). El webhook necesita `manage_webhooks`, y los eventos necesitan `manage_errors`.
- Un grupo silenciado nunca notifica.

<!-- source: errors-from-the-web-client e5b2f45b62be -->
### Errores del cliente web

El cliente web envía los errores que sus pantallas no gestionaron a `POST /api/v1/client_errors` en tu propio servidor, así que ningún tercero los ve y no cambia ninguna configuración de seguridad de contenido. El servidor responde `204` a todo y descarta lo que no acepta: un cuerpo de más de 16 KiB, una traza de pila sin ningún archivo de tu propio cliente web y más de 10 informes por minuto desde una misma dirección IP. El mismo error se envía una vez por visualización de página, y como máximo 20 por visualización de página.
Un informe enviado sin una sesión iniciada se cuenta y se registra, pero no notifica a nadie por sí solo, de modo que un script anónimo no pueda llenar tu bandeja de entrada. El grupo notifica cuando también lo informa el navegador de una persona con sesión iniciada.
Las trazas de pila son las de la compilación minificada y no se traducen de vuelta al código fuente, así que lee primero la ruta y el mensaje. La versión es la del servidor que sirvió la página. Una página que se abrió sin conexión, o tras 3 segundos sin respuesta, procede de la copia del navegador, que indica la versión de cuando el navegador guardó el cliente web: una versión anterior con el mismo cliente web.

<!-- source: ux-measurements 2edcd05329e6 -->
## Mediciones de UX

El cliente web envía muestras de lo rápido que responde a `POST /api/v1/client_measurements` en tu propio servidor, igual que sus errores: cuánto tarda una pantalla en aparecer (LCP) y en responder a una pulsación (INP) en cada ruta, sus frames largos, cada navegación por tipo, cada acción por su ID (`post.reply`), y el viaje de ida y vuelta, la fluctuación y la pérdida de paquetes de las llamadas de voz. Una muestra nombra solo la ruta, el tipo, la acción o el transporte de la llamada; no lleva ninguna cuenta, dirección ni URL. El servidor responde `204` a todo y acepta como máximo 10 solicitudes por minuto de una misma dirección IP.
El servidor cuenta las muestras por hora en la base de datos, escritas una vez por minuto, y las elimina tras 30 días. `GET /api/v1/admin/ux_measurements` (`manage_errors`) suma las últimas 24 horas, o `hours` hasta 720, con el percentil 75 de cada nombre; las métricas `my.ux.duration` y `my.ux.call.packet_loss` exportan las mismas muestras ([Métricas](running.md#metrics)). Cada tipo conserva sus primeros 200 nombres en un proceso; los posteriores cuentan como `other`.
Está desactivado por defecto; el ajuste de la instancia `ux_measurement` lo activa, y mientras está desactivado el servidor no toma ninguna muestra ([Ajustes de la instancia](instance-settings.md)). Mientras está activado, solo una parte de las sesiones del navegador envía muestras, el 10 % salvo que `ux_measurement_share` fije otra, y cada sesión se sortea una vez cuando arranca el cliente web.

Las personas cuyo rol tiene `manage_errors` ven «Mediciones de UX» en el menú de administración (`/admin/ux-measurements`). Elige las últimas 24 horas, 7 días o 30 días; cada métrica con muestras tiene una tabla de sus nombres con sus muestras y el percentil 75 y, para LCP e INP, la proporción de muestras buenas y malas (dentro de 2,5 s y más de 4 s para LCP, dentro de 200 ms y más de 500 ms para INP), con un percentil 75 por encima del umbral bueno marcado. «Medir el cliente web», bajo las tablas, es el ajuste `ux_measurement`, que solo cambian allí las personas cuyo rol también tiene `manage_settings`, con su propio «Guardar».

<!-- source: sending-reports-to-a-service eb5f208ca8fc -->
## Envío de informes a un servicio

Establece `errors_exporter` para enviar además cada informe a un rastreador de errores. La línea de registro y los grupos de la base de datos siguen activos. Un informe se envía una sola vez y nunca se reintenta, y nada espera al servicio.

| Value | Sends to |
| --- | --- |
| `none` (default) | A ninguna parte |
| `sentry` | Cualquier servicio compatible con Sentry en `errors_sentry_dsn` |
| `otlp` | Un colector o backend de OpenTelemetry, como registros de log |

`errors_environment` (por defecto `production`) da nombre al despliegue en cada informe, por ejemplo `staging`.

Ambos exportadores envían los mismos campos depurados que la línea de registro, con la huella y los ID de traza y de span, y nada sobre el host: ni nombre de host, ni dirección IP, ni usuario, ni cuerpo de solicitud, ni cabecera, ni cookie.

El servidor limita lo que envía para no saturar el servicio:

- La misma huella sale como máximo una vez por minuto y por proceso, y todos los informes como máximo 60 por minuto. El resto queda en el registro y en los grupos.
- Hasta 1000 informes esperan a ser enviados. Un informe que encuentra la cola llena se descarta. Cuando el servidor se detiene, espera hasta 2 segundos a que la cola se vacíe.
- Cuando el servicio responde `429` o pide que se reduzca el ritmo (`Retry-After`, `X-Sentry-Rate-Limits`), el servidor deja de enviar durante ese tiempo.
- El servidor nunca sigue una redirección del servicio, para que la clave del DSN y el informe no vayan a ningún otro sitio: una redirección cuenta como `failed`. Usa el DSN que muestra el servicio, que apunta directamente a su punto de entrada.

Vigila el contador `my.errors.exported`: `my.outcome` es `success`, `dropped` (por encima de un límite, la cola estaba llena o el servicio pidió reducir el ritmo) o `failed` (no se pudo contactar con el servicio o este rechazó el informe) ([Ejecutar el servidor](running.md#metrics)).

<!-- source: sentry-glitchtip-and-datadog 81bdd8389ace -->
### Sentry, GlitchTip y Datadog

```sh
MY_ERRORS_EXPORTER=sentry
MY_ERRORS_SENTRY_DSN_FILE=/run/secrets/sentry_dsn   # or MY_ERRORS_SENTRY_DSN; the DSN is a secret
```

Con los módulos de OpenTofu, deja los errores de la sección `observability` a los ajustes, y el módulo crea el secreto del DSN para que lo rellenes, que su salida `secrets_to_fill` enumera:

```hcl
  observability = { errors = "settings" }
  settings      = { errors_exporter = "sentry" }
  secrets       = ["errors_sentry_dsn"]
```

El servidor habla el protocolo de Sentry por sí mismo, así que no interviene ningún SDK de Sentry y no se lee ninguna variable de entorno `SENTRY_*`. Define la huella, por lo que el servicio agrupa por ella, y las etiquetas `service` (`my`), `service.namespace` (el `host`), `my.role`, `my.error.where` y `error.fingerprint`. La versión es la del servidor y el entorno es `errors_environment`.

- **Sentry** (alojado o autoalojado): copia el DSN de las claves de cliente del proyecto.
- **GlitchTip**: copia el DSN del proyecto. Es de código abierto y funciona en un servidor pequeño. Comprueba qué reglas de alerta ofrece tu versión para incidencias nuevas y recurrentes.
- **Datadog Error Tracking**: usa el DSN de Sentry de tu sitio de Datadog, `https://<DD_API_KEY>@sentry-intake.<DD_SITE>/1`. Datadog exige la etiqueta `service`, que el servidor siempre define. Datadog almacena estos eventos como registros y los agrupa en Error Tracking. Si envías la línea de registro a Datadog con un agente o un reenviador (Error Tracking for Logs), la pila está en `message`; asígnala a `error.stack` con un pipeline de registros, o Datadog no la ve como una pila. El exportador `sentry` no necesita tal asignación.

<!-- source: otlp-backends fe964b8f1b2c -->
### Backends OTLP

```sh
MY_ERRORS_EXPORTER=otlp
OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4318   # or OTEL_EXPORTER_OTLP_LOGS_ENDPOINT
OTEL_EXPORTER_OTLP_HEADERS=authorization=Bearer%20...
```

Cada informe es un registro de log de OpenTelemetry llamado `exception`, con severidad `ERROR` (`FATAL` cuando detiene el proceso), `exception.type`, `exception.message`, `exception.stacktrace`, `my.error.fingerprint`, `my.error.where`, `my.error.source`, `my.error.kind` y los ID de traza y de span. Va por OTLP/HTTP (protobuf, gzip) con el mismo recurso que las métricas y las trazas.

Que el backend convierta estos registros en errores agrupados depende del backend: algunos construyen sus vistas de errores solo a partir de datos de trazas, y Datadog necesita `error.stack`. Para Sentry, GlitchTip y Datadog, usa `sentry`. Usa `otlp` cuando quieras los registros junto a tus demás logs.

<!-- source: what-scrubbing-does-and-does-not-guarantee ccf1b4716bdd -->
## Qué garantiza y qué no garantiza la depuración

La depuración se ejecuta una sola vez, cuando se crea el informe, así que la línea de registro, la base de datos y todos los exportadores reciben solo el texto depurado.

- En el mensaje, las URL, las direcciones de correo electrónico, las menciones `@user@host`, las direcciones IP, los UUID y ULID, y los números de 10 dígitos o más se sustituyen por `<url>`, `<email>`, `<acct>`, `<ip>`, `<id>` y `<n>`, y el mensaje se recorta a 1 KiB.
- En la traza de pila, los valores de los argumentos de las funciones se sustituyen por `(...)` y la traza se recorta a 16 KiB. Las posiciones de archivo se conservan, relativas al árbol de código fuente, de modo que no aparecen las rutas de la máquina de compilación.
- Ningún informe contiene cuerpo de solicitud, cabecera, consulta, cookie, ID de cuenta, nombre de usuario, dirección IP ni User-Agent.

La depuración busca patrones, así que no garantiza que un mensaje no contenga datos personales. Pueden quedar un nombre, un texto libre o un número de menos de 10 dígitos que cite un mensaje de error. El campo `error` de las líneas ordinarias `http request` y `job` no se depura de este modo. Trata los registros y el rastreador de errores como lugares que pueden contener fragmentos de contenido de usuarios, y dales el mismo control de acceso y la misma retención que al resto de tus registros. Una vez que estableces `errors_exporter`, se aplican a los informes la ubicación, la retención y las reglas de acceso de ese servicio.
