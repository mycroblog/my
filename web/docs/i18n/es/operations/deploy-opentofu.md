<!-- source: deploying-with-opentofu 178b85dbd9bd -->
# Desplegar con OpenTofu

Lo que comparte todo despliegue con los módulos raíz de OpenTofu: [Desplegar en Google Cloud](deploy-google-cloud.md) y
[Desplegar en AWS](deploy-aws.md) añaden lo que necesita su plataforma. Ambos módulos necesitan
[OpenTofu](https://opentofu.org) 1.11 o posterior. Terraform no está soportado.

<!-- source: the-root-modules 4e0bf528bd0e -->
## Los módulos raíz

Un bloque de un módulo raíz despliega todo el servidor: el servicio, sus jobs y un backend para cada rol.

| Módulo raíz | Plataforma | Ejemplo que copiar |
| --- | --- | --- |
| `mycroblog-gcp` | Cloud Run en Google Cloud | `examples/gcp-cloud-run-neon` |
| `mycroblog-aws` | ECS en Fargate, en AWS. No se ha aplicado nunca | `examples/aws-ecs-neon` |

Un ejemplo es el proveedor de la plataforma y un bloque, `module "mycroblog"`, cuya fuente es una fuente de Git fijada a una
versión:

```hcl
module "mycroblog" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/mycroblog-gcp?ref=v0.1.0&depth=1"

  host = "social.example.org"
  # the sections, below
}
```

La fijación es obligatoria: un módulo sin fijar podría cambiar bajo tus pies los recursos de la KEK. La imagen del servidor es por defecto la
versión que nombra `ref`, así que `ref` por sí solo nombra lo que se ejecuta, y subirlo es una actualización. El módulo existe solo desde la primera
versión que lo contiene.

<!-- source: sections 728259c0b297 -->
### Secciones

Cada rol es una sección del bloque, un objeto cuyo `kind` elige el backend, como `database = { kind = "external" }`.
Todos los campos tienen un valor predeterminado, la configuración recomendada, así que solo escribes lo que cambias. Una combinación que no puede funcionar falla
antes del plan, con un mensaje que nombra las entradas. [La referencia de los módulos](../../deploy/opentofu/README.md#sections)
enumera las secciones, los tipos y los campos de cada módulo.

<!-- source: other-settings-and-secrets 112fdabc5bb0 -->
### Otros ajustes y secretos

`settings` acepta cualquier otro ajuste de la [referencia de configuración](../reference/configuration.md) por su clave, y el
módulo lo pasa al servicio, a los workers y a los jobs. `secrets` acepta las claves de los ajustes secretos: para cada una, el
módulo crea un secreto vacío `<name>-<key>`, con guiones en lugar de guiones bajos, se lo pasa al servidor y lo enumera para que lo rellenes.

```hcl
  settings = { translator_deepl_url = "https://api.deepl.com" }
  secrets  = ["translator_deepl_api_key"]
```

Una clave que fija una sección o que fija el módulo hace fallar el plan, y también una clave que el módulo no acepta, como `listen`:
el error da el motivo. Una sección que es nula, como `translation`, y una señal de `observability` definida como `settings`
no fijan nada, así que `settings` puede nombrar sus claves. Informes de errores a Sentry, por ejemplo:

```hcl
  observability = { errors = "settings" }
  settings      = { errors_exporter = "sentry" }
  secrets       = ["errors_sentry_dsn"]
```

Los ajustes cambiados a mano en la plataforma no duran: el siguiente apply restablece lo que dice el bloque.

<!-- source: secrets d58c587dc144 -->
### Secretos

Ningún valor secreto que des llega al plan ni al estado de OpenTofu. El módulo genera lo que puede mediante atributos de solo
escritura: la KEK, la clave VAPID y la contraseña de una base de datos que crea. La única excepción es el secreto de origen de
`ingress.kind = cloudflare`, que guarda la regla de Cloudflare, así que acaba en el estado, que debe permanecer privado.

Los secretos que solo tú tienes, como la URL de una base de datos externa, una contraseña SMTP, una clave de acceso o las claves de
`secrets`, el módulo los crea vacíos. `tofu output secrets_to_fill` enumera cada uno, con lo que va en él y el comando que
lo añade. Un servidor cuyo secreto no tiene valor no arranca. El servidor lee sus secretos cuando arranca una instancia o una tarea,
así que un valor nuevo surte efecto en la siguiente.

**OpenTofu genera la KEK una vez y nunca más.** El secreto y su versión tienen `prevent_destroy`, y la versión que
escribiría un valor nuevo está fijada en el módulo: una KEK aleatoria nueva hace ilegible todo lo que cifró la antigua. Para
rotarla, sigue [Rotar la KEK](key-management.md#rotate-the-kek), añadiendo el valor nuevo con la CLI de la plataforma, que
OpenTofu no rastrea.

<!-- source: first-deployment 9ae06fd14fc1 -->
## Primer despliegue

Necesitas `tofu` 1.11 o posterior, `jq` y la CLI de la plataforma (`gcloud` o `aws`) donde aplicas, ya que un apply ejecuta la migración desde
ahí; un dominio para [`host`](../reference/configuration.md#host), que no puede cambiar después
([Ajustes que no puedes cambiar después](production-checklist.md#settings-you-cannot-change-later)); y una versión de `my` de
las [versiones publicadas](running.md#get-the-image). No existe la etiqueta `latest`. Haz primero lo que pida la página de la plataforma, como crear
la base de datos.

<!-- source: 1-copy-the-example-and-set-the-variables 8f1ba80ba8d2 -->
### 1. Copia el ejemplo y define las variables

Copia el ejemplo, con su archivo de bloqueo de proveedores, de la versión que despliegas a un directorio vacío:

```sh
VERSION=v0.1.0
EXAMPLE=gcp-cloud-run-neon # or aws-ecs-neon
git clone --depth 1 --branch "$VERSION" https://github.com/mycroblog/my.git my-source
mkdir my-deployment
cp -r "my-source/deploy/opentofu/examples/$EXAMPLE/." my-deployment/
rm -rf my-source
cd my-deployment
cp terraform.tfvars.example terraform.tfvars
```

**Define `ref` con esa versión** en `main.tf`, que la copia de una versión estable ya tiene bien. Para una versión preliminar, define tanto
`ref` como la imagen, la versión sin su `v`, ya que el módulo de una versión preliminar no usa por defecto su propia imagen:
`sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf`, y `image = { version = "0.1.0-rc.4" }` en el bloque.

Rellena `terraform.tfvars` y añade al bloque de `main.tf` cualquier sección que cambies. Decide ya
[`account_domain`](../reference/configuration.md#account_domain), como `settings = { account_domain = "example.org" }`, si
los identificadores usan otro dominio.

Conserva este directorio: contiene tus ajustes y, por defecto, el estado. El estado no contiene valores secretos, pero es el único
registro de lo que se creó: guárdalo en un backend remoto, como un bucket de `gcs` o `s3`, si aplica más de una persona.

<!-- source: 2-create-the-secrets-and-add-their-values 9f13e8109bc9 -->
### 2. Crea los secretos y añade sus valores

```sh
tofu init
SCRIPTS=.terraform/modules/mycroblog/deploy/opentofu
```

`tofu init` descarga el módulo y sus scripts de esa versión, con el nombre de tu bloque: `migrate.sh`, que ejecuta un apply,
y `console.sh`, que ejecuta [la consola](#the-console).

Aplica primero solo los secretos, lo que los crea y genera la KEK y la clave VAPID, con el módulo del tipo de la KEK:

| Módulo raíz | Apply |
| --- | --- |
| `mycroblog-gcp` | `tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secret_manager` |
| `mycroblog-aws` | `tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secrets_manager` |

Con `kek = { kind = "kms" }`, usa como destino `module.mycroblog.module.kek_kms` en lugar del secreto de la KEK.

**Copia la KEK ahora**, y guarda la copia aparte de la base de datos, por ejemplo en un gestor de contraseñas: sin ella una
[copia de seguridad](backup.md) no puede restaurarse, y se pierden las claves de las cuentas. `tofu output kek` tiene el comando. Copia la clave
VAPID, `<name>-vapid-key`, del mismo modo: perderla termina todas las suscripciones de Web Push.

Después ejecuta cada comando de `tofu output secrets_to_fill` con su valor. Añádelos todos antes del paso siguiente.

<!-- source: 3-apply b9f715c7b55d -->
### 3. Aplica

```sh
tofu apply
```

Esto crea el job de migración con aquello de lo que depende, ejecuta en él `my upgrade` mediante `migrate.sh` y espera a que termine:
`my config check`, `my db upgrade -check`, `my schema apply` y `my db upgrade`, que se detiene en el primero que falla. Su
salida aparece en la del apply. El primer comando que usa la base de datos registra en ella `host` y `account_domain`. Solo entonces
el apply crea los servicios, cuya comprobación de salud falla hasta que existe el esquema.

Si la migración falla, el apply se detiene antes de los servicios. Corrige la causa, a menudo un secreto sin valor, y ejecuta
`tofu apply` de nuevo: ejecuta la migración otra vez.

<!-- source: 4-point-the-domain-at-the-server b7b1ead901bc -->
### 4. Apunta el dominio al servidor

`tofu output dns_records` enumera los registros que añadir en tu proveedor de DNS, y está vacío donde el módulo los crea.

<!-- source: 5-create-the-first-administrator bb599aec43c2 -->
### 5. Crea el primer administrador

La consola no tiene terminal ni entrada estándar, así que `my admin create` imprime un enlace que establece la contraseña en lugar de
leer una:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

Abre el enlace enseguida y establece la contraseña. Funciona una sola vez, durante una hora, y **permanece en los registros de la plataforma hasta entonces**,
donde cualquiera que pueda leerlos puede usarlo. Si caduca, ejecuta `my admin account password-reset-link @alice` mediante la
consola. Después activa la autenticación en dos pasos.

<!-- source: 6-go-through-the-checklist 12999c41613d -->
### 6. Repasa la lista de comprobación

Repasa la [lista de comprobación para producción](production-checklist.md), empezando por el [correo electrónico](production-checklist.md#email): SPF,
DKIM y DMARC en el dominio de `mail.from`.

<!-- source: the-console 1b3d6dfa9c3c -->
## La consola

La consola ejecuta un comando de `my` con la imagen, los ajustes y los secretos del servicio. Existe solo mientras
`console_enabled` está activado, así que desactívalo cuando termines:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` espera al comando, imprime su salida, que lee de nuevo de los registros de la plataforma, y termina con su código de
salida. Usa la imagen que ejecuta el servicio: tras una actualización que se detuvo en la migración, es la versión anterior, a la que
la base de datos aún se ajusta.

El comando no tiene entrada estándar ni terminal, así que un comando que lea de ellos una contraseña o un archivo no puede ejecutarse de este
modo. Lo que imprime permanece en los registros, así que no ejecutes un comando cuya salida no escribirías ahí.

<!-- source: upgrading 4be8a01734ce -->
## Actualización

Lee antes [Actualizar](database.md#upgrading) y las notas de la versión, y avanza una versión menor cada vez. Una actualización es un
apply de la versión nueva:

```sh
# 1. Back up the database while nothing writes: see "The maintenance mode" below, and Backup and restore.
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Name the new release in main.tf, as ?ref=v0.2.0, and fetch it.
tofu init

# 3. Read the plan, then apply it.
tofu plan
tofu apply
```

Lee el plan: el módulo de una versión nueva también puede cambiar recursos. El apply hace esto, y se detiene en el primer paso que falla:

1. Da a los jobs de migración y de consola (definiciones de tarea en AWS) la imagen nueva, con el resto del plan del que no dependen los servicios.
2. Ejecuta `my upgrade` en el job de migración mediante `migrate.sh`, desde donde aplicas, y espera a que termine, como en el
   [primer despliegue](#3-apply). Antes espera a cualquier ejecución que un apply interrumpido haya dejado en marcha.
3. Pasa cada worker a la imagen nueva, y después el servicio. Una revisión nueva recibe tráfico solo cuando supera su comprobación de salud.

**Donde aplicas** necesitas `jq`, la CLI de la plataforma y credenciales que puedan ejecutar el job y leer sus registros, que enumera la
página de la plataforma. Lo mismo vale para un trabajo de CI que aplica.

**Cuando un paso falla**, los servicios permanecen en sus revisiones, y las notas de la versión o la salida de la migración dicen por qué. Corrige
la causa y ejecuta `tofu apply` de nuevo con la misma versión: ejecuta la migración otra vez, y los pasos que se completaron no encuentran
nada que hacer. Un apply de la misma imagen no ejecuta nada. Para volver atrás después de que cambió el esquema, restaura la copia de seguridad: consulta
[Revertir](database.md#rolling-back). Un apply de una versión anterior se detiene en la migración, ya que esa versión rechaza
un esquema más nuevo, y no cambia ningún servicio.

Para probar un candidato a versión, o para conservar la imagen de una versión anterior bajo un módulo más nuevo, define tú mismo la versión, la
etiqueta de la imagen sin la `v` de la versión:

```hcl
  image = { version = "0.2.0-rc.1" }
```

<!-- source: checking-the-sql-first 84178f101ce0 -->
### Comprobar antes el SQL

El plan no muestra el SQL de la migración. Para leerlo antes del apply, aplica solo el job de migración con la imagen nueva,
ejecuta en él `my upgrade -dry-run`, que no cambia nada, y lee sus registros. La página de la plataforma tiene los comandos.

<!-- source: destructive-changes 0bea88d60224 -->
### Cambios destructivos

Cuando las notas de la versión pidan `-allow-destructive`, nombra esa versión en el bloque:

```hcl
  upgrade = { allow_destructive_for = "0.3.0" }
```

`my upgrade` recibe `-allow-destructive` solo en el apply de esa versión, así que un valor que se deje no permite que ninguna versión posterior
elimine datos.

<!-- source: pinning-the-image-by-digest 9c3f49e5ee10 -->
### Fijar la imagen por digest

La [lista de comprobación para producción](production-checklist.md#operations) recomienda un digest, de las notas de la versión:

```hcl
  image = { digest = "sha256:..." }
```

Un digest nombra una sola imagen, diga lo que diga la etiqueta que lo acompaña, así que cámbialo en la misma edición que `ref`: un digest que quede de la
versión anterior mantiene la imagen de esa versión bajo la versión nueva.

<!-- source: the-maintenance-mode 57bd5487b045 -->
### El modo de mantenimiento

El [modo de mantenimiento](maintenance.md) requiere tres apply, ya que los servicios cambian antes de la actualización y después de ella. Añádelo
a los ajustes del bloque y aplica, haz la copia de seguridad, sube `ref` y aplica, y luego quítalo y aplica:

```hcl
  settings = { maintenance = "true" }
```

Sin él, la revisión antigua sigue sirviendo durante la migración, lo que
[no se garantiza que funcione](database.md#the-server-during-an-upgrade).

<!-- source: splitting-jobs-into-workers 6336c0873c82 -->
## Dividir los jobs en workers

Cuando un tipo de job retiene al resto, dale su propio servicio. Decide qué dividir a partir de las métricas, como dice
[Dividir los workers por tipo de trabajo](running.md#splitting-workers-by-job-type). `workers` acepta un mapa con una clave que nombra cada
worker, con sus `job_types`, y el módulo crea a partir de él cada cola, ruta y ajuste de la división, así que no escribes ninguno
de ellos. Para evitar que las entregas retrasen al resto:

```hcl
  workers = {
    delivery = { job_types = ["federation.fanout", "federation.deliver"] }
  }
```

Para mantener ágiles el correo y las notificaciones:

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
    }
  }
```

El plan rechaza una división de los tipos que deben permanecer juntos. Deja fuera los tipos de ajustes que no has definido, como dice esa
página: un tipo que el servidor no ha registrado detiene el arranque, y el plan no puede ver cuáles son. El servicio predeterminado
ejecuta todos los tipos que ningún worker nombra, y ninguno de los que nombra alguno.

Un worker también acepta el tamaño de su servicio, cada uno el del servicio predeterminado donde no se nombra:
[la referencia de los módulos](../../deploy/opentofu/README.md#workers) enumera los campos de cada plataforma. Un worker no recibe
tráfico del exterior, y una actualización lo mueve después de la migración y antes del servicio predeterminado. Cada worker puede traer lo que
cuesta el servicio, y las conexiones de sus instancias cuentan para el límite de la base de datos.

<!-- source: changing-a-backend df886f733f8e -->
## Cambiar un backend

Cambia el `kind` de su sección. Cada tipo define lo que necesita en el servidor, y el plan dice qué cambia. Cambiar la
base de datos también significa mover sus datos, ya que una nueva está vacía: haz una [copia de seguridad](backup.md) de la antigua y restáurala en
la nueva mientras nada escribe.

<!-- source: the-kek 62356f591998 -->
### La KEK

Cambiar `kek.kind` es una línea, pero en una instancia con datos no sale gratis: la base de datos contiene secretos envueltos con la
clave actual, entre ellos las claves de todas las cuentas, así que una instancia que la pierde ya no puede leerlos. Sigue
[Pasar de la KEK local a un KMS y viceversa](key-management.md#move-between-the-local-kek-and-a-kms). Del secreto a un KMS, conserva
la KEK actual como `kek_previous` mientras se mueven los secretos: añade `secrets = ["kek_previous"]` y rellena el secreto
`<name>-kek-previous` con la KEK actual. Ejecuta `my kek rewrap` mediante [la consola](#the-console) y quítala cuando
`my kek status` ya no cuente claves `local:`. Conserva la KEK mientras conserves copias de seguridad de la base de datos anteriores al rewrap.

En una instancia nueva sin datos, no hay nada que mover.
