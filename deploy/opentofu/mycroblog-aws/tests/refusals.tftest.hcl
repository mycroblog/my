# SPDX-License-Identifier: AGPL-3.0-only

# What does not fit together fails before the plan, on the input whose message names the others.

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

mock_provider "random" {}

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
  network = { egress = "public-ip", availability_zones = ["ap-northeast-1a", "ap-northeast-1c"] }
  ingress = { certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc" }
  media   = { bucket_name = "social-example-org-media" }
  mail    = { from = "noreply@social.example.org" }

  monitoring = { notification_emails = ["ops@social.example.org"] }
}

run "the_latest_tag" {
  command = plan

  variables {
    image = { version = "latest" }
  }

  expect_failures = [var.image]
}

run "a_version_with_the_v_of_the_release" {
  command = plan

  variables {
    image = { version = "v0.2.0" }
  }

  expect_failures = [var.image]
}

run "a_digest_without_its_algorithm" {
  command = plan

  variables {
    image = { version = "nightly", digest = "0123456789abcdef" }
  }

  expect_failures = [var.image]
}

run "a_name_that_cannot_name_things" {
  command = plan

  variables {
    name = "My_Instance"
  }

  expect_failures = [var.name]
}

run "an_unknown_architecture" {
  command = plan

  variables {
    architecture = "riscv64"
  }

  expect_failures = [var.architecture]
}

run "an_unknown_kind_of_network" {
  command = plan

  variables {
    network = { kind = "default", egress = "public-ip" }
  }

  expect_failures = [var.network]
}

run "an_unknown_way_out" {
  command = plan

  variables {
    network = { egress = "ipv6", availability_zones = ["ap-northeast-1a", "ap-northeast-1c"] }
  }

  expect_failures = [var.network]
}

run "a_vpc_in_one_zone" {
  command = plan

  variables {
    network = { egress = "public-ip", availability_zones = ["ap-northeast-1a"] }
  }

  expect_failures = [var.network]
}

run "a_vpc_without_zones" {
  command = plan

  variables {
    network = { egress = "public-ip" }
  }

  expect_failures = [var.network]
}

run "an_existing_network_without_its_subnets" {
  command = plan

  variables {
    network = { kind = "existing", egress = "nat", vpc_id = "vpc-mine" }
  }

  expect_failures = [var.network]
}

run "zones_for_an_existing_network" {
  command = plan

  variables {
    network = { kind = "existing", egress = "nat", vpc_id = "vpc-mine", vpc_cidr = "10.30.0.0/16", public_subnet_ids = ["a", "b"], public_subnet_cidrs = ["10.30.0.0/24", "10.30.1.0/24"], private_subnet_ids = ["c", "d"], availability_zones = ["ap-northeast-1a", "ap-northeast-1c"] }
  }

  expect_failures = [var.network]
}

run "an_unknown_kind_of_ingress" {
  command = plan

  variables {
    ingress = { kind = "nginx", certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc" }
  }

  expect_failures = [var.ingress]
}

run "cloudfront_without_its_certificate" {
  command = plan

  variables {
    ingress = { kind = "cloudfront", certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc" }
  }

  expect_failures = [var.ingress]
}

run "a_cloudfront_certificate_without_cloudfront" {
  command = plan

  variables {
    ingress = { certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc", cloudfront_certificate_arn = "arn:aws:acm:us-east-1:111122223333:certificate/abc" }
  }

  expect_failures = [var.ingress]
}

run "cloudflare_ingress_without_cloudflare" {
  command = plan

  variables {
    ingress = { kind = "cloudflare", certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc" }
  }

  expect_failures = [var.ingress]
}

run "a_media_host_without_cloudflare" {
  command = plan

  variables {
    ingress = { certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc", media_host = "media.example.org" }
  }

  expect_failures = [var.ingress]
}

run "a_media_host_that_is_the_host" {
  command = plan

  variables {
    ingress    = { certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc", media_host = "social.example.org" }
    cloudflare = { zone_id = "0123456789abcdef0123456789abcdef" }
  }

  expect_failures = [var.ingress]
}

run "a_media_host_beside_cloudfront" {
  command = plan

  variables {
    ingress    = { kind = "cloudfront", certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc", cloudfront_certificate_arn = "arn:aws:acm:us-east-1:111122223333:certificate/abc", media_host = "media.example.org" }
    cloudflare = { zone_id = "0123456789abcdef0123456789abcdef" }
  }

  expect_failures = [var.ingress]
}

run "route53_beside_cloudflare" {
  command = plan

  variables {
    ingress    = { kind = "cloudflare", certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc", route53_zone_id = "Z0123456789ABCDEFGHIJ" }
    cloudflare = { zone_id = "0123456789abcdef0123456789abcdef" }
  }

  expect_failures = [var.ingress]
}

run "an_unknown_kind_of_database" {
  command = plan

  variables {
    database = { kind = "aurora" }
  }

  expect_failures = [var.database]
}

run "a_pooled_rds" {
  command = plan

  variables {
    database = { kind = "rds", pooled = true }
  }

  expect_failures = [var.database]
}

run "an_unknown_kind_of_media" {
  command = plan

  variables {
    media = { kind = "local", bucket_name = "b" }
  }

  expect_failures = [var.media]
}

run "s3_compatible_without_an_endpoint" {
  command = plan

  variables {
    media = { kind = "s3-compatible", bucket_name = "my-media" }
  }

  expect_failures = [var.media]
}

run "r2_without_cloudflare" {
  command = plan

  variables {
    media = { kind = "r2", bucket_name = "my-media" }
  }

  expect_failures = [var.media]
}

run "an_unknown_kind_of_kv" {
  command = plan

  variables {
    kv = { kind = "redis" }
  }

  expect_failures = [var.kv]
}

run "the_fields_of_an_external_valkey_on_another_kind" {
  command = plan

  variables {
    kv = { tls_ca = true }
  }

  expect_failures = [var.kv]
}

run "an_unknown_kind_of_stream" {
  command = plan

  variables {
    stream = { kind = "hub" }
  }

  expect_failures = [var.stream]
}

run "the_valkey_stream_without_a_valkey" {
  command = plan

  variables {
    stream = { kind = "valkey" }
  }

  expect_failures = [var.stream]
}

run "an_unknown_kind_of_mail" {
  command = plan

  variables {
    mail = { kind = "sendmail", from = "noreply@social.example.org" }
  }

  expect_failures = [var.mail]
}

run "smtp_without_its_server" {
  command = plan

  variables {
    mail = { kind = "smtp", from = "noreply@social.example.org" }
  }

  expect_failures = [var.mail]
}

run "an_smtp_server_beside_ses" {
  command = plan

  variables {
    mail = { from = "noreply@social.example.org", smtp = { host = "smtp.example.org" } }
  }

  expect_failures = [var.mail]
}

run "a_configuration_set_beside_smtp" {
  command = plan

  variables {
    mail = { kind = "smtp", from = "noreply@social.example.org", configuration_set = "my-events", smtp = { host = "smtp.example.org" } }
  }

  expect_failures = [var.mail]
}

run "an_unknown_kind_of_kek" {
  command = plan

  variables {
    kek = { kind = "local" }
  }

  expect_failures = [var.kek]
}

run "monitoring_that_notifies_nobody" {
  command = plan

  variables {
    monitoring = {}
  }

  expect_failures = [var.monitoring]
}
