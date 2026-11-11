# SPDX-License-Identifier: AGPL-3.0-only

# Each kind of each section, on mocked providers: the plan succeeds, the app receives that backend, and secrets_to_fill lists
# exactly what that kind needs.

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
}

# ---------------------------------------------------------------------------------------------------------------------------------
# ingress

run "ingress_domain_mapping" {
  command = plan

  variables {
    ingress = { kind = "domain-mapping" }
  }

  assert {
    condition     = module.app.service_ingress == "all" && module.app.env["MY_TRUSTED_PROXIES"] == "169.254.0.0/16" && module.ingress_load_balancer == null
    error_message = "the domain mapping must leave the service open, with nothing more to trust, and no load balancer"
  }
}

run "ingress_cloudflare" {
  command = plan

  variables {
    ingress    = { kind = "cloudflare" }
    cloudflare = { zone_id = "0123456789abcdef0123456789abcdef" }
  }

  assert {
    condition     = module.app.service_ingress == "all" && module.app.env["MY_TRUSTED_PROXY_HEADER"] == "x-forwarded-for" && module.app.env["MY_ORIGIN_SECRET_HEADER"] == "x-my-origin-secret" && module.app.secret_env["MY_ORIGIN_SECRET"] == "my-origin-secret"
    error_message = "Cloudflare must set the trust and the origin secret"
  }

  assert {
    condition     = module.ingress_domain_mapping != null && module.ingress_cloudflare != null && output.dns_records == []
    error_message = "a domain mapping of the host is Cloudflare's origin, and Cloudflare creates the record"
  }

  assert {
    condition     = !contains(keys(output.secrets_to_fill), "my-origin-secret")
    error_message = "the origin secret is generated, not filled"
  }
}

run "ingress_cloudflare_with_the_host_header_override" {
  command = plan

  variables {
    ingress    = { kind = "cloudflare", media_host = "media.example.org" }
    cloudflare = { zone_id = "0123456789abcdef0123456789abcdef", host_header_override = true }
  }

  assert {
    condition     = module.ingress_domain_mapping == null && module.media_origin == null && module.app.env["MY_MEDIA_ORIGIN"] == "https://media.example.org"
    error_message = "with the override, Cloudflare connects to the run.app URL, and needs no domain mapping"
  }
}

run "ingress_media_host_beside_the_load_balancer" {
  command = plan

  variables {
    ingress    = { media_host = "media.example.org" }
    cloudflare = { zone_id = "0123456789abcdef0123456789abcdef" }
  }

  assert {
    condition     = module.app.service_ingress == "internal-and-load-balancer" && module.app.env["MY_MEDIA_ORIGIN"] == "https://media.example.org" && module.media_origin != null && !contains(keys(module.app.secret_env), "MY_ORIGIN_SECRET")
    error_message = "a media host sits beside the load balancer, on a domain mapping of its own"
  }
}

# ---------------------------------------------------------------------------------------------------------------------------------
# database

run "database_external_pooled" {
  command = plan

  variables {
    database = { pooled = true }
  }

  assert {
    condition     = module.app.secret_env["MY_DATABASE_URL"] == "my-database-url-pooled" && module.app.one_off_secret_env["MY_DATABASE_URL"] == "my-database-url"
    error_message = "the service takes the pooled URL, and the one-off work the direct one"
  }

  assert {
    condition     = toset(keys(output.secrets_to_fill)) == toset(["my-database-url", "my-database-url-pooled", "my-smtp-password"])
    error_message = "both URLs are to fill"
  }
}

run "database_cloud_sql" {
  command = plan

  variables {
    database = { kind = "cloud-sql" }
  }

  assert {
    condition     = module.app.env["MY_DATABASE"] == "postgres" && module.app.secret_env["MY_DATABASE_URL"] == "my-database-url" && module.db_external == null
    error_message = "Cloud SQL must give the app its URL"
  }

  assert {
    condition     = toset(keys(output.secrets_to_fill)) == toset(["my-smtp-password"])
    error_message = "Cloud SQL writes its own URL: there is nothing to fill for it"
  }
}

run "database_cloud_sql_on_a_vpc" {
  command = plan

  variables {
    database = { kind = "cloud-sql" }
    network  = { kind = "vpc" }
  }

  assert {
    condition     = module.app.subnetwork != null && module.db_cloud_sql != null
    error_message = "with a VPC, the app and Cloud SQL share it"
  }
}

# ---------------------------------------------------------------------------------------------------------------------------------
# queue

run "queue_db" {
  command = plan

  variables {
    queue   = { kind = "db" }
    service = { min_instances = 1, cpu_always_allocated = true }
  }

  assert {
    condition     = module.app.env["MY_QUEUE"] == "db" && !contains(keys(module.app.env), "MY_QUEUE_CLOUDTASKS_URL") && module.queue_cloud_tasks == null
    error_message = "queue = db must reach the app, with no Cloud Tasks"
  }
}

run "workers_reach_the_service_and_the_queue" {
  command = plan

  variables {
    workers = { delivery = { job_types = ["federation.*"], max_instances = 20 } }
  }

  assert {
    condition     = module.app.worker_services == { delivery = "my-delivery" } && strcontains(module.app.env["MY_QUEUE_CLOUDTASKS_ROUTES"], "queues/my-jobs-delivery@https://my-delivery-123456789012.asia-northeast1.run.app")
    error_message = "one workers value must build the worker's service and its queue"
  }
}

# ---------------------------------------------------------------------------------------------------------------------------------
# media

run "media_r2" {
  command = plan

  variables {
    media      = { kind = "r2", bucket_name = "my-media" }
    cloudflare = { account_id = "0123456789abcdef0123456789abcdef" }
  }

  assert {
    condition     = module.app.env["MY_BLOB"] == "s3" && module.app.env["MY_BLOB_S3_ENDPOINT"] == "https://0123456789abcdef0123456789abcdef.r2.cloudflarestorage.com" && module.media_gcs == null
    error_message = "R2 must reach the app as an S3 store"
  }

  assert {
    condition     = toset(keys(output.secrets_to_fill)) == toset(["my-database-url", "my-smtp-password", "my-media-access-key-id", "my-media-secret-access-key"])
    error_message = "R2's access key is to fill"
  }
}

run "media_s3" {
  command = plan

  variables {
    media = { kind = "s3", bucket_name = "my-media", endpoint = "https://s3.example.org", s3_region = "garage" }
  }

  assert {
    condition     = module.app.env["MY_BLOB"] == "s3" && module.app.env["MY_BLOB_S3_ENDPOINT"] == "https://s3.example.org" && module.app.secret_env["MY_BLOB_S3_ACCESS_KEY_ID"] == "my-media-access-key-id"
    error_message = "an S3-compatible store must reach the app with its keys"
  }
}

# ---------------------------------------------------------------------------------------------------------------------------------
# kv and stream

run "kv_memorystore_on_a_vpc_with_the_valkey_stream" {
  command = plan

  variables {
    network = { kind = "vpc" }
    kv      = { kind = "memorystore" }
    stream  = { kind = "valkey" }
  }

  assert {
    condition     = module.app.env["MY_KV"] == "valkey" && module.app.env["MY_VALKEY_URL"] == "valkeys://10.20.0.8:6379" && module.app.env["MY_STREAM"] == "valkey" && module.app.env["MY_VALKEY_TLS_CA_FILE"] == "/secrets/my-valkey-tls-ca-file/value"
    error_message = "Memorystore must give the app its URL and CA, and the stream must use it"
  }

  assert {
    condition     = toset(keys(output.secrets_to_fill)) == toset(["my-database-url", "my-smtp-password"])
    error_message = "Memorystore needs nothing to fill"
  }
}

run "kv_external_with_its_ca" {
  command = plan

  variables {
    kv = { kind = "external", tls_ca = true }
  }

  assert {
    condition     = module.app.env["MY_KV"] == "valkey" && module.app.secret_env["MY_VALKEY_URL"] == "my-valkey-url" && module.app.env["MY_STREAM"] == "pubsub"
    error_message = "an external Valkey must reach the app by its secret"
  }

  assert {
    condition     = toset(keys(output.secrets_to_fill)) == toset(["my-database-url", "my-smtp-password", "my-valkey-url", "my-valkey-ca"])
    error_message = "the Valkey URL and its CA are to fill"
  }
}

run "stream_postgres" {
  command = plan

  variables {
    stream = { kind = "postgres" }
  }

  assert {
    condition     = module.app.env["MY_STREAM"] == "postgres" && module.stream_pubsub == null
    error_message = "stream = postgres must reach the app"
  }
}

# ---------------------------------------------------------------------------------------------------------------------------------
# mail, kek, network

run "mail_without_a_username" {
  command = plan

  variables {
    mail = { from = "noreply@social.example.org", smtp = { host = "relay.example.org", port = 25 } }
  }

  assert {
    condition     = !contains(keys(module.app.secret_env), "MY_MAIL_SMTP_PASSWORD") && toset(keys(output.secrets_to_fill)) == toset(["my-database-url"])
    error_message = "an SMTP server without a user needs no password"
  }
}

run "kek_kms" {
  command = plan

  variables {
    kek = { kind = "kms" }
  }

  assert {
    condition     = module.app.env["MY_KEY_MANAGER"] == "gcpkms" && !contains(keys(module.app.env), "MY_KEK_FILE") && output.kek.kind == "kms" && output.kek.copy == null
    error_message = "the KEK must come from Cloud KMS, which has nothing to copy"
  }
}

run "network_existing" {
  command = plan

  variables {
    network = {
      kind                          = "existing"
      network                       = "projects/my-project/global/networks/mine"
      subnetwork                    = "projects/my-project/regions/asia-northeast1/subnetworks/mine"
      memorystore_connection_policy = true
    }
    kv = { kind = "memorystore" }
  }

  assert {
    condition     = module.app.subnetwork == "projects/my-project/regions/asia-northeast1/subnetworks/mine" && module.network == null && module.app.env["MY_KV"] == "valkey"
    error_message = "a VPC of yours must reach the app and the backends"
  }
}

run "network_vpc_with_nat" {
  command = plan

  variables {
    network = { kind = "vpc", nat = true }
  }

  assert {
    condition     = module.network != null && module.app.subnetwork != null
    error_message = "a VPC with Cloud NAT must reach the app"
  }
}

# ---------------------------------------------------------------------------------------------------------------------------------
# operations, settings, and secrets

run "monitoring_and_billing" {
  command = plan

  variables {
    monitoring = { notification_emails = ["ops@example.org"] }
    billing    = { billing_account_id = "012345-6789AB-CDEF01" }
  }

  assert {
    condition     = module.app.env["MY_METRICS_EXPORTER"] == "none" && module.app.env["MY_BILLING_SOURCES"] == "gcp"
    error_message = "monitoring and billing must reach the app"
  }
}

run "settings_and_secrets_reach_the_server" {
  command = plan

  variables {
    settings = { translator_deepl_url = "https://api.deepl.com" }
    secrets  = ["translator_deepl_api_key"]
  }

  assert {
    condition     = module.app.env["MY_TRANSLATOR_DEEPL_URL"] == "https://api.deepl.com" && module.app.secret_env["MY_TRANSLATOR_DEEPL_API_KEY"] == "my-translator-deepl-api-key"
    error_message = "a setting must reach the server, and a secret by its secret"
  }

  assert {
    condition     = output.secrets_to_fill["my-translator-deepl-api-key"].what == "The value of the setting translator_deepl_api_key."
    error_message = "a secret of secrets is to fill"
  }
}

run "the_console" {
  command = plan

  variables {
    console_enabled = true
  }

  assert {
    condition     = output.console.platform == "gcp-cloud-run" && output.console.project == "my-project"
    error_message = "the console must be there to pipe to console.sh"
  }
}
