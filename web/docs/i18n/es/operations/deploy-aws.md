<!-- source: deploying-on-aws d7e8643cd93d -->
# Desplegar en AWS

Cómo ejecutar `my` en AWS con el módulo de OpenTofu [`mycroblog-aws`](../../deploy/opentofu/README.md#aws): ECS en Fargate detrás de
un Application Load Balancer, los jobs en la base de datos, S3, SNS y el correo mediante Amazon SES. Nada escala a cero: espera
unos 42 $ al mes en reposo antes de la base de datos, y lee [Costos](#costs).

**Nada de esto se ha aplicado nunca en AWS.** Las comprobaciones lo planifican con proveedores simulados y sin credenciales, lo que muestra que
la configuración es coherente, no que AWS la acepte ni que el servidor arranque. Trata un primer despliegue como una prueba.

Lo que comparte todo despliegue con OpenTofu, las entradas del módulo, sus secretos, la consola y las actualizaciones, está en
[Desplegar con OpenTofu](deploy-opentofu.md). Esta página añade lo que AWS necesita.

<!-- source: what-it-creates 1ca642b05ab0 -->
## Qué crea

Un bloque de `mycroblog-aws` crea todo esto. Cada fila es una sección del módulo, y su valor predeterminado es la configuración
recomendada; [la referencia del módulo](../../deploy/opentofu/README.md#sections-of-mycroblog-aws) enumera todos los campos.

| Parte | Sección | Valor predeterminado | Otros tipos |
| --- | --- | --- | --- |
| Servidor | `service`, `jobs` | Un clúster y un servicio de ECS en Fargate con `role = all`, definiciones de tarea para la migración y, mientras la necesites, la consola, y un grupo de registros `/ecs/<name>` | |
| Red | `network` | Una VPC en dos zonas o más, con una subred pública y una privada en cada una | Una VPC tuya: [Redes](#networks) |
| Tráfico | `ingress` | Un Application Load Balancer, siempre presente, con tu certificado | CloudFront o Cloudflare delante de él: [Cómo llega el tráfico al servicio](#how-traffic-reaches-the-service) |
| Jobs | | Ninguno: las tareas permanecen activas y toman los jobs de la base de datos | |
| Base de datos | `database` | Un secreto con la URL de un PostgreSQL que ejecutas tú, como Neon | Amazon RDS: [Elige la base de datos](#2-choose-the-database) |
| Multimedia | `media` | Un bucket de S3 privado | Cloudflare R2, o cualquier almacén compatible con S3 |
| Cachés y límites de tasa | `kv` | Ninguno: los guarda la base de datos | ElastiCache Serverless para Valkey, o un Valkey que ejecutes tú |
| Actualizaciones en tiempo real | `stream` | Un tema de SNS. El servidor crea y elimina sus propias colas de SQS | La base de datos, o el Valkey de `kv` |
| Correo | `mail` | La identidad de SES del dominio de `mail.from`: [Correo mediante Amazon SES](#email-through-amazon-ses) | Cualquier servicio SMTP |
| Secretos | | Secrets Manager: una clave VAPID generada y los secretos que solo tú puedes rellenar, vacíos | |
| KEK | `kek` | Un secreto generado en Secrets Manager | Una clave en AWS KMS, que nunca sale de él |
| Métricas, trazas, errores | `observability` | Métricas e informes de errores en CloudWatch, trazas a X-Ray mediante un collector en cada tarea: [Trazas a X-Ray](#traces-to-x-ray) | Tus propios exportadores, mediante `settings` |
| Alertas | `monitoring` | Un canary de CloudWatch Synthetics y alarmas: [Monitorización y alertas](#monitoring-and-alerts) | Ninguno, con `null` |

`translation = {}` añade Amazon Translate, `search = { domain_name = "..." }` un dominio de Amazon OpenSearch Service que
creaste, y `billing = {}` la importación del costo de uso desde Cost Explorer: [Traducir publicaciones](translation.md#amazon-translate),
[Motores de búsqueda](search-engines.md) e [Importar los cargos de uso de la nube](billing-import.md).

<!-- source: before-you-start 9d5614a650e1 -->
## Antes de empezar

- Lo que necesita [el primer despliegue](deploy-opentofu.md#first-deployment), con `aws` como CLI de la plataforma.
- Una cuenta de AWS, usada solo para esta instancia, y credenciales de ella que puedan crear todo lo anterior, como
  `AdministratorAccess`.
- Una región. SES envía desde ella, y el módulo la toma del proveedor `aws`.
- Recomendado: una zona alojada pública de `host` en Route 53, donde el módulo crea entonces el registro de `host`, también en el ápice
  de una zona.

<!-- source: first-deployment 26b562a3db03 -->
## Primer despliegue

Haz aquí los tres primeros pasos y sigue después [el primer despliegue](deploy-opentofu.md#first-deployment) con las notas del
cuarto.

<!-- source: 1-request-the-certificates 3b26d03ba06c -->
### 1. Solicita los certificados

El balanceador de carga necesita un certificado validado para `host` en AWS Certificate Manager, en la región. El módulo lo toma como un
ARN, ya que validarlo dentro del apply retendría el apply hasta que añadas registros que solo el apply imprime.

```sh
aws acm request-certificate --region ap-northeast-1 --domain-name social.example.org --validation-method DNS
aws acm describe-certificate --region ap-northeast-1 --certificate-arn "$CERTIFICATE_ARN" \
  --query 'Certificate.DomainValidationOptions[].ResourceRecord'
# Add that CNAME record at your DNS provider, then:
aws acm wait certificate-validated --region ap-northeast-1 --certificate-arn "$CERTIFICATE_ARN"
```

Con `ingress.kind = cloudfront`, solicita un segundo en `us-east-1`, la única región de la que CloudFront acepta uno; en `us-east-1`
un certificado sirve para ambos. Con `ingress.media_host`, el certificado también debe cubrir ese host.

<!-- source: 2-choose-the-database d9cc7b8b2210 -->
### 2. Elige la base de datos

`database.kind` la elige. En ambos casos las tareas consultan la base de datos en busca de jobs aproximadamente una vez por segundo, así que nunca descansa.

**`external`, el valor predeterminado y el recomendado**, lee la URL de un PostgreSQL que ejecutas tú, como [Neon](https://neon.com),
de un secreto que rellenas. El módulo nunca crea la base de datos, así que su contraseña queda fuera del estado de OpenTofu. En Neon,
crea un proyecto en una región cercana a la tuya y copia sus cadenas de conexión:

- La **directa** (el host sin `-pooler`) es obligatoria, en el secreto `<name>-database-url`. La migración y la
  consola siempre la usan: `my schema apply` mantiene un bloqueo de nivel de sesión, que un pooler en modo de transacción no conserva.
- La **agrupada** (el host con `-pooler`) es opcional, para el servicio: define `database = { kind = "external", pooled = true }`,
  y el módulo crea el secreto `<name>-database-url-pooled`. Consulta
  [A través de un pooler de conexiones](database.md#through-a-connection-pooler).

Cuenta con el plan Launch de Neon: como el cómputo nunca se detiene, las horas de cómputo del plan Free se agotan en el mes.

**`rds`** crea Amazon RDS para PostgreSQL en las subredes privadas, con una contraseña que el módulo genera y escribe él mismo en
`<name>-database-url`, así que no hay nada que rellenar:

```hcl
  database = { kind = "rds" }
```

Por defecto es `db.t4g.micro` con 20 GB (`instance_class`, `allocated_storage_gb`), conserva 7 días de copias de seguridad automáticas y
tiene protección contra eliminación. No tiene dirección pública, así que solo las tareas llegan a ella: un `pg_dump` debe ejecutarse desde dentro de la VPC, y
antes de una actualización un snapshot ocupa su lugar,
`aws rds create-db-snapshot --db-instance-identifier my --db-snapshot-identifier my-before-upgrade`. No tiene URL agrupada.

<!-- source: 3-write-the-block db167b7d9566 -->
### 3. Escribe el bloque

`examples/aws-ecs-neon` es una forma de empezar: el proveedor `aws` y un bloque con los valores predeterminados, cuyas variables
enumera `terraform.tfvars.example`. Empieces por donde empieces, el bloque necesita esto:

```hcl
provider "aws" {
  region = "ap-northeast-1"
}

module "mycroblog" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/mycroblog-aws?ref=v0.1.0&depth=1"

  host       = "social.example.org"
  network    = { egress = "public-ip", availability_zones = ["ap-northeast-1a", "ap-northeast-1c"] }
  ingress    = { certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/00000000-0000-0000-0000-000000000000" }
  media      = { bucket_name = "social-example-org-media" }
  mail       = { from = "noreply@social.example.org" }
  monitoring = { notification_emails = ["ops@social.example.org"] }
}
```

- `network.egress` no tiene valor predeterminado, ya que ambas salidas se facturan por hora: lee [Redes](#networks).
- `ingress.route53_zone_id` nombra la zona alojada de `host`, si tienes una.
- `media.bucket_name` debe ser único en todo AWS.
- `monitoring` nombra a quién notifican las alarmas, o es `null`.

Añade la sección de la base de datos que elegiste, y cualquier otra que cambies.

<!-- source: 4-deploy 891c5c7cef83 -->
### 4. Despliega

Sigue [el primer despliegue](deploy-opentofu.md#first-deployment). En AWS, donde los comandos de esta página usan el `name` predeterminado,
`my`:

- **Secretos**: están en Secrets Manager, con el nombre `<name>-...`. `tofu output kek` tiene el comando que copia la KEK, y
  `aws secretsmanager get-secret-value --secret-id my-vapid-key --query SecretString --output text` copia la clave VAPID. La
  KEK llega al servidor como `MY_KEK`, ya que Fargate no monta ningún archivo. Una tarea lee sus secretos al arrancar: después de cambiar
  uno, ejecuta `aws ecs update-service --cluster my --service my --force-new-deployment`, y lo mismo para cada worker,
  `my-<key>`.
- **Apply**: la definición de tarea de la migración va primero, con aquello de lo que depende, y los servicios solo cuando la migración
  tuvo éxito. La comprobación del balanceador de carga consulta `/readyz`, que responde `503` hasta que existe el esquema.
- **DNS**: `tofu output dns_records` enumera el registro de `host`, salvo que Route 53 o Cloudflare lo creen, y los tres registros DKIM
  de SES. En el ápice de una zona sin Route 53, el registro de `host` necesita lo que tu proveedor de DNS llama un `ALIAS` o
  `ANAME`, como dice `tofu output dns_note`.
- **Correo**: SES envía solo a las direcciones que verificaste hasta que AWS concede el acceso a producción, así que nadie más puede registrarse todavía:
  lee [Correo mediante Amazon SES](#email-through-amazon-ses).
- **Alertas**: cada dirección de `monitoring.notification_emails` recibe un correo de suscripción de SNS, que debe confirmar.
- **El primer administrador**: el enlace permanece en CloudWatch Logs, en `/ecs/<name>`, donde cualquiera que pueda leer los registros de la cuenta
  puede usarlo.

<!-- source: the-console-and-upgrades-on-aws 0e4bc0bdeff2 -->
## La consola y las actualizaciones en AWS

[La consola](deploy-opentofu.md#the-console) y [las actualizaciones](deploy-opentofu.md#upgrading) ejecutan tareas de Fargate puntuales desde donde
aplicas, mediante `aws`, y leen su salida de CloudWatch Logs. Un apply interrumpido deja la tarea en ejecución, y el siguiente
apply espera a que termine. En una actualización, cada definición de tarea recibe una revisión nueva con la imagen, que nada ejecuta hasta que un
servicio la toma; cada servicio espera entonces a que arranquen sus tareas nuevas, y en el servicio predeterminado a que superen la
comprobación del balanceador de carga, antes de que se vayan las antiguas, y el interruptor de circuito del despliegue devuelve una revisión cuyas tareas no arrancan a la
anterior.

Las credenciales de donde aplicas, o del trabajo de CI que aplica, necesitan además de lo que necesita el propio apply:

- para la migración: `ecs:RunTask`, `ecs:DescribeTasks`, `ecs:ListTasks`, `logs:FilterLogEvents` e `iam:PassRole` sobre los
  dos roles de las tareas;
- para la consola, además: `ecs:DescribeServices`, `ecs:DescribeTaskDefinition`, `ecs:ListTagsForResource` y
  `ecs:RegisterTaskDefinition`, ya que la consola toma primero la imagen del servicio en ejecución.

Para [comprobar antes el SQL](deploy-opentofu.md#checking-the-sql-first), activa la consola mientras el bloque aún nombra la
versión actual, luego sube `ref`, aplica solo la definición de tarea de la migración y ejecuta con ella `my upgrade -dry-run`:

```sh
tofu apply -var console_enabled=true
# Name the new release in main.tf, then:
tofu init
tofu apply -var console_enabled=true -target='module.mycroblog.module.app.aws_ecs_task_definition.task["migrate"]'
tofu output -json console | jq -c '.task_definition = "my-migrate" | .log_prefix = "migrate" | del(.service)' |
  "$SCRIPTS/console.sh" upgrade -dry-run
```

<!-- source: monitoring-and-alerts 9b712c82cae2 -->
## Monitorización y alertas

`monitoring` añade un canary de CloudWatch Synthetics de `/readyz`, métricas en CloudWatch y alarmas con los nombres de
[el runbook](alerts.md#aws-ecs), que notifican a las direcciones de `notification_emails` o a los temas de SNS de
`notification_topic_arns`. La entrada no tiene valor predeterminado: nombra a quién se notifica, o define `monitoring = null` para omitirlo todo.
Cada ejecución del canary se factura, y `uptime_period_minutes`, 15 por defecto, es la frecuencia con la que se ejecuta: lee [Costos](#costs).

<!-- source: choosing-the-backends b83753b1f860 -->
## Elegir los backends

Cada sección tiene por defecto el tipo recomendado; [cambia el `kind` de una sección](deploy-opentofu.md#changing-a-backend) para
otro. La base de datos está [arriba](#2-choose-the-database).

| Sección | Recomendado | Otros tipos, y cuándo |
| --- | --- | --- |
| `media` | `s3`, un bucket en la región, con `versioning` desactivado | `r2` en `cloudflare.account_id`, o `s3-compatible` en `endpoint`, cada uno con una clave de acceso en secretos que rellenas. Para compartir un bucket, `existing_bucket = true` usa el tuyo y `prefix` (`tenant-a/`) inicia el nombre de cada objeto; con `s3` el rol de las tareas llega entonces solo a los objetos que hay bajo él |
| `kv` | `db`, que no cuesta nada más | `elasticache`, que se factura aunque esté inactivo y vive en las subredes privadas, o `external`, una URL de Valkey en un secreto que rellenas |
| `stream` | `sns`, que llega a todas las tareas | `postgres`, mediante la conexión directa de la base de datos, o `valkey`, el Valkey de `kv` |
| `mail` | `ses`, sin contraseña | `smtp`, con `smtp.host`, `smtp.port` (587) y `smtp.username`, cuya contraseña va en un secreto que rellenas |
| `kek` | `secrets-manager`, que copias una vez | `kms`, una clave que nunca sale de AWS KMS. En una instancia con datos, [cambiarla](deploy-opentofu.md#the-kek) es una mudanza |
| `observability` | CloudWatch y X-Ray | `traces = "none"`, o `"settings"` para cualquier señal que envíes a otro sitio, como Sentry |

`architecture = "arm64"` ejecuta las tareas en Graviton, que cuesta menos; la imagen publicada tiene ambas.

<!-- source: email-through-amazon-ses 0d8bc9f93296 -->
## Correo mediante Amazon SES

`mail.kind = "ses"`, el valor predeterminado, envía con la API de SES en la región del despliegue, desde el dominio de `mail.from`. El
módulo crea la identidad del dominio con Easy DKIM y permite a las tareas `ses:SendEmail` sobre ella, así que no hay contraseña, y
`mail.configuration_set` crea un conjunto de configuración y lo aplica a cada mensaje, o con
`existing_configuration_set = true` nombra uno que comparten las instancias; cada instancia actúa entonces solo sobre las notificaciones del
correo que envió. Antes de que SES envíe a tus usuarios, añade los
registros que enumera `tofu output dns_records` y sal del sandbox:
[Enviar correo con Amazon SES](email-ses.md) tiene los pasos, y cómo se entera el servidor de los rebotes y las quejas.

<!-- source: traces-to-x-ray e5ad1c35fdd5 -->
## Trazas a X-Ray

`observability.traces = "x-ray"` es el valor predeterminado, porque el collector encaja con la configuración recomendada sin cargo adicional. Es el
collector de AWS Distro for OpenTelemetry, que se ejecuta junto al servidor en cada tarea, escucha solo en el loopback de la tarea y
escribe en X-Ray con el rol de las tareas (`xray:PutTraceSegments`). Toma 32 unidades de CPU y 128 MiB de cada tarea, que AWS
factura solo por el tamaño de la tarea, así que la tarea de servicio predeterminada de 256 unidades de CPU y 1024 MiB cuesta lo mismo y da al
servidor 224 unidades de CPU y 896 MiB. El `traces_sample_ratio` predeterminado, 1 %, registra unas 30 000 trazas al mes para 100 000
solicitudes al día, por debajo de las 100 000 trazas al mes que X-Ray registra gratis.

Lo que sí cuesta es la imagen del collector, descargada en cada arranque de una tarea, y la memoria del servidor. Define `traces = "none"` cuando
no leas las trazas, o para una tarea demasiado pequeña para ceder 128 MiB: el plan rechaza un tamaño que deje al servidor menos de
128 unidades de CPU o 512 MiB, en `service`, `jobs` o un worker. El collector no es esencial: si se detiene, el servidor sigue
sin trazas. No está verificado que X-Ray acepte sus spans.

<!-- source: networks 36dc33a6cbef -->
## Redes

ECS siempre necesita una VPC. `network.kind = "vpc"`, la recomendada, crea una en `cidr_block` (`10.0.0.0/16`) en
`availability_zones`, al menos dos, que necesitan el balanceador de carga y RDS: una subred pública y una privada en cada zona, y una
puerta de enlace de Internet. Es solo IPv4, así que el servidor no llega a ningún par que sea solo IPv6. `network.kind = "existing"` toma una VPC tuya
en su lugar: `vpc_id`, `vpc_cidr`, `public_subnet_ids`, `public_subnet_cidrs` y `private_subnet_ids`.

Una tarea necesita una salida a Internet, para la imagen, la base de datos, la federación y las API de AWS, y `network.egress` la elige.
El tráfico entrante llega a una tarea solo a través del balanceador de carga, en ambos casos:

| | `public-ip` (recomendado) | `nat` |
| --- | --- | --- |
| Subredes de las tareas | Públicas, con una dirección IPv4 pública en cada tarea | Privadas |
| Cada hora | 0,005 $ por la dirección de cada tarea | Una puerta de enlace NAT, unos 0,045 $, y su dirección |
| Cada GB | Nada por la dirección | Unos 0,045 $ a través de la puerta de enlace, multimedia y federación incluidas |
| Con `existing` | Nada más | Tus subredes privadas ya deben llegar a Internet |

Usa `public-ip` salvo que una política tuya prohíba las subredes públicas: cuesta mucho menos y no está más abierta. `vpc` crea una puerta de enlace
NAT, en la primera zona: cuando esa zona cae, las subredes privadas pierden Internet hasta que vuelve. RDS y
ElastiCache viven en las subredes privadas con cualquiera de las dos, y no necesitan salida.

<!-- source: how-traffic-reaches-the-service d009efcf3ee2 -->
## Cómo llega el tráfico al servicio

El balanceador de carga siempre está presente, con el certificado de `ingress.certificate_arn`, y las tareas solo lo admiten a él.
`ingress.kind` decide qué hay delante de él:

| | `load-balancer` (recomendado) | `cloudfront` | `cloudflare` |
| --- | --- | --- | --- |
| `host` apunta a | El balanceador de carga | Una distribución de CloudFront | Cloudflare, que se conecta al balanceador de carga |
| En caché | Nada | `/media/`, según el `Cache-Control` del servidor | Nada en `host`; `media_host` añade un host en caché |
| Registros | Route 53 los crea con `route53_zone_id`, o `dns_records` los enumera | Route 53 con `route53_zone_id`, o `dns_records` | Cloudflare lo crea |
| Necesita | | `cloudfront_certificate_arn`, en `us-east-1`. `cloudfront_price_class` elige las ubicaciones de borde | `cloudflare = { zone_id = "..." }`, y el token en `CLOUDFLARE_API_TOKEN` |
| Cierra el origen mediante | El grupo de seguridad de las tareas | El grupo de seguridad del balanceador de carga admite solo a CloudFront, y su listener rechaza una solicitud sin una cabecera que envía CloudFront | Una cabecera que añade la regla de Cloudflare, que el servidor comprueba |

Detrás de CloudFront o Cloudflare, el valor de la cabecera acaba en el estado de OpenTofu, que debe permanecer privado. Un registro del
balanceador de carga anterior a CloudFront debe eliminarse primero: un registro existente con el mismo nombre hace fallar el apply. Las
direcciones de CloudFront las da AWS al planificar, así que aplica de vez en cuando, o un borde con una dirección nueva comparte los límites de tasa de sus espectadores.

`ingress.media_host` añade un host de multimedia en caché en Cloudflare, junto a `load-balancer` o `cloudflare`, con
`cloudflare.zone_id`. Un archivo eliminado permanece en caché hasta que lo purgas: lee [Servir multimedia mediante una CDN](cdn.md). Con Cloudflare
delante de cualquier cosa, desactiva Bot Fight Mode en su panel, ya que un desafío en `/ap/`, `/.well-known/` o `/api/` rompe la
federación, y el módulo no lo gestiona. Los tipos de Cloudflare solo se han ejecutado con proveedores simulados.

<!-- source: limits 42ed50837851 -->
## Límites

- **Salud**: la única comprobación del balanceador de carga consulta `/readyz`, que también falla mientras la base de datos está caída: tras 2,5 minutos de
  fallos ECS sustituye la tarea, y sigue haciéndolo hasta que la base de datos vuelve. No hay comprobación de contenedor, así que un worker,
  que no tiene balanceador de carga, falla por una mala imagen solo en sus registros.
- **Subidas**: el balanceador de carga no fija ningún límite al cuerpo de una solicitud, así que deciden los ajustes `media_max_*` del servidor.
- **Streams**: un stream envía un latido cada 30 segundos, por debajo del tiempo de inactividad de 60 del balanceador de carga. Permanece abierto hasta que
  su tarea se va, y el cliente se vuelve a conectar entonces.
- **Direcciones de cliente**: los límites de tasa dependen de lo que el servidor considera de confianza, que el módulo define a partir del ingress. Comprueba las direcciones
  de cliente en los primeros registros de solicitudes: una dirección de las subredes del balanceador de carga, o de CloudFront, significa que es incorrecta.
- **El disco** se va con la tarea, así que el plan rechaza SQLite y la multimedia en el sistema de archivos.
- **Los secretos** bajo una clave de KMS gestionada por el cliente también necesitan `kms:Decrypt`, que el módulo no concede.

<!-- source: costs 19e0e43bc201 -->
## Costos

Nada escala a cero en ECS: la tarea, el balanceador de carga y sus direcciones públicas se facturan todo el mes. Estas cifras proceden de
la lista de precios de AWS para Tokio, de 2026-10 y 2026-11, salvo que una fila diga otra cosa, y no se han contrastado con una factura.

| Parte | Por mes | Elección |
| --- | --- | --- |
| Una tarea de 0,25 vCPU y 1 GB | 13,26 $ en x86, 10,61 $ en arm64 | `service`, `architecture`. Cada worker añade sus propias tareas |
| Balanceador de carga | 17,74 $, más de 0 a 5,84 $ por cada unidad de capacidad usada todo el mes | Siempre presente |
| Las direcciones públicas del balanceador de carga | 7,30 $, para dos zonas | Una por zona |
| La salida | 3,65 $ por tarea con `public-ip`; unos 36,50 $ y 0,045 $ por GB con `nat`, a la tarifa de Ohio | `network.egress` |
| Base de datos | Unos 19 $ en el plan Launch de Neon a 0,25 CU, ya que el cómputo nunca se detiene; unos 21 $ con `rds`, `db.t4g.micro` y 20 GB | `database` |
| Cachés y límites de tasa | 0 $ con `db`; con `elasticache`, unos 6,13 $, o unos 61 $ según otro documento de AWS, en el norte de Virginia | `kv` |
| El canary | Unos 3,34 $ cada 15 minutos; unos 51,72 $ cada minuto | `monitoring.uptime_period_minutes` |
| Trazas | 0 $ hasta 100 000 trazas al mes | `observability.traces` |

**En reposo, unos 42 $ con `public-ip` y unos 75 $ con `nat`**, más las unidades de capacidad, antes de la base de datos, el almacenamiento, el tráfico
y el canary. No se leen, así que no están en ninguna fila: CloudFront, Secrets Manager, CloudWatch Logs, S3, SNS y SQS, los datos que salen de AWS y cualquier
nivel gratuito. Las tareas puntuales cuestan su tamaño por los minutos que se ejecutan, y una actualización ejecuta una segunda tarea de servicio durante unos minutos.
