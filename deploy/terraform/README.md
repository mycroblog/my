# OpenTofu modules

Modules that compose a deployment of `my` from swappable backends. They are written for [OpenTofu](https://opentofu.org)
and work with Terraform too, from version 1.11 of either. To deploy, follow [Deploying on Cloud Run](../../docs/operations/deploy-cloud-run.md);
this file is the contract for whoever uses or adds a module.

```text
deploy/terraform/
  modules/app-gcp-cloud-run/      service, migration job, console, service account, public access
  modules/queue-gcp-cloud-tasks/  queue, Scheduler tick, the service account that calls the server
  modules/blob-gcp-gcs/           media bucket
  modules/stream-gcp-pubsub/      topic and the permissions for the server's own subscriptions
  modules/db-external/            a connection URL in Secret Manager: Neon, or any PostgreSQL
  modules/mail-smtp/              email through any SMTP service
  modules/ingress-gcp-domain-mapping/  traffic through a Cloud Run domain mapping
  modules/ingress-gcp-load-balancer/   traffic through a global external Application Load Balancer
  modules/ingress-cloudflare-app/      traffic through Cloudflare, for any origin
  modules/ingress-cloudflare-media/    a media host through Cloudflare, beside any ingress
  modules/secrets-gcp-secret-manager/  secrets to fill by hand, or a VAPID key it generates
  modules/kek-gcp-secret-manager/      the KEK it generates as a secret, mounted as a file
  modules/kek-gcp-kms/                 the KEK in Cloud KMS
  examples/gcp-cloud-run-neon/    all of the above, composed, on Git sources at MODULE_REF
  upgrade.sh                      migration job first, then the service
  console.sh                      runs one my command through an app module's console
```

## Using a module

A module is a Git source pinned to a release. A pin is required: an unpinned module could change the KEK's resources under you.

```hcl
module "db" {
  source = "git::https://github.com/mycroblog/my.git//deploy/terraform/modules/db-external?ref=v0.1.0&depth=1"
  # ...
}
```

Keep every block at the same release, the one you deploy, whether stable or a prerelease such as `v0.1.0-rc.4`. A module exists
only from the first tag that contains it, so an earlier tag has none of them. `.terraform/modules/app/deploy/terraform` holds
`upgrade.sh` and `console.sh` after `tofu init`, at that release. `examples/gcp-cloud-run-neon` works when copied to an empty
directory with its lock file: [the guide](../../docs/operations/deploy-cloud-run.md#3-copy-the-example-and-set-the-variables)
starts that way.

In this repository, `MODULE_REF` holds the release the examples name. Inside the repository the checks test the modules of the
same commit: `scripts/task.sh terraform:example-test` runs on a copy whose sources are rewritten to local paths, and never
fetches from GitHub. To update the example's lock file, use `scripts/task.sh terraform:lock`.

## Naming

A module is named `<role>-<provider>-<service>`, so that the name says what it is for before where it runs:

| Role | What it provides |
| --- | --- |
| `app` | The server itself. It merges what the others return |
| `db` | The database |
| `queue` | The job queue and what triggers the periodic jobs |
| `blob` | The media store |
| `stream` | The broker of the real-time updates |
| `mail` | Email |
| `secrets` | Secrets, filled by hand or generated |
| `kek` | The key that encrypts the secrets in the database |
| `ingress` | How traffic reaches the app |

A module with no provider of its own drops that part (`db-external`, `mail-smtp`). The provider is what the module talks to,
not the platform behind it: `ingress-cloudflare-app` takes its origin as an input, so the same module fronts Cloud Run or AWS. Cloud SQL would be `db-gcp-cloud-sql`.

## The backend contract

Every `db-*`, `queue-*`, `blob-*`, `stream-*`, `mail-*`, `secrets-*`, and `kek-*` module returns the same outputs, so that changing the database means
changing one module block:

| Output | Type | Meaning |
| --- | --- | --- |
| `env` | `map(string)` | Settings by variable name (`MY_DATABASE`) |
| `secret_env` | `map(string)` | Secret settings: variable name to a reference to a secret in the platform's store. The reference is the store's own: a secret ID in Secret Manager, a full ARN in AWS Secrets Manager. How the value reaches the server is the app module's business |
| `secret_files` (optional) | `map(string)` | Secret settings to mount as files: a variable name ending in `_FILE` to the secret's reference. The app sets the variable to the path, on the service and every job. A platform with no mounted secrets treats it as `secret_env` of the name without `_FILE` |
| `roles` | `list(object({ role, scope, resource, conditions }))` | What the app's identity needs |
| `invokers` (optional) | `list(string)` | Service accounts that call the app and need permission to invoke it, on a platform where something calls it |
| `event_sources` (optional) | `map(object({ arn, visibility_timeout_seconds }))` | Queues that the platform polls to invoke the app, on a platform where the app is invoked that way |

A role means what the platform's IAM means, and `scope` says which kind it is, so a backend of one platform handed to the app of
another fails the plan instead of granting nothing:

| Platform | `role` | `scope` | `resource` | `conditions` |
| --- | --- | --- | --- | --- |
| Google Cloud | A role, such as `roles/storage.objectUser` | `project` (with `resource` null), `bucket`, `topic`, `queue`, `service_account`, or `crypto_key`, each a different kind of binding | The resource's name | Not available |
| AWS | One IAM action, such as `sqs:SendMessage` | `policy`: every grant is a statement of one policy | An ARN or an ARN pattern, or `*` | Optional: `{ test, variable, values }` of the statement |

The service and its jobs share one identity, so a grant reaches them all. The app grants each role on that resource alone, so a
backend asks for as little as it can. Google Cloud's app keys one resource on each grant, so it reads the names of `roles` and
`invokers` at plan time: a backend there builds them from its inputs rather than from attributes of its resources, and uses
`depends_on` on the output for the order. AWS's app makes one document of them, so a backend there may name an attribute of its
resources.

The rules:

- No secret value reaches the plan or the state. A backend takes or creates a secret by ID, and the operator adds a value that
  comes from elsewhere by hand. What the deployment can generate itself (the KEK, a VAPID key) goes through an ephemeral
  resource and a write-only attribute, whose version number is fixed in the module, never an input: changing it writes a new
  value. `secrets-gcp-secret-manager` exposes the IDs as `ids`; a secret made elsewhere needs only its ID. The app can read every secret in `secret_env` and `secret_files` without a role for it.
- Exactly one KEK: a plan fails if the server would need one and no backend provides it (`kek` with `key_manager = local`,
  `kms_key` otherwise).
- A backend does not name the app. What it needs from the app is an input known in advance, such as `host`, so nothing
  is circular: the queue calls `https://{host}`, and its service account exists before the service does.
- A setting belongs to one place. The same setting named by two backends, by `env` and `secret_env`, by what the app
  module sets itself, or as both `X` and `X_FILE`, fails the plan and lists the names. `merge()` alone would let the last
  one win without a word.

Anything without a backend (VAPID, CAPTCHA, and every other setting) goes through the `env` and
`secret_env`, and `secret_files` inputs of the app module, so no setting is out of reach. A backend of your own needs nothing special: any
object with these fields can go in `backends`.

## The ingress contract

An `ingress-*` module decides how traffic reaches the app. It is a backend, with these outputs on top of the usual ones:

| Output | Type | Meaning |
| --- | --- | --- |
| `trusted_proxies` | `list(string)` | What this ingress puts in front of the platform. The app module adds what the platform itself puts in front of the server (Cloud Run's `169.254.0.0/16`) and sets `MY_TRUSTED_PROXIES` from both, so that the server finds the client's address in `X-Forwarded-For` and the ingress does not need to know the platform |
| `service_ingress` | `string` or null | What a module of one platform asks of that platform's service. On Cloud Run, `INGRESS_TRAFFIC_ALL`, or `INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER` when only a load balancer may reach it. A module of no particular platform returns null, which leaves the service open on every platform, and an app module refuses a value that is not its platform's |
| `dns_records` | `list(object({ name, type, rrdata }))` | What to add at the DNS provider |

It takes the service's name from the app (`service_name = module.app.service_name`) while the app takes its settings, which
is no cycle as long as `env`, `service_ingress`, and `trusted_proxies` do not depend on anything that reads the service's name.
`dns_records` may, so `backends` names those three outputs rather than the whole module:

```hcl
{ env = module.ingress.env, service_ingress = module.ingress.service_ingress, trusted_proxies = module.ingress.trusted_proxies },
```

A module that only adds a host, such as `ingress-cloudflare-media`, returns no `trusted_proxies` and no `service_ingress`, so
it sits beside any other. A Cloudflare zone has one ruleset per phase, so two modules in one zone share them: one owns them, and
takes the other's `rules` as `extra_rules`.

Swapping one for another changes only the module block. An ingress may also be left out, for a deployment that uses the
service's own URL or has something else in front. Remove the block and that line: the service is then open
(`INGRESS_TRAFFIC_ALL`), and `MY_TRUSTED_PROXIES` is only what the platform puts in front, or what `env` of the app says. Setting
it in `env` beside an ingress that names proxies fails the plan, and so do two ingresses that ask for different settings.

## The app contract

Every `app-*` module takes `backends`, `env`, `secret_env`, `secret_files`, `image`, and `host`, provides a console, and returns
`origin_host`, the host name its service answers on, for an ingress that takes the origin as an input.

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
tofu output -json console | deploy/terraform/console.sh admin create -username alice -email alice@example.com -password-link
```

### Upgrading

[`upgrade.sh`](upgrade.sh) runs the same steps in the same order on every platform: the one-off migration takes the new image, then
`config check`, `db upgrade -check`, `schema apply`, `db upgrade`, and only then the service. Each platform is a few functions there,
chosen by `--platform`, which is `gcp-cloud-run` unless you say otherwise. What holds a bad image back is the platform's: Cloud Run
gives a revision no traffic until `/readyz` answers, and Lambda has nothing like that.

## Checks

`scripts/task.sh terraform:check` runs `tofu fmt -check`, `tofu init -backend=false` and `tofu validate` for each module
and the example, the tests on mocked providers, a check that every `MY_*` name here exists in the configuration, and a real
apply against a local stub of Secret Manager, which shows the formats of the generated values, that none reaches the plan or
the state, and that another apply writes nothing. None of them reaches a cloud, so they show that the configuration is
consistent, not that an apply on Google Cloud succeeds. Only the example
pins its providers (`.terraform.lock.hcl`). `terraform:sources` fails on a relative source or a `ref` other than `MODULE_REF`, and
`terraform:git-source-test` runs `tofu init` on a copy of the example against a local bare repository with the tag.
