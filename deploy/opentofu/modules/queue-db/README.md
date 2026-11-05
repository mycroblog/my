# queue-db

The job queue in the database: `queue = db`, settings only. It creates nothing, so its name has no provider part, and it follows [the backend contract](../../README.md#the-backend-contract).

**This module has never been applied.** The test only checks its outputs. It needs an app whose process stays up, which is what stays unverified: on `app-aws-ecs`, [what that module has not run](../app-aws-ecs/README.md#unverified).

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
| `secret_env`, `permissions` | Empty |

## What it needs

**A process that stays up.** A process with `role = all` or `worker` polls the queue table and runs the periodic jobs itself, so nothing calls the server for its jobs. This is what makes it a fit for `app-aws-ecs`, and a poor one for the platforms that stop a process between requests:

| App | Fits | Why |
| --- | --- | --- |
| `app-aws-ecs` | Yes | One process stays up |
| `app-gcp-cloud-run` | No | With the module's settings CPU is billed only during requests and instances scale to zero, so jobs run only while a request happens to be open. Use `queue-gcp-cloud-tasks` |
| `app-aws-lambda` | No | A function runs only during an invocation, and nothing invokes it for this queue. Use `queue-aws-sqs` |

The plan does not refuse it on those two: it is a setting the server accepts, and whether jobs run depends on what else keeps the process alive.

**A database that may stay awake.** The process polls about once a second while no job is due, so a serverless database that suspends when idle, such as Neon, does not suspend. Budget for it to run all month: on Neon's Free plan its compute hours may run out, and [the figures of the Cloud Run guide](../../../../docs/operations/deploy-cloud-run.md#costs), which assume that it can rest, do not apply. Check [Neon's pricing](https://neon.com/pricing); the numbers for this case have not been worked out here.

Several processes can share it, each taking jobs from the table, and the periodic jobs run once for the whole instance. For what it takes to split job types among them, read [Running](../../../../docs/operations/running.md).
