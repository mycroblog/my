# SPDX-License-Identifier: AGPL-3.0-only

# The recommended setup, from the fewest inputs, on mocked providers: nothing here reaches Google Cloud.

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

# No mock of cloudflare: without a Cloudflare kind nothing uses the provider, which then plans with no CLOUDFLARE_API_TOKEN.

variables {
  host  = "social.example.org"
  image = "ghcr.io/mycroblog/my:0.1.0"
  media = { bucket_name = "my-project-my-media" }
  mail  = { from = "noreply@social.example.org", smtp = { host = "smtp.example.org", username = "my-smtp-user" } }
}

run "plans_the_recommended_setup_from_the_fewest_inputs" {
  command = plan

  assert {
    condition = module.app.env == {
      MY_BLOB                              = "gcs"
      MY_BLOB_GCS_BUCKET                   = "my-project-my-media"
      MY_DATABASE                          = "postgres"
      MY_HOST                              = "social.example.org"
      MY_KEK_FILE                          = "/secrets/my-kek-file/value"
      MY_MAIL                              = "smtp"
      MY_MAIL_FROM                         = "noreply@social.example.org"
      MY_MAIL_SMTP_HOST                    = "smtp.example.org"
      MY_MAIL_SMTP_PORT                    = "587"
      MY_MAIL_SMTP_USERNAME                = "my-smtp-user"
      MY_MEDIA_MAX_AV_BYTES                = "31457280"
      MY_PROFILE                           = "production"
      MY_QUEUE                             = "cloudtasks"
      MY_QUEUE_CLOUDTASKS_QUEUE            = "projects/my-project/locations/asia-northeast1/queues/my-jobs"
      MY_QUEUE_CLOUDTASKS_SERVICE_ACCOUNT  = "my-jobs-invoker@my-project.iam.gserviceaccount.com"
      MY_QUEUE_CLOUDTASKS_URL              = "https://my-123456789012.asia-northeast1.run.app"
      MY_ROLE                              = "all"
      MY_STREAM                            = "pubsub"
      MY_STREAM_PUBSUB_SUBSCRIPTION_PREFIX = "my-stream"
      MY_STREAM_PUBSUB_TOPIC               = "projects/my-project/topics/my-stream"
      MY_TRUSTED_PROXIES                   = "169.254.0.0/16, 203.0.113.10"
    }
    error_message = "the recommended setup does not compose the settings it should: ${jsonencode(module.app.env)}"
  }

  assert {
    condition     = module.app.secret_env == tomap({ MY_DATABASE_URL = "my-database-url", MY_MAIL_SMTP_PASSWORD = "my-smtp-password", MY_VAPID_PRIVATE_KEY = "my-vapid-key" })
    error_message = "the database URL, the SMTP password, and the VAPID key must arrive as secrets"
  }

  assert {
    condition     = module.app.service_ingress == "internal-and-load-balancer" && output.dns_records == [{ name = "social.example.org", type = "A", rrdata = "203.0.113.10" }]
    error_message = "the load balancer must close the run.app URL, and DNS must point at its address"
  }

  assert {
    condition     = toset(keys(output.secrets_to_fill)) == toset(["my-database-url", "my-smtp-password"])
    error_message = "the recommended setup has the database URL and the SMTP password to fill, and nothing else"
  }

  assert {
    condition     = output.secrets_to_fill["my-database-url"].command == "printf '%s' '<value>' | gcloud secrets versions add my-database-url --project=my-project --data-file=-"
    error_message = "each secret to fill must come with the command that adds it"
  }

  assert {
    condition     = output.kek.secret == "my-kek" && strcontains(output.kek.copy, "--secret=my-kek")
    error_message = "the KEK is generated as a secret, with the command that copies it"
  }

  assert {
    condition     = output.upgrade.project == "my-project" && output.upgrade.region == "asia-northeast1" && output.upgrade.name == "my"
    error_message = "upgrade.sh takes the project, the region, and the name"
  }

  assert {
    condition     = output.console == null
    error_message = "the console is off by default"
  }
}

run "takes_the_project_and_the_region_over_the_providers" {
  command = plan

  variables {
    project_id = "other-project"
    region     = "europe-west1"
  }

  assert {
    condition     = module.app.env["MY_QUEUE_CLOUDTASKS_QUEUE"] == "projects/other-project/locations/europe-west1/queues/my-jobs" && output.upgrade.region == "europe-west1"
    error_message = "project_id and region must win over the provider's"
  }
}

run "names_everything_from_name" {
  command = plan

  variables {
    name = "fedi"
  }

  assert {
    condition     = module.app.service_name == "fedi" && module.app.secret_env["MY_DATABASE_URL"] == "fedi-database-url" && module.kek_secret_manager.secret_files["MY_KEK_FILE"] == "fedi-kek" && module.app.env["MY_QUEUE_CLOUDTASKS_URL"] == "https://fedi-123456789012.asia-northeast1.run.app"
    error_message = "the service, the secrets, and the queue must take their names from name"
  }
}

# The first deployment creates these first, to hold the values before anything reads them.
run "the_secrets_can_be_created_first" {
  command = plan

  plan_options {
    target = [module.secrets, module.kek_secret_manager]
  }
}

# The first deployment creates this before the service, whose startup probe needs the schema.
run "the_migration_job_can_be_created_before_the_service" {
  command = plan

  plan_options {
    target = [module.app.google_cloud_run_v2_job.one_off["migrate"]]
  }
}
