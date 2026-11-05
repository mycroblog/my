# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches AWS.

mock_provider "aws" {
  mock_data "aws_ip_ranges" {
    defaults = {
      cidr_blocks      = ["130.176.0.0/18"]
      ipv6_cidr_blocks = ["2600:9000::/28"]
    }
  }
  mock_resource "aws_cloudfront_distribution" {
    defaults = {
      domain_name = "d111111abcdef8.cloudfront.net"
    }
  }
}

variables {
  host            = "social.example.org"
  origin_host     = "abc.lambda-url.ap-northeast-1.on.aws"
  certificate_arn = "arn:aws:acm:us-east-1:111122223333:certificate/abc"
}

run "it_serves_host_with_no_cache" {
  assert {
    condition     = contains(aws_cloudfront_distribution.this.aliases, "social.example.org") && aws_cloudfront_distribution.this.default_cache_behavior[0].cache_policy_id == "4135ea2d-6df8-44a3-9df3-4b5a84be39ad"
    error_message = "the API, ActivityPub, and HTML must never be cached"
  }

  assert {
    condition     = aws_cloudfront_distribution.this.default_cache_behavior[0].origin_request_policy_id == "b689b0a8-53d0-40ab-baf2-68738e2966ac"
    error_message = "a Function URL routes by Host, so the viewer's must not reach it"
  }

  assert {
    condition     = alltrue([for o in aws_cloudfront_distribution.this.origin : o.origin_access_control_id == null || o.origin_access_control_id == ""])
    error_message = "an origin access control would stop federation: a peer does not send the SHA-256 of a POST body"
  }

  assert {
    condition     = output.trusted_proxies == tolist(["130.176.0.0/18", "2600:9000::/28"]) && output.service_ingress == null
    error_message = "it says what connects, and asks nothing of the platform"
  }

  assert {
    condition     = one(output.dns_records).type == "CNAME" && one(output.dns_records).rrdata == "d111111abcdef8.cloudfront.net"
    error_message = "the host must point at the distribution"
  }
}

run "a_certificate_elsewhere_fails" {
  command = plan

  variables {
    certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc"
  }

  expect_failures = [var.certificate_arn]
}
