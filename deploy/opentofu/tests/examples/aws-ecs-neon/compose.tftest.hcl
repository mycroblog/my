# SPDX-License-Identifier: AGPL-3.0-only

# The example as it is composed, on a mocked provider: nothing here reaches AWS. The kinds of each section are tested in
# deploy/opentofu/mycroblog-aws. It shows that the configuration is consistent, not that it runs: nothing on AWS has been applied.

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

# No mock of cloudflare: the example chooses no Cloudflare kind, so it plans with no CLOUDFLARE_API_TOKEN.

variables {
  host               = "social.example.org"
  certificate_arn    = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc"
  bucket_name        = "social-example-org-media"
  availability_zones = ["ap-northeast-1a", "ap-northeast-1c"]
  egress             = "public-ip"
  mail_from          = "noreply@social.example.org"
  alert_emails       = ["ops@social.example.org"]
}

# By plan alone, as the other examples: prevent_destroy refuses the destroy that follows an apply, as it should.
run "plans_the_recommended_setup" {
  command = plan

  assert {
    condition     = output.version.image == "ghcr.io/mycroblog/my:${trimprefix(trimspace(file("../../MODULE_REF")), "v")}"
    error_message = "the example must run the image of its release: ${jsonencode(output.version)}"
  }

  assert {
    condition     = output.dns_records == [{ name = "social.example.org", type = "CNAME", rrdata = "my-123.ap-northeast-1.elb.amazonaws.com" }] && output.domain_name == "my-123.ap-northeast-1.elb.amazonaws.com"
    error_message = "the example's DNS must point at the load balancer"
  }

  assert {
    condition     = toset(keys(output.secrets_to_fill)) == toset(["my-database-url"])
    error_message = "the example has the database URL to fill, and SES needs no password"
  }

  assert {
    condition     = output.kek.secret == "my-kek"
    error_message = "the example generates the KEK as a secret"
  }
}

run "the_records_go_to_route53" {
  command = plan

  variables {
    route53_zone_id = "Z0123456789ABCDEFGHIJ"
  }

  assert {
    condition     = output.dns_records == []
    error_message = "with a hosted zone, the records of host are created there"
  }
}

run "the_console_can_be_turned_on" {
  command = plan

  variables {
    console_enabled = true
  }

  assert {
    condition     = output.console.platform == "aws-ecs"
    error_message = "console.sh reads the console output"
  }
}
