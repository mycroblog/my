# SPDX-License-Identifier: AGPL-3.0-only

# Traffic reaches the app through Cloudflare: a proxied DNS record of host, with the cache bypassed for everything, because
# the API, ActivityPub, and HTML must never be cached. It adds no challenge: peers and Cloud Tasks are not browsers. What
# stays open at the origin and what stays unverified is in the README.

terraform {
  required_version = ">= 1.11"

  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = ">= 5.0, < 6.0"
    }
  }
}

variable "zone_id" {
  description = "Cloudflare zone of host."
  type        = string
  nullable    = false
}

variable "host" {
  description = "Domain of the server."
  type        = string
  nullable    = false
}

variable "origin_host" {
  description = "Host name or IPv4 address Cloudflare connects to. It may come from the app module: env depends on nothing that reads it."
  type        = string
  nullable    = false
}

variable "rewrite_host_header" {
  description = "Sends origin_host as the Host header, for an origin that routes by Host, such as *.run.app. Cloudflare offers this on the Enterprise plan only. Leave it off when the origin accepts host itself: a domain mapping of host or a load balancer."
  type        = bool
  default     = false
  nullable    = false
}

variable "proxied" {
  description = "Proxies the record through Cloudflare. A domain mapping behind it may need this off until its certificate is issued."
  type        = bool
  default     = true
  nullable    = false
}

variable "extra_rules" {
  description = "The rules output of ingress-cloudflare-media in this zone. A zone has one ruleset per phase, so the two modules share them."
  type = object({
    cache    = optional(any, [])
    config   = optional(any, [])
    origin   = optional(any, [])
    firewall = optional(any, [])
  })
  default  = {}
  nullable = false
}

locals {
  host_match = "http.host eq \"${var.host}\""

  cache_rules = concat([
    {
      description       = "Never cache the API, ActivityPub, or HTML"
      expression        = local.host_match
      action            = "set_cache_settings"
      action_parameters = { cache = false }
    },
  ], var.extra_rules.cache)

  # Strict, since Flexible would talk to the origin over HTTP, which Cloud Run answers with a redirect to itself. No Browser
  # Integrity Check, since federation peers and Cloud Tasks do not send what it expects of a browser.
  config_rules = concat([
    {
      description       = "Verify the origin, and let non-browsers through"
      expression        = local.host_match
      action            = "set_config"
      action_parameters = { ssl = "strict", bic = false }
    },
  ], var.extra_rules.config)

  origin_rules = concat(var.rewrite_host_header ? [
    {
      description       = "The origin routes by Host"
      expression        = local.host_match
      action            = "route"
      action_parameters = { host_header = var.origin_host }
    },
  ] : [], var.extra_rules.origin)
}

data "cloudflare_ip_ranges" "this" {}

resource "cloudflare_dns_record" "host" {
  zone_id = var.zone_id
  name    = var.host
  type    = can(cidrhost("${var.origin_host}/32", 0)) ? "A" : "CNAME"
  content = var.origin_host
  proxied = var.proxied
  # Automatic, as a proxied record requires.
  ttl = 1

  lifecycle {
    precondition {
      condition     = var.rewrite_host_header || !(endswith(var.origin_host, ".run.app") || endswith(var.origin_host, ".on.aws"))
      error_message = "A *.run.app or *.on.aws origin routes by Host, so it needs rewrite_host_header, which Cloudflare offers on the Enterprise plan only. On other plans, use an origin that accepts host: read the README."
    }
  }
}

resource "cloudflare_ruleset" "cache" {
  zone_id = var.zone_id
  name    = "my cache"
  kind    = "zone"
  phase   = "http_request_cache_settings"
  rules   = local.cache_rules
}

resource "cloudflare_ruleset" "config" {
  zone_id = var.zone_id
  name    = "my configuration"
  kind    = "zone"
  phase   = "http_config_settings"
  rules   = local.config_rules
}

resource "cloudflare_ruleset" "origin" {
  zone_id = var.zone_id
  name    = "my origin"
  kind    = "zone"
  phase   = "http_request_origin"
  rules   = local.origin_rules

  lifecycle {
    enabled = length(local.origin_rules) > 0
  }
}

resource "cloudflare_ruleset" "firewall" {
  zone_id = var.zone_id
  name    = "my firewall"
  kind    = "zone"
  phase   = "http_request_firewall_custom"
  rules   = var.extra_rules.firewall

  lifecycle {
    enabled = length(var.extra_rules.firewall) > 0
  }
}

output "env" {
  description = "The header that the proxies write."
  value       = { MY_TRUSTED_PROXY_HEADER = "x-forwarded-for" }
}

output "trusted_proxies" {
  description = "Cloudflare connects to the platform, whose own front end the app module adds. Walking X-Forwarded-For from the right past both reaches the client that connected to Cloudflare."
  value       = concat(data.cloudflare_ip_ranges.this.ipv4_cidrs, data.cloudflare_ip_ranges.this.ipv6_cidrs)
}

output "secret_env" {
  description = "No secrets."
  value       = {}
}

output "roles" {
  description = "Nothing."
  value       = []
}

output "service_ingress" {
  description = "Nothing asked of the platform, which leaves the service open: no platform can limit its service to Cloudflare. Read the README."
  value       = null
}

output "dns_records" {
  description = "None to add: this module creates the record."
  value       = []
}
