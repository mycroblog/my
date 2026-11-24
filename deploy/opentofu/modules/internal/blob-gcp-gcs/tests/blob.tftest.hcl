# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches Google Cloud.

mock_provider "google" {
  mock_data "google_storage_bucket" {
    defaults = {
      uniform_bucket_level_access = true
    }
  }
}

variables {
  project_id  = "my-project"
  bucket_name = "my-media"
  location    = "us-central1"
}

run "a_bucket_of_its_own" {
  command = plan

  assert {
    condition     = google_storage_bucket.media.uniform_bucket_level_access && data.google_storage_bucket.existing == null
    error_message = "the module must create the bucket, with uniform access"
  }

  assert {
    condition     = output.env == { MY_BLOB = "gcs", MY_BLOB_GCS_BUCKET = "my-media" }
    error_message = "without a prefix, the settings must have none"
  }

  assert {
    condition     = length(output.permissions) == 1 && output.permissions[0].resource == "my-media" && output.permissions[0].condition == null
    error_message = "without a prefix, the service account must reach every object of the bucket"
  }
}

run "a_prefix_in_a_bucket_of_its_own" {
  command = plan

  variables {
    prefix = "tenant-a/"
  }

  assert {
    condition     = output.env["MY_BLOB_GCS_PREFIX"] == "tenant-a/"
    error_message = "the prefix must reach the server"
  }

  assert {
    condition = (
      output.permissions[0].permission == "roles/storage.objectUser" && output.permissions[0].scope == "bucket" &&
      output.permissions[0].condition.expression == "resource.name.startsWith(\"projects/_/buckets/my-media/objects/tenant-a/\")"
    )
    error_message = "the service account must reach only the objects under the prefix"
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
    condition     = google_storage_bucket.media == null && data.google_storage_bucket.existing.name == "shared-media"
    error_message = "the module must create nothing for a bucket of yours"
  }

  assert {
    condition     = output.env == { MY_BLOB = "gcs", MY_BLOB_GCS_BUCKET = "shared-media", MY_BLOB_GCS_PREFIX = "hosting/tenant-a/" }
    error_message = "the server must take the bucket and the prefix"
  }

  assert {
    condition     = output.permissions[0].condition.expression == "resource.name.startsWith(\"projects/_/buckets/shared-media/objects/hosting/tenant-a/\")"
    error_message = "the service account must reach only the objects under the prefix of the shared bucket"
  }
}

run "an_existing_bucket_without_uniform_access_fails_with_a_prefix" {
  command = plan

  override_data {
    target = data.google_storage_bucket.existing
    values = {
      uniform_bucket_level_access = false
    }
  }

  variables {
    existing_bucket = true
    prefix          = "tenant-a/"
  }

  expect_failures = [data.google_storage_bucket.existing]
}

run "an_existing_bucket_without_uniform_access_and_no_prefix" {
  command = plan

  override_data {
    target = data.google_storage_bucket.existing
    values = {
      uniform_bucket_level_access = false
    }
  }

  variables {
    existing_bucket = true
  }

  assert {
    condition     = output.permissions[0].condition == null
    error_message = "a grant on the whole bucket needs no uniform access"
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
