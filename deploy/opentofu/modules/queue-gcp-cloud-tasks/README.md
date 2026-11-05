# queue-gcp-cloud-tasks

The job queue: a Cloud Tasks queue, the Cloud Scheduler job of the tick, and the service account whose token both carry.
It follows [the backend contract](../../README.md#the-backend-contract).

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "queue" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/queue-gcp-cloud-tasks?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `project_id`, `region` | required | Where everything goes |
| `host` | required | Tasks and ticks call `https://{host}` |
| `name` | `my-jobs` | The queue, `<name>-tick`, and `<name>-invoker` |
| `tick_schedule` | `* * * * *` | How often the periodic jobs are checked. Each tick touches the database |
| `max_concurrent_dispatches` | `20` | Tasks at once, which bounds a burst of jobs |

| Output | Value |
| --- | --- |
| `env` | `MY_QUEUE`, `MY_QUEUE_CLOUDTASKS_QUEUE`, `MY_QUEUE_CLOUDTASKS_URL`, `MY_QUEUE_CLOUDTASKS_SERVICE_ACCOUNT` |
| `secret_env` | Empty |
| `roles` | `roles/cloudtasks.enqueuer` on the queue, `roles/iam.serviceAccountUser` on the invoker |
| `invokers` | The invoker, which the app gives `roles/run.invoker` |

The queue depends on no output of the app module: the URL comes from `host`, and the invoker is created here. No audience
is set, so tokens carry the URL called, which is what the server requires while `queue_cloudtasks_audience` is unset. Set
that setting and the Scheduler job's audience together if you need another.

Cloud Tasks refuses a queue name for about a week after its queue is deleted, so a deleted queue needs another `name`.
