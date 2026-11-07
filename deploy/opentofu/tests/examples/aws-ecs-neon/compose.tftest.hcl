# SPDX-License-Identifier: AGPL-3.0-only

# The example as it is composed, and every swap of a block that holds, on a mocked provider: nothing here reaches AWS. It shows
# that the configuration is consistent, not that it runs: read deploy/opentofu/modules/app-aws-ecs/README.md.

mock_provider "aws" {
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
  mock_data "aws_region" {
    defaults = {
      region = "ap-northeast-1"
    }
  }
  mock_data "aws_iam_policy_document" {
    defaults = {
      json          = "{}"
      minified_json = "{}"
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
  mock_resource "aws_secretsmanager_secret" {
    defaults = {
      arn = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:s-AbCdEf"
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
  mock_resource "aws_sesv2_email_identity" {
    defaults = {
      arn = "arn:aws:ses:ap-northeast-1:111122223333:identity/social.example.org"
    }
  }
  mock_resource "aws_kms_key" {
    defaults = {
      arn = "arn:aws:kms:ap-northeast-1:111122223333:key/abc"
    }
  }
  mock_resource "aws_lb" {
    defaults = {
      arn      = "arn:aws:elasticloadbalancing:ap-northeast-1:111122223333:loadbalancer/app/my/abc"
      dns_name = "my-123.ap-northeast-1.elb.amazonaws.com"
      zone_id  = "Z14GRHDCWA56QT"
    }
  }
  mock_resource "aws_lb_listener" {
    defaults = {
      arn = "arn:aws:elasticloadbalancing:ap-northeast-1:111122223333:listener/app/my/abc/def"
    }
  }
  mock_data "aws_ec2_managed_prefix_list" {
    defaults = {
      id      = "pl-cloudfront"
      entries = [{ cidr = "130.176.0.0/18", description = "" }]
    }
  }
  mock_resource "aws_cloudfront_distribution" {
    defaults = {
      domain_name    = "d111111abcdef8.cloudfront.net"
      hosted_zone_id = "Z2FDTNDAQYW2N"
    }
  }
  mock_resource "aws_lb_target_group" {
    defaults = {
      arn = "arn:aws:elasticloadbalancing:ap-northeast-1:111122223333:targetgroup/my/abc"
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
}

mock_provider "random" {}

variables {
  host               = "social.example.org"
  certificate_arn    = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc"
  bucket_name        = "social-example-org-media"
  availability_zones = ["ap-northeast-1a", "ap-northeast-1c"]
  assign_public_ip   = true

  mail_from          = "noreply@social.example.org"
  mail_smtp_host     = "smtp.example.org"
  mail_smtp_username = "my-smtp-user"
}

# By plan alone, as the other examples: prevent_destroy refuses the destroy that follows an apply, as it should.
run "composes_every_setting" {
  command = plan

  assert {
    condition = toset(keys(module.app.env)) == toset([
      "MY_BLOB",
      "MY_BLOB_S3_BUCKET",
      "MY_BLOB_S3_REGION",
      "MY_DATABASE",
      "MY_HOST",
      "MY_MAIL",
      "MY_MAIL_FROM",
      "MY_MAIL_SMTP_HOST",
      "MY_MAIL_SMTP_PORT",
      "MY_MAIL_SMTP_USERNAME",
      "MY_PROFILE",
      "MY_QUEUE",
      "MY_STREAM",
      "MY_STREAM_SNS_QUEUE_PREFIX",
      "MY_STREAM_SNS_TOPIC_ARN",
      "MY_TRUSTED_PROXIES",
      "MY_TRUSTED_PROXY_HEADER",
    ])
    error_message = "the settings must compose, and an ALB needs no origin secret"
  }

  assert {
    condition     = module.app.env["MY_QUEUE"] == "db" && module.app.env["MY_DATABASE"] == "postgres" && module.app.env["MY_MAIL"] == "smtp" && module.app.env["MY_STREAM"] == "sns"
    error_message = "the backends must be the example's"
  }

  assert {
    condition     = module.app.env["MY_TRUSTED_PROXIES"] == "10.0.0.0/20, 10.0.16.0/20"
    error_message = "the ranges of the load balancer's subnets are what the server trusts"
  }

  assert {
    condition     = toset(keys(module.app.secret_env)) == toset(["MY_DATABASE_URL", "MY_KEK", "MY_MAIL_SMTP_PASSWORD", "MY_VAPID_PRIVATE_KEY"])
    error_message = "every secret reaches the app, the two neutral blocks' among them"
  }

  assert {
    condition     = !module.network.nat && module.network.platform == "aws"
    error_message = "public addresses on the tasks leave no NAT gateway to pay for"
  }
}

# app-aws-ecs, ingress-aws-alb, and ingress-aws-cloudfront together: the load balancer is CloudFront's origin and admits nothing else.
run "cloudfront_in_front_of_the_load_balancer" {
  command = plan

  variables {
    cloudfront                 = true
    cloudfront_certificate_arn = "arn:aws:acm:us-east-1:111122223333:certificate/abc"
    origin_secret              = "0123456789abcdef0123456789abcdef"
    route53_zone_id            = "Z0123456789ABCDEFGHIJ"
  }

  assert {
    condition     = module.app.env["MY_TRUSTED_PROXIES"] == "10.0.0.0/20, 10.0.16.0/20, 130.176.0.0/18" && module.app.env["MY_TRUSTED_PROXY_HEADER"] == "x-forwarded-for"
    error_message = "the server must trust the load balancer and CloudFront, which each append to X-Forwarded-For"
  }

  assert {
    condition     = !contains(keys(module.app.env), "MY_ORIGIN_SECRET_HEADER") && !contains(keys(module.app.secret_env), "MY_ORIGIN_SECRET")
    error_message = "the load balancer checks the header, so the server checks none"
  }

  # What the load balancer admits is in the tests of ingress-aws-alb, which see its resources.
  assert {
    condition     = module.ingress.cloudfront_origin.domain_name == "my-123.ap-northeast-1.elb.amazonaws.com" && module.ingress.cloudfront_origin.origin_secret_header == "x-my-origin-secret"
    error_message = "CloudFront's origin is the load balancer, with the header that it checks"
  }

  assert {
    condition     = length(module.ingress.dns_records) == 0 && length(output.dns_records) == 0 && output.domain_name == "d111111abcdef8.cloudfront.net"
    error_message = "host points at the distribution, whose records the zone gets, not at the load balancer"
  }
}

run "tasks_with_no_public_address_get_a_nat_gateway" {
  command = plan

  variables {
    assign_public_ip = false
  }

  assert {
    condition     = module.network.nat
    error_message = "private tasks need the way out of the network"
  }
}

# The same blocks with one replaced at a time, as the comments of the example say.
run "the_kek_swaps_to_kms_in_one_block" {
  command = plan

  module {
    source = "../../tests/aws-ecs-swap"
  }

  variables {
    kek = "kms"
  }

  assert {
    condition     = output.env["MY_KEY_MANAGER"] == "awskms" && !contains(keys(output.secret_env), "MY_KEK")
    error_message = "the KEK must move to KMS by the block alone"
  }
}

run "the_mail_swaps_to_ses_in_one_block" {
  command = plan

  module {
    source = "../../tests/aws-ecs-swap"
  }

  variables {
    mail = "ses"
  }

  assert {
    condition     = output.env["MY_MAIL"] == "ses" && !contains(keys(output.secret_env), "MY_MAIL_SMTP_PASSWORD")
    error_message = "the mail must move to SES by the block alone"
  }
}
