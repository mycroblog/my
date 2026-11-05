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
  project_id    = "p"
  region        = "asia-northeast1"
  image         = "ghcr.io/mycroblog/my:0.1.0"
  host          = "social.example.org"
  kek_secret_id = "kek"
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
      MY_KEK_FILE        = "/secrets/kek/kek"
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
