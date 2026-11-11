# SPDX-License-Identifier: AGPL-3.0-only

# Each kind of each section, on mocked providers: the plan succeeds, the app receives that backend, and secrets_to_fill lists
# exactly what that kind needs. Nothing here reaches AWS or Cloudflare.

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

# ---------------------------------------------------------------------------------------------------------------------------------
# ingress

run "ingress_load_balancer_with_route53" {
  command = plan

  variables {
    ingress = { certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc", route53_zone_id = "Z0123456789ABCDEFGHIJ" }
  }

  assert {
    condition     = output.dns_records == [] && output.dns_note == null
    error_message = "with a hosted zone, the load balancer creates the records of host"
  }
}

run "ingress_load_balancer_at_an_apex" {
  command = plan

  variables {
    ingress = { certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc", host_is_apex = true }
  }

  assert {
    condition     = output.dns_records == [{ name = "social.example.org", type = "ALIAS", rrdata = "my-123.ap-northeast-1.elb.amazonaws.com" }] && output.dns_note != null
    error_message = "an apex without Route 53 needs an ALIAS, which dns_note explains"
  }
}

run "ingress_cloudfront" {
  command = plan

  variables {
    ingress = {
      kind                       = "cloudfront"
      certificate_arn            = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc"
      cloudfront_certificate_arn = "arn:aws:acm:us-east-1:111122223333:certificate/abc"
    }
  }

  assert {
    condition     = module.app.env["MY_TRUSTED_PROXIES"] == "10.0.0.0/20, 10.0.16.0/20, 130.176.0.0/18" && module.app.env["MY_TRUSTED_PROXY_HEADER"] == "x-forwarded-for"
    error_message = "the server must trust the load balancer and CloudFront, which each append to X-Forwarded-For"
  }

  assert {
    condition     = !contains(keys(module.app.env), "MY_ORIGIN_SECRET_HEADER") && !contains(keys(module.app.secret_env), "MY_ORIGIN_SECRET") && !contains(keys(output.secrets_to_fill), "my-origin-secret")
    error_message = "the load balancer checks CloudFront's header, so the server checks none, and nothing is to fill"
  }

  assert {
    condition     = module.ingress_cloudfront != null && output.domain_name == "d111111abcdef8.cloudfront.net" && output.dns_records == [{ name = "social.example.org", type = "CNAME", rrdata = "d111111abcdef8.cloudfront.net" }]
    error_message = "host points at the distribution, not at the load balancer"
  }
}

run "ingress_cloudflare" {
  command = plan

  variables {
    ingress    = { kind = "cloudflare", certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc" }
    cloudflare = { zone_id = "0123456789abcdef0123456789abcdef" }
  }

  assert {
    condition     = module.app.env["MY_TRUSTED_PROXIES"] == "10.0.0.0/20, 10.0.16.0/20, 173.245.48.0/20, 2400:cb00::/32" && module.app.env["MY_TRUSTED_PROXY_HEADER"] == "x-forwarded-for"
    error_message = "the server must trust the load balancer and Cloudflare, which each append to X-Forwarded-For"
  }

  assert {
    condition     = module.app.env["MY_ORIGIN_SECRET_HEADER"] == "x-my-origin-secret" && contains(keys(module.app.secret_env), "MY_ORIGIN_SECRET")
    error_message = "the server must check Cloudflare's origin secret, since the load balancer is open to the internet"
  }

  assert {
    condition     = output.dns_records == [] && !contains(keys(output.secrets_to_fill), "my-origin-secret")
    error_message = "Cloudflare creates the record, and the origin secret is generated, not filled"
  }
}

run "ingress_cloudflare_with_a_media_host" {
  command = plan

  variables {
    ingress    = { kind = "cloudflare", certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc", media_host = "media.example.org" }
    cloudflare = { zone_id = "0123456789abcdef0123456789abcdef" }
  }

  assert {
    condition     = module.app.env["MY_MEDIA_ORIGIN"] == "https://media.example.org" && module.app.env["MY_ORIGIN_SECRET_HEADER"] == "x-my-origin-secret"
    error_message = "the media host sits in the zone of the Cloudflare ingress, whose rule carries the origin secret"
  }
}

run "ingress_media_host_beside_the_load_balancer" {
  command = plan

  variables {
    ingress    = { certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc", media_host = "media.example.org" }
    cloudflare = { zone_id = "0123456789abcdef0123456789abcdef" }
  }

  assert {
    condition     = module.app.env["MY_MEDIA_ORIGIN"] == "https://media.example.org" && !contains(keys(module.app.secret_env), "MY_ORIGIN_SECRET") && module.app.env["MY_TRUSTED_PROXIES"] == "10.0.0.0/20, 10.0.16.0/20"
    error_message = "a media host sits beside the load balancer, with no origin secret and nothing more to trust"
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
    condition     = contains(keys(module.app.secret_env), "MY_DATABASE_URL") && contains(keys(module.app.one_off_secret_env), "MY_DATABASE_URL")
    error_message = "the service takes the pooled URL, and the one-off tasks the direct one"
  }

  assert {
    condition     = toset(keys(output.secrets_to_fill)) == toset(["my-database-url", "my-database-url-pooled"])
    error_message = "both URLs are to fill"
  }
}

run "database_rds" {
  command = plan

  variables {
    database = { kind = "rds" }
  }

  assert {
    condition     = module.app.env["MY_DATABASE"] == "postgres" && contains(keys(module.app.secret_env), "MY_DATABASE_URL") && module.db_external == null && module.db_rds != null
    error_message = "RDS must give the app its URL"
  }

  assert {
    condition     = length(module.db_rds.security_group_ids) > 0 && length(setsubtract(module.db_rds.security_group_ids, module.app.task_security_group_ids)) == 0
    error_message = "the tasks must join the group that RDS admits"
  }

  assert {
    condition     = output.secrets_to_fill == {}
    error_message = "RDS writes its own URL: there is nothing to fill for it"
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
    condition     = module.app.env["MY_BLOB"] == "s3" && module.app.env["MY_BLOB_S3_ENDPOINT"] == "https://0123456789abcdef0123456789abcdef.r2.cloudflarestorage.com" && module.media_s3 == null
    error_message = "R2 must reach the app as an S3 store"
  }

  assert {
    condition     = toset(keys(output.secrets_to_fill)) == toset(["my-database-url", "my-media-access-key-id", "my-media-secret-access-key"])
    error_message = "R2's access key is to fill"
  }
}

run "media_s3_compatible" {
  command = plan

  variables {
    media = { kind = "s3-compatible", bucket_name = "my-media", endpoint = "https://s3.example.org", s3_region = "garage" }
  }

  assert {
    condition     = module.app.env["MY_BLOB"] == "s3" && module.app.env["MY_BLOB_S3_ENDPOINT"] == "https://s3.example.org" && contains(keys(module.app.secret_env), "MY_BLOB_S3_ACCESS_KEY_ID")
    error_message = "an S3-compatible store must reach the app with its keys"
  }
}

# ---------------------------------------------------------------------------------------------------------------------------------
# kv and stream

run "kv_elasticache_with_the_valkey_stream" {
  command = plan

  variables {
    kv     = { kind = "elasticache" }
    stream = { kind = "valkey" }
  }

  assert {
    condition     = module.app.env["MY_KV"] == "valkey" && module.app.env["MY_VALKEY_URL"] == "valkeys://my-valkey-abc.serverless.apne1.cache.amazonaws.com:6379" && module.app.env["MY_STREAM"] == "valkey" && module.stream_sns == null
    error_message = "ElastiCache must give the app its URL, and the stream must use it"
  }

  assert {
    condition     = toset(keys(output.secrets_to_fill)) == toset(["my-database-url"])
    error_message = "ElastiCache needs nothing to fill"
  }
}

run "kv_external_with_its_ca" {
  command = plan

  variables {
    kv = { kind = "external", tls_ca = true }
  }

  assert {
    condition     = module.app.env["MY_KV"] == "valkey" && contains(keys(module.app.secret_env), "MY_VALKEY_URL") && contains([for name in keys(module.app.secret_env) : "${name}_FILE"], "MY_VALKEY_TLS_CA_FILE") && module.app.env["MY_STREAM"] == "sns"
    error_message = "an external Valkey must reach the app by its secrets"
  }

  assert {
    condition     = toset(keys(output.secrets_to_fill)) == toset(["my-database-url", "my-valkey-url", "my-valkey-ca"])
    error_message = "the Valkey URL and its CA are to fill"
  }
}

run "stream_postgres" {
  command = plan

  variables {
    stream = { kind = "postgres" }
  }

  assert {
    condition     = module.app.env["MY_STREAM"] == "postgres" && module.stream_sns == null
    error_message = "stream = postgres must reach the app"
  }
}

# ---------------------------------------------------------------------------------------------------------------------------------
# mail, kek, network

run "mail_ses_with_a_configuration_set" {
  command = plan

  variables {
    mail = { from = "noreply@social.example.org", configuration_set = "my-events" }
  }

  assert {
    condition     = module.app.env["MY_MAIL"] == "ses" && module.app.env["MY_MAIL_SES_CONFIGURATION_SET"] == "my-events" && module.mail_smtp == null
    error_message = "the configuration set must reach the app"
  }
}

run "mail_smtp" {
  command = plan

  variables {
    mail = { kind = "smtp", from = "noreply@social.example.org", smtp = { host = "smtp.example.org", username = "my-smtp-user" } }
  }

  assert {
    condition     = module.app.env["MY_MAIL"] == "smtp" && module.app.env["MY_MAIL_SMTP_PORT"] == "587" && contains(keys(module.app.secret_env), "MY_MAIL_SMTP_PASSWORD") && module.mail_ses == null
    error_message = "SMTP must reach the app with its password"
  }

  assert {
    condition     = toset(keys(output.secrets_to_fill)) == toset(["my-database-url", "my-smtp-password"]) && output.secrets_to_fill["my-smtp-password"].what == "The password of my-smtp-user at smtp.example.org."
    error_message = "the SMTP password is to fill"
  }

  assert {
    condition     = output.dns_records == [{ name = "social.example.org", type = "CNAME", rrdata = "my-123.ap-northeast-1.elb.amazonaws.com" }]
    error_message = "without SES there is no domain to verify"
  }
}

run "mail_smtp_without_a_username" {
  command = plan

  variables {
    mail = { kind = "smtp", from = "noreply@social.example.org", smtp = { host = "relay.example.org", port = 25 } }
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
    condition     = module.app.env["MY_KEY_MANAGER"] == "awskms" && !contains(keys(module.app.secret_env), "MY_KEK") && output.kek.kind == "kms" && output.kek.copy == null
    error_message = "the KEK must come from AWS KMS, which has nothing to copy"
  }
}

run "network_existing" {
  command = plan

  variables {
    network = {
      kind                = "existing"
      egress              = "nat"
      vpc_id              = "vpc-mine"
      vpc_cidr            = "10.30.0.0/16"
      public_subnet_ids   = ["subnet-pa", "subnet-pb"]
      public_subnet_cidrs = ["10.30.0.0/24", "10.30.1.0/24"]
      private_subnet_ids  = ["subnet-qa", "subnet-qb"]
    }
    database = { kind = "rds" }
  }

  assert {
    condition     = module.network == null && module.app.task_subnet_ids == tolist(["subnet-qa", "subnet-qb"]) && module.app.env["MY_TRUSTED_PROXIES"] == "10.30.0.0/24, 10.30.1.0/24"
    error_message = "a VPC of yours must reach the app, the load balancer, and the backends"
  }
}

# ---------------------------------------------------------------------------------------------------------------------------------
# operations, settings, and secrets

run "monitoring_through_a_topic_of_yours" {
  command = plan

  variables {
    monitoring = { notification_topic_arns = ["arn:aws:sns:ap-northeast-1:111122223333:ops"], uptime_period_minutes = 60 }
  }

  assert {
    condition     = module.monitoring != null && module.app.env["MY_METRICS_EXPORTER"] == "emf"
    error_message = "monitoring must reach the app"
  }
}

run "no_monitoring" {
  command = plan

  variables {
    monitoring = null
  }

  assert {
    condition     = module.monitoring == null && !contains(keys(module.app.env), "MY_METRICS_EMF_DIMENSIONS") && module.app.env["MY_METRICS_EXPORTER"] == "emf"
    error_message = "monitoring = null creates no canary and no alarm, and the metrics still go to CloudWatch"
  }
}

run "observability_to_cloudwatch" {
  command = plan

  variables {
    name = "social"
  }

  assert {
    condition = (
      module.app.env["MY_METRICS_EXPORTER"] == "emf" && module.app.env["MY_METRICS_EMF_NAMESPACE"] == "social" &&
      module.app.env["MY_TRACES_EXPORTER"] == "none" && module.app.env["MY_ERRORS_EXPORTER"] == "none" &&
      length(module.observability.permissions) == 0
    )
    error_message = "the metrics must go to CloudWatch in the deployment's namespace, which the alarms read: ${jsonencode(module.app.env)}"
  }
}

run "errors_to_sentry_through_the_settings" {
  command = plan

  variables {
    observability = { errors = "settings" }
    settings      = { errors_exporter = "sentry" }
    secrets       = ["errors_sentry_dsn"]
  }

  assert {
    condition     = module.app.env["MY_ERRORS_EXPORTER"] == "sentry" && contains(keys(module.app.secret_env), "MY_ERRORS_SENTRY_DSN") && module.app.env["MY_METRICS_EXPORTER"] == "emf"
    error_message = "the settings must carry the exporter that observability leaves to them"
  }
}

run "translation_by_amazon_translate" {
  command = plan

  variables {
    translation = {}
  }

  assert {
    condition = (
      module.app.env["MY_TRANSLATOR_AWS_REGION"] == "ap-northeast-1" &&
      toset([for p in module.translation.permissions : p.permission]) == toset(["translate:TranslateText", "comprehend:DetectDominantLanguage"])
    )
    error_message = "the role must be allowed Amazon Translate, in the deployment's region by default"
  }
}

run "search_on_a_domain_of_yours" {
  command = plan

  variables {
    search = { domain_name = "my-search" }
  }

  assert {
    condition = (
      module.app.env["MY_SEARCH"] == "opensearch" && module.app.env["MY_SEARCH_OPENSEARCH_AWS_SIGNING"] == "es" &&
      module.app.env["MY_SEARCH_OPENSEARCH_URL"] == "https://search-my-search-abc123.ap-northeast-1.es.amazonaws.com" &&
      alltrue([for p in module.search.permissions : startswith(p.permission, "es:ESHttp") && p.resource == "arn:aws:es:ap-northeast-1:111122223333:domain/my-search/*"])
    )
    error_message = "the server must sign its requests to the domain, which the role may call"
  }
}

run "no_translation_and_no_search" {
  command = plan

  assert {
    condition     = module.translation == null && module.search == null && !contains(keys(module.app.env), "MY_SEARCH")
    error_message = "translation and search are null by default, and grant nothing"
  }
}

run "billing" {
  command = plan

  variables {
    billing = {}
  }

  assert {
    condition     = module.app.env["MY_BILLING_SOURCES"] == "aws"
    error_message = "billing must reach the app"
  }
}

run "workers_reach_the_service_and_the_queue" {
  command = plan

  variables {
    workers = { delivery = { job_types = ["federation.fanout", "federation.deliver"], instances = 2, concurrency = 3 } }
  }

  assert {
    condition     = module.app.worker_services == { delivery = "my-delivery" } && output.upgrade.worker_services == { delivery = "my-delivery" }
    error_message = "one workers value must build the worker's service"
  }
}

run "settings_and_secrets_reach_the_server" {
  command = plan

  variables {
    settings = { translator_deepl_url = "https://api.deepl.com" }
    secrets  = ["translator_deepl_api_key"]
  }

  assert {
    condition     = module.app.env["MY_TRANSLATOR_DEEPL_URL"] == "https://api.deepl.com" && contains(keys(module.app.secret_env), "MY_TRANSLATOR_DEEPL_API_KEY")
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
    condition     = output.console.platform == "aws-ecs" && output.console.region == "ap-northeast-1"
    error_message = "the console must be there to pipe to console.sh"
  }
}
