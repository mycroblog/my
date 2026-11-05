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

resource "aws_s3_bucket" "media" {
  bucket = var.bucket_name
}

# The server reads the bucket and serves the media under /media/, so nothing reaches the bucket directly.
resource "aws_s3_bucket_public_access_block" "media" {
  bucket                  = aws_s3_bucket.media.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_ownership_controls" "media" {
  bucket = aws_s3_bucket.media.id

  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

# The server sends a large object in parts: what a frozen or timed out function leaves behind would be billed for ever.
resource "aws_s3_bucket_lifecycle_configuration" "media" {
  bucket = aws_s3_bucket.media.id

  rule {
    id     = "abort-incomplete-uploads"
    status = "Enabled"

    filter {}

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }
}

output "platform" {
  description = "The platform of this module: it works only with an app of AWS."
  value       = "aws"
}

output "env" {
  description = "The settings of the blob store. No access key: the server takes its execution role from the AWS SDK's default chain."
  value = {
    MY_BLOB           = "s3"
    MY_BLOB_S3_BUCKET = aws_s3_bucket.media.bucket
    MY_BLOB_S3_REGION = data.aws_region.current.region
  }
}

output "secret_env" {
  description = "No secrets: the server authenticates with its execution role."
  value       = {}
}

output "permissions" {
  description = "Reading and writing the objects of the bucket. Listing it makes a missing object a 404 rather than a 403."
  value = [
    { permission = "s3:GetObject", resource = "${aws_s3_bucket.media.arn}/*" },
    { permission = "s3:PutObject", resource = "${aws_s3_bucket.media.arn}/*" },
    { permission = "s3:DeleteObject", resource = "${aws_s3_bucket.media.arn}/*" },
    { permission = "s3:AbortMultipartUpload", resource = "${aws_s3_bucket.media.arn}/*" },
    { permission = "s3:ListBucket", resource = aws_s3_bucket.media.arn },
  ]
}
