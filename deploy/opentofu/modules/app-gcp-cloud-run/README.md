# app-gcp-cloud-run

The server on Cloud Run: the service, the migration job, the optional console job, the service account, and public access.
How traffic reaches the service is an [`ingress-*` module](../../README.md#the-ingress-contract).
It merges what the backends return ([the contract](../../README.md#the-backend-contract)) and follows
[the app contract](../../README.md#the-app-contract).

## Inputs

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "app" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/app-gcp-cloud-run?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `project_id`, `region` | required | Where everything goes |
| `image` | required | The image at creation, as `ghcr.io/mycroblog/my:<version>` or with a digest. `latest` is refused |
| `host` | required | `host` of the server |
| `backends` | `[]` | What each backend module returns |
| `env`, `secret_env` | `{}` | Every other setting: by value, or by secret ID |
| `secret_files` | `{}` | Secret settings to mount as files: a `_FILE` variable name to secret ID. Backends return the same, and `kek-gcp-secret-manager` delivers the KEK this way |
| `name` | `my` | The service, `<name>-migrate`, `<name>-console`, and `<name>-server` |
| `service_ingress` | required | `all`, `internal`, or `internal-and-load-balancer`: [who may reach the service](#who-may-reach-the-service) |
| `role`, `profile` | `all`, `production` | `MY_ROLE` and `MY_PROFILE`. `web` is refused: it does not serve what Cloud Tasks calls |
| `cpu`, `memory_mib` | `1`, `512` | Instance size. `GOMEMLIMIT` follows, at 90% |
| `max_instances`, `concurrency` | `3`, `80` | With `pool_max_conns` of the database URL, they bound the connections |
| `min_instances`, `cpu_always_allocated` | `0`, `false` | Instances kept running, and CPU outside requests, which is instance-based billing. `queue-db` needs both, and fails the plan without them: [its README](../queue-db/README.md#what-it-needs). With both, an instance is billed at the full rate all the time |
| `request_timeout_seconds` | `900` | Longer than the dispatch deadline of the queue |
| `job_memory_mib`, `job_timeout_seconds` | `1024`, `3600` | Size and limit of the jobs |
| `console_enabled` | `false` | Creates the console job |
| `network` | `null` | Attaches the service and the jobs through Direct VPC egress: a network from [`network-gcp-vpc`](../network-gcp-vpc/README.md), or an object of [its shape](../../README.md#the-network-on-google-cloud). A backend on private addresses must be on the same one. Null attaches nothing, which is all that a deployment on public endpoints needs |
| `deletion_protection` | `true` | Refuses to delete the service and the jobs |

## Outputs

| Output | Meaning |
| --- | --- |
| `console` | For `console.sh`, or null while the console is off |
| `service_name`, `service_uri`, `service_account_email`, `service_account_unique_id`, `migration_job` | What was created |
| `service_ingress` | Who may reach the service, as the input says |
| `origin_host` | Host name of the URL Cloud Run gave the service |
| `env`, `secret_env` | The merged settings of the service |
| `one_off_secret_env` | The secret settings of the jobs: `secret_env` with `direct_secret_env` over it |
| `subnetwork` | The subnet that the service and the jobs attach to, or null |

## Who may reach the service

`service_ingress` is required, so that nothing opens the service by default. An ingress module says what it needs, and a disagreement fails the plan.

| In front | `service_ingress` | Can anyone reach `run.app`? | The client address |
| --- | --- | --- | --- |
| `ingress-gcp-load-balancer` | `internal-and-load-balancer` | No: refused from the internet | From `X-Forwarded-For`, past the load balancer's address. A direct caller has no way in |
| `ingress-gcp-domain-mapping` | `all` | **Yes: the origin stays open.** A mapping cannot add a header | From `X-Forwarded-For`, past Cloud Run's front end, the same on either URL. An assumption, as below: check the first request logs |
| `ingress-cloudflare-app` | `all` | They can connect, and the server refuses them with a `403`: **closed by the header** that Cloudflare adds | Trusts Cloudflare's ranges, and only what carries the header gets that far, so a caller's own Cloudflare Worker cannot forge the address |
| Nothing | Your choice: `all` for a public instance | **Yes**, with `all` | As with the domain mapping |
| Nothing | `internal` | No | No client from the internet at all: federation and browsers cannot reach it, so this is not for a public instance |

Cloud Tasks and Cloud Scheduler reach the service under every setting, when they are in the same project and call the default `run.app` URL.

On the rows that say yes, nothing closes `run.app`. For the header, the value is in the state: read the module's README.

## What it does

- **It keeps the service's default `run.app` URL on**, because Cloud Tasks and Cloud Scheduler call it. Do not disable it: jobs stop.
- Without an ingress, `MY_TRUSTED_PROXIES` is `169.254.0.0/16`, taken to be Cloud Run's front end. A backend or `env` that names `MY_TRUSTED_PROXIES` replaces that. No document we have read says what Cloud Run adds to `X-Forwarded-For` for a direct request or what peer address a container sees, and the container contract says nothing about either, so `169.254.0.0/16` is an assumption. The external load balancer's documented form is `[<supplied>,]<client-ip>,<lb-ip>`.
- Request-based billing with min instances 0 unless those inputs say otherwise, a `/readyz` startup probe, and a `/healthz` liveness probe. The startup probe
  keeps a revision from taking traffic until the schema and the data upgrade steps of its release are in place.
- A plan fails without a KEK when the server needs one: `kek` with `key_manager = local`, or `kms_key` otherwise.
- A backend's `cloud_sql_instances` are mounted at `/cloudsql` on the service and every job, for `db-gcp-cloud-sql`. A backend's `direct_secret_env` goes to the jobs only.
- The service account can read every secret in `secret_env`, `direct_secret_env`, and `secret_files`, and holds each permission of the backends on
  the resource it names. Each `invokers` entry gets `roles/run.invoker`, and so does `allUsers`, because federation needs
  public paths. The server checks the tokens of `/_internal/*` itself.
- `apply` never changes the image of what exists: it would skip the migration. [`upgrade.sh`](../../upgrade.sh) does.
- The migration and console jobs share one resource (`google_cloud_run_v2_job.one_off`), so they cannot drift from each
  other. Each has no retries, since `my` steps are for an operator to look at when they fail.
