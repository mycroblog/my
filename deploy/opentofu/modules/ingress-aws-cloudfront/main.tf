# SPDX-License-Identifier: AGPL-3.0-only

# CloudFront at host, in front of the load balancer of ingress-aws-alb with behind_cloudfront. It is no backend of the app: the
# load balancer's module names what the server trusts. This module has never been applied: read the README.

terraform {
  required_version = ">= 1.11"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.0, < 7.0"
    }
  }
}

variable "host" {
  description = "Domain of the server."
  type        = string
  nullable    = false
}

variable "name" {
  description = "Start of the names of the media's cache and origin request policies, which are unique in the account."
  type        = string
  default     = "my"
  nullable    = false

  validation {
    condition     = can(regex("^[A-Za-z0-9_-]{1,100}$", var.name))
    error_message = "name must be 1 to 100 letters, digits, hyphens, and underscores."
  }
}

variable "origin" {
  description = "The load balancer, from cloudfront_origin of ingress-aws-alb with behind_cloudfront."
  type = object({
    domain_name          = string
    origin_secret_header = string
    idle_timeout         = number
  })
  nullable = false
}

variable "origin_secret" {
  description = "The value of the header that the load balancer checks, from origin_secret of ingress-aws-alb. THE PROVIDER HAS NO WRITE-ONLY FORM OF A CUSTOM ORIGIN HEADER, SO IT LANDS IN THE STATE: keep the state private."
  type        = string
  sensitive   = true
  nullable    = false
}

variable "certificate_arn" {
  description = "ARN of a validated certificate for host in AWS Certificate Manager, in us-east-1, whatever the region of the rest. Create it by hand first: validating one here would hold the apply until the records this module prints are in DNS."
  type        = string
  nullable    = false

  validation {
    condition     = can(regex("^arn:[a-z-]+:acm:us-east-1:[0-9]{12}:certificate/.+$", var.certificate_arn))
    error_message = "certificate_arn must be a certificate in us-east-1, the only region CloudFront takes one from."
  }
}

variable "route53_zone_id" {
  description = "Public Route 53 hosted zone of host. Set, the module creates alias records, which work at a zone apex and on a subdomain. Null, you create the records that dns_records lists."
  type        = string
  default     = null
}

variable "host_is_apex" {
  description = "Whether host is the apex of its DNS zone, which decides what dns_records asks for while route53_zone_id is null. An input, since the labels of a name cannot tell: social.example.org and example.co.uk both have three."
  type        = bool
  default     = false
  nullable    = false
}

variable "origin_read_timeout" {
  description = "Seconds CloudFront waits for bytes from the load balancer, above the 30 seconds between a stream's heartbeats and up to 120 without a quota increase. The idle timeout of the load balancer applies too, and the shorter ends a silent response."
  type        = number
  default     = 60
  nullable    = false

  validation {
    condition     = var.origin_read_timeout > 30 && var.origin_read_timeout <= 120
    error_message = "origin_read_timeout must be above 30, a stream's heartbeat, and at most 120 without a quota increase."
  }
}

variable "origin_keepalive_timeout" {
  description = "Seconds CloudFront keeps an idle connection to the load balancer for the next request. Below the load balancer's idle timeout, so that CloudFront never sends on a connection the load balancer is closing."
  type        = number
  default     = 5
  nullable    = false

  validation {
    condition     = var.origin_keepalive_timeout >= 1 && var.origin_keepalive_timeout <= 60
    error_message = "origin_keepalive_timeout must be from 1 to 60 without a quota increase."
  }
}

variable "price_class" {
  description = "Which edge locations serve the host."
  type        = string
  default     = "PriceClass_All"
  nullable    = false
}

locals {
  origin_id = "load-balancer"
}

# AWS's managed policies, by name. The API prefixes the names that AWS's guide lists with Managed-.
data "aws_cloudfront_cache_policy" "caching_disabled" {
  name = "Managed-CachingDisabled"
}

# Everything of the viewer, Host among it: CloudFront checks the load balancer's certificate against the Host it forwards, and
# the server sees the host the viewer asked for.
data "aws_cloudfront_origin_request_policy" "all_viewer" {
  name = "Managed-AllViewer"
}

# Media: the server's Cache-Control decides, and the query is in the key, since a proxy URL carries its signature there. Nothing
# without a Cache-Control is kept.
resource "aws_cloudfront_cache_policy" "media" {
  name        = "${var.name}-media"
  comment     = "Media of ${var.host}"
  min_ttl     = 0
  default_ttl = 0
  max_ttl     = 31536000

  parameters_in_cache_key_and_forwarded_to_origin {
    enable_accept_encoding_gzip   = false
    enable_accept_encoding_brotli = false

    cookies_config {
      cookie_behavior = "none"
    }
    headers_config {
      header_behavior = "none"
    }
    query_strings_config {
      query_string_behavior = "all"
    }
  }
}

# Host alone beside the key, for the certificate's check as above. No cookie: media needs none.
resource "aws_cloudfront_origin_request_policy" "media" {
  name    = "${var.name}-media"
  comment = "Media of ${var.host}"

  cookies_config {
    cookie_behavior = "none"
  }
  headers_config {
    header_behavior = "whitelist"
    headers {
      items = ["host"]
    }
  }
  query_strings_config {
    query_string_behavior = "all"
  }
}

resource "aws_cloudfront_distribution" "this" {
  enabled         = true
  aliases         = [var.host]
  comment         = "Traffic of ${var.host}"
  is_ipv6_enabled = true
  http_version    = "http2and3"
  price_class     = var.price_class

  origin {
    origin_id   = local.origin_id
    domain_name = var.origin.domain_name

    # It replaces what a viewer sent, and the load balancer forwards nothing without it.
    custom_header {
      name  = var.origin.origin_secret_header
      value = var.origin_secret
    }

    custom_origin_config {
      http_port                = 80
      https_port               = 443
      origin_protocol_policy   = "https-only"
      origin_ssl_protocols     = ["TLSv1.2"]
      origin_read_timeout      = var.origin_read_timeout
      origin_keepalive_timeout = var.origin_keepalive_timeout
    }
  }

  # No cache: the API, ActivityPub, and HTML must never be cached. No compression, which would hold back the events of a stream.
  default_cache_behavior {
    target_origin_id         = local.origin_id
    viewer_protocol_policy   = "redirect-to-https"
    allowed_methods          = ["GET", "HEAD", "OPTIONS", "PUT", "POST", "PATCH", "DELETE"]
    cached_methods           = ["GET", "HEAD"]
    cache_policy_id          = data.aws_cloudfront_cache_policy.caching_disabled.id
    origin_request_policy_id = data.aws_cloudfront_origin_request_policy.all_viewer.id
    compress                 = false
  }

  # /media/ and /media/proxy/, which the server serves with a Cache-Control of their own.
  ordered_cache_behavior {
    path_pattern             = "/media/*"
    target_origin_id         = local.origin_id
    viewer_protocol_policy   = "redirect-to-https"
    allowed_methods          = ["GET", "HEAD"]
    cached_methods           = ["GET", "HEAD"]
    cache_policy_id          = aws_cloudfront_cache_policy.media.id
    origin_request_policy_id = aws_cloudfront_origin_request_policy.media.id
    compress                 = false
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    acm_certificate_arn      = var.certificate_arn
    ssl_support_method       = "sni-only"
    minimum_protocol_version = "TLSv1.2_2021"
  }

  lifecycle {
    precondition {
      condition     = var.origin_keepalive_timeout < var.origin.idle_timeout
      error_message = "origin_keepalive_timeout must be below ${var.origin.idle_timeout}, the idle timeout of the load balancer."
    }
  }
}

# Alias records answer with the distribution's addresses, so no CNAME is involved and an apex is fine. IPv4 and IPv6 each need
# one, since the distribution enables IPv6.
resource "aws_route53_record" "alias" {
  for_each = var.route53_zone_id == null ? toset([]) : toset(["A", "AAAA"])

  zone_id = var.route53_zone_id
  name    = var.host
  type    = each.value

  alias {
    name                   = aws_cloudfront_distribution.this.domain_name
    zone_id                = aws_cloudfront_distribution.this.hosted_zone_id
    evaluate_target_health = false
  }
}

output "platform" {
  description = "The platform of this module: it works only with an app of AWS."
  value       = "aws"
}

output "dns_records" {
  description = "What to add at the DNS provider: nothing with route53_zone_id, a CNAME for a subdomain, and for an apex an ALIAS or ANAME, the name depending on the provider, since DNS allows no CNAME there."
  value = var.route53_zone_id != null ? [] : [
    { name = var.host, type = var.host_is_apex ? "ALIAS" : "CNAME", rrdata = aws_cloudfront_distribution.this.domain_name },
  ]
}

output "dns_note" {
  description = "What dns_records cannot say, or null."
  value       = var.route53_zone_id == null && var.host_is_apex ? "A plain CNAME cannot be used at a zone apex. Create an ALIAS or ANAME record, or a flattened CNAME, at your DNS provider. Not every provider has one: without one, move the zone to a provider that does, such as Route 53 with route53_zone_id." : null
}

output "domain_name" {
  description = "Domain name of the distribution, for records made elsewhere."
  value       = aws_cloudfront_distribution.this.domain_name
}

output "hosted_zone_id" {
  description = "Route 53 hosted zone of the distribution, for an alias record made elsewhere."
  value       = aws_cloudfront_distribution.this.hosted_zone_id
}
