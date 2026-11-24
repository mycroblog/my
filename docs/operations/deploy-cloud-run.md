# Deploying on Cloud Run

How to run `my` on Google Cloud with the OpenTofu module [`mycroblog-gcp`](../../deploy/opentofu/README.md): Cloud Run, Cloud Tasks, Cloud Scheduler, Cloud Storage, and Pub/Sub, with a PostgreSQL database on Neon, behind a global external Application Load Balancer. The service runs no instance while nobody uses it, except for a tick that wakes the server each minute. A server costs money: expect about $37 a month before storage, traffic, and open streams, and read [Costs](#costs) for what care can reduce.

What every OpenTofu deployment shares, the module's inputs, its secrets, the console, and upgrades among it, is in [Deploying with OpenTofu](deploy-opentofu.md). This page adds what Google Cloud needs.

## What it creates

One block of `mycroblog-gcp` creates all of this. Each row is a section of the module, and its default is what this guide deploys; [the module's README](../../deploy/opentofu/README.md#sections) lists the other kinds.

| Part | Section | Resources |
| --- | --- | --- |
| Server | `service`, `jobs` | A Cloud Run service with `role = all`, a migration job, a console job while you need it, a service account, and public access |
| Traffic | `ingress` | A global external Application Load Balancer with a certificate. A domain mapping or Cloudflare are the other kinds, and a media host on Cloudflare can sit beside any |
| Jobs | `queue` | A Cloud Tasks queue, a Cloud Scheduler job, and a service account that calls the server |
| Media | `media` | A private bucket |
| Real-time updates | `stream` | A topic and a custom role. The server creates and removes its own subscriptions |
| Database | `database` | A secret for its URL: you create the database by hand |
| Email | `mail` | A secret for the SMTP password: any SMTP service |
| Secrets | | A generated VAPID key, and the secrets that only you can fill, empty |
| KEK | `kek` | A generated secret mounted as a file. Cloud KMS is the other kind |
| Metrics, traces, errors | `observability` | The APIs of Cloud Monitoring, Cloud Trace, and Error Reporting, which read what Cloud Run sends them |

## Before you start

- What [the first deployment](deploy-opentofu.md#first-deployment) needs, with `gcloud` as the platform's CLI.
- A Google Cloud project with billing, used for this instance alone, and an account on it with the Owner role.

## First deployment

Do the first two steps here, then follow [the first deployment](deploy-opentofu.md#first-deployment) with
`examples/gcp-cloud-run-neon` and the notes of the third.

### 1. Enable the APIs

```sh
gcloud services enable --project="$PROJECT" run.googleapis.com cloudtasks.googleapis.com cloudscheduler.googleapis.com \
  pubsub.googleapis.com secretmanager.googleapis.com storage.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com cloudresourcemanager.googleapis.com logging.googleapis.com compute.googleapis.com \
  monitoring.googleapis.com
```

`compute.googleapis.com` is for the load balancer. With `kek = { kind = "kms" }`, enable `cloudkms.googleapis.com` too.

### 2. Create the database on Neon

Expect the Launch plan: with the default tick, the compute never stops. The Free plan works too, with the trade-offs under [Costs](#costs).

In the Neon console, create a project in a region near your Cloud Run region, and copy its connection strings. The module never creates it, so its password stays out of the OpenTofu state. A project has two connection strings, and the deployment uses each where it belongs:

- The **direct** one (the host without `-pooler`) is required, in the secret `my-database-url`. The migration job and the console always use it: `my schema apply` holds a session-level lock, which a pooler in transaction mode does not keep.
- The **pooled** one (the host with `-pooler`) is optional, for the service. Neon's pooler handles what the server needs: see [Through a connection pooler](database.md#through-a-connection-pooler). Set `database = { pooled = true }`, and the module creates a second secret for it.

Without a pooled string, the service uses the direct one too, at the database's connection limit, which `service.max_instances` bounds. To raise the pool from its default of 4 per instance, for the 80 requests a Cloud Run instance takes at once, add `pool_max_conns` to the string the service uses, for example `postgres://...-pooler...neon.tech/neondb?sslmode=require&pool_max_conns=8`. See [Waiting for a database connection](running.md#waiting-for-a-database-connection).

### 3. Deploy

Follow [the first deployment](deploy-opentofu.md#first-deployment). On Cloud Run:

- **Variables**: `mail_from`, `mail_smtp_host`, and `mail_smtp_username` configure [email](email.md); the port is 587, since Google Cloud blocks outbound port 25.
- **Secrets**: the KEK and the VAPID key are in `my-kek` and `my-vapid-key`, which `gcloud secrets versions access latest --secret=my-kek --project="$PROJECT"` copies. The KEK reaches the server as a mounted file, named by `MY_KEK_FILE`; the other secrets are environment variables, so deploy a new revision to make sure a new value takes effect.
- **Apply**: the migration job comes first with what it depends on, which includes the load balancer's address, since the server's settings name it. The service's startup probe waits for `/health/ready`, which answers `503` until the schema exists.
- **DNS**: `tofu output dns_records` lists one `A` record, of `host` to the load balancer's address. If your DNS provider has a proxy of its own, leave it off for this record. The certificate is Google-managed and becomes active only once DNS points at the address, which can take a while. Until then `https://{host}` fails with a TLS error, so federation and browsers cannot reach the server yet. Nothing else waits for it: Cloud Tasks and Cloud Scheduler call the service's default `run.app` URL, `https://<service>-<project number>.<region>.run.app`. **Do not disable the default URL**, or jobs stop. That URL refuses requests from the internet, and Cloud Run counts these two as internal only from the same project and on that URL, not on a custom domain. See [How traffic reaches the service](#how-traffic-reaches-the-service) for what else you can choose.
- **The first administrator**: the link stays in Cloud Logging, where anyone who can read the project's logs can use it.

## The console and upgrades on Cloud Run

[The console](deploy-opentofu.md#the-console) and [upgrades](deploy-opentofu.md#upgrading) run as executions of Cloud Run jobs from where you apply, through `gcloud`. They need credentials that may execute the jobs and read their logs: `roles/run.developer` and `roles/logging.viewer`, which Owner includes. An interrupted apply leaves the execution running on Cloud Run, and the next apply waits for it to end.

To [check the SQL first](deploy-opentofu.md#checking-the-sql-first):

```sh
tofu apply -target='module.mycroblog.module.app.google_cloud_run_v2_job.one_off["migrate"]'
gcloud run jobs execute my-migrate --project="$PROJECT" --region=asia-northeast1 --args=upgrade,-dry-run --wait
gcloud logging read 'resource.type="cloud_run_job" AND resource.labels.job_name="my-migrate"' --project="$PROJECT" \
  --freshness=1h --format='value(textPayload)' --order=asc
```

## Monitoring and alerts

The module adds an uptime check of `/health/live`, log-based metrics, and alert policies with the names of [the runbook](alerts.md), which notify the addresses of `monitoring.notification_emails` (`alert_emails` in the example) or the channels of `notification_channels`. The input has no default: name who is notified, or set `monitoring = null` to leave all of it out. It has never been applied, so try it once by making a check fail. Each run of the check wakes an instance, which adds to [the tick's trade-off](#costs): `uptime_period`, 15 minutes by default, is how often.

## Changing a backend

[Change the `kind` of its section](deploy-opentofu.md#changing-a-backend). For example, Cloud SQL in place of Neon:

```hcl
  database = { kind = "cloud-sql" }
```

For a store outside Google Cloud, set `media.kind` to `s3`, for any S3-compatible service with its `endpoint`, or to `r2`, with `cloudflare.account_id`. Both read an access key from the secrets that `secrets_to_fill` lists.

## When you need a network

This guide needs none: Neon, Cloud Tasks, Cloud Storage, and Pub/Sub are all reached over the internet with the service's own identity, so Cloud Run attaches to nothing and there is nothing to pay for or look after.

You need one to put Memorystore in the deployment, since it has only private addresses, or to give Cloud SQL a private address in place of its public one. `network = { kind = "vpc" }` creates one, and `network = { kind = "existing", ... }` attaches to a VPC you have, which you describe by `network`, `subnetwork`, `private_services_range`, and `memorystore_connection_policy`:

```hcl
  network = { kind = "vpc" }
  kv      = { kind = "memorystore" }
```

Only private ranges go through the network, so the service reaches the internet as it did, with no Cloud NAT. `nat = true` sends everything through Cloud NAT instead, for a static source address. Enable the Service Networking and Network Connectivity APIs first, and the Compute Engine API. A network adds to the cold start of an instance, and Memorystore bills while the server is idle.

## How traffic reaches the service

`ingress.kind` decides this. The default is the load balancer, which is meant for production. The domain mapping is for trying a deployment cheaply: set `ingress = { kind = "domain-mapping" }`.

| | `load-balancer` (the default) | `domain-mapping` |
| --- | --- | --- |
| Monthly fee | About $18 for the forwarding rules, plus a charge per GB. Check the [pricing page](https://cloud.google.com/vpc/network-pricing): this figure could not be confirmed there | None |
| Status | Google Cloud's recommended way | A Preview feature that Google Cloud does not recommend for production, because of latency |
| Regions | Any | `asia-northeast1`, `us-central1`, and a few others |
| DNS | An `A` record, from `tofu output dns_records` | The records that the same output lists |
| Certificate | Google-managed, active once DNS points at the address | Google-managed, up to 24 hours |
| Domain ownership | Nothing beyond DNS | You, and every other account that applies, must be a [verified owner](https://cloud.google.com/run/docs/mapping-custom-domains) before you apply |
| `run.app` URL | Refused from the internet, so nobody goes around the load balancer | Reachable by anyone: a domain mapping cannot add a header, so the origin stays open |
| `MY_TRUSTED_PROXIES` | `169.254.0.0/16` and the load balancer's address | `169.254.0.0/16` |

Cloud Tasks and Cloud Scheduler arrive through neither: they call the default `run.app` URL directly.

### Cloudflare

`ingress = { kind = "cloudflare" }` puts Cloudflare in front of `host`, with the cache bypassed for everything, and `ingress.media_host` adds a cached media host beside any kind. Both need `cloudflare = { zone_id = "..." }`, and the token goes in `CLOUDFLARE_API_TOKEN`. They have run only on mocked providers.

- **Your plan decides the origin.** Cloud Run routes by `Host`, and Cloudflare offers the Host header override on the Enterprise plan only. On any other plan the origin must accept your host itself, so the module creates a domain mapping of each host, with the trade-offs of the table above, and Cloudflare connects to it. Whether its certificate is issued and renewed behind the proxy is unverified, so start with `cloudflare.proxied = false`. On the Enterprise plan, `cloudflare.host_header_override = true` connects to the `run.app` URL instead, with no domain mapping.
- **A header closes the origin.** The module adds a request header transform rule, and the server refuses a request without the header with a `403`, apart from health checks and the jobs' own `POST`s, which check their tokens. So nobody goes around Cloudflare through the `run.app` URL, and a caller's own Cloudflare Worker cannot choose the client address the rate limits see.
- **The value lands in the state.** The module generates it and writes it to the secret `my-origin-secret`, but the Cloudflare provider has no write-only form of a rule's header value, so keep the state private. [The CDN guide](cdn.md#rotate-the-value) has how rotation works.
- **Nothing adds the header while a record is not proxied**, so the server refuses everything but its health checks: with `proxied = false` for a domain mapping's certificate, turn the proxy on once the certificate is active.
- **Turn off Bot Fight Mode** in the dashboard. It covers the whole zone and no rule can skip it, and a challenge on `/ap/`, `/.well-known/`, or `/api/` breaks federation, because other servers are not browsers. Jobs do not come through Cloudflare. The module adds no challenge and turns off Browser Integrity Check, but it does not manage Bot Fight Mode. Do not turn on "I'm Under Attack" for the same reason.
- **Email Address Obfuscation can stay on.** It rewrites email addresses in HTML into `[email protected]` and adds a script to restore them, which the server's Content-Security-Policy refuses. The server's own pages, such as the one that says where the confirmation email went, mark their content so that it leaves the addresses alone.
- **Turn on Always Use HTTPS** in the dashboard if you want `http://` redirected. It covers the whole zone, so the module leaves it to you.
- **Timeouts**: Cloudflare answers `524` when the origin sends nothing for 125 seconds. Jobs are not affected, since they call the `run.app` URL directly. Streams send a heartbeat every 30 seconds, under the 900-second idle limit. Whether it buffers streams on an ordinary proxied host is unverified.
- **Uploads**: 100 MB on Free and Pro, above Cloud Run's 32 MiB, which remains the limit.
- **Video**: Cloudflare's terms restrict serving video and a disproportionate amount of large files on Free, Pro, and Business, and it may redirect the content or take other action. Decide this before you cache video on the media host.

**A deleted file stays cached.** The media host follows the server's `Cache-Control`, so a deleted local file can stay visible for up to a year, and a remote one for `remote_media_cache_ttl_seconds`. The server does not purge it. When something must disappear, purge it by hand, under **Caching > Configuration > Custom Purge** in the dashboard, or:

```sh
curl https://api.cloudflare.com/client/v4/zones/$ZONE_ID/purge_cache \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -d '{"files": ["https://media.example.org/media/<path>"]}'
```

A proxy URL holds its `sig` in the query string, which is part of what is cached, so give the whole URL.

## Limits

- **Uploads**: Cloud Run refuses an HTTP/1 request above 32 MiB, and the server speaks HTTP/1.1. The module therefore sets `MY_MEDIA_MAX_AV_BYTES` to 30 MiB, against the default of 100 MiB, so that a larger upload gets the server's own answer.
- **Streams**: the request timeout, 15 minutes by default, ends an open stream, and the client connects again.
- **Client addresses**: the rate limits rely on `MY_TRUSTED_PROXIES`, which the module sets from the ingress. No document we have read says what Cloud Run adds to `X-Forwarded-For` for a direct request or what peer address a container sees, and the container contract says nothing about either, so `169.254.0.0/16` is an assumption. The external load balancer's documented form is `[<supplied>,]<client-ip>,<lb-ip>`. Check the addresses in the first request logs: a client IP that is a `169.254` address or a Google address means that the setting is wrong for your project.
- **Public access**: the service grants `roles/run.invoker` to `allUsers`, which an organization policy on domain-restricted sharing refuses. Such a project needs an exception.

## Costs

A server costs money. These figures are a guide to what to expect and to where care reduces it, not limits that the deployment is held to. They are approximate list prices for one region, from 2026-10 and 2026-11. Check them on the [Cloud Run](https://cloud.google.com/run/pricing) and [Neon](https://neon.com/pricing) pricing pages. Compute is free per billing account up to 180,000 vCPU-seconds and 360,000 GiB-seconds a month.

| Part | Idle | What adds to it |
| --- | --- | --- |
| Load balancer | About $18 a month, not confirmed on the pricing page | A charge per GB; see [How traffic reaches the service](#how-traffic-reaches-the-service) |
| Neon, on the Launch plan | About $19 a month, with the default tick | Storage at $0.35 per GB-month. `database.kind = cloud-sql` instead has a fixed cost of about $9.40 a month, for a smaller machine |
| Cloud Run | $0: min instances is 0 and CPU is billed only during requests | Each open stream keeps an instance billed; see below |
| Cloud Tasks | $0 | 1 million operations free, then $0.40 per million. A job takes about 2 |
| Cloud Scheduler | $0: 3 jobs free per billing account | $0.10 per job above that |
| Cloud Storage | About $0.02 per GiB a month | 5 GiB are free only in `us-east1`, `us-west1`, and `us-central1`. Media goes out through Cloud Run |
| Pub/Sub | $0: 10 GiB free | Only while streams are open |
| Secret Manager | Cents | A few free versions, then a few cents per version |

**The tick keeps Neon awake.** Each tick reads and writes the database, and Neon stops a compute only after 5 minutes without activity, so with the default `* * * * *` the compute never stops. That is 730 hours a month, or 182.5 CU-hours at 0.25 CU, which is the $19 above. The tick stays at every minute so that periodic jobs run on time, and a lost wake-up signal is recovered soon.

**An open stream keeps an instance billed.** Under request-based billing, an instance is billed while it handles any request, and the real-time updates are one long request. One client that keeps its stream open all month costs one instance around the clock: about $60 a month at 1 vCPU and 512 MiB, after the free allowance. More streams share the instance, up to `concurrency`. If streams stay open most of the time, compare with instance-based billing, or with a small VM and [Docker Compose](deploy-docker-compose.md).

### Where care reduces it

- **The domain mapping** in place of the load balancer takes the $18 away, with [the trade-offs above](#how-traffic-reaches-the-service). It suits trying a deployment.
- **Neon's Free plan** takes the $19 away, and a small instance can run on it, with these trade-offs:
  - It allows 100 CU-hours a month, which the default tick uses up after about 17 days. Stay inside it with a longer schedule, such as `queue = { tick_schedule = "*/10 * * * *" }`: at most half the time awake, about 91 CU-hours, before anything else wakes it. Periodic jobs then run up to 10 minutes late, and a lost wake-up signal is recovered later.
  - A federated server receives deliveries around the clock, which also wakes the database, so a busy instance outgrows it whatever the schedule.
  - It holds 1 GB per project and a 6-hour history to restore from, and its compute always stops after 5 minutes idle, so the first request after that waits for it.
