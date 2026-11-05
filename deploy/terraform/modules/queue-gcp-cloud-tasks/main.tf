# SPDX-License-Identifier: AGPL-3.0-only

terraform {
  required_version = ">= 1.9"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 7.0, < 8.0"
    }
  }
}

variable "project_id" {
  description = "Project of the queue, the Scheduler job, and the invoker service account."
  type        = string
  nullable    = false
}

variable "region" {
  description = "Location of the queue and the Scheduler job."
  type        = string
  nullable    = false
}

variable "host" {
  description = "Public domain of the server. Tasks and ticks call https://{host}, which is known before the service exists, so the service and the queue do not depend on each other."
  type        = string
  nullable    = false
}

variable "name" {
  description = "Name of the queue and start of the names of the other resources."
  type        = string
  default     = "my-jobs"
  nullable    = false
}

variable "tick_schedule" {
  description = "Cron schedule of the tick, in UTC. Each tick touches the database, so it decides whether a scale-to-zero database can stop."
  type        = string
  default     = "* * * * *"
  nullable    = false
}

variable "max_concurrent_dispatches" {
  description = "How many tasks run at once, which bounds the instances and the database connections that a burst of jobs takes."
  type        = number
  default     = 20
  nullable    = false
}

locals {
  url = "https://${var.host}"
  # Built from the inputs, not read from the resources, so that they are known at plan time: the app module keys its
  # IAM bindings on them. The outputs depend on the resources for the order.
  queue         = "projects/${var.project_id}/locations/${var.region}/queues/${var.name}"
  invoker_email = "${var.name}-invoker@${var.project_id}.iam.gserviceaccount.com"
}

# Created here and not with the service, so that the queue never needs the service and the service only needs its name.
resource "google_service_account" "invoker" {
  project      = var.project_id
  account_id   = "${var.name}-invoker"
  display_name = "Calls the server for Cloud Tasks and Cloud Scheduler"
}

resource "google_cloud_tasks_queue" "jobs" {
  project  = var.project_id
  location = var.region
  name     = var.name

  rate_limits {
    max_concurrent_dispatches = var.max_concurrent_dispatches
  }
}

resource "google_cloud_scheduler_job" "tick" {
  project   = var.project_id
  region    = var.region
  name      = "${var.name}-tick"
  schedule  = var.tick_schedule
  time_zone = "Etc/UTC"
  # A tick that fails is made up for by the next one.
  attempt_deadline = "180s"

  retry_config {
    retry_count = 0
  }

  http_target {
    http_method = "POST"
    uri         = "${local.url}/_internal/tick"

    # No audience: it defaults to the URI, which is what the server requires while queue_cloudtasks_audience is unset.
    oidc_token {
      service_account_email = google_service_account.invoker.email
    }
  }
}

output "env" {
  description = "The settings of the queue."
  value = {
    MY_QUEUE                            = "cloudtasks"
    MY_QUEUE_CLOUDTASKS_QUEUE           = local.queue
    MY_QUEUE_CLOUDTASKS_URL             = local.url
    MY_QUEUE_CLOUDTASKS_SERVICE_ACCOUNT = local.invoker_email
  }
  depends_on = [google_cloud_tasks_queue.jobs, google_service_account.invoker]
}

output "secret_env" {
  description = "No secrets: the server authenticates with its service account."
  value       = {}
}

output "roles" {
  description = "Creating tasks in the queue, and acting as the invoker service account so that the tasks carry its token."
  value = [
    { role = "roles/cloudtasks.enqueuer", scope = "queue", resource = local.queue },
    { role = "roles/iam.serviceAccountUser", scope = "service_account", resource = "projects/${var.project_id}/serviceAccounts/${local.invoker_email}" },
  ]
  depends_on = [google_cloud_tasks_queue.jobs, google_service_account.invoker]
}

output "invokers" {
  description = "Service accounts that call the server, which the app module gives roles/run.invoker on the service."
  value       = [local.invoker_email]
  depends_on  = [google_service_account.invoker]
}
