# queue-aws-sqs

The job queue: an SQS standard queue that carries a wake-up signal for each job, and the EventBridge Scheduler schedule of the tick.
It follows [the backend contract](../../README.md#the-backend-contract).

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that AWS accepts it. It needs an app whose worker takes the queue's events: `app-aws-lambda`, and what that module lacks.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "queue" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/queue-aws-sqs?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `name` | `my-jobs` | The queue, `<name>-tick`, and `<name>-scheduler` |
| `tick_schedule` | `rate(1 minute)` | How often the periodic jobs are checked. Each tick invokes the worker and touches the database |
| `visibility_timeout_seconds` | `5400` | At least the function's timeout, which Lambda enforces. AWS asks for six times that |
| `message_retention_seconds` | `345600` | How long a signal is kept |
| `workers` | `{}` | [The same value as the app](../../README.md#workers). Each gets a queue `<name>-<key>`, its job types a route, and the app a function that the queue invokes |

| Output | Value |
| --- | --- |
| `env` | `MY_QUEUE`, `MY_QUEUE_SQS_QUEUE_URL`, and with workers `MY_QUEUE_SQS_ROUTES` |
| `workers` | The job types of each worker, which the app compares with its own |
| `secret_env` | Empty |
| `permissions` | `sqs:SendMessage` on each queue |
| `event_sources` | Each queue, which the app maps to its worker: `jobs` to the default, and each key to that worker's. Lambda polls it, so there are no `invokers` |

Lambda's pollers read a queue all the time, which costs SQS requests. The first million a month are free. This module takes no `host`: nothing
calls the server, and `queue_sqs_events_path` stays at its default, which is also the adapter's.

Unlike Cloud Tasks, which is told to call at a time, SQS delays a message by 15 minutes at most: the server signals a job that is due later again
when the message arrives. That is the server's own behaviour, and it is unverified on AWS.
