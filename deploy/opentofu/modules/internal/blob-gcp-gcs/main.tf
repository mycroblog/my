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
  description = "Project of the bucket."
  type        = string
  nullable    = false
}

variable "bucket_name" {
  description = "Name of the bucket, unique across Google Cloud."
  type        = string
  nullable    = false
}

variable "location" {
  description = "Location of the bucket. The Always Free storage covers us-east1, us-west1, and us-central1 only."
  type        = string
  nullable    = false
}

variable "versioning" {
  description = "Keeps a deleted or overwritten object as a noncurrent version, so the bucket can go back to the time of a database backup. Off by default: every version costs storage until the rule of noncurrent_version_days removes it."
  type        = bool
  default     = false
  nullable    = false
}

variable "noncurrent_version_days" {
  description = "Days a noncurrent version is kept before it is deleted, when versioning is on. Keep it at least as long as your oldest database backup."
  type        = number
  default     = 35
  nullable    = false

  validation {
    condition     = var.noncurrent_version_days >= 1 && floor(var.noncurrent_version_days) == var.noncurrent_version_days
    error_message = "noncurrent_version_days must be a whole number of days, at least 1."
  }
}

resource "google_storage_bucket" "media" {
  project                     = var.project_id
  name                        = var.bucket_name
  location                    = var.location
  uniform_bucket_level_access = true
  # The server reads the bucket and serves the media under /media/, so nothing reaches the bucket directly.
  public_access_prevention = "enforced"

  versioning {
    enabled = var.versioning
  }

  # A version counts as noncurrent from the moment it is replaced or deleted, which is what days_since_noncurrent_time measures.
  dynamic "lifecycle_rule" {
    for_each = var.versioning ? [1] : []

    content {
      action {
        type = "Delete"
      }

      condition {
        days_since_noncurrent_time = var.noncurrent_version_days
        with_state                 = "ARCHIVED"
      }
    }
  }
}

output "platform" {
  description = "The platform of this module: it works only with an app of Google Cloud."
  value       = "gcp"
}

output "env" {
  description = "The settings of the blob store."
  value = {
    MY_BLOB            = "gcs"
    MY_BLOB_GCS_BUCKET = var.bucket_name
  }
  depends_on = [google_storage_bucket.media]
}

output "secret_env" {
  description = "No secrets: the server authenticates with its service account."
  value       = {}
}

output "permissions" {
  description = "Reading and writing the objects of the bucket."
  value = [
    { permission = "roles/storage.objectUser", scope = "bucket", resource = var.bucket_name },
  ]
  depends_on = [google_storage_bucket.media]
}
