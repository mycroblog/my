# queue-db

The job queue in the database: `queue = db`, settings only. It creates nothing, so its name has no provider part, its `platform` is `neutral`, and it follows [the backend contract](../../README.md#the-backend-contract).

**This module has never been applied.** The test only checks its outputs. It needs an app whose process stays up, which is what stays unverified: on `app-aws-ecs`, [what that module has not run](../app-aws-ecs/README.md#unverified), and on `app-gcp-cloud-run`, that jobs run as expected with the two settings below.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "queue" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/queue-db?ref=v0.1.0&depth=1"
}
```

It takes no input.

| Output | Value |
| --- | --- |
| `env` | `MY_QUEUE` = `db` |
| `platform` | `neutral` |
| `secret_env`, `permissions` | Empty |

## What it needs

**A process that stays up.** A process with `role = all` or `worker` polls the queue table and runs the periodic jobs itself, so nothing calls the server for its jobs. This is what makes it a fit for `app-aws-ecs`, and why an app that can stop the process between requests refuses it at plan:

| App | Takes it | Why |
| --- | --- | --- |
| `app-aws-ecs` | Yes | One process stays up |
| `app-gcp-cloud-run` | Only with `min_instances` of at least 1 and `cpu_always_allocated = true` | By default instances scale to zero, with CPU only during requests, so jobs would run only while a request happens to be open. Read below, or use `queue-gcp-cloud-tasks` |
| `app-aws-lambda` | No | A function runs only during an invocation, and nothing invokes it for this queue. Use `queue-aws-sqs` |

A refusal covers `queue = db` however it is set, and also a deployment that names no queue, since that is the server's default.

**On Cloud Run it takes both settings.** `min_instances` keeps an instance running, and `cpu_always_allocated` gives it CPU outside requests, which is instance-based billing. Each alone fails the plan: with request-based billing, Cloud Run allocates CPU only during request processing, however many instances it keeps, and with instance-based billing alone it can still remove an instance that no traffic needs. With both, an instance is billed at the full rate all the time. Minimum instances are a best-effort target and Cloud Run may restart one at any time, so jobs can wait for a replacement; a job that was running is picked up again when its lease expires. Read [Google's billing settings](https://docs.cloud.google.com/run/docs/configuring/billing-settings) and [minimum instances](https://docs.cloud.google.com/run/docs/configuring/min-instances).

```hcl
module "app" {
  # ...
  min_instances        = 1
  cpu_always_allocated = true
}
```

**A database that may stay awake.** The process polls about once a second while no job is due, so a serverless database that suspends when idle, such as Neon, does not suspend. Budget for it to run all month: on Neon's Free plan its compute hours may run out, and [the figures of the Cloud Run guide](../../../../docs/operations/deploy-cloud-run.md#costs), which assume that it can rest, do not apply. Check [Neon's pricing](https://neon.com/pricing); the numbers for this case have not been worked out here.

Several processes can share it, each taking jobs from the table, and the periodic jobs run once for the whole instance. For what it takes to split job types among them, read [Running](../../../../docs/operations/running.md).
