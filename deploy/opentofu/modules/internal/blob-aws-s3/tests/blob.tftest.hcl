# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches AWS.

mock_provider "aws" {
  mock_data "aws_region" {
    defaults = {
      region = "ap-northeast-1"
    }
  }
  mock_resource "aws_s3_bucket" {
    defaults = {
      arn = "arn:aws:s3:::my-media"
    }
  }
  mock_data "aws_s3_bucket" {
    defaults = {
      arn           = "arn:aws:s3:::shared-media"
      bucket_region = "eu-west-1"
    }
  }
}

variables {
  bucket_name = "my-media"
}

run "a_bucket_of_its_own" {
  command = plan

  assert {
    condition = (
      aws_s3_bucket.media.bucket == "my-media" && aws_s3_bucket_public_access_block.media != null &&
      aws_s3_bucket_lifecycle_configuration.media.rule[0].id == "abort-incomplete-uploads" && data.aws_s3_bucket.existing == null
    )
    error_message = "the module must create the bucket and its settings"
  }

  assert {
    condition     = output.env == { MY_BLOB = "s3", MY_BLOB_S3_BUCKET = "my-media", MY_BLOB_S3_REGION = "ap-northeast-1" }
    error_message = "without a prefix, the settings must have none"
  }

  assert {
    condition = (
      jsonencode([for p in output.permissions : p.resource]) == jsonencode(["arn:aws:s3:::my-media/*", "arn:aws:s3:::my-media/*", "arn:aws:s3:::my-media/*", "arn:aws:s3:::my-media/*", "arn:aws:s3:::my-media"]) &&
      length(output.permissions[4].conditions) == 0
    )
    error_message = "without a prefix, the role must reach every object and list the whole bucket"
  }
}

run "a_prefix_in_a_bucket_of_its_own" {
  command = plan

  variables {
    prefix = "tenant-a/"
  }

  assert {
    condition     = output.env["MY_BLOB_S3_PREFIX"] == "tenant-a/"
    error_message = "the prefix must reach the server"
  }

  assert {
    condition = (
      alltrue([for p in slice(output.permissions, 0, 4) : p.resource == "arn:aws:s3:::my-media/tenant-a/*"]) &&
      output.permissions[4].resource == "arn:aws:s3:::my-media" &&
      jsonencode(output.permissions[4].conditions) == jsonencode([{ test = "StringLike", variable = "s3:prefix", values = ["tenant-a/*"] }])
    )
    error_message = "the role must reach only the objects under the prefix, and list only under it"
  }
}

run "an_existing_bucket_with_a_prefix" {
  command = plan

  variables {
    bucket_name     = "shared-media"
    existing_bucket = true
    prefix          = "hosting/tenant-a/"
  }

  assert {
    condition = (
      aws_s3_bucket.media == null && aws_s3_bucket_public_access_block.media == null && aws_s3_bucket_ownership_controls.media == null &&
      aws_s3_bucket_lifecycle_configuration.media == null && length(aws_s3_bucket_versioning.media) == 0
    )
    error_message = "the module must create nothing for a bucket of yours, and leave its settings alone"
  }

  assert {
    condition = output.env == {
      MY_BLOB           = "s3"
      MY_BLOB_S3_BUCKET = "shared-media"
      MY_BLOB_S3_REGION = "eu-west-1"
      MY_BLOB_S3_PREFIX = "hosting/tenant-a/"
    }
    error_message = "the server must sign for the bucket's own region, and take the prefix"
  }

  assert {
    condition = (
      alltrue([for p in slice(output.permissions, 0, 4) : p.resource == "arn:aws:s3:::shared-media/hosting/tenant-a/*"]) &&
      jsonencode(output.permissions[4].conditions[0].values) == jsonencode(["hosting/tenant-a/*"])
    )
    error_message = "the role must reach only the objects under the prefix of the shared bucket"
  }
}

run "versioning_of_an_existing_bucket_fails" {
  command = plan

  variables {
    existing_bucket = true
    versioning      = true
  }

  expect_failures = [var.versioning]
}

run "a_prefix_without_a_slash_fails" {
  command = plan

  variables {
    prefix = "tenant-a"
  }

  expect_failures = [var.prefix]
}

run "a_prefix_with_a_dot_segment_fails" {
  command = plan

  variables {
    prefix = "../tenant-a/"
  }

  expect_failures = [var.prefix]
}

run "a_prefix_with_a_leading_slash_fails" {
  command = plan

  variables {
    prefix = "/tenant-a/"
  }

  expect_failures = [var.prefix]
}
