# SPDX-License-Identifier: AGPL-3.0-only

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.0, < 7.0"
    }
  }
}

data "aws_region" "current" {}

variable "bucket_name" {
  description = "Name of the bucket, unique across AWS."
  type        = string
  nullable    = false
}

variable "existing_bucket" {
  description = "Uses the bucket bucket_name, which already exists, perhaps beside other instances, in place of creating one. The module then manages none of its settings: its public access, versioning, and lifecycle stay yours."
  type        = bool
  default     = false
  nullable    = false
}

variable "prefix" {
  description = "Prefix of the name of every object of the instance, so that several instances share the bucket: empty, or segments of letters, digits, ., _, and - that each end with /, such as tenant-a/. With one, the role may touch only the objects under it."
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

resource "aws_s3_bucket" "media" {
  bucket = var.bucket_name

  lifecycle {
    enabled = !var.existing_bucket
  }
}

# A bucket of yours, read so that a missing one fails the plan and the server signs for the bucket's own region.
data "aws_s3_bucket" "existing" {
  bucket = var.bucket_name

  lifecycle {
    enabled = var.existing_bucket
  }
}

locals {
  bucket_arn = var.existing_bucket ? data.aws_s3_bucket.existing.arn : aws_s3_bucket.media.arn
}

# The server reads the bucket and serves the media under /media/, so nothing reaches the bucket directly.
resource "aws_s3_bucket_public_access_block" "media" {
  bucket                  = aws_s3_bucket.media.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true

  lifecycle {
    enabled = !var.existing_bucket
  }
}

resource "aws_s3_bucket_ownership_controls" "media" {
  bucket = aws_s3_bucket.media.id

  rule {
    object_ownership = "BucketOwnerEnforced"
  }

  lifecycle {
    enabled = !var.existing_bucket
  }
}

resource "aws_s3_bucket_versioning" "media" {
  count  = var.versioning ? 1 : 0
  bucket = aws_s3_bucket.media.id

  versioning_configuration {
    status = "Enabled"
  }
}

# The server sends a large object in parts: what a frozen or timed out function leaves behind would be billed for ever.
resource "aws_s3_bucket_lifecycle_configuration" "media" {
  bucket = aws_s3_bucket.media.id

  # Versioning goes first: a rule of noncurrent versions needs it in place.
  depends_on = [aws_s3_bucket_versioning.media]

  lifecycle {
    enabled = !var.existing_bucket
  }

  rule {
    id     = "abort-incomplete-uploads"
    status = "Enabled"

    filter {}

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }

  dynamic "rule" {
    for_each = var.versioning ? [1] : []

    content {
      id     = "noncurrent-versions"
      status = "Enabled"

      filter {}

      noncurrent_version_expiration {
        noncurrent_days = var.noncurrent_version_days
      }
    }
  }
}

output "platform" {
  description = "The platform of this module: it works only with an app of AWS."
  value       = "aws"
}

output "env" {
  description = "The settings of the blob store. No access key: the server takes its execution role from the AWS SDK's default chain."
  value = merge(
    {
      MY_BLOB           = "s3"
      MY_BLOB_S3_BUCKET = var.existing_bucket ? data.aws_s3_bucket.existing.bucket : aws_s3_bucket.media.bucket
      MY_BLOB_S3_REGION = var.existing_bucket ? data.aws_s3_bucket.existing.bucket_region : data.aws_region.current.region
    },
    var.prefix == "" ? {} : { MY_BLOB_S3_PREFIX = var.prefix },
  )
}

output "secret_env" {
  description = "No secrets: the server authenticates with its execution role."
  value       = {}
}

output "permissions" {
  description = "Reading and writing the objects of the bucket, only those under the prefix when there is one. Listing it, under the prefix too, makes a missing object a 404 rather than a 403."
  value = [
    { permission = "s3:GetObject", resource = "${local.bucket_arn}/${var.prefix}*" },
    { permission = "s3:PutObject", resource = "${local.bucket_arn}/${var.prefix}*" },
    { permission = "s3:DeleteObject", resource = "${local.bucket_arn}/${var.prefix}*" },
    { permission = "s3:AbortMultipartUpload", resource = "${local.bucket_arn}/${var.prefix}*" },
    {
      permission = "s3:ListBucket"
      resource   = local.bucket_arn
      conditions = var.prefix == "" ? [] : [{ test = "StringLike", variable = "s3:prefix", values = ["${var.prefix}*"] }]
    },
  ]
}
