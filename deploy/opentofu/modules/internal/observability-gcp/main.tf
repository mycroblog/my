# SPDX-License-Identifier: AGPL-3.0-only

# Where the logs, the metrics, the traces, and the error reports of a server on Cloud Run go: Google Cloud's own destinations,
# which read what Cloud Run already sends them, so the server exports nothing and holds no Google credentials for them.
#
# - Cloud Logging: the server's lines in its shape (logs_exporter = gcp), which link each line to its trace and show the
#   access lines as requests.
# - Cloud Monitoring: Cloud Run's metrics, and the log-based metrics of monitoring-gcp, made from the server's log lines.
# - Cloud Trace: the trace Cloud Run records of each request, whose ID the log lines carry.
# - Error Reporting: the error report line, which it reads from Cloud Logging.
#
# A signal set to settings is left to the settings input: the module sets nothing for it, so that Sentry, for one, goes
# through errors_exporter there. The server's exporters to Google that would need a collector beside it (otlp) are not here:
# monitoring-gcp/README.md says why.

terraform {
  required_version = ">= 1.11"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 7.0, < 8.0"
    }
  }
}

variable "project_id" {
  description = "The project of the service, where the APIs of the destinations are enabled."
  type        = string
  nullable    = false
}

variable "logs" {
  description = "cloud-logging, or settings to leave logs_exporter to the settings input."
  type        = string
  default     = "cloud-logging"
  nullable    = false

  validation {
    condition     = contains(["cloud-logging", "settings"], var.logs)
    error_message = "logs must be cloud-logging or settings."
  }
}

variable "metrics" {
  description = "cloud-monitoring, or settings to leave metrics_exporter to the settings input."
  type        = string
  default     = "cloud-monitoring"
  nullable    = false

  validation {
    condition     = contains(["cloud-monitoring", "settings"], var.metrics)
    error_message = "metrics must be cloud-monitoring or settings."
  }
}

variable "traces" {
  description = "cloud-trace, or settings to leave traces_exporter to the settings input."
  type        = string
  default     = "cloud-trace"
  nullable    = false

  validation {
    condition     = contains(["cloud-trace", "settings"], var.traces)
    error_message = "traces must be cloud-trace or settings."
  }
}

variable "errors" {
  description = "error-reporting, or settings to leave errors_exporter to the settings input, as for Sentry."
  type        = string
  default     = "error-reporting"
  nullable    = false

  validation {
    condition     = contains(["error-reporting", "settings"], var.errors)
    error_message = "errors must be error-reporting or settings."
  }
}

locals {
  # Each destination, by its signal: the API it needs, and the server's exporter for it: none since Cloud Run delivers, and
  # for the logs, which Cloud Run delivers too, the shape of Cloud Logging.
  destinations = {
    logs    = { on = var.logs == "cloud-logging", api = "logging.googleapis.com", env = "MY_LOGS_EXPORTER", value = "gcp" }
    metrics = { on = var.metrics == "cloud-monitoring", api = "monitoring.googleapis.com", env = "MY_METRICS_EXPORTER", value = "none" }
    traces  = { on = var.traces == "cloud-trace", api = "cloudtrace.googleapis.com", env = "MY_TRACES_EXPORTER", value = "none" }
    errors  = { on = var.errors == "error-reporting", api = "clouderrorreporting.googleapis.com", env = "MY_ERRORS_EXPORTER", value = "none" }
  }
}

# Kept when the module goes: the project may use them for other things, and the platform keeps writing to them.
resource "google_project_service" "destination" {
  for_each = { for signal, d in local.destinations : signal => d.api if d.on }

  project            = var.project_id
  service            = each.value
  disable_on_destroy = false
}

output "platform" {
  description = "The platform of this module: it works only with an app of Google Cloud."
  value       = "gcp"
}

output "env" {
  description = "The exporter of each signal that goes to Google Cloud: gcp for the logs, and none for the others, since Cloud Run sends the logs, the metrics, and the request traces itself."
  value       = { for signal, d in local.destinations : d.env => d.value if d.on }
  depends_on  = [google_project_service.destination]
}

output "secret_env" {
  description = "No secrets."
  value       = {}
}

output "permissions" {
  description = "Nothing: Cloud Run writes the logs, the metrics, and the request traces with its own identity."
  value       = []
}
