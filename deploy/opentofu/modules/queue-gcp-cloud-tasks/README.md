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
| `service_name` | `my` | The app module's `name`. Tasks and ticks call this service's `run.app` URL |
| `name` | `my-jobs` | The queue, `<name>-tick`, and `<name>-invoker` |
| `tick_schedule` | `* * * * *` | How often the periodic jobs are checked. Each tick touches the database |
| `max_concurrent_dispatches` | `20` | Tasks at once, which bounds a burst of jobs |

| Output | Value |
| --- | --- |
| `env` | `MY_QUEUE`, `MY_QUEUE_CLOUDTASKS_QUEUE`, `MY_QUEUE_CLOUDTASKS_URL`, `MY_QUEUE_CLOUDTASKS_SERVICE_ACCOUNT` |
| `secret_env` | Empty |
| `permissions` | `roles/cloudtasks.enqueuer` on the queue, `roles/iam.serviceAccountUser` on the invoker |
| `invokers` | The invoker, which the app gives `roles/run.invoker` |

Tasks and ticks call `https://<service_name>-<project number>.<region>.run.app`, Cloud Run's deterministic default URL, directly and never through an ingress:

- Cloudflare's limit of 125 seconds and its challenges stay out of the way of a job.
- Cloud Run counts Cloud Tasks and Cloud Scheduler as internal only from the same project and on the default `run.app` URL, not a custom domain, so this is what lets the service refuse the internet behind a load balancer.
- **Do not disable the service's default URL**: jobs stop. `app-gcp-cloud-run` keeps it on.

Google's documentation calls both the deterministic and the non-deterministic URL "default `run.app` URLs", and counts these two as internal from the same project on "the default `run.app` URL and not a custom URL or custom domain". So the deterministic URL qualifies by the documentation; the one thing no page states is the case of Cloud Tasks and Cloud Scheduler on that form by name. Confirm it on the first apply behind `internal-and-load-balancer`.

The project's number is read with `data.google_project`, so whoever runs a plan needs `resourcemanager.projects.get`. The app module refuses a plan in which this URL does not name its service and region.

The queue depends on no output of the app module: the URL comes from its inputs, and the invoker is created here. No audience
is set, so tokens carry the URL called, which is what the server requires while `queue_cloudtasks_audience` is unset. Set
that setting and the Scheduler job's audience together if you need another.

Cloud Tasks refuses a queue name for about a week after its queue is deleted, so a deleted queue needs another `name`.
