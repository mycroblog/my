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
  description = "Project of the export's dataset, or of the budget's topic and subscription."
  type        = string
  nullable    = false
}

variable "method" {
  description = "How the server reads the costs: bigquery, from the Cloud Billing export, or budget, from the notifications of a budget."
  type        = string
  default     = "bigquery"
  nullable    = false

  validation {
    condition     = contains(["bigquery", "budget"], var.method)
    error_message = "method must be bigquery or budget."
  }
}

variable "billing_account_id" {
  description = "The billing account of the export, such as 012345-6789AB-CDEF01, which names its table. Required with bigquery."
  type        = string
  default     = null

  validation {
    condition     = var.billing_account_id == null || can(regex("^[0-9A-Fa-f]{6}-[0-9A-Fa-f]{6}-[0-9A-Fa-f]{6}$", var.billing_account_id))
    error_message = "billing_account_id must look like 012345-6789AB-CDEF01."
  }

  validation {
    condition     = var.method != "bigquery" || var.billing_account_id != null
    error_message = "method = bigquery needs billing_account_id, which names the export's table."
  }
}

variable "dataset_id" {
  description = "The dataset that receives the export."
  type        = string
  default     = "billing_export"
  nullable    = false

  validation {
    condition     = can(regex("^[A-Za-z0-9_]+$", var.dataset_id))
    error_message = "dataset_id must hold letters, digits, and underscores only."
  }
}

variable "create_dataset" {
  description = "Whether this module creates the dataset. Off for an export that already has one."
  type        = bool
  default     = true
  nullable    = false
}

variable "dataset_location" {
  description = "Location of the dataset this module creates. A multi-region (US or EU) receives the current and the previous month when the export starts; a single region starts the day it is enabled. It cannot change later."
  type        = string
  default     = "US"
  nullable    = false
}

variable "query_project" {
  description = "The project that runs the queries and is billed for their bytes. Null uses project_id."
  type        = string
  default     = null
}

variable "name" {
  description = "Name of the budget's topic and subscription, used by this server alone."
  type        = string
  default     = "my-billing-budget"
  nullable    = false

  validation {
    condition     = can(regex("^[A-Za-z][A-Za-z0-9._~+%-]+$", var.name)) && length(var.name) >= 3 && length(var.name) <= 255 && !startswith(var.name, "goog")
    error_message = "name must be 3 to 255 letters, digits, and -._~+%, start with a letter, and not start with goog."
  }
}

variable "external_member" {
  description = "An IAM member for a server outside Google Cloud, such as the principalSet of a workload identity pool, which this module grants the roles itself: the app of another platform cannot. It must be known at plan time. Null returns them as permissions for a Google Cloud app."
  type        = string
  default     = null
}

locals {
  bigquery = var.method == "bigquery"
  budget   = var.method == "budget"
  external = var.external_member != null

  query_project = coalesce(var.query_project, var.project_id)

  # Built from the inputs so that they are known at plan time: the app module keys its IAM bindings on them. The outputs
  # depend on the resources for the order.
  dataset      = "projects/${var.project_id}/datasets/${var.dataset_id}"
  topic        = "projects/${var.project_id}/topics/${var.name}"
  subscription = "projects/${var.project_id}/subscriptions/${var.name}"
  # Cloud Billing replaces the hyphens of the account's ID.
  table = local.bigquery ? "${var.project_id}.${var.dataset_id}.gcp_billing_export_v1_${replace(var.billing_account_id, "-", "_")}" : null

  # READER is what the API makes of roles/bigquery.dataViewer in an access entry.
  bigquery_permissions = [
    { permission = "roles/bigquery.jobUser", scope = "project", resource = local.query_project },
    { permission = "READER", scope = "dataset", resource = local.dataset },
  ]
  budget_permissions = [
    { permission = "roles/pubsub.subscriber", scope = "subscription", resource = local.subscription },
  ]
}

resource "google_bigquery_dataset" "export" {
  project     = var.project_id
  dataset_id  = var.dataset_id
  location    = var.dataset_location
  description = "The Cloud Billing export that my imports."

  # No access block: it would remove the entry that Google adds for its export, and the grants here and in the app. No
  # expiration either: rows that expire do not come back. A dataset that holds tables refuses to be destroyed, since
  # delete_contents_on_destroy stays off.

  lifecycle {
    enabled = local.bigquery && var.create_dataset
  }
}

resource "google_pubsub_topic" "budget" {
  project = var.project_id
  name    = var.name

  lifecycle {
    enabled = local.budget
  }
}

# Pull, as the server reads it, so that a notification never wakes the server. Nothing here sets the topic's policy:
# connecting a budget in the Console lets Cloud Billing publish to it.
resource "google_pubsub_subscription" "budget" {
  project              = var.project_id
  name                 = var.name
  topic                = google_pubsub_topic.budget.id
  ack_deadline_seconds = 60

  # The latest notification of a month stays unacknowledged, and the previous month is read until the 15th: the most
  # Pub/Sub allows, 31 days.
  message_retention_duration = "2678400s"

  # Survives a period in which imports were stopped.
  expiration_policy {
    ttl = ""
  }

  lifecycle {
    enabled = local.budget
  }
}

# For a server outside Google Cloud. The app of Google Cloud makes the same grants from the output permissions.
resource "google_project_iam_member" "external_query" {
  project = local.query_project
  role    = "roles/bigquery.jobUser"
  member  = var.external_member

  lifecycle {
    enabled = local.external && local.bigquery
  }
}

resource "google_bigquery_dataset_access" "external_export" {
  project    = var.project_id
  dataset_id = var.dataset_id
  role       = "READER"
  iam_member = var.external_member

  depends_on = [google_bigquery_dataset.export]

  lifecycle {
    enabled = local.external && local.bigquery
  }
}

resource "google_pubsub_subscription_iam_member" "external_budget" {
  project      = var.project_id
  subscription = var.name
  role         = "roles/pubsub.subscriber"
  member       = var.external_member

  depends_on = [google_pubsub_subscription.budget]

  lifecycle {
    enabled = local.external && local.budget
  }
}

output "billing_sources" {
  description = "What this module adds to MY_BILLING_SOURCES."
  value       = ["gcp"]
}

output "env" {
  description = "The method, and what it reads."
  value = merge(
    { MY_BILLING_GCP_METHOD = var.method },
    local.bigquery ? { MY_BILLING_GCP_TABLE = local.table } : {},
    local.bigquery && var.query_project != null ? { MY_BILLING_GCP_QUERY_PROJECT = var.query_project } : {},
    local.budget ? { MY_BILLING_GCP_BUDGET_SUBSCRIPTION = local.subscription } : {},
  )
  depends_on = [google_pubsub_subscription.budget]
}

output "secret_env" {
  description = "No secrets: the server authenticates with Application Default Credentials."
  value       = {}
}

output "permissions" {
  description = "The roles for a Google Cloud app's service account. Empty with external_member, since this module has granted them."
  value       = local.external ? [] : (local.bigquery ? local.bigquery_permissions : local.budget_permissions)
  depends_on  = [google_bigquery_dataset.export, google_pubsub_subscription.budget]
}

output "budget_topic" {
  description = "The topic to connect the budget to in the Console, with method = budget, or null. Not part of the backend contract."
  value       = local.budget ? local.topic : null
  depends_on  = [google_pubsub_topic.budget]
}
