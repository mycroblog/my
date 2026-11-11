# observability-gcp

Where the metrics, the traces, and the error reports of a server on Cloud Run go, as the server's exporter settings, and the APIs of Google Cloud's destinations. It follows [the backend contract](../README.md#the-backend-contract), and works only with `app-gcp-cloud-run`. `mycroblog-gcp` calls it for its section `observability`, and gives `monitoring-gcp` the exporter it sets.

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that Google Cloud accepts it.

| Input | Default | Meaning |
| --- | --- | --- |
| `project_id` | required | The project of the service, where the APIs are enabled |
| `metrics` | `cloud-monitoring` | Or `settings` |
| `traces` | `cloud-trace` | Or `settings` |
| `errors` | `error-reporting` | Or `settings` |

| Output | Value |
| --- | --- |
| `env` | `MY_METRICS_EXPORTER`, `MY_TRACES_EXPORTER`, and `MY_ERRORS_EXPORTER`, each `none`, for the signals that go to Google Cloud |
| `secret_env`, `permissions` | Empty |

## The destinations

Each one reads what Cloud Run already sends, so the server exports nothing to it and holds no Google credentials for it:

| Signal | Destination | API the module enables | What arrives |
| --- | --- | --- | --- |
| `metrics` | Cloud Monitoring | `monitoring.googleapis.com` | Cloud Run's metrics of the service, and the log-based metrics that `monitoring-gcp` makes from the server's lines |
| `traces` | Cloud Trace | `cloudtrace.googleapis.com` | The trace that Cloud Run records of each request. The log lines carry its ID when the ingress is a trusted proxy |
| `errors` | Error Reporting | `clouderrorreporting.googleapis.com` | The `error report` line, which Error Reporting reads from Cloud Logging and groups |

`settings` leaves a signal to the `settings` input of the public module: the module sets nothing and enables nothing for it,
so that `errors_exporter = sentry`, for one, can go there. The APIs stay enabled when the module goes.

The server's spans and its own metrics could reach Google Cloud only through an OpenTelemetry Collector beside it, which
`app-gcp-cloud-run` does not run: [`monitoring-gcp`](../monitoring-gcp/README.md#how-the-servers-metrics-reach-cloud-monitoring) says what that would take.

Unverified: that Cloud Trace shows Cloud Run's traces of the service and Error Reporting groups the lines once the APIs are on, with no grant to the service account.
