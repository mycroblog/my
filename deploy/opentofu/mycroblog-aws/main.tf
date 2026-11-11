# SPDX-License-Identifier: AGPL-3.0-only

# One deployment of my on AWS: ECS on Fargate behind an Application Load Balancer, and a backend of each role picked by the kind
# of its section. It composes the modules under ../modules/internal, which resolve inside the same Git package, so a deployment
# pins one ref. NOTHING HERE HAS BEEN APPLIED: modules/internal/app-aws-ecs/README.md says what stays unverified.
#
# The composition, in three steps:
#   1. locals turn the inputs into decisions: which kind, which names, which network;
#   2. every kind is a module block with lifecycle.enabled, so that exactly one of each role is on;
#   3. the enabled blocks are gathered into the app's backends: a reference to a disabled block is null and drops out.
#
# A disabled block still evaluates its inputs against the validations of its module, so each block gets well-formed inputs
# whatever the kind: local.unused holds what a block that is off takes in place of what its section does not have.
#
# The region is the aws provider's. No provider alias: CloudFront's certificate in us-east-1 comes in as an ARN.

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.0, < 7.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.7, < 4.0"
    }
    archive = {
      source  = "hashicorp/archive"
      version = ">= 2.7, < 3.0"
    }
    # Without a Cloudflare kind no resource uses it, and a plan needs no CLOUDFLARE_API_TOKEN.
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = ">= 5.0, < 6.0"
    }
  }
}

data "aws_region" "current" {}

# ---------------------------------------------------------------------------------------------------------------------------------
# 1. Decisions

locals {
  region = data.aws_region.current.region

  # The release of this module, which MODULE_REF beside it names at the tag of a stable release, as v0.2.0, and the image of
  # the same version by default.
  module_release = trimprefix(trimspace(file("${path.module}/../MODULE_REF")), "v")
  version        = coalesce(var.image.version, local.module_release)
  image          = "${var.image.repository}:${local.version}${var.image.digest == null ? "" : "@${var.image.digest}"}"

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

  smtp_username = try(var.mail.smtp.username, null)

  # The secrets only you can fill, by key, with what goes in each. This module creates them empty.
  to_fill = merge(
    var.database.kind == "external" ? { database_url = "The direct connection URL of the database, as postgres://..." } : {},
    var.database.pooled ? { database_url_pooled = "The pooled connection URL of the database, which the service uses." } : {},
    var.mail.kind == "smtp" && local.smtp_username != null ? { smtp_password = "The password of ${local.smtp_username} at ${var.mail.smtp.host}." } : {},
    contains(["r2", "s3-compatible"], var.media.kind) ? {
      media_access_key_id     = "The access key ID of the media bucket ${var.media.bucket_name}."
      media_secret_access_key = "The secret access key of the media bucket ${var.media.bucket_name}."
    } : {},
    var.kv.kind == "external" ? { kv_url = "The URL of the Valkey server, as valkeys://... or rediss://..." } : {},
    var.kv.tls_ca ? { kv_tls_ca = "The PEM of the CA that signs the certificate of the Valkey server." } : {},
  )
  # Created and written by this module: the server checks the origin secret of Cloudflare. CloudFront's is checked by the
  # load balancer, and needs no secret.
  generated = var.ingress.kind == "cloudflare" ? ["origin_secret"] : []

  # The network of the tasks, the load balancer, and the backends on private addresses, in the shape of the network contract.
  network = var.network.kind == "vpc" ? {
    platform            = module.network.platform
    vpc_id              = module.network.vpc_id
    vpc_cidr            = module.network.vpc_cidr
    public_subnet_ids   = module.network.public_subnet_ids
    public_subnet_cidrs = module.network.public_subnet_cidrs
    private_subnet_ids  = module.network.private_subnet_ids
    nat                 = module.network.nat
    } : {
    platform            = "aws"
    vpc_id              = var.network.vpc_id
    vpc_cidr            = var.network.vpc_cidr
    public_subnet_ids   = var.network.public_subnet_ids
    public_subnet_cidrs = var.network.public_subnet_cidrs
    private_subnet_ids  = var.network.private_subnet_ids
    # A VPC of yours: with egress = nat, its private subnets must route through a NAT gateway of yours.
    nat = var.network.egress == "nat"
  }

  # What a block that is off takes where its section has nothing, so that its module's validations pass.
  unused = {
    id            = "unused"
    host          = "unused.invalid"
    endpoint      = "https://unused.invalid"
    origin_secret = "unused-unused-unused-unused-unused"
    zones         = ["unused-a", "unused-b"]
    certificate   = "arn:aws:acm:us-east-1:000000000000:certificate/unused"
    cloudfront_origin = {
      domain_name          = "unused.invalid"
      origin_secret_header = "x-unused"
      idle_timeout         = 60
    }
    network = {
      platform            = "aws"
      vpc_id              = "vpc-unused"
      vpc_cidr            = "10.0.0.0/16"
      public_subnet_ids   = ["subnet-unused-a", "subnet-unused-b"]
      public_subnet_cidrs = ["10.0.0.0/24", "10.0.1.0/24"]
      private_subnet_ids  = ["subnet-unused-c", "subnet-unused-d"]
      nat                 = false
    }
  }
}

# ---------------------------------------------------------------------------------------------------------------------------------
# 2. Generated values and secrets

# CloudFront's header and the load balancer's rule, or Cloudflare's rule, hold the value, so it lands in the state either way;
# generating it here saves a step.
resource "random_password" "origin_secret" {
  length  = 64
  special = false

  lifecycle {
    enabled = contains(["cloudfront", "cloudflare"], var.ingress.kind)
  }
}

locals {
  settings_list = jsondecode(file("${path.module}/../modules/internal/settings/settings.json"))

  # How this module handles each configuration key (coverage.json, which scripts/tofu-coverage.sh checks against the settings
  # list): an entry is a key, or a prefix that ends in *.
  coverage = jsondecode(file("${path.module}/coverage.json"))
  # The sections that are null set nothing, so settings may name their keys: billing_sources for a plugin, for one.
  sections_off = concat(
    var.billing == null ? ["billing"] : [],
    var.monitoring == null ? ["monitoring"] : [],
    var.translation == null ? ["translation"] : [],
    var.search == null ? ["search"] : [],
  )
  # A signal of observability left to the settings sets nothing either: errors_exporter = sentry, for one. Its keys start
  # with the signal's name.
  observability_off = [for signal, destination in var.observability : "${signal}_" if destination == "settings"]
  managed_entries = concat(
    flatten([
      for section, entries in local.coverage.section : [
        for entry in entries : entry if !anytrue([for prefix in local.observability_off : section == "observability" && startswith(entry, prefix)])
      ] if !contains(local.sections_off, section)
    ]),
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
  source = "../modules/internal/secrets-aws-secrets-manager"

  secrets = merge(
    { MY_VAPID_PRIVATE_KEY = local.secret_ids.vapid },
    # From the input and the settings list, not from module.settings, whose checks read what the sections set: through its
    # output, the sections, which read these secrets' ARNs, would depend on themselves. module.settings still refuses a wrong
    # key before the plan, since the app reads its env.
    { for key in var.secrets : local.settings_list[key].env => "${var.name}-${replace(key, "_", "-")}" if contains(keys(local.settings_list), key) },
  )
  generate_p256_keys = ["MY_VAPID_PRIVATE_KEY"]
  # Created empty for you to fill, or written here: the section's module sets the setting from the ARN.
  referenced   = { for key in concat(keys(local.to_fill), local.generated) : key => local.secret_ids[key] }
  write_keys   = local.generated
  write_values = var.ingress.kind == "cloudflare" ? { origin_secret = random_password.origin_secret.result } : {}
}

locals {
  # The ARN of a secret of module.secrets, through its output so that what reads it waits for the secret. A key that is not
  # there is one no enabled block reads, and keeps its name.
  secret = { for key, id in local.secret_ids : key => lookup(module.secrets.ids, key, id) }
}

module "kek_secrets_manager" {
  source = "../modules/internal/kek-aws-secrets-manager"

  secret_name = local.secret_ids.kek

  lifecycle {
    enabled = var.kek.kind == "secrets-manager"
  }
}

module "kek_kms" {
  source = "../modules/internal/kek-aws-kms"

  alias = local.secret_ids.kek

  lifecycle {
    enabled = var.kek.kind == "kms"
  }
}

# ---------------------------------------------------------------------------------------------------------------------------------
# 3. Network

module "network" {
  source = "../modules/internal/network-aws-vpc"

  name               = var.name
  cidr_block         = var.network.cidr_block
  availability_zones = var.network.kind == "vpc" ? coalesce(var.network.availability_zones, local.unused.zones) : local.unused.zones
  nat                = var.network.egress == "nat"

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

module "db_rds" {
  source = "../modules/internal/db-aws-rds"

  name                     = var.name
  network                  = var.database.kind == "rds" ? local.network : local.unused.network
  url_secret_name          = local.secret_ids.database_url
  instance_class           = var.database.instance_class
  allocated_storage_gb     = var.database.allocated_storage_gb
  max_allocated_storage_gb = var.database.max_allocated_storage_gb
  backup_retention_days    = var.database.backup_retention_days
  multi_az                 = var.database.multi_az
  deletion_protection      = var.database.deletion_protection

  lifecycle {
    enabled = var.database.kind == "rds"
  }
}

# The tasks stay up, so they poll the database for the jobs: the queue is always the database's.
module "queue" {
  source = "../modules/internal/queue-db"

  workers = var.workers
}

module "media_s3" {
  source = "../modules/internal/blob-aws-s3"

  bucket_name             = var.media.bucket_name
  versioning              = var.media.versioning
  noncurrent_version_days = var.media.noncurrent_version_days

  lifecycle {
    enabled = var.media.kind == "s3"
  }
}

module "media_r2" {
  source = "../modules/internal/blob-cloudflare-r2"

  account_id                  = coalesce(try(var.cloudflare.account_id, null), local.unused.id)
  bucket_name                 = var.media.kind == "r2" ? var.media.bucket_name : local.unused.id
  jurisdiction                = var.media.jurisdiction
  access_key_id_secret_id     = local.secret.media_access_key_id
  secret_access_key_secret_id = local.secret.media_secret_access_key

  lifecycle {
    enabled = var.media.kind == "r2"
  }
}

module "media_s3_compatible" {
  source = "../modules/internal/blob-s3"

  endpoint                    = var.media.kind == "s3-compatible" ? var.media.endpoint : local.unused.endpoint
  bucket                      = var.media.kind == "s3-compatible" ? var.media.bucket_name : local.unused.id
  region                      = var.media.kind == "s3-compatible" ? var.media.s3_region : null
  path_style                  = var.media.path_style
  access_key_id_secret_id     = local.secret.media_access_key_id
  secret_access_key_secret_id = local.secret.media_secret_access_key

  lifecycle {
    enabled = var.media.kind == "s3-compatible"
  }
}

# kv.kind = db needs no block: it is the server's default.
module "kv_elasticache" {
  source = "../modules/internal/kv-aws-elasticache"

  name    = "${var.name}-valkey"
  network = var.kv.kind == "elasticache" ? local.network : local.unused.network

  lifecycle {
    enabled = var.kv.kind == "elasticache"
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

module "stream_sns" {
  source = "../modules/internal/stream-aws-sns"

  name = "${var.name}-stream"

  lifecycle {
    enabled = var.stream.kind == "sns"
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

module "mail_ses" {
  source = "../modules/internal/mail-aws-ses"

  from              = var.mail.from
  configuration_set = var.mail.configuration_set

  lifecycle {
    enabled = var.mail.kind == "ses"
  }
}

module "mail_smtp" {
  source = "../modules/internal/mail-smtp"

  from               = var.mail.from
  host               = coalesce(try(var.mail.smtp.host, null), local.unused.host)
  port               = coalesce(try(var.mail.smtp.port, null), 587)
  username           = local.smtp_username
  password_secret_id = local.smtp_username != null ? local.secret.smtp_password : null

  lifecycle {
    enabled = var.mail.kind == "smtp"
  }
}

# Services the server calls, which need a grant: null, the default, leaves each to the settings.
module "translation" {
  source = "../modules/internal/translation-aws-translate"

  region = coalesce(try(var.translation.region, null), local.region)

  lifecycle {
    enabled = var.translation != null
  }
}

# A domain you created: the module reads it, grants the tasks' role its HTTP methods, and signs the server's requests.
module "search" {
  source = "../modules/internal/search-aws-opensearch"

  domain_name        = coalesce(try(var.search.domain_name, null), local.unused.id)
  index              = try(var.search.index, null)
  security_group_ids = try(var.search.security_group_ids, [])

  lifecycle {
    enabled = var.search != null
  }
}

# ---------------------------------------------------------------------------------------------------------------------------------
# 5. Ingress: the load balancer always, and what may stand in front of it. On ECS the load balancer reads nothing of the app,
# so no ingress depends on the app.

module "ingress_alb" {
  source = "../modules/internal/ingress-aws-alb"

  host            = var.host
  name            = var.name
  network         = local.network
  certificate_arn = var.ingress.certificate_arn
  # In front of the load balancer, the records of host belong to CloudFront or Cloudflare.
  route53_zone_id = var.ingress.kind == "load-balancer" ? var.ingress.route53_zone_id : null
  host_is_apex    = var.ingress.host_is_apex

  behind_cloudfront = var.ingress.kind == "cloudfront"
  origin_secret     = var.ingress.kind == "cloudfront" ? random_password.origin_secret.result : null
}

# Not a backend: the load balancer above names what the server trusts, and admits CloudFront alone.
module "ingress_cloudfront" {
  source = "../modules/internal/ingress-aws-cloudfront"

  host            = var.host
  name            = var.name
  origin          = var.ingress.kind == "cloudfront" ? module.ingress_alb.cloudfront_origin : local.unused.cloudfront_origin
  origin_secret   = var.ingress.kind == "cloudfront" ? module.ingress_alb.origin_secret : local.unused.origin_secret
  certificate_arn = coalesce(var.ingress.cloudfront_certificate_arn, local.unused.certificate)
  route53_zone_id = var.ingress.route53_zone_id
  host_is_apex    = var.ingress.host_is_apex
  price_class     = var.ingress.cloudfront_price_class

  lifecycle {
    enabled = var.ingress.kind == "cloudfront"
  }
}

# Cloudflare connects to the load balancer, whose certificate is for host.
module "ingress_cloudflare" {
  source = "../modules/internal/ingress-cloudflare-app"

  zone_id          = coalesce(try(var.cloudflare.zone_id, null), local.unused.id)
  host             = var.host
  origin_host      = module.ingress_alb.domain_name
  origin_secret    = var.ingress.kind == "cloudflare" ? random_password.origin_secret.result : local.unused.origin_secret
  origin_secret_id = local.secret.origin_secret
  extra_rules      = try(module.ingress_media.rules, {})

  lifecycle {
    enabled = var.ingress.kind == "cloudflare"
  }
}

module "ingress_media" {
  source = "../modules/internal/ingress-cloudflare-media"

  zone_id     = coalesce(try(var.cloudflare.zone_id, null), local.unused.id)
  host        = coalesce(var.ingress.media_host, local.unused.host)
  origin_host = module.ingress_alb.domain_name
  # Beside the Cloudflare ingress, the zone's rulesets are that module's, and its rule carries the origin secret here too.
  manage_rulesets = var.ingress.kind != "cloudflare"

  lifecycle {
    enabled = var.ingress.media_host != null
  }
}

locals {
  # The load balancer's fields of the backend contract, without its sensitive origin_secret, which would make the whole list of
  # backends sensitive. With Cloudflare in front, the load balancer still connects to the tasks, so one backend says what stands
  # in front of them: the load balancer, with Cloudflare's ranges added to what it trusts. Cloudflare's own block keeps the
  # origin secret.
  ingress_alb = {
    platform        = module.ingress_alb.platform
    env             = module.ingress_alb.env
    secret_env      = module.ingress_alb.secret_env
    permissions     = module.ingress_alb.permissions
    service_ingress = module.ingress_alb.service_ingress
    load_balancer   = module.ingress_alb.load_balancer
    trusted_proxies = concat(
      module.ingress_alb.trusted_proxies,
      var.ingress.kind == "cloudflare" ? module.ingress_cloudflare.trusted_proxies : [],
    )
  }
  ingress_cloudflare = var.ingress.kind == "cloudflare" ? merge(module.ingress_cloudflare, {
    env             = { for name, value in module.ingress_cloudflare.env : name => value if name != "MY_TRUSTED_PROXY_HEADER" }
    trusted_proxies = null
    service_ingress = null
  }) : null
}

# ---------------------------------------------------------------------------------------------------------------------------------
# 6. Operations

# The metrics' namespace is the deployment's name, so that two deployments in one region keep their alarms apart.
module "observability" {
  source = "../modules/internal/observability-aws"

  metrics               = var.observability.metrics
  traces                = var.observability.traces
  errors                = var.observability.errors
  metrics_emf_namespace = var.name
}

module "billing" {
  source = "../modules/internal/billing-aws"

  lifecycle {
    enabled = var.billing != null
  }
}

# The alarms read the metrics in the namespace that observability sets: var.monitoring needs metrics = cloudwatch.
module "monitoring" {
  source = "../modules/internal/monitoring-aws"

  name                           = var.name
  host                           = var.host
  load_balancer_arn_suffix       = module.ingress_alb.arn_suffix
  metrics_emf_namespace          = var.name
  notification_emails            = try(var.monitoring.notification_emails, [])
  notification_topic_arns        = try(var.monitoring.notification_topic_arns, [])
  page_notification_topic_arns   = try(var.monitoring.page_notification_topic_arns, [])
  uptime_period_minutes          = try(var.monitoring.uptime_period_minutes, 15)
  disabled_alerts                = try(var.monitoring.disabled_alerts, [])
  database_size_limit_bytes      = try(var.monitoring.database_size_limit_bytes, null)
  media_size_limit_bytes         = try(var.monitoring.media_size_limit_bytes, null)
  remote_media_cache_limit_bytes = try(var.monitoring.remote_media_cache_limit_bytes, null)

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
      module.kek_secrets_manager, module.kek_kms,
      module.db_external, module.db_rds,
      module.queue,
      module.media_s3, module.media_r2, module.media_s3_compatible,
      module.kv_elasticache, module.kv_external,
      module.stream_sns, module.stream_postgres, module.stream_valkey,
      module.mail_ses, module.mail_smtp,
      local.ingress_alb, local.ingress_cloudflare, module.ingress_media,
      module.translation, module.search,
      module.observability, module.billing, module.monitoring,
    ] : b if b != null
  ]
}

module "app" {
  source = "../modules/internal/app-aws-ecs"

  name  = var.name
  image = local.image
  # migrate.sh at the same release, which runs my upgrade with a new image before the workers and the service take it.
  migration = {
    script                = abspath("${path.module}/../migrate.sh")
    version               = local.version
    allow_destructive_for = var.upgrade.allow_destructive_for
  }
  architecture     = var.architecture
  host             = var.host
  network          = local.network
  assign_public_ip = var.network.egress == "public-ip"
  workers          = var.workers
  backends         = local.backends

  cpu                        = var.service.cpu
  memory_mib                 = var.service.memory_mib
  desired_count              = var.service.desired_count
  health_check_grace_seconds = var.service.health_check_grace_seconds
  job_cpu                    = var.jobs.cpu
  job_memory_mib             = var.jobs.memory_mib
  console_enabled            = var.console_enabled
  log_retention_days         = var.log_retention_days

  env = module.settings.env
}
