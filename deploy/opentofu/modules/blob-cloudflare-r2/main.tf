# SPDX-License-Identifier: AGPL-3.0-only

# An R2 bucket and the settings to use it. The S3 credentials are secret references, and not a cloudflare_account_token
# here: the provider can create one, but its value is a computed attribute with no write-only form, so it would land in the state.

terraform {
  required_version = ">= 1.11"

  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = ">= 5.0, < 6.0"
    }
  }
}

variable "account_id" {
  description = "Cloudflare account of the bucket."
  type        = string
  nullable    = false
}

variable "bucket_name" {
  description = "Name of the bucket, unique in the account."
  type        = string
  nullable    = false

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$", var.bucket_name))
    error_message = "bucket_name must be 3 to 63 lower-case letters, digits, and inner hyphens."
  }
}

variable "location" {
  description = "Location hint of the bucket: apac, eeur, enam, weur, wnam, or oc. Best effort, and honoured only when the name is first created. Null lets Cloudflare choose."
  type        = string
  default     = null

  validation {
    condition     = var.location == null || contains(["apac", "eeur", "enam", "weur", "wnam", "oc"], coalesce(var.location, "none"))
    error_message = "location must be apac, eeur, enam, weur, wnam, or oc."
  }
}

variable "jurisdiction" {
  description = "Where the objects are guaranteed to stay: default, eu, fedramp, or us. It decides the endpoint."
  type        = string
  default     = "default"
  nullable    = false

  validation {
    condition     = contains(["default", "eu", "fedramp", "us"], var.jurisdiction)
    error_message = "jurisdiction must be default, eu, fedramp, or us."
  }
}

variable "access_key_id_secret_id" {
  description = "Secret that holds the Access Key ID of the R2 token, as the platform's store refers to it: on Google Cloud its ID, on AWS its ARN."
  type        = string
  nullable    = false
}

variable "secret_access_key_secret_id" {
  description = "Secret that holds the Secret Access Key of the R2 token, as the platform's store refers to it."
  type        = string
  nullable    = false
}

# No custom domain and no managed domain: both would serve the bucket to the public. The server stays the origin of a CDN.
# No lifecycle rule either: R2 expires an unfinished multipart upload after seven days by default.
resource "cloudflare_r2_bucket" "media" {
  account_id   = var.account_id
  name         = var.bucket_name
  location     = var.location
  jurisdiction = var.jurisdiction
}

output "platform" {
  description = "The platform of this module: it works with any app."
  value       = "neutral"
}

output "env" {
  description = "The settings of the blob store."
  value = {
    MY_BLOB             = "s3"
    MY_BLOB_S3_BUCKET   = cloudflare_r2_bucket.media.name
    MY_BLOB_S3_ENDPOINT = var.jurisdiction == "default" ? "https://${var.account_id}.r2.cloudflarestorage.com" : "https://${var.account_id}.${var.jurisdiction}.r2.cloudflarestorage.com"
    MY_BLOB_S3_REGION   = "auto"
  }
}

output "secret_env" {
  description = "The S3 credentials of the R2 token, by reference."
  value = {
    MY_BLOB_S3_ACCESS_KEY_ID     = var.access_key_id_secret_id
    MY_BLOB_S3_SECRET_ACCESS_KEY = var.secret_access_key_secret_id
  }
}

output "permissions" {
  description = "Nothing beyond reading the secrets, which the app module grants for every secret it is given."
  value       = []
}
