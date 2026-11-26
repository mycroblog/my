<!-- source: deploying-on-google-cloud 03b87618467f -->
# Desplegar en Google Cloud

Cómo ejecutar `my` en Google Cloud con el módulo de OpenTofu `mycroblog-gcp`: un servicio de Cloud Run y un backend para cada rol
que eliges mediante el `kind` de su sección. Cada sección tiene por defecto la recomendación del módulo, que no ejecuta ninguna instancia
mientras nadie usa el servidor, salvo por un tick que la despierta cada minuto. Un servidor cuesta dinero: la configuración recomendada con una
base de datos en Neon sale a unos 37 $ al mes antes del almacenamiento y el tráfico, y [Costos](#costs) da la cifra de cada elección.

Lo que comparte todo despliegue con OpenTofu, el bloque y sus secciones, los secretos, la consola y las actualizaciones, está en
[Desplegar con OpenTofu](deploy-opentofu.md). Esta página añade lo que Google Cloud necesita.

<!-- source: what-it-creates 7b13c8bbc968 -->
## Qué crea

Un bloque de `mycroblog-gcp` crea lo siguiente. El primer tipo de cada fila es el recomendado y el predeterminado;
[la referencia de los módulos](../../deploy/opentofu/README.md#sections) enumera todos los campos.

| Rol | Sección | Recomendado | Otros tipos |
| --- | --- | --- | --- |
| Servidor | `service`, `jobs` | Un servicio de Cloud Run con `role = all`, un job de migración, un job de consola mientras lo necesites, una cuenta de servicio y acceso público | |
| Tráfico | `ingress` | `load-balancer`: un Application Load Balancer externo global con un certificado administrado por Google | `domain-mapping`, `cloudflare`, y un host de multimedia en caché en Cloudflare junto a cualquiera |
| Base de datos | `database` | `external`: un secreto con la URL de un PostgreSQL que ejecutas tú, como Neon | `cloud-sql`: una instancia, su base de datos, usuario y contraseña |
| Jobs | `queue` | `cloud-tasks`: una cola de Cloud Tasks, el job de Cloud Scheduler del tick y una cuenta de servicio que llama al servidor | `db`: los jobs en la base de datos |
| Multimedia | `media` | `gcs`: un bucket de Cloud Storage privado | `r2`, `s3` |
| Cachés y límites de tasa | `kv` | `db`: en la base de datos, sin nada que crear | `memorystore`, `external` (Valkey) |
| Actualizaciones en tiempo real | `stream` | `pubsub`: un tema y un rol personalizado con el que el servidor gestiona sus propias suscripciones | `postgres`, `valkey` |
| Correo | `mail` | Cualquier servicio SMTP, con un secreto para su contraseña | |
| KEK | `kek` | `secret-manager`: un secreto generado, montado como archivo | `kms`: una clave en Cloud KMS |
| Red | `network` | `none`: nada a lo que conectar | `vpc`, `existing` |
| Métricas, trazas, errores | `observability` | Cloud Monitoring, Cloud Trace y Error Reporting, que leen lo que Cloud Run les envía. El módulo activa sus API | `settings`, como Sentry |
| Alertas | `monitoring` | Una comprobación de disponibilidad, métricas basadas en registros y políticas de alerta: [Monitorización y alertas](#monitoring-and-alerts) | `null`, ninguna |

El módulo también genera la clave VAPID, y crea vacíos los secretos que solo tú puedes rellenar.

`examples/gcp-cloud-run-neon` es una forma de empezar: el proveedor y un bloque con cada tipo recomendado, que funciona con
cualquier base de datos externa, sea Neon o no. Añade a su bloque las secciones que cambies.

<!-- source: before-you-start 9869afda9a07 -->
## Antes de empezar

- Lo que necesita [el primer despliegue](deploy-opentofu.md#first-deployment), con `gcloud` como CLI de la plataforma, y
  `gcloud auth application-default login` para el proveedor.
- Un proyecto de Google Cloud con facturación, usado solo para esta instancia, y una cuenta en él con el rol Owner. El rol
  personalizado de `stream = pubsub` va en el proyecto, así que llegaría a las suscripciones de cualquier otra cosa que haya allí.

<!-- source: first-deployment 894b6af2cee9 -->
## Primer despliegue

Haz aquí los dos primeros pasos y sigue después [el primer despliegue](deploy-opentofu.md#first-deployment) con las notas del
tercero.

<!-- source: 1-enable-the-apis fdd2dc400c20 -->
### 1. Activa las API

Todo despliegue necesita estas:

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com
```

Añade la API de cada elección que necesite una:

| Elección | API |
| --- | --- |
| `ingress = load-balancer` | `compute.googleapis.com` |
| `database = cloud-sql` | `sqladmin.googleapis.com` |
| `kek = kms` | `cloudkms.googleapis.com` |
| `network = vpc` o `existing` | `compute.googleapis.com`, `servicenetworking.googleapis.com`, `networkconnectivity.googleapis.com` |
| `kv = memorystore` | `memorystore.googleapis.com`, `serviceconsumermanagement.googleapis.com` y las de la red |

`translation` activa su propia API, y también `observability`. [Importar los cargos de uso de la nube](billing-import.md) enumera lo que
necesita `billing`.

<!-- source: 2-prepare-the-database b54e3b01002b -->
### 2. Prepara la base de datos

- **`external`**: crea la base de datos y guarda su URL de conexión directa y, si la tiene, su URL agrupada para
  `secrets_to_fill`: [La base de datos](#the-database) dice cuál va dónde. El módulo nunca la crea, así que su contraseña queda
  fuera del estado de OpenTofu.
- **`cloud-sql`**: nada que hacer. El módulo crea la instancia y escribe su URL, con una contraseña generada, en el secreto.

<!-- source: 3-deploy 4470affd40e4 -->
### 3. Despliega

Sigue [el primer despliegue](deploy-opentofu.md#first-deployment). En Google Cloud:

- **Variables**: `mail_from`, `mail_smtp_host` y `mail_smtp_username` del ejemplo configuran el [correo](email.md). El puerto
  es el 587, ya que Google Cloud bloquea el puerto saliente 25.
- **Secretos**: la KEK es `my-kek` y la clave VAPID `my-vapid-key`, cada una empezando por `name`, y
  `gcloud secrets versions access latest --secret=<id> --project="$PROJECT"` las copia. La KEK llega al servidor como un
  archivo montado, nombrado por `MY_KEK_FILE`, y los demás secretos como variables de entorno.
- **Apply**: el job de migración va primero, con aquello de lo que depende, lo que incluye el ingress, ya que los ajustes del servidor
  lo nombran. La sonda de arranque del servicio espera a `/health/ready`, que responde `503` hasta que existe el esquema.
- **DNS**: añade los registros de `tofu output dns_records`, como dice [Cómo llega el tráfico al servicio](#how-traffic-reaches-the-service)
  para cada tipo. Hasta que el certificado esté activo, `https://{host}` falla con un error de TLS, así que la federación y los navegadores
  aún no pueden llegar al servidor. Nada más espera por él: Cloud Tasks y Cloud Scheduler llaman a la URL predeterminada del servicio,
  `https://<name>-<project number>.<region>.run.app`. **No desactives la URL predeterminada**, o los jobs se detienen. Cloud Run cuenta
  a estos dos llamadores como internos solo desde el mismo proyecto y en esa URL, lo que permite que un balanceador de carga la cierre a
  Internet.
- **El primer administrador**: el enlace permanece en Cloud Logging, donde cualquiera que pueda leer los registros del proyecto puede usarlo.

<!-- source: the-console-and-upgrades a40a1df56865 -->
## La consola y las actualizaciones

[La consola](deploy-opentofu.md#the-console) y [las actualizaciones](deploy-opentofu.md#upgrading) ejecutan ejecuciones de los jobs de Cloud Run
`<name>-console` y `<name>-migrate` desde donde aplicas, mediante `gcloud`. Necesitan credenciales que puedan ejecutar
los jobs y leer sus registros, `roles/run.developer` y `roles/logging.viewer`, que Owner incluye. Un apply interrumpido
deja la ejecución en marcha en Cloud Run, y el siguiente apply espera a que termine.

Para [comprobar antes el SQL](deploy-opentofu.md#checking-the-sql-first):

```sh
tofu apply -target='module.mycroblog.module.app.google_cloud_run_v2_job.one_off["migrate"]'
gcloud run jobs execute my-migrate --project="$PROJECT" --region="$REGION" --args=upgrade,-dry-run --wait
gcloud logging read 'resource.type="cloud_run_job" AND resource.labels.job_name="my-migrate"' --project="$PROJECT" \
  --freshness=1h --format='value(textPayload)' --order=asc
```

<!-- source: the-database 0303fdd046ef -->
## La base de datos

El módulo recomienda `external`, que no necesita red y no factura nada en Google Cloud: una base de datos que se detiene mientras está inactiva,
como Neon, mantiene barato un servidor tranquilo. `cloud-sql` lo mantiene todo en un solo proyecto, a un costo fijo. Mover datos entre
ellas es [un cambio de backend](deploy-opentofu.md#changing-a-backend).

<!-- source: an-external-postgresql d6604bfb9e2f -->
### Un PostgreSQL externo

`database = { kind = "external" }`, el valor predeterminado, acepta cualquier PostgreSQL al que Cloud Run pueda llegar: Neon, otro PostgreSQL
alojado, o uno que ejecutes tú, por Internet o en una dirección privada mediante [una red](#networks). El módulo crea el
secreto `my-database-url`, para la URL **directa**, que el job de migración y la consola usan siempre: `my schema apply` mantiene
un bloqueo de nivel de sesión, que un pooler en modo de transacción no conserva.

`database = { kind = "external", pooled = true }` añade el secreto `my-database-url-pooled`, para una URL **agrupada**, que el
servicio usa entonces. Un pooler le conviene al servidor, como dice [A través de un pooler de conexiones](database.md#through-a-connection-pooler).
Sin él, el servicio también usa la URL directa, así que `service.max_instances` multiplicado por el pool de cada instancia debe quedar
por debajo del límite de conexiones de la base de datos. El pool es de 4 conexiones por instancia por defecto, para las 80 solicitudes que una instancia
admite a la vez: súbelo con `pool_max_conns` en la URL que usa el servicio, como `...?sslmode=require&pool_max_conns=8`
([Esperar una conexión de base de datos](running.md#waiting-for-a-database-connection)).

**En Neon**, crea un proyecto en una región cercana a tu región de Cloud Run. Su cadena de conexión directa tiene el host sin más, y la
agrupada el host con `-pooler`, que es el pooler de Neon y le conviene al servidor: define `pooled = true`. Cuenta con el plan Launch,
ya que el tick predeterminado mantiene despierto el cómputo; el plan Free funciona con [las contrapartidas de Costos](#where-care-reduces-it).

<!-- source: cloud-sql dc9461df8cc2 -->
### Cloud SQL

`database = { kind = "cloud-sql" }` crea una instancia de PostgreSQL 18, `db-f1-micro` por defecto, con copias de seguridad diarias y
recuperación a un momento dado durante 7 días, su base de datos y su usuario, y una contraseña generada que no llega ni al plan ni al
estado. `tier`, `disk_size_gb`, `retained_backups` y `deletion_protection` la cambian. No se ha aplicado nunca: trata el
primer despliegue como su prueba.

- **Sin red**, Cloud Run llega a ella mediante el socket del conector de Cloud SQL, y la dirección pública de la instancia no admite
  nada más.
- **Con una red**, solo tiene una dirección privada, y un `pg_dump` debe ejecutarse desde dentro de la red.
- **No tiene pooler**: `pooled` se rechaza, y `service.max_instances` y `pool_max_conns` acotan las conexiones. Mantén el
  pool pequeño en `db-f1-micro`, que tiene 0,6 GB de memoria.
- **Nunca escala a cero**, así que se factura sea cual sea el tráfico, y el SLA de Google no cubre un nivel de núcleo compartido.

<!-- source: the-other-backends 91ef8e2dea11 -->
## Los demás backends

Cada uno tiene su recomendación como valor predeterminado, y [un cambio de `kind`](deploy-opentofu.md#changing-a-backend) pasa a otro.

- **Jobs**: `cloud-tasks` envía cada job al servicio, que puede escalar a cero. `queue.tick_schedule`, cada minuto por
  defecto, despierta al servidor y a la base de datos para los jobs periódicos. `db` consulta la base de datos desde un proceso que permanece activo, así que
  necesita `service.min_instances` de 1 o más y `service.cpu_always_allocated = true`.
- **Multimedia**: `gcs` crea un bucket privado, `media.bucket_name`, en la región. `r2` crea un bucket de R2 en
  `cloudflare.account_id`, y `s3` usa cualquier almacén compatible con S3 en su `endpoint`; ambos leen una clave de acceso de los secretos
  que enumera `secrets_to_fill`. Para compartir un bucket con otras instancias, `existing_bucket = true` usa el tuyo y `prefix`
  (`tenant-a/`) inicia el nombre de cada objeto; con `gcs` la cuenta de servicio llega entonces solo a los objetos que hay bajo él, lo que necesita
  acceso uniforme a nivel de bucket.
- **Cachés y límites de tasa**: `db` los guarda en la base de datos. `memorystore` crea Memorystore para Valkey, que necesita una
  red y no tiene contraseña: cualquier cosa de la red puede llegar a él. `external` lee de un secreto la URL de un Valkey que ejecutas tú.
- **Actualizaciones en tiempo real**: `pubsub` llega a todas las instancias. `postgres` usa `LISTEN` y `NOTIFY` mediante la conexión directa
  de la base de datos, y `valkey` el Valkey de `kv`.
- **La KEK**: `secret-manager` la genera una vez, para que la copies. `kms` la guarda en Cloud KMS, que nunca abandona, así que un
  volcado de la base de datos filtrado y una configuración filtrada juntos siguen sin exponer ningún secreto. Cambiarla en una instancia con datos es
  [una mudanza](deploy-opentofu.md#the-kek).

<!-- source: networks 9d28311e11a6 -->
## Redes

La configuración recomendada no necesita ninguna: una base de datos externa, Cloud Tasks, Cloud Storage y Pub/Sub se alcanzan todos por
Internet con la propia identidad del servicio, así que Cloud Run no se conecta a nada.

Necesitas una para Memorystore, que solo tiene direcciones privadas, para dar a Cloud SQL una dirección privada, o para llegar a una base de datos
tuya que esté en una. `network = { kind = "vpc" }` crea una VPC, sus subredes y el acceso a servicios privados, y
`network = { kind = "existing", ... }` se conecta a una VPC que tengas, que describes mediante `network`, `subnetwork`,
`private_services_range` y `memorystore_connection_policy`:

```hcl
  network = { kind = "vpc" }
  kv      = { kind = "memorystore" }
```

El servicio y los jobs se conectan mediante Direct VPC egress, y solo los rangos privados pasan por la red, así que Internet
conserva su propia ruta, sin Cloud NAT. Con `vpc`, `nat = true` envía todo por Cloud NAT en su lugar, para tener una dirección de
origen estática. Una red puede retrasar el arranque de una instancia un minuto o más, algo que un servidor que escala a cero encuentra
a menudo. La VPC que crea el módulo no se ha aplicado nunca.

<!-- source: how-traffic-reaches-the-service 094ddaf201c8 -->
## Cómo llega el tráfico al servicio

`ingress.kind` lo decide. El módulo recomienda el balanceador de carga, para producción. La asignación de dominio sirve para probar un
despliegue de forma barata.

| | `load-balancer` | `domain-mapping` |
| --- | --- | --- |
| Cuota mensual | Unos 18 $ por las reglas de reenvío, más un cargo por GB. Comprueba la [página de precios](https://cloud.google.com/vpc/network-pricing): no se pudo confirmar esta cifra allí | Ninguna |
| Estado | La forma que recomienda Google Cloud | Una función en Preview que Google Cloud no recomienda para producción, por la latencia |
| Regiones | Cualquiera | `asia-northeast1`, `us-central1` y unas pocas más |
| DNS | Un registro `A` de `host` a su dirección. Deja desactivado cualquier proxy de tu proveedor de DNS para él | Los registros que enumera `dns_records` |
| Certificado | Administrado por Google, activo cuando el DNS apunta a la dirección, lo que puede tardar un poco | Administrado por Google, hasta 24 horas |
| Propiedad del dominio | Nada más allá del DNS | Quien aplica debe ser antes un [propietario verificado](https://cloud.google.com/run/docs/mapping-custom-domains) del dominio |
| URL `run.app` | Rechazada desde Internet, así que nadie rodea el balanceador de carga | Accesible para cualquiera: una asignación de dominio no puede añadir una cabecera, así que el origen queda abierto |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` y la dirección del balanceador de carga | `169.254.0.0/16` |

Cloud Tasks y Cloud Scheduler no llegan por ninguno de los dos: llaman directamente a la URL `run.app` predeterminada.

<!-- source: cloudflare 35dc1e54d2bb -->
### Cloudflare

`ingress = { kind = "cloudflare" }` pone Cloudflare delante de `host`, con la caché omitida para todo, y
`ingress.media_host` añade un host de multimedia en caché junto a cualquier tipo. Ambos necesitan `cloudflare = { zone_id = "..." }`, el proveedor
lee su token de `CLOUDFLARE_API_TOKEN`, y el módulo crea los registros, así que `dns_records` está vacío. Solo se han ejecutado
con proveedores simulados.

- **Tu plan decide el origen.** Cloud Run enruta por `Host`, y Cloudflare sobrescribe la cabecera Host solo en el plan
  Enterprise. En cualquier otro plan el módulo crea una asignación de dominio de cada host, con las contrapartidas de la tabla de arriba, y
  Cloudflare se conecta a ella. No está verificado si su certificado se emite y se renueva detrás del proxy, así que empieza con
  `cloudflare.proxied = false`. En el plan Enterprise, `cloudflare.host_header_override = true` se conecta en su lugar a la URL
  `run.app`, sin asignación de dominio.
- **Una cabecera cierra el origen.** Una regla de transformación de cabeceras de solicitud añade un valor que genera el módulo, y el servidor
  rechaza con un `403` una solicitud que no lo lleve, salvo las comprobaciones de salud y los `POST` propios de los jobs, que comprueban sus tokens.
  Así nadie rodea Cloudflare mediante la URL `run.app`, y nadie puede elegir la dirección de cliente que ven los límites de tasa.
- **Nada añade la cabecera mientras un registro no pasa por el proxy**, así que el servidor rechaza entonces todo salvo sus comprobaciones de salud:
  con `proxied = false` para el certificado de una asignación de dominio, activa el proxy cuando el certificado esté activo.
- **El valor acaba en el estado**, como dice [Secretos](deploy-opentofu.md#secrets), y en el secreto `my-origin-secret`.
  [La guía de la CDN](cdn.md#rotate-the-value) explica cómo rotarlo.
- **Desactiva Bot Fight Mode** en el panel, y no actives «I'm Under Attack»: un desafío en `/ap/`, `/.well-known/`
  o `/api/` rompe la federación, ya que otros servidores no son navegadores. Ambos cubren toda la zona, y ninguna regla puede eximir.
  El módulo no añade ningún desafío y desactiva Browser Integrity Check.
- **Email Address Obfuscation puede seguir activado.** Las propias páginas del servidor marcan el contenido cuyas direcciones debe dejar intactas.
- **Activa Always Use HTTPS** en el panel si quieres que `http://` se redirija. Cubre toda la zona, así que el módulo
  te lo deja a ti.
- **Tiempos de espera**: Cloudflare responde `524` cuando el origen no envía nada durante 125 segundos. Los jobs no se ven afectados, ya que no
  pasan por Cloudflare. Los streams envían un latido cada 30 segundos, por debajo del límite de inactividad de 900 segundos. No está verificado si Cloudflare
  almacena en búfer los streams en un host con proxy.
- **Video**: los términos de Cloudflare restringen servir video y una cantidad desproporcionada de archivos grandes en Free, Pro y
  Business, y puede actuar sobre el contenido. Decide esto antes de guardar video en caché en el host de multimedia.

**Un archivo eliminado permanece en caché.** El host de multimedia sigue el `Cache-Control` del servidor, así que un archivo local eliminado puede seguir visible
hasta un año, y uno remoto durante `remote_media_cache_ttl_seconds`. El servidor no lo purga. Cuando algo deba
desaparecer, púrgalo a mano, en **Caching > Configuration > Custom Purge** en el panel, o con:

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

Una URL de proxy lleva su `sig` en la cadena de consulta, que forma parte de lo que se guarda en caché, así que da la URL completa.

<!-- source: monitoring-and-alerts eb28e653de36 -->
## Monitorización y alertas

`monitoring` añade una comprobación de disponibilidad de `/health/live`, métricas basadas en registros y políticas de alerta con los nombres de
[el runbook](alerts.md), que notifican a las direcciones de `notification_emails` (`alert_emails` en el ejemplo) o a los canales
de `notification_channels`. La sección no tiene valor predeterminado: nombra a quién se notifica, o define `monitoring = null` para omitirla
por completo. No se ha aplicado nunca, así que pruébala una vez haciendo fallar una comprobación. Cada ejecución de la comprobación despierta una instancia:
`uptime_period`, 15 minutos por defecto, es la frecuencia.

<!-- source: traces da2e58dc6a3a -->
### Trazas

Cloud Trace muestra la traza que Cloud Run registra de cada solicitud, y las líneas de registro del servidor llevan su `trace_id`, ya que el
módulo confía en el ingress (`MY_TRUSTED_PROXIES`). Las líneas no rellenan el campo de traza que enlaza Cloud Logging
(`logging.googleapis.com/trace`), y no está verificado si Cloud Run reemplaza siempre el `traceparent` de una solicitud pública.
Los spans propios del servidor necesitarían un OpenTelemetry Collector a su lado, que el módulo no ejecuta.

<!-- source: limits 3d4721415f18 -->
## Límites

- **Subidas**: Cloud Run rechaza una solicitud HTTP/1 de más de 32 MiB, y el servidor habla HTTP/1.1, así que el módulo define
  `MY_MEDIA_MAX_AV_BYTES` en 30 MiB, frente al valor predeterminado de 100 MiB, para que una subida mayor reciba la respuesta del propio servidor.
  El propio límite de Cloudflare, 100 MB en Free y Pro, queda por encima.
- **Streams**: `service.request_timeout_seconds`, 15 minutos por defecto, termina un stream abierto, y el cliente se vuelve a conectar.
- **Conexiones de base de datos**: Google limita una instancia de Cloud Run a 100 conexiones a Cloud SQL.
- **Direcciones de cliente**: los límites de tasa dependen de `MY_TRUSTED_PROXIES`, que el módulo define a partir del ingress. Ningún documento que
  hayamos leído dice qué añade Cloud Run a `X-Forwarded-For` en una solicitud directa ni qué dirección de par ve un contenedor, así que
  `169.254.0.0/16` es una suposición. La forma documentada del balanceador de carga es `[<supplied>,]<client-ip>,<lb-ip>`. Comprueba las
  direcciones en los primeros registros de solicitudes: una dirección de cliente en `169.254` o de Google significa que el ajuste es incorrecto para tu proyecto.
- **Acceso público**: el servicio concede `roles/run.invoker` a `allUsers`, lo que rechaza una política de la organización sobre
  uso compartido restringido a dominios. Un proyecto así necesita una excepción.
- **Nombres que siguen ocupados**: Cloud SQL y Cloud Tasks rechazan el nombre de una instancia o cola eliminada durante aproximadamente una semana, y un
  rol personalizado eliminado conserva su ID durante un tiempo, así que un despliegue que se rehaga tras un `destroy` necesita otro `name`.

<!-- source: costs ea658d24d79d -->
## Costos

Estas cifras son una guía de qué esperar y de dónde reduce los costos el cuidado, no límites a los que se someta el despliegue. Son
precios de lista aproximados para una región, de 2026-10 y 2026-11: compruébalos en las páginas de precios de
[Cloud Run](https://cloud.google.com/run/pricing), [Cloud SQL](https://cloud.google.com/sql/pricing) y
[Neon](https://neon.com/pricing). El cómputo de Cloud Run es gratuito por cuenta de facturación hasta 180 000 vCPU-segundos y 360 000
GiB-segundos al mes.

Lo que paga todo despliegue, unos 0 $ en reposo:

| Parte | En reposo | Qué lo aumenta |
| --- | --- | --- |
| Cloud Run | 0 $: no se ejecuta ninguna instancia, y la CPU se factura solo durante las solicitudes | Cada stream abierto mantiene una instancia facturada; consulta abajo |
| Cloud Tasks | 0 $ | 1 millón de operaciones gratis, luego 0,40 $ por millón. Un job toma unas 2 operaciones (ponerlo en cola y entregarlo) |
| Cloud Scheduler | 0 $: el tick es un job, y 3 son gratis por cuenta de facturación | 0,10 $ por job por encima de eso |
| Pub/Sub | 0 $: 10 GiB gratis | Solo mientras hay streams abiertos |
| Secret Manager | Céntimos | Unas pocas versiones gratis, luego unos céntimos por versión |
| Cloud Storage | Unos 0,02 $ por GiB al mes | 5 GiB son gratis solo en `us-east1`, `us-west1` y `us-central1`. La multimedia sale por Cloud Run |
| Monitoring | 0 $ dentro de las asignaciones gratuitas | |

Lo que añade cada elección:

| Sección | Elección | Por mes |
| --- | --- | --- |
| `ingress` | `load-balancer` | Unos 18 $, sin confirmar en la página de precios, más un cargo por GB |
| | `domain-mapping` | 0 $ |
| | `cloudflare` | Tu plan de Cloudflare, y el origen: 0 $ con una asignación de dominio |
| `database` | `external` en el plan Launch de Neon | Unos 19 $ con el tick predeterminado, más 0,35 $ por GB-mes |
| | `external` en el plan Free de Neon | 0 $, dentro de sus límites: [abajo](#where-care-reduces-it) |
| | `external` en otro sitio | Lo que cueste esa base de datos |
| | `cloud-sql` | Unos 9,40 $ por `db-f1-micro` y 10 GiB en `us-central1`, más las copias de seguridad, sea cual sea el tráfico |
| `queue` | `db` | Una instancia facturada las 24 horas, con su CPU siempre asignada |
| `kv` | `memorystore` | Unos 23 $ por el nodo más pequeño, sea cual sea el tráfico |
| `kek` | `kms` | Céntimos, por la clave y sus operaciones |
| `network` | `vpc` con `nat = true` | Cloud NAT, por hora y por GB |

La configuración recomendada en el plan Launch de Neon, el balanceador de carga y Neon, sale a unos 37 $ al mes; con Cloud SQL, a
unos 27 $.

**El tick mantiene despierto a Neon.** Cada tick lee y escribe en la base de datos, y Neon detiene un cómputo solo tras 5 minutos sin
actividad, así que con el `* * * * *` predeterminado el cómputo nunca se detiene. Son 730 horas al mes, o 182,5 CU-horas a 0,25 CU,
que son los 19 $ de arriba. El tick se mantiene cada minuto para que los jobs periódicos se ejecuten a tiempo y una señal de despertar perdida se
recupere pronto. Cloud SQL factura lo mismo sea cual sea el tick.

**Un stream abierto mantiene una instancia facturada.** Con la facturación basada en solicitudes, una instancia se factura mientras atiende cualquier solicitud,
y las actualizaciones en tiempo real son una solicitud larga. Un cliente que mantiene su stream abierto todo el mes cuesta una instancia las
24 horas: unos 60 $ al mes con 1 vCPU y 512 MiB, tras la asignación gratuita. Más streams comparten la instancia, hasta
`service.concurrency`. Si los streams permanecen abiertos la mayor parte del tiempo, compara con la facturación basada en instancia
(`service.cpu_always_allocated`), o con una VM pequeña y [Docker Compose](deploy-docker-compose.md).

<!-- source: where-care-reduces-it 427be9d46d32 -->
### Dónde reduce los costos el cuidado

- **La asignación de dominio** en lugar del balanceador de carga elimina los 18 $, con
  [sus contrapartidas](#how-traffic-reaches-the-service). Sirve para probar un despliegue.
- **El plan Free de Neon** elimina los 19 $, y una instancia pequeña puede funcionar en él, con estas contrapartidas:
  - Permite 100 CU-horas al mes, que el tick predeterminado agota tras unos 17 días. Mantente dentro con una programación más
    larga, como `queue = { tick_schedule = "*/10 * * * *" }`: despierta como máximo la mitad del tiempo, unas 91 CU-horas, antes de que
    nada más lo despierte. Los jobs periódicos se ejecutan entonces con hasta 10 minutos de retraso, y una señal de despertar perdida se recupera más tarde.
  - Un servidor federado recibe entregas a todas horas, lo que también despierta la base de datos, así que una instancia ocupada lo supera
    sea cual sea la programación.
  - Contiene 1 GB por proyecto y un historial de 6 horas desde el que restaurar, y su cómputo siempre se detiene tras 5 minutos de inactividad, así que
    la primera solicitud después de eso tiene que esperarlo.
