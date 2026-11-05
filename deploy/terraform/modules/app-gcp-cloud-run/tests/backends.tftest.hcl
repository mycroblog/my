# SPDX-License-Identifier: AGPL-3.0-only

# The contract of the backends, on a mocked provider: nothing here reaches Google Cloud.

mock_provider "google" {
  mock_resource "google_service_account" {
    defaults = {
      email  = "my-server@p.iam.gserviceaccount.com"
      member = "serviceAccount:my-server@p.iam.gserviceaccount.com"
    }
  }
}

variables {
  project_id   = "p"
  region       = "asia-northeast1"
  image        = "ghcr.io/mycroblog/my:0.1.0"
  host         = "social.example.org"
  secret_files = { MY_KEK_FILE = "kek" }
  backends = [
    {
      env        = { MY_DATABASE = "postgres" }
      secret_env = { MY_DATABASE_URL = "db-url" }
    },
    {
      env      = { MY_QUEUE = "cloudtasks" }
      invokers = ["invoker@p.iam.gserviceaccount.com"]
      roles = [
        { role = "roles/cloudtasks.enqueuer", scope = "queue", resource = "projects/p/locations/l/queues/q" },
      ]
    },
  ]
}

run "merges_every_source" {
  command = plan

  variables {
    env        = { MY_MAIL = "smtp" }
    secret_env = { MY_VAPID_PRIVATE_KEY = "vapid" }
  }

  assert {
    condition = output.env == {
      MY_DATABASE        = "postgres"
      MY_HOST            = "social.example.org"
      MY_KEK_FILE        = "/secrets/my-kek-file/value"
      MY_MAIL            = "smtp"
      MY_PROFILE         = "production"
      MY_QUEUE           = "cloudtasks"
      MY_ROLE            = "all"
      MY_TRUSTED_PROXIES = "169.254.0.0/16"
    }
    error_message = "env is not the merge of what the module sets, the backends, and the env input"
  }

  assert {
    condition     = output.secret_env == tomap({ MY_DATABASE_URL = "db-url", MY_VAPID_PRIVATE_KEY = "vapid" })
    error_message = "secret_env is not the merge of the backends and the secret_env input"
  }

  assert {
    condition     = toset(keys(google_secret_manager_secret_iam_member.server)) == toset(["db-url", "kek", "vapid"])
    error_message = "the service account must read every secret it is given"
  }

  assert {
    condition     = length(google_cloud_tasks_queue_iam_member.backend) == 1 && length(google_project_iam_member.backend) == 0
    error_message = "a backend's role must go on the scope it names"
  }

  assert {
    condition     = keys(google_cloud_run_v2_service_iam_member.invoker) == ["invoker@p.iam.gserviceaccount.com"]
    error_message = "each invoker must get run.invoker on the service"
  }
}

run "swapping_a_backend_changes_only_its_settings" {
  command = plan

  variables {
    backends = [
      {
        env        = { MY_DATABASE = "sqlite" }
        secret_env = { MY_DATABASE_URL = "other-url" }
      },
      {
        env      = { MY_QUEUE = "cloudtasks" }
        invokers = ["invoker@p.iam.gserviceaccount.com"]
        roles = [
          { role = "roles/cloudtasks.enqueuer", scope = "queue", resource = "projects/p/locations/l/queues/q" },
        ]
      },
    ]
  }

  assert {
    condition     = output.env["MY_DATABASE"] == "sqlite" && output.secret_env["MY_DATABASE_URL"] == "other-url"
    error_message = "the settings of the swapped backend did not change"
  }

  assert {
    condition     = output.env["MY_QUEUE"] == "cloudtasks" && output.env["MY_HOST"] == "social.example.org" && length(google_cloud_tasks_queue_iam_member.backend) == 1
    error_message = "swapping one backend changed something else"
  }
}

run "a_setting_named_by_two_backends_fails" {
  command = plan

  variables {
    backends = [
      { env = { MY_DATABASE = "postgres" } },
      { env = { MY_DATABASE = "sqlite" } },
    ]
  }

  expect_failures = [google_service_account.server]
}

run "a_setting_in_env_and_secret_env_fails" {
  command = plan

  variables {
    env        = { MY_VAPID_PRIVATE_KEY = "plain" }
    secret_env = { MY_VAPID_PRIVATE_KEY = "vapid" }
  }

  expect_failures = [google_service_account.server]
}

run "a_setting_the_module_owns_fails" {
  command = plan

  variables {
    env = { MY_HOST = "other.example.org" }
  }

  expect_failures = [google_service_account.server]
}

run "a_setting_and_its_file_form_fail" {
  command = plan

  variables {
    secret_env = { MY_KEK = "another-kek" }
  }

  expect_failures = [google_service_account.server]
}

run "a_role_without_its_scope_fails" {
  command = plan

  variables {
    backends = [
      { roles = [{ role = "roles/x", scope = "somewhere", resource = "r" }] },
    ]
  }

  expect_failures = [var.backends]
}

run "latest_is_refused" {
  command = plan

  variables {
    image = "ghcr.io/mycroblog/my:latest"
  }

  expect_failures = [var.image]
}

run "console_is_off_by_default" {
  command = plan

  assert {
    condition     = keys(google_cloud_run_v2_job.one_off) == ["migrate"] && output.console == null
    error_message = "only the migration job may exist while the console is off"
  }
}

run "console_on" {
  command = plan

  variables {
    console_enabled = true
  }

  assert {
    condition     = toset(keys(google_cloud_run_v2_job.one_off)) == toset(["migrate", "console"])
    error_message = "the console job is missing"
  }

  assert {
    condition     = output.console.platform == "gcp-cloud-run" && output.console.project == "p" && output.console.region == "asia-northeast1"
    error_message = "the console output does not say how to run it"
  }
}

run "no_kek_fails" {
  command = plan

  variables {
    secret_files = {}
  }

  expect_failures = [google_service_account.server]
}

run "a_kms_without_a_key_fails" {
  command = plan

  variables {
    secret_files = {}
    env          = { MY_KEY_MANAGER = "gcpkms" }
  }

  expect_failures = [google_service_account.server]
}

run "a_secret_file_needs_a_file_name" {
  command = plan

  variables {
    secret_files = { MY_KEK = "kek" }
  }

  expect_failures = [var.secret_files]
}

# Swapping the KEK: the same app with each kek-* module, in one line of backends.
run "kek_generated_in_secret_manager" {
  command = plan

  module {
    source = "../kek-gcp-secret-manager"
  }

  variables {
    project_id = "p"
  }

  assert {
    condition     = google_secret_manager_secret_version.kek[0].secret_data_wo_version == 1 && google_secret_manager_secret_version.kek[0].deletion_policy == "ABANDON"
    error_message = "the version of the generated KEK must stay fixed, and survive leaving the configuration"
  }
}

# Applied, for run.* to read its outputs below; nothing with prevent_destroy is created.
run "kek_in_secret_manager" {
  module {
    source = "../kek-gcp-secret-manager"
  }

  variables {
    project_id = "p"
    create     = false
  }

  assert {
    condition     = length(google_secret_manager_secret_version.kek) == 0
    error_message = "a secret made elsewhere must get no version from this module"
  }
}

run "kek_in_kms" {
  command = plan

  module {
    source = "../kek-gcp-kms"
  }

  variables {
    project_id = "p"
    location   = "asia-northeast1"
  }

  assert {
    condition     = length(google_kms_crypto_key.kek) == 1
    error_message = "the module must create the key"
  }
}

# Applied, for run.* to read its outputs below; nothing with prevent_destroy is created.
run "kek_in_kms_made_elsewhere" {
  module {
    source = "../kek-gcp-kms"
  }

  variables {
    project_id    = "p"
    location      = "asia-northeast1"
    crypto_key_id = "projects/p/locations/l/keyRings/r/cryptoKeys/other"
  }

  assert {
    condition     = length(google_kms_crypto_key.kek) == 0 && output.env["MY_KMS_KEY"] == "projects/p/locations/l/keyRings/r/cryptoKeys/other"
    error_message = "a key made elsewhere must be used, and none created"
  }
}

run "app_with_the_secret_manager_kek" {
  command = plan

  variables {
    secret_files = {}
    backends = [
      { secret_files = run.kek_in_secret_manager.secret_files },
    ]
  }

  assert {
    condition     = output.env["MY_KEK_FILE"] == "/secrets/my-kek-file/value" && !contains(keys(output.env), "MY_KEY_MANAGER")
    error_message = "the KEK must arrive as a mounted file"
  }

  assert {
    condition     = keys(google_secret_manager_secret_iam_member.server) == ["my-kek"] && length(google_kms_crypto_key_iam_member.backend) == 0
    error_message = "the service account must read the secret and hold no role on a key"
  }

  assert {
    condition     = one(google_cloud_run_v2_job.one_off["migrate"].template[0].template[0].volumes).secret[0].secret == "my-kek"
    error_message = "the jobs must mount the KEK as the service does"
  }
}

run "app_with_the_kms_kek" {
  command = plan

  variables {
    secret_files    = {}
    console_enabled = true
    backends = [
      { env = run.kek_in_kms_made_elsewhere.env, roles = run.kek_in_kms_made_elsewhere.roles },
    ]
  }

  assert {
    condition     = output.env["MY_KEY_MANAGER"] == "gcpkms" && output.env["MY_KMS_KEY"] == "projects/p/locations/l/keyRings/r/cryptoKeys/other" && !contains(keys(output.env), "MY_KEK_FILE")
    error_message = "the KMS settings must replace the mounted file"
  }

  assert {
    condition     = keys(google_kms_crypto_key_iam_member.backend) == ["projects/p/locations/l/keyRings/r/cryptoKeys/other/roles/cloudkms.cryptoKeyEncrypterDecrypter"] && length(google_project_iam_member.backend) == 0
    error_message = "the grant must be on the key, not on the project"
  }

  assert {
    condition     = length(google_secret_manager_secret_iam_member.server) == 0
    error_message = "the KMS needs no secret"
  }

  assert {
    condition     = google_cloud_run_v2_job.one_off["console"].template[0].template[0].service_account == google_service_account.server.email && google_cloud_run_v2_job.one_off["migrate"].template[0].template[0].service_account == google_service_account.server.email
    error_message = "the jobs must run as the account that holds the grant"
  }
}
