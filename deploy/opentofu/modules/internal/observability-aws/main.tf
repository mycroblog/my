# SPDX-License-Identifier: AGPL-3.0-only

# Where the metrics, the traces, and the error reports of a server on ECS go. Everything rides on the log lines, which the
# tasks' log driver sends to CloudWatch Logs with the execution role, so the server needs no grant and no collector:
#
# - CloudWatch: the metrics in Embedded Metric Format, which CloudWatch Logs extracts from the lines.
# - CloudWatch Logs: the error report line, with its stack.
# - Traces: none. X-Ray takes spans only through a collector beside the server, which app-aws-ecs does not run.
#
# A signal set to settings is left to the settings input: the module sets nothing for it, so that Sentry, for one, goes
# through errors_exporter there. It creates nothing, so it declares no provider.

terraform {
  required_version = ">= 1.11"
}

variable "metrics" {
  description = "cloudwatch, or settings to leave metrics_exporter and metrics_emf_namespace to the settings input."
  type        = string
  default     = "cloudwatch"
  nullable    = false

  validation {
    condition     = contains(["cloudwatch", "settings"], var.metrics)
    error_message = "metrics must be cloudwatch or settings."
  }
}

variable "traces" {
  description = "none, or settings to leave traces_exporter to the settings input."
  type        = string
  default     = "none"
  nullable    = false

  validation {
    condition     = contains(["none", "settings"], var.traces)
    error_message = "traces must be none or settings: X-Ray needs a collector beside the server, which the tasks do not run."
  }
}

variable "errors" {
  description = "cloudwatch, or settings to leave errors_exporter to the settings input, as for Sentry."
  type        = string
  default     = "cloudwatch"
  nullable    = false

  validation {
    condition     = contains(["cloudwatch", "settings"], var.errors)
    error_message = "errors must be cloudwatch or settings."
  }
}

variable "metrics_emf_namespace" {
  description = "CloudWatch namespace of the metrics, which monitoring-aws reads too."
  type        = string
  default     = "my"
  nullable    = false

  validation {
    condition     = can(regex("^[A-Za-z0-9._/#:-]{1,255}$", var.metrics_emf_namespace)) && !startswith(var.metrics_emf_namespace, "AWS/")
    error_message = "metrics_emf_namespace must be a CloudWatch namespace: letters, digits, and . _ / # : -, not starting with AWS/."
  }
}

output "platform" {
  description = "The platform of this module: it works only with an app of AWS."
  value       = "aws"
}

output "env" {
  description = "The exporter of each signal the module sets: emf and its namespace for CloudWatch, none for the traces and for the error reports, whose line CloudWatch Logs keeps."
  value = merge(
    var.metrics == "cloudwatch" ? { MY_METRICS_EXPORTER = "emf", MY_METRICS_EMF_NAMESPACE = var.metrics_emf_namespace } : {},
    var.traces == "none" ? { MY_TRACES_EXPORTER = "none" } : {},
    var.errors == "cloudwatch" ? { MY_ERRORS_EXPORTER = "none" } : {},
  )
}

output "secret_env" {
  description = "No secrets."
  value       = {}
}

output "permissions" {
  description = "Nothing: the log driver writes the lines with the execution role, and CloudWatch Logs extracts the metrics from them."
  value       = []
}
