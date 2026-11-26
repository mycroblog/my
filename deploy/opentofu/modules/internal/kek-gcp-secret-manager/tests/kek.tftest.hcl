# SPDX-License-Identifier: AGPL-3.0-only

# On mocked providers: nothing here reaches Google Cloud. By plan alone: prevent_destroy refuses the destroy that follows
# an apply.

mock_provider "google" {}

mock_provider "random" {}

variables {
  project_id = "my-project"
}

run "it_generates_the_kek_through_the_write_only_attribute" {
  command = plan

  assert {
    condition     = google_secret_manager_secret.kek.secret_id == "my-kek" && google_secret_manager_secret.kek.project == "my-project"
    error_message = "the module must create the secret"
  }

  assert {
    condition     = google_secret_manager_secret_version.kek.secret_data_wo_version == 1 && google_secret_manager_secret_version.kek.secret_data == null && google_secret_manager_secret_version.kek.deletion_policy == "ABANDON"
    error_message = "the value must go through the write-only attribute, at the fixed version, and stay when the resource leaves"
  }

  assert {
    condition     = output.secret_files == { MY_KEK_FILE = "my-kek" } && output.env == {} && output.secret_env == {} && output.permissions == []
    error_message = "the KEK must reach the server as a mounted file, and nothing else"
  }
}

run "a_secret_made_elsewhere_creates_nothing" {
  command = plan

  variables {
    secret_id = "theirs"
    create    = false
  }

  assert {
    condition     = google_secret_manager_secret.kek == null && google_secret_manager_secret_version.kek == null && output.secret_files == { MY_KEK_FILE = "theirs" }
    error_message = "a secret made elsewhere must be used as it is"
  }
}
