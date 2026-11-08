<!-- source: deploying-on-cloud-run 9d2acbfd443c -->
# Despliegue en Cloud Run

Cómo ejecutar `my` en Google Cloud con los módulos de OpenTofu de [`deploy/opentofu`](../../deploy/opentofu/README.md): Cloud Run, Cloud Tasks, Cloud Scheduler, Cloud Storage y Pub/Sub, con una base de datos PostgreSQL en Neon, detrás de un Application Load Balancer externo global. El servicio no ejecuta ninguna instancia mientras nadie lo usa, salvo un tick que despierta al servidor cada minuto. Un servidor cuesta dinero: cuenta con unos 37 $ al mes antes del almacenamiento, el tráfico y las transmisiones abiertas, y lee [Costos](#costs) para saber qué puede reducir el cuidado.

Los módulos necesitan OpenTofu 1.11 o posterior. Terraform no es compatible.

<!-- source: what-it-creates c95687df197f -->
## Qué crea

| Parte | Módulo | Recursos |
| --- | --- | --- |
| Servidor | `app-gcp-cloud-run` | Un servicio de Cloud Run con `role = all`, un trabajo de migración, un trabajo de consola mientras lo necesites, una cuenta de servicio y acceso público |
| Tráfico | `ingress-gcp-load-balancer`, o `ingress-gcp-domain-mapping` o `ingress-cloudflare-app`, y opcionalmente `ingress-cloudflare-media` | Un Application Load Balancer externo global con un certificado, o una asignación de dominio, o Cloudflare, y un host de multimedia |
| Trabajos | `queue-gcp-cloud-tasks` | Una cola de Cloud Tasks, un trabajo de Cloud Scheduler y una cuenta de servicio que llama al servidor |
| Multimedia | `blob-gcp-gcs` | Un bucket privado |
| Actualizaciones en tiempo real | `stream-gcp-pubsub` | Un tema y un rol personalizado. El servidor crea y elimina sus propias suscripciones |
| Base de datos | `db-external` | Nada: creas la base de datos a mano |
| Correo electrónico | `mail-smtp` | Nada: cualquier servicio SMTP |
| Secretos | `secrets-gcp-secret-manager` | Secretos de la URL de la base de datos y de la contraseña SMTP, que rellenas tú, y una clave VAPID generada |
| KEK | `kek-gcp-secret-manager` o `kek-gcp-kms` | Un secreto generado montado como archivo, o una clave de Cloud KMS |

Los valores de los secretos nunca llegan al plan ni al estado de OpenTofu: añades tú mismo los que vienen de otro lugar, y el despliegue genera la KEK y la clave VAPID mediante atributos de solo escritura.

<!-- source: before-you-start 4c624a6b7316 -->
## Antes de empezar

- `tofu` 1.11 o posterior, `gcloud` y `jq`.
- Un proyecto de Google Cloud con facturación, usado solo para esta instancia, y una cuenta en él con el rol Owner.
- Un dominio para [`host`](../reference/configuration.md#host). No puede cambiarse después: lee [Configuración que no puedes cambiar después](production-checklist.md#settings-you-cannot-change-later).
- Una versión de `my` de las [versiones publicadas](running.md#get-the-image). No existe la etiqueta `latest`.

<!-- source: first-deployment 996fcc6eb35e -->
## Primer despliegue

<!-- source: 1-enable-the-apis 2d9c831fb77d -->
### 1. Activar las API

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com compute.googleapis.com
```

`compute.googleapis.com` es para el balanceador de carga.

<!-- source: 2-create-the-database-on-neon 139308cfc452 -->
### 2. Crear la base de datos en Neon

Cuenta con el plan Launch: con el tick predeterminado, el cómputo nunca se detiene. El plan Free también sirve, con las contrapartidas indicadas en [Costos](#costs).

Hay dos formas, y el ejemplo usa la primera:

- **A mano, con `db-external`.** En la consola de Neon, crea un proyecto en una región cercana a tu región de Cloud Run y copia sus cadenas de conexión. La contraseña queda fuera del estado de OpenTofu.
- **Con [`db-neon`](../../deploy/opentofu/modules/db-neon/README.md)**, que crea el proyecto y escribe las URL en los secretos por ti. **La contraseña de la base de datos acaba entonces en el estado**, y el proveedor no tiene mantenimiento: su README explica lo que eso significa, y un comentario en el bloque `db` de `main.tf` muestra el cambio.

Un proyecto tiene dos cadenas de conexión, y el despliegue usa cada una donde corresponde:

- La **directa** (el host sin `-pooler`) es obligatoria. El trabajo de migración y la consola la usan siempre: `my schema apply` mantiene un bloqueo a nivel de sesión, que un pooler en modo transacción no conserva.
- La **agrupada** (el host con `-pooler`) es opcional, para el servicio. El pooler de Neon se encarga de lo que necesita el servidor: consulta [A través de un pooler de conexiones](database.md#through-a-connection-pooler). Pásala a `pooled_url_secret_id` del módulo `db`, en un secreto propio, añadido a `referenced` del módulo `secrets`.

Sin una cadena agrupada, el servicio usa también la directa, hasta el límite de conexiones de la base de datos, que `max_instances` del módulo app acota. Para subir el grupo de su valor predeterminado de 4 por instancia, para las 80 solicitudes que una instancia de Cloud Run atiende a la vez, añade `pool_max_conns` a la cadena que usa el servicio, por ejemplo `postgres://...-pooler...neon.tech/neondb?sslmode=require&pool_max_conns=8`. Consulta [Esperar una conexión a la base de datos](running.md#waiting-for-a-database-connection).

<!-- source: 3-copy-the-example-and-set-the-variables 3abe6b730543 -->
### 3. Copiar el ejemplo y definir las variables

Parte de una copia en un directorio vacío, con el archivo de bloqueo de proveedores, de la versión que despliegas:

```sh
VERSION=v0.1.0
git clone --depth 1 --branch "$VERSION" https://github.com/mycroblog/my.git my-source
mkdir my-deployment
cp -r my-source/deploy/opentofu/examples/gcp-cloud-run-neon/. my-deployment/
rm -rf my-source
cd my-deployment
cp terraform.tfvars.example terraform.tfvars
```

**Define `ref` con esa versión.** Cada bloque de módulo de `main.tf` nombra una versión con `?ref=`, que la copia de una versión estable ya tiene correcta. Para una versión preliminar, defínelo tú: `sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf`. Los módulos existen solo desde la primera versión que los contiene, así que una etiqueta anterior no tiene ningún ejemplo que copiar. Mantén la misma versión en `ref` y en `image`, y conserva este directorio: contiene tu estado y tu configuración.

Rellena `terraform.tfvars`. `mail_from`, `mail_smtp_host`, `mail_smtp_port` (587 de forma predeterminada; Google Cloud bloquea el puerto saliente 25) y `mail_smtp_username` configuran el [correo electrónico](email.md). `env` y `secret_env` aceptan cualquier otra configuración de la [referencia de configuración](../reference/configuration.md), por valor y por ID de secreto. Un configuración nombrado en dos lugares hace fallar el plan.

Para no enviar correo electrónico, quita el módulo `mail` y su secreto de `main.tf`, y `mail` se queda en `none`. Lee antes [lo que eso significa para los registros](production-checklist.md#email).

Decide ahora sobre [`account_domain`](../reference/configuration.md#account_domain), como `MY_ACCOUNT_DOMAIN` en `env`, si los identificadores usan otro dominio.

**El estado** permanece en este directorio de forma predeterminada. No contiene valores secretos, pero es el único registro de lo que se creó: guárdalo en un bucket con un bloque `backend "gcs"` si lo ejecuta más de una persona.

<!-- source: 4-create-the-secrets-and-add-their-values f94fd484e28f -->
### 4. Crear los secretos y añadir sus valores

Esto crea los secretos y genera la KEK y la clave VAPID:

```sh
tofu init
tofu apply -target=module.secrets -target=module.kek
```

`tofu init` también obtiene `upgrade.sh` y `console.sh` de esa versión, que usan los pasos siguientes:

```sh
SCRIPTS=.terraform/modules/app/deploy/opentofu
```

**Copia la KEK ahora**, y guarda la copia aparte de la base de datos, por ejemplo en un gestor de contraseñas. Sin ella no se puede restaurar una [copia de seguridad](backup.md) y se pierden las claves de las cuentas. Haz lo mismo con la clave VAPID, cuya pérdida acaba con todas las suscripciones de Web Push:

```sh
gcloud secrets versions access latest --secret=my-kek --project="$PROJECT"
gcloud secrets versions access latest --secret=my-vapid-key --project="$PROJECT"
```

Después añade los valores que vienen de otro lugar. Una instancia cuyo secreto no se puede leer no se inicia, así que añádelos antes que la aplicación:

```sh
printf '%s' 'postgres://...' | gcloud secrets versions add my-database-url --project="$PROJECT" --data-file=- # the direct string
printf '%s' '<the SMTP password>' | gcloud secrets versions add my-smtp-password --project="$PROJECT" --data-file=-
```

La KEK llega al servidor como un archivo montado, nombrado por `MY_KEK_FILE`. Los demás son variables de entorno que se resuelven cuando se inicia una instancia, así que una versión nueva surte efecto en la siguiente instancia: despliega una revisión nueva para asegurarte.

**OpenTofu genera la KEK una sola vez y nunca más.** El secreto y su versión tienen `prevent_destroy`, y el número de versión que escribiría un valor nuevo está fijado en el módulo: no lo cambies, porque una KEK aleatoria nueva hace ilegible todo lo que cifró la anterior. Para rotarla, sigue [Rotar la KEK](key-management.md#rotate-the-kek), añadiendo el valor nuevo con `gcloud secrets versions add`. OpenTofu no registra lo que añades ahí.

Para mantener la KEK en Cloud KMS desde el principio, sustituye el bloque `kek` de `main.tf` como indica su comentario y activa `cloudkms.googleapis.com`. Para traer una KEK o una clave VAPID propias, define `create = false` en el módulo `kek`, o quita la clave de `generate_p256_keys`, antes de este paso. Un secreto creado en otro lugar solo necesita su ID: pásalo a `secret_env` del módulo app, o al módulo que lo usa.

<!-- source: 5-create-what-the-migration-needs eab1bb33867c -->
### 5. Crear lo que necesita la migración

La sonda de inicio del servicio espera a `/readyz`, que responde `503` hasta que existe el esquema, así que el esquema tiene que ir primero. Crea el trabajo de migración y aquello de lo que depende, que incluye la dirección del balanceador de carga, ya que la configuración del servidor la nombra:

```sh
tofu apply -target='module.app.google_cloud_run_v2_job.one_off["migrate"]'
```

<!-- source: 6-apply-the-schema 2946ca18d263 -->
### 6. Aplicar el esquema

```sh
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.1.0 --migrate-only
```

Ejecuta `my config check`, `my db upgrade -check`, `my schema apply` y `my db upgrade`, cada uno como una ejecución del trabajo, y se detiene en el primero que falla. El primer comando que usa la base de datos registra en ella `host` y `account_domain`.

<!-- source: 7-create-the-rest 368abac4aa08 -->
### 7. Crear el resto

```sh
tofu apply
```

<!-- source: 8-point-the-domain-at-the-load-balancer 7e54edc163b2 -->
### 8. Apuntar el dominio al balanceador de carga

`tofu output dns_records` muestra un registro `A`, de `host` a la dirección del balanceador de carga. Añádelo en tu proveedor de DNS. Si ese proveedor tiene un proxy propio, déjalo desactivado para este registro.

El certificado lo gestiona Google y se activa solo cuando el DNS apunta a la dirección, lo que puede tardar un tiempo. Hasta entonces `https://{host}` falla con un error de TLS, de modo que la federación y los navegadores aún no pueden llegar al servidor. Nada más espera por ello: Cloud Tasks y Cloud Scheduler llaman a la URL `run.app` predeterminada del servicio, `https://<service>-<project number>.<region>.run.app`. **No desactives la URL predeterminada**, o los trabajos se detienen. Esa URL rechaza las solicitudes de internet, y Cloud Run considera estas dos como internas solo desde el mismo proyecto y en esa URL, no en un dominio personalizado.

Consulta [Cómo llega el tráfico al servicio](#how-traffic-reaches-the-service) para saber qué más puedes elegir.

<!-- source: 9-create-the-first-administrator c5b03212fb36 -->
### 9. Crear el primer administrador

Los trabajos de Cloud Run no tienen terminal ni entrada estándar, así que `my admin create` imprime un enlace que establece la contraseña en lugar de leer una. Activa la [consola](#the-console) y ejecuta:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

Abre el enlace de inmediato y establece la contraseña. Funciona una sola vez, durante una hora. **Permanece en Cloud Logging hasta entonces**, donde cualquiera que pueda leer los registros del proyecto puede usarlo. Si caduca, ejecuta `my admin account password-reset-link @alice` mediante la consola. Después activa la autenticación en dos pasos.

<!-- source: 10-go-through-the-checklist 90add86e4288 -->
### 10. Repasar la lista de comprobación

Repasa la [lista de comprobación de producción](production-checklist.md), empezando por el [correo electrónico](production-checklist.md#email): SPF, DKIM y DMARC en el dominio de `mail_from`.

<!-- source: the-console 5aebf704c6d7 -->
## La consola

La consola ejecuta un comando de `my` con la imagen, la configuración y los secretos del servicio. Existe solo mientras `console_enabled` está activado, así que desactívalo cuando termines:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` espera al comando, imprime su salida y su código de salida, y termina con ese código. Necesita `roles/logging.viewer`, porque la salida de un trabajo va a Cloud Logging. Usa la imagen que ejecuta el servicio, diga lo que diga `image`.

El comando no tiene entrada estándar ni terminal, así que un comando que lee de ellos una contraseña o un archivo no puede ejecutarse de este modo. Lo que imprime queda en los registros, así que no ejecutes un comando cuya salida no escribirías allí.

<!-- source: upgrading 1a1e204c5173 -->
## Actualización

Lee antes [Actualización](database.md#upgrading) y las notas de la versión, y avanza una versión menor cada vez.

```sh
# 1. Back up the database while nothing writes. Use pg_dump or a Neon branch (see Backup and restore).
gcloud run services update my --project="$PROJECT" --region=asia-northeast1 --update-env-vars=MY_MAINTENANCE=true
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Read the SQL it would run. This changes nothing but the image of the migration job.
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.2.0 --plan

# 3. Upgrade. These scripts are of the release your modules name, which is enough: they only run gcloud and my.
"$SCRIPTS/upgrade.sh" --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.2.0 --maintenance
```

El script hace lo siguiente y se detiene en el primer paso que falla:

1. Pone el servicio en el [modo de mantenimiento](maintenance.md) con `--maintenance`.
2. Da al trabajo de migración la imagen nueva.
3. Ejecuta `my config check`, `my db upgrade -check`, `my schema apply` y `my db upgrade`, cada uno esperando al anterior.
4. Despliega la revisión nueva, sin el modo de mantenimiento. Recibe tráfico solo cuando su sonda de inicio pasa en `/readyz`.

Sin `--maintenance`, la revisión antigua sigue sirviendo durante los pasos 2 y 3, lo que [no está garantizado que funcione](database.md#the-server-during-an-upgrade). Añade `--allow-destructive` cuando las notas de la versión lo pidan. Fija un digest con `--image ghcr.io/mycroblog/my:0.2.0@sha256:...`, lo que recomienda la [lista de comprobación de producción](production-checklist.md#operations).

Después define `image` en `terraform.tfvars` con el mismo valor, y `ref` en `main.tf` con la misma versión, seguido de `tofu init` y `tofu plan`. Lee ese plan antes de aplicarlo: los módulos de una versión nueva pueden cambiar recursos. `tofu apply` nunca cambia la imagen de un servicio o trabajo que existe, así que un `image` olvidado no hace daño, pero ese valor es el punto de partida de un trabajo nuevo o de un servicio recreado.

**Cuando un paso falla**, el servicio se queda en su revisión, en el modo de mantenimiento si lo pediste. Corrige la causa y ejecuta el script de nuevo: los pasos se pueden repetir sin riesgo. Para salir del modo de mantenimiento sin actualizar, cuando `my schema apply` no se ha ejecutado:

```sh
gcloud run services update my --project="$PROJECT" --region=asia-northeast1 --remove-env-vars=MY_MAINTENANCE
```

Para volver atrás después de que el esquema cambió, restaura la copia de seguridad: consulta [Volver a una versión anterior](database.md#rolling-back).

<!-- source: monitoring-and-alerts 67bc34d23fdf -->
## Monitorización y alertas

El módulo [`monitoring-gcp`](../../deploy/opentofu/modules/monitoring-gcp/README.md) añade una comprobación de disponibilidad de `/readyz`, políticas de alerta con los nombres de [la guía de actuación](alerts.md) y notificaciones por correo electrónico. Nunca se ha aplicado, así que pruébalo una vez haciendo fallar una comprobación. Activa antes `monitoring.googleapis.com` y lee en su README lo que cuesta la comprobación: cada ejecución despierta una instancia y, a través de `/readyz`, la base de datos, lo que se suma a [la contrapartida del tick](#costs).

<!-- source: changing-a-backend ef4660f60191 -->
## Cambiar un backend

Sustituye el bloque del módulo en `main.tf`. El módulo app toma lo que devuelva el bloque, así que nada más cambia. Cambiar la base de datos también significa mover sus datos, ya que una nueva está vacía: [`db-gcp-cloud-sql`](../../deploy/opentofu/modules/db-gcp-cloud-sql/README.md#coming-from-db-external) tiene los pasos. Por ejemplo, una base de datos propia:

```hcl
module "db" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/db-external?ref=v0.1.0&depth=1"

  url_secret_id = "another-database-url"
}
```

Para un almacén fuera de Google Cloud, sustituye el bloque `blob` por [`blob-s3`](../../deploy/opentofu/modules/blob-s3/README.md), para cualquier servicio compatible con S3, o por [`blob-cloudflare-r2`](../../deploy/opentofu/modules/blob-cloudflare-r2/README.md).

<!-- source: when-you-need-a-network fb392704a1e7 -->
## Cuándo necesitas una red

Esta guía no necesita ninguna: Neon, Cloud Tasks, Cloud Storage y Pub/Sub se alcanzan todos por internet con la propia identidad del servicio, así que Cloud Run no se conecta a nada y no hay nada que pagar ni que mantener.

Necesitas una para poner Memorystore en el despliegue, ya que solo tiene direcciones privadas, o para dar a Cloud SQL una dirección privada en lugar de la pública. Añade [`network-gcp-vpc`](../../deploy/opentofu/modules/network-gcp-vpc/README.md), o describe una VPC que ya tengas como un objeto de [la misma forma](../../deploy/opentofu/README.md#the-network-on-google-cloud), y pasa la misma a la aplicación y a cada módulo que la toma:

```hcl
module "network" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/network-gcp-vpc?ref=v0.1.0&depth=1"

  project_id = var.project_id
  region     = var.region
}

module "kv" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/kv-gcp-memorystore?ref=v0.1.0&depth=1"

  project_id = var.project_id
  region     = var.region
  network    = module.network
}
```

Da también a la aplicación `network = module.network` y añade `module.kv` a sus `backends`. Solo los rangos privados pasan por la red, así que el servicio llega a internet como antes, sin Cloud NAT. Un backend en direcciones privadas con una aplicación en otra red, o en ninguna, hace fallar el plan. Activa antes las API de Service Networking y Network Connectivity, y la de Compute Engine. Una red añade tiempo al arranque en frío de una instancia: lee [lo que añade la red privada](../../deploy/opentofu/modules/kv-gcp-memorystore/README.md#what-the-private-network-adds).

<!-- source: splitting-jobs-into-workers ed661aede0a3 -->
## Dividir los trabajos en workers

Cuando un tipo de trabajo retrasa al resto, dale su propio servicio. Decide qué dividir a partir de las métricas, como indica [Dividir los workers por tipo de trabajo](running.md#splitting-workers-by-job-type). Un solo valor va a la cola y a la aplicación, y cada cola, URL, ruta y configuración sale de sus claves, así que no escribes ninguno:

```hcl
locals {
  workers = {
    delivery = {
      job_types     = ["federation.fanout", "federation.deliver"]
      max_instances = 20
      concurrency   = 16
    }
  }
}
```

Define `workers = local.workers` en el bloque `queue` y en el bloque `app`. El plan rechaza una división de los tipos que deben permanecer juntos, y una cola y una aplicación con valores distintos. También necesita un broker que llegue a todos los procesos, como lo es `stream-gcp-pubsub`. El mismo valor va a `monitoring-gcp`, si lo usas.

Los dos diseños habituales de esa página son así. Para evitar que las entregas retrasen al resto, usa el valor de arriba. Para mantener ágiles el correo electrónico y las notificaciones:

```hcl
locals {
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
}
```

Omite los tipos cuya configuración no hayas definido, como dice esa página: un tipo que el servidor no ha registrado impide el inicio, y el plan no puede ver cuáles son. Con `workers`, el servicio predeterminado no ayuda con estos tipos cuando está libre, ya que sus tareas van a su propia cola.

Cada campo, y lo que un worker tiene de la aplicación, está en [el contrato](../../deploy/opentofu/README.md#workers). El servicio de un worker no recibe tráfico del exterior. `upgrade.sh` mueve cada uno después de la migración y antes del servicio predeterminado, y sin nada que se lo indique: solo lo detiene un paso que falla. Cada worker puede traer lo que cuesta el servicio, y las conexiones de sus instancias cuentan para el límite de la base de datos.

<!-- source: how-traffic-reaches-the-service d1e6d0ed195b -->
## Cómo llega el tráfico al servicio

El bloque `ingress` de `main.tf` lo decide. El ejemplo usa el balanceador de carga, pensado para producción. La asignación de dominio está en un comentario junto a él, para probar un despliegue de forma económica: para cambiarla, modifica ese bloque y `service_ingress` del bloque `app`, y el plan avisa si los dos discrepan.

| | `ingress-gcp-load-balancer` (el ejemplo) | `ingress-gcp-domain-mapping` |
| --- | --- | --- |
| Cuota mensual | Unos 18 $ por las reglas de reenvío, más un cargo por GB. Comprueba la [página de precios](https://cloud.google.com/vpc/network-pricing): no se pudo confirmar esta cifra allí | Ninguna |
| Estado | La forma recomendada por Google Cloud | Una función en Preview que Google Cloud no recomienda para producción, por la latencia |
| Regiones | Cualquiera | `asia-northeast1`, `us-central1` y algunas más |
| DNS | Un registro `A`, de `tofu output dns_records` | Los registros que lista la misma salida |
| Certificado | Gestionado por Google, activo cuando el DNS apunta a la dirección | Gestionado por Google, hasta 24 horas |
| Propiedad del dominio | Nada más allá del DNS | Tú, y toda otra cuenta que lo solicite, debéis ser [propietarios verificados](https://cloud.google.com/run/docs/mapping-custom-domains) antes de aplicar |
| URL `run.app` | Rechazada desde internet, así que nadie rodea el balanceador de carga | Accesible para cualquiera: una asignación de dominio no puede añadir una cabecera, así que el origen queda abierto |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` y la dirección del balanceador de carga | `169.254.0.0/16` |
| `service_ingress` del bloque `app` | `internal-and-load-balancer`. Una discrepancia con el bloque `ingress` hace fallar el plan | `all` |

Cloud Tasks y Cloud Scheduler no llegan por ninguno de los dos: llaman directamente a la URL `run.app` predeterminada.

Para usar la URL propia del servicio u otra cosa delante, como una CDN, quita el bloque `ingress` y su línea en `backends`, y deja `service_ingress` en lo que necesites: es siempre una elección explícita tuya, y `all` deja la URL `run.app` abierta a cualquiera. Define `MY_TRUSTED_PROXIES` en `env` con lo que se conecta al servidor. Lo que pongas delante debe seguir respondiendo en `https://{host}`.

<!-- source: cloudflare 95b7a9baaff1 -->
### Cloudflare

`ingress-cloudflare-app` pone Cloudflare delante de `host`, con la caché omitida para todo, e `ingress-cloudflare-media` añade un host de multimedia en caché junto a cualquier ingress. Lee antes el README de cada módulo ([app](../../deploy/opentofu/modules/ingress-cloudflare-app/README.md), [media](../../deploy/opentofu/modules/ingress-cloudflare-media/README.md)). El token va en `CLOUDFLARE_API_TOKEN`. Estos módulos solo se han ejecutado con proveedores simulados.

- **Tu plan decide el origen.** Cloud Run enruta por `Host`, y Cloudflare ofrece la sustitución de la cabecera Host solo en el plan Enterprise. En cualquier otro plan un origen `*.run.app` hace fallar el plan, así que el origen debe aceptar tu host por sí mismo: conserva un `ingress-gcp-domain-mapping` de ese host, fuera de `backends`, y define `origin_host = "ghs.googlehosted.com"`. Cada host necesita su propia asignación. No está verificado si su certificado se emite y se renueva detrás del proxy, así que empieza con `proxied = false`.
- **Una cabecera cierra el origen.** `ingress-cloudflare-app` añade una regla de transformación de cabecera de solicitud, y el servidor rechaza con un `403` una solicitud sin la cabecera, salvo las comprobaciones de estado y los `POST` propios de los trabajos, que comprueban sus tokens. Así nadie rodea Cloudflare a través de la URL `run.app`, y un Cloudflare Worker de quien llama no puede elegir la dirección de cliente que ven los límites de frecuencia.
- **El valor acaba en el estado.** El proveedor de Cloudflare no tiene una forma de solo escritura para el valor de la cabecera de una regla. Genera uno con `openssl rand -hex 32`, añádelo a un secreto con `gcloud secrets versions add`, pasa el mismo valor con `TF_VAR_origin_secret` y mantén el estado en privado. [La guía de la CDN](cdn.md#rotate-the-value) explica cómo rotarlo.
- **Nada añade la cabecera mientras un registro no pasa por el proxy**, así que el servidor rechaza todo salvo sus comprobaciones de estado: con `proxied = false` para el certificado de una asignación de dominio, configura el secreto después de eso.
- **Desactiva Bot Fight Mode** en el panel. Cubre toda la zona y ninguna regla puede omitirlo, y un desafío en `/ap/`, `/.well-known/` o `/api/` rompe la federación, porque otros servidores no son navegadores. Los trabajos no pasan por Cloudflare. Los módulos no añaden ningún desafío y desactivan Browser Integrity Check, pero no gestionan Bot Fight Mode. No actives «I'm Under Attack» por la misma razón.
- **Activa Always Use HTTPS** en el panel si quieres que `http://` se redirija. Cubre toda la zona, así que los módulos te lo dejan a ti.
- **Tiempos de espera**: Cloudflare responde `524` cuando el origen no envía nada durante 125 segundos. Los trabajos no se ven afectados, ya que llaman directamente a la URL `run.app`. Las transmisiones envían un latido cada 30 segundos, por debajo del límite de inactividad de 900 segundos. No está verificado si almacena en búfer las transmisiones en un host normal con proxy.
- **Subidas**: 100 MB en Free y Pro, por encima de los 32 MiB de Cloud Run, que sigue siendo el límite.
- **Video**: las condiciones de Cloudflare restringen servir video y una cantidad desproporcionada de archivos grandes en Free, Pro y Business, y puede redirigir el contenido o tomar otras medidas. Decide esto antes de poner video en caché en el host de multimedia.

**Un archivo eliminado sigue en caché.** El host de multimedia sigue el `Cache-Control` del servidor, así que un archivo local eliminado puede seguir visible hasta un año, y uno remoto durante `remote_media_cache_ttl_seconds`. El servidor no lo purga. Cuando algo deba desaparecer, purga a mano, en **Caching > Configuration > Custom Purge** en el panel, o:

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

Una URL de proxy lleva su `sig` en la cadena de consulta, que forma parte de lo que se guarda en caché, así que indica la URL completa.

<!-- source: changing-the-kek-backend c5e918ea5c81 -->
## Cambiar el backend de la KEK

Sustituir el bloque `kek` es un bloque en `main.tf`, pero en una instancia con datos no sale gratis. La base de datos contiene secretos envueltos con la clave actual, así que una instancia que la pierde ya no puede leerlos, y entre ellos están las claves de todas las cuentas.

Sigue [Pasar entre la KEK local y un KMS](key-management.md#move-between-the-local-kek-and-a-kms). Del secreto a Cloud KMS, conserva la KEK actual como `kek_previous` mientras los secretos se mueven, pasando su secreto a través del módulo app, con `secret_files = { MY_KEK_PREVIOUS_FILE = "my-kek" }`; ejecuta `my kek rewrap` mediante [la consola](#the-console) y quítala cuando `my kek status` ya no cuente claves `local:`. Conserva la KEK mientras conserves copias de seguridad de la base de datos anteriores al rewrap.

En una instancia nueva sin datos, no hace falta mover nada.

<!-- source: limits 5bd276dd3a89 -->
## Límites

- **Subidas**: Cloud Run rechaza una solicitud HTTP/1 de más de 32 MiB, y el servidor habla HTTP/1.1. Por eso el ejemplo define `MY_MEDIA_MAX_AV_BYTES` en 30 MiB, frente al valor predeterminado de 100 MiB, para que una subida mayor reciba la respuesta del propio servidor.
- **Transmisiones**: el tiempo de espera de la solicitud, 15 minutos de forma predeterminada, termina una transmisión abierta, y el cliente vuelve a conectarse.
- **Direcciones de cliente**: los límites de frecuencia dependen de `MY_TRUSTED_PROXIES`, que el módulo app define a partir de lo que devuelve el módulo `ingress`. Ningún documento que hayamos leído dice qué añade Cloud Run a `X-Forwarded-For` en una solicitud directa ni qué dirección de par ve un contenedor, y el contrato del contenedor no dice nada de ninguna de las dos, así que `169.254.0.0/16` es una suposición. La forma documentada del balanceador de carga externo es `[<supplied>,]<client-ip>,<lb-ip>`. Comprueba las direcciones en los primeros registros de solicitudes: una IP de cliente que sea una dirección `169.254` o una dirección de Google significa que la configuración es incorrecto para tu proyecto.
- **Acceso público**: el servicio concede `roles/run.invoker` a `allUsers`, lo que rechaza una política de la organización sobre uso compartido restringido por dominio. Un proyecto así necesita una excepción.

<!-- source: costs 1ab8d07f6847 -->
## Costos

Un servidor cuesta dinero. Estas cifras son una guía de lo que cabe esperar y de dónde reduce el cuidado el costo, no límites a los que se atenga el despliegue. Son precios de lista aproximados para una región, de 2026-10 y 2026-11. Compruébalos en las páginas de precios de [Cloud Run](https://cloud.google.com/run/pricing) y [Neon](https://neon.com/pricing). El cómputo es gratuito por cuenta de facturación hasta 180 000 vCPU-segundos y 360 000 GiB-segundos al mes.

| Parte | En reposo | Qué lo aumenta |
| --- | --- | --- |
| Balanceador de carga | Unos 18 $ al mes, no confirmado en la página de precios | Un cargo por GB; consulta [Cómo llega el tráfico al servicio](#how-traffic-reaches-the-service) |
| Neon, en el plan Launch | Unos 19 $ al mes, con el tick predeterminado | Almacenamiento a 0,35 $ por GB-mes. `db-gcp-cloud-sql` tiene en cambio un costo fijo de unos 9,40 $ al mes, para una máquina más pequeña: su README tiene el desglose |
| Cloud Run | 0 $: min instances es 0 y la CPU se factura solo durante las solicitudes | Cada transmisión abierta mantiene una instancia facturada; véase más abajo |
| Cloud Tasks | 0 $ | 1 millón de operaciones gratis, después 0,40 $ por millón. Un trabajo usa unas 2 |
| Cloud Scheduler | 0 $: 3 trabajos gratis por cuenta de facturación | 0,10 $ por trabajo por encima de eso |
| Cloud Storage | Unos 0,02 $ por GiB al mes | 5 GiB son gratis solo en `us-east1`, `us-west1` y `us-central1`. La multimedia sale a través de Cloud Run |
| Pub/Sub | 0 $: 10 GiB gratis | Solo mientras hay transmisiones abiertas |
| Secret Manager | Céntimos | Unas pocas versiones gratis, después unos pocos céntimos por versión |

**El tick mantiene despierto a Neon.** Cada tick lee y escribe en la base de datos, y Neon detiene un cómputo solo tras 5 minutos sin actividad, así que con el `* * * * *` predeterminado el cómputo nunca se detiene. Son 730 horas al mes, o 182,5 CU-horas a 0,25 CU, que son los 19 $ de arriba. El tick se mantiene cada minuto para que los trabajos periódicos se ejecuten a tiempo, y una señal de despertar perdida se recupera pronto.

**Una transmisión abierta mantiene una instancia facturada.** Con la facturación basada en solicitudes, una instancia se factura mientras atiende cualquier solicitud, y las actualizaciones en tiempo real son una solicitud larga. Un cliente que mantiene su transmisión abierta todo el mes cuesta una instancia día y noche: unos 60 $ al mes con 1 vCPU y 512 MiB, tras la cuota gratuita. Más transmisiones comparten la instancia, hasta `concurrency`. Si las transmisiones permanecen abiertas la mayor parte del tiempo, compáralo con la facturación basada en instancias, o con una VM pequeña y [Docker Compose](deploy-docker-compose.md).

<!-- source: where-care-reduces-it 5711f378aadb -->
### Dónde reduce el costo el cuidado

- **La asignación de dominio** en lugar del balanceador de carga elimina los 18 $, con [las contrapartidas de arriba](#how-traffic-reaches-the-service). Conviene para probar un despliegue.
- **El plan Free de Neon** elimina los 19 $, y una instancia pequeña puede funcionar con él, con estas contrapartidas:
  - Permite 100 CU-horas al mes, que el tick predeterminado agota en unos 17 días. Mantente dentro con una programación más larga, como `tick_schedule = "*/10 * * * *"`: despierta como máximo la mitad del tiempo, unas 91 CU-horas, antes de que algo más lo despierte. Los trabajos periódicos se ejecutan entonces con hasta 10 minutos de retraso, y una señal de despertar perdida se recupera más tarde.
  - Un servidor federado recibe entregas día y noche, lo que también despierta la base de datos, así que una instancia con mucha actividad lo supera sea cual sea la programación.
  - Admite 1 GB por proyecto y un historial de 6 horas desde el que restaurar, y su cómputo siempre se detiene tras 5 minutos de inactividad, así que la primera solicitud después de eso lo espera.
