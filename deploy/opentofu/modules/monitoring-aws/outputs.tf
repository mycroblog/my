# SPDX-License-Identifier: AGPL-3.0-only

output "env" {
  description = "Settings of the server, by variable name."
  value = {
    MY_METRICS_EXPORTER       = "emf"
    MY_METRICS_EMF_NAMESPACE  = var.metrics_emf_namespace
    MY_METRICS_EMF_DIMENSIONS = join(",", local.emf_dimensions)
  }
}

output "secret_env" {
  description = "Secret settings of the server: none."
  value       = {}
}

output "permissions" {
  description = "What the app's identity is permitted: nothing more, since Lambda writes the function's logs and CloudWatch Logs takes the metrics from them."
  value       = []
}

output "canary_name" {
  description = "The canary's name."
  value       = aws_synthetics_canary.ready.name
}

output "topic_arn" {
  description = "The SNS topic of the alarms."
  value       = aws_sns_topic.alerts.arn
}

output "alerts" {
  description = "The alerts of docs/operations/alerts.md: those this module has, by name, and those it cannot express."
  value = {
    alarms          = sort(keys(local.enabled_alarms))
    not_expressible = local.not_expressible
  }
}

output "alarm_metrics" {
  description = "How many metrics the alarms read in all, each of them billed. The first 10 are free."
  value       = sum([for alarm in values(local.enabled_alarms) : length(alarm.metrics)])
}
