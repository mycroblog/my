# SPDX-License-Identifier: AGPL-3.0-only

# Where the metrics, the traces, and the error reports of a server on ECS go. The metrics and the error reports ride on the log
# lines, which the tasks' log driver sends to CloudWatch Logs with the execution role; the traces go to X-Ray through a collector
# that the app runs beside the server in every task:
#
# - CloudWatch: the metrics in Embedded Metric Format, which CloudWatch Logs extracts from the lines.
# - CloudWatch Logs: the error report line, with its stack.
# - X-Ray: the spans, which the server sends over OTLP to the AWS Distro for OpenTelemetry collector on the task's loopback,
#   and the collector to X-Ray with the task role. X-Ray takes OTLP only signed, which the server does not do.
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
  description = "x-ray, none, or settings to leave traces_exporter to the settings input."
  type        = string
  default     = "x-ray"
  nullable    = false

  validation {
    condition     = contains(["x-ray", "none", "settings"], var.traces)
    error_message = "traces must be x-ray, none, or settings."
  }
}

variable "collector_image" {
  description = "The image of the AWS Distro for OpenTelemetry collector that every task runs beside the server with traces = x-ray, pinned by version and digest."
  type        = string
  default     = "public.ecr.aws/aws-observability/aws-otel-collector:v0.50.0@sha256:7968fb60db6a2390a47ba6a2df029745638486e285c9b2487da1b722d0855a3e"
  nullable    = false

  validation {
    condition     = can(regex("(:[^/:@]+|@sha256:[0-9a-f]{64})$", var.collector_image)) && !can(regex(":latest(@|$)", var.collector_image))
    error_message = "collector_image must name a version or a digest, never latest."
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

locals {
  # Where the collector listens: the task's loopback, which every container of a task on awsvpc shares and nothing outside
  # reaches. OTLP over HTTP, which is what the server sends.
  collector_endpoint = "127.0.0.1:4318"

  # Traces alone. The server samples (traces_sample_ratio), so the collector reads no sampling rules of X-Ray, and the
  # exporter sends no telemetry records of its own: the task role needs xray:PutTraceSegments and nothing else. The limiter
  # and GOMEMLIMIT keep the collector inside the memory it is given, and it reports no metrics of its own.
  collector_config = yamlencode({
    receivers = {
      otlp = { protocols = { http = { endpoint = local.collector_endpoint } } }
    }
    processors = {
      memory_limiter = { check_interval = "1s", limit_mib = 100, spike_limit_mib = 20 }
      batch          = { timeout = "5s" }
    }
    exporters = {
      awsxray = {}
    }
    service = {
      telemetry = { metrics = { level = "none" } }
      pipelines = {
        traces = { receivers = ["otlp"], processors = ["memory_limiter", "batch"], exporters = ["awsxray"] }
      }
    }
  })
}

output "platform" {
  description = "The platform of this module: it works only with an app of AWS."
  value       = "aws"
}

output "env" {
  description = "The exporter of each signal the module sets: emf and its namespace for CloudWatch, otlp to the collector for X-Ray, none for traces = none and for the error reports, whose line CloudWatch Logs keeps."
  value = merge(
    var.metrics == "cloudwatch" ? { MY_METRICS_EXPORTER = "emf", MY_METRICS_EMF_NAMESPACE = var.metrics_emf_namespace } : {},
    var.traces == "none" ? { MY_TRACES_EXPORTER = "none" } : {},
    var.traces == "x-ray" ? {
      MY_TRACES_EXPORTER                 = "otlp"
      OTEL_EXPORTER_OTLP_TRACES_ENDPOINT = "http://${local.collector_endpoint}/v1/traces"
    } : {},
    var.errors == "cloudwatch" ? { MY_ERRORS_EXPORTER = "none" } : {},
  )
}

output "secret_env" {
  description = "No secrets."
  value       = {}
}

output "permissions" {
  description = "With traces = x-ray, what the collector sends the spans with. Nothing else: the log driver writes the lines with the execution role, and CloudWatch Logs extracts the metrics from them."
  value       = var.traces == "x-ray" ? [{ permission = "xray:PutTraceSegments", resource = "*" }] : []
}

output "sidecars" {
  description = "With traces = x-ray, the collector, which the app runs beside the server in every task: 32 CPU units and 128 MiB taken from the task."
  value = var.traces == "x-ray" ? [{
    name       = "otel-collector"
    image      = var.collector_image
    cpu        = 32
    memory_mib = 128
    env = {
      AOT_CONFIG_CONTENT = local.collector_config
      GOMEMLIMIT         = "100MiB"
    }
  }] : []
}
