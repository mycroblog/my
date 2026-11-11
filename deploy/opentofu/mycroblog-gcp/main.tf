# SPDX-License-Identifier: AGPL-3.0-only

# One deployment of my on Google Cloud: Cloud Run, and a backend of each role picked by the kind of its section. It composes
# the modules under ../modules/internal, which resolve inside the same Git package, so a deployment pins one ref.
#
# The composition, in three steps:
#   1. locals turn the inputs into decisions: which kind, which names, which network, what the ingress needs of the service;
#   2. every kind is a module block with lifecycle.enabled, so that exactly one of each role is on;
#   3. the enabled blocks are gathered into the app's backends: a reference to a disabled block is null and drops out.
#
# A disabled block still evaluates its inputs against the validations of its module, so each block gets well-formed inputs
# whatever the kind: local.unused holds what a block that is off takes in place of what its section does not have.

terraform {
  required_version = ">= 1.11"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 7.0, < 8.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.7, < 4.0"
    }
    # Without a Cloudflare kind no resource uses it, and a plan needs no CLOUDFLARE_API_TOKEN.
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = ">= 5.0, < 6.0"
    }
  }
}

data "google_client_config" "current" {}

# ---------------------------------------------------------------------------------------------------------------------------------
# 1. Decisions

locals {
  project_id = coalesce(var.project_id, data.google_client_config.current.project)
  region     = coalesce(var.region, data.google_client_config.current.region)

  # Known before the service exists, so the ingress, the queue, and the monitoring take it from here rather than from the app.
  service_name = var.name

  # What the ingress decides of the service: a load balancer closes the run.app URL, the others come from the internet.
  service_ingress = var.ingress.kind == "load-balancer" ? "internal-and-load-balancer" : "all"

  cloudflare = {
    zone_id              = try(var.cloudflare.zone_id, null)
    account_id           = try(var.cloudflare.account_id, null)
    host_header_override = try(var.cloudflare.host_header_override, false)
    proxied              = try(var.cloudflare.proxied, true)
  }
  # Where Cloudflare connects: the service's run.app URL when it may rewrite the Host header, otherwise a domain mapping of the
  # host, which answers on Google's front end by its own name.
  cloudflare_origin = local.cloudflare.host_header_override ? "${local.service_name}-${data.google_project.this.number}.${local.region}.run.app" : "ghs.googlehosted.com"
  domain_mapping    = var.ingress.kind == "domain-mapping" || (var.ingress.kind == "cloudflare" && !local.cloudflare.host_header_override)

  # Every secret of the deployment, named from var.name.
  secret_ids = {
    kek                     = "${var.name}-kek"
    vapid                   = "${var.name}-vapid-key"
    database_url            = "${var.name}-database-url"
    database_url_pooled     = "${var.name}-database-url-pooled"
    smtp_password           = "${var.name}-smtp-password"
    media_access_key_id     = "${var.name}-media-access-key-id"
    media_secret_access_key = "${var.name}-media-secret-access-key"
    kv_url                  = "${var.name}-valkey-url"
    kv_tls_ca               = "${var.name}-valkey-ca"
    origin_secret           = "${var.name}-origin-secret"
  }

  # The secrets only you can fill, by key, with what goes in each. This module creates them empty.
  to_fill = merge(
    var.database.kind == "external" ? { database_url = "The direct connection URL of the database, as postgres://..." } : {},
    var.database.pooled ? { database_url_pooled = "The pooled connection URL of the database, which the service uses." } : {},
    var.mail.smtp.username != null ? { smtp_password = "The password of ${var.mail.smtp.username} at ${var.mail.smtp.host}." } : {},
    contains(["r2", "s3"], var.media.kind) ? {
      media_access_key_id     = "The access key ID of the media bucket ${var.media.bucket_name}."
      media_secret_access_key = "The secret access key of the media bucket ${var.media.bucket_name}."
    } : {},
    var.kv.kind == "external" ? { kv_url = "The URL of the Valkey server, as valkeys://... or rediss://..." } : {},
    var.kv.tls_ca ? { kv_tls_ca = "The PEM of the CA that signs the certificate of the Valkey server." } : {},
  )
  # Created and written by this module.
  generated = var.ingress.kind == "cloudflare" ? ["origin_secret"] : []

  # The network the service and the backends on private addresses attach to, in the shape of the network contract, or null.
  network = (
    var.network.kind == "vpc" ? {
      platform                      = module.network.platform
      network                       = module.network.network
      subnetwork                    = module.network.subnetwork
      private_services_range        = module.network.private_services_range
      memorystore_connection_policy = module.network.memorystore_connection_policy
      nat                           = module.network.nat
    } :
    var.network.kind == "existing" ? {
      platform                      = "gcp"
      network                       = var.network.network
      subnetwork                    = var.network.subnetwork
      private_services_range        = var.network.private_services_range
      memorystore_connection_policy = var.network.memorystore_connection_policy
      nat                           = false
    } : null
  )

  # What a block that is off takes where its section has nothing, so that its module's validations pass.
  unused = {
    id            = "unused"
    host          = "unused.invalid"
    endpoint      = "https://unused.invalid"
    origin_secret = "unused-unused-unused-unused-unused"
    network = {
      platform                      = "gcp"
      network                       = "projects/unused/global/networks/unused"
      subnetwork                    = "projects/unused/regions/unused/subnetworks/unused"
      private_services_range        = null
      memorystore_connection_policy = true
      nat                           = false
    }
  }
}

data "google_project" "this" {
  project_id = local.project_id
}

# ---------------------------------------------------------------------------------------------------------------------------------
# 2. Generated values and secrets

# Cloudflare's rule holds the value, so it lands in the state either way; generating it here saves a step.
resource "random_password" "origin_secret" {
  length  = 64
  special = false

  lifecycle {
    enabled = var.ingress.kind == "cloudflare"
  }
}

locals {
  settings_list = jsondecode(file("${path.module}/../modules/internal/settings/settings.json"))

  # How this module handles each configuration key (coverage.json, which scripts/tofu-coverage.sh checks against the settings
  # list): an entry is a key, or a prefix that ends in *.
  coverage = jsondecode(file("${path.module}/coverage.json"))
  # The sections that are null set nothing, so settings may name their keys: billing_sources for a plugin, for one.
  sections_off = concat(var.billing == null ? ["billing"] : [], var.monitoring == null ? ["monitoring"] : [])
  managed_entries = concat(
    flatten([for section, entries in local.coverage.section : entries if !contains(local.sections_off, section)]),
    local.coverage.fixed,
  )
  # The keys that a section sets or the module fixes, which settings and secrets must not name again. A static file, so it
  # depends on no resource.
  managed = toset([
    for key in keys(local.settings_list) : key
    if anytrue([for entry in local.managed_entries : endswith(entry, "*") ? startswith(key, trimsuffix(entry, "*")) : key == entry])
  ])
  # The keys this module does not take, each to the reason.
  excluded = merge([
    for entry, reason in local.coverage.excluded : {
      for key in keys(local.settings_list) : key => reason
      if endswith(entry, "*") ? startswith(key, trimsuffix(entry, "*")) : key == entry
    }
  ]...)
}

module "settings" {
  source = "../modules/internal/settings"

  name     = var.name
  settings = var.settings
  secrets  = var.secrets
  managed  = local.managed
  excluded = local.excluded
}

module "secrets" {
  source = "../modules/internal/secrets-gcp-secret-manager"

  project_id = local.project_id
  secrets = merge(
    { MY_VAPID_PRIVATE_KEY = local.secret_ids.vapid },
    # From the input and the settings list, not from module.settings, whose checks read what the sections set: through its
    # output, the sections, which read these secrets' IDs, would depend on themselves. module.settings still refuses a wrong
    # key before the plan, since the app reads its env.
    { for key in var.secrets : local.settings_list[key].env => "${var.name}-${replace(key, "_", "-")}" if contains(keys(local.settings_list), key) },
  )
  generate_p256_keys = ["MY_VAPID_PRIVATE_KEY"]
  # Created empty for you to fill, or written here: the section's module sets the setting from the ID.
  referenced   = { for key in concat(keys(local.to_fill), local.generated) : key => local.secret_ids[key] }
  write_keys   = local.generated
  write_values = var.ingress.kind == "cloudflare" ? { origin_secret = random_password.origin_secret.result } : {}
}

locals {
  # The ID of a secret of module.secrets, through its output so that what reads it waits for the secret.
  secret = { for key, id in local.secret_ids : key => lookup(module.secrets.ids, key, id) }
}

module "kek_secret_manager" {
  source = "../modules/internal/kek-gcp-secret-manager"

  project_id = local.project_id
  secret_id  = local.secret_ids.kek

  lifecycle {
    enabled = var.kek.kind == "secret-manager"
  }
}

module "kek_kms" {
  source = "../modules/internal/kek-gcp-kms"

  project_id = local.project_id
  location   = local.region
  key_ring   = var.name
  key        = local.secret_ids.kek

  lifecycle {
    enabled = var.kek.kind == "kms"
  }
}

# ---------------------------------------------------------------------------------------------------------------------------------
# 3. Network

module "network" {
  source = "../modules/internal/network-gcp-vpc"

  project_id            = local.project_id
  region                = local.region
  name                  = var.name
  subnet_cidr           = var.network.subnet_cidr
  service_connect_cidr  = var.network.service_connect_cidr
  private_services_cidr = var.network.private_services_cidr
  nat                   = var.network.nat

  lifecycle {
    enabled = var.network.kind == "vpc"
  }
}

# ---------------------------------------------------------------------------------------------------------------------------------
# 4. Backends: one block per kind, one enabled per role

module "db_external" {
  source = "../modules/internal/db-external"

  url_secret_id        = local.secret.database_url
  pooled_url_secret_id = var.database.pooled ? local.secret.database_url_pooled : null

  lifecycle {
    enabled = var.database.kind == "external"
  }
}

module "db_cloud_sql" {
  source = "../modules/internal/db-gcp-cloud-sql"

  project_id          = local.project_id
  region              = local.region
  name                = var.name
  url_secret_id       = local.secret_ids.database_url
  tier                = var.database.tier
  disk_size_gb        = var.database.disk_size_gb
  retained_backups    = var.database.retained_backups
  deletion_protection = var.database.deletion_protection
  # With a VPC, a private address; without one, the connector's socket.
  network = local.network

  lifecycle {
    enabled = var.database.kind == "cloud-sql"
  }
}

module "queue_cloud_tasks" {
  source = "../modules/internal/queue-gcp-cloud-tasks"

  project_id                = local.project_id
  region                    = local.region
  service_name              = local.service_name
  name                      = "${var.name}-jobs"
  tick_schedule             = var.queue.tick_schedule
  max_concurrent_dispatches = var.queue.max_concurrent_dispatches
  workers                   = var.workers

  lifecycle {
    enabled = var.queue.kind == "cloud-tasks"
  }
}

module "queue_db" {
  source = "../modules/internal/queue-db"

  workers = var.workers

  lifecycle {
    enabled = var.queue.kind == "db"
  }
}

module "media_gcs" {
  source = "../modules/internal/blob-gcp-gcs"

  project_id              = local.project_id
  bucket_name             = var.media.bucket_name
  location                = coalesce(var.media.location, local.region)
  versioning              = var.media.versioning
  noncurrent_version_days = var.media.noncurrent_version_days

  lifecycle {
    enabled = var.media.kind == "gcs"
  }
}

module "media_r2" {
  source = "../modules/internal/blob-cloudflare-r2"

  account_id                  = coalesce(local.cloudflare.account_id, local.unused.id)
  bucket_name                 = var.media.kind == "r2" ? var.media.bucket_name : local.unused.id
  jurisdiction                = var.media.jurisdiction
  access_key_id_secret_id     = local.secret.media_access_key_id
  secret_access_key_secret_id = local.secret.media_secret_access_key

  lifecycle {
    enabled = var.media.kind == "r2"
  }
}

module "media_s3" {
  source = "../modules/internal/blob-s3"

  endpoint                    = var.media.kind == "s3" ? var.media.endpoint : local.unused.endpoint
  bucket                      = var.media.kind == "s3" ? var.media.bucket_name : local.unused.id
  region                      = var.media.kind == "s3" ? var.media.s3_region : null
  path_style                  = var.media.path_style
  access_key_id_secret_id     = local.secret.media_access_key_id
  secret_access_key_secret_id = local.secret.media_secret_access_key

  lifecycle {
    enabled = var.media.kind == "s3"
  }
}

# kv.kind = db needs no block: it is the server's default.
module "kv_memorystore" {
  source = "../modules/internal/kv-gcp-memorystore"

  project_id    = local.project_id
  region        = local.region
  name          = "${var.name}-valkey"
  network       = var.kv.kind == "memorystore" ? local.network : local.unused.network
  node_type     = var.kv.node_type
  replica_count = var.kv.replica_count

  lifecycle {
    enabled = var.kv.kind == "memorystore"
  }
}

module "kv_external" {
  source = "../modules/internal/kv-external"

  url_secret_id    = local.secret.kv_url
  tls_ca_secret_id = var.kv.tls_ca ? local.secret.kv_tls_ca : null
  key_prefix       = var.kv.key_prefix

  lifecycle {
    enabled = var.kv.kind == "external"
  }
}

module "stream_pubsub" {
  source = "../modules/internal/stream-gcp-pubsub"

  project_id = local.project_id
  name       = "${var.name}-stream"

  lifecycle {
    enabled = var.stream.kind == "pubsub"
  }
}

module "stream_postgres" {
  source = "../modules/internal/stream-postgres"

  lifecycle {
    enabled = var.stream.kind == "postgres"
  }
}

# The Valkey of kv, whose block owns MY_VALKEY_URL and its CA: this one adds the broker alone.
module "stream_valkey" {
  source = "../modules/internal/stream-valkey"

  url_secret_id = null

  lifecycle {
    enabled = var.stream.kind == "valkey"
  }
}

module "mail" {
  source = "../modules/internal/mail-smtp"

  from               = var.mail.from
  host               = var.mail.smtp.host
  port               = var.mail.smtp.port
  username           = var.mail.smtp.username
  password_secret_id = var.mail.smtp.username != null ? local.secret.smtp_password : null
}

# ---------------------------------------------------------------------------------------------------------------------------------
# 5. Ingress: the service's name comes from locals, so no ingress depends on the app, and each is a whole backend.

module "ingress_load_balancer" {
  source = "../modules/internal/ingress-gcp-load-balancer"

  project_id   = local.project_id
  region       = local.region
  host         = var.host
  service_name = local.service_name
  name         = "${var.name}-lb"

  lifecycle {
    enabled = var.ingress.kind == "load-balancer"
  }
}

# The ingress with domain-mapping; with cloudflare, the origin it connects to, which is then no backend.
module "ingress_domain_mapping" {
  source = "../modules/internal/ingress-gcp-domain-mapping"

  project_id   = local.project_id
  region       = local.region
  host         = var.host
  service_name = local.service_name

  lifecycle {
    enabled = local.domain_mapping
  }
}

module "ingress_cloudflare" {
  source = "../modules/internal/ingress-cloudflare-app"

  zone_id             = coalesce(local.cloudflare.zone_id, local.unused.id)
  host                = var.host
  origin_host         = local.cloudflare_origin
  rewrite_host_header = local.cloudflare.host_header_override
  proxied             = local.cloudflare.proxied
  origin_secret       = var.ingress.kind == "cloudflare" ? random_password.origin_secret.result : local.unused.origin_secret
  origin_secret_id    = local.secret.origin_secret
  extra_rules         = try(module.ingress_media.rules, {})

  lifecycle {
    enabled = var.ingress.kind == "cloudflare"
  }
}

module "ingress_media" {
  source = "../modules/internal/ingress-cloudflare-media"

  zone_id             = coalesce(local.cloudflare.zone_id, local.unused.id)
  host                = coalesce(var.ingress.media_host, local.unused.host)
  origin_host         = local.cloudflare_origin
  rewrite_host_header = local.cloudflare.host_header_override
  proxied             = local.cloudflare.proxied
  # Beside the Cloudflare ingress, the zone's rulesets are that module's, and its rule carries the origin secret here too.
  manage_rulesets = var.ingress.kind != "cloudflare"

  lifecycle {
    enabled = var.ingress.media_host != null
  }
}

# The origin of the media host, unless Cloudflare may rewrite the Host header.
module "media_origin" {
  source = "../modules/internal/ingress-gcp-domain-mapping"

  project_id   = local.project_id
  region       = local.region
  host         = coalesce(var.ingress.media_host, local.unused.host)
  service_name = local.service_name

  lifecycle {
    enabled = var.ingress.media_host != null && !local.cloudflare.host_header_override
  }
}

# ---------------------------------------------------------------------------------------------------------------------------------
# 6. Operations

module "billing" {
  source = "../modules/internal/billing-gcp"

  project_id         = local.project_id
  billing_account_id = try(var.billing.billing_account_id, null)
  # Without billing, a method whose module asks nothing more, for the block that is off.
  method           = try(var.billing.method, "budget")
  dataset_location = try(var.billing.dataset_location, "US")
  name             = "${var.name}-billing-budget"

  lifecycle {
    enabled = var.billing != null
  }
}

module "monitoring" {
  source = "../modules/internal/monitoring-gcp"

  project_id                     = local.project_id
  name                           = var.name
  service_name                   = local.service_name
  host                           = var.host
  workers                        = var.workers
  notification_emails            = try(var.monitoring.notification_emails, [])
  notification_channels          = try(var.monitoring.notification_channels, [])
  page_notification_channels     = try(var.monitoring.page_notification_channels, [])
  metrics_exporter               = try(var.monitoring.metrics_exporter, "none")
  uptime_period                  = try(var.monitoring.uptime_period, "900s")
  disabled_alerts                = try(var.monitoring.disabled_alerts, [])
  database_size_limit_bytes      = try(var.monitoring.database_size_limit_bytes, null)
  media_size_limit_bytes         = try(var.monitoring.media_size_limit_bytes, null)
  remote_media_cache_limit_bytes = try(var.monitoring.remote_media_cache_limit_bytes, null)
  billing_sources                = var.billing != null ? module.billing.billing_sources : []

  lifecycle {
    enabled = var.monitoring != null
  }
}

# ---------------------------------------------------------------------------------------------------------------------------------
# 7. The app: every enabled block above, as one list. A disabled block is null and drops out.

locals {
  backends = [
    for b in [
      module.secrets,
      module.kek_secret_manager, module.kek_kms,
      module.db_external, module.db_cloud_sql,
      module.queue_cloud_tasks, module.queue_db,
      module.media_gcs, module.media_r2, module.media_s3,
      module.kv_memorystore, module.kv_external,
      module.stream_pubsub, module.stream_postgres, module.stream_valkey,
      module.mail,
      var.ingress.kind == "domain-mapping" ? module.ingress_domain_mapping : null,
      module.ingress_load_balancer, module.ingress_cloudflare, module.ingress_media,
      module.billing, module.monitoring,
    ] : b if b != null
  ]
}

module "app" {
  source = "../modules/internal/app-gcp-cloud-run"

  project_id      = local.project_id
  region          = local.region
  name            = local.service_name
  image           = var.image
  host            = var.host
  service_ingress = local.service_ingress
  network         = local.network
  # Static egress goes through Cloud NAT, so everything takes the VPC.
  vpc_egress = var.network.kind == "vpc" && var.network.nat ? "ALL_TRAFFIC" : "PRIVATE_RANGES_ONLY"
  workers    = var.workers
  backends   = local.backends

  cpu                     = var.service.cpu
  memory_mib              = var.service.memory_mib
  min_instances           = var.service.min_instances
  max_instances           = var.service.max_instances
  cpu_always_allocated    = var.service.cpu_always_allocated
  concurrency             = var.service.concurrency
  request_timeout_seconds = var.service.request_timeout_seconds
  job_memory_mib          = var.jobs.memory_mib
  job_timeout_seconds     = var.jobs.timeout_seconds
  console_enabled         = var.console_enabled
  deletion_protection     = var.deletion_protection

  env = merge(
    # Cloud Run refuses an HTTP/1 request above 32 MiB, and the server speaks HTTP/1.1.
    { MY_MEDIA_MAX_AV_BYTES = "31457280" },
    module.settings.env,
  )
}
