# SPDX-License-Identifier: AGPL-3.0-only

terraform {
  required_version = ">= 1.11"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 7.0, < 8.0"
    }
  }
}

locals {
  service  = "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${var.service_name}\""
  snapshot = "${local.service} AND jsonPayload.msg=\"metrics snapshot\""

  # A log-based counter counts the lines of its filter. Every alert on these fires on lines that exist, so an idle
  # service with no lines fires nothing. A value compared in the filter stands in for a gauge, which a counter cannot carry.
  log_counters = merge(
    {
      job_runs = {
        description = "Job runs by type and outcome."
        filter      = "${local.service} AND jsonPayload.msg=\"job\""
        labels      = { my_job_type = "EXTRACT(jsonPayload.\"my.job.type\")", my_outcome = "EXTRACT(jsonPayload.\"my.outcome\")" }
      }
      inbox_requests = {
        description = "POSTs to the inboxes by status."
        filter      = "${local.service} AND jsonPayload.msg=\"http request\" AND jsonPayload.\"http.request.method\"=\"POST\" AND jsonPayload.\"http.route\"=~\"/inbox$\""
        labels      = { status = "EXTRACT(jsonPayload.\"http.response.status_code\")" }
      }
      error_reports = {
        description = "Error reports of the server."
        filter      = "${local.service} AND jsonPayload.msg=\"error report\""
        labels      = {}
      }
      not_ready = {
        description = "Readiness checks that failed."
        filter      = "${local.service} AND jsonPayload.msg=\"not ready\""
        labels      = {}
      }
      metrics_snapshots = {
        description = "Lines of the metrics snapshot job, which the tick runs whether or not anyone uses the server."
        filter      = local.snapshot
        labels      = {}
      }
      queue_backlog = {
        description = "Snapshots in which the oldest ready job has waited over 5 minutes."
        filter      = "${local.snapshot} AND jsonPayload.\"my.queue.oldest_ready_age\">300"
        labels      = {}
      }
      queue_stalled = {
        description = "Snapshots in which the oldest ready job has waited over 30 minutes."
        filter      = "${local.snapshot} AND jsonPayload.\"my.queue.oldest_ready_age\">1800"
        labels      = {}
      }
    },
    var.database_size_limit_bytes == null ? {} : {
      database_large = {
        description = "Snapshots in which the database is over the limit."
        filter      = "${local.snapshot} AND jsonPayload.\"my.db.size\">${var.database_size_limit_bytes}"
        labels      = {}
      }
    },
    var.media_size_limit_bytes == null ? {} : {
      media_large = {
        description = "Snapshots in which the local media are over the limit."
        filter      = "${local.snapshot} AND jsonPayload.\"my.media.stored_bytes\".local>${var.media_size_limit_bytes}"
        labels      = {}
      }
    },
    var.remote_media_cache_limit_bytes == null ? {} : {
      remote_media_large = {
        description = "Snapshots in which the remote media cache is over the limit."
        filter      = "${local.snapshot} AND jsonPayload.\"my.media.stored_bytes\".remote_cache>${var.remote_media_cache_limit_bytes}"
        labels      = {}
      }
    },
    length(var.billing_sources) == 0 ? {} : {
      billing_failing = {
        description = "Snapshots in which the newest import of a billing source failed."
        filter      = "${local.snapshot} AND (${join(" OR ", [for source in var.billing_sources : "jsonPayload.\"my.billing.import.failing\".${source}=true"])})"
        labels      = {}
      }
    },
  )

  # How PromQL names these metrics and Cloud Run's: the first / becomes :, and every other special character _.
  resource = "monitored_resource=\"cloud_run_revision\""
  log      = "logging_googleapis_com:user_${var.name}"
  requests = "run_googleapis_com:request_count{${local.resource},service_name=\"${var.service_name}\""
  memory   = "run_googleapis_com:container_memory_utilizations_bucket{${local.resource},service_name=\"${var.service_name}\"}"
  jobs     = "${local.log}_job_runs{${local.resource}"
  deliver  = "${local.jobs},my_job_type=\"federation.deliver\""
  email    = "${local.jobs},my_job_type=~\".*[._]email.*\""
  inbox    = "${local.log}_inbox_requests{${local.resource}"
  errors   = "${local.log}_error_reports{${local.resource}}"

  # The names, thresholds, and durations of deploy/monitoring/prometheus/alerts.yml; the README says where each differs.
  # An empty result fires nothing, so no traffic means no alert. Only MyTargetDown and MyMetricsSnapshotStale, below,
  # watch something that goes on arriving while the service is idle.
  promql_alerts = merge(
    {
      MyQueueBacklog = {
        severity = "WARNING"
        for      = "0s"
        query    = "increase(${local.log}_queue_backlog{${local.resource}}[20m]) > 0"
      }
      MyQueueStalled = {
        severity = "CRITICAL"
        for      = "0s"
        query    = "increase(${local.log}_queue_stalled{${local.resource}}[20m]) > 0"
      }
      MyDeadJobsIncreasing = {
        severity = "WARNING"
        for      = "900s"
        query    = "sum(increase(${local.jobs},my_outcome=\"dead\"}[1h])) > 10"
      }
      MyDeliveryFailuresHigh = {
        severity = "WARNING"
        for      = "1800s"
        query    = "sum(rate(${local.deliver},my_outcome=~\"retry|dead\"}[15m])) / sum(rate(${local.deliver}}[15m])) > 0.5 and sum(rate(${local.deliver}}[15m])) > 0.01"
      }
      MyDeliveryAllFailing = {
        severity = "CRITICAL"
        for      = "900s"
        query    = "sum(rate(${local.deliver},my_outcome=~\"retry|dead\"}[15m])) / sum(rate(${local.deliver}}[15m])) > 0.95 and sum(rate(${local.deliver}}[15m])) > 0.05"
      }
      MyInboxRejectionsHigh = {
        severity = "WARNING"
        for      = "1800s"
        query    = "sum(rate(${local.inbox},status=~\"4..|503\"}[15m])) / sum(rate(${local.inbox}}[15m])) > 0.25 and sum(rate(${local.inbox}}[15m])) > 0.01"
      }
      MyInboxSignatureFailures = {
        severity = "WARNING"
        for      = "1800s"
        query    = "sum(rate(${local.inbox},status=\"401\"}[15m])) / sum(rate(${local.inbox}}[15m])) > 0.1 and sum(rate(${local.inbox}}[15m])) > 0.01"
      }
      MyInboxErrors = {
        severity = "WARNING"
        for      = "900s"
        query    = "sum(rate(${local.inbox},status=\"500\"}[15m])) / sum(rate(${local.inbox}}[15m])) > 0.05"
      }
      MyErrorReportsIncreasing = {
        severity = "WARNING"
        for      = "900s"
        query    = "sum(increase(${local.errors}[15m])) > 10 and sum(increase(${local.errors}[15m])) > 3 * (sum(increase(${local.errors}[1d] offset 15m)) / 96 or vector(0))"
      }
      MyHTTPErrorsHigh = {
        severity = "CRITICAL"
        for      = "600s"
        query    = "sum(rate(${local.requests},response_code_class=\"5xx\"}[5m])) / sum(rate(${local.requests}}[5m])) > 0.05 and sum(rate(${local.requests}}[5m])) > 0.1"
      }
      MyHTTPErrorsElevated = {
        severity = "WARNING"
        for      = "1800s"
        query    = "sum(rate(${local.requests},response_code_class=\"5xx\"}[30m])) / sum(rate(${local.requests}}[30m])) > 0.01 and sum(rate(${local.requests}}[30m])) > 0.01"
      }
      MyAPILatencyHigh = {
        severity = "WARNING"
        for      = "1800s"
        query    = "histogram_quantile(0.95, sum by (le) (rate(${local.log}_api_request_duration_bucket{${local.resource}}[10m]))) > 0.3"
      }
      MyNotReady = {
        severity = "CRITICAL"
        for      = "300s"
        query    = "sum(increase(${local.log}_not_ready{${local.resource}}[15m])) > 1"
      }
      MyMemoryNearLimit = {
        severity = "WARNING"
        for      = "900s"
        query    = "histogram_quantile(0.95, sum by (le) (rate(${local.memory}[15m]))) > 0.81"
      }
      MyEmailFailing = {
        severity = "WARNING"
        for      = "0s"
        query    = "sum(increase(${local.email},my_outcome=\"dead\"}[1h])) > 0"
      }
      MyEmailRetrying = {
        severity = "WARNING"
        for      = "900s"
        query    = "sum(rate(${local.email},my_outcome=\"retry\"}[15m])) / sum(rate(${local.email}}[15m])) > 0.5"
      }
    },
    # The snapshot arrives every 15 minutes and may skip one, so these windows outlast two of them.
    var.database_size_limit_bytes == null ? {} : {
      MyDatabaseStorageLarge = {
        severity = "WARNING"
        for      = "3600s"
        query    = "increase(${local.log}_database_large{${local.resource}}[35m]) > 0"
      }
    },
    var.media_size_limit_bytes == null ? {} : {
      MyMediaStorageLarge = {
        severity = "WARNING"
        for      = "3600s"
        query    = "increase(${local.log}_media_large{${local.resource}}[35m]) > 0"
      }
    },
    var.remote_media_cache_limit_bytes == null ? {} : {
      MyRemoteMediaCacheOverLimit = {
        severity = "WARNING"
        for      = "10800s"
        query    = "increase(${local.log}_remote_media_large{${local.resource}}[35m]) > 0"
      }
    },
    length(var.billing_sources) == 0 ? {} : {
      MyBillingImportFailing = {
        severity = "WARNING"
        for      = "21600s"
        query    = "increase(${local.log}_billing_failing{${local.resource}}[35m]) > 0"
      }
    },
  )

  enabled_promql_alerts = { for name, alert in local.promql_alerts : name => alert if !contains(var.disabled_alerts, name) }

  # What the module cannot express: the README says why for each.
  not_expressible = [
    "MyRemoteServersPaused",
    "MyProcessRestarting",
    "MyDBPoolSaturated",
    "MyDBPoolWaiting",
    "MyFileDescriptorsNearLimit",
    "MyAdapterErrors",
  ]

  channels      = concat([for channel in google_monitoring_notification_channel.email : channel.id], var.notification_channels)
  page_channels = concat(local.channels, var.page_notification_channels)
  period        = tonumber(trimsuffix(var.uptime_period, "s"))
  runbook       = "https://github.com/mycroblog/my/blob/main/docs/operations/alerts.md"
}

resource "google_monitoring_notification_channel" "email" {
  for_each = toset(var.notification_emails)

  project      = var.project_id
  display_name = "${var.name} alerts: ${each.value}"
  type         = "email"
  labels = {
    email_address = each.value
  }
}

resource "google_logging_metric" "counter" {
  for_each = local.log_counters

  project          = var.project_id
  name             = "${var.name}_${each.key}"
  description      = each.value.description
  filter           = each.value.filter
  label_extractors = each.value.labels

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"

    dynamic "labels" {
      for_each = each.value.labels
      content {
        key = labels.key
      }
    }
  }
}

# The server's own duration, so the wake-up of the platform and the streams, which stay open, are left out. 0.3 is a
# bound, so that the 95th percentile falls on the right side of the threshold.
resource "google_logging_metric" "api_request_duration" {
  project         = var.project_id
  name            = "${var.name}_api_request_duration"
  description     = "Duration of the requests of the client API, the streaming endpoint left out."
  filter          = "${local.service} AND jsonPayload.msg=\"http request\" AND jsonPayload.\"my.surface\"=\"client_api\" AND NOT jsonPayload.\"http.route\"=\"/api/v1/streaming\""
  value_extractor = "EXTRACT(jsonPayload.duration_seconds)"

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "DISTRIBUTION"
    unit        = "s"
  }

  bucket_options {
    explicit_buckets {
      bounds = [0.05, 0.1, 0.2, 0.3, 0.5, 1, 2, 5, 10]
    }
  }
}

resource "google_monitoring_uptime_check_config" "ready" {
  project          = var.project_id
  display_name     = "${var.name} ${var.uptime_path}"
  period           = var.uptime_period
  timeout          = var.uptime_timeout
  selected_regions = var.uptime_regions

  monitored_resource {
    type = "uptime_url"
    labels = {
      project_id = var.project_id
      host       = var.host
    }
  }

  http_check {
    path         = var.uptime_path
    port         = 443
    use_ssl      = true
    validate_ssl = true
  }
}

resource "google_monitoring_alert_policy" "target_down" {
  project               = var.project_id
  display_name          = "MyTargetDown"
  combiner              = "OR"
  severity              = "CRITICAL"
  notification_channels = local.page_channels

  # The checkers go on reporting whatever the service does, so this one needs no traffic. More than one checker must
  # fail, so that one checker's bad network pages nobody.
  conditions {
    display_name = "More than one checker fails ${var.uptime_path} on ${var.host}"
    condition_threshold {
      filter          = "metric.type=\"monitoring.googleapis.com/uptime_check/check_passed\" AND resource.type=\"uptime_url\" AND metric.label.check_id=\"${google_monitoring_uptime_check_config.ready.uptime_check_id}\""
      comparison      = "COMPARISON_GT"
      threshold_value = 1
      duration        = "${max(local.period, 300)}s"

      aggregations {
        alignment_period     = "${max(local.period, 300)}s"
        per_series_aligner   = "ALIGN_NEXT_OLDER"
        cross_series_reducer = "REDUCE_COUNT_FALSE"
        group_by_fields      = ["resource.label.host"]
      }

      trigger {
        count = 1
      }
    }
  }

  documentation {
    subject   = "MyTargetDown"
    mime_type = "text/markdown"
    content   = "${var.uptime_path} on ${var.host} does not answer. Runbook: ${local.runbook}#mytargetdown"
  }

  lifecycle {
    enabled = !contains(var.disabled_alerts, "MyTargetDown")
  }
}

resource "google_monitoring_alert_policy" "snapshot_stale" {
  project               = var.project_id
  display_name          = "MyMetricsSnapshotStale"
  combiner              = "OR"
  severity              = "WARNING"
  notification_channels = local.channels

  # The one alert on absence that a scale-to-zero service can have: the tick starts the snapshot job on an idle server
  # too, so a gap means that the tick or the queue stopped, not that nobody came.
  conditions {
    display_name = "No metrics snapshot for ${var.snapshot_stale_after_seconds} seconds"
    condition_absent {
      filter   = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.counter["metrics_snapshots"].name}\" AND resource.type=\"cloud_run_revision\""
      duration = "${var.snapshot_stale_after_seconds}s"

      aggregations {
        alignment_period     = "900s"
        per_series_aligner   = "ALIGN_SUM"
        cross_series_reducer = "REDUCE_SUM"
      }

      trigger {
        count = 1
      }
    }
  }

  documentation {
    subject   = "MyMetricsSnapshotStale"
    mime_type = "text/markdown"
    content   = "No metrics snapshot line arrived, so the queue and storage alerts see nothing. Runbook: ${local.runbook}#mymetricssnapshotstale"
  }

  lifecycle {
    enabled = !contains(var.disabled_alerts, "MyMetricsSnapshotStale")
  }
}

resource "google_monitoring_alert_policy" "promql" {
  for_each = local.enabled_promql_alerts

  project               = var.project_id
  display_name          = each.key
  combiner              = "OR"
  severity              = each.value.severity
  notification_channels = each.value.severity == "CRITICAL" ? local.page_channels : local.channels

  conditions {
    display_name = each.key
    condition_prometheus_query_language {
      query      = each.value.query
      duration   = each.value.for
      alert_rule = each.key
      rule_group = "my"
      # A log-based metric may have no descriptor until its first line.
      disable_metric_validation = true
    }
  }

  documentation {
    subject   = each.key
    mime_type = "text/markdown"
    content   = "Runbook: ${local.runbook}#${lower(each.key)}"
  }

  depends_on = [google_logging_metric.counter, google_logging_metric.api_request_duration]
}
