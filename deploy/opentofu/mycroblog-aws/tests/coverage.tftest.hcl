# SPDX-License-Identifier: AGPL-3.0-only

# The wiring of coverage.json: each key that it puts in a section or fixes, planned on mocked providers with the kind that
# sets it, reaches the environment of the tasks by its MY_ name, as a value or a secret. scripts/tofu-coverage.sh fails while
# this file does not name the MY_ name of such a key.

mock_provider "aws" {
  mock_data "aws_region" {
    defaults = {
      region = "ap-northeast-1"
    }
  }
  mock_data "aws_caller_identity" {
    defaults = {
      account_id = "111122223333"
    }
  }
  mock_data "aws_partition" {
    defaults = {
      partition = "aws"
    }
  }
  mock_data "aws_iam_policy_document" {
    defaults = {
      json          = "{}"
      minified_json = "{}"
    }
  }
  mock_data "aws_ec2_managed_prefix_list" {
    defaults = {
      id      = "pl-cloudfront"
      entries = [{ cidr = "130.176.0.0/18", description = "" }]
    }
  }
  mock_resource "aws_secretsmanager_secret" {
    defaults = {
      arn = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:s-AbCdEf"
    }
  }
  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::111122223333:role/my"
    }
  }
  mock_resource "aws_cloudwatch_log_group" {
    defaults = {
      arn = "arn:aws:logs:ap-northeast-1:111122223333:log-group:/ecs/my"
    }
  }
  mock_resource "aws_ecs_cluster" {
    defaults = {
      arn = "arn:aws:ecs:ap-northeast-1:111122223333:cluster/my"
    }
  }
  mock_resource "aws_ecs_task_definition" {
    defaults = {
      arn = "arn:aws:ecs:ap-northeast-1:111122223333:task-definition/my:1"
    }
  }
  mock_resource "aws_s3_bucket" {
    defaults = {
      arn = "arn:aws:s3:::social-example-org-media"
    }
  }
  mock_resource "aws_sns_topic" {
    defaults = {
      arn = "arn:aws:sns:ap-northeast-1:111122223333:my-stream"
    }
  }
  mock_resource "aws_kms_key" {
    defaults = {
      arn = "arn:aws:kms:ap-northeast-1:111122223333:key/abc"
    }
  }
  mock_data "aws_opensearch_domain" {
    defaults = {
      arn         = "arn:aws:es:ap-northeast-1:111122223333:domain/my-search"
      endpoint    = "search-my-search-abc123.ap-northeast-1.es.amazonaws.com"
      vpc_options = []
    }
  }
  mock_resource "aws_lb" {
    defaults = {
      arn        = "arn:aws:elasticloadbalancing:ap-northeast-1:111122223333:loadbalancer/app/my/abc"
      arn_suffix = "app/my/abc"
      dns_name   = "my-123.ap-northeast-1.elb.amazonaws.com"
      zone_id    = "Z14GRHDCWA56QT"
    }
  }
  mock_resource "aws_lb_listener" {
    defaults = {
      arn = "arn:aws:elasticloadbalancing:ap-northeast-1:111122223333:listener/app/my/abc/def"
    }
  }
  mock_resource "aws_lb_target_group" {
    defaults = {
      arn = "arn:aws:elasticloadbalancing:ap-northeast-1:111122223333:targetgroup/my/abc"
    }
  }
  mock_resource "aws_cloudfront_distribution" {
    defaults = {
      domain_name    = "d111111abcdef8.cloudfront.net"
      hosted_zone_id = "Z2FDTNDAQYW2N"
    }
  }
  mock_resource "aws_elasticache_serverless_cache" {
    defaults = {
      endpoint = [{ address = "my-valkey-abc.serverless.apne1.cache.amazonaws.com", port = 6379 }]
    }
  }
  mock_resource "aws_sesv2_email_identity" {
    defaults = {
      arn = "arn:aws:ses:ap-northeast-1:111122223333:identity/social.example.org"
    }
  }
  mock_resource "aws_sesv2_configuration_set" {
    defaults = {
      arn = "arn:aws:ses:ap-northeast-1:111122223333:configuration-set/my-events"
    }
  }
}

mock_provider "random" {
  mock_resource "random_password" {
    defaults = {
      result = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    }
  }
}

mock_provider "archive" {}

mock_provider "cloudflare" {
  mock_data "cloudflare_ip_ranges" {
    defaults = {
      ipv4_cidrs = ["173.245.48.0/20"]
      ipv6_cidrs = ["2400:cb00::/32"]
    }
  }
}

variables {
  host    = "social.example.org"
  image   = "ghcr.io/mycroblog/my:0.1.0"
  network = { egress = "public-ip", availability_zones = ["ap-northeast-1a", "ap-northeast-1c"] }
  ingress = { certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc" }
  media   = { bucket_name = "social-example-org-media" }
  mail    = { from = "noreply@social.example.org" }

  monitoring = { notification_emails = ["ops@social.example.org"] }
}

# Each assertion lists the names that must be in the service's environment, as a setting or a secret. Fargate mounts no file,
# so a setting whose name ends in _FILE arrives by the name without it.

run "the_recommended_setup" {
  command = plan

  variables {
    billing = {}
    workers = { delivery = { job_types = ["federation.*"], concurrency = 8 } }
  }

  assert {
    condition = length(setsubtract([
      "MY_HOST", "MY_PROFILE", "MY_VAPID_PRIVATE_KEY", "MY_TRUSTED_PROXIES", "MY_TRUSTED_PROXY_HEADER",
      "MY_DATABASE", "MY_DATABASE_URL",
      "MY_QUEUE", "MY_WORKER_JOB_TYPES_EXCEPT",
      "MY_BLOB", "MY_BLOB_S3_BUCKET", "MY_BLOB_S3_REGION",
      "MY_STREAM", "MY_STREAM_SNS_TOPIC_ARN", "MY_STREAM_SNS_QUEUE_PREFIX",
      "MY_MAIL", "MY_MAIL_FROM",
      "MY_KEK",
      "MY_BILLING_SOURCES",
      "MY_METRICS_EXPORTER", "MY_METRICS_EMF_NAMESPACE", "MY_METRICS_EMF_DIMENSIONS", "MY_TRACES_EXPORTER", "MY_ERRORS_EXPORTER",
    ], concat(keys(module.app.env), keys(module.app.secret_env)))) == 0
    error_message = "the recommended setup misses a setting of its sections: ${jsonencode(module.app.env)} ${jsonencode(module.app.secret_env)}"
  }

  assert {
    condition     = length(setsubtract(["MY_WORKER_JOB_TYPES", "MY_WORKER_JOB_CONCURRENCY"], keys(module.app.worker_env["delivery"]))) == 0
    error_message = "the worker must take its job types: ${jsonencode(module.app.worker_env)}"
  }
}

run "mail_ses_with_a_configuration_set" {
  command = plan

  variables {
    mail = { from = "noreply@social.example.org", configuration_set = "my-events" }
  }

  assert {
    condition     = length(setsubtract(["MY_MAIL", "MY_MAIL_FROM", "MY_MAIL_SES_CONFIGURATION_SET"], keys(module.app.env))) == 0
    error_message = "SES must reach the service: ${jsonencode(module.app.env)}"
  }
}

run "mail_smtp" {
  command = plan

  variables {
    mail = { kind = "smtp", from = "noreply@social.example.org", smtp = { host = "smtp.example.org", username = "my-smtp-user" } }
  }

  assert {
    condition = length(setsubtract([
      "MY_MAIL", "MY_MAIL_FROM", "MY_MAIL_SMTP_HOST", "MY_MAIL_SMTP_PORT", "MY_MAIL_SMTP_USERNAME", "MY_MAIL_SMTP_PASSWORD",
    ], concat(keys(module.app.env), keys(module.app.secret_env)))) == 0
    error_message = "SMTP must reach the service: ${jsonencode(module.app.env)} ${jsonencode(module.app.secret_env)}"
  }
}

run "ingress_cloudflare_with_a_media_host" {
  command = plan

  variables {
    ingress    = { kind = "cloudflare", certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc", media_host = "media.example.org" }
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

run "media_s3_compatible" {
  command = plan

  variables {
    media = { kind = "s3-compatible", bucket_name = "my-media", endpoint = "https://s3.example.org", s3_region = "garage", path_style = true }
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
    condition = length(setsubtract(
      [for name in ["MY_KV", "MY_VALKEY_URL", "MY_VALKEY_TLS_CA_FILE", "MY_VALKEY_KEY_PREFIX"] : trimsuffix(name, "_FILE")],
      concat(keys(module.app.env), keys(module.app.secret_env)),
    )) == 0
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
    error_message = "AWS KMS must reach the service: ${jsonencode(module.app.env)}"
  }
}

# MY_ROLE is set by the app module in each task definition, apart from env, so this run plans that module alone, where its
# task definitions can be read.
run "the_role_of_each_task" {
  command = plan

  module {
    source = "../modules/internal/app-aws-ecs"
  }

  providers = {
    aws = aws
  }

  variables {
    network = {
      platform            = "aws"
      vpc_id              = "vpc-1"
      vpc_cidr            = "10.0.0.0/16"
      public_subnet_ids   = ["subnet-a", "subnet-b"]
      public_subnet_cidrs = ["10.0.0.0/24", "10.0.1.0/24"]
    }
    assign_public_ip = true
    workers          = { delivery = { job_types = ["federation.*"] } }
    backends = [
      { platform = "neutral", env = { MY_DATABASE = "postgres", MY_BLOB = "s3", MY_QUEUE = "db", MY_STREAM = "postgres" }, workers = { delivery = ["federation.*"] } },
      { platform = "aws", secret_env = { MY_KEK = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:kek" } },
    ]
  }

  assert {
    condition = (
      strcontains(aws_ecs_task_definition.task["server"].container_definitions, jsonencode({ name = "MY_ROLE", value = "all" })) &&
      strcontains(aws_ecs_task_definition.task["worker-delivery"].container_definitions, jsonencode({ name = "MY_ROLE", value = "worker" }))
    )
    error_message = "the service must run every role, and a worker the worker's"
  }
}

run "translation_by_amazon_translate" {
  command = plan

  variables {
    translation = { region = "us-east-1" }
  }

  assert {
    condition     = module.app.env["MY_TRANSLATOR_AWS_REGION"] == "us-east-1"
    error_message = "Amazon Translate must reach the tasks: ${jsonencode(module.app.env)}"
  }
}

run "search_on_amazon_opensearch_service" {
  command = plan

  variables {
    search = { domain_name = "my-search", index = "social-statuses" }
  }

  assert {
    condition = length(setsubtract([
      "MY_SEARCH", "MY_SEARCH_OPENSEARCH_URL", "MY_SEARCH_OPENSEARCH_INDEX", "MY_SEARCH_OPENSEARCH_AWS_SIGNING",
      "MY_SEARCH_OPENSEARCH_AWS_REGION",
    ], keys(module.app.env))) == 0
    error_message = "the domain must reach the tasks: ${jsonencode(module.app.env)}"
  }
}

# The classification feeds the settings input, which refuses what a section sets, the module fixes, or the module does not
# take (the tests of modules/internal/settings: a test cannot expect the failure of a variable of a module it calls). A section
# that is off sets nothing, so its keys are free.

run "settings_may_set_what_a_section_that_is_off_would" {
  command = plan

  variables {
    monitoring = null
    settings = {
      billing_sources        = "cloudflare", metrics_emf_dimensions = "my.outcome", search = "meilisearch",
      search_meilisearch_url = "https://search.example.org", translator_aws_region = "us-east-1",
    }
  }

  assert {
    condition = (
      module.app.env["MY_BILLING_SOURCES"] == "cloudflare" && module.app.env["MY_METRICS_EMF_DIMENSIONS"] == "my.outcome" &&
      module.app.env["MY_SEARCH"] == "meilisearch" && module.app.env["MY_TRANSLATOR_AWS_REGION"] == "us-east-1"
    )
    error_message = "without billing, monitoring, search, and translation, settings may name their keys"
  }
}

run "settings_may_set_a_signal_that_observability_leaves_to_them" {
  command = plan

  variables {
    monitoring    = null
    observability = { metrics = "settings", errors = "settings" }
    settings      = { metrics_exporter = "otlp", errors_exporter = "sentry" }
    secrets       = ["errors_sentry_dsn"]
  }

  assert {
    condition = (
      module.app.env["MY_METRICS_EXPORTER"] == "otlp" && module.app.env["MY_ERRORS_EXPORTER"] == "sentry" &&
      module.app.env["MY_TRACES_EXPORTER"] == "none" && !contains(keys(module.app.env), "MY_METRICS_EMF_NAMESPACE") &&
      contains(keys(module.app.secret_env), "MY_ERRORS_SENTRY_DSN")
    )
    error_message = "a signal left to the settings takes them, and the others stay the section's: ${jsonencode(module.app.env)}"
  }
}
