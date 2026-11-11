# observability-aws

Where the metrics, the traces, and the error reports of a server on ECS go, as the server's exporter settings. The metrics and the error reports ride on the log lines, which the tasks' log driver sends to CloudWatch Logs with the execution role. The traces go to X-Ray through the AWS Distro for OpenTelemetry collector, which the module asks the app to run beside the server in every task (`sidecars`), with the one permission it writes with. It creates nothing and declares no provider. It follows [the backend contract](../README.md#the-backend-contract). `mycroblog-aws` calls it for its section `observability`, and gives `monitoring-aws` the namespace it sets.

**This module has never been applied.** Its test checks its outputs alone, so nothing has shown that X-Ray takes the spans the collector sends.

| Input | Default | Meaning |
| --- | --- | --- |
| `metrics` | `cloudwatch` | Or `settings` |
| `traces` | `x-ray` | Or `none`, or `settings` |
| `errors` | `cloudwatch` | Or `settings` |
| `metrics_emf_namespace` | `my` | The CloudWatch namespace of the metrics. `mycroblog-aws` passes its `name` |
| `collector_image` | `public.ecr.aws/aws-observability/aws-otel-collector:v0.50.0@sha256:…` | The collector's image, pinned by version and digest; never `latest` |

| Output | Value |
| --- | --- |
| `env` | `MY_METRICS_EXPORTER` (`emf`) and `MY_METRICS_EMF_NAMESPACE` for `cloudwatch` metrics; `MY_TRACES_EXPORTER` (`otlp`) and `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` (`http://127.0.0.1:4318/v1/traces`) for `x-ray`, or `MY_TRACES_EXPORTER` (`none`) for `none`; `MY_ERRORS_EXPORTER` (`none`) for `cloudwatch` errors |
| `permissions` | `xray:PutTraceSegments` on `*` for `x-ray`, otherwise none |
| `sidecars` | The collector for `x-ray`, with 32 CPU units and 128 MiB, otherwise none |
| `secret_env` | Empty |

## The destinations

| Signal | Destination | What arrives |
| --- | --- | --- |
| `metrics` | CloudWatch | The metrics in Embedded Metric Format, which CloudWatch Logs extracts from the lines. `monitoring-aws` sets the dimensions its alarms name |
| `traces` | X-Ray | The spans that the server records (`traces_sample_ratio`, 1% by default), through the collector. With `none`, no spans; the log lines carry `request_id` and `job_id` |
| `errors` | CloudWatch Logs | The `error report` line, with its stack |

## The collector

X-Ray takes spans only signed with AWS credentials, which the server does not do, so the server sends them over OTLP/HTTP to
the collector on the task's loopback, and the collector sends them to X-Ray with the task role. Its configuration, in
`AOT_CONFIG_CONTENT`, has one pipeline: the OTLP receiver on `127.0.0.1:4318`, a memory limiter and a batch, and the `awsxray`
exporter. It has no metrics pipeline (the metrics go in EMF) and reports no metrics of its own.

- **The permission is `xray:PutTraceSegments` alone**, not the managed policy `AWSXRayDaemonWriteAccess`: the server decides
  which traces to record, so the collector reads no sampling rules (`xray:GetSamplingRules`, `xray:GetSamplingTargets`), and the
  exporter sends no telemetry records (`xray:PutTelemetryRecords`) unless configured to. A permission of the contract is one
  IAM action, so a managed policy has no place in it either.
- **The `awsxray` exporter, not X-Ray's OTLP endpoint**: the endpoint needs CloudWatch Transaction Search turned on for the
  account, which is a setting of the whole account and is billed by what it ingests.
- **What it takes from a task**: 32 CPU units and 128 MiB, with `GOMEMLIMIT` at 100 MiB and the limiter below it. The app gives the
  server the rest and lowers the server's `GOMEMLIMIT` to match. The container is not essential: a collector that stops loses
  the spans and leaves the server running. It starts before the server and stops after it, so that the last spans of a task
  reach X-Ray.
- **Where the image comes from**: Amazon ECR Public, through the tasks' way out to the internet, like the server's image.

`settings` leaves a signal to the `settings` input of the public module: the module sets nothing for it, so that
`errors_exporter = sentry`, for one, can go there.
