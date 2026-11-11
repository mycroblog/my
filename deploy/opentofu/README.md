# OpenTofu modules

`mycroblog-gcp` deploys `my` on Google Cloud in one module block: a Cloud Run service and a backend for each role, which you
pick by the `kind` of a section and which defaults to the recommended setup. It is written for [OpenTofu](https://opentofu.org)
and needs version 1.11 or later. Terraform is not supported. To deploy, follow
[Deploying on Cloud Run](../../docs/operations/deploy-cloud-run.md).

```text
deploy/opentofu/
  mycroblog-gcp/                 the public module for Google Cloud
  examples/gcp-cloud-run-neon/   the google provider and one block of mycroblog-gcp, to copy
  upgrade.sh                     migration job first, then the workers, then the service
  console.sh                     runs one my command through the console job
  modules/internal/              the modules mycroblog-gcp composes, with no promise of compatibility
  examples/aws-ecs-neon/         AWS, composed from the internal modules, never applied: see "AWS" below
```

## Using the module

The module is a Git source pinned to a release. A pin is required: an unpinned module could change the KEK's resources under
you. Configure the `google` provider in your configuration; the module takes its project and region unless you give
`project_id` and `region`.

```hcl
provider "google" {
  project = "my-project"
  region  = "asia-northeast1"
}

module "mycroblog" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/mycroblog-gcp?ref=v0.1.0&depth=1"

  host  = "social.example.org"
  image = "ghcr.io/mycroblog/my:0.1.0"
  media = { bucket_name = "my-project-my-media" }
  mail = {
    from = "noreply@social.example.org"
    smtp = { host = "smtp.example.org", username = "my-smtp-user" }
  }
}
```

Keep `ref` at the release you deploy, whether stable or a prerelease such as `v0.1.0-rc.4`, and `image` at the same version.
The module exists only from the first release that contains it. After `tofu init`, `.terraform/modules/mycroblog/deploy/opentofu`
holds `upgrade.sh` and `console.sh` at that release, under the name of your block. `examples/gcp-cloud-run-neon` works when
copied to an empty directory with its lock file, which is where [the guide](../../docs/operations/deploy-cloud-run.md#3-copy-the-example-and-set-the-variables)
starts.

The module declares the `random` and `cloudflare` providers itself, which need no configuration. Without a Cloudflare kind no
resource uses `cloudflare`, so a plan needs no `CLOUDFLARE_API_TOKEN`; with one, the provider reads the token from it.

## Sections

Each section is an object, and every field has a default, so you write only what you change. The first kind of each row is the
default.

| Section | Kinds | What it does |
| --- | --- | --- |
| `ingress` | `load-balancer`, `domain-mapping`, `cloudflare` | How traffic reaches the service. The load balancer closes the `run.app` URL, at a monthly fee. The domain mapping costs nothing, is a Preview feature, and leaves the `run.app` URL open. Cloudflare closes the origin with a secret the module generates. `media_host` adds a cached media host on Cloudflare beside any of them |
| `database` | `external`, `cloud-sql` | `external` reads the URL of a PostgreSQL you run, such as Neon, from a secret you fill, and `pooled = true` adds a second secret for a pooled URL, which the service then uses. `cloud-sql` creates an instance and its password, and bills while idle: `tier`, `disk_size_gb`, `retained_backups`, `deletion_protection` |
| `queue` | `cloud-tasks`, `db` | `cloud-tasks` pushes jobs to the service, which can scale to zero. `db` polls the database from a process that stays up. `tick_schedule`, every minute by default, wakes the server and the database; `max_concurrent_dispatches` bounds a burst |
| `media` | `gcs`, `r2`, `s3` | `bucket_name` is required. `gcs` creates a bucket in `location`, the region by default. `r2` creates an R2 bucket in `cloudflare.account_id`. `s3` uses any S3-compatible store at `endpoint`, with `s3_region` and `path_style`. `r2` and `s3` read their access key from secrets you fill |
| `kv` | `db`, `memorystore`, `external` | The store of caches and rate limits. `memorystore` creates Memorystore for Valkey, which bills while idle and needs a VPC. `external` reads a Valkey URL from a secret you fill, and with `tls_ca = true` the CA of its certificate from another |
| `stream` | `pubsub`, `postgres`, `valkey` | The broker of real-time updates. `postgres` uses the database's direct connection, and `valkey` the Valkey of `kv` |
| `mail` | SMTP | Required: `from`, and `smtp.host`, `smtp.port` (587), and `smtp.username`. With a username, the password goes in a secret you fill |
| `kek` | `secret-manager`, `kms` | The key that encrypts the secrets in the database: a generated secret, mounted as a file, or a key in Cloud KMS that never leaves it |
| `network` | `none`, `vpc`, `existing` | The VPC the service attaches to. `vpc` creates one, and `nat = true` sends all egress through Cloud NAT for a static source address. `existing` takes yours: `network`, `subnetwork`, `private_services_range`, and `memorystore_connection_policy`. With a VPC, `cloud-sql` takes a private address on it |
| `cloudflare` | | `zone_id` and `account_id`, for the Cloudflare kinds. `host_header_override = true`, on the Enterprise plan only, lets Cloudflare connect to the `run.app` URL; otherwise a domain mapping of each host is its origin. `proxied = false` leaves the records unproxied, which a domain mapping may need until its certificate is issued |
| `service` | | The Cloud Run service: `min_instances` (0, which scales to zero), `max_instances` (3), `cpu`, `memory_mib`, `concurrency`, `cpu_always_allocated`, `request_timeout_seconds` |
| `jobs` | | The migration and console jobs: `memory_mib`, `timeout_seconds` |
| `workers` | | Services that run some job types apart from the default service, by a key that names each: [Workers](#workers) |
| `monitoring` | | Required: an uptime check, log-based metrics, and alert policies, which notify at least one of `notification_emails` and `notification_channels`. Null creates none. Its fields: `notification_emails`, `notification_channels`, `page_notification_channels`, `uptime_period`, `disabled_alerts`, `metrics_exporter`, and size limits. Each uptime check wakes an instance |
| `billing` | | Null by default. `billing_account_id`, `method`, and `dataset_location` import the usage cost into the server's dashboard |

`name` (`my` by default) starts every name the module creates, so two deployments can share a project: the service is `<name>`,
the secrets `<name>-kek`, `<name>-database-url`, and so on, the queue `<name>-jobs`. `console_enabled` creates the console
job for as long as you need it, and `deletion_protection` refuses to destroy the service.

**What does not fit together fails before the plan**, with a message that names the inputs:

- `kv.kind = memorystore` with `network.kind = none`;
- `stream.kind = valkey` with `kv.kind = db`;
- `queue.kind = db` without `service.min_instances` of 1 or more and `service.cpu_always_allocated = true`, which bill an
  instance all the time;
- `ingress.kind = cloudflare` or `ingress.media_host` without `cloudflare.zone_id`, and `media.kind = r2` without
  `cloudflare.account_id`;
- `media.kind = s3` without `media.endpoint`; `database.pooled` with `cloud-sql`; `network.nat` without `network.kind = vpc`;
  `network.kind = existing` without `network.network` and `network.subnetwork`; `kv.tls_ca` or `kv.key_prefix` without
  `kv.kind = external`.

The modules behind the sections also refuse what they cannot do, such as a split of workers that keeps apart the job types
that must stay together.

## Other settings and secrets

`settings` takes any other setting of the [configuration reference](../../docs/reference/configuration.md) by its key, such as
`translator_deepl_url`, and the module passes it to the service, the workers, and the jobs. `secrets` takes the keys of secret
settings: for each, the module creates an empty secret `<name>-<key>`, with hyphens for underscores, passes it to the server,
and lists it in `secrets_to_fill`. A key that a section sets or the module fixes fails the plan, and so does a key the
module does not take, such as `listen`: the error gives the reason.

```hcl
  settings = { translator_deepl_url = "https://api.deepl.com" }
  secrets  = ["translator_deepl_api_key"]
```

## Secrets

No secret value you give reaches the OpenTofu plan or state. The module generates what it can: the KEK and the VAPID key
through write-only attributes, which never reach the state, and the Cloud SQL password the same way. The one exception is the
origin secret of `ingress.kind = cloudflare`: Cloudflare's rule holds the value, so it lands in the state, which must stay
private.

The secrets only you have, such as the URL of an external database, the SMTP password, an access key, or the keys of
`secrets`, the module creates empty. The output `secrets_to_fill` lists each, with what goes in it and the `gcloud` command
that adds it. An instance whose secret has no version does not start, so add them before the service.

**OpenTofu generates the KEK once and never again.** The secret and its version have `prevent_destroy`, and the version number
that would write a new value is fixed in the module: a new random KEK would make everything the old one encrypted unreadable.
The output `kek` says where it is and how to copy it.

## Outputs

| Output | What it holds |
| --- | --- |
| `secrets_to_fill` | Secret ID to `what` goes in it and the `command` that adds it |
| `dns_records` | The records to add at your DNS provider for the ingress. Empty where Cloudflare creates them |
| `kek` | Where the KEK is, and with `secret-manager` the command that copies it |
| `service_uri` | The URL Cloud Run gave the service |
| `console` | What `console.sh` reads, or null while `console_enabled` is off |
| `upgrade` | The `project`, `region`, and `name` that `upgrade.sh` takes, and the services it moves |

## Workers

A busy type of job should not hold up the rest. `workers` takes a map by a key that names each worker, with its `job_types`
and its size; the module gives the same value to the service, the queue, and the monitoring, so no queue name, URL, or route
is written by hand:

```hcl
  workers = {
    delivery = { job_types = ["federation.fanout", "federation.deliver"], max_instances = 20, concurrency = 16 }
  }
```

Each worker is a Cloud Run service `<name>-<key>`, which takes no traffic from outside, with a Cloud Tasks queue
`<name>-jobs-<key>`. A worker also takes `min_instances`, `cpu`, `memory_mib`, `max_concurrent_dispatches`, and
`max_dispatches_per_second`. Whatever no worker names runs on the default service. [Splitting jobs into workers](../../docs/operations/deploy-cloud-run.md#splitting-jobs-into-workers)
has what to split.

## The scripts

[`upgrade.sh`](upgrade.sh) moves a deployment to another image: the migration job first, which runs `my config check`,
`my db upgrade -check`, `my schema apply`, and `my db upgrade`, then each worker, then the service, and it stops at the first
step that fails. An apply never changes the image of a service or job that exists: `image` is what a new one starts from.

[`console.sh`](console.sh) runs one `my` command through the console job. It reads the output `console` on standard input and
takes the command as its arguments, waits for it, prints its output, and exits with its exit code:

```sh
tofu output -json console | .terraform/modules/mycroblog/deploy/opentofu/console.sh admin create -username alice -email alice@example.com -password-link
```

The command has no standard input and no terminal. What it prints stays in Cloud Logging, so a command that needs a secret
must not take it from there or print it casually.

## AWS

AWS has no public module yet. `examples/aws-ecs-neon` composes ECS on Fargate from the internal modules, by Git source, and
**nothing of it has ever been applied**. The contract of those modules is in [modules/internal](modules/internal/README.md).

## Checks

`scripts/task.sh tofu:check` validates the public module, the internal modules, and the examples, and runs their tests on
mocked providers, which plan each kind of each section and each refused combination. None of the checks reaches a cloud, so
they show that the configuration is consistent, not that an apply on Google Cloud succeeds.

### Every configuration key

`coverage.json` in each public module says how the module handles every key of the configuration, one key or a prefix
ending in `*` (`plugin_*`) per entry:

- `section`, by the name of the module's variable: the section sets the key, for the kinds that need it;
- `fixed`: the module sets a value of its own, such as `media_max_av_bytes` on Cloud Run;
- `generic`: the operator sets it through `settings` or `secrets`;
- `excluded`, with a reason of a few words: the module does not take it.

The module reads the file: `settings` and `secrets` refuse the keys of `section` (while that section is set), `fixed`, and
`excluded`. `scripts/task.sh tofu:coverage` fails on a key with no class, a key that two entries match, an entry that matches
no key, and a `section` or `fixed` key whose `MY_` name `tests/coverage.tftest.hcl` does not name: that file plans the kind
that sets each such key on mocked providers and asserts that the service's environment has it.

A pull request that adds a configuration key, for a new adapter or anything else, runs `scripts/task.sh go:config-docs`, which
rewrites `modules/internal/settings/settings.json`, and then classifies the key in the `coverage.json` of every public
module:

- An adapter that needs only a URL and a secret is `generic`, and needs nothing more.
- An adapter for which the module must create a resource, grant a permission, enable an API, join a network, or generate a
  value is a new `kind` of a section, or a new section: add the internal module and its block, put its keys under that
  section, and add a run to `tests/coverage.tftest.hcl` that plans the kind and names each key's `MY_` variable.
- A key that cannot work on the platform is `excluded`, with the reason.
