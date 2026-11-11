# SPDX-License-Identifier: AGPL-3.0-only

# The example as it is composed, on a mocked provider: nothing here reaches Google Cloud. The kinds of each section are tested
# in deploy/opentofu/mycroblog-gcp.

mock_provider "google" {
  mock_data "google_client_config" {
    defaults = {
      project = "my-project"
      region  = "asia-northeast1"
    }
  }
  mock_data "google_project" {
    defaults = {
      number = "123456789012"
    }
  }
  mock_resource "google_compute_global_address" {
    defaults = {
      address = "203.0.113.10"
    }
  }
  mock_resource "google_service_account" {
    defaults = {
      email  = "sa@my-project.iam.gserviceaccount.com"
      member = "serviceAccount:sa@my-project.iam.gserviceaccount.com"
    }
  }
}

mock_provider "random" {}

# No mock of cloudflare: the example chooses no Cloudflare kind, so it plans with no CLOUDFLARE_API_TOKEN.

variables {
  project_id  = "my-project"
  host        = "social.example.org"
  image       = "ghcr.io/mycroblog/my:0.1.0"
  bucket_name = "my-project-my-media"

  mail_from          = "noreply@social.example.org"
  mail_smtp_host     = "smtp.example.org"
  mail_smtp_username = "my-smtp-user"
}

run "plans_the_recommended_setup" {
  command = plan

  assert {
    condition     = module.mycroblog.upgrade.service_name == "my" && module.mycroblog.upgrade.migration_job == "my-migrate"
    error_message = "the example must deploy the service and its migration job"
  }

  assert {
    condition     = output.dns_records == [{ name = "social.example.org", type = "A", rrdata = "203.0.113.10" }]
    error_message = "the example's DNS must point at the load balancer"
  }

  assert {
    condition     = toset(keys(output.secrets_to_fill)) == toset(["my-database-url", "my-smtp-password"])
    error_message = "the example has the database URL and the SMTP password to fill"
  }

  assert {
    condition     = output.kek.secret == "my-kek"
    error_message = "the example generates the KEK as a secret"
  }
}

run "the_console_can_be_turned_on" {
  command = plan

  variables {
    console_enabled = true
  }

  assert {
    condition     = output.console.platform == "gcp-cloud-run"
    error_message = "console.sh reads the console output"
  }
}
