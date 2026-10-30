# Database: initializing, upgrading, and recreating

How operators create the database, upgrade it to a new release, and, on development and test instances only, erase it.

The schema lives in `schema/<dialect>/` (one file per module). The server binary holds the schema of its release, and
`my schema apply` applies it: every deployment runs it as a one-off job before the server, with the server image and the server's
database settings, so the server image is the only image an instance needs.
`my serve` never applies the schema. Started on a database without it, the server answers `503` on `/readyz` (see [Start the server](running.md#start-the-server)).

Data that a schema change has to carry over (a column split in two, a value moved to a new table) is rewritten by the server image's
own `my db upgrade`, which runs once per release after the schema is applied. The full order is in [Upgrading](#upgrading).

## Initializing (applying the schema)

`my schema apply` runs only the statements that bring the database to the schema of its release: on an empty database it creates everything,
on an up-to-date one it does nothing. It is safe to run any number of times, and every upgrade runs it again.

### With `my schema apply`

`my schema apply` reads the database settings of the server's configuration (`db`, `database_url`, and `database_dev_url`,
from `-config <file>` or the environment; it needs no other setting) and brings the database to the schema of the binary's release:

```sh
my schema apply -dry-run   # print the SQL statements it would run, and change nothing
my schema apply            # run them
```

- It prints the SQL statements it runs, and runs them in one transaction: when one fails, none of them stays.
  A database that already matches the schema is left unchanged (`ok: the database matches the schema; nothing to apply`).
- A plan that drops a table or a column is refused, and nothing runs, unless `-allow-destructive` is given. Pass it only
  when the release notes call for it, after a backup. Rebuilding a SQLite table to change it does not count as a drop.
- On PostgreSQL it first runs the bootstrap SQL (`CREATE EXTENSION IF NOT EXISTS pg_trgm`), so the role of `database_url`
  needs the privilege to create that extension, or the extension must exist already. The owner of the database has it, since
  PostgreSQL trusts `pg_trgm`. It builds the schema it compares
  with in a temporary schema of the same database and drops it at the end, also on failure; the role needs the privilege
  to create a schema there. Where it has none, set `database_dev_url` to another database where it has
  ([`database_dev_url`](../reference/configuration.md#database_dev_url)). Runs against one database wait for each other, so two at once both succeed.
- It supports `db = sqlite` and `db = postgres`, and exits with `1` when it applied nothing because of an error or a refused drop.

From `v0.1.0` on, an upgrade between supported releases never contains a difference that loses data; releases before `v0.1.0` may drop columns
and tables, and need `-allow-destructive`. Read the SQL with `-dry-run` before every upgrade (see [Upgrading](#upgrading)).

### Docker Compose

In [`deploy/docker/compose.yaml`](../../deploy/docker/compose.yaml), three one-off services of the server image run in this order, each only
after the previous one succeeded, and the server (`my`) starts last:

| Service | Runs |
| --- | --- |
| `upgrade-check` | `my db upgrade -check` (passes on an empty database) |
| `schema` | `my schema apply` on `/data/my.db` |
| `upgrade` | `my db upgrade` |

```sh
cd deploy/docker
docker compose up -d --build     # the three one-off services run in order, then my starts
curl -fsS localhost:8080/readyz  # ready when it answers {"status":"ok"}
docker compose exec -T my my admin create -username alice -email alice@example.com < password.txt
```

- If any of the three fails, `my` does not start. Read its output with `docker compose logs upgrade-check` (or `schema`, `upgrade`).
- Creating administrators is covered in [Administrators and roles](admin-cli.md).
- `scripts/task.sh docker:smoke` (`scripts/docker-smoke.sh`) runs this setup on an empty volume and again on the same volume, as an upgrade does.

### Other platforms

Without Compose, run `my schema apply` of the server image as a one-off job (a Kubernetes Job or init container, a Cloud Run job, and so on),
with the same database settings as the server:

```sh
# SQLite: mount the volume that holds the database file at /data
docker run --rm -v my_data:/data -e MY_DB=sqlite -e MY_DATABASE_URL=/data/my.db my schema apply

# PostgreSQL
docker run --rm -e MY_DB=postgres -e MY_DATABASE_URL='postgres://my:...@db.example/my' my schema apply
```

### From the repository (development)

`scripts/task.sh db:apply` runs `my schema apply` of the working tree; `DB_DIALECT=sqlite|postgres` and `DB_URL` choose the target.
`scripts/task.sh db:diff` only prints the SQL it would run (`-dry-run`).

## Upgrading

An upgrade to a new release always runs these steps in this order, with the **new** release's server image:

1. **Back up** the database (and, with the filesystem blob store, the media). See [Backing up](#backing-up).
2. **`my db upgrade -check`**. It changes nothing. It exits with `1` when the new schema would drop data that the database has not
   carried over yet, and names the release to upgrade through first; then stop and follow [Skipping releases](#skipping-releases).
   An empty database passes.
3. **`my schema apply`**, as in [Initializing](#initializing-applying-the-schema). Check the SQL with `-dry-run` first.
4. **`my db upgrade`**. It runs each data upgrade step of the release that the database has not recorded yet, rebuilds the
   projections those steps name, and records each step. Run again, it does nothing. If a step fails it exits with `1`; the steps that completed
   stay recorded, and running it again resumes from the failed one.
5. **Start the new server.** It answers `503` on `/readyz` and logs the missing steps until every data upgrade step of its release is recorded.

`my serve` never runs `my db upgrade -check`, `my schema apply`, or `my db upgrade`; they are one-off jobs. Run only one of them against a database at a time.

Most releases have no data upgrade step: `my db upgrade -check` prints `ok: the database can be upgraded to this release` and
`my db upgrade` prints `ok: no data upgrade step to run`. Run both anyway, so the procedure does not depend on reading every release note.
The release notes' **Upgrading** section lists the steps of each release.

### The server during an upgrade

Zero-downtime upgrades are not guaranteed. The previous server keeps working on the new schema for the short time between steps 3 and 5,
but it cannot take part in the data upgrade steps. Switch to the [maintenance mode](maintenance.md) for the upgrade: in the maintenance mode
`my serve` opens no database connection, so the backup is consistent and nothing writes while the steps run.
Keep the maintenance window within a few hours ([how long](maintenance.md#how-long)).

### Checking the SQL with `-dry-run`

Before step 3 applies anything, run `my schema apply -dry-run` and read what it would run.
Additions (`CREATE TABLE`, `ADD COLUMN`, `CREATE INDEX`) are what most releases contain. A projection table may also be rebuilt,
which `my db upgrade` then refills from its events. A `DROP` of anything else, or a type change, should be listed in the release notes'
**Breaking changes** section; if it is not, stop and report it before applying. `my schema apply` refuses a plan that drops a table
or a column unless `-allow-destructive` is given.

### Docker Compose

`docker compose up -d` runs steps 2 to 5 by itself, in order (see the table in [Initializing](#docker-compose)). If any of them fails, the new server does not start.

```sh
cd deploy/docker

# 1. Switch to the maintenance mode. --no-deps keeps the one-off services from running.
MY_MAINTENANCE=true docker compose up -d --no-deps --force-recreate my

# 2. Back up the volume (database and media) while the maintenance mode holds no connection.
docker run --rm -v my_data:/data -v "$PWD":/backup busybox tar czf /backup/my-data-$(date +%Y%m%d%H%M).tar.gz -C /data .

# 3. Check out the new release and build its image (or set MY_VERSION in .env to the new version).
git fetch --tags && git checkout v0.2.0
docker compose build

# 4. Check the SQL that the schema service will run.
docker compose run --rm --no-deps upgrade-check
docker compose run --rm --no-deps schema schema apply -dry-run

# 5. Upgrade: upgrade-check, schema, upgrade, then my starts again without the maintenance mode.
docker compose up -d
curl -fsS localhost:8080/readyz
```

`docker compose up` replaces the `my` container before the one-off services run, and starts the new one only after all three succeeded.
If one fails, `docker compose up` reports it and the server stays stopped: put the [exported maintenance page](maintenance.md#docker-compose) in front meanwhile.
Read `docker compose logs <service>`, then either fix the cause and run `docker compose up -d` again, or [roll back](#rolling-back).
The volume name `my_data` is the Compose project name (`my`) followed by the volume name (`data`).

### Kubernetes

Run steps 2 to 4 as one Job before rolling out the new image. Init containers run one after another, each only after the previous one succeeded,
and the main container runs last, so a failure stops the Job before the next step. Give the containers the same configuration as the Deployment.

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
          image: registry.example/my:v0.2.0
          args: [db, upgrade, -check]
          envFrom: [{ secretRef: { name: my-config } }]   # the Deployment's settings
        - name: schema
          image: registry.example/my:v0.2.0
          args: [schema, apply]
          envFrom: [{ secretRef: { name: my-config } }]
      containers:
        - name: upgrade
          image: registry.example/my:v0.2.0
          args: [db, upgrade]
          envFrom: [{ secretRef: { name: my-config } }]
```

```sh
kubectl set env deployment/my MY_MAINTENANCE=true && kubectl rollout status deployment/my
# back up, and check the SQL with a one-off Pod of the new image: my schema apply -dry-run
kubectl apply -f my-upgrade-v0-2-0.yaml
kubectl wait --for=condition=complete --timeout=30m job/my-upgrade-v0-2-0
kubectl set image deployment/my my=registry.example/my:v0.2.0
kubectl set env deployment/my MY_MAINTENANCE-
kubectl rollout status deployment/my
```

If the Job fails, `kubectl logs job/my-upgrade-v0-2-0 -c upgrade-check` (or `-c schema`, `-c upgrade`) shows why, and the Deployment stays on the previous image.
With SQLite on a volume, mount the same volume in the Job and stop the Deployment's pods first: SQLite takes one writer.

### Cloud Run

Cloud Run jobs run one container each, so create one job per step and execute them in order with `--wait`, which fails when the job fails.
Give the jobs the same environment variables and secrets as the service.

```sh
gcloud run services update my --region=asia-northeast1 --update-env-vars=MY_MAINTENANCE=true
# back up (see below)

gcloud run jobs deploy my-upgrade-check --region=asia-northeast1 --image=registry.example/my:v0.2.0 \
  --args=db,upgrade,-check --set-env-vars=... --set-secrets=...
gcloud run jobs deploy my-schema --region=asia-northeast1 --image=registry.example/my:v0.2.0 \
  --args=schema,apply --set-env-vars=... --set-secrets=...
gcloud run jobs deploy my-upgrade --region=asia-northeast1 --image=registry.example/my:v0.2.0 \
  --args=db,upgrade --set-env-vars=... --set-secrets=...

gcloud run jobs execute my-upgrade-check --region=asia-northeast1 --wait &&
  gcloud run jobs execute my-schema --region=asia-northeast1 --wait &&
  gcloud run jobs execute my-upgrade --region=asia-northeast1 --wait &&
  gcloud run deploy my --region=asia-northeast1 --image=registry.example/my:v0.2.0 --remove-env-vars=MY_MAINTENANCE
```

To check the SQL first, deploy the schema job with `--args=schema,apply,-dry-run`, execute it, and read its logs before redeploying it for the apply.

### Backing up

Take the backup after the server stopped writing (in the maintenance mode, or stopped) and before any step of the upgrade.
[Backup and restore](backup.md) covers each part in full; for an upgrade:

- **SQLite**: copy the database file together with its `-wal` and `-shm` files, while no process has it open.
  With Compose, the `tar` of the volume above also covers the media in `/data/blobs`.
- **PostgreSQL**: `pg_dump` in the custom format, which `pg_restore` reads.

  ```sh
  pg_dump --format=custom --file=my-$(date +%Y%m%d%H%M).dump "postgres://my:...@db.example/my"
  ```

  A managed database's own backup (a Cloud SQL on-demand backup, an RDS snapshot) works too; take it at the same point.
- **Media**: the filesystem blob store is a directory to copy with the database.
- **The KEK** is not in the database. Keep it where it already is; a restored database needs the same KEK.

### Rolling back

There are no reverse migrations. To go back, restore the backup taken before the upgrade, then start the **previous** release:

1. Stop the new server (or switch it to the maintenance mode).
2. Restore the backup: with Compose, extract the archive into the emptied volume
   (`docker run --rm -v my_data:/data -v "$PWD":/backup busybox sh -c 'rm -rf /data/* && tar xzf /backup/<archive> -C /data'`);
   with PostgreSQL, `pg_restore --clean --if-exists --dbname=<url> <dump>`.
3. Start the previous release's image (with Compose, check out the previous tag or set `MY_VERSION` back, then `docker compose up -d`).

Whatever the new release wrote after the upgrade is lost, so roll back soon after the upgrade, before the instance federates much.

### Skipping releases

Each release supports upgrades from the same minor version (for example `v0.2.1` to `v0.2.4`) and from the latest patch release of the previous
minor version (the latest `v0.1.x` to `v0.2.y`). Patch releases never remove columns or tables, so within a minor version any order works.

To cross more than one minor version, upgrade through the latest patch release of each minor version in turn, running the whole procedure every time.
From `v0.1.3` to `v0.4.0`, for example: the latest `v0.1.x`, then the latest `v0.2.x`, then the latest `v0.3.x`, then `v0.4.0`.
`my db upgrade -check` stops a skip that would lose data and names the release to go through; nothing has changed at that point.

## Checking and rebuilding projections

Some tables are projections: the server derives them from the events and resources of their module (an account's restrictions from its
moderation actions, say). The worker checks every projection once a day and corrects any difference it finds. To check or correct one by hand:

```sh
my projection list              # each projection with its module and tables
my projection verify [<name>]   # derive every projection, or the one named, and print the rows that differ; changes nothing
my projection rebuild [<name>]  # derive them the same way and write the difference
```

- `<name>` is a projection or one of its tables.
- `verify` prints the rows the tables hold but the sources do not derive (`-`) and the rows the sources derive but the tables lack (`+`),
  and exits with `1` when any differ.
- `rebuild` writes at most 500 rows per transaction, so readers never see an emptied projection. Run it while the instance is quiet:
  a row that changes during the rebuild keeps its new value, and a later `verify` reports any difference left.

## Recreating (erasing every row)

Recreating means emptying the database and applying the schema again. **From `v0.1.0` on, recreating is for development and test instances only.
Never recreate a public instance that federates with other servers.**

Every row is lost and cannot be recovered. The account keys wrapped with the key encryption key (KEK) are lost too, so an account recreated under the
same host name gets a different key and is a different Actor to other servers, and its remote follow relationships do not come back.
A public instance that cannot apply a schema change [rolls back](#rolling-back) from its backup instead.

There is no `my` command that erases a database: remove it with the tools of the database, then apply the schema again.

### Docker Compose

```sh
cd deploy/docker
docker compose stop my                  # stop the server
docker run --rm -v my_data:/data busybox rm -f /data/my.db /data/my.db-wal /data/my.db-shm
docker compose up -d                    # the one-off services apply the schema to the empty database, then my starts
```

- Uploaded and processed media (`/data/blobs`) stay. To erase them with the database, remove the named volume with `docker compose down -v`, then run
  `docker compose up -d`. Without `-v` the volume is kept.
- To keep a copy first, stop the server and copy `my.db` (with its `-wal` and `-shm` files) from the volume.

### PostgreSQL

Drop the database and create it again, empty, with the same owner (or with the managed database's console), then run `my schema apply`:

```sh
# stop every server and job that uses the database first
dropdb --maintenance-db="postgres://postgres:...@db.example/postgres" my
createdb --maintenance-db="postgres://postgres:...@db.example/postgres" --owner=my my
my schema apply
```

### From the repository (development)

`scripts/task.sh db:reset` empties `DB_URL` without asking and then runs `scripts/task.sh db:apply`: use it on development databases only.
