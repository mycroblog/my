# OpenTofu modules

Modules that compose a deployment of `my` from swappable backends. They are written for [OpenTofu](https://opentofu.org)
and need version 1.11 or later: they use its `lifecycle` `enabled`, ephemeral resources, and write-only attributes. Only OpenTofu is supported. To deploy, follow [Deploying on Cloud Run](../../docs/operations/deploy-cloud-run.md);
this file is the contract for whoever uses or adds a module.

```text
deploy/opentofu/
  modules/app-gcp-cloud-run/      service, migration job, console, service account, public access
  modules/queue-gcp-cloud-tasks/  queue, Scheduler tick, the service account that calls the server
  modules/queue-db/               the queue in the database, for a process that stays up: settings only
  modules/blob-gcp-gcs/           media bucket
  modules/stream-gcp-pubsub/      topic and the permissions for the server's own subscriptions
  modules/db-external/            connection URLs in Secret Manager: Neon created by hand, or any PostgreSQL
  modules/db-neon/                a Neon project it creates, with the password in the state: read its README
  modules/db-gcp-cloud-sql/       Cloud SQL, through the connector's socket or on a network, never applied
  modules/blob-s3/                any S3-compatible store, given an endpoint and a key
  modules/blob-cloudflare-r2/     an R2 bucket, with its credentials by reference
  modules/kv-external/            a Valkey outside this configuration, by a URL in the secret store
  modules/kv-gcp-memorystore/     Memorystore for Valkey, on a network, never applied
  modules/kv-aws-elasticache/     ElastiCache Serverless for Valkey, on a network, never applied
  modules/network-gcp-vpc/        a VPC that the others take as network, never applied
  modules/network-aws-vpc/        the same on AWS, never applied
  modules/stream-valkey/          the broker on Valkey Pub/Sub, beside a kv module or alone
  modules/stream-postgres/        the broker on LISTEN / NOTIFY of the database
  modules/mail-smtp/              email through any SMTP service
  modules/ingress-gcp-domain-mapping/  traffic through a Cloud Run domain mapping
  modules/ingress-gcp-load-balancer/   traffic through a global external Application Load Balancer
  modules/ingress-cloudflare-app/      traffic through Cloudflare, for any origin
  modules/ingress-cloudflare-media/    a media host through Cloudflare, beside any ingress
  modules/secrets-gcp-secret-manager/  secrets to fill by hand, or a VAPID key it generates
  modules/kek-gcp-secret-manager/      the KEK it generates as a secret, mounted as a file
  modules/kek-gcp-kms/                 the KEK in Cloud KMS
  modules/billing-gcp/                 the usage cost import from Google Cloud, never applied
  modules/billing-aws/                 the usage cost import from AWS, never applied
  modules/monitoring-gcp/              uptime check, log-based metrics, and alert policies
  examples/gcp-cloud-run-neon/    all of the above, composed, on Git sources at MODULE_REF
  modules/*-aws-*/, examples/aws-lambda-neon/, examples/aws-ecs-neon/  the same on AWS, never applied: see "AWS: unverified" below
  upgrade.sh                      migration job first, then the service
  console.sh                      runs one my command through an app module's console
  aws-ecs.sh                      what those two share on ECS
```

## Using a module

A module is a Git source pinned to a release. A pin is required: an unpinned module could change the KEK's resources under you.

```hcl
module "db" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/db-external?ref=v0.1.0&depth=1"
  # ...
}
```

Keep every block at the same release, the one you deploy, whether stable or a prerelease such as `v0.1.0-rc.4`. A module exists
only from the first tag that contains it, so an earlier tag has none of them. `.terraform/modules/app/deploy/opentofu` holds
`upgrade.sh` and `console.sh` after `tofu init`, at that release. `examples/gcp-cloud-run-neon` works when copied to an empty
directory with its lock file: [the guide](../../docs/operations/deploy-cloud-run.md#3-copy-the-example-and-set-the-variables)
starts that way.

In this repository, `MODULE_REF` holds the release the examples name. Inside the repository the checks test the modules of the
same commit: `scripts/task.sh tofu:example-test` runs on a copy whose sources are rewritten to local paths, and never
fetches from GitHub. To update the example's lock file, use `scripts/task.sh tofu:lock`.

## Naming

A module is named `<role>-<provider>-<service>`, so that the name says what it is for before where it runs:

| Role | What it provides |
| --- | --- |
| `app` | The server itself. It merges what the others return |
| `db` | The database |
| `queue` | The job queue and what triggers the periodic jobs |
| `blob` | The media store |
| `kv` | The key-value store of caches and rate limits, for `kv = valkey`. `kv = db`, the default, needs no module |
| `stream` | The broker of the real-time updates |
| `mail` | Email |
| `secrets` | Secrets, filled by hand or generated |
| `kek` | The key that encrypts the secrets in the database |
| `ingress` | How traffic reaches the app |
| `billing` | What the import of a provider's usage costs reads |
| `monitoring` | The uptime check, the alerts, and the settings of the metrics that they read |
| `network` | The private network that the others take as an input. It is no backend |

A module with no provider of its own drops that part (`db-external`, `mail-smtp`). The provider is what the module talks to,
not the platform behind it: `ingress-cloudflare-app` takes its origin as an input, so the same module fronts Cloud Run or AWS. Cloud SQL would be `db-gcp-cloud-sql`.
A module that covers a provider's several services for one purpose drops the service (`billing-gcp`).

## Platforms

A module of one cloud works only with that cloud's app. Every module says which it belongs to in the output `platform`, and an app refuses a backend of another platform at plan, with what to use instead:

| `platform` | Modules | Apps |
| --- | --- | --- |
| `gcp` | Every module with `gcp` in its name | `app-gcp-cloud-run` |
| `aws` | Every module with `aws` in its name | `app-aws-ecs`, `app-aws-lambda` |
| `neutral` | `db-external`, `db-neon`, `mail-smtp`, `blob-s3`, `blob-cloudflare-r2`, `kv-external`, `ingress-cloudflare-*`, `stream-valkey`, `stream-postgres`, `queue-db` | Every app |

A network has a platform too, and every module that takes one refuses another platform's: [The network contract](#the-network-contract).

Mixing them would take identity bridges between clouds, and more combinations that nobody has applied. A store outside the platform is still a neutral module away: on Cloud Run, `blob-s3` with keys takes the place of `blob-aws-s3`.

**The one exception** is `billing-aws` with `web_identity = true` on `app-gcp-cloud-run`, since reading another cloud's costs is only an outside API call: [The billing modules](#the-billing-modules) has how. The validation of `backends` in that app is the only place that allows it. `billing-gcp` has no such exception, so a server on AWS does not import Google Cloud's costs through these modules.

**`queue-db` needs a process that stays up**, so being neutral does not make it fit everywhere. `app-aws-ecs` takes it, and `app-aws-lambda` refuses it, since a function runs only during an invocation. `app-gcp-cloud-run` refuses it unless `min_instances` is at least 1 and `cpu_always_allocated` is `true`, which bills an instance all the time. Each refusal covers a deployment that names no queue, since `db` is the server's default.

`scripts/task.sh tofu:backend-fields` fails when a module has no `platform`, or one that its name contradicts.

## The backend contract

Every `db-*`, `queue-*`, `blob-*`, `kv-*`, `stream-*`, `mail-*`, `secrets-*`, `kek-*`, `billing-*`, and `monitoring-*` module returns the same outputs, so that changing the database means
changing one module block:

| Output | Type | Meaning |
| --- | --- | --- |
| `platform` | `string` | `gcp`, `aws`, or `neutral`: read [Platforms](#platforms). The one field that every backend must have, also one that names its outputs one by one |
| `env` | `map(string)` | Settings by variable name (`MY_DATABASE`) |
| `secret_env` | `map(string)` | Secret settings: variable name to a reference to a secret in the platform's store. The reference is the store's own: a secret ID in Secret Manager, a full ARN in AWS Secrets Manager. How the value reaches the server is the app module's business |
| `secret_files` (optional) | `map(string)` | Secret settings to mount as files: a variable name ending in `_FILE` to the secret's reference. The app sets the variable to the path, on the service and every job. A platform with no mounted secrets treats it as `secret_env` of the name without `_FILE` |
| `permissions` | `list(object({ permission, resource, ... }))`, with `scope` on Google Cloud and `conditions` on AWS | What the app's identity is permitted |
| `direct_secret_env` (optional) | `map(string)` | What the one-off work takes over a setting of `secret_env`, which the service keeps: `MY_DATABASE_URL` as a direct connection when `secret_env` names a pooled one. Each name must be in `secret_env` |
| `cloud_sql_instances` (optional) | `list(string)` | Cloud SQL instances, as `<project>:<region>:<instance>`, that Cloud Run mounts at `/cloudsql` on the service and every job |
| `security_group_ids` (optional) | `list(string)` | Security groups the app joins to reach a backend in a VPC, on AWS, such as those of `db-aws-rds` and `kv-aws-elasticache` |
| `private_network` (optional) | `string` | The network on which this backend has private addresses: `network` of the shape on Google Cloud, `vpc_id` on AWS. The app must have the same network, and a plan fails otherwise. Read [The network contract](#the-network-contract) |
| `invokers` (optional) | `list(string)` | Service accounts that call the app and need permission to invoke it, on a platform where something calls it |
| `event_sources` (optional) | `map(object({ arn, visibility_timeout_seconds }))` | Queues that the platform polls to invoke the app, on a platform where the app is invoked that way |
| `billing_sources` (optional) | `list(string)` | What this module adds to the one setting `MY_BILLING_SOURCES`, which the app sets from all of them |

A permission means what the platform's IAM means: what the identity is permitted, which on Google Cloud is a role, so its shape is the platform's:

| Platform | `permission` | `scope` | `resource` | `conditions` |
| --- | --- | --- | --- | --- |
| Google Cloud | A role, such as `roles/storage.objectUser` | `project`, `bucket`, `topic`, `queue`, `service_account`, `crypto_key`, `dataset`, or `subscription`, each a different kind of binding | The resource's name. For `project`, null is the app's project, and otherwise another project's ID. For `dataset`, `projects/<project>/datasets/<dataset>`, and the permission is what an access entry holds | Not available |
| AWS | One IAM action, such as `sqs:SendMessage` | None: every grant is a statement of one policy | An ARN or an ARN pattern, or `*` | Optional: `{ test, variable, values }` of the statement |

The service and its jobs share one identity, so a grant reaches them all. The app grants each permission on that resource alone, so a
backend asks for as little as it can. Google Cloud's app keys one resource on each grant, so it reads the names of `permissions` and
`invokers` at plan time: a backend there builds them from its inputs rather than from attributes of its resources, and uses
`depends_on` on the output for the order. AWS's app makes one document of them, so a backend there may name an attribute of its
resources.

The rules:

- No secret value reaches the plan or the state, with one exception: the origin secret of an ingress, as its section says. A backend takes or creates a secret by ID, and the operator adds a value that
  comes from elsewhere by hand. What the deployment can generate itself (the KEK, a VAPID key) goes through an ephemeral
  resource and a write-only attribute, whose version number is fixed in the module, never an input: changing it writes a new
  value, and the old one cannot be had again. A value that another system also holds, which can be written again without losing anything, is the
  exception, and its version is an input: a database's password goes to the database and into the URL in one apply (`password_version`), and a value
  from a module with no write-only form goes through `write_values` of a `secrets-*` module (`write_version`). `secrets-gcp-secret-manager` exposes the IDs as `ids`; a secret made elsewhere needs only its ID. The app can read every secret in `secret_env`, `direct_secret_env`, and `secret_files` without a role for it.
- Exactly one KEK: a plan fails if the server would need one and no backend provides it (`kek` with `key_manager = local`,
  `kms_key` otherwise).
- A backend does not name the app. What it needs from the app is an input known in advance, such as `host`, so nothing
  is circular: the queue calls the service's `run.app` URL, built from the service's name, and its service account exists before the service does.
- A setting belongs to one place. The same setting named by two backends, by `env` and `secret_env`, by what the app
  module sets itself, or as both `X` and `X_FILE`, fails the plan and lists the names. `merge()` alone would let the last
  one win without a word. `MY_BILLING_SOURCES` is one setting that lists what several modules read, so no module names it: each
  returns `billing_sources`. The same source twice, or the setting in `env` beside them, fails the same way.

Anything without a backend (VAPID, CAPTCHA, and every other setting) goes through the `env` and
`secret_env`, and `secret_files` inputs of the app module, so no setting is out of reach. A backend of your own needs nothing special: any
object with `platform` and these fields can go in `backends`.

**An app takes every field of these tables and either uses it or refuses it**, because OpenTofu drops an unknown field without a word when it converts a module's
outputs to the type of `backends`, and a backend's settings or security groups would vanish while the plan passes. So a field of another platform fails the plan, and
`scripts/task.sh tofu:backend-fields` fails when the tables and the type of `backends` of an app disagree, or when a module returns an output that is neither here nor listed there as being for the operator.

### The database's two connections

A database has a direct connection, and may have a pooled one. `my schema apply` holds a session-level lock and `LISTEN` belongs to
a session, so neither can go through a pooler in transaction mode, while the service gets the most from one, and works through one
that handles protocol-level prepared statements: [Through a connection pooler](../../docs/operations/database.md#through-a-connection-pooler). So:

| Who | `MY_DATABASE_URL` | Comes from |
| --- | --- | --- |
| The service | The pooled connection when there is one, otherwise the direct one | `secret_env` |
| The migration job and the console | Always the direct one | `direct_secret_env`, over `secret_env` |

A `db-*` module with a pooled connection also returns `MY_STREAM_POSTGRES_URL`, the direct one, in `secret_env`, for `stream = postgres`.
Which module has what:

| Module | Direct | Pooled |
| --- | --- | --- |
| `db-external` | `url_secret_id`, required | `pooled_url_secret_id`, optional |
| `db-neon` | `connection_uri` | `connection_uri_pooler` |
| `db-gcp-cloud-sql` | The connector's socket, or the private address with a network | None: its pooling needs the Enterprise Plus edition |
| `db-aws-rds` | The instance's endpoint | None: RDS Proxy pins on what the server does. Its README has why |

The module list above names the database modules; each README says what stays unverified, and what moving your data takes. **Changing the `db` block is one block in the
configuration and a data migration in practice**: a new database is empty.

## The ingress contract

An `ingress-*` module decides how traffic reaches the app. It is a backend, with these outputs on top of the usual ones:

| Output | Type | Meaning |
| --- | --- | --- |
| `trusted_proxies` | `list(string)` | What this ingress puts in front of the platform. The app module adds what the platform itself puts in front of the server (Cloud Run's `169.254.0.0/16`) and sets `MY_TRUSTED_PROXIES` from both, so that the server finds the client's address in `X-Forwarded-For` and the ingress does not need to know the platform |
| `service_ingress` | `string` or null | What the ingress needs of the service: `all` when its traffic comes from the internet, `internal-and-load-balancer` for a load balancer of Cloud Run, `load-balancer` for one that the service attaches to, as on ECS. It must agree with what the app can do, and a disagreement fails the plan. Null for a module that needs nothing, such as a media host |
| `dns_records` | `list(object({ name, type, rrdata }))` | What to add at the DNS provider |
| `load_balancer` (optional) | `object({ target_group_arn, security_group_id })` | What the service attaches to, on a platform where the app registers with its load balancer rather than the load balancer naming the app. An app with no way to attach refuses the ingress by its `service_ingress` |

It takes the service's name from the app (`service_name = module.app.service_name`) while the app takes its settings, which
is no cycle as long as `platform`, `env`, `service_ingress`, and `trusted_proxies` do not depend on anything that reads the service's name.
`dns_records` may, so `backends` names those four outputs rather than the whole module:

```hcl
{ platform = module.ingress.platform, env = module.ingress.env, service_ingress = module.ingress.service_ingress, trusted_proxies = module.ingress.trusted_proxies },
```

A module that only adds a host, such as `ingress-cloudflare-media`, returns no `trusted_proxies` and no `service_ingress`, so
it sits beside any other. A Cloudflare zone has one ruleset per phase, so two modules in one zone share them: one owns them, and
takes the other's `rules` as `extra_rules`.

Swapping one for another changes its block, and `service_ingress` of the app if the new one needs another setting, which the plan says. `service_ingress` of Cloud Run's app is
required and has no default, so nothing opens a service by accident: [its README](modules/app-gcp-cloud-run/README.md#who-may-reach-the-service) has what each setting means. Neither AWS app has such an input, because each has one setting that works:
a Function URL accepts `all`, and ECS accepts `load-balancer`, and each refuses the others. An ingress may also be left out, for a deployment that uses the
service's own URL or has something else in front. Remove the block and that line: `MY_TRUSTED_PROXIES` is then only what the platform puts in front, or what `env` of the app says. Setting
it in `env` beside an ingress that names proxies fails the plan, and so do two ingresses that both name them.

An ingress closes the origin in one of three ways: on the platform (`ingress-gcp-load-balancer`, `ingress-aws-alb`), with a header that the server checks (`ingress-cloudflare-app`, `ingress-aws-cloudfront`), or not at all (`ingress-gcp-domain-mapping`). A module that sets the header returns `MY_ORIGIN_SECRET_HEADER` in `env` and `MY_ORIGIN_SECRET` by reference in `secret_env`, and takes the value as a sensitive input.
**That value lands in the state**: neither provider has a write-only form of it. So this is the one place that breaks the rule below that no secret value reaches the state, and the state must be kept private. Two modules must not name those settings twice, so `ingress-cloudflare-media` beside `ingress-cloudflare-app` leaves them out and comes through `extra_rules`.

## The network contract

A `network-*` module is **no backend**: it returns no settings, it goes in no `backends`, and an app's `backends` would drop what it returns. It is an input of the others, so that one deployment has one private network it controls:

```hcl
module "network" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/network-gcp-vpc?ref=v0.1.0&depth=1"
  # ...
}

module "db" {
  # ...
  network = module.network
}
```

**A network depends on nothing else, and the others only read it**, so nothing is circular, and the order follows from the references: its outputs wait for what they promise, so a database that reads `private_services_range` waits for the connection. A module takes `network` and no network of its own.

**A VPC you have already is the same object, with no module.** Write out the fields of your platform below and pass it where `module.network` would go. Nothing checks your VPC against what you say of it, so what you say of it must be true: a field you leave out is what a module that needs it refuses at plan.

A plan fails, with what to use instead:

- when a network of another platform reaches a module, by `platform`;
- when a field that a module needs is missing or in the wrong form;
- when a backend's `private_network` is not the app's network. On AWS an ID may be unknown until the apply that creates it, and the comparison waits for then.

### The network on Google Cloud

| Field | Type | Meaning |
| --- | --- | --- |
| `platform` | `string` | `gcp` |
| `network` | `string` | `projects/<project>/global/networks/<name>` |
| `subnetwork` | `string` | `projects/<project>/regions/<region>/subnetworks/<name>`, in the app's region, of `/26` or larger: what the service and the jobs attach to |
| `private_services_range` | `string` | Name of a reserved range with its connection to `servicenetworking.googleapis.com` made, of `/24` or larger, for `db-gcp-cloud-sql`. Null where you have none, which only that module refuses |
| `memorystore_connection_policy` | `bool` | Whether the VPC has a service connection policy of the class `gcp-memorystore` in this region, for `kv-gcp-memorystore`, which refuses `false` |
| `nat` | `bool` | Whether the subnet reaches the internet through Cloud NAT |

Keep every range in RFC 1918: an app with egress for private ranges sends only those through the VPC.

### The network on AWS

| Field | Type | Meaning |
| --- | --- | --- |
| `platform` | `string` | `aws` |
| `vpc_id` | `string` | The VPC |
| `vpc_cidr` | `string` | Its primary range, which the load balancer's group allows out to reach the tasks |
| `public_subnet_ids` | `list(string)` | Subnets with a route to an internet gateway: the load balancer's, in two zones or more, and the tasks' with `assign_public_ip` |
| `public_subnet_cidrs` | `list(string)` | The range of each of those, in the same order, which is what the server trusts behind the load balancer |
| `private_subnet_ids` | `list(string)` | Subnets for RDS, in two zones or more, ElastiCache, Lambda, and the tasks without `assign_public_ip` |
| `nat` | `bool` | Whether the private subnets reach the internet, through a NAT gateway or anything else that does it |

### Who takes it

| Module | With a network | With none |
| --- | --- | --- |
| `app-gcp-cloud-run` | Direct VPC egress of the service and the jobs | Attaches nothing: enough for everything on public endpoints |
| `db-gcp-cloud-sql` | A private address alone, reached over TCP | A public address that only the connector reaches, which needs no VPC |
| `kv-gcp-memorystore` | Required | Refused: only Private Service Connect reaches it |
| `app-aws-ecs` | Required: a task is always in a VPC | Refused |
| `app-aws-lambda` | Every function attaches to the private subnets | Attaches nothing: enough for everything on public endpoints |
| `db-aws-rds`, `kv-aws-elasticache`, `ingress-aws-alb` | Required | Refused: they live in a VPC |

Nothing else takes one. `ingress-gcp-load-balancer` reaches the service through a serverless network endpoint group, and `ingress-aws-cloudfront` through its Function URL, so neither has an address in a VPC.

### The way out to the internet

An instance that runs jobs must reach anyone, since federation does, so **a plan refuses every setting where it could not**:

| App | With `nat` of the network | With it off |
| --- | --- | --- |
| `app-gcp-cloud-run` with `vpc_egress = PRIVATE_RANGES_ONLY`, the default | Not used | Fine: only private ranges go into the VPC, and public destinations keep Cloud Run's own path |
| `app-gcp-cloud-run` with `vpc_egress = ALL_TRAFFIC` | Through Cloud NAT | **Refused** |
| `app-aws-ecs` with `assign_public_ip = true` | Not used: a public subnet, with the task's own address | Fine |
| `app-aws-ecs` with `assign_public_ip = false` | A private subnet, through the gateway | **Refused** |
| `app-aws-lambda` with a network | A private subnet, through the gateway | **Refused**: an attached function has no public address, and a public subnet does not give it one |

`nat` of a VPC of yours is what you say of it: set it when the private subnets, or the subnet of Cloud Run, reach the internet by whatever means. Nothing reads the routes.

Google's guide for Direct VPC egress says that `all-traffic` sends everything through the VPC and that resources need Private Google Access for Google's APIs, which `network-gcp-vpc` sets, and it does not say in so many words that the internet then takes Cloud NAT. That follows from a subnet whose instances have no public address, and it is why the plan requires it. `ALL_TRAFFIC` with the connector of Cloud SQL is refused too: the same guide says to use a private address when all egress goes through the VPC.

### What stays closed

The server's outbound HTTP to destinations that remote servers influence, such as federation and link previews, goes through a client that refuses private and link-local addresses at connection time, so a network adds no way for a remote server to reach what is on it. `http_allow_private_networks` turns that off, and it is refused in the `production` profile, and an app on a network refuses it at plan. The database and Valkey clients do not go through that client, since their addresses come from the operator, so a private address works for them.

## The billing modules

`billing-gcp` works on Google Cloud's app, and `billing-aws` on either platform's: on Cloud Run it is [the one exception](#platforms). Both can sit in the `backends` of Cloud Run. Each module uses
the provider of the cloud it reads and no other, and takes what it needs of the other cloud as a string, so the root
configuration needs the other provider only when it has that block. A permission on AWS's resource cannot go through
`permissions` of Cloud Run's app, so the module holds it itself:

| Block | App | Provider of the block | What holds the permission |
| --- | --- | --- | --- |
| `billing-aws` | AWS | `aws`, which the app has | The app's role, through `permissions` |
| `billing-aws` with `web_identity` | Google Cloud | `aws`, on top of the app's `google` | A role of this module, which trusts the app's service account |
| `billing-gcp` | Google Cloud | `google`, which the app has | The app's service account, through `permissions` |

`billing-aws` on Cloud Run takes `module.app.service_account_unique_id`, so name its outputs in `backends` rather than the whole
module, as with an ingress. Its `env` is built from names, so it does not wait for the role. Without `web_identity` it asks the app for a permission, and Cloud Run's app refuses it.

Neither module has ever been applied, and turning on a billing export or a budget stays a step in the Console: [each README](modules/billing-gcp/README.md#what-stays-manual) says which, and [the guide](../../docs/operations/billing-import.md) what the import does with them.

## The monitoring contract

A `monitoring-*` module is a backend: it returns `env`, with the settings of the metrics that its alerts read, so that a setting of them elsewhere fails the plan. It calls the service from outside and reads what the platform keeps, so it asks for no `permissions`, and it takes `host` as an input rather than an output of the app. It takes `notification_emails`, more channels of the platform as IDs, and the period of its uptime check, since each check wakes what scales to zero. Its alerts carry the names of [the runbook](../../docs/operations/alerts.md), and its README lists each alert of the runbook as covered, covered differently, or not expressible. No alert may fire on the absence of a metric that only traffic produces.

## The app contract

Every `app-*` module takes `backends`, `env`, `secret_env`, `secret_files`, `image`, and `host`, provides a console, and returns
`origin_host`, the host name its service answers on, for an ingress that takes the origin as an input. It is null on a platform
where the service has no host of its own, such as ECS, whose tasks each have another address: an ingress that needs a host then fails the plan.

### Private networking

Most backends are reached over the internet with the platform's own identity, and need no network of their own. A backend that has only private addresses says so through `backends`, and the backend still does not name the app:

| Platform | Field | What the app does | Returned by |
| --- | --- | --- | --- |
| Google Cloud, AWS | `private_network` | Fails the plan unless its own `network` is that one | `kv-gcp-memorystore`, `db-gcp-cloud-sql` with a network, `db-aws-rds`, `kv-aws-elasticache` |
| ECS | `security_group_ids` | The service and every one-off task join each group | `db-aws-rds`, `kv-aws-elasticache` |
| Lambda | `security_group_ids` | Every function joins each group | `db-aws-rds`, `kv-aws-elasticache` |

The server has one Valkey setting for `kv` and `stream`, so `MY_VALKEY_URL` has one owner: `url_secret_id` of `stream-valkey` is
null beside a `kv-*` module. Both managed Valkey modules bill while the server is idle, and each README says what it costs.

### The console

A console runs one `my` command as a one-off, with the same image, settings, and secrets as the app, for tasks such as
creating the first administrator.

- The input `console_enabled`, off by default, turns it on for as long as it is needed, so that nothing extra that can
  read the secrets stays around.
- The output `console` is null while it is off. Otherwise it is an object with a `platform`, and whatever else that
  platform needs.
- [`console.sh`](console.sh) is the one way to use it. It reads that output on standard input and takes the `my` command
  as its arguments, waits for it, prints its output, and exits with its exit code. A new platform adds one function
  there, chosen by `platform`.
- The command has no standard input and no terminal, on every platform. Anything it prints stays wherever the platform
  keeps output, so a command that needs a secret must not take it from there or print it casually:
  `my admin create -password-link` exists for this.

```sh
tofu output -json console | deploy/opentofu/console.sh admin create -username alice -email alice@example.com -password-link
```

### Upgrading

[`upgrade.sh`](upgrade.sh) runs the same steps in the same order on every platform: the one-off migration takes the new image, then
`config check`, `db upgrade -check`, `schema apply`, `db upgrade`, and only then the service. Each platform is a few functions there,
chosen by `--platform`, which is `gcp-cloud-run` unless you say otherwise. What holds a bad image back is the platform's: Cloud Run
gives a revision no traffic until `/readyz` answers, ECS goes back when a new task fails its load balancer's check, and Lambda has nothing like that.

## AWS: unverified

The modules for AWS test whether this contract holds on a second platform, in two shapes. **None has ever been applied**, so use Google Cloud unless you mean to be the first.

`app-aws-ecs` runs the released image as it is, with its secrets from ECS and its one-off commands as tasks, so nothing is known to be missing: `modules/app-aws-ecs/README.md` says what stays unverified and what it costs. Unlike the others it costs money while idle.

**A deployment cannot run on `app-aws-lambda` yet.** `modules/app-aws-lambda/README.md` says in detail what is missing, which comes to:

- the server does not read a secret from Secrets Manager, and Lambda delivers none itself;
- no released image runs on Lambda: it needs a single-architecture build with the Lambda Web Adapter, in ECR;
- `my` has no way to run one command in an invocation, which the migration and the console need.

| Module | Replaces on AWS |
| --- | --- |
| `app-aws-lambda`, `app-aws-ecs` | `app-gcp-cloud-run` |
| `db-aws-rds` | `db-gcp-cloud-sql` |
| `queue-aws-sqs` (Lambda), `queue-db` (ECS) | `queue-gcp-cloud-tasks` |
| `blob-aws-s3` | `blob-gcp-gcs` |
| `stream-aws-sns` | `stream-gcp-pubsub` |
| `mail-aws-ses` | Beside `mail-smtp`, which works unchanged |
| `secrets-aws-secrets-manager` | `secrets-gcp-secret-manager` |
| `kek-aws-secrets-manager`, `kek-aws-kms` | `kek-gcp-secret-manager`, `kek-gcp-kms` |
| `ingress-aws-cloudfront` (Lambda), `ingress-aws-alb` (ECS) | `ingress-gcp-domain-mapping`, `ingress-gcp-load-balancer` |
| `billing-aws` | `billing-gcp`, and it also runs on Google Cloud's app: read [The billing modules](#the-billing-modules) |
| `monitoring-aws` | `monitoring-gcp` |

`db-external`, `db-neon`, `mail-smtp`, and `ingress-cloudflare-app` take the same blocks, with an ARN where Google Cloud has a secret's ID. The app's `backends`
tests show that a block of one platform fails the plan on the other's app. `examples/aws-lambda-neon` and `examples/aws-ecs-neon` compose them, and their tests swap each block
their comments name; those tests mock the provider. Either shape's app refuses the queue and the ingress of the other at plan time.

## Checks

`scripts/task.sh tofu:check` runs `tofu fmt -check`, `tofu init -backend=false` and `tofu validate` for each module
and the example, the tests on mocked providers, a check that every `MY_*` name here exists in the configuration, and a real
apply against a local stub of Secret Manager, which shows the formats of the generated values, that none reaches the plan or
the state, and that another apply writes nothing; `tofu:db-state-test` does the same against a stub of Cloud SQL and shows that the database user and the URL get the same password. None of them reaches a cloud, so they show that the configuration is
consistent, not that an apply on Google Cloud succeeds. Only the example
pins its providers (`.terraform.lock.hcl`). `tofu:sources` fails on a relative source or a `ref` other than `MODULE_REF`, and
`tofu:git-source-test` runs `tofu init` on a copy of the example against a local bare repository with the tag.
`tofu:aws-plan-test` plans the AWS modules that generate a value, `db-aws-rds` among them, with no credentials and no call to AWS, and shows that the plan holds nothing of it. There is no
stub of AWS and no apply on that side, so nothing has shown what the state holds beyond what the provider's schema says.
