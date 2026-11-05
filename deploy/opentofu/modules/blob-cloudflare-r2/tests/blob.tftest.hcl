# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches Cloudflare.

mock_provider "cloudflare" {}

variables {
  account_id                  = "0123456789abcdef"
  bucket_name                 = "my-media"
  access_key_id_secret_id     = "r2-key-id"
  secret_access_key_secret_id = "r2-secret"
}

run "the_settings_and_the_references" {
  command = plan

  assert {
    condition = output.env == {
      MY_BLOB             = "s3"
      MY_BLOB_S3_BUCKET   = "my-media"
      MY_BLOB_S3_ENDPOINT = "https://0123456789abcdef.r2.cloudflarestorage.com"
      MY_BLOB_S3_REGION   = "auto"
    }
    error_message = "R2 takes the account's endpoint and the region auto"
  }

  assert {
    condition     = output.secret_env == { MY_BLOB_S3_ACCESS_KEY_ID = "r2-key-id", MY_BLOB_S3_SECRET_ACCESS_KEY = "r2-secret" }
    error_message = "the credentials must arrive by reference"
  }
}

run "a_jurisdiction_has_its_own_endpoint" {
  command = plan

  variables {
    jurisdiction = "eu"
  }

  assert {
    condition     = output.env["MY_BLOB_S3_ENDPOINT"] == "https://0123456789abcdef.eu.r2.cloudflarestorage.com" && cloudflare_r2_bucket.media.jurisdiction == "eu"
    error_message = "a bucket of a jurisdiction is reached only through that jurisdiction's endpoint"
  }
}

run "an_unknown_jurisdiction_fails" {
  command = plan

  variables {
    jurisdiction = "mars"
  }

  expect_failures = [var.jurisdiction]
}

run "an_unknown_location_fails" {
  command = plan

  variables {
    location = "mars"
  }

  expect_failures = [var.location]
}
