# SPDX-License-Identifier: AGPL-3.0-only

# Any S3-compatible store that lives outside this configuration. It creates nothing: the operator makes the bucket and
# the key, and puts the key in the platform's secret store, so no value reaches the state.

terraform {
  required_version = ">= 1.11"
}

variable "endpoint" {
  description = "Origin of the store's S3 API, such as https://storage.example.org. No path: the server adds the bucket itself. Use https unless the server is on a private network you trust, since the requests carry the media."
  type        = string
  nullable    = false

  validation {
    condition     = can(regex("^https?://[^/?#]+$", var.endpoint))
    error_message = "endpoint must be an origin: http or https, a host, and optionally a port, with no path."
  }
}

variable "bucket" {
  description = "Name of the bucket, which already exists. The server needs it private."
  type        = string
  nullable    = false

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9._-]{1,220}[a-z0-9]$", var.bucket))
    error_message = "bucket must be 3 to 222 lower-case letters, digits, dots, hyphens, and underscores."
  }
}

variable "region" {
  description = "Region of the bucket. Null leaves it unset, and the server then signs for us-east-1, which most S3-compatible servers expect."
  type        = string
  default     = null

  validation {
    condition     = var.region == null || can(regex("^[a-z0-9][a-z0-9-]{0,63}$", var.region))
    error_message = "region must be lower-case letters, digits, and hyphens."
  }
}

variable "path_style" {
  description = "Addresses the bucket in the path (endpoint/bucket/key), for a server with no host name per bucket, such as MinIO. False puts it in the host name."
  type        = bool
  default     = false
  nullable    = false
}

variable "access_key_id_secret_id" {
  description = "Secret that holds the access key ID, as the platform's store refers to it: on Google Cloud its ID, on AWS its ARN."
  type        = string
  nullable    = false
}

variable "secret_access_key_secret_id" {
  description = "Secret that holds the secret access key, as the platform's store refers to it."
  type        = string
  nullable    = false
}

output "platform" {
  description = "The platform of this module: it works with any app."
  value       = "neutral"
}

output "env" {
  description = "The settings of the blob store."
  value = merge(
    {
      MY_BLOB             = "s3"
      MY_BLOB_S3_BUCKET   = var.bucket
      MY_BLOB_S3_ENDPOINT = var.endpoint
    },
    var.path_style ? { MY_BLOB_S3_PATH_STYLE = "true" } : {},
    var.region == null ? {} : { MY_BLOB_S3_REGION = var.region },
  )
}

output "secret_env" {
  description = "The key of the store, by reference."
  value = {
    MY_BLOB_S3_ACCESS_KEY_ID     = var.access_key_id_secret_id
    MY_BLOB_S3_SECRET_ACCESS_KEY = var.secret_access_key_secret_id
  }
}

output "permissions" {
  description = "Nothing beyond reading the secrets, which the app module grants for every secret it is given."
  value       = []
}
