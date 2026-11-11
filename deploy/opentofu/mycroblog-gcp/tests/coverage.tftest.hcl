# SPDX-License-Identifier: AGPL-3.0-only

# The wiring of coverage.json: each key that it puts in a section or fixes, planned on mocked providers with the kind that
# sets it, reaches the service's environment by its MY_ name, as a value, a secret, or a mounted file (_FILE).
# scripts/tofu-coverage.sh fails while this file does not name the MY_ name of such a key.

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
  mock_resource "google_service_account" {
    defaults = {
      email  = "sa@my-project.iam.gserviceaccount.com"
      member = "serviceAccount:sa@my-project.iam.gserviceaccount.com"
    }
  }
  mock_resource "google_memorystore_instance" {
    defaults = {
      endpoints = [{
        connections = [{
          psc_auto_connection = [
            { connection_type = "CONNECTION_TYPE_PRIMARY", ip_address = "10.20.0.8", port = 6379, forwarding_rule = "", network = "", project_id = "", psc_connection_id = "", service_attachment = "" },
          ]
        }]
      }]
      managed_server_ca = [{ ca_certs = [{ certificates = ["CERT"] }] }]
    }
  }
}

override_resource {
  target = module.ingress_load_balancer.google_compute_global_address.this
  values = {
    address = "203.0.113.10"
  }
}

mock_provider "random" {
  mock_resource "random_password" {
    defaults = {
      result = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    }
  }
}

mock_provider "cloudflare" {
  mock_data "cloudflare_ip_ranges" {
    defaults = {
      ipv4_cidrs = ["173.245.48.0/20"]
      ipv6_cidrs = ["2400:cb00::/32"]
    }
  }
}

variables {
  host  = "social.example.org"
  image = "ghcr.io/mycroblog/my:0.1.0"
  media = { bucket_name = "my-project-my-media" }
  mail  = { from = "noreply@social.example.org", smtp = { host = "smtp.example.org", username = "my-smtp-user" } }

  monitoring = { notification_emails = ["ops@social.example.org"] }
}

# Each assertion lists the names that must be in the service's environment: a setting or a secret by its name, and a mounted
# file by its name without _FILE.

run "the_recommended_setup" {
  command = plan

  variables {
    billing = { billing_account_id = "012345-6789AB-CDEF01" }
    workers = { delivery = { job_types = ["federation.*"] } }
  }

  assert {
    condition = length(setsubtract([
      "MY_HOST", "MY_ROLE", "MY_PROFILE", "MY_MEDIA_MAX_AV_BYTES", "MY_VAPID_PRIVATE_KEY", "MY_TRUSTED_PROXIES",
      "MY_DATABASE", "MY_DATABASE_URL",
      "MY_QUEUE", "MY_QUEUE_CLOUDTASKS_AUDIENCE", "MY_QUEUE_CLOUDTASKS_QUEUE", "MY_QUEUE_CLOUDTASKS_ROUTES",
      "MY_QUEUE_CLOUDTASKS_SERVICE_ACCOUNT", "MY_QUEUE_CLOUDTASKS_URL",
      "MY_BLOB", "MY_BLOB_GCS_BUCKET",
      "MY_STREAM", "MY_STREAM_PUBSUB_TOPIC", "MY_STREAM_PUBSUB_SUBSCRIPTION_PREFIX",
      "MY_MAIL", "MY_MAIL_FROM", "MY_MAIL_SMTP_HOST", "MY_MAIL_SMTP_PORT", "MY_MAIL_SMTP_USERNAME", "MY_MAIL_SMTP_PASSWORD",
      "MY_KEK",
      "MY_BILLING_SOURCES", "MY_BILLING_GCP_METHOD", "MY_BILLING_GCP_TABLE",
      "MY_METRICS_EXPORTER",
    ], [for name in concat(keys(module.app.env), keys(module.app.secret_env)) : trimsuffix(name, "_FILE")])) == 0
    error_message = "the recommended setup misses a setting of its sections: ${jsonencode(module.app.env)} ${jsonencode(module.app.secret_env)}"
  }
}

run "billing_by_a_budget" {
  command = plan

  variables {
    billing = { billing_account_id = "012345-6789AB-CDEF01", method = "budget" }
  }

  assert {
    condition     = length(setsubtract(["MY_BILLING_SOURCES", "MY_BILLING_GCP_METHOD", "MY_BILLING_GCP_BUDGET_SUBSCRIPTION"], keys(module.app.env))) == 0
    error_message = "billing by a budget must reach the service: ${jsonencode(module.app.env)}"
  }
}

run "ingress_cloudflare_with_a_media_host" {
  command = plan

  variables {
    ingress    = { kind = "cloudflare", media_host = "media.example.org" }
    cloudflare = { zone_id = "0123456789abcdef0123456789abcdef" }
  }

  assert {
    condition = length(setsubtract([
      "MY_TRUSTED_PROXIES", "MY_TRUSTED_PROXY_HEADER", "MY_ORIGIN_SECRET", "MY_ORIGIN_SECRET_HEADER", "MY_MEDIA_ORIGIN",
    ], concat(keys(module.app.env), keys(module.app.secret_env)))) == 0
    error_message = "Cloudflare must set the trust, the origin secret, and the media origin: ${jsonencode(module.app.env)} ${jsonencode(module.app.secret_env)}"
  }
}

run "database_external_pooled" {
  command = plan

  variables {
    database = { pooled = true }
    stream   = { kind = "postgres" }
  }

  assert {
    condition = length(setsubtract([
      "MY_DATABASE", "MY_DATABASE_URL", "MY_STREAM", "MY_STREAM_POSTGRES_URL",
    ], concat(keys(module.app.env), keys(module.app.secret_env)))) == 0
    error_message = "a pooled database must give the listeners its direct URL: ${jsonencode(module.app.env)} ${jsonencode(module.app.secret_env)}"
  }
}

run "queue_db_with_workers" {
  command = plan

  variables {
    queue   = { kind = "db" }
    service = { min_instances = 1, cpu_always_allocated = true }
    workers = { delivery = { job_types = ["federation.*"], concurrency = 8 } }
  }

  assert {
    condition = (
      length(setsubtract(["MY_QUEUE", "MY_WORKER_JOB_TYPES_EXCEPT"], keys(module.app.env))) == 0 &&
      length(setsubtract(["MY_ROLE", "MY_WORKER_JOB_TYPES", "MY_WORKER_JOB_CONCURRENCY"], keys(module.app.worker_env["delivery"]))) == 0
    )
    error_message = "the workers must split the job types between the service and the worker: ${jsonencode(module.app.env)} ${jsonencode(module.app.worker_env)}"
  }
}

run "media_s3" {
  command = plan

  variables {
    media = { kind = "s3", bucket_name = "my-media", endpoint = "https://s3.example.org", s3_region = "garage", path_style = true }
  }

  assert {
    condition = length(setsubtract([
      "MY_BLOB", "MY_BLOB_S3_BUCKET", "MY_BLOB_S3_ENDPOINT", "MY_BLOB_S3_REGION", "MY_BLOB_S3_PATH_STYLE",
      "MY_BLOB_S3_ACCESS_KEY_ID", "MY_BLOB_S3_SECRET_ACCESS_KEY",
    ], concat(keys(module.app.env), keys(module.app.secret_env)))) == 0
    error_message = "an S3-compatible store must reach the service: ${jsonencode(module.app.env)} ${jsonencode(module.app.secret_env)}"
  }
}

run "kv_external" {
  command = plan

  variables {
    kv = { kind = "external", tls_ca = true, key_prefix = "my:" }
  }

  assert {
    condition = length(setsubtract([
      "MY_KV", "MY_VALKEY_URL", "MY_VALKEY_TLS_CA_FILE", "MY_VALKEY_KEY_PREFIX",
    ], concat(keys(module.app.env), keys(module.app.secret_env)))) == 0
    error_message = "an external Valkey must reach the service: ${jsonencode(module.app.env)} ${jsonencode(module.app.secret_env)}"
  }
}

run "kek_kms" {
  command = plan

  variables {
    kek = { kind = "kms" }
  }

  assert {
    condition     = length(setsubtract(["MY_KEY_MANAGER", "MY_KMS_KEY"], keys(module.app.env))) == 0
    error_message = "Cloud KMS must reach the service: ${jsonencode(module.app.env)}"
  }
}

# The classification feeds the settings input, which refuses what a section sets, the module fixes, or the module does not
# take (the tests of modules/internal/settings: a test cannot expect the failure of a variable of a module it calls). A section
# that is off sets nothing, so its keys are free.

run "settings_may_set_what_a_section_that_is_off_would" {
  command = plan

  variables {
    monitoring = null
    settings   = { billing_sources = "cloudflare", metrics_exporter = "prometheus" }
  }

  assert {
    condition     = module.app.env["MY_BILLING_SOURCES"] == "cloudflare" && module.app.env["MY_METRICS_EXPORTER"] == "prometheus"
    error_message = "without billing and monitoring, settings may name their keys"
  }
}
