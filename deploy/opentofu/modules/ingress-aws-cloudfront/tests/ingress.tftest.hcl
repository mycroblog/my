# SPDX-License-Identifier: AGPL-3.0-only

# On a mocked provider: nothing here reaches AWS.

mock_provider "aws" {
  mock_resource "aws_cloudfront_distribution" {
    defaults = {
      domain_name    = "d111111abcdef8.cloudfront.net"
      hosted_zone_id = "Z2FDTNDAQYW2N"
    }
  }
}

variables {
  host            = "social.example.org"
  certificate_arn = "arn:aws:acm:us-east-1:111122223333:certificate/abc"

  origin = {
    domain_name          = "my-123.ap-northeast-1.elb.amazonaws.com"
    origin_secret_header = "x-my-origin-secret"
    idle_timeout         = 60
  }
  origin_secret = "0123456789abcdef0123456789abcdef"
}

# A mock cannot set the id of these two lookups, which the provider's schema does not mark computed, so the tests check the names.
run "it_serves_host_with_no_cache" {
  assert {
    condition     = contains(aws_cloudfront_distribution.this.aliases, "social.example.org") && data.aws_cloudfront_cache_policy.caching_disabled.name == "Managed-CachingDisabled"
    error_message = "the API, ActivityPub, and HTML must never be cached"
  }

  assert {
    condition     = data.aws_cloudfront_origin_request_policy.all_viewer.name == "Managed-AllViewer"
    error_message = "the viewer's Host must reach the load balancer, whose certificate is for host, and the server"
  }

  assert {
    condition     = !aws_cloudfront_distribution.this.default_cache_behavior[0].compress
    error_message = "compression would hold back the events of a stream"
  }

  assert {
    condition     = one(output.dns_records).type == "CNAME" && one(output.dns_records).rrdata == "d111111abcdef8.cloudfront.net"
    error_message = "the host must point at the distribution"
  }
}

run "its_origin_is_the_load_balancer_with_the_header" {
  command = plan

  assert {
    condition     = one(aws_cloudfront_distribution.this.origin).domain_name == "my-123.ap-northeast-1.elb.amazonaws.com"
    error_message = "the origin is the load balancer"
  }

  assert {
    condition     = nonsensitive(one(one(aws_cloudfront_distribution.this.origin).custom_header).name) == "x-my-origin-secret"
    error_message = "CloudFront must send the header that the load balancer checks"
  }

  assert {
    condition = alltrue([
      for o in aws_cloudfront_distribution.this.origin : (
        o.custom_origin_config[0].origin_protocol_policy == "https-only" && o.custom_origin_config[0].origin_keepalive_timeout < 60 && o.custom_origin_config[0].origin_read_timeout > 30
      )
    ])
    error_message = "HTTPS only, a keepalive below the load balancer's idle timeout, and a read timeout above a stream's heartbeat"
  }
}

run "media_is_cached_by_the_servers_cache_control" {
  command = plan

  assert {
    condition     = one(aws_cloudfront_distribution.this.ordered_cache_behavior).path_pattern == "/media/*" && one(aws_cloudfront_distribution.this.ordered_cache_behavior).allowed_methods == toset(["GET", "HEAD"])
    error_message = "/media/ and /media/proxy/ are cached, and only read"
  }

  assert {
    condition     = aws_cloudfront_cache_policy.media.default_ttl == 0 && aws_cloudfront_cache_policy.media.max_ttl == 31536000
    error_message = "the server's Cache-Control decides, up to the year of local media, and nothing without one is kept"
  }

  assert {
    condition     = aws_cloudfront_cache_policy.media.parameters_in_cache_key_and_forwarded_to_origin[0].query_strings_config[0].query_string_behavior == "all"
    error_message = "a proxy URL's signature is in its query, so the query must be in the key"
  }

  assert {
    condition     = aws_cloudfront_origin_request_policy.media.headers_config[0].headers[0].items == toset(["host"]) && aws_cloudfront_origin_request_policy.media.cookies_config[0].cookie_behavior == "none"
    error_message = "media forwards Host for the certificate's check, and no cookie"
  }
}

run "a_keepalive_beyond_the_load_balancers_idle_timeout_fails" {
  command = plan

  variables {
    origin = {
      domain_name          = "my-123.ap-northeast-1.elb.amazonaws.com"
      origin_secret_header = "x-my-origin-secret"
      idle_timeout         = 31
    }
    origin_keepalive_timeout = 45
  }

  expect_failures = [aws_cloudfront_distribution.this]
}

run "a_read_timeout_at_the_heartbeat_fails" {
  command = plan

  variables {
    origin_read_timeout = 30
  }

  expect_failures = [var.origin_read_timeout]
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
