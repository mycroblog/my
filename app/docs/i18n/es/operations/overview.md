<!-- source: overview-of-a-mycroblog-server ce703d24935f -->
# Visión general de un servidor de mycroblog

La primera página que conviene leer antes de poner en marcha un servidor. mycroblog es un microblog que federa con el
resto del fediverso mediante ActivityPub; `my` es su software de servidor, un único binario que contiene el servidor, el
procesador de trabajos, el cliente web y los comandos que se ocupan de ellos. Esta página muestra de qué se compone un
servidor, qué eliges mediante la configuración, cómo un servidor pequeño crece hasta uno grande y qué leer a
continuación.

<!-- source: what-a-server-is-made-of 1210fe6ac0c4 -->
## De qué se compone un servidor

![La arquitectura de un servidor de mycroblog: los clientes llegan a los procesos de my mediante HTTPS; los procesos usan las partes elegidas por configuración y los servicios externos al servidor que actives. Las secciones de abajo describen cada parte.](images/architecture.svg)

De arriba abajo en el diagrama:

- **Quién se conecta.** Las personas usan el cliente web en su navegador (puede instalarse como aplicación), las aplicaciones y los bots usan la API de cliente, y otros servidores del fediverso entregan y recuperan publicaciones mediante ActivityPub. Todos llegan al servidor mediante HTTPS.
- **Delante del servidor.** `my` habla HTTP sin cifrar, así que un proxy inverso o un balanceador de carga termina HTTPS delante de él. Una CDN delante de los archivos multimedia es opcional.
- **Los procesos de `my`.** Cada proceso ejecuta `my serve`, y su [`role`](../reference/configuration.md#role) indica qué ejecuta: `web` el servidor HTTP, `worker` el procesador de trabajos, que entrega publicaciones, recupera contenido de otros servidores, envía correo electrónico, procesa contenido multimedia y ejecuta los trabajos periódicos, y `all`, el valor predeterminado, ambos en un solo proceso.
- **Las partes que eliges.** Cada parte del centro del diagrama tiene una configuración que elige su adaptador, enumerado en [la sección siguiente](#parts-you-choose-by-configuration).
- **Servicios externos al servidor.** CAPTCHA, el chat de voz, los pagos, los complementos y la importación de costos de la nube permanecen desactivados hasta que los configures; el servidor llama entonces al servicio que indiques.

<!-- source: parts-you-choose-by-configuration f7dbfce7a91c -->
## Partes que eliges mediante la configuración

Cada parte es una configuración de la [referencia de configuración](../reference/configuration.md), definido en un archivo TOML o como variable de entorno (`MY_` y la clave en mayúsculas, como `MY_DATABASE`). La primera opción de cada lista es la predeterminada. Cuando una parte guarda datos, elígela antes del primer arranque: los datos no se mueven solos cuando cambia la configuración.

| Parte | Configuración | Opciones | Leer a continuación |
| --- | --- | --- | --- |
| Base de datos | [`database`](../reference/configuration.md#database) | `sqlite` (un archivo), `postgres` (PostgreSQL y servicios compatibles) | [Base de datos](database.md), [copia de seguridad](backup.md) |
| Cola de trabajos | [`queue`](../reference/configuration.md#queue) | `db` (una tabla de la base de datos), `inline` (ejecuta cada trabajo de inmediato en el mismo proceso, para pruebas y desarrollo), `cloudtasks` (Google Cloud Tasks despierta al servidor para cada trabajo; requiere PostgreSQL) | [La cola de trabajos](running.md#the-job-queue-and-failed-dead-jobs) |
| Almacén clave-valor, para cachés y límites de frecuencia | [`kv`](../reference/configuration.md#kv) | `db` (una tabla de la base de datos), `memory` (cada proceso por su cuenta), `valkey` (Valkey u otro servidor con protocolo Redis, compartido por todos los procesos) | [`kv`](../reference/configuration.md#kv) |
| Almacenamiento multimedia | [`blob`](../reference/configuration.md#blob) | `fs` (un directorio), `s3` (Amazon S3, Cloudflare R2, MinIO, Garage y otros almacenes compatibles con S3), `gcs` (Google Cloud Storage) | [Archivos de Docker Compose](running.md#docker-compose-files), [copia de seguridad del contenido multimedia](backup.md#media-in-a-bucket) |
| Procesamiento multimedia | [`media_image`](../reference/configuration.md#media_image), [`media_video`](../reference/configuration.md#media_video), [`media_audio`](../reference/configuration.md#media_audio), uno para cada tipo de archivo | `native` (en el servidor), `plugin` (un complemento tuyo convierte los archivos de ese tipo) | [Convertir contenido multimedia](plugins.md#converting-media) |
| Flujo en tiempo real entre procesos | [`stream`](../reference/configuration.md#stream) | `hub` (dentro de un proceso), `postgres` (`LISTEN` / `NOTIFY`), `valkey`, `sns` (Amazon SNS y SQS), `pubsub` (Google Cloud Pub/Sub) | [`stream`](../reference/configuration.md#stream) |
| Gestor de claves, que cifra las claves privadas de las cuentas | [`key_manager`](../reference/configuration.md#key_manager) | `local` (una clave de cifrado de claves que conservas tú), `gcpkms` (Google Cloud KMS), `awskms` (AWS KMS), `plugin` | [Gestionar la clave](key-management.md) |
| Correo electrónico | [`mail`](../reference/configuration.md#mail) | `none`, `smtp`, `ses` (Amazon SES), `plugin` | [Enviar correo electrónico](email.md) |
| CAPTCHA | [`captcha`](../reference/configuration.md#captcha) | `none`, `turnstile` (Cloudflare Turnstile), `hcaptcha`, `recaptcha` (Google reCAPTCHA v2) | [CAPTCHA](captcha.md) |
| Chat de voz | [`voice`](../reference/configuration.md#voice) | `off`, `p2p` (de navegador a navegador, con TURN de `voice_turn`: `none`, `coturn` o `cloudflare`), `livekit`, `sora` | [Chat de voz](voice.md) |
| Pagos de mecenas | [`payments`](../reference/configuration.md#payments) | `none`, `stripe`, `payjp` (PAY.JP) | [Mecenas](supporters.md), [Stripe](payments-stripe.md), [PAY.JP](payments-payjp.md) |
| Complementos | `plugin_<kind>_address` | ninguno de forma predeterminada; una dirección para cada tipo: `translator`, `search`, `billing`, `mailer`, `media_image`, `media_video`, `media_audio`, `key_manager` y los ganchos de decisión `inbound.status`, `local.status`, `inbound.follow`, `signup.check`, `media.check`, `inbound.actor`, `local.profile` | [Complementos](plugins.md), [ganchos de decisión](decision-hooks.md) |
| Traducción de publicaciones | `translator_<provider>_*`, [`plugin_translator_address`](../reference/configuration.md#plugin_translator_address) | ninguna (solo la traducción del propio navegador); `libretranslate`, `deepl`, `google` (Google Cloud Translation), `aws` (Amazon Translate), o un complemento tuyo delante del servicio que elijas | [Traducir publicaciones](translation.md), [un complemento de traducción](plugins.md#translating-posts) |
| Búsqueda de texto completo de publicaciones | [`search`](../reference/configuration.md#search) | `none`; o `elasticsearch`, `opensearch` o `meilisearch` ([motores de búsqueda](search-engines.md)); o `plugin`, un complemento tuyo delante de otro motor | [Búsqueda de texto completo](search.md), [un complemento de búsqueda](plugins.md#searching-posts) |
| CDN para contenido multimedia | [`media_origin`](../reference/configuration.md#media_origin) | sin definir (el servidor sirve el contenido multimedia desde su propio origen), o el origen de tu CDN | [Servir contenido multimedia mediante una CDN](cdn.md) |
| Costos de la nube | [`billing_sources`](../reference/configuration.md#billing_sources) | ninguno, o cualquiera de `aws` (AWS Cost Explorer), `gcp` (la exportación de facturación o el presupuesto de Google Cloud), `plugin` | [Importar costos de la nube](billing-import.md) |
| Métricas | [`metrics_exporter`](../reference/configuration.md#metrics_exporter) | `none`, `prometheus`, `otlp` (OpenTelemetry), `emf` (CloudWatch) | [Métricas](running.md#metrics), [alertas](alerts.md) |
| Trazas | [`traces_exporter`](../reference/configuration.md#traces_exporter) | `none`, `otlp` | [Trazas](running.md#traces) |
| Informes de errores | [`errors_exporter`](../reference/configuration.md#errors_exporter) | `none` (solo la línea de registro y los grupos de errores), `sentry` (Sentry, GlitchTip, Datadog), `otlp` | [Informes de errores](error-reporting.md) |

El binario publicado y la imagen de contenedor incluyen todos los adaptadores de la tabla. Un binario que compilas tú
puede dejar fuera los que no uses con [etiquetas de compilación](../reference/build-tags.md), y entonces rechaza sus
configuraciones al arrancar.

`my config check` informa de todos los problemas de la configuración a la vez, antes de que arranques el servidor
([lista de comprobación para producción](production-checklist.md)).

<!-- source: from-a-small-server-to-a-large-one 42bbd3f2a76e -->
## De un servidor pequeño a uno grande

![Tres pasos de un servidor pequeño a uno grande: un proceso con SQLite y el sistema de archivos, después PostgreSQL y almacenamiento de objetos detrás de una CDN, y después procesos web y de trabajo separados con un broker de flujos. La lista de abajo describe cada paso.](images/growth.svg)

1. **Empieza pequeño.** Un proceso con `role = all`, con SQLite, con el contenido multimedia en el sistema de archivos y los valores predeterminados para el resto: la cola y el almacén clave-valor en la base de datos, y el flujo en tiempo real dentro del proceso. Los archivos de Docker Compose de `deploy/docker` arrancan esta configuración en una sola máquina ([desplegar con Docker Compose](deploy-docker-compose.md)). Es adecuada para un servidor personal o pequeño. `stream = hub` solo llega al proceso en el que se ejecuta, así que esta configuración se queda en un proceso.
2. **Saca los datos.** PostgreSQL para la base de datos, un bucket compatible con S3 o Google Cloud Storage para el contenido multimedia, y una CDN delante del contenido multimedia. El servidor sigue siendo un proceso, pero ya no depende de un disco, y los procesos del paso siguiente pueden compartir los datos. Decídelo antes del primer arranque si puedes: mover más tarde los datos desde SQLite y el sistema de archivos es una tarea aparte ([qué copiar](backup.md#what-to-back-up) enumera lo que hay que mover).
3. **Escala.** Ejecuta procesos con `role = web` para las peticiones y `role = worker` para los trabajos, tantos como necesite cada uno, y reparte los procesos de trabajo por tipo de trabajo ([repartir los procesos de trabajo](running.md#splitting-workers-by-job-type)). Con más de un proceso, el flujo en tiempo real necesita un broker que llegue a todos los procesos (`postgres`, `valkey`, `sns` o `pubsub`), y el almacén clave-valor debería compartirse (`db` o `valkey`), ya que `memory` mantiene separados los límites de frecuencia de cada proceso. La cola de trabajos se queda en PostgreSQL, o pasa a Google Cloud Tasks ([Cloud Tasks](running.md#cloud-tasks)).

El [despliegue en Cloud Run](deploy-cloud-run.md) es un ejemplo de configuración entre el segundo y el tercer paso:
`role = all` en Cloud Run, PostgreSQL en Neon, contenido multimedia en Cloud Storage, Cloud Tasks para los trabajos y
Pub/Sub para el flujo, sin ninguna instancia en ejecución mientras nadie usa el servidor.

<!-- source: where-to-go-next 2f1d25c01f8a -->
## A dónde ir a continuación

Lee las guías para operadores en este orden:

1. **Primeros pasos:** [ejecuta el servidor](running.md) o [despliégalo en Cloud Run](deploy-cloud-run.md), y repasa después la [lista de comprobación para producción](production-checklist.md) antes de abrirlo a los usuarios.
2. **Configura la instancia:** sus [configuraciones](instance-settings.md), el [registro](registration.md), los [proveedores de inicio de sesión externos](login-providers.md), el [correo electrónico](email.md), [CAPTCHA](captcha.md), [la CDN](cdn.md), las [vistas previas de enlaces](link-previews.md) y los [motores de búsqueda y rastreadores de IA](crawlers.md).
3. **Activa funciones:** la [moderación](moderation.md), los [relés](relays.md), los [webhooks](webhooks.md), los [complementos](plugins.md) y los [ganchos de decisión](decision-hooks.md), el [chat de voz](voice.md), la [búsqueda de texto completo](search.md), los [mecenas](supporters.md) y sus pagos, y la [importación de costos de la nube](billing-import.md).
4. **Mantenlo en el día a día:** la [monitorización](monitoring.md), las [alertas](alerts.md), los [informes de errores](error-reporting.md), el [mantenimiento](maintenance.md), la [copia de seguridad](backup.md), [la base de datos](database.md), [la clave de cifrado de claves](key-management.md) y los [administradores y roles](admin-cli.md).
5. **Aspectos legales y cierre:** las plantillas de [política de privacidad](privacy-policy-template.md) y de [registro de tratamiento](processing-record-template.md), y [cerrar el servicio](service-closure.md).

La [referencia de configuración](../reference/configuration.md) enumera todos las configuraciones, y la [referencia de trabajos](../reference/jobs.md)
todos los trabajos que ejecutan los procesos de trabajo.
