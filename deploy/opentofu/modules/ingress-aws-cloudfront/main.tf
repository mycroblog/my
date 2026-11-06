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
  description = "Host name of the origin. It may come from the app module: env, trusted_proxies, and service_ingress depend on nothing that reads it."
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

variable "origin_secret" {
  description = "Value of the header that shows the server a request came through CloudFront. THE PROVIDER HAS NO WRITE-ONLY FORM OF A CUSTOM ORIGIN HEADER, SO IT LANDS IN THE STATE: keep the state private. Pass it with TF_VAR_origin_secret, and put the same value in the secret of origin_secret_id."
  type        = string
  sensitive   = true
  nullable    = false

  validation {
    condition     = can(regex("^[!-~]{32,}$", var.origin_secret))
    error_message = "origin_secret must be at least 32 bytes of visible ASCII with no spaces: openssl rand -hex 32."
  }
}

variable "origin_secret_id" {
  description = "ARN of the same value in Secrets Manager, which the server reads as MY_ORIGIN_SECRET."
  type        = string
  nullable    = false
}

variable "origin_secret_header" {
  description = "Name of that header, which nothing else uses."
  type        = string
  default     = "x-my-origin-secret"
  nullable    = false

  validation {
    condition     = can(regex("^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$", var.origin_secret_header)) && !can(regex("(?i)^(cf-|x-cf-|x-forwarded-|x-amz|x-edge-|x-goog-|sec-|proxy-|if-|access-control-)", var.origin_secret_header))
    error_message = "origin_secret_header must hold letters, digits, and inner hyphens, and must not be a name that a CDN or a browser sets, such as x-forwarded-*. my config check refuses a few more, such as authorization."
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
  origin_id = "origin"
}

# AWS's managed policies, by name. The API prefixes the names that AWS's guide lists with Managed-.
data "aws_cloudfront_cache_policy" "caching_disabled" {
  name = "Managed-CachingDisabled"
}

# Everything but the viewer's Host, since the origin routes by its own Host.
data "aws_cloudfront_origin_request_policy" "all_viewer_except_host" {
  name = "Managed-AllViewerExceptHostHeader"
}

# CLOUDFRONT_ORIGIN_FACING rather than CLOUDFRONT: those are what connects to an origin.
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

    # It replaces what a viewer sent, and the server refuses a request without it.
    custom_header {
      name  = var.origin_secret_header
      value = var.origin_secret
    }

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

output "env" {
  description = "The header that the proxies write, and the name of the one that shows the server a request came through CloudFront."
  value = {
    MY_TRUSTED_PROXY_HEADER = "x-forwarded-for"
    MY_ORIGIN_SECRET_HEADER = var.origin_secret_header
  }
}

output "trusted_proxies" {
  description = "What connects to the origin from CloudFront. The app module adds the platform's own front end."
  value       = concat(data.aws_ip_ranges.origin_facing.cidr_blocks, data.aws_ip_ranges.origin_facing.ipv6_cidr_blocks)
}

output "service_ingress" {
  description = "All: the server refuses what lacks the header. Read the README."
  value       = "all"
}

output "secret_env" {
  description = "The secret that the server checks, by reference."
  value       = { MY_ORIGIN_SECRET = var.origin_secret_id }
}

output "permissions" {
  description = "Nothing."
  value       = []
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
