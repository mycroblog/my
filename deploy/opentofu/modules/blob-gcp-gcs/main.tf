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

resource "google_storage_bucket" "media" {
  project                     = var.project_id
  name                        = var.bucket_name
  location                    = var.location
  uniform_bucket_level_access = true
  # The server reads the bucket and serves the media under /media/, so nothing reaches the bucket directly.
  public_access_prevention = "enforced"
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
