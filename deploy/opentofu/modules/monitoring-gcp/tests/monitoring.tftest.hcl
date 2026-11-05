# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches Google Cloud, so none of the queries or filters has been evaluated.

mock_provider "google" {
  mock_resource "google_service_account" {
    defaults = {
      email  = "my-server@p.iam.gserviceaccount.com"
      member = "serviceAccount:my-server@p.iam.gserviceaccount.com"
    }
  }
}

variables {
  project_id                     = "p"
  host                           = "social.example"
  notification_emails            = ["ops@social.example"]
  database_size_limit_bytes      = 8589934592
  media_size_limit_bytes         = 53687091200
  remote_media_cache_limit_bytes = 1288490188
  billing_sources                = ["aws", "gcp"]
}

run "every_alert_of_the_runbook_is_a_policy_or_listed_as_not_expressible" {
  command = plan

  assert {
    condition = (
      toset(concat(output.alerts.policies, output.alerts.not_expressible))
      == toset(flatten(regexall("(?m)- alert: (My[A-Za-z]+)$", file("${path.module}/../../../monitoring/prometheus/alerts.yml"))))
    )
    error_message = "the names must be those of deploy/monitoring/prometheus/alerts.yml, each of them once"
  }

  assert {
    condition = alltrue([
      for name in concat(output.alerts.policies, output.alerts.not_expressible) :
      strcontains(file("${path.module}/README.md"), "`${name}`")
    ])
    error_message = "the README must list every alert"
  }

  assert {
    condition     = length(setintersection(toset(output.alerts.policies), toset(output.alerts.not_expressible))) == 0
    error_message = "an alert is one or the other"
  }
}

run "the_default_check_is_of_readyz_at_the_longest_period_from_three_checkers" {
  command = plan

  assert {
    condition     = google_monitoring_uptime_check_config.ready.http_check[0].path == "/readyz"
    error_message = "the check is of /readyz"
  }

  assert {
    condition     = google_monitoring_uptime_check_config.ready.period == "900s"
    error_message = "the longest period is the default, since each run costs a wake-up"
  }

  assert {
    condition     = length(google_monitoring_uptime_check_config.ready.selected_regions) == 3
    error_message = "the fewest checkers Cloud Monitoring takes, so the fewest requests"
  }

  assert {
    condition     = google_monitoring_uptime_check_config.ready.monitored_resource[0].labels["host"] == "social.example"
    error_message = "the check calls the host"
  }

  assert {
    condition     = tomap(output.env) == tomap({ MY_METRICS_EXPORTER = "none" })
    error_message = "the alerts read the log lines of none, and the app fails the plan when another block sets the exporter"
  }
}

run "the_period_is_an_input" {
  command = plan

  variables {
    uptime_period = "300s"
  }

  assert {
    condition     = google_monitoring_uptime_check_config.ready.period == "300s"
    error_message = "the period comes from the input"
  }

  assert {
    condition     = google_monitoring_alert_policy.target_down.conditions[0].condition_threshold[0].duration == "300s"
    error_message = "the condition lasts one period"
  }
}

run "a_period_cloud_monitoring_has_not_fails" {
  command = plan

  variables {
    uptime_period = "120s"
  }

  expect_failures = [var.uptime_period]
}

run "only_the_check_and_the_snapshot_watch_for_absence" {
  command = plan

  assert {
    condition     = length(google_monitoring_alert_policy.snapshot_stale.conditions[0].condition_absent) == 1
    error_message = "the snapshot comes from the tick, so its absence means something"
  }

  assert {
    condition = alltrue([
      for policy in google_monitoring_alert_policy.promql : length(policy.conditions[0].condition_absent) == 0
    ])
    error_message = "a request or job metric has gaps on a service that scales to zero, so an alert on its absence would fire falsely"
  }
}

run "the_filters_compare_the_snapshot_with_the_limits" {
  command = plan

  assert {
    condition     = strcontains(google_logging_metric.counter["database_large"].filter, "jsonPayload.\"my.db.size\">8589934592")
    error_message = "the limit goes into the filter"
  }

  assert {
    condition     = strcontains(google_logging_metric.counter["queue_stalled"].filter, "jsonPayload.\"my.queue.oldest_ready_age\">1800")
    error_message = "the threshold of MyQueueStalled is 30 minutes"
  }

  assert {
    condition     = strcontains(google_logging_metric.counter["billing_failing"].filter, "jsonPayload.\"my.billing.import.failing\".aws=true OR jsonPayload.\"my.billing.import.failing\".gcp=true")
    error_message = "every billing source is watched"
  }

  assert {
    condition     = google_monitoring_alert_policy.promql["MyQueueStalled"].severity == "CRITICAL" && google_monitoring_alert_policy.promql["MyQueueBacklog"].severity == "WARNING"
    error_message = "page is CRITICAL and ticket is WARNING"
  }
}

run "an_unset_limit_leaves_its_alert_out" {
  command = plan

  variables {
    database_size_limit_bytes      = null
    media_size_limit_bytes         = null
    remote_media_cache_limit_bytes = null
    billing_sources                = []
  }

  assert {
    condition     = length(setintersection(toset(output.alerts.policies), toset(["MyDatabaseStorageLarge", "MyMediaStorageLarge", "MyRemoteMediaCacheOverLimit", "MyBillingImportFailing"]))) == 0
    error_message = "a limit that nobody set is no alert"
  }

  assert {
    condition     = !contains(keys(google_logging_metric.counter), "database_large")
    error_message = "and no metric"
  }
}

run "the_channels_get_the_alerts_and_a_pager_gets_the_pages" {
  command = plan

  variables {
    notification_channels      = ["projects/p/notificationChannels/1"]
    page_notification_channels = ["projects/p/notificationChannels/2"]
  }

  assert {
    condition     = contains(google_monitoring_alert_policy.promql["MyQueueStalled"].notification_channels, "projects/p/notificationChannels/2")
    error_message = "a page goes to the pager"
  }

  assert {
    condition     = !contains(google_monitoring_alert_policy.promql["MyQueueBacklog"].notification_channels, "projects/p/notificationChannels/2")
    error_message = "a ticket does not"
  }

  assert {
    condition     = contains(google_monitoring_alert_policy.promql["MyQueueBacklog"].notification_channels, "projects/p/notificationChannels/1")
    error_message = "every alert goes to the channels"
  }
}

run "an_alert_that_notifies_nobody_fails" {
  command = plan

  variables {
    notification_emails = []
  }

  expect_failures = [var.notification_emails]
}

run "a_disabled_alert_is_left_out" {
  command = plan

  variables {
    disabled_alerts = ["MyTargetDown", "MyMemoryNearLimit"]
  }

  assert {
    condition     = !contains(output.alerts.policies, "MyTargetDown") && !contains(output.alerts.policies, "MyMemoryNearLimit")
    error_message = "both are left out"
  }
}

run "emf_fails_since_it_replaces_the_snapshot_line" {
  command = plan

  variables {
    metrics_exporter = "emf"
  }

  expect_failures = [var.metrics_exporter]
}

run "the_outputs_fit_the_app" {
  command = plan

  module {
    source = "../app-gcp-cloud-run"
  }

  variables {
    service_ingress = "all"
    region          = "asia-northeast1"
    image           = "ghcr.io/mycroblog/my:0.1.0"
    secret_files    = { MY_KEK_FILE = "kek" }
    backends = [
      # What this module returns: run blocks of a plan cannot pass outputs to another module.
      { platform = "gcp", env = { MY_METRICS_EXPORTER = "none" }, secret_env = {}, permissions = [] },
      { platform = "neutral", env = { MY_DATABASE = "postgres", MY_QUEUE = "cloudtasks" }, secret_env = { MY_DATABASE_URL = "db-url" } },
    ]
  }

  assert {
    condition     = output.env["MY_METRICS_EXPORTER"] == "none"
    error_message = "the app takes the setting"
  }
}
