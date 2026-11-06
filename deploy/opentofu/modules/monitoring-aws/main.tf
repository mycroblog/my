# SPDX-License-Identifier: AGPL-3.0-only

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.0, < 7.0"
    }
    archive = {
      source  = "hashicorp/archive"
      version = ">= 2.4, < 3.0"
    }
  }
}

data "aws_region" "current" {}
data "aws_caller_identity" "current" {}
data "aws_partition" "current" {}

locals {
  ns = var.metrics_emf_namespace

  # CloudWatch matches a metric on its exact set of dimensions, and bills each combination of their values. These are
  # the labels the alarms below name. A label added here changes the dimensions of every metric that has it, so the
  # alarms of those metrics would see nothing: it is not an input.
  emf_dimensions = [
    "my.outcome",
    "my.surface",
    "my.inbox.rejection",
    "my.metrics.snapshot.part",
    "my.federation.instance.state",
    "my.media.origin",
  ]

  delivery = {
    for outcome in ["success", "retry", "gone", "failed"] :
    "d_${outcome}" => { namespace = local.ns, name = "my.delivery.attempts", stat = "Sum", dimensions = { "my.outcome" = outcome } }
  }
  delivery_all    = join(" + ", [for id in keys(local.delivery) : "FILL(${id},0)"])
  delivery_failed = "(FILL(d_retry,0) + FILL(d_failed,0))"

  inbox = merge(
    {
      for outcome in ["accepted", "duplicate", "ignored", "error"] :
      "i_${outcome}" => { namespace = local.ns, name = "my.inbox.activities", stat = "Sum", dimensions = { "my.outcome" = outcome } }
    },
    {
      for reason in ["signature", "suspended", "federation_off", "rate_limited", "invalid"] :
      "r_${reason}" => { namespace = local.ns, name = "my.inbox.activities", stat = "Sum", dimensions = { "my.outcome" = "rejected", "my.inbox.rejection" = reason } }
    },
  )
  inbox_all      = join(" + ", [for id in keys(local.inbox) : "FILL(${id},0)"])
  inbox_rejected = join(" + ", [for id in keys(local.inbox) : "FILL(${id},0)" if startswith(id, "r_")])

  http = {
    t = { namespace = "AWS/ApplicationELB", name = "HTTPCode_Target_5XX_Count", stat = "Sum", dimensions = { LoadBalancer = var.load_balancer_arn_suffix } }
    l = { namespace = "AWS/ApplicationELB", name = "HTTPCode_ELB_5XX_Count", stat = "Sum", dimensions = { LoadBalancer = var.load_balancer_arn_suffix } }
    r = { namespace = "AWS/ApplicationELB", name = "RequestCount", stat = "Sum", dimensions = { LoadBalancer = var.load_balancer_arn_suffix } }
  }
  http_errors = "(FILL(t,0) + FILL(l,0))"

  adapter = {
    for outcome in ["success", "error"] :
    "a_${outcome}" => { namespace = local.ns, name = "my.adapter.operation.duration", stat = "SampleCount", dimensions = { "my.outcome" = outcome } }
  }

  queue_age = { m = { namespace = local.ns, name = "my.queue.oldest_ready_age", stat = "Maximum", dimensions = {} } }

  # period is of each sample, periods how many must breach; together they approach the for of the Prometheus rule.
  # missing is what a sample without data counts as. A request or job metric has gaps on a server without traffic,
  # so it is notBreaching; only MyTargetDown and MyMetricsSnapshotStale watch something that goes on arriving when idle.
  alarms = merge(
    {
      MyTargetDown = {
        severity   = "page", comparison = "LessThanThreshold", threshold = 100, missing = "breaching"
        period     = local.canary_period, periods = ceil(300 / local.canary_period)
        expression = "m"
        metrics    = { m = { namespace = "CloudWatchSynthetics", name = "SuccessPercent", stat = "Average", dimensions = { CanaryName = local.canary_name } } }
      }
      MyMetricsSnapshotStale = {
        severity   = "ticket", comparison = "GreaterThanThreshold", threshold = 0, missing = "breaching"
        period     = 3600, periods = 1
        expression = "MAX([q - 300, t - 3600])"
        metrics = {
          q = { namespace = local.ns, name = "my.metrics.snapshot.age", stat = "Maximum", dimensions = { "my.metrics.snapshot.part" = "queue" } }
          t = { namespace = local.ns, name = "my.metrics.snapshot.age", stat = "Maximum", dimensions = { "my.metrics.snapshot.part" = "totals" } }
        }
      }
      MyQueueBacklog = {
        severity   = "ticket", comparison = "GreaterThanThreshold", threshold = 300, missing = "notBreaching"
        period     = 300, periods = 2
        expression = "m"
        metrics    = local.queue_age
      }
      MyQueueStalled = {
        severity   = "page", comparison = "GreaterThanThreshold", threshold = 1800, missing = "notBreaching"
        period     = 300, periods = 1
        expression = "m"
        metrics    = local.queue_age
      }
      MyDeadJobsIncreasing = {
        severity   = "ticket", comparison = "GreaterThanThreshold", threshold = 10, missing = "notBreaching"
        period     = 3600, periods = 1
        expression = "m"
        metrics    = { m = { namespace = local.ns, name = "my.job.duration", stat = "SampleCount", dimensions = { "my.outcome" = "dead" } } }
      }
      MyDeliveryFailuresHigh = {
        severity   = "ticket", comparison = "GreaterThanThreshold", threshold = 0.5, missing = "notBreaching"
        period     = 900, periods = 2
        expression = "IF((${local.delivery_all}) > 9, ${local.delivery_failed} / (${local.delivery_all}), 0)"
        metrics    = local.delivery
      }
      MyDeliveryAllFailing = {
        severity   = "page", comparison = "GreaterThanThreshold", threshold = 0.95, missing = "notBreaching"
        period     = 900, periods = 1
        expression = "IF((${local.delivery_all}) > 45, ${local.delivery_failed} / (${local.delivery_all}), 0)"
        metrics    = local.delivery
      }
      MyInboxRejectionsHigh = {
        severity   = "ticket", comparison = "GreaterThanThreshold", threshold = 0.25, missing = "notBreaching"
        period     = 900, periods = 2
        expression = "IF((${local.inbox_all}) > 9, (${local.inbox_rejected}) / (${local.inbox_all}), 0)"
        metrics    = local.inbox
      }
      MyInboxSignatureFailures = {
        severity   = "ticket", comparison = "GreaterThanThreshold", threshold = 0.1, missing = "notBreaching"
        period     = 900, periods = 2
        expression = "IF((${local.inbox_all}) > 9, FILL(r_signature,0) / (${local.inbox_all}), 0)"
        metrics    = local.inbox
      }
      MyInboxErrors = {
        severity   = "ticket", comparison = "GreaterThanThreshold", threshold = 0.05, missing = "notBreaching"
        period     = 900, periods = 1
        expression = "FILL(i_error,0) / (${local.inbox_all})"
        metrics    = local.inbox
      }
      MyRemoteServersPaused = {
        severity   = "ticket", comparison = "GreaterThanOrEqualToThreshold", threshold = 5, missing = "notBreaching"
        period     = 86400, periods = 1
        expression = "DIFF(m)"
        metrics    = { m = { namespace = local.ns, name = "my.federation.instances", stat = "Maximum", dimensions = { "my.federation.instance.state" = "unavailable" } } }
      }
      MyErrorReportsIncreasing = {
        severity   = "ticket", comparison = "GreaterThanThreshold", threshold = 10, missing = "notBreaching"
        period     = 900, periods = 2
        expression = "m"
        metrics    = { m = { namespace = local.ns, name = "my.errors.reported", stat = "Sum", dimensions = {} } }
      }
      MyHTTPErrorsHigh = {
        severity   = "page", comparison = "GreaterThanThreshold", threshold = 0.05, missing = "notBreaching"
        period     = 300, periods = 2
        expression = "IF(r > 30, ${local.http_errors} / r, 0)"
        metrics    = local.http
      }
      MyHTTPErrorsElevated = {
        severity   = "ticket", comparison = "GreaterThanThreshold", threshold = 0.01, missing = "notBreaching"
        period     = 1800, periods = 1
        expression = "IF(r > 18, ${local.http_errors} / r, 0)"
        metrics    = local.http
      }
      MyAPILatencyHigh = {
        severity   = "ticket", comparison = "GreaterThanThreshold", threshold = 0.3, missing = "notBreaching"
        period     = 600, periods = 3
        expression = "m"
        metrics    = { m = { namespace = local.ns, name = "http.server.request.duration", stat = "p95", dimensions = { "my.surface" = "client_api" } } }
      }
      MyNotReady = {
        severity   = "page", comparison = "GreaterThanThreshold", threshold = 1, missing = "notBreaching"
        period     = 900, periods = 1
        expression = "m"
        metrics    = { m = { namespace = local.ns, name = "my.readiness.failures", stat = "Sum", dimensions = {} } }
      }
      MyAdapterErrors = {
        severity   = "ticket", comparison = "GreaterThanThreshold", threshold = 0.05, missing = "notBreaching"
        period     = 300, periods = 3
        expression = "FILL(a_error,0) / (FILL(a_error,0) + FILL(a_success,0))"
        metrics    = local.adapter
      }
      MyBillingImportFailing = {
        severity   = "ticket", comparison = "GreaterThanOrEqualToThreshold", threshold = 1, missing = "notBreaching"
        period     = 3600, periods = 6
        expression = "m"
        metrics    = { m = { namespace = local.ns, name = "my.billing.import.failing", stat = "Maximum", dimensions = {} } }
      }
    },
    var.database_size_limit_bytes == null ? {} : {
      MyDatabaseStorageLarge = {
        severity   = "ticket", comparison = "GreaterThanThreshold", threshold = var.database_size_limit_bytes, missing = "notBreaching"
        period     = 3600, periods = 1
        expression = "m"
        metrics    = { m = { namespace = local.ns, name = "my.db.size", stat = "Maximum", dimensions = {} } }
      }
    },
    var.media_size_limit_bytes == null ? {} : {
      MyMediaStorageLarge = {
        severity   = "ticket", comparison = "GreaterThanThreshold", threshold = var.media_size_limit_bytes, missing = "notBreaching"
        period     = 3600, periods = 1
        expression = "FILL(l,0) + FILL(c,0)"
        metrics = {
          l = { namespace = local.ns, name = "my.media.stored_bytes", stat = "Maximum", dimensions = { "my.media.origin" = "local" } }
          c = { namespace = local.ns, name = "my.media.stored_bytes", stat = "Maximum", dimensions = { "my.media.origin" = "remote_cache" } }
        }
      }
    },
    var.remote_media_cache_limit_bytes == null ? {} : {
      MyRemoteMediaCacheOverLimit = {
        severity   = "ticket", comparison = "GreaterThanThreshold", threshold = var.remote_media_cache_limit_bytes, missing = "notBreaching"
        period     = 3600, periods = 3
        expression = "m"
        metrics    = { m = { namespace = local.ns, name = "my.media.stored_bytes", stat = "Maximum", dimensions = { "my.media.origin" = "remote_cache" } } }
      }
    },
  )

  enabled_alarms = { for name, alarm in local.alarms : name => alarm if !contains(var.disabled_alerts, name) }

  # What the module cannot express: the README says why for each.
  not_expressible = [
    "MyEmailFailing",
    "MyEmailRetrying",
    "MyMemoryNearLimit",
    "MyProcessRestarting",
    "MyDBPoolSaturated",
    "MyDBPoolWaiting",
    "MyFileDescriptorsNearLimit",
  ]

  topics      = concat([aws_sns_topic.alerts.arn], var.notification_topic_arns)
  page_topics = concat(local.topics, var.page_notification_topic_arns)
  runbook     = "https://github.com/mycroblog/my/blob/main/docs/operations/alerts.md"

  canary_name   = "${var.name}-ready"
  canary_period = var.uptime_period_minutes * 60
  canary_logs   = "arn:${data.aws_partition.current.partition}:logs:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:log-group:/aws/lambda/cwsyn-${local.canary_name}-*"
}

resource "aws_sns_topic" "alerts" {
  name = "${var.name}-alerts"
}

resource "aws_sns_topic_subscription" "email" {
  for_each = toset(var.notification_emails)

  topic_arn = aws_sns_topic.alerts.arn
  protocol  = "email"
  endpoint  = each.value
}

resource "aws_s3_bucket" "canary" {
  bucket_prefix = "${var.name}-canary-"
  # Only the runs' reports are in it, and they expire anyway.
  force_destroy = true
}

resource "aws_s3_bucket_public_access_block" "canary" {
  bucket                  = aws_s3_bucket.canary.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_lifecycle_configuration" "canary" {
  bucket = aws_s3_bucket.canary.id

  rule {
    id     = "expire-reports"
    status = "Enabled"

    filter {}

    expiration {
      days = 7
    }
  }
}

data "aws_iam_policy_document" "canary_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "canary" {
  name               = "${var.name}-canary"
  assume_role_policy = data.aws_iam_policy_document.canary_assume.json
}

# The statements of "Basic canary" in the Synthetics documentation.
data "aws_iam_policy_document" "canary" {
  statement {
    actions   = ["s3:PutObject", "s3:GetObject"]
    resources = ["${aws_s3_bucket.canary.arn}/*"]
  }
  statement {
    actions   = ["s3:GetBucketLocation"]
    resources = [aws_s3_bucket.canary.arn]
  }
  statement {
    actions   = ["logs:CreateLogStream", "logs:PutLogEvents", "logs:CreateLogGroup"]
    resources = [local.canary_logs, "${local.canary_logs}:*"]
  }
  statement {
    actions   = ["s3:ListAllMyBuckets", "xray:PutTraceSegments"]
    resources = ["*"]
  }
  statement {
    actions   = ["cloudwatch:PutMetricData"]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "cloudwatch:namespace"
      values   = ["CloudWatchSynthetics"]
    }
  }
}

resource "aws_iam_role_policy" "canary" {
  name   = "canary"
  role   = aws_iam_role.canary.id
  policy = data.aws_iam_policy_document.canary.json
}

data "archive_file" "canary" {
  type        = "zip"
  output_path = "${path.root}/.terraform/tmp/${local.canary_name}.zip"

  source {
    filename = "ready.js"
    content  = <<-EOT
      exports.handler = async () => {
        const url = process.env.READY_URL;
        const response = await fetch(url, { redirect: "error" });
        if (!response.ok) {
          throw new Error(url + " answered " + response.status);
        }
      };
    EOT
  }
}

resource "aws_synthetics_canary" "ready" {
  name                 = local.canary_name
  artifact_s3_location = "s3://${aws_s3_bucket.canary.bucket}/runs/"
  execution_role_arn   = aws_iam_role.canary.arn
  handler              = "ready.handler"
  runtime_version      = var.canary_runtime_version
  zip_file             = data.archive_file.canary.output_path
  start_canary         = true

  success_retention_period = 7
  failure_retention_period = 31

  schedule {
    expression = "rate(${var.uptime_period_minutes} minute${var.uptime_period_minutes == 1 ? "" : "s"})"
    # A database that wakes up slowly gets a second chance before the run counts as failed.
    retry_config {
      max_retries = 1
    }
  }

  run_config {
    # Enough for a database that scales to zero to wake up.
    timeout_in_seconds = 60
    environment_variables = {
      READY_URL = "https://${var.host}${var.uptime_path}"
    }
  }

  depends_on = [aws_iam_role_policy.canary]
}

resource "aws_cloudwatch_metric_alarm" "alert" {
  for_each = local.enabled_alarms

  alarm_name          = each.key
  alarm_description   = "Severity: ${each.value.severity}. Runbook: ${local.runbook}#${lower(each.key)}"
  comparison_operator = each.value.comparison
  threshold           = each.value.threshold
  evaluation_periods  = each.value.periods
  treat_missing_data  = each.value.missing
  alarm_actions       = each.value.severity == "page" ? local.page_topics : local.topics
  ok_actions          = each.value.severity == "page" ? local.page_topics : local.topics

  metric_query {
    id          = "result"
    expression  = each.value.expression
    label       = each.key
    return_data = true
  }

  dynamic "metric_query" {
    for_each = each.value.metrics
    content {
      id          = metric_query.key
      return_data = false
      metric {
        namespace   = metric_query.value.namespace
        metric_name = metric_query.value.name
        stat        = metric_query.value.stat
        period      = each.value.period
        dimensions  = metric_query.value.dimensions
      }
    }
  }
}
