<!-- source: running-the-server-start-stop-and-health-checks 5164430b3203 -->
# Ejecutar el servidor: arranque, parada y comprobaciones de salud

Cómo arrancar y detener el servidor, obtener su imagen y vigilarlo mientras funciona: métricas, trazas, la cola de trabajos y las comprobaciones de salud.

<!-- source: start-the-server 93f89c7d7b66 -->
## Iniciar el servidor

```sh
my config check   # report every configuration problem at once
my config check -instance   # also warn about instance settings that are not set (opens the database)
my serve          # start according to the configured role (-config <file> reads a TOML file)
my version        # print the version of the binary
```

`my serve` valida la configuración, construye los adaptadores de base de datos, cola de jobs y gestión de claves, y acepta HTTP en `listen`.
Con `tls = off`, el valor predeterminado, habla allí HTTP/1.1 sin cifrar: pon delante un proxy inverso o un balanceador de carga que termine HTTPS (y HTTP/2 o HTTP/3 para los clientes) y reenvíe por HTTP/1.1.
Para servir HTTPS sin un proxy, consulta [Servir HTTPS desde el servidor](#serving-https-from-the-server).
Las actualizaciones en tiempo real (`GET /api/v1/streaming`, los streams de las cronologías local y federada y los streams de las salas de chat de voz) son respuestas de larga duración que envían un comentario de latido cada 30 segundos (15 en una sala de voz) y permanecen abiertas mientras están inactivas.
El proxy no debe almacenar en búfer estas respuestas (nginx: el servidor envía `X-Accel-Buffering: no`), y su timeout de inactividad o de lectura debe ser mayor que el intervalo del latido.
No aplica el esquema ni ejecuta los pasos de actualización de datos: ejecuta `my schema apply` y luego `my db upgrade` antes de iniciar (consulta [Base de datos](database.md); las actualizaciones ejecutan ambos como parte de [Actualizar](database.md#upgrading)). Hasta que `my db upgrade` ha registrado cada paso de la versión, `/readyz` responde `503`.
En la configuración de Docker Compose (`deploy/docker/compose.yaml`; consulta [Desplegar con Docker Compose](deploy-docker-compose.md)), los servicios puntuales `schema` y `upgrade` ejecutan `my schema apply` y `my db upgrade` antes de que arranque el servidor.
Si el esquema no está aplicado, el proceso arranca de todos modos y la readiness devuelve 503, y no sirve nada más hasta que el esquema exista.

El primer `my serve` o comando `my` que usa la base de datos registra en ella `host` y `account_domain`. Cuando la configuración difiere de los valores registrados, `my serve` registra `start failed` con ambos pares de valores y termina con el código 1, y los comandos imprimen el mismo error. Las excepciones son `my schema apply` y `my db backup`, que no comprueban, y el modo de mantenimiento, que no lee la base de datos. `my config check -instance` y `my db upgrade -check` comprueban pero no registran nada.
Un proceso que no pudo leer la base de datos cuando arrancó no está listo hasta que pueda, y hasta entonces no ejecuta jobs y responde `503` con `Retry-After` a toda solicitud salvo las comprobaciones de salud (`/healthz`, `/readyz`, `/health/live`, `/health/ready`, `/api/v1/instance/health`). Esto es también lo que hace un servidor iniciado antes de `my schema apply`. Si entonces encuentra valores distintos, se detiene con el mismo error.

En el perfil `production`, `my config check` también comprueba que los ajustes válidos son adecuados para una instancia de producción: los errores, como un `host` al que otros servidores no pueden llegar o una KEK que no es aleatoria, hacen que termine con 1, y las advertencias, como `mail = none`, no.
`my serve` registra ambos como advertencias cuando arranca pero arranca de todos modos, así que ejecuta `my config check` como un paso del despliegue.
La [lista de comprobación de producción](production-checklist.md) enumera las comprobaciones y lo que hay que verificar a mano antes de salir a producción.

| `role` | HTTP | Worker de jobs |
| --- | --- | --- |
| `all` (predeterminado) | Todas las rutas | Se ejecuta |
| `web` | Todas las rutas | No se ejecuta |
| `worker` | Solo comprobaciones de salud (para sondas de la plataforma) | Se ejecuta |

Los registros van a la salida de error estándar como JSON Lines. Cada línea tiene `level` (`DEBUG`, `INFO`, `WARN`, `ERROR`) y `severity`, el mismo nivel en la forma que lee Cloud Logging (`DEBUG`, `INFO`, `WARNING`, `ERROR` y `CRITICAL` para un error que detiene el proceso).
Los registros escritos mientras se atiende una solicitud llevan un `request_id`, y el mismo ID se devuelve en la cabecera de respuesta `X-Request-Id`.
Si el `X-Request-Id` de la solicitud tiene como máximo 128 caracteres de letras, dígitos y `-_.:`, se usa tal cual, como el ID asignado por un balanceador de carga.
Los registros dentro de una traza (solicitudes y jobs) también llevan `trace_id` y `span_id` (consulta [Trazas](#traces)).

Cada solicitud HTTP y cada ejecución de job escribe una línea cuando termina. La línea tiene los mismos nombres y valores que las etiquetas de las [métricas](#metrics) de abajo,
así que con `metrics_exporter = none` una plataforma puede construir métricas basadas en registros (contadores y distribuciones) a partir de estas líneas.

| Línea (`msg`) | Campos |
| --- | --- |
| `http request` (`INFO`; las comprobaciones de salud usan `DEBUG`) | `http.request.method`, `http.route`, `my.surface` y `http.response.status_code` (un número) tienen los mismos valores que las etiquetas de `http.server.request.duration`. También `duration_seconds` (un número de segundos), `url.path` (la ruta sin la cadena de consulta), `http.response.body.size` y `request_id`. `http.route` y `my.surface` están ausentes en las solicitudes que no llegaron a una ruta (comprobaciones de salud, solicitudes rechazadas antes del enrutado), y la métrica no las cuenta. Una solicitud cuyo cliente se fue antes de que el servidor pudiera responder tiene el estado `499` en lugar de un 5xx, salvo que un handler entrara en pánico; el cliente no recibió nada, así que no cuenta como un error del servidor |
| `job` (`INFO` si tiene éxito y para `postponed`, `WARN` para `retry`, `ERROR` para `dead`) | `my.job.type` y `my.outcome` tienen los mismos valores que las etiquetas de `my.job.duration`. También `duration_seconds` (`my.job.duration`), `wait_seconds` (`my.job.wait`), `job_id`, `attempt` y `error` en caso de fallo |
| `error report` (`ERROR`; `CRITICAL` para un fallo que detiene el proceso) | Una línea por error notificado ([Informes de errores](#error-reports)) |

Con `metrics_exporter = none`, las líneas `http request` y `job` también llevan las [métricas del sistema](#system-metrics) de los adaptadores y del pool de la base de datos.
Cuando la solicitud o el job ejecutó operaciones de cola, de clave-valor, de stream o de blobs: `my.adapter.operations.total`, `my.adapter.errors.total` y
`my.adapter.errors`, los errores por `my.adapter` (como `{"kv.valkey": 2}`). `db.client.connection.usage`, las conexiones usadas sobre el máximo del pool,
cuando lo tiene. Cuando no son cero, `my.db.client.connection.waits`, `db.client.connection.wait_time` (segundos) y `db.client.connection.timeouts`: el incremento
desde la línea anterior del proceso, así que su suma a lo largo de las líneas es el total del proceso.

<!-- source: error-reports b70c69ddef24 -->
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
| `context.httpRequest` | En el error de una solicitud: `method`, `url` (la ruta, como `/api/v1/statuses/{id}`, nunca la ruta que pidió el cliente) y `responseStatusCode`, el estado devuelto (500) |
| `message` | Lo último de la línea: `<type>: <message>`, un salto de línea y la traza de pila de Go |

Antes de que un informe salga del proceso, en el mensaje se sustituyen las URL, las direcciones de correo electrónico, las menciones `@user@host`, las direcciones IP, los UUID y ULID y los números de 10 dígitos o más por `<url>`, `<email>`, `<acct>`, `<ip>`, `<id>` y `<n>`, y se recorta a 1 KiB.
La traza de pila conserva las posiciones de archivo pero no los valores de los argumentos de sus líneas de función (se muestran como `(...)`), y se recorta a 16 KiB. Ningún informe lleva cuerpo de solicitud, cabecera, consulta, cookie, cuenta, nombre de usuario, dirección IP ni User-Agent.
Ninguna otra línea lleva una traza de pila, así que Error Reporting encuentra solo las trazas de pila de estas líneas.

El nombre de un campo es una única clave que contiene `.`; no está anidado. Entrecomíllalo en los filtros y extracciones de Cloud Logging (por ejemplo `jsonPayload."http.route"`, y `EXTRACT(jsonPayload.duration_seconds)` para el valor de una distribución).
Los campos usados como etiquetas toman valores solo de un conjunto finito; `url.path`, `request_id` y `job_id` nunca son etiquetas.

Con `metrics_exporter = emf`, estas dos líneas son las propias líneas EMF. La línea `http request` lleva el valor de `http.server.request.duration`, y la línea `job` lleva los valores de `my.job.duration` y `my.job.wait` (claves con el nombre de las métricas),
además de la directiva `_aws`, `service.name`, `service.version`, `my.role` y `service.namespace` (el `host`). Estas tres métricas no se escriben en ninguna otra línea. Los campos que eliges en `metrics_emf_dimensions` se escriben como cadenas, como exige EMF (por ejemplo, la dimensión `http.response.status_code` es `"200"`).
Los registros van a la salida de error estándar y las demás líneas EMF van a la salida estándar. El controlador de registros `awslogs` de ECS envía ambas a CloudWatch Logs, así que las métricas se extraen de las dos.

<!-- source: docker-compose-files 9f0633524036 -->
## Archivos de Docker Compose

`deploy/docker/` contiene lo que despliega una instancia. `compose.yaml` se ejecuta solo, y los demás archivos se apilan sobre él.

| Archivo | Añade | Úsalo cuando |
| --- | --- | --- |
| `compose.yaml` | El servidor y sus servicios puntuales de base de datos, con SQLite y archivos multimedia en el volumen `data` | Siempre |
| `compose.postgres.yaml` | PostgreSQL en un contenedor, en el volumen `postgres` | Quieres PostgreSQL en esta máquina. Para un servidor PostgreSQL en otro lugar, omítelo y define `MY_DATABASE` y `MY_DATABASE_URL` en `.env` |
| `compose.s3.yaml` | Multimedia en un bucket compatible con S3; con `COMPOSE_PROFILES=garage` también un almacén [Garage](https://garagehq.deuxfleurs.fr/) de un nodo en el volumen `garage` | La multimedia supera un disco, o la quieres en un bucket. Sin el perfil, el almacén es el externo de los ajustes `MY_BLOB_S3_*` |
| `compose.caddy.yaml` | Caddy, con certificados TLS en los puertos 80 y 443 | `MY_HOST` es un dominio público y nada más termina TLS en esta máquina |
| `compose.build.yaml` | Construye la imagen desde un checkout | Ejecutas tu propia compilación |

Elige la base de datos y el almacén de multimedia antes del primer arranque: los datos no se mueven solos cuando cambian los archivos
([Copias de seguridad](backup.md) muestra lo que hay que mover).

**Apila los archivos** con opciones `-f` en ese orden, o nómbralos una vez en `.env`, para que todos los comandos `docker compose` posteriores
(`exec`, `logs`, `pull` y los de las otras guías) usen los mismos archivos:

```sh
cd deploy/docker
cp example.env .env
# In .env: COMPOSE_FILE=compose.yaml:compose.postgres.yaml:compose.caddy.yaml
```

Un archivo llamado `compose.override.yaml` en el directorio se lee solo cuando ni `-f` ni `COMPOSE_FILE` nombran archivos. Con
ellos, inclúyelo también en la lista.

**Ajustes.** Copia `example.env` a `.env`. Compose lee `.env` para rellenar las variables de sus archivos, y entrega todo el
archivo a los contenedores del servidor, así que todos los ajustes de la [referencia de configuración](../reference/configuration.md) funcionan
definiéndolos allí. La plantilla los enumera todos, con una frase sobre cada uno.

- La clave de cifrado de claves queda fuera de `.env`, en el archivo `secrets/kek`, y los contenedores la leen como el secreto de Docker
  `/run/secrets/my_kek`, así que nunca se convierte en una variable de entorno. Una línea `MY_KEK` en `.env` detiene el servidor, que
  rechaza `MY_KEK` y `MY_KEK_FILE` juntos ([gestionar la clave](key-management.md)).
- El servidor se ejecuta como el usuario `65532`, y Compose monta el archivo con los permisos que tiene, así que el archivo tiene que ser
  legible por otros. Guárdalo en un directorio al que solo puedas entrar tú, como hacen los comandos de `example.env`.
- Los demás secretos llegan a los contenedores como variables de entorno. Para pasar uno como archivo en su lugar, móntalo con un secreto
  en un `compose.override.yaml` propio y define su `<NAME>_FILE`, como hace `compose.yaml` para la clave.
- `.env` contiene secretos: `chmod 600 .env`, y conserva una copia con tus copias de seguridad.
- `MY_LISTEN` permanece en `:8080`, ya que los archivos se conectan a ese puerto, salvo que el servidor sirva HTTPS por sí mismo
  ([Servir HTTPS desde el servidor](#serving-https-from-the-server)).

**TLS con Caddy.** Define `MY_HOST` con un dominio cuyo DNS apunte a la máquina, con los puertos 80 y 443 accesibles desde
internet, y elimina `MY_WEB_ORIGIN` de `.env`. Caddy obtiene y renueva el certificado por sí mismo en la primera solicitud.
El servidor confía en el `X-Forwarded-For` de los pools de direcciones predeterminados de Docker; define `MY_TRUSTED_PROXIES` si los tuyos difieren.

**El Garage incluido.** `compose.s3.yaml` con `COMPOSE_PROFILES=garage` necesita los valores que `example.env` enumera al principio.
El servicio puntual `garage-init` prepara el almacén en cada `up`. Permanece dentro de la red de Compose, ya que el servidor
sirve la multimedia por sí mismo. Un nodo es un disco: haz copia de seguridad del volumen `garage` o del bucket ([Copias de seguridad](backup.md#media-in-a-bucket)).

<!-- source: serving-https-from-the-server 45ec9231a41e -->
## Servir HTTPS desde el servidor

`my` puede servir HTTPS por sí mismo, de modo que una máquina pequeña ejecute todo el servidor como un solo binario con un dominio y los puertos 80 y 443,
sin un proxy inverso. Con `tls = acme`, obtiene sus certificados de una autoridad de certificación ACME como Let's
Encrypt. Con `tls = files`, sirve un certificado y una clave que tú proporcionas y mantienes al día con otra herramienta, como
certbot.

```sh
MY_TLS=acme
MY_TLS_ACME_AGREE_TERMS=true            # you agree to the terms of service of the certificate authority
# MY_TLS_ACME_EMAIL=ops@social.example  optional contact of the account
# MY_LISTEN=:443          the default with tls other than off
# MY_TLS_HTTP_LISTEN=:80  the default; empty opens no port 80
```

```sh
MY_TLS=files
MY_TLS_CERT_FILE=/etc/letsencrypt/live/social.example/fullchain.pem
MY_TLS_KEY_FILE=/etc/letsencrypt/live/social.example/privkey.pem
```

- **Puertos.** El servidor sirve HTTPS en `listen` (`:443` por defecto una vez que `tls` no es `off`). En `tls_http_listen` (`:80`
  por defecto) redirige con `301` las solicitudes `GET` y `HEAD` a la misma ruta en `https`, y responde `400` a todo otro
  método, para que ningún cuerpo de solicitud viaje en texto plano.
- **Hosts.** El servidor sirve `host`, y el host de `web_origin` cuando difiere. Un handshake para cualquier otro nombre
  falla sin certificado, y nunca hace que el servidor pida uno.
- **Conexiones.** TLS 1.2 o posterior con los conjuntos de cifrado predeterminados de Go, HTTP/2 y HTTP/1.1, como máximo 64 KiB de cabeceras
  de solicitud, y como máximo 100 streams concurrentes en una conexión HTTP/2. Los timeouts son los de HTTP sin cifrar. El servidor
  sigue añadiendo `Strict-Transport-Security` a cada respuesta ([Cabeceras de seguridad](#security-headers)).
- **Roles.** Solo los roles `web` y `all` escuchan con TLS. Un `worker` sirve sus comprobaciones de salud en HTTP sin cifrar en
  `listen` (`:8080` por defecto) y no abre el puerto 80.
- **Direcciones de cliente.** Sin un proxy delante, el par de la conexión es el cliente: deja `trusted_proxies` vacío.
  Defínelo solo cuando una CDN delante se conecta al servidor por `https`.
- **Modo de mantenimiento.** `my serve -maintenance` no lee ni `tls` ni los certificados y responde en HTTP sin cifrar en
  `listen` (`:8080` salvo que lo definas), así que apunta `MY_LISTEN` y tu cortafuegos a ese puerto mientras se ejecuta, o conserva un
  proxy inverso para el mantenimiento.

<!-- source: certificates-from-an-acme-certificate-authority 52037a75d641 -->
### Certificados de una autoridad de certificación ACME

Con `tls = acme`, el servidor pide un certificado para cada uno de sus hosts que no tiene ninguno, a la autoridad de certificación
de `tls_acme_directory` (Let's Encrypt por defecto).

- **Antes de empezar.** `host` debe ser un nombre DNS que apunte al servidor, y los puertos 443 y 80 deben ser accesibles desde
  internet: la autoridad de certificación comprueba que el servidor controla el nombre conectándose a él. Responde en
  `listen` (el desafío TLS-ALPN-01) o en `tls_http_listen` (el desafío HTTP-01), así que un `tls_http_listen` vacío
  sigue funcionando solo con el puerto 443. Los certificados comodín y la validación mediante registros DNS no están disponibles.
- **Primer arranque.** El servidor empieza a escuchar sin esperar a un certificado, y luego los pide. Hasta que un host tiene su
  certificado, un handshake para él falla; con Let's Encrypt los primeros certificados suelen llegar en un minuto. El registro
  dice `obtained a TLS certificate` para cada host, y `the TLS certificate could not be obtained` con el motivo cuando un
  pedido falla.
- **Renovaciones.** El servidor renueva cada certificado en el momento que le sugiere la autoridad de certificación (ACME Renewal
  Information): elige una vez una hora aleatoria dentro de la ventana sugerida, la mantiene mientras la ventana siga igual, y pregunta de nuevo
  según el `Retry-After` de la autoridad, al menos cada 12 horas, para que una autoridad que deba sustituir certificados
  antes de tiempo pueda mover la ventana. Cuando la autoridad no ofrece tal ventana, el servidor renueva tras dos tercios de la
  vida del certificado, o tras la mitad de ella para un certificado válido durante menos de 10 días. Una renovación dice a la
  autoridad qué certificado sustituye, lo que la mantiene fuera de los límites de tasa de Let's Encrypt. El certificado actual
  sigue en servicio hasta que llega su renovación, y una renovación que falla espera como lo hace un primer pedido.
- **Pedidos fallidos.** Tras un pedido fallido el servidor espera antes del siguiente para el mismo host: 1 minuto tras el
  primer fallo, 10 minutos tras el segundo, 100 minutos tras el tercero y luego un día, para mantenerse dentro de los
  límites de la autoridad de certificación. La espera se guarda en la base de datos y sobrevive a los reinicios. Corrige la causa (DNS, un cortafuegos,
  un puerto) y el siguiente pedido sale cuando termina la espera.
- **Dónde se guardan.** La cuenta con la autoridad de certificación, los certificados y el registro de cada pedido
  se guardan en la base de datos; las claves privadas se sellan con la clave de cifrado de claves, como los demás secretos
  ([gestión de claves](key-management.md)). Una copia de seguridad de la base de datos los incluye, y un servidor reiniciado sirve los
  certificados almacenados de inmediato, sin pedir. Cambiar la clave de cifrado de claves los vuelve a envolver; no pide
  certificados nuevos.
- **Varios procesos.** Los procesos que comparten la base de datos y el almacén `kv` se coordinan: uno pide el certificado de
  un host mientras los demás esperan y luego lo cargan, y el proceso al que se conecte la autoridad de certificación responde a su
  comprobación. Usa `kv = db` o `kv = valkey`; `kv = memory` se rechaza salvo que un proceso lo ejecute todo (`role = all`).
  El proceso que obtiene un certificado avisa a los demás mediante el broker de [`stream`](../reference/configuration.md#stream), y lo sirven en
  segundos; cada proceso `web` con `tls = acme` sigue por tanto escuchando en el broker. Con `stream = hub`, o cuando se pierde un
  aviso, todos los procesos siguen leyendo los certificados actuales de la base de datos cada 5 minutos, y cada
  minuto una vez que su certificado ha llegado a su hora de renovación, así que una renovación les llega mucho antes de que el certificado antiguo
  caduque.
- **Perfiles.** `tls_acme_profile` elige un perfil que ofrece la autoridad de certificación, como `shortlived` en Let's
  Encrypt. Vacío toma su valor predeterminado. Tras un cambio de perfil, la siguiente renovación lo sigue, y el certificado que trae
  se sirve incluso cuando caduca antes que el que sustituye.

Para probarlo sin gastar los límites del entorno de producción de Let's Encrypt, empieza con su entorno de staging,
que emite certificados en los que los navegadores no confían con límites mucho más altos:

```sh
MY_TLS_ACME_DIRECTORY=https://acme-staging-v02.api.letsencrypt.org/directory
```

Cuando el registro diga que se obtuvieron los certificados y `curl -kv https://social.example/` muestre un certificado de
`(STAGING)`, elimina el ajuste y reinicia. El servidor registra una cuenta nueva en el entorno de producción y
pide allí sus certificados; los de staging permanecen en la base de datos y ya no se sirven.

<!-- source: certificates-from-files 35f16ea23b33 -->
### Certificados desde archivos

Con `tls = files`, `tls_cert_file` contiene primero el certificado del servidor y luego sus intermedios (el
`fullchain.pem` de certbot); `tls_key_file` contiene su clave. El certificado debe cubrir `host`, y también sirve el host de
`web_origin` cuando difiere y el certificado lo cubre.

El servidor mira los tiempos de modificación de ambos archivos cada minuto y los carga de nuevo cuando cualquiera cambia, sin
reiniciar. Un par que aún no coincide (un certificado nuevo antes de su clave nueva) mantiene el certificado anterior en
servicio hasta la siguiente comprobación, y el registro dice por qué.

<!-- source: commands-and-metrics de97a3993e6b -->
### Comandos y métricas

`my tls` muestra y controla los certificados con la configuración del servidor, desde cualquier máquina que llegue a la
base de datos (la [CLI de administración](admin-cli.md#tls-certificates)):

```sh
my tls status                                          # each host: certificate, renewal, latest orders
my tls renew -host social.example                      # order now, without waiting
my tls revoke -host social.example -reason key_compromise
```

- **`status`** muestra, para cada host, el certificado actual (su número de serie, emisor y caducidad), cuándo se renueva (la
  ventana de la autoridad de certificación, o la hora derivada de su vida), cuántos pedidos seguidos no obtuvieron ninguno y
  los últimos pedidos con sus errores. Con `tls = files` muestra el certificado de `tls_cert_file`.
- **`renew`** pide un certificado ahora, omitiendo la hora de renovación y la espera tras pedidos fallidos: ejecútalo una vez que hayas
  corregido la causa de los pedidos fallidos. El servidor en ejecución responde a la comprobación de la autoridad de certificación, así que debe estar
  en marcha, y sus procesos sirven el certificado nuevo en segundos, o en 5 minutos con `stream = hub`.
- **`revoke`** pide primero un certificado que sustituya al actual, luego pide a la autoridad de certificación que revoque
  el actual y lo anota; todos los procesos dejan de servir el certificado revocado en segundos, o en 5
  minutos con `stream = hub`. Usa `key_compromise` cuando otras personas pueden conocer la clave privada, y `superseded` u
  `operator` en otro caso. Cuando no se puede pedir el sustituto, la revocación se lleva a cabo de todos modos: los procesos sirven el
  certificado anterior del host si no está revocado, o nada hasta que el servidor obtenga uno nuevo.
- `renew` y `revoke` necesitan `tls = acme` y un `kv` distinto de `memory`, ya que los procesos a los que se conecta la autoridad de
  certificación leen del almacén `kv` las respuestas a sus comprobaciones. Con `tls = files` se rechazan: renueva
  o revoca el certificado con la herramienta que escribe los archivos.

Con [`metrics_exporter`](#metrics) definido, pon alertas sobre `my.tls.certificate.expiry` por debajo de 7 días y sobre
`my.tls.order.consecutive_failures` de 3 o más ([alertas](alerts.md#mytlscertificateexpiring)): una renovación que falla se nota
semanas antes de que el certificado caduque.

<!-- source: checks-and-running-it 983028d27675 -->
### Comprobaciones y ejecución

`my config check` informa, con `tls = acme`, de unos términos a los que no se ha dado conformidad, un `host` que es una dirección IP, `localhost`
o un nombre sin punto, un `tls_acme_directory` que no es una URL `https` y `kv = memory` con un `role` distinto de
`all`, como errores. Con `tls = files` informa de un certificado o una clave que no se pueden leer, una clave que no coincide con el
certificado, y un certificado que no cubre `host`, ha caducado o aún no es válido, como errores, y avisa cuando
quedan menos de 14 días. Avisa cuando `trusted_proxies` está definido con `tls` distinto de `off`. Con `tls = off` avisa
cuando `listen` es el puerto 80 o 443 en una dirección distinta de loopback, ya que nada parece terminar TLS delante del
servidor; el servidor registra la misma advertencia una vez por arranque cuando un servidor así, con `trusted_proxies` vacío, recibe una solicitud
HTTP sin cifrar desde una dirección distinta de loopback.

**systemd.** Los puertos por debajo de 1024 necesitan una capability para un usuario distinto de root. Dásela al servicio y a nada más:

```ini
[Service]
User=my
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
```

Con `tls = files`, el usuario también necesita leer los archivos de certificado. Con certbot, sus directorios `live` y `archive`
son legibles solo por root: da acceso al grupo del servicio (`chgrp` y `chmod g+rx` en los directorios, `g+r` en
las claves), o copia los archivos en un deploy hook a un directorio que lea el servicio.

**Docker.** Docker 20.10 y posteriores permiten que un contenedor abra puertos por debajo de 1024 sin una capability, así que la imagen funciona tal
cual. La imagen define `MY_LISTEN=:8080`, así que define `MY_LISTEN=:443` en `.env` y publica ambos puertos en un
`compose.override.yaml` (`ports: ["80:80", "443:443"]`). Con `tls = files`, monta los archivos de certificado en solo lectura
donde apuntan `MY_TLS_CERT_FILE` y `MY_TLS_KEY_FILE`; el servidor se ejecuta como el usuario `65532`, que debe poder leerlos.
Con `tls = acme` no hay nada que montar: los certificados viven en la base de datos.
Omite `compose.caddy.yaml`, que tomaría los mismos puertos. Esto no vale con `network_mode: host`, donde
el contenedor comparte la red de la máquina y necesita la misma capability que un servicio fuera de un contenedor.

**Comparado con Caddy.** Servir HTTPS desde `my` renuncia a HTTP/3, a la compresión de las respuestas (el servidor envía los archivos del cliente
web comprimidos y no comprime nada más), a un registro de acceso propio (las líneas de registro `http request` del servidor permanecen) y a los certificados validados mediante
DNS, incluidos los certificados comodín. Conserva `compose.caddy.yaml`, u otro proxy, cuando necesites cualquiera de ellos.

<!-- source: get-the-image 123f0112b538 -->
## Obtener la imagen

La imagen del servidor es `ghcr.io/mycroblog/my`, pública, para `linux/amd64` y `linux/arm64`, con todos los adaptadores.

| Etiqueta | Apunta a | Úsala para |
| --- | --- | --- |
| `0.1.0` | Esa versión, para siempre | Nombrar una versión |
| `0.1` | El último parche de `v0.1` | Docker Compose, que aplica el esquema en cada `up` |
| `0.1.0-rc.1` | Esa versión preliminar | Probar una versión antes de que salga |
| `nightly` | El último commit de `main` que pasó las pruebas nocturnas, movido cada noche en que cambia | Probar cambios antes de una versión preliminar, en una instancia que puedas desechar |

No existe `latest` ni `0`: ambas cruzarían una versión menor, y cada actualización a una versión menor nueva debe hacerse [a través de la anterior](database.md#skipping-releases). Nombra una versión en cada comando; `docker pull ghcr.io/mycroblog/my` a secas falla con `manifest unknown`.
Una versión de parche también puede necesitar `my schema apply` y `my db upgrade`, así que fuera de Docker Compose sigue [Actualizar](database.md#upgrading) para cada versión. En producción, fija el digest que indican las notas de la versión:

```text
ghcr.io/mycroblog/my:0.1.0@sha256:...
```

**`nightly` no tiene ruta de actualización.** Las actualizaciones solo se prometen entre versiones publicadas: de una imagen `nightly` a
la siguiente, o de `nightly` a una versión, el esquema y los pasos de datos de `my db upgrade` pueden cambiar de maneras por las que no pasa ninguna
versión, así que espera tener que recrear su base de datos. No la ejecutes nunca en producción. `my version` imprime de qué commit se trata, como en
`v0.1.0-rc.5.0.20261109182600.g7a789720a1b2`: una versión posterior a la última publicada (`v0.1.0-rc.5`), la hora del commit en UTC y los
12 primeros caracteres del commit tras la `g`. Para conservar una compilación, fija su digest (`ghcr.io/mycroblog/my:nightly@sha256:...`): una compilación más antigua pierde
la etiqueta pero sigue pudiéndose descargar por su digest.

Dónde ejecutarla:

- **Docker Compose**: `deploy/docker/compose.yaml` ejecuta la imagen de `MY_VERSION` en `.env` ([Archivos de Docker Compose](#docker-compose-files)). Para compilarla desde un checkout en su lugar, añade el override `compose.build.yaml` (`docker compose -f compose.yaml -f compose.build.yaml up -d --build`) y establece `MY_VERSION` con un nombre propio, ya que da nombre a tu imagen local y es lo que imprime `my version`.
- **Google Cloud y AWS**: los módulos de OpenTofu ejecutan la imagen de `ghcr.io` tal cual, la versión que nombra su `ref` ([Desplegar con OpenTofu](deploy-opentofu.md#the-root-modules)).
- **Kubernetes**: descárgala de `ghcr.io` tal cual.

<!-- source: verify-an-image 76b9a1293edb -->
### Verificar una imagen

Cada imagen publicada tiene una procedencia firmada mediante Sigstore, que vincula el digest con el flujo de trabajo y el commit que la compiló, y un SBOM. Con la [GitHub CLI](https://cli.github.com/), y con la sesión iniciada en `ghcr.io` si el comando lo pide:

```sh
gh attestation verify oci://ghcr.io/mycroblog/my@sha256:... -R mycroblog/my \
  --signer-workflow mycroblog/my/.github/workflows/release.yml
docker buildx imagetools inspect ghcr.io/mycroblog/my@sha256:... --format '{{ json .SBOM }}'
```

El primer comando falla a menos que la imagen la haya compilado el flujo de trabajo de publicación de este repositorio. Para una imagen `nightly`, verifícala con
`--signer-workflow mycroblog/my/.github/workflows/nightly.yml` en su lugar. Imprime el commit, que es el de la etiqueta de la versión; la etiqueta `org.opencontainers.image.revision` de la imagen nombra el mismo.

<!-- source: get-the-binary 2608f7bdd54d -->
## Obtener el binario

Para ejecutar `my` sin un contenedor, cada versión adjunta el binario de la imagen, para `linux/amd64` y `linux/arm64`,
como `my_<version>_linux_<arch>.tar.gz`. Es un único binario estático con el cliente web dentro; el archivo también contiene
`LICENSE`, los avisos de licencia en `licenses/` y un README. No hay compilación para otros sistemas operativos.

```sh
curl -fsSLO https://github.com/mycroblog/my/releases/download/v0.1.0/my_v0.1.0_linux_amd64.tar.gz
tar xzf my_v0.1.0_linux_amd64.tar.gz my
./my version
```

Configúralo como se configura la imagen ([referencia de configuración](../reference/configuration.md)), y sigue
[Actualizar](database.md#upgrading) para cada versión: nada aplica el esquema por ti.

<!-- source: publish-the-source-code-agpl-30 2e87c44ffb2f -->
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

La imagen lleva la licencia de `my` en `/usr/share/doc/my/LICENSE`, y los avisos de licencia del código y los datos de terceros que hay en el binario en `/usr/share/doc/my/licenses/`:
`go-modules.md` para los módulos de Go, `npm-packages.md` y `npm-packages-sw.md` para los paquetes npm del cliente web, y las licencias de sus datos y fuentes incluidos.
Si distribuyes la imagen o el binario, entrega estos archivos junto con él.

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

<!-- source: metrics a5fd8092d408 -->
## Métricas

El ajuste `metrics_exporter` (`none` por defecto) exporta las métricas del servicio. Sus ajustes relacionados están en la [referencia de configuración](../reference/configuration.md).

| `metrics_exporter` | Ruta | Adecuado para |
| --- | --- | --- |
| `none` | No se exporta nada. La instrumentación cuesta casi nada. Los valores de toda la instancia siguen yendo a una línea de registro `metrics snapshot` cada 15 minutos, y los de los adaptadores y del pool a las líneas de solicitudes y jobs | Google Cloud, donde las métricas de la plataforma y las métricas basadas en registros ocupan su lugar ([Desplegar en Google Cloud](deploy-google-cloud.md#monitoring-and-alerts)) |
| `prometheus` | `GET /metrics` en `metrics_listen` (`127.0.0.1:9464` por defecto). Es un listener separado del `listen` público, sin autenticación, y se ejecuta para todo `role` | Autoalojado (consultado por un servidor compatible con Prometheus o KEDA) |
| `otlp` | Enviado a un OpenTelemetry Collector por OTLP/HTTP. El destino viene de las variables de entorno estándar de OTel como `OTEL_EXPORTER_OTLP_ENDPOINT` | Configuraciones donde puedes ejecutar un collector |
| `emf` | Líneas JSON de CloudWatch Embedded Metric Format. Las métricas de solicitudes y jobs viajan en las líneas de registro de cierre de arriba; las demás se escriben en la salida estándar, una línea por conjunto de etiquetas, cuando termina una solicitud o un job | AWS ([Desplegar en AWS](deploy-aws.md#monitoring-and-alerts)) |

Las métricas se enumeran abajo. En Prometheus, `.` en un nombre pasa a `_` y se añade un sufijo de unidad (`_seconds`, y `_total` para un contador).

| Métrica | Tipo | Etiquetas |
| --- | --- | --- |
| `http.server.request.duration` | Histograma, segundos | `http.request.method`, `http.route` (la ruta del patrón de ruta; `other` cuando ninguno coincide), `http.response.status_code`, `my.surface` (`client_api` / `admin` / `activitypub` / `oauth` / `media` / `web` / `payments` / `voice` / `queue`) |
| `http.server.active_requests` | UpDownCounter | `my.surface` |
| `my.inbox.activities` | Contador | `activity.type` (un tipo de Activity conocido, u `other`), `my.outcome` (`accepted` / `duplicate` / `rejected` / `ignored` / `error`), `my.inbox.rejection` (solo con `rejected`: `signature` para una firma o prueba de integridad que falta, no es válida o no se verifica, o cuya clave no pudo obtenerse; `suspended` para un dominio suspendido; `federation_off`; `rate_limited`; `invalid` para una actividad sin un `id`, `type`, `actor` u `object` válidos) |
| `my.relay.activities` | Contador | `my.outcome` (`admitted` / `over_limit` / `restricted` / `not_receiving`: si una actividad llevada por un relay se ingirió, y por qué se descartó; consulta [Relays de federación](relays.md)) |
| `my.move.activities` | Contador | `my.outcome` (`recorded` / `duplicate` / `not_own_actor` / `same_account` / `target_unavailable` / `alias_missing` / `target_moved` / `suspended` / `too_soon`: si un `Move` recibido se registró, y por qué se descartó) |
| `my.delivery.attempts` | Contador | `my.outcome` (`success` / `retry` / `gone` / `failed`; `gone` cubre un 410 y las entregas omitidas porque el servidor no está disponible o está suspendido) |
| `my.delivery.duration` | Histograma, segundos | `my.outcome` |
| `my.ux.duration` | Histograma, segundos | `my.ux.metric` (`lcp` / `inp` / `long_frame` / `transition` / `press` / `call_rtt` / `call_jitter`), `my.ux.name` (la ruta, el tipo de interacción, la acción o el transporte de la llamada; [mediciones de UX](error-reporting.md#ux-measurements)) |
| `my.ux.call.packet_loss` | Histograma, porcentaje | `my.ux.name` (el transporte de la llamada) |
| `my.plugin.failures` | Contador | `my.plugin.kind` (un tipo de [plugin](plugins.md)), `my.plugin.code` (el código Connect del fallo) |
| `my.plugin.slot_wait` | Histograma, segundos | `my.plugin.kind` (cuánto esperó una llamada su turno entre las llamadas de ese tipo que se ejecutan a la vez) |
| `my.hook.decisions` | Contador | `hook`, `mode`, `outcome` (las decisiones que registró un [hook de decisión](decision-hooks.md)) |
| `my.hook.failures` | Contador | `hook`, `outcome` (`timeout`, `error`, `invalid` o `breaker_open`) |
| `my.hook.duration` | Histograma | `hook` (segundos por llamada al plugin) |
| `my.job.duration` | Histograma, segundos | `my.job.type` (un tipo de job registrado, u `other`), `my.outcome` (`success` / `retry` / `dead` / `postponed`: `postponed` es una ejecución que no hizo trabajo y no usa ninguno de los intentos del job, como una obtención de tarjeta de enlace que espera a su host) |
| `my.job.wait` | Histograma, segundos | `my.job.type` |
| `my.link_card.host_pauses` | Contador | `http.response.status_code` (`429` / `503`). Uno por pausa de las obtenciones de tarjetas de enlace de un host; consulta [Vistas previas de enlaces](link-previews.md#limits-per-host) |
| `my.projection.drifts` | Contador | `my.projection` (el nombre de una proyección). Uno por verificación programada que encontró la proyección desviada; consulta [Comprobar y reconstruir proyecciones](database.md#checking-and-rebuilding-projections) |
| `my.translation.requests` | Contador | `my.outcome` (`author` / `cache` / `provider` / `no_provider` / `limit` / `unsupported` / `failed`). Uno por solicitud de traducir una publicación que el espectador puede ver, o una publicación o comentario de una denuncia que pide un moderador: respondida con las propias palabras del autor, con una traducción conservada o por un proveedor, o rechazada porque ningún proveedor puede recibir la publicación, se alcanzó un límite mensual, ningún proveedor traduce el idioma o ningún proveedor devolvió una traducción utilizable. |
| `my.translation.lock_waits` | Contador | Ninguna. Uno por solicitud que esperó a que otra solicitud tradujera la misma publicación al mismo idioma, en lugar de preguntar a un proveedor |
| `my.translation.characters` | Contador | `my.provider` (el ID de un proveedor de traducción). Los caracteres de publicaciones y comentarios de denuncias enviados al proveedor, sin marcadores, incluidas las llamadas que fallan. Esto es lo que cuentan los [límites mensuales](instance-settings.md#admin-screens-and-the-client-api), y puede diferir de lo que factura el proveedor |
| `my.timeline.delivery.lag` | Histograma, segundos | Ninguna (desde la creación de una publicación local hasta su escritura en las cronologías de inicio y el aviso a los streams) |
| `my.stream.connections` | UpDownCounter | `my.stream.kind` (`user`, `timeline` para los streams de las cronologías local y federada, `voice` para los streams de las salas de chat de voz) |
| `my.stream.signals` | Contador | `my.stream.kind` (`user` / `timeline`). Uno por señal que gestionó un stream abierto, enviara o no algo al cliente |
| `my.accounts.registered` | Contador | Ninguna |
| `my.statuses.created` | Contador | `my.visibility` |
| `my.billing.imports` | Contador | `my.billing.source` (`aws` / `gcp` / `plugin`), `my.outcome` (`success` / `failed`), `error.type` (solo con `failed`: el tipo de fallo, como en [Importar los cargos de uso en la nube](billing-import.md)). Cada intento cuenta, incluidos los reintentos y los fallos que esperan a la siguiente importación diaria |
| `my.supporter.tip_checkouts` | Contador | `my.outcome` (`started` / `rate_limited` / `paid` / `expired`). Uno por inicio de una propina que obtuvo la página del proveedor, por inicio que rechazaron los límites y por propina que se pagó o terminó sin pagar; consulta [propinas](supporters.md#tips) para las alertas a las que sirve |
| `my.voice.turn.credentials` | Contador | `my.voice.turn` (`coturn` / `cloudflare`), `my.outcome` (`issued` / `reused` / `failed`). Uno por credencial TURN que pidió un participante del chat de voz al unirse a una sala o al renovar su conexión: `reused` significa que se devolvieron de nuevo credenciales de Cloudflare emitidas antes para la misma conexión, y `failed` significa que no se pudo llegar a Cloudflare o este rechazó, así que el participante recibió un error y no pudo conectarse |
| `my.errors.exported` | Contador | `my.outcome` (`success` / `dropped` / `failed`: uno por informe de error para el servicio de `errors_exporter`. `dropped` significa que el informe no se envió porque ya esperaban 1000 informes o el servicio pidió al servidor que fuera más despacio, y el servidor espera lo que el servicio pida antes de enviar de nuevo; `failed` significa que no se pudo llegar al servicio o este rechazó el informe. Un informe se envía una vez y nunca se reintenta, y los informes que aún esperan 2 segundos después de iniciarse un apagado se pierden) |
| `my.queue.ready` | Indicador | `my.job.type` (el número de jobs ejecutables en la cola de la base de datos) |
| `my.queue.oldest_ready_age` | Indicador, segundos | `my.job.type` (cuánto ha esperado el job ejecutable más antiguo de cada tipo en la cola de la base de datos; un tipo sin jobs ejecutables no tiene valor) |
| `my.queue.dead` | Indicador | Ninguna |
| `my.instance.users`, `my.instance.statuses` | Indicador | Ninguna (los recuentos de usuarios locales y publicaciones, como en NodeInfo) |
| `my.media.stored_bytes` | Indicador, bytes | `my.media.origin` (`local` / `remote_cache`) |
| `my.db.size` | Indicador, bytes | Ninguna (el tamaño de la base de datos: las relaciones del esquema actual para PostgreSQL; el archivo de la base de datos sin su write-ahead log para SQLite) |
| `my.billing.import.failing` | Indicador | `my.billing.source` (1 cuando la importación más reciente de la fuente falló, 0 en otro caso) |
| `my.federation.instances` | Indicador | `my.federation.instance.state` (`available`: servidores a los que aún van las entregas pero que fallaron al menos un día desde que respondieron por última vez; `unavailable`: servidores cuyas entregas están en pausa) |
| `my.metrics.snapshot.age` | Indicador, segundos | `my.metrics.snapshot.part` (`queue` / `totals`) |
| `my.errors.reported` | Contador | `my.error.source` (`server` / `web`), `my.error.kind` (`panic` / `error`). Uno por [informe de error](#error-reports) |
| `my.tls.certificate.expiry` | Indicador, segundos | `host` (el tiempo hasta que caduca el certificado que este proceso sirve para uno de sus hosts, con `tls = acme` o `tls = files`; cada proceso informa del suyo, así que se nota un proceso que se quedó con un certificado antiguo. Consulta [Comandos y métricas](#commands-and-metrics)) |
| `my.tls.order.attempts` | Contador | `host`, `result` (`issued` / `failed`). Uno por pedido de un certificado a la autoridad de certificación ACME, por el proceso que lo hizo, incluidos `my tls renew` y `my tls revoke` |
| `my.tls.order.consecutive_failures` | Indicador | `host` (los pedidos desde el certificado actual del host que no obtuvieron ninguno: el recuento que fija la espera antes del siguiente pedido) |
| `my.tls.rounds` | Contador | Ninguna. Uno por ronda del gestor de certificados sobre sus hosts con `tls = acme`; un recuento que deja de crecer significa que el gestor se detuvo |

Las comprobaciones de salud (`/healthz`, `/readyz`, `/health/live`, `/health/ready`) no se miden. Ninguna etiqueta contiene un dominio remoto, una cuenta, una URL ni una IP de cliente.
`my.queue.*`, `my.instance.*`, `my.media.stored_bytes`, `my.db.size`, `my.federation.instances` y `my.billing.import.failing` son valores de toda la instancia: todos los procesos informan del mismo valor a partir de una instantánea conservada en el almacén clave-valor.

<!-- source: system-metrics 973069859f11 -->
### Métricas del sistema

Las métricas sobre el proceso, el runtime de Go y las conexiones a recursos se exportan mediante el mismo `metrics_exporter`.
La CPU del contenedor, la memoria, el número de instancias y las solicitudes de entrada se dejan a las métricas de la plataforma; estas cubren la saturación dentro del proceso que la plataforma no puede ver.
No las uses para activar el escalado; úsalas para alertas y para estimar el tope de instancias.

| Métrica | Tipo | Etiquetas |
| --- | --- | --- |
| `go.memory.used`, `go.memory.limit`, `go.memory.gc.goal` | Indicador, bytes | Solo `go.memory.used` tiene `go.memory.type` (`stack` / `other`). `go.memory.limit` aparece solo cuando `GOMEMLIMIT` está definido |
| `go.memory.allocated`, `go.memory.allocations` | Contador | Ninguna |
| `go.goroutine.count`, `go.processor.limit`, `go.config.gogc` | Indicador | Ninguna |
| `go.schedule.duration`, `go.memory.gc.pause.duration` | Histograma, segundos | Ninguna (la espera desde que pasa a ser ejecutable hasta que se ejecuta, y las pausas del GC) |
| `go.memory.gc.cycles` | Contador | Ninguna |
| `process.cpu.time` | Observable Counter, segundos (total desde el inicio) | `cpu.mode` (`user` / `system`) |
| `process.memory.usage` | Indicador, bytes | Ninguna (memoria residente, RSS) |
| `process.unix.file_descriptor.count`, `my.process.file_descriptor.limit` | Indicador | Ninguna (descriptores de archivo abiertos y el límite) |
| `process.uptime` | Indicador, segundos | Ninguna |
| `db.client.connection.count` | Indicador | `db.client.connection.pool.name` (`postgres` / `sqlite`), `db.client.connection.state` (`idle` / `used`) |
| `db.client.connection.max` | Indicador | `db.client.connection.pool.name` |
| `db.client.connection.wait_time`, `my.db.client.connection.waits`, `db.client.connection.timeouts` | Observable Counter (total desde el inicio; segundos o recuento) | `db.client.connection.pool.name`. `timeouts` es solo de PostgreSQL |
| `my.adapter.operation.duration` | Histograma, segundos | `my.adapter` (`queue.db` / `kv.db` / `kv.memory` / `kv.valkey` / `stream.<stream>` / `blob.fs`), `my.operation`, `my.outcome` (`success` / `error`) |
| `my.adapter.connections` | UpDownCounter | `my.adapter` (conexiones de `valkey` y suscriptores de `stream.<stream>`) |
| `my.adapter.reconnects` | Contador | `my.adapter` |
| `http.client.request.duration` | Histograma, segundos | `my.http.purpose` (`json` / `html` / `image` / `video` / `human_check` / `web_push` / `webhook` / `client_metadata` / `login_provider` / `robots` / `domain_list`), `http.request.method`, `my.http.status_class` (`2xx` y así sucesivamente), `error.type` (`forbidden_destination` es un rechazo de la protección SSRF) |
| `http.client.open_connections` | UpDownCounter | Ninguna |
| `my.readiness.failures` | Contador | Ninguna |
| `my.readiness.shared` | Contador | Ninguna. Uno por solicitud de readiness que tomó el resultado de una comprobación ya en curso en lugar de iniciar una |
| `my.http.origin_refused` | Contador | Ninguna. Uno por solicitud rechazada porque no llegó a través de la CDN ([`origin_secret`](cdn.md#refuse-requests-that-did-not-come-through-the-cdn)). Las sondas a la dirección del origen lo suben despacio; un salto mientras el tráfico se detiene significa que la CDN ya no envía la cabecera. El modo de mantenimiento no mide nada, así que solo su registro de acceso las muestra |

- Las métricas del runtime de Go (`go.*`) las exportan solo `prometheus` y `otlp`. Con `emf`, un job periódico de snapshot escribe los indicadores del proceso y del pool de conexiones y los contadores observables.
- Las esperas del pool y el tiempo de CPU son Observable Counters: el pool y el kernel conservan totales desde el inicio, y se leen en el momento de la recogida. En Prometheus son contadores `_total` (como `process_cpu_time_seconds_total`), en OTLP son sumas monótonas, y su crecimiento se ve con `rate()` de PromQL.
  `emf` no tiene contadores totales, así que escribe el aumento desde la escritura anterior (súmalos con la estadística Sum de CloudWatch).
- El `claim` de la cola se mide solo cuando toma un job o falla. El sondeo de un worker en espera no registra nada.
- `db.client.connection.pending_requests` no se exporta: las estadísticas de pgx y de `database/sql` no informan de cuántas esperan, así que el número y la duración de las esperas lo sustituyen.
- Pon alertas sobre la proporción de la suma de `go.memory.used` respecto a `go.memory.limit`, y sobre el crecimiento de `db.client.connection.wait_time` (se está alcanzando el límite del pool).

<!-- source: memory-limit-gomemlimit 66320a6c7b76 -->
### Límite de memoria (`GOMEMLIMIT`)

`GOMEMLIMIT` es una variable de entorno del runtime de Go que establece el operador; la aplicación no la elige.
Sin ella, Go se ejecuta con su valor por defecto (sin límite), y el heap no se recolecta con antelación al acercarse al límite de memoria del contenedor.
Con un límite de memoria del contenedor, establécela en torno al 90 % de ese límite (por ejemplo `GOMEMLIMIT=460MiB` para un contenedor de 512 MiB). Los módulos de OpenTofu la establecen así para cada servicio, según su tamaño.
No establezcas `GOMAXPROCS`: el valor por defecto de Go sigue el límite de CPU del contenedor.

<!-- source: traces 15eb90e8584a -->
## Trazas

El ajuste `traces_exporter` (por defecto `none`) exporta trazas distribuidas. Su configuración relacionada está en la [referencia de configuración](../reference/configuration.md).

| Span | Nombre | Atributos |
| --- | --- | --- |
| Solicitud HTTP (servidor) | `<method> <http.route>` (`<method>` para una solicitud que no llegó a una ruta) | `http.request.method`, `http.route`, `http.response.status_code`, `my.surface`. Un 5xx es un fallo |
| Ejecución de un trabajo (consumer) | El tipo de trabajo (`my.job.type`) | `my.job.type`, `my.job.attempt`, `my.outcome`. Cada reintento es un span aparte |
| Transacción | `my.db.transaction` | `db.system.name`. Cada reintento es un span aparte |
| Consulta (cliente) | El nombre de la consulta de sqlc (como `InsertJobs`; si no, la primera palabra del SQL, como `BEGIN`) | `db.system.name`, `db.operation.name` |
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
| Google Cloud | `traces_exporter = none` | Cloud Trace muestra la traza que Cloud Run registra de cada solicitud: [Desplegar en Google Cloud](deploy-google-cloud.md#traces) |
| AWS | `traces_exporter = otlp`, a un collector en cada tarea | X-Ray: [Trazas a X-Ray](deploy-aws.md#traces-to-x-ray) |

<!-- source: profiles d19dd2bf22e2 -->
## Perfiles

Las [métricas del sistema](#system-metrics) dicen cuánta memoria y CPU usa un proceso; un perfil del runtime de Go dice qué las usa.
`pprof_listen` nombra la dirección de un listener que sirve los perfiles en `/debug/pprof/`, para todo `role`. Está vacío por defecto, lo que no abre ningún listener.

```sh
MY_PPROF_LISTEN=127.0.0.1:6060 my serve

go tool pprof -sample_index=inuse_space http://127.0.0.1:6060/debug/pprof/heap   # what holds the heap now
go tool pprof -sample_index=alloc_space http://127.0.0.1:6060/debug/pprof/heap   # what has allocated since the start
go tool pprof 'http://127.0.0.1:6060/debug/pprof/profile?seconds=30'             # where the CPU goes, for 30 seconds
curl 'http://127.0.0.1:6060/debug/pprof/goroutine?debug=1'                       # every goroutine, by where it waits
go tool pprof 'http://127.0.0.1:6060/debug/pprof/mutex?seconds=30'               # where goroutines wait on locks, for 30 seconds
curl 'http://127.0.0.1:6060/debug/pprof/heap?debug=1' | grep '^# '               # the runtime's memory statistics
```

El listener no tiene autenticación, un perfil muestra las interioridades del proceso y tomar uno cuesta CPU: mantenlo en loopback y accede a él mediante un túnel (`ssh -L 6060:127.0.0.1:6060 <host>`, `kubectl port-forward`), y quítalo cuando termines.
El perfil del heap cubre solo el heap de Go. La memoria residente del proceso también contiene las páginas del binario que se leyeron, las pilas y las propias estructuras del runtime; las estadísticas de memoria las dan.
Los perfiles de bloqueo y de mutex solo muestrean mientras una solicitud con `seconds` los toma, y están vacíos sin ella.

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

<!-- source: the-job-queue-and-failed-dead-jobs 1fd5aaccbff5 -->
## La cola de jobs y los jobs fallidos (muertos)

[Jobs](../reference/jobs.md) enumera cada tipo de job: qué hace, qué lo inicia, cómo se reintenta y qué hace ejecutarlo de nuevo.

La página de jobs de administración (Administración → Jobs, `/admin/jobs`, permiso `manage_jobs`) tiene tres páginas:

- **Estado de la cola** (`/admin/jobs`): primero cuántos jobs fallaron, con un enlace a ellos; el recuento por estado (en cola, en ejecución, esperando reintento, fallidos), cada uno con enlace a los jobs de ese estado; cuánto ha esperado el job vencido más antiguo; y una tabla de los recuentos por tipo y estado con cuánto tiempo mantiene cada tipo a un job en espera.
- **Lista de jobs** (`/admin/jobs/list`): los jobs que no han terminado (en cola, en ejecución, esperando reintento), o de un estado, y de un tipo, una página cada vez, con el tipo, el número de intentos, la hora en que vence o falló, el estado y la primera línea del último error.
- **Un job** (`/admin/jobs/<id>`, desde el tipo en la lista): **Ejecutar de nuevo** para un job fallido, que pregunta antes, ya que una ejecución puede repetir lo que el job hizo antes de fallar; cada dato del job, su ID y claves cada una con un botón de copiar; su último error y su payload, cada uno con un botón de copiar; una tabla de los intentos fallidos, el más reciente primero (resultado, hora de fin, worker y el error); y un enlace a los jobs de su tipo.

La API de administración es `GET /api/v1/admin/queue`, `GET /api/v1/admin/jobs?state=&type=`, `GET /api/v1/admin/jobs/{id}`, `GET /api/v1/admin/jobs/{id}/attempts` y `POST /api/v1/admin/jobs/{id}/retry`.
Un job completado ya no está en la cola, así que su página no se encuentra.

- Los jobs en espera, en ejecución y esperando reintento solo pueden enumerarse para las colas cuyos jobs conserva el servidor: `queue = db`, `queue = cloudtasks` (los jobs están también en la base de datos) y `queue = inline`. Los jobs fallidos y el historial de intentos están en la base de datos de la aplicación para toda cola, así que siempre pueden enumerarse.
- El historial de intentos conserva solo los intentos fallidos (reintentos y el intento que finalmente falló). Sigue una ejecución con éxito mediante la línea de registro estructurada `job` (que lleva `job_id`).
  El historial lo elimina la limpieza horaria una vez que ha pasado `job_attempt_retention` (30 días por defecto; consulta el [ajuste](../reference/configuration.md#job_attempt_retention)) desde que terminó el intento.
  El historial no contiene payload ni registro del handler, y un mensaje de error se corta en sus primeros 2000 bytes. Los handlers de jobs mantienen las direcciones de correo, las direcciones IP, los tokens y los cuerpos de publicaciones fuera de los mensajes de error.
  Para una vista limitada a los tipos de job de correo, consulta [Enviar correo](email.md).

Un job cuyo handler decidió que falló definitivamente, y un job que agotó sus reintentos, pasa a ser un job muerto en la cola de la base de datos (`queue = db` y `queue = cloudtasks`).
`my job` los enumera, reintenta y elimina. El estado «Fallidos» de la página de jobs y la API de administración también pueden enumerarlos y reintentarlos.

```sh
my job list                  # newest failures first: ID, type, run count, time of failure, last error (-limit sets the count, default 50)
my job retry <id> [<id>...]  # reset the attempt count and put the job back in the queue to run immediately
my job delete <id> [<id>...] # remove without running
my job types                 # print every job type of this release
```

- Si ya espera un job con la misma clave de idempotencia, ese job sustituye al reintentado. Los handlers de jobs son idempotentes, así que devolver el mismo job a la cola cualquier número de veces es seguro.
- Un ID desconocido se imprime y el comando termina con el código 1; los demás ID se procesan igualmente. El código de salida 2 significa un error de uso.
- Un job muerto que nadie reintenta lo elimina la limpieza horaria una vez que ha pasado `dead_job_retention` (30 días por defecto; consulta el [ajuste](../reference/configuration.md#dead_job_retention)) desde el fallo.
- Los jobs muertos de `queue = inline` viven en la memoria del proceso del servidor, así que `my job` no puede gestionarlos (código de salida 1).

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

<!-- source: running-several-jobs-at-once 2a1137a12e8f -->
### Ejecutar varios jobs a la vez

[`worker_job_concurrency`](../reference/configuration.md#worker_job_concurrency) define cuántos jobs ejecuta un proceso a la vez, de 1 a 128. Funciona con `queue = db`; con Cloud Tasks lo decide la plataforma. El valor predeterminado es un job a la vez, y un ajuste de 1 se comporta igual.

Súbelo para jobs que pasan su tiempo esperando a otros servidores, como `federation.deliver`. Esos usan poca memoria y pocas conexiones de base de datos por job, así que un proceso hace el trabajo de muchos: en una prueba local donde un job solo esperaba 200 ms una respuesta, un proceso con 32 ranuras igualó a 32 procesos con una ranura cada uno, usando una pequeña parte de la memoria. Esos jobs no hacían nada más, así que mide tu propia instancia. Combínalo con `worker_job_types`, y da a cada tipo de job un proceso con su propio número.

Ten en cuenta estos límites:

- **Conexiones de base de datos**: un job necesita una conexión para ser reclamado, para su propio trabajo y para registrar su resultado. El ajuste debe estar por debajo del máximo del pool, o el servidor no arranca. El máximo es `pool_max_conns` en `database_url`, por defecto el mayor entre 4 y el número de CPU, así que una vCPU permite 3 hasta que lo subas. Comprueba que tu base de datos o pooler de conexiones acepta las conexiones de todos tus procesos.
- **SQLite**: escribe una transacción a la vez, y en una prueba local fue más lento con 128 que con 32.
- **Jobs que consumen mucha CPU**: las ranuras comparten la CPU y la memoria de un proceso. `media.process` espera su turno con un límite de la mitad de las CPU por defecto, y la espera cuenta contra el arrendamiento de 5 minutos del job, así que un proceso lleno de `media.process` puede pasarse del arrendamiento y empezar de nuevo. Da a esos jobs un proceso con pocas ranuras.
- **Un servidor a la vez**: un proceso hace como máximo 8 conexiones a un mismo host, así que un proceso entrega a un solo servidor no más rápido de lo que permiten 8 conexiones, tenga las ranuras que tenga. Donde la mayoría de las entregas van a unos pocos servidores grandes, más procesos ayudan donde más ranuras no.
- **Un servidor atascado retiene ranuras**: las ranuras que esperan a un servidor lento no están libres para otros jobs, así que mantén las entregas separadas del correo y las notificaciones.
- **Los despliegues consumen intentos**: cuando el servidor se detiene, los jobs que siguen en ejecución pasado `ShutdownTimeout` (25 segundos por defecto) se cancelan, y cada job cancelado consume un intento. Cuantas más ranuras ejecutan jobs lentos, más intentos consume cada despliegue. Un job cancelado cuyo resultado no se registra a tiempo espera a que caduque su arrendamiento antes de ejecutarse de nuevo.
- **Una caída se lleva todas las ranuras**: si el proceso se queda sin memoria, todos sus jobs esperan a que caduquen sus arrendamientos.

Para decidir si subirlo, lee `my.job.wait` por `my.job.type`: si los jobs de un tipo esperan mucho mientras `my.job.duration` es sobre todo tiempo esperando a otros servidores y la CPU y la memoria se mantienen bajas, más ranuras deberían ayudar. Si las esperas siguen altas tras subirlo, mira las conexiones de base de datos y el límite por host de arriba antes de subirlo de nuevo.

Mientras no vence ningún job, una ranura sondea y las demás esperan, así que un proceso inactivo cuesta a la base de datos lo que cuesta un proceso de un solo job.

<!-- source: waiting-for-a-database-connection 795c0fd7f769 -->
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
  - En Google Cloud, una instancia admite muchas solicitudes a la vez: [Un PostgreSQL externo](deploy-google-cloud.md#an-external-postgresql) dimensiona su pool.
- **`503` frecuentes o `db.client.connection.timeouts`** significan que el pool es demasiado pequeño o que las consultas se han vuelto más lentas; un tiempo de espera más largo solo lo oculta. Sigue [`MyDBPoolSaturated`](alerts.md#mydbpoolsaturated).

<!-- source: cloud-tasks f14d9721ba24 -->
### Cloud Tasks

[Desplegar en Google Cloud](deploy-google-cloud.md) configura todo lo que sigue con módulos de OpenTofu.

Con `queue = cloudtasks` los trabajos permanecen en la base de datos (`database = postgres`), y Google Cloud Tasks solo despierta al servidor para cada uno de ellos:

- Por cada trabajo que guarda, reintenta o devuelve a la cola, el servidor crea una tarea en `queue_cloudtasks_queue` cuando se confirma la transacción. La tarea llama a
  `POST {queue_cloudtasks_url}/_internal/jobs` a la hora en que vence el trabajo, con el ID del trabajo como cuerpo y un token de ID OIDC de
  `queue_cloudtasks_service_account`. Un trabajo que vence con más de 30 días de antelación se señala a los 30 días, y de nuevo cuando llega esa tarea.
- El proceso que recibe la tarea ejecuta el trabajo y responde `204` una vez registrado su resultado, también cuando el trabajo ya se ejecutó o no ha vencido.
  Un trabajo que falla lo reintenta el servidor en su siguiente hora de intento, no Cloud Tasks. Cloud Tasks reintenta una tarea solo cuando la
  respuesta es `5xx` (no se pudo acceder a la base de datos) o `401` (un token que el servidor rechazó), así que su `retryConfig` no necesita configuración.
- Ningún proceso sondea la tabla y ningún proceso ejecuta por sí mismo los trabajos periódicos. Un trabajo de Cloud Scheduler llama a
  `POST {queue_cloudtasks_url}/_internal/tick`, cada minuto (`* * * * *`) por defecto, con un token OIDC de la misma cuenta de servicio (y la audiencia de
  `queue_cloudtasks_audience`, cuando esté establecida). Cada tick pone en cola los trabajos periódicos cuyo periodo ha llegado, una vez para toda la instancia (los periodos se reclaman en el KV,
  así que `kv` debe ser `db` o `valkey`), y vuelve a señalar los trabajos que llevan vencidos más de 2 minutos sin ejecutarse, lo que recupera una señal perdida.
- `/_internal/jobs` y `/_internal/tick` los sirven los procesos con `role = all` o `worker`, y aceptan solo un token firmado por Google para
  `queue_cloudtasks_service_account` con un correo electrónico verificado, cuya única audiencia sea `queue_cloudtasks_audience` o, por defecto, la URL llamada. Cualquier otra solicitud recibe un `401`.
- Una entrega ejecuta un trabajo, durante como máximo la concesión de 5 minutos. Mantén `queue_cloudtasks_dispatch_deadline` (por defecto 10 minutos) y el tiempo de espera de solicitud del
  servicio de Cloud Run por encima de eso.
- La propia cuenta de servicio del servidor necesita `roles/cloudtasks.enqueuer` en la cola y `roles/iam.serviceAccountUser` en `queue_cloudtasks_service_account`.
  Se autentica con las Application Default Credentials.

<!-- source: how-late-periodic-jobs-run 775fb52d1df8 -->
### Con cuánto retraso se ejecutan los trabajos periódicos

Un trabajo periódico se ejecuta hasta un periodo de su disparador después de su hora, más el tiempo que tarda la plataforma en iniciarlo. Esto es lo
que decide con cuánto retraso llegan los recordatorios y las notificaciones de inicio de los eventos y se abren sus salas de chat de voz, ya que `gathering.queue_due` las inicia: un recordatorio
se retrasa hasta el intervalo de la tabla. Los valores son los que dan el código del planificador y el disparador de cada plataforma; no se
midieron en un servidor en funcionamiento.

| Plataforma | Qué inicia los trabajos periódicos | Retraso esperado de un recordatorio o inicio de evento |
| --- | --- | --- |
| Autoalojamiento y [Docker Compose](deploy-docker-compose.md) (`queue = db`) | El planificador de cada proceso con `role = all` o `worker` comprueba una vez por minuto, desde que arrancó el proceso, y encola cada trabajo una vez por periodo para todo el servidor | Hasta 1 minuto, más alrededor de un segundo para que un worker recoja el trabajo |
| [AWS](deploy-aws.md) (`queue = db`) | El mismo planificador, en cada tarea | Hasta 1 minuto, más alrededor de un segundo |
| [Google Cloud](deploy-google-cloud.md) (`queue = cloudtasks`) | Cloud Scheduler llama a `/_internal/tick` según `queue.tick_schedule`, cada minuto por defecto; Cloud Tasks despierta entonces al servidor para cada trabajo | Hasta 1 minuto, más el arranque de una instancia cuando no hay ninguna en ejecución (un arranque en frío, normalmente unos segundos). Con un `tick_schedule` más largo, hasta ese intervalo ([Dónde reduce los costos el cuidado](deploy-google-cloud.md#where-care-reduces-it)) |

Un recordatorio que llegaría después de que su evento empezara no se envía, ni tampoco una notificación de inicio después de que el evento terminara.
Así, con un tick de más de 5 minutos, los recordatorios más cortos pueden perderse.

<!-- source: routing-job-types-to-their-own-queues f4098db6215c -->
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
En Google Cloud, `workers` da a cada worker su propia cola y su propio servicio, y a cada servicio las mismas rutas y permisos:
[Dividir los jobs en workers](deploy-opentofu.md#splitting-jobs-into-workers).

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

<!-- source: health-checks e268d13fe7f2 -->
## Comprobaciones de salud

| Ruta | Tipo | Cuándo devuelve 200 |
| --- | --- | --- |
| `GET /healthz`, `GET /health/live` | liveness (vida) | El proceso puede responder HTTP. No mira dependencias como la base de datos (para que una base de datos lenta no reinicie el proceso) |
| `GET /readyz`, `GET /health/ready` | readiness (disponibilidad) | La base de datos es accesible y el esquema está aplicado |
| `GET /api/v1/instance/health` | Monitorización de disponibilidad, páginas de estado y clientes (lo sirve el rol `web`) | Igual que la disponibilidad. El resultado se guarda en caché en el proceso durante 60 segundos |

- Cloud Run reserva algunas rutas que terminan en `z`, entre ellas `/healthz`, y las responde él mismo con un `404`; detrás de él, llama a `/health/live` y `/health/ready`, que responden lo mismo.
- Un 200 de cualquiera de ellas tiene el cuerpo `{"status":"ok"}`.
- `/readyz` comprueba la base de datos en cada solicitud, y las solicitudes concurrentes comparten una sola comprobación.
- `/api/v1/instance/health` devuelve 200 y `{"status":"ok","checked_at":"<RFC 3339>"}` cuando está sano, y 503 y `{"status":"unavailable","checked_at":...}` cuando falla (`Cache-Control: no-store`).
  Cada proceso toca la base de datos como máximo una vez por minuto, con independencia de la frecuencia con que se monitorice o se solicite, y no mira ni el token de acceso ni el límite de frecuencia. Apunta los monitores de disponibilidad a este endpoint, no al sondeo (`/readyz` o `/health/ready`).
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

<!-- source: web-client 0a0b62acc010 -->
## Cliente web

Un proceso cuyo `role` es `web` o `all` sirve el cliente web incrustado en el binario.

| Ruta | Contenido | `Cache-Control` |
| --- | --- | --- |
| Cualquier `GET` que ninguna otra ruta coincida (`/`, `/@alice`, `/@alice/status/{id}`, `/settings`, etc.) | El shell HTML de la SPA. Los ajustes de la instancia se pasan en `<script type="application/json" id="config">` | `public, max-age=60, stale-while-revalidate=300`, ETag |
| `GET /assets/*` | Archivos de compilación con nombres con hash | `public, max-age=31536000, immutable`, ETag |
| `GET /manifest.webmanifest` | El manifiesto de la aplicación web construido a partir de los ajustes de la instancia | `public, max-age=60, stale-while-revalidate=300`, ETag |
| `GET /sw.js`, `/icon.svg` y otros archivos directamente bajo la compilación | Servidos tal cual | `no-cache`, ETag |
| `GET /licenses/*` | Las licencias de lo que incluye la compilación (avisos de paquetes npm, y `LICENSE-Unicode.txt` y `OFL.txt` para datos incluidos que no son de npm). Servidas como `text/plain; charset=utf-8` sea cual sea la extensión | `no-cache`, ETag |
| `GET /robots.txt`, `/.well-known/tdmrep.json`, `/.well-known/security.txt` | Construidos a partir de los ajustes de la instancia (consulta [Motores de búsqueda y rastreadores de IA](crawlers.md)) | `public, max-age=3600`, ETag |

- Las rutas bajo `/api`, `/oauth`, `/auth`, `/.well-known`, `/nodeinfo`, `/ap`, `/media`, `/uploads`, `/theme` (la hoja de estilo del tema del servidor) y `/licenses`, y `/robots.txt`, nunca devuelven el shell; si ninguna otra ruta coincide devuelven un problema 404.
- El shell se sirve después de la negociación de contenido de ActivityPub. Solicitar una ruta del shell con un `Accept` de ActivityPub devuelve 404 (las páginas de publicaciones redirigen, como se describe abajo).
  El shell siempre lleva `Vary: Accept`. Cuando pones una CDN delante, incluye el tipo de `Accept` (ActivityPub o HTML) en la clave de caché, o excluye de la caché el shell, `/ap/*` y `/@*`.
  Sirve solo `/assets/*` desde la CDN.
- Los archivos de texto de la compilación (scripts, hojas de estilo, imágenes SVG, avisos de licencia) se incrustan solo comprimidos. Una solicitud que acepta Brotli los recibe con `Content-Encoding: br`, una que acepta gzip con `Content-Encoding: gzip`, y cualquier otra solicitud los recibe descomprimidos, siempre con `Vary: Accept-Encoding` y un ETag para cada codificación.
  Un proxy delante no necesita compresión propia para ellos, y una CDN debe mantener `Vary: Accept-Encoding` (o la codificación) en su clave de caché para `/assets/*`.
- El shell y las pantallas de inicio de sesión llevan cabeceras `Link` que nombran las hojas de estilo y el script de entrada que carga primero la página, a partir de las cuales una CDN como Cloudflare (su función Early Hints) puede crear sus propias pistas tempranas.
  Con [`web_early_hints`](../reference/configuration.md#web_early_hints) = `true`, el servidor también las envía primero en una respuesta `103 Early Hints`, para que el navegador obtenga los archivos mientras el servidor prepara la página. Está desactivado por defecto, ya que un proxy que no puede reenviar un 103 puede hacer fallar la solicitud:
  - Un Application Load Balancer externo de Google Cloud responde 502 o 503 (`backend_interim_response_not_supported`): déjalo desactivado detrás de uno, como hace el despliegue en Cloud Run.
  - nginx lo reenvía desde la 1.29.0, con `early_hints` definido.
  - El `reverse_proxy` de Caddy lo reenvía; una directiva `encode` lo descarta en versiones anteriores a la 2.8.
  - Los navegadores usan un 103 solo para una página a la que navegan, y Chrome y Edge solo sobre HTTP/2 y HTTP/3.
- Un binario compilado sin el cliente web (un `go build` simple, sin compilar antes el cliente) registra una advertencia al arrancar y sirve un shell sin scripts.

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

<!-- source: verify-a-binary 03daabe0c940 -->
### Verificar un binario

Cada archivo tiene una procedencia firmada mediante Sigstore, como una imagen, y la versión enumera el SHA-256 de los archivos en `SHA256SUMS`:

```sh
gh attestation verify my_v0.1.0_linux_amd64.tar.gz -R mycroblog/my \
  --signer-workflow mycroblog/my/.github/workflows/release.yml
sha256sum --ignore-missing -c SHA256SUMS
```
