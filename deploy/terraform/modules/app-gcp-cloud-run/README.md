# app-gcp-cloud-run

The server on Cloud Run: the service, the migration job, the optional console job, the service account, and public access.
It merges what the backends return ([the contract](../../README.md#the-backend-contract)) and follows
[the app contract](../../README.md#the-app-contract).

## Inputs

| Input | Default | Meaning |
| --- | --- | --- |
| `project_id`, `region` | required | Where everything goes |
| `image` | required | The image at creation, as `ghcr.io/mycroblog/my:<version>` or with a digest. `latest` is refused |
| `host` | required | `host` of the server |
| `backends` | `[]` | What each backend module returns |
| `env`, `secret_env` | `{}` | Every other setting: by value, or by secret ID |
| `secret_files` | `{}` | Secret settings to mount as files: a `_FILE` variable name to secret ID. Backends return the same, and `kek-gcp-secret-manager` delivers the KEK this way |
| `name` | `my` | The service, `<name>-migrate`, `<name>-console`, and `<name>-server` |
| `role`, `profile` | `all`, `production` | `MY_ROLE` and `MY_PROFILE`. `web` is refused: it does not serve what Cloud Tasks calls |
| `trusted_proxies` | `169.254.0.0/16` | `MY_TRUSTED_PROXIES`, the addresses of Cloud Run's front end |
| `cpu`, `memory_mib` | `1`, `512` | Instance size. `GOMEMLIMIT` follows, at 90% |
| `max_instances`, `concurrency` | `3`, `80` | With `pool_max_conns` of the database URL, they bound the connections |
| `request_timeout_seconds` | `900` | Longer than the dispatch deadline of the queue |
| `job_memory_mib`, `job_timeout_seconds` | `1024`, `3600` | Size and limit of the jobs |
| `console_enabled` | `false` | Creates the console job |
| `domain_mapping` | `false` | Maps `host` to the service with a Cloud Run domain mapping |
| `deletion_protection` | `true` | Refuses to delete the service and the jobs |

## Outputs

| Output | Meaning |
| --- | --- |
| `console` | For `console.sh`, or null while the console is off |
| `service_name`, `service_uri`, `service_account_email`, `migration_job` | What was created |
| `dns_records` | The records of the domain mapping |
| `env`, `secret_env` | The merged settings |

## What it does

- Request-based billing with min instances 0, a `/readyz` startup probe, and a `/healthz` liveness probe. The startup probe
  keeps a revision from taking traffic until the schema and the data upgrade steps of its release are in place.
- A plan fails without a KEK when the server needs one: `kek` with `key_manager = local`, or `kms_key` otherwise.
- The service account can read every secret in `secret_env` and `secret_files`, and holds each role of the backends on
  the resource it names. Each `invokers` entry gets `roles/run.invoker`, and so does `allUsers`, because federation needs
  public paths. The server checks the tokens of `/_internal/*` itself.
- `apply` never changes the image of what exists: it would skip the migration. [`upgrade.sh`](../../upgrade.sh) does.
- The migration and console jobs share one resource (`google_cloud_run_v2_job.one_off`), so they cannot drift from each
  other. Each has no retries, since `my` steps are for an operator to look at when they fail.
