# OpenTofu modules

`mycroblog-gcp` deploys `my` on Google Cloud in one module block: a Cloud Run service and a backend for each role, which you
pick by the `kind` of a section and which defaults to the recommended setup. `mycroblog-aws` does the same on AWS, with ECS on
Fargate, and **has never been applied** ([AWS](#aws)). Both are written for [OpenTofu](https://opentofu.org) and need version
1.11 or later. Terraform is not supported. To deploy on Google Cloud, follow
[Deploying on Cloud Run](../../docs/operations/deploy-cloud-run.md). The sections up to [AWS](#aws) describe `mycroblog-gcp`.

```text
deploy/opentofu/
  mycroblog-gcp/                 the public module for Google Cloud
  mycroblog-aws/                 the public module for AWS, never applied: see "AWS" below
  examples/gcp-cloud-run-neon/   the google provider and one block of mycroblog-gcp, to copy
  migrate.sh                     the migration of an upgrade, which an apply of mycroblog-gcp runs
  console.sh                     runs one my command through the console job
  upgrade.sh                     the upgrade of mycroblog-aws: migration job first, then the workers, then the service
  examples/aws-ecs-neon/         the aws provider and one block of mycroblog-aws, never applied
  modules/internal/              the modules the public modules compose, with no promise of compatibility
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
  media = { bucket_name = "my-project-my-media" }
  mail = {
    from = "noreply@social.example.org"
    smtp = { host = "smtp.example.org", username = "my-smtp-user" }
  }
}
```

Keep `ref` at the release you deploy. The server image defaults to the same release, so `ref` alone names what runs, and
raising it is an upgrade: see [Upgrading](#upgrading). A prerelease, such as `v0.1.0-rc.4`, names its image itself, as
`image = { version = "0.1.0-rc.4" }`, since the module of a prerelease does not default to its own image. The module exists
only from the first release that contains it. After `tofu init`, `.terraform/modules/mycroblog/deploy/opentofu` holds
`migrate.sh` and `console.sh` at that release, under the name of your block. `examples/gcp-cloud-run-neon` works when
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
| `image` | | The server image: `repository` (`ghcr.io/mycroblog/my`), `version`, the image tag, which defaults to the release of the module, and `digest`, `sha256:...`, which pins it. `latest` is refused |
| `upgrade` | | `allow_destructive_for`: the one version whose migration runs with `-allow-destructive`, as its release notes call for |
| `service` | | The Cloud Run service: `min_instances` (0, which scales to zero), `max_instances` (3), `cpu`, `memory_mib`, `concurrency`, `cpu_always_allocated`, `request_timeout_seconds` |
| `jobs` | | The migration and console jobs: `memory_mib`, `timeout_seconds` |
| `workers` | | Services that run some job types apart from the default service, by a key that names each: [Workers](#workers) |
| `translation` | | Null by default. `{}` turns on Google Cloud Translation, the translation provider `google`: the module enables its API in `project_id` (the deployment's project by default), grants the service account `roles/cloudtranslate.user` there, and sets the provider's settings with `location` (`global`). The instance setting `translation_providers` then enables it. DeepL and LibreTranslate go through `settings` and `secrets` |
| `observability` | | Where the metrics, the traces, and the error reports go: `metrics = cloud-monitoring`, `traces = cloud-trace`, and `errors = error-reporting` by default, Google Cloud's destinations, which read what Cloud Run sends them. The module enables their APIs, and the server exports nothing. `settings` leaves a signal to `settings` and `secrets`, such as `errors = "settings"` for Sentry |
| `monitoring` | | Required: an uptime check, log-based metrics, and alert policies, which notify at least one of `notification_emails` and `notification_channels`. Null creates none. Its fields: `notification_emails`, `notification_channels`, `page_notification_channels`, `uptime_period`, `disabled_alerts`, and size limits. It reads the metrics of `observability.metrics = cloud-monitoring`. Each uptime check wakes an instance |
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
  `kv.kind = external`;
- `monitoring` with `observability.metrics = settings`, since its alerts read the metrics in Cloud Monitoring.

The modules behind the sections also refuse what they cannot do, such as a split of workers that keeps apart the job types
that must stay together.

## Other settings and secrets

`settings` takes any other setting of the [configuration reference](../../docs/reference/configuration.md) by its key, such as
`translator_deepl_url`, and the module passes it to the service, the workers, and the jobs. `secrets` takes the keys of secret
settings: for each, the module creates an empty secret `<name>-<key>`, with hyphens for underscores, passes it to the server,
and lists it in `secrets_to_fill`. A key that a section sets or the module fixes fails the plan, and so does a key the
module does not take, such as `listen`: the error gives the reason. A section that is null, such as `translation`, and a
signal of `observability` set to `settings` set nothing, so `settings` may name their keys.

```hcl
  settings = { translator_deepl_url = "https://api.deepl.com" }
  secrets  = ["translator_deepl_api_key"]
```

Error reports to Sentry, beside the log line that Error Reporting reads, leave the errors of `observability` to `settings`:

```hcl
  observability = { errors = "settings" }
  settings      = { errors_exporter = "sentry" }
  secrets       = ["errors_sentry_dsn"]
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
| `version` | The `version` applied, and the `image` the service, the workers, and the jobs run |

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

## Upgrading

An apply of another image is an upgrade, and runs it in the order the database needs, stopping at the first step that fails:

1. The migration and console jobs take the image.
2. [`migrate.sh`](migrate.sh), at the same release as the module, runs `my upgrade` as an execution of the migration job
   and waits for it: `my config check`, `my db upgrade -check`, `my schema apply`, and `my db upgrade`. It first waits for any
   execution that an interrupted apply left running, and passes `-allow-destructive` only when `upgrade.allow_destructive_for`
   names the version being applied.
3. Each worker takes the image, then the service.

A migration that fails, or an apply that stops during it, leaves the services on their revisions, and the next apply of the
same image runs the migration again: there is nothing to reset. An apply of the same image runs nothing. The script runs where
you apply, through `gcloud`, so that machine, or the CI job that applies, needs `gcloud`, `jq`, and credentials that may
execute the job and read its logs. The first apply runs the migration too, before the services exist. The guide has
[the whole procedure](../../docs/operations/deploy-cloud-run.md#upgrading), with a backup and the maintenance mode.

## The scripts

[`upgrade.sh`](upgrade.sh) upgrades the AWS example, whose modules leave the image to it: the migration job first, then each
worker, then the service, and it stops at the first step that fails.

[`console.sh`](console.sh) runs one `my` command through the console job. It reads the output `console` on standard input and
takes the command as its arguments, waits for it, prints its output, and exits with its exit code:

```sh
tofu output -json console | .terraform/modules/mycroblog/deploy/opentofu/console.sh admin create -username alice -email alice@example.com -password-link
```

The command has no standard input and no terminal. What it prints stays in Cloud Logging, so a command that needs a secret
must not take it from there or print it casually.

## AWS

`mycroblog-aws` deploys `my` on AWS in one block: an ECS service on Fargate behind an Application Load Balancer, the jobs in
the database, and a backend for each role, picked by the `kind` of a section as in `mycroblog-gcp`. **Nothing of it has ever
been applied on AWS**: the checks plan it on mocked providers and with no credentials, which shows that the configuration is
consistent, not that AWS accepts it or that the server starts. Treat a first deployment as a test.

```hcl
provider "aws" {
  region = "ap-northeast-1"
}

module "mycroblog" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/mycroblog-aws?ref=v0.1.0&depth=1"

  host       = "social.example.org"
  image      = "ghcr.io/mycroblog/my:0.1.0"
  network    = { egress = "public-ip", availability_zones = ["ap-northeast-1a", "ap-northeast-1c"] }
  ingress    = { certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/00000000-0000-0000-0000-000000000000" }
  media      = { bucket_name = "social-example-org-media" }
  mail       = { from = "noreply@social.example.org" }
  monitoring = { notification_emails = ["ops@social.example.org"] }
}
```

`examples/aws-ecs-neon` is this block with its variables, to copy with its lock file. The module takes the region of the `aws`
provider and declares `random`, `archive`, and `cloudflare` itself. It uses no provider alias, so the certificate that
CloudFront needs in `us-east-1` comes in as an ARN, created by hand in AWS Certificate Manager like the load balancer's.

### Sections of mycroblog-aws

Required: `network.egress`, `ingress.certificate_arn`, `media.bucket_name`, `mail.from`, and `monitoring`, which is null or
names who the alarms notify. Every other field has a default. The first kind of each row is the default.

| Section | Kinds | What it does |
| --- | --- | --- |
| `network` | `vpc`, `existing` | ECS always needs a VPC. `vpc` creates one over `availability_zones`, at least two, in `cidr_block`. `existing` takes yours: `vpc_id`, `vpc_cidr`, `public_subnet_ids`, `public_subnet_cidrs`, and `private_subnet_ids`. `egress` has no default, since both ways out bill by the hour: `public-ip` gives each task a public IPv4 address in a public subnet, `nat` puts the tasks in private subnets behind a NAT gateway, which `vpc` creates and a VPC of yours must have |
| `ingress` | `load-balancer`, `cloudfront`, `cloudflare` | The load balancer is always there, with `certificate_arn`, a certificate for `host` in the region. `load-balancer` serves `host` from it, with the records in the Route 53 zone `route53_zone_id` or listed in `dns_records` (`host_is_apex` for an apex without Route 53). `cloudfront` puts CloudFront in front, with `/media/` cached, and the load balancer then admits CloudFront alone: `cloudfront_certificate_arn` (in `us-east-1`) and `cloudfront_price_class`. `cloudflare` puts the zone of `cloudflare` in front, which creates the record, and the server refuses what lacks a secret the module generates. `media_host` adds a cached media host on Cloudflare beside `load-balancer` or `cloudflare`; the certificate must cover it too |
| `database` | `external`, `rds` | `external` reads the URL of a PostgreSQL you run, such as Neon, from a secret you fill, and `pooled = true` adds a secret for a pooled URL, which the service then uses. `rds` creates an instance and its password in the private subnets, and bills while idle: `instance_class`, `allocated_storage_gb`, `max_allocated_storage_gb`, `backup_retention_days`, `multi_az`, `deletion_protection` |
| `media` | `s3`, `r2`, `s3-compatible` | `bucket_name` is required. `s3` creates a bucket, with `versioning` and `noncurrent_version_days`. `r2` creates an R2 bucket in `cloudflare.account_id`. `s3-compatible` uses another store at `endpoint`, with `s3_region` and `path_style`. `r2` and `s3-compatible` read their access key from secrets you fill |
| `kv` | `db`, `elasticache`, `external` | The store of caches and rate limits. `elasticache` creates ElastiCache Serverless for Valkey, which bills while idle. `external` reads a Valkey URL from a secret you fill, and with `tls_ca = true` the CA of its certificate from another |
| `stream` | `sns`, `postgres`, `valkey` | The broker of real-time updates. `postgres` uses the database's direct connection, and `valkey` the Valkey of `kv` |
| `mail` | `ses`, `smtp` | `from` is required. `ses` sends through Amazon SES in the region, from the domain of `from`, with an optional `configuration_set`: read [Email through Amazon SES](#email-through-amazon-ses). `smtp` takes `smtp.host`, `smtp.port` (587), and `smtp.username`, whose password goes in a secret you fill |
| `kek` | `secrets-manager`, `kms` | The key that encrypts the secrets in the database: a generated secret, or a key in AWS KMS that never leaves it |
| `cloudflare` | | `zone_id` and `account_id`, for the Cloudflare kinds |
| `service` | | The ECS service: `desired_count` (1), `cpu` (256), `memory_mib` (1024), and `health_check_grace_seconds`. Nothing scales to zero: a task bills for as long as it runs |
| `jobs` | | The migration and console tasks: `cpu` and `memory_mib` |
| `workers` | | Services that run some job types apart from the default service, by a key that names each: `job_types`, `instances`, `cpu`, `memory_mib`, and `concurrency` |
| `translation` | | Null by default. `{}` turns on Amazon Translate, the translation provider `aws`: the module allows the tasks' role `translate:TranslateText` and `comprehend:DetectDominantLanguage`, and sets the provider's `region`, the deployment's by default. The instance setting `translation_providers` then enables it |
| `search` | | Null by default. `domain_name` names a domain of Amazon OpenSearch Service that you created in the region: the module reads it, allows the tasks' role the HTTP methods the server sends to it, and sets `search = opensearch` with signed requests, and the alias `index`. A domain in the VPC takes `security_group_ids` for the tasks to join. The module never creates the domain. Elasticsearch, Meilisearch, and OpenSearch with a user go through `settings` and `secrets` |
| `observability` | | Where the metrics, the traces, and the error reports go: `metrics = cloudwatch`, in Embedded Metric Format in the namespace `name`, `errors = cloudwatch`, the error report line in CloudWatch Logs, and `traces = none`, since X-Ray needs a collector beside the server, which the tasks do not run. Nothing needs a grant. `settings` leaves a signal to `settings` and `secrets`, such as `errors = "settings"` for Sentry |
| `monitoring` | | Required: a CloudWatch Synthetics canary, metrics in CloudWatch, and alarms, which notify at least one of `notification_emails` and `notification_topic_arns`. Null creates none. Also `page_notification_topic_arns`, `uptime_period_minutes` (15), `disabled_alerts`, and size limits. It reads the metrics of `observability.metrics = cloudwatch`. Each run of the canary is billed |
| `billing` | | Null by default. `{}` imports the usage cost from Cost Explorer into the server's dashboard, with the tasks' own role |

There is no `queue` section: the tasks stay up, so they take the jobs from the database. `name`, `console_enabled`,
`log_retention_days`, `settings`, and `secrets` work as in `mycroblog-gcp`, with the secrets in Secrets Manager.

**What does not fit together fails before the plan**, with a message that names the inputs:

- `network.kind = vpc` without two `availability_zones`, or `availability_zones` with `existing`; `existing` without its five
  fields;
- `ingress.kind = cloudfront` without `cloudfront_certificate_arn`, or that certificate with another kind;
  `ingress.kind = cloudflare` or `media_host` without `cloudflare.zone_id`; `route53_zone_id` with `cloudflare`;
  `media_host` with `cloudfront`, or equal to `host`;
- `database.pooled` with `rds`; `media.kind = s3-compatible` without `endpoint`; `r2` without `cloudflare.account_id`;
- `kv.tls_ca` or `kv.key_prefix` without `kv.kind = external`; `stream.kind = valkey` with `kv.kind = db`;
- `mail.kind = smtp` without `mail.smtp`, `mail.smtp` with `ses`, and `mail.configuration_set` with `smtp`;
- `monitoring` that names nobody to notify, or with `observability.metrics = settings`.

### Outputs of mycroblog-aws

| Output | What it holds |
| --- | --- |
| `secrets_to_fill` | Secret name to `what` goes in it and the `aws secretsmanager put-secret-value` command that adds it |
| `dns_records` | The records to add at your DNS provider: `host`, unless Route 53 or Cloudflare creates it, and with `mail.kind = ses` the DKIM records of the domain of `mail.from` |
| `dns_note` | What `dns_records` cannot say, or null: at an apex without Route 53, the record of `host` needs an ALIAS or ANAME |
| `kek` | Where the KEK is, and with `secrets-manager` the command that copies it |
| `domain_name` | What `host` points at: the distribution with `cloudfront`, the load balancer otherwise |
| `console` | What `console.sh` reads, or null while `console_enabled` is off |
| `upgrade` | The `region` and `name` that `upgrade.sh --platform aws-ecs` takes, and the services it moves |

### The first deployment on AWS

`/readyz` fails until the schema exists, so the schema comes first. Create the secrets, fill the ones that `secrets_to_fill`
lists, create the migration's task definition, run the migration, then apply everything:

```sh
tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secrets_manager
tofu output secrets_to_fill   # run each command with its value
tofu apply -target='module.mycroblog.module.app.aws_ecs_task_definition.task["migrate"]'

SCRIPTS=.terraform/modules/mycroblog/deploy/opentofu
"$SCRIPTS/upgrade.sh" --platform aws-ecs --region ap-northeast-1 --image ghcr.io/mycroblog/my:0.1.0 --migrate-only

tofu apply
tofu output dns_records       # add each at your DNS provider
```

`upgrade.sh --platform aws-ecs` upgrades later, as on Google Cloud, and `console.sh` runs a command through the console's task
definition, whose output stays in CloudWatch Logs.

### What it costs on AWS

Nothing scales to zero on ECS: the task, the load balancer, and its public addresses bill all month. With one task of 0.25 vCPU
and 1 GB, expect about $42 a month at idle in Tokyo with `egress = "public-ip"`, and about $75 with `egress = "nat"`, before
the database, storage, traffic, and the canary. These figures come from AWS's price list and have not been checked against a
bill.

### Email through Amazon SES

`mail.kind = ses`, the default, sends with the SES API in the region of the deployment, from the domain of `mail.from`. The
module creates the domain's identity and allows the tasks `ses:SendEmail` on it, so there is no password. Before SES sends to
your users, add the records that `tofu output dns_records` lists and leave the sandbox:
[Sending email through Amazon SES](../../docs/operations/email-ses.md) has the steps.

## Checks

`scripts/task.sh tofu:check` validates the public modules, the internal modules, and the examples, and runs their tests on
mocked providers, which plan each kind of each section and each refused combination. `tofu:aws-plan-test` plans the AWS
example with no credentials, a local stub answering its one read of the caller's identity. None of the checks reaches a
cloud, so they show that the configuration is consistent, not that an apply on Google Cloud or AWS succeeds.

### Every configuration key

`coverage.json` in each public module says how the module handles every key of the configuration, one key or a prefix
ending in `*` (`plugin_*`) per entry:

- `section`, by the name of the module's variable: the section sets the key, for the kinds that need it;
- `fixed`: the module sets a value of its own, such as `media_max_av_bytes` on Cloud Run;
- `generic`: the operator sets it through `settings` or `secrets`;
- `excluded`, with a reason of a few words: the module does not take it.

The module reads the file: `settings` and `secrets` refuse the keys of `section` (while that section is set, and for
`observability` while the signal that starts the key's name is not `settings`), `fixed`, and `excluded`. `scripts/task.sh tofu:coverage` fails on a key with no class, a key that two entries match, an entry that matches
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
