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
      domain_name    = "d111111abcdef8.cloudfront.net"
      hosted_zone_id = "Z2FDTNDAQYW2N"
    }
  }
}

variables {
  host            = "social.example.org"
  origin_host     = "abc.lambda-url.ap-northeast-1.on.aws"
  certificate_arn = "arn:aws:acm:us-east-1:111122223333:certificate/abc"

  origin_secret    = "0123456789abcdef0123456789abcdef"
  origin_secret_id = "arn:aws:secretsmanager:ap-northeast-1:111122223333:secret:origin-AbCdEf"
}

# A mock cannot set the id of these two lookups, which the provider's schema does not mark computed, so the tests check the names.
run "it_serves_host_with_no_cache" {
  assert {
    condition     = contains(aws_cloudfront_distribution.this.aliases, "social.example.org") && data.aws_cloudfront_cache_policy.caching_disabled.name == "Managed-CachingDisabled"
    error_message = "the API, ActivityPub, and HTML must never be cached"
  }

  assert {
    condition     = data.aws_cloudfront_origin_request_policy.all_viewer_except_host.name == "Managed-AllViewerExceptHostHeader"
    error_message = "a Function URL routes by Host, so the viewer's must not reach it"
  }

  assert {
    condition     = alltrue([for o in aws_cloudfront_distribution.this.origin : o.origin_access_control_id == null || o.origin_access_control_id == ""])
    error_message = "an origin access control would stop federation: a peer does not send the SHA-256 of a POST body"
  }

  assert {
    condition     = output.trusted_proxies == tolist(["130.176.0.0/18", "2600:9000::/28"]) && output.service_ingress == "all"
    error_message = "it says what connects, and that the service is open"
  }

  assert {
    condition     = one(output.dns_records).type == "CNAME" && one(output.dns_records).rrdata == "d111111abcdef8.cloudfront.net"
    error_message = "the host must point at the distribution"
  }
}

run "the_origin_secret_goes_by_reference_and_by_header" {
  command = plan

  assert {
    condition     = output.env["MY_ORIGIN_SECRET_HEADER"] == "x-my-origin-secret" && output.secret_env["MY_ORIGIN_SECRET"] == var.origin_secret_id
    error_message = "the server takes the header's name, and the secret by reference"
  }

  assert {
    condition     = nonsensitive(one(one(aws_cloudfront_distribution.this.origin).custom_header).name) == "x-my-origin-secret"
    error_message = "CloudFront must send the header"
  }
}

run "a_short_secret_fails" {
  command = plan

  variables {
    origin_secret = "short"
  }

  expect_failures = [var.origin_secret]
}

run "a_subdomain_elsewhere_asks_for_a_cname" {
  assert {
    condition     = one(output.dns_records).name == "social.example.org" && one(output.dns_records).type == "CNAME" && output.dns_note == null && length(aws_route53_record.alias) == 0
    error_message = "without a hosted zone the module creates nothing, and a subdomain takes a CNAME"
  }

  assert {
    condition     = output.domain_name == "d111111abcdef8.cloudfront.net" && output.hosted_zone_id == "Z2FDTNDAQYW2N"
    error_message = "the raw values are for records made elsewhere"
  }
}

run "an_apex_elsewhere_asks_for_an_alias_and_says_why" {
  variables {
    host         = "example.co.uk"
    host_is_apex = true
  }

  assert {
    condition     = one(output.dns_records).type == "ALIAS" && one(output.dns_records).rrdata == "d111111abcdef8.cloudfront.net" && strcontains(output.dns_note, "Not every provider") && length(aws_route53_record.alias) == 0
    error_message = "a plain CNAME cannot be used at an apex"
  }
}

run "a_hosted_zone_gets_alias_records" {
  variables {
    host            = "example.co.uk"
    host_is_apex    = true
    route53_zone_id = "Z0123456789ABCDEFGHIJ"
  }

  assert {
    condition     = toset(keys(aws_route53_record.alias)) == toset(["A", "AAAA"]) && aws_cloudfront_distribution.this.is_ipv6_enabled
    error_message = "IPv4 and IPv6 each need an alias record"
  }

  assert {
    condition = alltrue([
      for r in aws_route53_record.alias : (
        r.zone_id == "Z0123456789ABCDEFGHIJ" && r.name == "example.co.uk" && r.records == null &&
        r.alias[0].name == "d111111abcdef8.cloudfront.net" && r.alias[0].zone_id == "Z2FDTNDAQYW2N" && !r.alias[0].evaluate_target_health
      )
    ])
    error_message = "the records must alias the distribution from the operator's zone"
  }

  assert {
    condition     = length(output.dns_records) == 0 && output.dns_note == null
    error_message = "nothing is left to create elsewhere"
  }
}

run "a_certificate_elsewhere_fails" {
  command = plan

  variables {
    certificate_arn = "arn:aws:acm:ap-northeast-1:111122223333:certificate/abc"
  }

  expect_failures = [var.certificate_arn]
}
