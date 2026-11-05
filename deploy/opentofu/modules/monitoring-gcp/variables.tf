# SPDX-License-Identifier: AGPL-3.0-only

variable "project_id" {
  description = "Project of the service, the log-based metrics, the uptime check, and the alert policies."
  type        = string
  nullable    = false
}

variable "service_name" {
  description = "The app module's name. The log-based metrics and the request alerts read this service alone."
  type        = string
  default     = "my"
  nullable    = false
}

variable "name" {
  description = "Start of the names of the log-based metrics and the uptime check. A metric name goes into PromQL, so it has letters, digits, and underscores only."
  type        = string
  default     = "my"
  nullable    = false

  validation {
    condition     = can(regex("^[a-z][a-z0-9_]*$", var.name))
    error_message = "name must start with a lowercase letter and hold lowercase letters, digits, and underscores."
  }
}

variable "host" {
  description = "Host that the uptime check calls: the public domain, or the app module's origin_host when no ingress is in front. The check comes from the internet, so a service behind internal-and-load-balancer must be reached through its ingress."
  type        = string
  nullable    = false
}

variable "metrics_exporter" {
  description = "The server's metrics_exporter, returned in env. These alerts read the log lines of none. otlp writes the same lines and sends the metrics to a collector as well, which needs a sidecar the app module does not have. emf replaces the snapshot line with its own, so these alerts would stop seeing it."
  type        = string
  default     = "none"
  nullable    = false

  validation {
    condition     = contains(["none", "otlp"], var.metrics_exporter)
    error_message = "metrics_exporter must be none or otlp: with emf, and with prometheus on a platform that nothing scrapes, the alerts of this module see less or nothing."
  }
}

variable "notification_emails" {
  description = "Addresses that get every alert. Each becomes an email notification channel. At least one of this and notification_channels is required."
  type        = list(string)
  default     = []
  nullable    = false

  validation {
    condition     = length(var.notification_emails) + length(var.notification_channels) > 0
    error_message = "Name at least one of notification_emails and notification_channels: an alert that notifies nobody is not an alert."
  }
}

variable "notification_channels" {
  description = "Notification channels made elsewhere, by full ID (projects/<project>/notificationChannels/<id>), that get every alert."
  type        = list(string)
  default     = []
  nullable    = false
}

variable "page_notification_channels" {
  description = "Notification channels that also get the alerts of severity page, such as a pager, by full ID."
  type        = list(string)
  default     = []
  nullable    = false
}

variable "uptime_period" {
  description = "How often the uptime check runs: 60s, 300s, 600s, or 900s. Every run wakes an instance, and through /readyz a scale-to-zero database. The README says what that costs."
  type        = string
  default     = "900s"
  nullable    = false

  validation {
    condition     = contains(["60s", "300s", "600s", "900s"], var.uptime_period)
    error_message = "uptime_period must be 60s, 300s, 600s, or 900s: Cloud Monitoring has no others."
  }
}

variable "uptime_path" {
  description = "Path of the uptime check. /readyz touches the database on every request; /healthz never does, and so stays green while the database is down."
  type        = string
  default     = "/readyz"
  nullable    = false

  validation {
    condition     = contains(["/readyz", "/healthz", "/api/v1/instance/health"], var.uptime_path)
    error_message = "uptime_path must be /readyz, /healthz, or /api/v1/instance/health."
  }
}

variable "uptime_timeout" {
  description = "How long one check waits, from 1s to 60s. It has to outlast a cold start of the instance plus a database waking up, or an idle service fails its check."
  type        = string
  default     = "30s"
  nullable    = false
}

variable "uptime_regions" {
  description = "Regions the check runs from, which must hold at least 3 checkers. USA holds 3 (USA_OREGON, USA_IOWA, USA_VIRGINIA); EUROPE, SOUTH_AMERICA, and ASIA_PACIFIC hold 1 each. Every checker is one more request per period."
  type        = list(string)
  default     = ["USA_OREGON", "USA_IOWA", "USA_VIRGINIA"]
  nullable    = false
}

variable "snapshot_stale_after_seconds" {
  description = "MyMetricsSnapshotStale fires when no metrics snapshot line arrives for this long. The snapshot job runs every 15 minutes on the tick, so this must exceed 15 minutes plus the longest gap of tick_schedule."
  type        = number
  default     = 3600
  nullable    = false

  validation {
    condition     = var.snapshot_stale_after_seconds >= 1800 && var.snapshot_stale_after_seconds <= 84600
    error_message = "snapshot_stale_after_seconds must be between 1800 and 84600: Cloud Monitoring allows an absence of at most 23.5 hours."
  }
}

variable "database_size_limit_bytes" {
  description = "MyDatabaseStorageLarge fires over this size: your plan's limit less a margin. Null leaves the alert out."
  type        = number
  default     = null
}

variable "media_size_limit_bytes" {
  description = "MyMediaStorageLarge fires when the local media go over this size. Null leaves the alert out."
  type        = number
  default     = null
}

variable "remote_media_cache_limit_bytes" {
  description = "MyRemoteMediaCacheOverLimit fires over this size: 1.2 times remote_media_cache_max_bytes. Null leaves the alert out."
  type        = number
  default     = null
}

variable "billing_sources" {
  description = "The server's billing_sources. MyBillingImportFailing watches each one; empty leaves the alert out."
  type        = list(string)
  default     = []
  nullable    = false
}

variable "disabled_alerts" {
  description = "Alerts of this module to leave out, by name, such as the ones that do not fit your traffic."
  type        = set(string)
  default     = []
  nullable    = false
}
