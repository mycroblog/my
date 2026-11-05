# SPDX-License-Identifier: AGPL-3.0-only

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

variable "origin_host" {
  description = "Host name of the Function URL. It may come from the app module: env, trusted_proxies, and service_ingress depend on nothing that reads it."
  type        = string
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

variable "origin_read_timeout" {
  description = "Seconds CloudFront waits for bytes from the origin, from 1 to 120 without a quota increase. Streams send a heartbeat every 30 seconds."
  type        = number
  default     = 120
  nullable    = false

  validation {
    condition     = var.origin_read_timeout >= 1 && var.origin_read_timeout <= 120
    error_message = "origin_read_timeout must be from 1 to 120 without a quota increase."
  }
}

variable "price_class" {
  description = "Which edge locations serve the host."
  type        = string
  default     = "PriceClass_All"
  nullable    = false
}

locals {
  origin_id = "function-url"
}

# AWS's managed policies, by name. The API prefixes the names that AWS's guide lists with Managed-.
data "aws_cloudfront_cache_policy" "caching_disabled" {
  name = "Managed-CachingDisabled"
}

# Everything but the viewer's Host, since a Function URL routes by Host.
data "aws_cloudfront_origin_request_policy" "all_viewer_except_host" {
  name = "Managed-AllViewerExceptHostHeader"
}

# CLOUDFRONT_ORIGIN_FACING rather than CLOUDFRONT: those are what connects to an origin, and the full list does not fit in the
# 4 KB of Lambda's environment.
data "aws_ip_ranges" "origin_facing" {
  services = ["cloudfront_origin_facing"]
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
    domain_name = var.origin_host

    custom_origin_config {
      http_port              = 80
      https_port             = 443
      origin_protocol_policy = "https-only"
      origin_ssl_protocols   = ["TLSv1.2"]
      origin_read_timeout    = var.origin_read_timeout
    }
  }

  # No cache: the API, ActivityPub, and HTML must never be cached. No origin access control: it would make every client send
  # the SHA-256 of a POST body, which a peer does not.
  default_cache_behavior {
    target_origin_id         = local.origin_id
    viewer_protocol_policy   = "redirect-to-https"
    allowed_methods          = ["GET", "HEAD", "OPTIONS", "PUT", "POST", "PATCH", "DELETE"]
    cached_methods           = ["GET", "HEAD"]
    cache_policy_id          = data.aws_cloudfront_cache_policy.caching_disabled.id
    origin_request_policy_id = data.aws_cloudfront_origin_request_policy.all_viewer_except_host.id
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
}

output "env" {
  description = "The header that the proxies write."
  value       = { MY_TRUSTED_PROXY_HEADER = "x-forwarded-for" }
}

output "trusted_proxies" {
  description = "What connects to the Function URL from CloudFront. The app module adds the platform's own front end."
  value       = concat(data.aws_ip_ranges.origin_facing.cidr_blocks, data.aws_ip_ranges.origin_facing.ipv6_cidr_blocks)
}

output "service_ingress" {
  description = "Nothing asked of the platform: nothing can limit a Function URL to CloudFront for this server. Read the README."
  value       = null
}

output "secret_env" {
  description = "No secrets."
  value       = {}
}

output "roles" {
  description = "Nothing."
  value       = []
}

output "dns_records" {
  description = "What to add at the DNS provider."
  value = [
    { name = var.host, type = "CNAME", rrdata = aws_cloudfront_distribution.this.domain_name },
  ]
}
