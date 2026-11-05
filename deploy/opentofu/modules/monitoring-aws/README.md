# monitoring-aws

Monitoring and alerts on CloudWatch for a server on AWS Lambda: a Synthetics canary of `/readyz`, alarms with the names of [the runbook](../../../../docs/operations/alerts.md), and an SNS topic with email subscriptions. It follows [the backend contract](../../README.md#the-backend-contract).

**This module has never been applied, and it has nothing to watch yet.** `app-aws-lambda` [cannot run](../app-aws-lambda/README.md). The tests mock the provider, so they show that the configuration is consistent, not that AWS accepts it or that an alarm fires. [What stays unverified](#what-stays-unverified) lists it.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "monitoring" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/monitoring-aws?ref=v0.1.0&depth=1"

  host                = var.host
  notification_emails = ["ops@social.example"]
}
```

Add `module.monitoring` to `backends` of the app module: it sets the exporter and the dimensions that the alarms read. Each address gets a mail from SNS to confirm, and gets nothing until it does.

| Input | Default | Meaning |
| --- | --- | --- |
| `host` | required | The host that the canary calls: the public domain, or `origin_host` of the app when nothing is in front |
| `notification_emails` | empty | Addresses that get every alarm. This and `notification_topic_arns` cannot both be empty |
| `notification_topic_arns` | empty | SNS topics made elsewhere that get every alarm |
| `page_notification_topic_arns` | empty | SNS topics that also get the alarms of severity page |
| `service_name` | `my` | The app module's `name`. The HTTP alarms read `<service_name>-web` |
| `name` | `my` | Start of the names of the topic and the canary |
| `queue_name` | `my-jobs` | `name` of `queue-aws-sqs` |
| `metrics_emf_namespace` | `my` | Namespace of the metrics |
| `uptime_period_minutes` | `15` | From 1 to 60. [What it costs](#what-the-canary-costs) |
| `uptime_path` | `/readyz` | Or `/healthz`, or `/api/v1/instance/health` |
| `canary_runtime_version` | `syn-nodejs-puppeteer-18.0` | The newest on 2026-11-05. AWS retires old ones |
| `database_size_limit_bytes`, `media_size_limit_bytes`, `remote_media_cache_limit_bytes` | null | The limits of the storage alarms. Null leaves the alarm out |
| `disabled_alerts` | empty | Alerts to leave out, by name |

| Output | Value |
| --- | --- |
| `env` | `MY_METRICS_EXPORTER`, `MY_METRICS_EMF_NAMESPACE`, `MY_METRICS_EMF_DIMENSIONS` |
| `secret_env`, `permissions` | Empty. Lambda writes the logs, and CloudWatch Logs takes the metrics from them |
| `canary_name`, `topic_arn` | The canary and the topic |
| `alerts` | `alarms`, the alerts this module has, and `not_expressible` |
| `alarm_metrics` | How many metrics the alarms read, each one billed |

The dimensions are not an input. CloudWatch matches a metric on its exact set of dimensions, so one more label on a metric would leave its alarm with no data.

## How the server's metrics reach CloudWatch

`metrics_exporter = emf`: the server writes each measurement as a log line before the unit of work ends, so a frozen environment loses nothing, and CloudWatch Logs takes the metrics out of the function's log group. No collector runs. `MY_METRICS_EMF_DIMENSIONS` holds the labels that the alarms name, and the other labels stay fields of the line.

## Scaling to zero and absent data

A function with no traffic writes nothing, so a metric of requests or jobs has gaps. Those alarms set `treat_missing_data = notBreaching`, so they fire only on what is there. Two alarms count absence as a failure, and both on something that goes on arriving while the function is idle:

- `MyTargetDown` reads `SuccessPercent` of the canary. A sample lasts one run of the canary, or it would count the time between runs as a failure.
- `MyMetricsSnapshotStale` reads the age written by the snapshot job, which the tick starts on an idle server too.

## What the canary costs

Each run wakes the web function, and through `/readyz` it wakes a scale-to-zero database. Runs are billed at $0.0012 each after 100 a month, besides the Lambda, logs, and S3 they use ([CloudWatch pricing](https://aws.amazon.com/cloudwatch/pricing/), read 2026-11-05):

| `uptime_period_minutes` | Runs a month | Runs cost | Neon at 0.25 CU, stopping after 5 minutes |
| --- | --- | --- | --- |
| `60` | 720 | About $0.74 | Awake 1/12 of the time, about 15 CU-hours |
| `15` | 2,880 | About $3.34 | Awake 1/3 of the time, about 61 CU-hours |
| `10` | 4,320 | About $5.06 | Awake 1/2 of the time, about 91 CU-hours |
| `5` | 8,640 | About $10.25 | Never stops, 182.5 CU-hours |
| `1` | 43,200 | About $51.72 | Never stops, 182.5 CU-hours |

The default tick of `queue-aws-sqs`, `rate(1 minute)`, keeps the database awake anyway; then the check adds nothing to Neon's bill. Change the schedule and the two add up, as [the guide's costs](../../../../docs/operations/deploy-cloud-run.md#costs) explain for the tick.

A Route 53 health check would be cheaper to run and is not used, because it has no period to set: at its slowest, 30 seconds, [the endpoint gets a request about every two seconds](https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/health-checks-creating-values.html) on average, and no scale-to-zero database would ever stop.

The rest of the bill: each custom metric costs $0.30 a month after the first 10, and each metric an alarm reads $0.10 after the first 10, which `alarm_metrics` counts. Every name the server writes, times the values of the dimensions, is a metric, so expect tens of them, or $10 to $30 a month. That figure is an estimate, not a measurement. Log lines cost $0.50 per GB after 5 GB.

## The alerts of the runbook

**Covered** means the same signal and threshold. **Covered differently** says what stands in for it. The thresholds are starting points, as in the runbook.

| Alert | Status | How |
| --- | --- | --- |
| `MyTargetDown` | Covered differently | The canary instead of a scrape: a run and its retry fail, over at least 5 minutes. `/readyz` answers `200` in the maintenance mode, so it stays green during one |
| `MyNotReady` | Covered differently | `my.readiness.failures`: more than one in 15 minutes. A failure needs something to call `/readyz`, here the canary |
| `MyQueueBacklog` | Covered differently | `ApproximateAgeOfOldestMessage` of the SQS queue: the age of a signal, since with `queue = sqs` the jobs travel there |
| `MyQueueStalled` | Covered differently | The same, at 30 minutes |
| `MyMetricsSnapshotStale` | Covered differently | The age of each part, and absence counted as a failure, over an hour |
| `MyDeadJobsIncreasing` | Covered differently | `my.job.duration` with `my.outcome = dead` per hour, instead of the change of the gauge |
| `MyDeliveryFailuresHigh` | Covered | `my.delivery.attempts` by outcome, over 15-minute samples |
| `MyDeliveryAllFailing` | Covered | The same |
| `MyInboxRejectionsHigh` | Covered | `my.inbox.activities` by outcome and reason |
| `MyInboxSignatureFailures` | Covered | The same |
| `MyInboxErrors` | Covered | The same |
| `MyRemoteServersPaused` | Covered differently | `DIFF` of the day's maximum of `my.federation.instances`, instead of a value and its value a day before |
| `MyErrorReportsIncreasing` | Covered differently | More than 10 in 15 minutes for 30 minutes. The comparison with the day before has no form in an alarm |
| `MyHTTPErrorsHigh` | Covered differently | `Url5xxCount` per `UrlRequestCount` of the web function, so it also counts what Lambda answered itself |
| `MyHTTPErrorsElevated` | Covered differently | The same |
| `MyAPILatencyHigh` | Covered differently | p95 of `http.server.request.duration` on `my.surface = client_api`. The route is not a dimension, so the streaming endpoint is in it, and a stream that ends counts as one long request |
| `MyAdapterErrors` | Covered differently | `my.adapter.operation.duration` by outcome, over all adapters: the adapter is not a dimension |
| `MyDatabaseStorageLarge` | Covered | `my.db.size`, from the snapshot job |
| `MyMediaStorageLarge` | Covered | `my.media.stored_bytes` of both origins |
| `MyRemoteMediaCacheOverLimit` | Covered | The same, of the remote cache |
| `MyBillingImportFailing` | Covered differently | The maximum over all sources: the source is not a dimension |
| `MyEmailFailing` | Not expressible | It needs the job type as a dimension, which would multiply the billed metrics by about 50 job types, and an alarm cannot match a pattern of names. SES's bounce and complaint metrics are the nearest |
| `MyEmailRetrying` | Not expressible | The same |
| `MyMemoryNearLimit` | Not expressible | Lambda publishes no memory metric without Lambda Insights, and `go.*` gauges come only with the snapshot job, from one process every 15 minutes |
| `MyProcessRestarting` | Not expressible | An environment that starts and stops is what Lambda does, so restarts mean nothing here |
| `MyDBPoolSaturated` | Not expressible | The pool's gauges come only with the snapshot job, from one process every 15 minutes. Its effect shows in `MyHTTPErrorsHigh` and `MyAPILatencyHigh` |
| `MyDBPoolWaiting` | Not expressible | The same |
| `MyFileDescriptorsNearLimit` | Not expressible | The same, and Lambda sets the limit |

The runbook's budget entry is outside this module: use AWS Budgets.

## Adding other notification channels

Email is the input because it needs nothing but an address. Anything else subscribes to an SNS topic of its own, or to `topic_arn`, whose ARN goes in `notification_topic_arns`, or in `page_notification_topic_arns` for pages alone:

```hcl
resource "aws_sns_topic" "pager" {
  name = "my-pager"
}

resource "aws_sns_topic_subscription" "pager" {
  topic_arn = aws_sns_topic.pager.arn
  protocol  = "https"
  endpoint  = "https://hooks.example/my"
}

module "monitoring" {
  # ...
  page_notification_topic_arns = [aws_sns_topic.pager.arn]
}
```

A URL that holds a secret lands in the state if you make the subscription here, so make such a one in the console.

## What stays unverified

Read from `tofu providers schema -json` and AWS's documents, and never run:

- That the canary's script runs on that runtime without the Synthetics library, and that a thrown error fails the run. A Puppeteer runtime starts a browser it never uses here; a lighter one was not looked for.
- That the zip's layout fits: the documents say scripts may sit at the root from `syn-nodejs-puppeteer-11.0`.
- That `MAX([q - 300, t - 3600])`, `DIFF`, and `IF` work in an alarm, the limit of 10 metrics an alarm reads, and how a period of a day evaluates.
- That `SuccessPercent` has one sample per period, with no empty period that would count as a failure.
- That SNS's default policy lets CloudWatch publish.
- That the names and dimensions of the EMF lines become the metrics that the alarms name, since no function has run.
- The bill: the number of metrics is an estimate.
