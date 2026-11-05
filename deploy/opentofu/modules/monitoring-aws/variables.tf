# SPDX-License-Identifier: AGPL-3.0-only

variable "service_name" {
  description = "The app module's name. The alarms read its web function, <service_name>-web."
  type        = string
  default     = "my"
  nullable    = false
}

variable "name" {
  description = "Start of the names of the alarms' topic, the canary, and what the canary needs."
  type        = string
  default     = "my"
  nullable    = false

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9_-]*$", var.name))
    error_message = "name must hold lowercase letters, digits, hyphens, and underscores: a canary's name takes no others."
  }
}

variable "host" {
  description = "Host that the canary calls: the public domain, or the app module's origin_host when no ingress is in front."
  type        = string
  nullable    = false
}

variable "queue_name" {
  description = "The name of the job queue of queue-aws-sqs, whose age of the oldest message stands in for the queue's."
  type        = string
  default     = "my-jobs"
  nullable    = false
}

variable "metrics_emf_namespace" {
  description = "CloudWatch namespace of the server's metrics, returned in env."
  type        = string
  default     = "my"
  nullable    = false
}

variable "notification_emails" {
  description = "Addresses that get every alarm. Each becomes an email subscription of the topic, which its owner must confirm. At least one of this and notification_topic_arns is required."
  type        = list(string)
  default     = []
  nullable    = false

  validation {
    condition     = length(var.notification_emails) + length(var.notification_topic_arns) > 0
    error_message = "Name at least one of notification_emails and notification_topic_arns: an alarm that notifies nobody is not an alert."
  }
}

variable "notification_topic_arns" {
  description = "SNS topics made elsewhere that get every alarm."
  type        = list(string)
  default     = []
  nullable    = false
}

variable "page_notification_topic_arns" {
  description = "SNS topics that also get the alarms of severity page, such as a pager."
  type        = list(string)
  default     = []
  nullable    = false
}

variable "uptime_period_minutes" {
  description = "How often the canary runs, in minutes, from 1 to 60. Every run is billed, wakes a function, and through /readyz a scale-to-zero database. The README says what that costs."
  type        = number
  default     = 15
  nullable    = false

  validation {
    condition     = var.uptime_period_minutes >= 1 && var.uptime_period_minutes <= 60 && floor(var.uptime_period_minutes) == var.uptime_period_minutes
    error_message = "uptime_period_minutes must be a whole number from 1 to 60: CloudWatch Synthetics runs a rate between once a minute and once an hour."
  }
}

variable "uptime_path" {
  description = "Path that the canary calls. /readyz touches the database on every request; /healthz never does, and so stays green while the database is down."
  type        = string
  default     = "/readyz"
  nullable    = false

  validation {
    condition     = contains(["/readyz", "/healthz", "/api/v1/instance/health"], var.uptime_path)
    error_message = "uptime_path must be /readyz, /healthz, or /api/v1/instance/health."
  }
}

variable "canary_runtime_version" {
  description = "The CloudWatch Synthetics runtime. AWS retires old ones, so this needs raising over time."
  type        = string
  default     = "syn-nodejs-puppeteer-18.0"
  nullable    = false
}

variable "database_size_limit_bytes" {
  description = "MyDatabaseStorageLarge fires over this size: your plan's limit less a margin. Null leaves the alarm out."
  type        = number
  default     = null
}

variable "media_size_limit_bytes" {
  description = "MyMediaStorageLarge fires when the media go over this size. Null leaves the alarm out."
  type        = number
  default     = null
}

variable "remote_media_cache_limit_bytes" {
  description = "MyRemoteMediaCacheOverLimit fires over this size: 1.2 times remote_media_cache_max_bytes. Null leaves the alarm out."
  type        = number
  default     = null
}

variable "disabled_alerts" {
  description = "Alerts of this module to leave out, by name. Each alarm metric is billed, so this is also the way to pay less."
  type        = set(string)
  default     = []
  nullable    = false
}
