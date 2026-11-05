# SPDX-License-Identifier: AGPL-3.0-only

# The example as it is composed, on a mocked provider: nothing here reaches Google Cloud.

mock_provider "google" {
  mock_resource "google_service_account" {
    defaults = {
      email  = "sa@my-project.iam.gserviceaccount.com"
      member = "serviceAccount:sa@my-project.iam.gserviceaccount.com"
    }
  }
}

mock_provider "random" {}

variables {
  project_id  = "my-project"
  host        = "social.example.org"
  image       = "ghcr.io/mycroblog/my:0.1.0"
  bucket_name = "my-project-my-media"

  mail_from          = "noreply@social.example.org"
  mail_smtp_host     = "smtp.example.org"
  mail_smtp_username = "my-smtp-user"
}

run "composes_every_setting" {
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
      MY_QUEUE_CLOUDTASKS_URL              = "https://social.example.org"
      MY_ROLE                              = "all"
      MY_STREAM                            = "pubsub"
      MY_STREAM_PUBSUB_SUBSCRIPTION_PREFIX = "my-stream"
      MY_STREAM_PUBSUB_TOPIC               = "projects/my-project/topics/my-stream"
      MY_TRUSTED_PROXIES                   = "169.254.0.0/16"
    }
    error_message = "the example does not compose the settings it should"
  }

  assert {
    condition     = module.app.secret_env == tomap({ MY_DATABASE_URL = "my-database-url", MY_MAIL_SMTP_PASSWORD = "my-smtp-password", MY_VAPID_PRIVATE_KEY = "my-vapid-key" })
    error_message = "the database URL, the SMTP password, and the VAPID key must arrive as secrets"
  }
}

# The first deployment creates these first, to hold the values before anything reads them.
run "the_secrets_can_be_created_first" {
  command = plan

  plan_options {
    target = [module.secrets, module.kek]
  }
}

# The first deployment creates this before the service, whose startup probe needs the schema.
run "the_migration_job_can_be_created_before_the_service" {
  command = plan

  # A plan that succeeds is the test: outputs are not evaluated for a targeted plan.
  plan_options {
    target = [module.app.google_cloud_run_v2_job.one_off["migrate"]]
  }
}

run "the_tick_calls_the_public_origin" {
  command = plan

  assert {
    condition     = module.queue.env["MY_QUEUE_CLOUDTASKS_URL"] == "https://social.example.org"
    error_message = "the queue must call the public origin, which no resource has to create first"
  }
}

run "the_operator_can_set_any_other_setting" {
  command = plan

  variables {
    env        = { MY_MEDIA_MAX_AV_BYTES = "1048576", MY_CAPTCHA = "turnstile" }
    secret_env = { MY_CAPTCHA_SECRET_KEY = "captcha-secret" }
  }

  assert {
    condition     = module.app.env["MY_MEDIA_MAX_AV_BYTES"] == "1048576" && module.app.env["MY_CAPTCHA"] == "turnstile" && module.app.secret_env["MY_CAPTCHA_SECRET_KEY"] == "captcha-secret"
    error_message = "plain settings must reach the server"
  }
}
