# Deploying on AWS

How to run `my` on AWS with the OpenTofu module [`mycroblog-aws`](../../deploy/opentofu/README.md#aws): ECS on Fargate behind
an Application Load Balancer, the jobs in the database, S3, SNS, and email through Amazon SES. Nothing scales to zero: expect
about $42 a month at idle before the database, and read [Costs](#costs).

**Nothing of it has ever been applied on AWS.** The checks plan it on mocked providers and with no credentials, which shows that
the configuration is consistent, not that AWS accepts it or that the server starts. Treat a first deployment as a test.

What every OpenTofu deployment shares, the module's inputs, its secrets, the console, and upgrades among it, is in
[Deploying with OpenTofu](deploy-opentofu.md). This page adds what AWS needs.

## What it creates

One block of `mycroblog-aws` creates all of this. Each row is a section of the module, and its default is the recommended
setup; [the module's reference](../../deploy/opentofu/README.md#sections-of-mycroblog-aws) lists every field.

| Part | Section | Default | Other kinds |
| --- | --- | --- | --- |
| Server | `service`, `jobs` | An ECS cluster and service on Fargate with `role = all`, task definitions for the migration and, while you need it, the console, and a log group `/ecs/<name>` | |
| Network | `network` | A VPC over two zones or more, with a public and a private subnet in each | A VPC of yours: [Networks](#networks) |
| Traffic | `ingress` | An Application Load Balancer, always there, with your certificate | CloudFront or Cloudflare in front of it: [How traffic reaches the service](#how-traffic-reaches-the-service) |
| Jobs | | None: the tasks stay up and take the jobs from the database | |
| Database | `database` | A secret for the URL of a PostgreSQL you run, such as Neon | Amazon RDS: [Choose the database](#2-choose-the-database) |
| Media | `media` | A private S3 bucket | Cloudflare R2, or any S3-compatible store |
| Caches and rate limits | `kv` | None: the database holds them | ElastiCache Serverless for Valkey, or a Valkey you run |
| Real-time updates | `stream` | An SNS topic. The server creates and removes its own SQS queues | The database, or the Valkey of `kv` |
| Email | `mail` | The SES identity of the domain of `mail.from`: [Email through Amazon SES](#email-through-amazon-ses) | Any SMTP service |
| Secrets | | Secrets Manager: a generated VAPID key, and the secrets that only you can fill, empty | |
| KEK | `kek` | A generated secret in Secrets Manager | A key in AWS KMS, which never leaves it |
| Metrics, traces, errors | `observability` | Metrics and error reports in CloudWatch, traces to X-Ray through a collector in every task: [Traces to X-Ray](#traces-to-x-ray) | Your own exporters, through `settings` |
| Alerts | `monitoring` | A CloudWatch Synthetics canary and alarms: [Monitoring and alerts](#monitoring-and-alerts) | None, with `null` |

`translation = {}` adds Amazon Translate, `search = { domain_name = "..." }` a domain of Amazon OpenSearch Service that you
created, and `billing = {}` the import of the usage cost from Cost Explorer: [Translating posts](translation.md#amazon-translate),
[Search engines](search-engines.md), and [Importing cloud usage costs](billing-import.md).

## Before you start

- What [the first deployment](deploy-opentofu.md#first-deployment) needs, with `aws` as the platform's CLI.
- An AWS account, used for this instance alone, and credentials for it that may create all of the above, such as
  `AdministratorAccess`.
- A region. SES sends from it, and the module takes it from the `aws` provider.
- Recommended: a public hosted zone of `host` in Route 53, where the module then creates the record of `host`, at a zone apex
  too.

## First deployment

Do the first three steps here, then follow [the first deployment](deploy-opentofu.md#first-deployment) with the notes of the
fourth.

### 1. Request the certificates

The load balancer needs a validated certificate for `host` in AWS Certificate Manager, in the region. The module takes it as an
ARN, since validating it within the apply would hold the apply until you add records that only the apply prints.

```sh
aws acm request-certificate --region ap-northeast-1 --domain-name social.example.org --validation-method DNS
aws acm describe-certificate --region ap-northeast-1 --certificate-arn "$CERTIFICATE_ARN" \
  --query 'Certificate.DomainValidationOptions[].ResourceRecord'
# Add that CNAME record at your DNS provider, then:
aws acm wait certificate-validated --region ap-northeast-1 --certificate-arn "$CERTIFICATE_ARN"
```

With `ingress.kind = cloudfront`, request a second one in `us-east-1`, the only region CloudFront takes one from; in `us-east-1`
one certificate serves both. With `ingress.media_host`, the certificate must cover that host too.

### 2. Choose the database

`database.kind` picks it. Either way the tasks poll the database for jobs about once a second, so it never rests.

**`external`, the default and the recommended one**, reads the URL of a PostgreSQL you run, such as [Neon](https://neon.com),
from a secret you fill. The module never creates the database, so its password stays out of the OpenTofu state. On Neon,
create a project in a region near yours and copy its connection strings:

- The **direct** one (the host without `-pooler`) is required, in the secret `<name>-database-url`. The migration and the
  console always use it: `my schema apply` holds a session-level lock, which a pooler in transaction mode does not keep.
- The **pooled** one (the host with `-pooler`) is optional, for the service: set `database = { kind = "external", pooled = true }`,
  and the module creates the secret `<name>-database-url-pooled`. See
  [Through a connection pooler](database.md#through-a-connection-pooler).

Expect Neon's Launch plan: since the compute never stops, the Free plan's compute hours run out within the month.

**`rds`** creates Amazon RDS for PostgreSQL in the private subnets, with a password that the module generates and writes to
`<name>-database-url` itself, so there is nothing to fill:

```hcl
  database = { kind = "rds" }
```

It is `db.t4g.micro` with 20 GB by default (`instance_class`, `allocated_storage_gb`), keeps 7 days of automated backups, and
has deletion protection. It has no public address, so only the tasks reach it: a `pg_dump` must run from inside the VPC, and
before an upgrade a snapshot takes its place,
`aws rds create-db-snapshot --db-instance-identifier my --db-snapshot-identifier my-before-upgrade`. It has no pooled URL.

### 3. Write the block

`examples/aws-ecs-neon` is one way to start: the `aws` provider and one block with the defaults, whose variables
`terraform.tfvars.example` lists. Whatever you start from, the block needs these:

```hcl
provider "aws" {
  region = "ap-northeast-1"
}

module "mycroblog" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/mycroblog-aws?ref=v0.1.0&depth=1"

  host       = "social.example.org"
  network    = { egress = "public-ip", availability_zones = ["ap-northeast-1a", "ap-northeast-1c"] }
  ingress    = { certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/00000000-0000-0000-0000-000000000000" }
  media      = { bucket_name = "social-example-org-media" }
  mail       = { from = "noreply@social.example.org" }
  monitoring = { notification_emails = ["ops@social.example.org"] }
}
```

- `network.egress` has no default, since both ways out bill by the hour: read [Networks](#networks).
- `ingress.route53_zone_id` names the hosted zone of `host`, if you have one.
- `media.bucket_name` must be unique across AWS.
- `monitoring` names who the alarms notify, or is `null`.

Add the section of the database you chose, and any other you change.

### 4. Deploy

Follow [the first deployment](deploy-opentofu.md#first-deployment). On AWS, where the commands of this page use the default
`name`, `my`:

- **Secrets**: they are in Secrets Manager, named `<name>-...`. `tofu output kek` has the command that copies the KEK, and
  `aws secretsmanager get-secret-value --secret-id my-vapid-key --query SecretString --output text` copies the VAPID key. The
  KEK reaches the server as `MY_KEK`, since Fargate mounts no file. A task reads its secrets when it starts: after you change
  one, run `aws ecs update-service --cluster my --service my --force-new-deployment`, and the same for each worker,
  `my-<key>`.
- **Apply**: the migration's task definition comes first, with what it depends on, and the services only once the migration
  succeeded. The load balancer's check asks `/readyz`, which answers `503` until the schema exists.
- **DNS**: `tofu output dns_records` lists the record of `host`, unless Route 53 or Cloudflare creates it, and the three DKIM
  records of SES. At a zone apex without Route 53, the record of `host` needs what your DNS provider calls an `ALIAS` or
  `ANAME`, as `tofu output dns_note` says.
- **Email**: SES sends only to addresses you verified until AWS grants production access, so nobody else can sign up yet:
  read [Email through Amazon SES](#email-through-amazon-ses).
- **Alerts**: each address of `monitoring.notification_emails` receives a subscription email from SNS, which it must confirm.
- **The first administrator**: the link stays in CloudWatch Logs, in `/ecs/<name>`, where anyone who can read the account's logs
  can use it.

## The console and upgrades on AWS

[The console](deploy-opentofu.md#the-console) and [upgrades](deploy-opentofu.md#upgrading) run one-off Fargate tasks from where
you apply, through `aws`, and read their output from CloudWatch Logs. An interrupted apply leaves the task running, and the next
apply waits for it to end. In an upgrade, every task definition gets a new revision with the image, which nothing runs until a
service takes it; each service then waits for its new tasks to start, and on the default service to pass the load balancer's
check, before the old ones leave, and the deployment circuit breaker takes a revision whose tasks do not start back to the one
before.

The credentials where you apply, or of the CI job that applies, need beside what the apply itself needs:

- for the migration: `ecs:RunTask`, `ecs:DescribeTasks`, `ecs:ListTasks`, `logs:FilterLogEvents`, and `iam:PassRole` on the
  tasks' two roles;
- for the console, also: `ecs:DescribeServices`, `ecs:DescribeTaskDefinition`, `ecs:ListTagsForResource`, and
  `ecs:RegisterTaskDefinition`, since the console first takes the image of the running service.

To [check the SQL first](deploy-opentofu.md#checking-the-sql-first), turn the console on while the block still names the
current release, then raise `ref`, apply the migration's task definition alone, and run `my upgrade -dry-run` with it:

```sh
tofu apply -var console_enabled=true
# Name the new release in main.tf, then:
tofu init
tofu apply -var console_enabled=true -target='module.mycroblog.module.app.aws_ecs_task_definition.task["migrate"]'
tofu output -json console | jq -c '.task_definition = "my-migrate" | .log_prefix = "migrate" | del(.service)' |
  "$SCRIPTS/console.sh" upgrade -dry-run
```

## Monitoring and alerts

`monitoring` adds a CloudWatch Synthetics canary of `/readyz`, metrics in CloudWatch, and alarms with the names of
[the runbook](alerts.md#aws-ecs), which notify the addresses of `notification_emails` or the SNS topics of
`notification_topic_arns`. The input has no default: name who is notified, or set `monitoring = null` to leave all of it out.
Every run of the canary is billed, and `uptime_period_minutes`, 15 by default, is how often it runs: read [Costs](#costs).

## Choosing the backends

Every section defaults to the recommended kind; [change the `kind` of a section](deploy-opentofu.md#changing-a-backend) for
another. The database is [above](#2-choose-the-database).

| Section | Recommended | Other kinds, and when |
| --- | --- | --- |
| `media` | `s3`, a bucket in the region, with `versioning` off | `r2` in `cloudflare.account_id`, or `s3-compatible` at `endpoint`, each with an access key in secrets you fill |
| `kv` | `db`, which costs nothing more | `elasticache`, which bills while idle and lives in the private subnets, or `external`, a Valkey URL in a secret you fill |
| `stream` | `sns`, which reaches every task | `postgres`, over the database's direct connection, or `valkey`, the Valkey of `kv` |
| `mail` | `ses`, with no password | `smtp`, with `smtp.host`, `smtp.port` (587), and `smtp.username`, whose password goes in a secret you fill |
| `kek` | `secrets-manager`, which you copy once | `kms`, a key that never leaves AWS KMS. On an instance with data, [changing it](deploy-opentofu.md#the-kek) is a move |
| `observability` | CloudWatch and X-Ray | `traces = "none"`, or `"settings"` for any signal you send elsewhere, such as Sentry |

`architecture = "arm64"` runs the tasks on Graviton, which costs less; the released image has both.

## Email through Amazon SES

`mail.kind = "ses"`, the default, sends with the SES API in the region of the deployment, from the domain of `mail.from`. The
module creates the domain's identity with Easy DKIM and allows the tasks `ses:SendEmail` on it, so there is no password, and
`mail.configuration_set` creates a configuration set and applies it to every message. Before SES sends to your users, add the
records that `tofu output dns_records` lists and leave the sandbox:
[Sending email through Amazon SES](email-ses.md) has the steps, and how the server hears of bounces and complaints.

## Traces to X-Ray

`observability.traces = "x-ray"` is the default, because the collector fits the recommended setup at no extra charge. It is the
AWS Distro for OpenTelemetry collector, which runs beside the server in every task, listens on the task's loopback alone, and
writes to X-Ray with the tasks' role (`xray:PutTraceSegments`). It takes 32 CPU units and 128 MiB out of each task, which AWS
bills by the size of the task alone, so the default service task of 256 CPU units and 1,024 MiB costs the same and gives the
server 224 CPU units and 896 MiB. The default `traces_sample_ratio`, 1%, records about 30,000 traces a month for 100,000
requests a day, under the 100,000 traces a month that X-Ray records for free.

What it does cost is the collector's image, pulled at each start of a task, and the server's memory. Set `traces = "none"` when
you do not read the traces, or for a task too small to spare 128 MiB: the plan refuses a size that leaves the server less than
128 CPU units or 512 MiB, in `service`, `jobs`, or a worker. The collector is not essential: if it stops, the server goes on
without traces. That X-Ray takes its spans is unverified.

## Networks

ECS always needs a VPC. `network.kind = "vpc"`, the recommended one, creates one in `cidr_block` (`10.0.0.0/16`) over
`availability_zones`, at least two, which the load balancer and RDS need: a public and a private subnet in each zone, and an
internet gateway. It is IPv4 only, so the server reaches no IPv6-only peer. `network.kind = "existing"` takes a VPC of yours
instead: `vpc_id`, `vpc_cidr`, `public_subnet_ids`, `public_subnet_cidrs`, and `private_subnet_ids`.

A task needs a way out to the internet, for the image, the database, federation, and the AWS APIs, and `network.egress` picks it.
Inbound traffic reaches a task through the load balancer alone, either way:

| | `public-ip` (recommended) | `nat` |
| --- | --- | --- |
| Subnets of the tasks | Public, with a public IPv4 address on each task | Private |
| Each hour | $0.005 per task's address | One NAT gateway, about $0.045, and its address |
| Each GB | Nothing for the address | About $0.045 through the gateway, media and federation included |
| With `existing` | Nothing more | Your private subnets must already reach the internet |

Use `public-ip` unless a policy of yours forbids public subnets: it costs far less and is no more open. `vpc` creates one NAT
gateway, in the first zone: when that zone goes down, the private subnets lose the internet until it comes back. RDS and
ElastiCache live in the private subnets with either, and need no way out.

## How traffic reaches the service

The load balancer is always there, with the certificate of `ingress.certificate_arn`, and the tasks admit it alone.
`ingress.kind` decides what stands in front of it:

| | `load-balancer` (recommended) | `cloudfront` | `cloudflare` |
| --- | --- | --- | --- |
| `host` points at | The load balancer | A CloudFront distribution | Cloudflare, which connects to the load balancer |
| Cached | Nothing | `/media/`, by the server's `Cache-Control` | Nothing on `host`; `media_host` adds a cached host |
| Records | Route 53 creates them with `route53_zone_id`, or `dns_records` lists them | Route 53 with `route53_zone_id`, or `dns_records` | Cloudflare creates it |
| Needs | | `cloudfront_certificate_arn`, in `us-east-1`. `cloudfront_price_class` picks the edge locations | `cloudflare = { zone_id = "..." }`, and the token in `CLOUDFLARE_API_TOKEN` |
| Closes the origin by | The tasks' security group | The load balancer's security group admits CloudFront alone, and its listener refuses a request without a header that CloudFront sends | A header that Cloudflare's rule adds, which the server checks |

Behind CloudFront or Cloudflare, the value of the header lands in the OpenTofu state, which must stay private. A record of the
load balancer from before CloudFront must go first: an existing record of the same name fails the apply. CloudFront's
addresses come from AWS when you plan, so apply now and then, or an edge on a new address shares its viewers' rate limits.

`ingress.media_host` adds a cached media host on Cloudflare, beside `load-balancer` or `cloudflare`, with
`cloudflare.zone_id`. A deleted file stays cached until you purge it: read [Serving media through a CDN](cdn.md). With Cloudflare
in front of anything, turn off Bot Fight Mode in its dashboard, since a challenge on `/ap/`, `/.well-known/`, or `/api/` breaks
federation, and the module does not manage it. The Cloudflare kinds have run only on mocked providers.

## Limits

- **Health**: the load balancer's one check asks `/readyz`, which also fails while the database is down: after 2.5 minutes of
  failures ECS replaces the task, and goes on doing so until the database is back. There is no container check, so a worker,
  which has no load balancer, fails on a bad image only in its logs.
- **Uploads**: the load balancer sets no limit on a request's body, so the server's `media_max_*` settings decide.
- **Streams**: a stream sends a heartbeat every 30 seconds, under the load balancer's idle timeout of 60. It stays open until
  its task leaves, and the client then connects again.
- **Client addresses**: the rate limits rely on what the server trusts, which the module sets from the ingress. Check the client
  addresses in the first request logs: an address of the load balancer's subnets, or of CloudFront, means that it is wrong.
- **The disk** goes with the task, so the plan refuses SQLite and media on the filesystem.
- **Secrets** under a customer-managed KMS key also need `kms:Decrypt`, which the module does not grant.

## Costs

Nothing scales to zero on ECS: the task, the load balancer, and its public addresses bill all month. These figures come from
AWS's price list for Tokyo, from 2026-10 and 2026-11, unless a row says otherwise, and have not been checked against a bill.

| Part | Per month | Choice |
| --- | --- | --- |
| A task of 0.25 vCPU and 1 GB | $13.26 on x86, $10.61 on arm64 | `service`, `architecture`. Each worker adds its own tasks |
| Load balancer | $17.74, plus $0 to $5.84 for each capacity unit in use all month | Always there |
| The load balancer's public addresses | $7.30, for two zones | One per zone |
| The way out | $3.65 per task with `public-ip`; about $36.50 and $0.045 per GB with `nat`, at Ohio's rate | `network.egress` |
| Database | About $19 on Neon's Launch plan at 0.25 CU, since the compute never stops; about $21 with `rds`, `db.t4g.micro` and 20 GB | `database` |
| Caches and rate limits | $0 with `db`; with `elasticache`, about $6.13, or about $61 by another of AWS's documents, in N. Virginia | `kv` |
| The canary | About $3.34 at every 15 minutes; about $51.72 at every minute | `monitoring.uptime_period_minutes` |
| Traces | $0 up to 100,000 traces a month | `observability.traces` |

**At idle, about $42 with `public-ip` and about $75 with `nat`**, plus capacity units, before the database, storage, traffic,
and the canary. Not read, so in no row: CloudFront, Secrets Manager, CloudWatch Logs, S3, SNS and SQS, data out of AWS, and any
free tier. One-off tasks cost their size for the minutes they run, and an upgrade runs a second service task for a few minutes.
