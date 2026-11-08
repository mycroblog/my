<!-- source: database-initializing-upgrading-and-recreating b4189f003273 -->
# Base de datos: inicializar, actualizar y recrear

Cómo crean los operadores la base de datos, la actualizan a una versión nueva y, solo en instancias de desarrollo y de prueba, la borran.

El esquema está en `schema/`, un directorio por módulo con un archivo por base de datos (`schema/<NN>-<module>/sqlite.sql` y `postgres.sql`). El binario del servidor contiene el esquema de su versión, y
`my schema apply` lo aplica: todos los despliegues lo ejecutan como un trabajo puntual antes del servidor, con la imagen del servidor y la configuración
de base de datos del servidor, así que la imagen del servidor es la única imagen que necesita una instancia.
`my serve` nunca aplica el esquema. Si se inicia sobre una base de datos que no lo tiene, el servidor responde `503` en `/readyz` (consulta [Iniciar el servidor](running.md#start-the-server)).

Los datos que un cambio de esquema tiene que trasladar (una columna dividida en dos, un valor movido a una tabla nueva) los reescribe el propio
`my db upgrade` de la imagen del servidor, que se ejecuta una vez por versión después de aplicar el esquema. El orden completo está en [Actualizar](#upgrading).

<!-- source: initializing-applying-the-schema 3b0a5f0defa1 -->
## Inicializar (aplicar el esquema)

`my schema apply` ejecuta solo las sentencias que llevan la base de datos al esquema de su versión: en una base de datos vacía lo crea todo,
y en una al día no hace nada. Es seguro ejecutarlo cualquier número de veces, y cada actualización lo ejecuta de nuevo.

<!-- source: with-my-schema-apply df4290f9f1c4 -->
### Con `my schema apply`

`my schema apply` lee la configuración de base de datos de la configuración del servidor (`database`, `database_url` y `database_dev_url`,
de `-config <file>` o del entorno; no necesita ninguna otra configuración) y lleva la base de datos al esquema de la versión del binario:

```sh
my schema apply -dry-run   # print the SQL statements it would run, and change nothing
my schema apply            # run them
```

- Muestra las sentencias SQL que ejecuta y las ejecuta en una sola transacción: cuando una falla, no queda ninguna.
  Una base de datos que ya coincide con el esquema no se modifica (`ok: the database matches the schema; nothing to apply`).
- Un plan que elimina una tabla o una columna se rechaza y no se ejecuta nada, salvo que se indique `-allow-destructive`. Indícalo solo
  cuando las notas de la versión lo pidan, después de una copia de seguridad. Reconstruir una tabla de SQLite para cambiarla no cuenta como eliminación.
- En PostgreSQL construye el esquema con el que compara en un esquema temporal de la misma base de datos y lo elimina al final,
  también si falla; el rol necesita el privilegio para crear un esquema ahí. Donde no lo tenga, define `database_dev_url` como otra base de datos donde sí lo tenga
  ([`database_dev_url`](../reference/configuration.md#database_dev_url)). Las ejecuciones sobre una misma base de datos se esperan entre sí, así que dos a la vez tienen éxito ambas.
- Admite `database = sqlite` y `database = postgres`, y termina con `1` cuando no aplicó nada por un error o por una eliminación rechazada.

Desde `v0.1.0`, una actualización entre versiones admitidas nunca contiene una diferencia que pierda datos; las versiones anteriores a `v0.1.0` pueden eliminar columnas
y tablas, y necesitan `-allow-destructive`. Lee el SQL con `-dry-run` antes de cada actualización (consulta [Actualizar](#upgrading)).

<!-- source: through-a-connection-pooler dd56827cda63 -->
### A través de un agrupador de conexiones

`my schema apply` debe llegar directamente a la base de datos. Mantiene un bloqueo a nivel de sesión, que un agrupador en modo de transacción (la agrupación por transacción de PgBouncer, la cadena de conexión agrupada de Neon) no conserva: a través de uno, dos ejecuciones a la vez se quedaron colgadas en lugar de fallar. Dale una URL directa como `database_url`. `pool_max_conns` y los demás parámetros `pool_*` pueden quedarse en ella, y en `database_dev_url` y `stream_postgres_url`: los comandos que no abren ningún pool los ignoran. `my db upgrade`, `my db upgrade -check` y la consola usan la misma URL.

`my serve` puede usar un agrupador en modo de transacción, siempre que:

- el agrupador gestione las sentencias preparadas a nivel de protocolo, que el servidor usa en todas las consultas: PgBouncer 1.21 o posterior con `max_prepared_statements` por encima de 0, que tiene la cadena de conexión agrupada de Neon. Detrás de un agrupador sin ellas, las sentencias fallan al azar con `prepared statement ... does not exist` o `already exists`: añade `default_query_exec_mode=exec` a la URL, lo que las evita a costa de un viaje de ida y vuelta más por consulta.
- `stream_postgres_url` contenga una URL directa cuando `stream = postgres`, ya que `LISTEN` pertenece a una sesión. Un oyente en una URL agrupada se conecta y después no recibe nada.
- la URL no tenga `search_path` ni otro parámetro que el agrupador rechace en la conexión: PgBouncer cierra esa conexión salvo que `ignore_startup_parameters` lo liste.

<!-- source: docker-compose 7971a448363a -->
### Docker Compose

En [`deploy/docker/compose.yaml`](../../deploy/docker/compose.yaml), tres servicios puntuales de la imagen del servidor se ejecutan en este orden, cada uno solo
después de que el anterior haya tenido éxito, y el servidor (`my`) arranca el último:

| Servicio | Ejecuta |
| --- | --- |
| `upgrade-check` | `my db upgrade -check` (pasa en una base de datos vacía) |
| `schema` | `my schema apply`, sobre `/data/my.db` o, con la capa `compose.postgres.yaml`, sobre su PostgreSQL ([Archivos de Docker Compose](running.md#docker-compose-files)) |
| `upgrade` | `my db upgrade` |

```sh
cd deploy/docker
docker compose up -d             # pulls the image of MY_VERSION; the three one-off services run in order, then my starts
curl -fsS localhost:8080/readyz  # ready when it answers {"status":"ok"}
docker compose exec -T my my admin create -username alice -email alice@example.com < password.txt
```

- Si falla cualquiera de los tres, `my` no arranca. Lee su salida con `docker compose logs upgrade-check` (o `schema`, `upgrade`).
- La creación de administradores se explica en [Administradores y roles](admin-cli.md).

<!-- source: other-platforms 5f4cdac7fe7a -->
### Otras plataformas

Sin Compose, ejecuta `my schema apply` de la imagen del servidor como un trabajo puntual (un Job o un init container de Kubernetes, un job de Cloud Run, etcétera),
con la misma configuración de base de datos que el servidor:

```sh
# SQLite: mount the volume that holds the database file at /data
docker run --rm -v my_data:/data -e MY_DATABASE=sqlite -e MY_DATABASE_URL=/data/my.db my schema apply

# PostgreSQL
docker run --rm -e MY_DATABASE=postgres -e MY_DATABASE_URL='postgres://my:...@db.example/my' my schema apply
```

<!-- source: upgrading 4996c9dc9019 -->
## Actualizar

Una actualización a una versión nueva siempre ejecuta estos pasos en este orden, con la imagen del servidor de la versión **nueva**:

1. **Haz una copia de seguridad** de la base de datos (y, con el almacén de blobs en el sistema de archivos, de los archivos multimedia). Consulta [Hacer la copia](#backing-up).
2. **`my db upgrade -check`**. No cambia nada. Termina con `1` cuando el esquema nuevo eliminaría datos que la base de datos todavía no ha
   trasladado, y nombra la versión por la que hay que actualizar primero; en ese caso, detente y sigue [Saltar versiones](#skipping-releases).
   Una base de datos vacía pasa.
3. **`my schema apply`**, como en [Inicializar](#initializing-applying-the-schema). Comprueba antes el SQL con `-dry-run`.
4. **`my db upgrade`**. Ejecuta cada paso de actualización de datos de la versión que la base de datos todavía no ha registrado, reconstruye las
   proyecciones que esos pasos nombran y registra cada paso. Si se ejecuta de nuevo, no hace nada. Si un paso falla, termina con `1`; los pasos que se completaron
   siguen registrados, y al ejecutarlo de nuevo se reanuda desde el que falló.
5. **Inicia el servidor nuevo.** Responde `503` en `/readyz` y registra los pasos que faltan hasta que todos los pasos de actualización de datos de su versión estén registrados.

`my serve` nunca ejecuta `my db upgrade -check`, `my schema apply` ni `my db upgrade`; son trabajos puntuales. Ejecuta solo uno de ellos a la vez sobre una base de datos.

La mayoría de las versiones no tienen ningún paso de actualización de datos: `my db upgrade -check` muestra `ok: the database can be upgraded to this release` y
`my db upgrade` muestra `ok: no data upgrade step to run`. Ejecuta ambos de todos modos, para que el procedimiento no dependa de leer todas las notas de cada versión.
La sección **Actualizar** de las notas de la versión lista los pasos de cada versión.

<!-- source: the-server-during-an-upgrade ef479675745b -->
### El servidor durante una actualización

No se garantizan las actualizaciones sin tiempo de inactividad. El servidor anterior sigue funcionando con el esquema nuevo durante el breve tiempo entre los pasos 3 y 5,
pero no puede participar en los pasos de actualización de datos. Cambia al [modo de mantenimiento](maintenance.md) para la actualización: en el modo de mantenimiento
`my serve` no abre ninguna conexión con la base de datos, así que la copia es consistente y nada escribe mientras se ejecutan los pasos.
Mantén la ventana de mantenimiento en unas pocas horas ([cuánto dura](maintenance.md#how-long)).

![Los pasos de una actualización de arriba abajo, y lo que responde /readyz en cada uno: 200 con status ok antes, 200 con status maintenance desde el modo de mantenimiento hasta que arranca la versión nueva, y después de nuevo 200 con status ok. La lista de abajo describe cada paso.](images/upgrade.svg)

De arriba abajo en el diagrama:

- **Versión anterior.** El servidor funciona como de costumbre, y `/readyz` responde `200` con `{"status":"ok"}`.
- **Modo de mantenimiento.** Vuelve a desplegar con `MY_MAINTENANCE=true`. Desde ahora hasta que arranque la versión nueva, `/readyz` responde `200` con
  `{"status":"maintenance"}`, así que el balanceador de carga conserva las instancias mientras nada llega a la base de datos.
- **1. Copia de seguridad** de la base de datos y de los archivos multimedia.
- **2 a 4: los trabajos puntuales de la imagen nueva.** `my db upgrade -check` no cambia nada; `my schema apply` se ejecuta después de que
  leas su `-dry-run`; `my db upgrade` ejecuta los pasos de actualización de datos.
- **5. Inicia la versión nueva** sin `MY_MAINTENANCE`. `/readyz` responde `200` con `{"status":"ok"}` cuando todos los pasos de actualización de datos
  de la versión están registrados, y `503` mientras falte alguno (o el esquema no esté aplicado).

Con Docker Compose, `docker compose up` sustituye el contenedor del modo de mantenimiento antes de que se ejecuten los trabajos puntuales, así que nada responde
en `/readyz` durante los pasos 2 a 4 ([Docker Compose](#docker-compose-1)).

<!-- source: checking-the-sql-with--dry-run ac385f82c7d5 -->
### Comprobar el SQL con `-dry-run`

Antes de que el paso 3 aplique nada, ejecuta `my schema apply -dry-run` y lee lo que ejecutaría.
Las adiciones (`CREATE TABLE`, `ADD COLUMN`, `CREATE INDEX`) son lo que contienen la mayoría de las versiones. También puede reconstruirse una tabla de proyección,
que `my db upgrade` vuelve a llenar después a partir de sus eventos. Un `DROP` de cualquier otra cosa, o un cambio de tipo, debería figurar en la sección
**Cambios incompatibles** de las notas de la versión; si no figura, detente e infórmalo antes de aplicar. `my schema apply` rechaza un plan que elimina una tabla
o una columna salvo que se indique `-allow-destructive`.

<!-- source: docker-compose-1 c4c380ec8daa -->
### Docker Compose

`docker compose up -d` ejecuta por sí mismo los pasos 2 a 5, en orden (consulta la tabla de [Inicializar](#docker-compose)). Si falla cualquiera de ellos, el servidor nuevo no arranca.

```sh
cd deploy/docker

# 1. Switch to the maintenance mode. --no-deps keeps the one-off services from running.
MY_MAINTENANCE=true docker compose up -d --no-deps --force-recreate my

# 2. Back up the volume (database and media) while the maintenance mode holds no connection.
docker run --rm -v my_data:/data -v "$PWD":/backup busybox tar czf /backup/my-data-$(date +%Y%m%d%H%M).tar.gz -C /data .

# 3. Set MY_VERSION in .env to the new version (0.2.0, or its digest form 0.2.0@sha256:... from the notes), and pull it.
docker compose pull

# 4. Check the SQL that the schema service will run.
docker compose run --rm --no-deps upgrade-check
docker compose run --rm --no-deps schema schema apply -dry-run

# 5. Upgrade: upgrade-check, schema, upgrade, then my starts again without the maintenance mode.
docker compose up -d
curl -fsS localhost:8080/readyz
```

`docker compose up` sustituye el contenedor `my` antes de que se ejecuten los servicios puntuales, y arranca el nuevo solo después de que los tres hayan tenido éxito.
Si uno falla, `docker compose up` lo informa y el servidor queda detenido: mientras tanto, coloca delante la [página de mantenimiento exportada](maintenance.md#docker-compose).
Lee `docker compose logs <service>` y después corrige la causa y ejecuta `docker compose up -d` de nuevo, o [revierte](#rolling-back).
El nombre de volumen `my_data` es el nombre del proyecto de Compose (`my`) seguido del nombre del volumen (`data`).
`MY_VERSION` es lo único que cambia una actualización, y nada lo cambia por ti: no hay etiqueta `latest`, y `0.1` solo se mueve dentro de `v0.1`. Con `MY_VERSION=0.1`, el paso 3 es solo `docker compose pull`, y toma el último parche.
Para compilar desde el código fuente, extrae la etiqueta de la versión nueva y usa `docker compose -f compose.yaml -f compose.build.yaml build` en el paso 3, y las mismas opciones `-f` en los demás pasos.

<!-- source: kubernetes b423ca2c88f9 -->
### Kubernetes

Ejecuta los pasos 2 a 4 como un solo Job antes de desplegar la imagen nueva. Los init containers se ejecutan uno tras otro, cada uno solo después de que el anterior haya tenido éxito,
y el contenedor principal se ejecuta el último, así que un fallo detiene el Job antes del paso siguiente. Da a los contenedores la misma configuración que al Deployment.

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: my-upgrade-v0-2-0
spec:
  backoffLimit: 0
  template:
    spec:
      restartPolicy: Never
      initContainers:
        - name: upgrade-check
          image: ghcr.io/mycroblog/my:0.2.0
          args: [db, upgrade, -check]
          envFrom: [{ secretRef: { name: my-config } }]   # the Deployment's settings
        - name: schema
          image: ghcr.io/mycroblog/my:0.2.0
          args: [schema, apply]
          envFrom: [{ secretRef: { name: my-config } }]
      containers:
        - name: upgrade
          image: ghcr.io/mycroblog/my:0.2.0
          args: [db, upgrade]
          envFrom: [{ secretRef: { name: my-config } }]
```

```sh
kubectl set env deployment/my MY_MAINTENANCE=true && kubectl rollout status deployment/my
# back up, and check the SQL with a one-off Pod of the new image: my schema apply -dry-run
kubectl apply -f my-upgrade-v0-2-0.yaml
kubectl wait --for=condition=complete --timeout=30m job/my-upgrade-v0-2-0
kubectl set image deployment/my my=ghcr.io/mycroblog/my:0.2.0
kubectl set env deployment/my MY_MAINTENANCE-
kubectl rollout status deployment/my
```

Si el Job falla, `kubectl logs job/my-upgrade-v0-2-0 -c upgrade-check` (o `-c schema`, `-c upgrade`) muestra el motivo, y el Deployment se queda con la imagen anterior.
Con SQLite en un volumen, monta el mismo volumen en el Job y detén antes los pods del Deployment: SQLite admite un solo escritor.

<!-- source: cloud-run a6c3de3c814f -->
### Cloud Run

Con los módulos de OpenTofu, el script de [Desplegar en Cloud Run](deploy-cloud-run.md#upgrading) ejecuta estos pasos. A mano:

Los jobs de Cloud Run ejecutan un contenedor cada uno, así que crea un job por paso y ejecútalos en orden con `--wait`, que falla cuando falla el job.
Da a los jobs las mismas variables de entorno y secretos que al servicio.

```sh
gcloud run services update my --region=asia-northeast1 --update-env-vars=MY_MAINTENANCE=true
# back up (see below)

gcloud run jobs deploy my-upgrade-check --region=asia-northeast1 --image=ghcr.io/mycroblog/my:0.2.0 \
  --args=db,upgrade,-check --set-env-vars=... --set-secrets=...
gcloud run jobs deploy my-schema --region=asia-northeast1 --image=ghcr.io/mycroblog/my:0.2.0 \
  --args=schema,apply --set-env-vars=... --set-secrets=...
gcloud run jobs deploy my-upgrade --region=asia-northeast1 --image=ghcr.io/mycroblog/my:0.2.0 \
  --args=db,upgrade --set-env-vars=... --set-secrets=...

gcloud run jobs execute my-upgrade-check --region=asia-northeast1 --wait &&
  gcloud run jobs execute my-schema --region=asia-northeast1 --wait &&
  gcloud run jobs execute my-upgrade --region=asia-northeast1 --wait &&
  gcloud run deploy my --region=asia-northeast1 --image=ghcr.io/mycroblog/my:0.2.0 --remove-env-vars=MY_MAINTENANCE
```

Para comprobar antes el SQL, despliega el job del esquema con `--args=schema,apply,-dry-run`, ejecútalo y lee sus registros antes de volver a desplegarlo para aplicar.

<!-- source: backing-up b597d3139c2b -->
### Hacer la copia

Haz la copia de seguridad después de que el servidor haya dejado de escribir (en el modo de mantenimiento, o detenido) y antes de cualquier paso de la actualización.
[Copia de seguridad y restauración](backup.md) cubre cada parte por completo; para una actualización:

- **SQLite**: copia el archivo de la base de datos junto con sus archivos `-wal` y `-shm`, mientras ningún proceso lo tenga abierto.
  Con Compose, el `tar` del volumen de arriba también cubre los archivos multimedia en `/data/blobs`.
- **PostgreSQL**: `pg_dump` en el formato personalizado, que lee `pg_restore`.

  ```sh
  pg_dump --format=custom --file=my-$(date +%Y%m%d%H%M).dump "postgres://my:...@db.example/my"
  ```

  La copia propia de una base de datos administrada (una copia bajo demanda de Cloud SQL, una instantánea de RDS) también sirve; hazla en el mismo momento.
- **Archivos multimedia**: el almacén de blobs del sistema de archivos es un directorio que se copia junto con la base de datos.
- **La KEK** no está en la base de datos. Guárdala donde ya está; una base de datos restaurada necesita la misma KEK.

<!-- source: rolling-back 59a40002cd86 -->
### Revertir

No hay migraciones inversas. Para volver atrás, restaura la copia hecha antes de la actualización y luego inicia la versión **anterior**:

1. Detén el servidor nuevo (o ponlo en el modo de mantenimiento).
2. Restaura la copia: con Compose, extrae el archivo en el volumen vaciado
   (`docker run --rm -v my_data:/data -v "$PWD":/backup busybox sh -c 'rm -rf /data/* && tar xzf /backup/<archive> -C /data'`);
   con PostgreSQL, `pg_restore --clean --if-exists --dbname=<url> <dump>`.
3. Inicia la imagen de la versión anterior (con Compose, vuelve a poner `MY_VERSION` en la versión anterior y luego `docker compose up -d`).

Todo lo que la versión nueva haya escrito después de la actualización se pierde, así que revierte pronto tras la actualización, antes de que la instancia federe mucho.

<!-- source: skipping-releases 9c7038296a6d -->
### Saltar versiones

Cada versión admite actualizaciones desde la misma versión menor (por ejemplo, de `v0.2.1` a `v0.2.4`) y desde la última versión de parche de la versión
menor anterior (de la última `v0.1.x` a `v0.2.y`). Las versiones de parche nunca eliminan columnas ni tablas, así que dentro de una versión menor sirve cualquier orden.

Para cruzar más de una versión menor, actualiza pasando por la última versión de parche de cada versión menor, una tras otra, ejecutando todo el procedimiento cada vez.
De `v0.1.3` a `v0.4.0`, por ejemplo: la última `v0.1.x`, luego la última `v0.2.x`, luego la última `v0.3.x` y después `v0.4.0`.
`my db upgrade -check` detiene un salto que perdería datos y nombra la versión por la que hay que pasar; hasta ese punto no ha cambiado nada.

<!-- source: checking-and-rebuilding-projections 40b848afd40d -->
## Comprobar y reconstruir proyecciones

Algunas tablas son proyecciones: el servidor las deriva de los eventos y recursos de su módulo (las restricciones de una cuenta a partir de sus
acciones de moderación, por ejemplo). El worker comprueba todas las proyecciones una vez al día y corrige cualquier diferencia que encuentre. Para comprobar o corregir una a mano:

```sh
my projection list              # each projection with its module and tables
my projection verify [<name>]   # derive every projection, or the one named, and print the rows that differ; changes nothing
my projection rebuild [<name>]  # derive them the same way and write the difference
```

- `<name>` es una proyección o una de sus tablas.
- `verify` muestra las filas que contienen las tablas pero que las fuentes no derivan (`-`) y las filas que derivan las fuentes pero que faltan en las tablas (`+`),
  y termina con `1` cuando hay alguna diferencia.
- `rebuild` escribe como máximo 500 filas por transacción, de modo que los lectores nunca ven una proyección vaciada. Ejecútalo cuando la instancia esté tranquila:
  una fila que cambia durante la reconstrucción conserva su valor nuevo, y un `verify` posterior informa de cualquier diferencia que quede.

<!-- source: recreating-erasing-every-row 9ff993b99448 -->
## Recrear (borrar todas las filas)

Recrear significa vaciar la base de datos y aplicar de nuevo el esquema. **Desde `v0.1.0`, recrear es solo para instancias de desarrollo y de prueba.
Nunca recrees una instancia pública que federa con otros servidores.**

Se pierden todas las filas y no se pueden recuperar. También se pierden las claves de las cuentas envueltas con la clave de cifrado de claves (KEK), así que una cuenta recreada con el
mismo nombre de host obtiene una clave distinta y es un actor distinto para los demás servidores, y sus relaciones de seguimiento remotas no vuelven.
Una instancia pública que no puede aplicar un cambio de esquema [revierte](#rolling-back) desde su copia de seguridad.

No hay ningún comando `my` que borre una base de datos: elimínala con las herramientas de la base de datos y aplica el esquema de nuevo.
La base de datos vacía no tiene `host` ni `account_domain` registrados, así que el siguiente arranque registra los valores configurados.

<!-- source: docker-compose-2 eba2326609d1 -->
### Docker Compose

```sh
cd deploy/docker
docker compose stop my                  # stop the server
docker run --rm -v my_data:/data busybox rm -f /data/my.db /data/my.db-wal /data/my.db-shm
docker compose up -d                    # the one-off services apply the schema to the empty database, then my starts
```

- Los archivos multimedia subidos y procesados (`/data/blobs`) permanecen. Para borrarlos junto con la base de datos, elimina el volumen con nombre con `docker compose down -v` y ejecuta
  `docker compose up -d`. Sin `-v` el volumen se conserva.
- Para conservar antes una copia, detén el servidor y copia `my.db` (con sus archivos `-wal` y `-shm`) del volumen.

<!-- source: postgresql 679b0e3a5b1f -->
### PostgreSQL

Elimina la base de datos y créala de nuevo, vacía, con el mismo propietario (o con la consola de la base de datos administrada), y luego ejecuta `my schema apply`:

```sh
# stop every server and job that uses the database first
dropdb --maintenance-db="postgres://postgres:...@db.example/postgres" my
createdb --maintenance-db="postgres://postgres:...@db.example/postgres" --owner=my my
my schema apply
```
