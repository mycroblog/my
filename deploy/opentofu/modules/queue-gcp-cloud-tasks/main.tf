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

variable "project_id" {
  description = "Project of the queue, the Scheduler job, and the invoker service account. It must be the service's: Cloud Run counts Cloud Tasks and Cloud Scheduler as internal only from the same project."
  type        = string
  nullable    = false
}

variable "region" {
  description = "Location of the queue and the Scheduler job, and the region of the service."
  type        = string
  nullable    = false
}

variable "service_name" {
  description = "Name of the Cloud Run service, the name input of the app module. Tasks and ticks call its run.app URL, built from this name, so the service and the queue do not depend on each other."
  type        = string
  default     = "my"
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

variable "workers" {
  description = "The same workers as the app takes. Each gets a queue of its own, and its job types a route to that queue and to the service the app builds, both named by the key. Empty sends everything to the one queue."
  type        = any
  default     = {}
  nullable    = false
}

module "workers" {
  source = "../workers"

  workers = var.workers
  fields  = ["min_instances", "max_instances", "cpu", "memory_mib", "concurrency", "max_concurrent_dispatches", "max_dispatches_per_second"]
}

data "google_project" "this" {
  project_id = var.project_id
}

locals {
  # The service's default URL, directly and never through an ingress: Cloudflare's 125 seconds and challenges stay out of the
  # way, and Cloud Run counts these two as internal only on this URL, so the service can refuse the internet. The format is
  # Cloud Run's deterministic one, which is known before the service exists.
  url_label = "${var.service_name}-${data.google_project.this.number}"
  url       = "https://${local.url_label}.${var.region}.run.app"
  # Built from the inputs, not read from the resources, so that they are known at plan time: the app module keys its
  # IAM bindings on them. The outputs depend on the resources for the order.
  queue         = "projects/${var.project_id}/locations/${var.region}/queues/${var.name}"
  invoker_email = "${var.name}-invoker@${var.project_id}.iam.gserviceaccount.com"

  # Each from the key, as the service's own URL is from its name: the app builds <service_name>-<key>.
  worker_queues = {
    for key, w in module.workers.workers : key => {
      name  = "${var.name}-${key}"
      queue = "projects/${var.project_id}/locations/${var.region}/queues/${var.name}-${key}"
      label = "${var.service_name}-${key}-${data.google_project.this.number}"
      url   = "https://${var.service_name}-${key}-${data.google_project.this.number}.${var.region}.run.app"
      rates = w
    }
  }
  routes = sort(flatten([
    for key, q in local.worker_queues : [for pattern in q.rates.job_types : "${pattern}=${q.queue}@${q.url}"]
  ]))
  # Every process signs and checks one audience. A process checks the audience against its own queue_cloudtasks_url, and that
  # setting is also where it sends the default queue's tasks, so a worker cannot name its own URL there without taking those
  # tasks. So one fixed audience, which a worker service accepts as a custom audience. Empty leaves what Cloud Tasks does by
  # itself, the URL it calls.
  audience = length(var.workers) == 0 ? null : local.url
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

  lifecycle {
    precondition {
      condition     = length(local.url_label) <= 63
      error_message = "Cloud Run has no deterministic URL when the service's name and the project's number come to more than 63 characters together, and tasks and ticks call that URL: shorten the service's name."
    }
    precondition {
      condition     = length(module.workers.problems) == 0
      error_message = "workers: ${join(" ", module.workers.problems)}"
    }
  }
}

resource "google_cloud_tasks_queue" "worker" {
  for_each = local.worker_queues

  project  = var.project_id
  location = var.region
  name     = each.value.name

  rate_limits {
    max_concurrent_dispatches = coalesce(each.value.rates.max_concurrent_dispatches, var.max_concurrent_dispatches)
    max_dispatches_per_second = each.value.rates.max_dispatches_per_second
  }

  lifecycle {
    precondition {
      condition     = length(each.value.label) <= 63
      error_message = "Cloud Run has no deterministic URL when the name of a worker's service and the project's number come to more than 63 characters together: shorten the service's name or the key."
    }
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

    # With no workers, no audience: it defaults to the URI, which is what the server requires while queue_cloudtasks_audience
    # is unset. With workers, the one audience of every process.
    oidc_token {
      service_account_email = google_service_account.invoker.email
      audience              = local.audience
    }
  }
}

output "platform" {
  description = "The platform of this module: it works only with an app of Google Cloud."
  value       = "gcp"
}

output "env" {
  description = "The settings of the queue."
  value = merge(
    {
      MY_QUEUE                            = "cloudtasks"
      MY_QUEUE_CLOUDTASKS_QUEUE           = local.queue
      MY_QUEUE_CLOUDTASKS_URL             = local.url
      MY_QUEUE_CLOUDTASKS_SERVICE_ACCOUNT = local.invoker_email
    },
    length(var.workers) == 0 ? {} : {
      MY_QUEUE_CLOUDTASKS_ROUTES   = join(",", local.routes)
      MY_QUEUE_CLOUDTASKS_AUDIENCE = local.audience
    },
  )
  depends_on = [google_cloud_tasks_queue.jobs, google_cloud_tasks_queue.worker, google_service_account.invoker]
}

output "secret_env" {
  description = "No secrets: the server authenticates with its service account."
  value       = {}
}

output "permissions" {
  description = "Creating tasks in the queue, and acting as the invoker service account so that the tasks carry its token."
  value = concat(
    [
      { permission = "roles/cloudtasks.enqueuer", scope = "queue", resource = local.queue },
      { permission = "roles/iam.serviceAccountUser", scope = "service_account", resource = "projects/${var.project_id}/serviceAccounts/${local.invoker_email}" },
    ],
    [for q in values(local.worker_queues) : { permission = "roles/cloudtasks.enqueuer", scope = "queue", resource = q.queue }],
  )
  depends_on = [google_cloud_tasks_queue.jobs, google_cloud_tasks_queue.worker, google_service_account.invoker]
}

output "invokers" {
  description = "Service accounts that call the server, which the app module gives roles/run.invoker on the service."
  value       = [local.invoker_email]
  depends_on  = [google_service_account.invoker]
}

output "workers" {
  description = "The job types of each worker, which the app compares with its own."
  value       = module.workers.job_types
}
