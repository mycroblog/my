# Deploying on Google Cloud

How to run `my` on Google Cloud with the OpenTofu module `mycroblog-gcp`: a Cloud Run service, and a backend for each role
that you pick by the `kind` of its section. Every section defaults to the module's recommendation, which runs no instance
while nobody uses the server, except for a tick that wakes it each minute. A server costs money: the recommended setup with a
database on Neon comes to about $37 a month before storage and traffic, and [Costs](#costs) gives the figure of each choice.

What every OpenTofu deployment shares, the block and its sections, the secrets, the console, and upgrades among it, is in
[Deploying with OpenTofu](deploy-opentofu.md). This page adds what Google Cloud needs.

## What it creates

One block of `mycroblog-gcp` creates the following. The first kind of each row is the recommended one and the default;
[the modules' reference](../../deploy/opentofu/README.md#sections) lists every field.

| Role | Section | Recommended | Other kinds |
| --- | --- | --- | --- |
| Server | `service`, `jobs` | A Cloud Run service with `role = all`, a migration job, a console job while you need it, a service account, and public access | |
| Traffic | `ingress` | `load-balancer`: a global external Application Load Balancer with a Google-managed certificate | `domain-mapping`, `cloudflare`, and a cached media host on Cloudflare beside any |
| Database | `database` | `external`: a secret for the URL of a PostgreSQL you run, such as Neon | `cloud-sql`: an instance, its database, user, and password |
| Jobs | `queue` | `cloud-tasks`: a Cloud Tasks queue, the Cloud Scheduler job of the tick, and a service account that calls the server | `db`: the jobs in the database |
| Media | `media` | `gcs`: a private Cloud Storage bucket | `r2`, `s3` |
| Caches and rate limits | `kv` | `db`: in the database, with nothing to create | `memorystore`, `external` (Valkey) |
| Real-time updates | `stream` | `pubsub`: a topic, and a custom role with which the server manages its own subscriptions | `postgres`, `valkey` |
| Email | `mail` | Any SMTP service, with a secret for its password | |
| KEK | `kek` | `secret-manager`: a generated secret, mounted as a file | `kms`: a key in Cloud KMS |
| Network | `network` | `none`: nothing to attach | `vpc`, `existing` |
| Metrics, traces, errors | `observability` | Cloud Monitoring, Cloud Trace, and Error Reporting, which read what Cloud Run sends them. The module enables their APIs | `settings`, such as Sentry |
| Alerts | `monitoring` | An uptime check, log-based metrics, and alert policies: [Monitoring and alerts](#monitoring-and-alerts) | `null`, none |

The module also generates the VAPID key, and creates empty the secrets that only you can fill.

`examples/gcp-cloud-run-neon` is one way to start: the provider and one block with every recommended kind, which works with
any external database, Neon or not. Add to its block the sections you change.

## Before you start

- What [the first deployment](deploy-opentofu.md#first-deployment) needs, with `gcloud` as the platform's CLI, and
  `gcloud auth application-default login` for the provider.
- A Google Cloud project with billing, used for this instance alone, and an account on it with the Owner role. The custom
  role of `stream = pubsub` goes on the project, so it would reach the subscriptions of anything else there.

## First deployment

Do the first two steps here, then follow [the first deployment](deploy-opentofu.md#first-deployment) with the notes of the
third.

### 1. Enable the APIs

Every deployment needs these:

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com
```

Add the API of each choice that needs one:

| Choice | API |
| --- | --- |
| `ingress = load-balancer` | `compute.googleapis.com` |
| `database = cloud-sql` | `sqladmin.googleapis.com` |
| `kek = kms` | `cloudkms.googleapis.com` |
| `network = vpc` or `existing` | `compute.googleapis.com`, `servicenetworking.googleapis.com`, `networkconnectivity.googleapis.com` |
| `kv = memorystore` | `memorystore.googleapis.com`, `serviceconsumermanagement.googleapis.com`, and those of the network |

`translation` enables its own API, and so does `observability`. [Importing cloud usage costs](billing-import.md) lists what
`billing` needs.

### 2. Prepare the database

- **`external`**: create the database, and keep its direct connection URL and, if it has one, its pooled URL for
  `secrets_to_fill`: [The database](#the-database) says which goes where. The module never creates it, so its password stays
  out of the OpenTofu state.
- **`cloud-sql`**: nothing to do. The module creates the instance and writes its URL, with a generated password, to the secret.

### 3. Deploy

Follow [the first deployment](deploy-opentofu.md#first-deployment). On Google Cloud:

- **Variables**: the example's `mail_from`, `mail_smtp_host`, and `mail_smtp_username` configure [email](email.md). The port
  is 587, since Google Cloud blocks outbound port 25.
- **Secrets**: the KEK is `my-kek` and the VAPID key `my-vapid-key`, each starting with `name`, and
  `gcloud secrets versions access latest --secret=<id> --project="$PROJECT"` copies them. The KEK reaches the server as a
  mounted file, named by `MY_KEK_FILE`, and the other secrets as environment variables.
- **Apply**: the migration job comes first, with what it depends on, which includes the ingress, since the server's settings
  name it. The service's startup probe waits for `/health/ready`, which answers `503` until the schema exists.
- **DNS**: add the records of `tofu output dns_records`, as [How traffic reaches the service](#how-traffic-reaches-the-service)
  says for each kind. Until the certificate is active, `https://{host}` fails with a TLS error, so federation and browsers
  cannot reach the server yet. Nothing else waits for it: Cloud Tasks and Cloud Scheduler call the service's default URL,
  `https://<name>-<project number>.<region>.run.app`. **Do not disable the default URL**, or jobs stop. Cloud Run counts
  these two callers as internal only from the same project and on that URL, which is what lets a load balancer close it to
  the internet.
- **The first administrator**: the link stays in Cloud Logging, where anyone who can read the project's logs can use it.

## The console and upgrades

[The console](deploy-opentofu.md#the-console) and [upgrades](deploy-opentofu.md#upgrading) run executions of the Cloud Run
jobs `<name>-console` and `<name>-migrate` from where you apply, through `gcloud`. They need credentials that may execute
the jobs and read their logs, `roles/run.developer` and `roles/logging.viewer`, which Owner includes. An interrupted apply
leaves the execution running on Cloud Run, and the next apply waits for it to end.

To [check the SQL first](deploy-opentofu.md#checking-the-sql-first):

```sh
tofu apply -target='module.mycroblog.module.app.google_cloud_run_v2_job.one_off["migrate"]'
gcloud run jobs execute my-migrate --project="$PROJECT" --region="$REGION" --args=upgrade,-dry-run --wait
gcloud logging read 'resource.type="cloud_run_job" AND resource.labels.job_name="my-migrate"' --project="$PROJECT" \
  --freshness=1h --format='value(textPayload)' --order=asc
```

## The database

The module recommends `external`, which needs no network and bills nothing on Google Cloud: a database that stops while idle,
such as Neon, keeps a quiet server cheap. `cloud-sql` keeps everything in one project, at a fixed cost. Moving data between
them is [a change of backend](deploy-opentofu.md#changing-a-backend).

### An external PostgreSQL

`database = { kind = "external" }`, the default, takes any PostgreSQL that Cloud Run can reach: Neon, another hosted
PostgreSQL, or one you run, over the internet or on a private address through [a network](#networks). The module creates the
secret `my-database-url`, for the **direct** URL, which the migration job and the console always use: `my schema apply` holds
a session-level lock, which a pooler in transaction mode does not keep.

`database = { kind = "external", pooled = true }` adds the secret `my-database-url-pooled`, for a **pooled** URL, which the
service then uses. A pooler suits the server as [Through a connection pooler](database.md#through-a-connection-pooler)
says. Without one, the service uses the direct URL too, so `service.max_instances` times the pool of each instance must stay
under the database's connection limit. The pool is 4 connections per instance by default, for the 80 requests an instance
takes at once: raise it with `pool_max_conns` in the URL the service uses, such as `...?sslmode=require&pool_max_conns=8`
([Waiting for a database connection](running.md#waiting-for-a-database-connection)).

**On Neon**, create a project in a region near your Cloud Run region. Its direct connection string has the plain host, and its
pooled one the host with `-pooler`, which is Neon's pooler and suits the server: set `pooled = true`. Expect the Launch plan,
since the default tick keeps the compute awake; the Free plan works with [the trade-offs under Costs](#where-care-reduces-it).

### Cloud SQL

`database = { kind = "cloud-sql" }` creates a PostgreSQL 18 instance, `db-f1-micro` by default, with daily backups and
point-in-time recovery kept for 7 days, its database and user, and a generated password that reaches neither the plan nor the
state. `tier`, `disk_size_gb`, `retained_backups`, and `deletion_protection` change it. It has never been applied: treat the
first deployment as its test.

- **Without a network**, Cloud Run reaches it through the Cloud SQL connector's socket, and the instance's public address
  admits nothing else.
- **With a network**, it has a private address alone, and a `pg_dump` must run from inside the network.
- **It has no pooler**: `pooled` is refused, and `service.max_instances` and `pool_max_conns` bound the connections. Keep the
  pool small on `db-f1-micro`, which has 0.6 GB of memory.
- **It never scales to zero**, so it bills whatever the traffic, and Google's SLA does not cover a shared-core tier.

## The other backends

Each has its recommendation as its default, and [a change of `kind`](deploy-opentofu.md#changing-a-backend) moves to another.

- **Jobs**: `cloud-tasks` pushes each job to the service, which can scale to zero. `queue.tick_schedule`, every minute by
  default, wakes the server and the database for the periodic jobs. `db` polls the database from a process that stays up, so
  it needs `service.min_instances` of 1 or more and `service.cpu_always_allocated = true`.
- **Media**: `gcs` creates a private bucket, `media.bucket_name`, in the region. `r2` creates an R2 bucket in
  `cloudflare.account_id`, and `s3` uses any S3-compatible store at its `endpoint`; both read an access key from the secrets
  that `secrets_to_fill` lists.
- **Caches and rate limits**: `db` keeps them in the database. `memorystore` creates Memorystore for Valkey, which needs a
  network and has no password: anything on the network can reach it. `external` reads the URL of a Valkey you run from a secret.
- **Real-time updates**: `pubsub` reaches every instance. `postgres` uses `LISTEN` and `NOTIFY` over the database's direct
  connection, and `valkey` the Valkey of `kv`.
- **The KEK**: `secret-manager` generates it once, for you to copy. `kms` keeps it in Cloud KMS, which it never leaves, so a
  leaked database dump and a leaked configuration together still expose no secret. Changing it on an instance with data is
  [a move](deploy-opentofu.md#the-kek).

## Networks

The recommended setup needs none: an external database, Cloud Tasks, Cloud Storage, and Pub/Sub are all reached over the
internet with the service's own identity, so Cloud Run attaches to nothing.

You need one for Memorystore, which has only private addresses, to give Cloud SQL a private address, or to reach a database of
yours on one. `network = { kind = "vpc" }` creates a VPC, its subnets, and private services access, and
`network = { kind = "existing", ... }` attaches to a VPC you have, which you describe by `network`, `subnetwork`,
`private_services_range`, and `memorystore_connection_policy`:

```hcl
  network = { kind = "vpc" }
  kv      = { kind = "memorystore" }
```

The service and the jobs attach through Direct VPC egress, and only private ranges go through the network, so the internet
keeps its own path, with no Cloud NAT. With `vpc`, `nat = true` sends everything through Cloud NAT instead, for a static
source address. A network can delay the start of an instance by a minute or more, which a server that scales to zero meets
often. The VPC the module creates has never been applied.

## How traffic reaches the service

`ingress.kind` decides this. The module recommends the load balancer, for production. The domain mapping suits trying a
deployment cheaply.

| | `load-balancer` | `domain-mapping` |
| --- | --- | --- |
| Monthly fee | About $18 for the forwarding rules, plus a charge per GB. Check the [pricing page](https://cloud.google.com/vpc/network-pricing): this figure could not be confirmed there | None |
| Status | Google Cloud's recommended way | A Preview feature that Google Cloud does not recommend for production, because of latency |
| Regions | Any | `asia-northeast1`, `us-central1`, and a few others |
| DNS | One `A` record of `host` to its address. Leave any proxy of your DNS provider off for it | The records that `dns_records` lists |
| Certificate | Google-managed, active once DNS points at the address, which can take a while | Google-managed, up to 24 hours |
| Domain ownership | Nothing beyond DNS | Whoever applies must be a [verified owner](https://cloud.google.com/run/docs/mapping-custom-domains) of the domain first |
| `run.app` URL | Refused from the internet, so nobody goes around the load balancer | Reachable by anyone: a domain mapping cannot add a header, so the origin stays open |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` and the load balancer's address | `169.254.0.0/16` |

Cloud Tasks and Cloud Scheduler arrive through neither: they call the default `run.app` URL directly.

### Cloudflare

`ingress = { kind = "cloudflare" }` puts Cloudflare in front of `host`, with the cache bypassed for everything, and
`ingress.media_host` adds a cached media host beside any kind. Both need `cloudflare = { zone_id = "..." }`, the provider
reads its token from `CLOUDFLARE_API_TOKEN`, and the module creates the records, so `dns_records` is empty. They have run only
on mocked providers.

- **Your plan decides the origin.** Cloud Run routes by `Host`, and Cloudflare overrides the Host header on the Enterprise
  plan only. On any other plan the module creates a domain mapping of each host, with the trade-offs of the table above, and
  Cloudflare connects to it. Whether its certificate is issued and renewed behind the proxy is unverified, so start with
  `cloudflare.proxied = false`. On the Enterprise plan, `cloudflare.host_header_override = true` connects to the `run.app`
  URL instead, with no domain mapping.
- **A header closes the origin.** A request header transform rule adds a value that the module generates, and the server
  refuses a request without it with a `403`, apart from health checks and the jobs' own `POST`s, which check their tokens.
  So nobody goes around Cloudflare through the `run.app` URL, and nobody can choose the client address the rate limits see.
- **Nothing adds the header while a record is not proxied**, so the server then refuses everything but its health checks:
  with `proxied = false` for a domain mapping's certificate, turn the proxy on once the certificate is active.
- **The value lands in the state**, as [Secrets](deploy-opentofu.md#secrets) says, and in the secret `my-origin-secret`.
  [The CDN guide](cdn.md#rotate-the-value) has how to rotate it.
- **Turn off Bot Fight Mode** in the dashboard, and do not turn on "I'm Under Attack": a challenge on `/ap/`, `/.well-known/`,
  or `/api/` breaks federation, since other servers are not browsers. Both cover the whole zone, which no rule can exempt.
  The module adds no challenge and turns off Browser Integrity Check.
- **Email Address Obfuscation can stay on.** The server's own pages mark the content whose addresses it must leave alone.
- **Turn on Always Use HTTPS** in the dashboard if you want `http://` redirected. It covers the whole zone, so the module
  leaves it to you.
- **Timeouts**: Cloudflare answers `524` when the origin sends nothing for 125 seconds. Jobs are not affected, since they do
  not come through Cloudflare. Streams send a heartbeat every 30 seconds, under the 900-second idle limit. Whether Cloudflare
  buffers streams on a proxied host is unverified.
- **Video**: Cloudflare's terms restrict serving video and a disproportionate amount of large files on Free, Pro, and
  Business, and it may act on the content. Decide this before you cache video on the media host.

**A deleted file stays cached.** The media host follows the server's `Cache-Control`, so a deleted local file can stay visible
for up to a year, and a remote one for `remote_media_cache_ttl_seconds`. The server does not purge it. When something must
disappear, purge it by hand, under **Caching > Configuration > Custom Purge** in the dashboard, or:

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

A proxy URL holds its `sig` in the query string, which is part of what is cached, so give the whole URL.

## Monitoring and alerts

`monitoring` adds an uptime check of `/health/live`, log-based metrics, and alert policies with the names of
[the runbook](alerts.md), which notify the addresses of `notification_emails` (`alert_emails` in the example) or the channels
of `notification_channels`. The section has no default: name who is notified, or set `monitoring = null` to leave all of it
out. It has never been applied, so try it once by making a check fail. Each run of the check wakes an instance:
`uptime_period`, 15 minutes by default, is how often.

## Limits

- **Uploads**: Cloud Run refuses an HTTP/1 request above 32 MiB, and the server speaks HTTP/1.1, so the module sets
  `MY_MEDIA_MAX_AV_BYTES` to 30 MiB, against the default of 100 MiB, for a larger upload to get the server's own answer.
  Cloudflare's own limit, 100 MB on Free and Pro, is above it.
- **Streams**: `service.request_timeout_seconds`, 15 minutes by default, ends an open stream, and the client connects again.
- **Database connections**: Google limits a Cloud Run instance to 100 connections to Cloud SQL.
- **Client addresses**: the rate limits rely on `MY_TRUSTED_PROXIES`, which the module sets from the ingress. No document we
  have read says what Cloud Run adds to `X-Forwarded-For` for a direct request or what peer address a container sees, so
  `169.254.0.0/16` is an assumption. The load balancer's documented form is `[<supplied>,]<client-ip>,<lb-ip>`. Check the
  addresses in the first request logs: a client address in `169.254` or of Google means the setting is wrong for your project.
- **Public access**: the service grants `roles/run.invoker` to `allUsers`, which an organization policy on domain-restricted
  sharing refuses. Such a project needs an exception.
- **Names that stay taken**: Cloud SQL and Cloud Tasks refuse the name of a deleted instance or queue for about a week, and a
  deleted custom role keeps its ID for a while, so a deployment made again after a `destroy` needs another `name`.

## Costs

These figures are a guide to what to expect and to where care reduces it, not limits that the deployment is held to. They
are approximate list prices for one region, from 2026-10 and 2026-11: check them on the pricing pages of
[Cloud Run](https://cloud.google.com/run/pricing), [Cloud SQL](https://cloud.google.com/sql/pricing), and
[Neon](https://neon.com/pricing). Cloud Run's compute is free per billing account up to 180,000 vCPU-seconds and 360,000
GiB-seconds a month.

What every deployment pays, about $0 while idle:

| Part | Idle | What adds to it |
| --- | --- | --- |
| Cloud Run | $0: no instance runs, and CPU is billed only during requests | Each open stream keeps an instance billed; see below |
| Cloud Tasks | $0 | 1 million operations free, then $0.40 per million. A job takes about 2 |
| Cloud Scheduler | $0: the tick is one job, and 3 are free per billing account | $0.10 per job above that |
| Pub/Sub | $0: 10 GiB free | Only while streams are open |
| Secret Manager | Cents | A few free versions, then a few cents per version |
| Cloud Storage | About $0.02 per GiB a month | 5 GiB are free only in `us-east1`, `us-west1`, and `us-central1`. Media goes out through Cloud Run |
| Monitoring | $0 within the free allowances | |

What each choice adds:

| Section | Choice | Per month |
| --- | --- | --- |
| `ingress` | `load-balancer` | About $18, not confirmed on the pricing page, plus a charge per GB |
| | `domain-mapping` | $0 |
| | `cloudflare` | Your Cloudflare plan, and the origin: $0 with a domain mapping |
| `database` | `external` on Neon's Launch plan | About $19 with the default tick, plus $0.35 per GB-month |
| | `external` on Neon's Free plan | $0, within its limits: [below](#where-care-reduces-it) |
| | `external` elsewhere | What that database costs |
| | `cloud-sql` | About $9.40 for `db-f1-micro` and 10 GiB in `us-central1`, plus the backups, whatever the traffic |
| `queue` | `db` | An instance billed around the clock, with its CPU always allocated |
| `kv` | `memorystore` | About $23 for the smallest node, whatever the traffic |
| `kek` | `kms` | Cents, for the key and its operations |
| `network` | `vpc` with `nat = true` | Cloud NAT, by the hour and by the GB |

The recommended setup on Neon's Launch plan, the load balancer and Neon, comes to about $37 a month; with Cloud SQL, to
about $27.

**The tick keeps Neon awake.** Each tick reads and writes the database, and Neon stops a compute only after 5 minutes without
activity, so with the default `* * * * *` the compute never stops. That is 730 hours a month, or 182.5 CU-hours at 0.25 CU,
which is the $19 above. The tick stays at every minute so that periodic jobs run on time, and a lost wake-up signal is
recovered soon. Cloud SQL bills the same whatever the tick.

**An open stream keeps an instance billed.** Under request-based billing, an instance is billed while it handles any request,
and the real-time updates are one long request. One client that keeps its stream open all month costs one instance around the
clock: about $60 a month at 1 vCPU and 512 MiB, after the free allowance. More streams share the instance, up to
`service.concurrency`. If streams stay open most of the time, compare with instance-based billing
(`service.cpu_always_allocated`), or with a small VM and [Docker Compose](deploy-docker-compose.md).

### Where care reduces it

- **The domain mapping** in place of the load balancer takes the $18 away, with
  [its trade-offs](#how-traffic-reaches-the-service). It suits trying a deployment.
- **Neon's Free plan** takes the $19 away, and a small instance can run on it, with these trade-offs:
  - It allows 100 CU-hours a month, which the default tick uses up after about 17 days. Stay inside it with a longer
    schedule, such as `queue = { tick_schedule = "*/10 * * * *" }`: at most half the time awake, about 91 CU-hours, before
    anything else wakes it. Periodic jobs then run up to 10 minutes late, and a lost wake-up signal is recovered later.
  - A federated server receives deliveries around the clock, which also wakes the database, so a busy instance outgrows it
    whatever the schedule.
  - It holds 1 GB per project and a 6-hour history to restore from, and its compute always stops after 5 minutes idle, so
    the first request after that waits for it.
