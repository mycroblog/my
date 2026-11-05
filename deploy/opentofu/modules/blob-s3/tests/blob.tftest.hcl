# SPDX-License-Identifier: AGPL-3.0-only

# The module has no provider: nothing here reaches a store.

variables {
  endpoint                    = "https://storage.example.org"
  bucket                      = "my-media"
  access_key_id_secret_id     = "s3-key-id"
  secret_access_key_secret_id = "s3-secret"
}

run "the_settings_and_the_references" {
  command = plan

  assert {
    condition = output.env == {
      MY_BLOB             = "s3"
      MY_BLOB_S3_BUCKET   = "my-media"
      MY_BLOB_S3_ENDPOINT = "https://storage.example.org"
    }
    error_message = "an unset region and the host-name style must leave their settings out"
  }

  assert {
    condition     = output.secret_env == { MY_BLOB_S3_ACCESS_KEY_ID = "s3-key-id", MY_BLOB_S3_SECRET_ACCESS_KEY = "s3-secret" }
    error_message = "the key must arrive by reference"
  }
}

run "minio_style" {
  command = plan

  variables {
    endpoint   = "http://minio:9000"
    region     = "garage"
    path_style = true
  }

  assert {
    condition     = output.env["MY_BLOB_S3_PATH_STYLE"] == "true" && output.env["MY_BLOB_S3_REGION"] == "garage"
    error_message = "the path style and the region must reach the settings"
  }
}

run "an_endpoint_with_a_path_fails" {
  command = plan

  variables {
    endpoint = "https://storage.example.org/my-media"
  }

  expect_failures = [var.endpoint]
}
