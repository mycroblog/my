# app-aws-ecs

The server on Amazon ECS with Fargate: one service whose process stays up, and one-off tasks for the migration and the console. It follows [the app contract](../../README.md#the-app-contract).

**This module has never been applied.** The tests mock the provider and the scripts' tests stub the `aws` command, so they show that the configuration and the steps are consistent, not that AWS accepts them. What stays unverified is [listed below](#unverified). Unlike `app-aws-lambda`, nothing is known to be missing: the released image runs here as it is, ECS delivers the secrets, and `my` runs one command as a one-off task.

**It costs money while idle**, about $42 a month in Tokyo at the smallest useful shape, before the database. [What it costs](#what-it-costs) has the parts. Cloud Run costs nothing while nobody uses it; this shape does not scale to zero.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "app" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/app-aws-ecs?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `image` | required | Image at creation. After that `upgrade.sh` changes it, and changing this input does nothing |
| `host` | required | Public domain |
| `vpc_id`, `subnet_ids` | required | Where the tasks run. The module creates no network |
| `assign_public_ip` | required | Whether each task has a public address. Read [The network](#the-network) |
| `architecture` | `x86_64` | Or `arm64`. The released image has both |
| `name` | `my` | Name of the cluster, the service, and its family, and the `--name` of `upgrade.sh` |
| `profile` | `production` | `profile` of the server |
| `backends`, `env`, `secret_env`, `secret_files` | empty | [The backend contract](../../README.md#the-backend-contract). `secret_files` is treated as `secret_env`. The module sets `MY_BILLING_SOURCES` from what `backends` returns as `billing_sources` |
| `cpu`, `memory_mib` | `256`, `1024` | Size of a service task, from [the sizes Fargate offers](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task_definition_parameters.html). A size it does not offer fails the apply. `GOMEMLIMIT` is 90% of the memory |
| `desired_count` | `1` | Tasks of the service. Above 1 needs a stream other than `hub`, which the plan checks |
| `job_cpu`, `job_memory_mib` | `512`, `1024` | Size of a one-off task, billed only while it runs |
| `health_check_grace_seconds` | `120` | How long a new task may fail the load balancer's check |
| `console_enabled` | `false` | Creates the console's task definition |
| `log_retention_days` | `30` | CloudWatch Logs |

A backend may also return `direct_secret_env`: settings that the migration and the console take in the place of the service's, such as a direct connection to a database whose service goes through a pooler. Without it, they take the service's.

| Output | Value |
| --- | --- |
| `service_name`, `cluster_name` | Both are `name` |
| `migration_task_definition` | The family `upgrade.sh` runs |
| `origin_host` | Always null: read below |
| `security_group_id` | Of the tasks, for a rule of your own |
| `execution_role_arn`, `task_role_arn` | Read [Secrets](#secrets) |
| `log_group` | `/ecs/<name>` |
| `console` | What `console.sh` reads, or null |
| `env`, `secret_env`, `one_off_secret_env` | The merged settings: the service's, and those of the one-off tasks |

## How traffic reaches it

Only through a load balancer that the service attaches to, which `ingress-aws-alb` provides. A task's address changes with every task and it has no TLS, so **this app has no host of its own**: `origin_host` is null, and a module that takes one as its origin, such as `ingress-cloudflare-app` or `ingress-aws-cloudfront`, fails the plan. So does `service_ingress` of anything but `load-balancer`. There is no `service_ingress` input, because there is nothing to choose.

Without an ingress the service runs with no inbound rule and only the process's own start decides whether a deployment passes. To put something else in front, add a rule to `security_group_id`, and pass the app a backend of `{ service_ingress = "load-balancer", trusted_proxies = [...], load_balancer = { target_group_arn = "...", security_group_id = "..." } }`.

## Jobs

The process runs them itself, so use `queue-db`, or leave `MY_QUEUE` at its default, which is the same. `queue-aws-sqs` fails the plan: `queue = sqs` runs jobs only when the Lambda adapter passes it the queue's events, and no process here polls SQS. Read [what `queue-db` needs of the database](../queue-db/README.md#what-it-needs): the database never rests.

## Secrets

ECS resolves each secret into the variable the server reads when a task starts, by the `secrets` of the task definition, so **no value reaches the plan or the state**: both hold only ARNs. That makes two roles:

- The execution role starts the task: it pulls the image, writes the logs, and reads the secrets.
- The task role is what the server runs as, in the service and the one-off tasks alike, and holds what the backends ask. It cannot read the secrets, and needs no way to.

A task reads a secret once, at start. After you change one, run `aws ecs update-service --cluster my --service my --force-new-deployment`. Fargate mounts no secret, so a `_FILE` setting reaches the server as the variable without `_FILE`, as `MY_KEK` does from `kek-aws-secrets-manager`. A secret is named by its whole ARN: the module grants that ARN, so a `:json-key::` on the end does not work. A secret under a customer-managed KMS key also needs `kms:Decrypt`, which the module does not grant.

## The image

The released image runs as it is, which Lambda cannot do. Fargate pulls `ghcr.io/mycroblog/my` over the internet, so a task needs a way out, and the architecture you choose picks from its index. A copy in ECR works too, in the same region or another: the execution role may pull from any repository of the account.

## The first deployment

`/readyz` answers `503` until the schema exists, so the schema comes first, as on Cloud Run. Create the secrets and fill in the ones you bring, then the migration's task definition and what it needs, then run the migration, then everything:

```sh
tofu apply -target=module.secrets -target=module.kek
aws secretsmanager put-secret-value --secret-id my-database-url --secret-string "$DATABASE_URL"   # and the others
tofu apply -target='module.app.aws_ecs_task_definition.task["migrate"]'

SCRIPTS=.terraform/modules/app/deploy/opentofu
"$SCRIPTS/upgrade.sh" --platform aws-ecs --region ap-northeast-1 --image ghcr.io/mycroblog/my:0.1.0 --migrate-only

tofu apply
```

The first command that uses the database records `host` and `account_domain` in it. Create the first administrator through [the console](#the-console).

## Upgrading

```sh
"$SCRIPTS/upgrade.sh" --platform aws-ecs --region ap-northeast-1 --image ghcr.io/mycroblog/my:0.2.0
```

It runs [the steps of every platform](../../README.md#upgrading), each as a one-off task, with a new revision of `<name>-migrate` first. Then it registers a revision of the service with the new image and waits for that deployment. New tasks must pass the load balancer's check before the old ones leave, and a deployment that fails goes back to the old revision by itself, which the script reports as a failure, naming the service's events.

**How apply follows.** A revision holds the image and every setting together, so ignoring the image on it would also stop settings from reaching the service. So `upgrade.sh` writes the image to the parameter `/my/<name>/image` as its last step, a refresh reads that, and the task definitions take it. Changing a setting with `tofu apply` then rolls the service on the upgraded image, and a plan right after an upgrade shows nothing. Do not write that parameter by hand with an image that has not been migrated. While `--maintenance` is on, do not apply: the revision with `MY_MAINTENANCE` is not what the configuration says, and apply would end the maintenance mode.

Whoever runs the scripts needs `ecs:DescribeTaskDefinition`, `ecs:ListTagsForResource`, `ecs:RegisterTaskDefinition`, `ecs:DescribeServices`, `ecs:UpdateService`, `ecs:RunTask`, `ecs:DescribeTasks`, `ssm:GetParameter`, `ssm:PutParameter`, `logs:FilterLogEvents`, and `iam:PassRole` on both roles.

## The console

```sh
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
```

It starts one task with the command in place of the image's, waits for it however long it takes, prints what it wrote to CloudWatch Logs, and exits with its exit code. It first gives the console's family the image of the running service. Turn `console_enabled` off afterwards. The console has no terminal, and the module does not turn on ECS Exec, which would be a way into the server's secrets.

## The network

**The module creates no network, and how the tasks reach the internet is yours to choose and to pay for.** A task needs a way out for the image, the database, federation, and the AWS APIs. Inbound traffic is closed either way: the security group admits only the load balancer.

| | `assign_public_ip = true` | `false` |
| --- | --- | --- |
| Subnets | Public | Private, with a NAT gateway you run |
| Each hour | A public IPv4 address per task | A NAT gateway, and its own public address |
| Each GB | Nothing for the address | A charge per GB through the gateway, media and federation included |

Use the public subnets unless a policy of yours forbids them: it costs far less and is no more open. It has no default, so that nobody starts paying for either without choosing it.

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

Not read, so not in the total: Secrets Manager per secret, CloudWatch Logs, S3, SNS and SQS, parameters in Parameter Store (the two here are standard ones), data out of AWS, and any free tier. One-off tasks cost their size for the minutes they run, and an upgrade runs a second service task for a few minutes. The database comes on top, and [does not rest](../queue-db/README.md#what-it-needs).

## What differs from the other apps

| | Cloud Run | Lambda | Here |
| --- | --- | --- | --- |
| Processes | One with `role = all` | `web` and `worker` | One with `role = all`, or several: each runs the web and the jobs |
| Idle | Nothing | Nothing | The whole fixed cost above |
| Jobs | Cloud Tasks calls in | The queue invokes the worker | The process polls the table |
| Secrets | The platform's | Not delivered | The platform's, at the start of a task |
| Request size and time | 32 MiB, up to the timeout | 6 MB, 15 minutes | The server's own limits |
| One-off commands | A job, an hour by default | 15 minutes | No limit |
| A bad image | No traffic until `/readyz` answers | Nothing holds it back | The load balancer's check, then the deployment goes back. No container check: read below |
| Who may reach it | `service_ingress` | Everyone, and the server refuses by a header | The load balancer alone, by security group |
| The disk | Gone with the instance | Read-only | Gone with the task, so the plan refuses `sqlite` and `blob = fs` |

**There is no container health check.** ECS would run one inside the container, and the image is distroless, with no shell or `curl`, and `my` has no command that asks the server's own `/readyz`. So health comes from the load balancer alone: without an ingress, a deployment passes once the process has started. Also, the one check of a load balancer serves for both a new image and a running task, as [its README says](../ingress-aws-alb/README.md#health-checks).

## Unverified

- Everything against a real account: that ECS accepts the task definitions, and that the service starts.
- That a plan shows no change after `upgrade.sh`. It rests on what the provider's source does: `track_latest` reads a family's latest revision, a refresh reads `insecure_value` from the parameter, and the provider's comparison of `container_definitions` treats ECS's added defaults as no change. If it does not, the plan shows a replacement of the task definition, which rolls the tasks on the same image.
- What `register-task-definition` accepts from a revision that `describe-task-definition` printed. `scripts/tofu-scripts-test.sh` shows only what is sent.
- The fields of `rolloutState` during a rollback, on which `upgrade.sh` decides, and which come from what AWS documents.
- `stopTimeout` of 60 on Fargate, whose limit of 120 seconds comes from a part of AWS's page that was read only in summary.
- That everything `my serve` writes fits the task's disk, and that its user can write there.
- The first start's timing against `health_check_grace_seconds`, and two tasks of `role = all` running the jobs of one instance on Fargate.
