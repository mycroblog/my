<!-- source: deploying-on-cloud-run 7728d9a37c56 -->
# Desplegar en Cloud Run

Cómo ejecutar `my` en Google Cloud con el módulo de OpenTofu [`mycroblog-gcp`](../../deploy/opentofu/README.md): Cloud Run, Cloud Tasks, Cloud Scheduler, Cloud Storage y Pub/Sub, con una base de datos PostgreSQL en Neon, detrás de un Application Load Balancer externo global. El servicio no ejecuta ninguna instancia mientras nadie lo usa, salvo un tick que despierta al servidor cada minuto. Un servidor cuesta dinero: espera unos 37 $ al mes antes del almacenamiento, el tráfico y los streams abiertos, y lee [Costes](#costs) para saber qué se puede reducir con cuidado.

El módulo necesita OpenTofu 1.11 o posterior. Terraform no es compatible.

<!-- source: what-it-creates cae1e55a1722 -->
## Qué crea

Un bloque de `mycroblog-gcp` crea todo esto. Cada fila es una sección del módulo, y su valor predeterminado es lo que despliega esta guía; [el README del módulo](../../deploy/opentofu/README.md#sections) enumera los otros tipos.

| Parte | Sección | Recursos |
| --- | --- | --- |
| Servidor | `service`, `jobs` | Un servicio de Cloud Run con `role = all`, un job de migración, un job de consola mientras lo necesites, una cuenta de servicio y acceso público |
| Tráfico | `ingress` | Un Application Load Balancer externo global con un certificado. Una asignación de dominio o Cloudflare son los otros tipos, y un host de multimedia en Cloudflare puede acompañar a cualquiera |
| Jobs | `queue` | Una cola de Cloud Tasks, un job de Cloud Scheduler y una cuenta de servicio que llama al servidor |
| Multimedia | `media` | Un bucket privado |
| Actualizaciones en tiempo real | `stream` | Un topic y un rol personalizado. El servidor crea y elimina sus propias suscripciones |
| Base de datos | `database` | Un secreto para su URL: la base de datos la creas tú a mano |
| Correo | `mail` | Un secreto para la contraseña SMTP: cualquier servicio SMTP |
| Secretos | | Una clave VAPID generada, y los secretos que solo tú puedes rellenar, vacíos |
| KEK | `kek` | Un secreto generado montado como archivo. Cloud KMS es el otro tipo |
| Métricas, trazas, errores | `observability` | Las API de Cloud Monitoring, Cloud Trace y Error Reporting, que leen lo que Cloud Run les envía |

Los valores de los secretos nunca llegan al plan ni al estado de OpenTofu: los que vienen de otro lugar los añades tú mismo, y el módulo genera la KEK y la clave VAPID mediante atributos de solo escritura.

<!-- source: before-you-start 10846ce6f3b1 -->
## Antes de empezar

- `tofu` 1.11 o posterior, `gcloud` y `jq`, donde aplicas: un apply que cambia la versión ejecuta la migración desde allí, mediante `gcloud`.
- Un proyecto de Google Cloud con facturación, usado solo para esta instancia, y una cuenta en él con el rol Owner.
- Un dominio para [`host`](../reference/configuration.md#host). No se puede cambiar después: lee [Ajustes que no se pueden cambiar después](production-checklist.md#settings-you-cannot-change-later).
- Una versión de `my` de las [versiones publicadas](running.md#get-the-image). No hay etiqueta `latest`.

<!-- source: first-deployment 996fcc6eb35e -->
## Primer despliegue

<!-- source: 1-enable-the-apis c7f2ef4a9ee9 -->
### 1. Activar las API

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com compute.googleapis.com \
  monitoring.googleapis.com
```

`compute.googleapis.com` es para el balanceador de carga.

<!-- source: 2-create-the-database-on-neon ca5f51e8979e -->
### 2. Crear la base de datos en Neon

Cuenta con el plan Launch: con el tick predeterminado, el cómputo nunca se detiene. El plan Free también funciona, con las contrapartidas de [Costes](#costs).

En la consola de Neon, crea un proyecto en una región cercana a tu región de Cloud Run y copia sus cadenas de conexión. El módulo nunca lo crea, así que su contraseña queda fuera del estado de OpenTofu. Un proyecto tiene dos cadenas de conexión, y el despliegue usa cada una donde corresponde:

- La **directa** (el host sin `-pooler`) es obligatoria. El job de migración y la consola siempre la usan: `my schema apply` mantiene un bloqueo a nivel de sesión, que un pooler en modo de transacción no conserva.
- La **agrupada** (el host con `-pooler`) es opcional, para el servicio. El pooler de Neon gestiona lo que necesita el servidor: consulta [A través de un pooler de conexiones](database.md#through-a-connection-pooler). Define `database = { pooled = true }`, y el módulo crea un segundo secreto para ella.

Sin una cadena agrupada, el servicio usa también la directa, en el límite de conexiones de la base de datos, que `service.max_instances` acota. Para subir el pool desde su valor predeterminado de 4 por instancia, para las 80 solicitudes que una instancia de Cloud Run atiende a la vez, añade `pool_max_conns` a la cadena que usa el servicio, por ejemplo `postgres://...-pooler...neon.tech/neondb?sslmode=require&pool_max_conns=8`. Consulta [Esperar una conexión a la base de datos](running.md#waiting-for-a-database-connection).

<!-- source: 3-copy-the-example-and-set-the-variables 1b99f37cb3a7 -->
### 3. Copiar el ejemplo y definir las variables

Empieza con una copia en un directorio vacío, con el archivo de bloqueo de proveedores, de la versión que despliegas:

```sh
VERSION=v0.1.0
git clone --depth 1 --branch "$VERSION" https://github.com/mycroblog/my.git my-source
mkdir my-deployment
cp -r my-source/deploy/opentofu/examples/gcp-cloud-run-neon/. my-deployment/
rm -rf my-source
cd my-deployment
cp terraform.tfvars.example terraform.tfvars
```

`main.tf` es el proveedor `google` y un bloque, `module "mycroblog"`. **Define `ref` con esa versión.** El bloque nombra una versión con `?ref=`, que la copia de una versión estable ya tiene bien. La imagen del servidor es la misma versión, así que `ref` por sí solo nombra lo que se ejecuta. Para una versión preliminar, define ambos tú mismo: `sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf`, y `image = { version = "0.1.0-rc.4" }` en el bloque, la versión sin su `v`, ya que el módulo de una versión preliminar no usa por defecto su propia imagen. El módulo existe solo desde la primera versión que lo contiene, así que una etiqueta anterior no tiene ejemplo que copiar. Conserva este directorio: contiene tu estado y tus ajustes.

Rellena `terraform.tfvars`. `mail_from`, `mail_smtp_host` y `mail_smtp_username` configuran el [correo](email.md); el puerto es el 587, ya que Google Cloud bloquea el puerto de salida 25. Para cambiar cualquier otra cosa, añade la sección al bloque de `main.tf`, como `ingress = { kind = "domain-mapping" }`: cada sección y lo que rechaza está en [el README del módulo](../../deploy/opentofu/README.md#sections).

`settings` acepta cualquier otro ajuste de la [referencia de configuración](../reference/configuration.md) por su clave, y `secrets` las claves de los ajustes secretos, cada uno de los cuales recibe un secreto vacío que rellenar. Una clave que una sección o el módulo define hace fallar el plan, igual que una clave que el módulo no acepta en Cloud Run, como `listen`; el error dice por qué. Decide ahora [`account_domain`](../reference/configuration.md#account_domain), como `settings = { account_domain = "example.org" }`, si los identificadores usan otro dominio.

**El estado** permanece por defecto en este directorio. No contiene valores secretos, pero es el único registro de lo que se creó: guárdalo en un bucket con un bloque `backend "gcs"` si más de una persona ejecuta esto.

<!-- source: 4-create-the-secrets-and-add-their-values 51cddae33ec9 -->
### 4. Crear los secretos y añadir sus valores

Esto crea los secretos y genera la KEK y la clave VAPID:

```sh
tofu init
tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secret_manager
```

`tofu init` también obtiene los scripts de esa versión: `migrate.sh`, que ejecuta un apply, y `console.sh`, que usa un paso posterior:

```sh
SCRIPTS=.terraform/modules/mycroblog/deploy/opentofu
```

**Copia la KEK ahora**, y guarda la copia aparte de la base de datos, por ejemplo en un gestor de contraseñas. Sin ella no se puede restaurar una [copia de seguridad](backup.md), y se pierden las claves de las cuentas. `tofu output kek` tiene el comando. Haz lo mismo con la clave VAPID, cuya pérdida termina con todas las suscripciones de Web Push:

```sh
gcloud secrets versions access latest --secret=my-kek --project="$PROJECT"
gcloud secrets versions access latest --secret=my-vapid-key --project="$PROJECT"
```

Después añade los valores que vienen de otro lugar. `tofu output secrets_to_fill` enumera cada secreto que el módulo creó vacío, con lo que va en él y el comando que lo añade. Una instancia cuyo secreto no se puede leer no arranca, así que añádelos antes del servicio:

```sh
printf '%s' 'postgres://...' | gcloud secrets versions add my-database-url --project="$PROJECT" --data-file=- # the direct string
printf '%s' '<the SMTP password>' | gcloud secrets versions add my-smtp-password --project="$PROJECT" --data-file=-
```

La KEK llega al servidor como un archivo montado, nombrado por `MY_KEK_FILE`. Los demás son variables de entorno que se resuelven cuando arranca una instancia, así que una versión nueva surte efecto en la siguiente instancia: despliega una revisión nueva para asegurarte.

**OpenTofu genera la KEK una vez y nunca más.** El secreto y su versión tienen `prevent_destroy`, y el número de versión que escribiría un valor nuevo está fijado en el módulo: una KEK aleatoria nueva deja ilegible todo lo que cifró la anterior. Para rotarla, sigue [Rotar la KEK](key-management.md#rotate-the-kek), añadiendo el valor nuevo con `gcloud secrets versions add`. OpenTofu no sigue lo que añades ahí.

Para mantener la KEK en Cloud KMS desde el principio, define `kek = { kind = "kms" }` antes de este paso, usa como destino `module.mycroblog.module.kek_kms` en lugar de `kek_secret_manager`, y activa `cloudkms.googleapis.com`.

<!-- source: 5-apply 2076866c0f3b -->
### 5. Aplicar

```sh
tofu apply
```

Esto crea todo lo demás, en un orden que necesita la sonda de arranque del servicio: la sonda espera a `/health/ready`, que responde `503` hasta que existe el esquema. El job de migración va primero, con aquello de lo que depende, que incluye la dirección del balanceador de carga, ya que los ajustes del servidor la nombran. Después el apply ejecuta `my upgrade` como una ejecución del job, desde tu máquina mediante `gcloud`, y espera a que termine: `my config check`, `my db upgrade -check`, `my schema apply` y `my db upgrade`, que se detiene en el primero que falla. Su salida aparece en la del apply. El primer comando que usa la base de datos registra en ella `host` y `account_domain`. Solo entonces el apply crea los servicios.

Si la migración falla, el apply se detiene antes de los servicios. Corrige la causa, a menudo un secreto sin valor, y ejecuta `tofu apply` de nuevo: ejecuta la migración otra vez.

<!-- source: 6-point-the-domain-at-the-load-balancer c81cc51be2af -->
### 6. Apuntar el dominio al balanceador de carga

`tofu output dns_records` enumera un registro `A`, de `host` a la dirección del balanceador de carga. Añádelo en tu proveedor de DNS. Si ese proveedor tiene un proxy propio, déjalo desactivado para este registro.

El certificado lo gestiona Google y se activa solo cuando el DNS apunta a la dirección, lo que puede tardar un rato. Hasta entonces `https://{host}` falla con un error TLS, así que la federación y los navegadores aún no pueden llegar al servidor. Nada más espera por ello: Cloud Tasks y Cloud Scheduler llaman a la URL `run.app` predeterminada del servicio, `https://<service>-<project number>.<region>.run.app`. **No desactives la URL predeterminada**, o los jobs se detienen. Esa URL rechaza las solicitudes de internet, y Cloud Run considera estas dos como internas solo desde el mismo proyecto y en esa URL, no en un dominio personalizado.

Consulta [Cómo llega el tráfico al servicio](#how-traffic-reaches-the-service) para ver qué más puedes elegir.

<!-- source: 7-create-the-first-administrator 19137d6d01f0 -->
### 7. Crear al primer administrador

Los jobs de Cloud Run no tienen terminal ni entrada estándar, así que `my admin create` imprime un enlace que establece la contraseña en lugar de leer una. Activa la [consola](#the-console) y ejecuta:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

Abre el enlace enseguida y establece la contraseña. Funciona una vez, durante una hora. **Permanece en Cloud Logging hasta entonces**, donde cualquiera que pueda leer los registros del proyecto puede usarlo. Si caduca, ejecuta `my admin account password-reset-link @alice` mediante la consola. Después activa la autenticación en dos pasos.

<!-- source: 8-go-through-the-checklist e7fe33d10e80 -->
### 8. Repasar la lista de comprobación

Repasa la [lista de comprobación de producción](production-checklist.md), empezando por el [correo](production-checklist.md#email): SPF, DKIM y DMARC en el dominio de `mail_from`.

<!-- source: the-console 52df6f8c43ac -->
## La consola

La consola ejecuta un comando de `my` con la imagen, los ajustes y los secretos del servicio. Existe solo mientras `console_enabled` está activado, así que desactívalo cuando termines:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` espera al comando, imprime su salida y su código de salida, y termina con ese código. Necesita `roles/logging.viewer`, porque la salida de un job va a Cloud Logging. Usa la imagen que ejecuta el servicio: tras una actualización que se detuvo en la migración, esa es la versión anterior, a la que la base de datos todavía se ajusta.

El comando no tiene entrada estándar ni terminal, así que un comando que lee de ellos una contraseña o un archivo no puede ejecutarse de este modo. Lo que imprime permanece en los registros, así que no ejecutes un comando cuya salida no escribirías allí.

<!-- source: upgrading fe1bed77d3af -->
## Actualizar

Lee antes [Actualizar](database.md#upgrading) y las notas de la versión, y avanza una versión menor cada vez. Una actualización es un apply de la versión nueva: el plan muestra cada servicio y job tomando su imagen, y el apply ejecuta la migración antes de que ninguno la sirva.

```sh
# 1. Back up the database while nothing writes: see "The maintenance mode" below, and Backup and restore.
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Name the new release in main.tf, as ?ref=v0.2.0, and fetch it.
tofu init

# 3. Read the plan, then apply it.
tofu plan
tofu apply
```

La imagen es la versión que nombra `ref`, así que cambiar `ref` es toda la actualización. Lee el plan antes de aplicarlo: el módulo de una versión nueva también puede cambiar recursos. El apply hace lo siguiente, y se detiene en el primer paso que falla:

1. Da a los jobs de migración y de consola la imagen nueva, con el resto del plan de lo que los servicios no dependen.
2. Ejecuta `my upgrade` como una ejecución del job de migración, desde donde aplicas, y espera a que termine: `my config check`, `my db upgrade -check`, `my schema apply` y `my db upgrade`. Su salida aparece en la del apply.
3. Despliega la revisión nueva de cada worker, y después la del servicio. Una revisión recibe tráfico solo cuando su sonda de arranque pasa en `/health/ready`.

**Donde aplicas** se necesitan `gcloud` y `jq`, y credenciales que puedan ejecutar el job y leer sus registros (`roles/run.developer` y `roles/logging.viewer`, que Owner incluye). Lo mismo vale para un job de CI que aplica.

**Cuando un paso falla**, los servicios permanecen en sus revisiones, y las notas de la versión o la salida de la migración dicen por qué. Corrige la causa y ejecuta `tofu apply` de nuevo, con la misma versión: ejecuta la migración otra vez, y los pasos que se completaron no encuentran nada que hacer. Un apply que se interrumpió, o que perdió su conexión, deja la ejecución en curso en Cloud Run; el siguiente apply espera a que termine antes de iniciar la suya. Para volver atrás después de que cambió el esquema, restaura la copia de seguridad: consulta [Revertir](database.md#rolling-back). Un apply con una versión anterior se detiene en la migración, ya que esa versión rechaza un esquema más nuevo, y no cambia ningún servicio.

Para probar un candidato a versión, o para mantener la imagen de una versión anterior bajo un módulo más nuevo, define tú mismo la versión; es la etiqueta de la imagen, sin la `v` de la versión:

```hcl
  image = { version = "0.2.0-rc.1" }
```

<!-- source: checking-the-sql-first 9ff10080c36d -->
### Comprobar antes el SQL

El plan no muestra el SQL de la migración. Para leerlo antes del apply, da al job de migración solo la imagen nueva, y ejecuta en él `my upgrade -dry-run`, que no cambia nada:

```sh
tofu apply -target='module.mycroblog.module.app.google_cloud_run_v2_job.one_off["migrate"]'
gcloud run jobs execute my-migrate --project="$PROJECT" --region=asia-northeast1 --args=upgrade,-dry-run --wait
gcloud logging read 'resource.type="cloud_run_job" AND resource.labels.job_name="my-migrate"' --project="$PROJECT" \
  --freshness=1h --format='value(textPayload)' --order=asc
```

<!-- source: destructive-changes 5b3fdea90a55 -->
### Cambios destructivos

Cuando las notas de la versión piden `-allow-destructive`, nombra esa versión en el bloque:

```hcl
  upgrade = { allow_destructive_for = "0.3.0" }
```

`my upgrade` recibe `-allow-destructive` solo en el apply de esa versión, así que un valor que se deja puesto no permite que ninguna versión posterior elimine datos.

<!-- source: pinning-the-image-by-digest b31962ecc908 -->
### Fijar la imagen por digest

La [lista de comprobación de producción](production-checklist.md#operations) recomienda un digest, de las notas de la versión:

```hcl
  image = { digest = "sha256:..." }
```

Un digest nombra una sola imagen, diga lo que diga la etiqueta que lo acompaña, así que cámbialo en la misma edición que `ref`: un digest que queda de la versión anterior mantiene la imagen de esa versión bajo la versión nueva.

<!-- source: the-maintenance-mode 518e9aa176c0 -->
### El modo de mantenimiento

El [modo de mantenimiento](maintenance.md) requiere tres apply, ya que los servicios cambian antes de la actualización y después de ella. Añádelo a los ajustes del bloque y aplica, haz la copia de seguridad, sube `ref` y aplica, y luego quítalo y aplica:

```hcl
  settings = { maintenance = "true" }
```

Sin él, la revisión antigua sigue sirviendo durante la migración, lo que [no está garantizado que funcione](database.md#the-server-during-an-upgrade). Los ajustes cambiados a mano con `gcloud` no perduran: el siguiente apply restablece lo que dice el bloque.

<!-- source: monitoring-and-alerts 930234cd0dde -->
## Monitorización y alertas

El módulo añade una comprobación de disponibilidad de `/health/live`, métricas basadas en registros y políticas de alerta con los nombres de [el runbook](alerts.md), que notifican a las direcciones de `monitoring.notification_emails` (`alert_emails` en el ejemplo) o a los canales de `notification_channels`. La entrada no tiene valor predeterminado: indica a quién se notifica, o define `monitoring = null` para omitirlo todo. Nunca se ha aplicado, así que pruébalo una vez haciendo fallar una comprobación. Cada ejecución de la comprobación despierta una instancia, lo que se suma a [la contrapartida del tick](#costs): `uptime_period`, 15 minutos por defecto, es la frecuencia.

<!-- source: changing-a-backend c20598caf230 -->
## Cambiar un backend

Cambia el `kind` de su sección. Cada tipo define lo que necesita en el servidor, y el plan dice qué cambia. Cambiar la base de datos también significa mover sus datos, ya que una nueva está vacía: haz una [copia de seguridad](backup.md) de la antigua y restáurala en la nueva mientras nada escribe. Por ejemplo, Cloud SQL en lugar de Neon:

```hcl
  database = { kind = "cloud-sql" }
```

Para un almacén fuera de Google Cloud, define `media.kind` como `s3`, para cualquier servicio compatible con S3 con su `endpoint`, o como `r2`, con `cloudflare.account_id`. Ambos leen una clave de acceso de los secretos que enumera `secrets_to_fill`.

<!-- source: when-you-need-a-network 0a4359c002ed -->
## Cuándo necesitas una red

Esta guía no necesita ninguna: Neon, Cloud Tasks, Cloud Storage y Pub/Sub se alcanzan todos por internet con la propia identidad del servicio, así que Cloud Run no se conecta a nada y no hay nada que pagar ni que mantener.

Necesitas una para poner Memorystore en el despliegue, ya que solo tiene direcciones privadas, o para dar a Cloud SQL una dirección privada en lugar de su dirección pública. `network = { kind = "vpc" }` crea una, y `network = { kind = "existing", ... }` se conecta a una VPC que tienes, que describes con `network`, `subnetwork`, `private_services_range` y `memorystore_connection_policy`:

```hcl
  network = { kind = "vpc" }
  kv      = { kind = "memorystore" }
```

Solo los rangos privados pasan por la red, así que el servicio llega a internet como antes, sin Cloud NAT. `nat = true` envía todo por Cloud NAT en su lugar, para una dirección de origen estática. Activa antes las API de Service Networking y Network Connectivity, y la API de Compute Engine. Una red añade al arranque en frío de una instancia, y Memorystore factura mientras el servidor está inactivo.

<!-- source: splitting-jobs-into-workers 831929244c6d -->
## Dividir los jobs en workers

Cuando un tipo de job retrasa a los demás, dale su propio servicio. Decide qué dividir a partir de las métricas, como dice [Dividir los workers por tipo de job](running.md#splitting-workers-by-job-type). Un solo valor, `workers`, llega al servicio, a la cola y a la monitorización, y cada cola, URL, ruta y ajuste sale de sus claves, así que no escribes ninguno:

```hcl
  workers = {
    delivery = {
      job_types     = ["federation.fanout", "federation.deliver"]
      max_instances = 20
      concurrency   = 16
    }
  }
```

El plan rechaza una división de los tipos que deben permanecer juntos. También necesita un broker que llegue a todos los procesos, lo que es `stream.kind = pubsub`.

Los dos esquemas comunes de esa página son así. Para evitar que las entregas retrasen al resto, usa el valor de arriba. Para mantener rápidos el correo y las notificaciones:

```hcl
  workers = {
    notify = {
      job_types = [
        "identity.verification_email", "identity.password_reset_email", "identity.password_notice_email",
        "identity.closure_email", "identity.deactivation_email",
        "notification.email", "notification.moderator_email", "notification.supporter_email",
        "notification.push", "notification.post_subscribers",
        "instance.notify_announcement", "voice.notify_started",
      ]
      max_instances = 10
    }
  }
```

Omite los tipos de los ajustes que no has definido, como dice esa página: un tipo que el servidor no ha registrado detiene el arranque, y el plan no puede ver cuáles son. Con `workers`, el servicio predeterminado no ayuda con estos tipos cuando está libre, ya que sus tareas van a su propia cola.

Cada worker acepta `job_types`, y `min_instances`, `max_instances`, `cpu`, `memory_mib`, `concurrency`, `max_concurrent_dispatches` y `max_dispatches_per_second`, cada uno el del servicio o el de la cola cuando no se nombra. El servicio de un worker no recibe tráfico del exterior. Una actualización mueve cada uno después de la migración y antes del servicio predeterminado, y sin nada que se lo indique: solo un paso que falla lo detiene. Cada worker puede traer lo que cuesta el servicio, y las conexiones de sus instancias cuentan para el límite de la base de datos.

<!-- source: how-traffic-reaches-the-service 156fc6f76743 -->
## Cómo llega el tráfico al servicio

`ingress.kind` lo decide. El valor predeterminado es el balanceador de carga, pensado para producción. La asignación de dominio es para probar un despliegue de forma barata: define `ingress = { kind = "domain-mapping" }`.

| | `load-balancer` (el predeterminado) | `domain-mapping` |
| --- | --- | --- |
| Cuota mensual | Unos 18 $ por las reglas de reenvío, más un cargo por GB. Comprueba la [página de precios](https://cloud.google.com/vpc/network-pricing): esta cifra no se pudo confirmar allí | Ninguna |
| Estado | La forma recomendada por Google Cloud | Una función en Preview que Google Cloud no recomienda para producción, por la latencia |
| Regiones | Cualquiera | `asia-northeast1`, `us-central1` y algunas más |
| DNS | Un registro `A`, de `tofu output dns_records` | Los registros que enumera la misma salida |
| Certificado | Gestionado por Google, activo cuando el DNS apunta a la dirección | Gestionado por Google, hasta 24 horas |
| Propiedad del dominio | Nada más allá del DNS | Tú, y toda otra cuenta que aplique, debéis ser [propietarios verificados](https://cloud.google.com/run/docs/mapping-custom-domains) antes de aplicar |
| URL `run.app` | Rechazada desde internet, así que nadie rodea el balanceador de carga | Accesible por cualquiera: una asignación de dominio no puede añadir una cabecera, así que el origen queda abierto |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` y la dirección del balanceador de carga | `169.254.0.0/16` |

Cloud Tasks y Cloud Scheduler no llegan por ninguno de los dos: llaman directamente a la URL `run.app` predeterminada.

<!-- source: cloudflare e55e367a0064 -->
### Cloudflare

`ingress = { kind = "cloudflare" }` pone Cloudflare delante de `host`, con la caché omitida para todo, y `ingress.media_host` añade un host de multimedia en caché junto a cualquier tipo. Ambos necesitan `cloudflare = { zone_id = "..." }`, y el token va en `CLOUDFLARE_API_TOKEN`. Solo se han ejecutado con proveedores simulados.

- **Tu plan decide el origen.** Cloud Run enruta por `Host`, y Cloudflare ofrece la sustitución de la cabecera Host solo en el plan Enterprise. En cualquier otro plan el origen debe aceptar tu host por sí mismo, así que el módulo crea una asignación de dominio de cada host, con las contrapartidas de la tabla de arriba, y Cloudflare se conecta a ella. No está verificado si su certificado se emite y se renueva detrás del proxy, así que empieza con `cloudflare.proxied = false`. En el plan Enterprise, `cloudflare.host_header_override = true` se conecta a la URL `run.app` en su lugar, sin asignación de dominio.
- **Una cabecera cierra el origen.** El módulo añade una regla de transformación de cabecera de solicitud, y el servidor rechaza con un `403` una solicitud sin la cabecera, salvo las comprobaciones de salud y los `POST` propios de los jobs, que comprueban sus tokens. Así nadie rodea a Cloudflare por la URL `run.app`, y un Cloudflare Worker de un tercero no puede elegir la dirección de cliente que ven los límites de tasa.
- **El valor acaba en el estado.** El módulo lo genera y lo escribe en el secreto `my-origin-secret`, pero el proveedor de Cloudflare no tiene forma de solo escritura para el valor de la cabecera de una regla, así que mantén el estado privado. [La guía de la CDN](cdn.md#rotate-the-value) explica cómo funciona la rotación.
- **Nada añade la cabecera mientras un registro no pasa por el proxy**, así que el servidor rechaza todo salvo sus comprobaciones de salud: con `proxied = false` para el certificado de una asignación de dominio, activa el proxy cuando el certificado esté activo.
- **Desactiva Bot Fight Mode** en el panel. Cubre toda la zona y ninguna regla puede omitirlo, y un desafío en `/ap/`, `/.well-known/` o `/api/` rompe la federación, porque otros servidores no son navegadores. Los jobs no pasan por Cloudflare. El módulo no añade ningún desafío y desactiva Browser Integrity Check, pero no gestiona Bot Fight Mode. No actives «I'm Under Attack» por la misma razón.
- **Email Address Obfuscation puede seguir activado.** Reescribe las direcciones de correo del HTML como `[email protected]` y añade un script para restaurarlas, que la Content-Security-Policy del servidor rechaza. Las propias páginas del servidor, como la que dice adónde fue el correo de confirmación, marcan su contenido para que deje las direcciones en paz.
- **Activa Always Use HTTPS** en el panel si quieres que `http://` se redirija. Cubre toda la zona, así que el módulo te lo deja a ti.
- **Timeouts**: Cloudflare responde `524` cuando el origen no envía nada durante 125 segundos. Los jobs no se ven afectados, ya que llaman directamente a la URL `run.app`. Los streams envían un latido cada 30 segundos, por debajo del límite de inactividad de 900 segundos. No está verificado si almacena en búfer los streams en un host normal con proxy.
- **Subidas**: 100 MB en Free y Pro, por encima de los 32 MiB de Cloud Run, que sigue siendo el límite.
- **Video**: las condiciones de Cloudflare restringen servir video y una cantidad desproporcionada de archivos grandes en Free, Pro y Business, y puede redirigir el contenido o tomar otras medidas. Decide esto antes de almacenar video en caché en el host de multimedia.

**Un archivo eliminado permanece en la caché.** El host de multimedia sigue el `Cache-Control` del servidor, así que un archivo local eliminado puede seguir visible hasta un año, y uno remoto durante `remote_media_cache_ttl_seconds`. El servidor no lo purga. Cuando algo deba desaparecer, purga a mano, en **Caching > Configuration > Custom Purge** del panel, o:

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

La URL de un proxy lleva su `sig` en la cadena de consulta, que forma parte de lo que se almacena en caché, así que da la URL completa.

<!-- source: changing-the-kek-backend b1e8cbb9ccfb -->
## Cambiar el backend de la KEK

Cambiar `kek.kind` es una línea en `main.tf`, pero en una instancia con datos no sale gratis. La base de datos contiene secretos envueltos con la clave actual, así que una instancia que la pierde ya no puede leerlos, y las claves de todas las cuentas están entre ellos.

Sigue [Moverse entre la KEK local y un KMS](key-management.md#move-between-the-local-kek-and-a-kms). Del secreto a Cloud KMS, conserva la KEK actual como `kek_previous` mientras los secretos se trasladan: añade `secrets = ["kek_previous"]` y rellena el secreto `my-kek-previous` con la KEK actual. Ejecuta `my kek rewrap` mediante [la consola](#the-console) y quítalo cuando `my kek status` ya no cuente claves `local:`. Conserva la KEK mientras conserves copias de seguridad de la base de datos anteriores al rewrap.

En una instancia nueva sin datos, no hace falta mover nada.

<!-- source: limits 3ff113a57279 -->
## Límites

- **Subidas**: Cloud Run rechaza una solicitud HTTP/1 de más de 32 MiB, y el servidor habla HTTP/1.1. Por eso el módulo define `MY_MEDIA_MAX_AV_BYTES` en 30 MiB, frente al valor predeterminado de 100 MiB, para que una subida mayor reciba la respuesta del propio servidor.
- **Streams**: el timeout de solicitud, 15 minutos por defecto, termina un stream abierto, y el cliente se conecta de nuevo.
- **Direcciones de cliente**: los límites de tasa dependen de `MY_TRUSTED_PROXIES`, que el módulo define a partir del ingress. Ningún documento que hayamos leído dice qué añade Cloud Run a `X-Forwarded-For` para una solicitud directa ni qué dirección de par ve un contenedor, y el contrato del contenedor no dice nada de ninguna de las dos, así que `169.254.0.0/16` es una suposición. La forma documentada del balanceador de carga externo es `[<supplied>,]<client-ip>,<lb-ip>`. Comprueba las direcciones en los primeros registros de solicitudes: una IP de cliente que sea una dirección `169.254` o una dirección de Google significa que el ajuste es incorrecto para tu proyecto.
- **Acceso público**: el servicio concede `roles/run.invoker` a `allUsers`, lo que rechaza una política de organización de uso compartido restringido por dominio. Un proyecto así necesita una excepción.

<!-- source: costs b8be3997d723 -->
## Costes

Un servidor cuesta dinero. Estas cifras son una guía de lo que cabe esperar y de dónde se reduce con cuidado, no límites a los que esté sujeto el despliegue. Son precios de lista aproximados para una región, de 2026-10 y 2026-11. Compruébalos en las páginas de precios de [Cloud Run](https://cloud.google.com/run/pricing) y [Neon](https://neon.com/pricing). El cómputo es gratuito por cuenta de facturación hasta 180 000 vCPU-segundos y 360 000 GiB-segundos al mes.

| Parte | Inactivo | Qué lo aumenta |
| --- | --- | --- |
| Balanceador de carga | Unos 18 $ al mes, sin confirmar en la página de precios | Un cargo por GB; consulta [Cómo llega el tráfico al servicio](#how-traffic-reaches-the-service) |
| Neon, en el plan Launch | Unos 19 $ al mes, con el tick predeterminado | Almacenamiento a 0,35 $ por GB-mes. `database.kind = cloud-sql` en su lugar tiene un coste fijo de unos 9,40 $ al mes, para una máquina más pequeña |
| Cloud Run | 0 $: las instancias mínimas son 0 y la CPU se factura solo durante las solicitudes | Cada stream abierto mantiene una instancia facturada; ve abajo |
| Cloud Tasks | 0 $ | 1 millón de operaciones gratis, luego 0,40 $ por millón. Un job usa unas 2 |
| Cloud Scheduler | 0 $: 3 jobs gratis por cuenta de facturación | 0,10 $ por job por encima de eso |
| Cloud Storage | Unos 0,02 $ por GiB al mes | 5 GiB son gratis solo en `us-east1`, `us-west1` y `us-central1`. La multimedia sale por Cloud Run |
| Pub/Sub | 0 $: 10 GiB gratis | Solo mientras hay streams abiertos |
| Secret Manager | Céntimos | Unas pocas versiones gratis, luego unos céntimos por versión |

**El tick mantiene despierto a Neon.** Cada tick lee y escribe en la base de datos, y Neon detiene un cómputo solo tras 5 minutos sin actividad, así que con el `* * * * *` predeterminado el cómputo nunca se detiene. Son 730 horas al mes, o 182,5 CU-horas a 0,25 CU, que son los 19 $ de arriba. El tick se mantiene cada minuto para que los jobs periódicos se ejecuten a tiempo, y una señal de despertar perdida se recupera pronto.

**Un stream abierto mantiene una instancia facturada.** Con la facturación por solicitud, una instancia se factura mientras atiende cualquier solicitud, y las actualizaciones en tiempo real son una solicitud larga. Un cliente que mantiene su stream abierto todo el mes cuesta una instancia las 24 horas: unos 60 $ al mes con 1 vCPU y 512 MiB, tras la franquicia gratuita. Más streams comparten la instancia, hasta `concurrency`. Si los streams permanecen abiertos la mayor parte del tiempo, compara con la facturación por instancia, o con una VM pequeña y [Docker Compose](deploy-docker-compose.md).

<!-- source: where-care-reduces-it 28bfc2e5ffcc -->
### Dónde se reduce con cuidado

- **La asignación de dominio** en lugar del balanceador de carga quita los 18 $, con [las contrapartidas de arriba](#how-traffic-reaches-the-service). Sirve para probar un despliegue.
- **El plan Free de Neon** quita los 19 $, y una instancia pequeña puede funcionar con él, con estas contrapartidas:
  - Permite 100 CU-horas al mes, que el tick predeterminado agota en unos 17 días. Mantente dentro con un calendario más largo, como `queue = { tick_schedule = "*/10 * * * *" }`: como máximo la mitad del tiempo despierto, unas 91 CU-horas, antes de que cualquier otra cosa lo despierte. Los jobs periódicos se ejecutan entonces hasta 10 minutos tarde, y una señal de despertar perdida se recupera más tarde.
  - Un servidor federado recibe entregas las 24 horas, lo que también despierta la base de datos, así que una instancia ocupada lo supera sea cual sea el calendario.
  - Contiene 1 GB por proyecto y un historial de 6 horas desde el que restaurar, y su cómputo siempre se detiene tras 5 minutos inactivo, así que la primera solicitud después de eso lo espera.
