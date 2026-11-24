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

variable "existing_bucket" {
  description = "Uses the bucket bucket_name, which already exists, perhaps beside other instances, in place of creating one. The module then manages none of its settings: its access, versioning, and lifecycle stay yours, and location is not used."
  type        = bool
  default     = false
  nullable    = false
}

variable "prefix" {
  description = "Prefix of the name of every object of the instance, so that several instances share the bucket: empty, or segments of letters, digits, ., _, and - that each end with /, such as tenant-a/. With one, the service account may touch only the objects under it, through an IAM condition, which needs uniform bucket-level access."
  type        = string
  default     = ""
  nullable    = false

  validation {
    condition     = var.prefix == "" || (length(var.prefix) <= 256 && can(regex("^([A-Za-z0-9_-][A-Za-z0-9._-]*/)+$", var.prefix)))
    error_message = "prefix must be empty or segments of letters, digits, ., _, and - that end with /, such as tenant-a/, none starting with a dot, at most 256 bytes."
  }
}

variable "versioning" {
  description = "Keeps a deleted or overwritten object as a noncurrent version, so the bucket can go back to the time of a database backup. Off by default: every version costs storage until the rule of noncurrent_version_days removes it."
  type        = bool
  default     = false
  nullable    = false

  validation {
    condition     = !(var.versioning && var.existing_bucket)
    error_message = "versioning needs a bucket this module creates: the versioning of an existing bucket is yours."
  }
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

  lifecycle {
    enabled = !var.existing_bucket
  }
}

# A bucket of yours, read so that a missing one fails the plan.
data "google_storage_bucket" "existing" {
  name = var.bucket_name

  lifecycle {
    enabled = var.existing_bucket

    postcondition {
      condition     = var.prefix == "" || self.uniform_bucket_level_access
      error_message = "The bucket ${var.bucket_name} needs uniform bucket-level access for a prefix: the grant under the prefix is an IAM condition, which a bucket with object ACLs refuses."
    }
  }
}

output "platform" {
  description = "The platform of this module: it works only with an app of Google Cloud."
  value       = "gcp"
}

output "env" {
  description = "The settings of the blob store."
  value = merge(
    {
      MY_BLOB            = "gcs"
      MY_BLOB_GCS_BUCKET = var.bucket_name
    },
    var.prefix == "" ? {} : { MY_BLOB_GCS_PREFIX = var.prefix },
  )
  depends_on = [google_storage_bucket.media, data.google_storage_bucket.existing]
}

output "secret_env" {
  description = "No secrets: the server authenticates with its service account."
  value       = {}
}

output "permissions" {
  description = "Reading and writing the objects of the bucket, only those under the prefix when there is one."
  value = [
    {
      permission = "roles/storage.objectUser"
      scope      = "bucket"
      resource   = var.bucket_name
      condition = var.prefix == "" ? null : {
        title      = "objects-under-prefix"
        expression = "resource.name.startsWith(\"projects/_/buckets/${var.bucket_name}/objects/${var.prefix}\")"
      }
    },
  ]
  depends_on = [google_storage_bucket.media, data.google_storage_bucket.existing]
}
