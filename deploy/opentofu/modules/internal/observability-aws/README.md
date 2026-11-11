# observability-aws

Where the metrics, the traces, and the error reports of a server on ECS go, as the server's exporter settings. Everything rides on the log lines, which the tasks' log driver sends to CloudWatch Logs with the execution role, so it creates nothing, asks for no permission, and declares no provider. It follows [the backend contract](../README.md#the-backend-contract). `mycroblog-aws` calls it for its section `observability`, and gives `monitoring-aws` the namespace it sets.

**This module has never been applied.** Its test checks its outputs alone.

| Input | Default | Meaning |
| --- | --- | --- |
| `metrics` | `cloudwatch` | Or `settings` |
| `traces` | `none` | Or `settings` |
| `errors` | `cloudwatch` | Or `settings` |
| `metrics_emf_namespace` | `my` | The CloudWatch namespace of the metrics. `mycroblog-aws` passes its `name` |

| Output | Value |
| --- | --- |
| `env` | `MY_METRICS_EXPORTER` (`emf`) and `MY_METRICS_EMF_NAMESPACE` for `cloudwatch` metrics, `MY_TRACES_EXPORTER` (`none`), and `MY_ERRORS_EXPORTER` (`none`) for `cloudwatch` errors |
| `secret_env`, `permissions` | Empty |

## The destinations

| Signal | Destination | What arrives |
| --- | --- | --- |
| `metrics` | CloudWatch | The metrics in Embedded Metric Format, which CloudWatch Logs extracts from the lines. `monitoring-aws` sets the dimensions its alarms name |
| `traces` | None | No spans. The log lines carry `request_id` and `job_id` |
| `errors` | CloudWatch Logs | The `error report` line, with its stack |

X-Ray takes the server's spans only through a collector beside it, the AWS Distro for OpenTelemetry, which `app-aws-ecs` does not run, so `traces` has no destination of AWS.

`settings` leaves a signal to the `settings` input of the public module: the module sets nothing for it, so that
`errors_exporter = sentry`, for one, can go there.
