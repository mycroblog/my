# monitoring-gcp

Monitoring and alerts on Google Cloud for a server on Cloud Run: an uptime check, log-based metrics, alert policies with the names of [the runbook](../../../../docs/operations/alerts.md), and email notification channels. It follows [the backend contract](../../README.md#the-backend-contract).

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that Google Cloud accepts it or that an alert fires. [What stays unverified](#what-stays-unverified) lists it. The first apply and the first failure are the test: after applying, stop the database or set a wrong `host` for one check, and see that a mail arrives.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "monitoring" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/monitoring-gcp?ref=v0.1.0&depth=1"

  project_id          = var.project_id
  host                = var.host
  notification_emails = ["ops@social.example"]
}
```

Add `module.monitoring` to `backends` of the app module, so that a setting of `metrics_exporter` elsewhere fails the plan. Enable `monitoring.googleapis.com` first; `logging.googleapis.com` is in the guide's list already.

| Input | Default | Meaning |
| --- | --- | --- |
| `project_id`, `host` | required | Where everything goes, and the host that the check calls. `host` is the public domain, or `origin_host` of the app when nothing is in front. A service behind `internal-and-load-balancer` refuses the internet on its own URL, so give it the public domain |
| `notification_emails` | empty | Addresses that get every alert. This and `notification_channels` cannot both be empty |
| `notification_channels` | empty | Channels made elsewhere, by full ID, that get every alert |
| `page_notification_channels` | empty | Channels that also get the alerts of severity page |
| `service_name` | `my` | The app module's `name` |
| `name` | `my` | Start of the names of the metrics and the check |
| `uptime_period` | `900s` | `60s`, `300s`, `600s`, or `900s`. [What it costs](#what-the-uptime-check-costs) |
| `uptime_path` | `/readyz` | Or `/healthz`, or `/api/v1/instance/health` |
| `uptime_timeout` | `30s` | Long enough for a cold start and a database that wakes up |
| `uptime_regions` | three in the USA | Must hold 3 checkers. Each is one more request per period |
| `metrics_exporter` | `none` | `none` or `otlp`. Returned in `env` |
| `snapshot_stale_after_seconds` | `3600` | When `MyMetricsSnapshotStale` fires. It must exceed 15 minutes plus the longest gap of `tick_schedule` |
| `database_size_limit_bytes`, `media_size_limit_bytes`, `remote_media_cache_limit_bytes` | null | The limits of the storage alerts. Null leaves the alert out |
| `billing_sources` | empty | The server's `billing_sources`. Empty leaves `MyBillingImportFailing` out |
| `disabled_alerts` | empty | Alerts to leave out, by name |

| Output | Value |
| --- | --- |
| `env` | `MY_METRICS_EXPORTER` |
| `secret_env`, `permissions` | Empty. Cloud Run writes the logs and its own metrics by itself |
| `uptime_check_id` | The check |
| `alerts` | `policies`, the alerts this module has, and `not_expressible` |
| `notification_channels` | The IDs of the channels that get every alert |

## How the server's metrics reach Cloud Monitoring

Through log lines: `metrics_exporter = none`, and log-based metrics made here. The server writes a line per request (`http request`), per job run (`job`), per error report (`error report`), per failed readiness check (`not ready`), and, every 15 minutes, one with the instance-wide gauges (`metrics snapshot`). Cloud Run sends them to Cloud Logging, and each line is written before the request ends, while the instance has CPU. Nothing has to run between requests, so nothing here breaks scaling to zero.

The other exporters, and why they are not used:

| Way | Does it work? | What it needs |
| --- | --- | --- |
| `otlp` with a Collector sidecar | Not with this app module. It is what Google's [Collector on Cloud Run](https://docs.cloud.google.com/stackdriver/docs/instrumentation/opentelemetry-collector-cloud-run) describes | A contract change: an input of `app-gcp-cloud-run` for sidecars, a second container and its start order, the Collector's configuration mounted from a secret, `roles/telemetry.writer` in `permissions`, and `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318` (the server speaks HTTP, not gRPC). The service sets `cpu_idle = true`, so CPU exists only during requests, and a 60-second export loop in either container would run only by luck. It needs instance-based billing, and the tick keeps an instance up: about $60 a month at 1 vCPU and 512 MiB, where this module adds next to nothing |
| `otlp` straight to `telemetry.googleapis.com` | No | Google's endpoint takes OTLP metrics but [needs Google credentials](https://docs.cloud.google.com/stackdriver/docs/reference/telemetry/overview). The server sends only the fixed headers of `OTEL_EXPORTER_OTLP_HEADERS`, and an access token expires within the hour, so this needs server code. The CPU problem above stays |
| `prometheus` | No | Nothing scrapes it. A Prometheus sidecar has the same two problems as the Collector; its documents were not read |
| `emf` | Writes its lines, for nothing to read | CloudWatch reads those lines, and Cloud Logging would only store them. It also replaces the `metrics snapshot` line, so this module refuses it |

The Prometheus names in the runbook do not exist here. What is lost compared with `otlp` is in the table below, as `not expressible`.

## Scaling to zero and absent data

An idle service has no lines and no platform points, so its metrics have gaps. An alert on the absence of a request or job metric would fire every quiet night, so no alert here does that:

- Every PromQL condition compares what is there. No data gives an empty result, which fires nothing and closes what was open. So a server that dies with no traffic trips none of them.
- `MyTargetDown` watches the check's own metric. The checkers report whether or not the service has an instance, so it is what notices a dead idle server. This is the base of the module.
- `MyMetricsSnapshotStale` is the one alert on absence, and it is safe for a reason: the tick starts the snapshot job on an idle server too. A gap means the tick or the queue stopped. If you lengthen `tick_schedule`, keep `snapshot_stale_after_seconds` above it plus 15 minutes.

A rate over few points is noisy, which is why the ratios keep the runbook's lower bound on traffic. Check them against a quiet instance after the first week.

## What the uptime check costs

Each run wakes an instance, and through `/readyz`, which touches the database on every request, it wakes a scale-to-zero database. The checks are few enough for the free allowances: with the default 3 checkers, `900s` makes 8,640 requests a month and `60s` makes 129,600, against Cloud Run's 2 million and Cloud Monitoring's 1 million executions (then $0.30 per 1,000, from a search of [the pricing page](https://cloud.google.com/products/observability/pricing), which could not be read directly). The cost is Neon's. It stops a compute after 5 minutes without activity, so at 0.25 CU:

| `uptime_period` | Checkers fire together | Checkers spread over the period |
| --- | --- | --- |
| `900s` | Awake 1/3 of the time, about 61 CU-hours a month | Never stops, 182.5 CU-hours |
| `600s` | Awake 1/2 of the time, about 91 CU-hours | Never stops, 182.5 CU-hours |
| `300s`, `60s` | Never stops, 182.5 CU-hours | Never stops, 182.5 CU-hours |

Google's documents do not say which of the two columns holds, so look at the requests on the first day. Neon's Free plan has 100 CU-hours, and [the tick's trade-off in the guide](../../../../docs/operations/deploy-cloud-run.md#costs) has the same arithmetic:

- With the default `tick_schedule` of `* * * * *`, the compute never stops anyway, and the check adds nothing to the bill. Choose the period for how soon you want to know.
- With `*/10 * * * *`, the tick alone is about 91 CU-hours. The default check on top of it comes to as much as 2/3 of the time, about 121 CU-hours, which is over the Free plan. To stay inside it, set `uptime_path = "/healthz"`, which wakes the instance and not the database, at the price of not seeing a database outage from outside. `MyNotReady` and the HTTP alerts still see one when anything calls the server.

The default is `900s` because it is the longest there is, and it means up to about 15 minutes more before `MyTargetDown` fires. `/readyz` answers `200` in the maintenance mode, so the check stays green during one.

## The alerts of the runbook

**Covered** means the same signal and threshold. **Covered differently** says what stands in for it. The thresholds are starting points, as in the runbook. Silence a policy for the maintenance mode with a snooze in Cloud Monitoring.

| Alert | Status | How |
| --- | --- | --- |
| `MyTargetDown` | Covered differently | The uptime check instead of a scrape: more than one checker fails for one period, at least 5 minutes. It stays green in the maintenance mode |
| `MyNotReady` | Covered differently | Lines `not ready`: more than one in 15 minutes for 5 minutes. A failure needs something to call `/readyz`, here the check or a starting revision. More than one, because a database that wakes slowly fails the 3-second check once |
| `MyQueueBacklog` | Covered differently | The snapshot line every 15 minutes, with the age compared in the filter. One sample over the threshold fires, in place of 10 minutes above it |
| `MyQueueStalled` | Covered differently | The same, at 30 minutes. A stalled queue also holds back the snapshot job, so `MyMetricsSnapshotStale` may come first |
| `MyMetricsSnapshotStale` | Covered differently | The line's absence, instead of the age of each part |
| `MyDeadJobsIncreasing` | Covered differently | Job lines with `dead` per hour, instead of the change of the gauge |
| `MyDeliveryFailuresHigh` | Covered differently | Job lines of `federation.deliver`: `retry` and `dead` per run, instead of attempts by outcome |
| `MyDeliveryAllFailing` | Covered differently | The same |
| `MyInboxRejectionsHigh` | Covered differently | Status of POSTs to the inboxes: 4xx and 503 per POST |
| `MyInboxSignatureFailures` | Covered differently | Status 401 per POST. A key that could not be fetched answers 503 and is left out |
| `MyInboxErrors` | Covered differently | Status 500 per POST |
| `MyErrorReportsIncreasing` | Covered | Lines `error report`. Error Reporting groups the same lines too |
| `MyHTTPErrorsHigh` | Covered differently | Cloud Run's `request_count`, so it also counts what the platform answered itself, and the calls of Cloud Tasks and the tick |
| `MyHTTPErrorsElevated` | Covered differently | The same |
| `MyAPILatencyHigh` | Covered | The duration of the request lines of the client API, the streaming endpoint left out |
| `MyMemoryNearLimit` | Covered differently | Cloud Run's memory utilization over 81% of the container's limit, instead of the Go runtime over 90% of `GOMEMLIMIT`, which the app module sets to 90% of that limit |
| `MyDatabaseStorageLarge` | Covered differently | The snapshot line against `database_size_limit_bytes`. Neon's own usage alerts see more |
| `MyMediaStorageLarge` | Covered differently | The snapshot line, local media alone against `media_size_limit_bytes`: a filter cannot add two fields |
| `MyRemoteMediaCacheOverLimit` | Covered differently | The snapshot line against `remote_media_cache_limit_bytes`, which you set to 1.2 times the setting |
| `MyEmailFailing` | Covered | Job lines of the email jobs with `dead` |
| `MyEmailRetrying` | Covered | Job lines of the email jobs with `retry` |
| `MyBillingImportFailing` | Covered differently | The snapshot line, for each of `billing_sources` |
| `MyRemoteServersPaused` | Not expressible | It compares a gauge with its value a day before, and a counter of lines holds no value. See the remote servers screen |
| `MyProcessRestarting` | Not expressible | An instance that starts and stops is what scaling to zero does, so restarts mean nothing here. An out-of-memory kill shows in `MyHTTPErrorsHigh` and in Cloud Run's logs |
| `MyDBPoolSaturated` | Not expressible | No log line carries the pool's use. It needs `otlp`. Its effect shows in `MyHTTPErrorsHigh` and `MyAPILatencyHigh`, and on Neon's own screens |
| `MyDBPoolWaiting` | Not expressible | The same |
| `MyFileDescriptorsNearLimit` | Not expressible | No line carries it, and Cloud Run sets the limit |
| `MyAdapterErrors` | Not expressible | No line carries it. It needs `otlp` |

The runbook's budget entry is outside this module: use a Cloud Billing budget.

## Adding other notification channels

Email is the input because it needs nothing but an address. Any other kind is a channel of its own, whose full ID goes in `notification_channels`, or in `page_notification_channels` for pages alone:

```hcl
resource "google_monitoring_notification_channel" "webhook" {
  project      = var.project_id
  display_name = "my alerts"
  type         = "webhook_tokenauth"
  labels       = { url = "https://hooks.example/my" }
}

module "monitoring" {
  # ...
  notification_channels = [google_monitoring_notification_channel.webhook.id]
}
```

**A channel with a secret, such as Slack or PagerDuty, puts it in the state** if you make it here. Make it in the console (Monitoring, Alerting, Edit notification channels) and pass its ID instead. `gcloud beta monitoring channels list` prints the IDs.

## What stays unverified

Read from `tofu providers schema -json` and Google's documents, and never run:

- That Cloud Monitoring's PromQL accepts `rate()` and `increase()` on a delta metric, and `_bucket` on a log-based distribution. The documents describe the suffixes and the naming rule; none of the pages read treats delta metrics. If it refuses, each query fails at apply and needs rewriting as `condition_threshold`.
- The metric names `logging_googleapis_com:user_*`, `run_googleapis_com:request_count`, and `run_googleapis_com:container_memory_utilizations_bucket`, which come from the rule rather than an example, and the label names on them.
- The numeric comparison on a dotted field, `EXTRACT` on one, and the fields of a nested group (`jsonPayload."my.media.stored_bytes".local`).
- The aggregation of `MyTargetDown` and of the absence condition.
- That a log-based metric whose filter matches nothing yet takes `disable_metric_validation`.
- That the checkers get through an ingress. Cloudflare may challenge them, and `/readyz` answers without the origin secret only at the origin.
- The prices, and whether the checkers fire together.
- The output of the server on Cloud Run: the fields come from its code and tests, not from Cloud Logging.
