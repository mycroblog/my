# SPDX-License-Identifier: AGPL-3.0-only

# The example as it is composed, and every swap of a block that holds, on a mocked provider: nothing here reaches AWS. It shows
# that the configuration is consistent, not that it runs: read deploy/terraform/modules/app-aws-lambda/README.md.

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
  mock_data "aws_cloudfront_cache_policy" {
    defaults = {
      id = "cache-policy-id"
    }
  }
  mock_data "aws_cloudfront_origin_request_policy" {
    defaults = {
      id = "origin-request-policy-id"
    }
  }
  mock_data "aws_ip_ranges" {
    defaults = {
      cidr_blocks      = ["130.176.0.0/18"]
      ipv6_cidr_blocks = ["2600:9000::/28"]
    }
  }
  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::111122223333:role/my"
    }
  }
  mock_resource "aws_cloudwatch_log_group" {
    defaults = {
      arn = "arn:aws:logs:ap-northeast-1:111122223333:log-group:/aws/lambda/my"
    }
  }
  mock_resource "aws_secretsmanager_secret" {
    defaults = {
      arn = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:s-AbCdEf"
    }
  }
  mock_resource "aws_sqs_queue" {
    defaults = {
      arn = "arn:aws:sqs:ap-northeast-1:111122223333:my-jobs"
      url = "https://sqs.ap-northeast-1.amazonaws.com/111122223333/my-jobs"
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
  mock_resource "aws_lambda_function_url" {
    defaults = {
      function_url = "https://abc.lambda-url.ap-northeast-1.on.aws/"
    }
  }
  mock_resource "aws_cloudfront_distribution" {
    defaults = {
      domain_name = "d111111abcdef8.cloudfront.net"
    }
  }
}

mock_provider "random" {}

mock_provider "cloudflare" {
  mock_data "cloudflare_ip_ranges" {
    defaults = {
      ipv4_cidrs = ["173.245.48.0/20"]
      ipv6_cidrs = ["2400:cb00::/32"]
    }
  }
}

variables {
  host            = "social.example.org"
  image           = "111122223333.dkr.ecr.ap-northeast-1.amazonaws.com/my:0.1.0"
  certificate_arn = "arn:aws:acm:us-east-1:111122223333:certificate/abc"
  bucket_name     = "social-example-org-media"

  mail_from          = "noreply@social.example.org"
  mail_smtp_host     = "smtp.example.org"
  mail_smtp_username = "my-smtp-user"
}

# By plan alone, as the example of Google Cloud: prevent_destroy refuses the destroy that follows an apply, as it should. So what
# only an apply knows, such as the queue's URL, is checked by name.
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
      "MY_MEDIA_MAX_AV_BYTES",
      "MY_MEDIA_MAX_IMAGE_BYTES",
      "MY_PROFILE",
      "MY_QUEUE",
      "MY_QUEUE_SQS_QUEUE_URL",
      "MY_STREAM",
      "MY_STREAM_SNS_QUEUE_PREFIX",
      "MY_STREAM_SNS_TOPIC_ARN",
      "MY_TRUSTED_PROXIES",
      "MY_TRUSTED_PROXY_HEADER",
    ])
    error_message = "the settings must compose"
  }

  assert {
    condition     = module.app.env["MY_QUEUE"] == "sqs" && module.app.env["MY_DATABASE"] == "postgres" && module.app.env["MY_MAIL"] == "smtp" && module.app.env["MY_STREAM"] == "sns" && module.app.env["MY_BLOB"] == "s3"
    error_message = "the backends must be the example's"
  }

  assert {
    condition     = module.app.env["MY_TRUSTED_PROXIES"] == "127.0.0.1/32, 130.176.0.0/18, 2600:9000::/28"
    error_message = "the app adds the adapter to what CloudFront says"
  }

  assert {
    condition     = toset(keys(module.app.secret_env)) == toset(["MY_DATABASE_URL", "MY_KEK", "MY_MAIL_SMTP_PASSWORD", "MY_VAPID_PRIVATE_KEY"])
    error_message = "every secret reaches the app, the two neutral blocks' among them"
  }
}

# The same blocks with one replaced at a time, as the comments of the example say.
run "the_kek_swaps_to_kms_in_one_block" {
  command = plan

  module {
    source = "../../tests/aws-swap"
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
    source = "../../tests/aws-swap"
  }

  variables {
    mail = "ses"
  }

  assert {
    condition     = output.env["MY_MAIL"] == "ses" && !contains(keys(output.secret_env), "MY_MAIL_SMTP_PASSWORD")
    error_message = "the mail must move to SES by the block alone"
  }
}

run "the_ingress_swaps_to_cloudflare_in_one_block" {
  command = plan

  module {
    source = "../../tests/aws-swap"
  }

  variables {
    ingress = "cloudflare"
  }

  assert {
    condition     = output.env["MY_TRUSTED_PROXIES"] == "127.0.0.1/32, 173.245.48.0/20, 2400:cb00::/32" && output.env["MY_TRUSTED_PROXY_HEADER"] == "x-forwarded-for"
    error_message = "Cloudflare's block must carry over with the adapter in the place of Cloud Run's front end"
  }
}

run "the_ingress_can_be_left_out" {
  command = plan

  module {
    source = "../../tests/aws-swap"
  }

  variables {
    ingress = "none"
  }

  assert {
    condition     = output.env["MY_TRUSTED_PROXIES"] == "127.0.0.1/32" && !contains(keys(output.env), "MY_TRUSTED_PROXY_HEADER")
    error_message = "without an ingress only the adapter is trusted"
  }
}
