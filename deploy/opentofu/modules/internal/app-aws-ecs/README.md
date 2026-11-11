# app-aws-ecs

The server on Amazon ECS with Fargate: one service whose process stays up, and one-off tasks for the migration and the console. It follows [the app contract](../README.md#the-app-contract).

**This module has never been applied.** The tests mock the provider and the scripts' tests stub the `aws` command, so they show that the configuration and the steps are consistent, not that AWS accepts them. What stays unverified is [listed below](#unverified). Nothing is known to be missing: the released image runs here as it is, ECS delivers the secrets, and `my` runs one command as a one-off task.

**It costs money while idle**, about $42 a month in Tokyo at the smallest useful shape, before the database. [What it costs](#what-it-costs) has the parts. Cloud Run costs nothing while nobody uses it; this shape does not scale to zero.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "app" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/internal/app-aws-ecs?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `image` | required | Server image, with a version or a digest. Every task definition takes it, and with `migration` a new one is [an upgrade](#upgrading) |
| `migration` | null | `script`, the path of `migrate.sh`; `version`, the version applied; and `allow_destructive_for`. Null runs no migration, and the services take a new image at once |
| `host` | required | Public domain |
| `network` | required | Where the tasks run: a network from [`network-aws-vpc`](../network-aws-vpc/README.md), or an object of [its shape](../README.md#the-network-on-aws). A backend in a VPC must be on the same one |
| `assign_public_ip` | required | Whether each task has a public address, in a public subnet, or none, in a private one. Read [The network](#the-network) |
| `architecture` | `x86_64` | Or `arm64`. The released image has both |
| `name` | `my` | Name of the cluster, the service, and its family |
| `profile` | `production` | `profile` of the server |
| `backends`, `env`, `secret_env`, `secret_files` | empty | [The backend contract](../README.md#the-backend-contract). `secret_files` is treated as `secret_env`. The module sets `MY_BILLING_SOURCES` from what `backends` returns as `billing_sources` |
| `cpu`, `memory_mib` | `256`, `1024` | Size of a service task, from [the sizes Fargate offers](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task_definition_parameters.html). A size it does not offer fails the apply. `GOMEMLIMIT` is 90% of the memory |
| `desired_count` | `1` | Tasks of the service. Above 1 needs a stream other than `hub`, which the plan checks |
| `job_cpu`, `job_memory_mib` | `512`, `1024` | Size of a one-off task, billed only while it runs |
| `health_check_grace_seconds` | `120` | How long a new task may fail the load balancer's check |
| `console_enabled` | `false` | Creates the console's task definition |
| `log_retention_days` | `30` | CloudWatch Logs |
| `workers` | `{}` | Services that run some job types apart from the default: [the contract](../README.md#workers). Give `queue-db` the same value. Each task costs what a service task does, all the time |

[`direct_secret_env`](../README.md#the-databases-two-connections) of a backend is what the migration and the console take over a setting of `secret_env`, which the service keeps. Each name must be in `secret_env`, and a plan fails otherwise. `security_group_ids` of a backend are groups that the service and every one-off task join, beside the tasks' own, to reach it in the VPC: `db-aws-rds` and `kv-aws-elasticache` return one each.

| Output | Value |
| --- | --- |
| `service_name`, `cluster_name` | Both are `name` |
| `migration_task_definition` | The family of the migration |
| `origin_host` | Always null: read below |
| `security_group_id` | Of the tasks, for a rule of your own |
| `task_security_group_ids` | Every group a task joins: that one, and those of the backends |
| `task_subnet_ids` | The subnets of the service and the one-off tasks |
| `worker_services`, `worker_env` | The service of each worker, and what it sets over `env` |
| `execution_role_arn`, `task_role_arn` | Read [Secrets](#secrets) |
| `log_group` | `/ecs/<name>` |
| `console` | What `console.sh` reads, or null |
| `env`, `secret_env`, `one_off_secret_env` | The merged settings: the service's, and those of the one-off tasks |
| `images` | The image of the service's task definition, of each worker's, and of the migration's |
| `migration_id` | The run of the migration for the current image, or null without `migration` |

## How traffic reaches it

Only through a load balancer that the service attaches to, which `ingress-aws-alb` provides. A task's address changes with every task and it has no TLS, so **this app has no host of its own**: `origin_host` is null, and a module that takes one as its origin, such as `ingress-cloudflare-app`, fails the plan. So does `service_ingress` of anything but `load-balancer`. There is no `service_ingress` input, because there is nothing to choose.

To serve `host` from CloudFront, put [`ingress-aws-cloudfront`](../ingress-aws-cloudfront/README.md) in front of the load balancer, with `behind_cloudfront` of `ingress-aws-alb`. `backends` still names the load balancer's module alone: it returns the trust of both hops and closes itself to all but CloudFront, so nothing of this app changes.

Without an ingress the service runs with no inbound rule and only the process's own start decides whether a deployment passes. To put something else in front, add a rule to `security_group_id`, and pass the app a backend of `{ service_ingress = "load-balancer", trusted_proxies = [...], load_balancer = { target_group_arn = "...", security_group_id = "..." } }`.

## Jobs

The process runs them itself, so use `queue-db`, or leave `MY_QUEUE` at its default, which is the same. Read [what `queue-db` needs of the database](../queue-db/README.md#what-it-needs): the database never rests.

## Secrets

ECS resolves each secret into the variable the server reads when a task starts, by the `secrets` of the task definition, so **no value reaches the plan or the state**: both hold only ARNs. That makes two roles:

- The execution role starts the task: it pulls the image, writes the logs, and reads the secrets.
- The task role is what the server runs as, in the service and the one-off tasks alike, and holds what the backends ask. It cannot read the secrets, and needs no way to.

A task reads a secret once, at start. After you change one, run `aws ecs update-service --cluster my --service my --force-new-deployment`. Fargate mounts no secret, so a `_FILE` setting reaches the server as the variable without `_FILE`, as `MY_KEK` does from `kek-aws-secrets-manager`. A secret is named by its whole ARN: the module grants that ARN, so a `:json-key::` on the end does not work. A secret under a customer-managed KMS key also needs `kms:Decrypt`, which the module does not grant.

## The image

The released image runs as it is. Fargate pulls `ghcr.io/mycroblog/my` over the internet, so a task needs a way out, and the architecture you choose picks from its index. A copy in ECR works too, in the same region or another: the execution role may pull from any repository of the account.

## The first deployment

`/readyz` answers `503` until the schema exists, so the schema comes first, as on Cloud Run. Create the secrets and fill in the ones you bring, then apply everything:

```sh
tofu apply -target=module.secrets -target=module.kek
aws secretsmanager put-secret-value --secret-id my-database-url --secret-string "$DATABASE_URL"   # and the others
tofu apply
```

With `migration`, the second apply creates the migration's task definition and what it needs, runs the migration, and only then creates the services. The first command that uses the database records `host` and `account_domain` in it. Create the first administrator through [the console](#the-console).

## Upgrading

With `migration`, an apply of another image runs [the steps of every platform](../README.md#upgrading). Every family gets a new revision with the image, which nothing runs yet. `terraform_data.migrate`, which the new image replaces, runs `migrate.sh` through `local-exec`: it waits for any task of `<name>-migrate` whose desired status is `RUNNING`, then runs `my upgrade` as one task of the new revision, by its ARN, and fails the apply when the task fails, which taints the resource so that the next apply of the same image runs it again. Then each worker's service takes its revision, and the default service after them.

Each service sets `wait_for_steady_state`, so the apply waits for its deployment, and the default service waits for the workers. New tasks must pass the load balancer's check before the old ones leave, and the deployment circuit breaker takes a revision whose tasks do not start back to the one before.

The apply runs `migrate.sh` where it runs, which needs the `aws` CLI, `jq`, and `ecs:ListTasks`, `ecs:RunTask`, `ecs:DescribeTasks`, `logs:FilterLogEvents`, and `iam:PassRole` on both roles. `console.sh` also needs `ecs:DescribeServices`, `ecs:DescribeTaskDefinition`, `ecs:ListTagsForResource`, and `ecs:RegisterTaskDefinition`.

## The console

```sh
SCRIPTS=.terraform/modules/app/deploy/opentofu
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
```

It starts one task with the command in place of the image's, waits for it however long it takes, prints what it wrote to CloudWatch Logs, and exits with its exit code. It first gives the console's family the image of the running service, which differs after an apply that stopped at a failed migration; the next apply gives the family back the image of the input. Turn `console_enabled` off afterwards. The console has no terminal, and the module does not turn on ECS Exec, which would be a way into the server's secrets.

## The network

**The module creates no network, and how the tasks reach the internet is yours to choose and to pay for.** [`network-aws-vpc`](../network-aws-vpc/README.md) makes one. A task needs a way out for the image, the database, federation, and the AWS APIs. Inbound traffic is closed either way: the security group admits only the load balancer.

| | `assign_public_ip = true` | `false` |
| --- | --- | --- |
| Subnets | `public_subnet_ids` of the network | `private_subnet_ids` of the network |
| Each hour | A public IPv4 address per task | A NAT gateway, and its own public address |
| Each GB | Nothing for the address | A charge per GB through the gateway, media and federation included |

With `nat` of the network off, `assign_public_ip = false` fails the plan: such a task would have no way out, and the service would not even pull its image. Use the public subnets unless a policy of yours forbids them: it costs far less and is no more open. It has no default, so that nobody starts paying for either without choosing it.

## What it costs

Fixed costs per month (730 hours), from AWS's price list for Tokyo where it says so. **Check them yourself before you rely on them**:

| Part | Per month | Source |
| --- | --- | --- |
| One task of 0.25 vCPU and 1 GB, x86 | $13.26 ($10.61 on arm64) | Read: $0.05056 per vCPU-hour and $0.00553 per GB-hour, or $0.04045 and $0.00442 on ARM (price list, Tokyo, 2026-10) |
| Load balancer | $17.74 | Read: $0.0243 an hour (price list, Tokyo, 2026-11) |
| Its capacity units | $0 to $5.84 for each unit in use all month | Read: $0.008 per unit-hour. What your traffic uses has not been measured |
| The load balancer's public addresses | $7.30 for two zones | Read: $0.005 per address-hour, in every commercial region, one address per zone |
| A task's public address, or | $3.65 | Read: the same rate |
| A NAT gateway, with its address | About $36.50, plus a charge per GB | **Tokyo not read.** $0.045 an hour and $0.045 per GB are the figures AWS's page gives for Ohio |
| **Total, at idle, public subnets** | **About $42**, plus capacity units | |
| **Total, at idle, NAT** | **About $75**, plus capacity units and data | Using Ohio's rate for the gateway |

Not read, so not in the total: Secrets Manager per secret, CloudWatch Logs, S3, SNS and SQS, data out of AWS, and any free tier. One-off tasks cost their size for the minutes they run, and an upgrade runs a second service task for a few minutes. The database comes on top, and [does not rest](../queue-db/README.md#what-it-needs).

## What differs from the other apps

| | Cloud Run | Here |
| --- | --- | --- |
| Processes | One with `role = all` | One with `role = all`, or several: each runs the web and the jobs |
| Idle | Nothing | The whole fixed cost above |
| Jobs | Cloud Tasks calls in | The process polls the table |
| Secrets | The platform's | The platform's, at the start of a task |
| Request size and time | 32 MiB, up to the timeout | The server's own limits |
| One-off commands | A job, an hour by default | No limit |
| A bad image | No traffic until `/readyz` answers | The load balancer's check, then the deployment goes back. No container check: read below |
| Who may reach it | `service_ingress` | The load balancer alone, by security group |
| The disk | Gone with the instance | Gone with the task, so the plan refuses `sqlite` and `blob = fs` |

**There is no container health check.** ECS would run one inside the container, and the image is distroless, with no shell or `curl`, and `my` has no command that asks the server's own `/readyz`. So health comes from the load balancer alone: without an ingress, a deployment passes once the process has started. Also, the one check of a load balancer serves for both a new image and a running task, as [its README says](../ingress-aws-alb/README.md#health-checks).

## Unverified

- That a worker's deployment means anything beyond its tasks having started: it has no load balancer and no container check, so a worker on a bad image fails only in its logs, and the apply goes on to the default.
- Everything against a real account: that ECS accepts the task definitions, and that the service starts.
- That a plan shows no change after an apply. It rests on what the provider's source does: `track_latest` reads a family's latest revision, and the provider's comparison of `container_definitions` treats ECS's added defaults as no change. If it does not, the plan shows a replacement of the task definition, which rolls the tasks on the same image.
- What `register-task-definition` accepts from a revision that `describe-task-definition` printed. `scripts/tofu-scripts-test.sh` shows only what is sent.
- What `wait_for_steady_state` reports when the circuit breaker rolls a deployment back: whether the apply fails, or succeeds with the service back on the old revision while the state holds the new one, which the next apply would deploy again.
- That `ecs list-tasks --family` with `--desired-status RUNNING` lists a migration task that is still starting, which `migrate.sh` waits for.
- `stopTimeout` of 60 on Fargate, whose limit of 120 seconds comes from a part of AWS's page that was read only in summary.
- That everything `my serve` writes fits the task's disk, and that its user can write there.
- The first start's timing against `health_check_grace_seconds`, and two tasks of `role = all` running the jobs of one instance on Fargate.
