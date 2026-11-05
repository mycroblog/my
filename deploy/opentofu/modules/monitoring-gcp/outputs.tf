# SPDX-License-Identifier: AGPL-3.0-only

output "env" {
  description = "Settings of the server, by variable name."
  value = {
    MY_METRICS_EXPORTER = var.metrics_exporter
  }
}

output "secret_env" {
  description = "Secret settings of the server: none."
  value       = {}
}

output "permissions" {
  description = "What the app's identity is permitted: nothing more, since Cloud Run writes the logs and the platform metrics itself."
  value       = []
}

output "uptime_check_id" {
  description = "The uptime check's ID."
  value       = google_monitoring_uptime_check_config.ready.uptime_check_id
}

output "alerts" {
  description = "The alerts of docs/operations/alerts.md: those this module has, by name, and those it cannot express."
  value = {
    policies = sort(concat(
      keys(local.enabled_promql_alerts),
      [for name in ["MyTargetDown", "MyMetricsSnapshotStale"] : name if !contains(var.disabled_alerts, name)],
    ))
    not_expressible = local.not_expressible
  }
}

output "notification_channels" {
  description = "The IDs of the channels that get every alert."
  value       = local.channels
}
