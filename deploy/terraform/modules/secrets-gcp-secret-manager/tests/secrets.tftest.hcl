# SPDX-License-Identifier: AGPL-3.0-only

mock_provider "google" {}

mock_provider "random" {}

variables {
  project_id = "p"
  secrets    = { MY_VAPID_PRIVATE_KEY = "my-vapid-key" }
  referenced = { smtp_password = "my-smtp-password" }
}

run "creates_every_secret_and_sets_only_its_own" {
  command = plan

  assert {
    condition     = toset(keys(google_secret_manager_secret.this)) == toset(["my-vapid-key", "my-smtp-password"])
    error_message = "every secret must be created"
  }

  assert {
    condition     = output.secret_env == tomap({ MY_VAPID_PRIVATE_KEY = "my-vapid-key" })
    error_message = "a referenced secret must stay out of secret_env, or its setting is named twice"
  }

  assert {
    condition     = output.ids == tomap({ MY_VAPID_PRIVATE_KEY = "my-vapid-key", smtp_password = "my-smtp-password" })
    error_message = "ids must list every secret"
  }
}

run "generates_only_what_is_asked" {
  command = plan

  variables {
    generate_p256_keys = ["MY_VAPID_PRIVATE_KEY"]
  }

  assert {
    condition     = keys(google_secret_manager_secret_version.generated) == ["MY_VAPID_PRIVATE_KEY"] && google_secret_manager_secret_version.generated["MY_VAPID_PRIVATE_KEY"].secret_data_wo_version == 1
    error_message = "only the keys asked for get a generated version, at a fixed number"
  }
}

run "nothing_is_generated_by_default" {
  command = plan

  assert {
    condition     = length(google_secret_manager_secret_version.generated) == 0
    error_message = "generating is opt-in"
  }
}

run "generating_a_key_outside_secrets_fails" {
  command = plan

  variables {
    generate_p256_keys = ["smtp_password"]
  }

  expect_failures = [var.generate_p256_keys]
}

run "a_key_in_both_maps_fails" {
  command = plan

  variables {
    referenced = { MY_VAPID_PRIVATE_KEY = "other" }
  }

  expect_failures = [var.referenced]
}
