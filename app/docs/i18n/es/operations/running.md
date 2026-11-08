<!-- source: running-the-server-start-stop-and-health-checks 5164430b3203 -->
# Ejecutar el servidor: arranque, parada y comprobaciones de salud

Cómo arrancar y detener el servidor, obtener su imagen y vigilarlo mientras funciona: métricas, trazas, la cola de trabajos y las comprobaciones de salud.

<!-- source: start-the-server 0d5f8898a2f5 -->
## Arrancar el servidor

```sh
my config check   # report every configuration problem at once
my config check -instance   # also warn about instance settings that are not set (opens the database)
my serve          # start according to the configured role (-config <file> reads a TOML file)
my version        # print the version of the binary
```

`my serve` valida la configuración, construye los adaptadores de base de datos, cola de trabajos y gestión de claves, y acepta HTTP en `listen`.
Habla HTTP/1.1 simple, sin TLS: pon delante un proxy inverso o balanceador de carga que termine HTTPS (y HTTP/2 o HTTP/3 para los clientes) y reenvíe por HTTP/1.1.
Las actualizaciones en tiempo real (`GET /api/v1/streaming`, los flujos de las cronologías local y federada y los flujos de las salas de chat de voz) son respuestas de larga duración que envían un comentario de latido cada 30 segundos (15 en una sala de voz) y siguen abiertas mientras están inactivas.
El proxy no debe almacenar en búfer estas respuestas (nginx: el servidor envía `X-Accel-Buffering: no`), y su tiempo de espera de inactividad o de lectura debe ser mayor que el intervalo del latido.
No aplica el esquema: aplícalo con `my schema apply` antes de arrancar (consulta [Base de datos](database.md); las actualizaciones lo ejecutan como parte de [Actualización](database.md#upgrading)).
En la configuración de Docker Compose (`deploy/docker/compose.yaml`; consulta [Despliegue con Docker Compose](deploy-docker-compose.md)), un servicio `schema` de una sola ejecución ejecuta `my schema apply` antes de que arranque el servidor.
Si el esquema no está aplicado, el proceso arranca igualmente y la comprobación de disponibilidad devuelve 503, y no sirve nada más hasta que el esquema exista.

El primer `my serve` o comando `my` que usa la base de datos registra en ella `host` y `account_domain`. Cuando la configuración difiere de los valores registrados, `my serve` registra `start failed` con ambos pares de valores y termina con el código 1, y los comandos muestran el mismo error. Las excepciones son `my schema apply` y `my db backup`, que no lo comprueban, y el modo de mantenimiento, que no lee la base de datos. `my config check -instance` y `my db upgrade -check` lo comprueban pero no registran nada.
Un proceso que no pudo leer la base de datos al arrancar no está listo hasta que pueda, y hasta entonces no ejecuta trabajos y responde `503` con `Retry-After` a toda solicitud salvo las tres comprobaciones de salud siguientes (`/healthz`, `/readyz`, `/api/v1/instance/health`). Esto es también lo que ocurre con un servidor arrancado antes de `my schema apply`. Si después encuentra valores distintos, se detiene con el mismo error.

En el perfil `production`, `my config check` también comprueba que la configuración válida es adecuada para una instancia de producción: los errores, como un `host` al que otros servidores no pueden llegar o una KEK que no es aleatoria, hacen que termine con código 1, y las advertencias, como `mail = none`, no.
`my serve` registra ambos como advertencias al arrancar pero arranca igualmente, así que ejecuta `my config check` como un paso del despliegue.
La [lista de comprobación para producción](production-checklist.md) enumera las comprobaciones y lo que hay que verificar a mano antes de salir en producción.

| `role` | HTTP | Job worker |
| --- | --- | --- |
| `all` (por defecto) | Todas las rutas | Se ejecuta |
| `web` | Todas las rutas | No se ejecuta |
| `worker` | Solo las comprobaciones de salud (para los sondeos de la plataforma) | Se ejecuta |

Los registros van a la salida de error estándar como JSON Lines. Cada línea tiene `level` (`DEBUG`, `INFO`, `WARN`, `ERROR`) y `severity`, el mismo nivel en la forma que lee Cloud Logging (`DEBUG`, `INFO`, `WARNING`, `ERROR` y `CRITICAL` para un error que detiene el proceso).
Los registros escritos al gestionar una solicitud llevan un `request_id`, y el mismo ID se devuelve en la cabecera de respuesta `X-Request-Id`.
Si el `X-Request-Id` de la solicitud tiene como máximo 128 caracteres entre letras, dígitos y `-_.:`, se usa tal cual, como el ID asignado por un balanceador de carga.
Los registros dentro de una traza (solicitudes y trabajos) llevan además `trace_id` y `span_id` (consulta [Trazas](#traces)).

Cada solicitud HTTP y cada ejecución de un trabajo escribe una línea al terminar. La línea tiene los mismos nombres y valores que las etiquetas de las [métricas](#metrics) siguientes,
así que en Cloud Run con `metrics_exporter = none` puedes crear métricas basadas en registros de Cloud Logging (contadores y distribuciones) a partir de estas líneas.

| Línea (`msg`) | Campos |
| --- | --- |
| `http request` (`INFO`; las comprobaciones de salud usan `DEBUG`) | `http.request.method`, `http.route`, `my.surface` y `http.response.status_code` (un número) tienen los mismos valores que las etiquetas de `http.server.request.duration`. También `duration_seconds` (un número de segundos), `url.path` (la ruta sin la cadena de consulta), `http.response.body.size` y `request_id`. `http.route` y `my.surface` están ausentes en las solicitudes que no llegaron a una ruta (comprobaciones de salud, solicitudes rechazadas antes del enrutado), y la métrica no las cuenta. Una solicitud cuyo cliente se fue antes de que el servidor pudiera responder tiene el estado `499` en lugar de un 5xx, salvo que un manejador entrara en pánico; el cliente no recibió nada, así que no cuenta como error del servidor |
| `job` (`INFO` si tiene éxito y en `postponed`, `WARN` en `retry`, `ERROR` en `dead`) | `my.job.type` y `my.outcome` tienen los mismos valores que las etiquetas de `my.job.duration`. También `duration_seconds` (`my.job.duration`), `wait_seconds` (`my.job.wait`), `job_id`, `attempt` y `error` cuando falla |
| `error report` (`ERROR`; `CRITICAL` si el fallo detiene el proceso) | Una línea por cada error informado ([Informes de errores](#error-reports)) |

<!-- source: error-reports 3d780744461c -->
### Informes de errores

La guía de [Informes de errores](error-reporting.md) explica dónde leer estas líneas en cada plataforma, la pantalla de errores, las notificaciones y el envío de informes a un rastreador de errores.

El servidor informa de los errores que requieren la atención del operador como una línea `error report` cada uno:

- un pánico en un manejador HTTP, un trabajo o una tarea en segundo plano;
- un error detrás de una respuesta 5xx de la API de cliente, la API de administración, las páginas de inicio de sesión, ActivityPub, OAuth o los archivos multimedia;
- un trabajo que falla definitivamente (`dead`): agotó los reintentos o se negó a ejecutarse de nuevo. No se informa de las entregas que se abandonaron porque el otro servidor seguía fallando;
- un fallo al arrancar: una configuración no válida, o una base de datos o un adaptador que no se puede preparar.

No se informa de las respuestas 4xx, los reintentos de trabajos, el modo de mantenimiento, las comprobaciones de disponibilidad fallidas, las solicitudes cuyo cliente se fue ni los fallos causados por otros servidores; los cubren las métricas y las demás líneas de registro.

Cuando el cliente de una solicitud se va, las demás líneas de error del servidor para esa solicitud se escriben en `INFO` en lugar de `ERROR`, ya que la mayoría solo dicen que la solicitud se canceló.

La línea tiene la forma que lee Google Cloud Error Reporting: en Cloud Run, los informes aparecen en Error Reporting agrupados como errores sin ninguna configuración. En otras plataformas es una línea JSON simple:

| Campo | Valor |
| --- | --- |
| `@type`, `serviceContext` | El tipo de evento de Error Reporting, y `{"service": "my", "version": "<version>"}` |
| `my.error.fingerprint` | 32 dígitos hexadecimales que son iguales en todas las apariciones del mismo error: construidos a partir del origen, el lugar, el tipo de error y las funciones del servidor en la traza de pila, sin números de línea |
| `my.error.source`, `my.error.kind` | `server` (o `web`), y `panic` o `error` |
| `my.error.where` | Dónde ocurrió: `<surface> <method> <route>` para una solicitud (como `client_api GET /api/v1/statuses/{id}`), el tipo de trabajo para un trabajo, el nombre de la tarea para una tarea en segundo plano, `startup` para un fallo al arrancar |
| `my.role`, `deployment.environment.name` | El `role` del proceso y `errors_environment` |
| `request_id`, `job_id`, `attempt`, `trace_id`, `span_id` | Los ID para encontrar las líneas de solicitud o de trabajo del mismo error, cuando los tiene |
| `message` | Lo último de la línea: `<type>: <message>`, un salto de línea y la traza de pila de Go |

Antes de que un informe salga del proceso, en el mensaje se sustituyen las URL, las direcciones de correo electrónico, las menciones `@user@host`, las direcciones IP, los UUID y ULID y los números de 10 dígitos o más por `<url>`, `<email>`, `<acct>`, `<ip>`, `<id>` y `<n>`, y se recorta a 1 KiB.
La traza de pila conserva las posiciones de archivo pero no los valores de los argumentos de sus líneas de función (se muestran como `(...)`), y se recorta a 16 KiB. Ningún informe lleva cuerpo de solicitud, cabecera, consulta, cookie, cuenta, nombre de usuario, dirección IP ni User-Agent.
Ninguna otra línea lleva una traza de pila, así que Error Reporting encuentra solo las trazas de pila de estas líneas.

El nombre de un campo es una única clave que contiene `.`; no está anidado. Entrecomíllalo en los filtros y extracciones de Cloud Logging (por ejemplo `jsonPayload."http.route"`, y `EXTRACT(jsonPayload.duration_seconds)` para el valor de una distribución).
Los campos usados como etiquetas toman valores solo de un conjunto finito; `url.path`, `request_id` y `job_id` nunca son etiquetas.

Con `metrics_exporter = emf`, estas dos líneas son las propias líneas EMF. La línea `http request` lleva el valor de `http.server.request.duration`, y la línea `job` lleva los valores de `my.job.duration` y `my.job.wait` (claves con el nombre de las métricas),
además de la directiva `_aws`, `service.name`, `service.version` y `my.role`. Estas tres métricas no se escriben en ninguna otra línea. Los campos que eliges en `metrics_emf_dimensions` se escriben como cadenas, como exige EMF (por ejemplo, la dimensión `http.response.status_code` es `"200"`).
Los registros van a la salida de error estándar y las demás líneas EMF van a la salida estándar. El controlador de registros `awslogs` de ECS envía ambas a CloudWatch Logs, así que las métricas se extraen de las dos.

<!-- source: docker-compose-files df45a902f45b -->
## Archivos de Docker Compose

`deploy/docker/` contiene lo necesario para desplegar una instancia. `compose.yaml` funciona solo, y los demás archivos se apilan encima.

| Archivo | Añade | Úsalo cuando |
| --- | --- | --- |
| `compose.yaml` | El servidor y sus servicios puntuales de base de datos, con SQLite y los archivos multimedia en el volumen `data` | Siempre |
| `compose.postgres.yaml` | PostgreSQL en un contenedor, en el volumen `postgres` | Quieres PostgreSQL en este equipo. Para un servidor PostgreSQL en otro lugar, déjalo fuera y establece `MY_DATABASE` y `MY_DATABASE_URL` en `.env` |
| `compose.s3.yaml` | Archivos multimedia en un bucket compatible con S3; con `COMPOSE_PROFILES=garage` también un almacén [Garage](https://garagehq.deuxfleurs.fr/) de un solo nodo en el volumen `garage` | Los archivos multimedia superan un disco, o los quieres en un bucket. Sin el perfil, el almacén es el externo de la configuración `MY_BLOB_S3_*` |
| `compose.caddy.yaml` | Caddy, con certificados TLS en los puertos 80 y 443 | `MY_HOST` es un dominio público y nada más termina TLS en este equipo |
| `compose.build.yaml` | Compila la imagen desde un checkout | Ejecutas tu propia compilación |

Elige la base de datos y el almacén de archivos multimedia antes del primer arranque: los datos no se mueven solos cuando cambian los archivos
([Copias de seguridad](backup.md) muestra lo que hay que mover).

**Apila los archivos** con opciones `-f` en ese orden, o nómbralos una sola vez en `.env`, para que todos los comandos `docker compose` posteriores
(`exec`, `logs`, `pull` y los de las demás guías) usen los mismos archivos:

```sh
cd deploy/docker
cp example.env .env
# In .env: COMPOSE_FILE=compose.yaml:compose.postgres.yaml:compose.caddy.yaml
```

Un archivo llamado `compose.override.yaml` en el directorio se lee solo cuando ni `-f` ni `COMPOSE_FILE` nombran archivos. Si los usas,
inclúyelo también en la lista.

**Ajustes.** Copia `example.env` a `.env`. Compose lee `.env` para rellenar las variables de sus archivos y entrega el archivo
completo a los contenedores del servidor, así que toda la configuración de la [referencia de configuración](../reference/configuration.md) funciona
al establecerlos ahí. La plantilla los enumera todos, con una frase sobre cada uno.

- La clave de cifrado de claves queda fuera de `.env`, en el archivo `secrets/kek`, y los contenedores la leen como el secreto de Docker
  `/run/secrets/my_kek`, así que nunca se convierte en una variable de entorno. Una línea `MY_KEK` en `.env` detiene el servidor, que
  rechaza `MY_KEK` y `MY_KEK_FILE` a la vez ([gestionar la clave](key-management.md)).
- El servidor se ejecuta como el usuario `65532`, y Compose monta el archivo con los permisos que tiene, así que el archivo debe ser
  legible por otros. Guárdalo en un directorio al que solo tú puedas entrar, como hacen los comandos de `example.env`.
- Los demás secretos llegan a los contenedores como variables de entorno. Para pasar uno como archivo, móntalo con un secreto
  en un `compose.override.yaml` propio y establece su `<NAME>_FILE`, como hace `compose.yaml` con la clave.
- `.env` contiene secretos: `chmod 600 .env`, y guarda una copia con tus copias de seguridad.
- `MY_LISTEN` se queda en `:8080`, ya que los archivos se conectan a ese puerto.

**TLS con Caddy.** Establece `MY_HOST` con un dominio cuyo DNS apunte al equipo, con los puertos 80 y 443 accesibles desde
internet, y quita `MY_WEB_ORIGIN` de `.env`. Caddy obtiene y renueva el certificado por sí mismo en la primera solicitud.
El servidor confía en el `X-Forwarded-For` de los grupos de direcciones por defecto de Docker; establece `MY_TRUSTED_PROXIES` si los tuyos difieren.

**El Garage incluido.** `compose.s3.yaml` con `COMPOSE_PROFILES=garage` necesita los valores que `example.env` enumera al principio.
El servicio puntual `garage-init` prepara el almacén en cada `up`. Se mantiene dentro de la red de Compose, ya que el servidor
sirve los archivos multimedia por sí mismo. Un nodo es un disco: haz una copia de seguridad del volumen `garage` o del bucket ([Copias de seguridad](backup.md#media-in-a-bucket)).

<!-- source: get-the-image 238d00ea9826 -->
## Obtener la imagen

La imagen del servidor es `ghcr.io/mycroblog/my`, pública, para `linux/amd64` y `linux/arm64`, con todos los adaptadores.

| Etiqueta | Apunta a | Úsala para |
| --- | --- | --- |
| `0.1.0` | Esa versión, para siempre | Nombrar una versión |
| `0.1` | El último parche de `v0.1` | Docker Compose, que aplica el esquema en cada `up` |
| `0.1.0-rc.1` | Esa versión preliminar | Probar una versión antes de que salga |

No existe `latest` ni `0`: ambas cruzarían una versión menor, y cada actualización a una versión menor nueva debe hacerse [a través de la anterior](database.md#skipping-releases). Nombra una versión en cada comando; `docker pull ghcr.io/mycroblog/my` a secas falla con `manifest unknown`.
Una versión de parche también puede necesitar `my schema apply` y `my db upgrade`, así que fuera de Docker Compose sigue [Actualización](database.md#upgrading) para cada versión. En producción, fija el digest que indican las notas de la versión:

```text
ghcr.io/mycroblog/my:0.1.0@sha256:...
```

Dónde ejecutarla:

- **Docker Compose**: `deploy/docker/compose.yaml` ejecuta la imagen de `MY_VERSION` en `.env` ([Archivos de Docker Compose](#docker-compose-files)). Para compilarla desde un checkout en su lugar, añade el override `compose.build.yaml` (`docker compose -f compose.yaml -f compose.build.yaml up -d --build`) y establece `MY_VERSION` con un nombre propio, ya que da nombre a tu imagen local y es lo que imprime `my version`.
- **Kubernetes**: descárgala de `ghcr.io` tal cual.
- **Cloud Run**: usa directamente la imagen de `ghcr.io`. Un despliegue resuelve la etiqueta a un digest.
- **AWS (ECS, EKS)**: copia la imagen a Amazon ECR, o usa un pull through cache de ECR, que necesita credenciales de ghcr.io.
- **Una imagen más pequeña**: `docker build --build-arg VERSION=<name> --build-arg GO_TAGS=... -t my:<name> .` en un checkout compila una con menos adaptadores ([etiquetas de compilación](../reference/build-tags.md)). Es tu propia compilación, así que no la cubren las comprobaciones siguientes.

<!-- source: verify-an-image b696c257662d -->
### Verificar una imagen

Cada imagen publicada tiene una procedencia firmada mediante Sigstore, que vincula el digest con el flujo de trabajo y el commit que la compiló, y un SBOM. Con la [GitHub CLI](https://cli.github.com/), y con la sesión iniciada en `ghcr.io` si el comando lo pide:

```sh
gh attestation verify oci://ghcr.io/mycroblog/my@sha256:... -R mycroblog/my \
  --signer-workflow mycroblog/my/.github/workflows/release.yml
docker buildx imagetools inspect ghcr.io/mycroblog/my@sha256:... --format '{{ json .SBOM }}'
```

El primer comando falla a menos que la imagen la haya compilado el flujo de trabajo de publicación de este repositorio. Imprime el commit, que es el de la etiqueta de la versión; la etiqueta `org.opencontainers.image.revision` de la imagen nombra el mismo.

<!-- source: publish-the-source-code-agpl-30 2f3b73ce9dfc -->
## Publicar el código fuente (AGPL-3.0)

`my` se distribuye con la licencia AGPL-3.0-only. La sección 13 de la AGPL-3.0 exige que quien ejecuta el servidor y permite que los usuarios interactúen con él a través de una red
les ofrezca una forma de obtener el código fuente correspondiente del programa que se está ejecutando. Esto se aplica a todas las personas que interactúan con el servidor, no solo a quienes tienen una cuenta.

Indica dónde se publica el código fuente con el ajuste `source_url` (variable de entorno `MY_SOURCE_URL`; consulta la [referencia de configuración](../reference/configuration.md#source_url)).

| Valor | Significado |
| --- | --- |
| Sin establecer | El valor por defecto, `https://github.com/mycroblog/my`. Úsalo cuando ejecutes `my` sin modificar: el `GET /instance` de una versión publicada nombra entonces el código fuente de esa versión, `https://github.com/mycroblog/my/tree/v0.1.0`, y NodeInfo el repositorio |
| Una URL `https:` | Un lugar donde se puede leer el código fuente (el repositorio de un fork, la etiqueta o el archivo de una versión concreta, etc.) |
| Un URI `mailto:` | Una dirección de correo electrónico a la que solicitar el código fuente (por ejemplo `mailto:source@example.org`; se admite `?subject=...`) |

Cualquier otro valor (`http:`, `git@...`, una ruta relativa, una URL con información de usuario o un `#`, un `mailto:` sin dirección, etc.) hace que `my config check` falle, y el servidor no arranca.

El valor aparece en estos lugares:

- `source_url` en el `GET /instance` de la API de cliente
- La sección «Código fuente» de la página de información de la instancia del cliente web (`/about`) y un enlace en la página de inicio (`/`)
- `metadata.sourceCode` en NodeInfo (`/nodeinfo/2.1`). Una URL `https:` también se publica como `software.repository`
  (el esquema de NodeInfo 2.1 solo admite una URL `http(s)` en `software.repository`, así que un valor `mailto:` aparece únicamente en `metadata.sourceCode`)

<!-- source: when-you-fork-or-modify-my be83256fe6a3 -->
### Si haces un fork o modificas `my`

Si ejecutas un `my` modificado (aplicas parches, compilas un fork o cambias dependencias), la URL por defecto ya no apunta al código fuente de lo que se está ejecutando.
Cambia `source_url` por el lugar donde se puede encontrar el código fuente de la versión en ejecución, con tus modificaciones. Apuntar al commit, la etiqueta o el archivo que corresponde a la versión en ejecución, en lugar de al último estado de un repositorio, permite a los usuarios obtener de forma fiable el mismo código fuente.
Si ejecutas una versión oficial sin modificar, conserva el valor por defecto.

<!-- source: when-you-use-mailto 11155a8b71ed -->
### Si usas `mailto:`

- Prepárate para entregar el código fuente correspondiente (incluidos los scripts necesarios para compilarlo) con prontitud cuando alguien lo pida. Solo nombrar una dirección, sin estar preparado para entregarlo, no cumple la obligación.
- Usa una dirección que siga funcionando cuando dejes de operar el servidor o cambie la persona responsable. Una dirección compartida de operaciones es mejor que un buzón personal.
- Asegúrate de que el filtro de spam no descarte las solicitudes.
- Usa una dirección que no te importe hacer pública: cualquiera puede verla en la página de información, la página de inicio y en NodeInfo.

<!-- source: metrics 2dea3a7f8f83 -->
## Métricas

El ajuste `metrics_exporter` (por defecto `none`) exporta las métricas del servicio. Su configuración relacionada está en la [referencia de configuración](../reference/configuration.md).

| `metrics_exporter` | Ruta | Adecuado para |
| --- | --- | --- |
| `none` | No se exporta nada. La instrumentación casi no cuesta | Cloud Run (usa en su lugar las métricas de la plataforma y las métricas basadas en registros) |
| `prometheus` | `GET /metrics` en `metrics_listen` (por defecto `127.0.0.1:9464`). Es un listener aparte del `listen` público, sin autenticación, y se ejecuta con cualquier `role` | Autoalojado (consultado por un servidor compatible con Prometheus o por KEDA) |
| `otlp` | Se envía a un OpenTelemetry Collector por OTLP/HTTP. El destino procede de las variables de entorno estándar de OTel, como `OTEL_EXPORTER_OTLP_ENDPOINT` | Configuraciones en las que puedes ejecutar un colector |
| `emf` | Líneas JSON en el formato de métricas incrustadas (Embedded Metric Format) de CloudWatch. Las métricas de solicitudes y trabajos van en las líneas de cierre de registro anteriores; las demás se escriben en la salida estándar, una línea por conjunto de etiquetas, cuando termina una solicitud o un trabajo | AWS ECS |

Las métricas se enumeran a continuación. En Prometheus, `.` en un nombre se convierte en `_` y se añade un sufijo de unidad (`_seconds`, y `_total` para un contador).

| Métrica | Tipo | Etiquetas |
| --- | --- | --- |
| `http.server.request.duration` | Histograma, segundos | `http.request.method`, `http.route` (la ruta del patrón de ruta; `other` cuando ninguno coincide), `http.response.status_code`, `my.surface` (`client_api` / `admin` / `activitypub` / `oauth` / `media` / `web` / `payments` / `voice` / `queue`) |
| `http.server.active_requests` | UpDownCounter | `my.surface` |
| `my.inbox.activities` | Contador | `activity.type` (un tipo de Activity conocido, o `other`), `my.outcome` (`accepted` / `duplicate` / `rejected` / `ignored` / `error`), `my.inbox.rejection` (solo con `rejected`: `signature` para una firma o prueba de integridad que falta, no es válida o no se verifica, o cuya clave no se pudo obtener; `suspended` para un dominio suspendido; `federation_off`; `rate_limited`; `invalid` para una actividad sin un `id`, `type`, `actor` u `object` válido) |
| `my.relay.activities` | Contador | `my.outcome` (`admitted` / `over_limit` / `restricted` / `not_receiving`: si una actividad transportada por un relé se ingirió, y por qué se descartó; consulta [Relés de federación](relays.md)) |
| `my.move.activities` | Contador | `my.outcome` (`recorded` / `duplicate` / `not_own_actor` / `same_account` / `target_unavailable` / `alias_missing` / `target_moved` / `suspended` / `too_soon`: si un `Move` recibido se registró, y por qué se descartó) |
| `my.delivery.attempts` | Contador | `my.outcome` (`success` / `retry` / `gone` / `failed`; `gone` cubre un 410 y las entregas omitidas porque el servidor no está disponible o está suspendido) |
| `my.delivery.duration` | Histograma, segundos | `my.outcome` |
| `my.plugin.failures` | Contador | `my.plugin.kind` (un tipo de [plugin](plugins.md)), `my.plugin.code` (el código Connect del fallo) |
| `my.plugin.slot_wait` | Histograma, segundos | `my.plugin.kind` (cuánto esperó una llamada su turno entre las llamadas de ese tipo que se ejecutan a la vez) |
| `my.hook.decisions` | Contador | `hook`, `mode`, `outcome` (las decisiones que registró un [gancho de decisión](decision-hooks.md)) |
| `my.hook.failures` | Contador | `hook`, `outcome` (`timeout`, `error`, `invalid` o `breaker_open`) |
| `my.hook.duration` | Histograma | `hook` (segundos por llamada al plugin) |
| `my.job.duration` | Histograma, segundos | `my.job.type` (un tipo de trabajo registrado, o `other`), `my.outcome` (`success` / `retry` / `dead` / `postponed`: `postponed` es una ejecución que no hizo trabajo y no usa ninguno de los intentos del trabajo, como la consulta de una tarjeta de enlace que espera a su host) |
| `my.job.wait` | Histograma, segundos | `my.job.type` |
| `my.link_card.host_pauses` | Contador | `http.response.status_code` (`429` / `503`). Una por cada pausa de las consultas de tarjetas de enlace a un host; consulta [Vistas previas de enlaces](link-previews.md#limits-per-host) |
| `my.translation.requests` | Contador | `my.outcome` (`author` / `cache` / `provider` / `no_provider` / `limit` / `unsupported` / `failed`). Una por cada solicitud de traducir una publicación que el espectador puede ver, o la publicación o el comentario de una denuncia que pide un moderador: respondida con las propias palabras del autor, con una traducción guardada o con un proveedor, o rechazada porque ningún proveedor puede recibir la publicación, se alcanzó un límite mensual, ningún proveedor traduce el idioma o ningún proveedor devolvió una traducción utilizable. |
| `my.translation.characters` | Contador | `my.provider` (el ID de un proveedor de traducción). Los caracteres de publicaciones y comentarios de denuncias enviados al proveedor, sin marcadores, incluidas las llamadas que fallan. Es lo que cuentan los [límites mensuales](instance-settings.md#admin-screens-and-the-client-api), y puede diferir de lo que factura el proveedor |
| `my.timeline.delivery.lag` | Histograma, segundos | Ninguna (desde la creación de una publicación local hasta su escritura en las cronologías de inicio y la notificación a los flujos) |
| `my.stream.connections` | UpDownCounter | `my.stream.kind` (`user`, `timeline` para los flujos de las cronologías local y federada, `voice` para los flujos de las salas de chat de voz) |
| `my.accounts.registered` | Contador | Ninguna |
| `my.statuses.created` | Contador | `my.visibility` |
| `my.billing.imports` | Contador | `my.billing.source` (`aws` / `gcp` / `plugin`), `my.outcome` (`success` / `failed`), `error.type` (solo con `failed`: el tipo de fallo, como en [Importar los cargos de uso de la nube](billing-import.md)). Cuenta todos los intentos, incluidos los reintentos y los fallos que esperan a la siguiente importación diaria |
| `my.voice.turn.credentials` | Contador | `my.voice.turn` (`coturn` / `cloudflare`), `my.outcome` (`issued` / `reused` / `failed`). Una por cada credencial TURN que pidió un participante de un chat de voz al unirse a una sala o renovar su conexión: `reused` significa que se devolvieron de nuevo las credenciales de Cloudflare emitidas antes para la misma conexión, y `failed` significa que no se pudo contactar con Cloudflare o este lo rechazó, así que el participante recibió un error y no pudo conectarse |
| `my.errors.exported` | Contador | `my.outcome` (`success` / `dropped` / `failed`: una por cada informe de error para el servicio de `errors_exporter`. `dropped` significa que el informe no se envió porque ya había 1000 informes esperando o el servicio pidió al servidor que redujera el ritmo, y el servidor espera todo el tiempo que pida el servicio antes de volver a enviar; `failed` significa que no se pudo contactar con el servicio o este rechazó el informe. Un informe se envía una sola vez y nunca se reintenta, y los informes que siguen esperando 2 segundos después de iniciar un apagado se pierden) |
| `my.queue.ready` | Gauge | `my.job.type` (el número de trabajos ejecutables en la cola de la base de datos) |
| `my.queue.oldest_ready_age` | Gauge, segundos | `my.job.type` (cuánto lleva esperando el trabajo ejecutable más antiguo de cada tipo en la cola de la base de datos; un tipo sin trabajos ejecutables no tiene valor) |
| `my.queue.dead` | Gauge | Ninguna |
| `my.instance.users`, `my.instance.statuses` | Gauge | Ninguna (los recuentos de usuarios y publicaciones locales, como en NodeInfo) |
| `my.media.stored_bytes` | Gauge, bytes | `my.media.origin` (`local` / `remote_cache`) |
| `my.db.size` | Gauge, bytes | Ninguna (el tamaño de la base de datos: `pg_database_size` en PostgreSQL; el archivo de la base de datos sin su registro de escritura anticipada en SQLite) |
| `my.billing.import.failing` | Gauge | `my.billing.source` (1 cuando la importación más reciente de la fuente falló, 0 en caso contrario) |
| `my.federation.instances` | Gauge | `my.federation.instance.state` (`available`: servidores a los que aún se hacen entregas pero que fallaron al menos un día desde que respondieron por última vez; `unavailable`: servidores cuyas entregas están en pausa) |
| `my.metrics.snapshot.age` | Gauge, segundos | `my.metrics.snapshot.part` (`queue` / `totals`) |
| `my.errors.reported` | Contador | `my.error.source` (`server` / `web`), `my.error.kind` (`panic` / `error`). Una por cada [informe de error](#error-reports) |

Las comprobaciones de salud (`/healthz`, `/readyz`) no se miden. Ninguna etiqueta contiene un dominio remoto, una cuenta, una URL ni una IP de cliente.
`my.queue.*`, `my.instance.*`, `my.media.stored_bytes`, `my.db.size`, `my.federation.instances` y `my.billing.import.failing` son valores de toda la instancia: todos los procesos informan el mismo valor a partir de una instantánea guardada en el almacén clave-valor.

<!-- source: system-metrics 8b5d40a4eb98 -->
### Métricas del sistema

Las métricas sobre el proceso, el runtime de Go y las conexiones a recursos se exportan mediante el mismo `metrics_exporter`.
La CPU del contenedor, la memoria, el número de instancias y las solicitudes de entrada las cubren las métricas de la plataforma; estas cubren la saturación dentro del proceso que la plataforma no puede ver.
No las uses para activar el escalado; úsalas para alertas y para estimar el tope de instancias.

| Métrica | Tipo | Etiquetas |
| --- | --- | --- |
| `go.memory.used`, `go.memory.limit`, `go.memory.gc.goal` | Gauge, bytes | Solo `go.memory.used` tiene `go.memory.type` (`stack` / `other`). `go.memory.limit` aparece solo cuando `GOMEMLIMIT` está establecido |
| `go.memory.allocated`, `go.memory.allocations` | Contador | Ninguna |
| `go.goroutine.count`, `go.processor.limit`, `go.config.gogc` | Gauge | Ninguna |
| `go.schedule.duration`, `go.memory.gc.pause.duration` | Histograma, segundos | Ninguna (la espera desde que pasa a ser ejecutable hasta que se ejecuta, y las pausas del GC) |
| `go.memory.gc.cycles` | Contador | Ninguna |
| `process.cpu.time` | Observable Counter, segundos (total desde el arranque) | `cpu.mode` (`user` / `system`) |
| `process.memory.usage` | Gauge, bytes | Ninguna (memoria residente, RSS) |
| `process.unix.file_descriptor.count`, `my.process.file_descriptor.limit` | Gauge | Ninguna (descriptores de archivo abiertos y el límite) |
| `process.uptime` | Gauge, segundos | Ninguna |
| `db.client.connection.count` | Gauge | `db.client.connection.pool.name` (`postgres` / `sqlite`), `db.client.connection.state` (`idle` / `used`) |
| `db.client.connection.max` | Gauge | `db.client.connection.pool.name` |
| `db.client.connection.wait_time`, `my.db.client.connection.waits`, `db.client.connection.timeouts` | Observable Counter (total desde el arranque; segundos o recuento) | `db.client.connection.pool.name`. `timeouts` es solo de PostgreSQL |
| `my.adapter.operation.duration` | Histograma, segundos | `my.adapter` (`queue.db` / `kv.db` / `kv.memory` / `kv.valkey` / `stream.<stream>` / `blob.fs`), `my.operation`, `my.outcome` (`success` / `error`) |
| `my.adapter.connections` | UpDownCounter | `my.adapter` (conexiones de `valkey` y suscriptores de `stream.<stream>`) |
| `my.adapter.reconnects` | Contador | `my.adapter` |
| `http.client.request.duration` | Histograma, segundos | `my.http.purpose` (`json` / `html` / `image` / `video` / `human_check` / `web_push` / `webhook` / `client_metadata` / `login_provider` / `robots`), `http.request.method`, `my.http.status_class` (`2xx` y así sucesivamente), `error.type` (`forbidden_destination` es un rechazo del guardián SSRF) |
| `http.client.open_connections` | UpDownCounter | Ninguna |
| `my.readiness.failures` | Contador | Ninguna |
| `my.http.origin_refused` | Contador | Ninguna. Una por cada solicitud rechazada porque no pasó por el CDN ([`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn)). Los sondeos a la dirección del origen la aumentan despacio; un salto mientras el tráfico se detiene significa que el CDN ya no envía la cabecera. El modo de mantenimiento no mide nada, así que solo su registro de acceso los muestra |

- Las métricas del runtime de Go (`go.*`) las exportan solo `prometheus` y `otlp`. Con `emf`, un trabajo periódico de instantáneas escribe los gauges y los observable counters del proceso y del pool de conexiones.
- Las esperas del pool y el tiempo de CPU son Observable Counters: el pool y el kernel guardan totales desde el arranque, y se leen en el momento de la recolección. En Prometheus son contadores `_total` (como `process_cpu_time_seconds_total`), en OTLP son sumas monótonas, y su crecimiento se ve con `rate()` de PromQL.
  `emf` no tiene contadores de totales, así que escribe el incremento desde la escritura anterior (súmalos con la estadística Sum de CloudWatch).
- El `claim` de la cola se mide solo cuando toma un trabajo o falla. El sondeo de un worker en espera no registra nada.
- `db.client.connection.pending_requests` no se exporta: las estadísticas de pgx y de `database/sql` no informan de cuántas solicitudes esperan, así que el número y la duración de las esperas lo sustituyen.
- Crea alertas sobre la proporción de la suma de `go.memory.used` respecto a `go.memory.limit`, y sobre el crecimiento de `db.client.connection.wait_time` (se está alcanzando el límite del pool).

<!-- source: memory-limit-gomemlimit 8ea44722eefd -->
### Límite de memoria (`GOMEMLIMIT`)

`GOMEMLIMIT` es una variable de entorno del runtime de Go que establece el operador; la aplicación no la elige.
Sin ella, Go se ejecuta con su valor por defecto (sin límite), y el heap no se recolecta con antelación al acercarse al límite de memoria del contenedor.
En configuraciones con un límite de memoria del contenedor (Cloud Run, ECS, Kubernetes), establécela en torno al 90 % de ese límite (por ejemplo `GOMEMLIMIT=460MiB` para un contenedor de 512 MiB).
No establezcas `GOMAXPROCS`: el valor por defecto de Go sigue el límite de CPU del contenedor.

<!-- source: traces e274793f5885 -->
## Trazas

El ajuste `traces_exporter` (por defecto `none`) exporta trazas distribuidas. Su configuración relacionada está en la [referencia de configuración](../reference/configuration.md).

| Span | Nombre | Atributos |
| --- | --- | --- |
| Solicitud HTTP (servidor) | `<method> <http.route>` (`<method>` para una solicitud que no llegó a una ruta) | `http.request.method`, `http.route`, `http.response.status_code`, `my.surface`. Un 5xx es un fallo |
| Ejecución de un trabajo (consumer) | El tipo de trabajo (`my.job.type`) | `my.job.type`, `my.job.attempt`, `my.outcome`. Cada reintento es un span aparte |
| Transacción | `my.db.transaction` | `db.system.name`. Cada reintento es un span aparte |
| Consulta (cliente) | El nombre de la consulta de sqlc (como `InsertJob`; si no, la primera palabra del SQL, como `BEGIN`) | `db.system.name`, `db.operation.name` |
| Entrega (`my.delivery`) | `my.delivery` | `activity.type`, `my.outcome` |
| HTTP saliente (cliente) | `<method>` | `http.request.method`, `server.address` (solo el nombre del host de destino), `http.response.status_code`. Un 4xx o 5xx es un fallo |

Un span fallido tiene `error.type` (un nombre finito como `timeout`, o un código de estado). Los atributos nunca contienen ID de cuentas, publicaciones o trabajos, URL, rutas o consultas, IP de cliente, valores de User-Agent, texto SQL ni cadenas de error.
`server.address` deja los nombres de host de los servidores remotos en manos del receptor, así que tú decides cuánto tiempo los conserva el receptor y quién puede verlos. Las comprobaciones de salud y las solicitudes rechazadas por `origin_secret` no crean ningún span.

- La ejecución de un trabajo no es hija del span que lo puso en cola (normalmente una solicitud HTTP). Es la **raíz de una traza nueva** con un enlace al span que lo puso en cola. En un visor, sigue el enlace desde la traza de la solicitud.
- Un `traceparent` entrante pasa a ser el padre solo cuando el par que se conecta está en `trusted_proxies` y, con [`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn), la solicitud lleva el secreto. En caso contrario se descarta y empieza una traza nueva. `tracestate` se descarta siempre. El HTTP saliente (federación, WebFinger, archivos multimedia, Web Push, etc.) no lleva `traceparent`.
- Muestreo: una solicitud se registra si se registró su padre de proxy de confianza, o si se registró el destino del enlace de un trabajo; en caso contrario se registra con la proporción `traces_sample_ratio` (por defecto 0,01). Reducir la proporción nunca deja sin su traza a los trabajos que continúan una solicitud registrada.

| Destino | Ajustes | Ruta |
| --- | --- | --- |
| Por defecto, autoalojado pequeño | `traces_exporter = none` | No se exporta ningún span. Sigue `request_id` y `job_id` en los registros. Si un proxy de confianza aporta un `traceparent`, su `trace_id` y `span_id` se añaden a los registros |
| Autoalojado (Jaeger, Tempo, un OpenTelemetry Collector, etc.) | `traces_exporter = otlp`, `traces_flush = batch` | Se envía por OTLP/HTTP al receptor en `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` (o `OTEL_EXPORTER_OTLP_ENDPOINT`, por defecto `http://localhost:4318`). Las cabeceras de autenticación van en `OTEL_EXPORTER_OTLP_HEADERS`. Para conservar de forma fiable los errores y las trazas lentas, usa el muestreo en cola (tail sampling) del Collector |
| Google Cloud (Cloud Run) | `none`, u `otlp` con `unit` para activar las trazas | Consulta más abajo |
| AWS (ECS) | `none`, u `otlp` con `batch` para activar las trazas | Consulta más abajo |

<!-- source: cloud-run 7f1a21bbfbef -->
### Cloud Run

Incluso con `none`, la traza de solicitud que Cloud Run crea automáticamente y el `trace_id` de los registros comparten el mismo ID (cuando confías con `trusted_proxies` en la conexión que viene delante de Cloud Run).
Aún no se ha verificado si Cloud Run sobrescribe siempre el `traceparent` de las solicitudes públicas.
Los registros no tienen la forma del campo de traza que reconoce Cloud Logging (`logging.googleapis.com/trace`).

Para enviar spans de la aplicación a Cloud Trace, ejecuta un **OpenTelemetry Collector como sidecar** en el mismo servicio. El servidor no guarda credenciales de Google; el sidecar envía al endpoint OTLP de Cloud Trace con las Application Default Credentials.

- Servidor (el contenedor de ingreso): `MY_TRACES_EXPORTER=otlp`, `MY_TRACES_FLUSH=unit`, `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`
- Sidecar: un OpenTelemetry Collector que recibe con un receptor `otlp` por HTTP en `localhost:4318` y envía con el exportador `googlecloud`, o el exportador `otlphttp` (`https://telemetry.googleapis.com`, con la extensión `googleclientauth`). Concede a la cuenta de servicio permiso para escribir en Cloud Trace.
- Al final de cada solicitud o trabajo registrado, `unit` vacía los spans acumulados hacia el sidecar antes de volver. Para que el sidecar pueda enviarlos mientras no hay CPU asignada, usa la facturación basada en instancia en una configuración con trazas activadas.
- El costo son los recursos del sidecar y el volumen de spans (precios de Cloud Trace). Elige la proporción con `traces_sample_ratio`.

<!-- source: aws-ecs 52fa09412f20 -->
### AWS ECS

Para enviar a X-Ray, ejecuta el **AWS Distro for OpenTelemetry (ADOT) Collector** como contenedor sidecar en la tarea. El servidor no firma con SigV4; el Collector firma y envía al endpoint OTLP de X-Ray (`https://xray.<region>.amazonaws.com/v1/traces`).

- Servidor: `MY_TRACES_EXPORTER=otlp`, `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318`. En la configuración del Collector, recibe con un receptor `otlp` en `localhost:4318` y envía a X-Ray con un exportador `otlphttp` que use la extensión `sigv4auth`.
- Concede al rol de la tarea permiso para escribir en X-Ray.
- El costo es la parte de los recursos de la tarea que usa el sidecar y el número de trazas que registra X-Ray (las primeras 100 000 al mes son gratuitas).

<!-- source: stop-the-server 37d4c0cc3d97 -->
## Detener el servidor

Con SIGTERM o SIGINT el servidor se apaga de forma ordenada.

1. Deja de aceptar conexiones nuevas y de tomar trabajos.
2. Espera hasta 25 segundos a que terminen las solicitudes en curso y los trabajos en ejecución.
3. Pasados 25 segundos cancela el contexto de las solicitudes y trabajos restantes y cierra las conexiones.
   Después da a un trabajo cancelado hasta 3 segundos más para registrar su resultado y termina con el código 1 (28 segundos como máximo en total, dentro del periodo de gracia de 30 segundos por defecto de Kubernetes y ECS).
   Un trabajo cancelado se reintenta. Un trabajo que no se detiene en esos 3 segundos, o que se mata a medias, se abandona y un worker lo vuelve a tomar cuando caduca su concesión, así que puede ejecutarse dos veces.

Un pánico en una de las tareas en segundo plano del servidor inicia el mismo apagado, para que el servidor nunca siga funcionando en un estado roto.
La línea de registro `serve failed` nombra la tarea, y el proceso termina con el código 1 para que la plataforma o el gestor de procesos lo reinicie.

Para trabajar en la base de datos, el almacén clave-valor o la cola, reinicia en modo de mantenimiento (`MY_MAINTENANCE=true`) en lugar de detenerlo. Consulta [Mantenimiento](maintenance.md).

<!-- source: the-job-queue-and-failed-dead-jobs 55023b12a1aa -->
## La cola de trabajos y los trabajos fallidos (dead)

[Trabajos](../reference/jobs.md) enumera todos los tipos de trabajo: qué hace cada uno, qué lo inicia, cómo se reintenta y qué ocurre al ejecutarlo de nuevo.

La página de trabajos de administración (`/admin/jobs`, permiso `manage_jobs`) tiene tres páginas:

- **Estado de la cola** (`/admin/jobs`): el recuento por estado y por tipo y estado, cuánto tiempo mantiene cada tipo a un trabajo en espera y cuánto lleva esperando el trabajo en espera más antiguo.
- **Trabajos** (`/admin/jobs/list`): una tabla de los trabajos en un estado (pendiente, en espera, en ejecución, a la espera de reintento, fallido), una página cada vez, con el tipo, el estado, la hora programada o del fallo y el número de intentos. Los trabajos fallidos tienen **Ejecutar de nuevo**.
- **Un trabajo** (`/admin/jobs/<id>`, desde el tipo en la tabla): todos los datos del trabajo, su último error, claves y carga útil, el historial de intentos fallidos (resultado, error, worker, y hora de inicio y fin de cada intento) y **Ejecutar de nuevo** para un trabajo fallido.

La API de administración es `GET /api/v1/admin/queue`, `GET /api/v1/admin/jobs?state=&type=`, `GET /api/v1/admin/jobs/{id}`, `GET /api/v1/admin/jobs/{id}/attempts` y `POST /api/v1/admin/jobs/{id}/retry`.
Un trabajo completado ya no está en la cola, así que su página no se encuentra.

- Los trabajos en espera, en ejecución y a la espera de reintento solo se pueden enumerar en las colas cuyos trabajos guarda el servidor: `queue = db`, `queue = cloudtasks` (los trabajos también están en la base de datos) y `queue = inline`. Los trabajos fallidos y el historial de intentos están en la base de datos de la aplicación para todas las colas, así que siempre se pueden enumerar.
- El historial de intentos conserva solo los intentos fallidos (los reintentos y el intento que finalmente falló). Sigue una ejecución correcta mediante la línea de registro estructurada `job` (que lleva `job_id`).
  La limpieza horaria elimina el historial una vez que han pasado `job_attempt_retention` (por defecto 30 días; consulta el [ajuste](../reference/configuration.md#job_attempt_retention)) desde que terminó el intento.
  El historial no contiene carga útil ni registro del manejador, y un mensaje de error se recorta a sus primeros 2000 bytes. Los manejadores de trabajos mantienen las direcciones de correo electrónico, las direcciones IP, los tokens y los cuerpos de publicaciones fuera de los mensajes de error.
  Para una vista limitada a los tipos de trabajo de correo electrónico, consulta [Envío de correo electrónico](email.md).

Un trabajo cuyo manejador decidió que ha fallado definitivamente, y un trabajo que agotó sus reintentos, pasa a ser un trabajo dead en la cola de la base de datos (`queue = db` y `queue = cloudtasks`).
`my job` los enumera, reintenta y elimina. El estado «Fallido» de la página de trabajos y la API de administración también pueden enumerarlos y reintentarlos.

```sh
my job list                  # newest failures first: ID, type, run count, time of failure, last error (-limit sets the count, default 50)
my job retry <id> [<id>...]  # reset the attempt count and put the job back in the queue to run immediately
my job delete <id> [<id>...] # remove without running
my job types                 # print every job type of this release
```

- Si ya hay en espera un trabajo con la misma clave de idempotencia, ese trabajo sustituye al reintentado. Los manejadores de trabajos son idempotentes, así que volver a poner el mismo trabajo cualquier número de veces es seguro.
- Un ID desconocido se imprime y el comando termina con el código 1; los demás ID se procesan igualmente. El código de salida 2 indica un error de uso.
- Un trabajo dead que nadie reintenta lo elimina la limpieza horaria una vez que han pasado `dead_job_retention` (por defecto 30 días; consulta el [ajuste](../reference/configuration.md#dead_job_retention)) desde el fallo.
- Los trabajos dead de `queue = inline` viven en la memoria del proceso del servidor, así que `my job` no puede gestionarlos (código de salida 1).

<!-- source: splitting-workers-by-job-type 89ced36a3c80 -->
### Dividir los workers por tipo de trabajo

Por defecto cada worker ejecuta todos los tipos de trabajo, en el orden en que vencieron, un trabajo a la vez por proceso (consulta [Ejecutar varios trabajos a la vez](#running-several-jobs-at-once)). Entonces una ráfaga de entregas retrasa el correo electrónico y las notificaciones, y cualquier worker que añadas reparte su tiempo entre todos. Con `queue = db` puedes dar a algunos tipos sus propios workers:

- [`worker_job_types`](../reference/configuration.md#worker_job_types): este proceso ejecuta solo estos tipos.
- [`worker_job_types_except`](../reference/configuration.md#worker_job_types_except): este proceso ejecuta todos los tipos menos estos.

Cada uno es una lista separada por comas de [tipos de trabajo](../reference/jobs.md) (`my job types` los imprime; `my config check` rechaza un nombre que no coincide con ninguno), escritos como un nombre (`federation.deliver`) o un prefijo (`federation.*`). Establécelos por proceso, por ejemplo con `MY_WORKER_JOB_TYPES`. No puedes establecer ambos en un mismo proceso. Un nombre o prefijo que no coincide con ningún tipo de trabajo detiene el arranque, lo que también detecta un tipo que una versión renombró, o un tipo que solo existe con un ajuste que no has establecido. No funcionan con `role = web`, ni con `queue = cloudtasks` o `inline`, donde el arranque también falla: allí una señal nombra el trabajo que ejecutar, así que un proceso no puede elegir sus trabajos.

Al arrancar, un proceso con uno de estos valores registra los tipos que omite. No puede ver los valores de los demás procesos, así que **asegúrate de que algún proceso ejecute todos los tipos**, o esos trabajos esperarán para siempre. La limpieza periódica y el correo electrónico también son tipos de trabajo. Todos los procesos siguen iniciando los trabajos periódicos; qué proceso los ejecuta depende de esta configuración.

Decide qué dividir a partir de las métricas, no de los nombres de los tipos. `my.job.wait` (Prometheus: `my_job_wait_seconds`) muestra cuánto esperaron los trabajos por tipo antes de empezar:

```promql
histogram_quantile(0.95, sum by (my_job_type, le) (rate(my_job_wait_seconds_bucket[15m])))
```

`sum by (my_job_type) (rate(my_job_duration_seconds_sum[1h]))` muestra el tiempo de worker que usa cada tipo, que es también aproximadamente cuántos procesos necesita. Un tipo que usa la mayor parte del tiempo y hace esperar a los demás es el que hay que mover.

Tres disposiciones habituales, con el número de procesos que elijas:

| Disposición | Procesos |
| --- | --- |
| Evitar que las entregas retrasen el resto | `worker_job_types = federation.fanout,federation.deliver` para las entregas, y `worker_job_types_except = federation.fanout,federation.deliver` para todo lo demás. Añade procesos al primer grupo durante una ráfaga. |
| Evitar que un backend de búsqueda retrase el resto | `worker_job_types = search.*` para los trabajos de búsqueda, y `worker_job_types_except = search.*` para todo lo demás ([Evitar que la búsqueda retrase otros trabajos](search.md#keeping-search-from-delaying-other-jobs)). |
| Mantener rápidos el correo electrónico y las notificaciones | `worker_job_types = identity.verification_email,identity.password_reset_email,identity.password_notice_email,identity.closure_email,identity.deactivation_email,notification.email,notification.moderator_email,notification.supporter_email,notification.push,notification.post_subscribers,instance.notify_announcement,voice.notify_started` para estos, y un worker sin ese ajuste para todo, que además ayuda con estos tipos cuando está libre. Deja fuera los tipos que no has establecido, o el arranque se detiene: `notification.push` necesita `vapid_private_key`, y `notification.email`, `notification.moderator_email` y `notification.supporter_email` necesitan `mail`. Consulta [Trabajos que dependen de configuración](../reference/jobs.md#jobs-that-depend-on-settings). |

Un prefijo como `identity.*` es el módulo de código que posee los tipos, no una clase de trabajo. `identity.*` incluye también las exportaciones e importaciones de datos y los trabajos de caducidad, y no incluye `instance.notify_announcement` ni `voice.notify_started`, así que una disposición para una clase de trabajo nombra cada tipo.

Mantén estos juntos, en workers con el mismo ajuste. Los trabajos de una misma clave de orden se ejecutan en orden sea cual sea su tipo, así que si nada ejecuta el trabajo anterior, el posterior también espera, y si el grupo del trabajo anterior está saturado, el trabajo posterior lo espera:

| Tipos | Qué mantienen en orden |
| --- | --- |
| `federation.fanout`, `federation.deliver` | Las entregas de un favorito y de su deshacer |
| `voice.apply_media_event`, `voice.apply` | Los eventos de una sala de voz |
| `supporters.sync`, `supporters.command` | Los cambios de una suscripción |

Cuando actualices:

- Un proceso con `worker_job_types` no ejecuta los tipos que añade una versión nueva. Consulta las notas de la versión y los tipos omitidos en el registro. Un proceso con `worker_job_types_except` sí los ejecuta, lo que lo convierte en el lugar más seguro para todo lo demás.
- Un tipo renombrado detiene el arranque, así que lee primero las notas de la versión.

Para desplegar una división, arranca primero los workers nuevos y después añade el ajuste a los antiguos. Para deshacerla, quita el ajuste de un grupo antes de detener el otro. Los trabajos permanecen en la cola, así que no se pierde nada, y un tipo que nadie ejecuta durante un tiempo solo espera.

**Detectar un tipo que nada ejecuta.** Los trabajos de ese tipo esperan, así que `my.queue.oldest_ready_age` crece para ese tipo, y las alertas `MyQueueBacklog` y `MyQueueStalled` saltan para el tipo que espera más. Para ver cuál es, o para crear alertas por tipo:

```promql
max by (my_job_type) (my_queue_oldest_ready_age_seconds) > 1800
```

La página de trabajos de administración muestra la misma espera por tipo. Un worker con uno de estos valores también deja en paz un trabajo cuyo tipo no tiene ningún manejador (se cuenta como `other` en la métrica), así que, sin ningún worker sin ajuste, ese trabajo espera en lugar de convertirse en un trabajo dead.

<!-- source: running-several-jobs-at-once 91e5181261b2 -->
### Ejecutar varios trabajos a la vez

[`worker_job_concurrency`](../reference/configuration.md#worker_job_concurrency) establece cuántos trabajos ejecuta un proceso a la vez, de 1 a 128. Funciona con `queue = db`; con Cloud Tasks lo decide la plataforma. El valor por defecto es un trabajo a la vez, y un ajuste de 1 se comporta igual.

Súbelo para los trabajos que pasan el tiempo esperando a otros servidores, como `federation.deliver`. Esos usan poca memoria y pocas conexiones de base de datos por trabajo, así que un proceso hace el trabajo de muchos: en una prueba local en la que un trabajo solo esperaba 200 ms una respuesta, un proceso con 32 ranuras igualó a 32 procesos con una ranura cada uno, usando una pequeña parte de la memoria. Esos trabajos no hacían nada más, así que mide tu propia instancia. Combínalo con `worker_job_types` y da a cada clase de trabajo un proceso con su propio número.

Ten presentes estos límites:

- **Conexiones de base de datos**: un trabajo necesita una conexión para ser reclamado, para su propio trabajo y para registrar su resultado. Con `database = postgres` el ajuste debe ser inferior al máximo del pool, o el servidor no arranca. El máximo es `pool_max_conns` en `database_url`, por defecto el mayor entre 4 y el número de CPU, así que una vCPU permite 3 hasta que lo subas. Comprueba que tu base de datos o tu agrupador de conexiones acepta las conexiones de todos tus procesos.
- **SQLite**: escribe una transacción a la vez. Por encima de 32 el servidor registra una advertencia, y en una prueba local SQLite fue más lento con 128 que con 32.
- **Trabajos que consumen mucha CPU**: las ranuras comparten la CPU y la memoria de un proceso. `media.process` espera su turno con un límite de la mitad de las CPU por defecto, y la espera cuenta contra la concesión de 5 minutos del trabajo, así que un proceso lleno de `media.process` puede pasarse de la concesión y empezar de nuevo. Da a esos trabajos un proceso con pocas ranuras.
- **Un servidor a la vez**: un proceso hace como máximo 8 conexiones a un mismo host, así que un proceso entrega a un único servidor no más rápido de lo que permiten 8 conexiones, tenga las ranuras que tenga. Cuando la mayoría de las entregas van a unos pocos servidores grandes, más procesos ayudan donde más ranuras no.
- **Un servidor atascado retiene ranuras**: las ranuras que esperan a un servidor lento no quedan libres para otros trabajos, así que mantén las entregas separadas del correo electrónico y las notificaciones.
- **Los despliegues consumen intentos**: cuando el servidor se detiene, los trabajos que siguen en ejecución tras `ShutdownTimeout` (25 segundos por defecto) se cancelan, y cada trabajo cancelado consume un intento. Cuantas más ranuras ejecutan trabajos lentos, más intentos consume cada despliegue. Un trabajo cancelado cuyo resultado no se registra a tiempo espera a que caduque su concesión antes de ejecutarse de nuevo.
- **Un fallo se lleva todas las ranuras**: si el proceso se queda sin memoria, todos sus trabajos esperan a que caduquen sus concesiones.

Para decidir si subirlo, lee `my.job.wait` por `my.job.type`: si los trabajos de un tipo esperan mucho mientras `my.job.duration` es sobre todo tiempo de espera a otros servidores y la CPU y la memoria se mantienen bajas, más ranuras deberían ayudar. Si las esperas siguen altas tras subirlo, mira las conexiones de base de datos y el límite por host anteriores antes de volver a subirlo.

Mientras ningún trabajo vence, una ranura sondea y las demás esperan, así que un proceso inactivo cuesta a la base de datos lo mismo que un proceso de un solo trabajo.

<!-- source: waiting-for-a-database-connection f481d577ea58 -->
### Esperar una conexión de base de datos

Con `database = postgres`, cada sentencia y transacción espera primero una conexión libre del pool del proceso.
[`database_acquire_timeout`](../reference/configuration.md#database_acquire_timeout) (por defecto `60s`) limita esa espera, y nada más: una consulta o
transacción larga que ya tiene su conexión sigue ejecutándose, tarde lo que tarde. `0` espera sin límite. SQLite no tiene límite de conexiones por las que esperar, así que establecerlo ahí falla al arrancar.

Cuando la espera se agota:

- Una solicitud recibe como respuesta `503 Service Unavailable` con `Retry-After: 5`.
- Un trabajo se ejecuta de nuevo 30 segundos después y no consume ningún intento, así que una base de datos ocupada nunca convierte un trabajo en un trabajo dead.
- La métrica `db.client.connection.timeouts` lo cuenta, y salta la alerta [`MyDBPoolWaiting`](alerts.md#mydbpoolwaiting).

El valor por defecto es el tiempo de espera de escritura de 60 segundos del servidor: una solicitud que esperara más no podría enviar su respuesta de todos modos, así que el valor por defecto no hace fallar ninguna solicitud que aún pudiera haber tenido éxito.
Elige un valor según cómo funcione tu instancia:

- **Redúcelo** para descartar carga rápidamente: los clientes reintentan tras `Retry-After` en lugar de mantener sus solicitudes abiertas. Mantenlo por encima de tus transacciones normales más lentas, o una ráfaga breve empezará a recibir `503`.
- **Dimensiona primero el pool.** Un proceso necesita aproximadamente tantas conexiones como trabajo ejecuta a la vez. `pool_max_conns` en `database_url` es por defecto el mayor entre 4 y el número de CPU, y todos los procesos juntos deben quedar por debajo del límite de conexiones de la base de datos.
  - Con [`worker_job_concurrency`](#running-several-jobs-at-once), las ranuras también esperan conexiones, y el ajuste debe quedar por debajo del máximo del pool.
  - En Cloud Run, un servicio admite 80 solicitudes simultáneas por instancia por defecto. Con un pool de 4, una ráfaga de solicitudes lentas hace cola aquí, así que sube `pool_max_conns`, reduce la concurrencia del servicio, o ambas cosas.
- **`503` frecuentes o `db.client.connection.timeouts`** significan que el pool es demasiado pequeño o que las consultas se han vuelto más lentas; un tiempo de espera más largo solo lo oculta. Sigue [`MyDBPoolSaturated`](alerts.md#mydbpoolsaturated).

<!-- source: cloud-tasks ef3f7d7cc896 -->
### Cloud Tasks

[Despliegue en Cloud Run](deploy-cloud-run.md) configura todo lo que sigue con módulos de OpenTofu.

Con `queue = cloudtasks` los trabajos permanecen en la base de datos (`database = postgres`), y Google Cloud Tasks solo despierta al servidor para cada uno de ellos:

- Por cada trabajo que guarda, reintenta o devuelve a la cola, el servidor crea una tarea en `queue_cloudtasks_queue` cuando se confirma la transacción. La tarea llama a
  `POST {queue_cloudtasks_url}/_internal/jobs` a la hora en que vence el trabajo, con el ID del trabajo como cuerpo y un token de ID OIDC de
  `queue_cloudtasks_service_account`. Un trabajo que vence con más de 30 días de antelación se señala a los 30 días, y de nuevo cuando llega esa tarea.
- El proceso que recibe la tarea ejecuta el trabajo y responde `204` una vez registrado su resultado, también cuando el trabajo ya se ejecutó o no ha vencido.
  Un trabajo que falla lo reintenta el servidor en su siguiente hora de intento, no Cloud Tasks. Cloud Tasks reintenta una tarea solo cuando la
  respuesta es `5xx` (no se pudo acceder a la base de datos) o `401` (un token que el servidor rechazó), así que su `retryConfig` no necesita configuración.
- Ningún proceso sondea la tabla y ningún proceso ejecuta por sí mismo los trabajos periódicos. Crea un trabajo de Cloud Scheduler que llame a
  `POST {queue_cloudtasks_url}/_internal/tick` cada minuto (`* * * * *`) con un token OIDC de la misma cuenta de servicio (y la audiencia de
  `queue_cloudtasks_audience`, cuando esté establecida). Cada tick pone en cola los trabajos periódicos cuyo periodo ha llegado, una vez para toda la instancia (los periodos se reclaman en el KV,
  así que `kv` debe ser `db` o `valkey`), y vuelve a señalar los trabajos que llevan vencidos más de 2 minutos sin ejecutarse, lo que recupera una señal perdida.
- `/_internal/jobs` y `/_internal/tick` los sirven los procesos con `role = all` o `worker`, y aceptan solo un token firmado por Google para
  `queue_cloudtasks_service_account` con un correo electrónico verificado, cuya audiencia sea `queue_cloudtasks_audience` o, por defecto, la URL llamada. Cualquier otra solicitud recibe un `401`.
- Una entrega ejecuta un trabajo, durante como máximo la concesión de 5 minutos. Mantén `queue_cloudtasks_dispatch_deadline` (por defecto 10 minutos) y el tiempo de espera de solicitud del
  servicio de Cloud Run por encima de eso.
- La propia cuenta de servicio del servidor necesita `roles/cloudtasks.enqueuer` en la cola y `roles/iam.serviceAccountUser` en `queue_cloudtasks_service_account`.
  Se autentica con las Application Default Credentials.

<!-- source: routing-job-types-to-their-own-queues 292b3f6071a8 -->
### Dirigir tipos de trabajo a sus propias colas

Con `queue = cloudtasks`, un worker no puede elegir los trabajos que ejecuta: `worker_job_types` funciona solo con `queue = db`, porque la plataforma decide quién recibe una señal.
Para dar capacidad propia a algunos tipos de trabajo, envía sus señales a su propia cola y establece la capacidad de esa cola en la plataforma.
Nada cambia a menos que establezcas una ruta.

[`queue_cloudtasks_routes`](../reference/configuration.md#queue_cloudtasks_routes): `federation.*=projects/p/locations/l/queues/my-federation@https://federation-worker.example.com,notification.email=projects/p/locations/l/queues/my-mail`.
Una ruta sin `@` ni URL llama a `queue_cloudtasks_url`.

La parte izquierda es un tipo de trabajo de la [referencia de trabajos](../reference/jobs.md) o un prefijo. Un nombre gana a un prefijo, y un prefijo más largo gana a uno más corto.
Los tipos que no coinciden con ninguna ruta, incluidos los que añada una versión posterior, usan la cola por defecto (`queue_cloudtasks_queue`), así que siguen ejecutándose.
`my config check` comprueba la forma de las rutas; una parte izquierda que no coincide con ningún tipo de trabajo detiene `my serve` al arrancar.

**Da las mismas rutas a todos los procesos, incluido `role = web`.** Una señal la envía el proceso que guarda el trabajo: un proceso web en la mayor parte de lo que hacen los usuarios, como el
primer trabajo de una publicación, y un worker en los trabajos que guarda un trabajo, los reintentos, el siguiente trabajo de una clave de orden y el barrido de señales perdidas.
Donde cada rol tiene su propia configuración, como los servicios de Cloud Run, pasa el mismo valor a todos ellos:

```sh
routes='federation.*=projects/p/locations/l/queues/my-federation@https://federation-worker.example.com'
gcloud run services update my-web --update-env-vars "^|^MY_QUEUE_CLOUDTASKS_ROUTES=$routes"
gcloud run services update my-worker --update-env-vars "^|^MY_QUEUE_CLOUDTASKS_ROUTES=$routes"
gcloud run services update my-federation-worker --update-env-vars "^|^MY_QUEUE_CLOUDTASKS_ROUTES=$routes"
```

Un proceso con otras rutas no provoca ningún error ni pierde ningún trabajo: sus señales van a otra cola, y los trabajos se ejecutan allí, fuera de la capacidad que estableciste.
`my.queue.oldest_ready_age` no lo muestra, porque esos trabajos no esperan. Para comparar los procesos, cada uno registra al arrancar una línea con sus `routes` y su `routes_hash`:
el hash debe ser el mismo en todas partes. Para ver adónde van las señales, lee las métricas propias de la plataforma de cada cola.

**Permisos.** Las credenciales de cada proceso necesitan poder enviar a todas las colas de las rutas: `roles/cloudtasks.enqueuer`.
Un envío que falla se registra y no se prueba en otra cola, para que un tipo que limitas no se desborde hacia los demás. Esos trabajos empiezan tarde, cuando el barrido vuelve a enviar sus señales tras 2 minutos.

**Capacidad.** Cualquier receptor ejecuta cualquier trabajo, así que tú eliges hasta dónde separarlos:

| Plataforma | Un receptor | Receptores separados |
| --- | --- | --- |
| Google Cloud | Un servicio. Cada cola tiene sus propios `maxConcurrentDispatches` y `maxDispatchesPerSecond`; el límite de instancias del servicio se comparte | Un servicio por cola, nombrado por la URL de la ruta, cada uno con su propio límite de instancias y concurrencia. Cada uno establece `queue_cloudtasks_url` con su propia URL |

Una cola necesita la misma configuración que la de por defecto: el rol en la cola.
Solo la cola por defecto necesita el tick.

**Qué añade una cola.** Cada cola es una cosa más que crear, vigilar y pagar, y cuánto importa depende del tamaño de tu instancia.
Divide lo que hayas visto esperar en `my.job.wait`, no todo.

**El barrido.** Cada minuto el barrido busca trabajos que llevan vencidos más de 2 minutos sin ejecutarse, hasta 500 por ruta y 500 para la cola por defecto, y vuelve a señalarlos.
Así, una cola que no da abasto recibe hasta 500 señales duplicadas por minuto. No ejecutan un trabajo dos veces, pero son mensajes que pagas, y cesan cuando se despeja la acumulación.

**Las claves de orden** siguen vigentes: la base de datos decide qué puede ejecutarse. Los trabajos de una misma clave pueden estar en colas distintas, pero una cola atascada también retrasa los trabajos posteriores de sus claves.

**Añadir, mover y eliminar una cola.**

1. Crea la cola y su receptor, y da a todos los procesos permiso para enviarle.
2. Cambia las rutas en todos los procesos. Mientras difieran, las señales van a una u otra cola y los trabajos se ejecutan en ambos sitios.
3. Para eliminar una cola, quítala primero de las rutas en todas partes y espera a que esté vacía antes de borrarla. Una señal en una cola borrada se pierde, no su trabajo: el barrido vuelve a señalar el trabajo cuando lleva vencido 2 minutos.
   Una tarea de Cloud Tasks puede guardar la señal de un trabajo hasta con 30 días de antelación, así que un trabajo que vence más tarde que la espera simplemente lo señala el barrido cuando llega su hora.

<!-- source: health-checks 06a8834ade0d -->
## Comprobaciones de salud

| Ruta | Tipo | Cuándo devuelve 200 |
| --- | --- | --- |
| `GET /healthz` | liveness (vida) | El proceso puede responder HTTP. No mira dependencias como la base de datos (para que una base de datos lenta no reinicie el proceso) |
| `GET /readyz` | readiness (disponibilidad) | La base de datos es accesible y el esquema está aplicado |
| `GET /api/v1/instance/health` | Monitorización de disponibilidad, páginas de estado y clientes (lo sirve el rol `web`) | Igual que la disponibilidad. El resultado se guarda en caché en el proceso durante 60 segundos |

- Un 200 de `/healthz` o `/readyz` tiene el cuerpo `{"status":"ok"}`.
- `/readyz` comprueba la base de datos en cada solicitud, y las solicitudes concurrentes comparten una sola comprobación.
- `/api/v1/instance/health` devuelve 200 y `{"status":"ok","checked_at":"<RFC 3339>"}` cuando está sano, y 503 y `{"status":"unavailable","checked_at":...}` cuando falla (`Cache-Control: no-store`).
  Cada proceso toca la base de datos como máximo una vez por minuto, con independencia de la frecuencia con que se monitorice o se solicite, y no mira ni el token de acceso ni el límite de frecuencia. Apunta los monitores de disponibilidad a este endpoint, no al sondeo (`/readyz`).
  Para la configuración por servicio, el intervalo de monitorización y el costo, y cómo se trata el mantenimiento, consulta la [guía de monitorización de disponibilidad](monitoring.md).
- Los tres registran en el nivel Debug del registro de acceso y no crean ningún span de traza.
- Cuando no se cumple la disponibilidad, la respuesta es 503 con un cuerpo `application/problem+json` según el RFC 9457.
  Su `title` se traduce según `Accept-Language` y no contiene detalles internos. La causa (un error de conexión o los nombres de las tablas que faltan) va al registro.
- Una comprobación de disponibilidad se corta a los 3 segundos.
- La disponibilidad también exige que la base de datos contenga el `host` y el `account_domain` configurados, o que el proceso haya podido registrarlos.
- **Esquema aplicado** significa que existe toda tabla que los archivos de esquema de la base de datos (`schema/<NN>-<module>/<dialect>.sql`) declaran con `CREATE TABLE`
  (SQLite se comprueba mediante `sqlite_schema`, PostgreSQL mediante el `current_schema()` de la conexión). `my schema apply` aplica todo el esquema, así que una tabla que falta significa que el esquema no está aplicado o es más antiguo que el binario.
  No se comprueban las diferencias de columnas e índices; `my schema apply -dry-run` imprime lo que cambiaría.

<!-- source: requests-that-bypass-the-cdn 2cca4127af07 -->
## Solicitudes que esquivan el CDN

Cuando el servidor está detrás de un CDN y su propia dirección sigue siendo pública, establece `origin_secret` y `origin_secret_header` para que rechace lo que no ha pasado por el CDN. Las comprobaciones de salud siguen siendo accesibles. Consulta [Rechazar las solicitudes que no han pasado por el CDN](cdn.md#refuse-requests-that-did-not-come-through-the-cdn).

<!-- source: security-headers 8c5e82848149 -->
## Cabeceras de seguridad

Cada respuesta lleva `Strict-Transport-Security`, `X-Content-Type-Options`, `Referrer-Policy`, `Permissions-Policy`, `Cross-Origin-Opener-Policy`, `Cross-Origin-Resource-Policy: same-site` y `X-Frame-Options`,
y, en las respuestas que no son documentos, `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'; base-uri 'none'`.
Un manejador las sobrescribe en las respuestas que necesitan otros valores, como el JSON de ActivityPub y los archivos multimedia públicos.
`Permissions-Policy` deniega la cámara, la geolocalización, los pagos y USB. Deniega también el micrófono, salvo mientras el chat de voz está activado (`voice` no es `off`):
entonces permite el micrófono solo a las páginas del propio origen del servidor (`microphone=(self)`), para que los navegadores puedan hablar en una llamada.
El HTML del cliente web recibe una CSP de documento que no usa nonce (`script-src 'self'`, etc.).
Exige Trusted Types (`require-trusted-types-for 'script'`) y limita las políticas que se pueden crear a las dos del cliente (`trusted-types my-server-sanitized-html my-service-worker`).

<!-- source: web-client f35b79dd2cc3 -->
## Cliente web

Un proceso cuyo `role` es `web` o `all` sirve el cliente web incrustado en el binario.

| Ruta | Contenido | `Cache-Control` |
| --- | --- | --- |
| Cualquier `GET` que no coincida con otra ruta (`/`, `/@alice`, `/@alice/status/{id}`, `/settings`, etc.) | La carcasa HTML de la SPA. La configuración de la instancia se pasa en `<script type="application/json" id="config">` | `public, max-age=60, stale-while-revalidate=300`, ETag |
| `GET /assets/*` | Archivos de la compilación con nombres con hash | `public, max-age=31536000, immutable`, ETag |
| `GET /manifest.webmanifest` | El manifiesto de la aplicación web construido a partir de la configuración de instancia | `public, max-age=60, stale-while-revalidate=300`, ETag |
| `GET /sw.js`, `/icon.svg` y otros archivos directamente bajo la compilación | Se sirven tal cual | `no-cache`, ETag |
| `GET /licenses/*` | Las licencias de lo que incluye la compilación (avisos de paquetes npm, y `LICENSE-Unicode.txt` y `OFL.txt` para datos incluidos que no son de npm). Se sirven como `text/plain; charset=utf-8` sea cual sea la extensión | `no-cache`, ETag |
| `GET /robots.txt`, `/.well-known/tdmrep.json`, `/.well-known/security.txt` | Construidos a partir de la configuración de instancia (consulta [Buscadores y rastreadores de IA](crawlers.md)) | `public, max-age=3600`, ETag |

- Las rutas bajo `/api`, `/oauth`, `/auth`, `/.well-known`, `/nodeinfo`, `/ap`, `/media`, `/uploads`, `/theme` (la hoja de estilos del tema del servidor) y `/licenses`, y `/robots.txt`, nunca devuelven la carcasa; si ninguna otra ruta coincide devuelven un problema 404.
- La carcasa se sirve después de la negociación de contenido de ActivityPub. Solicitar una ruta de la carcasa con un `Accept` de ActivityPub devuelve 404 (las páginas de publicaciones redirigen, como se describe más abajo).
  La carcasa lleva siempre `Vary: Accept`. Cuando pongas un CDN delante, incluye el tipo de `Accept` (ActivityPub o HTML) en la clave de caché, o excluye de la caché la carcasa, `/ap/*` y `/@*`.
  Sirve solo `/assets/*` desde el CDN.
- Un binario compilado sin el cliente web (un `go build` simple, sin compilar antes el cliente) registra una advertencia al arrancar y sirve una carcasa sin scripts.

<!-- source: federation-endpoints 7aaee8c9da42 -->
## Endpoints de federación

Un proceso cuyo `role` es `web` o `all` responde a estos endpoints, que otros servidores del fediverso usan para encontrar y mostrar tus cuentas.

| Ruta | Contenido |
| --- | --- |
| `GET /.well-known/webfinger` | WebFinger (RFC 7033). `resource` es `acct:alice@{account_domain}` (`acct:alice@{host}` también funciona), un `id` de Actor, o la URL `/@alice`. `subject` es siempre el identificador del lado de `account_domain` |
| `GET /.well-known/host-meta`, `/.well-known/host-meta.json` | La plantilla de URI de WebFinger (XRD / JRD) |
| `GET /.well-known/nodeinfo`, `GET /nodeinfo/2.1` | NodeInfo 2.1. `metadata.sourceCode` es el ajuste `source_url`, y `software.repository` también se establece para una URL `https:` (consulta [Publicar el código fuente](#publish-the-source-code-agpl-30)) |
| `GET /ap/actors/{uuid}` | El Actor (`publicKey` es el `#main-key` RSA). Una solicitud que prefiere HTML recibe un 303 a `/@{username}` |
| `GET /ap/actors/{uuid}/{outbox,followers,following,featured,featured-tags}` | Colecciones |
| `POST /ap/actors/{uuid}/inbox`, `POST /ap/inbox` | La bandeja de entrada (inbox) de cada actor local y la bandeja compartida. Las solicitudes deben llevar firma HTTP |
| `GET /@{username}` | Un 302 al `id` del Actor para un `Accept` de ActivityPub; en caso contrario, el HTML del cliente web (consulta más abajo) |
| `GET /@{username}/status/{uuid}` | Para un `Accept` de ActivityPub, un 302 al `id` (`/ap/objects/{uuid}`) de la publicación si un visitante sin sesión puede verla, y 404 si no. En caso contrario, el HTML del cliente web |

- El identificador del Actor de la instancia es `{account_domain}@{account_domain}` (la misma forma que en Mastodon).
- Una cuenta que no existe o no se puede mostrar (suspendida, desactivada, eliminada o remota) recibe siempre el mismo 404 (`Cache-Control: no-store`).
- Las respuestas anónimas llevan `Cache-Control: public, max-age=60`. Las respuestas que varían según `Accept` llevan `Vary: Accept`.
  Para un CDN que no puede gestionar `Vary: Accept`, excluye de la caché `/ap/*` y `/@*`.

<!-- source: separate-handle-and-delivery-domains b1c23d994d0e -->
### Dominios de identificadores y de entrega separados

Cuando `account_domain` difiere de `host`, las solicitudes a `/.well-known/webfinger`, `/.well-known/host-meta` y `/.well-known/nodeinfo` del lado de `account_domain` se redirigen a `host` con un 301 (con `Access-Control-Allow-Origin: *`).
Si diriges a este servidor las solicitudes de `account_domain`, el servidor devuelve esa redirección por sí mismo.
Si responde otro servidor o proxy inverso, redirige a `https://{host}` conservando la ruta y la cadena de consulta.
