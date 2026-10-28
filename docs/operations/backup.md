# Backup and restore

What to back up, how to take each backup while the instance keeps running, how to restore an instance from them, and how to rehearse a restore
before you need one. Upgrades take a backup too; [Upgrading](database.md#upgrading) covers the steps around it.

## What to back up

| What | Where it lives | Back up | Lost without a backup |
| --- | --- | --- | --- |
| The database | the SQLite file (`database_url`) or the PostgreSQL database | [SQLite](#sqlite) or [PostgreSQL](#postgresql) | everything: accounts, posts, follows, settings |
| The media | the directory `blob_fs_dir` of the `fs` blob store, or the bucket `blob_s3_bucket` of the `s3` blob store | [Media](#media) or [Media in a bucket](#media-in-a-bucket) | the uploaded images, videos, and audio files, and the cached remote files |
| The KEK | wherever you keep `MY_KEK` or the file of `MY_KEK_FILE` (never in the database) | [The KEK](key-management.md#back-it-up), separately from the database | every actor's private key and with it the actor's identity in the fediverse, and the other secrets the database stores |
| The configuration | the environment variables, the configuration file (`MY_CONFIG_FILE`), the secrets (`MY_VAPID_PRIVATE_KEY`, mail and payment keys), `.env` with Compose | copy them with your deployment files | nothing in the data, but the instance cannot start the same way; changing `host` breaks every federated URL |

What does not need a backup:

- **The key-value store** (`kv`). With `kv = db` it is part of the database. With `kv = memory` or `kv = valkey` it holds caches, rate-limit counters,
  and short-lived state such as a sign-in waiting for its second factor. After a restore with an empty Valkey, the caches fill again,
  the rate limits start over, and a sign-in or a voice call in progress has to start again.
- **The job queue** (`queue`). With `queue = db` the jobs are part of the database and come back with it. With `queue = sqs` or `queue = cloudtasks`,
  the jobs waiting in the queue are not in the backup; each job only carries out work that the database already recorded
  (a delivery to another server, the processing of an upload), so losing them loses at most those deliveries and that processing.
- **Logs and metrics**, which the server does not read back.

The VAPID key of Web Push is a secret of the configuration. A new key stops push notifications to every existing subscription
([Web Push VAPID key](key-management.md#web-push-vapid-key)), so keep it with the configuration.

## How often and where

- **Database**: at least once a day, and before every upgrade. Keep several generations, for example 7 daily, 4 weekly, and 3 monthly backups,
  so that a mistake noticed late can still be undone. A managed PostgreSQL with point-in-time recovery covers the days its retention keeps.
- **Media**: together with the database, every time (see [The order of the database and the media](#the-order-of-the-database-and-the-media)).
- **KEK and configuration**: when they change. A KEK rotation is a change: back up the new KEK before you use it, and keep the previous one
  as long as `kek_previous` lists it.
- **Where**: away from the server and its disk, in another failure domain (another machine, another region, another provider or account).
  A backup on the server's own disk does not survive the loss of that disk.
- **Encryption**: the database backup holds email addresses, password hashes, and direct messages; encrypt it at rest
  (an encrypted bucket, or encrypt the file with `age` or `gpg` before upload). The secrets in it, such as the actors' private keys,
  are encrypted with the KEK, so **never keep the KEK in the same place as the database backups**: whoever gets both can read every secret.
- **Retention**: a backup keeps data that users deleted or that you erased on request. Delete old generations on a schedule that your privacy
  policy states.

## Backing up

The server can keep running during a backup. To take one that matches a point in time exactly, for example before an upgrade,
switch to the [maintenance mode](maintenance.md) first: in the maintenance mode `my serve` opens no database connection and writes nothing.

### SQLite

`my db backup <file>` writes a consistent copy of the SQLite database to a new file while the server keeps running.
The copy holds every write committed before the command started, including those still in the `-wal` file, and none after.
It is a single file, smaller than the original because free pages are left out. The command refuses a file that already exists,
and works only with `db = sqlite`.

```sh
my db backup /backup/my-$(date +%Y%m%d%H%M).db
```

With Docker Compose, run it in the server's container. The copy is written inside the container, so write it to the data volume
and move it out from there:

```sh
cd deploy/docker
docker compose exec -T my my db backup /data/backup.db
docker run --rm -v my_data:/data -v "$PWD":/backup busybox sh -c 'mv /data/backup.db /backup/my-$(date +%Y%m%d%H%M).db'
```

Do not copy the database file with `cp` or `tar` while the server runs: the file alone misses the writes still in the `-wal` file,
and a copy taken in the middle of a write can be corrupt. A plain copy is fine when no process has the database open
(the server stopped, or in the maintenance mode); then copy the `-wal` and `-shm` files with it.

### PostgreSQL

Use `pg_dump` in the custom format, which `pg_restore` reads. It reads one snapshot, so the server can keep running.

```sh
pg_dump --format=custom --file=my-$(date +%Y%m%d%H%M).dump "postgres://my:...@db.example/my"
```

Use the `pg_dump` of the same major version as the server or a newer one.

A managed PostgreSQL has its own backups, which are the better choice for a large database:

- **Cloud SQL** and **Amazon RDS** take automated daily backups and, with point-in-time recovery turned on, restore to any second
  within the retention period. Turn both on, set the retention to cover the generations you want, and take an on-demand backup
  (a Cloud SQL on-demand backup, an RDS snapshot) before every upgrade.
- Their restore creates a new instance (or overwrites one): point `database_url` at it afterwards.
- These backups stay inside the provider's account. Export a `pg_dump` now and then to somewhere outside it as well,
  so that losing the account does not lose the data.

### Media

The `fs` blob store keeps every file under `blob_fs_dir` (`/data/blobs` with Compose): the uploads in `objects/`, and files being written in `tmp/`.
Copy the directory, `tmp/` excepted, with any tool that copies files (`tar`, `rsync`, a snapshot of the volume or the disk).
A file is written under `tmp/` and then renamed into `objects/` in one step, so a copy taken while the server runs never holds half a file in `objects/`.

```sh
docker run --rm -v my_data:/data -v "$PWD":/backup busybox \
  tar czf /backup/blobs-$(date +%Y%m%d%H%M).tar.gz -C /data --exclude=blobs/tmp blobs
```

`rsync` to the backup host keeps the copy small after the first time, since media files never change once written.

### Media in a bucket

The `s3` blob store keeps every file as an object of the bucket `blob_s3_bucket`, named by its key: the processed media under `media/`,
the uploads as received under `uploads/`, and the cached remote files under `remote/`. The bucket stays private: the server reads the
files and serves them under `/media/`. An object appears only once it is written whole, so a copy taken while the server runs never holds
half a file. Protect the bucket in three layers, from the cheapest to the safest.

**Versioning.** Turn on the bucket's object versioning where the service offers it (Amazon S3, Google Cloud Storage, MinIO, and others).
A deleted or overwritten object then stays as a noncurrent version, and the bucket can go back to the moment of a database backup.

```sh
aws s3api put-bucket-versioning --bucket my-media --versioning-configuration Status=Enabled
```

**Lifecycle rules.** With versioning, every deleted file keeps its storage cost until a rule removes it. Add a rule that deletes noncurrent versions
after the age of your oldest database backup (35 days for the generations of [How often and where](#how-often-and-where) without the monthly ones),
so that every database backup you keep still finds its files and a file a user deleted does not stay longer than your privacy policy says.
Add a rule that aborts incomplete multipart uploads after a day: the server aborts a failed upload of a large file itself, but a process
that stops in the middle of one leaves its parts behind.

```json
{
  "Rules": [
    {
      "ID": "noncurrent-versions",
      "Status": "Enabled",
      "Filter": {},
      "NoncurrentVersionExpiration": { "NoncurrentDays": 35 },
      "AbortIncompleteMultipartUpload": { "DaysAfterInitiation": 1 }
    }
  ]
}
```

```sh
aws s3api put-bucket-lifecycle-configuration --bucket my-media --lifecycle-configuration file://lifecycle.json
```

**A copy in another account or region.** Versioning protects against a mistake, not against losing the bucket or the account that owns it,
and the server's own credentials can delete objects. Keep a copy that those credentials cannot reach:

- **Replication.** Amazon S3 replicates a versioned bucket to a bucket in another region or another account (Cross-Region or Same-Region Replication,
  with the destination owned by the other account). Turn versioning on for the destination too, and do not replicate deletions
  (leave delete marker replication off), so that a deletion on the instance does not reach the copy.
  Other services have their own replication, such as the dual-region buckets of Cloud Storage and the bucket replication of MinIO.
- **A scheduled copy.** Where there is no replication, or to copy to another provider, copy the bucket on the schedule of the database backup,
  right after it, with credentials that can only read the instance's bucket and only write the backup bucket:

  ```sh
  rclone copy --checksum source:my-media backup:my-media-backup
  ```

  `rclone copy` and `aws s3 sync` without `--delete` add new objects and never remove one from the copy. Files never change once written,
  so after the first run each copy transfers only the new ones. Delete old objects from the copy on the schedule of your retention policy,
  or turn on versioning and a lifecycle rule there as well.

Credentials stay out of the copy: the bucket holds no secret, but the configuration names the key in `blob_s3_access_key_id`
and `blob_s3_secret_access_key`, which belong with the configuration.

### The order of the database and the media

Back up **the database first, then the media**, and keep the two together as one generation. For a bucket, the copy or the replication
that runs after the database backup is the media backup; with versioning, the versions as of the time of the database backup are.

The database names the media files that it uses, and the media copy is taken a moment later. In that moment:

- New uploads can land in the media copy without a row in the database copy. They are files that nothing uses; they take space and do no harm.
- A file deleted after the database backup (a deleted post's media, an upload that was never attached) is in the database copy but not in the media copy.
  After a restore, that file answers `404`, and the post that showed it shows a missing image.

The other order is worse: every file uploaded between the media backup and the database backup would be missing after a restore,
including the media of new posts. Taking both in the maintenance mode avoids either gap.

### The KEK and the configuration

The KEK is not in the database. Back it up once, separately from the database backups, as [Managing the key-encryption key](key-management.md#back-it-up)
describes, and again after every rotation. Keep a copy of the configuration (the environment, the configuration file, `.env`, and the secret files)
with your deployment files, without the KEK.

## Restoring

Restore into an empty environment (a new machine, new volumes, or a new database), in this order: the database, the media (the directory or the bucket),
then the configuration with **the same KEK** (and the same `kek_previous`) that was in use when the backup was taken. Then start the server as usual.

### SQLite

Stop the server, put the backup file in place of the database file, and remove the old `-wal` and `-shm` files if any are left.
With Docker Compose, into the volume `my_data` (an existing one, or a new one on another machine):

```sh
cd deploy/docker
docker compose down          # keeps the volume; on a new machine, `docker compose up --no-start` creates it
docker run --rm -v my_data:/data -v "$PWD":/backup busybox sh -c '
  rm -rf /data/my.db /data/my.db-wal /data/my.db-shm /data/blobs &&
  cp /backup/my-202601010000.db /data/my.db &&
  tar xzf /backup/blobs-202601010000.tar.gz -C /data &&
  chown -R 65532:65532 /data'
docker compose up -d
```

The server image runs as the user `65532`, which must own the files of the volume (the `chown` above).

### PostgreSQL

Restore into a new, empty database prepared as for a new instance, but without applying the schema: the dump contains it.

1. Create the role and the database, and run the preparation SQL `schema/postgres.bootstrap.sql` on it
   (see [Using the schema image directly](database.md#using-the-schema-image-directly)).
2. Restore the dump. As a superuser, the restored tables keep their owner (the role must exist); as the database owner, add `--no-owner`.

   ```sh
   pg_restore --exit-on-error --dbname="postgres://postgres:...@db.example/my" my-202601010000.dump
   ```

3. With a managed database's own backup, restore it the provider's way instead, and point `database_url` at the restored instance.

To restore over an existing database instead (rolling back an upgrade), use `pg_restore --clean --if-exists`, as [Rolling back](database.md#rolling-back) shows.

### Media

Extract the media copy into `blob_fs_dir` before the server starts, owned by the user the server runs as.

### Media in a bucket

Restore the objects into an empty bucket before the server starts, then set `blob_s3_bucket` (and the endpoint, region, and credentials) to it.
The keys must stay the same: the database names each file by its key.

- From a copy in another bucket, copy it back:

  ```sh
  rclone copy --checksum backup:my-media-backup target:my-media-restored
  ```

- From the versions of the instance's own bucket, copy the objects as they were at the time of the database backup to a new bucket.
  `rclone` reads the versions of that moment with `--s3-version-at`:

  ```sh
  rclone copy --s3-version-at "2026-01-01 00:00:00" source:my-media target:my-media-restored
  ```

- From a replica, point `blob_s3_bucket` at the replica, or copy it back to a bucket in the original account.

The server's credentials need to read, write, and delete objects of the restored bucket (`s3:GetObject`, `s3:PutObject`, `s3:DeleteObject`,
and `s3:AbortMultipartUpload` on Amazon S3).

### Starting and checking

Start the server the usual way. With Docker Compose, `docker compose up -d` runs `my db upgrade -check`, the schema, and `my db upgrade` first;
on a backup of the same release they change nothing. Then check:

1. `/readyz` answers `200`, and the server logs no `unknown key-encryption key` error.
2. `my projection verify` reports no difference ([Checking and rebuilding projections](database.md#checking-and-rebuilding-projections)).
3. You can sign in, and the latest posts and their media show.
4. **The actors' keys have not changed**: the `publicKey.publicKeyPem` of an actor
   (`curl -H 'Accept: application/activity+json' https://<host>/ap/actors/<id>`) is the same as before the restore.
   Other servers verify the instance's signatures with that key, so an unchanged key means federation continues where it stopped.
5. A post, follow, or unfollow reaches another server (a test account on another instance sees it).

Writes made after the backup are lost: posts, follows, and uploads since then are gone on this instance, while other servers may still have
the activities they received. That cannot be repaired from here; restore from the newest backup you have.

### Without the right KEK

- **No KEK**: the server does not start. `my serve` and the other `my` commands that read the configuration stop at it with
  `The required setting MY_KEK is not set.` and `kek: required when key_manager is local`.
- **Another KEK**: the server starts and answers `/readyz`, but anything that reads a secret encrypted with the old KEK fails with
  `unknown key-encryption key` and the key ID of the old KEK (`local key manager: key "local:…" is not configured`): deliveries to other servers and signed fetches (the actors' private keys),
  sign-ins with two-factor authentication, webhook signatures, and the external login providers. Media proxy URLs issued before stop working too.
  Passwords and posts are not encrypted with the KEK, so sign-in with a password and reading posts still work, which can hide the problem at first:
  watch the log for the error.
- Find the KEK whose key ID the error names, set it as `kek` (or in `kek_previous`), and restart. There is no way to recover a secret without its KEK;
  if the KEK is lost for good, the actors' keys must be regenerated, and other servers see each actor as a new key for the same account.

## Recovery drill

A backup is only known to work once it has been restored. Rehearse a restore before the instance goes public, and again after a change to how
backups are taken (at least every few months):

1. Take a backup of the live instance as above: the database, then the media.
2. On another machine or in another Compose project, restore it with the KEK and the configuration from where you keep them
   (not copied from the live server), and with `host` unchanged so the actors' URLs match.
3. Do not let the copy federate: keep it off the internet, or switch on the maintenance mode once the checks below pass.
4. Go through [Starting and checking](#starting-and-checking): sign in, posts and media, and the `publicKey` of a few actors compared with the live instance.
5. Start it once without the KEK and once with a different one, and check that it fails as [Without the right KEK](#without-the-right-kek) describes.
6. Note how long each step took: that is how long a real restore keeps the instance down.

From the repository, `scripts/task.sh docker:restore-drill` runs the same drill on Docker Compose for SQLite and PostgreSQL:
it fills an instance, backs it up while it runs, removes every container and volume, restores into new ones,
and checks the sign-in, the posts and media, the follows, the actors' keys, a signed delivery to another server, and the failures without the KEK.
