# SPDX-License-Identifier: AGPL-3.0-only

# The recommended setup, from the fewest inputs, on mocked providers: nothing here reaches AWS. It shows that the configuration
# is consistent, not that it runs: nothing on AWS has been applied.

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

# No mock of cloudflare: without a Cloudflare kind nothing uses the provider, which then plans with no CLOUDFLARE_API_TOKEN.

variables {
  host    = "social.example.org"
  network = { egress = "public-ip", availability_zones = ["ap-northeast-1a", "ap-northeast-1c"] }
  ingress = { certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc" }
  media   = { bucket_name = "social-example-org-media" }
  mail    = { from = "noreply@social.example.org" }

  monitoring = { notification_emails = ["ops@social.example.org"] }
}

run "plans_the_recommended_setup_from_the_fewest_inputs" {
  command = plan

  # The tokens of Easy DKIM are a block of the identity, which a mock cannot fill: the records as SES returns them.
  override_module {
    target = module.mail_ses
    outputs = {
      platform    = "aws"
      env         = { MY_MAIL = "ses", MY_MAIL_FROM = "noreply@social.example.org" }
      secret_env  = {}
      permissions = [{ permission = "ses:SendEmail", resource = "arn:aws:ses:ap-northeast-1:111122223333:identity/social.example.org" }]
      dns_records = [
        { name = "tokena._domainkey.social.example.org", type = "CNAME", rrdata = "tokena.dkim.amazonses.com" },
        { name = "tokenb._domainkey.social.example.org", type = "CNAME", rrdata = "tokenb.dkim.amazonses.com" },
        { name = "tokenc._domainkey.social.example.org", type = "CNAME", rrdata = "tokenc.dkim.amazonses.com" },
      ]
    }
  }

  assert {
    condition = toset(keys(module.app.env)) == toset([
      "MY_BLOB",
      "MY_BLOB_S3_BUCKET",
      "MY_BLOB_S3_REGION",
      "MY_DATABASE",
      "MY_ERRORS_EXPORTER",
      "MY_HOST",
      "MY_MAIL",
      "MY_MAIL_FROM",
      "MY_METRICS_EMF_DIMENSIONS",
      "MY_METRICS_EMF_NAMESPACE",
      "MY_METRICS_EXPORTER",
      "MY_PROFILE",
      "MY_QUEUE",
      "MY_STREAM",
      "MY_STREAM_SNS_QUEUE_PREFIX",
      "MY_STREAM_SNS_TOPIC_ARN",
      "MY_TRACES_EXPORTER",
      "MY_TRUSTED_PROXIES",
      "MY_TRUSTED_PROXY_HEADER",
    ])
    error_message = "the recommended setup does not compose the settings it should: ${jsonencode(keys(module.app.env))}"
  }

  assert {
    condition     = module.app.env["MY_QUEUE"] == "db" && module.app.env["MY_DATABASE"] == "postgres" && module.app.env["MY_MAIL"] == "ses" && module.app.env["MY_STREAM"] == "sns" && module.app.env["MY_BLOB"] == "s3" && module.app.env["MY_METRICS_EXPORTER"] == "emf"
    error_message = "the recommended setup is the database's queue, an external database, SES, SNS, S3, and EMF"
  }

  assert {
    condition     = module.app.env["MY_TRUSTED_PROXIES"] == "10.0.0.0/20, 10.0.16.0/20"
    error_message = "the ranges of the load balancer's subnets are what the server trusts"
  }

  assert {
    condition     = toset(keys(module.app.secret_env)) == toset(["MY_DATABASE_URL", "MY_KEK", "MY_VAPID_PRIVATE_KEY"])
    error_message = "the database URL, the KEK, and the VAPID key must arrive as secrets, and SES needs none"
  }

  assert {
    condition     = toset(keys(output.secrets_to_fill)) == toset(["my-database-url"])
    error_message = "the recommended setup has the database URL to fill, and nothing else"
  }

  assert {
    condition     = output.secrets_to_fill["my-database-url"].command == "aws secretsmanager put-secret-value --region ap-northeast-1 --secret-id my-database-url --secret-string '<value>'"
    error_message = "each secret to fill must come with the command that adds it"
  }

  assert {
    condition = output.dns_records == [
      { name = "social.example.org", type = "CNAME", rrdata = "my-123.ap-northeast-1.elb.amazonaws.com" },
      { name = "tokena._domainkey.social.example.org", type = "CNAME", rrdata = "tokena.dkim.amazonses.com" },
      { name = "tokenb._domainkey.social.example.org", type = "CNAME", rrdata = "tokenb.dkim.amazonses.com" },
      { name = "tokenc._domainkey.social.example.org", type = "CNAME", rrdata = "tokenc.dkim.amazonses.com" },
    ]
    error_message = "DNS must point host at the load balancer and verify the domain of mail.from in SES: ${jsonencode(output.dns_records)}"
  }

  assert {
    condition     = output.kek.secret == "my-kek" && strcontains(output.kek.copy, "--secret-id my-kek")
    error_message = "the KEK is generated as a secret, with the command that copies it"
  }

  assert {
    condition     = !module.network.nat && module.app.task_subnet_ids == tolist(module.network.public_subnet_ids)
    error_message = "egress = public-ip puts the tasks in the public subnets, with no NAT gateway to pay for"
  }

  assert {
    condition     = output.version.version == trimprefix(trimspace(file("../MODULE_REF")), "v") && module.app.images.migration == "ghcr.io/mycroblog/my:${output.version.version}"
    error_message = "the version must default to the release of the module, which MODULE_REF names: ${jsonencode(output.version)}"
  }

  assert {
    condition     = output.console == null && module.ingress_cloudfront == null && module.ingress_cloudflare == null && module.billing == null
    error_message = "the console, CloudFront, Cloudflare, and the billing import are off by default"
  }
}

run "takes_a_version_and_a_digest" {
  command = plan

  variables {
    image = { version = "0.2.0-rc.1", digest = "sha256:${join("", [for i in range(64) : "a"])}" }
  }

  assert {
    condition     = output.version.version == "0.2.0-rc.1" && module.app.images.migration == "ghcr.io/mycroblog/my:0.2.0-rc.1@sha256:${join("", [for i in range(64) : "a"])}"
    error_message = "a version and a digest must make the image: ${jsonencode(module.app.images)}"
  }
}

run "takes_another_repository" {
  command = plan

  variables {
    image = { repository = "111122223333.dkr.ecr.ap-northeast-1.amazonaws.com/my", version = "0.2.0" }
  }

  assert {
    condition     = module.app.images.migration == "111122223333.dkr.ecr.ap-northeast-1.amazonaws.com/my:0.2.0"
    error_message = "the repository must replace the default"
  }
}

run "names_everything_from_name" {
  command = plan

  variables {
    name = "fedi"
  }

  assert {
    condition     = module.app.service_name == "fedi" && module.kek_secrets_manager.secret_env["MY_KEK"] != null && output.kek.secret == "fedi-kek" && contains(keys(output.secrets_to_fill), "fedi-database-url") && module.app.env["MY_STREAM_SNS_QUEUE_PREFIX"] == "fedi-stream"
    error_message = "the service, the secrets, and the stream must take their names from name"
  }
}

run "egress_through_a_nat_gateway" {
  command = plan

  variables {
    network = { egress = "nat", availability_zones = ["ap-northeast-1a", "ap-northeast-1c"] }
  }

  assert {
    condition     = module.network.nat && module.app.task_subnet_ids == tolist(module.network.private_subnet_ids)
    error_message = "egress = nat puts the tasks in the private subnets, behind the NAT gateway the network creates"
  }
}

# The first deployment creates these first, to hold the values before anything reads them.
run "the_secrets_can_be_created_first" {
  command = plan

  plan_options {
    target = [module.secrets, module.kek_secrets_manager]
  }
}
