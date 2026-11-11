# SPDX-License-Identifier: AGPL-3.0-only

# An upgrade by apply, on mocked providers with a stand-in for migrate.sh: nothing here reaches a cloud. Each run applies, and
# the next compares its migration with the one before, through previous_migration. That the workers move after the migration,
# and the service after the workers, is the order of depends_on, which no assertion here can watch.

mock_provider "google" {
  mock_resource "google_service_account" {
    defaults = {
      email  = "my-server@p.iam.gserviceaccount.com"
      member = "serviceAccount:my-server@p.iam.gserviceaccount.com"
    }
  }
  mock_resource "google_cloud_run_v2_service" {
    defaults = {
      uri = "https://my-abc-an.a.run.app"
    }
  }
}

run "the_first_deployment_migrates" {
  module {
    source = "../../../tests/upgrade-gcp"
  }

  variables {
    version_applied = "0.1.0"
  }

  assert {
    condition     = output.migration_id != null
    error_message = "the first deployment must run the migration before the services"
  }

  assert {
    condition     = output.images == { service = "ghcr.io/mycroblog/my:0.1.0", workers = { delivery = "ghcr.io/mycroblog/my:0.1.0" }, migration = "ghcr.io/mycroblog/my:0.1.0" }
    error_message = "the job, the worker, and the service must take the image"
  }
}

run "the_same_version_replaces_nothing" {
  module {
    source = "../../../tests/upgrade-gcp"
  }

  variables {
    version_applied    = "0.1.0"
    previous_migration = run.the_first_deployment_migrates.migration_id
  }

  assert {
    condition     = output.migration_id == var.previous_migration
    error_message = "an apply of the same version must not run the migration again"
  }
}

run "another_version_migrates_and_moves_the_services" {
  module {
    source = "../../../tests/upgrade-gcp"
  }

  variables {
    version_applied    = "0.2.0"
    previous_migration = run.the_same_version_replaces_nothing.migration_id
  }

  assert {
    condition     = output.migration_id != var.previous_migration
    error_message = "a new version must replace the migration, which runs it again"
  }

  assert {
    condition     = output.images == { service = "ghcr.io/mycroblog/my:0.2.0", workers = { delivery = "ghcr.io/mycroblog/my:0.2.0" }, migration = "ghcr.io/mycroblog/my:0.2.0" }
    error_message = "an apply of a new version must move the job, the worker, and the service: no image is ignored"
  }
}

run "without_migration_there_is_none" {
  command = plan

  variables {
    project_id      = "p"
    region          = "asia-northeast1"
    image           = "ghcr.io/mycroblog/my:0.1.0"
    host            = "social.example.org"
    service_ingress = "all"
    env             = { MY_QUEUE = "cloudtasks" }
    secret_files    = { MY_KEK_FILE = "kek" }
  }

  assert {
    condition     = output.migration_id == null
    error_message = "without migration, the module runs none"
  }
}
