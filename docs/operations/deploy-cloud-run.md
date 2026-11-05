# Deploying on Cloud Run

How to run `my` on Google Cloud with the OpenTofu modules in [`deploy/terraform`](../../deploy/terraform/README.md): Cloud Run, Cloud Tasks, Cloud Scheduler, Cloud Storage, and Pub/Sub, with a PostgreSQL database on Neon. Nothing runs while nobody uses it, except a tick that wakes the server each minute. Read [Costs](#costs) first: the tick and open streams decide what it costs.

The modules also work with Terraform 1.11 or later: use `terraform` where this guide says `tofu`. The lock file in the example is for OpenTofu, so `terraform init` adds its own entries to it.

## What it creates

| Part | Module | Resources |
| --- | --- | --- |
| Server | `app-gcp-cloud-run` | A Cloud Run service with `role = all`, a migration job, a console job while you need it, a service account, public access, and a domain mapping |
| Jobs | `queue-gcp-cloud-tasks` | A Cloud Tasks queue, a Cloud Scheduler job, and a service account that calls the server |
| Media | `blob-gcp-gcs` | A private bucket |
| Real-time updates | `stream-gcp-pubsub` | A topic and a custom role. The server creates and removes its own subscriptions |
| Database | `db-external` | Nothing: you create the database by hand |
| Email | `mail-smtp` | Nothing: any SMTP service |
| Secrets | `secrets-gcp-secret-manager` | Secrets of the database URL and the SMTP password, which you fill, and a generated VAPID key |
| KEK | `kek-gcp-secret-manager` or `kek-gcp-kms` | A generated secret mounted as a file, or a Cloud KMS key |

Secret values never reach the OpenTofu plan or state: you add the ones that come from elsewhere yourself, and the deployment generates the KEK and the VAPID key through write-only attributes.

## Before you start

- `tofu` 1.11 or later, `gcloud`, and `jq`.
- A Google Cloud project with billing, used for this instance alone, and an account on it with the Owner role.
- A domain for [`host`](../reference/configuration.md#host). It cannot change later: read [Settings you cannot change later](production-checklist.md#settings-you-cannot-change-later).
- A version of `my` from the [releases](running.md#get-the-image). There is no `latest` tag.

## First deployment

### 1. Enable the APIs

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com
```

### 2. Create the database on Neon

In the Neon console, create a project in a region near your Cloud Run region, and copy its connection string.

- Use the **direct** connection string (the host without `-pooler`). `my schema apply` holds a session-level lock, which does not work through a pooler in transaction mode.
- Add `pool_max_conns` for the concurrency you expect, for example `postgres://...neon.tech/neondb?sslmode=require&pool_max_conns=8`. The default is 4 per instance, and a Cloud Run instance takes 80 requests at once: see [Waiting for a database connection](running.md#waiting-for-a-database-connection). All instances together must stay under the database's limit, so `max_instances` of the app module matters too.

### 3. Set the variables

```sh
cd deploy/terraform/examples/gcp-cloud-run-neon
cp terraform.tfvars.example terraform.tfvars
```

Fill in `terraform.tfvars`. `mail_from`, `mail_smtp_host`, `mail_smtp_port` (587 by default; Google Cloud blocks outbound port 25), and `mail_smtp_username` configure [email](email.md). `env` and `secret_env` take any other setting from the [configuration reference](../reference/configuration.md), by value and by secret ID. A setting named in two places fails the plan.

To send no email, remove the `mail` module and its secret from `main.tf`, and `mail` stays `none`. Read [what that means for sign-ups](production-checklist.md#email) first.

Decide on [`account_domain`](../reference/configuration.md#account_domain) now, as `MY_ACCOUNT_DOMAIN` in `env`, if handles use another domain.

**The state** stays in this directory by default. It holds no secret values, but it is the only record of what was created: keep it in a bucket with a `backend "gcs"` block if more than one person runs this.

### 4. Create the secrets and add their values

This creates the secrets, and generates the KEK and the VAPID key:

```sh
tofu init
tofu apply -target=module.secrets -target=module.kek
```

**Copy the KEK now**, and keep the copy apart from the database, for example in a password manager. Without it a [backup](backup.md) cannot be restored, and the accounts' keys are lost. Do the same with the VAPID key, whose loss ends every Web Push subscription:

```sh
gcloud secrets versions access latest --secret=my-kek --project="$PROJECT"
gcloud secrets versions access latest --secret=my-vapid-key --project="$PROJECT"
```

Then add the values that come from elsewhere. An instance whose secret cannot be read does not start, so add them before the app:

```sh
printf '%s' 'postgres://...' | gcloud secrets versions add my-database-url --project="$PROJECT" --data-file=-
printf '%s' '<the SMTP password>' | gcloud secrets versions add my-smtp-password --project="$PROJECT" --data-file=-
```

The KEK reaches the server as a mounted file, named by `MY_KEK_FILE`. The others are environment variables resolved when an instance starts, so a new version takes effect on the next instance: deploy a new revision to make sure.

**OpenTofu generates the KEK once and never again.** The secret and its version have `prevent_destroy`, and the version number that would write a new value is fixed in the module: do not change it, because a new random KEK makes everything the old one encrypted unreadable. To rotate, follow [Rotate the KEK](key-management.md#rotate-the-kek), adding the new value with `gcloud secrets versions add`. OpenTofu does not track what you add there.

To keep the KEK in Cloud KMS from the start, replace the `kek` block in `main.tf` as its comment shows, and enable `cloudkms.googleapis.com`. To bring a KEK or a VAPID key of your own, set `create = false` on the `kek` module, or remove the key from `generate_p256_keys`, before this step. A secret made elsewhere needs only its ID: pass it to `secret_env` of the app module, or to the module that uses it.

### 5. Create what the migration needs

The service's startup probe waits for `/readyz`, which answers `503` until the schema exists, so the schema has to come first. Create the migration job and what it depends on:

```sh
tofu apply -target='module.app.google_cloud_run_v2_job.one_off["migrate"]'
```

### 6. Apply the schema

From the example's directory:

```sh
../../upgrade.sh --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.1.0 --migrate-only
```

It runs `my config check`, `my db upgrade -check`, `my schema apply`, and `my db upgrade`, each as an execution of the job, and stops at the first that fails. The first command that uses the database records `host` and `account_domain` in it.

### 7. Create the rest

```sh
tofu apply
```

### 8. Point the domain at the service

`tofu output dns_records` lists the records to add at your DNS provider. The domain mapping has limits:

- It is a Preview feature that Google Cloud does not recommend for production, because of latency, and it exists in some regions only (`asia-northeast1`, `us-central1`, and a few others).
- You must be a [verified owner of the domain](https://cloud.google.com/run/docs/mapping-custom-domains) before you apply, and so must every other account that applies.
- The certificate can take up to 24 hours.

The alternative is a global external Application Load Balancer, which costs a monthly fee. Set `domain_mapping = false` and add its forwarding rule's address to `trusted_proxies` of the app module. Whatever you choose, the server must answer at `https://{host}`, because Cloud Tasks and Cloud Scheduler call that address.

Until the domain works, tasks and ticks fail and are tried again, which costs next to nothing.

### 9. Create the first administrator

Cloud Run jobs have no terminal and no standard input, so `my admin create` prints a link that sets the password instead of reading one. Turn the [console](#the-console) on, and run:

```sh
tofu apply -var console_enabled=true
tofu output -json console | ../../console.sh admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

Open the link right away and set the password. It works once, for one hour. **It stays in Cloud Logging until then**, where anyone who can read the project's logs can use it. If it expires, run `my admin account password-reset-link @alice` through the console. Then turn on two-factor authentication.

### 10. Go through the checklist

Go through the [production checklist](production-checklist.md), starting with [email](production-checklist.md#email): SPF, DKIM, and DMARC on the domain of `mail_from`.

## The console

The console runs one `my` command with the image, settings, and secrets of the service. It exists only while `console_enabled` is on, so turn it off when you finish:

```sh
tofu apply -var console_enabled=true
tofu output -json console | ../../console.sh instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` waits for the command, prints its output and exit code, and exits with that code. It needs `roles/logging.viewer`, because a job's output goes to Cloud Logging. It uses the image the service runs, whatever `image` says.

The command has no standard input and no terminal, so a command that reads a password or a file from them cannot run this way. What it prints stays in the logs, so do not run a command whose output you would not write there.

## Upgrading

Read [Upgrading](database.md#upgrading) and the release notes first, and go one minor version at a time.

```sh
# 1. Back up the database while nothing writes. Use pg_dump or a Neon branch (see Backup and restore).
gcloud run services update my --project="$PROJECT" --region=asia-northeast1 --update-env-vars=MY_MAINTENANCE=true
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Read the SQL it would run. This changes nothing but the image of the migration job.
../../upgrade.sh --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.2.0 --plan

# 3. Upgrade.
../../upgrade.sh --project "$PROJECT" --region asia-northeast1 --image ghcr.io/mycroblog/my:0.2.0 --maintenance
```

The script does this, and stops at the first step that fails:

1. Switches the service to the [maintenance mode](maintenance.md) with `--maintenance`.
2. Gives the migration job the new image.
3. Runs `my config check`, `my db upgrade -check`, `my schema apply`, and `my db upgrade`, each waiting for the one before.
4. Deploys the new revision, without the maintenance mode. It takes traffic only when its startup probe passes on `/readyz`.

Without `--maintenance`, the old revision keeps serving during steps 2 and 3, which [is not guaranteed to work](database.md#the-server-during-an-upgrade). Add `--allow-destructive` when the release notes call for it. Pin a digest with `--image ghcr.io/mycroblog/my:0.2.0@sha256:...`, which the [production checklist](production-checklist.md#operations) recommends.

Then set `image` in `terraform.tfvars` to the same value. `tofu apply` never changes the image of a service or job that exists, so a forgotten `image` does no harm, but the value is what a new job or a recreated service would start from.

**When a step fails**, the service stays on its revision, in the maintenance mode if you asked for it. Fix the cause and run the script again: the steps are safe to repeat. To leave the maintenance mode without upgrading, when `my schema apply` has not run:

```sh
gcloud run services update my --project="$PROJECT" --region=asia-northeast1 --remove-env-vars=MY_MAINTENANCE
```

To go back after the schema changed, restore the backup: see [Rolling back](database.md#rolling-back).

## Changing a backend

Replace the module's block in `main.tf`. The app module takes whatever the block returns, so nothing else changes. For example, a database of your own:

```hcl
module "db" {
  source = "../../modules/db-external"

  url_secret_id = "another-database-url"
}
```

For a store outside Google Cloud, such as an S3-compatible service, leave the `blob` module out of `backends` and pass its settings through `env` and `secret_env`.

## Changing the KEK backend

Replacing the `kek` block is one block in `main.tf`, but on an instance with data it is not free. The database holds secrets wrapped with the current key, so an instance that loses it can no longer read them, and every account's keys are among them.

Follow [Move between the local KEK and a KMS](key-management.md#move-between-the-local-kek-and-a-kms). From the secret to Cloud KMS, keep the current KEK as `kek_previous` while the secrets move, by passing its secret through the app module, with `secret_files = { MY_KEK_PREVIOUS_FILE = "my-kek" }`; run `my kek rewrap` through [the console](#the-console), then remove it when `my kek status` no longer counts `local:` keys. Keep the KEK for as long as you keep database backups from before the rewrap.

On a new instance with no data, nothing needs to move.

## Limits

- **Uploads**: Cloud Run refuses an HTTP/1 request above 32 MiB, and the server speaks HTTP/1.1. The example therefore sets `MY_MEDIA_MAX_AV_BYTES` to 30 MiB, against the default of 100 MiB, so that a larger upload gets the server's own answer.
- **Streams**: the request timeout, 15 minutes by default, ends an open stream, and the client connects again.
- **Client addresses**: the rate limits rely on `trusted_proxies`, `169.254.0.0/16` by default. Check the addresses in the first request logs: a client IP that shows a `169.254` address means that the setting is wrong for your project.
- **Public access**: the service grants `roles/run.invoker` to `allUsers`, which an organization policy on domain-restricted sharing refuses. Such a project needs an exception.

## Costs

The figures are approximate list prices for one region, from 2026-10 and 2026-11. Check them on the [Cloud Run](https://cloud.google.com/run/pricing) and [Neon](https://neon.com/pricing) pricing pages. Compute is free per billing account up to 180,000 vCPU-seconds and 360,000 GiB-seconds a month.

| Part | Idle | What adds to it |
| --- | --- | --- |
| Cloud Run | $0: min instances is 0 and CPU is billed only during requests | Each open stream keeps an instance billed; see below |
| Cloud Tasks | $0 | 1 million operations free, then $0.40 per million. A job takes about 2 |
| Cloud Scheduler | $0: 3 jobs free per billing account | $0.10 per job above that |
| Cloud Storage | About $0.02 per GiB a month | 5 GiB are free only in `us-east1`, `us-west1`, and `us-central1`. Media goes out through Cloud Run |
| Pub/Sub | $0: 10 GiB free | Only while streams are open |
| Secret Manager | Cents | A few free versions, then a few cents per version |
| Neon | See below | Storage: 1 GB on the Free plan, then $0.35 per GB-month |

Two things decide the real cost:

**The tick keeps Neon awake.** Each tick reads and writes the database, and Neon stops a compute only after 5 minutes without activity, so with the default `* * * * *` the compute never stops. That is 730 hours a month, or 182.5 CU-hours at 0.25 CU, above the 100 CU-hours of Neon's Free plan: the allowance runs out after about 17 days. On the Launch plan it costs about $19 a month.

To stay inside the Free plan, set a longer schedule, such as `tick_schedule = "*/10 * * * *"`: at most half the time awake, about 91 CU-hours, before anything else wakes it. The cost is that periodic jobs run up to 10 minutes late, and that a lost wake-up signal is recovered later. A federated server receives deliveries around the clock, which also wakes the database, so a busy instance needs a paid plan either way.

**An open stream keeps an instance billed.** Under request-based billing, an instance is billed while it handles any request, and the real-time updates are one long request. One client that keeps its stream open all month costs one instance around the clock: about $60 a month at 1 vCPU and 512 MiB, after the free allowance. More streams share the instance, up to `concurrency`. If streams stay open most of the time, compare with instance-based billing, or with a small VM and [Docker Compose](../../README.md#quick-start-docker-compose).
